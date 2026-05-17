import sqlalchemy as sa
from sqlalchemy.ext.declarative import declared_attr
from srht.database import Base


class Redirect(Base):
    @declared_attr
    def __tablename__(cls):
        return "redirect"

    id = sa.Column(sa.Integer, primary_key=True)
    owner_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'), nullable=False)
    owner = sa.orm.relationship('User')
    created = sa.Column(sa.DateTime, nullable=False)
    name = sa.Column(sa.Unicode(256), nullable=False)
    new_tracker_id = sa.Column(
        sa.Integer,
        sa.ForeignKey('tracker.id', ondelete="CASCADE"),
        nullable=False
    )
    new_tracker = sa.orm.relationship('Tracker')
