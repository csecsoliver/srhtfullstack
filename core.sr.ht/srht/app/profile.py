from flask import abort
from srht.graphql import exec_gql, GraphQLError, Error
from srht.config import get_origin

def get_profile(user):
    """
    Gets the profile details necessary to render profile.html for the given
    user.
    """
    try:
        resp = exec_gql("meta.sr.ht", """
        query {
            me {
                avatar
                bio
                location
                pronouns
                url
            }
        }
        """, user=user)
        return resp["me"]
    except GraphQLError as err:
        if err.has(Error.UNAUTHORIZED):
            abort(404)
        raise
