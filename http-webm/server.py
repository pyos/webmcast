import cno
import asyncio
import itertools

from webmffi import ffi
from webmffi.lib import *


@ffi.def_extern('webm_callback', -1)
def _webm_callback(self, data, size):
    ffi.from_handle(self).cb(ffi.buffer(data, size)[:])
    return 0


class BroadcastMap:
    def __init__(self):
        self._c = webm_broadcast_map_new()
        self.active = set()

    def __del__(self):
        webm_broadcast_map_destroy(self._c)

    def start(self, i):
        if webm_broadcast_start(self._c, i):
            raise ValueError('this stream is already running')
        self.active.add(i)

    def send(self, i, chunk):
        s = ffi.new('uint8_t[]', chunk)
        if webm_broadcast_send(self._c, i, s, len(chunk)):
            raise ValueError('invalid stream/bad data')

    def stop(self, i):
        if webm_broadcast_stop(self._c, i):
            raise ValueError('this stream does not exist')
        self.active.discard(i)


class Receiver:
    def __init__(self, m, stream, cb):
        self._c = ffi.new_handle(self)
        self._m = m
        self.cb = cb
        if webm_broadcast_register(self._m._c, self._c, webm_callback, stream):
            raise ValueError('stream does not exist')

    def __del__(self):
        if webm_broadcast_unregister(self._m._c, self._c):
            pass


bmap = BroadcastMap()


async def handle(req, idgen=itertools.count(0)):
    if req.method == 'POST':
        sid = next(idgen)
        try:
            print('+++', sid)
            bmap.start(sid)
            while True:
                chunk = await req.payload.read(16384)
                if chunk == b'':
                    break
                bmap.send(sid, chunk)
        finally:
            print('---', sid)
            bmap.stop(sid)
        return

    if req.path == '/':
        if not bmap.active:
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
                '''.format(max(bmap.active)).encode('utf-8')
            )
        return

    if req.path.endswith('.webm'):
        queue = cno.Channel(loop=req.conn.loop)
        try:
            sid = int(req.path.lstrip('/')[:-5])
            rcv = Receiver(bmap, sid, queue.put_nowait)
        except ValueError:
            pass
        else:
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
