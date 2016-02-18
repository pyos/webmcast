import ssl
import sys
import asyncio

from .server import root
from .stdhttp import serve


if len(sys.argv) == 1:
    sctx = None
elif len(sys.argv) == 3:
    sctx = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH)
    sctx.load_cert_chain(certfile=sys.argv[1], keyfile=sys.argv[2])
    sctx.set_alpn_protocols(['h2', 'http/1.1'])
    sctx.set_npn_protocols(['h2', 'http/1.1'])
else:
    exit('usage: python -m webmcast [<ssl cert> <ssl key>]')

loop = asyncio.get_event_loop()
server = loop.run_until_complete(serve(loop, root, '', 8000, ssl=sctx))
try:
    print('https://localhost:8000/' if sctx else 'http://localhost:8000/')
    loop.run_forever()
finally:
    loop.close()
