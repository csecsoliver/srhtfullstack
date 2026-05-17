from flask import url_for
from srht.config import cfg, get_origin

def clone_urls(repo):
    """Returns the readonly and read/write URL for a given repo."""
    git_user = cfg("git.sr.ht", "ssh-user", "git")
    base = (get_origin("git.sr.ht", external=True)
        .replace("http://", "")
        .replace("https://", ""))
    return [
        url.format(base, repo.owner.canonical_name, repo.name)
        for url in ["https://{}/{}/{}", git_user+"@{}:{}/{}"]
    ]

def log_rss_url(repo, ref=None):
    ref = ref if ref != repo.git_repo.default_branch_name() else None
    return url_for("repo.log_rss",
        owner=repo.owner.canonical_name,
        repo=repo.name,
        ref=ref)

def refs_rss_url(repo):
    return url_for("repo.refs_rss",
        owner=repo.owner.canonical_name,
        repo=repo.name)
