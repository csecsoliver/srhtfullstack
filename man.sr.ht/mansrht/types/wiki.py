import sqlalchemy as sa
from enum import Enum
from sqlalchemy.dialects import postgresql
from srht.database import Base

class Visibility(Enum):
    PUBLIC = 'PUBLIC'
    PRIVATE = 'PRIVATE'
    UNLISTED = 'UNLISTED'

class Wiki(Base):
    __tablename__ = 'wiki'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    name = sa.Column(sa.Unicode(256), nullable=False)
    owner_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'), nullable=False)
    owner = sa.orm.relationship('User', backref=sa.orm.backref('wikis'))
    visibility = sa.Column(
            postgresql.ENUM(Visibility, name='visibility'),
            nullable=False)
    repo_name = sa.Column(sa.Unicode, nullable=False)
    repo_ref = sa.Column(sa.Unicode, nullable=False)

class RootWiki(Base):
    __tablename__ = 'root_wiki'
    id = sa.Column(
            sa.Integer,
            sa.ForeignKey('wiki.id'),
            primary_key=True,
            nullable=False)
