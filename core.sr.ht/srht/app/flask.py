from cryptography.fernet import InvalidToken
from flask import Flask as FlaskApp
from flask import Response, request, url_for, render_template, redirect
from flask import g, abort, session as flask_session, make_response
from flask import current_app
from jinja2 import PackageLoader, ChoiceLoader
from srht.app import icon, datef, date_handler
from srht.app.csrf import issue_csrf_token, verify_request_csrf, csrf_bypass
from srht.app.pagination import pagination, coalesce_search_terms
from srht.assets import static_blueprint, static_resource
from srht.config import cfg, cfgi, cfgkeys, config
from srht.config import get_api, get_origin, get_global_domain
from srht.crypto import fernet, verify_request_signature
from srht.database import db
from srht.email import mail_exception
from srht.markdown import markdown
from srht.oauth import OAuthService
from srht.rid import to_rid, from_rid
from srht.validation import Validation
from prometheus_client import Histogram, CollectorRegistry, REGISTRY, make_wsgi_app
from prometheus_client.multiprocess import MultiProcessCollector
from timeit import default_timer
from urllib.parse import urlparse, quote, quote_plus
from werkzeug.middleware.dispatcher import DispatcherMiddleware
from werkzeug.routing import UnicodeConverter
import json
import os
import psycopg2.errors
import sqlalchemy.exc
from importlib import resources

_network = [
        s for s in config
        if s.endswith(".sr.ht") and s not in [
            "paste.sr.ht",
            "pages.sr.ht",
        ]
]

class ModifiedUnicodeConverter(UnicodeConverter):
    """Added ~ and ^ to safe URL characters, otherwise no changes."""
    def to_url(self, value):
        if not isinstance(value, str):
            value = str(value)
        return quote(value, safe='/:~^')

