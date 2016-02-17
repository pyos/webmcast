// #include "buffer.h"
#ifndef EBML_BINARY_H
#define EBML_BINARY_H


enum EBML_TAG_ID  // https://www.matroska.org/technical/specs/index.html
{
    EBML_TAG_Void           = 0xEC,
    EBML_TAG_EBML           = 0x1A45DFA3,
    EBML_TAG_Segment        = 0x18538067,
      EBML_TAG_SeekHead       = 0x114D9B74,
      EBML_TAG_Info           = 0x1549A966,
        EBML_TAG_TimecodeScale  = 0x2AD7B1,
        EBML_TAG_Duration       = 0x4489,
        EBML_TAG_DateUTC        = 0x4461,
        EBML_TAG_MuxingApp      = 0x4D80,
        EBML_TAG_WritingApp     = 0x5741,
      EBML_TAG_Tracks         = 0x1654AE6B,
        EBML_TAG_TrackEntry     = 0xAE,
          EBML_TAG_TrackNumber    = 0xD7,
          EBML_TAG_TrackUID       = 0x73C5,
          EBML_TAG_TrackType      = 0x83,
          EBML_TAG_FlagEnabled    = 0x88,
          EBML_TAG_FlagDefault    = 0x88,
          EBML_TAG_FlagForced     = 0x55AA,
          EBML_TAG_FlagLacing     = 0x9C,
          EBML_TAG_DefaultDuration= 0x23E383,
          EBML_TAG_Name           = 0x536E,
          EBML_TAG_CodecID        = 0x86,
          EBML_TAG_CodecName      = 0x228688,
          EBML_TAG_Video          = 0xE0,
            EBML_TAG_PixelWidth     = 0xB0,
            EBML_TAG_PixelHeight    = 0xBA,
          EBML_TAG_Audio          = 0xE1,
      EBML_TAG_Cluster        = 0x1F43B675,
        EBML_TAG_Timecode       = 0xE7,
        EBML_TAG_PrevSize       = 0xAB,
        EBML_TAG_SimpleBlock    = 0xA3,
        EBML_TAG_BlockGroup     = 0xA0,
          EBML_TAG_Block          = 0xA1,
          EBML_TAG_BlockDuration  = 0x9B,
          EBML_TAG_ReferenceBlock = 0xFB,
          EBML_TAG_DiscardPadding = 0x75A2,
      EBML_TAG_Cues           = 0x1C53BB6B,
      EBML_TAG_Chapters       = 0x1043A770,
};


static const uint64_t EBML_INDETERMINATE = 0xFFFFFFFFFFFFFFULL;
static const uint64_t EBML_INDETERMINATE_MARKERS[] = {
    // shortest encodings of uints with these values have special meaning
    0x0000000000007FULL, 0x00000000003FFFULL, 0x000000001FFFFFULL, 0x0000000FFFFFFFULL,
    0x000007FFFFFFFFULL, 0x0003FFFFFFFFFFULL, 0x01FFFFFFFFFFFFULL, 0xFFFFFFFFFFFFFFULL,
};


struct ebml_uint
{
    size_t   consumed;
    uint64_t value;
};


struct ebml_tag
{
    size_t consumed;
    size_t length;
    uint32_t /* enum EBML_TAG_ID */ id;
};


static inline uint64_t ebml_parse_fixed_uint(struct ebml_buffer buf)
{
    uint64_t x = 0;
    while (buf.size--) x = x << 8 | *(buf.data)++;
    return x;
}


static inline size_t ebml_parse_uint_size(uint8_t first_byte)
{
    /* EBML-coded variable-size uints look like this:
         1xxxxxxx
         01xxxxxx xxxxxxxx
         001xxxxx xxxxxxxx xxxxxxxx
         ...
         00000001 xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
                ^---- this length marker is included in tag ids but not in other ints
     */
    return __builtin_clz((int) first_byte) - (sizeof(int) - sizeof(uint8_t)) * 8 + 1;
}


static struct ebml_uint ebml_parse_uint(struct ebml_buffer buf, int keep_marker)
{
    if (buf.size < 1)
        return (struct ebml_uint) { 0, 0 };

    size_t length = ebml_parse_uint_size(buf.data[0]);

    if (buf.size < length)
        return (struct ebml_uint) { 0, 0 };

    uint64_t i = ebml_parse_fixed_uint(ebml_view(buf.data, length));
    if (i == EBML_INDETERMINATE_MARKERS[length - 1])
        i = EBML_INDETERMINATE;

    return (struct ebml_uint) { length, keep_marker ? i : i & ~(1ULL << (7 * length)) };
}


static struct ebml_tag ebml_parse_tag(struct ebml_buffer buf)
{
    struct ebml_uint id = ebml_parse_uint(buf, 1);
    if (!id.consumed)
        return (struct ebml_tag) { 0, 0, 0 };

    struct ebml_uint length = ebml_parse_uint(ebml_buffer_shift(buf, id.consumed), 0);
    if (!length.consumed)
        return (struct ebml_tag) { 0, 0, 0 };

    return (struct ebml_tag) { id.consumed + length.consumed, length.value, (uint32_t) id.value };
}


static inline struct ebml_buffer ebml_tag_contents(struct ebml_buffer b, struct ebml_tag t)
{
    return ebml_view(b.data + t.consumed, t.length);
}


static inline struct ebml_buffer ebml_tag_encoded(struct ebml_buffer b, struct ebml_tag t)
{
    return ebml_view(b.data, t.consumed + t.length);
}


static inline void ebml_write_fixed_uint_at(uint8_t *b, uint64_t v, size_t size)
{
    while (size--) *b++ = v >> (8 * size);
}


static inline int ebml_write_fixed_uint(struct ebml_buffer_dyn *b, uint64_t v, size_t size)
{
    uint8_t data[size];
    ebml_write_fixed_uint_at(data, v, size);
    return ebml_buffer_dyn_concat(b, ebml_view(data, size));
}


static inline int ebml_write_uint(struct ebml_buffer_dyn *b, uint64_t v, int has_marker)
{
    size_t size = 0;
    while (v >> ((7 + has_marker) * size)) size++;

    if (v && v < EBML_INDETERMINATE && EBML_INDETERMINATE_MARKERS[size - 1] == v)
        size++;  /* encode as a longer sequence to avoid placing an indeterminate value */

    return ebml_write_fixed_uint(b, has_marker ? v : v | 1ull << (7 * size), size);
}


static inline int ebml_write_tag(struct ebml_buffer_dyn *b, struct ebml_tag v)
{
    if (ebml_write_uint(b, v.id, 1))
        return -1;

    return ebml_write_uint(b, v.length, 0);
}


#endif
