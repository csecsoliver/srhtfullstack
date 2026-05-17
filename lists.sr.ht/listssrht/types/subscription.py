import sqlalchemy as sa
from srht.database import Base
import base64
import os

class Subscription(Base):
    __tablename__ = 'subscription'

    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    email = sa.Column(sa.Unicode(512))

    list_id = sa.Column(sa.Integer,
            sa.ForeignKey('list.id', ondelete="CASCADE"),
            nullable=False)
    list = sa.orm.relationship('List', backref=sa.orm.backref('subscribers'))

    # Non-users can subscribe, so this might be null
    user_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'))
    user = sa.orm.relationship('User', backref=sa.orm.backref('subscriptions'))

    def __repr__(self):
        return '<Subscription {} {} -> list {}>'.format(
                self.id, self.email or self.user_id, self.list_id)
