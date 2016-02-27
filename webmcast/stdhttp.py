import os
import time
import zlib
import asyncio
import functools
import mimetypes
import posixpath

from urllib.parse import unquote

import cno

from . import static, templates, stdws


async def _read_file_to_queue(fd, queue):
    try:
        while True:
            data = fd.read(8192)
            if not data:
                break
            await queue.put(data)
    finally:
        queue.close()


async def _compress_into_queue(data, queue):
    try:
        gz = zlib.compressobj(wbits=31)
        if isinstance(data, cno.Channel):
            async for chunk in data:
                await queue.put(gz.compress(chunk))
        else:
            await queue.put(gz.compress(data))
        await queue.put(gz.flush())
    finally:
        queue.close()


def _rfc1123(ts):
    ts = time.gmtime(ts)
    return '%s, %02d %s %04d %02d:%02d:%02d GMT' % (
        ('Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun')[ts.tm_wday], ts.tm_mday,
        ('---', 'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
         'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec')[ts.tm_mon],
        ts.tm_year, ts.tm_hour, ts.tm_min, ts.tm_sec
    )


class Request (cno.Request):
    def render(self, _name, **kwargs):
        return templates.load(_name).render(request=self, **kwargs)

    async def respond_with_gzip(self, code, headers, data):
        for k, v in self.accept_headers:
            if k == 'accept-encoding' and 'gzip' in v:
                break
        else:
            return await self.respond(code, headers, data)

        headers = headers + [('content-encoding', 'gzip')]
        ch = cno.Channel(loop=self.conn.loop)
        writer = asyncio.ensure_future(_compress_into_queue(data, ch), loop=self.conn.loop)
        try:
            return await self.respond(code, headers, ch)
        finally:
            writer.cancel()

    async def respond_with_template(self, code, headers, name, **kwargs):
        await self.respond_with_gzip(code, headers, self.render(name, **kwargs).encode('utf-8'))

    async def respond_with_error(self, code, headers, message, **kwargs):
        headers = headers + [('cache-control', 'no-cache')]
        try:
            payload = self.render('error', code=code, message=message, **kwargs)
        except Exception as e:
            self.conn.loop.call_exception_handler({'message': 'error while rendering error page',
                                                   'exception': e, 'protocol': self.conn})
            payload = 'error {}: {}'.format(code, message)
        try:
            await self.respond_with_gzip(code, headers, payload.encode('utf-8'))
        except ConnectionError:
            self.cancel()

    async def respond_with_static(self, path, headers = [], cacheable = True,
                                  root = next(iter(static.__path__))):
        if self.method not in ('GET', 'HEAD'):
            return await self.respond_with_error(405, [], 'This resource is static.')
        # i'd serve everything as application/octet-stream, but then browsers
        # refuse to use these files as stylesheets/scripts.
        mime = mimetypes.guess_type(path, False)[0] or 'application/octet-stream'
        try:
            # `path` expected to be in normal form (no `.`/`..`)
            fd = open(os.path.join(root, path), 'rb', buffering=8192)
            headers = headers + (
                [('last-modified', _rfc1123(os.stat(fd.fileno()).st_mtime)),
                 ('cache-control', 'private, max-age=31536000'),
                 ('content-type', mime)] if cacheable else [('content-type', mime)])
        except IOError:
            return await self.respond_with_error(404, [], None)
        ch = cno.Channel(1, loop=self.conn.loop)
        writer = asyncio.ensure_future(_read_file_to_queue(fd, ch), loop=self.conn.loop)
        try:
            await self.respond_with_gzip(200, headers, ch)
        finally:
            writer.cancel()
            fd.close()

    async def websocket(self):
        if (self.conn.is_http2
         or self.method != 'GET'
         or self.header_map.get('upgrade').lower() != 'websocket'
         or self.header_map.get('sec-websocket-version') != '13'
         or 'sec-websocket-key' not in self.header_map):
            await self.respond_with_error(400, [], 'WebSocket handshake failed.')
            raise stdws.ProtocolError('invalid handshake')
        await self.respond(101,
            [('upgrade', 'websocket'),
             ('connection', 'upgrade'),
             ('sec-websocket-accept', stdws.accept(self.header_map['sec-websocket-key']))], b'')
        reader = asyncio.StreamReader(loop=self.conn.loop)
        self.conn.data_received = reader.feed_data
        self.conn.eof_received  = reader.feed_eof
        return stdws.Socket(self.conn.loop, reader, self.conn.transport)


async def serve(loop, root, *args, **kwargs):
    async def handle(req):
        req.__class__ = Request
        req.path, _, req.query = req.path.partition('?')
        req.path = posixpath.normpath('///' + unquote(req.path))
        req.header_map = dict(req.headers)
        req.accept_headers = [(k, v) for k, v in req.headers if k.startswith('accept')]
        try:
            await root(req)
        except asyncio.CancelledError:
            raise
        except Exception as err:
            loop.call_exception_handler({'message': 'error in request handler',
                                         'exception': err, 'protocol': req.conn})
            await req.respond_with_error(500, [], 'This is an error-handling message.')
    return await loop.create_server(lambda: cno.Server(loop, handle), *args, **kwargs)
