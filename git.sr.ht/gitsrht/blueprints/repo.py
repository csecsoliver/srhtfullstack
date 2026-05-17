import binascii
import json
import mimetypes
import os
import pygit2
import pygments
import subprocess
import sys
from datetime import datetime, timedelta
from flask import Blueprint, render_template, abort, current_app, send_file, make_response, request
from flask import Response, url_for, session, redirect
from gitsrht.editorconfig import EditorConfig
from gitsrht.formatting import get_formatted_readme, get_highlighted_file
from gitsrht.git import Repository as GitRepository, commit_time, annotate_tree
from gitsrht.git import diffstat, get_log, diff_for_commit, strip_pgp_signature
from gitsrht.rss import generate_refs_feed, generate_commits_feed
from gitsrht.spdx import SPDX_LICENSES
from gitsrht.types import Artifact, User
from gitsrht.urls import clone_urls
from io import BytesIO
from markupsafe import Markup, escape
from jinja2.utils import url_quote
from gitsrht.access import get_repo, get_repo_or_redir
from srht.config import cfg, get_origin
from srht.markdown import markdown, sanitize
from srht.oauth import loginrequired
from urllib.parse import urlparse

repo = Blueprint('repo', __name__)

def get_license_info_for_tip(tip):
        license_exists = False
        licences_names = ["LICENSES", "licenses", "LICENCES", "licences"]
        for path in [
                "LICENSE", "LICENCE", "COPYING",
                "LICENSE.txt", "license.txt",
                "LICENCE.txt", "licence.txt",
                "COPYING.txt", "copying.txt",
                "COPYRIGHT.txt", "copyright.txt",
                "LICENSE.md", "license.md",
                "LICENCE.md", "licence.md",
                "COPYING.md", "copying.md",
                "COPYRIGHT.md", "copyright.md",
                "COPYRIGHT", "copyright",
        ] + licences_names:
            if path in tip.tree:
                license_exists = True
                break

        licenses = []
        for lic in licences_names:
            if lic in tip.tree and isinstance(tip.tree[lic], pygit2.Tree):
                for o in tip.tree[lic]:
                    license_id = o.name
                    if license_id not in SPDX_LICENSES:
                        license_id = os.path.splitext(o.name)[0]
                    if license_id in SPDX_LICENSES:
                        licenses.append({
                            'id': license_id,
                            'name': SPDX_LICENSES[license_id],
                        })
                break

        return license_exists, licenses

def get_readme(repo, git_repo, tip, link_prefix=None):
    if repo.readme is not None:
        return Markup(sanitize(repo.readme))

    if not tip:
        return None

    def file_finder(name):
        try:
            blob = tip.tree[name]
        except KeyError:
            return None, None

        if blob:
            btype = (blob.type_str
                    if hasattr(blob, "type_str") else blob.type)
            if btype == "blob":
                return str(blob.id), blob
        return None, None

    def content_getter(blob):
        return git_repo.get(blob.id).data.decode()

    return get_formatted_readme(file_finder, content_getter,
            link_prefix=link_prefix)

def _highlight_file(repo, ref, entry, data, blob_id, commit_id):
    link_prefix = url_for('repo.tree', owner=repo.owner,
            repo=repo.name, ref=ref)
    if entry.filemode == pygit2.GIT_FILEMODE_LINK:
        return Markup("<div class=\"highlight\"><pre>" +
                f"<a href=\"{url_quote(data.encode('utf-8'))}\">" +
                f"{escape(data)}</a><pre></div>")
    else:
        return get_highlighted_file(entry.name, blob_id, data)

def linecounter(count):
    out = []
    for i in range(1, count + 1):
        out.append(f'<a href="#L{i}" id="L{i}">{i}\n</a>')
    return "".join(out)

def render_empty_repo(owner, repo, view):
    origin = cfg("git.sr.ht", "origin")
    git_user = cfg("git.sr.ht", "ssh-user", "git")
    return render_template("empty-repo.html", owner=owner, repo=repo, view=view,
            clone_urls=clone_urls(repo))

