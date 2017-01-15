package main

import (
	"errors"
	"sync"
	"time"
)

const (
	// a special value for a tag's length than means "until the next tag of same level".
	ebmlIndeterminate = 0xFFFFFFFFFFFFFF
	// some of the possible tag ids.
	// https://www.matroska.org/technical/specs/index.html
	ebmlTagVoid            = 0xEC
	ebmlTagEBML            = 0x1A45DFA3
	ebmlTagSegment         = 0x18538067
	ebmlTagSeekHead        = 0x114D9B74
	ebmlTagInfo            = 0x1549A966
	ebmlTagTimecodeScale   = 0x2AD7B1
	ebmlTagDuration        = 0x4489
	ebmlTagDateUTC         = 0x4461
	ebmlTagMuxingApp       = 0x4D80
	ebmlTagWritingApp      = 0x5741
	ebmlTagTracks          = 0x1654AE6B
	ebmlTagTrackEntry      = 0xAE
	ebmlTagTrackNumber     = 0xD7
	ebmlTagTrackUID        = 0x73C5
	ebmlTagTrackType       = 0x83
	ebmlTagFlagEnabled     = 0xB9
	ebmlTagFlagDefault     = 0x88
	ebmlTagFlagForced      = 0x55AA
	ebmlTagFlagLacing      = 0x9C
	ebmlTagDefaultDuration = 0x23E383
	ebmlTagName            = 0x536E
	ebmlTagCodecID         = 0x86
	ebmlTagCodecName       = 0x228688
	ebmlTagVideo           = 0xE0
	ebmlTagPixelWidth      = 0xB0
	ebmlTagPixelHeight     = 0xBA
	ebmlTagAudio           = 0xE1
	ebmlTagCluster         = 0x1F43B675
	ebmlTagTimecode        = 0xE7
	ebmlTagPrevSize        = 0xAB
	ebmlTagSimpleBlock     = 0xA3
	ebmlTagBlockGroup      = 0xA0
	ebmlTagBlock           = 0xA1
	ebmlTagBlockDuration   = 0x9B
	ebmlTagReferenceBlock  = 0xFB
	ebmlTagDiscardPadding  = 0x75A2
	ebmlTagCues            = 0x1C53BB6B
	ebmlTagChapters        = 0x1043A770
	ebmlTagTags            = 0x1254C367
	ebmlTagTag             = 0x7373
	ebmlTagTargets         = 0x63C0
	ebmlTagTargetType      = 0x63CA
	ebmlTagTagTrackUID     = 0x63C5
	ebmlTagSimpleTag       = 0x67C8
	ebmlTagTagName         = 0x45A3
	ebmlTagTagLanguage     = 0x447A
	ebmlTagTagDefault      = 0x4484
	ebmlTagTagString       = 0x4487
	ebmlTagTagBinary       = 0x4485
)

var ebmlIndeterminateCoding = [...]uint64{
	0, // these values in the "length" field all decode to `ebmlIndeterminate`.
	0xFF,
	0x7FFF,
	0x3FFFFF,
	0x1FFFFFFF,
	0x0FFFFFFFFF,
	0x07FFFFFFFFFF,
	0x03FFFFFFFFFFFF,
	0x01FFFFFFFFFFFFFF,
}

func fixedUint(data []byte) uint64 {
	var x uint64 = 0
	for _, b := range data {
		x = x<<8 | uint64(b)
	}
	return x
}

func ebmlTagID(data []byte) (uint64, int) {
	if len(data) != 0 && data[0] != 0 {
		// 1xxxxxxx
		// 01xxxxxx xxxxxxxx
		// 001xxxxx xxxxxxxx xxxxxxxx
		// ...
		// 00000001 xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
		//        ^---- this length marker is included in tag ids but not in other ints
		consumed := 9
		for b := data[0]; b != 0; b >>= 1 {
			consumed -= 1
		}
		if len(data) >= consumed {
			return fixedUint(data[:consumed]), consumed
		}
	}
	return 0, 0
}

