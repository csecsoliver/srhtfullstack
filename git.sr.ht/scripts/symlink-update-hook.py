#!/usr/bin/env python3
from srht.config import cfg
from srht.database import DbSession
db = DbSession(cfg("git.sr.ht", "connection-string"))
from gitsrht.types import Repository
db.init()
import contextlib
import os

post_update = cfg("git.sr.ht", "post-update-script")

def migrate(path, link):
    if not os.path.exists(path) \
            or not os.path.islink(path) \
            or os.readlink(path) != link:
        with contextlib.suppress(Exception):
            os.remove(path)
        with contextlib.suppress(Exception):
            os.symlink(link, path)
        return True
    return False

for repo in Repository.query.all():
    if migrate(os.path.join(repo.path, "hooks", "update"), post_update) \
        or migrate(os.path.join(repo.path, "hooks", "post-update"), post_update) \
        or migrate(os.path.join(repo.path, "hooks", "pre-receive"), post_update):
        print("Migrated {}".format(repo.name))
