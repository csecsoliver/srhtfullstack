from enum import IntFlag
from srht.database import Base
from srht.oauth import UserMixin
import sqlalchemy as sa

class User(Base, UserMixin):
    notify_self = sa.Column(sa.Boolean, nullable=False, server_default="FALSE")

class TicketAccess(IntFlag):
    NONE = 0
    BROWSE = 1
    SUBMIT = 2
    COMMENT = 4
    EDIT = 8
    TRIAGE = 16
    ALL = BROWSE | SUBMIT | COMMENT | EDIT | TRIAGE

    @property
    def friendly_name(self):
        match self:
            case TicketAccess.NONE:
                return "None"
            case TicketAccess.BROWSE:
                return "Browse"
            case TicketAccess.SUBMIT:
                return "Submit"
            case TicketAccess.COMMENT:
                return "Comment"
            case TicketAccess.EDIT:
                return "Edit"
            case TicketAccess.TRIAGE:
                return "Triage"

from todosrht.types.event import Event, EventType, EventNotification
from todosrht.types.label import Label, TicketLabel
from todosrht.types.participant import Participant, ParticipantType
from todosrht.types.redirect import Redirect
from todosrht.types.ticket import Ticket
from todosrht.types.ticketassignee import TicketAssignee
from todosrht.types.ticketcomment import TicketComment
from todosrht.types.ticketsubscription import TicketSubscription
from todosrht.types.tracker import Tracker
from todosrht.types.useraccess import UserAccess
