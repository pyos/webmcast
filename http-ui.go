// GET /<name>
//     Open a simple HTML5-based player with a stream-local chat.
//
// POST /user/new
//     Create a new user, duh.
//     Parameters: username string, password string, email string
//
// POST /user/login
//     Obtain a session cookie.
//     Parameters: username string, password string
//
// POST /user/restore
//     Request a password reset.
//     Parameters: username string OR email string
//
// GET /user/logout
//     Remove the session cookie.
//
// GET,POST /user/cfg
//     View/update the current user's data.
//     Parameters: password-old string,
//                 username, displayname, email, password, about string optional
//
// POST /user/new-token
//     Request a new stream token.
//
package main

import (
	"net/http"
	"strconv"
	"strings"
)

type UIHandler struct {
	*Context
}

func NewUIHandler(c *Context) UIHandler {
	return UIHandler{c}
}

func (ctx UIHandler) ServeHTTPUnsafe(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/" {
		if r.Method != "GET" {
			return RenderInvalidMethod(w, "GET")
		}
		auth, err := ctx.GetAuthInfo(r)
		if err != nil && err != ErrUserNotExist {
			return err
		}
		return Render(w, http.StatusOK, Landing{auth})
	}
	if !strings.ContainsRune(r.URL.Path[1:], '/') {
		return ctx.Player(w, r, r.URL.Path[1:])
	}
	if strings.HasPrefix(r.URL.Path, "/user/") {
		return ctx.UserControl(w, r, r.URL.Path[5:])
	}
	return RenderError(w, http.StatusNotFound, "")
}

func redirectBack(w http.ResponseWriter, r *http.Request, fallback string, code int) error {
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" && code == http.StatusSeeOther {
		w.WriteHeader(http.StatusNoContent)
		return nil
	} else {
		ref := r.Referer()
		if ref == "" {
			ref = fallback
		}
		http.Redirect(w, r, ref, code)
	}
	return nil
}

func (ctx UIHandler) Player(w http.ResponseWriter, r *http.Request, id string) error {
	if r.Method != "GET" {
		return RenderInvalidMethod(w, "GET")
	}

	auth, err := ctx.GetAuthInfo(r)
	if err != nil && err != ErrUserNotExist {
		return err
	}

	tpl := Room{ID: id, Owned: auth != nil && id == auth.Login, Online: true, User: auth}
	tpl.Meta, err = ctx.GetStreamMetadata(id)
	switch err {
	default:
		return err
	case ErrStreamNotExist:
		return RenderError(w, http.StatusNotFound, "Invalid stream name.")
	case ErrStreamOffline:
		tpl.Online = false
	case nil:
	}
	return Render(w, http.StatusOK, tpl)
}