def get_last_3_commits(git_repo, commit):
    commits = list()
    for c in git_repo.walk(commit.id, pygit2.GIT_SORT_NONE):
        commits.append(c)
        if len(commits) >= 3:
            break
    return commits

@repo.route("/<owner>/<repo>")
def summary(owner, repo):
    owner, repo = get_repo_or_redir(owner, repo)

    with GitRepository(repo.path) as git_repo:
        if git_repo.is_empty:
            return render_empty_repo(owner, repo, "summary")

        default_branch = git_repo.default_branch()
        default_branch_name = default_branch.raw_name \
            .decode("utf-8", "replace")[len("refs/heads/"):]
        tip = git_repo.get(default_branch.raw_target)
        commits = get_last_3_commits(git_repo, tip)
        link_prefix = url_for(
            'repo.tree', owner=repo.owner, repo=repo.name,
            ref=f"{default_branch_name}/")  # Trailing slash needed
        blob_prefix = url_for(
            'repo.raw_blob', owner=repo.owner, repo=repo.name,
            ref=f"{default_branch_name}/", path="")  # Trailing slash needed
        readme = get_readme(repo, git_repo, tip,
            link_prefix=[link_prefix, blob_prefix])
        tags = [(ref, git_repo.get(git_repo.references[ref.decode('utf-8')].raw_target))
            for ref in git_repo.raw_listall_references()
            if ref.startswith(b"refs/tags/")]
        tags = [tag for tag in tags
                if (isinstance(tag[1], pygit2.Tag) or isinstance(tag[1], pygit2.Commit))
                and (isinstance(tag[1], pygit2.Commit) or isinstance(tag[1].get_object(), pygit2.Commit))]
        tags = sorted(tags, key=lambda c: commit_time(c[1]), reverse=True)
        latest_tag = tags[0] if len(tags) else None

        message = session.pop("message", None)

        license_exists, licenses = get_license_info_for_tip(tip)

        if latest_tag:
            sig = lookup_signature(git_repo, latest_tag[0].decode('utf-8'))[1]
        else:
            sig = None
        return render_template("summary.html", view="summary",
                owner=owner, repo=repo, readme=readme, commits=commits,
                signature=sig,
                latest_tag=latest_tag, default_branch=default_branch,
                is_annotated=lambda t: isinstance(t, pygit2.Tag),
                message=message, license_exists=license_exists,
                licenses=licenses)

@repo.route("/<owner>/<repo>/<path:path>")
def go_get(owner, repo, path):
    if "go-get" not in request.args:
        abort(404)
    owner, repo = get_repo_or_redir(owner, repo)
    root = get_origin("git.sr.ht", external=True)
    origin = urlparse(root).netloc
    return "".join(['<!doctype html>',
        '<title>Placeholder page for Go import resolution</title>',
        '<meta name="go-import" content="',
            f'{origin}/{owner.canonical_name}/{repo.name} ',
            f'git ',
            f'{root}/{owner.canonical_name}/{repo.name}',
        '" />',
    ])

def lookup_ref(git_repo, ref, path):
    branch = git_repo.default_branch()
    if not branch:
        abort(404)
    ref = ref.encode("utf-8") if ref else branch.raw_name[len(b"refs/heads/"):]
    if not path:
        path = []
    else:
        path = path.split("/")
    commit = None
    try:
        commit = git_repo.revparse_single(ref)
    except KeyError:
        pass
    except ValueError:
        pass
    while commit is None and len(path):
        ref += b"/" + path[0].encode("utf-8")
        path = path[1:]
        try:
            commit = git_repo.revparse_single(ref)
        except KeyError:
            pass
        except ValueError:
            pass
    if commit is None:
        abort(404)
    if isinstance(commit, pygit2.Tag):
        commit = git_repo.get(commit.target)
    if not commit:
        abort(404)
    return commit, ref, "/".join(path)

