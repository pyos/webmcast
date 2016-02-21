typedef int on_chunk(void *, const uint8_t *, size_t, int force);


struct ebml_buffer
{
    const uint8_t *data;
    size_t size;
};


struct ebml_buffer_dyn
{
    uint8_t *data;
    size_t size;
    size_t offset;
    size_t reserve;
};


struct callback;
struct broadcast
{
    struct ebml_buffer_dyn buffer;
    struct ebml_buffer_dyn header;  // [EBML .. Segment)
    struct ebml_buffer_dyn tracks;  // [Segment .. Cluster)
    struct {
        struct callback *xs;
        size_t size;
        size_t reserve;
    } recvs;
    struct {
        unsigned long long last;
        unsigned long long shift;
        unsigned long long recv;
        unsigned long long sent;
    } time;
};


int  broadcast_start      (struct broadcast *);
int  broadcast_send       (struct broadcast *, const uint8_t *, size_t);
void broadcast_stop       (struct broadcast *);
int  broadcast_connect    (struct broadcast *, on_chunk *, void *, int skip_headers);
void broadcast_disconnect (struct broadcast *, int);
