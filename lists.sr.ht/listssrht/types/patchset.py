import sqlalchemy as sa
import sqlalchemy_utils as sau
from enum import Enum
from srht.database import Base

class PatchsetStatus(Enum):
    unknown = "unknown"
    proposed = "proposed"
    needs_revision = "needs_revision"
    superseded = "superseded"
    approved = "approved"
    rejected = "rejected"
    applied = "applied"

class Patchset(Base):
    __tablename__ = 'patchset'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    subject = sa.Column(sa.Unicode(2048), nullable=False)
    prefix = sa.Column(sa.Unicode)
    version = sa.Column(sa.Integer, nullable=False)

    # These 3 fields are used to help software reply to patchsets in an
    # automated fashion
    submitter = sa.Column(sa.Unicode)
    """From header of the last patch"""
    reply_to = sa.Column(sa.Unicode)
    """Reply-To header of the last patch"""
    message_id = sa.Column(sa.Unicode)
    """Message ID of the last message in the patchset (for In-Reply-To)"""

    status = sa.Column(sau.ChoiceType(PatchsetStatus, impl=sa.String()),
            nullable=False, server_default="proposed")

    list_id = sa.Column(sa.Integer,
            sa.ForeignKey('list.id', ondelete="CASCADE"),
            nullable=False)
    list = sa.orm.relationship('List',
            backref=sa.orm.backref('patchsets'))

    cover_letter_id = sa.Column(sa.Integer, sa.ForeignKey('email.id'))
    cover_letter = sa.orm.relationship("Email",
            foreign_keys=[cover_letter_id],
            passive_deletes=True)

    superseded_by_id = sa.Column(sa.Integer, sa.ForeignKey('patchset.id'))
    superseded_by = sa.orm.relationship('Patchset',
            backref=sa.orm.backref('previous_version', remote_side=[id]),
            foreign_keys=[superseded_by_id])
    supersedes_id = sa.Column(sa.Integer, sa.ForeignKey('patchset.id'))
    supersedes = sa.orm.relationship('Patchset',
            backref=sa.orm.backref('next_version', remote_side=[id]),
            foreign_keys=[supersedes_id])

class ToolIcon(Enum):
    pending = "pending"
    waiting = "waiting"
    success = "success"
    failed = "failed"
    cancelled = "cancelled"

class PatchsetTool(Base):
    __tablename__ = 'patchset_tool'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    patchset_id = sa.Column(sa.Integer,
            sa.ForeignKey('patchset.id', ondelete='CASCADE'))
    patchset = sa.orm.relationship('Patchset', backref=sa.orm.backref('tools'))

    icon = sa.Column(sau.ChoiceType(ToolIcon, impl=sa.String()),
            nullable=False, server_default="pending")
    details = sa.Column(sa.Unicode, nullable=False)
    key = sa.Column(sa.Unicode(128), nullable=False, index=True)
