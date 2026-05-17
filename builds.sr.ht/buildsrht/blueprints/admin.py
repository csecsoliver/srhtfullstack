from buildsrht.blueprints.jobs import jobs_page
from buildsrht.types import Job, JobStatus
from flask import Blueprint, render_template
from buildsrht.decorators import adminrequired

admin = Blueprint("admin", __name__)

@admin.route("/admin")
@adminrequired
def dashboard():
    return jobs_page(
        Job.query.filter(Job.status not in [
            JobStatus.success,
            JobStatus.failed,
            JobStatus.timeout,
            JobStatus.cancelled,
        ]), "index.html")
