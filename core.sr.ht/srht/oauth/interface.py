from datetime import datetime
from flask import url_for
from sqlalchemy.sql import text
from srht.api import ensure_webhooks
from srht.config import cfg, get_origin
from srht.database import db
from srht.oauth import UserType

metasrht = get_origin("meta.sr.ht")

class OAuthService:
    """
    Implements hooks that sr.ht can use to authorize clients to an
    OAuth-enabled API.
    """
    def __init__(self, site, user_class=None, oauthtoken_class=None):
        self.site = site
        self.User = user_class
        self.OAuthToken = oauthtoken_class

    def get_token(self, token, token_hash):
        """Fetch an OAuth token given the provided token & token_hash."""
        now = datetime.utcnow()
        oauth_token = (self.OAuthToken.query
            .filter(self.OAuthToken.token_hash == token_hash)
            .filter(self.OAuthToken.expires > now)
        ).one_or_none()

        if oauth_token:
            oauth_token.updated = now
            db.session.commit()

        return oauth_token

    def fetch_unknown_user(self, username):
        from srht.graphql import exec_gql, GraphQLError, Error
        """
        Fetch an unknown user profile from meta.sr.ht.
        """
        User = self.User
        try:
            resp = exec_gql("meta.sr.ht", """
            query {
                me {
                    id
                    created
                    updated
                    username
                    email
                    userType
                    url
                    location
                    bio
                    suspensionNotice
                }
            }
            """, user=type("User", tuple(), {"username": username}))
        except GraphQLError as err:
            if not err.has(Error.UNAUTHORIZED):
                raise
            return None

        profile = resp["me"]

        with db.engine.connect() as conn:
            results = conn.execute(text("""
            INSERT INTO "user" (
                id, created, updated,
                username, email, user_type,
                url, location, bio,
                suspension_notice
            ) VALUES (
                :id, :created, :updated,
                :username, :email, :userType,
                :url, :location, :bio,
                :suspensionNotice
            )
            RETURNING
                id, created, updated,
                username, email, user_type,
                url, location, bio,
                suspension_notice;
            """), profile)
            row = results.fetchone()
            conn.commit()

            user = User()
            user.id, user.created, user.updated = row[:3]
            user.username, user.email = row[3:5]
            user.user_type = UserType(row[5])
            user.url, user.location, user.bio = row[6:9]
            user.suspension_notice = row[9]

        # TODO: Get rid of this webhook
        origin = get_origin(self.site)
        webhook_url = origin + url_for("profile_update")
        self.ensure_meta_webhooks(user, {
            webhook_url: ["profile:update"],
        })

        return user

    def lookup_user(self, username):
        User = self.User
        user = User.query.filter(User.username == username).one_or_none()
        if user or self.site == "meta.sr.ht":
            return user
        return self.fetch_unknown_user(username)

    def ensure_meta_webhooks(self, user, webhooks):
        """
        Ensures that the given webhooks are rigged up with meta.sr.ht for this
        user. Webhooks should be a dict whose key is the webhook URL and whose
        values are the list of events to send to that URL.
        """
        try:
            ensure_webhooks(user, f"{metasrht}/api/user/webhooks", webhooks)
        except Exception as ex:
            print("Warning: failed to ensure meta.sr.ht webhooks:")
            print(ex)

    def profile_update_hook(self, user, payload):
        if "user_type" in payload:
            user.user_type = UserType(payload["user_type"])
        if "suspension_notice" in payload:
            user.suspension_notice = payload["suspension_notice"]
        user.email = payload["email"]
        user.bio = payload["bio"]
        user.location = payload["location"]
        user.url = payload["url"]
        db.session.commit()
