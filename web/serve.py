#!/usr/bin/env python3
"""Serves web/static for development.

Two things the stock http.server does not do, both of which cost an afternoon
once:

  - It does not send application/wasm, so instantiateStreaming refuses the
    module. The page falls back, so this only shows up as a slower load and a
    console warning, which is the kind of problem nobody looks for.

  - It lets the browser cache. Editing app.css and reloading then shows the old
    stylesheet against the new markup, which does not look like a stale cache —
    it looks like the CSS is broken. An <ol> whose list-style rule went missing
    grows numbers and stacks vertically, and the obvious reading is that the
    layout is wrong rather than absent.

Usage: python3 web/serve.py [port]
"""

import http.server
import mimetypes
import os
import socketserver
import sys

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8731
ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "static")

mimetypes.add_type("application/wasm", ".wasm")
mimetypes.add_type("text/javascript", ".js")


class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=ROOT, **kwargs)

    def end_headers(self):
        # Nothing here is worth a second look from the cache: the whole point
        # of running it is that it changes.
        self.send_header("Cache-Control", "no-store, must-revalidate")
        self.send_header("Pragma", "no-cache")
        self.send_header("Expires", "0")
        super().end_headers()

    def log_message(self, fmt, *args):
        # One line per request drowns the one line that matters, which is the
        # 404 for the file that was renamed.
        if not args or not str(args[1]).startswith("2"):
            super().log_message(fmt, *args)


socketserver.TCPServer.allow_reuse_address = True
with socketserver.TCPServer(("127.0.0.1", PORT), Handler) as server:
    print(f"serving {ROOT} on http://127.0.0.1:{PORT}/  (no caching)")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
