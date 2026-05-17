import socket
from datetime import datetime, timedelta
from flask import Blueprint, render_template, request, redirect, url_for, abort
from flask import session
from metasrht.decorators import adminrequired
from metasrht.audit import audit_log
from metasrht.email import send_email
from metasrht.graphql import Client, PaymentStatus, PaymentInterval
from metasrht.types import User, UserAuthFactor, FactorType, AuditLogEntry
from metasrht.types import UserNote
from metasrht.webhooks import UserWebhook, deliver_profile_update
from sqlalchemy import and_
from srht.app import paginate_query
from srht.config import cfg
from srht.database import db
from srht.graphql import InternalAuth, DATE_FORMAT
from srht.oauth import UserType, login_user, current_user
from srht.search import search_by
from srht.validation import Validation
from string import Template

users = Blueprint("users", __name__)

@users.route("/users")
@adminrequired
def users_GET():
    terms = request.args.get("search")
    users = User.query.order_by(User.created.desc())

    search_error = None
    try:
        users = search_by(users, terms, [User.username, User.email])
    except ValueError as ex:
        search_error = str(ex)

    users, pagination = paginate_query(users)
    return render_template("users.html",
            users=users, search=terms, search_error=search_error, **pagination)

def render_user_template(user, **kwargs):
    totp = (UserAuthFactor.query
        .filter(UserAuthFactor.user_id == user.id)
        .filter(UserAuthFactor.factor_type == FactorType.totp)).one_or_none()
    audit_log = (AuditLogEntry.query
        .filter(AuditLogEntry.user_id == user.id)
        .order_by(AuditLogEntry.created.desc())).limit(15)

    rdns = dict()
    for log in audit_log:
        addr = str(log.ip_address)
        if addr not in rdns:
            try:
                host, _, _ = socket.gethostbyaddr(addr)
                rdns[addr] = host
            except (socket.herror, socket.gaierror):
                continue
    one_year = datetime.utcnow()
    if one_year.month == 2 and one_year.day == 29:
        # leap day
        one_year = datetime(year=one_year.year + 1,
                month=one_year.month, day=one_year.day - 1)
    else:
        one_year = datetime(year=one_year.year + 1,
                month=one_year.month, day=one_year.day)

    details = Client(InternalAuth(user)).get_user_admin()
    return render_template("user.html", user=user,
            totp=totp, audit_log=audit_log,
            one_year=one_year, rdns=rdns,
            personal_tokens=details.personal_access_tokens,
            oauth_clients=details.oauth_clients,
            oauth_grants=details.oauth_grants,
            invoices=details.user.invoices.results,
            sub=details.user.subscription,
            **kwargs)

@users.route("/users/~<username>")
@adminrequired
def user_by_username_GET(username):
    user = User.query.filter(User.username == username).one_or_none()
    if not user:
        abort(404)
    return render_user_template(user)

@users.route("/users/~<username>/add-note", methods=["POST"])
@adminrequired
def user_add_note(username):
    user = User.query.filter(User.username == username).one_or_none()
    if not user:
        abort(404)
    valid = Validation(request)
    notes = valid.require("notes")
    if not valid.ok:
        return render_user_template(user)
    note = UserNote()
    note.user_id = user.id
    note.note = notes
    db.session.add(note)
    db.session.commit()
    return redirect(url_for(".user_by_username_GET", username=username))

@users.route("/users/~<from_user>/transfer-billing", methods=["POST"])
@adminrequired
def user_transfer_billing(from_user):
    valid = Validation(request)
    to_user = valid.require("to")
    if not valid.ok:
        user = User.query.filter(User.username == from_user).one_or_none()
        return render_user_template(user, **valid.kwargs)

    with valid:
        user = Client().transfer_billing_info(from_user, to_user).user

    if not valid.ok:
        user = User.query.filter(User.username == from_user).one_or_none()
        return render_user_template(user, **valid.kwargs)

    return redirect(url_for(".user_by_username_GET", username=user.username))

@users.route("/users/~<username>/disable-totp", methods=["POST"])
@adminrequired
def user_disable_totp(username):
    user = User.query.filter(User.username == username).one_or_none()
    if not user:
        abort(404)
    UserAuthFactor.query.filter(UserAuthFactor.user_id == user.id).delete()
    db.session.commit()
    return redirect(url_for(".user_by_username_GET", username=username))

@users.route("/users/~<username>/set-type", methods=["POST"])
@adminrequired
def set_user_type(username):
    user = User.query.filter(User.username == username).one_or_none()
    if not user:
        abort(404)

    valid = Validation(request)
    user_type = valid.require("user_type", cls=UserType)
    if not valid.ok:
        return redirect(url_for(".user_by_username_GET", username=username))

    user.user_type = user_type
    db.session.commit()

    deliver_profile_update(user)
    return redirect(url_for(".user_by_username_GET", username=username))

@users.route("/users/~<username>/suspend", methods=["POST"])
@adminrequired
def user_suspend(username):
    user = User.query.filter(User.username == username).one_or_none()
    if not user:
        abort(404)
    valid = Validation(request)
    reason = valid.optional("reason")
    user.user_type = UserType.suspended
    user.suspension_notice = reason
    db.session.commit()
    deliver_profile_update(user)
    return redirect(url_for(".user_by_username_GET", username=username))

@users.route("/users/<uid>/subsidize", methods=["POST"])
@adminrequired
def user_subsidize(uid):
    valid = Validation(request)
    valid_thru = valid.require("valid_thru")
    if not valid.ok:
        abort(400)

    valid_thru = datetime.strptime(valid_thru, "%Y-%m-%d")
    user = Client().subsidize_user(uid, valid_thru.strftime(DATE_FORMAT)).user
    return redirect(url_for(".user_by_username_GET", username=user.username))

@users.route("/users/~<username>/impersonate", methods=["POST"])
@adminrequired
def user_impersonate_POST(username):
    user = User.query.filter(User.username == username).one_or_none()
    if not user:
        abort(404)
    valid = Validation(request)
    reason = valid.require("reason", friendly_name="Reason")
    if not valid.ok:
        return redirect(url_for(".user_by_username_GET", username=username))

    details = f"admin log-in from {current_user.canonical_name}: {reason}"
    audit_log(details, details=details, user=user, email=True,
            subject="A sourcehut administrator has logged into your account",
            email_details=details)

    security_addr = cfg("sr.ht", "security-address", default=None)
    if security_addr is not None:
        tmpl = Template("""Subject: A sourcehut admin has impersonated another user

Administrator $admin_user has impersonated $target_user for the following reason:

$reason""")
        rendered = tmpl.substitute(**{
                'admin_user': current_user.canonical_name,
                'target_user': user.canonical_name,
                'reason': reason,
            })
        send_email(security_addr, rendered)

    note = UserNote()
    note.user_id = user.id
    note.note = f"Admin {current_user.canonical_name} impersonated this user: {reason}"
    db.session.add(note)
    db.session.commit()

    login_user(user, set_cookie=True)
    return redirect("/")

@users.route("/users/~<username>/delete", methods=["POST"])
@adminrequired
def user_delete_POST(username):
    if request.form.get("safe-1") != "on":
        return redirect(url_for(".user_by_username_GET", username=username))
    if request.form.get("safe-2") != "on":
        return redirect(url_for(".user_by_username_GET", username=username))

    user = User.query.filter(User.username == username).one_or_none()
    Client(InternalAuth(user)).delete_user(False)

    session["notice"] = "This user account is being deleted."
    return redirect("/")
