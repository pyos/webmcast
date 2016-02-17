// $CXX -std=c++11 webmdump.cc -o webmdump -Wall -Wextra -Wno-unused-function
#include <errno.h>
#include <stdio.h>
#include <signal.h>

#include "io.h"
#include "ebml/buffer.h"
#include "ebml/binary.h"

#ifndef PORT
#define PORT 12345
#endif


static const char * ebml_tag_name(const struct ebml_tag t)
{
    switch (t.id) {
        case EBML_TAG_EBML:           return "EBML";
        case EBML_TAG_Void:           return "Void";
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


static int next_protocol_id = 0;
struct protocol : aio::protocol
{
    const int id;
    std::string buffer;

    protocol(aio::transport *t) : aio::protocol(t), id(next_protocol_id++)
    {
        printf("<%d> +++\n", id);
    }

    virtual ~protocol()
    {
        if (buffer.size()) {
            struct ebml_buffer buf = { (uint8_t *) buffer.data(), buffer.size() };
            struct ebml_tag tag = ebml_parse_tag_incomplete(buf);
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
            struct ebml_tag tag = ebml_parse_tag_incomplete(buf);
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


static aio::evloop loop;


static void sigcatch(int)
{
    loop.stop();
    signal(SIGINT, SIG_DFL);
}


int main(void)
{
    signal(SIGINT, &sigcatch);

    fprintf(stderr, "[-] 127.0.0.1:%d\n", PORT);
    aio::server server(&loop, [](aio::transport *t) { return new protocol(t); }, 0, PORT);

    if (!server.ok) {
        fprintf(stderr, "[%d] could not create a server: %s\n", errno, strerror(errno));
        return 1;
    }

    if (loop.run()) {
        fprintf(stderr, "[%d] loop terminated: %s\n", errno, strerror(errno));
        return 1;
    }

    return 0;
}
