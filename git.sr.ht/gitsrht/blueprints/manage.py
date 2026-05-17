from flask import Blueprint, current_app, request, render_template, abort
from flask import redirect, url_for
from srht.app import session
from srht.database import db
from srht.oauth import current_user, loginrequired, UserType
from srht.validation import Validation
from gitsrht.access import check_access, UserAccess
from gitsrht.errors import handle_gql_error
from gitsrht.graphql import AccessMode, Client, Visibility, RepoInput
from gitsrht.types import Access, User, Redirect

manage = Blueprint('manage', __name__)

@manage.route("/create")
@loginrequired
def create_GET():
    another = request.args.get("another")
    name = request.args.get("name")
    return render_template("create.html", another=another, name=name)

@manage.route("/create", methods=["POST"])
@loginrequired
def create_POST():
    valid = Validation(request)
    name = valid.require("name", friendly_name="Name")
    desc = valid.optional("description")
    vis = valid.require("visibility", cls=Visibility)
    if not valid.ok:
        return render_template("create.html", **valid.kwargs)
    if desc == "":
        desc = None

    with valid:
        repo = Client().create_repository(name, vis, desc).repository
    if not valid.ok:
        return render_template("create.html", **valid.kwargs)

    another = valid.optional("another")
    if another == "on":
        return redirect("/create?another")
    else:
        return redirect(url_for("repo.summary",
            owner=current_user.canonical_name,
            repo=repo.name))

@manage.route("/clone")
@loginrequired
def clone():
    return render_template("clone.html", visibility="UNLISTED")

@manage.route("/clone", methods=["POST"])
@loginrequired
def clone_POST():
    valid = Validation(request)
    clone_url = valid.require("cloneUrl", friendly_name="Clone URL")
    name = valid.require("name", friendly_name="Name")
    desc = valid.optional("description")
    vis = valid.require("visibility", cls=Visibility)
    if not valid.ok:
        return render_template("clone.html", **valid.kwargs)

    with valid:
        repo = Client().clone_repository(clone_url, name, vis, desc).repository
    if not valid.ok:
        return render_template("clone.html", **valid.kwargs)

    return redirect(url_for("repo.summary",
        owner=current_user.canonical_name, repo=repo.name))

@manage.route("/<owner_name>/<repo_name>/settings/info")
@loginrequired
def settings_info(owner_name, repo_name):
    owner, repo = check_access(owner_name, repo_name, UserAccess.manage)
    if isinstance(repo, Redirect):
        return redirect(url_for(".settings_info",
            owner_name=owner_name, repo_name=repo.new_repo.name))
    return render_template("settings_info.html", owner=owner, repo=repo)

@manage.route("/<owner_name>/<repo_name>/settings/info", methods=["POST"])
@loginrequired
def settings_info_POST(owner_name, repo_name):
    owner, repo = check_access(owner_name, repo_name, UserAccess.manage)
    if isinstance(repo, Redirect):
        repo = repo.new_repo

    valid = Validation(request)
    desc = valid.optional("description")
    head = valid.optional("HEAD")
    vis = valid.require("visibility", cls=Visibility)
    updates = RepoInput(visibility=vis)
    if desc:
        updates.description = desc
    if head:
        updates.head = head

    with valid:
        Client().update_repository(repo.id, updates)
    if not valid.ok:
        return render_template("settings_info.html",
                owner=owner, repo=repo, **valid.kwargs)

    return redirect(url_for("manage.settings_info",
        owner_name=owner_name, repo_name=repo_name))

@manage.route("/<owner_name>/<repo_name>/settings/rename")
@loginrequired
def settings_rename(owner_name, repo_name):
    owner, repo = check_access(owner_name, repo_name, UserAccess.manage)
    if isinstance(repo, Redirect):
        return redirect(url_for(".settings_rename",
            owner_name=owner_name, repo_name=repo.new_repo.name))
    return render_template("settings_rename.html", owner=owner, repo=repo)

@manage.route("/<owner_name>/<repo_name>/settings/rename", methods=["POST"])
@loginrequired
def settings_rename_POST(owner_name, repo_name):
    owner, repo = check_access(owner_name, repo_name, UserAccess.manage)
    if isinstance(repo, Redirect):
        repo = repo.new_repo

    valid = Validation(request)
    name = valid.require("name", friendly_name="Name")
    if not valid.ok:
        return render_template("settings_rename.html", owner=owner, repo=repo,
                **valid.kwargs)

    with valid:
        repo = Client().rename_repository(repo.id, name).repository
    if not valid.ok:
        return render_template("settings_rename.html",
                owner=owner, repo=repo, **valid.kwargs)
    return redirect(url_for("repo.summary",
        owner=owner_name, repo=repo.name))

