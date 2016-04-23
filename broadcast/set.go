package broadcast

import (
	"sync"
	"time"
)

type Set struct {
	mutex   sync.Mutex // protects `streams`
	streams map[string]*SetItem
	// How long to keep a stream alive after a call to `Close`.
	Timeout time.Duration
	// Called when the stream actually is actually closed (<=> timeout has elapsed.)
	OnStreamClose func(id string)
}

type SetItem struct {
	Broadcast
	Created time.Time
	// The stream will close when this reaches `Set.Timeout`.
	closing time.Duration
	// These values are for the whole stream, so they include audio and muxing overhead.
	// The latter is negligible, however, and the former is normally about 64k,
	// so also negligible. Or at least predictable.
	Rate struct {
		unit float64
		Mean float64
		Var  float64
	}
}

func (ctx *Set) Readable(id string) (*SetItem, bool) {
	if ctx.streams == nil {
		return nil, false
	}
	ctx.mutex.Lock()
	stream, ok := ctx.streams[id]
	ctx.mutex.Unlock()
	return stream, ok
}

func (ctx *Set) Writable(id string) (*SetItem, bool) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	if ctx.streams == nil {
		ctx.streams = make(map[string]*SetItem)
	}
	if stream, ok := ctx.streams[id]; ok {
		if stream.closing == -1 {
			return nil, false
		}
		stream.closing = -1
		return stream, true
	}
	stream := &SetItem{
		Broadcast: NewBroadcast(),
		Created:   time.Now().UTC(),
	}
	ctx.streams[id] = stream
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			if stream.closing >= 0 {
				if stream.closing += time.Second; stream.closing > ctx.Timeout {
					ctx.mutex.Lock()
					delete(ctx.streams, id)
					ctx.mutex.Unlock()
					ticker.Stop()
					stream.Broadcast.Close()
					if ctx.OnStreamClose != nil {
						ctx.OnStreamClose(id)
					}
				}
			}
			// exponentially weighted moving moments at a = 0.5
			//     avg[n] = a * x + (1 - a) * avg[n - 1]
			//     var[n] = a * (x - avg[n]) ** 2 / (1 - a) + (1 - a) * var[n - 1]
			stream.Rate.Mean += stream.Rate.unit / 2
			stream.Rate.Var += stream.Rate.unit*stream.Rate.unit - stream.Rate.Var/2
			stream.Rate.unit = -stream.Rate.Mean
		}
	}()
	return stream, true
}

func (stream *SetItem) Write(data []byte) (int, error) {
	stream.Rate.unit += float64(len(data))
	return stream.Broadcast.Write(data)
}

func (stream *SetItem) Close() error {
	stream.closing = 0
	return nil
}
