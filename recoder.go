package main

import (
	"errors"
	"fmt"
	"github.com/ziutek/glib"
	"github.com/ziutek/gst"
	"strings"
)

func RunGlibForever() {
	go glib.NewMainLoop(nil).Run()
}

type Demuxer struct {
	//         |                      /-- pad video_%u -->
	// stream -|-> source --> demux --+
	//         |                      \-- pad audio_%u -->
	//     app | gstreamer
	lastID int
	stream *Broadcast
	pipe   *gst.Pipeline
	source *gst.Element
	demux  *gst.Element
	outs   []*splitter
	stop   chan int
}

type splitter struct {
	// pad --> decode --> tee --+-- pad -->
	//                          +-- pad -->
	//                          ...
	lastPadID   int
	isVideo     bool
	isVP9OrOpus bool // Exact meaning determined by `isVideo`.
	demux       *Demuxer
	source      *gst.Pad
	decode      *gst.Element // May be nil, in which case source is copied as is.
	tee         *gst.Element
	fakesink    *gst.Element // Have to push the data somewhere when noone is reading.
}

type Muxer struct {
	// pad --> coder --> queue --\                    |            /--> client
	//                           +--> muxer --> sink -|-> stream --+
	// pad --> coder --> queue --/                    |            \--> client
	//                                      gstreamer | app
	demux  *Demuxer
	inputs []*gst.Pad
	coders []*gst.Element // May be nil if the corresponding splitter has nil decoder.
	queues []*gst.Element
	muxer  *gst.Element
	sink   *gst.Element
	Broadcast
}

func (d *Demuxer) obtainName(kind string) string {
	d.lastID += 1
	return fmt.Sprintf("%s_%d", kind, d.lastID)
}

func NewDemuxer(stream *Broadcast) (*Demuxer, error) {
	d := Demuxer{stream: stream, stop: make(chan int)}

	d.pipe = gst.NewPipeline(d.obtainName("pipeline"))

	d.demux = gst.ElementFactoryMake("matroskademux", d.obtainName("demuxer"))
	if d.demux == nil {
		return nil, errors.New("could not create matroskademux")
	}
	d.demux.ConnectNoi("pad-added", d.newOut, nil)

	d.source = gst.ElementFactoryMake("appsrc", d.obtainName("appsrc"))
	if d.source == nil {
		return nil, errors.New("could not create appsrc")
	}
	d.source.SetProperty("max-bytes", 8192)

	d.pipe.Add(d.source, d.demux)
	d.source.Link(d.demux)

	go func() {
		bus := d.pipe.GetBus()
		for {
			msg := bus.TimedPopFiltered(6000000, gst.MESSAGE_ERROR)
			if msg != nil {
				err, _ := msg.ParseError()
				fmt.Println(err.Error(), err.Code)
				return
			}
		}
	}()

	go d.run()
	return &d, nil
}

func (d *Demuxer) Close() {
	d.stop <- 1
}

func (d *Demuxer) run() {
	d.pipe.SetState(gst.STATE_PLAYING)
	defer d.pipe.SetState(gst.STATE_NULL)

	ch := make(chan []byte, 30)
	defer close(ch)

	d.stream.Connect(ch, false)
	defer d.stream.Disconnect(ch)

	for {
		select {
		case chunk := <-ch:
			if d.stream.Closed {
				break
			}
			buf := gst.NewBufferWrapped(chunk)
			d.source.Emit("push-buffer", buf)
			buf.Unref()

		case <-d.stop:
			break
		}
	}
	d.source.Emit("end-of-stream")
}

func (d *Demuxer) newOut(pad *gst.Pad) {
	kind, _ := pad.GetCurrentCaps().GetStructure(0)

	s := splitter{
		isVideo:     strings.HasPrefix(kind, "video/"),
		isVP9OrOpus: kind == "video/x-vp9" || kind == "audio/x-opus",
		demux:       d,
		source:      pad,
	}

	if s.isVideo {
		if s.isVP9OrOpus {
			s.decode = gst.ElementFactoryMake("vp9dec", d.obtainName("vp9dec"))
		} else {
			s.decode = gst.ElementFactoryMake("vp8dec", d.obtainName("vp8dec"))
		}
		if s.decode == nil {
			// TODO something???
			return
		}
	}

	s.tee = gst.ElementFactoryMake("tee", d.obtainName("tee"))
	if s.tee == nil {
		// TODO there may be nothing to do though...
		return
	}

	s.fakesink = gst.ElementFactoryMake("fakesink", d.obtainName("fakesink"))
	if s.fakesink == nil {
		// TODO ??????
		return
	}

	d.outs = append(d.outs, &s)
	d.pipe.Add(s.tee, s.fakesink)

	if s.decode != nil {
		d.pipe.Add(s.decode)
		pad.Link(s.decode.GetStaticPad("sink"))
		s.decode.Link(s.tee)
		s.decode.SetState(gst.STATE_PLAYING)
	} else {
		pad.Link(s.tee.GetStaticPad("sink"))
	}

	s.tee.GetRequestPad(fmt.Sprintf("src_0")).Link(s.fakesink.GetStaticPad("sink"))
	s.tee.SetState(gst.STATE_PLAYING)
	s.fakesink.SetState(gst.STATE_PLAYING)
}

