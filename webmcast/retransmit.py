import os
import asyncio
import weakref

import cno

from . import config, static, templates
from .ebml import ffi, lib


@ffi.def_extern(error=-1)
def on_chunk_cb(handle, data, size, force):
    queue = ffi.from_handle(handle)
    if isinstance(queue, Stream):
        # this may look like a pointless copy now, but wait until you see
        # how much more copies will be created anyway...
        return queue.send(ffi.buffer(data, size)[:])
    if not force and queue.qsize() >= config.MAX_ENQUEUED_FRAMES:
        # if the queue overflows, we're already screwed -- the tcp buffer
        # is also full. it will take a while to clear.
        return -1
    queue.put_nowait(ffi.buffer(data, size)[:])
    return 0


class Stream:
    def __init__(self, loop):
        self.loop = loop
        self.cffi = ffi.new('struct broadcast *')
        self.done = asyncio.Event(loop=loop)
        self._upd_rate(0)
        lib.broadcast_start(self.cffi)

    def _upd_rate(self, value=None):
        self.rate = 0.5 * self._rate_pending + 0.5 * self.rate if value is None else value
        self._rate_pending = 0
        self._rate_updater = self.loop.call_later(1, self._upd_rate)

    def __del__(self):
        lib.broadcast_stop(self.cffi)
        self._rate_updater.cancel()

    def send(self, chunk):
        self._rate_pending += len(chunk)
        return lib.broadcast_send(self.cffi, ffi.new('uint8_t[]', chunk), len(chunk))

    async def attach(self, queue, skip_headers=False):
        handle = ffi.new_handle(queue)
        slot = lib.broadcast_connect(self.cffi, lib.on_chunk_cb, handle, skip_headers)
        try:
            await self.done.wait()
            queue.close()
        finally:
            lib.broadcast_disconnect(self.cffi, slot)

    def close(self):
        self.done.set()

    async def close_later(self, timeout, loop=None):
        # can't just use `loop.call_later(timeout, self.close)` because that handle would
        # keep this object alive until destroyed explicitly. a finished task does not.
        await asyncio.sleep(timeout, loop=loop)
        self.close()


async def root(req, static_root = next(iter(static.__path__)),
                    streams = weakref.WeakValueDictionary(),
                    collectors = weakref.WeakKeyDictionary()):
    req.template = templates.load

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
        return await req.respond_with_file(os.path.join(static_root, req.path[8:]))

    if req.path.startswith('/stream/') and req.path.find('/', 8) == -1:
        stream_id = req.path[8:]

        if req.method in ('POST', 'PUT'):
            if stream_id in streams:
                stream = streams[stream_id]
                try:
                    collectors.pop(stream).cancel()
                except KeyError:
                    return await req.respond_with_error(403, [], 'Stream ID already taken.')
            else:
                stream = streams[stream_id] = Stream(req.conn.loop)
            try:
                while True:
                    chunk = await req.payload.read(16384)
                    if chunk == b'':
                        return await req.respond(204, [], b'')
                    if stream.send(chunk):
                        return await req.respond_with_error(400, [], 'Malformed EBML.')
            finally:
                collectors[stream] = req.conn.loop.create_task(
                    stream.close_later(config.MAX_DOWNTIME, req.conn.loop))

        try:
            stream = streams[stream_id]
        except KeyError:
            return await req.respond_with_error(404, [], None)

        if req.method not in ('GET', 'HEAD'):
            return await req.respond_with_error(405, [], 'Streams can only be GET or POSTed.')

        queue = cno.Channel(loop=req.conn.loop)
        writer = req.conn.loop.create_task(stream.attach(queue))
        try:
            return await req.respond(200, [('content-type', 'video/webm'),
                                           ('cache-control', 'no-cache')], queue)
        finally:
            writer.cancel()

    return await req.respond_with_error(404, [], None)
