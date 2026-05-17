import json
from flask import Blueprint, render_template, request, redirect, url_for, abort
from hubsrht.projects import ProjectAccess, get_project, get_project_or_redir
from hubsrht.services.lists import ListsClient, Visibility as ListVisibility
from hubsrht.types import Event, EventType
from hubsrht.types import MailingList, Visibility
from hubsrht.types.eventprojectassoc import EventProjectAssociation
from hubsrht.webhooks import get_user_webhooks
from srht.app import paginate_query
from srht.config import get_origin
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.search import search_by
from srht.validation import Validation

mailing_lists = Blueprint("mailing_lists", __name__)

LIST_WEBHOOK_VERSION = 6

def get_user_lists(project, client, search=None):
    # TODO: Pagination
    cursor = None
    lists = []
    while True:
        batch = client.get_lists(cursor).me.lists
        lists.extend(batch.results)
        cursor = batch.cursor
        if not cursor:
            break

    lists = sorted(lists, key=lambda r: r.updated, reverse=True)
    existing = [l.remote_id for l in (MailingList.query
            .filter(MailingList.project_id == project.id)).all()]

    if search:
        # TODO: Better searching
        lists = [l for l in lists if search.lower() in l.name.lower()]

    return (lists, existing)

@mailing_lists.route("/<owner>/<project_name>/lists")
def lists_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.read)
    mailing_lists = (MailingList.query
            .filter(MailingList.project_id == project.id)
            .order_by(MailingList.updated.desc()))
    if not current_user or current_user.id != owner.id:
        mailing_lists = mailing_lists.filter(
                MailingList.visibility == Visibility.PUBLIC)

    terms = request.args.get("search")
    search_error = None
    try:
        mailing_lists = search_by(mailing_lists, terms,
                [MailingList.name, MailingList.description])
    except ValueError as ex:
        search_error = str(ex)

    mailing_lists, pagination = paginate_query(mailing_lists)
    return render_template("mailing-lists.html", view="mailing lists",
            owner=owner, project=project,
            search=terms, search_error=search_error,
            mailing_lists=mailing_lists,
            **pagination)

@mailing_lists.route("/<owner>/<project_name>/lists/new")
@loginrequired
def new_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    client = ListsClient()
    lists, existing = get_user_lists(project, client)
    return render_template("mailing-list-new.html", view="new-resource",
            owner=owner, project=project, lists=lists, existing=existing)

def finalize_add_list(client, owner, project, mailing_list):
    ml = MailingList()
    ml.remote_id = mailing_list.id
    ml.remote_rid = mailing_list.rid
    ml.project_id = project.id
    ml.owner_id = project.owner_id
    ml.name = mailing_list.name
    ml.webhook_id = -1
    ml.webhook_version = LIST_WEBHOOK_VERSION
    ml.description = mailing_list.description
    ml.visibility = Visibility(mailing_list.visibility.value)
    db.session.add(ml)
    db.session.flush()

    webhook_url = (get_origin("hub.sr.ht", external=False) +
           url_for("webhooks.project_mailing_list", list_id=ml.id))
    ml.webhook_id = client.create_list_webhook(
            list_id=ml.remote_id,
            payload=ListsClient.event_webhook_query,
            url=webhook_url).webhook.id

    uwh = get_user_webhooks(owner)
    if uwh.lists_webhook_id is None:
        user_webhook_url = (get_origin("hub.sr.ht", external=False) +
               url_for("webhooks.mailing_list_user", user_id=owner.id))
        uwh.lists_webhook_id = client.create_user_webhook(
                payload=ListsClient.event_webhook_query,
                url=user_webhook_url).webhook.id
        uwh.lists_webhook_version = LIST_WEBHOOK_VERSION

    event = Event()
    event.event_type = EventType.mailing_list_added 
    event.mailing_list_id = ml.id
    event.user_id = project.owner_id
    db.session.add(event)
    db.session.flush()

    assoc = EventProjectAssociation()
    assoc.event_id = event.id
    assoc.project_id = project.id
    db.session.add(assoc)

    db.session.commit()

