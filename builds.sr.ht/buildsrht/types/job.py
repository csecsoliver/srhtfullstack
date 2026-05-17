import sqlalchemy as sa
import sqlalchemy_utils as sau
from buildsrht.graphql import Visibility
from buildsrht.manifest import Manifest
from enum import Enum
from sqlalchemy.dialects import postgresql
from srht.database import Base

# TODO: Replace me with a PG enum type
class JobStatus(Enum):
    pending = 'pending'
    queued = 'queued'
    running = 'running'
    success = 'success'
    failed = 'failed'
    timeout = 'timeout'
    cancelled = 'cancelled'

class Job(Base):
    __tablename__ = 'job'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    manifest = sa.Column(sa.Unicode(16384), nullable=False)
    owner_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'), index=True, nullable=False)
    owner = sa.orm.relationship('User', backref=sa.orm.backref('jobs'))
    job_group_id = sa.Column(sa.Integer, sa.ForeignKey('job_group.id'))
    job_group = sa.orm.relationship('JobGroup', backref=sa.orm.backref('jobs'))
    secrets = sa.Column(sa.Boolean, nullable=False, server_default="t")
    note = sa.Column(sa.Unicode(4096))
    tags = sa.Column(sa.String())
    runner = sa.Column(sa.String)
    status = sa.Column(
            sau.ChoiceType(JobStatus, impl=sa.String()),
            nullable=False,
            default=JobStatus.pending)
    image = sa.Column(sa.String(256))
    visibility = sa.Column(
        postgresql.ENUM(Visibility, name='visibility'),
        nullable=False)

    def __init__(self, owner, manifest):
        self.owner_id = owner.id
        self.manifest = manifest

    def to_dict(self, short=False):
        # When updating this, also update worker/triggers.go
        return {
            "id": self.id,
            "status": self.status.value,
            **({
                "setup_log": (
                    "http://{}/logs/{}/log".format(self.runner, self.id)
                    if self.runner else None
                ),
                "tasks": [
                    {
                        "name": task.name,
                        "status": task.status.value,
                        "log": (
                            "http://{}/logs/{}/{}/log".format(
                                self.runner, self.id, task.name)
                            if self.runner else None
                        ),
                    } for task in self.tasks
                ],
                "note": self.note,
                "runner": self.runner,
                "tags": self.tags,
                "owner": self.owner.to_dict(short=True),
            } if not short else {}),
        }
