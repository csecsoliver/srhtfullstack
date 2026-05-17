import grp
import os
import pwd
import sys

from srht.config import cfg, cfgb
from srht.database import db
from srht.validation import Validation

from metasrht.audit import audit_log
from metasrht.auth.base import AuthMethod, get_user
from metasrht.auth_validation import validate_username
from metasrht.types import User, UserType


class PamAuthMethod(AuthMethod):
    def __init__(self):
        try:
            from pam import pam
            self.pam = pam
        except ImportError:
            print(
                "could not import 'pam', this is necessary for PAM "
                "authentication; please install python_pam or change "
                "'auth-method=unix-pam' in the configuration file.",
                file=sys.stderr)
            sys.exit(1)

        self.domain = cfg('meta.sr.ht::auth::unix-pam', 'email-default-domain')
        self.service = cfg('meta.sr.ht::auth::unix-pam', 'service', 'sshd')
        self.create_users = cfgb('meta.sr.ht::auth::unix-pam', 'create-users')
        user_group = cfg('meta.sr.ht::auth::unix-pam', 'user-group', '')
        admin_group = cfg('meta.sr.ht::auth::unix-pam', 'admin-group', '')
        self.user_group = grp.getgrnam(user_group).gr_gid if user_group \
            else None
        self.admin_group = grp.getgrnam(admin_group).gr_gid if admin_group \
            else None

    def user_valid(self, valid: Validation, username: str, password: str) \
            -> bool:
        user = get_user(username)

        if user is None:
            # Since users will get auto-created here (in prepare_user), validate
            # the username to ensure valid names in the database
            valid_dummy = Validation({})

            validate_username(valid_dummy, username)

            if not valid_dummy.ok:
                valid.error('Username or password incorrect')
                return False

            if not self.create_users:
                valid.error('Username or password incorrect')
                return False
        else:
            # Make sure we're using the actual user name for PAM authentication,
            # even when the user logs in with the email address
            username = user.username

        if not self.pam().authenticate(username, password, self.service):
            valid.error('Username or password incorrect')
            return False

        if self.user_group is not None:
            groups = get_user_groups(username)
            if self.user_group not in groups and (
                    self.admin_group is None or self.admin_group not in groups):
                valid.error('Username or password incorrect')
                return False

        return True

    def prepare_user(self, username: str) -> User:
        user = get_user(username)

        if user is None:
            assert self.create_users, \
                "tried to call prepare_user for an user that doesn't exist, " \
                "and create_users is false"
            user = self.create(username)

        if self.admin_group is not None:
            user_groups = os.getgrouplist(user.username,
                                          pwd.getpwnam(user.username).pw_gid)

            should_be_admin = False
            if self.admin_group in user_groups:
                should_be_admin = True

            is_admin = user.user_type == UserType.admin

            if should_be_admin and not is_admin:
                user.user_type = UserType.admin
                db.session.commit()
            elif not should_be_admin and is_admin:
                user.user_type = UserType.user
                db.session.commit()

        return user

    def create(self, username: str) -> User:
        user = User(username)
        user.email = f'{username}@{self.domain}'
        user.password = ''

        user.user_type = UserType.user

        db.session.add(user)
        db.session.commit()

        audit_log("account created", user=user)

        return user


def get_user_groups(username: str) -> [int]:
    return os.getgrouplist(username, pwd.getpwnam(username).pw_gid)
