import cffi


with open('webmwrap.h') as fd:
    ffi = cffi.FFI()
    ffi.set_source('webmffi', '#include <webmwrap.h>', libraries=['webmwrap', 'stdc++'],
        library_dirs=['./obj'],
        include_dirs=['.'])
    ffi.cdef(fd.read() + '''
        extern "Python" int webm_callback(void *, const uint8_t *, size_t);
    ''')
    ffi.compile()
