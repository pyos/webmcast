typedef int webm_write_cb(void *, const uint8_t *, size_t);

struct WebMBroadcaster;
struct WebMBroadcaster *webm_broadcast_start(void);
int                     webm_broadcast_send (struct WebMBroadcaster *, const uint8_t *, size_t);
void                    webm_broadcast_stop (struct WebMBroadcaster *);

int  webm_slot_connect   (struct WebMBroadcaster *, webm_write_cb *, void*);
void webm_slot_disconnect(struct WebMBroadcaster *, int);
