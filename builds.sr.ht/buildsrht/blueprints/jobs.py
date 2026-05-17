from ansi2html import Ansi2HTMLConverter
from buildsrht.graphql import Client, Visibility
from buildsrht.manifest import Manifest
from buildsrht.meta import MetaClient
from buildsrht.rss import generate_feed
from buildsrht.search import apply_search
from buildsrht.types import Job, JobStatus, Task, TaskStatus, User
from datetime import datetime, timedelta
from flask import Blueprint, render_template, request, abort, redirect
from flask import Response, url_for
from markupsafe import Markup, escape
from prometheus_client import Counter
from srht.app import paginate_query, session
from srht.cache import get_cache, set_cache
from srht.config import cfg, get_origin
from srht.crypto import encrypt_request_authorization
from srht.database import db
from srht.oauth import current_user, loginrequired, UserType
from srht.redis import redis
from srht.validation import Validation
import sqlalchemy as sa
import hashlib
import requests
import yaml
import json
import textwrap

jobs = Blueprint("jobs", __name__)
allow_free = cfg("builds.sr.ht", "allow-free", default="no") == "yes"

metrics = type("metrics", tuple(), {
    c.describe()[0].name: c
    for c in [
        Counter("buildsrht_logcache_hit", "Number of log cache hits"),
        Counter("buildsrht_logcache_miss", "Number of log cache misses"),
    ]
})

requests_session = requests.Session()

def user_can_submit():
    if allow_free:
        return True
    return MetaClient().can_submit_builds().me.receives_paid_services

def get_access(job, user=None):
    user = user or current_user

    # Anonymous
    if not user:
        if job.visibility == Visibility.PRIVATE:
            return False
        return True

    if user.user_type == UserType.admin:
        return True

    # Owner
    if user.id == job.owner_id:
        return True

    if job.visibility == Visibility.PRIVATE:
        return False
    return True

def tags(tags):
    if not tags:
        return list()
    previous = list()
    results = list()
    for tag in tags.split("/"):
        results.append({
            "name": tag,
            "url": "/" + "/".join(previous + [tag])
        })
        previous.append(tag)
    return results

status_map = {
    JobStatus.pending: "text-info",
    JobStatus.queued: "text-info",
    JobStatus.success: "text-success",
    JobStatus.failed: "text-danger",
    JobStatus.running: "text-info icon-spin",
    JobStatus.timeout: "text-danger",
    JobStatus.cancelled: "text-warning",
    TaskStatus.success: "text-success",
    TaskStatus.failed: "text-danger",
    TaskStatus.running: "text-primary icon-spin",
    TaskStatus.pending: "text-info",
    TaskStatus.skipped: "text-muted",
}

icon_map = {
    JobStatus.pending: "clock",
    JobStatus.queued: "clock",
    JobStatus.success: "check",
    JobStatus.failed: "times",
    JobStatus.running: "circle-notch",
    JobStatus.timeout: "clock",
    JobStatus.cancelled: "times",
    TaskStatus.success: "check",
    TaskStatus.failed: "times",
    TaskStatus.running: "circle-notch",
    TaskStatus.pending: "circle",
    TaskStatus.skipped: "minus",
}

def get_jobs(jobs, terms):
    jobs = jobs.order_by(Job.created.desc())
    if terms:
        jobs = apply_search(jobs, terms)
    return jobs

def jobs_for_feed(jobs):
    terms = request.args.get("search")
    try:
        jobs = get_jobs(jobs, terms)
    except ValueError:
        jobs = jobs.filter(False)

    if terms is not None and "status:" not in terms:
        # by default, return only terminated jobs in feed
        terminated_statuses = [
            JobStatus.success,
            JobStatus.cancelled,
            JobStatus.failed,
            JobStatus.timeout,
        ]
        jobs = jobs.filter(Job.status.in_(terminated_statuses))
    return jobs, terms

