from collections import deque
from datetime import datetime, timedelta, timezone
from pygit2 import Repository as GitRepository, Tag
from markupsafe import Markup, escape
from stat import filemode
import pygit2
import json, re
from srht.config import get_origin
from srht.markdown import PlainLink

def strip_pgp_signature(text):
    if not text.strip().endswith("-----END PGP SIGNATURE-----"):
        return text
    i = text.rindex('-----BEGIN PGP SIGNATURE-----')
    if i < 0:
        return text
    return text[:i]

def trim_commit(msg):
    if "\n" not in msg:
        return msg
    return msg[:msg.index("\n")]

def signature_time(signature):
    # Time handling in python is so dumb
    try:
        tzinfo = timezone(timedelta(minutes=signature.offset))
        tzaware = datetime.fromtimestamp(float(signature.time), tzinfo)
        diff = datetime.now(timezone.utc) - tzaware
        return datetime.utcnow() - diff
    except Exception:
        return datetime.utcnow()

def commit_time(commit):
    author = commit.author if hasattr(commit, 'author') else commit.get_object().author
    return signature_time(author)

_gitsrht = get_origin('git.sr.ht')
_commit_id_re = re.compile( r'\b[a-f0-9]{7,40}\b')

def commit_links(message, repo):
    def url_or_mail(match):
        mail = match['mail']
        if mail:
            return f'<a href="mailto:{mail}">{mail}</a>'
        url = match['url']
        return f'<a href="{url}">{url}</a>'

    msg = PlainLink.pattern.sub(url_or_mail, escape(message))

    repo_url = f'{_gitsrht}/{repo.owner.canonical_name}/{repo.name}'
    def commit_link(match):
        try:
            commit = repo.git_repo.revparse_single(match[0])
            return f'<a href="{repo_url}/commit/{commit.id}">{match[0]}</a>'
        except (KeyError, ValueError):
            # not a valid commit id in this repository, ignore
            return match[0]

    return _commit_id_re.sub(commit_link, msg)

def _get_ref(repo, ref):
    return repo._get(ref)

def diff_for_commit(git_repo, commit):
    try:
        parent = git_repo.revparse_single(str(commit.id) + "^")
        diff = git_repo.diff(parent, commit)
    except KeyError:
        parent = None
        diff = commit.tree.diff_to_tree(swap=True)
    diff.find_similar(pygit2.GIT_DIFF_FIND_RENAMES)
    return parent, diff

def get_log(git_repo, commit, path="", commits_per_page=20, until=None):
    commits = list()
    for commit in git_repo.walk(commit.id, pygit2.GIT_SORT_NONE):
        if path:
            _, diff = diff_for_commit(git_repo, commit)
            for patch in diff:
                exact = False
                new_path = patch.delta.new_file.raw_path.decode(errors="replace")
                if new_path == path:
                    path = patch.delta.old_file.path
                    exact = True
                if exact or new_path.startswith(path + "/"):
                    commits.append(commit)
                    break
        else:
            commits.append(commit)

        if until is not None and commit == until:
            break
        elif len(commits) >= commits_per_page:
            break
    return commits

class Repository(GitRepository):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.free()

    def get(self, ref):
        return _get_ref(self, ref)

    def _get(self, ref):
        return super().get(ref)

    def default_branch(self):
        head_ref = self.lookup_reference("HEAD")
        return self.branches.get(head_ref.raw_target[len(b"refs/heads/"):])

    def default_branch_name(self):
        branch = self.default_branch()
        if branch:
            return branch.raw_name.decode("utf-8", "replace")[len("refs/heads/"):]
        else:
            return None

    @property
    def is_empty(self):
        return (len(self.raw_listall_branches(pygit2.GIT_BRANCH_LOCAL)) == 0
                or not self.default_branch())

