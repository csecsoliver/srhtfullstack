import os.path
import pygments
from datetime import timedelta
from jinja2 import Template
from markupsafe import Markup
from pygments import highlight
from pygments.lexers import guess_lexer, guess_lexer_for_filename, TextLexer
from pygments.formatters import HtmlFormatter
from srht.markdown import SRHT_MARKDOWN_VERSION, markdown
from srht.cache import get_cache, set_cache
from prometheus_client import Counter

metrics = type("metrics", tuple(), {
    c.describe()[0].name: c
    for c in [
        Counter("scmsrht_readme_cache_access", "Number of readme cache accesses"),
        Counter("scmsrht_readme_cache_miss", "Number of readme cache misses"),
        Counter("scmsrht_highlight_cache_access", "Number of highlight cache accesses"),
        Counter("scmsrht_highlight_cache_miss", "Number of highlight cache misses"),
    ]
})

def get_formatted_readme(cache_prefix, file_finder, content_getter,
        link_prefix=None):
    readme_names = ['README.md', 'README.markdown', 'README']
    for name in readme_names:
        content_hash, user_obj = file_finder(name)
        if content_hash:
            return format_readme(cache_prefix, content_hash, name,
                        content_getter, user_obj, link_prefix=link_prefix)
    return None

def format_readme(cache_prefix, content_hash, name, content_getter, user_obj,
        link_prefix=None):
    """Formats a `README` file for display on a repository's summary page."""
    key = f"{cache_prefix}:readme:{content_hash}:{link_prefix}:v{SRHT_MARKDOWN_VERSION}:v10"
    html = get_cache(key)
    metrics.scmsrht_readme_cache_access.inc()
    if html:
        return Markup(html.decode())

    metrics.scmsrht_readme_cache_miss.inc()
    try:
        raw = content_getter(user_obj)
    except:
        raw = "Error decoding readme - is it valid UTF-8?"

    basename, ext = os.path.splitext(name)
    if ext in ['.md', '.markdown']:
        html = markdown(raw,
                link_prefix=link_prefix)
    else:
        # Unsupported/unknown markup type.
        html = Template(
                 "<pre>{{ readme }}</pre>",
                 autoescape=True
               ).render(readme=raw)

    set_cache(key, timedelta(days=7), html)
    return Markup(html)

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

def get_highlighted_file(cache_prefix, name, content_hash,
        content, formatter=None):
    """Highlights a file for display in a repository's browsing UI."""
    # We incorporate SRHT_MARKDOWN_VERSION in this cache key because it can be
    # used for git.sr.ht annotations
    key = f"{cache_prefix}:highlight:{content_hash}:v{SRHT_MARKDOWN_VERSION}:v6"
    html = get_cache(key)
    metrics.scmsrht_highlight_cache_access.inc()
    if html:
        return Markup(html.decode())

    metrics.scmsrht_highlight_cache_miss.inc()
    lexer = _get_lexer(name, content)
    if formatter is None:
        formatter = HtmlFormatter()
    html = highlight(content, lexer, formatter)
    set_cache(key, timedelta(days=7), html)
    return Markup(html)
