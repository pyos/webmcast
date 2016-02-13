import os
import ssl
import sys
import base64
import asyncio
import hashlib
import itertools
import mimetypes
import urllib.parse


import dg
import dogeweb.websocket
import cno


def improve_request(req):
    async def abort(code, description, headers=[]):
        try:
            await req.respond(code, headers, description.encode('utf-8'))
        except ConnectionError:
            req.cancel()  # already called `respond`
        raise asyncio.CancelledError

    async def static(path):
        path = os.path.join(os.path.dirname(__file__), 'static',
                            os.path.normpath('/' + path).lstrip('/'))
        mime = mimetypes.guess_type(path, strict=False)[0] or 'application/octet-stream'

        try:
            with open(path, 'rb', buffering=0) as fd:
                channel = cno.Channel(1, loop=req.conn.loop)

                async def writer():
                    while True:
                        data = fd.read(8196)
                        if not data:
                            break
                        await channel.put(data)
                    channel.close()

                writer = asyncio.ensure_future(writer(), loop=req.conn.loop)
                try:
                    await req.respond(200, [('content-type', mime)], channel)
                finally:
                    writer.cancel()
        except OSError:
            await abort(404, 'not found')

        raise asyncio.CancelledError

    async def websocket():
        ok = (not req.conn.is_http2
              and req.method == 'GET'
              and req.headdict.get('connection', '').lower()    == 'upgrade'
              and req.headdict.get('upgrade', '').lower()       == 'websocket'
              and req.headdict.get('sec-websocket-version', '') == '13')
        key = req.headdict.get('sec-websocket-key', '')

        try:
            ok &= len(base64.b64decode(key)) == 16
        except Exception:
            ok = False

        if not ok:
            await abort(400, 'this is a websocket node')

        response = hashlib.sha1(key.encode('ascii') + dogeweb.websocket.GUID).digest()
        await req.respond(101,
            [ ('upgrade', 'websocket')
            , ('connection', 'upgrade')
            , ('sec-websocket-accept', base64.b64encode(response).decode('ascii'))
            ], b'')

        reader = asyncio.StreamReader(loop=req.conn.loop)
        proto  = asyncio.StreamReaderProtocol(reader, loop=req.conn.loop)
        writer = asyncio.StreamWriter(req.conn.transport, proto, reader, req.conn.loop)
        req.conn.connection_lost = proto.connection_lost
        req.conn.data_received   = proto.data_received
        req.conn.eof_received    = proto.eof_received
        req.conn.pause_writing   = proto.pause_writing
        req.conn.resume_writing  = proto.resume_writing
        proto.connection_made(req.conn.transport)
        return dogeweb.websocket.WebSocket(req.conn.loop, reader, writer, False)

    path, _, query = req.path.partition('?')
    req.headdict   = dict(req.headers)
    req.path       = urllib.parse.unquote(path)
    req.query      = query
    req.abort      = abort
    req.static     = static
    req.websocket  = websocket


async def handle(req):
    try:
        improve_request(req)
        if req.path == '/':
            await req.static('index.html')
        elif req.path.startswith('/static/'):
            await req.static(req.path[8:])
        elif req.path.startswith('/stream/'):
            if len(req.path) == 8:
                await broadcast((await req.websocket()))
            else:
                await connect((await req.websocket()), req.path[8:])
        else:
            await req.abort(404, 'unknown endpoint')
    except asyncio.CancelledError:
        raise
    except Exception as err:
        sys.excepthook(err.__class__, err, err.__traceback__)


streams = {}


class Stream:
    def __init__(self, sid, owner, loop):
        self.id = sid
        self.loop = loop
        self.sync = asyncio.Future(loop=loop)
        self.owner = owner
        self.subscribers = set()

    def set_frame(self, frame):
        self.sync.set_result(frame)
        self.sync = asyncio.Future(loop=self.loop)


async def broadcast(io, id_generator=itertools.count(0)):
    sid = str(next(id_generator))
    streams[sid] = stream = Stream(sid, io, io.loop)
    io.text(sid)

    try:
        while True:
            msg = await io
            if isinstance(msg, str):
                stream.set_frame(msg)
    finally:
        for sub in stream.subscribers:
            sub.close()

        del streams[sid]


async def connect(io, sid):
    try:
        stream = streams[sid]
    except KeyError:
        await io.close()
        return

    stream.subscribers.add(io)

    async def writer():
        while True:
            await io.text((await stream.sync))
    writer = asyncio.ensure_future(writer(), loop=io.loop)

    try:
        while True:
            await io
    finally:
        writer.cancel()
        stream.subscribers.discard(io)


async def main(loop, args):
    if len(args) == 0:
        ssl_context = None
    elif len(args) == 2:
        ssl_context = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH)
        ssl_context.load_cert_chain(certfile=args[0], keyfile=args[1])
        ssl_context.set_npn_protocols  (['h2', 'http/1.1'])
        ssl_context.set_alpn_protocols (['h2', 'http/1.1'])
    else:
        exit('usage: ... [<ssl cert> <ssl key>]')

    print('https://localhost:8000/' if ssl_context else 'http://localhost:8000')
    http = await loop.create_server(lambda: cno.Server(loop, handle), '', 8000, ssl=ssl_context)
    try:
        await asyncio.Future(loop=loop)
    finally:
        http.close()


loop = asyncio.get_event_loop()
loop.run_until_complete(main(loop, sys.argv[1:]))
