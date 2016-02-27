import asyncio
import base64
import random
import struct
import hashlib
import operator
import itertools

OP_CONTINUATION = 0x0
OP_TEXT         = 0x1
OP_DATA         = 0x2
OP_CONTROL      = 0x8  # > all non-control codes
OP_CLOSE        = 0x8
OP_PING         = 0x9
OP_PONG         = 0xA
OP_VALID = {OP_CONTINUATION, OP_TEXT, OP_DATA, OP_CLOSE, OP_PING, OP_PONG}

CLOSE_CODE_PRIVATE = 3000
CLOSE_CODE_UNDEF   = 5000
CLOSE_CODES = {
    0:    '',
    1000: 'OK',
    1001: 'going away',
    1002: 'protocol error',
    1003: 'unsupported',
    1007: 'invalid data',
    1008: 'policy violation',
    1009: 'data frame too large',
    1010: 'missing extension',
    1011: 'internal error',
    1012: 'service restart',
    1013: 'try again later',
}


class ProtocolError (asyncio.CancelledError):
    # these exceptions are subclasses of CancelledError because
    # they're mostly noise. non-compliant clients are not our problem.
    pass


class FrameSizeError (asyncio.CancelledError):
    pass


class UTFError (asyncio.CancelledError):
    pass


class ConnectionClosedError (asyncio.CancelledError):
    pass


def accept(key):
    return base64.b64encode(hashlib.sha1(key.encode('ascii')
        # it's a standard-mandated magic constant. don't worry about it.
        + b'258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest()).decode('ascii')


async def read_frame(reader, max_size):
    a, b = await reader.readexactly(2)
    fin  = a & 0x80
    code = a & 0x0F
    if a & 0x70 or code not in OP_VALID:
        raise ProtocolError('unknown extension code')
    if code >= OP_CONTROL and (not fin or b & 0x7E == 0x7E):
        raise ProtocolError('multipart/oversized control frame')
    if code == OP_CLOSE and b & 0x7F == 1:
        raise ProtocolError('truncated CLOSE frame')

    size = int.from_bytes(await reader.readexactly(2), 'big') if b & 0x7F == 0x7E \
      else int.from_bytes(await reader.readexactly(8), 'big') if b & 0x7F == 0x7F \
      else b & 0x7F
    if size > max_size:
        raise FrameSizeError('frame too big')

    mask = await reader.readexactly(4) if b & 0x80 else None
    data = await reader.readexactly(size)
    if mask:
        data = bytes(map(operator.xor, data, itertools.cycle(mask)))
    return fin, code, data


async def read_message(reader, continued, max_size):
    fin, code, *chunks = continued or await read_frame(reader, max_size)
    if code == OP_CONTINUATION:
        raise ProtocolError('unexpected CONTINUATION')
    while not fin:
        max_size -= len(chunks[-1])
        fin, ctrl, part = await read_frame(reader, max_size)
        if ctrl >= OP_CONTROL:
            return ctrl, part, (False, code, b''.join(chunks))
        if ctrl != OP_CONTINUATION:
            raise ProtocolError('interrupted data frame')
        chunks.append(part)
    return code, b''.join(chunks), None


def make_frame(code, data, fin=True, mask=False):
    a = 0x80 | code if fin else code
    b = 0x80 if mask else 0x00
    head = struct.pack('!BBQ', a, b | 0x7F, len(data)) if len(data) > 0xFFFF \
      else struct.pack('!BBH', a, b | 0x7E, len(data)) if len(data) > 0x7D \
      else struct.pack('!BB',  a, b | len(data))
    if mask:
        mask = random.getrandbits(32).to_bytes(4, 'big')
        data = mask + bytes(map(operator.xor, data, itertools.cycle(mask)))
    return head + data


class Socket:
    def __init__(self, loop, reader, transport, max_message_size=32 * 1024 * 1024):
        self.loop = loop
        self.reader = reader
        self.transport = transport
        self.max_message_size = max_message_size
        self._closed = False

    def __enter__(self):
        return self

    def __exit__(self, t, v, tb):
        if not self._closed and not self.transport.is_closing():
            self.close(0    if t is ConnectionClosedError
                  else 1000 if t is asyncio.CancelledError
                  else 1001 if t is None
                  else 1002 if t is ProtocolError
                  else 1007 if t is UTFError
                  else 1011)
        self.transport.close()

    async def __aiter__(self):
        return self

    async def __anext__(self):
        try:
            contd = None
            while True:
                code, data, contd = await read_message(self.reader, contd, self.max_message_size)
                if code == OP_PING:
                    self.transport.write(make_frame(OP_PONG, data))
                elif code == OP_CLOSE:
                    code = int.from_bytes(data[:2], 'big')
                    if code < CLOSE_CODE_PRIVATE and code not in CLOSE_CODES:
                        raise ProtocolError('invalid close code')
                    raise ConnectionClosedError(code, data[2:].decode('utf-8'))
                elif code == OP_TEXT:
                    return data.decode('utf-8')
                elif code == OP_DATA:
                    return data
        except UnicodeDecodeError as err:
            raise UTFError(err)
        except asyncio.IncompleteReadError:
            raise ProtocolError('truncated message')

    def send(self, xs):
        assert not self._closed
        if isinstance(xs, str):
            self.transport.write(make_frame(OP_TEXT, xs.encode('utf-8')))
        else:
            self.transport.write(make_frame(OP_DATA, xs))

    def close(self, code=1000, data=b''):
        assert not self._closed
        if code == 0 and not data:
            self.transport.write(make_frame(OP_CLOSE, b''))
        else:
            self.transport.write(make_frame(OP_CLOSE, (code or 1000).to_bytes(2, 'big') + data))
        self._closed = True