def jobs_page(jobs, sidebar="sidebar.html", **kwargs):
    search = request.args.get("search")
    search_error = None

    try:
        jobs = (get_jobs(jobs, search))
    except ValueError as ex:
        search_error = str(ex)

    jobs = jobs.options(sa.orm.joinedload(Job.tasks))
    jobs, pagination = paginate_query(jobs)
    return render_template("jobs.html",
        jobs=jobs, status_map=status_map, icon_map=icon_map, tags=tags,
        sort_tasks=lambda tasks: sorted(tasks, key=lambda t: t.id),
        sidebar=sidebar, search=search, search_error=search_error,
        **pagination, **kwargs)

def jobs_feed(jobs, title, endpoint, **urlvalues):
    jobs, terms = jobs_for_feed(jobs)
    if terms is not None:
        title = f"{title} (filtered by: {terms})"
    description = title
    origin = cfg("builds.sr.ht", "origin")
    assert "search" not in urlvalues
    if terms is not None:
        urlvalues["search"] = terms
    link = origin + url_for(endpoint, **urlvalues)
    jobs=jobs.options(sa.orm.joinedload(Job.owner))
    return generate_feed(jobs, title, link, description)

badge_success = """
<svg width="107.6" height="20" viewBox="0 0 1076 200" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" role="img" aria-label="builds: success"><title>builds: success</title><linearGradient id="IQLCU" x2="0" y2="100%"><stop offset="0" stop-opacity=".1" stop-color="#EEE"/><stop offset="1" stop-opacity=".1"/></linearGradient><mask id="udkBp"><rect width="1076" height="200" rx="30" fill="#FFF"/></mask><g mask="url(#udkBp)"><rect width="555" height="200" fill="#555"/><rect width="521" height="200" fill="#3C1" x="555"/><rect width="1076" height="200" fill="url(#IQLCU)"/></g><g aria-hidden="true" fill="#fff" text-anchor="start" font-family="Verdana,DejaVu Sans,sans-serif" font-size="110"><text x="190" y="148" textLength="325" fill="#000" opacity="0.25">builds</text><text x="180" y="138" textLength="325">builds</text><text x="610" y="148" textLength="421" fill="#000" opacity="0.25">success</text><text x="600" y="138" textLength="421">success</text></g><image x="40" y="35" width="100" height="130" xlink:href="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIGZpbGw9IiNmZmYiIHZpZXdCb3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTEyIDBDNS4zOSAwIDAgNS4zOSAwIDEyczUuMzkgMTIgMTIgMTIgMTItNS4zOSAxMi0xMlMxOC42MSAwIDEyIDBabTAgMi45NDdBOS4wMyA5LjAzIDAgMCAxIDIxLjA1MyAxMiA5LjAzIDkuMDMgMCAwIDEgMTIgMjEuMDUzIDkuMDMgOS4wMyAwIDAgMSAyLjk0NyAxMiA5LjAzIDkuMDMgMCAwIDEgMTIgMi45NDd6Ii8+PC9zdmc+"/></svg>
"""

badge_failure = """
<svg width="100.3" height="20" viewBox="0 0 1003 200" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" role="img" aria-label="builds: failure"><title>builds: failure</title><linearGradient id="njRfc" x2="0" y2="100%"><stop offset="0" stop-opacity=".1" stop-color="#EEE"/><stop offset="1" stop-opacity=".1"/></linearGradient><mask id="gcQxg"><rect width="1003" height="200" rx="30" fill="#FFF"/></mask><g mask="url(#gcQxg)"><rect width="555" height="200" fill="#555"/><rect width="448" height="200" fill="#e05d44" x="555"/><rect width="1003" height="200" fill="url(#njRfc)"/></g><g aria-hidden="true" fill="#fff" text-anchor="start" font-family="Verdana,DejaVu Sans,sans-serif" font-size="110"><text x="190" y="148" textLength="325" fill="#000" opacity="0.25">builds</text><text x="180" y="138" textLength="325">builds</text><text x="610" y="148" textLength="348" fill="#000" opacity="0.25">failure</text><text x="600" y="138" textLength="348">failure</text></g><image x="40" y="35" width="100" height="130" xlink:href="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIGZpbGw9IiNmZmYiIHZpZXdCb3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTEyIDBDNS4zOSAwIDAgNS4zOSAwIDEyczUuMzkgMTIgMTIgMTIgMTItNS4zOSAxMi0xMlMxOC42MSAwIDEyIDBabTAgMi45NDdBOS4wMyA5LjAzIDAgMCAxIDIxLjA1MyAxMiA5LjAzIDkuMDMgMCAwIDEgMTIgMjEuMDUzIDkuMDMgOS4wMyAwIDAgMSAyLjk0NyAxMiA5LjAzIDkuMDMgMCAwIDEgMTIgMi45NDd6Ii8+PC9zdmc+"/></svg>
"""

