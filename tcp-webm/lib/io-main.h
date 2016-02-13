#ifndef AIO_MAIN_H
#define AIO_MAIN_H

#include <errno.h>
#include <stdio.h>
#include <signal.h>
#include <thread>

#include "io.h"


#ifndef THREADS
#define THREADS 1
#endif


#ifndef PORT
#define PORT 12345
#endif


namespace aio_main
{
    static void run(int thread, aio::server *srv, aio::evloop *loop)
    {
        aio::server(loop, srv);
        fprintf(stderr, "[%d] ready\n", thread);
        if (loop->run())
        fprintf(stderr, "[%d] error: %s [%d]\n", thread, strerror(errno), errno);
        fprintf(stderr, "[%d] stopped\n", thread);
    }


    static void sigcatch(int signum)
    {
        signal(signum, SIG_DFL);
    }


    static int run_server(aio::protocol::factory pf)
    {
        aio::evloop evloops[THREADS];
        std::thread threads[THREADS];
        aio::server owner(evloops, pf, 0, PORT);
        fprintf(stderr, "[-] 127.0.0.1:%d\n", PORT);

        if (!owner.ok) {
            fprintf(stderr, "[-] fatal: %s\n", strerror(errno));
            return 1;
        }

        for (unsigned long i = 0; i < THREADS; i++)
            threads[i] = std::thread(&run, i, &owner, &evloops[i]);

        signal(SIGINT, &sigcatch);
        signal(SIGPIPE, SIG_IGN);
        pause();
        for (auto &e : evloops) e.stop();
        for (auto &t : threads) t.join();
        return 0;
    }
}

#endif
