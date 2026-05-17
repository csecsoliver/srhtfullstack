import base64
import json
import requests
import urllib
from metasrht.graphql import Client, GraphQLClientError
from datetime import datetime, timedelta
from urllib.parse import urlparse
from flask import Blueprint, render_template, redirect, request, session
from flask import url_for
from srht.app import csrf_bypass, cross_origin
from srht.config import config, cfg, get_origin
from srht.graphql import InternalAuth, BearerAuth, gql_time
from srht.oauth import current_user, loginrequired
from srht.validation import Validation, valid_url

oauth2 = Blueprint('oauth2', __name__)

print("Discovering APIs...")
access_grants = []
service_scopes = {}
for s in config:
    if not s.endswith(".sr.ht"):
        continue
    origin = cfg(s, "api-origin", default=get_origin(s))
    try:
        r = requests.get(f"{origin}/query/api-meta.json", timeout=5)
        if r.status_code != 200:
            continue
    except (requests.exceptions.ConnectionError, requests.exceptions.ReadTimeout):
        continue
    try:
        scopes = r.json()["scopes"]
    except json.decoder.JSONDecodeError:
        print(f"  Skipping {s}: invalid JSON response")
        continue
    print(f"  Found {s}")
    access_grants.append({
        "name": s,
        "scopes": scopes,
    })
    service_scopes[s] = scopes
print(f"Discovered {len(access_grants)} APIs")

def parse_grant(grant):
    svc, scope = grant.split("/")
    if ":" in scope:
        scope, access = scope.split(":")
    else:
        access = "RO"
    return svc, scope, access

def validate_grants(literal, valid, field="literal_grants"):
    grants = []
    for grant in literal.split(" "):
        valid.expect("/" in grant,
                f"Invalid grant {grant}; expected service/scope:access",
                field=field)
        if not valid.ok:
            continue
        try:
            svc, scope, access = parse_grant(grant)
            valid.expect(access in ["RO", "RW"],
                    f"Invalid grant access level '{access}'", field=field)
            valid.expect(svc in service_scopes,
                    f"Invalid grant service '{svc}'", field=field)
            if not valid.ok:
                continue
            valid.expect(scope in service_scopes[svc],
                    f"Invalid scope '{scope}' for service {svc}", field=field)
            grants.append((svc, scope, access))
        except ValueError:
            valid.error("Invalid grant string. The expected format is a list of space-separated grants in the form &lt;service&gt;/&lt;permission&gt;:&lt;RO|RW&gt;", field=field)
            continue
    return grants

@oauth2.route("/oauth2")
@loginrequired
def dashboard():
    client = Client()
    dashboard = client.o_auth_dashboard()
    return render_template("oauth2-dashboard.html",
            client_revoked=session.pop("client_revoked", False),
            personal_tokens=dashboard.personal_access_tokens,
            oauth_clients=dashboard.oauth_clients,
            oauth_grants=dashboard.oauth_grants)

@oauth2.route("/oauth2/personal-token")
@loginrequired
def personal_token_GET():
    return render_template("oauth2-personal-token-registration.html",
            access_grants=access_grants,
            fixed_literal_grants=request.args.get("grants"))

@oauth2.route("/oauth2/personal-token", methods=["POST"])
@loginrequired
def personal_token_POST():
    valid = Validation(request)
    comment = valid.optional("comment")
    literal = valid.optional("literal_grants")
    ro = valid.optional("read_only", default="off") == "on"
    valid.expect(not literal or "grants" not in valid.source,
            "Use either the selection box or a grant string; not both",
            field="literal_grants")
    grants = []

    if "grants" in valid.source:
        for grant in request.form.getlist("grants"):
            grants.append(f"{grant}:{'RO' if ro else 'RW'}")
        literal = " ".join(grants)
    elif literal:
        grants = validate_grants(literal, valid)

    if not valid.ok:
        kwargs = valid.kwargs
        kwargs["grants"] = grants
        return render_template("oauth2-personal-token-registration.html",
                access_grants=access_grants,
                fixed_literal_grants=request.args.get("grants"),
                **valid.kwargs)

    pat = Client().issue_pat(literal, comment).token
    session["personal_access_token"] = {
        "expires": pat.token.expires,
        "secret": pat.secret,
    }
    return redirect(url_for("oauth2.personal_token_issued_GET"))

