import sqlalchemy as sa
from srht.database import Base

class UserWebhooks(Base):
    __tablename__ = "user_webhooks"
    id = sa.Column(sa.Integer, primary_key=True)
    user_id = sa.Column(sa.Integer, sa.ForeignKey("user.id"))

    git_webhook_id = sa.Column(sa.Integer)
    git_webhook_version = sa.Column(sa.Integer)
    hg_webhook_id = sa.Column(sa.Integer)
    hg_webhook_version = sa.Column(sa.Integer)
    lists_webhook_id = sa.Column(sa.Integer)
    lists_webhook_version = sa.Column(sa.Integer)
    todo_webhook_id = sa.Column(sa.Integer)
    todo_webhook_version = sa.Column(sa.Integer)