func ebmlUint(data []byte) (uint64, int) {
	id, consumed := ebmlTagID(data)
	if ebmlIndeterminateCoding[consumed] == id {
		return ebmlIndeterminate, consumed
	}
	return id & ^(1 << uint(7*consumed)), consumed
}

type ebmlTag struct {
	Consumed int
	ID       uint
	Length   uint64
}

func ebmlParseTagIncomplete(data []byte) ebmlTag {
	if id, off := ebmlTagID(data); off != 0 {
		if length, off2 := ebmlUint(data[off:]); off2 != 0 {
			return ebmlTag{off + off2, uint(id), length}
		}
	}
	return ebmlTag{0, 0, 0}
}

func ebmlParseTag(data []byte) ebmlTag {
	if tag := ebmlParseTagIncomplete(data); tag.Length+uint64(tag.Consumed) <= uint64(len(data)) {
		return tag
	}
	return ebmlTag{0, 0, 0}
}

func (t ebmlTag) Contents(data []byte) []byte {
	return data[t.Consumed : uint64(t.Consumed)+t.Length]
}

func (t ebmlTag) Skip(data []byte) []byte {
	return data[uint64(t.Consumed)+t.Length:]
}

type frame struct {
	buf   []byte // Either a Block(Group) or a Cluster.
	track uint64 // 64 for a Cluster (track masks are 32-bit, so streams with a real 64-th track are rejected)
	key   bool
}

type framebuffer struct {
	data        []frame
	start       int
	headCluster []byte // Last Cluster popped off the ring. Blocks at the head still belong to it.
}

func (fb *framebuffer) PushCluster(buf []byte) {
	fb.PushFrame(frame{buf, 64, false})
}

func (fb *framebuffer) PushFrame(packed frame) {
	if len(fb.data) == cap(fb.data) {
		if fb.data[fb.start].track == 64 {
			fb.headCluster = fb.data[fb.start].buf
		}
		fb.data[fb.start] = packed
		fb.start = (fb.start + 1) % len(fb.data)
	} else {
		fb.data = append(fb.data, packed)
	}
}

func (fb *framebuffer) Read(cb func(cluster []byte, forceCluster bool, packed frame)) {
	cluster, forceCluster := fb.headCluster, true
	for i, s, n := 0, fb.start, len(fb.data); i < n; i++ {
		f := fb.data[(i+s)%n]
		if f.track == 64 {
			cluster = f.buf
			forceCluster = true
		} else if cluster != nil {
			cb(cluster, forceCluster, f)
			forceCluster = false
		}
	}
}

type viewer struct {
	// This function may return `false` to signal that it cannot write any more data.
	// The stream will resynchronize at next keyframe.
	write func(data []byte) bool
	// Viewers may hop between streams, but should only receive headers once.
	// This includes track info, as codecs must stay the same between segments.
	skipHeaders bool
	// We group blocks into indeterminate-length clusters. So long as
	// the cluster's timecode has not changed, there's no need to start a new one.
	skipCluster bool
	// Bit vector of tracks for which the viewer has both reference frames
	// (the previous frame and the last keyframe.)
	seenKeyframes uint32
}

func (cb *viewer) WriteFrame(cluster []byte, forceCluster bool, packed frame) {
	trackMask := uint32(1) << packed.track
	if forceCluster {
		cb.skipCluster = false
	}
	if packed.key {
		cb.seenKeyframes |= trackMask
	}
	if cb.seenKeyframes&trackMask != 0 {
		if !cb.skipCluster {
			cb.skipCluster = cb.write(cluster)
		}
		if !cb.skipCluster || !cb.write(packed.buf) {
			cb.seenKeyframes &= ^trackMask
		}
	}
}

type BroadcastSet struct {
	mutex   sync.Mutex
	streams map[string]*Broadcast
	// How long to keep a stream alive after a call to `Close`.
	Timeout time.Duration
	// Called right after a stream is destroyed. (`Timeout` seconds after a `Close`.)
	OnStreamClose     func(id string)
	OnStreamTrackInfo func(id string, info *StreamTrackInfo)
}

