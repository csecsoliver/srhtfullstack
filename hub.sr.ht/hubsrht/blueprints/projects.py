import re
import string
from flask import Blueprint, Response, render_template, request, redirect, url_for
from flask import session, abort, make_response
from hubsrht.decorators import adminrequired
from hubsrht.projects import ProjectAccess, get_project, get_project_or_redir
from hubsrht.services.git import GitClient
from hubsrht.services.hg import HgClient
from hubsrht.services.lists import ListsClient
from hubsrht.services.todo import TodoClient
from hubsrht.types import Feature, Event, EventType
from hubsrht.types import Project, RepoType, Visibility
from hubsrht.types import Redirect
from hubsrht.types import SourceRepo, MailingList, Tracker
from hubsrht.types.eventprojectassoc import EventProjectAssociation
from markupsafe import Markup, escape
from sqlalchemy import or_
from sqlalchemy.sql import text
from srht.app import csrf_bypass, paginate_query
from srht.config import cfg, get_origin
from srht.database import db
from srht.graphql import InternalAuth
from srht.markdown import markdown, sanitize
from srht.oauth import current_user, loginrequired
from srht.validation import Validation, valid_url

projects = Blueprint("projects", __name__)

site_name = cfg("sr.ht", "site-name")
ext_origin = get_origin("hub.sr.ht", external=True)

def get_clone_message(owner, project, scm, sources):
    repo_urls = ""
    for repo in sources:
        # Python doesn't allow \ in format string blocks ({}) so we add
        # it here to get a newline before URLs block.
        repo_urls += f"\n  {repo.url()}{' - ' + repo.description if repo.description else ''}"

    return f"""

You have tried to clone a project from {site_name}, but you probably meant to
clone a specific {scm} repository for this project instead. A single project on
{site_name} often has more than one {scm} repository.

{"You may want one of the following repositories:" + repo_urls if repo_urls != "" else ""}

To browse all of the available repositories for this project, visit this URL:

  {ext_origin}{url_for("sources.sources_GET",
      owner=owner.canonical_name, project_name=project.name)}
"""

def get_readme(owner, repo):
    auth = InternalAuth(owner)
    html, plaintext, md = None, None, None

    if repo.repo_type == RepoType.git:
        blob_prefix = repo.url() + "/blob/HEAD/"
        rendered_prefix = repo.url() + "/tree/HEAD/"

        client = GitClient(auth)
        git_repo = client.get_readme(owner.username, repo.name).user.repository
        if not git_repo:
            raise Exception(f"git.sr.ht returned no repository for {owner.username}/{repo.name}")
        html = git_repo.html
        if git_repo.plaintext:
            plaintext = git_repo.plaintext.object.text
        if git_repo.md or git_repo.markdown:
            md = (git_repo.md or git_repo.markdown).object.text
    elif repo.repo_type == RepoType.hg:
        blob_prefix = repo.url() + "/raw/"
        rendered_prefix = repo.url() + "/browse/"

        client = HgClient(auth)
        hg_repo = client.get_readme(owner.username, repo.name).user.repository
        html = hg_repo.html
        plaintext = hg_repo.plaintext
        md = hg_repo.md or hg_repo.markdown

    if html:
        return Markup(sanitize(html))

    if md:
        html = markdown(md, link_prefix=[rendered_prefix, blob_prefix])
        return Markup(html)

    if plaintext:
        return Markup(f"<pre>{escape(plaintext)}</pre>")

    return None

@projects.route("/<owner>/<project_name>/")
def summary_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.read)

    # Mercurial clone
    if request.args.get("cmd") == "capabilities":
        sources = (SourceRepo.query
                .filter(SourceRepo.project_id == project.id)
                .filter(SourceRepo.repo_type == RepoType.hg)
                .filter(SourceRepo.visibility == Visibility.PUBLIC)
                .order_by(SourceRepo.updated.desc())
                .limit(5))

        return Response(get_clone_message(owner, project, "hg", sources),
                mimetype="text/plain")

    summary = None
    summary_error = False
    if project.summary_repo_id is not None:
        repo = project.summary_repo
        try:
            summary = get_readme(owner, repo)
        except Exception as ex:
            summary = None
            summary_error = True

    events = (Event.query
        .outerjoin(EventProjectAssociation)
        .filter(EventProjectAssociation.project_id == project.id)
        .order_by(Event.created.desc()))
    if not current_user or current_user.id != owner.id:
        events = (events
            .outerjoin(SourceRepo)
            .outerjoin(MailingList)
            .outerjoin(Tracker)
            .filter(or_(Event.source_repo == None, SourceRepo.visibility == Visibility.PUBLIC),
                or_(Event.mailing_list == None, MailingList.visibility == Visibility.PUBLIC),
                or_(Event.tracker == None, Tracker.visibility == Visibility.PUBLIC)))
    events = events.limit(2).all()

    return render_template("project-summary.html", view="summary",
            owner=owner, project=project,
            summary=summary, summary_error=summary_error,
            events=events, EventType=EventType)

