// this file is passed to cffi without preprocessing.
// don't use macros/directives here.
struct WebMBroadcaster;
struct WebMBroadcaster *webm_broadcast_start(void);
int                     webm_broadcast_send (struct WebMBroadcaster *, const uint8_t *, size_t);
void                    webm_broadcast_stop (struct WebMBroadcaster *);

int  webm_slot_connect   (struct WebMBroadcaster *, int (*)(void*, const uint8_t*, size_t), void*);
void webm_slot_disconnect(struct WebMBroadcaster *, int);
