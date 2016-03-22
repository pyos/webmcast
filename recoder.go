package main

import (
	"errors"
	"fmt"
	// NOTE: pull changes from https://github.com/pyos/{glib,gst}
	"github.com/ziutek/glib"
	"github.com/ziutek/gst"
	"strings"
)

type Demuxer struct {
	//         |                      /-- pad video_%u -->
	// stream -|-> source --> demux --+
	//         |                      \-- pad audio_%u -->
	//     app | gstreamer pipeline
	lastID       int
	stream       *Broadcast
	pipe         *gst.Pipeline
	source       *gst.Element
	demux        *gst.Element
	demuxHandler glib.SigHandlerId
	outs         []*splitter
	stop         chan int
	active       bool
}

type splitter struct {
	// pad --> decode --> tee --+-- pad -->
	//                          +-- pad -->
	//                          ...
	lastPadID int
	name      string
	kind      string
	demux     *Demuxer
	source    *gst.Pad
	decode    *gst.Element // May be nil, in which case source is copied as is.
	tee       *gst.Element
	fakesink  *gst.Element // Have to push the data somewhere when noone is reading.
}

type Muxer struct {
	// pad --> coder --> queue --\                    |            /--> client
	//                           +--> muxer --> sink -|-> stream --+
	// pad --> coder --> queue --/                    |            \--> client
	//                                      gstreamer | app
	demux       *Demuxer
	inputs      []*gst.Pad
	coders      []*gst.Element // May be nil if the corresponding splitter has nil decoder.
	queues      []*gst.Element
	muxer       *gst.Element
	sink        *gst.Element
	sinkHandler glib.SigHandlerId
	Broadcast
}

var lastDemuxerID = 0

func NewDemuxer(stream *Broadcast) (*Demuxer, error) {
	d := Demuxer{stream: stream, stop: make(chan int)}
	d.pipe = gst.NewPipeline(fmt.Sprintf("Transcoder %d", lastDemuxerID))

	var err error
	if d.demux, err = d.createElement("matroskademux"); err != nil {
		return nil, err
	}
	if d.source, err = d.createElement("appsrc"); err != nil {
		return nil, err
	}
	d.source.SetProperty("max-bytes", 8192)
	d.source.Link(d.demux)
	return &d, nil
}

func (d *Demuxer) Run() {
	d.pipe.SetState(gst.STATE_PLAYING)
	d.active = true
	defer d.pipe.SetState(gst.STATE_NULL)

	d.demuxHandler = d.demux.ConnectNoi("pad-added", d.createSplitter, nil)
	defer d.demux.Disconnect(d.demuxHandler)

	ch := make(chan []byte, 30)
	defer close(ch)

	d.stream.Connect(ch, false)
	defer d.stream.Disconnect(ch)

	go func() {
		bus := d.pipe.GetBus()
		for {
			msg := bus.TimedPopFiltered(6000000, gst.MESSAGE_ERROR)
			// TODO stop sometime
			if msg != nil {
				err, _ := msg.ParseError()
				fmt.Println(err.Error(), err.Code)
				return
			}
		}
	}()

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
	d.active = false
}

func (d *Demuxer) Close() {
	d.stop <- 1
}

func (d *Demuxer) createElement(kind string) (*gst.Element, error) {
	// All element within a single pipeline must have unique names.
	// Problem is, we're generating muxers dynamically...
	d.lastID += 1

	if elem := gst.ElementFactoryMake(kind, fmt.Sprintf("%s_%d", kind, d.lastID)); elem != nil {
		d.pipe.Add(elem)
		if d.active {
			elem.SetState(gst.STATE_PLAYING)
		}
		return elem, nil
	}

	return nil, errors.New(fmt.Sprintf("could not create %s", kind))
}

func (d *Demuxer) removeElement(e *gst.Element) {
	e.SetState(gst.STATE_NULL)
	d.pipe.Remove(e)
}