func (s *splitter) Acquire() *gst.Pad {
	s.lastPadID += 1
	pad := s.tee.GetRequestPad(fmt.Sprintf("src_%d", s.lastPadID))
	return pad
}

func (s *splitter) Release(pad *gst.Pad) {
	// ...?
}

func (d *Demuxer) NewMuxer(targetRate int) (*Muxer, error) {
	m := Muxer{
		demux:     d,
		inputs:    make([]*gst.Pad, 0, len(d.outs)),
		coders:    make([]*gst.Element, 0, len(d.outs)),
		queues:    make([]*gst.Element, 0, len(d.outs)),
		muxer:     gst.ElementFactoryMake("webmmux", d.obtainName("webmmux")),
		sink:      gst.ElementFactoryMake("appsink", d.obtainName("appsink")),
		Broadcast: NewBroadcast(),
	}
	if m.muxer == nil {
		return nil, errors.New("could not create webmmux")
	}
	if m.sink == nil {
		return nil, errors.New("could not create appsink")
	}

	// FIXME if the following code fails, these elements remain in the pipe.
	d.pipe.Add(m.muxer, m.sink)
	m.muxer.SetProperty("streamable", true)
	m.muxer.Link(m.sink)
	m.sink.ConnectNoi("new-sample", m.retrieveBuffer, nil)
	m.sink.SetProperty("emit-signals", true)

	for _, s := range d.outs {
		input := s.Acquire()
		queue := gst.ElementFactoryMake("queue", d.obtainName("queue"))
		if queue == nil {
			return nil, errors.New("could not create queue")
		}

		var coder *gst.Element
		if s.decode != nil {
			if !s.isVideo {
				return nil, errors.New("don't want to recode audio")
			}
			if s.isVP9OrOpus {
				coder = gst.ElementFactoryMake("vp9enc", d.obtainName("vp9enc"))
			} else {
				coder = gst.ElementFactoryMake("vp8enc", d.obtainName("vp8enc"))
			}
			if coder == nil {
				return nil, errors.New("could not create vp[89]dec")
			}
			coder.SetProperty("keyframe-max-dist", 60)
			coder.SetProperty("deadline", 1)
			coder.SetProperty("end-usage", 1) // constant bitrate
			coder.SetProperty("target-bitrate", targetRate*8)
			// FIXME elements will remain in pipe if the next iteration fails.
			d.pipe.Add(queue, coder)
			coder.Link(queue)
			input.Link(coder.GetStaticPad("sink"))
		} else {
			d.pipe.Add(queue)
			input.Link(queue.GetStaticPad("sink"))
		}

		queue.GetStaticPad("src").Link(m.muxer.GetRequestPad(s.source.GetName()))
		m.inputs = append(m.inputs, input)
		m.coders = append(m.coders, coder)
		m.queues = append(m.queues, queue)
	}

	for _, e := range m.coders {
		if e != nil {
			e.SetState(gst.STATE_PLAYING)
		}
	}
	for _, e := range m.queues {
		e.SetState(gst.STATE_PLAYING)
	}
	m.muxer.SetState(gst.STATE_PLAYING)
	m.sink.SetState(gst.STATE_PLAYING)
	return &m, nil
}

func (m *Muxer) retrieveBuffer() {
	sample := gst.SampleFromPointer(m.sink.Emit("pull-sample"))
	defer sample.Unref()

	buffer := sample.GetBuffer()
	info := buffer.Map(gst.MAP_READ)
	if info == nil {
		// ...
		return
	}
	defer buffer.Unmap(info)

	if _, err := m.Write(info.Data()); err != nil {
		// ...
		return
	}
}

func (m *Muxer) Close() {
	// TODO unlink the elements
	m.Broadcast.Close()
}
