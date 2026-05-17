import re
from itertools import chain
from srht.config import cfg
from srht.database import db
from todosrht.access import get_access
from todosrht.types import Participant, ParticipantType
from todosrht.types import User, Ticket, Tracker, TicketAccess
from sqlalchemy import or_, and_

origin = cfg("todo.sr.ht", "origin")

# Matches user mentions, e.g. ~username
USER_MENTION_PATTERN = re.compile(r"""
    (?<![^\s(])  # No leading non-whitespace characters
    ~            # Literal tilde
    ([\w-]+)     # The username
    \b           # Word boundary
    (?!/)        # Not followed by slash, possible qualified ticket mention
""", re.VERBOSE)

# Matches ticket mentions, e.g. #17, tracker#17 and ~user/tracker#17
TICKET_MENTION_PATTERN = re.compile(r"""
    (?<![^\s(])                         # No leading non-whitespace characters
    (~(?P<username>\w+)/)?              # Optional username
    (?P<tracker_name>[A-Za-z0-9_.-]+)?  # Optional tracker name
    \#(?P<ticket_id>\d+)                # Ticket ID
    \b                                  # Word boundary
""", re.VERBOSE)

# Matches ticket URL
TICKET_URL_PATTERN = re.compile(f"""
    (?<![^\\s(])                        # No leading non-whitespace characters
    {origin}/                           # Base URL
    ~(?P<username>\\w+)/                # Username
    (?P<tracker_name>[A-Za-z0-9_.-]+)/  # Tracker name
    (?P<ticket_id>\\d+)                 # Ticket ID
    \\b                                 # Word boundary
""", re.VERBOSE)

def get_participant_for_user(user):
    participant = Participant.query.filter(
            Participant.user_id == user.id).one_or_none()
    if not participant:
        participant = Participant()
        participant.user_id = user.id
        participant.participant_type = ParticipantType.user
        db.session.add(participant)
        db.session.flush()
    return participant

def get_participant_for_email(email, email_name=None):
    user = User.query.filter(User.email == email).one_or_none()
    if user:
        return get_participant_for_user(user)
    participant = Participant.query.filter(
            Participant.email == email).one_or_none()
    if not participant:
        participant = Participant()
        participant.email = email
        participant.email_name = email_name
        participant.participant_type = ParticipantType.email
        db.session.add(participant)
        db.session.flush()
    return participant

def get_participant_for_external(external_id, external_url):
    participant = Participant.query.filter(
            Participant.external_id == external_id).one_or_none()
    if not participant:
        participant = Participant()
        participant.external_id = external_id
        participant.external_url = external_url
        participant.participant_type = ParticipantType.external
        db.session.add(participant)
        db.session.flush()
    return participant

def find_mentioned_users(text):
    if text is None:
        return set()
    # TODO: Find mentioned email addresses as well
    usernames = re.findall(USER_MENTION_PATTERN, text)
    users = User.query.filter(User.username.in_(usernames)).all()
    participants = set([get_participant_for_user(u) for u in set(users)])
    return participants

def find_mentioned_tickets(tracker, text):
    if text is None:
        return set()
    filters = []
    matches = chain(
        re.finditer(TICKET_MENTION_PATTERN, text),
        re.finditer(TICKET_URL_PATTERN, text),
    )

    for match in matches:
        username = match.group('username') or tracker.owner.username
        tracker_name = match.group('tracker_name') or tracker.name
        ticket_id = int(match.group('ticket_id'))

        filters.append(and_(
            Ticket.scoped_id == ticket_id,
            Tracker.name == tracker_name,
            User.username == username,
        ))

    # No tickets mentioned
    if len(filters) == 0:
        return set()

    tickets = set(Ticket.query
        .join(Tracker).join(User)
        .filter(or_(*filters))
        .all())

    def filter_by_access(tickets):
        for t in tickets:
            access = get_access(t.tracker, None)
            if not TicketAccess.BROWSE in access:
                continue
            yield t

    return list(filter_by_access(tickets))