@projects.route("/<owner>/<project_name>/info/refs")
def summary_refs(owner, project_name):
    if request.args.get("service") == "git-upload-pack":
        owner, project = get_project_or_redir(owner, project_name, ProjectAccess.read)

        sources = (SourceRepo.query
                .filter(SourceRepo.project_id == project.id)
                .filter(SourceRepo.repo_type == RepoType.git)
                .filter(SourceRepo.visibility == Visibility.PUBLIC)
                .order_by(SourceRepo.updated.desc())
                .limit(5))

        msg = get_clone_message(owner, project, "git", sources)

        return Response(f"""001e# service=git-upload-pack
000000400000000000000000000000000000000000000000 HEAD\0agent=hubsrht
{'{:04x}'.format(4 + 3 + 1 + len(msg))}ERR {msg}0000""",
                mimetype="application/x-git-upload-pack-advertisement")
    else:
        abort(404)

@projects.route("/<owner>/<project_name>/feed")
def feed_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.read)

    events = (Event.query
        .outerjoin(EventProjectAssociation)
        .filter(EventProjectAssociation.project_id == project.id)
        .order_by(Event.created.desc()))

    if not current_user or current_user.id != owner.id:
        events = (events
            .outerjoin(SourceRepo)
            .outerjoin(MailingList)
            .outerjoin(Tracker)
            .filter(or_(Event.source_repo == None, SourceRepo.visibility == Visibility.PUBLIC),
                or_(Event.mailing_list == None, MailingList.visibility == Visibility.PUBLIC),
                or_(Event.tracker == None, Tracker.visibility == Visibility.PUBLIC)))

    events, pagination = paginate_query(events)

    return render_template("project-feed.html",
            view="summary", owner=owner, project=project,
            events=events, EventType=EventType, **pagination)

@projects.route("/<owner>/<project_name>/feed.rss")
def feed_rss_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.read)

    events = (Event.query
        .outerjoin(EventProjectAssociation)
        .filter(EventProjectAssociation.project_id == project.id)
        .order_by(Event.created.desc()))

    if not current_user or current_user.id != owner.id:
        events = (events
            .outerjoin(SourceRepo)
            .outerjoin(MailingList)
            .outerjoin(Tracker)
            .filter(or_(Event.source_repo == None, SourceRepo.visibility == Visibility.PUBLIC),
                or_(Event.mailing_list == None, MailingList.visibility == Visibility.PUBLIC),
                or_(Event.tracker == None, Tracker.visibility == Visibility.PUBLIC)))

    events, pagination = paginate_query(events)

    res = make_response(render_template("project-feed-rss.html",
            view="summary", owner=owner, project=project,
            events=events, EventType=EventType, **pagination))
    res.headers['Content-Type'] = 'text/xml; charset=utf-8'
    return res

@projects.route("/<owner>/<project_name>/dismiss-checklist", methods=["POST"])
@loginrequired
def dismiss_checklist_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)
    project.checklist_complete = True
    db.session.commit()
    return redirect(url_for("projects.summary_GET",
        owner=current_user.canonical_name,
        project_name=project.name))

def _verify_tags(valid, raw_tags):
    raw_tags = raw_tags or ""
    tags = list(filter(lambda t: t,
            map(lambda t: t.strip(string.whitespace + "#"), raw_tags.split(","))))
    valid.expect(len(tags) <= 3,
            f"Too many tags ({len(tags)}, max 3)",
            field="tags")
    valid.expect(all(len(t) <= 16 for t in tags),
            "Tags may be no longer than 16 characters",
            field="tags")
    valid.expect(all(re.match(r"^[A-Za-z0-9_][A-Za-z0-9_.-]*$", t) for t in tags),
            "Tags must start with alphanumerics or underscores " +
                "and may additionally include dots and dashes",
            field="tags")
    return tags

@projects.route("/projects/create")
@loginrequired
def create_GET():
    return render_template("project-create.html")

@projects.route("/projects/create", methods=["POST"])
@loginrequired
def create_POST():
    valid = Validation(request)
    name = valid.require("name")
    description = valid.require("description")
    raw_tags = valid.optional("tags")
    visibility = valid.require("visibility", cls=Visibility)
    valid.expect(not name or len(name) < 128,
            "Name must be fewer than 128 characters", field="name")
    valid.expect(not name or re.match(r'^[A-Za-z0-9._-]+$', name),
            "Name must match [A-Za-z0-9._-]+", field="name")
    valid.expect(not name or name not in [".", ".."],
            "Name cannot be '.' or '..'", field="name")
    valid.expect(not name or name not in [".git", ".hg"],
            "Name must not be '.git' or '.hg'", field="name")
    valid.expect(not name or Project.query
            .filter(Project.name.ilike(name))
            .filter(Project.owner_id == current_user.id).count() == 0,
            "Name must be unique among your projects", field="name")
    valid.expect(not description or len(description) < 512,
            "Description must be fewer than 512 characters",
            field="description")
    tags = _verify_tags(valid, raw_tags)
    if not valid.ok:
        kwargs = valid.kwargs
        kwargs.pop("tags")
        return render_template("project-create.html", **kwargs, tags=tags)

    project = Project()
    project.name = name
    project.description = description
    project.tags = tags
    project.visibility = visibility
    project.owner_id = current_user.id
    db.session.add(project)
    db.session.commit()

    return redirect(url_for("projects.summary_GET",
        owner=current_user.canonical_name,
        project_name=project.name))

