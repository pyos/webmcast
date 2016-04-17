package main

import (
	"errors"
	"golang.org/x/net/websocket"
	"strings"
	"sync"
	"unicode"
)

type ChatMessage struct {
	name string
	text string
}

type ChatMessageQueue struct {
	mutex sync.RWMutex
	data  []ChatMessage
	start int
}

type ChatContext struct {
	mutex   sync.RWMutex            // protects `Users` and `Names`
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
		History: ChatMessageQueue{data: make([]ChatMessage, 0, qsize), start: 0},
	}
}

func (q *ChatMessageQueue) Push(x ChatMessage) {
	q.mutex.Lock()
	if len(q.data) == cap(q.data) {
		q.data[q.start] = x
		q.start = (q.start + 1) % len(q.data)
	} else {
		q.data = q.data[:len(q.data)+1]
		q.data[len(q.data)-1] = x
	}
	q.mutex.Unlock()
}

func (q *ChatMessageQueue) Iterate(f func(x ChatMessage) error) error {
	q.mutex.RLock()
	for i := 0; i < len(q.data); i++ {
		if err := f(q.data[(i+q.start)%len(q.data)]); err != nil {
			q.mutex.RUnlock()
			return err
		}
	}
	q.mutex.RUnlock()
	return nil
}

func (c *ChatContext) Connect(ws *websocket.Conn) *ChatterContext {
	chatter := &ChatterContext{name: "", socket: ws, chat: c}
	// XXX maybe send a reference into a channel from which `monitor` would read?
	c.mutex.Lock()
	c.Users[chatter] = 0
	c.mutex.Unlock()
	c.mutex.RLock()
	for u := range c.Users {
		u.pushViewerCount()
	}
	c.mutex.RUnlock()
	return chatter
}

func (c *ChatContext) Disconnect(u *ChatterContext) {
	c.mutex.Lock()
	delete(c.Users, u)
	if u.name != "" {
		delete(c.Names, u.name)
	}
	c.mutex.Unlock()
	c.mutex.RLock()
	for v := range c.Users {
		v.pushViewerCount()
	}
	c.mutex.RUnlock()
}

func (c *ChatContext) Close() {
	c.mutex.RLock()
	for u := range c.Users {
		u.socket.Close()
	}
	c.mutex.RUnlock()
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
	ctx.chat.mutex.Lock()
	if _, ok := ctx.chat.Names[name]; ok {
		ctx.chat.mutex.Unlock()
		return errors.New("name already taken")
	}
	ctx.chat.Names[name] = 0
	if ctx.name != "" {
		delete(ctx.chat.Names, ctx.name)
	}
	ctx.chat.mutex.Unlock()
	ctx.name = name
	ctx.pushName(name)
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
	ctx.chat.mutex.RLock()
	for u := range ctx.chat.Users {
		u.pushMessage(msg)
	}
	ctx.chat.mutex.RUnlock()
	ctx.chat.History.Push(msg)
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
