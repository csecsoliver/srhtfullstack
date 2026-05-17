from datetime import datetime
from flask import Blueprint, render_template, abort, request, redirect
from flask import url_for
from metasrht.audit import audit_log
from metasrht.auth import allow_registration, user_valid, prepare_user
from metasrht.auth import is_external_auth, set_user_password, set_user_email
from metasrht.auth.builtin import hash_password, check_password
from metasrht.auth_validation import validate_password
from metasrht.blueprints.security import metrics as security_metrics
from metasrht.blueprints.profile import change_email_GET
from metasrht.graphql import Client, GraphQLClientGraphQLMultiError
from metasrht.totp import totp
from metasrht.types import User, UserType
from metasrht.types import UserAuthFactor, FactorType, PGPKey
from metasrht.webhooks import UserWebhook
from prometheus_client import Counter
from srht.app import csrf_bypass, session
from srht.config import cfg, get_global_domain, config, get_origin
from srht.database import db
from srht.graphql import InternalAuth, has_error
from srht.oauth import current_user, login_user, logout_user
from srht.validation import Validation
from urllib.parse import urlparse

try:
    # This file is kept private to prevent spammers from reading it to
    # understand how to circumvent our spam prevention mechanisms.
    with open("/etc/abuse.py") as f:
        try:
            exec(f.read())
        except Exception as ex:
            print("Error loading abuse.py", ex)
            raise
except:
    def is_abuse(valid):
        return False

auth = Blueprint('auth', __name__)

origin = cfg("meta.sr.ht", "origin")
owner_name = cfg("sr.ht", "owner-name")
owner_email = cfg("sr.ht", "owner-email")
site_name = cfg("sr.ht", "site-name")
onboarding_redirect = cfg("meta.sr.ht::settings", "onboarding-redirect")
site_key_id = cfg("mail", "pgp-key-id", None)

metrics = type("metrics", tuple(), {
    c.describe()[0].name: c
    for c in [
        Counter("meta_registrations", "Number of new user registrations"),
        Counter("meta_confirmations", "Number of account confirmations"),
        Counter("meta_logins_failed", "Number of failed logins"),
        Counter("meta_logins_success", "Number of successful logins"),
        Counter("meta_logouts", "Number of sessions logged out"),
        Counter("meta_pw_resets", "Number of password resets completed"),
    ]
})

# Returns return_to if it points within the SouceHut installation,
# "/" otherwise.
def validate_return_url(return_to):
    if return_to and return_to[0] == "/":
        # Relative path => valid.
        return return_to
    parsed = urlparse(return_to)
    if parsed.hostname is None:
        # Unparsable host in an absolute URL => invalid.
        return "/"
    # Absolute URLs are only valid if they point within the global domain...
    global_domain = cfg("sr.ht", "global-domain", None)
    if global_domain and parsed.hostname.endswith(global_domain):
        return return_to
    # ... or a configured sr.ht service.
    srht_sites = [ s for s in config if s.endswith(".sr.ht") ]
    for site in srht_sites:
        origin = get_origin(site, True, None)
        if origin and return_to.startswith(origin):
            return return_to
    return "/"

def issue_reset(user):
    client = Client(InternalAuth.anonymous())
    client.request_password_reset(user.email)
    return render_template("forgot.html", done=True)

@auth.route("/")
def index():
    if current_user:
        return redirect(url_for("profile.profile_GET"))
    return render_template("index.html")

@auth.route("/register")
def register():
    if current_user:
        return redirect("/")
    if cfg("meta.sr.ht::billing", "enabled") != "yes":
        return redirect(url_for("auth.register_step2_GET"))
    return render_template("register.html", site_key=site_key_id)

@auth.route("/register", methods=["POST"])
def register_POST():
    is_open = allow_registration()

    valid = Validation(request)
    payment = valid.require("payment")
    if not valid.ok:
        abort(400)
    payment = payment == "yes"
    session["payment"] = payment

    return redirect(url_for("auth.register_step2_GET"))

