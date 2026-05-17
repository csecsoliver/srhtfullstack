import sqlalchemy as sa
import sqlalchemy_utils as sau
from sqlalchemy.dialects import postgresql
from enum import Enum
from hubsrht.types import Visibility
from srht.config import get_origin
from srht.database import Base

_gitsrht = get_origin("git.sr.ht", external=True, default=None)
_hgsrht = get_origin("hg.sr.ht", external=True, default=None)

class RepoType(Enum):
    git = "GIT"
    hg = "HG"

class SourceRepo(Base):
    __tablename__ = "source_repo"
    __table_args__ = (
        sa.UniqueConstraint(
            "project_id", "remote_id", "repo_type",
            name="project_source_repo_unique",
        ),
    )
    id = sa.Column(sa.Integer, primary_key=True)
    remote_id = sa.Column(sa.Integer, nullable=False)
    remote_rid = sa.Column(sa.String, nullable=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    project_id = sa.Column(sa.Integer,
            sa.ForeignKey("project.id", ondelete="CASCADE"), nullable=False)
    project = sa.orm.relationship("Project",
            backref=sa.orm.backref("source_repos", cascade="all, delete"),
            foreign_keys=[project_id])

    # Note: in theory this may eventually be different from the project owner(?)
    owner_id = sa.Column(sa.Integer,
            sa.ForeignKey("user.id", ondelete="CASCADE"), nullable=False)
    owner = sa.orm.relationship("User")

    name = sa.Column(sa.Unicode(128), nullable=False)
    description = sa.Column(sa.Unicode)
    repo_type = sa.Column(sau.ChoiceType(RepoType, impl=sa.String()),
            nullable=False)
    visibility = sa.Column(postgresql.ENUM(Visibility),
            nullable=False, server_default="UNLISTED")

    webhook_id = sa.Column(sa.Integer)
    webhook_version = sa.Column(sa.Integer)

    def __repr__(self):
        return f"<SourceRepo {self.id}>"

    def url(self):
        origin = _gitsrht if self.repo_type == RepoType.git else _hgsrht
        return f"{origin}/{self.owner.canonical_name}/{self.name}"