class AnnotatedTreeEntry:
    def __init__(self, repo, entry):
        self._entry = entry
        self._repo = repo
        self.commit = None
        if entry is not None:
            self.id = str(entry.id)
            self.name = entry.raw_name.decode("utf-8", "replace")
            self.type = (entry.type_str
                    if hasattr(entry, "type_str") else entry.type)
            self.filemode = entry.filemode

    def fetch_blob(self):
        if self.type == "tree":
            self.tree = self._repo.get(self.id)
        else:
            self.blob = self._repo.get(self.id)
        return self

    def serialize(self):
        return {
            "id": self.id,
            "name": self.name,
            "type": self.type,
            "filemode": self.filemode,
            "commit": (str(self.commit.id)
                if hasattr(self, "commit") and self.commit else None),
        }

    @staticmethod
    def deserialize(res, repo):
        _id = res["id"]
        self = AnnotatedTreeEntry(repo, None)
        self.id = res["id"]
        self.name = res["name"]
        self.type = res["type"]
        self.filemode = res["filemode"]
        self.commit = repo.get(res["commit"]) if "commit" in res else None
        return self

    def __hash__(self):
        return hash(f"{self.id}:{self.name}")

    def __eq__(self, other):
        return self.id == other.id and self.name == other.name

    def __repr__(self):
        return f"<AnnotatedTreeEntry {self.name} {self.id}>"

def annotate_tree(repo, tree, commit):
    return [AnnotatedTreeEntry(repo, entry).fetch_blob() for entry in tree]

def _diffstat_mark_up(anchor, delta, data_s):
    return Markup(
            f"<a href='#{escape(anchor)}{escape(delta.old_file.raw_path.decode('utf-8', 'replace'))}'>" +
            f"{escape(data_s)}" +
            f"</a>")

def _diffstat_name(delta, anchor):
    if delta.status == pygit2.GIT_DELTA_DELETED:
        return Markup(escape(delta.old_file.path))
    if delta.old_file.raw_path == delta.new_file.raw_path:
        return _diffstat_mark_up(anchor, delta, delta.old_file.raw_path.decode('utf-8', 'replace'))
    # Based on git/diff.c
    pfx_length = 0
    sfx_length = 0
    old_path = delta.old_file.raw_path
    new_path = delta.new_file.raw_path
    for i in range(min(len(old_path), len(new_path))):
        if old_path[i] != new_path[i]:
            break
        if old_path[i] == b'/'[0]:
            pfx_length = i + 1
    for i in range(1, min(len(old_path), len(new_path)) + 1):
        if old_path[-i] != new_path[-i]:
            break
        if old_path[-i] == b'/'[0] or old_path[-i] == b'.'[0]:
            sfx_length = i

    ret = f"{old_path[:pfx_length].decode('utf-8', 'replace')}"

    if pfx_length != 0 or sfx_length != 0:
        ret += "{"
    if sfx_length != 0:
        ret += (f"{old_path[pfx_length:-sfx_length].decode('utf-8', 'replace')} => " +
                f"{new_path[pfx_length:-sfx_length].decode('utf-8', 'replace')}")
    else:
        ret += (f"{old_path[pfx_length:].decode('utf-8', 'replace')} => " +
                f"{new_path[pfx_length:].decode('utf-8', 'replace')}")
    if pfx_length != 0 or sfx_length != 0:
        ret += "}"

    if sfx_length != 0:
        ret += f"{old_path[-sfx_length:].decode('utf-8', 'replace')}"
    return _diffstat_mark_up(anchor, delta, ret)

def _diffstat_line(delta, patch, anchor):
    name = _diffstat_name(delta, anchor)
    change = ""
    if delta.status not in [
                pygit2.GIT_DELTA_ADDED,
                pygit2.GIT_DELTA_DELETED,
            ]:
        if delta.old_file.mode != delta.new_file.mode:
            change = Markup(
                f" <span title='{delta.old_file.mode}'>" +
                f"{filemode(delta.old_file.mode)}</span> => " +
                f"<span title='{delta.new_file.mode}'>" +
                f"{filemode(delta.new_file.mode)}</span>")
    return Markup(f"{delta.status_char()} {name}{change}\n")

def diffstat(diff, anchor=""):
    stat = Markup(f"""{diff.stats.files_changed} files changed, <strong
        class="text-success">{diff.stats.insertions
        }</strong> insertions(+), <strong
        class="text-danger">{diff.stats.deletions
        }</strong> deletions(-)\n\n""")
    for delta, patch in zip(diff.deltas, diff):
        stat += _diffstat_line(delta, patch, anchor)
    return stat
