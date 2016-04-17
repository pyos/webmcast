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
	events  chan interface{}
	Users   map[*ChatterContext]int // A hash set. Values are ignored.
	Names   map[string]int          // Same
	History ChatMessageQueue
}

type ChatterContext struct {
	name   string
	socket *websocket.Conn
	chat   *ChatContext
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
	// this should be safe to use without a mutex. at worst, pushing more than
	// `cap(q.data)` messages while iterating may result in skipping over some of them.
	for i, s, n := 0, q.start, len(q.data); i < n; i++ {
		if err := f(q.data[(i+s)%n]); err != nil {
			return err
		}
	}
	return nil
}

func NewChat(qsize int) *ChatContext {
	ctx := &ChatContext{
		events:  make(chan interface{}),
		Users:   make(map[*ChatterContext]int),
		Names:   make(map[string]int),
		History: ChatMessageQueue{make([]ChatMessage, 0, qsize), 0},
	}
	go ctx.handle()
	return ctx
}

type chatSetNameEvent struct {
	user *ChatterContext
	name string
}

func (c *ChatContext) handle() {
	closed := false
	for genericEvent := range c.events {
		switch event := genericEvent.(type) {
		case nil:
			closed = true
			for u := range c.Users {
				u.socket.Close()
			}
			if len(c.Users) == 0 {
				return // else must handle pending events first
			}

		case *ChatterContext:
			if _, exists := c.Users[event]; exists {
				delete(c.Users, event)
				if event.name != "" {
					delete(c.Names, event.name)
				}
				if closed && len(c.Users) == 0 {
					return // if these events were left unhandled, senders would block forever
				}
			} else {
				c.Users[event] = 0
			}
			for u := range c.Users {
				u.pushViewerCount()
			}

		case chatSetNameEvent:
			if _, ok := c.Names[event.name]; ok {
				// XXX should push an error notification, maybe?..
				continue
			}
			c.Names[event.name] = 0
			if event.user.name != "" {
				delete(c.Names, event.user.name)
			}
			event.user.name = event.name
			event.user.pushName(event.name)

		case ChatMessage:
			c.History.Push(event)
			for u := range c.Users {
				u.pushMessage(event)
			}
		}
	}
}

func (c *ChatContext) Connect(ws *websocket.Conn) *ChatterContext {
	chatter := &ChatterContext{name: "", socket: ws, chat: c}
	c.events <- chatter
	return chatter
}

func (c *ChatContext) Disconnect(u *ChatterContext) {
	c.events <- u
}

func (c *ChatContext) Close() {
	c.events <- nil
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
	ctx.chat.events <- chatSetNameEvent{ctx, name}
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
	ctx.chat.events <- msg
	return nil
}

func (ctx *ChatterContext) RequestHistory(_ *interface{}, _ *interface{}) error {
	return ctx.chat.History.Iterate(ctx.pushMessage)
}

func (ctx *ChatterContext) pushName(name string) error {
	return RPCPushEvent(ctx.socket, "Chat.AcquiredName", []interface{}{name})
}

func (ctx *ChatterContext) pushMessage(msg ChatMessage) error {
	return RPCPushEvent(ctx.socket, "Chat.Message", []interface{}{msg.name, msg.text})
}

func (ctx *ChatterContext) pushViewerCount() error {
	return RPCPushEvent(ctx.socket, "Stream.ViewerCount", []interface{}{len(ctx.chat.Users)})
}
