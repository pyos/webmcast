import cffi
import subprocess


with open('webmcast/ebml/ffi.h') as ebml:
    ffi = cffi.FFI()
    ffi.set_source(
        'webmcast.c', '''
            #include "ebml/ffi.h"
            #include "ebml/buffer.h"
            #include "ebml/binary.h"
            #include "ebml/rewriting.h"
            #include "ebml/broadcast.h"
        ''', include_dirs=['./webmcast']
    )
    ffi.cdef(
        ebml.read() + '''
            extern "Python" int on_chunk_cb(void *, const uint8_t *, size_t, int);
        '''
    )
    ffi.compile()
