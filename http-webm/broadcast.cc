#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include <vector>
#include <atomic>
#include <algorithm>
#include <functional>

extern "C" {
    #include "broadcast.h"
}


namespace EBML
{
    struct buffer
    {
        const uint8_t * base;
        size_t size;

        buffer(const uint8_t *b, size_t s) : base(b), size(s) {}
        buffer(const std::vector<uint8_t> &v) : buffer(&v[0], v.size()) {}

        buffer operator+(int shift) const
        {
            return buffer(base + shift, size - shift);
        }

        buffer& operator+=(int shift)
        {
            base += shift;
            size -= shift;
            return *this;
        }
    };


    enum struct ID : uint32_t
    {
        // https://www.matroska.org/technical/specs/index.html
        // All these constants have the length marker
        // (the most significant set bit) stripped.
        EBML           = 0x0A45DFA3UL,
        Segment        = 0x08538067UL,
        SeekHead       = 0x014D9B74UL,
        Info           = 0x0549A966UL,
        Tracks         = 0x0654AE6BUL,
        Cluster        = 0x0F43B675UL,
        Cues           = 0x0C53BB6BUL,
        Chapters       = 0x0043A770UL,
        Void           = 0x6CUL,
        CRC32          = 0x3FUL,
        // PrevSize (level 2 tag inside a Cluster) should be reset to 0
        // on the first frame sent over a connection.
        PrevSize       = 0x2BUL,
        // Third, or some later byte of a SimpleBlock (if it exists; level 2 tag
        // inside a Cluster) has 0-th bit set if this block only contains keyframes.
        SimpleBlock    = 0x23UL,
        // BlockGroups (level 2 tags inside Clusters) contain Blocks.
        // A block followed by a ReferenceBlock with a value of 0
        // is a keyframe. Probably.
        BlockGroup     = 0x20UL,
        Block          = 0x21UL,
        ReferenceBlock = 0x7BUL,
    };


    struct Tag
    {
        const ID     id;
        const size_t length;

        const char * name() const
        {
            switch (id) {
                case ID::EBML:     return "EBML";
                case ID::Segment:  return "Segment";
                case ID::SeekHead: return "SeekHead";
                case ID::Info:     return "Info";
                case ID::Tracks:   return "Tracks";
                case ID::Cluster:  return "Cluster";
                case ID::Void:     return "Void";
                case ID::CRC32:    return "CRC32";
                case ID::Cues:     return "Cues";
                default:           return "Unknown";
            }
        }

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


    static inline size_t parse_uint_size(uint8_t first_byte)
    {
        return __builtin_clz((int) first_byte) - (sizeof(int) - sizeof(uint8_t)) * 8;
    }


    Parsed<uint64_t> parse_uint(const struct buffer data)
    {
        if (data.size < 1)
            return Parsed<uint64_t> { 0, 0 };

        size_t length = parse_uint_size(data.base[0]);

        if (data.size < length + 1)
            return Parsed<uint64_t> { 0, 0 };

        uint64_t i = data.base[0] & (0x7F >> length);

        for (size_t k = 1; k <= length; k++)
            i = i << 8 | data.base[k];

        return Parsed<uint64_t> { length + 1, i };
    }


    Parsed<Tag> parse_tag(const struct buffer buf)
    {
        auto id = parse_uint(buf);
        if (!id.consumed)
            return Parsed<Tag> { 0, { ID(0), 0 } };

        auto length = parse_uint(buf + id.consumed);
        if (!length.consumed)
            return Parsed<Tag> { 0, { ID(0), 0 } };

        return Parsed<Tag> { id.consumed + length.consumed, { ID(id.value), length.value } };
    }

