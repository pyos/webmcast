// $CXX -std=c++11 webmdump.cc -pthread -o webmdump -Wall -Wextra -Werror
#include "io-main.h"
#include "../broadcast.cc"


static const char * ebml_tag_name(struct EBML::Tag t)
{
    switch (t.id) {
        case EBML::ID::EBML:           return "EBML";
        case EBML::ID::Void:           return "Void";
        case EBML::ID::CRC32:          return "CRC32";
        case EBML::ID::Segment:        return "Segment";
        case EBML::ID::SeekHead:       return "SeekHead";
        case EBML::ID::Info:           return "Info";
        case EBML::ID::Cluster:        return "Cluster";
        case EBML::ID::PrevSize:       return "PrevSize";
        case EBML::ID::SimpleBlock:    return "SimpleBlock";
        case EBML::ID::BlockGroup:     return "BlockGroup";
        case EBML::ID::Block:          return "Block";
        case EBML::ID::ReferenceBlock: return "ReferenceBlock";
        case EBML::ID::Tracks:         return "Tracks";
        case EBML::ID::Cues:           return "Cues";
        case EBML::ID::Chapters:       return "Chapters";
    }

    static char unknown[32];
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
        printf("<%d> ---\n", id);
    }

    int data_received(const struct aio::stringview data)
    {
        buffer.append(data.base, data.size);
        EBML::buffer buf = { (const uint8_t *) buffer.data(), buffer.size() };

        while (1) {
            auto tag = EBML::parse_tag(buf);
            if (!tag.consumed)
                break;

            if (!tag.value.is_endless() && tag.value.id != EBML::ID::Segment) {
                if (tag.consumed + tag.value.length > buf.size)
                    break;
                buf += tag.value.length;
            }

            buf += tag.consumed;
            printf("<%d> %s [%zu]\n", id, ebml_tag_name(tag.value), tag.value.length);
        }

        buffer.erase(0, (const char *) buf.base - buffer.data());
        return 0;
    }
};


int protocol::_id = 0;


int main(void)
{
    return aio_main::run_server([](aio::transport *t) { return new protocol(t); });
}
