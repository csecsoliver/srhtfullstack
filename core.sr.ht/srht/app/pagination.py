from flask import request
from jinja2 import pass_context
from markupsafe import Markup
from urllib.parse import quote_plus

@pass_context
def coalesce_search_terms(context):
    ret = ""
    for key in ["search"] + (context.get("search_keys") or []):
        val = context.get(key)
        if val:
            val = quote_plus(val)
            ret += f"&{key}={val}"
    return ret

@pass_context
def pagination(context):
    template = context.environment.get_template("pagination.html")
    return Markup(template.render(**context.parent))

def paginate_query(query, results_per_page=15):
    page = request.args.get("page")
    total_results = query.count()
    total_pages = total_results // results_per_page + 1
    if total_results % results_per_page == 0:
        total_pages -= 1
    if page is not None:
        try:
            page = int(page) - 1
            query = query.offset(page * results_per_page)
        except:
            page = 0
    else:
        page = 0
    if page < 0:
        abort(400)
    query = query.limit(results_per_page).all()
    return query, {
        "total_pages": total_pages,
        "page": page + 1,
        "total_results": total_results
    }
