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
package ui

import (
	"net/http"
	"strconv"
	"strings"

	"../common"
	"../templates"
)

type HTTPContext struct {
	*common.Context
}

func NewHTTPContext(c *common.Context) *HTTPContext {
	return &HTTPContext{c}
}

func (ctx *HTTPContext) ServeHTTPUnsafe(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/" {
		if r.Method != "GET" {
			return templates.InvalidMethod(w, "GET")
		}
		auth, err := ctx.GetAuthInfo(r)
		if err != nil && err != common.ErrUserNotExist {
			return err
		}
		return templates.Page(w, http.StatusOK, templates.Landing{auth})
	}
	if !strings.ContainsRune(r.URL.Path[1:], '/') {
		return ctx.Player(w, r, r.URL.Path[1:])
	}
	if strings.HasPrefix(r.URL.Path, "/user/") {
		return ctx.UserControl(w, r, r.URL.Path[5:])
	}
	return templates.Error(w, http.StatusNotFound, "")
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

func (ctx *HTTPContext) Player(w http.ResponseWriter, r *http.Request, id string) error {
	if r.Method != "GET" {
		return templates.InvalidMethod(w, "GET")
	}

	auth, err := ctx.GetAuthInfo(r)
	if err != nil && err != common.ErrUserNotExist {
		return err
	}

	tpl := templates.Room{ID: id, Owned: auth != nil && id == auth.Login, Online: true, User: auth}
	tpl.Meta, err = ctx.GetStreamMetadata(id)
	switch err {
	default:
		return err
	case common.ErrStreamNotExist:
		return templates.Error(w, http.StatusNotFound, "Invalid stream name.")
	case common.ErrStreamOffline:
		tpl.Online = false
	case nil:
	}
	return templates.Page(w, http.StatusOK, tpl)
}

func (ctx *HTTPContext) UserControl(w http.ResponseWriter, r *http.Request, path string) error {
	switch path {
	case "/new":
		switch r.Method {
		case "GET":
			return templates.Page(w, http.StatusOK, templates.UserSignup(0))

		case "POST":
			username := strings.TrimSpace(r.FormValue("username"))
			password := r.FormValue("password")
			email := r.FormValue("email")

			switch user, err := ctx.NewUser(username, email, []byte(password)); err {
			case common.ErrInvalidUsername, common.ErrInvalidPassword, common.ErrInvalidEmail, common.ErrUserNotUnique:
				return templates.Error(w, http.StatusBadRequest, err.Error())
			case common.ErrNotSupported:
				return templates.Error(w, http.StatusNotImplemented, "Authentication is disabled.")
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
		return templates.InvalidMethod(w, "GET, POST")

	case "/login":
		switch r.Method {
		case "GET":
			_, err := ctx.GetAuthInfo(r)
			if err == common.ErrUserNotExist {
				return templates.Page(w, http.StatusOK, templates.UserLogin(0))
			}
			if err == nil {
				http.Redirect(w, r, "/user/cfg", http.StatusSeeOther)
			}
			return err

		case "POST":
			uid, err := ctx.GetUserID(r.FormValue("username"), []byte(r.FormValue("password")))
			if err == common.ErrUserNotExist {
				return templates.Error(w, http.StatusForbidden, "Invalid username/password.")
			}
			if err = ctx.SetAuthInfo(w, uid); err != nil {
				return err
			}
			return redirectBack(w, r, "/", http.StatusSeeOther)
		}
		return templates.InvalidMethod(w, "GET, POST")

	case "/restore":
		switch r.Method {
		case "GET":
			return templates.Page(w, http.StatusOK, templates.UserRestore(0))

		case "POST":
			return templates.Error(w, http.StatusNotImplemented, "There is no UI yet.")
		}
		return templates.InvalidMethod(w, "GET, POST")

	case "/logout": // TODO some protection against XSS?
		if r.Method != "GET" {
			return templates.InvalidMethod(w, "GET")
		}
		ctx.SetAuthInfo(w, -1) // should not fail
		return redirectBack(w, r, "/", http.StatusSeeOther)

	case "/cfg":
		switch r.Method {
		case "GET":
			user, err := ctx.GetAuthInfo(r)
			if err == common.ErrUserNotExist {
				http.Redirect(w, r, "/user/login", http.StatusSeeOther)
				return nil
			}
			if err != nil {
				return err
			}
			return templates.Page(w, http.StatusOK, templates.UserConfig{user})

		case "POST":
			//     Parameters: password-old string,
			//                 username, displayname, email, password, about string optional
			user, err := ctx.GetAuthInfo(r)
			if err == common.ErrUserNotExist {
				return templates.Error(w, http.StatusForbidden, "Must be logged in.")
			}
			if err != nil {
				return err
			}
			switch err = user.CheckPassword([]byte(r.FormValue("password-old"))); err {
			default:
				return err
			case common.ErrUserNotExist:
				return templates.Error(w, http.StatusForbidden, "Invalid old password.")
			case nil:
			}

			_, err = ctx.SetUserData(user.ID,
				r.FormValue("displayname"), r.FormValue("username"), r.FormValue("email"),
				r.FormValue("about"), []byte(r.FormValue("password")),
			)
			switch err {
			default:
				return err
			case common.ErrInvalidUsername, common.ErrInvalidPassword, common.ErrInvalidEmail, common.ErrUserNotUnique:
				return templates.Error(w, http.StatusBadRequest, err.Error())
			case common.ErrStreamActive:
				return templates.Error(w, http.StatusForbidden, "Stop streaming first.")
			case nil:
				return redirectBack(w, r, "/user/cfg", http.StatusSeeOther)
			}
		}
		return templates.InvalidMethod(w, "GET")

	case "/new-token":
		if r.Method != "POST" {
			return templates.InvalidMethod(w, "POST")
		}
		user, err := ctx.GetAuthInfo(r)
		if err == common.ErrUserNotExist {
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
			return templates.InvalidMethod(w, "GET")
		}
		uid, err := strconv.ParseInt(r.FormValue("uid"), 10, 64)
		if err != nil {
			return templates.Error(w, http.StatusBadRequest, "Invalid user ID.")
		}
		err = ctx.ActivateUser(uid, r.FormValue("token"))
		if err == common.ErrInvalidToken {
			return templates.Error(w, http.StatusBadRequest, "Invalid activation token.")
		}
		if err != nil {
			return err
		}
		return redirectBack(w, r, "/user/cfg", http.StatusSeeOther)

	case "/set-stream-name", "/add-stream-panel", "/set-stream-panel", "/del-stream-panel":
		if r.Method != "POST" {
			return templates.InvalidMethod(w, "POST")
		}

		auth, err := ctx.GetAuthInfo(r)
		if err == common.ErrUserNotExist {
			return templates.Error(w, http.StatusForbidden, "You own no streams.")
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
					return templates.Error(w, http.StatusBadRequest, "Invalid panel id.")
				}
				err = ctx.SetStreamPanel(auth.ID, id, r.FormValue("value"))
			} else {
				err = ctx.AddStreamPanel(auth.ID, r.FormValue("value"))
			}
		case "/del-stream-panel":
			id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
			if err != nil {
				return templates.Error(w, http.StatusBadRequest, "Invalid panel id.")
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

	return templates.Error(w, http.StatusNotFound, "")
}
