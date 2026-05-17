import sqlalchemy as sa
from srht.database import Base

class Label(Base):
    __tablename__ = 'label'

    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    tracker_id = sa.Column(sa.Integer,
            sa.ForeignKey("tracker.id", ondelete="CASCADE"),
            nullable=False)
    tracker = sa.orm.relationship("Tracker",
            backref=sa.orm.backref(
                "labels", cascade="all, delete-orphan", order_by="Label.name"))

    name = sa.Column(sa.Text, nullable=False)
    color = sa.Column(sa.Text, nullable=False)
    text_color = sa.Column(sa.Text, nullable=False)

    tickets = sa.orm.relationship("Ticket",
            secondary="ticket_label", viewonly=True)

    __table_args__ = (
        sa.UniqueConstraint("tracker_id", "name",
            name="idx_tracker_name_unique"),
    )

    def __repr__(self):
        return '<Label {} {}>'.format(self.id, self.name)

    def to_dict(self, short=False):
        return {
            "name": self.name,
            "colors": {
                "background": self.color,
                "text": self.text_color,
            },
            **({
                "created": self.created,
                "tracker": self.tracker.to_dict(short=True),
            } if not short else {})
        }

class TicketLabel(Base):
    __tablename__ = 'ticket_label'
    __table_args__ = (
        sa.UniqueConstraint("ticket_id", "label_id",
            name="idx_label_ticket_unique"),
    )

    ticket_id = sa.Column(sa.Integer,
            sa.ForeignKey('ticket.id', ondelete="CASCADE"),
            primary_key=True)
    ticket = sa.orm.relationship("Ticket",
            backref=sa.orm.backref("ticket_labels",
                cascade="all, delete-orphan"))

    label_id = sa.Column(sa.Integer,
            sa.ForeignKey('label.id', ondelete="CASCADE"),
            primary_key=True)
    label = sa.orm.relationship("Label",
            backref=sa.orm.backref("ticket_labels",
                cascade="all, delete-orphan"))

    user_id = sa.Column(sa.Integer,
            sa.ForeignKey("user.id"),
            nullable=False)
    user = sa.orm.relationship("User",
            backref=sa.orm.backref("ticket_labels",
                cascade="all, delete-orphan"))

    created = sa.Column(sa.DateTime, nullable=False)

    def __repr__(self):
        return '<TicketLabel {} {}>'.format(self.ticket_id, self.label_id)
