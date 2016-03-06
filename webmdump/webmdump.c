// $CC -std=c11 webmdump.c -o webmdump -Wall -Wextra -Wno-unused-function
#include <errno.h>
#include <stdio.h>
#include <signal.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "ebml/api.h"
#include "ebml/buffer.h"
#include "ebml/binary.h"


static const char * ebml_tag_name(const struct ebml_tag t)
{
    switch (t.id) {
        case EBML_TAG_EBML:        return "EBML";
        case EBML_TAG_Void:        return "Void";
        case EBML_TAG_Segment:     return "Segment";
        case EBML_TAG_SeekHead:    return "Segment.SeekHead";
        case EBML_TAG_Info:        return "Segment.Info";
        case EBML_TAG_Tracks:      return "Segment.Tracks";
        case EBML_TAG_Cluster:     return "Segment.Cluster";
        case EBML_TAG_Timecode:    return "Segment.Cluster.Timecode";
        case EBML_TAG_PrevSize:    return "Segment.Cluster.PrevSize";
        case EBML_TAG_SimpleBlock: return "Segment.Cluster.SimpleBlock";
        case EBML_TAG_BlockGroup:  return "Segment.Cluster.BlockGroup";
        case EBML_TAG_Cues:        return "Segment.Cues";
        case EBML_TAG_Chapters:    return "Segment.Chapters";
        case EBML_TAG_Tags:        return "Segment.Tags";
    }

    static char unknown[16];
    snprintf(unknown, sizeof(unknown), "0x%X", (unsigned) t.id);
    return unknown;
}


int main(void)
{
    struct ebml_buffer_dyn buffer = EBML_BUFFER_EMPTY_DYN;

    while (1) {
        uint8_t data[4096];
        size_t i = fread(data, 1, 4096, stdin);

        if (i == 0)
            break;

        if (ebml_buffer_dyn_concat(&buffer, (struct ebml_buffer) { data, i }))
            return 1;

        struct ebml_buffer buf = ebml_buffer_static(&buffer);

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

        ebml_buffer_dyn_shift(&buffer, buf.data - buffer.data);
    }

    if (buffer.size) {
        struct ebml_buffer buf = ebml_buffer_static(&buffer);
        struct ebml_tag tag = ebml_parse_tag_incomplete(buf);
        if (!tag.consumed)
            printf("junk at end of stream\n");
        else
            printf("incomplete %s [%zu; got %zu]\n", ebml_tag_name(tag), tag.length, buffer.size);
    }

    ebml_buffer_dyn_clear(&buffer);
    return 0;
}