@auth.route("/register/step2")
def register_step2_GET():
    payment = session.get("payment", "no")
    if current_user:
        return redirect("/")
    return render_template("register-step2.html",
            site_key=site_key_id, payment=payment)

@auth.route("/register/step2", methods=["POST"])
def register_step2_POST():
    if current_user:
        abort(400)
    is_open = allow_registration()
    payment = session.get("payment", False)

    valid = Validation(request)
    username = valid.require("username", friendly_name="Username")
    email = valid.require("email", friendly_name="Email address")
    password = valid.require("password", friendly_name="Password")
    pgp_key = valid.optional("pgpKey", default=None)
    if not pgp_key:
        pgp_key = None

    if not valid.ok:
        return render_template("register-step2.html", is_open=is_open,
                site_key=site_key_id, payment=payment, **valid.kwargs), 400

    if is_abuse(valid):
        return redirect("/registered")

    allow_plus_in_email = valid.optional("allow-plus-in-email")
    if "+" in email and allow_plus_in_email != "yes":
        return render_template("register-step2.html", is_open=is_open,
                site_key=site_key_id, payment=payment, **valid.kwargs), 400

    client = Client(InternalAuth.anonymous())
    with valid:
        client.register_account(email, username, password, pgp_key)

    if not valid.ok:
        return render_template("register-step2.html", is_open=is_open,
                site_key=site_key_id, payment=payment, **valid.kwargs), 400

    metrics.meta_registrations.inc()
    return redirect("/registered")

@auth.route("/registered")
def registered():
    return render_template("registered.html")

@auth.route("/confirm-account/<token>")
def confirm_account(token):
    client = Client(InternalAuth.anonymous())

    try:
        resp = client.confirm_registration(token)
    except GraphQLClientGraphQLMultiError as err:
        if has_error(err, "ERR_INVALID_EMAIL_TOKEN"):
            abort(404)
        raise

    user = User.query.get(resp.user.id)
    assert user is not None
    login_user(user, set_cookie=True)

    metrics.meta_confirmations.inc()

    payment = session.pop("payment", False)
    if payment and cfg("meta.sr.ht::billing", "enabled") == "yes":
        return redirect(url_for("billing.billing_setup_GET"))
    else:
        return redirect(onboarding_redirect)

@auth.route("/login")
def login_GET():
    if current_user:
        return redirect("/")
    return_to = request.args.get('return_to')
    context = session.get("login_context")
    return render_template("login.html",
           return_to=return_to,
           login_context=context)

def get_challenge(factor):
    if factor.factor_type == FactorType.totp:
        return redirect("/login/challenge/totp")
    abort(500)

@auth.route("/login", methods=["POST"])
def login_POST():
    if current_user:
        return redirect("/")
    valid = Validation(request)

    username = valid.require("username", friendly_name="Username")
    password = valid.require("password", friendly_name="Password")
    return_to = valid.optional("return_to", "/")

    if not valid.ok:
        return render_template("login.html", **valid.kwargs), 400

    user_valid(valid, username, password)

    if not valid.ok:
        metrics.meta_logins_failed.inc()
        print(f"{datetime.utcnow()} Login attempt failed for {username}")
        return render_template("login.html",
            username=username,
            valid=valid)

    user = prepare_user(username)
    valid.expect(user.user_type != UserType.pending,
            "Your account is pending confirmation. Please check your inbox, or reach out to support if you did not receive an email.")
    valid.expect(user.user_type != UserType.suspended,
            f"Your account is suspended: {user.suspension_notice}. Contact support.")
    if not valid.ok:
        return render_template("login.html", **valid.kwargs), 400

    factors = (UserAuthFactor.query
        .filter(UserAuthFactor.user_id == user.id)).all()

    session.pop("login_context", None)
    if any(factors):
        session['extra_factors'] = [f.id for f in factors]
        session['authorized_user'] = user.id
        session['challenge_type'] = 'login'
        session['return_to'] = return_to
        return get_challenge(factors[0])

    login_user(user, set_cookie=True)
    print("session_login = True")
    session["session_login"] = True
    audit_log("logged in")
    print(f"Logged in account: {user.username} ({user.email})")
    db.session.commit()
    metrics.meta_logins_success.inc()
    return_to = validate_return_url(return_to)
    return redirect(return_to)

