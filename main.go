// Usage: webmcast [[interface]:port]
//
// A Twitch-like WebM broadcasting service.
//
package main

import (
	"github.com/gorilla/securecookie"
	"golang.org/x/net/websocket"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"./broadcast"
	"./chat"
	"./database"
	"./templates"
)

var (
	// the key used to sign client-side secure session cookies.
	// should probably be changed in production, but not random
	// so that cookies stay valid across nodes/app restarts.
	secretKey = []byte("12345678901234567890123456789012")
	// default interface & port to bind on
	iface = ":8000"
)

// a bunch of net/http hacks first. this structure wraps a filesystem interface
// used to serve static files and disallows any accesses to directories,
// returning 404 instead of listing contents.
//
//      http.FileServer(fs) --> http.FileServer(disallowDirectoryListing{fs})
//
type disallowDirectoryListing struct {
	http.FileSystem
}

func (fs disallowDirectoryListing) Open(name string) (http.File, error) {
	f, err := fs.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	if stat, _ := f.Stat(); stat.IsDir() {
		return nil, os.ErrNotExist
	}
	return f, nil
}

// redirect the client back to the page that referred it here.
// if the client does not send the `Referer` header, redirect it to a fallback URL
// instead. never fails; the `nil` return value is for convenience. XHR requests
// are never redirected with 303 See Other; instead, they get 204 No Content.
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

// an HTTP interface to webmcast. uses a database to assign ownership
// to normally owner-less streams in a broadcasting context. implements `http.Handler`.
type HTTPContext struct {
	cookieCodec *securecookie.SecureCookie
	chats       map[string]*chat.Context
	database.Interface
	broadcast.Set
}

func NewHTTPContext(d database.Interface) *HTTPContext {
	// NOTE go vet complains about passing a mutex in `Set` by value.
	//      this is fine; the mutex must not be held while creating a context anyway.
	ctx := &HTTPContext{
		cookieCodec: securecookie.New(secretKey, nil),
		chats:       make(map[string]*chat.Context),
		Interface:   d,
	}
	ctx.OnStreamClose = func(id string) {
		if chat, ok := ctx.chats[id]; ok {
			chat.Close()
			delete(ctx.chats, id)
		}
		if err := ctx.StopStream(id); err != nil {
			log.Println("Error stopping the stream: ", err)
		}
	}
	return ctx
}

func (ctx *HTTPContext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := ctx.ServeHTTPUnsafe(w, r); err != nil {
		log.Println("error rendering template", r.URL.Path, err.Error())
		if err = templates.Error(w, http.StatusInternalServerError, ""); err != nil {
			log.Println("error rendering error", err.Error())
			http.Error(w, "Error while rendering error message.", http.StatusInternalServerError)
		}
	}
}

func (ctx *HTTPContext) ServeHTTPUnsafe(w http.ResponseWriter, r *http.Request) error {
	if r.Method == "HEAD" {
		r.Method = "GET"
	}
	if r.URL.Path == "/" {
		if r.Method != "GET" {
			return templates.InvalidMethod(w, "GET")
		}
		auth, err := ctx.GetAuthInfo(r)
		if err != nil && err != database.ErrUserNotExist {
			return err
		}
		return templates.Page(w, http.StatusOK, templates.Landing{auth})
	}
	if !strings.ContainsRune(r.URL.Path[1:], '/') {
		return ctx.Player(w, r, r.URL.Path[1:])
	}
	if strings.HasPrefix(r.URL.Path, "/stream/") && !strings.ContainsRune(r.URL.Path[8:], '/') {
		return ctx.Stream(w, r, r.URL.Path[8:])
	}
	if strings.HasPrefix(r.URL.Path, "/user/") {
		return ctx.UserControl(w, r, r.URL.Path[5:])
	}
	return templates.Error(w, http.StatusNotFound, "")
}

// read the id of the logged-in user from a secure session cookie.
// returns an ErrUserNotFound if there is no cookie, the cookie is invalid,
// or the user has since been removed from the database. all other errors
// are sql-related and are unrecoverable. probably.
func (ctx *HTTPContext) GetAuthInfo(r *http.Request) (*database.UserShortData, error) {
	var uid int64
	if cookie, err := r.Cookie("uid"); err == nil {
		if err = ctx.cookieCodec.Decode("uid", cookie.Value, &uid); err == nil {
			return ctx.GetUserShort(uid)
		}
	}
	return nil, database.ErrUserNotExist
}

// write a secure session cookie containing the specified user id to be read
// by `GetAuthInfo` later. or, if id is -1, erase the session cookie instead.
func (ctx *HTTPContext) SetAuthInfo(w http.ResponseWriter, id int64) error {
	if id == -1 {
		http.SetCookie(w, &http.Cookie{Name: "uid", Value: "", Path: "/", MaxAge: 0})
	} else {
		enc, err := ctx.cookieCodec.Encode("uid", id)
		if err != nil {
			return err
		}
		http.SetCookie(w, &http.Cookie{
			Name: "uid", Value: enc, Path: "/", HttpOnly: true, MaxAge: 31536000,
		})
	}
	return nil
}

