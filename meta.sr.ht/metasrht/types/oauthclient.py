import sqlalchemy as sa
from srht.database import Base
from srht.oauth import OAuthClientMixin
import hashlib
import binascii
import os

class OAuthClient(Base, OAuthClientMixin):
    __tablename__ = 'oauthclient'
    client_name = sa.Column(sa.Unicode(256), nullable=False)
    client_id = sa.Column(sa.String(16), nullable=False)
    client_secret_hash = sa.Column(sa.String(128), nullable=False)
    client_secret_partial = sa.Column(sa.String(8), nullable=False)
    redirect_uri = sa.Column(sa.String(256))
    preauthorized = sa.Column(sa.Boolean, nullable=False, default=False)

    def __init__(self, user, client_name, redirect_uri):
        self.user_id = user.id
        self.client_name = client_name
        self.redirect_uri = redirect_uri
        self.gen_client_id()

    def gen_client_id(self):
        self.client_id = binascii.hexlify(os.urandom(8)).decode()

    def gen_client_secret(self):
        secret = binascii.hexlify(os.urandom(16)).decode()
        self.client_secret_partial = secret[:8]
        self.client_secret_hash = hashlib.sha512(secret.encode()).hexdigest()
        return secret
