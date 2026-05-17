import yaml
from markupsafe import escape
from srht.markdown import markdown

def extract_frontmatter(content):
    """Separate YAML frontmatter, if it exists, from the content."""

    start_marker = "---\n"
    end_marker = "\n---\n"
    if not content.startswith(start_marker):
        return content, None

    start_pos = len(start_marker)
    end_pos = content.find(end_marker, len(start_marker))
    if end_pos < 0:
        return content, None

    frontmatter = content[start_pos:end_pos].strip()
    if not frontmatter:
        return content, None

    try:
        frontmatter = yaml.safe_load(frontmatter)
    except yaml.YAMLError:
        return content, None

    if not isinstance(frontmatter, dict):
        return content, None

    content_start = end_pos + len(end_marker)
    content = content[content_start:].strip()
    return content, frontmatter

def render_frontmatter(frontmatter):
    """Render parsed YAML frontmatter into a (nested) HTML table."""

    def render(val):
        if isinstance(val, str):
            return escape(val)

        if isinstance(val, list):
            html = '<div class="fm-list">'
            for v in val:
                html += f'<div class="fm-list-item">{render(v)}</div>'
            html += '</div>'
            return html

        if isinstance(val, dict):
            html = '<table class="fm-dict">'
            for k, v in val.items():
                html += f"<tr><th>{escape(k)}</th><td>{render(v)}</td></tr>"
            html += "</table>"
            return html

        return escape(str(val))

    return f'<div class="frontmatter">{render(frontmatter)}</div><hr />'

def render_markdown(content):
    content, frontmatter = extract_frontmatter(content)
    if frontmatter:
        content = render_frontmatter(frontmatter) + "\n\n" + content
    return markdown(content)
