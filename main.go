package main

import (
	"golang.org/x/net/websocket"
	"net/http"
	"strings"
	"time"
)

func wantsWebsocket(r *http.Request) bool {
	upgrade, ok := r.Header["Upgrade"]
	if !ok {
		return false
	}

	for i := range upgrade {
		if upgrade[i] == "websocket" {
			return true
		}
	}
	// func is_a_language_that_has_no_Array_Contains_method_any_good() bool {
	return false
	// }
}

func (ctx *Context) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		RenderError(w, http.StatusNotImplemented, "There is no UI yet.")
		return
	}

	if strings.HasPrefix(r.URL.Path, "/stream/") {
		streamID := strings.TrimPrefix(r.URL.Path, "/stream/")

		switch r.Method {
		case "GET", "HEAD":
			stream, ok := ctx.Get(streamID)
			if !ok {
				RenderError(w, http.StatusNotFound, "")
				return
			}

			if wantsWebsocket(r) {
				websocket.Handler(stream.RunRPC).ServeHTTP(w, r)
				return
			}

			header := w.Header()
			header["Content-Type"] = []string{"video/webm"}
			header["Cache-Control"] = []string{"no-cache"}
			w.WriteHeader(http.StatusOK)

			ch := make(chan []byte, 60)
			defer close(ch)

			stream.Connect(ch, false)
			defer stream.Disconnect(ch)

			for chunk := range ch {
				if stream.Done {
					break
				}

				_, err := w.Write(chunk)
				if err != nil {
					break
				}
			}
			return

		case "PUT", "POST":
			stream, ok := ctx.Acquire(streamID)
			if !ok {
				RenderError(w, http.StatusForbidden, "Stream ID already taken.")
				return
			}

			defer stream.Release()

			buffer := [16384]byte{}
			for {
				n, err := r.Body.Read(buffer[:])
				if n != 0 {
					_, err2 := stream.Write(buffer[:n])
					if err2 != nil {
						RenderError(w, http.StatusBadRequest, err.Error())
						return
					}
				}

				if err != nil {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
	}

	streamID := strings.TrimPrefix(r.URL.Path, "/")
	stream, ok := ctx.Get(streamID)
	if !ok {
		RenderError(w, http.StatusNotFound, "")
		return
	}

	RenderHtml(w, http.StatusOK, "room.html", roomViewModel{streamID, stream})
}

func main() {
	ctx := Context{Timeout: time.Second * 10, ChatHistory: 20}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.Dir(".")))
	mux.Handle("/", &ctx)
	http.ListenAndServe(":8000", mux)
}