@auth.route("/login/challenge/totp")
def totp_challenge_GET():
    user = session.get('authorized_user')
    if not user:
        return redirect("/login")
    challenge_type = session.get('challenge_type')
    return render_template("totp-challenge.html", challenge_type=challenge_type)

@auth.route("/login/challenge/totp", methods=["POST"])
def totp_challenge_POST():
    user_id = session.get('authorized_user')
    factors = session.get('extra_factors')
    challenge_type = session.get('challenge_type')
    return_to = session.get('return_to') or '/'
    if not user_id or not factors:
        return redirect("/login")
    valid = Validation(request)

    code = valid.require("code")
    if not valid.ok:
        return render_template("totp-challenge.html",
            return_to=return_to, valid=valid)

    code = code.replace(" ", "")
    try:
        code = int(code)
    except:
        valid.error(
                "This TOTP code is invalid (expected a number)", field="code")
    if not valid.ok:
        return render_template("totp-challenge.html",
            return_to=return_to, valid=valid)

    factor = UserAuthFactor.query.get(factors[0])
    secret = factor.secret.decode('utf-8')

    valid.expect(totp(secret, code),
            'The code you entered is incorrect.', field='code')

    user = User.query.get(user_id)
    if not valid.ok:
        print(f"{challenge_type} attempt failed (TOTP) for " +
            f"{user.username} ({user.email})")
        return render_template("totp-challenge.html",
            valid=valid, return_to=return_to)

    factors = factors[1:]
    if len(factors) != 0:
        return get_challenge(UserAuthFactor.query.get(factors[0]))

    session.pop('authorized_user', None)
    session.pop('extra_factors', None)
    session.pop('challenge_type', None)
    session.pop('return_to', None)

    if challenge_type == "login":
        login_user(user, set_cookie=True)
        session["session_login"] = True
        audit_log("logged in")
        print(f"Logged in account: {user.username} ({user.email})")
        db.session.commit()
        metrics.meta_logins_success.inc()
        return_to = validate_return_url(return_to)
        return redirect(return_to)
    elif challenge_type == "reset":
        return issue_reset(user)
    elif challenge_type == "disable_totp":
        db.session.delete(factor)
        audit_log("Disable TOTP", details="Disabled two-factor authentication",
                email=True, subject=f"TOTP has been disabled for your {cfg('sr.ht', 'site-name')} account",
                email_details="2FA via TOTP was disabled")
        db.session.commit()
        security_metrics.meta_totp_disabled.inc()
        return redirect(return_to)
    else:
        raise NotImplemented

@auth.route("/login/challenge/totp-recovery")
def totp_recovery_GET():
    user = session.get('authorized_user')
    if not user:
        return redirect("/login")
    factors = session.get('extra_factors')
    factor = UserAuthFactor.query.get(factors[0])
    supported = factor.extra is not None
    return render_template("totp-recovery.html", supported=supported)

