package main

import (
	"time"
)

type BroadcastContext struct {
	Broadcast
	// There is a timeout after releasing a stream during which it is possible
	// to reacquire the same object and continue broadcasting. Once the timeout
	// elapses, the stream is closed for good.
	ping    chan int
	closing bool
}

type Context struct {
	streams map[string]*BroadcastContext
	timeout time.Duration
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
			Broadcast: NewBroadcast(), ping: make(chan int), closing: false,
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
