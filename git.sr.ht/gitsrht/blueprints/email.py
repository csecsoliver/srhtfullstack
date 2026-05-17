import email
import email.policy
import mailbox
import pygit2
import re
import subprocess
import sys
import hashlib
from email.utils import make_msgid, parseaddr
from email.message import EmailMessage
from flask import Blueprint, render_template, abort, request, url_for, session
from flask import redirect
from gitsrht.access import get_repo_or_redir
from gitsrht.git import Repository as GitRepository, commit_time, diffstat
from gitsrht.git import get_log
from markupsafe import Markup
from srht.config import cfg, cfgi, cfgb
from srht.email import start_smtp
from srht.oauth import loginrequired, current_user
from srht.validation import Validation
from tempfile import NamedTemporaryFile
from textwrap import TextWrapper

mail = Blueprint('mail', __name__)

smtp_from = cfg("mail", "smtp-from", default=None)
outgoing_domain = cfg("git.sr.ht", "outgoing-domain")

def render_send_email_start(owner, repo, git_repo, selected_branch,
        ncommits=8, **kwargs):
    branches = [(
            branch,
            git_repo.branches[branch],
            git_repo.get(git_repo.branches[branch].target)
        ) for branch
          in git_repo.raw_listall_branches(pygit2.GIT_BRANCH_LOCAL)]
    branches = sorted(branches,
            key=lambda b: (b[0].decode() == selected_branch, commit_time(b[2])),
            reverse=True)

    commits = dict()
    for branch in branches[:2]:
        commits[branch[0]] = get_log(git_repo,
                branch[2], commits_per_page=ncommits)

    return render_template("send-email.html",
            view="send-email", owner=owner, repo=repo,
            selected_branch=selected_branch, branches=branches,
            commits=commits, **kwargs)

@mail.route("/<owner>/<repo>/send-email")
@loginrequired
def send_email_start(owner, repo):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        ncommits = int(request.args.get("commits", default=8))
        if ncommits > 32:
            ncommits = 32
        if ncommits < 8:
            ncommits = 8
        selected_branch = request.args.get("branch", default=None)

        return render_send_email_start(owner, repo, git_repo, selected_branch,
                ncommits)

@mail.route("/<owner>/<repo>/send-email/end", methods=["POST"])
@loginrequired
def send_email_end(owner, repo):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        valid = Validation(request)
        branch = valid.require("branch")
        if not branch in git_repo.branches:
            valid.error(f"Branch {branch} not found", field="branch")
        commit = valid.require(f"commit-{branch}")
        if not valid.ok:
            return render_send_email_start(owner, repo, git_repo, branch,
                    **valid.kwargs)

        branch = git_repo.branches[branch]
        tip = git_repo.get(branch.target)
        start = git_repo.get(commit)

        log = get_log(git_repo, tip, until=start)
        diffs = list()
        for commit in log:
            try:
                parent = git_repo.revparse_single(str(commit.id) + "^")
                diff = git_repo.diff(parent, commit)
            except KeyError:
                parent = None
                diff = commit.tree.diff_to_tree(swap=True)
            diff.find_similar(pygit2.GIT_DIFF_FIND_RENAMES)
            diffs.append(diff)

        return render_template("send-email-end.html",
                view="send-email", owner=owner, repo=repo,
                commits=log, start=start, diffs=diffs,
                diffstat=diffstat)

def wrap_each_line(text):
    # Account for TextWrapper ignoring newlines (see Python issue #1859)
    wrapper = TextWrapper(
        expand_tabs=False,
        replace_whitespace=False,
        width=72,
        drop_whitespace=True,
        break_long_words=False)

    short_lines = []
    for long_line in text.splitlines():
        if len(long_line) == 0 or long_line.isspace():
            # Bypass TextWrapper to ensure a line is still inserted.
            short_lines.append('')
        else:
            for short_line in wrapper.wrap(long_line):
                short_lines.append(short_line)
    # Replace the original newline indicators.
    return '\n'.join(short_lines)

commentary_re = re.compile(r"""
---\n
(?P<context>
    (\ .*\ +\|\ +\d+\ [-+]+\n)+
    \ \d+\ files?\ changed,.*\n
    \n
    diff\ --git
)
""", re.MULTILINE | re.VERBOSE)

class PatchsetSizeError(Exception):
    pass

