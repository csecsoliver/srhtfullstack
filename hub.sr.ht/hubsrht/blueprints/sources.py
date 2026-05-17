from flask import Blueprint, render_template, request, redirect, url_for, abort
from hubsrht.projects import ProjectAccess, get_project, get_project_or_redir
from hubsrht.services.hg import HgClient, Visibility as HgVisibility
from hubsrht.services.git import GitClient, Visibility as GitVisibility
from hubsrht.services.git import GraphQLClientGraphQLMultiError
from hubsrht.types import Event, EventType
from hubsrht.types import RepoType, SourceRepo, Visibility
from hubsrht.types.eventprojectassoc import EventProjectAssociation
from hubsrht.webhooks import get_user_webhooks
from srht.app import paginate_query
from srht.config import get_origin
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.search import search_by
from srht.validation import Validation

sources = Blueprint("sources", __name__)

GIT_WEBHOOK_VERSION = 3
HG_WEBHOOK_VERSION = 2

def get_repos(owner, project, repo_type):
    match repo_type:
        case RepoType.git:
            client = GitClient()
        case RepoType.hg:
            client = HgClient()

    # TODO: Pagination
    cursor = None
    repos = []
    while True:
        batch = client.get_repos(cursor).me.repositories
        repos.extend(batch.results)
        cursor = batch.cursor
        if not cursor:
            break

    repos = sorted(repos, key=lambda r: r.updated, reverse=True)
    existing = [r.remote_id for r in (SourceRepo.query
            .filter(SourceRepo.project_id == project.id)
            .filter(SourceRepo.repo_type == repo_type)).all()]
    return repos, existing

@sources.route("/<owner>/<project_name>/sources")
def sources_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.read)
    sources = (SourceRepo.query
            .filter(SourceRepo.project_id == project.id)
            .order_by(SourceRepo.updated.desc()))
    if not current_user or current_user.id != owner.id:
        sources = sources.filter(SourceRepo.visibility == Visibility.PUBLIC)

    terms = request.args.get("search")
    search_error = None
    try:
        sources = search_by(sources, terms,
                [SourceRepo.name, SourceRepo.description])
    except ValueError as ex:
        search_error = str(ex)

    sources, pagination = paginate_query(sources)
    return render_template("sources.html", view="sources",
            owner=owner, project=project, sources=sources,
            search=terms, search_error=search_error,
            **pagination)

@sources.route("/<owner>/<project_name>/sources/new")
@loginrequired
def new_GET(owner, project_name):
    # TODO: Redirect appropriately if this instance only has git or hg support
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    return render_template("sources-new.html", view="new-resource",
            owner=owner, project=project)

@sources.route("/<owner>/<project_name>/git/new")
@loginrequired
def git_new_GET(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    repos, existing = get_repos(owner, project, RepoType.git)
    return render_template("sources-select.html",
            view="new-resource", vcs="git",
            owner=owner, project=project, repos=repos, existing=existing,
            origin=get_origin("git.sr.ht", external=True))

@sources.route("/<owner>/<project_name>/hg/new")
@loginrequired
def hg_new_GET(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    repos, existing = get_repos(owner, project, RepoType.hg)
    return render_template("sources-select.html",
            view="new-resource", vcs="hg",
            owner=owner, project=project, repos=repos, existing=existing,
            origin=get_origin("hg.sr.ht", external=True))

@sources.route("/<owner>/<project_name>/git/new", methods=["POST"])
@loginrequired
def git_new_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)

    git_client = GitClient()
    valid = Validation(request)
    visibility = GitVisibility(project.visibility.value)

    if "create" in valid:
        name = valid.require("name")
        desc = valid.optional("description")
        with valid:
            git_repo = git_client.create_repo(name, visibility, desc).repository
        if not valid.ok:
            repos, existing = get_repos(owner, project, RepoType.git)
            return render_template("sources-select.html",
                    view="new-resource", vcs="git",
                    owner=owner, project=project, repos=repos,
                    existing=existing, **valid.kwargs)
    else:
        repo_rid = None
        for field in valid.source:
            if field.startswith("existing-"):
                repo_rid = field[len("existing-"):]
                break

        if not repo_rid:
            search = valid.optional("search")
            repos, existing = get_repos(owner, project, RepoType.git)
            # TODO: Search properly
            repos = filter(lambda r: search.lower() in r.name.lower(), repos)
            return render_template("sources-select.html",
                    view="new-resource", vcs="git",
                    owner=owner, project=project, repos=repos,
                    existing=existing, search=search)

        git_repo = GitClient().get_repo(repo_rid).repository

    repo = SourceRepo()
    repo.remote_id = git_repo.id
    repo.remote_rid = git_repo.rid
    repo.project_id = project.id
    repo.owner_id = owner.id
    repo.name = git_repo.name
    repo.description = git_repo.description
    repo.repo_type = RepoType.git
    repo.visibility = Visibility(git_repo.visibility.value)
    repo.webhook_id = -1
    repo.webhook_version = GIT_WEBHOOK_VERSION
    db.session.add(repo)
    db.session.flush()

    webhook_url = (get_origin("hub.sr.ht", external=False) +
           url_for("webhooks.git_repo", repo_id=repo.id))
    repo.webhook_id = git_client.create_repo_webhook(
            repo_id=repo.remote_id,
            payload=GitClient.event_webhook_query,
            url=webhook_url).webhook.id

    uwh = get_user_webhooks(owner)
    if uwh.git_webhook_id is None:
        user_webhook_url = (get_origin("hub.sr.ht", external=False) +
               url_for("webhooks.git_user", user_id=owner.id))
        uwh.git_webhook_id = git_client.create_user_webhook(
                payload=GitClient.event_webhook_query,
                url=user_webhook_url).webhook.id
        uwh.git_webhook_version = GIT_WEBHOOK_VERSION

    event = Event()
    event.event_type = EventType.source_repo_added
    event.source_repo_id = repo.id
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

@sources.route("/<owner>/<project_name>/hg/new", methods=["POST"])
@loginrequired
def hg_new_POST(owner, project_name):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)

    hg_client = HgClient()
    valid = Validation(request)
    visibility = HgVisibility(project.visibility.value)

    if "create" in valid:
        name = valid.require("name")
        desc = valid.require("description")
        with valid:
            hg_repo = hg_client.create_repo(name, visibility, desc).repository
        if not valid.ok:
            repos, existing = get_repos(owner, project, RepoType.hg)
            return render_template("sources-select.html",
                    view="new-resource", vcs="hg",
                    owner=owner, project=project, repos=repos,
                    existing=existing, **valid.kwargs)
    else:
        repo_rid = None
        for field in valid.source:
            if field.startswith("existing-"):
                repo_rid = field[len("existing-"):]
                break

        if not repo_rid:
            search = valid.optional("search")
            repos, existing = get_repos(owner, project, RepoType.git)
            # TODO: Search properly
            repos = filter(lambda r: search.lower() in r.name.lower(), repos)
            return render_template("sources-select.html",
                    view="new-resource", vcs="hg",
                    owner=owner, project=project, repos=repos,
                    existing=existing, **valid.kwargs)

        hg_repo = hg_client.get_repo(repo_rid).repository

    repo = SourceRepo()
    repo.remote_id = hg_repo.id
    repo.remote_rid = hg_repo.rid
    repo.project_id = project.id
    repo.owner_id = owner.id
    repo.name = hg_repo.name
    repo.description = hg_repo.description
    repo.repo_type = RepoType.hg
    repo.visibility = Visibility(hg_repo.visibility.value)
    repo.webhook_id = -1
    repo.webhook_version = HG_WEBHOOK_VERSION
    db.session.add(repo)
    db.session.flush()

    uwh = get_user_webhooks(owner)
    if uwh.hg_webhook_id is None:
        user_webhook_url = (get_origin("hub.sr.ht", external=False) +
               url_for("webhooks.hg_user", user_id=owner.id))
        uwh.hg_webhook_id = hg_client.create_user_webhook(
                payload=HgClient.event_webhook_query,
                url=user_webhook_url).webhook.id
        uwh.hg_webhook_version = HG_WEBHOOK_VERSION

    event = Event()
    event.event_type = EventType.source_repo_added
    event.source_repo_id = repo.id
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

