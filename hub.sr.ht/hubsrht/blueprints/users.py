from flask import Blueprint, render_template, request, abort
from hubsrht.types import User, Project, Visibility
from sqlalchemy.sql import operators
from srht.app import paginate_query, get_profile
from srht.oauth import current_user, UserType
from srht.search import search_by

users = Blueprint("users", __name__)

@users.route("/~<username>/")
def summary_GET(username):
    user = (User.query
            .filter(User.username == username)
            .filter(User.user_type != UserType.suspended)).first()
    if not user:
        abort(404)
    projects = (Project.query
            .filter(Project.owner_id == user.id)
            .order_by(Project.updated.desc()))

    if not current_user or current_user.id != user.id:
        # TODO: ACLs
        projects = projects.filter(Project.visibility == Visibility.PUBLIC)

    projects, pagination = paginate_query(projects, results_per_page=5)

    return render_template("profile-summary.html",
            user=user, projects=projects,
            profile=get_profile(user), view="about", **pagination)

@users.route("/projects/<owner>/")
def projects_GET(owner):
    if owner.startswith("~"):
        owner = owner[1:]
    owner = User.query.filter(User.username == owner).first()
    if not owner:
        abort(404)
    projects = (Project.query
        .filter(Project.owner_id == owner.id)
        .order_by(Project.updated.desc()))
    if not current_user or current_user.id != owner.id:
        # TODO: ACLs
        projects = projects.filter(Project.visibility == Visibility.PUBLIC)

    search = request.args.get("search")
    search_error = None
    if search:
        try:
            projects = search_by(projects, search,
                    [Project.name, Project.description],
                    key_fns={"tag": lambda t:
                        Project.tags.any(t, operator=operators.ilike_op)},
                    term_map=lambda t: f"tag:{t[1:]}" if t.startswith("#") else t)
        except ValueError as e:
            search_error = str(e)

    projects, pagination = paginate_query(projects)

    return render_template("profile-projects.html",
            user=owner, projects=projects,
            search=search, search_error=search_error,
            profile=get_profile(owner), view="projects", **pagination)
