package main

import (
	"errors"
	"golang.org/x/net/websocket"
	"strings"
	"unicode"
)

type ChatMessage struct {
	name   string
	login  string
	text   string
	authed bool
}

type ChatMessageQueue struct {
	data  []ChatMessage
	start int
}

type ChatContext struct {
	events  chan interface{}
	Users   map[*ChatterContext]int // A hash set. Values are ignored.
	Names   map[string]*ChatterContext
	History ChatMessageQueue
}

type ChatterContext struct {
	name   string
	login  string
	authed bool
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
		Names:   make(map[string]*ChatterContext),
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
				if event.login != "" {
					delete(c.Names, event.login)
				}
				if closed && len(c.Users) == 0 {
					return // if these events were left unhandled, senders would block forever
				}
			} else {
				c.Users[event] = 0
				if event.login != "" {
					if old, exists := c.Names[event.login]; exists {
						old.name = ""
						old.login = ""
						old.pushName("", "")
					}
					c.Names[event.login] = event
					event.pushName(event.name, event.login)
				}
			}
			for u := range c.Users {
				u.pushViewerCount()
			}

		case chatSetNameEvent:
			if _, ok := c.Names[event.name]; ok {
				event.user.pushName(event.user.name, event.user.login)
				continue
			}
			c.Names[event.name] = event.user
			if event.user.login != "" {
				delete(c.Names, event.user.login)
			}
			event.user.name = event.name
			event.user.login = event.name
			event.user.pushName(event.name, event.name)

		case ChatMessage:
			c.History.Push(event)
			for u := range c.Users {
				u.pushMessage(event)
			}
		}
	}
}

func (c *ChatContext) Connect(ws *websocket.Conn, auth *UserShortData) *ChatterContext {
	chatter := &ChatterContext{socket: ws, chat: c}
	if auth != nil {
		chatter.name = auth.Name
		chatter.login = auth.Login
		chatter.authed = true
	}
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
	if ctx.login == "" {
		return errors.New("must obtain a name first")
	}
	msg := ChatMessage{ctx.name, ctx.login, strings.TrimSpace(args.First), ctx.authed}
	if len(msg.text) == 0 || len(msg.text) > 256 {
		return errors.New("message must have between 1 and 256 characters")
	}
	ctx.chat.events <- msg
	return nil
}

func (ctx *ChatterContext) RequestHistory(_ *interface{}, _ *interface{}) error {
	return ctx.chat.History.Iterate(ctx.pushMessage)
}

func (ctx *ChatterContext) pushName(name, login string) error {
	return RPCPushEvent(ctx.socket, "Chat.AcquiredName", []interface{}{name, login})
}

func (ctx *ChatterContext) pushMessage(msg ChatMessage) error {
	return RPCPushEvent(ctx.socket, "Chat.Message",
		[]interface{}{msg.name, msg.text, msg.login, msg.authed})
}

func (ctx *ChatterContext) pushViewerCount() error {
	return RPCPushEvent(ctx.socket, "Stream.ViewerCount", []interface{}{len(ctx.chat.Users)})
}
