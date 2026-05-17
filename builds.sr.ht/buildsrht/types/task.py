import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.database import Base
from enum import Enum

class TaskStatus(Enum):
    pending = 'pending'
    running = 'running'
    success = 'success'
    failed = 'failed'
    skipped = 'skipped'

class Task(Base):
    __tablename__ = 'task'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    name = sa.Column(sa.Unicode(256), nullable=False)
    status = sa.Column(
            sau.ChoiceType(TaskStatus, impl=sa.String()),
            nullable=False,
            default=TaskStatus.pending)
    job_id = sa.Column(sa.Integer, sa.ForeignKey("job.id"), index=True, nullable=False)
    job = sa.orm.relationship("Job", backref=sa.orm.backref("tasks"))

    def __init__(self, job, name):
        self.job_id = job.id
        self.name = name