@oauth2.route("/oauth2/personal-token/issued")
@loginrequired
def personal_token_issued_GET():
    token = session.pop("personal_access_token", None)
    if not token:
        return redirect(url_for("oauth2.dashboard"))
    return render_template("oauth2-personal-token-issued.html",
            expiry=token["expires"], secret=token["secret"])

@oauth2.route("/oauth2/client-registration")
@loginrequired
def client_registration_GET():
    return render_template("oauth2-register-client.html")

def valid_redirect_uri(uri):
    # valid_url is too strict: we want to allow private-use URI scheme
    # redirection. See RFC 8252 section 7.1.
    try:
        u = urlparse(uri)
    except:
        return False
    if '.' in u.scheme:
        return bool(u.path)

    return valid_url(uri)

@oauth2.route("/oauth2/client-registration", methods=["POST"])
@loginrequired
def client_registration_POST():
    valid = Validation(request)
    client_name = valid.require("client_name")
    redirect_uri = valid.require("redirect_uri")
    client_description = valid.optional("client_description")
    client_url = valid.optional("client_url")
    valid.expect(valid_redirect_uri(redirect_uri), "Invalid URL", field="redirect_uri")
    valid.expect(not client_url or valid_url(client_url),
            "Invalid URL", field="client_url")
    if not valid.ok:
        return render_template("oauth2-register-client.html", **valid.kwargs)

    registration = Client().register_client(
        client_name,
        redirect_uri,
        client_description,
        client_url,
    ).registration

    session["client_uuid"] = registration.client.uuid
    session["client_secret"] = registration.secret
    return redirect(url_for("oauth2.client_registration_complete_GET"))

@oauth2.route("/oauth2/client-registered")
@loginrequired
def client_registration_complete_GET():
    client_uuid = session.pop("client_uuid", None)
    client_secret = session.pop("client_secret", None)
    client_reissued = session.pop("client_reissued", False)
    if not client_uuid or not client_secret:
        return redirect(url_for("oauth2.dashboard"))
    return render_template("oauth2-client-registered.html",
            client_uuid=client_uuid, client_secret=client_secret,
            client_reissued=client_reissued)

@oauth2.route("/oauth2/revoke-personal/<int:token_id>")
@loginrequired
def personal_token_revoke_GET(token_id):
    return render_template("are-you-sure.html",
            blurb="revoke this personal access token",
            action=url_for("oauth2.personal_token_revoke_POST", token_id=token_id),
            cancel=url_for("oauth2.dashboard"))

@oauth2.route("/oauth2/revoke-personal/<int:token_id>", methods=["POST"])
@loginrequired
def personal_token_revoke_POST(token_id):
    Client().revoke_pat(token_id)
    return redirect(url_for("oauth2.dashboard"))

@oauth2.route("/oauth2/revoke-bearer/<token_hash>")
@loginrequired
def bearer_token_revoke_GET(token_hash):
    return render_template("are-you-sure.html",
            blurb="revoke this access token",
            action=url_for("oauth2.bearer_token_revoke_POST",
                token_hash=token_hash),
            cancel=url_for("oauth2.dashboard"))

@oauth2.route("/oauth2/revoke-bearer/<token_hash>", methods=["POST"])
@loginrequired
def bearer_token_revoke_POST(token_hash):
    Client().revoke_bearer(token_hash)
    return redirect(url_for("oauth2.dashboard"))

@oauth2.route("/oauth2/client-registration/<uuid>")
@loginrequired
def manage_client_GET(uuid):
    client = Client().get_o_auth_client(uuid).client
    return render_template("oauth2-manage-client.html", client=client)

@oauth2.route("/oauth2/client-registration/<uuid>/reissue", methods=["POST"])
@loginrequired
def reissue_client_secrets_POST(uuid):
    graphql = Client()
    client = graphql.get_o_auth_client(uuid).client
    registration = graphql.reregister_o_auth_client(
        client.uuid,
        client.name,
        client.redirect_url,
        client.description,
        client.url,
    ).registration
    session["client_reissued"] = True
    session["client_uuid"] = registration.client.uuid
    session["client_secret"] = registration.secret
    return redirect(url_for("oauth2.client_registration_complete_GET"))

@oauth2.route("/oauth2/client-registration/<uuid>/unregister", methods=["POST"])
@loginrequired
def unregister_client_POST(uuid):
    Client().unregister_o_auth_client(uuid)
    session["client_revoked"] = True
    return redirect(url_for("oauth2.client_registration_complete_GET"))

