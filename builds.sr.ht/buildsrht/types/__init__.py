from srht.database import Base
from srht.oauth import UserMixin

class User(Base, UserMixin):
    pass

from .job import Job, JobStatus
from .task import Task, TaskStatus
from .job_group import JobGroup
from .trigger import Trigger, TriggerType, TriggerCondition
from .secret import Secret, SecretType
from .artifact import Artifact
