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


static const uint64_t EBML_INDETERMINATE = 0xFFFFFFFFFFFFFFULL;
static const uint64_t EBML_INDETERMINATE_MARKERS[] = {
    // shortest encodings of uints with these values have special meaning
    0x0000000000007FULL, 0x00000000003FFFULL, 0x000000001FFFFFULL, 0x0000000FFFFFFFULL,
    0x000007FFFFFFFFULL, 0x0003FFFFFFFFFFULL, 0x01FFFFFFFFFFFFULL, 0xFFFFFFFFFFFFFFULL,
};


static inline struct ebml_buffer ebml_buffer_advance(struct ebml_buffer b, size_t shift)
{
    return (struct ebml_buffer) { b.base + shift, b.size - shift };
}


static inline struct ebml_buffer ebml_buffer_contents(struct ebml_buffer b, struct ebml_tag t)
{
    return (struct ebml_buffer) { b.base + t.consumed, t.length };
}


static inline uint64_t ebml_parse_fixed_uint(const uint8_t *buf, size_t length)
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
static inline size_t ebml_parse_uint_size(uint8_t first_byte)
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
    if (i == EBML_INDETERMINATE_MARKERS[length - 1])
        i = EBML_INDETERMINATE;

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


static inline uint8_t *ebml_write_fixed_uint(uint8_t *t, uint64_t v, size_t size)
{
    while (size--) *t++ = v >> (8 * size);
    return t;
}


static inline uint8_t *ebml_write_uint(uint8_t *t, uint64_t v, bool has_marker)
{
    size_t size = 0;
    while (v >> ((7 + has_marker) * size)) size++;

    if (v && v < EBML_INDETERMINATE && EBML_INDETERMINATE_MARKERS[size - 1] == v)
        size++;  /* encode as a longer sequence to avoid placing an indeterminate value */

    return ebml_write_fixed_uint(t, has_marker ? v : v | 1ull << (7 * size), size);
}


static inline uint8_t *ebml_write_tag(uint8_t *t, struct ebml_tag v)
{
    return ebml_write_uint(ebml_write_uint(t, v.id, true), v.length, false);
}


