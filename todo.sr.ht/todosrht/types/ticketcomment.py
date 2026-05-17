import sqlalchemy as sa
from sqlalchemy.dialects import postgresql
from srht.database import Base
from todosrht.graphql import Authenticity

class TicketComment(Base):
    __tablename__ = 'ticket_comment'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    submitter_id = sa.Column(sa.Integer,
            sa.ForeignKey("participant.id"), nullable=False)
    submitter = sa.orm.relationship("Participant")

    ticket_id = sa.Column(sa.Integer,
            sa.ForeignKey("ticket.id", ondelete="CASCADE"),
            nullable=False)
    ticket = sa.orm.relationship("Ticket",
            backref=sa.orm.backref("comments", cascade="all, delete-orphan"))

    text = sa.Column(sa.Unicode(16384))

    authenticity = sa.Column(
            postgresql.ENUM(Authenticity, name='authenticity'),
            nullable=False)
    """
    The authenticity of the comment. Comments submitted by logged-in users are
    considered authentic. Comments which have been exported and re-imported are
    considered authentic if the signature validates, unauthenticated if no
    signature is present, or tampered if the signature does not validate.
    """

    superceeded_by_id = sa.Column(sa.Integer,
            sa.ForeignKey("ticket_comment.id", ondelete="SET NULL"))
    superceeded_by = sa.orm.relationship("TicketComment",
            backref=sa.orm.backref("superceedes"),
            remote_side=[id])

    def to_dict(self, short=False):
        return {
            "id": self.id,
            "created": self.created,
            "submitter": self.submitter.to_dict(short=True),
            "text": self.text,
            **({
                "ticket": self.ticket.to_dict(short=True),
            } if not short else {})
        }