@sources.route("/<owner>/<project_name>/sources/manage")
@loginrequired
def manage_GET(owner, project_name):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    sources = (SourceRepo.query
            .filter(SourceRepo.project_id == project.id)
            .order_by(SourceRepo.updated.desc()))

    terms = request.args.get("search")
    search_error = None
    try:
        sources = search_by(sources, terms,
                [SourceRepo.name, SourceRepo.description])
    except ValueError as ex:
        search_error = str(ex)

    sources, pagination = paginate_query(sources)
    return render_template("sources-manage.html", view="sources",
            owner=owner, project=project, sources=sources,
            search=terms, search_error=search_error,
            **pagination)

@sources.route("/<owner>/<project_name>/sources/summary/<int:repo_id>",
        methods=["POST"])
@loginrequired
def summary_POST(owner, project_name, repo_id):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)
    repo = (SourceRepo.query
        .filter(SourceRepo.id == repo_id)
        .filter(SourceRepo.project_id == project.id)).one_or_none()
    if not repo:
        abort(404)
    project.summary_repo_id = repo.id
    db.session.commit()
    return redirect(url_for("projects.summary_GET",
        owner=owner.canonical_name, project_name=project.name))

@sources.route("/<owner>/<project_name>/sources/delete/<int:repo_id>")
@loginrequired
def delete_GET(owner, project_name, repo_id):
    owner, project = get_project_or_redir(owner, project_name, ProjectAccess.write)
    repo = (SourceRepo.query
        .filter(SourceRepo.id == repo_id)
        .filter(SourceRepo.project_id == project.id)).one_or_none()
    if not repo:
        abort(404)
    return render_template("resource-delete.html", view="sources",
            owner=owner, project=project, resource=repo,
            resource_type=repo.repo_type.value + " repository")

@sources.route("/<owner>/<project_name>/sources/delete/<int:repo_id>",
        methods=["POST"])
@loginrequired
def delete_POST(owner, project_name, repo_id):
    owner, project = get_project(owner, project_name, ProjectAccess.write)
    if project is None:
        abort(404)
    repo = (SourceRepo.query
        .filter(SourceRepo.id == repo_id)
        .filter(SourceRepo.project_id == project.id)).one_or_none()
    if not repo:
        abort(404)

    if project.summary_repo_id == repo.id:
        project.summary_repo_id = None
        db.session.commit()

    match repo.repo_type:
        case RepoType.git:
            client = GitClient()
        case RepoType.hg:
            client = HgClient()

    try:
        if repo.repo_type == RepoType.git:
            client.delete_repo_webhook(repo.webhook_id)

        valid = Validation(request)
        delete_remote = valid.optional("delete-remote") == "on"
        if delete_remote:
            client.delete_repo(repo.remote_id)
    except GraphQLClientGraphQLMultiError:
        # This generally occurs if the remote repo (or webhook) was deleted and
        # we didn't hear about it. TODO: Replace me with semantic errors
        pass

    repo_name = repo.name
    repo_id = repo.remote_id
    db.session.delete(repo)
    db.session.commit()

    return redirect(url_for("projects.summary_GET",
        owner=owner.canonical_name, project_name=project.name))
