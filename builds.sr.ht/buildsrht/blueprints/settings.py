from flask import Blueprint, current_app, render_template, request, url_for, abort, redirect
from flask import current_app
from srht.oauth import current_user, loginrequired
from srht.validation import Validation
from buildsrht.blueprints.jobs import tags
from buildsrht.graphql import Client, Visibility, GraphQLClientGraphQLMultiError
from buildsrht.types import Job

settings = Blueprint("settings", __name__)

@settings.route("/~<username>/job/<int:job_id>/settings/details")
@loginrequired
def details_GET(username, job_id):
    job = Job.query.get(job_id)
    if not job:
        abort(404)
    if current_user.id != job.owner_id:
        abort(404)
    return render_template("job-details.html",
        view="details", job=job)

@settings.route("/~<username>/job/<int:job_id>/settings/details", methods=["POST"])
@loginrequired
def details_POST(username, job_id):
    valid = Validation(request)
    visibility = valid.require("visibility", cls=Visibility)
    tags_string = valid.optional("tags", default="")
    tag_list = [ t ["name"] for t in tags(tags_string) ]
    if not valid.ok:
        job = Job.query.get(job_id)
        return render_template("job-details.html",
            job=job, **valid.kwargs), 400

    job = Client().update_job(job_id, visibility, tag_list).update
    if not job:
        abort(404)

    return redirect(url_for("settings.details_GET",
        username=job.owner.username,
        job_id=job.id))
