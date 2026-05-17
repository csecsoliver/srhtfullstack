from datetime import datetime
from flask import Blueprint, render_template, request, redirect
from metasrht.graphql import Client, GraphQLClientGraphQLMultiError
from srht.graphql import Error, has_error
from srht.oauth import current_user, loginrequired
from srht.validation import Validation

keys = Blueprint('keys', __name__)

def describe_last_used(last_used):
    if last_used is None:
        return "Never"
    delta = (datetime.utcnow() - last_used).days
    nweeks = delta // 7
    ndays = delta % 7
    if delta == 0:
        desc = "24 hours"
    elif nweeks == 0 or (nweeks == 1 and ndays == 0):
        desc = "week"
    elif nweeks < 4:
        desc = f"{nweeks+1} weeks"
    elif nweeks == 4 and ndays == 0:
        desc = "month"
    elif nweeks < 52 or (nweeks == 52 and ndays == 0):
        # Use average number of weeks per month to keep things simple
        desc = f"{nweeks*100//433+1} months"
    else:
        desc = f"{nweeks//52+1} years"
    return f"Within last {desc}"

@keys.route("/keys")
@loginrequired
def keys_GET():
    return render_template("keys.html",
                           now=datetime.utcnow(),
                           describe_last_used=describe_last_used)

@keys.route("/keys/ssh-keys", methods=["POST"])
@loginrequired
def ssh_keys_POST():
    valid = Validation(request)
    ssh_key = valid.require("key")
    if not valid.ok:
        return render_template("keys.html",
                now=datetime.utcnow(),
                describe_last_used=describe_last_used,
                ssh_key=ssh_key, **valid.kwargs), 400

    client = Client()
    with valid:
        client.create_ssh_key(ssh_key)

    if not valid.ok:
        return render_template("keys.html",
                now=datetime.utcnow(),
                describe_last_used=describe_last_used,
                ssh_key=ssh_key, **valid.kwargs), 400

    return redirect("/keys")

@keys.route("/keys/delete-ssh/<int:key_id>", methods=["POST"])
@loginrequired
def ssh_keys_delete(key_id):
    try:
        Client().delete_ssh_key(key_id)
    except GraphQLClientGraphQLMultiError as err:
        if not has_error(err, Error.NOT_FOUND): # Idempotency
            raise
    return redirect("/keys")

@keys.route("/keys/pgp-keys", methods=["POST"])
@loginrequired
def pgp_keys_POST():
    valid = Validation(request)
    pgp_key = valid.require("key")
    if not valid.ok:
        return render_template("keys.html",
                now=datetime.utcnow(),
                describe_last_used=describe_last_used,
                pgp_key=pgp_key, **valid.kwargs), 400

    client = Client()
    with valid:
        client.create_pgp_key(pgp_key)

    if not valid.ok:
        return render_template("keys.html",
                now=datetime.utcnow(),
                describe_last_used=describe_last_used,
                pgp_key=pgp_key, **valid.kwargs), 400

    return redirect("/keys")

@keys.route("/keys/delete-pgp/<int:key_id>", methods=["POST"])
@loginrequired
def pgp_keys_delete(key_id):
    # TODO: Move this logic into GQL
    if key_id == current_user.pgp_key_id:
        return render_template("keys.html",
                now=datetime.utcnow(),
                describe_last_used=describe_last_used,
                tried_to_delete_key_in_use=True), 400

    try:
        Client().delete_pgp_key(key_id)
    except GraphQLClientGraphQLMultiError as err:
        if not has_error(err, Error.NOT_FOUND): # Idempotency
            raise

    return redirect("/keys")
