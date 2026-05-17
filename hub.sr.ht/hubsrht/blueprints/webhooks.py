import html
import json
import re
from datetime import datetime
from flask import Blueprint, request, current_app
from hubsrht.builds import submit_patchset
from hubsrht.services.hg import EventWebhook as HgEventWebhook
from hubsrht.services.hg import WebhookEvent as HgWebhookEvent
from hubsrht.services.todo import TodoClient, SubmitCommentInput
from hubsrht.services.todo import TicketStatus, TicketResolution
from hubsrht.services.todo import GraphQLClientGraphQLMultiError
from hubsrht.services.todo import EventWebhook as TodoEventWebhook
from hubsrht.services.todo import WebhookEvent as TodoWebhookEvent
from hubsrht.services.todo import EventType as TodoEventType
from hubsrht.services.git import EventWebhook as GitEventWebhook
from hubsrht.services.git import WebhookEvent as GitWebhookEvent
from hubsrht.services.lists import ListsClient, ToolIcon
from hubsrht.services.lists import EventWebhook as ListEventWebhook
from hubsrht.services.lists import WebhookEvent as ListWebhookEvent
from hubsrht.trailers import commit_trailers
from hubsrht.types import Event, EventType, EventProjectAssociation
from hubsrht.types import Tracker, MailingList, SourceRepo, RepoType
from hubsrht.types import User, Visibility
from srht.app import csrf_bypass
from srht.config import get_origin
from srht.crypto import fernet, verify_request_signature
from srht.database import db
from srht.graphql import InternalAuth, Error, has_error
from urllib.parse import quote

webhooks = Blueprint("webhooks", __name__)

_gitsrht = get_origin("git.sr.ht", external=True, default=None)
_hgsrht = get_origin("hg.sr.ht", external=True, default=None)
_todosrht = get_origin("todo.sr.ht", external=True, default=None)
_listssrht = get_origin("lists.sr.ht", external=True, default=None)

_ticket_url_re = re.compile(
    rf"""
    ^
    {re.escape(_todosrht)}
    /(?P<owner>~[a-z_][a-z0-9_-]+)
    /(?P<tracker>[\w.-]+)
    /(?P<ticket>\d+)
    $
    """,
    re.VERBOSE,
) if _todosrht else None

@csrf_bypass
@webhooks.route("/webhooks/gql/git-user/<int:user_id>", methods=["POST"])
def git_user(user_id):
    payload = verify_request_signature(request)
    payload = json.loads(payload.decode('utf-8'))["data"]
    webhook = GitEventWebhook.model_validate(payload).webhook
    repo = webhook.repository

    match webhook.event:
        case GitWebhookEvent.REPO_DELETED:
            source_repos = (SourceRepo.query
                .filter(SourceRepo.repo_type == RepoType.git)
                .filter(SourceRepo.remote_id == repo.id)
            )
            for source_repo in source_repos:
                if source_repo.project.summary_repo_id == repo.id:
                    source_repo.project_summary_repo = None
                    db.session.commit()
                db.session.delete(source_repo)
                db.session.commit()
            return f"Deleted repository with remote ID {webhook.repository.id}"
        case GitWebhookEvent.REPO_UPDATE:
            source_repos = (SourceRepo.query
                .filter(SourceRepo.repo_type == RepoType.git)
                .filter(SourceRepo.remote_id == repo.id)
            )
            for source_repo in source_repos:
                source_repo.name = repo.name
                source_repo.description = repo.description
                source_repo.visibility = Visibility(repo.visibility.value)
                source_repo.project.updated = datetime.utcnow()
                db.session.commit()
            return f"Updated repository with remote ID {webhook.repository.id}"

    return "No action required"