def _oauth2_redirect(redirect_uri, **params):
    parts = list(urllib.parse.urlparse(redirect_uri))
    parsed = urllib.parse.parse_qs(parts[4])
    parsed.update(params)
    parts[4] = urllib.parse.urlencode(parsed)
    return redirect(urllib.parse.urlunparse(parts))

def _authorize_error(redirect_uri, state, error_code, error_description):
    if not redirect_uri:
        return render_template("oauth2-error.html",
                code=error_code, description=error_description)
    return _oauth2_redirect(redirect_uri, error=error_code,
            error_description=error_description,
            error_uri="https://man.sr.ht/meta.sr.ht/oauth.md",
            state=state)

@oauth2.route("/oauth2/authorize")
@loginrequired
def authorize_GET():
    response_type = request.args.get("response_type")
    client_id = request.args.get("client_id")
    scope = request.args.get("scope")
    state = request.args.get("state")

    if not client_id:
        return _authorize_error(None, state, "invalid_request",
                "The client_id parameter is required")

    try:
        client = Client().get_o_auth_client(client_id).client
    except Exception as ex:
        return _authorize_error(None, state, "server_error", str(ex))
    if not client:
        return _authorize_error(None, state, "invalid_request", "Invalid client ID")

    redirect_uri = client.redirect_url
    if "redirect_uri" in request.args and request.args["redirect_uri"] != redirect_uri:
        return _authorize_error(None, state, "invalid_request",
                "The redirect_uri parameter doesn't match the registered client's")

    if response_type != "code":
        return _authorize_error(redirect_uri, state, "unsupported_response_type",
                "The response_type parameter must be set to 'code'")
    if not scope:
        return _authorize_error(redirect_uri, state, "invalid_scope",
                "The scope parameter is required")

    valid = Validation({})
    grants = validate_grants(scope, valid)
    if not valid.ok:
        return _authorize_error(redirect_uri, state, "invalid_scope",
                ", ".join(e.message for e in valid.errors))

    return render_template("oauth2-authorization.html",
            client=client, grants=grants, client_id=client_id,
            redirect_uri=redirect_uri, state=state)

@oauth2.route("/oauth2/authorize", methods=["POST"])
@loginrequired
def authorize_POST():
    valid = Validation(request)
    client_id = valid.require("client_id")
    redirect_uri = valid.require("redirect_uri")
    state = valid.optional("state")

    if "reject" in request.form:
        return _authorize_error(redirect_uri, state, "access_denied",
                                "The resource owner denied the request.")

    grants = []
    for grant in request.form:
        if grant in ["accept", "client_id", "redirect_uri", "state",
                "_csrf_token"]:
            continue
        svc, scope, access = parse_grant(grant)
        grants.append((svc, scope, access))

    final_grants = []
    for grant in grants:
        svc, scope, access = grant
        valid.expect(access != "RW" or (svc, scope, "RO") in grants,
                "Cannot remove read access without also removing write access",
                field=f"{svc}/{scope}:RO")
        if access != "RO" or (svc, scope, "RW") not in grants:
            final_grants.append(grant) # de-dupe RO+RW
    grants = final_grants

    graphql = Client()
    if not valid.ok:
        try:
            client = graphql.get_o_auth_client(client_id).client
        except Exception as ex:
            return _authorize_error(None, state, "server_error", str(ex))
        return render_template("oauth2-authorization.html",
                client=client, grants=grants, **valid.kwargs)

    grants = " ".join(f"{g[0]}/{g[1]}:{g[2]}" for g in grants)
    code = graphql.issue_auth_code(client_id, grants).code
    origin = get_origin("meta.sr.ht", external=True)

    return _oauth2_redirect(redirect_uri, **{
        "iss": origin, # RFC 9207
        "code": code,
        **({ "state": state } if state else {}),
    })

def access_token_error(code, description, status=400):
    return {
        "error": code,
        "error_description": description,
        "error_uri": "https://man.sr.ht/meta.sr.ht/oauth.md",
    }, status