def lists_from_template(owner, project, template):
    project_url = url_for("projects.summary_GET",
        owner=owner.canonical_name, project_name=project.name)
    project_url = get_origin("hub.sr.ht", external=True) + project_url
    templates = {
        "public-inbox": ["public-inbox"],
        "announce-devel": [f"{project.name}-announce", f"{project.name}-devel"],
        "announce-devel-discuss": [
            f"{project.name}-announce",
            f"{project.name}-devel",
            f"{project.name}-discuss", 
        ],
    }
    descs = {
        "public-inbox": f"""
General catch-all for patches, questions, and discussions for any of
{current_user.canonical_name}'s projects which do not have their own mailing
list.

When posting patches to this list, please edit the [PATCH] line to include the
specific project you're contributing to, e.g.

    [PATCH {project.name} v2] Add thing to stuff
""",
        f"{project.name}-announce": f"""
Low-volume mailing list for announcements related to the
[{project.name}]({project_url}) project.
""",
        f"{project.name}-devel": f"""
Mailing list for development discussion and patches related to the
[{project.name}]({project_url}) project. For help sending patches to this
list, please consult [git-send-email.io](https://git-send-email.io).
""",
        f"{project.name}-discuss": f"""
Mailing list for end-user discussion and questions related to the
[{project.name}]({project_url}) project.
""",
    }
    template = templates[template]

    client = ListsClient()

    for list_name in template:
        desc = descs[list_name]
        list_name = list_name.lower() # Per lists.sr.ht naming rules
        mailing_list = client.get_list_by_name(list_name).me.mailing_list
        if mailing_list is not None:
            in_project = [l.remote_id for l in (MailingList.query
                    .filter(MailingList.project_id == project.id)
                    .filter(MailingList.name == list_name)).all()]
            if in_project:
                continue
        else:
            mailing_list = client.create_list(
                name=list_name,
                description=desc,
                visibility=ListVisibility(project.visibility.value)
            ).mailing_list
        finalize_add_list(client, owner, project, mailing_list)

    return redirect(project_url)

@mailing_lists.route("/<owner>/<project_name>/lists/new", methods=["POST"])
@loginrequired
def new_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)

    client = ListsClient()
    valid = Validation(request)

    if "from-template" in valid:
        template = valid.require("template")
        valid.expect(template in ["public-inbox",
                "announce-devel", "announce-devel-discuss"],
            "Invalid template selection")
        if not valid.ok:
            lists, existing = get_user_lists(project, client)
            return render_template("mailing-list-new.html", view="new-resource",
                    owner=owner, project=project, lists=lists,
                    existing=existing, **valid.kwargs)
        return lists_from_template(owner, project, template)
    elif "create" in valid:
        with valid:
            mailing_list = client.create_list(
                name=valid.require("name"),
                description=valid.optional("description"),
                visibility=ListVisibility(project.visibility.value)
            ).mailing_list
        if not valid.ok:
            lists, existing = get_user_lists(project, client)
            return render_template("mailing-list-new.html", view="new-resource",
                    owner=owner, project=project, lists=lists,
                    existing=existing, **valid.kwargs)
    else:
        list_rid = None
        for field in valid.source:
            if field.startswith("existing-"):
                list_rid = field[len("existing-"):]
                break
        if not list_rid:
            search = valid.optional("search")
            lists, existing = get_user_lists(project, client, search)
            return render_template("mailing-list-new.html", view="new-resource",
                    owner=owner, project=project, lists=lists,
                    existing=existing, **valid.kwargs)
        mailing_list = client.get_list(list_rid).mailing_list

    finalize_add_list(client, owner, project, mailing_list)
    return redirect(url_for("projects.summary_GET",
        owner=owner.canonical_name, project_name=project.name))

@mailing_lists.route("/<owner>/<project_name>/lists/manage")
@loginrequired
def manage_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    mailing_lists = (MailingList.query
            .filter(MailingList.project_id == project.id)
            .order_by(MailingList.updated.desc()))

    terms = request.args.get("search")
    search_error = None
    try:
        mailing_lists = search_by(mailing_lists, terms,
                [MailingList.name, MailingList.description])
    except ValueError as ex:
        search_error = str(ex)

    mailing_lists, pagination = paginate_query(mailing_lists)
    return render_template("mailing-lists-manage.html",
            view="mailing lists", owner=owner, project=project,
            search=terms, search_error=search_error,
            mailing_lists=mailing_lists,
            **pagination)

@mailing_lists.route("/<owner>/<project_name>/lists/delete/<int:list_id>")
@loginrequired
def delete_GET(owner, project_name, list_id):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    mailing_list = (MailingList.query
        .filter(MailingList.id == list_id)
        .filter(MailingList.project_id == project.id)).one_or_none()
    if not mailing_list:
        abort(404)
    return render_template("resource-delete.html", view="mailing lists",
            owner=owner, project=project, resource=mailing_list,
            resource_type="mailing list")

@mailing_lists.route("/<owner>/<project_name>/lists/delete/<int:list_id>",
        methods=["POST"])
@loginrequired
def delete_POST(owner, project_name, list_id):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)
    mailing_list = (MailingList.query
        .filter(MailingList.id == list_id)
        .filter(MailingList.project_id == project.id)).one_or_none()
    if not mailing_list:
        abort(404)

    list_id = mailing_list.remote_id
    hook_id = mailing_list.webhook_id
    db.session.delete(mailing_list)
    db.session.commit()

    client = ListsClient()
    client.delete_list_webhook(hook_id)

    valid = Validation(request)
    delete_remote = valid.optional("delete-remote") == "on"
    if delete_remote:
        client.delete_list(list_id)

    return redirect(url_for("projects.summary_GET",
        owner=owner.canonical_name, project_name=project.name))
