from flask import abort
from flask import request
from flask import redirect
from flask import url_for
from srht.oauth import current_user, UserType
from todosrht.graphql import Visibility
from todosrht.types import TicketAccess, UserAccess, Participant
from todosrht.types import User, Tracker, Ticket, Redirect

# TODO: get_access for any participant
def get_access(tracker, ticket, user=None):
    user = user or current_user

    # Anonymous
    if not user:
        if tracker.visibility == Visibility.PRIVATE:
            return TicketAccess.NONE
        return tracker.default_access

    # Owner
    if user.id == tracker.owner_id:
        return TicketAccess.ALL

    # ACL entry?
    user_access = UserAccess.query.filter_by(tracker=tracker, user=user).first()
    if user_access:
        return user_access.permissions

    if tracker.visibility == Visibility.PRIVATE:
        return TicketAccess.NONE
    return tracker.default_access


def get_tracker(owner, name, with_for_update=False, user=None):
    """Returns a Tracker object, along with its access."""
    if not owner:
        return None, None

    if not isinstance(owner, User):
        if owner[0] == "~":
            owner = owner[1:]
            if not isinstance(owner, User): # FIXME: can never be false
                owner = (User.query
                         .filter(User.username == owner)
                         .filter(User.user_type != UserType.suspended)).one_or_none()
                if not owner:
                    return None, None
        else:
            # TODO: org trackers
            return None, None
    tracker = (Tracker.query
        .filter(Tracker.owner_id == owner.id)
        .filter(Tracker.name == name))
    if with_for_update:
        tracker = tracker.with_for_update()
    tracker = tracker.one_or_none()
    if not tracker:
        return None, None
    access = get_access(tracker, None, user=user)
    if access == TicketAccess.NONE and tracker.visibility == Visibility.PRIVATE:
        abort(401)
    return tracker, access


def get_tracker_or_redir(owner: str, name: str):
    """Get tracker and its access, or implicitly redirect if necessary."""

    if isinstance(owner, str):
        if owner[0] == "~":
            owner = (User.query
                     .filter(User.username == owner[1:])
                     .filter(User.user_type != UserType.suspended)).one_or_none()
            if not owner:
                return None, None
        else:  # TODO: org trackers
            return None, None

    tracker, access = get_tracker(owner, name)
    if not tracker:
        redir = (Redirect.query
            .filter(Redirect.owner == owner)
            .filter(Redirect.name == name)
        ).first()
        if isinstance(redir, Redirect):
            view_args = request.view_args
            view_args["owner"] = redir.new_tracker.owner.canonical_name
            view_args["name"] = redir.new_tracker.name
            abort(redirect(url_for(request.endpoint, **view_args)))
        abort(404)
    return tracker, access

def get_ticket(tracker, ticket_id, user=None):
    user = user or current_user
    ticket = (Ticket.query
            .join(Participant)
            .filter(Ticket.scoped_id == ticket_id)
            .filter(Ticket.tracker_id == tracker.id)).one_or_none()
    if not ticket:
        return None, None
    access = get_access(tracker, ticket, user=user)
    if user and user.id == ticket.submitter.user_id:
        access |= TicketAccess.BROWSE
    if not TicketAccess.BROWSE in access:
        return None, None
    return ticket, access
