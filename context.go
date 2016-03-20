package main

import (
	"container/ring"
	"errors"
	"golang.org/x/net/websocket"
	"time"
)

type Context struct {
	streams map[string]*BroadcastContext
	timeout time.Duration
}

type BroadcastContext struct {
	Broadcast
	// There is a timeout after releasing a stream during which it is possible
	// to reacquire the same object and continue broadcasting. Once the timeout
	// elapses, the stream is closed for good.
	ping    chan int
	closing bool

	chatHistory  *ring.Ring
	viewers      map[*ViewerContext]string
	viewerRoster map[string]*ViewerContext
}

type ViewerContext struct {
	Socket *websocket.Conn
	Stream *BroadcastContext
}

type ChatMessage struct {
	name string
	text string
}

func NewContext(timeout time.Duration) Context {
	return Context{
		timeout: timeout, streams: make(map[string]*BroadcastContext),
	}
}

// Acquire a stream for writing. Only one "writable" reference can be held;
// until it is released, this function will return an error.
func (ctx *Context) Acquire(id string) (*BroadcastContext, bool) {
	stream, ok := ctx.streams[id]

	if !ok {
		v := BroadcastContext{
			Broadcast:    NewBroadcast(),
			ping:         make(chan int),
			closing:      false,
			chatHistory:  ring.New(20),
			viewers:      make(map[*ViewerContext]string),
			viewerRoster: make(map[string]*ViewerContext),
		}

		ctx.streams[id] = &v
		go ctx.closeOnRelease(id, &v)
		return &v, true
	}

	if !stream.closing {
		return nil, false
	}

	stream.closing = false
	stream.ping <- 1
	return stream, true
}

func (stream *BroadcastContext) Release() {
	stream.closing = true
	stream.ping <- 1
}

// Acquire a stream for reading. There is no limit on the number of concurrent readers.
func (ctx *Context) Get(id string) (*BroadcastContext, bool) {
	stream, ok := ctx.streams[id]
	return stream, ok
}

func (ctx *Context) closeOnRelease(id string, stream *BroadcastContext) {
	for {
		if stream.closing {
			timer := time.NewTimer(ctx.timeout)

			select {
			case <-stream.ping:
				timer.Stop()
			case <-timer.C:
				delete(ctx.streams, id)
				stream.Close()
				return
			}
		} else {
			<-stream.ping
		}
	}
}

func (ctx *ViewerContext) Open() {
	ctx.Stream.viewers[ctx] = ""
}

func (ctx *ViewerContext) SetName(name string) error {
	// TODO check that the name is alphanumeric
	// TODO check that the name is not too long
	if _, ok := ctx.Stream.viewerRoster[name]; ok {
		return errors.New("name already taken")
	}

	ctx.Stream.viewerRoster[name] = ctx
	if oldName, ok := ctx.Stream.viewers[ctx]; ok && oldName != "" {
		delete(ctx.Stream.viewerRoster, oldName)
	}
	ctx.Stream.viewers[ctx] = name
	return nil
}

func (ctx *ViewerContext) SendMessage(text string) error {
	// TODO check that the message is not whitespace-only
	// TODO check that the message is not too long
	name, ok := ctx.Stream.viewers[ctx]
	if !ok || name == "" {
		return errors.New("must obtain a name first")
	}

	msg := ChatMessage{name, text}

	for viewer := range ctx.Stream.viewers {
		viewer.OnMessage(msg)
	}

	ctx.Stream.chatHistory.Value = msg
	ctx.Stream.chatHistory = ctx.Stream.chatHistory.Next()
	return nil
}

func (ctx *ViewerContext) RequestHistory() error {
	r := ctx.Stream.chatHistory

	for i := 0; i < r.Len(); i++ {
		if r.Value != nil {
			ctx.OnMessage(r.Value.(ChatMessage))
		}
		r = r.Next()
	}

	return nil
}

func (ctx *ViewerContext) Close() {
	if name, ok := ctx.Stream.viewers[ctx]; ok {
		if name != "" {
			delete(ctx.Stream.viewerRoster, name)
		}

		delete(ctx.Stream.viewers, ctx)
	}

	ctx.Socket.Close()
}
