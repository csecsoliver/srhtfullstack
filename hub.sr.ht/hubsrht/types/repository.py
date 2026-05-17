import sqlalchemy as sa
import sqlalchemy_utils as sau
from sqlalchemy.dialects import postgresql
from hubsrht.types import Visibility
from srht.database import Base
from enum import Enum

class RepositoryType(Enum):
    git = "GIT"
    hg = "HG"

class Repository(Base):
    __tablename__ = "repository"
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    remote_id = sa.Column(sa.Integer, nullable=False)

    project_id = sa.Column(sa.Integer, sa.ForeignKey("project.id"))
    project = sa.orm.relationship("Project", backref=sa.orm.backref("repositories"))

    name = sa.Column(sa.Unicode(128), nullable=False)
    description = sa.Column(sa.Unicode(512), nullable=False)
    visibility = sa.Column(postgresql.ENUM(Visibility),
            nullable=False, server_default="UNLISTED")

    webhook_id = sa.Column(sa.Integer, nullable=False)
    webhook_version = sa.Column(sa.Integer, nullable=False)
