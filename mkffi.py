import cffi
import subprocess


ffi = cffi.FFI()
ffi.set_source('webm_stream.c', '#include <broadcast.c>', include_dirs=['./src'])
ffi.cdef(
    subprocess.check_output(['cpp', '-I./src', '-std=c11', '-P'], input=b'''
        #include <broadcast.h>
        extern "Python" int on_chunk_cb(void *, const uint8_t *, size_t, int);
    ''').decode('utf-8')
)
ffi.compile()
