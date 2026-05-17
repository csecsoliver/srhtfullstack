import sqlalchemy as sa
import sqlalchemy_utils as sau
from datetime import datetime
from enum import Enum
from enum import IntFlag
from flask import abort, current_app, request, redirect, url_for
from gitsrht.graphql import AccessMode, Visibility
from gitsrht.types import Access, Repository, Redirect, User
from sqlalchemy.ext.declarative import declared_attr
from srht.database import db
from srht.oauth import current_user

class UserAccess(IntFlag):
    none = 0
    read = 1
    write = 2
    manage = 4

def get_repo(owner_name, repo_name):
    if owner_name[0] == "~":
        user = User.query.filter(User.username == owner_name[1:]).first()
        if user:
            repo = Repository.query.filter(Repository.owner_id == user.id)\
                .filter(Repository.name == repo_name).first()
        else:
            repo = None
        if user and not repo:
            repo = (Redirect.query
                    .filter(Redirect.owner_id == user.id)
                    .filter(Redirect.name == repo_name)
                ).first()
        return user, repo
    else:
        # TODO: organizations
        return None, None

def get_repo_or_redir(owner, repo):
    owner, repo = get_repo(owner, repo)
    if not repo or not has_access(repo, UserAccess.read):
        abort(404)
    if isinstance(repo, Redirect):
        view_args = request.view_args
        if not "repo" in view_args or not "owner" in view_args:
            return redirect(url_for(".summary",
                owner=repo.new_repo.owner.canonical_name,
                repo=repo.new_repo.name))
        view_args["owner"] = repo.new_repo.owner.canonical_name
        view_args["repo"] = repo.new_repo.name
        abort(redirect(url_for(request.endpoint, **view_args)))
    return owner, repo

def get_access(repo, user=None):
	# Note: when updating push access logic, also update git.sr.ht/git.sr.ht-shell
    if not user:
        user = current_user
    if not repo:
        return UserAccess.none
    if isinstance(repo, Redirect):
        # Just pretend they have full access for long enough to do the redirect
        return UserAccess.read | UserAccess.write | UserAccess.manage
    if not user:
        if repo.visibility == Visibility.PUBLIC or \
                repo.visibility == Visibility.UNLISTED:
            return UserAccess.read
        return UserAccess.none
    if repo.owner_id == user.id:
        return UserAccess.read | UserAccess.write | UserAccess.manage
    acl = Access.query.filter(
            Access.user_id == user.id,
            Access.repo_id == repo.id).first()
    if acl:
        acl.updated = datetime.utcnow()
        db.session.commit()
        if acl.mode == AccessMode.RO:
            return UserAccess.read
        else:
            return UserAccess.read | UserAccess.write
    if repo.visibility == Visibility.PRIVATE:
        return UserAccess.none
    return UserAccess.read

def has_access(repo, access, user=None):
    return access in get_access(repo, user)

def check_access(owner_name, repo_name, access):
    owner, repo = get_repo(owner_name, repo_name)
    if not owner or not repo:
        abort(404)
    a = get_access(repo)
    if not access in a:
        abort(403)
    return owner, repo
