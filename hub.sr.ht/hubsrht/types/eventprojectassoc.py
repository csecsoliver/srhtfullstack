import sqlalchemy as sa
import sqlalchemy_utils as sau
from enum import Enum
from sqlalchemy.dialects import postgresql
from srht.database import Base

class EventProjectAssociation(Base):
    __tablename__ = "event_project_association"

    event_id =  sa.Column(
        sa.Integer,
        sa.ForeignKey("event.id", ondelete="CASCADE"),
        primary_key=True,
    )

    project_id = sa.Column(
        sa.Integer,
        sa.ForeignKey("project.id", ondelete="CASCADE"),
        primary_key=True,
    )
