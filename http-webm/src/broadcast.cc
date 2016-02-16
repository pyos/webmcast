#include <string.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>

#include <atomic>
#include <vector>

extern "C"
{
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
    uint8_t * base;
    size_t    size;
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


static inline struct ebml_buffer ebml_buffer_advance(const struct ebml_buffer b, size_t shift)
{
    return (struct ebml_buffer) { b.base + shift, b.size - shift };
}


/* true if a tag has indeterminate length, i.e. spans until the next tag
 * defined to only appear at the same level. as this one. we only expect
 * `Segment`s to have indeterminate length. */
static inline bool ebml_tag_is_endless(const struct ebml_tag t)
{
    return t.length == 0x0000007FUL || t.length == 0x000007FFFFFFFFULL ||
           t.length == 0x00003FFFUL || t.length == 0x0003FFFFFFFFFFULL ||
           t.length == 0x001FFFFFUL || t.length == 0x01FFFFFFFFFFFFULL ||
           t.length == 0x0FFFFFFFUL || t.length == 0xFFFFFFFFFFFFFFULL;
}


/* all of the above values are valid, but chrome only accepts 0x7F.
 * other kinds must be recoded as this shortest form. however, this introduces
 * some empty bytes into the stream, which must be filled with a `Void` tag to avoid
 * decoding errors. but `Void` takes up at least 2 bytes, which means 0x3FFF, when
 * recoded as 0x7F, leaves no space for a `Void`. */
static inline bool ebml_tag_is_long_coded_endless(const struct ebml_tag t)
{
    return t.length != 0x7FUL && t.length != 0x3FFFUL && ebml_tag_is_endless(t);
}


static uint64_t ebml_parse_fixed_uint(const uint8_t *buf, size_t length)
{
    uint64_t x = 0;
    while (length--) x = x << 8 | *buf++;
    return x;
}


/* EBML-coded variable-size uints look like this:
 *   1xxxxxxx
 *   01xxxxxx xxxxxxxx
 *   001xxxxx xxxxxxxx xxxxxxxx
 *   ...
 *   00000001 xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
 *          ^---- this length marker is included in tag ids but not in other ints
 */
static size_t ebml_parse_uint_size(uint8_t first_byte)
{
    return __builtin_clz((int) first_byte) - (sizeof(int) - sizeof(uint8_t)) * 8 + 1;
}


static struct ebml_uint ebml_parse_uint(const struct ebml_buffer data, bool keep_marker)
{
    if (data.size < 1)
        return (struct ebml_uint) { 0, 0 };

    size_t length = ebml_parse_uint_size(data.base[0]);

    if (data.size < length)
        return (struct ebml_uint) { 0, 0 };

    uint64_t i = ebml_parse_fixed_uint(data.base, length);
    return (struct ebml_uint) { length, keep_marker ? i : i & ~(1ULL << (7 * length)) };
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


/* create a copy of a `Cluster` with all `(Simple)Block`s before the one
 * containing the first keyframe removed. `out->base` must be a chunk of writable
 * memory of same size as `buffer` (as the cluster can *start* with a keyframe).
 * `out->size` will be set to the size of data written to that memory.
 * 1 is returned if there are no keyframes in this cluster (=> `out` is empty)
 *
 * this is necessary because if a decoder happens to receive a block that references
 * a block it did not see, it will error and drop the stream, and that would be bad.
 * a keyframe, however, guarantees that no later block will reference any frame
 * before that keyframe, while also not referencing anything itself. */
static int ebml_strip_reference_frames(struct ebml_buffer buffer, struct ebml_buffer *out)
{
    struct ebml_tag cluster = ebml_parse_tag(buffer);

    if (cluster.id != EBML_TAG_Cluster)
        return -1;

    bool found_keyframe = false;
    memcpy(out->base, buffer.base, cluster.consumed);
    out->size = cluster.consumed;

    for (buffer = ebml_buffer_advance(buffer, cluster.consumed); buffer.size; ) {
        struct ebml_tag tag = ebml_parse_tag(buffer);

        if (!tag.consumed || tag.consumed + tag.length > buffer.size)
            return -1;

        if (tag.id == EBML_TAG_SimpleBlock && !found_keyframe) {
            if (tag.length == 0)
                return -1;

            size_t skip_field = ebml_parse_uint_size(buffer.base[tag.consumed]);
            if (tag.length < skip_field + 3)
                return -1;

            if (buffer.base[tag.consumed + skip_field + 2] & 0x80) {
                found_keyframe = true;
                goto copy_tag;
            }
        }

        else if (tag.id == EBML_TAG_BlockGroup && !found_keyframe) {
            /* a `BlockGroup` actually contains only a single `Block`.
               it does have some additional tags with metadata, though.
               we're looking for one either w/o a `ReferenceBlock`, or a zeroed one. */
            for (struct ebml_buffer sdata = { buffer.base + tag.consumed, tag.length };;) {
                if (!sdata.size) {
                    found_keyframe = true;
                    goto copy_tag;
                }

                struct ebml_tag tag = ebml_parse_tag(sdata);
                if (!tag.consumed)
                    return -1;

                if (tag.id == EBML_TAG_ReferenceBlock)
                    if (ebml_parse_fixed_uint(sdata.base + tag.consumed, tag.length) != 0)
                        break;

                sdata = ebml_buffer_advance(sdata, tag.consumed + tag.length);
            }
        }

        /* we're assuming that this is the first `Cluster` in a `Segment`. */
        else if (tag.id != EBML_TAG_PrevSize) copy_tag: {
            memcpy(out->base + out->size, buffer.base, tag.consumed + tag.length);
            out->size += tag.consumed + tag.length;
        }


        buffer = ebml_buffer_advance(buffer, tag.consumed + tag.length);
    }

    return !found_keyframe;
}


/* inside each cluster is a timecode. these must be strictly increasing,
 * or else the decoder will silently drop frames from clusters "from the past".
 * this is true even across segments -- if segment 1 contains a cluster with timecode
 * 10000, and segment 2 starts with a timecode 0, frames will get dropped.
 * which is why, when switching streams, we need to ensure that the timecodes
 * in the new stream are at least as high as the last timecode seen in the old stream.
 *
 * `out->base` must be writable and at least `buffer.size + 8` in length.
 * `out->size` will be set on successful return. */
static int ebml_adjust_timecode(struct ebml_buffer buffer, struct ebml_buffer *out,
                                int64_t *shift, uint64_t *minimum)
{
    struct ebml_buffer start = buffer;
    struct ebml_tag cluster = ebml_parse_tag(buffer);

    if (cluster.id != EBML_TAG_Cluster)
        return -1;

    for (buffer = ebml_buffer_advance(buffer, cluster.consumed); buffer.size;)
    {
        struct ebml_tag tag = ebml_parse_tag(buffer);

        if (!tag.consumed || tag.consumed + tag.length > buffer.size)
            return -1;

        if (tag.id == EBML_TAG_Timecode) {
            uint64_t tc = ebml_parse_fixed_uint(buffer.base + tag.consumed, tag.length);

            if (*shift + tc <= *minimum)
                *shift = *minimum + 1 - tc;
            *minimum = tc += *shift;

            /* replace the contents of this tag with the new value */
            memcpy(out->base, start.base, buffer.base + 1 - start.base);

            size_t copied = buffer.base + 1 - start.base;
            out->base[copied++] = 0x88;  /* length = 8 [1 octet] */
            for (size_t i = 0; i < 8; i++)
                out->base[copied++] = tc >> (56 - 8 * i);

            size_t tag_size = tag.consumed + tag.length;
            memcpy(out->base + copied, buffer.base + tag_size, buffer.size - tag_size);
            out->size = buffer.base - start.base + buffer.size - tag_size + 10;
            return 0;  /* there's only one timecode */
        }

        buffer = ebml_buffer_advance(buffer, tag.consumed + tag.length);
    }

    return -1;  /* each cluster *must* contain a timecode */
}


struct webm_slot_t
{
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
     int64_t timecode_shift = 0;
    uint64_t timecode_last  = 0;
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

        struct ebml_buffer tagbuf = { buf.base, tag.consumed + tag.length };

        if (tag.id == EBML_TAG_Segment) {
            if (ebml_tag_is_long_coded_endless(tag)) {
                buf.base[4] = 0xFF;           /* EBML_TAG_Segment = 4 octets */
                buf.base[5] = EBML_TAG_Void;  /* EBML_TAG_Void = 1 octet */
                buf.base[6] = 0x80 | (tag.consumed - 7);
            }

            tagbuf.size = tag.consumed;  /* forward the header, parse the contents */
            b->saw_segments = true;
            b->saw_clusters = false;
            b->tracks.clear();
        } else if (ebml_tag_is_endless(tag) || tag.length > 1024 * 1024)
            /* wow, a megabyte-sized cluster? not forwarding that. */
            return -1;

        if (tagbuf.size > buf.size)
            break;

        /* skip this tag. it's not required by the spec.
           it contains offsets relative to the beginning of the first segment,
           and we don't know how much data our subscribers got already. */
        if (tag.id == EBML_TAG_SeekHead) {}

        /* ignore duplicate EBML headers. the broadcaster probably had a connection
           failure or something. */
        else if (!b->saw_segments) {
            b->header.insert(b->header.end(), tagbuf.base, tagbuf.base + tagbuf.size);
            for (auto &c : b->callbacks)
                if (!c.skip_headers)
                    c.write(c.data, tagbuf.base, tagbuf.size, 1);
        }

        else if (tag.id == EBML_TAG_Cluster) {
            struct ebml_buffer cluster = { new uint8_t[tagbuf.size + 8], 0 };
            if (ebml_adjust_timecode(tagbuf, &cluster, &b->timecode_shift, &b->timecode_last)) {
                delete[] cluster.base;
                return -1;
            }

            b->saw_clusters = true;

            int no_keyframes = 0;
            struct ebml_buffer stripped = { NULL, 0 };

            for (auto &c : b->callbacks) {
                if (!c.had_keyframe) {
                    if (stripped.base == NULL) {
                        stripped.base = new uint8_t[cluster.size];
                        no_keyframes = ebml_strip_reference_frames(cluster, &stripped);
                    }

                    if (no_keyframes)
                        continue;

                    if (c.write(c.data, stripped.base, stripped.size, 0) == 0)
                        c.had_keyframe = true;
                }

                else if (c.write(c.data, cluster.base, cluster.size, 0) != 0)
                    c.had_keyframe = false;
            }

            delete[] stripped.base;
            delete[] cluster.base;
        }

        /* also skip stuff in between and after clusters (cueing data, attachments, ...) */
        else if (!b->saw_clusters) {
            b->tracks.insert(b->tracks.end(), tagbuf.base, tagbuf.base + tagbuf.size);
            for (auto &c : b->callbacks)
                c.write(c.data, tagbuf.base, tagbuf.size, 1);
        }

        buf = ebml_buffer_advance(buf, tagbuf.size);
    }

    b->buffer.erase(b->buffer.begin(), b->buffer.begin() + (buf.base - &b->buffer[0]));
    return 0;
}


void webm_broadcast_stop(struct webm_broadcast_t *b)
{
    delete b;
}


static std::atomic<int> next_callback_id(0);


int webm_slot_connect(struct webm_broadcast_t *b, webm_write_cb *f, void *d, int skip_headers)
{
    int id = next_callback_id++;
    if (!skip_headers)
        f(d, &b->header[0], b->header.size(), 1);
    f(d, &b->tracks[0], b->tracks.size(), 1);
    b->callbacks.push_back((struct webm_slot_t) {f, d, id, skip_headers, false});
    return id;
}


void webm_slot_disconnect(struct webm_broadcast_t *b, int id)
{
    for (auto it = b->callbacks.begin(); it != b->callbacks.end(); )
        it = it->id == id ? b->callbacks.erase(it) : it + 1;
}
