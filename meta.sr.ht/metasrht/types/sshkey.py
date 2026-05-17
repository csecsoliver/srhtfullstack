import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.database import Base, db

class SSHKey(Base):
    __tablename__ = 'sshkey'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime)
    user_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'))
    user = sa.orm.relationship('User', backref=sa.orm.backref('ssh_keys'))
    key = sa.Column("key", sa.String(4096), nullable=False)
    key_type = sa.Column("key_type", sa.String(256), nullable=False)
    fingerprint_sha256 = sa.Column(sa.String(512), nullable=False, unique=True)
    comment = sa.Column(sa.String(256))
    last_used = sa.Column(sa.DateTime)

    def __repr__(self):
        return '<SSHKey {} {}>'.format(self.id, self.fingerprint_sha256)

    def as_authorized_key(self):
        return '{} {} {}'.format(self.key_type, self.key, self.comment).strip()
