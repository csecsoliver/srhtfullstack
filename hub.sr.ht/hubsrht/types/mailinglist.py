import sqlalchemy as sa
import sqlalchemy_utils as sau
from sqlalchemy.dialects import postgresql
from hubsrht.types import Visibility
from srht.config import get_origin
from srht.database import Base
from urllib.parse import urlparse

_listsrht = get_origin("lists.sr.ht", external=True, default=None)

class MailingList(Base):
    __tablename__ = "mailing_list"
    id = sa.Column(sa.Integer, primary_key=True)
    remote_id = sa.Column(sa.Integer, nullable=False)
    remote_rid = sa.Column(sa.String, nullable=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    project_id = sa.Column(sa.Integer,
            sa.ForeignKey("project.id", ondelete="CASCADE"), nullable=False)
    project = sa.orm.relationship("Project",
            backref=sa.orm.backref("mailing_lists", cascade="all, delete"),
            foreign_keys=[project_id])

    # Note: in theory this may eventually be different from the project owner(?)
    owner_id = sa.Column(sa.Integer,
            sa.ForeignKey("user.id", ondelete="CASCADE"), nullable=False)
    owner = sa.orm.relationship("User")

    name = sa.Column(sa.Unicode(128), nullable=False)
    description = sa.Column(sa.Unicode)
    visibility = sa.Column(postgresql.ENUM(Visibility),
            nullable=False, server_default="UNLISTED")

    webhook_id = sa.Column(sa.Integer, nullable=False)
    webhook_version = sa.Column(sa.Integer, nullable=False)

    def __repr__(self):
        return f"<MailingList {self.id}>"

    def url(self):
        return f"{_listsrht}/{self.owner.canonical_name}/{self.name}"

    def posting_addr(self):
        p = urlparse(_listsrht)
        return f"{self.owner.canonical_name}/{self.name}@{p.netloc}"
