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
//        * `Chat.Message(user string, text string)`: a broadcasted text message.
//
package main

import (
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func wantsWebsocket(r *http.Request) bool {
	if upgrade, ok := r.Header["Upgrade"]; ok {
    	for i := range upgrade {
    		if strings.ToLower(upgrade[i]) == "websocket" {
    			return true
    		}
    	}
	}
	// func is_a_language_that_has_no_Array_Contains_method_any_good() bool {
	return false
	// }
}

func (ctx *Context) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := ctx.RootHTTP(w, r); err != nil {
		log.Println("error rendering template", r.URL.Path, err.Error())
		if err = RenderError(w, http.StatusInternalServerError, ""); err != nil {
			log.Println("error rendering error", err.Error())
			http.Error(w, "Error while rendering error message.", http.StatusInternalServerError)
		}
	}
}

func (ctx *Context) RootHTTP(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/" {
		return RenderError(w, http.StatusNotImplemented, "There is no UI yet.")
	}

	if strings.HasPrefix(r.URL.Path, "/stream/") {
		streamID := strings.TrimPrefix(r.URL.Path, "/stream/")

		switch r.Method {
		case "GET", "HEAD":
			stream, ok := ctx.Get(streamID)
			if !ok {
				return RenderError(w, http.StatusNotFound, "Invalid stream name.")
			}

			if wantsWebsocket(r) {
				websocket.Handler(stream.RunRPC).ServeHTTP(w, r)
				return nil
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
				if stream.Closed {
					break
				}
				_, err := w.Write(chunk)
				if err != nil {
					break
				}
			}
			return nil

		case "PUT", "POST":
			stream, ok := ctx.Acquire(streamID)
			if !ok {
				return RenderError(w, http.StatusForbidden, "Stream ID already taken.")
			}

			defer stream.Close()

			buffer := [16384]byte{}
			for {
				n, err := r.Body.Read(buffer[:])
				if n != 0 {
					if _, err2 := stream.Write(buffer[:n]); err2 != nil {
						return RenderError(w, http.StatusBadRequest, err2.Error())
					}
				}
				if err != nil {
					w.WriteHeader(http.StatusNoContent)
					return nil
				}
			}
		}
	}

	streamID := strings.TrimPrefix(r.URL.Path, "/")
	stream, ok := ctx.Get(streamID)
	if !ok {
		return RenderError(w, http.StatusNotFound, "Invalid stream name.")
	}

	return Render(w, http.StatusOK, "room.html", roomViewModel{streamID, stream})
}

type noIndexFilesystem struct {
	fs http.FileSystem
}

func (fs noIndexFilesystem) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	if stat, _ := f.Stat(); stat.IsDir() {
		return nil, os.ErrNotExist
	}
	return f, nil
}

func main() {
	ctx := Context{Timeout: time.Second * 10, ChatHistory: 20}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(noIndexFilesystem{http.Dir(".")}))
	mux.Handle("/", &ctx)
	http.ListenAndServe(":8000", mux)
}
