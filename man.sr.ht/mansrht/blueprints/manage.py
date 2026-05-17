from flask import Blueprint, render_template, abort, request, redirect, url_for
from srht.database import db
from srht.oauth import loginrequired
from srht.validation import Validation
from mansrht.access import UserAccess, check_access
from mansrht.types import Visibility
from mansrht.wikis import delete_wiki, is_root_wiki

manage = Blueprint('manage', __name__)

@manage.route("/manage/<owner_name>/<wiki_name>/info")
@loginrequired
def info(owner_name, wiki_name):
    owner, wiki = check_access(owner_name, wiki_name, UserAccess.manage)
    is_root = is_root_wiki(wiki)
    return render_template("info.html", owner=owner, wiki=wiki, is_root=is_root)

@manage.route("/manage/<owner_name>/<wiki_name>/info", methods=["POST"])
@loginrequired
def info_POST(owner_name, wiki_name):
    owner, wiki = check_access(owner_name, wiki_name, UserAccess.manage)
    valid = Validation(request)
    visibility = valid.optional("visibility",
            cls=Visibility, default=wiki.visibility)
    wiki.visibility = visibility
    db.session.commit()
    return redirect(url_for("manage.info",
        owner_name=owner_name, wiki_name=wiki_name))

@manage.route("/manage/<owner_name>/<wiki_name>/delete")
@loginrequired
def delete(owner_name, wiki_name):
    # check_access() guarantees owner and wiki are valid.
    owner, wiki = check_access(owner_name, wiki_name, UserAccess.manage)
    return render_template("delete.html", owner=owner, wiki=wiki)

@manage.route("/manage/<owner_name>/<wiki_name>/delete", methods=["POST"])
@loginrequired
def delete_POST(owner_name, wiki_name):
    valid = Validation(request)
    delete_repo = valid.optional("delete_repo")

    # check_access() guarantees owner and wiki are valid.
    owner, wiki = check_access(owner_name, wiki_name, UserAccess.manage)
    delete_wiki(wiki, delete_repo == "on")
    return redirect("/")