def lookup_signature(git_repo, ref, fmt=['tar', 'tar.gz']):
    try:
        commit_or_tag = git_repo.revparse_single(ref)
    except (KeyError, ValueError):
        return None, None
    if not isinstance(commit_or_tag, (pygit2.Commit, pygit2.Tag)):
        return None, None

    for trial in fmt:
        try:
            note = git_repo.lookup_note(str(commit_or_tag.id), f'refs/notes/signatures/{trial}')
        except KeyError:
            continue

        return note.message, trial
    return None, None

@repo.route("/<owner>/<repo>/tree", defaults={"ref": None, "path": ""})
@repo.route("/<owner>/<repo>/tree/<path:ref>", defaults={"path": ""})
@repo.route("/<owner>/<repo>/tree/<path:ref>/item/<path:path>")
def tree(owner, repo, ref, path):
    owner, repo = get_repo_or_redir(owner, repo)

    if ref and "/" in ref and not path:
        ref, _, path = ref.partition("/")

    with GitRepository(repo.path) as git_repo:
        if git_repo.is_empty:
            return render_empty_repo(owner, repo, "tree")

        # lookup_ref will cycle through the path to separate
        # the actual ref from the actual path
        commit, ref, path = lookup_ref(git_repo, ref, path)
        refname = ref.decode('utf-8', 'replace')
        if isinstance(commit, pygit2.Tag):
            commit = git_repo.get(commit.target)
        orig_commit = commit
        if not isinstance(commit, pygit2.Commit):
            abort(404)
        tree = commit.tree
        if not tree:
            abort(404)
        editorconfig = EditorConfig(git_repo, tree, path)

        entry = tree
        path = path.split("/")
        for part in path:
            if part == "":
                continue
            if not tree or part not in tree:
                abort(404)
            entry = tree[part]
            etype = (entry.type_str
                    if hasattr(entry, "type_str") else entry.type)
            if etype == "blob":
                tree = annotate_tree(git_repo, tree, commit)
                commit = next(
                        (e.commit for e in tree if e.name == entry.name), None)
                blob = git_repo.get(entry.id)
                data = None
                if not blob.is_binary:
                    try:
                        data = blob.data.decode()
                    except ValueError:
                        data = '[unable to decode]'
                md = not blob.is_binary and entry.name.endswith(".md")
                if md:
                    link_prefix = url_for('repo.tree', owner=repo.owner,
                            repo=repo.name, ref=refname,
                            path=os.path.dirname("/".join(path)))
                    blob_prefix = url_for(
                        'repo.raw_blob', owner=repo.owner, repo=repo.name,
                        ref=refname, path=os.path.dirname("/".join(path)))
                    md = markdown(data,
                            link_prefix=[link_prefix, blob_prefix])
                force_source = "view-source" in request.args

                return render_template("blob.html", view="blob",
                        owner=owner, repo=repo, ref=refname, path=path, entry=entry,
                        blob=blob, data=data, commit=orig_commit,
                        highlight_file=_highlight_file,
                        linecounter=linecounter,
                        editorconfig=editorconfig,
                        markdown=md, force_source=force_source, pygit2=pygit2)
            tree = git_repo.get(entry.id)

        if not tree or tree.type != pygit2.GIT_OBJECT_TREE:
            abort(404)
        tree = annotate_tree(git_repo, tree, commit)
        tree = sorted(tree, key=lambda e: e.name)

        default_branch = git_repo.default_branch()
        tip = git_repo.get(default_branch.raw_target)
        license_exists, licenses = get_license_info_for_tip(tip)

        return render_template("tree.html", view="tree", owner=owner, repo=repo,
                ref=refname, commit=commit, entry=entry, tree=tree, path=path,
                pygit2=pygit2, license_exists=license_exists, licenses=licenses)

