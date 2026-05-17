import binascii
import sqlalchemy as sa
import sqlalchemy_utils as sau
from enum import Enum
from srht.database import Base, db

class PGPKey(Base):
    __tablename__ = 'pgpkey'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime)
    user_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'))
    user = sa.orm.relationship('User',
            backref=sa.orm.backref('pgp_keys'),
            foreign_keys=[user_id])
    key = sa.Column(sa.String(32768), nullable=False)
    fingerprint = sa.Column(sa.LargeBinary, nullable=False, unique=True)
    expiration = sa.Column(sa.DateTime)

    def __repr__(self):
        return '<PGPKey {} {}>'.format(self.id, self.key_id)

    @property
    def fingerprint_hex(self):
        return binascii.hexlify(self.fingerprint).decode().upper()

    def to_dict(self):
        return {
            "id": self.id,
            "key": self.key,
            "fingerprint": self.fingerprint_hex,
            "authorized": self.created,
            "owner": self.user.to_dict(short=True),
            "expiration": self.expiration,
        }
