#include <string.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>

#include <atomic>
#include <vector>

extern "C" {
    #include "broadcast.h"
}


enum EBML_TAG_ID  // https://www.matroska.org/technical/specs/index.html
{
    EBML_TAG_EBML           = 0x1A45DFA3UL,
    EBML_TAG_Void           = 0xECUL,
    EBML_TAG_CRC32          = 0xBFUL,
    EBML_TAG_Segment        = 0x18538067UL,
    EBML_TAG_SeekHead       = 0x114D9B74UL,
    EBML_TAG_Info           = 0x1549A966UL,
    EBML_TAG_Cluster        = 0x1F43B675UL,
    EBML_TAG_Timecode       = 0xE7UL,
    EBML_TAG_PrevSize       = 0xABUL,
    EBML_TAG_SimpleBlock    = 0xA3UL,
    EBML_TAG_BlockGroup     = 0xA0UL,
    EBML_TAG_Block          = 0xA1UL,
    EBML_TAG_ReferenceBlock = 0xFBUL,
    EBML_TAG_Tracks         = 0x1654AE6BUL,
    EBML_TAG_Cues           = 0x1C53BB6BUL,
    EBML_TAG_Chapters       = 0x1043A770UL,
};


struct ebml_buffer
{
    const uint8_t * base;
    size_t size;
};


static inline struct ebml_buffer ebml_buffer_advance(struct ebml_buffer b, size_t shift)
{
    return (struct ebml_buffer) { b.base + shift, b.size - shift };
}


struct ebml_uint
{
    const size_t   consumed;
    const uint64_t value;
};


struct ebml_tag
{
    const size_t consumed;
    const size_t length;
    const uint32_t /* enum EBML_TAG_ID */ id;
};


static inline bool ebml_tag_is_endless(struct ebml_tag t)
{
    return t.length == 0x0000007FUL || t.length == 0x000007FFFFFFFFULL ||
           t.length == 0x00003FFFUL || t.length == 0x0003FFFFFFFFFFULL ||
           t.length == 0x001FFFFFUL || t.length == 0x01FFFFFFFFFFFFULL ||
           t.length == 0x0FFFFFFFUL || t.length == 0xFFFFFFFFFFFFFFULL;
}


static inline bool ebml_tag_is_long_coded_endless(struct ebml_tag t)
{
    // ffmpeg: "endless = 0xFFFFFFFFFFFFFF"
    // chrome: "endless = 0xFF"
    // matroska standard: "endless = all ones"
    // ...but the field is of variable length? what the fuck
    return t.length != 0x7FUL && t.length != 0x3FFFUL && ebml_tag_is_endless(t);
}


static size_t ebml_parse_uint_size(uint8_t first_byte)
{
    return __builtin_clz((int) first_byte) - (sizeof(int) - sizeof(uint8_t)) * 8;
}


static struct ebml_uint ebml_parse_uint(const struct ebml_buffer data, bool keep_marker)
{
    if (data.size < 1)
        return (struct ebml_uint) { 0, 0 };

    size_t length = ebml_parse_uint_size(data.base[0]);

    if (data.size < length + 1)
        return (struct ebml_uint) { 0, 0 };

    uint64_t i = keep_marker ? data.base[0] : data.base[0] & (0x7F >> length);

    for (size_t k = 1; k <= length; k++)
        i = i << 8 | data.base[k];

    return (struct ebml_uint) { length + 1, i };
}


static uint64_t ebml_parse_fixed_uint(const uint8_t *buf, size_t length)
{
    uint64_t x = 0;
    for (size_t i = 0; i < length; i++)
        x = x << 8 | buf[i];
    return x;
}


static struct ebml_tag ebml_parse_tag(const struct ebml_buffer buf)
{
    struct ebml_uint id = ebml_parse_uint(buf, 1);
    if (!id.consumed)
        return (struct ebml_tag) { 0, 0, 0 };

    struct ebml_uint length = ebml_parse_uint(ebml_buffer_advance(buf, id.consumed), 0);
    if (!length.consumed)
        return (struct ebml_tag) { 0, 0, 0 };

    return (struct ebml_tag) { id.consumed + length.consumed, length.value, (uint32_t) id.value };
}