@manage.route("/<owner_name>/<repo_name>/settings/access")
@loginrequired
def settings_access(owner_name, repo_name):
    cursor = request.args.get("nextacls")
    username = owner_name[1:]
    with handle_gql_error():
        resp = Client().get_repo_access_settings(username, repo_name, cursor)
    if resp.user is None or resp.user.repository is None:
        abort(404)
    return render_template("settings_access.html",
        owner=resp.user, repo=resp.user.repository)

@manage.route("/<owner_name>/<repo_name>/settings/access", methods=["POST"])
@loginrequired
def settings_access_POST(owner_name, repo_name):
    owner, repo = check_access(owner_name, repo_name, UserAccess.manage)
    if isinstance(repo, Redirect):
        repo = repo.new_repo
    valid = Validation(request)
    username = valid.require("user", friendly_name="User")
    mode = valid.optional("access", cls=AccessMode, default=AccessMode.RO)
    if not valid.ok:
        with handle_gql_error():
            resp = Client().get_repo_access_settings(owner_name[1:], repo_name, None)
        if resp.user is None or resp.user.repository is None:
            abort(404)
        return render_template("settings_access.html",
                owner=resp.user, repo=resp.user.repository, **valid.kwargs)
    # TODO: Group access
    if username[0] == "~":
        username = username[1:]
    try:
        user = current_app.oauth_service.lookup_user(username)
    except Exception:
        user = None
    valid.expect(user, "User not found.", field="user")
    valid.expect(not user or user.id != current_user.id,
            "You can't adjust your own access controls. You always have full read/write access.",
            field="user")
    valid.expect(not user or user.user_type != UserType.pending,
            "This account has not been confirmed yet.", field="user")
    valid.expect(not user or user.user_type != UserType.suspended,
            "This account has been suspended.", field="user")
    if not valid.ok:
        with handle_gql_error():
            resp = Client().get_repo_access_settings(owner_name[1:], repo_name, None)
        if resp.user is None or resp.user.repository is None:
            abort(404)
        return render_template("settings_access.html",
                owner=resp.user, repo=resp.user.repository, **valid.kwargs)
    grant = (Access.query
        .filter(Access.repo_id == repo.id, Access.user_id == user.id)
    ).first()
    if not grant:
        grant = Access()
        grant.repo_id = repo.id
        grant.user_id = user.id
        db.session.add(grant)
    grant.mode = mode
    db.session.commit()
    return redirect(url_for("manage.settings_access",
        owner_name=owner.canonical_name, repo_name=repo.name))

@manage.route("/<owner_name>/<repo_name>/settings/access/revoke/<grant_id>", methods=["POST"])
@loginrequired
def settings_access_revoke_POST(owner_name, repo_name, grant_id):
    owner, repo = check_access(owner_name, repo_name, UserAccess.manage)
    if isinstance(repo, Redirect):
        repo = repo.new_repo
    grant = (Access.query
        .filter(Access.repo_id == repo.id, Access.id == grant_id)
    ).first()
    if not grant:
        abort(404)
    db.session.delete(grant)
    db.session.commit()
    return redirect("/{}/{}/settings/access".format(
        owner.canonical_name, repo.name))

@manage.route("/<owner_name>/<repo_name>/settings/delete")
@loginrequired
def settings_delete(owner_name, repo_name):
    owner, repo = check_access(owner_name, repo_name, UserAccess.manage)
    if isinstance(repo, Redirect):
        return redirect(url_for(".settings_delete",
            owner_name=owner_name, repo_name=repo.new_repo.name))
    return render_template("settings_delete.html", owner=owner, repo=repo)

@manage.route("/<owner_name>/<repo_name>/settings/delete", methods=["POST"])
@loginrequired
def settings_delete_POST(owner_name, repo_name):
    owner, repo = check_access(owner_name, repo_name, UserAccess.manage)
    if isinstance(repo, Redirect):
        # Normally we'd redirect but we don't want to fuck up some other repo
        abort(404)
    repo = Client().delete_repository(repo.id).repository
    session["notice"] = "{}/{} was deleted.".format(
        repo.owner.canonical_name, repo.name)
    return redirect(url_for("public.user_index", username=repo.owner.username))
