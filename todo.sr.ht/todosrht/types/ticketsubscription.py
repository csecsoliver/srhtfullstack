import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.database import Base

class TicketSubscription(Base):
    """One of user, email, or webhook will be valid. The rest will be null."""
    __tablename__ = 'ticket_subscription'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    tracker_id = sa.Column(sa.Integer,
            sa.ForeignKey("tracker.id", ondelete="CASCADE"))
    tracker = sa.orm.relationship("Tracker",
            backref=sa.orm.backref("subscriptions",
                cascade="all, delete-orphan"))
    """Used for subscriptions to all tickets on a tracker"""

    ticket_id = sa.Column(sa.Integer,
            sa.ForeignKey("ticket.id", ondelete="CASCADE"))
    ticket = sa.orm.relationship("Ticket",
            backref=sa.orm.backref("subscriptions",
                cascade="all, delete-orphan"))
    """Used for subscriptions to specific tickets"""

    participant_id = sa.Column(sa.Integer,
            sa.ForeignKey("participant.id"))
    participant = sa.orm.relationship("Participant",
            backref=sa.orm.backref("subscriptions"))

    def __repr__(self):
        return (f"<TicketSubscription {self.id} {self.participant}; " +
            f"tk: {self.ticket_id}; tr: {self.tracker_id}>")
