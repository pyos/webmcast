import cffi
import subprocess


with open('./webmcast/ebml/api.h') as api:
    ffi = cffi.FFI()
    ffi.set_source('webmcast.ebml',
        '''#include "ebml/api.h"
           #include "ebml/buffer.h"
           #include "ebml/binary.h"
           #include "ebml/broadcast.h"''', include_dirs=['./webmcast'])
    ffi.cdef(api.read() + '''
        extern "Python" int on_chunk_cb(void *, const uint8_t *, size_t, int);''')
    ffi.compile()
