from flask import Blueprint, render_template, abort, request, redirect, url_for
from flask import current_app, session
from srht.config import cfg
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.validation import Validation
from listssrht.blueprints.archives import get_list
from listssrht.graphql import Client, MailingListInput, ACLInput, Visibility
from listssrht.graphql import Upload
from listssrht.types import Access, List, ListAccess, User

settings = Blueprint("settings", __name__)

access_help_map = {
    ListAccess.browse:
        "Permission to subscribe and browse the archives",
    ListAccess.reply:
        "Permission to reply to threads submitted by an authorized user.",
    ListAccess.post:
        "Permission to submit new threads.",
    ListAccess.moderate:
        "Permission to moderate threads and patches.",
}

@settings.route("/<owner_name>/<list_name>/settings/info")
@loginrequired
def info_GET(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)
    return render_template("settings-info.html", view="info",
            ml=ml, owner=owner, access_type_list=ListAccess,
            access_help_map=access_help_map)

@settings.route("/<owner_name>/<list_name>/settings/info", methods=["POST"])
@loginrequired
def info_POST(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)

    client = Client()

    valid = Validation(request)
    updates = MailingListInput()
    updates.visibility = valid.require("visibility", cls=Visibility)
    desc = valid.optional("description")
    if not desc:
        updates.description = None
    else:
        updates.description = desc

    with valid:
        client.update_mailing_list(ml.id, updates)

    if not valid.ok:
        return render_template("settings-info.html", ml=ml, owner=owner,
                access_type_list=ListAccess, access_help_map=access_help_map,
                view="info", **valid.kwargs)

    return redirect(url_for("settings.info_GET",
        owner_name=owner_name, list_name=list_name))

@settings.route("/<owner_name>/<list_name>/settings/access")
@loginrequired
def access_GET(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)
    return render_template("settings-access.html", view="access",
            ml=ml, owner=owner, access_type_list=ListAccess,
            access_help_map=access_help_map)

def _process_access(valid, perm):
    bitfield = ListAccess.none
    for access in ListAccess:
        if access in [ListAccess.none]:
            continue
        if valid.optional("perm_{}_{}".format(
                perm, access.name)) != None:
            bitfield |= access
    return bitfield

@settings.route("/<owner_name>/<list_name>/settings/access", methods=["POST"])
@loginrequired
def access_POST(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)

    client = Client()

    valid = Validation(request)
    access = _process_access(valid, "default")

    acl = ACLInput(
        browse=(access & ListAccess.browse) != 0,
        reply=(access & ListAccess.reply) != 0,
        post=(access & ListAccess.post) != 0,
        moderate=(access & ListAccess.moderate) != 0,
    )

    with valid:
        client.update_mailing_list_access(ml.id, acl)

    if not valid.ok:
        return render_template("settings-access.html", view="access",
                ml=ml, owner=owner, access_type_list=ListAccess,
                access_help_map=access_help_map, **valid.kwargs)

    return redirect(url_for("settings.access_GET",
        owner_name=owner_name, list_name=list_name))

@settings.route("/<owner_name>/<list_name>/settings/acl", methods=["POST"])
@loginrequired
def acl_POST(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)

    valid = Validation(request)

    username = valid.require("user")
    if not valid.ok:
        return render_template("settings-access.html", view="access",
                ml=ml, owner=owner, access_type_list=ListAccess,
                access_help_map=access_help_map, hide_global=True,
                **valid.kwargs)
    if username.startswith("~"):
        username = username[1:]

    if "@" in username:
        # TODO: Figure out if we can associate emails with users for users we
        # haven't seen yet
        user = User.query.filter(User.email == username).one_or_none()
    else:
        user = current_app.oauth_service.lookup_user(username)
        valid.expect(user, "User not found", field="user")

    if not valid.ok:
        return render_template("settings-access.html", view="access",
                ml=ml, owner=owner, access_type_list=ListAccess,
                access_help_map=access_help_map, hide_global=True,
                **valid.kwargs)

    # Edit existing ACL entry if present
    if user:
        acl = (Access.query
                .filter(Access.list_id == ml.id)
                .filter(Access.user_id == user.id)
            ).one_or_none()
    else:
        acl = (Access.query
                .filter(Access.list_id == ml.id)
                .filter(Access.email == username)
            ).one_or_none()

    if not acl:
        acl = Access()
        acl.list_id = ml.id
        if user:
            acl.user_id = user.id
        else:
            acl.email = username
    acl.permissions = _process_access(valid, "acl")
    if ListAccess.browse in ml.default_access:
        acl.permissions |= ListAccess.browse
    db.session.add(acl)
    db.session.commit()
    return redirect(url_for("settings.access_GET",
        owner_name=owner_name, list_name=list_name))