@csrf_bypass
@webhooks.route("/webhooks/gql/git-repo/<int:repo_id>", methods=["POST"])
def git_repo(repo_id):
    payload = verify_request_signature(request)
    payload = json.loads(payload.decode('utf-8'))["data"]
    webhook = GitEventWebhook.model_validate(payload).webhook
    repo = SourceRepo.query.get(repo_id)
    if not repo:
        return "No action required; unknown repository"
    if webhook.event != GitWebhookEvent.GIT_POST_RECEIVE:
        return "No action required; unknown event"

    repo_name = repo.owner.canonical_name + "/" + repo.name
    pusher_name = webhook.pusher.canonical_name
    pusher_url = f"{_gitsrht}/{pusher_name}"
    pusher = current_app.oauth_service.lookup_user(webhook.pusher.username)

    for update in webhook.updates:
        if not update.new:
            continue

        commit_sha = update.new.short_id
        commit_url = repo.url() + f"/commit/{commit_sha}"
        commit_message = update.new.message.split("\n")[0]

        event = Event()
        event.id = _dedupe_event("git.sr.ht", pusher, repo, commit_url)
        if event.id is None:
            # This is a brand new event.
            event.event_type = EventType.external_event
            event.source_repo_id = repo.id
            event.user_id = pusher.id

            event.external_source = "git.sr.ht"
            event.external_summary = (
                f"<a href='{commit_url}'>{commit_sha}</a> " +
                f"<code>{html.escape(commit_message)}</code>")
            event.external_summary_plain = f"{commit_sha} - {commit_message}"
            event.external_details = (
                f"<a href='{pusher_url}'>{pusher_name}</a> pushed to " +
                f"<a href='{repo.url()}'>{repo_name}</a> git")
            event.external_details_plain = f"{pusher_name} pushed to {repo_name} git"
            event.external_url = commit_url
            repo.project.updated = datetime.utcnow()
            db.session.add(event)
            db.session.flush()

            # That needs associating to the project.
            assoc = EventProjectAssociation()
            assoc.event_id = event.id
            assoc.project_id = repo.project_id
            db.session.add(assoc)

    db.session.commit()

    for upd in webhook.updates:
        if not upd.old or not upd.new:
            continue # New ref, or ref deleted

        if not upd.log or not upd.log.results or not any(upd.log.results):
            continue

        for commit in reversed(upd.log.results):
            for trailer, value in commit_trailers(commit.message):
                _handle_commit_trailer(trailer, value, pusher, repo, commit)

    return f"Processed push event for local repo ID {repo.id}"

@csrf_bypass
@webhooks.route("/webhooks/gql/hg-user/<int:user_id>", methods=["POST"])
def hg_user(user_id):
    payload = verify_request_signature(request)
    payload = json.loads(payload.decode('utf-8'))["data"]
    webhook = HgEventWebhook.model_validate(payload).webhook
    repo = webhook.repository

    match webhook.event:
        case HgWebhookEvent.REPO_DELETED:
            source_repos = (SourceRepo.query
                .filter(SourceRepo.repo_type == RepoType.hg)
                .filter(SourceRepo.remote_id == repo.id)
            )
            for source_repo in source_repos:
                if source_repo.project.summary_repo_id == repo.id:
                    source_repo.project_summary_repo = None
                    db.session.commit()
                db.session.delete(source_repo)
                db.session.commit()
            return f"Deleted repository with remote ID {webhook.repository.id}"
        case HgWebhookEvent.REPO_UPDATE:
            source_repos = (SourceRepo.query
                .filter(SourceRepo.repo_type == RepoType.hg)
                .filter(SourceRepo.remote_id == repo.id)
            )
            for source_repo in source_repos:
                source_repo.name = repo.name
                source_repo.description = repo.description
                source_repo.visibility = Visibility(repo.visibility.value)
                source_repo.project.updated = datetime.utcnow()
                db.session.commit()
            return f"Updated repository with remote ID {webhook.repository.id}"

    return "No action required"

@csrf_bypass
@webhooks.route("/webhooks/gql/mailing-list-user/<int:user_id>", methods=["POST"])
def mailing_list_user(user_id):
    payload = verify_request_signature(request)
    payload = json.loads(payload.decode('utf-8'))["data"]
    webhook = ListEventWebhook.model_validate(payload).webhook
    mlist = webhook.mailing_list

    match webhook.event:
        case ListWebhookEvent.LIST_DELETED:
            mailing_lists = (MailingList.query
                .filter(MailingList.remote_id == mlist.id))
            for mailing_list in mailing_lists:
                db.session.delete(mailing_list)
                db.session.commit()
            return f"Deleted mailing list with remote ID {mlist.id}"
        case ListWebhookEvent.LIST_UPDATED:
            mailing_lists = (MailingList.query
                .filter(MailingList.remote_id == webhook.mailing_list.id))
            for mailing_list in mailing_lists:
                mailing_list.name = mlist.name
                mailing_list.description = mlist.description
                mailing_list.visibility = Visibility(mlist.visibility.value)
                db.session.commit()
            return f"Updated mailing list with remote ID {mlist.id}"

    return "No action required"

