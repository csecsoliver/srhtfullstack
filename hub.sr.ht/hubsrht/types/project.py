import sqlalchemy as sa
import sqlalchemy_utils as sau
from sqlalchemy.dialects import postgresql
from hubsrht.types import Visibility
from hubsrht.types.eventprojectassoc import EventProjectAssociation
from srht.database import Base

class Project(Base):
    __tablename__ = "project"
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)

    owner_id = sa.Column(sa.Integer,
            sa.ForeignKey("user.id", ondelete="CASCADE"),
            nullable=False)
    owner = sa.orm.relationship("User",
            backref=sa.orm.backref("projects", cascade="all, delete"))

    name = sa.Column(sa.Unicode(128), nullable=False)
    description = sa.Column(sa.Unicode(512), nullable=False)
    tags = sa.Column(sa.ARRAY(sa.String(16), dimensions=1),
            nullable=False, server_default="{}")
    website = sa.Column(sa.Unicode)
    visibility = sa.Column(postgresql.ENUM(Visibility),
            nullable=False, server_default="UNLISTED")

    checklist_complete = sa.Column(sa.Boolean,
            nullable=False, server_default='f')

    summary_repo_id = sa.Column(sa.Integer,
            sa.ForeignKey("source_repo.id", ondelete="CASCADE"))
    summary_repo = sa.orm.relationship("SourceRepo",
            foreign_keys=[summary_repo_id],
            cascade="all, delete")

    events = sa.orm.relationship(
        "Event",
        secondary=EventProjectAssociation.__table__,
        back_populates="projects",
    )
