package main

import (
	"net/http"
	"time"
)

var DefaultContext = NewContext(time.Second * 10)

func root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" {
		RenderError(w, http.StatusNotImplemented, "There is no UI yet.")
		return
	}

	stream, ok := DefaultContext.Get(r.URL.Path)
	if !ok {
		RenderError(w, http.StatusNotFound, "")
		return
	}

	RenderHtml(w, http.StatusOK, "room.html", roomViewModel{r.URL.Path, stream})
}

func stream(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET", "HEAD":
		stream, ok := DefaultContext.Get(r.URL.Path)
		if !ok {
			RenderError(w, http.StatusNotFound, "")
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

	case "PUT", "POST":
		stream, ok := DefaultContext.Acquire(r.URL.Path)
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

func main() {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	mux.Handle("/stream/", http.StripPrefix("/stream/", http.HandlerFunc(stream)))
	mux.Handle("/", http.StripPrefix("/", http.HandlerFunc(root)))
	http.ListenAndServe(":8000", mux)
}
