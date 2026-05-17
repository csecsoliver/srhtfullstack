from flask import Blueprint, Response, render_template, request, redirect
from metasrht.audit import audit_log
from metasrht.email import send_email
from metasrht.types import User, PGPKey
from srht.config import cfg
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.validation import Validation
from string import Template

privacy = Blueprint('privacy', __name__)

origin = cfg("meta.sr.ht", "origin")
owner_name = cfg("sr.ht", "owner-name")
owner_email = cfg("sr.ht", "owner-email")
site_key = cfg("mail", "pgp-pubkey", None)
site_key_id = cfg("mail", "pgp-key-id", None)
site_name = cfg("sr.ht", "site-name")

@privacy.route("/privacy")
@loginrequired
def privacy_GET():
    owner = {'name': cfg("sr.ht", "owner-name"),
             'email': cfg("sr.ht", "owner-email")}
    return render_template("privacy.html", pgp_key_id=site_key_id, owner=owner)

@privacy.route("/privacy/pubkey")
def privacy_pubkey_GET():
    if site_key:
        with open(site_key, "r") as f:
            pubkey = f.read()
    else:
        pubkey = ''
    return Response(pubkey, mimetype="text/plain")

@privacy.route("/privacy", methods=["POST"])
@loginrequired
def privacy_POST():
    valid = Validation(request)

    key_id = valid.require("pgp-key")
    key_id = key_id if key_id != "null" else None
    key = None

    if key_id:
        key = PGPKey.query.get(int(key_id))
        valid.expect(key and key.user_id == current_user.id, "Invalid PGP key")

    if not valid.ok:
        return redirect("/privacy")

    user = User.query.get(current_user.id)
    user.pgp_key = key
    audit_log("changed pgp key",
            "Set default PGP key to {}".format(key.fingerprint_hex if key else None))
    db.session.commit()

    return redirect("/privacy")

@privacy.route("/privacy/test-email", methods=["POST"])
@loginrequired
def privacy_testemail_POST():
    tmpl = Template("""Subject: Test email

This is a test email sent from $site_name to confirm that PGP is working as you
expect. This email is signed with this key:

$site_key

You may update your PGP settings here:

$root/privacy

-- 
$owner_name
$site_name
""")
    rendered = tmpl.substitute(**{
            'owner_name': owner_name,
            'site_name': site_name,
            'site_key': site_key_id,
            'root': origin
        })
    send_email(current_user.email, rendered)
    return redirect("/privacy")
