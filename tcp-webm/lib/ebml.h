#ifndef EBML_H
#define EBML_H

#include <stddef.h>
#include <stdint.h>

#include <vector>

#include "io.h"


namespace EBML
{
    struct ID
    {
        // https://www.matroska.org/technical/specs/index.html
        // All these constants have the length marker
        // (the most significant set bit) stripped.
        static const uint64_t EBML        = 0x0A45DFA3ull;
        static const uint64_t Segment     = 0x08538067ull;
        static const uint64_t SeekHead    = 0x014D9B74ull;
        static const uint64_t Info        = 0x0549A966ull;
        static const uint64_t Tracks      = 0x0654AE6Bull;
        static const uint64_t Cluster     = 0x0F43B675ull;
        static const uint64_t Cues        = 0x0C53BB6Bull;
        static const uint64_t Attachments = 0x0941A469ull;  // not webm
        static const uint64_t Chapters    = 0x0043A770ull;
        static const uint64_t Tags        = 0x0254C367ull;  // not webm
        static const uint64_t Void        = 0x6Cull;
        static const uint64_t CRC32       = 0x3Full;

        // PrevSize (level 2 tag inside a Cluster) should be reset to 0
        // on the first frame sent over a connection.
        static const uint64_t PrevSize = 0x2Bull;
        // Second byte of a SimpleBlock (if it exists; level 2 tag inside a Cluster)
        // has 0-th bit set if this block describes a keyframe.
        static const uint64_t SimpleBlock = 0x23ull;
        // BlockGroups (level 2 tags inside Clusters) contain Blocks.
        // A block followed by a ReferenceBlock with a value of 0
        // is a keyframe.
        static const uint64_t BlockGroup = 0x20ull;
        static const uint64_t Block = 0x21ull;
        static const uint64_t ReferenceBlock = 0x7Bull;

        static const char * name(uint64_t id)
        {
            switch (id) {
                case EBML:     return "EBML";
                case Segment:  return "Segment";
                case SeekHead: return "SeekHead";
                case Info:     return "Info";
                case Tracks:   return "Tracks";
                case Cluster:  return "Cluster";
                case Void:     return "Void";
                case CRC32:    return "CRC32";
                case Cues:     return "Cues";
                default:       return "Unknown";
            }
        }
    };

    struct Marker
    {
        // Elements with this length have indeterminate size.
        static const uint64_t ENDLESS = 0xFFFFFFFFFFFFFFull;
    };


    static size_t get_encoded_uint_size(uint8_t first_byte)
    {
        return __builtin_clz((int) first_byte) - (sizeof(int) - sizeof(uint8_t)) * 8;
    }


    std::pair<size_t, uint64_t>
    consume_encoded_uint(const struct aio::stringview data, size_t offset)
    {
        const uint8_t *buf = (const uint8_t *) data.base + offset;
        size_t size = data.size - offset;

        if (!size)
            return std::pair<size_t, uint64_t>{0u, 0u};

        size_t length = get_encoded_uint_size(buf[0]);

        if (size <= length)
            return std::pair<size_t, uint64_t>{0u, 0u};

        uint64_t i = buf[0] & (0x7F >> length);

        for (size_t k = 0; k < length; k++)
            i = i << 8 | *++buf;

        return std::make_pair(offset + length + 1, i);
    }


    size_t
    consume_tag_header(const struct aio::stringview buf, size_t offset)
    {
        // header = 2 uints
        for (int i = 0; i < 2; i++) {
            if (offset >= buf.size)
                return 0;

            offset += get_encoded_uint_size(buf.base[offset]) + 1;
        }

        return offset;
    }


    std::pair<size_t, uint64_t>
    consume_tag(const struct aio::stringview buf, size_t offset)
    {
        auto tag_id = consume_encoded_uint(buf, offset);
        if (!tag_id.first)
            return tag_id;

        auto length = consume_encoded_uint(buf, tag_id.first);
        if (!length.first)
            return length;

        if (length.second == Marker::ENDLESS || tag_id.second == ID::Segment)
            return std::make_pair(length.first, tag_id.second);

        if (length.first + length.second <= buf.size)
            // there are nested elements inside, but we don't care.
            return std::make_pair(length.first + length.second, tag_id.second);

        return std::pair<size_t, uint64_t>{ 0u, 0u };
    }


    std::pair<bool, std::vector<char>>
    drop_non_key_frames_from_cluster(const struct aio::stringview buf)
    {
        auto cluster = consume_tag(buf, 0);
        if (!cluster.first || cluster.second != ID::Cluster)
            // not actually a cluster
            return std::make_pair(false, std::vector<char>());

        std::vector<char> copy(buf.size);
        auto *target = &copy[0];
        #define COPY(offset, length) do {              \
            memcpy(target, buf.base + offset, length); \
            target += length;                          \
        } while (0)

        auto offset = consume_tag_header(buf, 0);
        auto have_blocks = false;
        COPY(0, offset);

        while (offset < buf.size) {
            auto tag = consume_tag(buf, offset);
            if (!tag.first)
                goto error;

            if (tag.second == ID::PrevSize)
                // there is no previous cluster.
                goto skip_tag;

            if (tag.second == ID::SimpleBlock && !have_blocks) {
                auto start = consume_tag_header(buf, offset);

                if (tag.first - start < 4)
                    // malformed SimpleBlock -- length < 4
                    goto error;

                if (!(buf.base[start + 3] & 0x80))
                    goto skip_tag;  // nope, not a keyframe.

                have_blocks = true;
            }

            if (tag.second == ID::BlockGroup) {
                auto off2 = consume_tag_header(buf, offset);

                while (off2 < tag.first) {
                    auto tag2 = consume_tag(buf, off2);
                    if (!tag2.first)
                        goto error;

                    if (tag2.second == ID::ReferenceBlock) {
                        auto start2 = consume_tag_header(buf, off2);

                        for (; start2 < tag2.first; start2++)
                            if (buf.base[start2] != 0)
                                // nonzero ReferenceBlock => not a keyframe
                                goto skip_tag;
                    }
                }

                have_blocks = true;
            }

            COPY(offset, tag.first - offset);

            skip_tag: offset = tag.first;
        }

        if (have_blocks) {
            copy.resize(target - &copy[0]);
            return std::make_pair(true, copy);
        }

        error: return std::make_pair(false, std::vector<char>());
    }
}

#endif
