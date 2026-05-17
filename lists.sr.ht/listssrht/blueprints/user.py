from email.mime.text import MIMEText
from email.utils import parseaddr, formatdate, make_msgid
from flask import current_app, Blueprint, render_template, request, redirect, url_for, abort
from flask import session
from srht.app import paginate_query, get_profile
from srht.config import cfg, cfgi
from srht.database import db
from srht.oauth import UserType, current_user, loginrequired
from srht.search import search_by
from srht.validation import Validation
from sqlalchemy import or_
from sqlalchemy import nullslast
from listssrht.graphql import Client, Visibility, PreferencesInput
from listssrht.types import List, ListAccess, User, Email, Subscription, Mirror
import re
import smtplib

user = Blueprint("user", __name__)

smtp_host = cfg("mail", "smtp-host", default=None)
smtp_port = cfgi("mail", "smtp-port", default=None)
smtp_user = cfg("mail", "smtp-user", default=None)
smtp_password = cfg("mail", "smtp-password", default=None)

@user.route("/")
def index():
    if not current_user:
        return render_template("index.html")
    recent = (Email.query
            .join(List)
            .join(Subscription)
            .filter(Email.list_id == List.id)
            .filter(Subscription.list_id == List.id)
            .filter(Subscription.user_id == current_user.id)
            .order_by(Email.created.desc())).limit(10).all()
    subs = [sub.list for sub in (Subscription.query
            .join(List)
            .filter(Subscription.user_id == current_user.id)
            .order_by(nullslast(List.last_activity.desc()))).limit(10).all()]
    notice = session.pop("notice", None)
    client = Client()
    copy_self = client.get_preferences().preferences.copy_self
    return render_template("dashboard.html", recent=recent,
            subs=subs, notice=notice, copy_self=copy_self)

@user.route("/", methods=["POST"])
@loginrequired
def index_POST():
    valid = Validation(request)
    copy_self = valid.optional("copy-self", default=False)
    client = Client()
    client.update_preferences(PreferencesInput(copy_self=copy_self))
    session["prefs_updated"] = True
    return redirect(url_for("user.index"))

@user.route("/~<username>")
def user_profile(username):
    user = User.query.filter(User.username == username).one_or_none()
    if not user:
        abort(404)

    lists = List.query.filter(List.owner_id == user.id)

    if not current_user or current_user.id != user.id:
        lists = lists.filter(List.visibility == Visibility.PUBLIC)

    lists = lists.order_by(nullslast(List.last_activity.desc()))
    terms = request.args.get('search')
    if terms:
        lists = search_by(lists, terms, [List.name, List.description], {})
    lists, pagination = paginate_query(lists)

    return render_template("profile-lists.html",
            user=user, lists=lists, search=terms,
            profile=get_profile(user), view="lists",
            **pagination)

# Deprecated route
@user.route("/lists/~<username>")
def lists_for_user(username):
    return redirect(url_for(".user_profile", username=username))

@user.route("/lists/create")
@loginrequired
def create_list_GET():
    if (cfg("lists.sr.ht", "allow-new-lists", default="yes") != "yes"
            and current_user.user_type != UserType.admin):
        abort(401)
    return render_template("create.html")

@user.route("/lists/create", methods=["POST"])
def create_list_POST():
    if (cfg("lists.sr.ht", "allow-new-lists", default="yes") != "yes"
            and current_user.user_type != UserType.admin):
        abort(401)

    valid = Validation(request)
    name = valid.require("name", friendly_name="Name")
    desc = valid.optional("description")
    visibility = valid.require("visibility", cls=Visibility)
    if not valid.ok:
        return render_template("create.html", **valid.kwargs)

    client = Client()

    with valid:
        mailing_list = client.create_mailing_list(name, visibility, desc).mailing_list
    if not valid.ok:
        return render_template("create.html", **valid.kwargs)

    return redirect(url_for("archives.archive",
            owner_name=mailing_list.owner.canonical_name,
            list_name=mailing_list.name))

@user.route("/lists/create-mirror")
@loginrequired
def create_mirror_GET():
    return render_template("create-mirror.html")

def mirror_subscribe(ml, mirror):
    posting_domain = cfg("lists.sr.ht", "posting-domain")
    list_name = "u.{}.{}".format(ml.owner.username, ml.name)

    smtp = smtplib.SMTP(smtp_host, smtp_port)
    smtp.ehlo()
    smtp.starttls()
    smtp.login(smtp_user, smtp_password)

    mail = MIMEText(f"Subscription request for {posting_domain} on behalf of "
        f"{ml.owner.canonical_name}\n\n"
        "If this email is unexpected, feel free to ignore it, or send "
        "questions to:\n\n"
        f"{cfg('sr.ht', 'owner-name')} <{cfg('sr.ht', 'owner-email')}>")
    mail["X-Mirroring-To"] = posting_domain
    mail["Subject"] = "subscribe"
    mail["To"] = mirror.list_subscribe
    mail["From"] = f"{posting_domain} mirror <{list_name}@{posting_domain}>"
    mail["Date"] = formatdate()
    mail["Message-ID"] = make_msgid()
    smtp.sendmail(smtp_user, [mirror.list_subscribe], mail.as_string(
        unixfrom=True, maxheaderlen=998))
    smtp.quit()

@user.route("/lists/create-mirror", methods=["POST"])
@loginrequired
def create_mirror_POST():
    valid = Validation(request)
    ml = List(current_user, valid)
    address = valid.require("address", friendly_name="Subscription address")
    valid.expect(not address or "@" in address,
            "A valid email address is required", field="address")
    weird_ok = valid.optional("weird-email-okay")
    valid.expect(not address or weird_ok == "yes" or "subscribe" in address,
            "This address does not look like a subscription address. Double "
            "check it and click 'Create' if you're certain.", field="address")
    if not valid.ok:
        return render_template("create-mirror.html", **valid.kwargs)

    posting_domain = cfg("lists.sr.ht", "posting-domain")

    user, domain = address.split("@")
    valid.expect(domain != posting_domain,
            "You can't mirror a list from {{cfg('sr.ht', 'site-name')}}!",
            field="address")
    if not valid.ok:
        return render_template("create-mirror.html", **valid.kwargs)

    mirror = Mirror()
    mirror.list_subscribe = address
    db.session.add(mirror)
    db.session.flush()
    ml.mirror_id = mirror.id

    mirror_subscribe(ml, mirror)

    db.session.commit()
    return redirect(url_for("archives.archive",
            owner_name=current_user.canonical_name,
            list_name=ml.name))
