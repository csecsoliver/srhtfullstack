import os
import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.config import cfg
from srht.database import Base

class Artifact(Base):
    __tablename__ = 'artifacts'
    __table_args__ = (
        sa.UniqueConstraint("repo_id", "filename",
            name="repo_artifact_filename_unique"),
    )

    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    user_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'), nullable=False)
    user = sa.orm.relationship('User')
    repo_id = sa.Column(sa.Integer, sa.ForeignKey('repository.id'), nullable=False)
    repo = sa.orm.relationship('Repository')
    commit = sa.Column(sa.Unicode, nullable=False)
    filename = sa.Column(sa.Unicode, nullable=False)
    checksum = sa.Column(sa.Unicode, nullable=False)
    size = sa.Column(sa.Integer, nullable=False)

    def __repr__(self):
        return '<Artifact {} {}>'.format(self.id, self.filename)
