import sqlalchemy as sa
from sqlalchemy.dialects import postgresql
from srht.database import Base
from todosrht.graphql import Authenticity, TicketStatus, TicketResolution
from todosrht.types import TicketAccess

class Ticket(Base):
    __tablename__ = 'ticket'
    __table_args__ = (
        sa.UniqueConstraint('tracker_id', 'scoped_id',
            name="uq_ticket_tracker_id_scoped_id"),
    )
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    tracker_id = sa.Column(sa.Integer,
            sa.ForeignKey("tracker.id", ondelete="CASCADE"),
            nullable=False)
    tracker = sa.orm.relationship("Tracker",
            backref=sa.orm.backref("tickets", cascade="all, delete-orphan"))

    scoped_id = sa.Column(sa.Integer, nullable=False, index=True)

    dupe_of_id = sa.Column(sa.Integer,
            sa.ForeignKey("ticket.id", ondelete="SET NULL"))
    dupe_of = sa.orm.relationship("Ticket",
            backref=sa.orm.backref("dupes"),
            remote_side=[id])

    submitter_id = sa.Column(sa.Integer,
            sa.ForeignKey("participant.id"), nullable=False)
    submitter = sa.orm.relationship("Participant",
            backref=sa.orm.backref("submitted", cascade="all, delete-orphan"))

    title = sa.Column(sa.Unicode(2048), nullable=False)
    description = sa.Column(sa.Unicode(16384))
    comment_count = sa.Column(sa.Integer,
            server_default='0', nullable=False, index=True)

    status = sa.Column(
            postgresql.ENUM(TicketStatus, name='ticket_status'),
            nullable=False)

    resolution = sa.Column(
            postgresql.ENUM(TicketResolution, name='ticket_resolution'),
            nullable=False)


    labels = sa.orm.relationship("Label",
        secondary="ticket_label", order_by="Label.name",
        viewonly=True)

    assigned_users = sa.orm.relationship("User",
        secondary="ticket_assignee",
        foreign_keys="[TicketAssignee.ticket_id,TicketAssignee.assignee_id]",
        viewonly=True)

    authenticity = sa.Column(
            postgresql.ENUM(Authenticity, name='authenticity'),
            nullable=False)
    """
    The authenticity of the ticket. Tickets submitted by logged-in users are
    considered authentic. Tickets which have been exported and re-imported are
    considered authentic if the signature validates, unauthenticated if no
    signature is present, or tampered if the signature does not validate.
    """

    def ref(self, short=False, email=False):
        if short:
            return "#" + str(self.scoped_id)
        if email:
            return "{}/{}/{}".format(
                self.tracker.owner.canonical_name,
                self.tracker.name,
                self.scoped_id)
        return "{}/{}#{}".format(
            self.tracker.owner.canonical_name,
            self.tracker.name,
            self.scoped_id)

    def __repr__(self):
        return f"<Ticket {self.id}>"

    def to_dict(self, short=False):
        return {
            "id": self.scoped_id,
            "ref": self.ref(),
            "tracker": self.tracker.to_dict(short=True),
            "title": self.title,
            **({
                "created": self.created,
                "updated": self.updated,
                "submitter": self.submitter.to_dict(short=True),
                "description": self.description,
                "status": self.status.name.lower(),
                "resolution": self.resolution.name.lower(),
                "labels": [l.name for l in self.labels],
                "assignees": [u.to_dict(short=True) for u in self.assigned_users],
            } if not short else {}),
        }