// GET /<name>
//     Open a simple HTML5-based player with a stream-local chat.
//
func (ctx *HTTPContext) Player(w http.ResponseWriter, r *http.Request, id string) error {
	if r.Method != "GET" {
		return templates.InvalidMethod(w, "GET")
	}

	auth, err := ctx.GetAuthInfo(r)
	if err != nil && err != database.ErrUserNotExist {
		return err
	}
	owns := auth != nil && id == auth.Login
	stream, ok := ctx.Readable(id)
	if !ok {
		switch _, err := ctx.GetStreamServer(id); err {
		case nil:
			return database.ErrStreamNotExist
		case database.ErrStreamNotHere:
			// TODO redirect
			return templates.Error(w, http.StatusNotFound, "This stream is not here.")
		case database.ErrStreamOffline:
			meta, err := ctx.GetStreamMetadata(id)
			if err != database.ErrStreamOffline {
				return err
			}
			return templates.Page(w, http.StatusOK, templates.Room{id, owns, nil, meta, auth, nil})
		case database.ErrStreamNotExist:
			return templates.Error(w, http.StatusNotFound, "Invalid stream name.")
		default:
			return err
		}
	}
	meta, err := ctx.GetStreamMetadata(id)
	if err != nil {
		// since we know the stream exists (it is on this server),
		// this has to be an sql error.
		return err
	}
	return templates.Page(w, http.StatusOK, templates.Room{id, owns, stream, meta, auth, ctx.chats[id]})
}