@csrf_bypass
@webhooks.route("/webhooks/gql/mailing-list/<int:list_id>", methods=["POST"])
def project_mailing_list(list_id):
    event = request.headers.get("X-Webhook-Event")
    payload = verify_request_signature(request)
    payload = json.loads(payload.decode('utf-8'))["data"]
    webhook = ListEventWebhook.model_validate(payload).webhook

    mailing_list = (MailingList.query
             .filter(MailingList.id == list_id)).one_or_none()
    if not mailing_list:
        return "I don't recognize that mailing list.", 404

    match webhook.event:
        case ListWebhookEvent.EMAIL_RECEIVED:
            email = webhook.email
            sender_canon = email.sender.canonical_name
            if hasattr(email.sender, "username"):
                sender_username = email.sender.username
            else:
                sender_username = None

            subject = email.subject
            message_id = f"<{email.message_id}>"
            archive_url = f"{mailing_list.url()}/{quote(message_id)}"

            event = Event()
            if sender_username:
                sender = current_app.oauth_service.lookup_user(sender_username)
                event.user_id = sender.id
                attrib = f"<a href='{_listssrht}/{sender_canon}'>{sender_canon}</a>"
            else:
                attrib = sender_canon
                sender = None

            event.id = _dedupe_event("lists.sr.ht", sender, mailing_list, archive_url)
            if event.id is None:
                # This is a brand new event.
                event.event_type = EventType.external_event
                event.mailing_list_id = mailing_list.id
                event.external_source = "lists.sr.ht"
                event.external_summary = f"<a href='{archive_url}'>{html.escape(subject)}</a>"
                event.external_details = (f"{attrib} via " +
                        f"<a href='{mailing_list.url()}'>{mailing_list.name}</a>")
                event.external_url = archive_url
                db.session.add(event)
                db.session.flush()

                # That needs associating to the project.
                assoc = EventProjectAssociation()
                assoc.event_id = event.id
                assoc.project_id = mailing_list.project_id
                db.session.add(assoc)

            db.session.commit()
            return f"Assigned event ID {event.id}"
        case ListWebhookEvent.PATCHSET_RECEIVED:
            patchset = webhook.patchset

            sender = None
            if hasattr(patchset.submitter, "username"):
                sender = current_app.oauth_service.lookup_user(patchset.submitter.username)

            for email in patchset.patches.results:
                if email.patch.trailers:
                    _handle_patch_trailers(sender, mailing_list, email)

            job_ids = []
            ids = submit_patchset(mailing_list, patchset)
            if ids is not None:
                job_ids.extend(ids)
            try:
                pass
            except Exception as ex:
                return "Error submitting builds: " + str(ex)

            return "Submitted builds: " + ", ".join([str(x) for x in job_ids])

    return "No action required"

@csrf_bypass
@webhooks.route("/webhooks/gql/todo-user/<int:user_id>", methods=["POST"])
def todo_user(user_id):
    payload = verify_request_signature(request)
    payload = json.loads(payload.decode('utf-8'))["data"]
    webhook = TodoEventWebhook.model_validate(payload).webhook
    tracker = webhook.tracker

    match webhook.event:
        case TodoWebhookEvent.TRACKER_DELETED:
            trackers = Tracker.query.filter(Tracker.remote_id == tracker.id)
            for tr in trackers:
                tr.project.updated = datetime.utcnow()
                db.session.delete(tr)
            db.session.commit()
            return f"Deleted local trackers corresponding to remote ID {tracker.id}"
        case TodoWebhookEvent.TRACKER_UPDATE:
            trackers = Tracker.query.filter(Tracker.remote_id == tracker.id)
            for tr in trackers:
                tr.name = tracker.name
                tr.description = tracker.description
                tr.visibility = Visibility(tracker.visibility.value)
                tr.project.updated = datetime.utcnow()
            db.session.commit()
            return f"Updated local trackers corresponding to remote ID {tracker.id}"

    return "No action required"

