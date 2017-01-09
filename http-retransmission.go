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
// GET /stream/<name> [Upgrade: websocket]
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

type RetransmissionHandler struct {
	BroadcastSet
	chatLock sync.Mutex
	chats    map[string]*Chat
	*Context
}

func NewRetransmissionHandler(c *Context) *RetransmissionHandler {
	ctx := &RetransmissionHandler{chats: make(map[string]*Chat), Context: c}
	ctx.Timeout = c.StreamKeepAlive
	ctx.OnStreamClose = func(id string) {
		ctx.chatLock.Lock()
		if chat, ok := ctx.chats[id]; ok {
			chat.Close()
			delete(ctx.chats, id)
		}
		ctx.chatLock.Unlock()
		if err := ctx.StopStream(id); err != nil {
			log.Println("Error stopping the stream: ", err)
		}
	}
	ctx.OnStreamTrackInfo = func(id string, info *StreamTrackInfo) {
		if err := ctx.SetStreamTrackInfo(id, info); err != nil {
			log.Println("Error setting stream metadata: ", err)
		}
	}
	return ctx
}

func (ctx *RetransmissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	switch {
	case r.URL.Path == "/stream/" || strings.ContainsRune(r.URL.Path[8:], '/'):
		return RenderError(w, http.StatusNotFound, "")
	case r.Method == "GET":
		return ctx.watch(w, r, r.URL.Path[8:])
	case r.Method == "POST" || r.Method == "PUT":
		return ctx.stream(w, r, r.URL.Path[8:])
	default:
		return RenderInvalidMethod(w, "GET, PUT, POST")
	}
}

func wantsWebsocket(r *http.Request) bool {
	if upgrade, ok := r.Header["Upgrade"]; ok {
		for i := range upgrade {
			if strings.ToLower(upgrade[i]) == "websocket" {
				return true
			}
		}
	}
	return false
}

func (ctx *RetransmissionHandler) watch(w http.ResponseWriter, r *http.Request, id string) error {
	if r.URL.RawQuery != "" {
		return RenderError(w, http.StatusBadRequest, "Send WebMs here, watch using the other links.")
	}

	stream, ok := ctx.Readable(id)
	if !ok {
		switch server, err := ctx.GetStreamServer(id); err {
		case ErrStreamNotHere:
			if wantsWebsocket(r) {
				// simply redirecting won't do -- browsers will throw an error.
				websocket.Handler(func(ws *websocket.Conn) {
					RPCPushEvent(ws, "RPC.Redirect", "//"+server+r.URL.Path)
				}).ServeHTTP(w, r)
				return nil
			}
			http.Redirect(w, r, "//"+server+r.URL.Path, http.StatusTemporaryRedirect)
			return nil
		case ErrStreamOffline, nil:
			return RenderError(w, http.StatusNotFound, "Stream offline.")
		case ErrStreamNotExist:
			return RenderError(w, http.StatusNotFound, "Invalid stream name.")
		default:
			return err
		}
	}

	if wantsWebsocket(r) {
		auth, err := ctx.GetAuthInfo(r)
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

	header := w.Header()
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Cache-Control", "no-cache")
	header.Set("Content-Type", "video/webm")
	w.WriteHeader(http.StatusOK)
	f, flushable := w.(http.Flusher)

	ch := make(chan []byte, 240)
	defer close(ch)

	stream.Connect(ch, false)
	defer stream.Disconnect(ch)

	for chunk := range ch {
		if _, err := w.Write(chunk); err != nil || stream.Closed {
			break
		}
		if flushable {
			f.Flush()
		}
	}
	return nil
}

func (ctx *RetransmissionHandler) stream(w http.ResponseWriter, r *http.Request, id string) error {
	switch err := ctx.StartStream(id, r.URL.RawQuery); err {
	case ErrInvalidToken:
		return RenderError(w, http.StatusForbidden, "Invalid token.")
	case ErrStreamNotExist:
		return RenderError(w, http.StatusNotFound, "Invalid stream ID.")
	case ErrStreamNotHere:
		return RenderError(w, http.StatusBadRequest, "Wrong server.")
	default:
		return err
	case nil:
	}

	stream, ok := ctx.Writable(id)
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
