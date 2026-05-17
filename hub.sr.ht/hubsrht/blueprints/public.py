from sqlalchemy.sql import operators
from flask import Blueprint, render_template, request, session
from hubsrht.types import Project, Feature, Event, EventType, Visibility, User
from srht.app import paginate_query
from srht.database import db
from srht.oauth import UserType, current_user, loginrequired
from srht.search import search_by

public = Blueprint("public", __name__)

@public.route("/")
def index():
    if current_user:
        notice = session.pop("notice", None)
        projects = (Project.query
                .filter(Project.owner_id == current_user.id)
                .order_by(Project.updated.desc())
                .limit(5)).all()
        if any(projects):
            events = (Event.query
                    .filter(Event.user_id == current_user.id)
                    .order_by(Event.created.desc())
                    .limit(2)).all()
            return render_template("dashboard.html",
                    projects=projects, EventType=EventType, events=events,
                    notice=notice)
        return render_template("new-user-dashboard.html", notice=notice)

    features = (Feature.query
            .join(Project, Feature.project_id == Project.id)
            .join(User, Project.owner_id == User.id)
            .filter(Project.visibility == Visibility.PUBLIC)
            .order_by(Feature.created.desc())
            .limit(6)).all()
    return render_template("index.html", features=features)

@public.route("/getting-started")
@loginrequired
def getting_started():
    notice = session.pop("notice", None)
    return render_template("new-user-dashboard.html", notice=notice)

@public.route("/projects")
def project_index():
    projects = (Project.query.join(User)
        .filter(User.user_type != UserType.suspended)
        .filter(Project.visibility == Visibility.PUBLIC))

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

    sort = request.args.get("sort")
    if sort and sort == "recently-updated":
        projects = projects.order_by(Project.updated.desc())
    elif sort and sort == "longest-active":
        projects = projects.order_by((Project.updated - Project.created).desc())
    else:
        projects = projects.order_by(Project.updated.desc())

    try:
        projects, pagination = paginate_query(projects)
    except ValueError as e:
        search_error = str(e)

    features = (Feature.query
            .join(Project, Feature.project_id == Project.id)
            .join(User, Project.owner_id == User.id)
            .filter(Project.visibility == Visibility.PUBLIC)
            .order_by(Feature.created.desc())
            .limit(5)).all()

    return render_template("project-index.html", projects=projects,
            search=search, features=features, sort=sort, **pagination,
            search_keys=["sort"], search_error=search_error)

@public.route("/projects/featured")
def featured_projects():
    features = (Feature.query
            .join(Project, Feature.project_id == Project.id)
            .join(User, Project.owner_id == User.id)
            .filter(Project.visibility == Visibility.PUBLIC)
            .order_by(Feature.created.desc()))
    features, pagination = paginate_query(features)
    return render_template("featured-projects.html",
            features=features, **pagination)
