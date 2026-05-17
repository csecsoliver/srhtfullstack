import sqlalchemy as sa
from srht.database import Base
from srht.flagtype import FlagType
from listssrht.types.listaccess import ListAccess

class Access(Base):
    __tablename__ = "access"
    __table_args__ = (
            sa.UniqueConstraint('list_id', 'user_id',
                name="uq_access_list_id_user_id"),
            sa.UniqueConstraint('list_id', 'email',
                name="uq_access_list_id_email"),
    )

    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    email = sa.Column(sa.Unicode)
    user_id = sa.Column(sa.Integer, sa.ForeignKey('user.id'))
    user = sa.orm.relationship('User', backref='access_grants')
    list_id = sa.Column(sa.Integer,
            sa.ForeignKey('list.id', ondelete="CASCADE"), nullable=False)
    list = sa.orm.relationship('List',
            backref=sa.orm.backref('acls', cascade="all, delete"))
    permissions = sa.Column(FlagType(ListAccess),
            nullable=False, server_default=str(ListAccess.all.value))

    def __repr__(self):
        return '<Access {} {}->{}:{}>'.format(
                self.id, self.user_id, self.list_id, self.permissions)
