// $CXX -std=c++11 webmdump.cc -o webmdump -Wall -Wextra -Wno-unused-function
#include <errno.h>
#include <stdio.h>
#include <signal.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "ebml/buffer.h"
#include "ebml/binary.h"

#include <string>


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


int main(void)
{
    std::string buffer;

    while (1) {
        char data[4096];
        auto i = fread(data, 1, 4096, stdin);
        if (i == 0)
            break;

        buffer.append(data, i);

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
            printf("%s [%zu]\n", ebml_tag_name(tag), tag.length);
        }

        buffer.erase(0, (const char *) buf.data - buffer.data());
    }

    if (buffer.size()) {
        struct ebml_buffer buf = { (uint8_t *) buffer.data(), buffer.size() };
        struct ebml_tag tag = ebml_parse_tag_incomplete(buf);
        if (!tag.consumed)
            printf("junk at end of stream\n");
        else
            printf("incomplete %s [%zu; got %zu]\n", ebml_tag_name(tag), tag.length, buffer.size());
    }

    return 0;
}
