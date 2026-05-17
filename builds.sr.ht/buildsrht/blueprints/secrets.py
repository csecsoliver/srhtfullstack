from buildsrht.graphql import Client
from buildsrht.types import Secret, SecretType
from cryptography.hazmat.backends import default_backend
from cryptography.hazmat.primitives import serialization
from flask import Blueprint, render_template, request, redirect, abort
from srht.app import session
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.validation import Validation

secrets = Blueprint('secrets', __name__)

@secrets.route("/secrets")
@loginrequired
def secrets_GET():
    message = session.get("message")
    if message:
        del session["message"]
    # TODO: Pagination I guess
    secrets = Secret.query.filter(Secret.user_id == current_user.id).all()
    return render_template("secrets.html", message=message, secrets=secrets)

@secrets.route("/secrets", methods=["POST"])
@loginrequired
def secrets_POST():
    valid = Validation(request)

    name = valid.optional("name")
    secret_type = valid.require("secret_type", friendly_name="Secret Type")
    valid.expect(not name or 3 < len(name) < 512,
            "Name must be between 3 and 512 characters", field="name")
    if secret_type is not None:
        try:
            secret_type = SecretType(secret_type)
        except:
            valid.error("{} is not a valid secret type".format(secret_type),
                    field="secret_type")

    if secret_type == SecretType.plaintext_file:
        _secret = valid.optional("secret")
        secret_file = valid.optional("file-file", max_file_size=16384)
        for f in ["secret", "file-file"]:
            valid.expect(bool(_secret) ^ bool(secret_file),
                    "Either secret text or file have to be provided", field=f)
    else:
        _secret = valid.require("secret", friendly_name="Secret")

    if secret_type in (SecretType.plaintext_file, SecretType.ssh_key):
        if _secret:
            _secret = _secret.replace('\r\n', '\n')
            if not _secret.endswith('\n'):
                _secret += '\n'
        elif secret_type == SecretType.plaintext_file:
            _secret = secret_file

    if isinstance(_secret, str):
        _secret = _secret.encode()

    if not valid.ok:
        secrets = Secret.query.filter(Secret.user_id == current_user.id).all()
        return render_template("secrets.html", secrets=secrets, **valid.kwargs)

    secret = Secret(current_user, secret_type)

    if secret_type == SecretType.plaintext_file:
        file_path = valid.require("file-path", friendly_name="Path")
        file_mode = valid.require("file-mode", friendly_name="Mode")
        if not valid.ok:
            secrets = Secret.query.filter(Secret.user_id == current_user.id).all()
            return render_template("secrets.html", secrets=secrets, **valid.kwargs)
        try:
            file_mode = int(file_mode, 8)
        except:
            valid.error("Must be specified in octal",
                    field="file-mode")
        if not valid.ok:
            secrets = Secret.query.filter(Secret.user_id == current_user.id).all()
            return render_template("secrets.html", secrets=secrets, **valid.kwargs)
        secret.path = file_path
        secret.mode = file_mode
    if not valid.ok:
        secrets = Secret.query.filter(Secret.user_id == current_user.id).all()
        return render_template("secrets.html", secrets=secrets, **valid.kwargs)

    secret.name = name
    secret.secret = _secret

    db.session.add(secret)
    db.session.commit()

    session["message"] = "Successfully added secret {}.".format(secret.uuid)
    return redirect("/secrets")

@secrets.route("/secret/delete/<uuid>")
@loginrequired
def secret_delete_GET(uuid):
    secret = (Secret.query
        .filter(Secret.uuid == uuid)
        .filter(Secret.user_id == current_user.id).first())
    if not secret:
        abort(404)
    return render_template("secret_delete.html", secret=secret)


@secrets.route("/secret/delete", methods=["POST"])
@loginrequired
def secret_delete_POST():
    valid = Validation(request)

    uuid = valid.require("uuid")
    if not uuid:
        abort(404)

    secret = (Secret.query
        .filter(Secret.uuid == uuid)
        .filter(Secret.user_id == current_user.id).first())
    if not secret:
        abort(404)

    name = secret.name
    db.session.delete(secret)
    db.session.commit()

    session["message"] = "Successfully removed secret {}{}.".format(uuid,
            " ({})".format(name) if name else "")
    return redirect("/secrets")

@secrets.route("/secret/share/<uuid>")
@loginrequired
def secret_share_GET(uuid):
    secret = (Secret.query
        .filter(Secret.uuid == uuid)
        .filter(Secret.user_id == current_user.id).first())
    if not secret:
        abort(404)
    return render_template("secret_share.html", secret=secret)

@secrets.route("/secret/share/<uuid>", methods=["POST"])
@loginrequired
def secret_share_POST(uuid):
    secret = (Secret.query
        .filter(Secret.uuid == uuid)
        .filter(Secret.user_id == current_user.id).first())
    if not secret:
        abort(404)

    valid = Validation(request)
    target_user = valid.require("username", friendly_name="Username")
    if not valid.ok:
        return render_template("secret_share.html",
                   secret=secret, valid=valid)

    with valid:
        secret = Client().share_secret(uuid, target_user).secret
    if not valid.ok:
        return render_template("secret_share.html",
                   secret=secret, valid=valid)

    session["message"] = "{} successfully shared with {}.".format(
            secret.name if secret.name else secret.uuid,
            target_user)
    return redirect("/secrets")
