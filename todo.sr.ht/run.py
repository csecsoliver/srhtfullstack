from todosrht.app import app
from srht.config import cfg, cfgi

import os

app.static_folder = os.path.join(os.getcwd(), "static")

if __name__ == '__main__':
    app.run(host=cfg("todo.sr.ht", "debug-host"),
            port=cfgi("todo.sr.ht", "debug-port"),
            debug=True)