type Broadcast struct {
	StreamTrackInfo
	closing time.Duration
	Closed  bool
	dirty   bool // (Has unseen data in `StreamTrackInfo`.)
	buffer  []byte
	header  []byte // The EBML (DocType) tag.
	tracks  []byte // The beginning of the Segment (Tracks + Info).
	frames  framebuffer
	// outbound clusters must have monotonically increasing timecodes even if the inbound
	// stream restarts from the beginning.
	firstBlockInSegment bool
	sentTimecode        uint64
	sentClusterTimecode uint64
	recvClusterTimecode uint64
	timecodeShift       uint64
	// these values are for the whole stream, so they include audio and muxing overhead.
	// the latter is negligible, however, and the former is normally about 64k,
	// so also negligible. or at least predictable.
	rateUnit float64
	RateMean float64
	RateVar  float64

	vlock   sync.Mutex
	viewers map[chan<- []byte]*viewer
}

func (ctx *BroadcastSet) Readable(id string) (*Broadcast, bool) {
	if ctx.streams == nil {
		return nil, false
	}
	ctx.mutex.Lock()
	cast, ok := ctx.streams[id]
	ctx.mutex.Unlock()
	return cast, ok
}

func (ctx *BroadcastSet) Writable(id string) (*Broadcast, bool) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	if ctx.streams == nil {
		ctx.streams = make(map[string]*Broadcast)
	}
	if cast, ok := ctx.streams[id]; ok {
		if cast.closing == -1 {
			return nil, false
		}
		cast.closing = -1
		return cast, true
	}
	cast := Broadcast{
		closing:             -1,
		frames:              framebuffer{make([]frame, 0, 120), 0, nil},
		viewers:             make(map[chan<- []byte]*viewer),
		sentClusterTimecode: 0xFFFFFFFFFFFFFFFF,
	}
	ctx.streams[id] = &cast
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			if cast.dirty {
				cast.dirty = false
				ctx.OnStreamTrackInfo(id, &cast.StreamTrackInfo)
			}
			if cast.closing >= 0 {
				if cast.closing += time.Second; cast.closing > ctx.Timeout {
					break
				}
			}
			// exponentially weighted moving moments at a = 0.5
			//     avg[n] = a * x + (1 - a) * avg[n - 1]
			//     var[n] = a * (x - avg[n]) ** 2 / (1 - a) + (1 - a) * var[n - 1]
			cast.RateMean += cast.rateUnit / 2
			cast.RateVar += cast.rateUnit*cast.rateUnit - cast.RateVar/2
			cast.rateUnit = -cast.RateMean
		}
		ticker.Stop()

		ctx.mutex.Lock()
		delete(ctx.streams, id)
		ctx.mutex.Unlock()
		cast.Closed = true
		cast.vlock.Lock()
		for _, cb := range cast.viewers {
			cb.write([]byte{})
		}
		cast.vlock.Unlock()
		if ctx.OnStreamClose != nil {
			ctx.OnStreamClose(id)
		}
	}()
	return &cast, true
}

func (cast *Broadcast) Close() error {
	cast.closing = 0
	return nil
}

func (cast *Broadcast) Connect(ch chan<- []byte, skipHeaders bool) {
	blocked := false
	write := func(data []byte) bool {
		// `Broadcast.Write` emits data in block-sized chunks.
		// Thus the buffer size is measured in frames, not bytes.
		blocked = len(ch) == cap(ch) || (blocked && len(ch)*2 >= cap(ch))
		if !blocked {
			ch <- data
		}
		return !blocked
	}

	cast.vlock.Lock()
	cast.viewers[ch] = &viewer{write: write}
	cast.vlock.Unlock()
}

func (cast *Broadcast) Disconnect(ch chan<- []byte) {
	cast.vlock.Lock()
	delete(cast.viewers, ch)
	cast.vlock.Unlock()
}

func (cast *Broadcast) Reset() {
	cast.buffer = nil
}

