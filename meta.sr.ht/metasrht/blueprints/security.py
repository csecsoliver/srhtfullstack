from flask import Blueprint, render_template, request, redirect, abort, url_for
from metasrht.audit import audit_log
from metasrht.auth.builtin import hash_password
from metasrht.graphql import Client
from metasrht.qrcode import gen_qr
from metasrht.totp import totp
from metasrht.types import User, UserAuthFactor, FactorType, AuditLogEntry
from prometheus_client import Counter
from srht.app import session
from srht.config import cfg
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.validation import Validation, valid_url
from urllib.parse import quote
import base64
import os

security = Blueprint('security', __name__)

site_name = cfg("sr.ht", "site-name")

metrics = type("metrics", tuple(), {
    c.describe()[0].name: c
    for c in [
        Counter("meta_totp_enabled", "Number of times TOTP was disabled for a user"),
        Counter("meta_totp_disabled", "Number of times TOTP was enabled for a user"),
    ]
})

@security.route("/security")
@loginrequired
def security_GET():
    totp = (UserAuthFactor.query
        .filter(UserAuthFactor.user_id == current_user.id)
        .filter(UserAuthFactor.factor_type == FactorType.totp)).one_or_none()
    audit_log = (AuditLogEntry.query
        .filter(AuditLogEntry.user_id == current_user.id)
        .order_by(AuditLogEntry.created.desc())).limit(15)
    return render_template("security.html",
        audit_log=audit_log,
        totp=totp)

@security.route("/security/audit/log")
@loginrequired
def security_audit_log_GET():
    audit_log = (AuditLogEntry.query
        .filter(AuditLogEntry.user_id == current_user.id)
        .order_by(AuditLogEntry.created.desc())).all()
    return render_template("audit-log.html", audit_log=audit_log)

def totp_get_qrcode(secret):
    return gen_qr(otpauth_uri(secret))

def otpauth_uri(secret):
    return "otpauth://totp/{}:{}?secret={}&issuer={}".format(
        quote(site_name), quote("{} <{}>".format(current_user.username,
            current_user.email)), secret, quote(site_name))

@security.route("/security/totp/enable")
@loginrequired
def security_totp_enable_GET():
    secret = base64.b32encode(os.urandom(20)).decode('utf-8')
    return render_template("totp-enable.html",
        qrcode=totp_get_qrcode(secret),
        otpauth_uri=otpauth_uri(secret),
        secret=secret)

@security.route("/security/totp/enable", methods=["POST"])
@loginrequired
def security_totp_enable_POST():
    valid = Validation(request)

    secret = valid.require("secret")
    code = valid.require("code")

    if not valid.ok:
        return render_template("totp-enable.html",
            qrcode=totp_get_qrcode(secret),
            otpauth_uri=otpauth_uri(secret),
            secret=secret, valid=valid), 400
    code = code.replace(" ", "")
    try:
        code = int(code)
    except:
        valid.error(
                "This TOTP code is invalid (expected a number)", field="code")
    if not valid.ok:
        return render_template("totp-enable.html",
            qrcode=totp_get_qrcode(secret),
            otpauth_uri=otpauth_uri(secret),
            secret=secret, valid=valid), 400

    valid.expect(totp(secret, code),
            "The code you entered is incorrect.", field="code")

    if not valid.ok:
        return render_template("totp-enable.html",
            qrcode=totp_get_qrcode(secret),
            otpauth_uri=otpauth_uri(secret),
            secret=secret, valid=valid), 400

    codes = Client().enable_totp(secret).config.recovery_codes
    session["recovery-codes"] = codes
    metrics.meta_totp_enabled.inc()
    return redirect("/security/totp/complete")

@security.route("/security/totp/complete")
@loginrequired
def security_totp_complete():
    codes = session.pop("recovery-codes", None)
    if not codes:
        return redirect("/security")
    return render_template("totp-enabled.html", codes=codes)

@security.route("/security/totp/disable", methods=["POST"])
@loginrequired
def security_totp_disable_POST():
    factor = (UserAuthFactor.query
        .filter(UserAuthFactor.user_id == current_user.id)
        .filter(UserAuthFactor.factor_type == FactorType.totp)).one_or_none()
    if not factor:
        return redirect("/security")

    session["extra_factors"] = [factor.id]
    session["authorized_user"] = current_user.id
    session["challenge_type"] = "disable_totp"
    session["return_to"] = "/security"
    return redirect(url_for("auth.totp_challenge_GET"))
