from srht.database import Base
import sqlalchemy as sa
import sqlalchemy_utils as sau

class OAuth2Client(Base):
    __tablename__ = 'oauth2_client'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    owner_id = sa.Column(sa.Integer,
            sa.ForeignKey("user.id", ondelete="CASCADE"),
            nullable=False)

    client_uuid = sa.Column(sau.UUIDType, nullable=False)
    client_secret_hash = sa.Column(sa.String(128), nullable=False)
    client_secret_partial = sa.Column(sa.String(8), nullable=False)
    redirect_url = sa.Column(sa.Unicode)
    revoked = sa.Column(sa.Boolean, nullable=False, server_default='f')

    client_name = sa.Column(sa.Unicode(256), nullable=False)
    client_description = sa.Column(sa.Unicode)
    client_url = sa.Column(sa.Unicode)
