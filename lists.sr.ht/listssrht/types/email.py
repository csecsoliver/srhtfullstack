import email
import io
import pygit2
import sqlalchemy as sa
from srht.database import Base

class Email(Base):
    __tablename__ = 'email'
    _no_autoupdate = True

    __table_args__ = (
        sa.UniqueConstraint("list_id", "message_id",
            name="uq_email_list_message_id"),
    )

    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    subject = sa.Column(sa.Unicode(2048), nullable=False)
    message_id = sa.Column(sa.Unicode(2048), nullable=False)
    in_reply_to = sa.Column(sa.Unicode(2048))
    headers = sa.Column(sa.JSON, nullable=False)
    body = sa.Column(sa.Unicode, nullable=False)
    raw_message = sa.Column(sa.LargeBinary, nullable=False)
    is_patch = sa.Column(sa.Boolean, nullable=False)
    """true if email is via git format-patch"""
    is_request_pull = sa.Column(sa.Boolean, nullable=False)
    """true if email is via git request-pull"""
    message_date = sa.Column(sa.DateTime)

    list_id = sa.Column(sa.Integer,
            sa.ForeignKey('list.id', ondelete="CASCADE"),
            nullable=False)
    list = sa.orm.relationship('List', backref=sa.orm.backref('messages'))

    parent_id = sa.Column(sa.Integer, sa.ForeignKey('email.id'))
    replies = sa.orm.relationship('Email',
            backref=sa.orm.backref('parent',
                remote_side=[id]),
            foreign_keys=[parent_id])

    thread_id = sa.Column(sa.Integer, sa.ForeignKey('email.id'))
    descendants = sa.orm.relationship('Email',
            backref=sa.orm.backref('thread',
                remote_side=[id]),
            foreign_keys=[thread_id])

    nreplies = sa.Column(sa.Integer, server_default='0')
    nparticipants = sa.Column(sa.Integer, server_default='1')

    sender_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'))
    sender = sa.orm.relationship('User',
            backref=sa.orm.backref('sent_messages'))

    # "[PATCH meta.sr.ht v2 1/4] Add thing to stuff"
    # patch_index: 1; patch_count: 4
    # patch_version: 2
    # patch_prefix: meta.sr.ht
    # patch_subject: Add thing to stuff
    patch_index = sa.Column(sa.Integer)
    patch_count = sa.Column(sa.Integer)
    patch_version = sa.Column(sa.Integer)
    patch_prefix = sa.Column(sa.Unicode)
    patch_subject = sa.Column(sa.Unicode)

    superseded_by_id = sa.Column(sa.Integer, sa.ForeignKey('email.id'))
    superseded_by = sa.orm.relationship('Email',
            backref=sa.orm.backref('previous_version', remote_side=[id]),
            foreign_keys=[superseded_by_id])

    patchset_id = sa.Column(sa.Integer,
            sa.ForeignKey('patchset.id', ondelete="SET NULL"))
    patchset = sa.orm.relationship("Patchset",
            backref=sa.orm.backref("patches"), foreign_keys=[patchset_id])

    # TODO: Enumerate CC's and create a relationship there

    def __repr__(self):
        return '<Email {} {}>'.format(self.id, self.subject)

    def parsed(self):
        if hasattr(self, "_parsed"):
            return self._parsed
        policy = email.policy.SMTPUTF8.clone(max_line_length=998)
        self._parsed = email.message_from_bytes(
                self.raw_message, policy=policy)
        self._parsed._email = self
        return self._parsed

    # libgit2 Diff object parsed from message body (if it exists)
    def patch(self):
        if not hasattr(self, "_patch"):
            body = self.body.replace("\r\n", "\n")
            # mercurial/patchbomb emails' body may not end with a EOL; this
            # makes pygit2 fail to parse diff hunks
            if not body.endswith("\n"):
                body += "\n"
            try:
                self._patch = pygit2.Diff.parse_diff(body)
                self.is_patch = len(self._patch) > 0
            except:
                self.is_patch = False

        if self.is_patch:
            return self._patch

        return None
