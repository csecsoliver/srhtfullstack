from flask import Blueprint, render_template, request, url_for, abort, redirect
from srht.app import paginate_query, session
from srht.config import cfg
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.validation import Validation
from todosrht.access import get_tracker, get_tracker_or_redir, get_ticket
from todosrht.color import color_from_hex, color_to_hex, get_text_color
from todosrht.color import valid_hex_color_code
from todosrht.filters import render_markup
from todosrht.graphql import Client, SubmitTicketInput, UpdateLabelInput
from todosrht.graphql import GraphQLClientError
from todosrht.search import apply_search
from todosrht.tickets import get_participant_for_user
from todosrht.types import Label, TicketSubscription, Participant
from todosrht.types import Ticket, TicketAccess
from todosrht.urls import tracker_url, ticket_url
from urllib.parse import quote
import sqlalchemy as sa

tracker = Blueprint("tracker", __name__)

posting_domain = cfg("todo.sr.ht::mail", "posting-domain")
tracker_subscribe_body = """\
Sending this email will subscribe your email address to {tracker_ref},
in so doing you will start receiving new tickets and all comments for this tracker.

You can unsubscribe at any time by mailing <{tracker_ref}/unsubscribe@""" + \
    posting_domain + ">.\n"

@tracker.route("/tracker/create")
@loginrequired
def create_GET():
    return render_template("tracker-create.html")

@tracker.route("/tracker/create", methods=["POST"])
@loginrequired
def create_POST():
    valid = Validation(request)
    name = valid.require("name", friendly_name="Name")
    visibility = valid.require("visibility")
    desc = valid.optional("description")
    if not valid.ok:
        return render_template("tracker-create.html", **valid.kwargs), 400

    with valid:
        tracker = Client().create_tracker(name, desc, visibility).tracker

    if not valid.ok:
        return render_template("tracker-create.html", **valid.kwargs), 400

    if "create-configure" in valid:
        return redirect(url_for("settings.details_GET",
                owner=current_user.canonical_name,
                name=tracker.name))

    return redirect(url_for("tracker.tracker_GET",
        owner=tracker.owner.canonical_name, name=tracker.name))

def return_tracker(tracker, access, **kwargs):
    another = session.get("another") or False
    if another:
        del session["another"]
    is_subscribed = False
    tracker_subscribe = None
    if current_user:
        sub = (TicketSubscription.query
            .join(Participant)
            .filter(TicketSubscription.tracker_id == tracker.id)
            .filter(TicketSubscription.ticket_id == None)
            .filter(Participant.user_id == current_user.id)
        ).one_or_none()
        is_subscribed = bool(sub)
    else:
        subj = quote("Subscribing to " + tracker.ref())
        tracker_subscribe = f"mailto:{tracker.ref()}/subscribe@" + \
            f"{posting_domain}?subject={subj}&body=" + \
            quote(tracker_subscribe_body.format(tracker_ref=tracker.ref()))

    if TicketAccess.BROWSE in access:
        tickets = Ticket.query.filter(Ticket.tracker_id == tracker.id)
    elif current_user:
        tickets = (Ticket.query
                .join(Participant, Participant.user_id == current_user.id)
                .filter(Ticket.tracker_id == tracker.id)
                .filter(Ticket.submitter_id == Participant.id))
    else:
        tickets = Ticket.query.filter(False)

    try:
        terms = request.args.get("search")
        tickets = apply_search(tickets, terms, current_user)
    except ValueError as e:
        kwargs["search_error"] = str(e)

    tickets = tickets.options(sa.orm.joinedload(Ticket.submitter))

    tickets, pagination = paginate_query(tickets, results_per_page=25)

    if "another" in kwargs:
        another = kwargs["another"]
        del kwargs["another"]

    return render_template("tracker.html",
            tracker=tracker, another=another, tickets=tickets,
            access=access, is_subscribed=is_subscribed, search=terms,
            tracker_subscribe=tracker_subscribe, **pagination, **kwargs)

@tracker.route("/<owner>/<name>")
def tracker_GET(owner, name):
    tracker, access = get_tracker_or_redir(owner, name)
    if not tracker:
        abort(404)

    # Populate title and description if given as URL parameters
    kwargs = {
        "title": request.args.get("title"),
        "description": request.args.get("description"),
        "notice": session.pop("notice", None),
    }

    return return_tracker(tracker, access, **kwargs)

@tracker.route("/<owner>/<name>/enable_notifications", methods=["POST"])
@loginrequired
def enable_notifications(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)

    participant = get_participant_for_user(current_user)
    sub = (TicketSubscription.query
        .filter(TicketSubscription.tracker_id == tracker.id)
        .filter(TicketSubscription.ticket_id == None)
        .filter(TicketSubscription.participant_id == participant.id)
    ).one_or_none()

    if sub:
        return redirect(tracker_url(tracker))

    sub = TicketSubscription()
    sub.tracker_id = tracker.id
    sub.participant_id = participant.id
    db.session.add(sub)
    db.session.commit()
    return redirect(tracker_url(tracker))