@auth.route("/login/challenge/totp-recovery", methods=["POST"])
def totp_recovery_POST():
    user_id = session.get('authorized_user')
    factors = session.get('extra_factors')
    challenge_type = session.get('challenge_type')
    return_to = session.get('return_to') or '/'
    if not user_id or not factors:
        return redirect("/login")
    valid = Validation(request)

    code = valid.require('recovery-code')
    if not valid.ok:
        return render_template("totp-recovery.html",
            return_to=return_to, supported=True, **valid.kwargs)

    factor = UserAuthFactor.query.get(factors[0])
    is_valid = False
    for h in factor.extra:
        if check_password(code, h):
            is_valid = True
            break
    valid.expect(is_valid, "Incorrect recovery code", field="recovery-code")
    if not valid.ok:
        return render_template("totp-recovery.html",
            return_to=return_to, supported=True, **valid.kwargs)

    user = User.query.get(user_id)

    db.session.delete(factor)
    audit_log("TOTP recovery code used", user=user, email=True,
            subject=f"A recovery code was used for your {cfg('sr.ht', 'site-name')} account",
            email_details="Two-factor authentication recovery code used")
    session["notice"] = "TOTP has been disabled for your account."
    db.session.commit()

    factors = factors[1:]
    if len(factors) != 0:
        return get_challenge(UserAuthFactor.query.get(factors[0]))

    session.pop('authorized_user', None)
    session.pop('extra_factors', None)
    session.pop('return_to', None)
    session.pop('challenge_type', None)

    if challenge_type == "login":
        login_user(user, set_cookie=True)
        session["session_login"] = True
        audit_log("logged in")
        print(f"Logged in account: {user.username} ({user.email})")
        db.session.commit()
        metrics.meta_logins_success.inc()
        return_to = validate_return_url(return_to)
        return redirect(return_to)
    elif challenge_type == "reset":
        return issue_reset(user)
    elif challenge_type == "disable_totp":
        security_metrics.meta_totp_disabled.inc()
        return redirect(return_to)
    else:
        raise NotImplemented

@auth.route("/logout")
def logout():
    if current_user:
        audit_log("logged out")
        logout_user()
        db.session.commit()
        metrics.meta_logouts.inc()
    if request.args.get("return_to"):
        return_to = validate_return_url(request.args["return_to"])
        return redirect(return_to)
    return redirect("/login")

@auth.route("/forgot")
def forgot_GET():
    return render_template("forgot.html")

@auth.route("/forgot", methods=["POST"])
def forgot_POST():
    valid = Validation(request)
    email = valid.require("email", friendly_name="Email")
    if not valid.ok:
        return render_template("forgot.html", **valid.kwargs)
    user = User.query.filter(User.email == email).first()
    valid.expect(user, "No account found with this email address.")
    valid.expect(not user or user.user_type != UserType.admin,
            "You can't reset the password of an admin.")
    valid.expect(not user or user.user_type != UserType.pending,
            f"Your account has not been confirmed. Please contact support via {cfg('sr.ht', 'owner-email')} if you did not receive a confirmation email.")
    if not valid.ok:
        return render_template("forgot.html", **valid.kwargs)

    factors = (UserAuthFactor.query
        .filter(UserAuthFactor.user_id == user.id)).all()
    if any(factors):
        session['extra_factors'] = [f.id for f in factors]
        session['authorized_user'] = user.id
        session['challenge_type'] = 'reset'
        return get_challenge(factors[0])

    return issue_reset(user)

@auth.route("/reset-password/<token>")
def reset_GET(token):
    return render_template("reset.html")

@auth.route("/reset-password/<token>", methods=["POST"])
def reset_POST(token):
    valid = Validation(request)
    password = valid.require("password", friendly_name="Password")
    if not valid.ok:
        return render_template("reset.html", valid=valid)
    validate_password(valid, password)
    if not valid.ok:
        return render_template("reset.html", valid=valid)

    client = Client(InternalAuth.anonymous())

    try:
        resp = client.confirm_password_change(token, password)
    except GraphQLClientGraphQLMultiError:
        return render_template("reset.html", invalid_token=True)

    user = User.query.get(resp.user.id)
    assert user is not None

    session["session_login"] = True
    session["notice"] = "Your password has been changed."
    login_user(user, set_cookie=True)

    metrics.meta_pw_resets.inc()
    return redirect("/")
