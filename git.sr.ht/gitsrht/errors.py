from flask import abort, redirect, request, url_for
from contextlib import contextmanager
from srht.graphql import Error, has_error, get_redirect
from gitsrht.graphql import GraphQLClientGraphQLMultiError

@contextmanager
def handle_gql_error():
    try:
        yield
    except GraphQLClientGraphQLMultiError as err:
        if has_error(err, Error.REDIRECT):
            new_owner, new_repo = get_redirect(err)
            abort(redirect(url_for(request.endpoint,
                owner_name=new_owner, repo_name=new_repo)))
        if has_error(err, Error.ACCESS_DENIED):
            abort(404)
        raise
