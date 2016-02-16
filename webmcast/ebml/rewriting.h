// #include "buffer.h"
// #include "binary.h"
#ifndef EBML_REWRITING_H
#define EBML_REWRITING_H


/* create a copy of a `Cluster` with all `(Simple)Block`s before the one
 * containing the first keyframe removed, return 1 if the resulting `Cluster`
 * contains no blocks (i.e. there were no keyframes.)
 *
 * this is necessary because if a decoder happens to receive a block that references
 * a block it did not see, it will error and drop the stream, and that would be bad.
 * a keyframe, however, guarantees that no later block will reference any frame
 * before that keyframe, while also not referencing anything itself. */
static int ebml_strip_reference_frames(struct ebml_buffer buffer, struct ebml_buffer_dyn *out)
{
    struct ebml_tag cluster = ebml_parse_tag(buffer);

    if (cluster.id != EBML_TAG_Cluster || cluster.consumed + cluster.length > buffer.size)
        return -1;

    uint64_t found_keyframe = 0;  /* 1 bit per track (up to 64) */
    uint64_t seen_tracks = 0;

    if (ebml_buffer_dyn_concat(out, ebml_view(buffer.data, cluster.consumed)))
        return -1;

    for (buffer = ebml_tag_contents(buffer, cluster); buffer.size;) {
        struct ebml_tag tag = ebml_parse_tag(buffer);

        if (!tag.consumed || tag.consumed + tag.length > buffer.size)
            return -1;

        if (tag.id == EBML_TAG_SimpleBlock) {
            struct ebml_uint track = ebml_parse_uint(ebml_buffer_shift(buffer, tag.consumed), 0);

            if (!track.consumed || tag.length < track.consumed + 3 || track.value >= 64)
                return -1;

            seen_tracks |= 1ull << track.value;

            if (!(found_keyframe & (1ull << track.value))) {
                if (!(buffer.data[tag.consumed + track.consumed + 2] & 0x80))
                    goto skip_tag;

                found_keyframe |= 1 << track.value;
            }
        }

        else if (tag.id == EBML_TAG_BlockGroup) {
            /* a `BlockGroup` actually contains only a single `Block`. it does
               have some additional tags with metadata, though. we're looking
               for one either w/o a `ReferenceBlock`, or with a zeroed one. */
            struct ebml_uint track = { 0, 0 };
            uint64_t refblock = 0;

            for (struct ebml_buffer sdata = ebml_tag_contents(buffer, tag); sdata.size;) {
                struct ebml_tag tag = ebml_parse_tag(sdata);
                if (!tag.consumed)
                    return -1;

                if (tag.id == EBML_TAG_Block)
                    track = ebml_parse_uint(ebml_buffer_shift(sdata, tag.consumed), 0);

                if (tag.id == EBML_TAG_ReferenceBlock)
                    refblock = ebml_parse_fixed_uint(ebml_tag_contents(sdata, tag));

                sdata = ebml_buffer_shift(sdata, tag.consumed + tag.length);
            }

            if (!track.consumed || track.value >= 64)
                return -1;

            seen_tracks |= 1ull << track.value;

            if (refblock != 0 && !(found_keyframe & (1ull << track.value)))
                goto skip_tag;

            found_keyframe |= 1ull << track.value;
        }

        if (ebml_buffer_dyn_concat(out, ebml_view(buffer.data, tag.consumed + tag.length)))
            return -1;

        skip_tag: buffer = ebml_buffer_shift(buffer, tag.consumed + tag.length);
    }

    cluster.length = out->size - cluster.consumed;
    /* have to recode cluster's length. 4 is the length of tag's id. */
    size_t space = cluster.consumed - 4;
    ebml_write_fixed_uint_at(out->data + 4, cluster.length | 1ull << (7 * space), space);
    return found_keyframe != seen_tracks;
}


/* create a copy of a `Cluster` with its `Timecode` advanced by some value,
 * plus some more to ensure monotonicity. the resulting shift and timecode overwrite
 * the provided parameters. if nothing is written to `out`, then the original cluster's
 * timecode is good enough.
 *
 * this is needed because decoders will drop frames with timecodes less than
 * what they've already seen, even for clusters in a different segment. thus if we
 * choose to switch a client to a different segment, we need to make sure timecodes
 * do not decrease. */
static int ebml_adjust_timecode(struct ebml_buffer buffer, struct ebml_buffer_dyn *out,
                                uint64_t *shift, uint64_t *minimum)
{
    struct ebml_buffer start = buffer;
    struct ebml_tag cluster = ebml_parse_tag(buffer);

    if (cluster.id != EBML_TAG_Cluster || cluster.consumed + cluster.length > buffer.size)
        return -1;

    for (buffer = ebml_tag_contents(buffer, cluster); buffer.size;)
    {
        struct ebml_tag tag = ebml_parse_tag(buffer);

        if (!tag.consumed || tag.consumed + tag.length > buffer.size)
            return -1;

        if (tag.id == EBML_TAG_Timecode) {
            uint64_t tc = ebml_parse_fixed_uint(ebml_tag_contents(buffer, tag));

            if (*shift + tc < *minimum)
                *shift = *minimum - tc;
            *minimum = tc += *shift;

            if (*shift) {
                struct ebml_buffer head = ebml_view(start.data + cluster.consumed,
                                                    buffer.data - start.data - cluster.consumed);
                struct ebml_buffer tail = ebml_buffer_shift(buffer, tag.consumed + tag.length);
                /* there's only one timecode in a cluster. this is it. */
                cluster.length += 8 - tag.length;
                if (ebml_write_tag(out, cluster)
                ||  ebml_buffer_dyn_concat(out, head)
                ||  ebml_write_tag(out, (struct ebml_tag) { 0, 8, EBML_TAG_Timecode })
                ||  ebml_write_fixed_uint(out, tc, 8)
                ||  ebml_buffer_dyn_concat(out, tail))
                    return -1;
            }
            return 0;
        }

        buffer = ebml_buffer_shift(buffer, tag.consumed + tag.length);
    }

    return -1;  /* each cluster *must* contain a timecode */
}


#endif
