import os
import sys
import zlib
import asyncio
import mimetypes
import posixpath

import cno

from urllib.parse import unquote
from . import static


async def _read_file_to_queue(fd, ch):
    try:
        while True:
            data = fd.read(8196)
            if not data:
                break
            await ch.put(data)
    finally:
        ch.close()
        fd.close()


async def _compress_into_queue(data, ch):
    co = zlib.compressobj(wbits=31)
    if isinstance(data, cno.Channel):
        async for it in data:
            ch.put_nowait(co.compress(it))
    else:
        ch.put_nowait(co.compress(data))
    ch.put_nowait(co.flush())
    ch.close()


class Request (cno.Request):
    async def respond_with_gzip(self, code, headers, data):
        for k, v in self.headers:
            if k == 'accept-encoding' and 'gzip' in v:
                break
        else:
            return await self.respond(code, headers, data)

        headers.append(('content-encoding', 'gzip'))
        ch = cno.Channel(loop=self.conn.loop)
        writer = asyncio.ensure_future(_compress_into_queue(data, ch), loop=self.conn.loop)
        try:
            return await self.respond(code, headers, ch)
        finally:
            writer.cancel()

    async def respond_with_error(self, code, headers, description):
        headers.append(('cache-control', 'no-cache'))
        try:
            await self.respond_with_gzip(code, headers, description.encode('utf-8'))
        except ConnectionError:
            self.cancel()

    async def respond_with_static(self, path, headers = [], root = next(iter(static.__path__))):
        if self.method not in ('GET', 'HEAD'):
            return await self.respond_with_error(405, [], 'this resource is static')

        # i'd serve everything as application/octet-stream, but then browsers
        # refuse to use these files as stylesheets/scripts.
        mime = mimetypes.guess_type(path, False)[0] or 'application/octet-stream'
        try:
            # `path` expected to be in normal form (no `.`/`..`)
            fd = open(os.path.join(root, path), 'rb', buffering=0)
        except IOError:
            return await self.respond_with_error(404, [], 'resource not found')

        ch = cno.Channel(1, loop=self.conn.loop)
        writer = asyncio.ensure_future(_read_file_to_queue(fd, ch), loop=self.conn.loop)
        try:
            await self.respond_with_gzip(200, [('content-type', mime)] + headers, ch)
        finally:
            writer.cancel()


async def main(loop, root, *args, **kwargs):
    async def handle(req):
        req.__class__ = Request
        req.fullpath = req.path
        req.path, _, req.query = req.path.partition('?')
        req.path = posixpath.normpath(unquote(req.path))
        try:
            await root(req)
        except asyncio.CancelledError:
            raise
        except Exception as err:
            sys.excepthook(err.__class__, err, err.__traceback__)
            await req.respond_with_error(500, [], 'exception')

    http = await loop.create_server(lambda: cno.Server(loop, handle), *args, **kwargs)
    try:
        await asyncio.Future(loop=loop)
    finally:
        http.close()
