import sqlalchemy as sa
from srht.database import Base
from srht.oauth import ExternalUserMixin
from enum import Enum

class User(Base, ExternalUserMixin):
    webhooks = sa.orm.relationship("UserWebhooks",
            backref="user", uselist=False)

class Visibility(Enum):
    PUBLIC = "PUBLIC"
    PRIVATE = "PRIVATE"
    UNLISTED = "UNLISTED"

from hubsrht.types.event import Event, EventType
from hubsrht.types.eventprojectassoc import EventProjectAssociation
from hubsrht.types.feature import Feature
from hubsrht.types.mailinglist import MailingList
from hubsrht.types.project import Project
from hubsrht.types.redirect import Redirect
from hubsrht.types.sourcerepo import SourceRepo, RepoType
from hubsrht.types.tracker import Tracker
from hubsrht.types.webhooks import UserWebhooks