// POST /stream/<name> or PUT /stream/<name>
//     Broadcast a WebM video/audio file.
//
//     Accepted input: valid WebM split into arbitrarily many requests in absolutely
//     any way. Multiple files can be concatenated into a single stream as long as they
//     contain exactly the same tracks (i.e. their number, codecs, and dimensions.
//     Otherwise any connected decoders will error and have to restart. Changing,
//     for example, bitrate or tags is fine.)
//
// GET /stream/<name>
//     Receive a published WebM stream. Note that the server makes no attempt
//     at buffering; if the stream is being broadcast faster than its native framerate,
//     the client will have to buffer and/or drop frames.
//
// GET /stream/<name> with Upgrade: websocket
//     Connect to a JSON-RPC v2.0 node.
//
//     Methods of `Chat`:
//
//        * `SetName(string)`: assign a (unique) name to this client. This is required to...
//        * `SendMessage(string)`: broadcast a simple text message to all viewers.
//        * `RequestHistory()`: ask the server to emit notifications containing the last
//          few broadcasted text messages.
//
//     TODO Methods of `Stream`.
//
//     Notifications:
//
//        * `Chat.AcquiredName(user string)`: upon a successful `SetName`.
//          May be emitted automatically at the start of a connection if already logged in.
//        * `Chat.Message(user string, text string)`: a broadcasted text message.
//
func (ctx *HTTPContext) Stream(w http.ResponseWriter, r *http.Request, id string) error {
	switch r.Method {
	case "GET":
		if r.URL.RawQuery != "" {
			return templates.Error(w, http.StatusBadRequest, "POST or PUT, don't GET.")
		}

		stream, ok := ctx.Readable(id)
		if !ok {
			switch _, err := ctx.GetStreamServer(id); err {
			case nil:
				// this is a server-side error. this stream is supposed to be
				// on this server, but somehow it is not.
				return database.ErrStreamNotExist
			case database.ErrStreamNotHere:
				// TODO redirect
				return templates.Error(w, http.StatusNotFound, "This stream is not here.")
			case database.ErrStreamOffline:
				return templates.Error(w, http.StatusNotFound, "Stream offline.")
			case database.ErrStreamNotExist:
				return templates.Error(w, http.StatusNotFound, "Invalid stream name.")
			default:
				return err
			}
		}

		if upgrade, ok := r.Header["Upgrade"]; ok {
			for i := range upgrade {
				if strings.ToLower(upgrade[i]) == "websocket" {
					auth, err := ctx.GetAuthInfo(r)
					if err != nil && err != database.ErrUserNotExist {
						return err
					}
					websocket.Handler(func(ws *websocket.Conn) {
						c, ok := ctx.chats[id]
						if !ok {
							c = chat.New(20)
							ctx.chats[id] = c
						}
						c.RunRPC(ws, auth)
					}).ServeHTTP(w, r)
					return nil
				}
			}
		}

		header := w.Header()
		header.Set("Cache-Control", "no-cache")
		header.Set("Content-Type", "video/webm")
		w.WriteHeader(http.StatusOK)

		ch := make(chan []byte, 60)
		defer close(ch)

		stream.Connect(ch, false)
		defer stream.Disconnect(ch)

		for chunk := range ch {
			if _, err := w.Write(chunk); err != nil || stream.Closed {
				break
			}
		}
		return nil

	case "PUT", "POST":
		err := ctx.StartStream(id, r.URL.RawQuery)
		switch err {
		case nil:
		case database.ErrInvalidToken:
			return templates.Error(w, http.StatusForbidden, "Invalid token.")
		case database.ErrStreamNotExist:
			return templates.Error(w, http.StatusNotFound, "Invalid stream ID.")
		case database.ErrStreamNotHere:
			return templates.Error(w, http.StatusForbidden, "The stream is on another server.")
		default:
			return err
		}

		stream, ok := ctx.Writable(id)
		if !ok {
			return templates.Error(w, http.StatusForbidden, "Stream ID already taken.")
		}
		defer stream.Close()

		buffer := [16384]byte{}
		for {
			n, err := r.Body.Read(buffer[:])
			if n != 0 {
				if _, err := stream.Write(buffer[:n]); err != nil {
					stream.Reset()
					return templates.Error(w, http.StatusBadRequest, err.Error())
				}
			}
			if err != nil {
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
		}
	}

	return templates.InvalidMethod(w, "GET, POST, PUT")
}

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
// POST /user/set-stream-name
//     [XHR-only] Change the display name of the stream.
//     All connected viewers receive an RPC event.
//     Parameters: value string
//
// POST /user/set-stream-about
//     [XHR only] Change the text in the "about" section of the stream.
//     Parameters: value string
//
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
			case database.ErrInvalidUsername, database.ErrInvalidPassword, database.ErrInvalidEmail, database.ErrUserNotUnique:
				return templates.Error(w, http.StatusBadRequest, err.Error())
			case database.ErrNotSupported:
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
			if err == database.ErrUserNotExist {
				return templates.Page(w, http.StatusOK, templates.UserLogin(0))
			}
			if err == nil {
				http.Redirect(w, r, "/user/cfg", http.StatusSeeOther)
			}
			return err

		case "POST":
			uid, err := ctx.GetUserID(r.FormValue("username"), []byte(r.FormValue("password")))
			if err == database.ErrUserNotExist {
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
			if err == database.ErrUserNotExist {
				http.Redirect(w, r, "/user/login", http.StatusSeeOther)
				return nil
			}
			if err != nil {
				return err
			}
			userFull, err := ctx.GetUserFull(user.ID)
			if err != nil {
				return err
			}
			return templates.Page(w, http.StatusOK, templates.UserConfig{userFull})

		case "POST":
			//     Parameters: password-old string,
			//                 username, displayname, email, password, about string optional
			user, err := ctx.GetAuthInfo(r)
			if err == database.ErrUserNotExist {
				return templates.Error(w, http.StatusForbidden, "Must be logged in.")
			}
			if err != nil {
				return err
			}
			switch err = user.CheckPassword([]byte(r.FormValue("password-old"))); err {
			default:
				return err
			case database.ErrUserNotExist:
				return templates.Error(w, http.StatusForbidden, "Invalid old password.")
			case nil:
			}

			_, err = ctx.SetUserMetadata(user.ID,
				r.FormValue("username"), r.FormValue("displayname"), r.FormValue("email"),
				r.FormValue("about"), []byte(r.FormValue("password")),
			)
			switch err {
			default:
				return err
			case database.ErrInvalidUsername, database.ErrInvalidPassword, database.ErrInvalidEmail, database.ErrUserNotUnique:
				return templates.Error(w, http.StatusBadRequest, err.Error())
			case database.ErrStreamActive:
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
		if err == database.ErrUserNotExist {
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
		if err == database.ErrInvalidToken {
			return templates.Error(w, http.StatusBadRequest, "Invalid activation token.")
		}
		if err != nil {
			return err
		}
		return redirectBack(w, r, "/user/cfg", http.StatusSeeOther)

	case "/set-stream-name", "/set-stream-about":
		if r.Method != "POST" {
			return templates.InvalidMethod(w, "POST")
		}

		auth, err := ctx.GetAuthInfo(r)
		if err == database.ErrUserNotExist {
			return templates.Error(w, http.StatusForbidden, "You own no streams.")
		}
		if err != nil {
			return err
		}

		value := r.FormValue("value")
		if path == "/set-stream-name" {
			err = ctx.SetStreamName(auth.ID, value)
		} else {
			err = ctx.SetStreamAbout(auth.ID, value)
		}
		if err != nil {
			return err
		}

		if chat, ok := ctx.chats[auth.Login]; ok {
			if path == "/set-stream-name" {
				chat.NewStreamName(value)
			} else {
				chat.NewStreamAbout(value)
			}
		}

		w.WriteHeader(http.StatusNoContent)
		return nil

	}

	return templates.Error(w, http.StatusNotFound, "")
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	if len(os.Args) >= 2 {
		iface = os.Args[1]
	}

	db, err := database.NewSQLDatabase(iface, "sqlite3", "development.db")
	if err != nil {
		log.Fatal("Could not connect to the database: ", err)
	}
	defer db.Close()

	ctx := NewHTTPContext(db)
	ctx.Timeout = 10 * time.Second
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(disallowDirectoryListing{http.Dir(".")}))
	mux.Handle("/", ctx)
	log.Fatal(http.ListenAndServe(iface, mux))
}
