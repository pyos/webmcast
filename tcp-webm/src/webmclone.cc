// $CXX -std=c++11 tcpdump.cc -pthread -o tcpdump -Wall -Wextra -Werror
#include <ctype.h>
#include <stdio.h>

#include <string>
#include <unordered_map>
#include <unordered_set>

#include "io.h"
#include "io-main.h"
#include "ebml.h"
#include "picohttpparser.h"


struct webm_target
{
    virtual void write(aio::stringview) = 0;
    virtual void close() = 0;
};


struct dump_protocol : aio::protocol
{
    const  int  id;
    static int _id;

    std::unordered_set<webm_target *> subscribers;
    static std::unordered_map<int, dump_protocol *> sources;

    dump_protocol(aio::transport *t) : aio::protocol(t), id(_id++)
    {
        printf("<%d> +++\n", id);
        dump_protocol::sources[id] = this;
    }

    virtual ~dump_protocol()
    {
        dump_protocol::sources.erase(id);

        auto subs_copy = subscribers;
        for (auto sub : subs_copy)
            sub->close();

        printf("<%d> ---\n", id);
    }

    bool saw_clusters = false;
    std::string buffer;
    std::string header;

    int data_received(struct aio::stringview data) override
    {
        buffer.append(data.base, data.size);
        data = aio::stringview(buffer);

        while (1) {
            auto tag = EBML::parse_tag(data);
            if (!tag.consumed)
                break;  // wait for more data

            auto fwd_length = tag.consumed + tag.value.length;

            if (tag.value.id == EBML::ID::Segment) {
                if (tag.value.is_long_coded_endless()) {
                    uint8_t * writable = (uint8_t *) data.base;
                    // tag id = 4 bytes, the rest is its length.
                    writable[4] = 0xFF;
                    // fill the rest of the space with a Void tag to avoid
                    // decoding errors.
                    writable[5] = 0x80 | (unsigned) EBML::ID::Void;
                    writable[6] = 0x80 | (tag.consumed - 7);
                }
                // forward only the headers of this tag.
                // we'll deal with the contents later.
                fwd_length = tag.consumed;
            } else if (tag.value.is_endless()) {
                // can't forward blocks on infinite size.
                this->transport->close();
                return -1;
            }

            if (fwd_length > data.size)
                break;

            if (tag.value.id == EBML::ID::Cluster)
                saw_clusters = true;  // ignore any further metadata
            else if (!saw_clusters)
                header.append(data.base, fwd_length);
            else
                goto skip_tag;

            for (auto sub : subscribers)
                sub->write(aio::stringview{ data.base, fwd_length });

            skip_tag: data = data + fwd_length;
        }

        buffer.erase(0, data.base - buffer.data());
        return 0;
    }
};


int dump_protocol::_id = 0;
std::unordered_map<int, dump_protocol *> dump_protocol::sources;


struct http_protocol : webm_target, aio::protocol
{
    dump_protocol *source = NULL;

    http_protocol(aio::transport *t) : aio::protocol(t)
    {
        printf("<?> +++ http client\n");
    }

    virtual ~http_protocol()
    {
        printf("<?> --- http client\n");
        if (source)
            source->subscribers.erase(this);
    }

    std::string buffer;

    int data_received(const aio::stringview data) override
    {
        buffer.append(data.base, data.size);

        int minor;
        aio::stringview method;
        aio::stringview path;
        struct phr_header headers[64];
        size_t headers_len = 64;

        int ok = phr_parse_request(buffer.data(), buffer.size(),
            &method.base, &method.size, &path.base, &path.size,
            &minor, headers, &headers_len, 1);

        if (ok == -2) {
            if (buffer.size() > 65535) {
                printf("<?> http request too big\n");
                this->transport->close();
                return -1;
            }

            return 0;
        }

        if (ok == -1) {
            printf("<?> not actually http\n");
            this->transport->close();
            return -1;
        }

        if (!(method == "GET" && path.size && path.base[0] == '/'))
            return this->abort(400, "GET /stream_id");

        if (path == "/") {
            if (dump_protocol::sources.empty())
                return this->abort(404, "no active streams");

            int stream = -1;
            for (auto &s : dump_protocol::sources)
                if (stream < s.first)
                    stream = s.first;

            char buffer[4096];
            snprintf(buffer, sizeof(buffer),
                "HTTP/1.1 200 OK\r\n"
                "Connection: close\r\n"
                "Content-Type: text/html\r\n"
                "\r\n"
                "<!doctype html>"
                "<html>"
                  "<head>"
                    "<meta charset='utf-8' />"
                    "<title>asd</title>"
                  "</head>"
                  "<body>"
                    "<video controls autoplay preload='none'>"
                      "<source src='/%d' type='video/webm' />"
                    "</video>"
                  "</body>"
                "</html>", stream);
            this->transport->write(buffer);
            this->transport->close_on_drain();
            return 0;
        }

        char *p;
        int x = strtol(path.base + 1, &p, 10);

        if (x < 0 || p != path.base + path.size)
            return this->abort(404, "non-int stream id");

        auto it = dump_protocol::sources.find(x);
        if (it == dump_protocol::sources.end())
            return this->abort(404, "invalid stream");

        source = it->second;
        source->subscribers.insert(this);
        this->transport->write(
            "HTTP/1.1 200 OK\r\n"
            "Connection: close\r\n"
            "Transfer-Encoding: chunked\r\n"
            "Content-Type: video/webm\r\n"
            "\r\n");
        this->write(source->header);
        return 0;
    }

