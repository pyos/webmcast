```bash
pip install cffi
pip install git+https://github.com/pyos/libcno
make
python -m webm_stream.server &
ffmpeg ... -c:v vp8 -keyint_min 60 -g 60 \
           -deadline realtime -static-thresh 0 \
           -speed 6 -max-intra-rate 300 -b:v 2000k \
           -f webm http://127.0.0.1:8000/STREAM_NAME.webm &
# to stream with audio, add `-c:a opus -b:a 64k` before -f webm
open http://127.0.0.1:8000/STREAM_NAME
```