@oauth2.route("/oauth2/access-token", methods=["POST"])
@csrf_bypass
@cross_origin
def access_token_POST():
    content_type = request.headers.get("Content-Type")
    if content_type != "application/x-www-form-urlencoded":
        return access_token_error("invalid_request",
                "Content-Type must be application/x-www-form-urlencoded")

    grant_type = request.form.get("grant_type")
    code = request.form.get("code")
    refresh_token = request.form.get("refresh_token")
    scope = request.form.get("scope")
    redirect_uri = request.form.get("redirect_uri")
    client_id = request.form.get("client_id")
    client_secret = request.form.get("client_secret")

    auth = request.headers.get("Authorization")
    if auth and (client_id or client_secret):
        return access_token_error("invalid_client",
                "Cannot supply both client_id & client_secret and Authorization header")
    elif auth:
        parts = auth.split(" ")
        if len(parts) != 2 or parts[0] != "Basic":
            return access_token_error("invalid_client",
                    "Invalid Authorization header")
        auth = base64.b64decode(parts[1]).decode()
        if not ":" in auth:
            return access_token_error("invalid_client",
                    "Invalid Authorization header")
        client_id, client_secret = auth.split(":", 1)
        client_id = urllib.parse.unquote(client_id)
        client_secret = urllib.parse.unquote(client_secret)
    elif not client_id or not client_secret:
        return access_token_error("invalid_client",
                "Missing client authorization", status=401)

    if not grant_type:
        return access_token_error("invalid_request",
                "The grant_type parameter is required")

    bearer_client = Client(InternalAuth(client_id=client_id))

    try:
        match grant_type:
            case "authorization_code":
                if not code:
                    return access_token_error("invalid_request",
                            "The code parameter is required")

                grant = bearer_client.issue_bearer_token(
                    code, client_id, client_secret, redirect_uri).grant
            case "refresh_token":
                if not refresh_token:
                    return access_token_error("invalid_request",
                            "The refresh_token parameter is required")

                grant = bearer_client.refresh_grant(
                    refresh_token, client_id, client_secret, scope).grant
            case _:
                return access_token_error("unsupported_grant_type",
                        f"Unsupported grant type '{grant_type}'")
    except GraphQLClientError:
        return access_token_error("invalid_grant", "The access grant was denied.")

    if not grant:
        return access_token_error("invalid_grant", "The access grant was denied.")

    # OAuth 2.0 specifies that the expiration in the response only affects the
    # access token, and clients should use the refresh token after the access
    # token has expired. Return an expiration before the actual one, to give
    # clients a chance to refresh their access token.
    expires = grant.grant.expires - timedelta(days=31)
    return {
        "access_token": grant.secret,
        "token_type": "bearer",
        "expires_in": int((expires - datetime.utcnow()).seconds),
        "scope": grant.grants,
        "refresh_token": grant.refresh_token,
    }

# Sends the OAuth 2 server metadata as specified by RFC 8414.
@oauth2.route("/.well-known/oauth-authorization-server")
@csrf_bypass
@cross_origin
def server_metadata_GET():
    origin = get_origin("meta.sr.ht", external=True)
    scopes = []
    for service in access_grants:
        svc = service["name"]
        for scope in service["scopes"]:
            for access in ["RO", "RW"]:
                scopes.append(f"{svc}/{scope}:{access}")
    return {
        "issuer": origin,
        "authorization_endpoint": origin + "/oauth2/authorize",
        "token_endpoint": origin + "/oauth2/access-token",
        "scopes_supported": scopes,
        "response_types_supported": ["code"],
        "grant_types_supported": ["authorization_code", "refresh_token"],
        "service_documentation": "https://man.sr.ht/meta.sr.ht/oauth.md",
        "introspection_endpoint": origin + "/oauth2/introspect",
        "introspection_endpoint_auth_methods_supported": ["none"],
        "authorization_response_iss_parameter_supported": True,
    }

# Access token introspection defined in RFC 7662.
@oauth2.route("/oauth2/introspect", methods=["POST"])
@csrf_bypass
@cross_origin
def introspect_POST():
    content_type = request.headers.get("Content-Type")
    if content_type != "application/x-www-form-urlencoded":
        return access_token_error("invalid_request",
                "Content-Type must be application/x-www-form-urlencoded")

    token = request.form.get("token")
    if not token:
        return access_token_error("invalid_request", "Missing token parameter")

    client = Client(BearerAuth(token))
    intro = client.get_introspection()
    user, grant = intro.user, intro.grant

    if not intro.grant:
        return { "active": False }

    data = {
        "active": True,
        "client_id": grant.client.uuid,
        "username": user.username,
        "token_type": "bearer",
        "exp": int(grant.expires.timestamp()),
        "iat": int(grant.issued.timestamp()),
    }
    if grant.grants:
        data["scope"] = grant.grants
    return data
