import json
from collections import OrderedDict
from srht.app import date_handler
from srht.config import get_origin
from srht.crypto import sign_payload
from todosrht.types import Event, EventType, Ticket, ParticipantType

def tracker_export(tracker):
    """
    Exports a tracker as a JSON string.
    """
    dump = list()
    tickets = Ticket.query.filter(Ticket.tracker_id == tracker.id).all()
    for ticket in tickets:
        td = ticket.to_dict()
        td["upstream"] = get_origin("todo.sr.ht", external=True)
        if ticket.submitter.participant_type == ParticipantType.user:
            sigdata = OrderedDict({
                "description": ticket.description,
                "ref": ticket.ref(),
                "submitter": ticket.submitter.user.canonical_name,
                "title": ticket.title,
                "upstream": get_origin("todo.sr.ht", external=True),
            })
            sigdata = json.dumps(sigdata)
            signature = sign_payload(sigdata)
            td.update(signature)

        events = Event.query.filter(Event.ticket_id == ticket.id).all()
        if any(events):
            td["events"] = list()
        for event in events:
            ev = event.to_dict()
            ev["upstream"] = get_origin("todo.sr.ht", external=True)
            if (EventType.comment in event.event_type
                    and event.participant.participant_type == ParticipantType.user):
                sigdata = OrderedDict({
                    "comment": event.comment.text,
                    "id": event.id,
                    "ticket": event.ticket.ref(),
                    "user": event.participant.user.canonical_name,
                    "upstream": get_origin("todo.sr.ht", external=True),
                })
                sigdata = json.dumps(sigdata)
                signature = sign_payload(sigdata)
                ev.update(signature)
            td["events"].append(ev)
        dump.append(td)

    dump = json.dumps({
        "owner": tracker.owner.to_dict(),
        "name": tracker.name,
        "labels": [l.to_dict() for l in tracker.labels],
        "tickets": dump,
    }, default=date_handler)
    return dump
