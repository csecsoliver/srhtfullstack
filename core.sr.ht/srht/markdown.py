from bs4 import BeautifulSoup
from collections import namedtuple
from markupsafe import Markup, escape
from urllib.parse import urljoin
from pygments import highlight
from pygments.formatters import HtmlFormatter, ClassNotFound
from pygments.lexers import get_lexer_by_name
from urllib.parse import urlparse, urlunparse
import bleach
import html
import mistletoe as m
from mistletoe.span_token import SpanToken, RawText
import re

SRHT_MARKDOWN_VERSION = 17

class PlainLink(SpanToken):
    """
    Plain link and mail tokens. ("http://www.google.com" and "test@example.org")

    Attributes:
        children (iterator): a single RawText node for alternative text.
        target (str): link target.
    """
    pattern = re.compile(r"(?<!\\)(?:\\\\)*" # Fail if prefixed by odd number of backslashes
            r"((?P<url>[A-Za-z][A-Za-z0-9+.-]{1,31}://[^ \t\n\r\f\v<>]*)" # URLs: 'scheme'://'path'
            r"|(?P<mail>@?[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Za-z0-9]" # Emails: 'user'@'domain
                r"(?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?"
                r"(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+))")
    parse_inner = False
    precedence = 3

    def __init__(self, match):
        content = match.group(1)
        self.children = (RawText(content),)
        self.target = content
        self.mailto = match.group("mail") is not None

class SrhtRenderer(m.HTMLRenderer):
    def __init__(self, link_prefix=None, baselevel=1):
        super().__init__(PlainLink)
        self.baselevel = baselevel
        if isinstance(link_prefix, (tuple, list)):
            # If passing a 2 item list/tuple than assume the second
            # item is to be used to fetch raw_blob url's (ie, images)
            try:
                self.link_prefix, self.blob_prefix = link_prefix
            except ValueError:
                self.link_prefix = link_prefix[0]
                self.blob_prefix = link_prefix[0]
        else:
            self.link_prefix = link_prefix
            self.blob_prefix = link_prefix

    def _relative_url(self, url, use_blob=False):
        try:
            p = urlparse(url)
        except ValueError:
            return url
        link_prefix = self.link_prefix if not use_blob else self.blob_prefix
        if not link_prefix:
            return url
        if not link_prefix.endswith("/"):
            link_prefix += "/"
        if not p.netloc and not p.scheme and link_prefix:
            path = urljoin(link_prefix, p.path)
            url = urlunparse(('', '', path, p.params, p.query, p.fragment))
        return url

    def render_link(self, token):
        template = '<a href="{target}"{title}>{inner}</a>'
        url = token.target
        if token.title:
            title = ' title="{}"'.format(self.escape_html_text(token.title))
        else:
            title = ''
        if not url.startswith("#"):
            url = self._relative_url(url)
        target = self.escape_url(url)

        for i in range(len(token.children)):
            if isinstance(token.children[i], PlainLink):
                token.children[i] = RawText(token.children[i].target)
        inner = self.render_inner(token)
        return template.format(target=target, title=title, inner=inner)

    def render_plain_link(self, token):
        template = '<a href="{target}">{inner}</a>'
        if token.mailto:
            if token.target.startswith('@'): # mastodon "@user@domain" as plain text
                return token.target
            target = 'mailto:{}'.format(token.target)
        else:
            target = self.escape_url(token.target)
        inner = self.render_inner(token)
        return template.format(target=target, inner=inner)

    def render_image(self, token):
        template = '<img src="{}" alt="{}"{} />'
        url = self._relative_url(token.src, use_blob=True)
        if token.title:
            title = ' title="{}"'.format(self.escape_html_text(token.title))
        else:
            title = ''
        alt = self.render_to_plain(token)
        return template.format(url, alt, title)

    def render_block_code(self, token):
        template = '<pre><code{attr}>{inner}</code></pre>'
        if token.language:
            try:
                lexer = get_lexer_by_name(token.language, stripall=True)
            except ClassNotFound:
                lexer = None
            if lexer:
                formatter = HtmlFormatter()
                return highlight(token.children[0].content, lexer, formatter)
            else:
                attr = ' class="{}"'.format('language-{}'.format(self.escape_html_text(token.language)))
        else:
            attr = ''
        inner = html.escape(token.children[0].content)
        return template.format(attr=attr, inner=inner)

    def render_heading(self, token):
        template = '<h{level} id="{_id}"><a href="#{_id}" aria-hidden="true">#</a>{inner}</h{level}>'
        level = token.level + self.baselevel
        if level > 6:
            level = 6
        inner = self.render_inner(token)
        _id = re.sub(r'[^a-z0-9-_]', '', inner.lower().replace(" ", "-"))
        return template.format(level=level, inner=inner, _id=_id)

    def escape_html_text(self, value):
        # This wrapper handles a name change introduced in mistletoe v1.1.0.
        # It can simply be removed once backwards compatibility is no longer needed.
        sup = super()
        if hasattr(sup, 'escape_html_text'):
            return sup.escape_html_text(value)
        return sup.escape_html(value)

def _img_filter(tag, name, value):
    if name in ["alt", "height", "width"]:
        return True
    if name == "src":
        p = urlparse(value)
        return p.scheme in ["http", "https", ""]
    return False