@tracker.route("/<owner>/<name>/disable_notifications", methods=["POST"])
@loginrequired
def disable_notifications(owner, name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)

    participant = get_participant_for_user(current_user)
    sub = (TicketSubscription.query
        .filter(TicketSubscription.tracker_id == tracker.id)
        .filter(TicketSubscription.ticket_id == None)
        .filter(TicketSubscription.participant_id == participant.id)
    ).one_or_none()

    if not sub:
        return redirect(tracker_url(tracker))

    db.session.delete(sub)
    db.session.commit()
    return redirect(tracker_url(tracker))

@tracker.route("/<owner>/<name>", methods=["POST"])
@loginrequired
def tracker_submit_POST(owner, name):
    tracker, access = get_tracker(owner, name, True)
    if not tracker:
        abort(404)
    if not TicketAccess.SUBMIT in access:
        abort(403)

    db.session.commit() # Unlock tracker row

    valid = Validation(request)
    subject = valid.require("title", friendly_name="Title")
    body = valid.optional("description")
    another = valid.optional("another")

    if not valid.ok:
        return return_tracker(tracker, access, **valid.kwargs), 400

    if "preview" in request.form:
        preview = render_markup(tracker, body or "")
        return return_tracker(tracker, access,
                rendered_preview=preview, **valid.kwargs), 200

    with valid:
        details = SubmitTicketInput(subject=subject, body=body)
        ticket = Client().submit_ticket(tracker.id, details).ticket

    if not valid.ok:
        return return_tracker(tracker, access, **valid.kwargs), 400

    if another:
        session["another"] = True
        return redirect(url_for(".tracker_GET",
                owner=tracker.owner.canonical_name,
                name=name))

    return redirect(ticket_url(ticket))

@tracker.route("/<owner>/<name>/labels")
def tracker_labels_GET(owner, name):
    tracker, access = get_tracker_or_redir(owner, name)
    if not tracker:
        abort(404)

    is_owner = current_user and current_user.id == tracker.owner_id
    return render_template("tracker-labels.html",
        tracker=tracker, access=access, is_owner=is_owner)

def validate_label(request):
    valid = Validation(request)
    name = valid.require("name")
    color = valid.require("color")
    if not valid.ok:
        return None, valid

    valid.expect(2 <= len(name) <= 50,
            "Must be between 2 and 50 characters", field="name")
    valid.expect(valid_hex_color_code(color),
            "Invalid hex color code", field="color")
    if not valid.ok:
        return None, valid

    # Determine a foreground color to use
    color_rgb = color_from_hex(color)
    text_color_rgb = get_text_color(color_rgb)
    text_color = color_to_hex(text_color_rgb)

    label = dict(name=name, color=color, text_color=text_color)
    return label, valid


@tracker.route("/<owner>/<name>/labels", methods=["POST"])
@loginrequired
def tracker_labels_POST(owner, name):
    tracker, access = get_tracker(owner, name)
    is_owner = current_user.id == tracker.owner_id
    if not tracker:
        abort(404)
    if not is_owner:
        abort(403)

    data, valid = validate_label(request)
    if not valid.ok:
        return render_template("tracker-labels.html",
            tracker=tracker, access=access, is_owner=is_owner,
            **valid.kwargs), 400

    with valid:
        Client().create_label(
                tracker_id=tracker.id,
                name=data["name"],
                foreground_color=data["text_color"],
                background_color=data["color"],
        )

    if not valid.ok:
        return render_template("tracker-labels.html",
            tracker=tracker, access=access, is_owner=is_owner,
            **valid.kwargs), 400

    return redirect(url_for(".tracker_labels_GET", owner=owner, name=name))

@tracker.route("/<owner>/<name>/labels/<path:label_name>")
@loginrequired
def label_edit_GET(owner, name, label_name):
    tracker, access = get_tracker_or_redir(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    label = Label.query.filter_by(tracker=tracker, name=label_name).first()
    if not label:
        abort(404)

    return render_template("tracker-label-edit.html",
        tracker=tracker, access=access, label=label)

@tracker.route("/<owner>/<name>/labels/<path:label_name>", methods=["POST"])
@loginrequired
def label_edit_POST(owner, name, label_name):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    label = Label.query.filter_by(
        tracker=tracker, name=label_name).one_or_none()
    if not label:
        abort(404)

    data, valid = validate_label(request)
    if not valid.ok:
        return render_template("tracker-label-edit.html",
            tracker=tracker, access=access, label=label, **valid.kwargs), 400

    update = UpdateLabelInput(
        name=data["name"],
        foreground_color=data["text_color"],
        background_color=data["color"],
    )
    with valid:
        Client().update_label(label.id, update)

    if not valid.ok:
        return render_template("tracker-label-edit.html",
            tracker=tracker, access=access, **valid.kwargs), 400

    return redirect(url_for(".tracker_labels_GET", owner=owner, name=name))

@tracker.route("/<owner>/<name>/labels/<int:label_id>/delete", methods=["POST"])
@loginrequired
def delete_label(owner, name, label_id):
    tracker, access = get_tracker(owner, name)
    if not tracker:
        abort(404)
    if current_user.id != tracker.owner_id:
        abort(403)

    try:
        Client().delete_label(label_id)
    except GraphQLClientError:
        abort(404)

    return redirect(url_for(".tracker_labels_GET", owner=owner, name=name))
