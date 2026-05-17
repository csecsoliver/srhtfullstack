import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.database import Base
from enum import Enum

class TriggerCondition(Enum):
    success = 'success'
    failure = 'failure'
    always = 'always'

class TriggerType(Enum):
    job = 'job'
    email = 'email'
    irc = 'irc'
    webhook = 'webhook'

class Trigger(Base):
    __tablename__ = 'trigger'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    details = sa.Column(sa.String(4096), nullable=False)
    condition = sa.Column(
            sau.ChoiceType(TriggerCondition, impl=sa.String()),
            nullable=False)
    trigger_type = sa.Column(
            sau.ChoiceType(TriggerType, impl=sa.String()),
            nullable=False)
    job_id = sa.Column(sa.Integer, sa.ForeignKey('job.id'))
    job = sa.orm.relationship('Job', backref=sa.orm.backref('triggers'))
    job_group_id = sa.Column(sa.Integer, sa.ForeignKey('job_group.id'))
    job_group = sa.orm.relationship('JobGroup', backref=sa.orm.backref('triggers'))

    def __init__(self, job_or_group):
        from buildsrht.types import Job, JobGroup
        if isinstance(job_or_group, Job):
            self.job_id = job_or_group.id
        if isinstance(job_or_group, JobGroup):
            self.job_group_id = job_or_group.id
