import cffi
import subprocess


ffi = cffi.FFI()
ffi.set_source(
    'webmcast.ebml', '''
        #include "ebml/buffer.h"
        #include "ebml/binary.h"
        #include "ebml/broadcast.h"
    ''', include_dirs=['./webmcast']
)
ffi.cdef('''
    typedef int on_chunk(void *, const uint8_t *, size_t, int force);
    struct broadcast;
    struct broadcast *broadcast_start(void);
    int  broadcast_send       (struct broadcast *, const uint8_t *, size_t);
    void broadcast_stop       (struct broadcast *);
    int  broadcast_connect    (struct broadcast *, on_chunk *, void *, int skip_headers);
    void broadcast_disconnect (struct broadcast *, int);
    extern "Python" int on_chunk_cb(void *, const uint8_t *, size_t, int);
''')
ffi.compile()
