// GET /<name>
//     Open a simple HTML5-based player with a stream-local chat.
//
// GET /rec/<name>
//     View a list of previously recorded streams.
//
// GET /rec/<name>/<id>
//     Watch a particular recording in the HTML5 player.
//
// GET /user/
// POST /user/
//     >> password-old string, username, displayname, email, password, about optional[string]
//
// POST /user/new
//     >> username, password, email string
//
// POST /user/login
//     >> username, password string
//
// POST /user/restore
//     >> username string OR email string
//
// POST /user/restore?uid=int64&token=string
//     >> password string
//
// GET /user/logout
//
// POST /user/new-token
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

func redirectBack(w http.ResponseWriter, r *http.Request, fallback string, code int) error {
	ref := r.Referer()
	if ref == "" {
		ref = fallback
	}
	http.Redirect(w, r, ref, code)
	return nil
}

func (ctx UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	user, err := ctx.GetAuthInfo(r)
	if err != nil && err != ErrUserNotExist {
		return err
	}

	if r.URL.Path != "/" && !strings.ContainsRune(r.URL.Path[1:], '/') {
		if r.Method != "GET" {
			return RenderInvalidMethod(w, "GET")
		}

		id := r.URL.Path[1:]
		meta, err := ctx.GetStreamMetadata(id)
		switch err {
		default:
			return err
		case ErrStreamNotExist:
			return RenderError(w, http.StatusNotFound, "Invalid stream name.")
		case nil, ErrStreamOffline:
		}
		return Render(w, http.StatusOK, Room{ID: id, Editable: user != nil && meta.OwnerID == user.ID, Online: err == nil, Meta: meta, User: user})
	}

	if strings.HasPrefix(r.URL.Path, "/rec/") {
		id := r.URL.Path[5:]
		if sep := strings.IndexRune(id, '/'); sep != -1 {
			recid, err := strconv.ParseUint(id[sep+1:], 10, 63)
			if err != nil {
				return RenderError(w, http.StatusNotFound, "")
			}
			meta, err := ctx.GetRecording(id[:sep], int64(recid))
			if err == ErrStreamNotExist {
				return RenderError(w, http.StatusNotFound, "Recording not found.")
			}
			if err != nil {
				return err
			}
			return Render(w, http.StatusOK, Recording{ID: id[:sep], Meta: meta, User: user})
		}

		recs, err := ctx.GetRecordings(id)
		if err == nil {
			err = Render(w, http.StatusOK, Recordings{id, user != nil && recs.OwnerID == user.ID, user, recs})
		} else if err == ErrStreamNotExist {
			err = RenderError(w, http.StatusNotFound, "Invalid stream name.")
		}
		return err
	}

	switch r.URL.Path {
	case "/":
		if r.Method != "GET" {
			return RenderInvalidMethod(w, "GET")
		}
		return Render(w, http.StatusOK, Landing{user})

	case "/user/":
		switch r.Method {
		case "GET":
			if user == nil {
				http.Redirect(w, r, "/user/login", http.StatusSeeOther)
				return nil
			}
			return Render(w, http.StatusOK, UserConfig{user})

		case "POST":
			if user == nil {
				return RenderError(w, http.StatusForbidden, "Must be logged in.")
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
				return redirectBack(w, r, "/user/", http.StatusSeeOther)
			}
		}
		return RenderInvalidMethod(w, "GET")

	case "/user/new":
		switch r.Method {
		case "GET":
			if user != nil {
				http.Redirect(w, r, "/user/", http.StatusSeeOther)
				return nil
			}
			return Render(w, http.StatusOK, UserControl{r.URL.Path})

		case "POST":
			if user != nil {
				return RenderError(w, http.StatusForbidden, "Already logged in.")
			}
			username := strings.TrimSpace(r.FormValue("username"))
			password := r.FormValue("password")
			email := r.FormValue("email")
			switch user, err = ctx.NewUser(username, email, []byte(password)); err {
			default:
				return err
			case ErrInvalidUsername, ErrInvalidPassword, ErrInvalidEmail, ErrUserNotUnique:
				return RenderError(w, http.StatusBadRequest, err.Error())
			case ErrNotSupported:
				return RenderError(w, http.StatusNotImplemented, "Authentication is disabled.")
			case nil:
			}
			if err = ctx.SetAuthInfo(w, user.ID); err != nil {
				return err
			}
			return redirectBack(w, r, "/user/", http.StatusSeeOther)
		}
		return RenderInvalidMethod(w, "GET, POST")

	case "/user/login":
		switch r.Method {
		case "GET":
			if user != nil {
				http.Redirect(w, r, "/user/", http.StatusSeeOther)
				return nil
			}
			return Render(w, http.StatusOK, UserControl{r.URL.Path})

		case "POST":
			if user != nil {
				return RenderError(w, http.StatusForbidden, "Already logged in.")
			}
			uid, err := ctx.GetUserID(r.FormValue("username"), []byte(r.FormValue("password")))
			if err == ErrUserNotExist {
				return RenderError(w, http.StatusForbidden, "Invalid username/password.")
			}
			if err = ctx.SetAuthInfo(w, uid); err != nil {
				return err
			}
			return redirectBack(w, r, "/user/", http.StatusSeeOther)
		}
		return RenderInvalidMethod(w, "GET, POST")

	case "/user/restore":
		switch r.Method {
		case "GET":
			if r.URL.RawQuery != "" {
				return Render(w, http.StatusOK, UserControl{"/user/restore?2"})
			}
			return Render(w, http.StatusOK, UserControl{r.URL.Path})

		case "POST":
			if r.URL.RawQuery != "" {
				uid, err := strconv.ParseInt(r.FormValue("uid"), 10, 64)
				if err != nil {
					return RenderError(w, http.StatusBadRequest, "Invalid user ID.")
				}
				err = ctx.ResetUserStep2(uid, r.FormValue("token"), []byte(r.FormValue("password")))
				if err == nil {
					err = ctx.SetAuthInfo(w, uid)
				}
				switch err {
				default:
					return err
				case ErrInvalidPassword:
					return RenderError(w, http.StatusBadRequest, err.Error())
				case ErrUserNotExist, nil:
					http.Redirect(w, r, "/user/", http.StatusSeeOther)
				}
				return nil
			}
			uid, token, err := ctx.ResetUser(r.FormValue("username"), r.FormValue("email"))
			if err != nil && err != ErrUserNotExist {
				return err
			}
			return Render(w, http.StatusOK, UserRestoreEmailSent{UserControl{"/user/restore?1"}, uid, token})
		}
		return RenderInvalidMethod(w, "GET, POST")

	case "/user/logout": // TODO some protection against XSS?
		if r.Method != "GET" {
			return RenderInvalidMethod(w, "GET")
		}
		ctx.SetAuthInfo(w, -1) // should not fail
		return redirectBack(w, r, "/", http.StatusSeeOther)

	case "/user/activate":
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
		return redirectBack(w, r, "/user/", http.StatusSeeOther)

	case "/user/new-token", "/user/set-stream-name", "/user/set-stream-panel", "/user/del-stream-panel":
		if r.Method != "POST" {
			return RenderInvalidMethod(w, "POST")
		}
		if user == nil {
			return RenderError(w, http.StatusForbidden, "You own no streams.")
		}

		switch r.URL.Path {
		case "/user/new-token":
			err = ctx.NewStreamToken(user.ID)

		case "/user/set-stream-name":
			err = ctx.SetStreamName(user.ID, r.FormValue("value"), r.FormValue("nsfw") == "yes")

		case "/user/set-stream-panel":
			// TODO image
			if r.FormValue("id") != "" {
				id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
				if err != nil {
					return RenderError(w, http.StatusBadRequest, "Invalid panel id.")
				}
				err = ctx.SetStreamPanel(user.ID, id, r.FormValue("value"))
			} else {
				err = ctx.AddStreamPanel(user.ID, r.FormValue("value"))
			}

		case "/user/del-stream-panel":
			id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
			if err != nil {
				return RenderError(w, http.StatusBadRequest, "Invalid panel id.")
			}
			err = ctx.DelStreamPanel(user.ID, id)
		}

		if err == nil {
			return redirectBack(w, r, "/user/", http.StatusSeeOther)
		}
		return err

	}

	return RenderError(w, http.StatusNotFound, "")
}