badge_unknown = """
<svg width="115.7" height="20" viewBox="0 0 1157 200" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" role="img" aria-label="builds: unknown"><title>builds: unknown</title><linearGradient id="sTWot" x2="0" y2="100%"><stop offset="0" stop-opacity=".1" stop-color="#EEE"/><stop offset="1" stop-opacity=".1"/></linearGradient><mask id="STygr"><rect width="1157" height="200" rx="30" fill="#FFF"/></mask><g mask="url(#STygr)"><rect width="555" height="200" fill="#555"/><rect width="602" height="200" fill="#9f9f9f" x="555"/><rect width="1157" height="200" fill="url(#sTWot)"/></g><g aria-hidden="true" fill="#fff" text-anchor="start" font-family="Verdana,DejaVu Sans,sans-serif" font-size="110"> <text x="190" y="148" textLength="325" fill="#000" opacity="0.25">builds</text><text x="180" y="138" textLength="325">builds</text><text x="610" y="148" textLength="502" fill="#000" opacity="0.25">unknown</text><text x="600" y="138" textLength="502">unknown</text></g><image x="40" y="35" width="100" height="130" xlink:href="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIGZpbGw9IiNmZmYiIHZpZXdCb3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTEyIDBDNS4zOSAwIDAgNS4zOSAwIDEyczUuMzkgMTIgMTIgMTIgMTItNS4zOSAxMi0xMlMxOC42MSAwIDEyIDBabTAgMi45NDdBOS4wMyA5LjAzIDAgMCAxIDIxLjA1MyAxMiA5LjAzIDkuMDMgMCAwIDEgMTIgMjEuMDUzIDkuMDMgOS4wMyAwIDAgMSAyLjk0NyAxMiA5LjAzIDkuMDMgMCAwIDEgMTIgMi45NDd6Ii8+PC9zdmc+"/></svg>
"""

def svg_page(jobs):
    job = (get_jobs(jobs, None)
        .filter(Job.status.in_([
            JobStatus.success,
            JobStatus.failed,
            JobStatus.timeout]))
        .first())
    if not job:
        return badge_unknown
    elif job.status == JobStatus.success:
        return badge_success

    return badge_failure

@jobs.route("/")
def index():
    if not current_user:
        return render_template("index-logged-out.html")
    origin = cfg("builds.sr.ht", "origin")
    rss_feed = {
        "title": f"{current_user.username}'s jobs",
        "url": origin + url_for("jobs.user_rss",
                                username=current_user.username,
                                search=request.args.get("search")),
    }
    return jobs_page(
            Job.query.filter(Job.owner_id == current_user.id),
            "index.html", rss_feed=rss_feed)

@jobs.route("/submit")
@loginrequired
def submit_GET():
    if request.args.get("manifest"):
        manifest = request.args.get("manifest")
    else:
        manifest = session.pop("manifest", default=None)
    if request.args.get("note"):
        note = request.args.get("note")
    else:
        note = session.pop("note", default=None)
    note_rows = len(note.splitlines()) if isinstance(note, str) else 1
    status = 200

    can_submit = user_can_submit()
    if not can_submit:
        status = 402

    return render_template("submit.html",
            manifest=manifest,
            note=note,
            note_rows=note_rows,
            can_submit=can_submit), status