@settings.route("/<owner_name>/<list_name>/settings/acl/<int:acl_id>/delete",
        methods=["POST"])
@loginrequired
def acl_delete_POST(owner_name, list_name, acl_id):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)
    acl = Access.query.filter(Access.id == acl_id).one_or_none()
    if not acl:
        abort(404)
    if acl.list_id != ml.id:
        abort(403)
    db.session.delete(acl)
    db.session.commit()
    return redirect(url_for("settings.access_GET",
        owner_name=owner_name, list_name=list_name))

@settings.route("/<owner_name>/<list_name>/settings/content")
@loginrequired
def content_GET(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)
    return render_template("settings-content.html",
            view="content", ml=ml, owner=owner)

@settings.route("/<owner_name>/<list_name>/settings/content", methods=["POST"])
@loginrequired
def content_POST(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)

    client = Client()

    valid = Validation(request)
    updates = MailingListInput()
    updates.permit_mime = valid.require("permitMime").split(",")
    updates.reject_mime = valid.require("rejectMime").split(",")

    with valid:
        client.update_mailing_list(ml.id, updates)

    if not valid.ok:
        return render_template("settings-content.html",
                view="content", ml=ml, owner=owner,
                **valid.kwargs)

    return redirect(url_for("settings.content_GET",
        owner_name=owner_name, list_name=list_name))

@settings.route("/<owner_name>/<list_name>/settings/import-export")
@loginrequired
def import_export_GET(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)
    return render_template("settings-import-export.html",
            view="import/export", ml=ml, owner=owner)

@settings.route("/<owner_name>/<list_name>/settings/import", methods=["POST"])
@loginrequired
def import_POST(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)
    if ml.import_in_progress:
        abort(400)

    spool = request.files.get("spool")
    valid = Validation(request)
    valid.expect(spool is not None, "Mail spool is required", field="spool")

    if not valid.ok:
        return render_template("settings-import-export.html",
                view="import/export", ml=ml, owner=owner, **valid.kwargs)

    client = Client()
    spool = Upload(spool.filename, spool, "application/octet-stream")
    with valid:
        client.import_spool(ml.id, spool)

    if not valid.ok:
        return render_template("settings-import-export.html",
                view="import/export", ml=ml, owner=owner, **valid.kwargs)

    return redirect(url_for("archives.archive",
        owner_name=owner_name, list_name=list_name))

@settings.route("/<owner_name>/<list_name>/settings/delete")
@loginrequired
def delete_GET(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)
    return render_template("settings-delete.html",
            view="delete", ml=ml, owner=owner)

@settings.route("/<owner_name>/<list_name>/settings/delete", methods=["POST"])
@loginrequired
def delete_POST(owner_name, list_name):
    owner, ml, access = get_list(owner_name, list_name)
    if not ml:
        abort(404)
    if ml.owner_id != current_user.id:
        abort(403)
    Client().delete_mailing_list(ml.id)
    session["notice"] = f"{ml.name} is being deleted. This may take a few minutes."
    return redirect(url_for("user.index"))
