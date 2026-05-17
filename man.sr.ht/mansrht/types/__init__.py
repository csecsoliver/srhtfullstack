import sqlalchemy as sa
from srht.database import Base
from srht.oauth import ExternalUserMixin

class User(Base, ExternalUserMixin):
    pass

from mansrht.types.wiki import Wiki, RootWiki, Visibility
