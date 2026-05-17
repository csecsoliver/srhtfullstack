from flask import session
from metasrht.auth import allow_registration, is_external_auth, allow_password_reset
from metasrht.types import User, UserType, OAuthToken
from metasrht.graphql import PaymentStatus, PaymentInterval, SubscriptionStatus
from srht.app import Flask
from srht.config import cfg, get_s3_upstream
from srht.database import DbSession

db = DbSession(cfg("meta.sr.ht", "connection-string"))
db.init()

class MetaApp(Flask):
    def __init__(self):
        super().__init__("meta.sr.ht", __name__,
            user_class=User, legacy_oauthtoken_class=OAuthToken)

        from metasrht.blueprints.api import register_api
        from metasrht.blueprints.auth import auth
        from metasrht.blueprints.keys import keys
        from metasrht.blueprints.oauth2 import oauth2
        from metasrht.blueprints.privacy import privacy
        from metasrht.blueprints.profile import profile
        from metasrht.blueprints.security import security
        from metasrht.blueprints.users import users
        from srht.graphql import gql_blueprint

        self.register_blueprint(auth)
        self.register_blueprint(keys)
        self.register_blueprint(oauth2)
        self.register_blueprint(privacy)
        self.register_blueprint(profile)
        self.register_blueprint(security)
        self.register_blueprint(users)
        register_api(self)
        self.register_blueprint(gql_blueprint)

        self.jinja_env.globals['allow_registration'] = allow_registration
        self.jinja_env.globals['allow_password_reset'] = allow_password_reset
        self.jinja_env.globals['is_external_auth'] = is_external_auth

        extra_ctx = dict()
        if cfg("meta.sr.ht::billing", "enabled") == "yes":
            from metasrht.blueprints.billing import billing, CURRENCY_SYMBOLS, CURRENCY_NAMES
            self.register_blueprint(billing)
            extra_ctx = {
                    "CURRENCY_SYMBOLS": CURRENCY_SYMBOLS,
                    "CURRENCY_NAMES": CURRENCY_NAMES,
            }

        from metasrht.webhooks import webhook_metrics_collector
        self.metrics_registry.register(webhook_metrics_collector)

        avatar_bucket = cfg("meta.sr.ht", "avatar-bucket", default=None)
        avatar_prefix = cfg("meta.sr.ht", "avatar-prefix", default=None)
        avatar_upstream = get_s3_upstream()

        def avatar_url(user):
            return "/".join([
                avatar_upstream,
                avatar_bucket,
                avatar_prefix,
                user.avatar,
            ])

        @self.context_processor
        def inject():
            return {
                'UserType': UserType,
                'PaymentStatus': PaymentStatus,
                'PaymentInterval': PaymentInterval,
                'SubscriptionStatus': SubscriptionStatus,
                'notice': session.pop('notice', None),
                'avatar_url': avatar_url,
                **extra_ctx,
            }

app = MetaApp()
