import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.database import Base
from enum import Enum
import uuid

class SecretType(Enum):
    ssh_key = "ssh_key"
    pgp_key = "pgp_key"
    plaintext_file = "plaintext_file"

    @property
    def pretty_name(self):
        if self == SecretType.ssh_key:
            return "SSH Key"
        elif self == SecretType.pgp_key:
            return "PGP Key"
        elif self == SecretType.plaintext_file:
            return "File"
        return ""

class Secret(Base):
    __tablename__ = "secret"
    id = sa.Column(sa.Integer, primary_key=True)
    user_id = sa.Column(sa.Integer, sa.ForeignKey("user.id"), nullable=False)
    user = sa.orm.relationship("User", backref="secrets", foreign_keys=[user_id])
    from_user_id = sa.Column(sa.Integer, sa.ForeignKey("user.id"))
    from_user = sa.orm.relationship("User", foreign_keys=[from_user_id])
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    uuid = sa.Column(sau.UUIDType, nullable=False)
    name = sa.Column(sa.Unicode(512))
    secret_type = sa.Column(
            sau.ChoiceType(SecretType, impl=sa.String()),
            nullable=False)
    secret = sa.Column(sa.LargeBinary(16384), nullable=False)
    path = sa.Column(sa.Unicode(512))
    mode = sa.Column(sa.Integer())

    def __init__(self, user, secret_type):
        self.uuid = uuid.uuid4()
        self.user_id = user.id
        self.secret_type = secret_type
