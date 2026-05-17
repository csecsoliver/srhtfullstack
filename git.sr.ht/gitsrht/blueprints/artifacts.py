import requests
from flask import Blueprint, redirect, render_template, request
from flask import send_file, abort, url_for
from gitsrht.access import check_access, UserAccess
from gitsrht.git import Repository as GitRepository, strip_pgp_signature
from gitsrht.graphql import Client, Upload, GraphQLClientGraphQLMultiError
from srht.crypto import encrypt_request_authorization
from srht.graphql import InternalAuth, Error, has_error
from srht.oauth import loginrequired
from srht.validation import Validation

artifacts = Blueprint('artifacts', __name__)

@artifacts.route("/<owner>/<repo>/refs/upload/<path:ref>", methods=["POST"])
@loginrequired
def ref_upload(owner, repo, ref):
    client = Client()
    owner, repo = check_access(owner, repo, UserAccess.manage)
    with GitRepository(repo.path) as git_repo:
        valid = Validation(request)
        valid.expect(request.files.get("file"), "File is required", field="file")
        file_list = request.files.getlist("file")
        default_branch = git_repo.default_branch()
        if not valid.ok:
            return render_template("ref.html", view="refs",
                    owner=owner, repo=repo, git_repo=git_repo, tag=ref,
                    strip_pgp_signature=strip_pgp_signature,
                    default_branch=default_branch, **valid.kwargs)
        for f in file_list:
            upload = Upload(f.filename, f, "application/octet-stream")
            with valid:
                client.upload_artifact(repo.id, f"refs/tags/{ref}", upload)
            if not valid.ok:
                return render_template("ref.html", view="refs",
                        owner=owner, repo=repo, git_repo=git_repo, tag=ref,
                        strip_pgp_signature=strip_pgp_signature,
                        default_branch=default_branch, **valid.kwargs)
        return redirect(url_for("repo.ref",
            owner=owner.canonical_name,
            repo=repo.name,
            ref=ref))

@artifacts.route("/<owner>/<repo>/refs/download/<path:ref>/<filename>")
def ref_download(owner, repo, ref, filename):
    owner, repo = check_access(owner, repo, UserAccess.read)

    auth = InternalAuth(owner)
    try:
        ref = Client(auth).get_artifact_url(owner.username, repo.name,
            f"refs/tags/{ref}", filename).user.repository.reference
    except GraphQLClientGraphQLMultiError as err:
        if has_error(err, Error.NOT_FOUND):
            abort(404)
        raise

    if ref is None or ref.artifact is None:
        abort(404)

    artifact = ref.artifact

    auth = encrypt_request_authorization(user=owner)
    resp = requests.get(artifact.url, headers=auth, stream=True)
    return send_file(resp.raw,
        mimetype="application/octet-stream",
        as_attachment=True,
        download_name=artifact.filename)

@artifacts.route("/~<owner>/<repo>/refs/delete/<path:ref>/<filename>", methods=["POST"])
@loginrequired
def ref_delete(owner, repo, ref, filename):
    client = Client()
    check_access("~" + owner, repo, UserAccess.manage)

    try:
        reference = client.get_artifact(owner, repo, f"refs/tags/{ref}",
            filename).user.repository.reference
    except GraphQLClientGraphQLMultiError as err:
        if has_error(err, Error.NOT_FOUND):
            abort(404)
        raise

    if not reference or not reference.artifact:
        abort(404)

    client.delete_artifact(reference.artifact.id)
    return redirect(url_for("repo.ref",
        owner="~" + owner, repo=repo, ref=ref))
