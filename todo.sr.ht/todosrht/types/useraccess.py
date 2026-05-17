import sqlalchemy as sa
from srht.database import Base
from srht.flagtype import FlagType
from todosrht.types import TicketAccess


class UserAccess(Base):
    """
    Custom permissions for user on a given tracker.
    """
    __tablename__ = 'user_access'
    id = sa.Column(sa.Integer, primary_key=True)

    tracker_id = sa.Column(sa.Integer,
            sa.ForeignKey("tracker.id", ondelete="CASCADE"), nullable=False)
    tracker = sa.orm.relationship("Tracker",
            backref=sa.orm.backref("user_accesses"))

    user_id = sa.Column(sa.Integer,
            sa.ForeignKey("user.id", ondelete="CASCADE"), nullable=False)
    user = sa.orm.relationship("User")

    permissions = sa.Column(FlagType(TicketAccess), nullable=False)
    created = sa.Column(sa.DateTime, nullable=False)

    __table_args__ = (
        sa.UniqueConstraint("tracker_id", "user_id",
            name="idx_useraccess_tracker_user_unique"),
    )

    def __repr__(self):
        return '<UserAccess {}>'.format(self.id)