def resolve_blob(git_repo, ref, path):
    commit, ref, path = lookup_ref(git_repo, ref, path)
    if not isinstance(commit, pygit2.Commit):
        abort(404)

    blob = None
    entry = None
    tree = commit.tree
    if not tree:
        abort(404)
    orig_commit = commit
    path = path.split("/")
    for part in path:
        if part == "":
            continue
        if part not in tree:
            abort(404)
        entry = tree[part]
        etype = (entry.type_str
                if hasattr(entry, "type_str") else entry.type)
        if etype == "blob":
            tree = annotate_tree(git_repo, tree, commit)
            commit = next(e.commit for e in tree if e.name == entry.name)
            blob = git_repo.get(entry.id)
            break
        tree = git_repo.get(entry.id)
        if not tree:
            abort(404)

    if not blob:
        abort(404)

    return orig_commit, ref, path, blob, entry

def resolve_mimetype(path, blob):
    filename = path[-1]
    mimetype, enc = mimetypes.guess_type(filename)
    if mimetype and mimetype.startswith('image/'):
        return mimetype
    if not blob.is_binary:
        return "text/plain"
    return None

@repo.route("/<owner>/<repo>/blob/<path:ref>/<path:path>")
def raw_blob(owner, repo, ref, path):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        orig_commit, ref, path, blob, entry = resolve_blob(git_repo, ref, path)

        response = send_file(BytesIO(blob.data),
                as_attachment=blob.is_binary,
                download_name=entry.name,
                mimetype=resolve_mimetype(path, blob))
        response = make_response(response)
        # Do not allow any other resources, including scripts, to be loaded from this resourse
        # This prevents XSS attacks in SVG files!
        response.headers['Content-Security-Policy'] = "upgrade-insecure-requests; sandbox; frame-src 'none'; media-src 'none'; script-src 'none'; object-src 'none'; worker-src 'none';"
        return response

def _lookup_user(email, cache):
    if email not in cache:
        cache[email] = current_app.lookup_user(email)
    return cache[email]

def lookup_user():
    cache = {}
    return lambda email: _lookup_user(email, cache)

# We only care about these fields in in blame.html, so we discard
# boundary, final_start_line_number, orig_commit_id, orig_committer,
# orig_path, and orig_start_line_number here
class FakeBlameHunk:
    def __init__(self, hunk):
        self.final_commit_id = hunk.final_commit_id
        self.final_committer = hunk.final_committer
        self.lines_in_hunk = hunk.lines_in_hunk

# Blame hunks of the same final commit are split if they're not consecutive
# lines in the original commit (cf. https://todo.sr.ht/~sircmpwn/git.sr.ht/357)
def weld_hunks(blame):
    last = None
    for nxt in map(FakeBlameHunk, blame):
        if last is None:
            last = nxt
            continue
        if last.final_commit_id == nxt.final_commit_id:
            last.lines_in_hunk += nxt.lines_in_hunk
        else:
            yield last
            last = nxt
    if last is not None:
        yield last

@loginrequired
@repo.route("/<owner>/<repo>/blame/<path:ref>", defaults={"path": ""})
@repo.route("/<owner>/<repo>/blame/<path:ref>/<path:path>")
def blame(owner, repo, ref, path):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        orig_commit, ref, path, blob, entry = resolve_blob(git_repo, ref, path)
        refname = ref.decode('utf-8', 'replace')
        if blob.is_binary:
            return redirect(url_for("repo.log",
                owner=repo.owner.canonical_name, repo=repo.name, ref=refname,
                path="/".join(path)))
        try:
            data = blob.data.decode()
        except ValueError:
            return redirect(url_for("repo.log",
                owner=repo.owner.canonical_name, repo=repo.name, ref=refname,
                path="/".join(path)))

        try:
            blame = git_repo.blame("/".join(path), newest_commit=orig_commit.id)
        except KeyError as ke:  # Path not in the tree
            abort(404)
        except ValueError:
            # ValueError: object at path 'hubsrht/' is not of the asked-for type 3
            abort(400)

        return render_template("blame.html", view="blame", owner=owner,
                repo=repo, ref=refname, path=path, entry=entry, blob=blob, data=data,
                blame=list(weld_hunks(blame)), commit=orig_commit, highlight_file=_highlight_file,
                editorconfig=EditorConfig(git_repo, orig_commit.tree, path),
                lookup_user=lookup_user(), pygit2=pygit2)

