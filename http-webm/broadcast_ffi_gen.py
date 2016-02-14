import cffi


with open('broadcast.h') as fd:
    ffi = cffi.FFI()
    ffi.set_source('broadcast_ffi', '#include <broadcast.h>', libraries=['broadcast', 'stdc++'],
        library_dirs=['./obj'],
        include_dirs=['.'])
    ffi.cdef(fd.read() + '''
        extern "Python" int webm_on_write(void *, const uint8_t *, size_t);
    ''')
    ffi.compile()
