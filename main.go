package main

import (
	"fmt"
	"net/http"
)

var streams map[string]*Broadcast

func root(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, World!")
}

func stream(w http.ResponseWriter, r *http.Request) {
	headers := w.Header()

	switch r.Method {
	case "GET", "HEAD":
		stream, ok := streams[r.URL.Path]
		if !ok {
			headers["Content-Type"] = []string{"text/plain"}
			w.WriteHeader(404)
			fmt.Fprintf(w, "Not found\n")
			return
		}

		headers["Content-Type"] = []string{"video/webm"}
		headers["Cache-Control"] = []string{"no-cache"}
		w.WriteHeader(200)

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
		streamID := r.URL.Path
		stream, ok := streams[streamID]

		if ok {
			headers["Content-Type"] = []string{"text/plain"}
			w.WriteHeader(403)
			fmt.Fprintf(w, "ID already taken\n")
			return
		} else {
			stream = NewBroadcast()
			streams[streamID] = stream
		}

		defer func() {
			// TODO wait a bit and abort if the broadcaster reconnects
			delete(streams, streamID)
			stream.Close()
		}()

		buffer := [16384]byte{}
		for {
			n, err := r.Body.Read(buffer[:])
			if n != 0 {
				_, err2 := stream.Write(buffer[:n])
				if err2 != nil {
					headers["Content-Type"] = []string{"text/plain"}
					w.WriteHeader(400)
					fmt.Fprintf(w, "Error: %s\n", err.Error())
					return
				}
			}

			if err != nil {
				w.WriteHeader(204)
				return
			}
		}
	}
}

func main() {
	streams = make(map[string]*Broadcast)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("."))))
	http.Handle("/stream/", http.StripPrefix("/stream/", http.HandlerFunc(stream)))
	http.Handle("/", http.StripPrefix("/", http.HandlerFunc(root)))
	http.ListenAndServe(":8000", nil)
}
