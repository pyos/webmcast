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
      EBML_TAG_Tags           = 0x1254C367L,
        EBML_TAG_Tag            = 0x7373L,
          EBML_TAG_Targets        = 0x63C0L,
            EBML_TAG_TargetType     = 0x63CAL,
            EBML_TAG_TagTrackUID    = 0x63C5L,
          EBML_TAG_SimpleTag      = 0x67C8L,
            EBML_TAG_TagName        = 0x45A3L,
            EBML_TAG_TagLanguage    = 0x447AL,
            EBML_TAG_TagDefault     = 0x4484L,
            EBML_TAG_TagString      = 0x4487L,
            EBML_TAG_TagBinary      = 0x4485L,
};


static const unsigned long long EBML_INDETERMINATE = 0xFFFFFFFFFFFFFFULL;
static const unsigned long long EBML_INDETERMINATE_MARKERS[] = {
    0xFFULL, 0x7FFFULL, 0x3FFFFFULL, 0x1FFFFFFFULL, 0x0FFFFFFFFFULL,
    0x07FFFFFFFFFFULL, 0x03FFFFFFFFFFFFULL, 0x01FFFFFFFFFFFFFFULL,
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


static inline unsigned long long ebml_parse_fixed_uint(struct ebml_buffer buf)
{
    unsigned long long x = 0;
    while (buf.size--) x = x << 8 | *(buf.data)++;
    return x;
}


static inline unsigned ebml_parse_uint_size(unsigned char first_byte)
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


static inline struct ebml_uint ebml_parse_tagid(struct ebml_buffer buf)
{
    struct ebml_buffer view = { buf.data, buf.size ? ebml_parse_uint_size(buf.data[0]) : 1 };
    return buf.size >= view.size
         ? (struct ebml_uint) { view.size, ebml_parse_fixed_uint(view) }
         : (struct ebml_uint) { 0, 0 };
}


static inline struct ebml_uint ebml_parse_uint(struct ebml_buffer buf)
{
    struct ebml_uint u = ebml_parse_tagid(buf);
    if (u.consumed) {
        u.value = u.value == EBML_INDETERMINATE_MARKERS[u.consumed - 1] ? EBML_INDETERMINATE
                : u.value & ~(1ULL << (7 * u.consumed));
    }
    return u;
}


static struct ebml_tag ebml_parse_tag_incomplete(struct ebml_buffer buf)
{
    struct ebml_uint id = ebml_parse_tagid(buf);
    if (!id.consumed)
        return (struct ebml_tag) { 0, 0, 0 };

    struct ebml_uint len = ebml_parse_uint(ebml_buffer_shift(buf, id.consumed));
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


static inline struct ebml_buffer ebml_tag_contents(struct ebml_buffer b, struct ebml_tag t)
{
    return (struct ebml_buffer) { b.data + t.consumed, t.length };
}


#endif
