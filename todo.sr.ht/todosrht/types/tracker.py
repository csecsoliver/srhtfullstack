import sqlalchemy as sa
from enum import Enum
from sqlalchemy.dialects import postgresql
from srht.database import Base
from srht.flagtype import FlagType
from srht.validation import Validation
from todosrht.graphql import TicketStatus, TicketResolution, Visibility
from todosrht.types import TicketAccess

class Tracker(Base):
    __tablename__ = 'tracker'
    __table_args__ = (
        sa.UniqueConstraint("owner_id", "name",
            name="tracker_owner_id_name_unique"),
    )

    id = sa.Column(sa.Integer, primary_key=True)
    rid = sa.Column(sa.UUID, nullable=False)
    owner_id = sa.Column(sa.Integer, sa.ForeignKey("user.id"), nullable=False)
    owner = sa.orm.relationship("User", backref=sa.orm.backref("owned_trackers"))
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    visibility = sa.Column(
        postgresql.ENUM(Visibility, name="visibility"),
        nullable=False)
    name = sa.Column(sa.Unicode(1024))
    """
    May include slashes to serve as categories (nesting is supported,
    builds.sr.ht style)
    """
    next_ticket_id = sa.Column(sa.Integer, nullable=False, server_default='1')

    description = sa.Column(sa.Unicode(8192))
    """Markdown"""

    default_access = sa.Column(FlagType(TicketAccess), nullable=False)

    import_in_progress = sa.Column(sa.Boolean,
            nullable=False, server_default='f')

    def ref(self):
        return "{}/{}".format(
            self.owner.canonical_name,
            self.name)

    def __repr__(self):
        return '<Tracker {} {}>'.format(self.id, self.name)

    def to_dict(self, short=False):
        return {
            "id": self.id,
            "owner": self.owner.to_dict(short=True),
            "created": self.created,
            "updated": self.updated,
            "name": self.name,
        }
