from flask import redirect, abort, current_app, request
from functools import wraps
from srht.oauth import current_user, UserType

def adminrequired(f):
    @wraps(f)
    def wrapper(*args, **kwargs):
        if not current_user:
            return redirect(current_app.login_url)
        elif current_user.user_type != UserType.admin:
            abort(403)
        else:
            return f(*args, **kwargs)
    return wrapper
