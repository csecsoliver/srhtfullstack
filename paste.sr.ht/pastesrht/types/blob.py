import sqlalchemy as sa
from srht.database import Base

class Blob(Base):
    __tablename__ = 'blob'
    __table_args__ = (
        sa.UniqueConstraint('sha', name='sha_unique'),
    )
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    sha = sa.Column(sa.String(40), nullable=False, index=True)
    contents = sa.Column(sa.Unicode, nullable=False)