@repo.route("/<owner>/<repo>/archive/<path:ref>.tar.gz", defaults = {"fmt": "tar.gz"})
@repo.route("/<owner>/<repo>/archive/<path:ref>.<any('tar.gz','tar'):fmt>")
def archive(owner, repo, ref, fmt):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        commit, ref, _ = lookup_ref(git_repo, ref, None)
        if not isinstance(commit, pygit2.Commit):
            abort(404)

        refname = ref.decode('utf-8', 'replace')
        args = [
            "git",
            "--git-dir", repo.path,
            "archive",
            "--format", fmt,
            "--prefix", f"{repo.name}-{refname}/",
            "--",
            ref
        ]
        subp = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=sys.stderr)

        return send_file(subp.stdout, mimetype="application/tar+gzip",
                as_attachment=True, download_name=f"{repo.name}-{refname}.{fmt}")

@repo.route("/<owner>/<repo>/archive/<path:ref>.<any('tar.gz','tar'):fmt>.asc")
def archivesig(owner, repo, ref, fmt):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        sigdata, _ = lookup_signature(git_repo, ref, [fmt])
        if sigdata is None:
            abort(404)

        return send_file(BytesIO(sigdata.encode('utf-8')), mimetype="application/pgp-signature",
                as_attachment=True, download_name=f"{repo.name}-{ref}.{fmt}.asc")

class _AnnotatedRef:
    def __init__(self, repo, ref):
        self.ref = ref
        self.target = ref.target
        if ref.raw_name.startswith(b"refs/heads/"):
            self.type = "branch"
            self.name = ref.raw_name[len(b"refs/heads/"):]
            self.branch = repo.get(ref.target)
            self.commit = self.branch
        elif ref.raw_name.startswith(b"refs/tags/"):
            self.type = "tag"
            self.name = ref.raw_name[len(b"refs/tags/"):]
            self.tag = repo.get(self.target)
            if isinstance(self.tag, pygit2.Commit):
                self.commit = self.tag
            elif isinstance(self.tag, pygit2.Tag):
                self.commit = repo.get(self.tag.target)
        else:
            self.type = None

def collect_refs(git_repo):
    refs = {}
    for _ref in git_repo.raw_listall_references():
        _ref = _AnnotatedRef(git_repo, git_repo.references[_ref.decode('utf-8', 'replace')])
        if not _ref.type or not hasattr(_ref, "commit"):
            continue
        if str(_ref.commit.id) not in refs:
            refs[str(_ref.commit.id)] = []
        refs[str(_ref.commit.id)].append(_ref)
    return refs

@repo.route("/<owner>/<repo>/log", defaults={"ref": None, "path": ""})
@repo.route("/<owner>/<repo>/log/<path:ref>", defaults={"path": ""})
@repo.route("/<owner>/<repo>/log/<path:ref>/item/<path:path>")
def log(owner, repo, ref, path):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        if git_repo.is_empty:
            return render_empty_repo(owner, repo, "log")

        commit, ref, path = lookup_ref(git_repo, ref, path)
        refname = ref.decode("utf-8", "replace")
        if not isinstance(commit, pygit2.Commit):
            abort(404)
        refs = collect_refs(git_repo)

        from_id = request.args.get("from")
        if from_id:
            try:
                commit = git_repo.get(from_id)
            except ValueError:
                abort(404)
        if not commit:
            abort(404)

        num_commits = 20
        commits = get_log(git_repo, commit, path, num_commits + 1)

        entry = None
        if path and commit.tree and path in commit.tree:
            entry = commit.tree[path]

        has_more = commits and len(commits) == num_commits + 1
        next_commit = commits[-1] if has_more else None

        author_emails = set((commit.author.email for commit in commits[:20]))
        authors = {user.email:user for user in User.query.filter(User.email.in_(author_emails)).all()}

        default_branch = git_repo.default_branch()
        tip = git_repo.get(default_branch.raw_target)
        license_exists, licenses = get_license_info_for_tip(tip)

        return render_template("log.html", view="log",
                owner=owner, repo=repo, ref=refname, path=path.split("/"),
                commits=commits[:num_commits], refs=refs, entry=entry, pygit2=pygit2,
                next_commit=next_commit, authors=authors,
                license_exists=license_exists, licenses=licenses)