def _input_filter(tag, name, value):
    if name in ["checked", "disabled"]:
        return True
    return name == "type" and value in ["checkbox"]

def _div_filter(tag, name, value):
    if name == "class":
        # For code highlighting
        return value in ["highlight"]
    return name in ["style"]

def _span_filter(tag, name, value):
    if name == "class":
        # For code highlighting
        return value in [
            "bp", "c", "c1", "ch", "cm", "cp", "cpf", "cs", "dl", "err", "fm",
            "gd", "ge", "gh", "gi", "go", "gp", "gr", "gs", "gt", "gu", "hll",
            "il", "k", "kc", "kd", "kn", "kp", "kr", "kt", "l", "ld", "m",
            "mb", "mf", "mh", "mi", "mo", "n", "na", "nb", "nc", "nd", "ne",
            "nf", "ni", "nl", "nn", "no", "nt", "nv", "nx", "o", "ow", "p",
            "py", "s", "s1", "s2", "sa", "sb", "sc", "sd", "se", "sh", "si",
            "sr", "ss", "sx", "vc", "vg", "vi", "vm", "w"
        ]
    return name in ["style"]

def _wildcard_filter(tag, name, value):
    return name in [
        "aria-braillelabel", "aria-brailleroledescription", "aria-describedby",
        "aria-description", "aria-label", "aria-labelledby", "colspan", "id",
        "lang", "role", "rowspan",
    ]

_sanitizer_attrs = {
    "a": ["href", "title"],
    "abbr": ["title"],
    "bdo": ["dir"],
    "blockquote": ["cite"],
    "q": ["cite"],
    "time": ["datetime"],
    "th": ["align"],
    "td": ["align"],
    "img": _img_filter,
    "input": _input_filter,
    "div": _div_filter,
    "section": _div_filter,
    "aside": _div_filter,
    "span": _span_filter,
    "*": _wildcard_filter,
}
_sanitizer_styles = [
        "margin", "padding",
        "text-align", "font-weight",
        "text-decoration"
    ]
_sanitizer_styles += [f"padding-{p}" for p in ["left", "right", "bottom", "top"]]
_sanitizer_styles += [f"margin-{p}" for p in ["left", "right", "bottom", "top"]]

_sanitizer_css = {}
try:
    # bleach >= 5.0.0
    from bleach.css_sanitizer import CSSSanitizer
    _sanitizer_css["css_sanitizer"] = CSSSanitizer(allowed_svg_properties=[], allowed_css_properties=_sanitizer_styles)
except ImportError:
    # bleach < 5.0.0
    _sanitizer_css["styles"] = bleach.sanitizer.ALLOWED_STYLES + _sanitizer_styles

_sanitizer = bleach.sanitizer.Cleaner(
    tags=list(bleach.sanitizer.ALLOWED_TAGS) + [
        "p", "div", "span", "hr", "br", "small",
        "b", "i", "u", "s", "strong", "em", "mark",
        "pre", "kbd", "samp", "var",
        "dd", "dt", "dl",
        "table", "thead", "tbody", "tr", "th", "td",
        "input",
        "time",
        "img",
        "q", "blockquote", "cite",
        "h1", "h2", "h3", "h4", "h5", "h6",
        "details", "summary",
        "abbr", "dfn",
        "del", "ins",
        "sup", "sub",
        "section", "aside",
        "figure", "figcaption",
        "ruby", "rp", "rt",
        "bdi", "bdo",
    ],
    attributes={**bleach.sanitizer.ALLOWED_ATTRIBUTES, **_sanitizer_attrs},
    protocols=[
        'ftp',
        'gemini',
        'gopher',
        'http',
        'https',
        'irc',
        'ircs',
        'mailto',
        'matrix',
        'xmpp',
    ],
    strip=True,
    **_sanitizer_css)

def sanitize(html):
    return add_noopener(_sanitizer.clean(html))

def add_noopener(html):
    soup = BeautifulSoup(str(html), 'html.parser')
    for a in soup.findAll('a'):
        a['rel'] = 'nofollow noopener'
    return str(soup)

def markdown(text, baselevel=1, link_prefix=None, with_styles=True, sanitize_output=True):
    text = text.replace("\r\n", "\n") # https://github.com/miyuchina/mistletoe/issues/124
    with SrhtRenderer(link_prefix, baselevel) as renderer:
        html = renderer.render(m.Document(text))
    if sanitize_output:
        html = sanitize(html)
    if with_styles:
        style = ".highlight { background: inherit; }"
        return Markup(f"<style>{style}</style>"
                + "<div class='markdown'>"
                + html
                + "</div>")
    else:
        return Markup(html)

Heading = namedtuple("Header", ["level", "name", "id", "children", "parent"])

def extract_toc(markup):
    soup = BeautifulSoup(str(markup), "html5lib")
    cur = top = Heading(
        level=0, children=list(),
        name=None, id=None, parent=None
    )
    for el in list(soup.descendants):
        try:
            level = ["h1", "h2", "h3", "h4", "h5", "h6"].index(el.name)
        except ValueError:
            continue
        while cur.level >= level:
            cur = cur.parent
        if el.a:
            el.a.extract()
        heading = Heading(
            level=level, name=el.text,
            id=el.attrs.get("id"),
            children=list(),
            parent=cur
        )
        cur.children.append(heading)
        cur = heading
    return top.children
