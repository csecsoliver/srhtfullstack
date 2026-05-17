from srht.database import Base
import sqlalchemy as sa

class OAuth2Grant(Base):
    __tablename__ = 'oauth2_grant'
    id = sa.Column(sa.Integer, primary_key=True)
    issued = sa.Column(sa.DateTime, nullable=False)
    expires = sa.Column(sa.DateTime, nullable=False)
    comment = sa.Column(sa.Unicode)
    grants = sa.Column(sa.Unicode)

    token_hash = sa.Column(sa.String(128), nullable=False)

    user_id = sa.Column(sa.Integer,
            sa.ForeignKey("user.id", ondelete="CASCADE"),
            nullable=False)
    client_id = sa.Column(sa.Integer,
            sa.ForeignKey("oauth2_client.id", ondelete="CASCADE"))
