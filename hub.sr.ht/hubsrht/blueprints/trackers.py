from flask import Blueprint, render_template, request, redirect, url_for
from flask import abort
from hubsrht.projects import ProjectAccess, get_project, get_project_or_redir
from hubsrht.services.todo import TodoClient, Visibility as TrackerVisibility
from hubsrht.types import Event, EventType, Tracker, Visibility
from hubsrht.types.eventprojectassoc import EventProjectAssociation
from hubsrht.webhooks import get_user_webhooks
from srht.app import paginate_query
from srht.config import get_origin
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.search import search_by
from srht.validation import Validation

trackers = Blueprint("trackers", __name__)

TODO_WEBHOOK_VERSION = 2

@trackers.route("/<owner>/<project_name>/trackers")
def trackers_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.read)
    trackers = (Tracker.query
            .filter(Tracker.project_id == project.id)
            .order_by(Tracker.updated.desc()))
    if not current_user or current_user.id != owner.id:
        trackers = trackers.filter(Tracker.visibility == Visibility.PUBLIC)

    terms = request.args.get("search")
    search_error = None
    try:
        trackers = search_by(trackers, terms,
                [Tracker.name, Tracker.description])
    except ValueError as ex:
        search_error = str(ex)

    trackers, pagination = paginate_query(trackers)
    return render_template("trackers.html", view="tickets",
            owner=owner, project=project, trackers=trackers,
            search=terms, search_error=search_error,
            **pagination)

def get_trackers(owner, project):
    client = TodoClient()

    # TODO: Pagination
    cursor = None
    trackers = []
    while True:
        batch = client.get_trackers(cursor).me.trackers
        trackers.extend(batch.results)
        cursor = batch.cursor
        if not cursor:
            break

    trackers = sorted(trackers, key=lambda r: r.updated, reverse=True)
    existing = [t.remote_id for t in (Tracker.query
            .filter(Tracker.project_id == project.id)).all()]
    return trackers, existing

@trackers.route("/<owner>/<project_name>/trackers/new")
@loginrequired
def new_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    trackers, existing = get_trackers(owner, project)
    return render_template("tracker-new.html", view="new-resource",
            owner=owner, project=project, trackers=trackers, existing=existing)

@trackers.route("/<owner>/<project_name>/trackers/new", methods=["POST"])
@loginrequired
def new_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)

    todo_client = TodoClient()
    valid = Validation(request)
    visibility = TrackerVisibility(project.visibility.value)

    if "create" in valid:
        with valid:
            remote_tracker = todo_client.create_tracker(
                    name=valid.require("name"),
                    description=valid.optional("description"),
                    visibility=visibility,
            ).tracker
        if not valid.ok:
            trackers, existing = get_trackers(owner, project)
            return render_template("tracker-new.html",
                    view="new-resource", owner=owner, project=project,
                    trackers=trackers, existing=existing, **valid.kwargs)
    else:
        tracker_rid = None
        for field in valid.source:
            if field.startswith("existing-"):
                tracker_rid = field[len("existing-"):]
                break

        if not tracker_rid:
            search = valid.optional("search")
            trackers, existing = get_trackers(owner, project)
            trackers = filter(lambda r: search.lower() in r.name.lower(), trackers)
            return render_template("tracker-new.html",
                    view="new-resource", owner=owner, project=project,
                    trackers=trackers, existing=existing, **valid.kwargs)

        remote_tracker = todo_client.get_tracker(tracker_rid).tracker

    tracker = Tracker()
    tracker.remote_id = remote_tracker.id
    tracker.remote_rid = remote_tracker.rid
    tracker.project_id = project.id
    tracker.owner_id = owner.id
    tracker.name = remote_tracker.name
    tracker.description = remote_tracker.description
    tracker.visibility = Visibility(remote_tracker.visibility.value)
    tracker.webhook_id = -1
    tracker.webhook_version = TODO_WEBHOOK_VERSION
    db.session.add(tracker)
    db.session.flush()

    webhook_url = (get_origin("hub.sr.ht", external=False) +
           url_for("webhooks.todo_tracker", tracker_id=tracker.id))
    tracker.webhook_id = todo_client.create_tracker_webhook(
            tracker_id=tracker.remote_id,
            payload=TodoClient.event_webhook_query,
            url=webhook_url).webhook.id
    tracker.webhook_version = TODO_WEBHOOK_VERSION

    uwh = get_user_webhooks(owner)
    if uwh.todo_webhook_id is None:
        user_webhook_url = (get_origin("hub.sr.ht", external=False) +
               url_for("webhooks.todo_user", user_id=owner.id))
        uwh.todo_webhook_id = todo_client.create_user_webhook(
                payload=TodoClient.event_webhook_query,
                url=user_webhook_url).webhook.id
        uwh.todo_webhook_version = TODO_WEBHOOK_VERSION

    event = Event()
    event.event_type = EventType.tracker_added
    event.tracker_id = tracker.id
    event.user_id = project.owner_id
    db.session.add(event)
    db.session.flush()

    assoc = EventProjectAssociation()
    assoc.event_id = event.id
    assoc.project_id = project.id
    db.session.add(assoc)

    db.session.commit()

    return redirect(url_for("projects.summary_GET",
        owner=owner.canonical_name, project_name=project.name))

@trackers.route("/<owner>/<project_name>/trackers/manage")
@loginrequired
def manage_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    trackers = (Tracker.query
            .filter(Tracker.project_id == project.id)
            .order_by(Tracker.updated.desc()))

    terms = request.args.get("search")
    search_error = None
    try:
        trackers = search_by(trackers, terms,
                [Tracker.name, Tracker.description])
    except ValueError as ex:
        search_error = str(ex)

    trackers, pagination = paginate_query(trackers)
    return render_template("trackers-manage.html", view="tickets",
            owner=owner, project=project, trackers=trackers,
            search=terms, search_error=search_error,
            **pagination)

@trackers.route("/<owner>/<project_name>/trackers/delete/<int:tracker_id>")
@loginrequired
def delete_GET(owner, project_name, tracker_id):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    tracker = (Tracker.query
        .filter(Tracker.id == tracker_id)
        .filter(Tracker.project_id == project.id)).one_or_none()
    if not tracker:
        abort(404)
    return render_template("resource-delete.html", view="tickets",
            owner=owner, project=project, resource=tracker,
            resource_type="ticket tracker")

@trackers.route("/<owner>/<project_name>/trackers/delete/<int:tracker_id>",
        methods=["POST"])
@loginrequired
def delete_POST(owner, project_name, tracker_id):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)
    tracker = (Tracker.query
        .filter(Tracker.id == tracker_id)
        .filter(Tracker.project_id == project.id)).one_or_none()
    if not tracker:
        abort(404)
    remote_id = tracker.remote_id
    db.session.delete(tracker)
    db.session.commit()

    client = TodoClient()
    client.delete_tracker_webhook(tracker.webhook_id)

    valid = Validation(request)
    delete_remote = valid.optional("delete-remote") == "on"
    if delete_remote:
        client.delete_tracker(remote_id)

    return redirect(url_for("projects.summary_GET",
        owner=owner.canonical_name, project_name=project.name))
