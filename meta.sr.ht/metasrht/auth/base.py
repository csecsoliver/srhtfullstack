from typing import Optional

from srht.database import db
from srht.validation import Validation

from metasrht.types.user import User


def get_user(username: str) -> Optional[User]:
    return User.query.filter(
        (User.username == username.lower()) |
        (User.email == username.strip())).one_or_none()


class AuthMethod:
    def user_valid(self, valid: Validation, username: str, password: str) \
            -> bool:
        raise NotImplementedError()

    def prepare_user(self, username: str) -> User:
        raise NotImplementedError()

    def set_user_password(self, user: User, password: str) -> bool:
        raise NotImplementedError()

    def set_user_email(self, user: User, email: str) -> bool:
        user.email = email
        db.session.commit()
