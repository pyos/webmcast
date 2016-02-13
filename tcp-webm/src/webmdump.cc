// $CXX -std=c++11 tcpdump.cc -pthread -o tcpdump -Wall -Wextra -Werror
#include <ctype.h>
#include <stdio.h>

#include <string>

#include "io.h"
#include "io-main.h"


char to_hex(int i)
{
    return 0 <= i && i < 10 ? '0' + i : 10 <= i && i < 16 ? 'a' + i - 10 : '?';
}


struct dump_protocol : aio::protocol
{
    const  int  id;
    static int _id;
    std::string buffer;


    dump_protocol(aio::transport *t) : aio::protocol(t), id(_id++)
    {
        printf("<%d> +++\n", id);
    }

    virtual ~dump_protocol()
    {
        printf("<%d> ---\n", id);
    }

    int data_received(const struct aio::stringview data)
    {
        buffer.append(data.base, data.size);

        size_t result   = 0;
        size_t consumed = 0;
        while ((result = consume_tag(consumed)))
            consumed = result;

        buffer.erase(0, consumed);
        return 0;
    }

    struct ID
    {
        // https://www.matroska.org/technical/specs/index.html
        // Only top-level (level 0 and level 1 inside Segments)
        // elements are listed here, as we don't really need all the other
        // information. All these constants have the length marker
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

    struct EBMLMarker
    {
        // Elements with this length have indeterminate size.
        static const uint64_t ENDLESS = 0xFFFFFFFFFFFFFFull;
    };

    std::pair<size_t, uint64_t> consume_encoded_uint(size_t offset)
    {
        const uint8_t *buf = (const uint8_t *) buffer.data() + offset;
        size_t size = buffer.size() - offset;

        if (!size)
            return std::pair<size_t, uint64_t>{0u, 0u};

        size_t length = __builtin_clz(buf[0]) - (sizeof(int) - 1) * 8;

        if (size <= length)
            return std::pair<size_t, uint64_t>{0u, 0u};

        uint64_t i = buf[0] & (0x7F >> length);

        for (size_t k = 0; k < length; k++)
            i = i << 8 | *++buf;

        return std::make_pair(offset + length + 1, i);
    }

    size_t consume_tag(size_t offset)
    {
        auto tag_id = consume_encoded_uint(offset);

        if (!tag_id.first)
            return 0;

        auto length = consume_encoded_uint(tag_id.first);
        if (!length.first)
            return 0;

        if (length.second == EBMLMarker::ENDLESS) {
            printf(">>> %s [ENDLESS]\n", ID::name(tag_id.second));
            return length.first;
        }

        if (tag_id.second == ID::Segment) {
            printf(">>> Segment [%zu]\n", length.second);
            return length.first;
        }

        if (length.first + length.second < buffer.size()) {
            printf(">>> %s [%zu]\n", ID::name(tag_id.second), length.second);
            // there are nested elements inside, but we don't care.
            return length.first + length.second;
        }

        return 0;
    }
};


int dump_protocol::_id = 0;


int main(void)
{
    return aio_main::run_server([](aio::transport *t) { return new dump_protocol(t); });
}
