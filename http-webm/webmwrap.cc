#include "webm.cc"

extern "C" {
    #include "webmwrap.h"
}

#include <unordered_map>


struct WebMReceiver
{
    int stream;
    WebMCallback *func;
    void         *data;

    WebMReceiver(int s, WebMCallback *f, void *d) : stream(s), func(f), data(d) {}
    WebMReceiver(const WebMReceiver &x) : stream(x.stream), func(x.func), data(x.data) {}

    const WebM::Broadcaster::callback on_chunk = [this](const uint8_t *b, size_t s)
    {
        return func(data, b, s);
    };
};


struct WebMBroadcastMap
{
    std::unordered_map<int, WebM::Broadcaster> streams;
    std::unordered_map<void *, WebM::UnerringReceiver> ureceivers;
    std::unordered_map<void *, WebMReceiver>           mreceivers;
};


struct WebMBroadcastMap * webm_broadcast_map_new(void)
{
    return new WebMBroadcastMap;
}


void webm_broadcast_map_destroy(struct WebMBroadcastMap *m)
{
    delete m;
}


int webm_broadcast_start(struct WebMBroadcastMap *m, int stream)
{
    if (m->streams.find(stream) != m->streams.end())
        return -1;

    m->streams[stream];
    return 0;
}


int webm_broadcast_send(struct WebMBroadcastMap *m, int stream, const uint8_t *b, size_t s)
{
    auto it = m->streams.find(stream);
    if (it == m->streams.end())
        return -1;

    return it->second.broadcast(b, s);
}


int webm_broadcast_stop(struct WebMBroadcastMap *m, int stream)
{
    auto it = m->streams.find(stream);
    if (it == m->streams.end())
        return -1;

    m->streams.erase(it);
    return 0;
}


int webm_broadcast_register(struct WebMBroadcastMap *m, void *data, WebMCallback *c, int stream)
{
    auto st = m->streams.find(stream);
    if (st == m->streams.end())
        return -1;

    auto mt = m->mreceivers.emplace(data, WebMReceiver{stream, c, data});
    if (!mt.second)
        return -1;

    auto ut = m->ureceivers.emplace(data, WebM::UnerringReceiver(&mt.first->second.on_chunk));
    st->second += &ut.first->second.on_chunk;
    return 0;
}


int webm_broadcast_unregister(struct WebMBroadcastMap *m, void *data)
{
    auto mt = m->mreceivers.find(data);
    if (mt == m->mreceivers.end())
        return -1;

    auto ut = m->ureceivers.find(data);
    if (ut != m->ureceivers.end()) {
        auto st = m->streams.find(mt->second.stream);
        if (st != m->streams.end())
            st->second -= &ut->second.on_chunk;

        m->ureceivers.erase(ut);
    }

    m->mreceivers.erase(mt);
    return 0;
}