static inline uint8_t *ebml_memcpy(uint8_t *t, uint8_t *s, size_t size)
{
    return (uint8_t *) memcpy(t, s, size) + size;
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

    if (cluster.id != EBML_TAG_Cluster || cluster.consumed + cluster.length > buffer.size)
        return -1;

    uint64_t found_keyframe = 0;  /* 1 bit per track (up to 64) */
    uint64_t seen_tracks = 0;
    uint8_t *p = ebml_memcpy(out->base, buffer.base, cluster.consumed);
    uint8_t *q = out->base + 4 /* length of EBML_TAG_Cluster */;
    uint8_t *r = p;

    for (buffer = ebml_buffer_contents(buffer, cluster); buffer.size;) {
        struct ebml_tag tag = ebml_parse_tag(buffer);

        if (!tag.consumed || tag.consumed + tag.length > buffer.size)
            return -1;

        if (tag.id == EBML_TAG_SimpleBlock) {
            const ebml_uint track = ebml_parse_uint(ebml_buffer_advance(buffer, tag.consumed), 0);

            if (!track.consumed || tag.length < track.consumed + 3 || track.value >= 64)
                return -1;

            seen_tracks |= 1ull << track.value;

            if (!(found_keyframe & (1ull << track.value))) {
                if (!(buffer.base[tag.consumed + track.consumed + 2] & 0x80))
                    goto skip_tag;

                found_keyframe |= 1 << track.value;
            }
        }

        else if (tag.id == EBML_TAG_BlockGroup) {
            /* a `BlockGroup` actually contains only a single `Block`. it does
               have some additional tags with metadata, though. we're looking
               for one either w/o a `ReferenceBlock`, or with a zeroed one. */
            struct ebml_uint track = { 0, 0 };
            uint64_t refblock = 0;

            for (struct ebml_buffer sdata = ebml_buffer_contents(buffer, tag); sdata.size;) {
                struct ebml_tag tag = ebml_parse_tag(sdata);
                if (!tag.consumed)
                    return -1;

                if (tag.id == EBML_TAG_Block)
                    track = ebml_parse_uint(ebml_buffer_advance(sdata, tag.consumed), 0);

                if (tag.id == EBML_TAG_ReferenceBlock)
                    refblock = ebml_parse_fixed_uint(sdata.base + tag.consumed, tag.length);

                sdata = ebml_buffer_advance(sdata, tag.consumed + tag.length);
            }

            if (!track.consumed || track.value >= 64)
                return -1;

            seen_tracks |= 1ull << track.value;

            if (refblock != 0 && !(found_keyframe & (1ull << track.value)))
                goto skip_tag;

            found_keyframe |= 1ull << track.value;
        }

        p = ebml_memcpy(p, buffer.base, tag.consumed + tag.length);
        skip_tag: buffer = ebml_buffer_advance(buffer, tag.consumed + tag.length);
    }

    out->size = p - out->base;
    /* have to recode cluster's length. `p - r <= cluster.length` => fits in `r - q` bytes. */
    ebml_write_fixed_uint(q, (p - r) | 1ull << (7 * (r - q)), r - q);
    return found_keyframe != seen_tracks;
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
                                uint64_t *shift, uint64_t *minimum)
{
    struct ebml_buffer start = buffer;
    struct ebml_tag cluster = ebml_parse_tag(buffer);

    if (cluster.id != EBML_TAG_Cluster || cluster.consumed + cluster.length > buffer.size)
        return -1;

    for (buffer = ebml_buffer_contents(buffer, cluster); buffer.size;)
    {
        struct ebml_tag tag = ebml_parse_tag(buffer);

        if (!tag.consumed || tag.consumed + tag.length > buffer.size)
            return -1;

        if (tag.id == EBML_TAG_Timecode) {
            uint64_t tc = ebml_parse_fixed_uint(buffer.base + tag.consumed, tag.length);

            if (*shift + tc < *minimum)
                *shift = *minimum - tc;
            *minimum = tc += *shift;

            if (*shift) {
                start = ebml_buffer_advance(start, cluster.consumed);
                cluster.length += 8 - tag.length;
                uint8_t *p = ebml_write_tag(out->base, cluster);
                p = ebml_memcpy(p, start.base, buffer.base - start.base);
                p = ebml_write_tag(p, (struct ebml_tag) { 0, 8, EBML_TAG_Timecode });
                p = ebml_write_fixed_uint(p, tc, 8);
                buffer = ebml_buffer_advance(buffer, tag.consumed + tag.length);
                out->size = ebml_memcpy(p, buffer.base, buffer.size) - out->base;
            } else
                memcpy(out->base, start.base, out->size = start.size);
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
    uint64_t timecode_shift = 0;
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
            if (tag.length == EBML_INDETERMINATE && tag.consumed >= 7) {
                buf.base[4] = 0xFF;           /* EBML_TAG_Segment = 4 octets */
                buf.base[5] = EBML_TAG_Void;  /* EBML_TAG_Void = 1 octet */
                buf.base[6] = 0x80 | (tag.consumed - 7);
            }

            tagbuf.size = tag.consumed;  /* forward the header, parse the contents */
            b->saw_segments = true;
            b->saw_clusters = false;
            b->tracks.clear();
        } else if (tag.length > 1024 * 1024)
            /* wow, a megabyte-sized cluster? not forwarding that. */
            return -1;

        if (tagbuf.size > buf.size)
            break;

        if (tag.id == EBML_TAG_Cluster) {
            struct ebml_buffer cluster = { new uint8_t[tagbuf.size + 8], 0 };

            if (!ebml_adjust_timecode(tagbuf, &cluster, &b->timecode_shift, &b->timecode_last)) {
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
            }

            delete[] cluster.base;
        }
        /* skip this tag. it's not required by the spec.
           it contains offsets relative to the beginning of the first segment,
           and we don't know how much data our subscribers got already. */
        else if (tag.id == EBML_TAG_SeekHead) {}
        /* ignore duplicate EBML headers. the broadcaster probably had a connection
           failure or something. */
        else if (!b->saw_segments) {
            b->header.insert(b->header.end(), tagbuf.base, tagbuf.base + tagbuf.size);
            for (auto &c : b->callbacks)
                if (!c.skip_headers)
                    c.write(c.data, tagbuf.base, tagbuf.size, 1);
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
