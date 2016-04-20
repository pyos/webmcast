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
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
// if the client does not send the `Referer` header, redirect it
// to a fallback URL instead. never fails; the `nil` return value is for convenience.
func redirectBack(w http.ResponseWriter, r *http.Request, fallback string, code int) error {
	ref := r.Referer()
	if ref == "" {
		ref = fallback
	}
	http.Redirect(w, r, ref, code)
	return nil
}

// an HTTP interface to webmcast. uses a database to assign ownership
// to normally owner-less streams in a broadcasting context. implements `http.Handler`.
type HTTPContext struct {
	cookieCodec *securecookie.SecureCookie
	Database
	Context
}

func NewHTTPContext(d Database, c Context) *HTTPContext {
	// NOTE go vet complains about passing a mutex in `Context` by value.
	//      this is fine; the mutex must not be held while creating a context anyway.
	ctx := &HTTPContext{Database: d, Context: c}
	ctx.cookieCodec = securecookie.New(secretKey, nil)
	ctx.OnStreamClose = func(id string) {
		if err := ctx.StopStream(id); err != nil {
			log.Println("Error stopping the stream: ", err)
		}
	}
	return ctx
}

func (ctx *HTTPContext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := ctx.ServeHTTPUnsafe(w, r); err != nil {
		log.Println("error rendering template", r.URL.Path, err.Error())
		if err = RenderError(w, http.StatusInternalServerError, ""); err != nil {
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
			return RenderInvalidMethod(w, "GET")
		}
		auth, err := ctx.GetAuthInfo(r)
		if err != nil && err != ErrUserNotExist {
			return err
		}
		return Render(w, http.StatusOK, "landing.html", landingViewModel{auth})
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
	return RenderError(w, http.StatusNotFound, "Page not found.")
}

// read the id of the logged-in user from a secure session cookie.
// returns an ErrUserNotFound if there is no cookie, the cookie is invalid,
// or the user has since been removed from the database. all other errors
// are sql-related and are unrecoverable. probably.
func (ctx *HTTPContext) GetAuthInfo(r *http.Request) (*UserShortData, error) {
	var uid int64
	if cookie, err := r.Cookie("uid"); err == nil {
		if err = ctx.cookieCodec.Decode("uid", cookie.Value, &uid); err == nil {
			return ctx.GetUserShort(uid)
		}
	}
	return nil, ErrUserNotExist
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
		return RenderInvalidMethod(w, "GET")
	}

	auth, err := ctx.GetAuthInfo(r)
	if err != nil && err != ErrUserNotExist {
		return err
	}
	stream, ok := ctx.Get(id)
	if !ok {
		switch _, err := ctx.GetStreamServer(id); err {
		case nil:
			return ErrStreamNotExist
		case ErrStreamNotHere:
			// TODO redirect
			return RenderError(w, http.StatusNotFound, "This stream is not here.")
		case ErrStreamOffline:
			meta, err := ctx.GetStreamMetadata(id)
			if err != ErrStreamOffline {
				return err
			}
			return Render(w, http.StatusOK, "room.html", roomViewModel{id, nil, meta, auth})
		case ErrStreamNotExist:
			return RenderError(w, http.StatusNotFound, "Invalid stream name.")
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
	return Render(w, http.StatusOK, "room.html", roomViewModel{id, stream, meta, auth})
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
		stream, ok := ctx.Get(id)
		if !ok {
			switch _, err := ctx.GetStreamServer(id); err {
			case nil:
				// this is a server-side error. this stream is supposed to be
				// on this server, but somehow it is not.
				return ErrStreamNotExist
			case ErrStreamNotHere:
				// TODO redirect
				return RenderError(w, http.StatusNotFound, "This stream is not here.")
			case ErrStreamOffline:
				return RenderError(w, http.StatusNotFound, "Stream offline.")
			case ErrStreamNotExist:
				return RenderError(w, http.StatusNotFound, "Invalid stream name.")
			default:
				return err
			}
		}

		if upgrade, ok := r.Header["Upgrade"]; ok {
			for i := range upgrade {
				if strings.ToLower(upgrade[i]) == "websocket" {
					websocket.Handler(stream.RunRPC).ServeHTTP(w, r)
					return nil
				}
			}
		}

		header := w.Header()
		header.Set("Cache-Control", "no-cache")
		if stream.HasVideo {
			header.Set("Content-Type", "video/webm")
		} else {
			header.Set("Content-Type", "audio/webm")
		}
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
		case ErrInvalidToken:
			return RenderError(w, http.StatusForbidden, "Invalid token.")
		case ErrStreamNotExist:
			return RenderError(w, http.StatusNotFound, "Invalid stream ID.")
		case ErrStreamNotHere:
			return RenderError(w, http.StatusForbidden, "The stream is on another server.")
		default:
			return err
		}

		stream, ok := ctx.Acquire(id)
		if !ok {
			return RenderError(w, http.StatusForbidden, "Stream ID already taken.")
		}
		defer stream.Close()

		buffer := [16384]byte{}
		for {
			n, err := r.Body.Read(buffer[:])
			if n != 0 {
				if _, err := stream.Write(buffer[:n]); err != nil {
					stream.Reset()
					return RenderError(w, http.StatusBadRequest, err.Error())
				}
			}
			if err != nil {
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
		}
	}

	return RenderInvalidMethod(w, "GET, POST, PUT")
}

// POST /user/new
//     ...
//
// POST /user/login
//     Obtain a session cookie.
//
//     Parameters: email string, password string
//
// GET /user/logout
//     Remove the session cookie.
//
func (ctx *HTTPContext) UserControl(w http.ResponseWriter, r *http.Request, path string) error {
	switch path {
	case "/new":
		if r.Method != "POST" {
			return RenderInvalidMethod(w, "POST")
		}
		return RenderError(w, http.StatusNotImplemented, "There is no UI yet.")

	case "/login":
		if r.Method != "POST" {
			return RenderInvalidMethod(w, "POST")
		}
		uid, err := ctx.GetUserID(r.FormValue("username"), []byte(r.FormValue("password")))
		if err == ErrUserNotExist {
			return RenderError(w, http.StatusForbidden, "Invalid username/password.")
		}
		if err = ctx.SetAuthInfo(w, uid); err != nil {
			return err
		}
		return redirectBack(w, r, "/", http.StatusSeeOther)

	case "/logout":
		if r.Method != "GET" {
			return RenderInvalidMethod(w, "GET")
		}
		ctx.SetAuthInfo(w, -1) // should not fail
		return redirectBack(w, r, "/", http.StatusSeeOther)
	}

	return RenderError(w, http.StatusNotFound, "Page not found.")
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	if len(os.Args) >= 2 {
		iface = os.Args[1]
	}

	ctx := NewHTTPContext(NewAnonDatabase(), Context{Timeout: time.Second * 10, ChatHistory: 20})
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(disallowDirectoryListing{http.Dir(".")}))
	mux.Handle("/", ctx)
	http.ListenAndServe(iface, mux)
}
