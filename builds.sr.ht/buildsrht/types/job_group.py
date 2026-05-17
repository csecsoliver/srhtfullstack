import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.database import Base
from buildsrht.manifest import Manifest

class JobGroup(Base):
    __tablename__ = 'job_group'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    owner_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'), nullable=False)
    owner = sa.orm.relationship('User', backref=sa.orm.backref('job_groups'))
    note = sa.Column(sa.Unicode(4096))

    def to_dict(self):
        # When updating this, also update worker/triggers.go
        return {
            "id": self.id,
            "note": self.note,
            "owner": self.owner.to_dict(short=True),
            "jobs": [job.to_dict(short=True) for job in self.jobs],
        }
