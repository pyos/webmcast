// $CXX -std=c++11 tcpdump.cc -pthread -o tcpdump -Wall -Wextra -Werror
#include <ctype.h>
#include <stdio.h>

#include <string>

#include "io.h"
#include "io-main.h"
#include "ebml.h"


char to_hex(int i)
{
    return 0 <= i && i < 10 ? '0' + i : 10 <= i && i < 16 ? 'a' + i - 10 : '?';
}


struct dump_protocol : aio::protocol
{
    const  int  id;
    static int _id;
    std::string buffer;


    dump_protocol(aio::transport *t) : aio::protocol(t), id(_id++)
    {
        printf("<%d> +++\n", id);
    }

    virtual ~dump_protocol()
    {
        printf("<%d> ---\n", id);
    }

    int data_received(const struct aio::stringview data)
    {
        buffer.append(data.base, data.size);

        size_t consumed = 0;
        while (1) {
            auto tag = EBML::consume_tag(buffer, consumed);
            if (!tag.first)
                break;

            printf("<%d> %s [%zu]\n", id, EBML::ID::name(tag.second), tag.first - consumed);
            consumed = tag.first;
        }

        buffer.erase(0, consumed);
        return 0;
    }
};


int dump_protocol::_id = 0;


int main(void)
{
    return aio_main::run_server([](aio::transport *t) { return new dump_protocol(t); });
}
