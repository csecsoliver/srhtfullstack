import bcrypt

from srht.database import db
from srht.validation import Validation

from metasrht.auth.base import AuthMethod, get_user
from metasrht.types.user import User


def check_password(password: str, hash: str) -> bool:
    return bcrypt.checkpw(password.encode(), hash.encode())


def hash_password(password: str) -> str:
    return bcrypt.hashpw(password.encode(), salt=bcrypt.gensalt()).decode()


class BuiltinAuthMethod(AuthMethod):
    def user_valid(self, valid: Validation,
            username: str, password: str) -> bool:
        username = get_user(username)

        valid.expect(username is not None, "Username or password incorrect")

        if valid.ok:
            valid.expect(username.password, "Username or password incorrect")

        if valid.ok:
            valid.expect(check_password(password, username.password),
                         "Username or password incorrect")

        return valid.ok

    def prepare_user(self, username: str) -> User:
        return get_user(username)

    def set_user_password(self, user: User, password: str) -> None:
        user.password = hash_password(password)
        db.session.commit()
