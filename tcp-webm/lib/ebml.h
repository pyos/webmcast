#ifndef EBML_H
#define EBML_H

#include <stddef.h>
#include <stdint.h>

#include <vector>

#include "io.h"


namespace EBML
{
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


    Parsed<uint64_t> parse_uint(const struct aio::stringview data)
    {
        if (data.size < 1)
            return Parsed<uint64_t> { 0, 0 };

        const uint8_t *buf = (const uint8_t *) data.base;
        size_t length = parse_uint_size(buf[0]);

        if (data.size < length + 1)
            return Parsed<uint64_t> { 0, 0 };

        uint64_t i = buf[0] & (0x7F >> length);

        for (size_t k = 1; k <= length; k++)
            i = i << 8 | buf[k];

        return Parsed<uint64_t> { length + 1, i };
    }


    Parsed<Tag> parse_tag(const struct aio::stringview buf)
    {
        auto id = parse_uint(buf);
        if (!id.consumed)
            return Parsed<Tag> { 0, { ID(0), 0 } };

        auto length = parse_uint(buf + id.consumed);
        if (!length.consumed)
            return Parsed<Tag> { 0, { ID(0), 0 } };

        return Parsed<Tag> { id.consumed + length.consumed, { ID(id.value), length.value } };
    }
}

#endif