class Flask(FlaskApp):
    def __init__(self, site, name, *args,
             user_class=None, legacy_oauthtoken_class=None, **kwargs):
        super().__init__(name, static_folder=None, *args, **kwargs)
        self.site = site

        if os.environ.get("prometheus_multiproc_dir"):
            self.metrics_registry = CollectorRegistry()
            MultiProcessCollector(self.metrics_registry)
        else:
            self.metrics_registry = REGISTRY
        self.wsgi_app = DispatcherMiddleware(self.wsgi_app, {
            "/metrics": make_wsgi_app(registry=self.metrics_registry),
        })
        self.metrics = type("metrics", tuple(), {
            m.describe()[0].name: m
            for m in [
                Histogram("request_time", "Duration of HTTP requests", [
                    "method", "route", "status"
                ]),
            ]
        })

        self.url_map.converters['default'] = ModifiedUnicodeConverter
        self.url_map.converters['string'] = ModifiedUnicodeConverter

        self.register_blueprint(static_blueprint)

        # XXX LEGACY
        self.oauth_service = OAuthService(self.site,
                user_class=user_class, oauthtoken_class=legacy_oauthtoken_class)

        mod_files = resources.files(__import__(name))
        try:
            self.graphql_schema = mod_files.joinpath("schema.graphqls").read_text()
            self.graphql_query = mod_files.joinpath("default_query.graphql").read_text()
        except:
            pass

        choices = [
            PackageLoader(name),
            PackageLoader("srht"),
        ]

        self.jinja_env.globals['pagination'] = pagination
        self.jinja_env.globals['icon'] = icon
        self.jinja_env.filters['date'] = datef
        self.jinja_env.globals['csrf_token'] = issue_csrf_token

        self.jinja_loader = ChoiceLoader(choices)
        self.jinja_env.add_extension('jinja2.ext.do')
        self.secret_key = cfg("sr.ht", "service-key",
            default=cfg("sr.ht", "secret-key", default=None))
        if self.secret_key is None:
            raise Exception("[sr.ht]service-key missing from config")

        @self.before_request
        def _csrf_check():
            verify_request_csrf()

        @self.teardown_appcontext
        def expire_db(err):
            db.session.expire_all()

        @self.errorhandler(500)
        def handle_500(e):
            if isinstance(e.original_exception, sqlalchemy.exc.InternalError):
                e = e.original_exception.orig
                if isinstance(e, psycopg2.errors.ReadOnlySqlTransaction):
                    return render_template("read_only.html")
            # shit
            try:
                from srht.oauth import current_user
                user = None
                if hasattr(db, 'session'):
                    db.session.rollback()
                    if current_user:
                        user = f"{current_user.canonical_name} " + \
                                f"<{current_user.email}>"
                    db.session.close()
                mail_exception(e, user=user)
            except Exception as e2:
                # shit shit
                raise e2.with_traceback(e2.__traceback__)
            return render_template("internal_error.html"), 500

        @self.errorhandler(401)
        def handle_401(e):
            if request.path.startswith("/api"):
                return { "errors": [ { "reason": "401 unauthorized" } ] }, 401
            return render_template("unauthorized.html"), 401

        @self.errorhandler(404)
        def handle_404(e):
            if request.path.startswith("/api"):
                return { "errors": [ { "reason": "404 not found" } ] }, 404
            return render_template("not_found.html"), 404

        @self.context_processor
        def inject():
            root = get_origin(self.site, external=True)

            ctx = {
                'root': root,
                'domain': urlparse(root).netloc,
                'app': self,
                'len': len,
                'any': any,
                'str': str,
                'request': request,
                'url_for': url_for,
                'cfg': cfg,
                'cfgi': cfgi,
                'cfgkeys': cfgkeys,
                'get_origin': get_origin,
                'get_api': get_api,
                'valid': Validation(request),
                'site': site,
                'site_name': cfg("sr.ht", "site-name", default=None),
                'login_url': self.login_url,
                'logout_url': self.logout_url,
                'environment': cfg("sr.ht", "environment", default="production"),
                'network': _network,
                'static_resource': static_resource,
                'coalesce_search_terms': coalesce_search_terms,
                'to_rid': to_rid,
                'from_rid': from_rid,
            }

            try:
                from srht.oauth import current_user
                user_class = (current_user._get_current_object().__class__
                        if current_user else None)
                ctx = {
                    **ctx,
                    'current_user': (user_class.query
                        .filter(user_class.id == current_user.id)
                    ).one_or_none() if current_user else None,
                }
            except sqlalchemy.orm.exc.DetachedInstanceError:
                pass # Can happen while cleaning up from 500 errors
            except sqlalchemy.exc.InvalidRequestError:
                pass # Can happen while cleaning up from 500 errors
            return ctx

        @self.teardown_appcontext
        def shutdown_session(resp):
            db.session.remove()
            return resp

        @self.template_filter()
        def md(text):
            return markdown(text)

        @self.before_request
        def get_session_cookie():
            cookie = request.cookies.get("sr.ht.unified-login.v1")
            if not cookie:
                return
            try:
                user_info = json.loads(fernet.decrypt(cookie.encode()).decode())
                user = self.oauth_service.lookup_user(user_info["name"])
            except InvalidToken:
                # Remove invalid session cookie
                user = None
            if user:
                g.current_user = user
            else:
                # We can end up here after an account is deleted but the cookie
                # is still around. Get rid of it.
                g.set_current_user = True
                g.current_user = None

        @self.before_request
        def begin_track_request():
            request._srht_start_time = default_timer()

        @self.after_request
        def track_request(resp):
            if not hasattr(request, "_srht_start_time"):
                return resp
            self.metrics.request_time.labels(
                method=request.method,
                route=request.endpoint,
                status=resp.status_code,
            ).observe(max(default_timer() - request._srht_start_time, 0))
            return resp

        # XXX LEGACY
        @self.route("/oauth/webhook/profile-update", methods=["POST"])
        @csrf_bypass
        def profile_update():
            payload = verify_request_signature(request)
            profile = json.loads(payload.decode('utf-8'))
            User = current_app.oauth_service.User
            user = User.query.filter(User.username == profile["name"]).one_or_none()
            if not user:
                return "Unknown user.", 404
            current_app.oauth_service.profile_update_hook(user, profile)
            return f"Profile updated for {user.username}."

    def make_response(self, rv):
        # Converts responses from dicts to JSON response objects
        response = None

        def jsonify_wrap(obj):
            jsonification = json.dumps(obj, default=date_handler)
            return Response(jsonification, mimetype='application/json')

        if isinstance(rv, tuple) and \
            (isinstance(rv[0], dict) or isinstance(rv[0], list)):
            response = jsonify_wrap(rv[0]), rv[1]
        elif isinstance(rv, dict):
            response = jsonify_wrap(rv)
        elif isinstance(rv, list):
            response = jsonify_wrap(rv)
        else:
            response = rv
        response = super(Flask, self).make_response(response)

        global_domain = get_global_domain(self.site)
        if "set_current_user" in g and g.set_current_user:
            cookie_key = f"sr.ht.unified-login.v1"
            if not g.current_user:
                # Clear user info cookie
                response.set_cookie(cookie_key, "",
                        domain=global_domain, 
                        httponly=True,
                        max_age=0)
            else:
                # Set user info cookie
                user_info = g.current_user.to_dict(first_party=True)
                user_info = {k:v for k,v in user_info.items() if k not in ['bio', 'location', 'url']}
                user_info = json.dumps(user_info)
                response.set_cookie(cookie_key,
                        fernet.encrypt(user_info.encode()).decode(),
                        domain=global_domain,
                        httponly=True,
                        max_age=60 * 60 * 24 * 365)

        path = request.path
        return response

    @property
    def login_url(self):
        root = get_origin(self.site, external=True)
        meta = get_origin("meta.sr.ht", external=True)
        return_to = quote_plus(root + request.full_path)
        return f"{meta}/login?return_to={return_to}"

    @property
    def logout_url(self):
        root = get_origin(self.site, external=True)
        meta = get_origin("meta.sr.ht", external=True)
        return_to = quote_plus(root + request.full_path)
        return f"{meta}/logout?return_to={return_to}"
