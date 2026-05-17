from flask import abort
from flask import request
from flask import url_for
from flask import redirect
from hubsrht.types import Project, User, Visibility
from hubsrht.types import Redirect
from srht.oauth import current_user
from enum import Enum

class ProjectAccess(Enum):
    read = "read"
    write = "write"

def get_project(owner, project_name, access, user=current_user):
    """Get project owner and project."""
    if owner.startswith("~"):
        owner = owner[1:]
    else:
        abort(404)
    project = (Project.query
            .join(User, Project.owner_id == User.id)
            .filter(User.username == owner)
            .filter(Project.name == project_name)
        ).one_or_none()
    if not project:
        return None, None
    if user != None and user.id == project.owner_id:
        return project.owner, project
    if access == ProjectAccess.write:
        abort(401)
    # TODO: ACLs
    if project.visibility in (Visibility.PUBLIC, Visibility.UNLISTED):
        return project.owner, project
    elif project.visibility == Visibility.PRIVATE:
        abort(401)
    assert False

def get_project_or_redir(owner, project_name, access, user=current_user):
    """Get owner and project, implicitly redirect if necessary."""
    o, project = get_project(owner, project_name, access, user)
    if project:
        return o, project

    if owner.startswith("~"):
        owner = owner[1:]
    else:
        abort(404)

    if redir := (Redirect.query
        .join(User, Redirect.owner_id == User.id)
        .filter(User.username == owner)
        .filter(Redirect.name == project_name)
     ).first():
        view_args = request.view_args
        view_args["owner"] = redir.new_project.owner.canonical_name
        view_args["project_name"] = redir.new_project.name
        abort(redirect(url_for(request.endpoint, **view_args)))
    abort(404)
