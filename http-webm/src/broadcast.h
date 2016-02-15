typedef int webm_write_cb(void *, const uint8_t *, size_t, int force);

struct webm_broadcast_t;
struct webm_broadcast_t *
       webm_broadcast_start(void);
int    webm_broadcast_send (struct webm_broadcast_t *, const uint8_t *, size_t);
void   webm_broadcast_stop (struct webm_broadcast_t *);

int      webm_slot_connect   (struct webm_broadcast_t *, webm_write_cb *, void*, int, uint64_t);
uint64_t webm_slot_disconnect(struct webm_broadcast_t *, int);
