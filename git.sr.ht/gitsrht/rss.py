import pygit2
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta, timezone
from flask import Response, url_for
from gitsrht.git import Repository as GitRepository
from srht.config import cfg

# Date format used by RSS
RFC_822_FORMAT = "%a, %d %b %Y %H:%M:%S %z"

ORIGIN = cfg("git.sr.ht", "origin")

def aware_time(author):
    tzinfo = timezone(timedelta(minutes=author.offset))
    return datetime.fromtimestamp(author.time, tzinfo)

def ref_name(reference):
    return reference.raw_name.decode("utf-8", "replace").split("/")[-1]

def ref_url(repo, reference):
    return ORIGIN + url_for("repo.ref",
        owner=repo.owner.canonical_name,
        repo=repo.name,
        ref=ref_name(reference))

def commit_url(repo, commit):
    return ORIGIN + url_for("repo.commit",
        owner=repo.owner.canonical_name,
        repo=repo.name,
        ref=str(commit.id))

def commit_title_description(commit):
    """Split the commit message to title (first line) and the description
    (remaining lines)."""
    lines = commit.message.strip().split("\n")
    if lines:
        title = lines[0]
        description = "\n".join(lines[1:]).strip().replace("\n", "<br />")
        return title, description

    # Empty message fallback
    return str(commit.id), ""

def tag_title_description(tag):
    commit = tag.get_object()

    """Split the tag message to title (first line) and the description
    (remaining lines)."""
    lines = tag.message.strip().split("\n") if tag.message else None
    if lines:
        title = lines[0]
        description = "\n".join(lines[1:]).strip().replace("\n", "<br />")
        return title, description

    # Empty message fallback
    return str(commit.id), ""

def ref_to_item(repo, reference):
    with GitRepository(repo.path) as git_repo:
        target = git_repo.get(reference.target)

    author = target.author if hasattr(target, 'author') else target.get_object().author
    time = aware_time(author).strftime(RFC_822_FORMAT)
    url = ref_url(repo, reference)
    description = target.message.strip().replace("\n", "<br />")

    element = ET.Element("item")
    ET.SubElement(element, "title").text = ref_name(reference)
    ET.SubElement(element, "description").text = description
    ET.SubElement(element, "author").text = f"{author.email} ({author.name})"
    ET.SubElement(element, "link").text = url
    ET.SubElement(element, "guid").text = url
    ET.SubElement(element, "pubDate").text = time

    return element

def commit_to_item(repo, commit):
    time = aware_time(commit.author).strftime(RFC_822_FORMAT)
    url = commit_url(repo, commit)
    title, description = commit_title_description(commit)
    author = f"{commit.author.email} ({commit.author.name})"

    element = ET.Element("item")
    ET.SubElement(element, "title").text = title
    ET.SubElement(element, "description").text = description
    ET.SubElement(element, "author").text = author
    ET.SubElement(element, "link").text = url
    ET.SubElement(element, "guid").text = url
    ET.SubElement(element, "pubDate").text = time

    return element

def tag_to_item(repo, tag):
    time = aware_time(tag.tagger).strftime(RFC_822_FORMAT)
    url = ref_url(repo, tag)
    title, description = tag_title_description(tag)
    author = f"{tag.tagger.email} ({tag.tagger.name})"

    element = ET.Element("item")
    ET.SubElement(element, "title").text = title
    ET.SubElement(element, "description").text = description
    ET.SubElement(element, "author").text = author
    ET.SubElement(element, "link").text = url
    ET.SubElement(element, "guid").text = url
    ET.SubElement(element, "pubDate").text = time

    return element

def to_item(repo, item):
    if isinstance(item, pygit2.Reference):
        return ref_to_item(repo, item)

    if isinstance(item, pygit2.Commit):
        return commit_to_item(repo, item)

    if isinstance(item, pygit2.Tag):
        return tag_to_item(repo, item)

    raise ValueError(f"Don't know how to convert {type(item)} to an RSS item.")

def generate_refs_feed(repo, items, title, link, description):
    root = ET.Element("rss", version="2.0")
    channel = ET.SubElement(root, "channel")

    ET.SubElement(channel, "title").text = title
    ET.SubElement(channel, "link").text = link
    ET.SubElement(channel, "description").text = description
    ET.SubElement(channel, "language").text = "en"

    for item in items:
        channel.append(to_item(repo, item[1]))

    xml = ET.tostring(root, encoding="UTF-8")
    return Response(xml, mimetype='application/rss+xml')

def generate_commits_feed(repo, items, title, link, description):
    root = ET.Element("rss", version="2.0")
    channel = ET.SubElement(root, "channel")

    ET.SubElement(channel, "title").text = title
    ET.SubElement(channel, "link").text = link
    ET.SubElement(channel, "description").text = description
    ET.SubElement(channel, "language").text = "en"

    for item in items:
        channel.append(to_item(repo, item))

    xml = ET.tostring(root, encoding="UTF-8")
    return Response(xml, mimetype='application/rss+xml')