func (ctx UIHandler) UserControl(w http.ResponseWriter, r *http.Request, path string) error {
	switch path {
	case "/new":
		switch r.Method {
		case "GET":
			return Render(w, http.StatusOK, UserNew(0))

		case "POST":
			username := strings.TrimSpace(r.FormValue("username"))
			password := r.FormValue("password")
			email := r.FormValue("email")

			switch user, err := ctx.NewUser(username, email, []byte(password)); err {
			case ErrInvalidUsername, ErrInvalidPassword, ErrInvalidEmail, ErrUserNotUnique:
				return RenderError(w, http.StatusBadRequest, err.Error())
			case ErrNotSupported:
				return RenderError(w, http.StatusNotImplemented, "Authentication is disabled.")
			case nil:
				if err = ctx.SetAuthInfo(w, user.ID); err != nil {
					return err
				}
				http.Redirect(w, r, "/user/cfg", http.StatusSeeOther)
				return nil
			default:
				return err
			}
		}
		return RenderInvalidMethod(w, "GET, POST")

	case "/login":
		switch r.Method {
		case "GET":
			_, err := ctx.GetAuthInfo(r)
			if err == ErrUserNotExist {
				return Render(w, http.StatusOK, UserLogin(0))
			}
			if err == nil {
				http.Redirect(w, r, "/user/cfg", http.StatusSeeOther)
			}
			return err

		case "POST":
			uid, err := ctx.GetUserID(r.FormValue("username"), []byte(r.FormValue("password")))
			if err == ErrUserNotExist {
				return RenderError(w, http.StatusForbidden, "Invalid username/password.")
			}
			if err = ctx.SetAuthInfo(w, uid); err != nil {
				return err
			}
			return redirectBack(w, r, "/", http.StatusSeeOther)
		}
		return RenderInvalidMethod(w, "GET, POST")

	case "/restore":
		switch r.Method {
		case "GET":
			return Render(w, http.StatusOK, UserRestore(0))

		case "POST":
			return RenderError(w, http.StatusNotImplemented, "There is no UI yet.")
		}
		return RenderInvalidMethod(w, "GET, POST")

	case "/logout": // TODO some protection against XSS?
		if r.Method != "GET" {
			return RenderInvalidMethod(w, "GET")
		}
		ctx.SetAuthInfo(w, -1) // should not fail
		return redirectBack(w, r, "/", http.StatusSeeOther)

	case "/cfg":
		switch r.Method {
		case "GET":
			user, err := ctx.GetAuthInfo(r)
			if err == ErrUserNotExist {
				http.Redirect(w, r, "/user/login", http.StatusSeeOther)
				return nil
			}
			if err != nil {
				return err
			}
			return Render(w, http.StatusOK, UserConfig{user})

		case "POST":
			//     Parameters: password-old string,
			//                 username, displayname, email, password, about string optional
			user, err := ctx.GetAuthInfo(r)
			if err == ErrUserNotExist {
				return RenderError(w, http.StatusForbidden, "Must be logged in.")
			}
			if err != nil {
				return err
			}
			switch err = user.CheckPassword([]byte(r.FormValue("password-old"))); err {
			default:
				return err
			case ErrUserNotExist:
				return RenderError(w, http.StatusForbidden, "Invalid old password.")
			case nil:
			}

			_, err = ctx.SetUserData(user.ID,
				r.FormValue("displayname"), r.FormValue("username"), r.FormValue("email"),
				r.FormValue("about"), []byte(r.FormValue("password")),
			)
			switch err {
			default:
				return err
			case ErrInvalidUsername, ErrInvalidPassword, ErrInvalidEmail, ErrUserNotUnique:
				return RenderError(w, http.StatusBadRequest, err.Error())
			case ErrStreamActive:
				return RenderError(w, http.StatusForbidden, "Stop streaming first.")
			case nil:
				return redirectBack(w, r, "/user/cfg", http.StatusSeeOther)
			}
		}
		return RenderInvalidMethod(w, "GET")

	case "/new-token":
		if r.Method != "POST" {
			return RenderInvalidMethod(w, "POST")
		}
		user, err := ctx.GetAuthInfo(r)
		if err == ErrUserNotExist {
			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
			return nil
		}
		if err != nil {
			return err
		}
		if err = ctx.NewStreamToken(user.ID); err != nil {
			return err
		}
		return redirectBack(w, r, "/user/cfg", http.StatusSeeOther)

	case "/activate":
		if r.Method != "GET" {
			return RenderInvalidMethod(w, "GET")
		}
		uid, err := strconv.ParseInt(r.FormValue("uid"), 10, 64)
		if err != nil {
			return RenderError(w, http.StatusBadRequest, "Invalid user ID.")
		}
		err = ctx.ActivateUser(uid, r.FormValue("token"))
		if err == ErrInvalidToken {
			return RenderError(w, http.StatusBadRequest, "Invalid activation token.")
		}
		if err != nil {
			return err
		}
		return redirectBack(w, r, "/user/cfg", http.StatusSeeOther)

	case "/set-stream-name", "/add-stream-panel", "/set-stream-panel", "/del-stream-panel":
		if r.Method != "POST" {
			return RenderInvalidMethod(w, "POST")
		}

		auth, err := ctx.GetAuthInfo(r)
		if err == ErrUserNotExist {
			return RenderError(w, http.StatusForbidden, "You own no streams.")
		}
		if err != nil {
			return err
		}

		switch path {
		case "/set-stream-panel":
			// TODO image
			if r.FormValue("id") != "" {
				id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
				if err != nil {
					return RenderError(w, http.StatusBadRequest, "Invalid panel id.")
				}
				err = ctx.SetStreamPanel(auth.ID, id, r.FormValue("value"))
			} else {
				err = ctx.AddStreamPanel(auth.ID, r.FormValue("value"))
			}
		case "/del-stream-panel":
			id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
			if err != nil {
				return RenderError(w, http.StatusBadRequest, "Invalid panel id.")
			}
			err = ctx.DelStreamPanel(auth.ID, id)
		default:
			err = ctx.SetStreamName(auth.ID, r.FormValue("value"))
		}

		if err == nil {
			w.WriteHeader(http.StatusNoContent)
		}
		return err

	}

	return RenderError(w, http.StatusNotFound, "")
}
