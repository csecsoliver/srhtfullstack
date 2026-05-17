#!/usr/bin/env python3
from pastesrht.app import app
from srht.config import cfg, cfgi

import os

app.static_folder = os.path.join(os.getcwd(), "static")

if __name__ == '__main__':
    app.run(host=cfg("paste.sr.ht", "debug-host"),
            port=cfgi("paste.sr.ht", "debug-port"),
            debug=True)
