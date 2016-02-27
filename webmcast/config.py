#: How long, in seconds, to keep the stream alive when
#: there's nothing feeding data into it.
MAX_DOWNTIME = 10

#: How may frames can the output queue hold at once.
#: If this limit is reached, everything until the next keyframe
#: gets dropped.
MAX_ENQUEUED_FRAMES = 20
