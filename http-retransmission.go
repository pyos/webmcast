// POST /<name> or PUT /<name>
//     Broadcast a WebM video/audio file.
//
//     Accepted input: valid WebM split into arbitrarily many requests in absolutely
//     any way. Multiple files can be concatenated into a single stream as long as they
//     contain exactly the same tracks (i.e. their number, codecs, and dimensions.
//     Otherwise any connected decoders will error and have to restart. Changing,
//     for example, bitrate or tags is fine.)
//
// GET /<name>
//     Receive a published WebM stream. Note that the server makes no attempt
//     at buffering; if the stream is being broadcast faster than its native framerate,
//     the client will have to buffer and/or drop frames.
//
// GET /<name> with Upgrade: websocket
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
package main

import (
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"strings"
	"sync"
)

type RetransmissionContext struct {
	BroadcastSet
	chatLock sync.Mutex
	chats    map[string]*Chat
	context  *Context
}

func NewRetransmissionContext(c *Context) *RetransmissionContext {
	ctx := &RetransmissionContext{chats: make(map[string]*Chat), context: c}
	ctx.Timeout = c.StreamKeepAlive
	ctx.OnStreamClose = func(id string) {
		ctx.chatLock.Lock()
		if chat, ok := ctx.chats[id]; ok {
			chat.Close()
			delete(ctx.chats, id)
		}
		ctx.chatLock.Unlock()
		if err := ctx.context.StopStream(id); err != nil {
			log.Println("Error stopping the stream: ", err)
		}
	}
	ctx.OnStreamTrackInfo = func(id string, info *StreamTrackInfo) {
		if err := ctx.context.SetStreamTrackInfo(id, info); err != nil {
			log.Println("Error setting stream metadata: ", err)
		}
	}
	return ctx
}

func (ctx *RetransmissionContext) ServeHTTPUnsafe(w http.ResponseWriter, r *http.Request) error {
	switch {
	case r.URL.Path == "/":
		http.Error(w, "This is not an UI node.", http.StatusBadRequest)
	case strings.ContainsRune(r.URL.Path[1:], '/'):
		http.Error(w, "", http.StatusNotFound)
	case r.Method == "GET":
		return ctx.watch(w, r, r.URL.Path[1:])
	case r.Method == "POST" || r.Method == "PUT":
		return ctx.stream(w, r, r.URL.Path[1:])
	default:
		w.Header().Set("Allow", "GET, PUT, POST")
		http.Error(w, "Invalid HTTP method.", http.StatusMethodNotAllowed)
	}
	return nil
}

func (ctx *RetransmissionContext) watch(w http.ResponseWriter, r *http.Request, id string) error {
	stream, ok := ctx.Readable(id)
	if !ok {
		switch _, err := ctx.context.GetStreamServer(id); err {
		case ErrStreamNotHere:
			http.Error(w, "This stream is not here.", http.StatusNotFound)
		case ErrStreamOffline, nil:
			http.Error(w, "Stream offline.", http.StatusNotFound)
		case ErrStreamNotExist:
			http.Error(w, "Invalid stream name.", http.StatusNotFound)
		default:
			return err
		}
		return nil
	}

	if upgrade, ok := r.Header["Upgrade"]; ok {
		for i := range upgrade {
			if strings.ToLower(upgrade[i]) == "websocket" {
				auth, err := ctx.context.GetAuthInfo(r)
				if err != nil && err != ErrUserNotExist {
					return err
				}
				websocket.Handler(func(ws *websocket.Conn) {
					ctx.chatLock.Lock()
					chat, ok := ctx.chats[id]
					if !ok {
						chat = NewChat(20)
						ctx.chats[id] = chat
					}
					ctx.chatLock.Unlock()
					chat.RunRPC(ws, auth)
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
}

func (ctx *RetransmissionContext) stream(w http.ResponseWriter, r *http.Request, id string) error {
	switch err := ctx.context.StartStream(id, r.URL.RawQuery); err {
	case ErrInvalidToken:
		http.Error(w, "Invalid token.", http.StatusForbidden)
		return nil
	case ErrStreamNotExist:
		http.Error(w, "Invalid stream ID.", http.StatusNotFound)
		return nil
	case ErrStreamNotHere:
		http.Error(w, "The stream is on another server.", http.StatusBadRequest)
		return nil
	default:
		return err
	case nil:
	}

	stream, ok := ctx.Writable(id)
	if !ok {
		http.Error(w, "Stream ID already taken.", http.StatusForbidden)
		return nil
	}
	defer stream.Close()

	buffer := [16384]byte{}
	for {
		n, err := r.Body.Read(buffer[:])
		if n != 0 {
			if _, err := stream.Write(buffer[:n]); err != nil {
				stream.Reset()
				http.Error(w, err.Error(), http.StatusBadRequest)
				return nil
			}
		}
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
	}
}