def prepare_patchset(repo, git_repo, cover_letter=None, extra_headers=False,
        to=None, cc=None):
    with NamedTemporaryFile() as ntf:
        valid = Validation(request)
        start_commit = valid.require("start_commit")
        end_commit = valid.require("end_commit")
        version = valid.require("version")
        cover_letter_subject = valid.optional("cover_letter_subject")
        if cover_letter is None:
            cover_letter = valid.optional("cover_letter")
        if not valid.ok:
            return None
        version = int(version)

        args = [
            "git",
            "--git-dir", repo.path,
            "-c", f"user.name={current_user.canonical_name}",
            "-c", f"user.email={current_user.username}@{outgoing_domain}",
            "format-patch",
            f"--from={current_user.canonical_name} <{current_user.username}@{outgoing_domain}>",
            f"--subject-prefix=PATCH {repo.name}",
            "--stdout",
        ]
        if cover_letter:
            args += ["--cover-letter"]
        if version != 1:
            args += ["-v", str(version)]

        start_rev = git_repo.get(start_commit)
        if not start_rev:
            abort(404)
        if start_rev.parent_ids:
            args += [f"{start_commit}^..{end_commit}"]
        else:
            args += ["--root", "--end-of-options", end_commit]
        print(args)
        p = subprocess.run(args, timeout=30,
                stdout=subprocess.PIPE, stderr=sys.stderr)
        if p.returncode != 0:
            abort(400) # TODO: Something more useful, I suppose.

        # Default postfix maximum mail size
        if len(p.stdout) >= 10240000:
            raise PatchsetSizeError()

        ntf.write(p.stdout)
        ntf.flush()

        # By default mailbox.mbox creates email.Message objects. We want the
        # more modern email.EmailMessage class which handles things like header
        # continuation lines better. For this reason we need to explicitly
        # specify a policy via a factory.
        policy = email.policy.default
        factory = lambda f: email.message_from_binary_file(f, policy=policy)
        mbox = mailbox.mbox(ntf.name, factory=factory)
        emails = list(mbox)

        # git-format-patch doesn't set the charset attribute of the
        # Content-Type header field. The Python stdlib assumes ASCII and chokes
        # on UTF-8.
        for msg in emails:
            # replace_header doesn't allow setting params, so we have to unset
            # the header field and re-add it
            t = msg.get_content_type()
            del msg["Content-Type"]
            msg.add_header("Content-Type", t, charset="utf-8")

        if cover_letter:
            subject = emails[0]["Subject"]
            del emails[0]["Subject"]
            emails[0]["Subject"] = (subject
                    .replace("*** SUBJECT HERE ***", cover_letter_subject))
            body = emails[0].get_content()
            cover_letter = wrap_each_line(cover_letter)
            body = body.replace("*** BLURB HERE ***", cover_letter)
            emails[0].set_content(body)

        for i, msg in enumerate(emails[(1 if cover_letter else 0):]):
            commentary = valid.optional(f"commentary_{i}")
            if not commentary:
                commentary = session.get(f"commentary_{i}")
            if not commentary:
                continue
            commentary = wrap_each_line(commentary)
            body = msg.get_content()
            body = commentary_re.sub(r"---\n" + commentary.replace(
                "\\", r"\\") + r"\n\n\g<context>", body, count=1)
            msg.set_content(body)

        if extra_headers:
            msgid = make_msgid().split("@")
            for i, msg in enumerate(emails):
                msg["Message-ID"] = f"{msgid[0]}-{i}@{msgid[1]}"
                msg["X-Mailer"] = "git.sr.ht"
                msg["Reply-to"] = (f"{current_user.canonical_name} " +
                    f"<{current_user.email}>")
                if i != 0:
                    msg["In-Reply-To"] = f"{msgid[0]}-{0}@{msgid[1]}"
                if to:
                    msg["To"] = to
                if cc:
                    msg["Cc"] = cc

        return emails

