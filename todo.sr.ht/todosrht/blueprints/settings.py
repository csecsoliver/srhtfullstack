import gzip
import json
import os
from collections import OrderedDict
from flask import Blueprint, render_template, request, url_for, abort, redirect
from flask import current_app, send_file
from srht.app import date_handler, session
from srht.config import get_origin
from srht.crypto import sign_payload
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.validation import Validation
from tempfile import NamedTemporaryFile
from todosrht.access import get_tracker, get_tracker_or_redir
from todosrht.trackers import get_recent_users
from todosrht.graphql import Client, TrackerInput, ACLInput, Upload, Visibility
from todosrht.types import Event, EventType, Ticket, TicketAccess
from todosrht.types import ParticipantType, UserAccess, User
from todosrht.urls import tracker_url

settings = Blueprint("settings", __name__)

def parse_html_perms(short, valid):
    result = 0
    for sub_perm in TicketAccess:
        new_perm = valid.optional("perm_{}_{}".format(short, sub_perm.name))
        if new_perm:
            result |= int(new_perm)
    return result

access_help_map={
    TicketAccess.BROWSE:
        "Permission to view tickets",
    TicketAccess.SUBMIT:
        "Permission to submit tickets",
    TicketAccess.COMMENT:
        "Permission to comment on tickets",
    TicketAccess.EDIT:
        "Permission to edit tickets",
    TicketAccess.TRIAGE:
        "Permission to resolve, re-open, or label tickets",
}

@settings.route("/<owner>/<name>/settings/details")
@loginrequired
def details_GET(owner, name):
    tracker, access = get_tracker_or_redir(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)
    return render_template("tracker-details.html",
        view="details", tracker=tracker)

