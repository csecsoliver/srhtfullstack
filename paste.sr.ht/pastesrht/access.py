from enum import IntFlag
from flask import abort
from hashlib import sha1
from pastesrht.types import Blob, User, Paste, PasteFile
from pastesrht.graphql import Visibility
from srht.database import db
from srht.oauth import current_user

class UserAccess(IntFlag):
    none = 0
    read = 1
    write = 2

def get_paste(user, sha):
    user = User.query.filter(User.username == user).one_or_none()
    if not user:
        paste = None
    else:
        paste = (Paste.query
                .filter(Paste.user_id == user.id)
                .filter(Paste.sha == sha)).first()
    return user, paste

def get_paste_or_abort(user, sha):
    user, paste = get_paste(user, sha)
    if not paste:
        abort(404)
    if not has_access(current_user, paste, UserAccess.read):
        abort(401)
    return user, paste

def get_access(user, paste):
    if user and user.id == paste.user_id:
        return UserAccess.read | UserAccess.write
    if paste.visibility == Visibility.PRIVATE:
        return UserAccess.none
    return UserAccess.read

def has_access(user, paste, access):
    return access in get_access(user, paste)

def get_user_or_abort(user):
    user = User.query.filter(User.username == user).one_or_none()
    if not user:
        abort(404)
    return user

def paste_add_file(paste, contents, filename):
    sha = sha1()
    sha.update(contents.encode())
    sha = sha.hexdigest()

    with DbLock(int(sha[-15:], 16)):
        blob = Blob.query.filter(Blob.sha == sha).one_or_none()
        if not blob:
            blob = Blob()
            blob.sha = sha
            blob.contents = contents
            db.session.add(blob)
            db.session.flush()

        paste_file = PasteFile()
        paste_file.paste_id = paste.id
        paste_file.blob_id = blob.id
        paste_file.filename = filename
        db.session.add(paste_file)
        db.session.commit()

    return paste_file, blob

def paste_drop(paste):
    blobs = set(file.blob for file in paste.files)
    db.session.delete(paste)
    db.session.commit()
    for blob in blobs:
        with DbLock(int(blob.sha[-15:], 16)):
            pfile = PasteFile.query.filter(PasteFile.blob_id == blob.id).first()
            if not pfile:
                db.session.delete(blob)
                db.session.commit()

class DbLock:
    def __init__(self, lock_id, transaction=True):
        self.lock_id = lock_id
        self.transaction = transaction

    def __enter__(self):
        if self.transaction:
            db.session.execute("SELECT pg_advisory_xact_lock(:lock_id)", {"lock_id": self.lock_id})
        else:
            db.session.execute("SELECT pg_advisory_lock(:lock_id)", {"lock_id": self.lock_id})
        return self

    def __exit__(self, *_):
        if not self.transaction:
            db.session.execute("SELECT pg_advisory_unlock(:lock_id)", {"lock_id": self.lock_id})
