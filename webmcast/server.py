import time
import asyncio
import weakref

import cno

from .ebml import ffi, lib


@ffi.def_extern(error=-1)
def on_chunk_cb(handle, data, size, force):
    return ffi.from_handle(handle).send(ffi.buffer(data, size)[:], force) or 0


class Broadcast (asyncio.Event):
    def __init__(self, *a, **k):
        super().__init__(*a, **k)
        self.obj = ffi.new('struct broadcast *');
        self.chunks = 128 * [0]
        self.stops  = 128 * [time.monotonic()]
        self.stop   = 0
        self.rate   = 0
        lib.broadcast_start(self.obj)

    def __del__(self):
        lib.broadcast_stop(self.obj)

    def send(self, chunk):
        i = self.stop
        self.chunks[i] = len(chunk)
        self.stops[i] = t = time.monotonic()
        i = self.stop = (i + 1) & 127
        self.rate = sum(self.chunks) / (t - self.stops[i])
        return lib.broadcast_send(self.obj, ffi.new('uint8_t[]', chunk), len(chunk))

    async def attach(self, queue, skip_headers=False):
        handle = ffi.new_handle(queue)
        # XXX we can switch streams in the middle of the video
        #     by disconnecting the queue and reconnecting it
        #     with skip_headers=True. (that would make the server
        #     start a new webm segment) this might be useful
        #     for adaptive streaming.
        slot = lib.broadcast_connect(self.obj, lib.on_chunk_cb, handle, skip_headers)
        try:
            await self.wait()
            queue.close()
        finally:
            lib.broadcast_disconnect(self.obj, slot)

    def stop(self):
        self.set()

    async def stop_later(self, timeout, loop=None):
        # can't just `return loop.call_later(timeout, self.stop)` because that handle
        # would reference the object, preventing it from being collected.
        await asyncio.sleep(timeout, loop=loop)
        self.set()


class Client (cno.Channel):
    def __init__(self, stream, loop=None):
        super().__init__(loop=loop)
        self._stream = stream
        self._writer = asyncio.ensure_future(stream.attach(self), loop=loop)

    def cancel(self):
        self._writer.cancel()

    def send(self, chunk, force):
        self.put_nowait(chunk)
        if not force and self.qsize() > 50:
            while not self.empty():
                self.get_nowait()
            return -1


async def root(req, streams = weakref.WeakValueDictionary(),
                    collectors = weakref.WeakKeyDictionary()):
    # force excess blocks to stay in the channel.
    req.conn.transport.set_write_buffer_limits(2048, 1024)

    if req.path == '/':
        req.push('GET', '/static/css/uikit.min.css', req.accept_headers)
        req.push('GET', '/static/css/layout.css',    req.accept_headers)
        req.push('GET', '/static/js/jquery.min.js',  req.accept_headers)
        req.push('GET', '/static/js/uikit.min.js',   req.accept_headers)
        return await req.respond_with_error(501, [], 'There is no UI yet.')

    if req.path.startswith('/error/'):
        try:
            code = int(req.path[7:])
        except ValueError:
            return await req.respond_with_error(400, [], 'Error codes are numbers, silly.')
        return await req.respond_with_error(code, [], None)

    if req.path.startswith('/static/'):
        return await req.respond_with_static(req.path[8:])

    if req.path.startswith('/stream/') and req.path.find('/', 8) == -1:
        stream_id = req.path[8:]

        if req.method == 'POST' or req.method == 'PUT':
            if stream_id in streams:
                stream = streams[stream_id]
                try:
                    collectors.pop(stream).cancel()
                except KeyError:
                    return await req.respond_with_error(403, [], 'Stream ID already taken.')
            else:
                stream = streams[stream_id] = Broadcast(loop=req.conn.loop)
            try:
                while True:
                    chunk = await req.payload.read(16384)
                    if not chunk:
                        return await req.respond(204, [], b'')
                    if stream.send(chunk):
                        return await req.respond_with_error(400, [], 'Malformed EBML.')
            finally:
                collectors[stream] = asyncio.ensure_future(
                    stream.stop_later(10, req.conn.loop), loop=req.conn.loop)
        elif req.method in ('GET', 'HEAD'):
            try:
                stream = streams[stream_id]
            except KeyError:
                return await req.respond_with_error(404, [], None)

            queue = Client(stream, loop=req.conn.loop)
            try:
                return await req.respond(200, [('content-type', 'video/webm'),
                                               ('cache-control', 'no-cache')], queue)
            finally:
                queue.cancel()

        return await req.respond_with_error(405, [], 'Streams can only be GET or POSTed.')

    return await req.respond_with_error(404, [], None)
