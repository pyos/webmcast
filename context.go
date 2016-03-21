package main

import (
	"container/ring"
	"encoding/json"
	"errors"
	"github.com/powerman/rpc-codec/jsonrpc2"
	"golang.org/x/net/websocket"
	"net/rpc"
	"time"
)

type Context struct {
	// There is a timeout after releasing a stream during which it is possible
	// to reacquire the same object and continue broadcasting. Once the timeout
	// elapses, the stream is closed for good.
	Timeout time.Duration
	// How many messages to transmit on a `Chat.RequestHistory` RPC call.
	ChatHistory int

	streams map[string]*BroadcastContext
}

type BroadcastContext struct {
	Broadcast

	closing             bool
	closingStateChanged chan bool

	chatHistory *ring.Ring // of `chatMessage`
	chatViewers map[*chatViewer]interface{}
	chatRoster  map[string]*chatViewer
}

type chatViewer struct {
	name   string
	socket *websocket.Conn
	stream *BroadcastContext
}

type chatMessage struct {
	name string
	text string
}

// Acquire a stream for writing. Only one "writable" reference can be held;
// until it is released, this function will return an error.
func (ctx *Context) Acquire(id string) (*BroadcastContext, bool) {
	if ctx.streams == nil {
		ctx.streams = make(map[string]*BroadcastContext)
	}
	stream, ok := ctx.streams[id]

	if !ok {
		v := BroadcastContext{
			Broadcast: NewBroadcast(),

			closing:             false,
			closingStateChanged: make(chan bool),

			chatHistory: ring.New(ctx.ChatHistory),
			chatViewers: make(map[*chatViewer]interface{}),
			chatRoster:  make(map[string]*chatViewer),
		}

		ctx.streams[id] = &v
		go ctx.closeOnRelease(id, &v)
		return &v, true
	}

	if !stream.closing {
		return nil, false
	}

	stream.closingStateChanged <- false
	return stream, true
}

func (stream *BroadcastContext) Release() {
	stream.closingStateChanged <- true
}

// Acquire a stream for reading. There is no limit on the number of concurrent readers.
func (ctx *Context) Get(id string) (*BroadcastContext, bool) {
	if ctx.streams == nil {
		return nil, false
	}
	stream, ok := ctx.streams[id]
	return stream, ok
}

func (ctx *Context) closeOnRelease(id string, stream *BroadcastContext) {
	for {
		if stream.closing {
			timer := time.NewTimer(ctx.Timeout)

			select {
			case stream.closing = <-stream.closingStateChanged:
				timer.Stop()
			case <-timer.C:
				delete(ctx.streams, id)
				stream.Close()
				return
			}
		} else {
			stream.closing = <-stream.closingStateChanged
		}
	}
}

type RPCSingleStringArg struct {
	First string
}

func (x *RPCSingleStringArg) UnmarshalJSON(buf []byte) error {
	fields := []interface{}{&x.First}
	expect := len(fields)
	if err := json.Unmarshal(buf, &fields); err != nil {
		return err
	}
	if len(fields) != expect {
		return errors.New("invalid number of arguments")
	}
	return nil
}

func (ctx *chatViewer) SetName(args *RPCSingleStringArg, _ *interface{}) error {
	name := args.First
	// TODO check that the name is alphanumeric
	// TODO check that the name is not too long
	if _, ok := ctx.stream.chatRoster[name]; ok {
		return errors.New("name already taken")
	}

	ctx.stream.chatRoster[name] = ctx
	if ctx.name != "" {
		delete(ctx.stream.chatRoster, ctx.name)
	}
	ctx.name = name
	return nil
}

func (ctx *chatViewer) SendMessage(args *RPCSingleStringArg, _ *interface{}) error {
	// TODO check that the message is not whitespace-only
	// TODO check that the message is not too long
	if ctx.name == "" {
		return errors.New("must obtain a name first")
	}

	msg := chatMessage{ctx.name, args.First}

	for viewer := range ctx.stream.chatViewers {
		viewer.onMessage(msg)
	}

	ctx.stream.chatHistory.Value = msg
	ctx.stream.chatHistory = ctx.stream.chatHistory.Next()
	return nil
}

func (ctx *chatViewer) RequestHistory(_ *interface{}, _ *interface{}) error {
	r := ctx.stream.chatHistory

	for i := 0; i < r.Len(); i++ {
		if r.Value != nil {
			ctx.onMessage(r.Value.(chatMessage))
		}
		r = r.Next()
	}

	return nil
}

func (ctx *chatViewer) onEvent(name string, args []interface{}) error {
	return websocket.JSON.Send(ctx.socket, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  name,
		"params":  args,
	})
}

func (ctx *chatViewer) onMessage(msg chatMessage) {
	ctx.onEvent("Chat.Message", []interface{}{msg.name, msg.text})
}

func (stream *BroadcastContext) RunRPC(ws *websocket.Conn) {
	chatter := chatViewer{name: "", socket: ws, stream: stream}

	stream.chatViewers[&chatter] = nil
	defer func() {
		delete(stream.chatViewers, &chatter)

		if chatter.name != "" {
			delete(stream.chatRoster, chatter.name)
		}
	}()

	server := rpc.NewServer()
	server.RegisterName("Chat", &chatter)
	server.ServeCodec(jsonrpc2.NewServerCodec(ws, server))
}
