package main

import (
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
	closing            bool
	closingStateChange chan bool
	chatters           map[*chatter]int // A hash set. Values are ignored.
	chattersNames      map[string]int   // Same.
	chatHistory        chatMessageQueue
}

type chatter struct {
	name   string
	socket *websocket.Conn
	stream *BroadcastContext
}

type chatMessage struct {
	name string
	text string
}

type chatMessageQueue struct {
	data  []chatMessage
	start int
}

func newChatMessageQueue(size int) chatMessageQueue {
	return chatMessageQueue{make([]chatMessage, 0, size), 0}
}

func (q *chatMessageQueue) Push(x chatMessage) {
	if len(q.data) == cap(q.data) {
		q.data[q.start] = x
		q.start = (q.start + 1) % len(q.data)
	} else {
		q.data = q.data[:len(q.data)+1]
		q.data[len(q.data)-1] = x
	}
}

func (q *chatMessageQueue) Iterate(f func(x chatMessage) error) error {
	for i := 0; i < len(q.data); i++ {
		if err := f(q.data[(i+q.start)%len(q.data)]); err != nil {
			return err
		}
	}
	return nil
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
			Broadcast:          NewBroadcast(),
			closingStateChange: make(chan bool),
			chatters:           make(map[*chatter]int),
			chattersNames:      make(map[string]int),
			chatHistory:        newChatMessageQueue(ctx.ChatHistory),
		}
		ctx.streams[id] = &v
		go ctx.closeOnRelease(id, &v)
		return &v, true
	}

	if !stream.closing {
		return nil, false
	}
	stream.closingStateChange <- false
	return stream, true
}

func (stream *BroadcastContext) Release() {
	stream.closingStateChange <- true
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
			case stream.closing = <-stream.closingStateChange:
				timer.Stop()
			case <-timer.C:
				delete(ctx.streams, id)
				stream.Close()
				return
			}
		} else {
			stream.closing = <-stream.closingStateChange
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

func (ctx *chatter) SetName(args *RPCSingleStringArg, _ *interface{}) error {
	name := args.First
	// TODO check that the name is alphanumeric
	// TODO check that the name is not too long
	if _, ok := ctx.stream.chattersNames[name]; ok {
		return errors.New("name already taken")
	}

	ctx.stream.chattersNames[name] = 0
	if ctx.name != "" {
		delete(ctx.stream.chattersNames, ctx.name)
	}
	ctx.name = name
	return nil
}

func (ctx *chatter) SendMessage(args *RPCSingleStringArg, _ *interface{}) error {
	// TODO check that the message is not whitespace-only
	// TODO check that the message is not too long
	if ctx.name == "" {
		return errors.New("must obtain a name first")
	}

	msg := chatMessage{ctx.name, args.First}
	for viewer := range ctx.stream.chatters {
		viewer.pushMessage(msg)
	}
	ctx.stream.chatHistory.Push(msg)
	return nil
}

func (ctx *chatter) RequestHistory(_ *interface{}, _ *interface{}) error {
	return ctx.stream.chatHistory.Iterate(ctx.pushMessage)
}

func (ctx *chatter) pushMessage(msg chatMessage) error {
	return pushEvent(ctx.socket, "Chat.Message", []interface{}{msg.name, msg.text})
}

func pushEvent(ws *websocket.Conn, name string, args []interface{}) error {
	return websocket.JSON.Send(ws, map[string]interface{}{
		"jsonrpc": "2.0", "method": name, "params": args,
	})
}

func (stream *BroadcastContext) RunRPC(ws *websocket.Conn) {
	chatter := chatter{name: "", socket: ws, stream: stream}

	stream.chatters[&chatter] = 0
	defer func() {
		delete(stream.chatters, &chatter)

		if chatter.name != "" {
			delete(stream.chattersNames, chatter.name)
		}
	}()

	server := rpc.NewServer()
	server.RegisterName("Chat", &chatter)
	server.ServeCodec(jsonrpc2.NewServerCodec(ws, server))
}
