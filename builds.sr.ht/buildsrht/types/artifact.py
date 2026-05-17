import sqlalchemy as sa
from srht.database import Base

class Artifact(Base):
    __tablename__ = 'artifact'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    job_id = sa.Column(sa.Integer, sa.ForeignKey('job.id'), nullable=False)
    job = sa.orm.relationship('Job', backref=sa.orm.backref('artifacts'))
    path = sa.Column(sa.Unicode, nullable=False)
    """Original path on the guest the file was pulled from"""
    name = sa.Column(sa.Unicode, nullable=False)
    """Basename of the file"""
    url = sa.Column(sa.Unicode, nullable=False)
    """URL from which the file may be downloaded"""
    size = sa.Column(sa.Integer, nullable=False)
    """File size in bytes"""

    def to_dict(self):
        return {
            "id": self.id,
            "created": self.created,
            "path": self.path,
            "name": self.name,
            "url": self.url,
            "size": self.size,
        }