func (cast *Broadcast) Write(data []byte) (int, error) {
	cast.rateUnit += float64(len(data))
	cast.buffer = append(cast.buffer, data...)

	for {
		buf := cast.buffer
		tag := ebmlParseTagIncomplete(buf)
		if tag.Consumed == 0 {
			return len(data), nil
		}

		if tag.ID == ebmlTagSegment || tag.ID == ebmlTagTracks || tag.ID == ebmlTagCluster {
			// Parse the contents of these tags in the same loop.
			buf = buf[:tag.Consumed]
		} else {
			if tag.Length == ebmlIndeterminate {
				return 0, errors.New("exact length required for all tags but Segments and Clusters")
			}
			total := tag.Length + uint64(tag.Consumed)
			if total > 1024*1024 {
				return 0, errors.New("data block too big")
			}

			if total > uint64(len(buf)) {
				return len(data), nil
			}

			buf = buf[:total]
		}

		switch tag.ID {
		case ebmlTagSeekHead:
			// Disallow seeking.
		case ebmlTagChapters:
			// Disallow seeking again.
		case ebmlTagCues:
			// Disallow even more seeking.
		case ebmlTagVoid:
			// Waste of space.
		case ebmlTagTags:
			// Maybe later.
		case ebmlTagCluster:
			// Ignore boundaries, we'll regroup the data anyway.
		case ebmlTagPrevSize:
			// Disallow backward seeking too.

		case ebmlTagEBML:
			// The header is the same in all WebM-s.
			if len(cast.header) == 0 {
				cast.header = append([]byte{}, buf...)
			}

		case ebmlTagSegment:
			cast.StreamTrackInfo = StreamTrackInfo{}
			// Always reset length to indeterminate.
			cast.tracks = append([]byte{}, buf[0], buf[1], buf[2], buf[3], 0xFF)
			// Will recalculate this when the first block arrives.
			cast.timecodeShift = 0
			cast.firstBlockInSegment = true

		case ebmlTagInfo:
			// Default timecode resolution in Matroska is 1 ms. This value is required
			// in WebM; we'll check just in case. Obviously, our timecode rewriting
			// logic won't work with non-millisecond resolutions.
			scale := uint64(0)

			for buf2 := tag.Contents(buf); len(buf2) != 0; {
				tag2 := ebmlParseTag(buf2)

				switch tag2.ID {
				case 0:
					return 0, errors.New("malformed EBML")

				case ebmlTagDuration:
					total := tag2.Length + uint64(tag2.Consumed) - 2
					if total > 0x7F {
						// I'd rather avoid shifting memory. What kind of integer
						// needs 128 bytes, anyway?
						return 0, errors.New("EBML Duration too large")
					}
					// Live streams must not have a duration.
					buf2[0] = ebmlTagVoid
					buf2[1] = 0x80 | byte(total)

				case ebmlTagTimecodeScale:
					scale = fixedUint(tag2.Contents(buf2))
				}

				buf2 = tag2.Skip(buf2)
			}

			if scale != 1000000 {
				return 0, errors.New("invalid timecode scale")
			}

			cast.tracks = append(cast.tracks, buf...)

		case ebmlTagTrackEntry:
			// Since `viewer.seenKeyframes` is a 32-bit vector,
			// we need to check that there are at most 32 tracks.
			for buf2 := tag.Contents(buf); len(buf2) != 0; {
				tag2 := ebmlParseTag(buf2)

				switch tag2.ID {
				case 0:
					return 0, errors.New("malformed EBML")

				case ebmlTagTrackNumber:
					// go needs sizeof.
					if fixedUint(tag2.Contents(buf2)) >= 32 {
						return 0, errors.New("too many tracks?")
					}

				case ebmlTagAudio:
					cast.HasAudio = true

				case ebmlTagVideo:
					cast.HasVideo = true
					// While we're here, let's grab some metadata, too.
					for buf3 := tag2.Contents(buf2); len(buf3) != 0; {
						tag3 := ebmlParseTag(buf3)

						switch tag3.ID {
						case 0:
							return 0, errors.New("malformed EBML")

						case ebmlTagPixelWidth:
							cast.Width = uint(fixedUint(tag3.Contents(buf3)))

						case ebmlTagPixelHeight:
							cast.Height = uint(fixedUint(tag3.Contents(buf3)))
						}

						buf3 = tag3.Skip(buf3)
					}
				}

				buf2 = tag2.Skip(buf2)
			}

			cast.tracks = append(cast.tracks, buf...)
			cast.dirty = true

		case ebmlTagTracks:
			cast.tracks = append(cast.tracks, buf...)

		case ebmlTagTimecode:
			cast.recvClusterTimecode = fixedUint(tag.Contents(buf)) + cast.timecodeShift

		case ebmlTagBlockGroup, ebmlTagSimpleBlock:
			key := false
			block := tag.Contents(buf)

			if tag.ID == ebmlTagBlockGroup {
				key, block = true, nil

				for buf2 := tag.Contents(buf); len(buf2) != 0; {
					tag2 := ebmlParseTag(buf2)

					switch tag2.ID {
					case 0:
						return 0, errors.New("malformed EBML")

					case ebmlTagBlock:
						block = tag2.Contents(buf2)

					case ebmlTagReferenceBlock:
						// Keyframes, by definition, have no reference frame.
						key = fixedUint(tag2.Contents(buf2)) == 0
					}

					buf2 = tag2.Skip(buf2)
				}

				if block == nil {
					return 0, errors.New("a BlockGroup contains no Blocks")
				}
			}

			track, consumed := ebmlUint(block)
			if consumed == 0 || track >= 32 || len(block) < consumed+3 {
				return 0, errors.New("invalid track")
			}
			// This bit is always 0 in a Block, but 1 in a keyframe SimpleBlock.
			key = key || block[consumed+2]&0x80 != 0
			// Block timecodes are relative to cluster ones.
			timecode := uint64(block[consumed+0])<<8 | uint64(block[consumed+1])
			if cast.recvClusterTimecode+timecode < cast.sentTimecode {
				// Allow non-monotonic blocks within a single segment (this simply means that
				// coding order is not the same as display order)
				if cast.firstBlockInSegment {
					shift := cast.sentTimecode - (cast.recvClusterTimecode + timecode)
					cast.timecodeShift += shift
					cast.recvClusterTimecode += shift
				}
			} else {
				cast.sentTimecode = cast.recvClusterTimecode + timecode
			}

			ctc := cast.recvClusterTimecode
			cluster := []byte{
				// indeterminate length cluster
				ebmlTagCluster >> 24 & 0xFF, ebmlTagCluster >> 16 & 0xFF, ebmlTagCluster >> 8 & 0xFF, ebmlTagCluster & 0xFF, 0xFF,
				// first child: 8-byte timecode
				ebmlTagTimecode, 0x88,
				byte(ctc >> 56), byte(ctc >> 48), byte(ctc >> 40), byte(ctc >> 32),
				byte(ctc >> 24), byte(ctc >> 16), byte(ctc >> 8), byte(ctc),
			}
			packed := frame{buf, track, key}

			forceCluster := ctc != cast.sentClusterTimecode
			cast.vlock.Lock()
			for _, cb := range cast.viewers {
				if !cb.skipHeaders {
					if !cb.write(cast.header) || !cb.write(cast.tracks) {
						continue // FIXME: if second write failed, the stream will not be a valid mkv
					}
					cb.skipHeaders = true
					cast.frames.Read(cb.WriteFrame)
				}
				cb.WriteFrame(cluster, forceCluster, packed)
			}
			cast.vlock.Unlock()
			if forceCluster {
				cast.frames.PushCluster(cluster)
			}
			cast.frames.PushFrame(packed)
			cast.sentClusterTimecode = ctc
			cast.firstBlockInSegment = false

		default:
			return 0, errors.New("unknown EBML tag")
		}

		cast.buffer = cast.buffer[len(buf):]
	}
}