@settings.route("/<owner>/<name>/settings/details", methods=["POST"])
@loginrequired
def details_POST(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    valid = Validation(request)
    desc = valid.optional("description")
    visibility = valid.require("visibility", cls=Visibility)

    with valid:
        update = TrackerInput(visibility=visibility, description=desc)
        result = Client().update_tracker(tracker.id, update).tracker

    if not valid.ok:
        return render_template("tracker-details.html",
            tracker=tracker, **valid.kwargs), 400

    return redirect(url_for("settings.details_GET",
        owner=result.owner.canonical_name, name=result.name))


@settings.route("/<owner>/<name>/settings/rename")
@loginrequired
def settings_rename(owner, name):
    tracker, access = get_tracker_or_redir(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)
    return render_template("settings_rename.html", owner=owner, tracker=tracker)

@settings.route("/<owner>/<name>/settings/rename", methods=["POST"])
@loginrequired
def settings_rename_POST(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    valid = Validation(request)
    name = valid.require("name", friendly_name="Name")
    if not valid.ok:
        return render_template("settings_rename.html", owner=owner, tracker=tracker,
                **valid.kwargs)

    with valid:
        update = TrackerInput(name=name)
        result = Client().update_tracker(tracker.id, update).tracker

    if not valid.ok:
        return render_template("settings_rename.html",
            owner=owner, tracker=tracker, **valid.kwargs)

    return redirect(url_for("tracker.tracker_GET",
        owner=owner, name=result.name))


def render_tracker_access(tracker, **kwargs):
    recent_users = get_recent_users(tracker)
    return render_template("tracker-access.html",
        view="access", tracker=tracker, access_type_list=TicketAccess,
        access_help_map=access_help_map, recent_users=recent_users, **kwargs)


@settings.route("/<owner>/<name>/settings/access")
@loginrequired
def access_GET(owner, name):
    tracker, access = get_tracker_or_redir(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)
    return render_tracker_access(tracker)

@settings.route("/<owner>/<name>/settings/access", methods=["POST"])
@loginrequired
def access_POST(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    valid = Validation(request)
    access = parse_html_perms('default', valid)
    acl_in = ACLInput(
        browse=(access & TicketAccess.BROWSE) != 0,
        submit=(access & TicketAccess.SUBMIT) != 0,
        comment=(access & TicketAccess.COMMENT) != 0,
        edit=(access & TicketAccess.EDIT) != 0,
        triage=(access & TicketAccess.TRIAGE) != 0)

    with valid:
        Client().update_tracker_access(tracker.id, acl_in)

    if not valid.ok:
        return render_tracker_access(tracker, **valid.kwargs), 400

    return redirect(tracker_url(tracker))

@settings.route("/<owner>/<name>/settings/user-access/create", methods=["POST"])
@loginrequired
def user_access_create_POST(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    valid = Validation(request)
    username = valid.require("username")
    permissions = parse_html_perms("user_access", valid)
    if not valid.ok:
        return render_tracker_access(tracker, **valid.kwargs), 400

    username = username.lstrip("~")
    user = current_app.oauth_service.lookup_user(username)
    valid.expect(user, "User not found.", field="username")
    if not valid.ok:
        return render_tracker_access(tracker, **valid.kwargs), 400

    existing = UserAccess.query.filter_by(user=user, tracker=tracker).count()

    valid.expect(user != tracker.owner,
        "Cannot override tracker owner's permissions.", field="username")
    valid.expect(existing == 0,
        "This user already has custom permissions assigned.", field="username")
    if not valid.ok:
        return render_tracker_access(tracker, **valid.kwargs), 400

    ua = UserAccess(tracker=tracker, user_id=user.id, permissions=permissions)
    db.session.add(ua)
    db.session.commit()

    return redirect(url_for("settings.access_GET",
            owner=tracker.owner.canonical_name,
            name=name))

@settings.route("/<owner>/<name>/settings/user-access/<user_id>/delete",
    methods=["POST"])
@loginrequired
def user_access_delete_POST(owner, name, user_id):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    UserAccess.query.filter_by(user_id=user_id, tracker_id=tracker.id).delete()
    db.session.commit()

    return redirect(url_for("settings.access_GET",
            owner=tracker.owner.canonical_name,
            name=name))

@settings.route("/<owner>/<name>/settings/delete")
@loginrequired
def delete_GET(owner, name):
    tracker, access = get_tracker_or_redir(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)
    return render_template("tracker-delete.html",
        view="delete", tracker=tracker)

@settings.route("/<owner>/<name>/settings/delete", methods=["POST"])
@loginrequired
def delete_POST(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    tracker = Client().delete_tracker(tracker.id).tracker
    session["notice"] = f'"{tracker.name}" was deleted successfully.'
    return redirect(url_for("html.index_GET"))

@settings.route("/<owner>/<name>/settings/import-export")
@loginrequired
def import_export_GET(owner, name):
    tracker, access = get_tracker_or_redir(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)
    return render_template("tracker-import-export.html",
        view="import/export", tracker=tracker)

@settings.route("/<owner>/<name>/settings/export", methods=["POST"])
@loginrequired
def export_POST(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    upstream = get_origin("todo.sr.ht", external=True)

    def participant_to_dict(self):
        if self.participant_type == ParticipantType.user:
            return {
                "type": "user",
                "user_id": self.user.id,
                "canonical_name": self.user.canonical_name,
                "name": self.user.username,
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

    dump = list()
    tickets = Ticket.query.filter(Ticket.tracker_id == tracker.id).all()
    for ticket in tickets:
        td = {
            "id": ticket.scoped_id,
            "created": ticket.created,
            "updated": ticket.updated,
            "submitter": participant_to_dict(ticket.submitter),
            "ref": ticket.ref(),
            "subject": ticket.title,
            "body": ticket.description,
            "status": ticket.status.name.upper(),
            "resolution": ticket.resolution.name.upper(),
            "labels": [l.name for l in ticket.labels],
            "assignees": [u.to_dict(short=True) for u in ticket.assigned_users],
        }
        td["upstream"] = upstream
        if ticket.submitter.participant_type == ParticipantType.user:
            sigdata = OrderedDict({
                "tracker_id": tracker.id,
                "ticket_id": ticket.scoped_id,
                "subject": ticket.title,
                "body": ticket.description,
                "submitter_id": ticket.submitter.user.id,
                "upstream": upstream,
            })
            sigdata = json.dumps(sigdata, separators=(',',':'))
            signature = sign_payload(sigdata)
            td.update(signature)

        events = Event.query.filter(Event.ticket_id == ticket.id).all()
        if any(events):
            td["events"] = list()
        for event in events:
            ev = {
                "id": event.id,
                "created": event.created,
                "event_type": [t.name.upper() for t in EventType if t in event.event_type],
                "old_status": event.old_status.name.upper()
                    if event.old_status else None,
                "old_resolution": event.old_resolution.name.upper()
                    if event.old_resolution else None,
                "new_status": event.new_status.name.upper()
                    if event.new_status else None,
                "new_resolution": event.new_resolution.name.upper()
                    if event.new_resolution else None,
                "participant": participant_to_dict(event.participant)
                    if event.participant else None,
                "ticket_id": event.ticket.scoped_id
                        if event.ticket else None,
                "comment": {
                    "id": event.comment.id,
                    "created": event.comment.created,
                    "author": participant_to_dict(event.comment.submitter),
                    "text": event.comment.text,
                } if event.comment else None,
                "label": event.label.name if event.label else None,
                "by_user": participant_to_dict(event.by_participant)
                    if event.by_participant else None,
                "from_ticket_id": event.from_ticket.scoped_id
                    if event.from_ticket else None,
            }
            ev["upstream"] = upstream
            if (EventType.COMMENT in event.event_type
                    and event.participant.participant_type == ParticipantType.user):
                sigdata = OrderedDict({
                    "tracker_id": tracker.id,
                    "ticket_id": ticket.scoped_id,
                    "comment": event.comment.text,
                    "author_id": event.comment.submitter.user.id,
                    "upstream": upstream
                })
                sigdata = json.dumps(sigdata, separators=(',',':'))
                signature = sign_payload(sigdata)
                ev.update(signature)
            td["events"].append(ev)
        dump.append(td)

    dump = json.dumps({
        "id": tracker.id,
        "owner": tracker.owner.to_dict(short=True),
        "created": tracker.created,
        "updated": tracker.updated,
        "name": tracker.name,
        "description": tracker.description,
        "labels": [{
            "id": l.id,
            "created": l.created,
            "name": l.name,
            "background_color": l.color,
            "foreground_color": l.text_color,
        } for l in tracker.labels],
        "tickets": dump,
    }, default=date_handler)
    with NamedTemporaryFile() as ntf:
        ntf.write(gzip.compress(dump.encode()))
        f = open(ntf.name, "rb")

    return send_file(f, as_attachment=True,
            download_name=f"{tracker.owner.username}-{tracker.name}.json.gz",
            mimetype="application/gzip")

@settings.route("/<owner>/<name>/settings/import", methods=["POST"])
@loginrequired
def import_POST(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    valid = Validation(request)
    dump = request.files.get("dump")
    valid.expect(dump is not None,
            "Tracker dump file is required", field="dump")

    if not valid.ok:
        return render_template("tracker-import-export.html",
            view="import/export", tracker=tracker, **valid.kwargs)

    upload = Upload(dump.filename, dump, "application/octet-stream")
    with valid:
        Client().import_tracker_dump(tracker.id, upload)

    if not valid.ok:
        return render_template("tracker-import-export.html",
            view="import/export", tracker=tracker, **valid.kwargs)

    return redirect(tracker_url(tracker))