@csrf_bypass
@webhooks.route("/webhooks/gql/todo-tracker/<int:tracker_id>", methods=["POST"])
def todo_tracker(tracker_id):
    payload = verify_request_signature(request)
    payload = json.loads(payload.decode('utf-8'))["data"]
    webhook = TodoEventWebhook.model_validate(payload).webhook

    tracker = Tracker.query.get(tracker_id)
    if not tracker:
        return "Unknown tracker", 404

    event = Event()
    event.event_type = EventType.external_event
    event.tracker_id = tracker.id

    match webhook.event:
        case TodoWebhookEvent.TICKET_CREATED:
            submitter = webhook.ticket.submitter
        case TodoWebhookEvent.EVENT_CREATED:
            comments = [
                ch for ch in webhook.new_event.changes
                if ch.event_type == TodoEventType.COMMENT
            ]
            if not any(comments):
                return "No action required"
            assert len(comments) == 1
            comment = comments[0]
            submitter = comment.author

    match submitter.typename__:
        case "User":
            event.user_id = (current_app.oauth_service
                .lookup_user(submitter.username).id)
            canonical_name = submitter.canonical_name
            submitter_url = f"{_todosrht}/{canonical_name}"
            submitter_url = f"<a href='{submitter_url}'>{canonical_name}</a>"
        case "EmailAddress":
            mailbox = html.escape(submitter.mailbox)
            if submitter.name:
                name = html.escape(submitter.name)
                submitter_url = f"<a href='mailto:{mailbox}'>{name}</a>"
            else:
                submitter_url = f"<a href='mailto:{mailbox}'>{mailbox}</a>"
        case "ExternalUser":
            external_id = html.escape(submitter.external_id)
            if submitter.external_url:
                external_url = html.escape(submitter.external_url)
                submitter_url = f"<a href='{external_url}' rel='nofollow noopener'>{external_id}</a>"
            else:
                submitter_url = f"{external_id}"

    match webhook.event:
        case TodoWebhookEvent.TICKET_CREATED:
            ticket = webhook.ticket
            ticket_url = tracker.url() + f"/{ticket.id}"
            event.external_source = "todo.sr.ht"
            event.external_summary = (
                f"<a href='{ticket_url}'>#{ticket.id}</a> " +
                f"{html.escape(ticket.subject)}")
            event.external_summary_plain = f"#{ticket.id} {ticket.subject}"
            event.external_details = (
                f"{submitter_url} filed ticket on " +
                f"<a href='{tracker.url()}'>{tracker.name}</a> todo")
            event.external_details_plain = f"{submitter.canonical_name} filed ticket on {tracker.name} todo"
            event.external_url = ticket_url

            db.session.add(event)
            db.session.flush()

            assoc = EventProjectAssociation()
            assoc.event_id = event.id
            assoc.project_id = tracker.project_id
            db.session.add(assoc)
            db.session.commit()

            return "Processed new ticket"
        case TodoWebhookEvent.EVENT_CREATED:
            ticket = webhook.new_event.ticket
            ticket_url = tracker.url() + f"/{ticket.id}"

            event.external_source = "todo.sr.ht"
            event.external_summary = (
                f"<a href='{ticket_url}'>#{ticket.id}</a> " +
                f"{html.escape(ticket.subject)}")
            event.external_summary_plain = f"#{ticket.id} {ticket.subject}"
            event.external_details = (
                f"{submitter_url} commented on " +
                f"<a href='{tracker.url()}'>{tracker.name}</a> todo")

            event.external_details_plain = f"{submitter.canonical_name} commented on {tracker.name} todo"
            event.external_url = ticket_url

            db.session.add(event)
            db.session.flush()

            assoc = EventProjectAssociation()
            assoc.event_id = event.id
            assoc.project_id = tracker.project_id
            db.session.add(assoc)
            db.session.commit()
            return "Processed new comment"

    return "No action required"

@csrf_bypass
@webhooks.route("/webhooks/build-complete/<details>", methods=["POST"])
def build_complete(details):
    payload = verify_request_signature(request)
    payload = json.loads(payload.decode('utf-8'))
    details = fernet.decrypt(details.encode())
    if not details:
        return "Bad payload", 400
    details = json.loads(details.decode())
    ml = (MailingList.query
            .filter(MailingList.id == details["mailing_list"])).one_or_none()
    if not ml:
        return "Unknown mailing list", 404
    project = ml.project
    if payload["owner"]["canonical_name"] != details["user"]:
        return "Discarding webhook from unauthorized build", 401

    builds_origin = get_origin("builds.sr.ht", external=True)
    build_url = f"{builds_origin}/{project.owner.canonical_name}/job/{payload['id']}"

    lists_client = ListsClient(InternalAuth(project.owner))
    # TODO: Update me once builds.sr.ht adds a native enum for this
    match payload["status"]:
        case 'pending':
            icon = ToolIcon.PENDING
        case 'queued' | 'running':
            icon = ToolIcon.WAITING
        case 'success':
            icon = ToolIcon.SUCCESS
        case 'failed' | 'timeout':
            icon = ToolIcon.FAILED
        case 'cancelled':
            icon = ToolIcon.CANCELLED

    status_details = f"[#{payload['id']}]({build_url}) {details['name']} {payload['status']}"
    lists_client.update_tool(tool_id=details["tool_id"],
            icon=icon, details=status_details)

    return "Thanks!"

