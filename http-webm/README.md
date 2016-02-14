```bash
make
pip3.6 install cffi
python3.6 broadcast_ffi_gen.py
python3.6 server.py &
ffmpeg ... -c:v vp8 -keyint_min 60 -g 60 \
           -deadline realtime -speed 6 -frame-parallel 1 \
           -static-thresh 0 -max-intra-rate 300 -b:v 2000k \
           -f webm http://127.0.0.1:8000/ &
open http://127.0.0.1:8000/
```
