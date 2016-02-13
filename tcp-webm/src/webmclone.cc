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
    virtual void write(bool headers, const aio::stringview) = 0;
    virtual void close() = 0;
};


struct dump_protocol : aio::protocol
{
    const  int  id;
    static int _id;

    bool saw_clusters = false;
    std::string buffer;
    std::string header;
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

    int data_received(const struct aio::stringview data) override
    {
        buffer.append(data.base, data.size);

        size_t consumed = 0;

        while (1) {
            auto tag = EBML::consume_tag(buffer, consumed);
            if (!tag.first)
                break;  // wait for more data

            const char *start = buffer.data() + consumed;
            const size_t size = tag.first - consumed;

            if (tag.second == EBML::ID::Cluster) {
                saw_clusters = true;  // ignore any further metadata
                for (auto sub : subscribers)
                    sub->write(false, aio::stringview{ start, size });
            } else if (!saw_clusters) {
                header.append(start, size);
                for (auto sub : subscribers)
                    sub->write(true, aio::stringview{ start, size });
            }

            consumed = tag.first;
        }

        buffer.erase(0, consumed);
        return 0;
    }
};


int dump_protocol::_id = 0;
std::unordered_map<int, dump_protocol *> dump_protocol::sources;


struct http_protocol : webm_target, aio::protocol
{
    std::string buffer;
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
        this->write(true, source->header);
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

    void write(bool headers, const aio::stringview data) override
    {
        if (!headers && !had_ref_frame) {
            auto p = EBML::drop_non_key_frames_from_cluster(data);
            if (p.first) {
                had_ref_frame = true;
                write(false, aio::stringview(&p.second[0], p.second.size()));
            }
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