def addsuffix(note: str, suffix: str) -> str:
    """
    Given a note and a suffix, return the note with the suffix concatenated/

    The returned string is guaranteed to fit in the Job.note DB field.
    """
    maxlen = Job.note.prop.columns[0].type.length
    assert len(suffix) + 1 <= maxlen, f"Suffix was too long ({len(suffix)})"
    if note.endswith(suffix) or not note:
        return note
    result = f"{note} {suffix}"
    if len(result) <= maxlen:
        return result
    note = textwrap.shorten(note, maxlen - len(suffix) - 1, placeholder="…")
    return f"{note} {suffix}"

@jobs.route("/resubmit/<int:job_id>")
@loginrequired
def resubmit_GET(job_id):
    job = Job.query.filter(Job.id == job_id).one_or_none()
    if not job:
        abort(404)
    if not get_access(job):
        abort(404)
    session["manifest"] = job.manifest
    if isinstance(job.note, str) and len(job.note.splitlines()) == 1:
        note = addsuffix(job.note, "(resubmitted)")
    else:
        note = job.note
    session["note"] = note
    return redirect("/submit")

@jobs.route("/submit", methods=["POST"])
@loginrequired
def submit_POST():
    can_submit = user_can_submit()

    valid = Validation(request)
    _manifest = valid.require("manifest", friendly_name="Manifest")
    max_len = Job.manifest.prop.columns[0].type.length
    note = valid.optional("note", default="Submitted on the web")
    tags_string = valid.optional("tags", default="")
    tag_list = [ t["name"] for t in tags(tags_string) ]
    valid.expect(not _manifest or len(_manifest) < max_len,
            "Manifest must be less than {} bytes".format(max_len),
            field="manifest")
    visibility = valid.require("visibility")

    if not valid.ok:
        return render_template("submit.html",
            can_submit=can_submit, **valid.kwargs)

    try:
        _manifest = _manifest.replace("\r\n", "\n")
        manifest = Manifest(yaml.safe_load(_manifest))
    except Exception as ex:
        valid.error(str(ex), field="manifest")
        return render_template("submit.html",
            can_submit=can_submit, **valid.kwargs)

    with valid:
        job = Client().submit_build(_manifest, note, visibility, tag_list).job

    if not valid.ok:
        return render_template("submit.html",
            can_submit=can_submit, **valid.kwargs)

    return redirect(url_for("jobs.job_by_id",
        username=current_user.username, job_id=job.id))

@jobs.route("/cancel/<int:job_id>", methods=["POST"])
@loginrequired
def cancel(job_id):
    job = Job.query.filter(Job.id == job_id).one_or_none()
    if not job:
        abort(404)
    if job.owner_id != current_user.id and current_user.user_type != UserType.admin:
        abort(401)
    requests_session.post(f"http://{job.runner}/job/{job.id}/cancel")
    return redirect("/~" + current_user.username + "/job/" + str(job.id))

@jobs.route("/~<username>")
def user(username):
    user = User.query.filter(User.username == username).first()
    if not user:
        abort(404)
    jobs = Job.query.filter(Job.owner_id == user.id)
    if not current_user or user.id != current_user.id:
        jobs = jobs.filter(Job.visibility == Visibility.PUBLIC)
    origin = cfg("builds.sr.ht", "origin")
    rss_feed = {
        "title": f"{user.username}'s jobs",
        "url": origin + url_for("jobs.user_rss", username=username,
                                search=request.args.get("search")),
    }
    return jobs_page(jobs, user=user, breadcrumbs=[
        { "name": "~" + user.username, "link": "" }
    ], rss_feed=rss_feed)

@jobs.route("/~<username>/rss.xml")
def user_rss(username):
    user = User.query.filter(User.username == username).first()
    if not user:
        abort(404)
    jobs = Job.query.filter(Job.owner_id == user.id)
    if not current_user or user.id != current_user.id:
        jobs = jobs.filter(Job.visibility == Visibility.PUBLIC)
    return jobs_feed(jobs, f"{user.username}'s jobs",
                     "jobs.user", username=username)