@projects.route("/<owner>/<project_name>/settings")
@loginrequired
def config_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    return render_template("project-config.html", view="add more",
            owner=owner, project=project)

@projects.route("/<owner>/<project_name>/settings", methods=["POST"])
@loginrequired
def config_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)

    valid = Validation(request)
    description = valid.require("description")
    tags = _verify_tags(valid, valid.optional("tags"))
    website = valid.optional("website")
    visibility = valid.require("visibility", cls=Visibility)
    valid.expect(not website or valid_url(website),
            "Website must be a valid http or https URL")
    if not valid.ok:
        return render_template("project-config.html", view="add more",
                owner=owner, project=project, **valid.kwargs)

    project.description = description
    project.tags = tags
    project.website = website
    project.visibility = visibility
    db.session.commit()

    return redirect(url_for("projects.summary_GET",
        owner=current_user.canonical_name,
        project_name=project.name))


@projects.route("/<owner>/<project_name>/settings/rename")
@loginrequired
def settings_rename(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)
    return render_template("project-rename.html", owner=owner, project=project)

@projects.route("/<owner>/<project_name>/settings/rename", methods=["POST"])
@loginrequired
def settings_rename_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)

    valid = Validation(request)
    name = valid.require("name", friendly_name="Name")

    valid.expect(not name or len(name) < 128,
            "Name must be fewer than 128 characters", field="name")
    valid.expect(not name or re.match(r'^[A-Za-z0-9._-]+$', name),
            "Name must match [A-Za-z0-9._-]+", field="name")
    valid.expect(not name or name not in [".", ".."],
            "Name cannot be '.' or '..'", field="name")
    valid.expect(not name or name not in [".git", ".hg"],
            "Name must not be '.git' or '.hg'", field="name")
    valid.expect(not name or Project.query
            .filter(Project.name.ilike(name))
            .filter(Project.owner_id == current_user.id).count() == 0,
            "Name must be unique among your projects", field="name")


    if not valid.ok:
        return render_template("project-rename.html", owner=owner, project=project,
                **valid.kwargs)

    project.name = name
    redir = Redirect()
    redir.owner_id = project.owner_id
    redir.name = project_name
    redir.new_project_id = project.id
    db.session.add(redir)
    db.session.commit()

    return redirect(url_for("projects.summary_GET", owner=owner, project_name=project.name))


@projects.route("/<owner>/<project_name>/delete")
@loginrequired
def delete_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    return render_template("project-delete.html", view="add more",
            owner=owner, project=project)

@projects.route("/<owner>/<project_name>/delete", methods=["POST"])
@loginrequired
def delete_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)
    session["notice"] = f"{project.name} has been deleted."

    git_client = GitClient()
    lists_client = ListsClient()
    todo_client = TodoClient()

    # Any mailing list, repository or tracker associated to the project will
    # be deleted via the foreign key it has on project.id; we need to clean-up
    # remote resources associated to it.
    associated_lists = (MailingList.query
        .filter(MailingList.project_id == project.id))
    for i in associated_lists:
        try:
            lists_client.delete_list_webhook(i.webhook_id)
        except:
            pass # Resource likely removed by user

    associated_repos = (SourceRepo.query
        .filter(SourceRepo.project_id == project.id)
        .filter(SourceRepo.repo_type == RepoType.git))
    for r in associated_repos:
        try:
            git_client.delete_repo_webhook(r.webhook_id)
        except:
            pass # Resource likely removed by user

    associated_trackers = (Tracker.query
        .filter(Tracker.project_id == project.id))
    for t in associated_trackers:
        try:
            todo_client.delete_tracker_webhook(t.webhook_id)
        except:
            pass # Resource likely removed by user

    with db.engine.connect() as conn:
        conn.execute(text(f"DELETE FROM project WHERE id = {project.id}"))
        conn.commit()
    return redirect(url_for("public.index"))

@projects.route("/<owner>/<project_name>/feature", methods=["POST"])
@adminrequired
def feature_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.read)
    if project is None:
        abort(404)
    valid = Validation(request)

    feature = Feature()
    feature.project_id = project.id
    feature.summary = valid.require("summary")
    if not valid.ok:
        abort(400) # admin-only route, who cares
    db.session.add(feature)
    db.session.commit()
    return redirect(url_for("public.project_index"))
