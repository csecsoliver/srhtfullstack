import sqlalchemy as sa
import sqlalchemy_utils as sau
from enum import Enum
from srht.database import Base

class FactorType(Enum):
    totp = "totp"
    u2f = "u2f"
    email = "email"

class UserAuthFactor(Base):
    __tablename__ = 'user_auth_factor'
    id = sa.Column(sa.Integer, primary_key=True)
    user_id = sa.Column(sa.Integer, sa.ForeignKey("user.id"), unique=True)
    user = sa.orm.relationship('User', backref=sa.orm.backref('auth_factors'))
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    factor_type = sa.Column(
            sau.ChoiceType(FactorType, impl=sa.String()),
            nullable=False)
    secret = sa.Column(sa.LargeBinary(4096))
    extra = sa.Column(sa.JSON)
    """Used for additional data, such as the list of recovery codes for 2FA"""

    def __init__(self, user, factor_type):
        self.user_id = user.id
        self.factor_type = factor_type

    def __repr__(self):
        return '<UserAuthFactor {}>'.format(self.id)
