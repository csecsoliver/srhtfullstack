from io import BytesIO
from bs4 import BeautifulSoup
from flask import Blueprint, render_template, abort, request, redirect
from flask import send_file
from srht.app import session, date_handler
from srht.markdown import SRHT_MARKDOWN_VERSION, markdown, extract_toc
from srht.oauth import UserType, current_user
from srht.cache import set_cache, get_cache
from srht.crypto import encrypt_request_authorization
from srht.graphql import InternalAuth
from srht.validation import Validation
from mansrht.access import UserAccess, check_access
from mansrht.git import GitClient, ObjectType
from mansrht.git import GetTreeMeRepositoryPath as TreeEntry
from mansrht.git import GetTreeMeRepositoryPathObjectObject as GitObject
from mansrht.types import User, Wiki, RootWiki
from mansrht.wikis import is_root_wiki
from prometheus_client import Counter
from datetime import timedelta
from markupsafe import Markup
from urllib.parse import urlparse
import json
import mimetypes
import os
import requests
import yaml

html = Blueprint('html', __name__)

metrics = type("metrics", tuple(), {
    c.describe()[0].name: c
    for c in [
        Counter("mansrht_tree_cache_access", "Number of tree cache accesses"),
        Counter("mansrht_tree_cache_miss", "Number of tree cache misses"),
        Counter("mansrht_blob_cache_access", "Number of blob cache accesses"),
        Counter("mansrht_blob_cache_miss", "Number of blob cache misses"),
        Counter("mansrht_markdown_cache_access", "Number of markdown cache accesses"),
        Counter("mansrht_markdown_cache_miss", "Number of markdown cache misses"),
    ]
})

class MissingRepositoryError(Exception):
    pass

class MissingReferenceError(Exception):
    def __init__(self, repo):
        self.repo = repo

def get_root_tree(git_client, wiki):
    """
    The root tree (/) in any given git repository is unique because it is not
    itself situated within a parent tree (and is unnamed). We construct a fake
    tree entry here to handle this special case.
    """
    ref = f"refs/heads/{wiki.repo_ref}"
    repo = git_client.get_root_tree(wiki.repo_name, ref).me.repository
    if not repo:
        raise MissingRepositoryError
    if not repo.reference:
        raise MissingReferenceError(repo)

    ref = repo.reference
    tree = GitObject(typename__="Tree", id="-1", type=ObjectType.TREE)
    return repo, TreeEntry(name="/", object=tree), ref.commit

def get_page(wiki, path, is_root=False):
    """
    Fetch the wiki page at the given path. Also fetches the latest commit
    details from git.sr.ht for good measure, to reduce GraphQL round-trips.
    """
    git_client = GitClient(InternalAuth(user=wiki.owner))
    branch = wiki.repo_ref
    ref = f"refs/heads/{branch}"

    if path is None:
        path = ""

    if not path:
        # Special case
        repo, item, commit = get_root_tree(git_client, wiki)
    else:
        repo = git_client.get_tree(wiki.repo_name,
            ref, branch, path.rstrip("/")).me.repository
        if not repo:
            raise MissingRepositoryError
        item, commit = repo.path, repo.reference.commit

    if not commit:
        raise MissingReferenceError(repo)

    n = 0
    # If /foo resolves to a tree:
    # 1. Redirect /foo => /foo/
    # 2. Look up /foo/index.md
    # 3. Account for /foo/index.md/ being a possibility
    while item and item.object.type == ObjectType.TREE and n < 5:
        if not request.path.endswith("/") and (not is_root or path != ""):
            abort(redirect(request.path + "/"))

        index_md = os.path.join(path, "index.md")
        index_html = os.path.join(path, "index.html")

        repo = git_client.get_tree(wiki.repo_name,
            ref, branch, index_md).me.repository
        item, commit = repo.path, repo.reference.commit
        path = index_md

        # Also try index.html for the root wiki
        if not item and is_root:
            repo = git_client.get_tree(wiki.repo_name,
                ref, branch, index_html).me.repository
            item, commit = repo.path, repo.reference.commit
            path = index_html

        n += 1

    # At this point we can definitively give up
    if not item or item.object.type != ObjectType.BLOB:
        abort(404)

    return repo, item, commit

