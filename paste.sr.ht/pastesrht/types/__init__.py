import sqlalchemy as sa
import sqlalchemy_utils as sau
from enum import Enum
from srht.database import Base
from srht.oauth import UserMixin
from sqlalchemy.dialects import postgresql
from pastesrht.graphql import Visibility
from pastesrht.types.blob import Blob

class User(Base, UserMixin):
    pass

class Paste(Base):
    __tablename__ = 'paste'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    sha = sa.Column(sa.String(40), index=True) # null for incomplete pastes
    user_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'), nullable=False)
    user = sa.orm.relationship('User', backref=sa.orm.backref('pastes'))
    visibility = sa.Column(postgresql.ENUM(Visibility, name='visibility'), nullable=False)

class PasteFile(Base):
    __tablename__ = 'paste_file'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    filename = sa.Column(sa.Unicode(1024))

    blob_id = sa.Column(sa.Integer,
            sa.ForeignKey('blob.id'), nullable=False)
    blob = sa.orm.relationship("Blob")

    paste_id = sa.Column(sa.Integer,
            sa.ForeignKey('paste.id', ondelete="CASCADE"),
            nullable=False)
    paste = sa.orm.relationship("Paste",
            backref=sa.orm.backref("files",
                cascade="save-update, merge, delete"))
