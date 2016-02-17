// #include "buffer.h"
#ifndef EBML_BINARY_H
#define EBML_BINARY_H


enum EBML_TAG_ID  // https://www.matroska.org/technical/specs/index.html
{
    EBML_TAG_Void           = 0xECL,
    EBML_TAG_EBML           = 0x1A45DFA3L,
    EBML_TAG_Segment        = 0x18538067L,
      EBML_TAG_SeekHead       = 0x114D9B74L,
      EBML_TAG_Info           = 0x1549A966L,
        EBML_TAG_TimecodeScale  = 0x2AD7B1L,
        EBML_TAG_Duration       = 0x4489L,
        EBML_TAG_DateUTC        = 0x4461L,
        EBML_TAG_MuxingApp      = 0x4D80L,
        EBML_TAG_WritingApp     = 0x5741L,
      EBML_TAG_Tracks         = 0x1654AE6BL,
        EBML_TAG_TrackEntry     = 0xAEL,
          EBML_TAG_TrackNumber    = 0xD7L,
          EBML_TAG_TrackUID       = 0x73C5L,
          EBML_TAG_TrackType      = 0x83L,
          EBML_TAG_FlagEnabled    = 0x88L,
          EBML_TAG_FlagDefault    = 0x88L,
          EBML_TAG_FlagForced     = 0x55AAL,
          EBML_TAG_FlagLacing     = 0x9CL,
          EBML_TAG_DefaultDuration= 0x23E383L,
          EBML_TAG_Name           = 0x536EL,
          EBML_TAG_CodecID        = 0x86L,
          EBML_TAG_CodecName      = 0x228688L,
          EBML_TAG_Video          = 0xE0L,
            EBML_TAG_PixelWidth     = 0xB0L,
            EBML_TAG_PixelHeight    = 0xBAL,
          EBML_TAG_Audio          = 0xE1L,
      EBML_TAG_Cluster        = 0x1F43B675L,
        EBML_TAG_Timecode       = 0xE7L,
        EBML_TAG_PrevSize       = 0xABL,
        EBML_TAG_SimpleBlock    = 0xA3L,
        EBML_TAG_BlockGroup     = 0xA0L,
          EBML_TAG_Block          = 0xA1L,
          EBML_TAG_BlockDuration  = 0x9BL,
          EBML_TAG_ReferenceBlock = 0xFBL,
          EBML_TAG_DiscardPadding = 0x75A2L,
      EBML_TAG_Cues           = 0x1C53BB6BL,
      EBML_TAG_Chapters       = 0x1043A770L,
};


static const uint64_t EBML_INDETERMINATE = 0xFFFFFFFFFFFFFFULL;
static const uint64_t EBML_INDETERMINATE_MARKERS[] = {
    // shortest encodings of uints with these values have special meaning
    0x0000000000007FULL, 0x00000000003FFFULL, 0x000000001FFFFFULL, 0x0000000FFFFFFFULL,
    0x000007FFFFFFFFULL, 0x0003FFFFFFFFFFULL, 0x01FFFFFFFFFFFFULL, 0xFFFFFFFFFFFFFFULL,
};


struct ebml_uint
{
    unsigned consumed;
    unsigned long long value;
};


struct ebml_tag
{
    unsigned consumed;
    unsigned long /* enum EBML_TAG_ID */ id;
    size_t length;
};


static unsigned long long ebml_parse_fixed_uint(struct ebml_buffer buf)
{
    unsigned long long x = 0;
    while (buf.size--) x = x << 8 | *(buf.data)++;
    return x;
}


static unsigned ebml_parse_uint_size(unsigned char first_byte)
{
    /* EBML-coded variable-size uints look like this:
         1xxxxxxx
         01xxxxxx xxxxxxxx
         001xxxxx xxxxxxxx xxxxxxxx
         ...
         00000001 xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
                ^---- this length marker is included in tag ids but not in other ints
     */
    return __builtin_clz((int) first_byte) - (sizeof(int) - 1) * 8 + 1;
}


static struct ebml_uint ebml_parse_uint(struct ebml_buffer buf, int keep_marker)
{
    if (buf.size < 1)
        return (struct ebml_uint) { 0, 0 };

    unsigned length = ebml_parse_uint_size(buf.data[0]);

    if (buf.size < length)
        return (struct ebml_uint) { 0, 0 };

    unsigned long long i = ebml_parse_fixed_uint(ebml_view(buf.data, length));
    if (i == EBML_INDETERMINATE_MARKERS[length - 1])
        i = EBML_INDETERMINATE;

    return (struct ebml_uint) { length, keep_marker ? i : i & ~(1ULL << (7 * length)) };
}


static struct ebml_tag ebml_parse_tag_incomplete(struct ebml_buffer buf)
{
    struct ebml_uint id = ebml_parse_uint(buf, 1);
    if (!id.consumed)
        return (struct ebml_tag) { 0, 0, 0 };

    struct ebml_uint len = ebml_parse_uint(ebml_buffer_shift(buf, id.consumed), 0);
    if (!len.consumed)
        return (struct ebml_tag) { 0, 0, 0 };

    return (struct ebml_tag) { id.consumed + len.consumed, (unsigned long) id.value, len.value };
}


static struct ebml_tag ebml_parse_tag(struct ebml_buffer buf)
{
    struct ebml_tag tag = ebml_parse_tag_incomplete(buf);
    if (tag.length + tag.consumed > buf.size)
        return (struct ebml_tag) { 0, 0, 0 };
    return tag;
}


static struct ebml_buffer ebml_tag_contents(struct ebml_buffer b, struct ebml_tag t)
{
    return ebml_view(b.data + t.consumed, t.length);
}


static void ebml_write_fixed_uint_at(uint8_t *b, unsigned long long v, size_t size)
{
    while (size--) *b++ = v >> (8 * size);
}


static int ebml_write_fixed_uint(struct ebml_buffer_dyn *b, unsigned long long v, size_t s)
{
    uint8_t data[s];
    ebml_write_fixed_uint_at(data, v, s);
    return ebml_buffer_dyn_concat(b, ebml_view(data, s));
}


static int ebml_write_uint(struct ebml_buffer_dyn *b, unsigned long long v, int has_marker)
{
    size_t size = 0;
    while (v >> ((7 + has_marker) * size)) size++;

    if (v && v < EBML_INDETERMINATE && EBML_INDETERMINATE_MARKERS[size - 1] == v)
        size++;  /* encode as a longer sequence to avoid placing an indeterminate value */

    return ebml_write_fixed_uint(b, has_marker ? v : v | 1ull << (7 * size), size);
}


static int ebml_write_tag(struct ebml_buffer_dyn *b, struct ebml_tag v)
{
    return ebml_write_uint(b, v.id, 1) ? -1
         : ebml_write_uint(b, v.length, 0);
}


#endif