def content(wiki, path, is_root=False):
    try:
        repo, item, commit = get_page(wiki, path, is_root)
    except MissingRepositoryError:
        if current_user == wiki.owner:
            return render_template("missing-repo.html", wiki=wiki)
        abort(404)
    except MissingReferenceError as err:
        if current_user == wiki.owner:
            return render_template("new-wiki.html", wiki=wiki, repo=err.repo)
        abort(404)

    head, tail = os.path.split(path) if path else (None, None)
    path = tuple(p for p in (head, tail) if p)

    cachekey = f"{wiki.repo_name}:{item.object.id}"
    html_cachekey = f"man.sr.ht:content:{cachekey}:v{SRHT_MARKDOWN_VERSION}:v6"
    frontmatter_cachekey = f"man.sr.ht:frontmatter:{cachekey}"
    html = get_cache(html_cachekey)
    metrics.mansrht_markdown_cache_access.inc()
    if not html:
        metrics.mansrht_markdown_cache_miss.inc()

        if not hasattr(item.object, "text"):
            mimetype, enc = mimetypes.guess_type(path[-1] if path else "")
            if mimetype and mimetype.startswith('image/'):
                url = item.object.content
                auth = encrypt_request_authorization(user=current_user)
                resp = requests.get(url, headers=auth, stream=True)
                return send_file(resp.raw, mimetype=mimetype)
            else:
                abort(404)

        md = item.object.text

        frontmatter = dict()
        if md.startswith("---\n"):
            try:
                end = md.index("---\n", 1)
            except ValueError:
                end = -1
            if end != -1:
                frontmatter = md[4:end]
                md = md[end+4:]

            try:
                frontmatter = next(yaml.safe_load_all(frontmatter))
                if not isinstance(frontmatter, dict):
                    raise Exception("Expected frontmatter to be a dictionary")
            except Exception as ex:
                md = "**Warning:** Error parsing YAML frontmatter\n\n" + md
                frontmatter = dict()

        if is_root:
            if item.name.endswith(".html"):
                html = Markup(md)
            elif item.name.endswith(".md"):
                html = markdown(md, baselevel=2, sanitize_output=False)
            else:
                abort(404)
        elif item.name.endswith(".md"):
            html = markdown(md, baselevel=2)
        else:
            abort(404)

        set_cache(html_cachekey, timedelta(days=7), html)
        set_cache(frontmatter_cachekey,
                timedelta(days=7), json.dumps(frontmatter, default=date_handler))
    else:
        html = Markup(html.decode())
        frontmatter = get_cache(frontmatter_cachekey)
        frontmatter = json.loads(frontmatter.decode())

    title = path[-1].rstrip(".md") if path else "index"
    toc = extract_toc(html)
    if "title" in frontmatter:
        title = frontmatter["title"]

    soup = BeautifulSoup(html, "html.parser")
    firstpara = ""
    if frontmatter.get("toc", True) and len(toc) != 0:
        if soup.contents[0].name == "p":
            firstpara = soup.find("p")
            if firstpara:
                firstpara = firstpara.extract()

    return render_template("content.html",
            content=Markup(soup), firstpara=Markup(firstpara),
            title=title, toc=toc, wiki=wiki, is_root=is_root,
            path=path, frontmatter=frontmatter,
            repo=repo, commit=commit)

@html.route("/")
@html.route("/<path:path>")
def root_content(path=None):
    root_wiki = RootWiki.query.all()
    if not root_wiki:
        if current_user and current_user.user_type == UserType.admin:
            session["configure_root"] = True
            return redirect("/wiki/create")
        # Fallback page
        return render_template("index.html")
    root_wiki = root_wiki[0]
    wiki = Wiki.query.filter(Wiki.id == root_wiki.id).first()
    if not wiki:
        abort(404)
    return content(wiki, path, is_root=True)

# The tilde (~) in the route is necessary in order to differentiate between the
# root wiki and user wikis.
@html.route("/~<owner_name>/<wiki_name>")
@html.route("/~<owner_name>/<wiki_name>/")
@html.route("/~<owner_name>/<wiki_name>/<path:path>")
def user_content(owner_name, wiki_name, path=None):
    owner, wiki = check_access(f"~{owner_name}", wiki_name, UserAccess.read)
    if not owner or not wiki:
        abort(404)
    # Redirect to root if it _is_ the root.
    if is_root_wiki(wiki):
        return redirect("/")
    return content(wiki, path)
