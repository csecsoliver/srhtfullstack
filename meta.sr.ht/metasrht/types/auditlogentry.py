import sqlalchemy as sa
import sqlalchemy_utils as sau
from enum import Enum
from srht.database import Base

class AuditLogEntry(Base):
    __tablename__ = 'audit_log_entry'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    user_id = sa.Column(sa.Integer,
            sa.ForeignKey('user.id'),
            nullable=False)
    user = sa.orm.relationship('User', backref=sa.orm.backref('audit_log'))
    ip_address = sa.Column(sau.IPAddressType, nullable=False)
    event_type = sa.Column(sa.String(256), nullable=False)
    details = sa.Column(sa.Unicode(512))

    def __init__(self, user_id, event_type, ip_address, details):
        self.user_id = user_id
        self.event_type = event_type
        self.ip_address = ip_address
        self.details = details

    def __repr__(self):
        return "<AuditLogEntry {} {} {}>".format(
                self.id, self.ip_address, self.event_type)

    def to_dict(self):
        return {
            "id": self.id,
            "ip": str(self.ip_address),
            "action": self.event_type,
            "details": self.details,
            "created": self.created,
        }
