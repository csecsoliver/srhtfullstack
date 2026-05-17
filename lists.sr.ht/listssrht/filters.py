import re
from markupsafe import Markup, escape
from jinja2.filters import urlize
from srht.config import cfg

def post_address(ml, suffix=""):
    if ml.mirror_id:
        if suffix == "+subscribe":
            return ml.mirror.list_subscribe
        elif suffix == "+unsubscribe":
            return ml.mirror.list_unsubscribe
        elif suffix == "":
            return ml.mirror.list_post
        else:
            return None

    domain = cfg("lists.sr.ht", "posting-domain")
    return "{}/{}{}@{}".format(
            ml.owner.canonical_name, ml.name, suffix, domain)

def format_body(msg, limit=None, nowrap=False):
    if msg.patch() is not None:
        return _format_patch(msg, limit, nowrap)

    message = msg.parsed()
    body = message.get_body(preferencelist=('plain'))
    if not body:
        return _format_plain(msg, limit, nowrap)

    content_type = body.get("Content-Type")
    if content_type:
        attrs = [attr.strip() for attr in content_type.split(";")]
        for attr in attrs:
            # This is under-specified by the RFC and can appear in many
            # different ways in practice; be tolerant of variants
            flowed_attrs = [
                "format=flowed",
                "format='flowed'",
                'format="flowed"',
            ]
            if attr.lower() in flowed_attrs:
                return _format_flowed(msg, limit, nowrap)

    return _format_plain(msg, limit, nowrap)

def _format_patch(msg, limit=None, nowrap=False):
    text = Markup("")
    if not nowrap:
        text += Markup("<pre class='message-body'>")
    is_diff = False

    # Predict the starting lines of each file name
    patch = msg.patch()
    old_files = {delta.old_file.path for delta in patch.deltas}
    new_files = {delta.new_file.path for delta in patch.deltas}
    file_lines = {
        f" {p} ": p for p in old_files | new_files
    }
    line_no = 0

    for line in msg.body.replace("\r", "").split("\n"):
        line_no += 1
        if line_no == limit:
            text = text.rstrip()
            text += Markup(
                "\n<span class='text-muted'>[message trimmed]</span>"
            )
            break
        if not is_diff:
            f = next((
                key for key in file_lines.keys() if line.startswith(key)
            ), None)
            if f != None:
                f = file_lines[f]
                text += Markup(" <a href='#{}'>{}</a>".format(
                    escape(msg.message_id) + "+" + escape(f), escape(f)))
                try:
                    stat = line[line.rindex(" ") + 1:]
                    line = line[:line.rindex(" ") + 1]
                    if "+" in stat and "-" in stat:
                        removed = stat[stat.index("-"):]
                        added = stat[:stat.index("-")]
                        stat = Markup(("<span class='text-success'>{}</span>" +
                            "<span class='text-danger'>{}</span>"
                        ).format(escape(added), escape(removed)))
                    elif "-" in stat:
                        stat = Markup(
                                "<span class='text-danger'>{}</span>".format(
                                    escape(stat)))
                    elif "+" in stat:
                        stat = Markup(
                                "<span class='text-success'>{}</span>".format(
                                    escape(stat)))
                    else:
                        stat = escape(stat)
                except ValueError:
                    stat = Markup("")
                text += escape(line[len(f) + 1:])
                text += escape(stat)
                text += Markup("\n")
            else:
                text += escape(line + "\n")
            if line.startswith("diff"):
                is_diff = True
        else:
            if line.strip() == "--":
                text += escape(line + "\n")
            elif line.startswith("+++") and not line.startswith("++++"):
                path = line[4:].lstrip("b/")
                if f" {path} " in file_lines:
                    text += (
                        Markup("<a href='#{0}' id='{0}' class='text-info'>".format(
                            escape(msg.message_id) + "+" + escape(path)
                        ))
                        + escape(line)
                        + Markup("</a>\n"))
                    continue
            elif line.startswith("---") and not line.startswith("----"):
                text += (
                    Markup("<span class='text-info'>")
                    + escape(line)
                    + Markup("</span>\n"))
                continue
            elif line.startswith("+"):
                text += (
                    Markup("<span class='text-success'>")
                    + Markup(
                        ("<a aria-hidden='true' class='lineno text-success' " +
                        "href='#{0}-{1}' id='{0}-{1}'>+</a>").format(
                            escape(msg.message_id), line_no))
                    + escape(line[1:])
                    + Markup("</span>\n"))
            elif line.startswith("-"):
                text += (
                    Markup("<span class='text-danger'>")
                    + Markup(
                        ("<a aria-hidden='true' class='lineno text-danger' " +
                        "href='#{0}-{1}' id='{0}-{1}'>-</a>").format(
                            escape(msg.message_id), line_no))
                    + escape(line[1:])
                    + Markup("</span>\n"))
            elif line.startswith(" "):
                text += (
                    Markup("<a aria-hidden='true' " +
                        "class='lineno' href='#{0}-{1}'" +
                        "id='{0}-{1}'> </a>".format(
                            escape(msg.message_id), line_no))
                    + escape(line[1:] + "\n"))
            else:
                text += escape(line + "\n")
    text = text.rstrip()
    if not nowrap:
        text += Markup("</pre>")
    return text

