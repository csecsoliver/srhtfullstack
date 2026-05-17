from sqlalchemy import or_
from srht import search
from pastesrht.graphql import Visibility
from pastesrht.types import Paste, PasteFile, Blob


def visibility_filter(value):
    visibility = getattr(Visibility, value, None)
    if visibility is None:
        raise ValueError(f"Invalid visibility: '{value}'")

    return Paste.visibility == visibility

def sha_filter(value):
    try:
        int(value, 16)
    except ValueError:
        raise ValueError(f"Invalid SHA: '{value}'")

    return or_(
        Paste.sha.ilike(f"%{value}%"),
        Blob.sha.ilike(f"%{value}%"),
    )

def default_filter(value):
    return Paste.files.any(PasteFile.filename.ilike(f"%{value}%"))

def apply_search(query, search_string):
    terms = list(search.parse_terms(search_string))
    if not terms:
        return query

    return search.apply_terms(query.join(Paste.files).join(PasteFile.blob), terms, default_filter, key_fns={
        "visibility": visibility_filter,
        "sha": sha_filter,
    })
