import hashlib
import json
import re
from datetime import timedelta
from functools import wraps
from markupsafe import Markup, escape
from srht.app import icon, issue_csrf_token, date_handler
from srht.markdown import markdown, SRHT_MARKDOWN_VERSION 
from srht.cache import get_cache, set_cache
from todosrht import urls
from todosrht.tickets import find_mentioned_users, find_mentioned_tickets
from todosrht.tickets import TICKET_MENTION_PATTERN, USER_MENTION_PATTERN
from prometheus_client import Counter

metrics = type("metrics", tuple(), {
    c.describe()[0].name: c
    for c in [
        Counter("todosrht_markup_cache_access", "Number of markup cache accesses"),
        Counter("todosrht_markup_cache_miss", "Number of markup cache misses"),
    ]
})

def cache_rendered_markup_if(func):
    @wraps(func)
    def wrap(obj):
        class_name = obj.__class__.__name__
        sha = hashlib.sha1()
        sha.update(json.dumps(obj.to_dict(), default=date_handler).encode())
        key = f"todo.sr.ht:cache_rendered_markup_if:{class_name}:{sha.hexdigest()}:v{SRHT_MARKDOWN_VERSION}"
        value = get_cache(key)
        metrics.todosrht_markup_cache_access.inc()
        if value:
            return Markup(value.decode())

        metrics.todosrht_markup_cache_miss.inc()
        value, cacheable = func(obj)
        if cacheable:
            set_cache(key, timedelta(days=30), value)
        return value
    return wrap

def render_markup(tracker, text):
    markup, cacheable = render_markup_get_cacheable(tracker, text)
    return markup

# Returns a tuple of the markup and a boolean that indicates whether or not
# the markup should be cached.
def render_markup_get_cacheable(tracker, text):
    users = find_mentioned_users(text)
    tickets = find_mentioned_tickets(tracker, text)

    users_map = {u.identifier: u for u in users}
    tickets_map = {t.ref(): t for t in tickets}

    def urlize_user(match):
        # TODO: Handle mentions for non-user participants
        username = match.group(0)
        if username in users_map:
            url = urls.participant_url(users_map[username])
            return f'<a href="{url}">{escape(username)}</a>'

        return username

    def urlize_ticket(match):
        text = match.group(0)
        ticket_id = match.group('ticket_id')
        tracker_name = match.group('tracker_name') or tracker.name
        owner = match.group('username') or tracker.owner.username

        ticket_ref = f"~{owner}/{tracker_name}#{ticket_id}"
        if ticket_ref not in tickets_map:
            return text

        ticket = tickets_map[ticket_ref]
        url = urls.ticket_url(ticket)
        title = escape(f"{ticket.ref()}: {ticket.title}")
        return f'<a href="{url}" title="{title}">{text}</a>'

    # Replace ticket and username mentions with linked version
    text = re.sub(USER_MENTION_PATTERN, urlize_user, text)
    text, ticket_mentions = re.subn(TICKET_MENTION_PATTERN, urlize_ticket, text)

    # Ticket references should not be cached, as they depend on whether a user
    # has access to the ticket.
    return markdown(text), ticket_mentions == 0

@cache_rendered_markup_if
def render_comment(comment):
    return render_markup_get_cacheable(comment.ticket.tracker, comment.text)

@cache_rendered_markup_if
def render_ticket_description(ticket):
    return render_markup_get_cacheable(ticket.tracker, ticket.description)

def label_badge(label, cls="", remove_from_ticket=None, terms=None):
    """Return HTML markup rendering a label badge.

    Additional HTML classes can be passed via the `cls` parameter.

    If a Ticket is passed in `remove_from_ticket`, a removal button will also
    be rendered for removing the label from given ticket.
    """
    name = escape(label.name)
    color = escape(label.text_color)
    bg_color = escape(label.color)
    html_class = escape(f"label {cls}".strip())

    style = f"color: {color}; background-color: {bg_color}"
    if terms:
        search_url = urls.label_search_url(label, terms=terms)
    else:
        search_url = urls.label_search_url(label)

    if remove_from_ticket:
        remove_url = urls.label_remove_url(label, remove_from_ticket)
        remove_form = f"""
            <form method="POST" action="{remove_url}">
              {issue_csrf_token()}
              <button type="submit" class="btn btn-link" aria-label="Remove">
                {icon('times')}
              </button>
            </form>
        """
    else:
        remove_form = ""

    return Markup(
        f"""<span style="{style}" class="{html_class}" href="{search_url}">
            <a rel="nofollow" href="{search_url}">{name}</a>
            {remove_form}
        </span>"""
    )