@jobs.route("/~<username>.svg")
def user_svg(username):
    key = f"builds.sr.ht.svg.user.{username}"
    badge = redis.get(key)
    if not badge:
        user = User.query.filter(User.username == username).first()
        if not user:
            abort(404)
        jobs = Job.query.filter(Job.owner_id == user.id)
        badge = svg_page(jobs).encode()
        redis.setex(key, timedelta(seconds=30), badge)
    return Response(badge, mimetype="image/svg+xml", headers={
        "Cache-Control": "no-cache",
        "ETag": hashlib.sha1(badge).hexdigest(),
    })

@jobs.route("/~<username>/<path:path>")
def tag(username, path):
    user = User.query.filter(User.username == username).first()
    if not user:
        abort(404)
    jobs = Job.query.filter(Job.owner_id == user.id)\
        .filter(Job.tags.ilike(path + "%"))
    if not current_user or current_user.id != user.id:
        jobs = jobs.filter(Job.visibility == Visibility.PUBLIC)
    origin = cfg("builds.sr.ht", "origin")
    rss_feed = {
        "title": "/".join([f"~{user.username}"] +
                          [t["name"] for t in tags(path)]),
        "url": origin + url_for("jobs.tag_rss", username=username, path=path,
                                search=request.args.get("search")),
    }
    return jobs_page(jobs, user=user, breadcrumbs=[
        { "name": "~" + user.username, "url": "" }
    ] + tags(path), rss_feed=rss_feed)

@jobs.route("/~<username>/<path:path>/rss.xml")
def tag_rss(username, path):
    user = User.query.filter(User.username == username).first()
    if not user:
        abort(404)
    jobs = Job.query.filter(Job.owner_id == user.id)\
        .filter(Job.tags.ilike(path + "%"))
    if not current_user or current_user.id != user.id:
        jobs = jobs.filter(Job.visibility == Visibility.PUBLIC)
    base_title = "/".join([f"~{user.username}"] +
                          [t["name"] for t in tags(path)])
    return jobs_feed(jobs, base_title + " jobs",
                     "jobs.tag", username=username, path=path)

@jobs.route("/~<username>/<path:path>.svg")
def tag_svg(username, path):
    key = f"builds.sr.ht.svg.tag.{username}/{path}"
    badge = redis.get(key)
    if not badge:
        user = User.query.filter(User.username == username).first()
        if not user:
            abort(404)
        jobs = Job.query.filter(Job.owner_id == user.id)\
            .filter(Job.tags.ilike(path + "%"))
        badge = svg_page(jobs).encode()
        redis.setex(key, timedelta(seconds=30), badge)
    return Response(badge, mimetype="image/svg+xml", headers={
        "Cache-Control": "no-cache",
        "ETag": hashlib.sha1(badge).hexdigest(),
    })

log_max = 131072

ansi = Ansi2HTMLConverter(scheme="mint-terminal", linkify=True)

def logify(text, task, log_url):
    text = ansi.convert(text, full=False)
    if len(text) >= log_max:
        text = text[-log_max:]
        try:
            text = text[text.index('\n')+1:]
        except ValueError:
            pass
        nlines = len(text.splitlines())
        text = (Markup('<pre>')
                + Markup('<span class="text-muted">'
                    'This is a big file! Only the last 128KiB is shown. '
                    f'<a target="_blank" href="{escape(log_url)}">'
                        'Click here to download the full log</a>.'
                    '</span>\n\n')
                + Markup(text)
                + Markup('</pre>'))
        linenos = Markup('<pre>\n\n\n')
    else:
        nlines = len(text.splitlines())
        text = (Markup('<pre>')
                + Markup(text)
                + Markup('</pre>'))
        linenos = Markup('<pre>')
    for no in range(1, nlines + 1):
        linenos += Markup(f'<a href="#{escape(task)}-{no-1}" '
                + f'id="{escape(task)}-{no-1}">{no}</a>')
        if no != nlines:
            linenos += Markup("\n")
    linenos += Markup("</pre>")
    return (Markup('<td>')
            + linenos
            + Markup('</td><td>')
            + Markup(ansi.produce_headers())
            + text
            + Markup('</td>'))