static int ebml_strip_reference_frames(struct ebml_buffer buffer, uint8_t *target, size_t *size)
{
    struct ebml_tag cluster = ebml_parse_tag(buffer);

    if (cluster.id != EBML_TAG_Cluster)
        return -1;

    bool had_keyframe = false;
    // ok so this is the first cluster we forward. if it references
    // older blocks/clusters (which this client doesn't have), the decoder
    // will error and drop the stream. so we need to drop frames
    // until the next keyframe. and boy is that hard.
    const uint8_t *refptr = target;
    memcpy(target, buffer.base, cluster.consumed);
    target += cluster.consumed;

    // a cluster can actually contain many blocks. we can send
    // the first keyframe-only block and all that follow
    for (buffer = ebml_buffer_advance(buffer, cluster.consumed); buffer.size; ) {
        struct ebml_tag tag = ebml_parse_tag(buffer);
        if (!tag.consumed)
            return -1;

        if (tag.id == EBML_TAG_PrevSize)
            // there is no previous cluster, so this data is not applicable.
            goto skip_tag;

        else if (tag.id == EBML_TAG_SimpleBlock && !had_keyframe) {
            if (tag.length < 4)
                return -1;

            // the very first field has a variable length. what a bummer.
            size_t skip_field = ebml_parse_uint_size(buffer.base[tag.consumed]) + 1;

            if (tag.length < 3 + skip_field)
                return -1;

            if (!(buffer.base[tag.consumed + skip_field + 2] & 0x80))
                goto skip_tag;  // nope, not a keyframe.

            had_keyframe = true;
        }

        else if (tag.id == EBML_TAG_BlockGroup && !had_keyframe) {
            // a BlockGroup actually contains only a single Block.
            // it does have some additional tags with metadata, though.
            // if there's a nonzero ReferenceBlock, this is def not a keyframe.
            struct ebml_buffer sdata = { buffer.base + tag.consumed, tag.length };

            while (sdata.size) {
                struct ebml_tag tag = ebml_parse_tag(sdata);
                if (!tag.consumed)
                    return -1;

                if (tag.id == EBML_TAG_ReferenceBlock)
                    if (ebml_parse_fixed_uint(sdata.base + tag.consumed, tag.length) != 0)
                        goto skip_tag;

                sdata = ebml_buffer_advance(sdata, tag.consumed + tag.length);
            }

            had_keyframe = true;
        }

        memcpy(target, buffer.base, tag.consumed + tag.length);
        target += tag.consumed + tag.length;

        skip_tag: buffer = ebml_buffer_advance(buffer, tag.consumed + tag.length);
    }

    if (had_keyframe)
        *size = target - refptr;
    return 0;
}


struct ebml_tc_shift
{
    uint64_t last_timecode;
    uint64_t shifted_by;
};


static int ebml_ensure_monotonicity(struct ebml_buffer *buffer, struct ebml_tc_shift *tc)
{
    // timecodes must be strictly increasing. if a stream is interrupted,
    // however, or if a client switches between streams with different timecodes,
    // so we need to look for a timecode, and rewrite it if necessary.
    struct ebml_buffer source = *buffer;
    struct ebml_tag cluster = ebml_parse_tag(source);

    if (!cluster.consumed || cluster.id != EBML_TAG_Cluster)
        return -1;

    for (source = ebml_buffer_advance(source, cluster.consumed); source.size; )
    {
        struct ebml_tag tag = ebml_parse_tag(source);

        if (!tag.consumed || tag.consumed + tag.length > source.size)
            return -1;

        if (tag.id == EBML_TAG_Timecode) {
            uint64_t timecode = ebml_parse_fixed_uint(source.base + tag.consumed, tag.length);

            if (tc->shifted_by + timecode < tc->last_timecode)
                tc->shifted_by = tc->last_timecode - timecode;

            if (tc->shifted_by == 0) {
                tc->last_timecode = timecode + 1;
                return 0;
            }

            timecode += tc->shifted_by;
            tc->last_timecode = timecode + 1;

            uint8_t *copy = new uint8_t[buffer->size + 8];
            size_t offset = source.base - buffer->base + 1;

            memcpy(copy, buffer->base, offset);
            copy[offset++] = 0x88;
            for (size_t i = 0; i < 8; i++)
                copy[offset++] = timecode >> (56 - 8 * i);
            memcpy(copy + offset, source.base + tag.consumed + tag.length,
                                  source.size - tag.consumed - tag.length);
            buffer->base = copy;
            buffer->size = offset + (source.size - tag.consumed - tag.length);
            return 1;
        }

        source = ebml_buffer_advance(source, tag.consumed + tag.length);
    }

    return -1;  // clusters *must* contain timecodes
}


struct webm_slot_t
{
    struct ebml_tc_shift tc_shift;
    webm_write_cb *write;
    void *data;
    int id;
    bool skip_headers;
    bool had_keyframe;
};


struct webm_broadcast_t
{
    std::vector<webm_slot_t> callbacks;
    std::vector<uint8_t> buffer;
    std::vector<uint8_t> header;  // [EBML .. Segment) -- once per webm
    std::vector<uint8_t> tracks;  // [Segment .. Cluster) -- can occur many times
    bool saw_clusters = false;
    bool saw_segments = false;
};