@mail.route("/<owner>/<repo>/send-email/review", methods=["POST"])
@loginrequired
def send_email_review(owner, repo):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        valid = Validation(request)
        start_commit = valid.require("start_commit")
        end_commit = valid.require("end_commit")
        cover_letter = valid.optional("cover_letter")
        cover_letter_subject = valid.optional("cover_letter_subject")
        version = valid.require("version")
        if cover_letter and not cover_letter_subject:
            valid.error("Cover letter subject is required.",
                    field="cover_letter_subject")
        if cover_letter_subject and not cover_letter:
            valid.error("Cover letter body is required.", field="cover_letter")

        default_branch = git_repo.default_branch()
        tip = git_repo.get(default_branch.target)
        readme = None
        if "README.md" in tip.tree:
            readme = "README.md"
        elif "README" in tip.tree:
            readme = "README"

        try:
            emails = prepare_patchset(repo, git_repo)
        except PatchsetSizeError:
            valid.error(Markup('<strong>Error</strong>: This patchset exceeds the maximum size for an emailed patch set. ' +
                               'Consider using <a href="https://git-scm.com/docs/git-request-pull">git request-pull</a> instead.'))
            emails = None

        start = git_repo.get(start_commit)
        tip = git_repo.get(end_commit)
        if not emails or not valid.ok:
            log = get_log(git_repo, tip, until=start)
            diffs = list()
            for commit in log:
                try:
                    parent = git_repo.revparse_single(str(commit.id) + "^")
                    diff = git_repo.diff(parent, commit)
                except KeyError:
                    parent = None
                    diff = commit.tree.diff_to_tree(swap=True)
                diff.find_similar(pygit2.GIT_DIFF_FIND_RENAMES)
                diffs.append(diff)

            return render_template("send-email-end.html",
                    view="send-email", owner=owner, repo=repo,
                    commits=log, start=start, diffs=diffs,
                    diffstat=diffstat, **valid.kwargs)

        version = int(version)
        for i, email in enumerate(emails):
            comm = valid.optional(f"commentary_{i}")
            if comm:
                session[f"commentary_{i}"] = comm
            else:
                # If a previous "Prepare a patchset" was aborted, the
                # session might contain a stale commentary; clear it.
                session.pop("commentary_{i}", None)

        session["cover_letter"] = cover_letter
        return render_template("send-email-review.html",
                view="send-email", owner=owner, repo=repo,
                readme=readme, emails=emails,
                start=start,
                end=tip,
                cover_letter=bool(cover_letter),
                cover_letter_subject=cover_letter_subject,
                version=version)

@mail.route("/<owner>/<repo>/send-email/send", methods=["POST"])
@loginrequired
def send_email_send(owner, repo):
    owner, repo = get_repo_or_redir(owner, repo)
    with GitRepository(repo.path) as git_repo:
        valid = Validation(request)
        start_commit = valid.require("start_commit")
        end_commit = valid.require("end_commit")
        cover_letter_subject = valid.optional("cover_letter_subject")

        to = valid.require("patchset_to", friendly_name="To")
        cc = valid.optional("patchset_cc")
        recipients = list()

        if to:
            to_recipients = [parseaddr(r)[1] for r in to.split(",")]
            valid.expect('' not in to_recipients,
                    "Invalid recipient.", field="patchset_to")
            recipients += to_recipients
        if cc:
            cc_recipients = [parseaddr(r)[1] for r in cc.split(",")]
            valid.expect('' not in cc_recipients,
                    "Invalid recipient.", field="patchset_cc")
            recipients += cc_recipients

        if not valid.ok:
            cover_letter = session.get("cover_letter")
            emails = prepare_patchset(repo, git_repo, cover_letter=cover_letter)

            default_branch = git_repo.default_branch()
            tip = git_repo.get(default_branch.target)
            readme = None
            if "README.md" in tip.tree:
                readme = "README.md"
            elif "README" in tip.tree:
                readme = "README"

            return render_template("send-email-review.html",
                    view="send-email", owner=owner, repo=repo,
                    readme=readme, emails=emails,
                    start=git_repo.get(start_commit),
                    end=git_repo.get(end_commit),
                    cover_letter=bool(cover_letter),
                    **valid.kwargs)

        cover_letter = session.pop("cover_letter", None)
        emails = prepare_patchset(repo, git_repo,
                cover_letter=cover_letter, extra_headers=True,
                to=to, cc=cc)
        if not emails:
            abort(400) # Should work by this point

        # git-format-patch doesn't encode messages, this is done by
        # git-send-email. Since we're parsing the message Python doesn't do it
        # automatically for us, it keeps the unencoded message as-is. Re-create
        # the message with the same header and body to fix that.
        # TODO: remove cte_type once [1] is merged
        # [1]: https://github.com/python/cpython/pull/8303
        policy = email.policy.SMTP.clone(cte_type="7bit")
        for i, msg in enumerate(emails):
            encoded = EmailMessage(policy=policy)
            for (k, v) in msg.items():
                encoded.add_header(k, v)
            encoded.set_content(msg.get_content())
            emails[i] = encoded

        # TODO: Send emails asyncronously
        smtp = start_smtp()
        print("Sending to recipients", recipients)
        for i, msg in enumerate(emails):
            session.pop("commentary_{i}", None)
            smtp.send_message(msg, smtp_from, recipients)
        smtp.quit()

        # TODO: If we're connected to a lists.sr.ht address, link to their URL
        # in the archives.
        session["message"] = "Your patchset has been sent."
        return redirect(url_for('repo.summary',
            owner=repo.owner, repo=repo.name))

@mail.app_template_filter('hash')
def to_hash(value):
    hashed_value = hashlib.sha256(value.encode() if isinstance(value, str) else value)
    return hashed_value.hexdigest()
