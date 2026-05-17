from mansrht.types import User, Wiki, Visibility
from srht.app import Flask
from srht.config import cfg, get_origin
from srht.database import DbSession
from urllib.parse import urlparse

db = DbSession(cfg("man.sr.ht", "connection-string"))
db.init()

_git_origin = get_origin("git.sr.ht")
_git_origin_ext = get_origin("git.sr.ht", external=True)
_git_ssh_user = cfg("git.sr.ht", "ssh-user", "git")

def git_repo_url(repo):
    name = repo.name
    if repo is Wiki:
        name = wiki.repo_name
    return f"{_git_origin_ext}/{repo.owner.canonical_name}/{repo.name}"

def git_ref_url(repo, ref):
    return f"{_git_origin_ext}/{repo.owner.canonical_name}/{repo.name}/tree/{ref.name}"

def git_clone_urls(repo):
    ssh_host = urlparse(_git_origin_ext).hostname
    return {
        "https": f"{_git_origin_ext}/{repo.owner.canonical_name}/{repo.name}",
        "ssh": f"{_git_ssh_user}@{ssh_host}:{repo.owner.canonical_name}/{repo.name}",
    }

class ManApp(Flask):
    def __init__(self):
        super().__init__("man.sr.ht", __name__, user_class=User)

        from mansrht.blueprints.html import html
        from mansrht.blueprints.create import create
        from mansrht.blueprints.manage import manage
        from mansrht.blueprints.public import public

        self.register_blueprint(html)
        self.register_blueprint(create)
        self.register_blueprint(manage)
        self.register_blueprint(public)

        self.url_map.strict_slashes = False

        @self.context_processor
        def inject():
            return {
                "Visibility": Visibility,
                "git_repo_url": git_repo_url,
                "git_ref_url": git_ref_url,
                "git_clone_urls": git_clone_urls,
            }

app = ManApp()
