import json
import pygments
from datetime import timedelta
from flask import Blueprint, render_template, request, redirect, Response
from flask import url_for, abort
from hashlib import sha1
from markupsafe import Markup
from pastesrht.access import get_paste_or_abort, get_user_or_abort, has_access, UserAccess
from pastesrht.graphql import Client, Upload, Visibility
from pastesrht.search import apply_search
from pastesrht.types import Paste, Blob
from pygments import highlight
from pygments.formatters import HtmlFormatter
from pygments.lexers import guess_lexer, guess_lexer_for_filename, TextLexer
from srht.app import paginate_query
from srht.database import db
from srht.oauth import current_user, loginrequired
from srht.validation import Validation

public = Blueprint("public", __name__)

@public.route("/")
def index():
    if current_user:
        return render_template("new-paste.html")
    return render_template("index.html")

def create_paste(valid, files, visibility):
    client = Client()

    uploads = []
    for file in files:
        filename = file.get("filename")
        contents = file.get("content")
        if contents is None:
            abort(404)
        uploads.append(Upload(filename, contents, "text/plain"))

    paste = client.create_paste(uploads, visibility)
    return paste.create.id

@loginrequired
@public.route("/new-paste", methods=["POST"])
def new_paste_POST():
    valid = Validation(request)
    contents = valid.require("contents", friendly_name="File contents")
    if contents:
        contents = contents.replace("\r\n", "\n").replace("\r", "\n")
    filename = valid.optional("filename")
    commit = valid.require("commit")
    visibility = valid.require("visibility", cls=Visibility)
    filename = filename.strip() if filename else filename

    files = valid.optional("files")
    files = json.loads(files) if files else []
    valid.kwargs.pop("files", None)

    def dict_without(d, key):
        new_d = d.copy()
        new_d.pop(key)
        return new_d

    if commit == "force":
        valid.errors = [] # Clear validation errors since contents is not required
        paste_id = create_paste(valid, files, visibility)
        if not valid.ok:
            return render_template("new-paste.html", visibility=visibility,
                    **dict_without(valid.kwargs, "visibility"))
        return redirect(url_for(".paste_GET", user=current_user.username,
                sha=paste_id))

    for f in files:
        if f.get("filename") == filename:
            # TODO: Edit this file?
            valid.error("A file with this name already exists in this paste.",
                    field="filename")
    if not valid.ok:
        return render_template("new-paste.html",
                files=files, visibility=visibility,
                **dict_without(valid.kwargs, "visibility"))

    sha = sha1()
    sha.update(contents.encode())
    sha = sha.hexdigest()

    files.append({
        "sha": sha,
        "filename": filename,
        "size": len(contents),
        "content": contents,
    })

    if commit == "no":
        return render_template("new-paste.html",
                files=files, visibility=visibility)

    paste_id = create_paste(valid, files, visibility)
    if not valid.ok:
        return render_template("new-paste.html",
                files=files, visibility=visibility,
                **dict_without(valid.kwargs, "visibility"))
    return redirect(url_for(".paste_GET", user=current_user.username,
            sha=paste_id))

def _get_shebang(data):
    if not data.startswith('#!'):
        return None
    endline = data.find('\n')
    if endline == -1:
        shebang = data
    else:
        shebang = data[:endline]
    return shebang

def _get_lexer(name, data):
    try:
        return guess_lexer_for_filename(name, data)
    except pygments.util.ClassNotFound:
        try:
            shebang = _get_shebang(data)
            if not shebang:
                return TextLexer()

            return guess_lexer(shebang)
        except pygments.util.ClassNotFound:
            return TextLexer()

def highlight_file(name, content):
    lexer = _get_lexer(name, content)
    formatter = HtmlFormatter()
    style = formatter.get_style_defs('.highlight')
    html = f"<style>{style}</style>" + highlight(content, lexer, formatter)
    return Markup(html)

@public.route("/~<user>")
def pastes_user(user):
    user = get_user_or_abort(user)
    pastes = (Paste.query
            .filter(Paste.user_id == user.id)
            .filter(Paste.sha != None)
            .order_by(Paste.updated.desc()))
    if (current_user and user.id != current_user.id) or not current_user:
        pastes = pastes.filter(Paste.visibility == Visibility.PUBLIC)

    try:
        terms = request.args.get("search")
        pastes = apply_search(pastes, terms)
        search_error = None
    except ValueError as e:
        search_error = str(e)

    pastes, pagination = paginate_query(pastes)
    return render_template("pastes.html", pastes=pastes, user=user,
            search=terms, search_error=search_error, **pagination)

@public.route("/~<user>/<sha>")
def paste_GET(user, sha):
    user, paste = get_paste_or_abort(user, sha)
    return render_template("paste.html", paste=paste,
            highlight_file=highlight_file)

@loginrequired
@public.route("/~<user>/<sha>/manage")
def paste_manage(user, sha):
    user, paste = get_paste_or_abort(user, sha)
    if not has_access(current_user, paste, UserAccess.write):
        abort(401)

    return render_template("paste-manage.html", paste=paste)

@loginrequired
@public.route("/~<user>/<sha>/manage", methods=['POST'])
def paste_manage_POST(user, sha):
    user, paste = get_paste_or_abort(user, sha)
    if not has_access(current_user, paste, UserAccess.write):
        abort(401)

    valid = Validation(request)
    visibility = valid.optional("visibility",
            cls=Visibility,
            default=paste.visibility)

    client = Client()
    client.update_paste(sha, visibility)
    return redirect(url_for('.paste_manage', user=user.username, sha=sha))

@loginrequired
@public.route("/~<user>/<sha>/delete")
def paste_delete(user, sha):
    user, paste = get_paste_or_abort(user, sha)
    if not has_access(current_user, paste, UserAccess.write):
        abort(401)

    return render_template("paste-delete.html", paste=paste)

@loginrequired
@public.route("/~<user>/<sha>/delete", methods=['POST'])
def paste_delete_POST(user, sha):
    user, paste = get_paste_or_abort(user, sha)
    if not has_access(current_user, paste, UserAccess.write):
        abort(401)

    client = Client()
    client.delete_paste(sha)
    return redirect(url_for('.index'))

@public.route("/blob/<sha>")
def blob_GET(sha):
    blob = Blob.query.filter(Blob.sha == sha).one_or_none()
    if not blob:
        abort(404)
    return Response(blob.contents, mimetype="text/plain")

@loginrequired
@public.route("/pastes")
def pastes():
    return redirect(url_for('.pastes_user', user=current_user.username))