@jobs.route("/~<username>/job/<int:job_id>")
def job_by_id(username, job_id):
    user = User.query.filter(User.username == username).first()
    if not user:
        abort(404)
    job = Job.query.options(sa.orm.joinedload(Job.tasks)).get(job_id)
    if not job:
        abort(404)
    if not get_access(job):
        abort(404)
    if job.owner_id != user.id:
        abort(404)
    logs = list()
    build_user = cfg("builds.sr.ht", "ssh-user", "builds")
    final_status = [
        TaskStatus.success,
        TaskStatus.failed,
        TaskStatus.skipped,
        JobStatus.success,
        JobStatus.timeout,
        JobStatus.failed,
        JobStatus.cancelled,
    ]
    def get_log(log_url, name, status):
        cachekey = f"builds.sr.ht:logs:{log_url}"
        log = get_cache(cachekey)
        if log:
            metrics.buildsrht_logcache_hit.inc()
            log = json.loads(log)
            log["log"] = Markup(log["log"])
        if not log:
            metrics.buildsrht_logcache_miss.inc()
            try:
                r = requests_session.head(log_url,
                  headers=encrypt_request_authorization())
                cl = int(r.headers["Content-Length"])
                if cl > log_max:
                    r = requests_session.get(log_url, headers={
                        "Range": f"bytes={cl-log_max}-{cl-1}",
                        **encrypt_request_authorization(),
                    }, timeout=3)
                else:
                    r = requests_session.get(log_url, timeout=3,
                        headers=encrypt_request_authorization())
                if r.status_code >= 200 and r.status_code <= 299:
                    log = {
                        "name": name,
                        "log": logify(r.content.decode('utf-8', errors='replace'),
                            "task-" + name if name else "setup", log_url),
                        "more": True,
                    }
                    if status in final_status:
                        set_cache(cachekey, timedelta(days=2), json.dumps(log))
                else:
                    raise Exception()
            except:
                log = {
                    "name": name,
                    "log": Markup('<td></td><td><pre><strong class="text-danger">'
                        f'Error fetching logs for task "{escape(name)}"</strong>'
                        '</pre></td>'),
                    "more": False,
                }
        logs.append(log)
        return log["more"]
    origin = cfg("builds.sr.ht", "api-origin", default=get_origin("builds.sr.ht"))
    log_url = f"{origin}/query/log/{job.id}/log"
    if get_log(log_url, None, job.status):
        for task in sorted(job.tasks, key=lambda t: t.id):
            if task.status == TaskStatus.pending:
                continue
            log_url = f"{origin}/query/log/{job.id}/{task.name}/log"
            if not get_log(log_url, task.name, task.status):
                break
    min_artifact_date = datetime.utcnow() - timedelta(days=90)
    if current_user:
        can_submit = user_can_submit()
    else:
        can_submit = False
    return render_template("job.html",
            job=job, logs=logs,
            build_user=build_user,
            status_map=status_map,
            icon_map=icon_map,
            sort_tasks=lambda tasks: sorted(tasks, key=lambda t: t.id),
            min_artifact_date=min_artifact_date,
            can_submit=can_submit)

@jobs.route("/~<username>/job/<int:job_id>/manifest")
def manifest_by_job_id(username, job_id):
    user = User.query.filter(User.username == username).first()
    if not user:
        abort(404)
    job = Job.query.options(sa.orm.joinedload(Job.tasks)).get(job_id)
    if not job:
        abort(404)
    if not get_access(job):
        abort(404)
    if job.owner_id != user.id:
        abort(404)
    return Response(job.manifest, mimetype="text/plain")
