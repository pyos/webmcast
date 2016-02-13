```bash
make obj/webmclone
./obj/webmclone &
ffmpeg ... -c:v vp8 -keyint_min 60 -g 60 \
           -deadline realtime -speed 6 -frame-parallel 1 \
           -static-thresh 0 -max-intra-rate 300 -b:v 2000k \
           -f webm tcp://127.0.0.1:12345 &
open http://127.0.0.1:12345/
```
