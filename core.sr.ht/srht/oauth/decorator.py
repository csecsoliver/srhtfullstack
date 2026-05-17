from datetime import datetime, timedelta
from flask import current_app, request, g, abort
from functools import wraps
from srht.config import cfg, get_origin
from srht.crypto import encrypt_request_authorization
from srht.crypto import verify_encrypted_authorization
from srht.database import db
from srht.oauth import OAuthError, UserType
from srht.oauth.scope import OAuthScope
from srht.validation import Validation
from prometheus_client import Counter
import hashlib
import requests

metasrht = get_origin("meta.sr.ht")

deprecated_oauth_counter = Counter("deprecated_oauth", "Deprecated legacy OAuth token use")

def _internal_auth(f, auth, *args, **kwargs):
    # Used for authenticating internal API users, like other sr.ht sites.
    oauth_service = current_app.oauth_service
    OAuthToken = oauth_service.OAuthToken
    User = oauth_service.User

    auth = verify_encrypted_authorization(auth)
    node_id = auth["node_id"]
    username = auth.get("name", auth.get("username"))
    assert username

    # Create a synthetic OAuthToken based on the client ID and username
    token = node_id
    token_hash = hashlib.sha512((token + username).encode()).hexdigest()
    oauth_token = OAuthToken.query.filter(
            OAuthToken.token_hash == token_hash).one_or_none()
    if not oauth_token:
        user = User.query.filter(User.username == username).one_or_none()
        if user == None:
            if current_app.site == "meta.sr.ht":
                # This is accepted for /api/user/profile for the purpose of
                # supporting srht.oauth.AbstractOAuthService.fetch_unknown_user
                assert request.path == "/api/user/profile",\
                    "Internal authorization token issued for unknown user"
                abort(404)
            profile = oauth_service.fetch_unknown_user(username)
            user = oauth_service.get_user(profile)
        oauth_token = OAuthToken()
        oauth_token.user_id = user.id
        # Note: the expiration is meaningless
        oauth_token.expires = datetime.utcnow() + timedelta(days=9999)
        oauth_token.token_hash = token_hash
        oauth_token.token_partial = "internal"
        oauth_token._scopes = "*"
        db.session.add(oauth_token)
        db.session.commit()

    g.current_oauth_token = oauth_token
    return f(*args, **kwargs)

def oauth(scopes):
    """
    Validates OAuth authorization for a wrapped function. Scopes should be a
    string-formatted list of required scopes, or None if no particular scopes
    are required.
    """
    def wrap(f):
        @wraps(f)
        def wrapper(*args, **kwargs):
            internal = request.headers.get('X-Srht-Authorization')
            if internal:
                return _internal_auth(f, internal, *args, **kwargs)

            token = request.headers.get('Authorization')
            valid = Validation(request)
            if not token or not (token.startswith('token ')
                    or token.startswith('Bearer ')
                    or token.startswith('Internal ')):
                return valid.error("No authorization supplied (expected an "
                    "OAuth token)", status=401)

            token = token.split(' ')
            if len(token) != 2:
                return valid.error("Invalid authorization supplied", status=401)

            if token[0] == "Internal":
                return _internal_auth(f, token[1], *args, **kwargs)

            token = token[1]
            token_hash = hashlib.sha512(token.encode()).hexdigest()
            deprecated_oauth_counter.inc()

            try:
                oauth_token = current_app.oauth_service.get_token(token, token_hash)
            except OAuthError as err:
                return err.response

            if not oauth_token:
                return valid.error("Invalid or expired OAuth token", status=401)

            g.current_oauth_token = oauth_token

            # No longer possible to specify scopes other than *
            return f(*args, **kwargs)
        return wrapper
    return wrap
