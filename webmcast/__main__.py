import ssl
import sys
import asyncio

from .server import root
from .stdhttp import main


if len(sys.argv) == 1:
    sctx = None
    print('http://127.0.0.1:8000/')
elif len(sys.argv) == 3:
    sctx = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH)
    sctx.load_cert_chain(certfile=sys.argv[1], keyfile=sys.argv[2])
    sctx.set_alpn_protocols(['h2', 'http/1.1'])
    sctx.set_npn_protocols(['h2', 'http/1.1'])
    print('https://127.0.0.1:8000/')
else:
    exit('usage: python -m webmcast [<ssl cert> <ssl key>]')

loop = asyncio.get_event_loop()
loop.run_until_complete(main(loop, root, '', 8000, ssl=sctx))
