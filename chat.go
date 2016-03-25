package main

import (
	"errors"
	"golang.org/x/net/websocket"
	"strings"
	"unicode"
)

type ChatMessage struct {
	name string
	text string
}

type ChatMessageQueue struct {
	data  []ChatMessage
	start int
}

type ChatContext struct {
	Users   map[*ChatterContext]int // A hash set. Values are ignored.
	Names   map[string]int          // Same
	History ChatMessageQueue
}

type ChatterContext struct {
	name   string
	socket *websocket.Conn
	chat   *ChatContext
}

func NewChat(qsize int) ChatContext {
	return ChatContext{
		Users:   make(map[*ChatterContext]int),
		Names:   make(map[string]int),
		History: ChatMessageQueue{make([]ChatMessage, 0, qsize), 0},
	}
}

func (q *ChatMessageQueue) Push(x ChatMessage) {
	if len(q.data) == cap(q.data) {
		q.data[q.start] = x
		q.start = (q.start + 1) % len(q.data)
	} else {
		q.data = q.data[:len(q.data)+1]
		q.data[len(q.data)-1] = x
	}
}

func (q *ChatMessageQueue) Iterate(f func(x ChatMessage) error) error {
	for i := 0; i < len(q.data); i++ {
		if err := f(q.data[(i+q.start)%len(q.data)]); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChatContext) Connect(ws *websocket.Conn) *ChatterContext {
	chatter := ChatterContext{name: "", socket: ws, chat: c}
	// XXX maybe send a reference into a channel from which `monitor` would read?
	c.Users[&chatter] = 0
	for u := range c.Users {
		u.pushViewerCount()
	}
	return &chatter
}

func (c *ChatContext) Disconnect(u *ChatterContext) {
	delete(c.Users, u)
	if u.name != "" {
		delete(c.Names, u.name)
	}
	for v := range c.Users {
		v.pushViewerCount()
	}
}

func (c *ChatContext) Close() {
	for u := range c.Users {
		u.socket.Close()
	}
}

func (ctx *ChatterContext) SetName(args *RPCSingleStringArg, _ *interface{}) error {
	name := strings.TrimSpace(args.First)
	if len(name) == 0 {
		return errors.New("name must not be empty")
	}
	if len(name) > 32 {
		return errors.New("name too long")
	}
	for _, c := range name {
		if !unicode.IsGraphic(c) {
			return errors.New("name contains invalid characters")
		}
	}
	if _, ok := ctx.chat.Names[name]; ok {
		return errors.New("name already taken")
	}
	ctx.chat.Names[name] = 0
	if ctx.name != "" {
		delete(ctx.chat.Names, ctx.name)
	}
	ctx.name = name
	return nil
}

func (ctx *ChatterContext) SendMessage(args *RPCSingleStringArg, _ *interface{}) error {
	if ctx.name == "" {
		return errors.New("must obtain a name first")
	}
	msg := ChatMessage{ctx.name, strings.TrimSpace(args.First)}
	if len(msg.text) == 0 || len(msg.text) > 256 {
		return errors.New("message must have between 1 and 256 characters")
	}
	for u := range ctx.chat.Users {
		u.pushMessage(msg)
	}
	ctx.chat.History.Push(msg)
	return nil
}

func (ctx *ChatterContext) RequestHistory(_ *interface{}, _ *interface{}) error {
	return ctx.chat.History.Iterate(ctx.pushMessage)
}

func (ctx *ChatterContext) pushMessage(msg ChatMessage) error {
	return RPCPushEvent(ctx.socket, "Chat.Message", []interface{}{msg.name, msg.text})
}

func (ctx *ChatterContext) pushViewerCount() error {
	return RPCPushEvent(ctx.socket, "Stream.ViewerCount", []interface{}{len(ctx.chat.Users)})
}
