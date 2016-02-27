import ssl
import sys
import asyncio

from . import retransmit, framework


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
server = loop.run_until_complete(framework.http.server(loop, retransmit.root, '', 8000, ssl=sctx))
try:
    loop.run_forever()
except KeyboardInterrupt:
    pass
finally:
    loop.close()
