from flask import Blueprint, abort, current_app, request, session
from markupsafe import Markup, escape
import binascii
import os
import secrets

def issue_csrf_token():
    if '_csrf_token_v2' not in session:
        session['_csrf_token_v2'] = binascii.hexlify(os.urandom(64)).decode()
    return Markup("""<input
        type='hidden'
        name='_csrf_token'
        value='{}' />""".format(escape(session['_csrf_token_v2'])))

_csrf_bypass_views = set()
_csrf_bypass_blueprints = set()

def verify_request_csrf():
    if request.method != 'POST':
        return
    if request.blueprint in _csrf_bypass_blueprints:
        return
    view = current_app.view_functions.get(request.endpoint)
    if not view:
        return
    view = "{0}.{1}".format(view.__module__, view.__name__)
    if view in _csrf_bypass_views:
        return
    # TODO: Remove this exception
    if request.path.startswith("/api"):
        return
    token = session.get('_csrf_token_v2', None)
    if not token:
        abort(403)
    if not request.form.get('_csrf_token'):
        abort(403)
    if not secrets.compare_digest(token, request.form.get('_csrf_token')):
        abort(403)

def csrf_bypass(f):
    if isinstance(f, Blueprint):
        _csrf_bypass_blueprints.update([f])
    else:
        view = '.'.join((f.__module__, f.__name__))
        _csrf_bypass_views.update([view])
    return f
