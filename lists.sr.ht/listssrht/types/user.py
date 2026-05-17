from srht.database import Base
from srht.oauth import UserMixin
import sqlalchemy as sa

class User(Base, UserMixin):
    # TODO: move sessions into core.sr.ht
    session = sa.Column(sa.String(128))
