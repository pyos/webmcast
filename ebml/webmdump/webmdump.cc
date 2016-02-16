// $CXX -std=c++11 webmdump.cc -pthread -o webmdump -Wall -Wextra -Werror
#include "io-main.h"
#include "../buffer.h"
#include "../binary.h"


static const char * ebml_tag_name(const struct ebml_tag t)
{
    switch (t.id) {
        case EBML_TAG_EBML:           return "EBML";
        case EBML_TAG_Void:           return "Void";
        case EBML_TAG_CRC32:          return "CRC32";
        case EBML_TAG_Segment:        return "Segment";
        case EBML_TAG_SeekHead:       return "SeekHead";
        case EBML_TAG_Info:           return "Info";
        case EBML_TAG_Cluster:        return "Cluster";
        case EBML_TAG_PrevSize:       return "PrevSize";
        case EBML_TAG_SimpleBlock:    return "SimpleBlock";
        case EBML_TAG_BlockGroup:     return "BlockGroup";
        case EBML_TAG_Block:          return "Block";
        case EBML_TAG_ReferenceBlock: return "ReferenceBlock";
        case EBML_TAG_Tracks:         return "Tracks";
        case EBML_TAG_Cues:           return "Cues";
        case EBML_TAG_Chapters:       return "Chapters";
    }

    static char unknown[16];
    snprintf(unknown, sizeof(unknown), "0x%X", (unsigned) t.id);
    return unknown;
}


struct protocol : aio::protocol
{
    const  int  id;
    static int _id;
    std::string buffer;

    protocol(aio::transport *t) : aio::protocol(t), id(_id++)
    {
        printf("<%d> +++\n", id);
    }

    virtual ~protocol()
    {
        if (buffer.size()) {
            struct ebml_buffer buf = { (uint8_t *) buffer.data(), buffer.size() };
            struct ebml_tag tag = ebml_parse_tag(buf);
            if (!tag.consumed)
                printf("<%d> junk at end of stream\n", id);
            else
                printf("<%d> incomplete %s [%zu; got %zu]\n", id,
                    ebml_tag_name(tag), tag.length, buffer.size());
        }
        printf("<%d> ---\n", id);
    }

    int data_received(const struct aio::stringview data)
    {
        buffer.append(data.base, data.size);
        struct ebml_buffer buf = { (uint8_t *) buffer.data(), buffer.size() };

        while (1) {
            struct ebml_tag tag = ebml_parse_tag(buf);
            if (!tag.consumed)
                break;

            if (tag.length != EBML_INDETERMINATE && tag.id != EBML_TAG_Segment) {
                if (tag.consumed + tag.length > buf.size)
                    break;
                buf = ebml_buffer_shift(buf, tag.length);
            }

            buf = ebml_buffer_shift(buf, tag.consumed);
            printf("<%d> %s [%zu]\n", id, ebml_tag_name(tag), tag.length);
        }

        buffer.erase(0, (const char *) buf.data - buffer.data());
        return 0;
    }
};


int protocol::_id = 0;


int main(void)
{
    return aio_main::run_server([](aio::transport *t) { return new protocol(t); });
}
