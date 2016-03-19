package ebml

const (
	VoidTag            = 0xEC
	EBMLTag            = 0x1A45DFA3
	SegmentTag         = 0x18538067
	SeekHeadTag        = 0x114D9B74
	InfoTag            = 0x1549A966
	TimecodeScaleTag   = 0x2AD7B1
	DurationTag        = 0x4489
	DateUTCTag         = 0x4461
	MuxingAppTag       = 0x4D80
	WritingAppTag      = 0x5741
	TracksTag          = 0x1654AE6B
	TrackEntryTag      = 0xAE
	TrackNumberTag     = 0xD7
	TrackUIDTag        = 0x73C5
	TrackTypeTag       = 0x83
	FlagEnabledTag     = 0x88
	FlagDefaultTag     = 0x88
	FlagForcedTag      = 0x55AA
	FlagLacingTag      = 0x9C
	DefaultDurationTag = 0x23E383
	NameTag            = 0x536E
	CodecIDTag         = 0x86
	CodecNameTag       = 0x228688
	VideoTag           = 0xE0
	PixelWidthTag      = 0xB0
	PixelHeightTag     = 0xBA
	AudioTag           = 0xE1
	ClusterTag         = 0x1F43B675
	TimecodeTag        = 0xE7
	PrevSizeTag        = 0xAB
	SimpleBlockTag     = 0xA3
	BlockGroupTag      = 0xA0
	BlockTag           = 0xA1
	BlockDurationTag   = 0x9B
	ReferenceBlockTag  = 0xFB
	DiscardPaddingTag  = 0x75A2
	CuesTag            = 0x1C53BB6B
	ChaptersTag        = 0x1043A770
	TagsTag            = 0x1254C367
	TagTag             = 0x7373
	TargetsTag         = 0x63C0
	TargetTypeTag      = 0x63CA
	TagTrackUIDTag     = 0x63C5
	SimpleTagTag       = 0x67C8
	TagNameTag         = 0x45A3
	TagLanguageTag     = 0x447A
	TagDefaultTag      = 0x4484
	TagStringTag       = 0x4487
	TagBinaryTag       = 0x4485
	Indeterminate      = 0xFFFFFFFFFFFFFF
)

var indeterminateCoding = [...]uint64{
	0xFF,
	0x7FFF,
	0x3FFFFF,
	0x1FFFFFFF,
	0x0FFFFFFFFF,
	0x07FFFFFFFFFF,
	0x03FFFFFFFFFFFF,
	0x01FFFFFFFFFFFFFF,
}

type Uint struct {
	Consumed int // How many bytes to skip to the next token; 0 if a parsing error has occurred.
	Value    uint64
}

type Tag struct {
	Consumed int
	ID       uint
	Length   uint64
}

func ParseFixedUint(data []byte) uint64 {
	var x uint64 = 0
	for _, b := range data {
		x = x<<8 | uint64(b)
	}
	return x
}

func ParseUintSize(first byte) int {
	if first == 0 {
		return 1
	}

	// 1xxxxxxx
	// 01xxxxxx xxxxxxxx
	// 001xxxxx xxxxxxxx xxxxxxxx
	// ...
	// 00000001 xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
	//        ^---- this length marker is included in tag ids but not in other ints
	i := 1
	for first&0x80 == 0 {
		i += 1
		first <<= 1
	}
	return i
}

func ParseTagID(data []byte) Uint {
	if len(data) == 0 {
		return Uint{0, 0}
	}

	consumed := ParseUintSize(data[0])
	if consumed == 0 || len(data) < consumed {
		return Uint{0, 0}
	}

	return Uint{consumed, ParseFixedUint(data[:consumed])}
}

func ParseUint(data []byte) Uint {
	id := ParseTagID(data)
	if id.Consumed == 0 {
		return Uint{0, 0}
	}

	if indeterminateCoding[id.Consumed-1] == id.Value {
		return Uint{id.Consumed, Indeterminate}
	}

	return Uint{id.Consumed, id.Value & ^(1 << uint(7*id.Consumed))}
}

func ParseTagIncomplete(data []byte) Tag {
	id := ParseTagID(data)
	if id.Consumed == 0 {
		return Tag{0, 0, 0}
	}

	length := ParseUint(data[id.Consumed:])
	if length.Consumed == 0 {
		return Tag{0, 0, 0}
	}

	return Tag{id.Consumed + length.Consumed, uint(id.Value), length.Value}
}

func ParseTag(data []byte) Tag {
	tag := ParseTagIncomplete(data)
	if tag.Length+uint64(tag.Consumed) > uint64(len(data)) {
		return Tag{0, 0, 0}
	}
	return tag
}

func (t Tag) Contents(data []byte) []byte {
	return data[t.Consumed : uint64(t.Consumed)+t.Length]
}

func (t Tag) Skip(data []byte) []byte {
	return data[uint64(t.Consumed)+t.Length:]
}
