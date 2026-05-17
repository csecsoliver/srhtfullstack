from srht.config import cfg, cfgb
from srht.validation import Validation

from metasrht.auth.builtin import BuiltinAuthMethod
from metasrht.auth.pam import PamAuthMethod
from metasrht.types.user import User

auth_method = cfg('meta.sr.ht::auth', 'auth-method', 'builtin')

_auth_method_types = {
    'builtin': BuiltinAuthMethod,
    'unix-pam': PamAuthMethod,
}

if auth_method not in _auth_method_types:
    methods = ', '.join(k for k in _auth_method_types.keys())
    raise Exception(f"invalid auth-method {auth_method}; "
                    f"must be one of {methods}")

_auth_method = _auth_method_types[auth_method]()


def allow_registration() -> bool:
    return (not is_external_auth() and
            cfgb("meta.sr.ht::settings", "registration"))


def is_external_auth() -> bool:
    return auth_method != 'builtin'

def allow_password_reset() -> bool:
    return auth_method == 'builtin'

def user_valid(valid: Validation, user: str, password: str) -> bool:
    return _auth_method.user_valid(valid, user, password)

def prepare_user(user: str) -> User:
    return _auth_method.prepare_user(user)

def set_user_password(user: User, password: str) -> None:
    return _auth_method.set_user_password(user, password)

def set_user_email(user: User, email: str) -> None:
    return _auth_method.set_user_email(user, email)
