// $CXX -std=c++11 tcpdump.cc -pthread -o tcpdump -Wall -Wextra -Werror
#include <ctype.h>
#include <stdio.h>

#include <string>

#include "io.h"
#include "io-main.h"


char to_hex(int i)
{
    return 0 <= i && i < 10 ? '0' + i : 10 <= i && i < 16 ? 'a' + i - 10 : '?';
}


struct dump_protocol : aio::protocol
{
    const  int  id;
    static int _id;

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
        std::string readable(std::min(data.size * 4, 80ul), ' ');

        for (size_t i = 0, k = 0; k < readable.size(); i++) {
            if (::isprint(data.base[i])) {
                readable[k++] = data.base[i];
            } else if (readable.size() - k < 4) {
                break;
            } else {
                readable[k++] = '\\';
                readable[k++] = 'x';
                readable[k++] = to_hex((data.base[i] >> 4) & 0xF);
                readable[k++] = to_hex(data.base[i] & 0xF);
            }
        }

        printf("<%d> <- [%zu]: %s\n", id, data.size, readable.data());
        return 0;
    }
};


int dump_protocol::_id = 0;


int main(void)
{
    return aio_main::run_server([](aio::transport *t) { return new dump_protocol(t); });
}
