#ifndef EBML_BROADCAST_H
#define EBML_BROADCAST_H


struct broadcast;
struct broadcast *broadcast_start(void);
typedef int on_chunk(void *, const uint8_t *, size_t, int force);


int  broadcast_send       (struct broadcast *, const uint8_t *, size_t);
void broadcast_stop       (struct broadcast *);
int  broadcast_connect    (struct broadcast *, on_chunk *, void *, int skip_headers);
void broadcast_disconnect (struct broadcast *, int);


#endif