# If we already have an event from source and sender on resource for the same
# event_key (this can happen for lists/repositories/trackers shared by several
# projects), add a new mapping in the event/project association table and
# return the ID of the event deduped into; otherwise return None.
def _dedupe_event(source, sender, resource, event_key):
    q = (Event.query
        .filter(Event.event_type == EventType.external_event)
        .filter(Event.external_source == source)
        .filter(Event.external_url == event_key))
    if sender:
        q = q.filter(Event.user_id == sender.id)

    existing_evt = q.one_or_none()

    if existing_evt:
        assoc = EventProjectAssociation()
        assoc.event_id = existing_evt.id
        assoc.project_id = resource.project_id
        db.session.add(assoc)
        return existing_evt.id

    return None

def _handle_commit_trailer(trailer, value, pusher, repo, commit):
    if not _todosrht:
        return

    if trailer == "Closes":
        resolution = TicketResolution.CLOSED
    elif trailer == "Fixes":
        resolution = TicketResolution.FIXED
    elif trailer == "Implements":
        resolution = TicketResolution.IMPLEMENTED
    elif trailer == "References":
        resolution = None
    else:
        return

    match = _ticket_url_re.match(value.strip())
    if not match:
        return

    commit_message = html.escape(commit.message.split("\n")[0])
    commit_author = html.escape(commit.author.name.strip())
    commit_sha = commit.id[:7]
    commit_url = repo.url() + f"/commit/{commit_sha}"
    comment = f"""\
*{commit_author} referenced this ticket in commit [{commit_sha}] on [{repo.name}]({repo.url()}).*

[{commit_sha}]: {commit_url} "{commit_message}"\
"""

    auth = InternalAuth(pusher)
    todo_client = TodoClient(auth)

    tracker = todo_client.get_ticket_comments(
            username=match["owner"][1:],
            tracker=match["tracker"],
            ticket_id=int(match["ticket"])
    ).user.tracker
    ticket = tracker.ticket
    if not ticket or _ticket_has_comment(ticket, comment):
        return

    try:
        comment_input = SubmitCommentInput(text=comment)
        if resolution is not None:
            comment_input.status = TicketStatus.RESOLVED
            comment_input.resolution = resolution

        todo_client.submit_comment(
                tracker_id=tracker.id,
                ticket_id=ticket.id,
                comment=comment_input)
    except GraphQLClientGraphQLMultiError as err:
        if not has_error(err, Error.ACCESS_DENIED):
            raise

        # Try again without resolving the ticket, in case this user has comment
        # access but not triage access
        try:
            todo_client.submit_comment(
                    tracker_id=tracker.id,
                    ticket_id=ticket.id,
                    comment=SubmitCommentInput(text=comment),
            )
        except GraphQLClientGraphQLMultiError as err:
            if not has_error(Error.ACCESS_DENIED):
                # Silently discard further access denied errors
                raise

def _handle_patch_trailers(sender, mailing_list, email):
    if not _todosrht:
        return

    subject = email.subject
    message_id = f"<{email.message_id}>"
    archive_url = f"{mailing_list.url()}/patches/{email.patchset.id}#{quote(message_id)}"

    match email.sender.typename__:
        case "User":
            sender_name = email.sender.canonical_name
        case "Mailbox":
            sender_name = email.sender.name

    for trailer in email.patch.trailers:
        match trailer.name:
            case "References" | "Implements" | "Fixes" | "Closes":
                match = _ticket_url_re.match(trailer.value.strip())
                if not match:
                    return

                auth = InternalAuth(sender or mailing_list.owner)
                todo_client = TodoClient(auth)

                comment = f"""\
*{sender_name} referenced this ticket in a patch:*

[{subject}]({archive_url})\
"""

                tracker = todo_client.get_ticket_comments(
                        username=match["owner"][1:],
                        tracker=match["tracker"],
                        ticket_id=int(match["ticket"])
                ).user.tracker
                ticket = tracker.ticket
                if _ticket_has_comment(ticket, comment):
                    continue

                todo_client.submit_comment(
                        tracker_id=tracker.id,
                        ticket_id=ticket.id,
                        comment=SubmitCommentInput(text=comment),
                )

def _ticket_has_comment(ticket, comment):
    for event in ticket.events.results:
        for change in event.changes:
            if not hasattr(change, "text"):
                continue
            if change.text == comment:
                return True
    return False
