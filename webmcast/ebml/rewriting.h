// #include "buffer.h"
// #include "binary.h"
#ifndef EBML_REWRITING_H
#define EBML_REWRITING_H


static unsigned long long ebml_get_timescale(struct ebml_buffer buffer)
{
    struct ebml_tag lv1 = ebml_parse_tag(buffer);

    if (lv1.id == EBML_TAG_Info) for (buffer = ebml_tag_contents(buffer, lv1); buffer.size;) {
        struct ebml_tag lv2 = ebml_parse_tag(buffer);
        if (!lv2.consumed)
            return 0;

        if (lv2.id == EBML_TAG_TimecodeScale)
            return ebml_parse_fixed_uint(ebml_tag_contents(buffer, lv2));

        buffer = ebml_buffer_shift(buffer, lv2.consumed + lv2.length);
    }

    return 0;
}


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
    struct ebml_tag lv1 = ebml_parse_tag(buffer);
    if (lv1.id != EBML_TAG_Cluster)
        return -1;

    unsigned long long found_keyframe = 0;  // 1 bit per track (up to 64)
    unsigned long long seen_tracks = 0;

    if (ebml_buffer_dyn_concat(out, ebml_view(buffer.data, lv1.consumed)))
        return -1;

    for (buffer = ebml_tag_contents(buffer, lv1); buffer.size;) {
        struct ebml_tag lv2 = ebml_parse_tag(buffer);
        if (!lv2.consumed)
            return -1;

        switch (lv2.id) {
            case EBML_TAG_Timecode:
            copy_tag:
                if (ebml_buffer_dyn_concat(out, ebml_view(buffer.data, lv2.consumed + lv2.length)))
                    return -1;

            case EBML_TAG_PrevSize:
                break;

            case EBML_TAG_SimpleBlock: {
                struct ebml_uint track = ebml_parse_uint(ebml_tag_contents(buffer, lv2), 0);
                if (!track.consumed || track.value >= 64 || lv2.length < track.consumed + 3)
                    return -1;

                seen_tracks |= 1ull << track.value;
                if (found_keyframe & (1ull << track.value))
                    goto copy_tag;
                if (!(buffer.data[lv2.consumed + track.consumed + 2] & 0x80))
                    break;
                found_keyframe |= 1 << track.value;
                goto copy_tag;
            }

            case EBML_TAG_BlockGroup: {
                // there's actually only one Block in a BlockGroup
                struct ebml_uint track = { 0, 0 };
                unsigned long long refblock = 0;

                for (struct ebml_buffer grp = ebml_tag_contents(buffer, lv2); grp.size;) {
                    struct ebml_tag lv3 = ebml_parse_tag(grp);
                    if (!lv3.consumed)
                        return -1;

                    switch (lv3.id) {
                        case EBML_TAG_Block:
                            track = ebml_parse_uint(ebml_tag_contents(grp, lv3), 0);
                            break;

                        case EBML_TAG_ReferenceBlock:
                            refblock = ebml_parse_fixed_uint(ebml_tag_contents(grp, lv3));
                            break;
                    }

                    grp = ebml_buffer_shift(grp, lv3.consumed + lv3.length);
                }

                if (!track.consumed || track.value >= 64)
                    return -1;

                seen_tracks |= 1ull << track.value;
                if (refblock && !(found_keyframe & (1ull << track.value)))
                    break;
                found_keyframe |= 1ull << track.value;
                goto copy_tag;
            }

            default: return -1;
        }

        buffer = ebml_buffer_shift(buffer, lv2.consumed + lv2.length);
    }

    lv1.length = out->size - lv1.consumed;
    // have to recode Cluster's length. 4 is the length of tag's id.
    size_t space = lv1.consumed - 4;
    ebml_write_fixed_uint_at(out->data + 4, lv1.length | 1ull << (7 * space), space);
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
                                unsigned long long *shift, unsigned long long *minimum)
{
    struct ebml_tag lv1 = ebml_parse_tag(buffer);
    if (lv1.id != EBML_TAG_Cluster)
        return -1;
    buffer = ebml_tag_contents(buffer, lv1);
    // https://matroska.org/technical/order/index.html
    // >the Cluster Timecode must be the first element in the Cluster
    struct ebml_tag lv2 = ebml_parse_tag(buffer);
    if (lv2.id != EBML_TAG_Timecode)
        return -1;

    unsigned long long tc = ebml_parse_fixed_uint(ebml_tag_contents(buffer, lv2));
    if (*shift + tc < *minimum)
        *shift = *minimum - tc;
    *minimum = tc += *shift;

    if (!*shift)
        return 0;

    lv1.length += 8 + 1 + 1 - lv2.length - lv2.consumed;
    return ebml_write_tag(out, lv1) ? -1
         : ebml_write_tag(out, (struct ebml_tag) { 0, EBML_TAG_Timecode, 8 }) ? -1
         : ebml_write_fixed_uint(out, tc, 8) ? -1
         : ebml_buffer_dyn_concat(out, ebml_buffer_shift(buffer, lv2.consumed + lv2.length)) ? -1
         : 0;
}


#endif
