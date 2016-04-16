package main

import (
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type noIndexFileSystem struct {
	http.FileSystem
}

func (fs noIndexFileSystem) Open(name string) (http.File, error) {
	f, err := fs.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	if stat, _ := f.Stat(); stat.IsDir() {
		return nil, os.ErrNotExist
	}
	return f, nil
}

type HTTPContext struct {
	Database
	Context
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
	if r.URL.Path == "/" {
		return RenderError(w, http.StatusNotImplemented, "There is no UI yet.")
	}
	if !strings.ContainsRune(r.URL.Path[1:], '/') {
		return ctx.Player(w, r, r.URL.Path[1:])
	}
	if strings.HasPrefix(r.URL.Path, "/stream/") && !strings.ContainsRune(r.URL.Path[8:], '/') {
		return ctx.Stream(w, r, r.URL.Path[8:])
	}
	return RenderError(w, http.StatusNotFound, "Page not found.")
}

// GET /<name>
//     Open a simple HTML5-based player with a stream-local chat.
//
func (ctx *HTTPContext) Player(w http.ResponseWriter, r *http.Request, id string) error {
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
			return Render(w, http.StatusOK, "room.html", roomViewModel{id, nil, meta})
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
	return Render(w, http.StatusOK, "room.html", roomViewModel{id, stream, meta})
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
	case "GET", "HEAD":
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
		// TODO obtain token from params
		err := ctx.StartStream(id, "")
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

	w.Header().Set("Allow", "GET, POST, PUT")
	return RenderError(w, http.StatusMethodNotAllowed, "Invalid HTTP method")
}

func main() {
	db := NewAnonDatabase()

	ctx := &HTTPContext{db, Context{Timeout: time.Second * 10, ChatHistory: 20}}
	ctx.OnStreamClose = func(id string) {
		if err := ctx.StopStream(id); err != nil {
			log.Println("Error stopping the stream: ", err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(noIndexFileSystem{http.Dir(".")}))
	mux.Handle("/", ctx)
	http.ListenAndServe(":8000", mux)
}
