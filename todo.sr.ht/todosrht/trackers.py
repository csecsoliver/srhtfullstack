from srht.database import db
from todosrht.types import Event, Ticket, User, Participant


def get_recent_users(tracker, limit=20):
    """Find users who recently interacted with a tracker."""

    recent_user_events = (db.session
        .query(Event.id, Participant.id, User.username)
        .join(Participant, Participant.id == Event.participant_id)
        .join(User, User.id == Participant.user_id)
        .join(Ticket, Ticket.id == Event.ticket_id)
        .filter(Ticket.tracker_id == tracker.id)
        .order_by(Event.created.desc())
        .limit(20))

    return {e[2] for e in recent_user_events}
