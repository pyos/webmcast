package broadcast

const (
	EBMLVoidTag            = 0xEC
	EBMLEBMLTag            = 0x1A45DFA3
	EBMLSegmentTag         = 0x18538067
	EBMLSeekHeadTag        = 0x114D9B74
	EBMLInfoTag            = 0x1549A966
	EBMLTimecodeScaleTag   = 0x2AD7B1
	EBMLDurationTag        = 0x4489
	EBMLDateUTCTag         = 0x4461
	EBMLMuxingAppTag       = 0x4D80
	EBMLWritingAppTag      = 0x5741
	EBMLTracksTag          = 0x1654AE6B
	EBMLTrackEntryTag      = 0xAE
	EBMLTrackNumberTag     = 0xD7
	EBMLTrackUIDTag        = 0x73C5
	EBMLTrackTypeTag       = 0x83
	EBMLFlagEnabledTag     = 0xB9
	EBMLFlagDefaultTag     = 0x88
	EBMLFlagForcedTag      = 0x55AA
	EBMLFlagLacingTag      = 0x9C
	EBMLDefaultDurationTag = 0x23E383
	EBMLNameTag            = 0x536E
	EBMLCodecIDTag         = 0x86
	EBMLCodecNameTag       = 0x228688
	EBMLVideoTag           = 0xE0
	EBMLPixelWidthTag      = 0xB0
	EBMLPixelHeightTag     = 0xBA
	EBMLAudioTag           = 0xE1
	EBMLClusterTag         = 0x1F43B675
	EBMLTimecodeTag        = 0xE7
	EBMLPrevSizeTag        = 0xAB
	EBMLSimpleBlockTag     = 0xA3
	EBMLBlockGroupTag      = 0xA0
	EBMLBlockTag           = 0xA1
	EBMLBlockDurationTag   = 0x9B
	EBMLReferenceBlockTag  = 0xFB
	EBMLDiscardPaddingTag  = 0x75A2
	EBMLCuesTag            = 0x1C53BB6B
	EBMLChaptersTag        = 0x1043A770
	EBMLTagsTag            = 0x1254C367
	EBMLTagTag             = 0x7373
	EBMLTargetsTag         = 0x63C0
	EBMLTargetTypeTag      = 0x63CA
	EBMLTagTrackUIDTag     = 0x63C5
	EBMLSimpleTagTag       = 0x67C8
	EBMLTagNameTag         = 0x45A3
	EBMLTagLanguageTag     = 0x447A
	EBMLTagDefaultTag      = 0x4484
	EBMLTagStringTag       = 0x4487
	EBMLTagBinaryTag       = 0x4485
	EBMLIndeterminate      = 0xFFFFFFFFFFFFFF
)

var ebmlIndeterminateCoding = [...]uint64{
	0xFF,
	0x7FFF,
	0x3FFFFF,
	0x1FFFFFFF,
	0x0FFFFFFFFF,
	0x07FFFFFFFFFF,
	0x03FFFFFFFFFFFF,
	0x01FFFFFFFFFFFFFF,
}

type EBMLUint struct {
	Consumed int // How many bytes to skip to the next token; 0 if a parsing error has occurred.
	Value    uint64
}

type EBMLTag struct {
	Consumed int
	ID       uint
	Length   uint64
}

func EBMLParseFixedUint(data []byte) uint64 {
	var x uint64 = 0
	for _, b := range data {
		x = x<<8 | uint64(b)
	}
	return x
}

func EBMLParseUintSize(first byte) int {
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

func EBMLParseTagID(data []byte) EBMLUint {
	if len(data) == 0 {
		return EBMLUint{0, 0}
	}

	consumed := EBMLParseUintSize(data[0])
	if consumed == 0 || len(data) < consumed {
		return EBMLUint{0, 0}
	}

	return EBMLUint{consumed, EBMLParseFixedUint(data[:consumed])}
}

func EBMLParseUint(data []byte) EBMLUint {
	id := EBMLParseTagID(data)
	if id.Consumed == 0 {
		return EBMLUint{0, 0}
	}

	if ebmlIndeterminateCoding[id.Consumed-1] == id.Value {
		return EBMLUint{id.Consumed, EBMLIndeterminate}
	}

	return EBMLUint{id.Consumed, id.Value & ^(1 << uint(7*id.Consumed))}
}

func EBMLParseTagIncomplete(data []byte) EBMLTag {
	id := EBMLParseTagID(data)
	if id.Consumed == 0 {
		return EBMLTag{0, 0, 0}
	}

	length := EBMLParseUint(data[id.Consumed:])
	if length.Consumed == 0 {
		return EBMLTag{0, 0, 0}
	}

	return EBMLTag{id.Consumed + length.Consumed, uint(id.Value), length.Value}
}

func EBMLParseTag(data []byte) EBMLTag {
	tag := EBMLParseTagIncomplete(data)
	if tag.Length+uint64(tag.Consumed) > uint64(len(data)) {
		return EBMLTag{0, 0, 0}
	}
	return tag
}

func (t EBMLTag) Contents(data []byte) []byte {
	return data[t.Consumed : uint64(t.Consumed)+t.Length]
}

func (t EBMLTag) Skip(data []byte) []byte {
	return data[uint64(t.Consumed)+t.Length:]
}
