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
	"flag"
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"strings"
	"sync"

	"../common"
	"../templates"
)

type HTTPContext struct {
	BroadcastSet
	chatLock sync.Mutex
	chats    map[string]*Chat
	common.Database
}

func NewHTTPContext(d common.Database) *HTTPContext {
	ctx := &HTTPContext{chats: make(map[string]*Chat), Database: d}
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
	ctx.OnStreamTrackInfo = func(id string, info *common.StreamTrackInfo) {
		if err := ctx.SetStreamTrackInfo(id, info); err != nil {
			log.Println("Error setting stream metadata: ", err)
		}
	}
	return ctx
}

func (ctx *HTTPContext) ServeHTTPUnsafe(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/" {
		return templates.Error(w, http.StatusBadRequest, "This is not an UI node.")
	}
	if !strings.ContainsRune(r.URL.Path[1:], '/') {
		return ctx.Stream(w, r, r.URL.Path[1:])
	}
	return templates.Error(w, http.StatusNotFound, "")
}

func (ctx *HTTPContext) GetAuthInfo(r *http.Request) (*common.UserShortData, error) {
	var uid int64
	if cookie, err := r.Cookie("uid"); err == nil {
		if err = common.CookieCodec.Decode("uid", cookie.Value, &uid); err == nil {
			return ctx.GetUserShort(uid)
		}
	}
	return nil, common.ErrUserNotExist
}

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
				return common.ErrStreamNotExist
			case common.ErrStreamNotHere:
				// TODO redirect
				return templates.Error(w, http.StatusNotFound, "This stream is not here.")
			case common.ErrStreamOffline:
				return templates.Error(w, http.StatusNotFound, "Stream offline.")
			case common.ErrStreamNotExist:
				return templates.Error(w, http.StatusNotFound, "Invalid stream name.")
			default:
				return err
			}
		}

		if upgrade, ok := r.Header["Upgrade"]; ok {
			for i := range upgrade {
				if strings.ToLower(upgrade[i]) == "websocket" {
					auth, err := ctx.GetAuthInfo(r)
					if err != nil && err != common.ErrUserNotExist {
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

	case "PUT", "POST":
		err := ctx.StartStream(id, r.URL.RawQuery)
		switch err {
		case nil:
		case common.ErrInvalidToken:
			return templates.Error(w, http.StatusForbidden, "Invalid token.")
		case common.ErrStreamNotExist:
			return templates.Error(w, http.StatusNotFound, "Invalid stream ID.")
		case common.ErrStreamNotHere:
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

func main() {
	flag.Parse()
	ctx := NewHTTPContext(common.CreateDatabase(common.DefaultInterface))
	ctx.Timeout = common.StreamKeepAlive
	mux := http.NewServeMux()
	mux.Handle("/static/", templates.StaticHandler("."))
	mux.Handle("/", templates.UnsafeHandler{ctx})
	log.Fatal(http.ListenAndServe(common.DefaultInterface, mux))
}
