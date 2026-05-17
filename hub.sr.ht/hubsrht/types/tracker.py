import sqlalchemy as sa
import sqlalchemy_utils as sau
from sqlalchemy.dialects import postgresql
from hubsrht.types import Visibility
from srht.config import get_origin
from srht.database import Base

_todosrht = get_origin("todo.sr.ht", external=True, default=None)

class Tracker(Base):
    __tablename__ = "tracker"
    id = sa.Column(sa.Integer, primary_key=True)
    remote_id = sa.Column(sa.Integer, nullable=False)
    remote_rid = sa.Column(sa.String, nullable=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    project_id = sa.Column(sa.Integer,
            sa.ForeignKey("project.id", ondelete="CASCADE"), nullable=False)
    project = sa.orm.relationship("Project",
            backref=sa.orm.backref("trackers", cascade="all, delete"),
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
        return f"<Tracker {self.id}>"

    def url(self):
        return f"{_todosrht}/{self.owner.canonical_name}/{self.name}"