def _format_plain(msg, limit=None, nowrap=False):
    text = Markup("")
    if not nowrap:
        text += Markup("<pre class='message-body'>")
    line_no = 0
    body = urlize(msg.body, rel="noopener nofollow")
    for line in msg.body.replace("\r", "").split("\n"):
        line_no += 1
        if line_no == limit:
            break
        if line.startswith(">"):
            text += (
                Markup("<span class='text-muted'>")
                    + Markup(urlize(escape(line), rel="noopener nofollow"))
                + Markup("</span>\n"))
        else:
            text += Markup(urlize(escape(line), rel="noopener nofollow")) + "\n"
    text = text.rstrip()
    if not nowrap:
        text += Markup("</pre>")
    return text

_whitespace_re = r"^[ \t]*"

def _format_flowed(msg, limit=None, nowrap=False):
    text = Markup("")
    if not nowrap:
        text += Markup("<div class='message-body flowed'>")
    line_no = 0
    body = urlize(msg.body, rel="noopener nofollow")

    first_line = True
    was_flowed = False
    was_empty = False
    was_code = False
    p_open = False
    prior_quote_depth = 0
    code_prefix = ""
    for line in msg.body.replace("\r", "").split("\n"):
        line_no += 1
        if line_no == limit:
            break

        if line == "-- ":
            text += Markup("<p>&mdash; <br />")
            p_open = True
            continue

        content = line.lstrip(">")
        quote_depth = len(line) - len(content)
        is_stuffed = content.startswith(" ")
        if is_stuffed:
            content = content[1:]
        is_flowed = content.endswith(" ")
        if is_flowed:
            content = content[:-1]
        is_empty = content == ""

        # XXX: Extension not specified by RFC 3676
        is_code = content.startswith("    ") or content.startswith("\t")
        if is_code and code_prefix == "":
            # Trim off leading whitespace based on the first line depth
            code_prefix = re.match(_whitespace_re, content).group()

        if not is_empty and not was_empty and not is_code and not was_code:
            if not first_line and not was_flowed:
                text += Markup("<br>")

        if quote_depth > prior_quote_depth:
            n = prior_quote_depth
            while n < quote_depth:
                if p_open:
                    text += Markup("</p>")
                    p_open = False
                text += Markup('<blockquote class="text-muted">')
                n += 1
        elif quote_depth < prior_quote_depth:
            n = prior_quote_depth
            while n > quote_depth:
                if p_open:
                    text += Markup("</p>")
                    p_open = False
                text += Markup("</blockquote>")
                n -= 1

        if not is_code and was_code:
            text += Markup("</pre>")
            code_prefix = ""
        if is_code and not was_code:
            text += Markup("<pre>")

        if is_code:
            line = content.removeprefix(code_prefix)
            text += Markup(urlize(escape(line), rel="noopener nofollow")) + "\n"
        else:
            if is_empty and p_open:
                text += Markup("</p>")
                p_open = False
            elif not is_empty and (is_flowed and not was_flowed or not p_open
                    or quote_depth != prior_quote_depth):
                text += Markup("<p>")
                p_open = True
            text += Markup(urlize(escape(content), rel="noopener nofollow")) + "\n"

        first_line = False
        was_flowed = is_flowed
        was_empty = is_empty
        was_code = is_code
        prior_quote_depth = quote_depth

    while prior_quote_depth != 0:
        if p_open:
            text += Markup("</p>")
            p_open = False
        text += Markup("</blockquote>")
        prior_quote_depth -= 1

    if was_code:
        text += Markup("</pre>")

    text = text.rstrip()
    if not nowrap:
        text += Markup("</div>")
    return text

def diffstat(patch_email):
    stats = patch_email.patch().stats
    return type("diffstat", tuple(), {
        "added": stats.insertions,
        "removed": stats.deletions,
    })