    int strip_reference_frames(EBML::buffer buffer, uint8_t *target, size_t *size)
    {
        auto cluster = EBML::parse_tag(buffer);
        auto refptr  = target;

        if (cluster.value.id == EBML::ID::Cluster) {
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
                auto tag = EBML::parse_tag(buffer);
                if (!tag.consumed)
                    return -1;

                if (tag.value.id == EBML::ID::PrevSize)
                    // there is no previous cluster, so this data is not applicable.
                    goto skip_tag;

                else if (tag.value.id == EBML::ID::SimpleBlock && !had_keyframe) {
                    if (tag.value.length < 4)
                        return -1;

                    // the very first field has a variable length. what a bummer.
                    // it doesn't even follow the same format as tag ids.
                 // auto field = buffer.base[tag.consumed];
                 // auto skip_field = EBML::parse_uint_size(~field) + 1;
                    auto skip_field = 1u;

                    if (tag.value.length < 3u + skip_field)
                        return -1;

                    if (!(buffer.base[tag.consumed + skip_field + 2] & 0x80))
                        goto skip_tag;  // nope, not a keyframe.

                    had_keyframe = true;
                }

                else if (tag.value.id == EBML::ID::BlockGroup && !had_keyframe) {
                    // a BlockGroup actually contains only a single Block.
                    // it does have some additional tags with metadata, though.
                    // if there's a nonzero ReferenceBlock, this is def not a keyframe.
                    auto sdata = EBML::buffer(buffer.base + tag.consumed, tag.value.length);

                    while (sdata.size) {
                        auto tag = EBML::parse_tag(sdata);
                        if (!tag.consumed)
                            return -1;

                        if (tag.value.id == EBML::ID::ReferenceBlock)
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

            if (!had_keyframe)
                return 0;

            *size = target - refptr;
            return 0;
        }

        return -1;
    }
}


static std::atomic<int> next_callback_id(0);


struct WebMBroadcaster
{
    struct Callback
    {
        int id;
        int (*write)(void *, const uint8_t *, size_t);
        void *data;
        bool had_keyframe;
    };

    std::vector<Callback> callbacks;
    std::vector<uint8_t> buffer;
    std::vector<uint8_t> header;
    bool saw_clusters = false;

    int connect(int (*f)(void *, const uint8_t *, size_t), void *d)
    {
        int id = next_callback_id++;
        callbacks.push_back(Callback {id, f, d, false});
        return id;
    }

    void disconnect(int id)
    {
        callbacks.erase(std::find_if(callbacks.begin(), callbacks.end(),
            [id](const Callback &c) { return c.id == id; }));
    }

    int broadcast(const uint8_t *data, size_t size)
    {
        buffer.insert(buffer.end(), data, data + size);
        EBML::buffer buf(buffer);

        while (1) {
            auto tag = EBML::parse_tag(buf);
            if (!tag.consumed)
                break;

            auto fwd_length = tag.consumed + tag.value.length;

            if (tag.value.id == EBML::ID::Segment) {
                if (tag.value.is_long_coded_endless()) {
                    uint8_t * writable = (uint8_t *) buf.base;
                    // tag id = 4 bytes, the rest is its length.
                    writable[4] = 0xFF;
                    // fill the rest of the space with a Void tag to avoid
                    // decoding errors.
                    writable[5] = 0x80 | (unsigned) EBML::ID::Void;
                    writable[6] = 0x80 | (tag.consumed - 7);
                }
                // forward only the headers of this tag.
                // we'll deal with the contents later.
                fwd_length = tag.consumed;
            } else if (tag.value.is_endless())
                // can't forward blocks on infinite size.
                return -1;

            if (fwd_length > buf.size)
                break;

            if (tag.value.id == EBML::ID::Cluster) {
                saw_clusters = true;  // ignore any further metadata

                uint8_t *refstripped = NULL;
                size_t   refstripped_length = 0;

                for (Callback &c : callbacks) {
                    if (c.had_keyframe) {
                        c.write(c.data, buf.base, fwd_length);
                        continue;
                    }

                    if (refstripped == NULL) {
                        refstripped = new uint8_t[buf.size];
                        EBML::buffer b(buf.base, fwd_length);
                        EBML::strip_reference_frames(b, refstripped, &refstripped_length);
                    }

                    if (refstripped_length != 0) {
                        c.had_keyframe = true;
                        c.write(c.data, refstripped, refstripped_length);
                    }
                }

                delete refstripped;
            } else if (!saw_clusters) {
                header.insert(header.end(), buf.base, buf.base + fwd_length);
                for (auto &c : callbacks)
                    c.write(c.data, buf.base, fwd_length);
            }

            buf += fwd_length;
        }

        buffer.erase(buffer.begin(), buffer.begin() + (buf.base - &buffer[0]));
        return 0;
    }
};


struct WebMBroadcaster * webm_broadcast_start(void)
{
    return new WebMBroadcaster;
}


int webm_broadcast_send(struct WebMBroadcaster *b, const uint8_t *d, size_t s)
{
    return b->broadcast(d, s);
}


void webm_broadcast_stop(struct WebMBroadcaster *b)
{
    delete b;
}


int webm_slot_connect(struct WebMBroadcaster *b, int (*f)(void *, const uint8_t *, size_t), void *d)
{
    f(d, &b->header[0], b->header.size());
    return b->connect(f, d);
}

void webm_slot_disconnect(struct WebMBroadcaster *b, int id)
{
    b->disconnect(id);
}
