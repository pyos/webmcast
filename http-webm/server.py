import cno
import asyncio
import itertools
import contextlib

from broadcast_ffi import ffi
from broadcast_ffi.lib import *


@ffi.def_extern('webm_on_write', -1)
def _(self, data, size):
    ffi.from_handle(self)(ffi.buffer(data, size)[:])
    return 0


class Broadcast (asyncio.Event):
    def __enter__(self):
        self.clear()
        self.obj = webm_broadcast_start()
        return self

    def __exit__(self, t, v, tb):
        webm_broadcast_stop(self.obj)
        self.set()

    def send(self, chunk):
        s = ffi.new('uint8_t[]', chunk)
        if webm_broadcast_send(self.obj, s, len(chunk)):
            raise ValueError('bad data')

    @contextlib.contextmanager
    def connect(self, cb):
        handle = ffi.new_handle(cb)
        slot = webm_slot_connect(self.obj, webm_on_write, handle)
        try:
            yield None
        finally:
            if not self.is_set():
                webm_slot_disconnect(self.obj, slot)


bmap = {}


async def handle(req, idgen=itertools.count(0)):
    if req.method == 'POST':
        sid = next(idgen)
        with Broadcast(loop=req.conn.loop) as stream:
            bmap[sid] = stream
            try:
                while True:
                    chunk = await req.payload.read(16384)
                    if chunk == b'':
                        break
                    stream.send(chunk)
            finally:
                del bmap[sid]
        return

    if req.path == '/':
        if not bmap:
            await req.respond(404, [], b'no active streams\n')
        else:
            await req.respond(200, [('content-type', 'text/html')],
                '''<!doctype html>
                    <html>
                        <head>
                            <meta charset='utf-8' />
                            <title>asd</title>
                        </head>
                        <body>
                            <video autoplay preload='none'>
                                <source src='/{}.webm' type='video/webm' />
                            </video>
                    </html>
                '''.format(max(bmap)).encode('utf-8')
            )
        return

    if req.path.endswith('.webm'):
        try:
            sid = int(req.path.lstrip('/')[:-5])
            stream = bmap[sid]
        except (ValueError, KeyError):
            await req.respond(404, [], b'invalid stream\n')
        else:
            queue = cno.Channel(loop=req.conn.loop)
            with stream.connect(queue.put_nowait):
                await req.respond(200, [('content-type', 'video/webm')], queue)
        return

    await req.respond(404, [], b'not found\n')


async def main(loop):
    http = await loop.create_server(lambda: cno.Server(loop, handle), '', 8000)
    try:
        print('http://127.0.0.1:8000/')
        await asyncio.Future(loop=loop)
    finally:
        http.close()


loop = asyncio.get_event_loop()
loop.run_until_complete(main(loop))