    int abort(int code, std::string error)
    {
        std::string buffer(error.length() + 1024, ' ');

        snprintf(&buffer[0], buffer.size(),
            "HTTP/1.1 %d Something\r\n"
            "Connection: close\r\n"
            "Content-Length: %zu\r\n"
            "Content-Type: text-plain\r\n"
            "\r\n"
            "%s", code, error.size(), error.data());

        this->transport->write(buffer);
        this->transport->close_on_drain();
        return -1;
    }

    bool had_ref_frame = false;

    void write(aio::stringview data) override
    {
        auto cluster = EBML::parse_tag(data);

        if (!had_ref_frame && cluster.value.id == EBML::ID::Cluster) {
            // ok so this is the first cluster we forward. if it references
            // older blocks/clusters (which this client doesn't have), the decoder
            // will error and drop the stream. so we need to drop frames
            // until the next keyframe. and boy is that hard.
            std::vector<char> copy(data.base, data.base + data.size);
            // a cluster can actually contain many blocks. we can send
            // the first keyframe-only block and all that follow
            auto *target = &copy[cluster.consumed];

            for (data = data + cluster.consumed; data.size; ) {
                auto tag = EBML::parse_tag(data);
                if (!tag.consumed)
                    return;

                if (tag.value.id == EBML::ID::PrevSize)
                    // there is no previous cluster, so this data is not applicable.
                    goto skip_tag;

                else if (tag.value.id == EBML::ID::SimpleBlock && !had_ref_frame) {
                    if (tag.value.length < 4)
                        return;

                    // the very first field has a variable length. what a bummer.
                    // it doesn't even follow the same format as tag ids.
                 // auto field = ((uint8_t *) data.base)[tag.consumed];
                 // auto skip_field = EBML::parse_uint_size(~field) + 1;
                    auto skip_field = 1u;

                    if (tag.value.length < 3u + skip_field)
                        return;

                    if (!(((uint8_t *) data.base)[tag.consumed + skip_field + 2] & 0x80))
                        goto skip_tag;  // nope, not a keyframe.

                    had_ref_frame = true;
                }

                else if (tag.value.id == EBML::ID::BlockGroup && !had_ref_frame) {
                    // a BlockGroup actually contains only a single Block.
                    // it does have some additional tags with metadata, though.
                    // if there's a nonzero ReferenceBlock, this is def not a keyframe.
                    auto sdata = aio::stringview{ data.base + tag.consumed, tag.value.length };

                    while (sdata.size) {
                        auto tag = EBML::parse_tag(sdata);
                        if (!tag.consumed)
                            return;

                        if (tag.value.id == EBML::ID::ReferenceBlock)
                            for (size_t i = 0; i < tag.value.length; i++)
                                if (sdata.base[tag.consumed + i] != 0)
                                    goto skip_tag;

                        sdata = sdata + tag.consumed + tag.value.length;
                    }

                    had_ref_frame = true;
                }

                memcpy(target, data.base, tag.consumed + tag.value.length);
                target += tag.consumed + tag.value.length;

                skip_tag: data = data + tag.consumed + tag.value.length;
            }

            if (!had_ref_frame)
                return;

            write(aio::stringview(&copy[0], copy.size()));
            return;
        }

        char length_buffer[16];
        snprintf(length_buffer, sizeof(length_buffer), "%zX\r\n", data.size);
        this->transport->write(length_buffer);
        this->transport->write(data);
        this->transport->write("\r\n");
    }

    void close() override
    {
        this->transport->write("0\r\n\r\n");
        this->transport->close_on_drain();
        source->subscribers.erase(this);
        source = NULL;
    }
};


struct auto_protocol : aio::protocol
{
    aio::protocol *target = NULL;

    auto_protocol(aio::transport *t) : aio::protocol(t)
    {
        // ...
    }

    virtual ~auto_protocol()
    {
        delete target;
    }

    int data_received(const aio::stringview data) override
    {
        if (target)
            return target->data_received(data);

        if (data.base[0] == 0x1A)
            // clearly not an http client. probably EBML, though.
            target = new dump_protocol(this->transport);
        else
            target = new http_protocol(this->transport);

        return data_received(data);
    }
};


int main(void)
{
    return aio_main::run_server([](aio::transport *t) { return new auto_protocol(t); });
}
