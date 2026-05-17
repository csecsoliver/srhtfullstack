import sqlalchemy as sa
import sqlalchemy_utils as sau
from srht.database import Base
from enum import Enum

class ParticipantType(Enum):
    user = "user"
    email = "email"
    external = "external"

class Participant(Base):
    __tablename__ = "participant"
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)

    participant_type = sa.Column(sau.ChoiceType(
            ParticipantType, impl=sa.String()), nullable=False)

    # ParticipantType.user
    user_id = sa.Column(sa.Integer,
            sa.ForeignKey("user.id", ondelete="CASCADE"),
            unique=True)
    user = sa.orm.relationship('User')

    # ParticipantType.email
    email = sa.Column(sa.String, unique=True)
    email_name = sa.Column(sa.String)

    # ParticipantType.external
    external_id = sa.Column(sa.String, unique=True)
    external_url = sa.Column(sa.String)

    @property
    def name(self):
        """Returns a human-friendly display name for this participant"""
        if self.participant_type == ParticipantType.user:
            return self.user.canonical_name
        elif self.participant_type == ParticipantType.email:
            return self.email_name or self.email
        elif self.participant_type == ParticipantType.external:
            return self.external_id
        assert False

    @property
    def identifier(self):
        """Returns a human-friendly unique identifier for this participant"""
        if self.participant_type == ParticipantType.user:
            return self.user.canonical_name
        elif self.participant_type == ParticipantType.email:
            return self.email
        elif self.participant_type == ParticipantType.external:
            return self.external_id
        assert False

    @property
    def notify_self(self):
        if self.participant_type == ParticipantType.user:
            return self.user.notify_self
        else:
            return False

    def __str__(self):
        return self.name

    def __repr__(self):
        return f"<Participant {self.id} [{self.participant_type.value}]>"

    def to_dict(self, short=False):
        if self.participant_type == ParticipantType.user:
            return {
                "type": "user",
                **self.user.to_dict(short),
            }
        elif self.participant_type == ParticipantType.email:
            return {
                "type": "email",
                "address": self.email,
                "name": self.email_name,
            }
        elif self.participant_type == ParticipantType.external:
            return {
                "type": "external",
                "external_id": self.external_id,
                "external_url": self.external_url,
            }
        assert False