@repo.route("/<owner>/<repo>/log/rss.xml", defaults={"ref": None})
@repo.route("/<owner>/<repo>/log/<path:ref>/rss.xml")
def log_rss(owner, repo, ref):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        commit, ref, _ = lookup_ref(git_repo, ref, None)
        if not isinstance(commit, pygit2.Commit):
            abort(404)
        commits = get_log(git_repo, commit)
        default_branch = git_repo.default_branch_name()

    repo_name = f"{repo.owner.canonical_name}/{repo.name}"
    title = f"{repo_name} log"
    description = f"Git log for {repo_name} {ref.decode('utf-8', 'replace')}"
    link = cfg("git.sr.ht", "origin") + url_for("repo.log",
        owner=repo.owner.canonical_name,
        repo=repo.name,
        ref=ref if ref != default_branch else None)

    return generate_commits_feed(repo, commits, title, link, description)

@repo.route("/<owner>/<repo>/commit/<path:ref>")
def commit(owner, repo, ref):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        commit, ref, _ = lookup_ref(git_repo, ref, None)
        if not isinstance(commit, pygit2.Commit):
            abort(404)
        parent, diff = diff_for_commit(git_repo, commit)
        refs = collect_refs(git_repo)
        return render_template("commit.html", view="log",
            owner=owner, repo=repo, ref=ref, refs=refs,
            commit=commit, parent=parent,
            diff=diff, diffstat=diffstat, pygit2=pygit2,
            default_branch=git_repo.default_branch())

@repo.route("/<owner>/<repo>/commit/<path:ref>.patch")
def patch(owner, repo, ref):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        commit, ref, _ = lookup_ref(git_repo, ref, None)
        if not isinstance(commit, pygit2.Commit):
            abort(404)
        try:
            commit = git_repo.revparse_single(ref)
        except KeyError:
            abort(404)
        if isinstance(commit, pygit2.Tag):
            ref = git_repo.get(commit.target)
        try:
            subp = subprocess.run([
                "git",
                "--git-dir", repo.path,
                "format-patch",
                "--stdout", "-1",
                "--full-index",
                "--no-signature",
                "--end-of-options",
                str(commit.id)
            ], timeout=10, stdout=subprocess.PIPE, stderr=sys.stderr)
        except subprocess.TimeoutExpired:
            return "Operation timed out", 500
        if subp.returncode != 0:
            return "Error preparing patch", 500
        return Response(subp.stdout, mimetype='text/plain')

@repo.route("/<owner>/<repo>/refs")
def refs(owner, repo):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        if git_repo.is_empty:
            return render_empty_repo(owner, repo, "refs")

        tags = [(
                ref,
                git_repo.get(git_repo.references[ref.decode('utf-8')].target),
                lookup_signature(git_repo, ref.decode('utf-8'))[1]
            ) for ref in git_repo.raw_listall_references()
              if ref.startswith(b"refs/tags/")]
        tags = [tag for tag in tags
                if isinstance(tag[1], pygit2.Commit) or isinstance(tag[1], pygit2.Tag)]
        def _tag_key(tag):
            if isinstance(tag[1], pygit2.Commit):
                return tag[1].commit_time
            elif isinstance(tag[1], pygit2.Tag):
                return _tag_key([None, tag[1].get_object()])
            return 0
        tags = sorted(tags, key=_tag_key, reverse=True)
        branches = [(
                branch,
                git_repo.branches[branch],
                git_repo.get(git_repo.branches[branch].target)
            ) for branch in git_repo.raw_listall_branches(pygit2.GIT_BRANCH_LOCAL)]
        default_branch = git_repo.default_branch()
        if default_branch:
            _branch_key = lambda b: (b[1].raw_name == default_branch.raw_name, b[2].commit_time)
        else:
            _branch_key = lambda b: b[2].commit_time
        branches = sorted(branches,
                key=_branch_key,
                reverse=True)

        results_per_page = 10
        page = request.args.get("page")
        total_results = len(tags)
        total_pages = total_results // results_per_page + 1
        if total_results % results_per_page == 0:
            total_pages -= 1
        if page is not None:
            try:
                page = int(page) - 1
                tags = tags[page*results_per_page:page*results_per_page+results_per_page]
            except Exception:
                page = 0
        else:
            page = 0
            tags = tags[:results_per_page]

        tip = git_repo.get(default_branch.raw_target)
        license_exists, licenses = get_license_info_for_tip(tip)

        return render_template("refs.html", view="refs",
                owner=owner, repo=repo, tags=tags, branches=branches,
                git_repo=git_repo, isinstance=isinstance, pygit2=pygit2,
                page=page + 1, total_pages=total_pages,
                default_branch=default_branch,
                strip_pgp_signature=strip_pgp_signature,
                license_exists=license_exists, licenses=licenses)

