#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include <vector>
#include <atomic>

extern "C" {
    #include "broadcast.h"
}


namespace EBML
{
    struct buffer
    {
        const uint8_t * base;
        size_t size;

        buffer operator+(int shift) const
        {
            return buffer { base + shift, size - shift };
        }

        buffer& operator+=(int shift)
        {
            return *this = *this + shift;
        }
    };


    enum struct ID : uint32_t  // https://www.matroska.org/technical/specs/index.html
    {
        EBML           = 0x1A45DFA3UL,
        Void           = 0xECUL,
        CRC32          = 0xBFUL,
        Segment        = 0x18538067UL,
        SeekHead       = 0x114D9B74UL,
        Info           = 0x1549A966UL,
        Cluster        = 0x1F43B675UL,
        PrevSize       = 0xABUL,
        SimpleBlock    = 0xA3UL,
        BlockGroup     = 0xA0UL,
        Block          = 0xA1UL,
        ReferenceBlock = 0xFBUL,
        Tracks         = 0x1654AE6BUL,
        Cues           = 0x1C53BB6BUL,
        Chapters       = 0x1043A770UL,
    };


    struct Tag
    {
        const ID     id;
        const size_t length;

        bool is_endless() const
        {
            return length == 0x0000007FUL || length == 0x000007FFFFFFFFULL ||
                   length == 0x00003FFFUL || length == 0x0003FFFFFFFFFFULL ||
                   length == 0x001FFFFFUL || length == 0x01FFFFFFFFFFFFULL ||
                   length == 0x0FFFFFFFUL || length == 0xFFFFFFFFFFFFFFULL;
        }

        bool is_long_coded_endless() const
        {
            // ffmpeg: "endless = 0xFFFFFFFFFFFFFF"
            // chrome: "endless = 0xFF"
            // matroska standard: "endless = all ones"
            // ...but the field is of variable length? what the fuck
            return length != 0x7FUL && length != 0x3FFFUL && is_endless();
        }
    };


    template <typename T> struct Parsed
    {
        const size_t consumed;
        const T      value;
    };


    static size_t parse_uint_size(uint8_t first_byte)
    {
        return __builtin_clz((int) first_byte) - (sizeof(int) - sizeof(uint8_t)) * 8;
    }


    static Parsed<uint64_t> parse_uint(const struct buffer data, bool preserve_marker = false)
    {
        if (data.size < 1)
            return Parsed<uint64_t> { 0, 0 };

        size_t length = parse_uint_size(data.base[0]);

        if (data.size < length + 1)
            return Parsed<uint64_t> { 0, 0 };

        uint64_t i = preserve_marker ? data.base[0] : data.base[0] & (0x7F >> length);

        for (size_t k = 1; k <= length; k++)
            i = i << 8 | data.base[k];

        return Parsed<uint64_t> { length + 1, i };
    }


    static Parsed<Tag> parse_tag(const struct buffer buf)
    {
        auto id = parse_uint(buf, true);
        if (!id.consumed)
            return Parsed<Tag> { 0, { ID(0), 0 } };

        auto length = parse_uint(buf + id.consumed);
        if (!length.consumed)
            return Parsed<Tag> { 0, { ID(0), 0 } };

        return Parsed<Tag> { id.consumed + length.consumed, { ID(id.value), length.value } };
    }

    static int strip_reference_frames(struct buffer buffer, uint8_t *target, size_t *size)
    {
        auto cluster = parse_tag(buffer);
        auto refptr  = target;

        if (cluster.value.id != ID::Cluster)
            return -1;

        bool had_keyframe = false;
        // ok so this is the first cluster we forward. if it references
        // older blocks/clusters (which this client doesn't have), the decoder
        // will error and drop the stream. so we need to drop frames
        // until the next keyframe. and boy is that hard.
        memcpy(target, buffer.base, cluster.consumed);
        target += cluster.consumed;

        // a cluster can actually contain many blocks. we can send
        // the first keyframe-only block and all that follow
        for (buffer += cluster.consumed; buffer.size; ) {
            auto tag = parse_tag(buffer);
            if (!tag.consumed)
                return -1;

            if (tag.value.id == ID::PrevSize)
                // there is no previous cluster, so this data is not applicable.
                goto skip_tag;

            else if (tag.value.id == ID::SimpleBlock && !had_keyframe) {
                if (tag.value.length < 4)
                    return -1;

                // the very first field has a variable length. what a bummer.
                // it doesn't even follow the same format as tag ids.
             // auto field = buffer.base[tag.consumed];
             // auto skip_field = parse_uint_size(~field) + 1;
                auto skip_field = 1u;

                if (tag.value.length < 3u + skip_field)
                    return -1;

                if (!(buffer.base[tag.consumed + skip_field + 2] & 0x80))
                    goto skip_tag;  // nope, not a keyframe.

                had_keyframe = true;
            }

            else if (tag.value.id == ID::BlockGroup && !had_keyframe) {
                // a BlockGroup actually contains only a single Block.
                // it does have some additional tags with metadata, though.
                // if there's a nonzero ReferenceBlock, this is def not a keyframe.
                struct buffer sdata = { buffer.base + tag.consumed, tag.value.length };

                while (sdata.size) {
                    auto tag = parse_tag(sdata);
                    if (!tag.consumed)
                        return -1;

                    if (tag.value.id == ID::ReferenceBlock)
                        for (size_t i = 0; i < tag.value.length; i++)
                            if (sdata.base[tag.consumed + i] != 0)
                                goto skip_tag;

                    sdata += tag.consumed + tag.value.length;
                }

                had_keyframe = true;
            }

            memcpy(target, buffer.base, tag.consumed + tag.value.length);
            target += tag.consumed + tag.value.length;

            skip_tag: buffer += tag.consumed + tag.value.length;
        }

        if (had_keyframe)
            *size = target - refptr;
        return 0;
    }
}


