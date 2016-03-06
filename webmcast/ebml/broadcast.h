// #include "api.h"
// #include "buffer.h"
// #include "binary.h"
#ifndef EBML_BROADCAST_H
#define EBML_BROADCAST_H


enum EBML_CONST
{
    MAX_TRACK = sizeof(int) * 8 - 2,
    MAX_BUFFER_SIZE = 1024 * 1024,
};


struct callback
{
    int id;
    unsigned skip_headers : 1;
    unsigned skip_cluster : 1;
    unsigned keyframes : MAX_TRACK;
    void *data;
    on_chunk *write;
};


#define EACH_CALLBACK(var, array) \
    (struct callback *var = &(array).xs[0]; var < &(array).xs[(array).size]; var++)


int broadcast_start(struct broadcast *cast)
{
    return 0;
}


int broadcast_send(struct broadcast *cast, const uint8_t *data, size_t size)
{
    if (ebml_buffer_dyn_concat(&cast->buffer, (struct ebml_buffer) { data, size }))
        return -1;

    while (1) {
        struct ebml_buffer buf = ebml_buffer_static(&cast->buffer);
        struct ebml_tag    tag = ebml_parse_tag_incomplete(buf);
        if (!tag.consumed)
            break;

        if (tag.id == EBML_TAG_Segment || tag.id == EBML_TAG_Cluster || tag.id == EBML_TAG_Tracks) {
            buf.size = tag.consumed;  /* forward the header, parse the contents */

            if (tag.length == EBML_INDETERMINATE && tag.consumed >= 7) {
                // XXX Chrome crashes if an indeterminate length is not encoded as 0xFF.
                cast->buffer.data[4] = 0xFF;           /* EBML_TAG_Segment = 4 octets */
                cast->buffer.data[5] = EBML_TAG_Void;  /* EBML_TAG_Void = 1 octet */
                cast->buffer.data[6] = 0x80 | (tag.consumed - 7);
            }
        } else {
            if (tag.consumed + tag.length > MAX_BUFFER_SIZE)
                // too much metadata.
                return -1;

            buf.size = tag.consumed + tag.length;
        }

        if (buf.size > cast->buffer.size)
            break;

        switch (tag.id) {
            case EBML_TAG_EBML:
                // this tag is probably the same for all muxers. we'll simply forward it.
                if (!cast->header.size) {
                    if (ebml_buffer_dyn_concat(&cast->header, buf))
                        return -1;

                    for EACH_CALLBACK(c, cast->recvs)
                        if (!c->skip_headers)
                            c->write(c->data, buf.data, buf.size, 1);
                }
                break;

            case EBML_TAG_Segment:
                cast->time.shift = 0;
                ebml_buffer_dyn_clear(&cast->tracks);

            case EBML_TAG_Info:
                // look for the timecode scale.
                // 1000000 (1 ms) is the default value. using anything else
                // will screw with cross-stream synchronization.
                if (tag.id == EBML_TAG_Info) {
                    unsigned long long scale = 0;

                    for (struct ebml_buffer b = ebml_tag_contents(buf, tag); b.size;) {
                        struct ebml_tag lv2 = ebml_parse_tag(b);
                        if (!lv2.consumed)
                            break;

                        if (lv2.id == EBML_TAG_Duration) {
                            if (lv2.consumed + lv2.length > 0x7F)
                                return -1;

                            // live streams have none.
                            uint8_t *writable = (uint8_t *) b.data;
                            writable[0] = EBML_TAG_Void;
                            writable[1] = 0x80 | (lv2.consumed + lv2.length - 2);
                        }

                        if (lv2.id == EBML_TAG_TimecodeScale)
                            scale = ebml_parse_fixed_uint(ebml_tag_contents(b, lv2));

                        b = ebml_buffer_shift(b, lv2.consumed + lv2.length);
                    }

                    if (scale != 1000000ull)
                        return -1;
                }

            case EBML_TAG_TrackEntry:
                // at most MAX_TRACK streams (with ids 0 to MAX_TRACK-1) are allowed.
                // keyframes are detected for each stream separately.
                if (tag.id == EBML_TAG_TrackEntry) {
                    for (struct ebml_buffer ent = ebml_tag_contents(buf, tag); ent.size;) {
                        struct ebml_tag tid = ebml_parse_tag(ent);
                        if (!tid.consumed)
                            return -1;

                        if (tid.id == EBML_TAG_TrackNumber) {
                            unsigned long long t = ebml_parse_fixed_uint(
                                                   ebml_tag_contents(ent, tid));
                            if (t >= MAX_TRACK)
                                return -1;
                        }

                        ent = ebml_buffer_shift(ent, tid.consumed + tid.length);
                    }
                }

            case EBML_TAG_Tracks:
                if (ebml_buffer_dyn_concat(&cast->tracks, buf))
                    return -1;

            case EBML_TAG_SeekHead:  // disallow seeking
            case EBML_TAG_Chapters:  // disallow seeking again
            case EBML_TAG_Cues:      // disallow even more seeking
            case EBML_TAG_Void:      // waste of space
            case EBML_TAG_Tags:      // maybe later
            case EBML_TAG_Cluster:   // ignore boundaries, we'll regroup the data anyway
            case EBML_TAG_PrevSize:  // disallow backward seeking too
                break;

            case EBML_TAG_Timecode:
                // timecode = this value + two bytes in the block struct
                cast->time.recv = ebml_parse_fixed_uint(ebml_tag_contents(buf, tag));
                break;

            case EBML_TAG_BlockGroup:
            case EBML_TAG_SimpleBlock: {
                int key = 0;
                struct ebml_buffer block = ebml_tag_contents(buf, tag);

                // a SimpleBlock is simple: there a track id followed by
                // a timecode followed by flags, among which is a keyframe flag.
                // a Block (there's actually only one) in a BlockGroup is
                // almost the same, but without the keyframe flag; instead,
                // ReferenceBlock is nonzero if this is a keyframe.
                if (tag.id == EBML_TAG_BlockGroup) {
                    key   = 1;
                    block = EBML_BUFFER_EMPTY;

                    for (struct ebml_buffer grp = ebml_tag_contents(buf, tag); grp.size;) {
                        struct ebml_tag lv3 = ebml_parse_tag(grp);
                        if (lv3.consumed == 0)
                            return -1;

                        if (lv3.id == EBML_TAG_Block)
                            block = ebml_tag_contents(grp, lv3);

                        if (lv3.id == EBML_TAG_ReferenceBlock)
                            key = !ebml_parse_fixed_uint(ebml_tag_contents(grp, lv3));

                        grp = ebml_buffer_shift(grp, lv3.consumed + lv3.length);
                    }

                    if (!block.data)  // lol no blocks
                        return -1;
                }

                struct ebml_uint track = ebml_parse_uint(block);
                if (!track.consumed || track.value >= MAX_TRACK || block.size < track.consumed + 3)
                    return -1;
                key |= block.data[track.consumed + 2] & 0x80;

                unsigned long long track_mask = 1ull << track.value;
                unsigned long long blockshift = block.data[track.consumed + 0] << 8
                                              | block.data[track.consumed + 1];
                unsigned long long tc = cast->time.recv + blockshift;

                if (cast->time.shift + tc < cast->time.last)
                    cast->time.shift += cast->time.last - tc;
                cast->time.last = tc += cast->time.shift;

                tc -= blockshift;  // to avoid rewriting the block itself.
                uint8_t cluster[] = {  // manually encoded EBML
                    (uint8_t) (EBML_TAG_Cluster >> 24),
                    (uint8_t) (EBML_TAG_Cluster >> 16),
                    (uint8_t) (EBML_TAG_Cluster >>  8),
                    (uint8_t)  EBML_TAG_Cluster,  0xFF,
                    (uint8_t)  EBML_TAG_Timecode, 0x88,
                    tc >> 56, tc >> 48, tc >> 40, tc >> 32,
                    tc >> 24, tc >> 16, tc >>  8, tc,
                };

                for EACH_CALLBACK(c, cast->recvs) {
                    if (key)
                        c->keyframes |= track_mask;

                    if (c->keyframes & track_mask) {
                        if (!c->skip_headers) {
                            if (c->write(c->data, cast->tracks.data, cast->tracks.size, 1))
                                continue;
                            c->skip_headers = 1;
                            c->skip_cluster = 0;
                        }
                        if (!c->skip_cluster || tc != cast->time.sent)
                            c->skip_cluster = !c->write(c->data, cluster, sizeof(cluster), 0);

                        if (!c->skip_cluster || c->write(c->data, buf.data, buf.size, 0))
                            c->keyframes &= ~track_mask;
                    }
                }

                cast->time.sent = tc;
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
}


static int broadcast_reserve_cbs(struct broadcast *cast, size_t n)
{
    struct callback *m = (struct callback *)
            malloc(sizeof(struct callback) * (cast->recvs.size + n));

    if (m == NULL)
        return -1;

    memcpy(m, cast->recvs.xs, sizeof(struct callback) * cast->recvs.size);
    cast->recvs.reserve = n;
    cast->recvs.xs = m;
    return 0;
}


int next_callback_id = 0;
int broadcast_connect(struct broadcast *cast, on_chunk *cb, void *data, int skip_headers)
{
    if (cast->header.size && !skip_headers)
        cb(data, cast->header.data, cast->header.size, 1);

    if (!cast->recvs.reserve)
        if (broadcast_reserve_cbs(cast, 5))
            return -1;

    int id = next_callback_id++;
    cast->recvs.xs[cast->recvs.size] = (struct callback) { id, skip_headers, 0, 0, data, cb };
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

    if (cast->recvs.reserve > cast->recvs.size * 2 + 5)
        broadcast_reserve_cbs(cast, 5);
}


#undef EACH_CALLBACK
#endif
