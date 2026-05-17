from flask import has_app_context, url_for
from jinja2.utils import url_quote
from todosrht.types import ParticipantType

def tracker_url(tracker):
    return url_for("tracker.tracker_GET",
        owner=tracker.owner.canonical_name,
        name=tracker.name)

def tracker_labels_url(tracker):
    return url_for("tracker.tracker_labels_GET",
        owner=tracker.owner.canonical_name,
        name=tracker.name)

def ticket_url(ticket, event=None):
    # Tickets have scoped_id via SQLAlchemy, id via GraphQL
    ticket_id = ticket.scoped_id if hasattr(ticket, "scoped_id") else ticket.id

    if has_app_context():
        ticket_url = url_for("ticket.ticket_GET",
                owner=ticket.tracker.owner.canonical_name,
                name=ticket.tracker.name,
                ticket_id=ticket_id)
    else:
        ticket_url = (f"/{ticket.tracker.owner.canonical_name}" +
            f"/{ticket.tracker.name}" +
            f"/{ticket_id}")

    if event:
        ticket_url += "#event-" + str(event.id)

    return ticket_url

def ticket_edit_url(ticket):
    return url_for("ticket.ticket_edit_GET",
        owner=ticket.tracker.owner.canonical_name,
        name=ticket.tracker.name,
        ticket_id=ticket.scoped_id)

def ticket_delete_url(ticket):
    return url_for("ticket.ticket_delete_GET",
        owner=ticket.tracker.owner.canonical_name,
        name=ticket.tracker.name,
        ticket_id=ticket.scoped_id)

def ticket_assign_url(ticket):
    return url_for("ticket.ticket_assign",
        owner=ticket.tracker.owner.canonical_name,
        name=ticket.tracker.name,
        ticket_id=ticket.scoped_id)

def ticket_unassign_url(ticket):
    return url_for("ticket.ticket_unassign",
        owner=ticket.tracker.owner.canonical_name,
        name=ticket.tracker.name,
        ticket_id=ticket.scoped_id)

def label_edit_url(label):
    return url_for("tracker.label_edit_GET",
        owner=label.tracker.owner.canonical_name,
        name=label.tracker.name,
        label_name=label.name)

def label_search_url(label, terms=""):
    """Return the URL to the tracker page listing all tickets which have the
    label applied."""
    label_term = f"label:\"{label.name}\""
    if not terms:
        terms = label_term
    elif label_term not in terms:
        terms += " " + label_term
    return "{}?search={}".format(
        tracker_url(label.tracker),
        url_quote(terms))

def label_add_url(ticket):
    """Return the URL to add a label to a ticket."""
    return url_for("ticket.ticket_add_label",
            owner=ticket.tracker.owner.canonical_name,
            name=ticket.tracker.name,
            ticket_id=ticket.scoped_id)

def label_remove_url(label, ticket):
    """Return the URL to remove a label from a ticket."""
    return url_for("ticket.ticket_remove_label",
            owner=ticket.tracker.owner.canonical_name,
            name=ticket.tracker.name,
            ticket_id=ticket.scoped_id,
            label_id=label.id)

def participant_url(participant):
    if participant.participant_type == ParticipantType.user:
        return url_for("html.user_GET", username=participant.user.username)
    elif participant.participant_type == ParticipantType.email:
        if participant.email_name:
            return f"mailto:{participant.email_name} <{participant.email}>"
        else:
            return f"mailto:{participant.email}"
    elif participant.participant_type == ParticipantType.external:
        return participant.external_url

def user_url(user):
    return url_for("html.user_GET", username=user.username)