@repo.route("/<owner>/<repo>/licenses")
def licenses(owner, repo):
    owner, repo = get_repo_or_redir(owner, repo)

    with GitRepository(repo.path) as git_repo:
        if git_repo.is_empty:
            return render_empty_repo(owner, repo, "licenses")

        default_branch = git_repo.default_branch()
        if not default_branch:
            return render_empty_repo(owner, repo, "licenses")

        default_branch_name = default_branch.raw_name \
            .decode("utf-8", "replace")[len("refs/heads/"):]
        tip = git_repo.get(default_branch.raw_target)

        license_exists, licenses = get_license_info_for_tip(tip)

        message = session.pop("message", None)

        return render_template("licenses.html", view="licenses",
                owner=owner, repo=repo,
                message=message, license_exists=license_exists,
                licenses=licenses)


@repo.route("/<owner>/<repo>/refs/rss.xml")
def refs_rss(owner, repo):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        references = [(
                ref,
                git_repo.get(git_repo.references[ref.decode('utf-8')].target),
                lookup_signature(git_repo, ref.decode('utf-8'))[1]
            ) for ref in git_repo.raw_listall_references()
              if ref.startswith(b"refs/tags/")]

    def _tag_key(tag):
        if isinstance(tag[1], pygit2.Commit):
            return tag[1].commit_time
        elif isinstance(tag[1], pygit2.Tag):
            return _tag_key([None, tag[1].get_object()])
        return 0

    references = sorted(references, key=_tag_key, reverse=True)[:20]

    repo_name = f"{repo.owner.canonical_name}/{repo.name}"
    title = f"{repo_name} refs"
    description = f"Git refs for {repo_name}"
    link = cfg("git.sr.ht", "origin") + url_for("repo.refs",
        owner=repo.owner.canonical_name, repo=repo.name)

    return generate_refs_feed(repo, references, title, link, description)

@repo.route("/<owner>/<repo>/refs/<path:ref>")
def ref(owner, repo, ref):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        try:
            tag = git_repo.revparse_single(ref)
        except KeyError:
            abort(404)
        except ValueError:
            abort(404)
        if isinstance(tag, pygit2.Commit):
            return redirect(url_for(".commit",
                owner=owner, repo=repo.name, ref=str(tag.id)))
        artifacts = (Artifact.query
                .filter(Artifact.user_id == repo.owner_id)
                .filter(Artifact.repo_id == repo.id)
                .filter(Artifact.commit == str(tag.target))).all()
        artifacts.sort(key=lambda ar: ar.filename)
        return render_template("ref.html", view="refs",
                owner=owner, repo=repo, git_repo=git_repo, tag=tag,
                signature=lookup_signature(git_repo, ref)[1],
                artifacts=artifacts, default_branch=git_repo.default_branch(),
                strip_pgp_signature=strip_pgp_signature)