func (d *Demuxer) createSplitter(pad *gst.Pad) {
	kind, _ := pad.GetCurrentCaps().GetStructure(0)

	s := splitter{demux: d, name: pad.GetName(), kind: kind}

	var err error
	if s.fakesink, err = d.createElement("fakesink"); err != nil {
		panic("resource leak") // FIXME
	}
	if s.tee, err = d.createElement("tee"); err != nil {
		panic("resource leak") // FIXME
	}
	s.tee.GetRequestPad(fmt.Sprintf("src_0")).Link(s.fakesink.GetStaticPad("sink"))

	if strings.HasPrefix(kind, "video/") {
		var err error
		decodeKind := "vp8dec"
		if kind == "video/x-vp9" {
			decodeKind = "vp9dec"
		}
		if s.decode, err = d.createElement(decodeKind); err != nil {
			panic("resource leak") // FIXME
		}
		s.decode.Link(s.tee)
		pad.Link(s.decode.GetStaticPad("sink"))
	} else {
		pad.Link(s.tee.GetStaticPad("sink"))
	}

	d.outs = append(d.outs, &s)
}

func (s *splitter) createOutput() *gst.Pad {
	s.lastPadID += 1
	pad := s.tee.GetRequestPad(fmt.Sprintf("src_%d", s.lastPadID))
	return pad
}

func (s *splitter) removeOutput(pad *gst.Pad) {
	pad.SetActive(false)
	s.tee.RemovePad(pad)
}

func (d *Demuxer) NewMuxer(targetRate int) (*Muxer, error) {
	m := Muxer{
		demux:     d,
		inputs:    make([]*gst.Pad, 0, len(d.outs)),
		coders:    make([]*gst.Element, 0, len(d.outs)),
		queues:    make([]*gst.Element, 0, len(d.outs)),
		Broadcast: NewBroadcast(),
	}

	var err error
	if m.muxer, err = d.createElement("webmmux"); err != nil {
		panic("resource leak") // DAMMIT
	}
	if m.sink, err = d.createElement("appsink"); err != nil {
		panic("resource leak") // FIXME
	}

	m.muxer.SetProperty("streamable", true)
	m.muxer.Link(m.sink)

	m.sink.SetProperty("emit-signals", true)
	m.sinkHandler = m.sink.ConnectNoi("new-sample", m.fetchSample, nil)

	for _, s := range d.outs {
		input := s.createOutput()
		queue, err := d.createElement("queue")
		if err != nil {
			panic("resource leak") // FIXME
		}

		var coder *gst.Element
		if s.decode != nil {
			if !strings.HasPrefix(s.kind, "video/") {
				panic(fmt.Sprintf("unexpected caps %s", s.kind))
			}

			coderKind := "vp8enc"
			if s.kind == "video/x-vp9" {
				coderKind = "vp9enc"
			}
			if coder, err = d.createElement(coderKind); err != nil {
				panic("resource leak") // FIXME
			}

			coder.SetProperty("deadline", 1)
			coder.SetProperty("end-usage", 1) // constant bitrate
			coder.SetProperty("target-bitrate", targetRate*8)
			coder.SetProperty("keyframe-max-dist", 60)
			coder.Link(queue)
			input.Link(coder.GetStaticPad("sink"))
		} else {
			input.Link(queue.GetStaticPad("sink"))
		}

		queue.GetStaticPad("src").Link(m.muxer.GetRequestPad(s.name))
		m.inputs = append(m.inputs, input)
		m.coders = append(m.coders, coder)
		m.queues = append(m.queues, queue)
	}

	return &m, nil
}

func (m *Muxer) fetchSample() {
	sample := gst.SampleFromPointer(m.sink.Emit("pull-sample"))
	defer sample.Unref()
	buffer := sample.GetBuffer()
	if info := buffer.Map(gst.MAP_READ); info != nil {
		m.Write(info.Data())
		buffer.Unmap(info)
	}
}

func (m *Muxer) Close() {
	for i, s := range m.demux.outs {
		s.removeOutput(m.inputs[i])
		if m.coders[i] != nil {
			m.demux.removeElement(m.coders[i])
		}
		m.demux.removeElement(m.queues[i])
	}
	m.sink.Disconnect(m.sinkHandler)
	m.demux.removeElement(m.muxer)
	m.demux.removeElement(m.sink)
	m.Broadcast.Close()
}
