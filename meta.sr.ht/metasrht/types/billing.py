import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.database import Base
from metasrht.graphql import SubscriptionStatus

class Subscription(Base):
    __tablename__ = "subscription"
    id = sa.Column(sa.Integer, primary_key=True)
    user_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'))
    user = sa.orm.relationship("User")
    status = sa.Column(
            sau.ChoiceType(SubscriptionStatus, impl=sa.String()),
            nullable=False,
            server_default='PENDING')
    payment_intent = sa.Column(sa.Text)
