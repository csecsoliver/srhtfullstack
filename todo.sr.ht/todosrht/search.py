from sqlalchemy import or_
from srht import search
from todosrht.graphql import TicketStatus
from todosrht.types import Label, Ticket, TicketComment, Participant, User

STATUS_ALIASES = {
    "open": [
        TicketStatus.REPORTED,
        TicketStatus.CONFIRMED,
        TicketStatus.IN_PROGRESS,
        TicketStatus.PENDING,
    ],
    "closed": [TicketStatus.RESOLVED]
}

def status_filter(value):
    value = value.lower()

    if value == "any":
        return True

    if value in STATUS_ALIASES:
        return Ticket.status.in_(STATUS_ALIASES[value])

    status = getattr(TicketStatus, value, None)
    if status is None:
        raise ValueError(f"Invalid status: '{value}'")

    return Ticket.status == status

def submitter_filter(value, current_user):
    if value == "me" and current_user:
        return Ticket.submitter.has(Participant.user == current_user)
    else:
        return Ticket.submitter.has(
            Participant.user.has(User.username.ilike(value.lstrip("~")))
        )

def assignee_filter(value, current_user):
    if value == "me" and current_user:
        return Ticket.assigned_users.contains(current_user)
    else:
        return Ticket.assigned_users.any(
            User.username.ilike(value.lstrip("~"))
        )

def label_filter(value):
    return Ticket.labels.any(Label.name == value)

def no_filter(value):
    if value == "assignee":
        return Ticket.assigned_users == None

    if value == "label":
        return Ticket.labels == None

    raise ValueError(f"Invalid search term: 'no:{value}'")

def default_filter(value):
    return or_(
        Ticket.description.ilike(f"%{value}%"),
        Ticket.title.ilike(f"%{value}%"),
        Ticket.comments.any(TicketComment.text.ilike(f"%{value}%"))
    )

def apply_sort(query, terms, column_map):
    for term in terms:
        column_name = term.value

        if column_name.startswith("-"):
            column_name = column_name[1:]

        if column_name not in column_map:
            valid = ", ".join(f"'{c}'" for c in column_map.keys())
            raise ValueError(
                f"Invalid {term.key} value: '{column_name}'. "
                f"Supported values are: {valid}."
            )

        column = column_map[column_name]
        reverse = term.key == "rsort"
        ordering = column.asc() if reverse else column.desc()
        query = query.order_by(ordering)

    return query

def apply_search(query, search_string, current_user):
    terms = list(search.parse_terms(search_string))
    sort_terms = [t for t in terms if t.key in ["sort", "rsort"]]
    search_terms = [t for t in terms if t.key not in ["sort", "rsort"]]

    # If search does not include a status filter, show open tickets
    if not any([term.key == "status" for term in search_terms]):
        search_terms.append(search.Term("status", "open", False))

    # Set default sort to 'updated desc' if not specified
    if not sort_terms:
        sort_terms = [search.Term("sort", "updated", True)]

    query = search.apply_terms(query, search_terms, default_filter, key_fns={
        "status": status_filter,
        "submitter": lambda v: submitter_filter(v, current_user),
        "assigned": lambda v: assignee_filter(v, current_user),
        "assignee": lambda v: assignee_filter(v, current_user),
        "label": label_filter,
        "no": no_filter,
    })

    return apply_sort(query, sort_terms, {
        "created": Ticket.created,
        "updated": Ticket.updated,
        "comments": Ticket.comment_count,
    })

def find_usernames(query, limit=20):
    """Given a partial username string, returns matching usernames."""
    if not query or query == '~':
        return []

    if query.startswith("~"):
        where = User.username.startswith(query[1:], autoescape=True)
    else:
        where = User.username.contains(query, autoescape=True)

    from todosrht.app import db
    rows = (db.session
        .query(User.username)
        .filter(where)
        .order_by(User.username)
        .limit(limit))

    return [f"~{r[0]}" for r in rows]
