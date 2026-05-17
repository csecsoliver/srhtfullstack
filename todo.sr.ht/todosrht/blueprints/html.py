from flask import Blueprint, render_template, request, abort, redirect, url_for
from todosrht.access import get_tracker, get_access
from todosrht.graphql import Visibility
from todosrht.tickets import get_participant_for_user
from todosrht.types import Event, EventNotification, EventType
from todosrht.types import Tracker, Ticket, TicketAccess, User, Participant
from srht.app import paginate_query, session, get_profile
from srht.config import cfg
from srht.database import db
from srht.oauth import current_user, loginrequired, UserType
from srht.validation import Validation
from sqlalchemy import and_, or_

html = Blueprint('html', __name__)

def filter_authorized_events(events):
    events = (events
        .join(Ticket, Ticket.id == Event.ticket_id)
        .join(Tracker, Tracker.id == Ticket.tracker_id))
    # TODO: Filter based on user ACLs?
    events = (events.filter(and_(
            Tracker.visibility == Visibility.PUBLIC,
            Tracker.default_access.op('&')(TicketAccess.BROWSE) > 0)))
    return events

@html.route("/")
def index_GET():
    if not current_user:
        return render_template("index.html")
    trackers = (Tracker.query
        .filter(Tracker.owner_id == current_user.id)
        .order_by(Tracker.updated.desc())
    )
    limit_trackers = 10
    total_trackers = trackers.count()
    trackers = trackers.limit(limit_trackers).all()

    events = (Event.query
            .join(EventNotification)
            .filter(EventNotification.user_id == current_user.id)
            .order_by(Event.created.desc()))
    events = events.limit(10).all()

    notice = session.pop("notice", None)
    prefs_updated = session.pop("prefs_updated", None)

    return render_template("dashboard.html",
        trackers=trackers, notice=notice,
        tracker_list_msg="Your Trackers",
        more_trackers=total_trackers > limit_trackers,
        events=events, EventType=EventType,
        prefs_updated=prefs_updated)

@html.route("/", methods=["POST"])
@loginrequired
def index_POST():
    valid = Validation(request)
    notify_self = valid.require("notify-self")
    current_user.notify_self = notify_self == "on"
    db.session.commit()
    session["prefs_updated"] = True
    return redirect(url_for("html.index_GET"))

@html.route("/~<username>")
def user_GET(username):
    user = User.query.filter(User.username == username.lower()).one_or_none()
    if not user:
        abort(404)

    trackers = Tracker.query.filter(Tracker.owner_id == user.id)
    if not current_user or user.id != current_user.id:
        trackers = trackers.filter(Tracker.visibility == Visibility.PUBLIC)

    search = request.args.get("search")
    if search:
        trackers = trackers.filter(or_(
            Tracker.name.ilike("%" + search + "%")))

    trackers = trackers.order_by(Tracker.updated.desc())
    trackers, pagination = paginate_query(trackers)

    return render_template("profile-todo.html",
            user=user, trackers=trackers, search=search,
            profile=get_profile(user), view="todo",
            **pagination)

# Deprecated route
@html.route("/trackers/~<username>")
def trackers_for_user(username):
    return redirect(url_for(".user_GET", username=username))
