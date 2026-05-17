import sqlalchemy as sa
import sqlalchemy_utils as sau
from hubsrht.types import Visibility
from srht.database import Base

class Feature(Base):
    __tablename__ = "features"
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)

    project_id = sa.Column(sa.Integer,
            sa.ForeignKey("project.id", ondelete="CASCADE"),
            nullable=False)
    project = sa.orm.relationship("Project")

    summary = sa.Column(sa.Unicode(), nullable=False)