struct webm_broadcast_t * webm_broadcast_start(void)
{
    return new webm_broadcast_t;
}


int webm_broadcast_send(struct webm_broadcast_t *b, const uint8_t *data, size_t size)
{
    b->buffer.insert(b->buffer.end(), data, data + size);
    struct ebml_buffer buf = { &b->buffer[0], b->buffer.size() };

    while (1) {
        struct ebml_tag tag = ebml_parse_tag(buf);
        if (!tag.consumed)
            break;

        size_t fwd_length = tag.consumed + tag.length;

        if (tag.id == EBML_TAG_Segment) {
            if (ebml_tag_is_long_coded_endless(tag)) {
                uint8_t * writable = (uint8_t *) buf.base;
                // tag id = 4 bytes, the rest is its length.
                writable[4] = 0xFF;  // a shorter representation of "endless"
                writable[5] = EBML_TAG_Void;  // gotta hide all this now-unused space
                writable[6] = 0x80 | (tag.consumed - 7);
            }

            fwd_length = tag.consumed;
            b->saw_segments = true;
            b->saw_clusters = false;
            b->tracks.clear();
        } else if (ebml_tag_is_endless(tag) || tag.length > 256 * 1024)
            return -1;

        if (fwd_length > buf.size)
            break;

        if (tag.id == EBML_TAG_SeekHead) {
            // skip this. (it's not required by the spec.)
            // it contains offsets relative to the beginning of the first segment,
            // and we don't know how much data our subscribers got already.
        } else if (!b->saw_segments) {
            b->header.insert(b->header.end(), buf.base, buf.base + fwd_length);
            for (auto &c : b->callbacks)
                if (!c.skip_headers)
                    c.write(c.data, buf.base, fwd_length, 1);
        } else if (tag.id == EBML_TAG_Cluster) {
            b->saw_clusters = true;  // ignore any further metadata

            uint8_t *refstripped = NULL;
            size_t   refstripped_length = 0;

            for (auto &c : b->callbacks) {
                struct ebml_buffer b = { buf.base, fwd_length };

                if (c.had_keyframe) {
                    int nonmonotonic = ebml_ensure_monotonicity(&b, &c.tc_shift);
                    if (nonmonotonic < 0)
                        return -1;

                    if (c.write(c.data, b.base, b.size, 0) != 0)
                        c.had_keyframe = false;

                    if (nonmonotonic > 0)
                        delete (uint8_t *) b.base;
                    continue;
                }

                if (refstripped == NULL) {
                    refstripped = new uint8_t[fwd_length];
                    ebml_strip_reference_frames(b, refstripped, &refstripped_length);
                }

                if (refstripped_length != 0) {
                    struct ebml_buffer b = { refstripped, refstripped_length };
                    int nonmonotonic = ebml_ensure_monotonicity(&b, &c.tc_shift);
                    if (nonmonotonic < 0)
                        return -1;

                    if (c.write(c.data, b.base, b.size, 0) == 0)
                        c.had_keyframe = true;

                    if (nonmonotonic > 0)
                        delete (uint8_t *) b.base;
                }
            }

            delete refstripped;
        } else if (!b->saw_clusters) {
            b->tracks.insert(b->tracks.end(), buf.base, buf.base + fwd_length);
            for (auto &c : b->callbacks) {
                c.write(c.data, buf.base, fwd_length, 1);
                c.had_keyframe = true;
            }
        }

        buf = ebml_buffer_advance(buf, fwd_length);
    }

    b->buffer.erase(b->buffer.begin(), b->buffer.begin() + (buf.base - &b->buffer[0]));
    return 0;
}


void webm_broadcast_stop(struct webm_broadcast_t *b)
{
    delete b;
}


static std::atomic<int> next_callback_id(0);


int webm_slot_connect(struct webm_broadcast_t *b, webm_write_cb *f, void *d,
                      int skip_headers, uint64_t last_timecode)
{
    int id = next_callback_id++;
    if (!skip_headers)
        f(d, &b->header[0], b->header.size(), 1);
    f(d, &b->tracks[0], b->tracks.size(), 1);
    b->callbacks.push_back((struct webm_slot_t) {{ last_timecode, 0 }, f, d, id,
                                                 skip_headers, false});
    return id;
}


uint64_t webm_slot_disconnect(struct webm_broadcast_t *b, int id)
{
    for (auto it = b->callbacks.begin(); it != b->callbacks.end(); ) {
        if (it->id == id) {
            uint64_t tc = it->tc_shift.last_timecode;
            b->callbacks.erase(it);
            return tc;
        }
    }

    return 0;
}
