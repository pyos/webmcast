// #include <stddef.h>
// #include <stdint.h>
// this file is passed to cffi. do not use the preprocessor here.
typedef int on_chunk(void *, const uint8_t *, size_t, int force);
struct broadcast;
struct broadcast *broadcast_start(void);
int  broadcast_send       (struct broadcast *, const uint8_t *, size_t);
void broadcast_stop       (struct broadcast *);
int  broadcast_connect    (struct broadcast *, on_chunk *, void *, int skip_headers);
void broadcast_disconnect (struct broadcast *, int);