struct WebMSlot
{
    webm_write_cb *write;
    void *data;
    int id;
    bool had_keyframe;
};


struct WebMBroadcaster
{
    std::vector<WebMSlot> callbacks;
    std::vector<uint8_t> buffer;
    std::vector<uint8_t> header;  // [EBML .. Segment) -- once per webm
    std::vector<uint8_t> tracks;  // [Segment .. Cluster) -- can occur many times
    bool saw_clusters = false;
    bool saw_segments = false;
};


struct WebMBroadcaster * webm_broadcast_start(void)
{
    return new WebMBroadcaster;
}


int webm_broadcast_send(struct WebMBroadcaster *b, const uint8_t *data, size_t size)
{
    b->buffer.insert(b->buffer.end(), data, data + size);
    EBML::buffer buf = { &b->buffer[0], b->buffer.size() };

    while (1) {
        auto tag = EBML::parse_tag(buf);
        if (!tag.consumed)
            break;

        auto fwd_length = tag.consumed + tag.value.length;

        if (tag.value.id == EBML::ID::Segment) {
            if (tag.value.is_long_coded_endless()) {
                uint8_t * writable = (uint8_t *) buf.base;
                // tag id = 4 bytes, the rest is its length.
                writable[4] = 0xFF;  // a shorter representation of "endless"
                writable[5] = (unsigned) EBML::ID::Void;  // gotta hide all this
                writable[6] = 0x80 | (tag.consumed - 7);  // now-unused space
            }

            fwd_length = tag.consumed;
            b->saw_segments = true;
            b->saw_clusters = false;
            b->tracks.clear();
        } else if (tag.value.is_endless() || tag.value.length > 256 * 1024)
            return -1;

        if (fwd_length > buf.size)
            break;

        if (!b->saw_segments) {
            b->header.insert(b->header.end(), buf.base, buf.base + fwd_length);
            for (auto &c : b->callbacks)
                c.write(c.data, buf.base, fwd_length);
        } else if (tag.value.id == EBML::ID::Cluster) {
            b->saw_clusters = true;  // ignore any further metadata

            uint8_t *refstripped = NULL;
            size_t   refstripped_length = 0;

            for (auto &c : b->callbacks) {
                if (c.had_keyframe) {
                    c.write(c.data, buf.base, fwd_length);
                    continue;
                }

                if (refstripped == NULL) {
                    refstripped = new uint8_t[buf.size];
                    EBML::buffer b = { buf.base, fwd_length };
                    EBML::strip_reference_frames(b, refstripped, &refstripped_length);
                }

                if (refstripped_length != 0) {
                    c.had_keyframe = true;
                    c.write(c.data, refstripped, refstripped_length);
                }
            }

            delete refstripped;
        } else if (!b->saw_clusters) {
            b->tracks.insert(b->tracks.end(), buf.base, buf.base + fwd_length);
            for (auto &c : b->callbacks)
                c.write(c.data, buf.base, fwd_length);
        }

        buf += fwd_length;
    }

    b->buffer.erase(b->buffer.begin(), b->buffer.begin() + (buf.base - &b->buffer[0]));
    return 0;
}


void webm_broadcast_stop(struct WebMBroadcaster *b)
{
    delete b;
}


static std::atomic<int> next_callback_id(0);


int webm_slot_connect(struct WebMBroadcaster *b, webm_write_cb *f, void *d)
{
    int id = next_callback_id++;
    // TODO webm can contain multiple segments; what if we switch
    //      to a stream with different quality mid-air by sending
    //      a new track set and a new segment?
    f(d, &b->header[0], b->header.size());
    f(d, &b->tracks[0], b->tracks.size());
    b->callbacks.push_back({f, d, id, false});
    return id;
}

void webm_slot_disconnect(struct WebMBroadcaster *b, int id)
{
    for (auto it = b->callbacks.begin(); it != b->callbacks.end(); )
        it = it->id == id ? b->callbacks.erase(it) : it + 1;
}
