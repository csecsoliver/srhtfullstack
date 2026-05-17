import re
from markupsafe import Markup
from metasrht.blacklist import email_blacklist, username_blacklist
from metasrht.types import User
from srht.config import cfg
from zxcvbn import zxcvbn


def validate_username(valid, username, check_blacklist=True):
    user = User.query.filter(User.username == username).first()
    valid.expect(user is None, "This username is already in use.", "username")
    valid.expect(2 <= len(username) <= 30,
                 "Username must contain between 2 and 30 characters.",
                 "username")
    valid.expect(re.match("^[a-z_]", username),
                 "Username must start with a lowercase letter or underscore.",
                 "username")
    valid.expect(re.match("^[a-z0-9_-]+$", username),
                 "Username may contain only lowercase letters, numbers, "
                 "hyphens and underscores", "username")
    valid.expect(not check_blacklist or username not in username_blacklist,
                 "This username is not available", "username")


def validate_email(valid, email):
    user = User.query.filter(User.email == email).first()
    valid.expect(user is None, "This email address is already in use.", "email")
    valid.expect(len(email) <= 256,
                 "Email must be no more than 256 characters.", "email")
    valid.expect("@" in email, "This is not a valid email address.", "email")
    if valid.ok:
        [user, domain] = email.split("@")
        valid.expect(domain not in email_blacklist,
                     "This email domain is blacklisted. Disposable email "
                     "addresses are prohibited by the terms of service - we "
                     "must be able to reach you at your account's primary "
                     "email address. Contact support if you believe this "
                     "domain was blacklisted in error.", "email")


def validate_password(valid, password):
    valid.expect(len(password) <= 512,
                 "Password must be no more than 512 characters.", "password")

    if cfg("sr.ht", "environment", default="production") == "development":
        return
    strength = zxcvbn(password)
    time = strength["crack_times_display"]["offline_slow_hashing_1e4_per_second"]
    valid.expect(strength["score"] >= 3, Markup(
        "This password is too weak &mdash; it could be cracked in " +
        f"{time} if our database were broken into. Try using " +
        "a few words instead of random letters and symbols. A " +
        "<a href='https://www.passwordstore.org/'>password manager</a> " +
        "is strongly recommended."), field="password")
