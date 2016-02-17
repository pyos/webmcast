// #include "ffi.h"
// #include "buffer.h"
// #include "binary.h"
// #include "rewriting.h"
#ifndef EBML_BROADCAST_H
#define EBML_BROADCAST_H


struct callback
{
    int id;
    int skip_headers;
    int had_keyframe;
    void *data;
    on_chunk *write;
};


struct callback_array
{
    struct callback *xs;
    size_t size;
    size_t reserve;
};


#define EACH_CALLBACK(var, array) \
    (struct callback *var = &(array).xs[0]; var < &(array).xs[(array).size]; var++)


struct broadcast
{
    struct ebml_buffer_dyn buffer;
    struct ebml_buffer_dyn header;  // [EBML .. Segment) -- once per webm
    struct ebml_buffer_dyn tracks;  // [Segment .. Cluster) -- can occur many times
    struct callback_array recvs;
    unsigned long long time_shift;
    unsigned long long time_last;
};


struct broadcast * broadcast_start(void)
{
    return (struct broadcast *) calloc(1, sizeof(struct broadcast));
}


int broadcast_send(struct broadcast *cast, const uint8_t *data, size_t size)
{
    if (ebml_buffer_dyn_concat(&cast->buffer, ebml_view(data, size)))
        return -1;

    while (1) {
        struct ebml_buffer buf = ebml_buffer_static(&cast->buffer);
        struct ebml_tag    tag = ebml_parse_tag_incomplete(buf);
        if (!tag.consumed)
            break;

        buf.size = tag.consumed + tag.length;

        if (tag.id == EBML_TAG_Segment) {
            buf.size = tag.consumed;  /* forward the header, parse the contents */

            if (tag.length == EBML_INDETERMINATE && tag.consumed >= 7) {
                cast->buffer.data[4] = 0xFF;           /* EBML_TAG_Segment = 4 octets */
                cast->buffer.data[5] = EBML_TAG_Void;  /* EBML_TAG_Void = 1 octet */
                cast->buffer.data[6] = 0x80 | (tag.consumed - 7);
            }
        }

        if (buf.size > 1024 * 1024)
            return -1;

        if (buf.size > cast->buffer.size)
            break;

        switch (tag.id) {
            case EBML_TAG_EBML:
                if (!cast->header.size) {
                    if (ebml_buffer_dyn_concat(&cast->header, buf))
                        return -1;

                    for EACH_CALLBACK(c, cast->recvs)
                        if (!c->skip_headers)
                            c->write(c->data, buf.data, buf.size, 1);
                }
                break;

            case EBML_TAG_Segment:
                ebml_buffer_dyn_clear(&cast->tracks);

            case EBML_TAG_Info:
            case EBML_TAG_Tracks:
                if (ebml_buffer_dyn_concat(&cast->tracks, buf))
                    return -1;

                for EACH_CALLBACK(c, cast->recvs)
                    c->had_keyframe = !c->write(c->data, buf.data, buf.size, 1);

            case EBML_TAG_SeekHead:
            case EBML_TAG_Chapters:
            case EBML_TAG_Cues:
            case EBML_TAG_Void:
                break;

            case EBML_TAG_Cluster: {
                struct ebml_buffer_dyn fixed  = EBML_BUFFER_EMPTY_DYN;
                struct ebml_buffer_dyn unrefd = EBML_BUFFER_EMPTY_DYN;
                int no_keyframes = -2;

                if (!ebml_adjust_timecode(buf, &fixed, &cast->time_shift, &cast->time_last)) {
                    struct ebml_buffer cluster = fixed.data ? ebml_buffer_static(&fixed) : buf;

                    for EACH_CALLBACK(c, cast->recvs) {
                        if (c->had_keyframe)
                            c->had_keyframe = !c->write(c->data, cluster.data, cluster.size, 0);
                        else {
                            if (no_keyframes == -2)
                                 no_keyframes = ebml_strip_reference_frames(cluster, &unrefd);

                            if (!no_keyframes)
                                c->had_keyframe = !c->write(c->data, unrefd.data, unrefd.size, 0);
                        }

                    }
                }

                ebml_buffer_dyn_clear(&unrefd);
                ebml_buffer_dyn_clear(&fixed);
                break;
            }

            default: return -1;
        }

        ebml_buffer_dyn_shift(&cast->buffer, buf.size);
    }

    return 0;
}


void broadcast_stop(struct broadcast *cast)
{
    ebml_buffer_dyn_clear(&cast->buffer);
    ebml_buffer_dyn_clear(&cast->header);
    ebml_buffer_dyn_clear(&cast->tracks);
    free(cast->recvs.xs);
    free(cast);
}


int next_callback_id = 0;
int broadcast_connect(struct broadcast *cast, on_chunk *cb, void *data, int skip_headers)
{
    if (!skip_headers)
        cb(data, cast->header.data, cast->header.size, 1);

    cb(data, cast->tracks.data, cast->tracks.size, 1);

    if (!cast->recvs.reserve) {
        struct callback *m = (struct callback *)
                malloc(sizeof(struct callback) * (cast->recvs.size + 5));

        if (m == NULL)
            return -1;

        memcpy(m, cast->recvs.xs, sizeof(struct callback) * cast->recvs.size);
        cast->recvs.reserve = 5;
        cast->recvs.xs = m;
    }

    int id = next_callback_id++;
    cast->recvs.xs[cast->recvs.size] = (struct callback) { id, skip_headers, 0, data, cb };
    cast->recvs.size++;
    cast->recvs.reserve--;
    return id;
}


void broadcast_disconnect(struct broadcast *cast, int id)
{
    for EACH_CALLBACK(c, cast->recvs) {
        if (c->id == id) {
            memmove(c, c + 1, (&cast->recvs.xs[cast->recvs.size] - (c + 1))
                                 * sizeof(struct callback));
            cast->recvs.size--;
            cast->recvs.reserve++;
            return;
        }
    }
}


#undef EACH_CALLBACK
#endif
