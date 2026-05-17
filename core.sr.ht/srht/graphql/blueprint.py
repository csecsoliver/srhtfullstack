import json
import pygments
import requests
import shlex
from flask import Blueprint, current_app, render_template, request, abort
from markupsafe import Markup
from pygments import highlight
from pygments.formatters import HtmlFormatter
from pygments.lexers import JsonLexer
from srht.gql_lexer import GraphqlLexer
from srht.config import get_api
from srht.oauth import loginrequired
from srht.validation import Validation
from urllib.parse import urlparse

gql_blueprint = Blueprint('srht_graphql', __name__)

_schema_html = None

def format_result(result):
    j = json.dumps(result, indent=2)
    lexer = JsonLexer()
    formatter = HtmlFormatter()
    style = formatter.get_style_defs('.highlight')
    html = (f"<style>{style}</style>"
            + highlight(j, lexer, formatter))
    return Markup(html)

def execute_gql(query, variables={}):
    origin = get_api(current_app.site)
    r = requests.post(f"{origin}/query",
            cookies=request.cookies,
            headers={"Content-Type": "application/json"},
            json={"query": query, "variables": variables})
    return format_result(r.json())

@gql_blueprint.route("/graphql")
@loginrequired
def query_explorer():
    schema = current_app.graphql_schema
    query = current_app.graphql_query
    global _schema_html
    if _schema_html is None:
        lexer = GraphqlLexer()
        formatter = HtmlFormatter()
        style = formatter.get_style_defs('.highlight')
        _schema_html = (f"<style>{style}</style>"
                + highlight(schema, lexer, formatter))
        _schema_html = Markup(_schema_html)
    results = execute_gql(query)
    return render_template("graphql.html",
            schema=_schema_html, query=query, variables="",
            json=shlex.quote(json.dumps({
                "query": query,
            }, indent=4)),
            results=results)

@gql_blueprint.route("/graphql", methods=["POST"])
@loginrequired
def query_explorer_POST():
    schema = current_app.graphql_schema
    valid = Validation(request)
    query = valid.require("query")
    variables = valid.optional("variables")

    global _schema_html
    if _schema_html is None:
        lexer = GraphqlLexer()
        formatter = HtmlFormatter()
        style = formatter.get_style_defs('.highlight')
        _schema_html = (f"<style>{style}</style>"
                + highlight(schema, lexer, formatter))
        _schema_html = Markup(_schema_html)

    try:
        variables = json.loads(variables) if variables else {}
    except json.JSONDecodeError as e:
        valid.error(f"{e.msg} at line {e.lineno}, column {e.colno}",
                    field="variables")
    if not valid.ok:
        (err, status) = valid.response
        results = format_result(err)
        return render_template("graphql.html", schema=_schema_html,
                results=results, **valid.kwargs), 400

    results = execute_gql(query, variables)
    return render_template("graphql.html",
            schema=_schema_html, results=results,
            json=shlex.quote(json.dumps({
                "query": query,
                "variables": variables,
            }, indent=4)),
            **valid.kwargs)
