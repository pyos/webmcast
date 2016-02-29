#: How long, in seconds, to keep the stream alive when
#: there's nothing feeding data into it.
MAX_DOWNTIME = 10  # seconds

#: How may frames can the output queue hold at once.
#: If this limit is reached, everything until the next keyframe
#: gets dropped.
MAX_ENQUEUED_FRAMES = 20  # frames

#: Maximum bitrate to accept. Going over this limit...does nothing. Yet.
#: Maybe it'll result in automatic termination. Later. TODO, I guess.
MAX_BITRATE = 3500  # kbps (kilobits/second)

#: Bitrates to offer for adaptive streaming.
#: Naturally, only ones equal to or lower than the original stream's bitrate
#: will be available.
ADAPTIVE_RATES = 600, 1200, 2400, 3300  # kbps
