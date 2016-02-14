// this file is passed to cffi without preprocessing.
// don't use macros/directives here.
typedef int WebMCallback(void *, const uint8_t *, size_t);
struct WebMBroadcastMap;
struct WebMBroadcastMap * webm_broadcast_map_new(void);
void   webm_broadcast_map_destroy (struct WebMBroadcastMap *);
int    webm_broadcast_start       (struct WebMBroadcastMap *, int stream);
int    webm_broadcast_send        (struct WebMBroadcastMap *, int stream, const uint8_t *, size_t);
int    webm_broadcast_stop        (struct WebMBroadcastMap *, int stream);
int    webm_broadcast_register    (struct WebMBroadcastMap *, void *, WebMCallback *, int stream);
int    webm_broadcast_unregister  (struct WebMBroadcastMap *, void *);
