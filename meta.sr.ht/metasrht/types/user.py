import sqlalchemy as sa
import sqlalchemy_utils as sau
from metasrht.graphql import PaymentStatus
from srht.database import Base
from srht.oauth import UserMixin, UserType

class UserNote(Base):
    __tablename__ = 'user_notes'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    user_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'), nullable=False)
    user = sa.orm.relationship('User', backref=sa.orm.backref('notes'))
    note = sa.Column(sa.Unicode())

class User(Base, UserMixin):
    password = sa.Column(sa.String(256), nullable=False)
    avatar = sa.Column(sa.String)
    pronouns = sa.Column(sa.String)

    pgp_key_id = sa.Column(sa.Integer, sa.ForeignKey('pgpkey.id'))
    pgp_key = sa.orm.relationship('PGPKey', foreign_keys=[pgp_key_id])

    payment_status = sa.Column(
            sau.ChoiceType(PaymentStatus, impl=sa.String()),
            nullable=False,
            server_default='UNPAID')
    payment_due = sa.Column(sa.DateTime)

    welcome_emails = sa.Column(sa.Integer, nullable=False, server_default='0')

    def __init__(self, username):
        self.username = username

    def to_dict(self, first_party=False, short=False):
        return {
            "id": self.id,
            "canonical_name": self.canonical_name,
            "name": self.username,
            **({
                "user_type": self.user_type.value,
                "suspension_notice": self.suspension_notice,
            } if first_party else {}),
            **({
                "email": self.email,
                "url": self.url,
                "location": self.location,
                "bio": self.bio,
                "use_pgp_key": self.pgp_key.fingerprint_hex if self.pgp_key else None,
            } if not short else {})
        }
