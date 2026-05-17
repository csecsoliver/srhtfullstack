from flask import current_app, make_response, request
from functools import update_wrapper

def cross_origin(f):
    """
    Enable CORS headers on a route.
    """

    f.required_methods = getattr(f, "required_methods", set())
    f.required_methods.add("OPTIONS")
    f.provide_automatic_options = False

    def wrapped_function(*args, **kwargs):
        if request.method == "OPTIONS":
            resp = current_app.make_default_options_response()
        else:
            resp = make_response(f(*args, **kwargs))
        resp.headers["Access-Control-Allow-Origin"] = "*"
        resp.headers["Access-Control-Allow-Methods"] = "OPTIONS, GET, POST"
        resp.headers["Access-Control-Allow-Headers"] = "Content-Type, Authorization"
        return resp

    return update_wrapper(wrapped_function, f)
