```bash
pip install cffi
pip install git+https://github.com/pyos/libcno
python mkffi.py
python -m webmcast.server &
ffmpeg ... -c:v vp8 -keyint_min 60 -g 60 \
           -deadline realtime -static-thresh 0 \
           -speed 6 -max-intra-rate 300 -b:v 2000k \
           -f webm http://127.0.0.1:8000/STREAM_NAME.webm &
# When streaming from a file, don't forget `-re` so that ffmpeg
# doesn't remux the video faster than it will be played back.
# To stream with audio, add `-c:a opus -b:a 64k` before `-f webm`.
open http://127.0.0.1:8000/STREAM_NAME
```
