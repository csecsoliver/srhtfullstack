from enum import Enum
from typing import Union

class Error(Enum):
    """
    Standard error codes defined by core-go.
    """
    ACCESS_DENIED = "ERR_ACCESS_DENIED"
    NOT_FOUND = "ERR_NOT_FOUND"
    UNSUPPORTED = "ERR_UNSUPPORTED"
    UNAUTHORIZED = "ERR_UNAUTHORIZED"
    INTERNAL_ERROR = "ERR_INTERNAL"
    TIMEOUT = "ERR_TIMEOUT"
    REDIRECT = "ERR_REDIRECT"

def has_error(exc, err):
    """
    Returns true if the given error code is present in an Exception generated
    by ariadne-codegen.
    """
    err_code = err.value if hasattr(err, "value") else err
    for error in exc.errors:
        if error.extensions.get("code") == err_code:
            return True
    return False

def get_redirect(exc):
    for error in exc.errors:
        if error.extensions.get("code") == Error.REDIRECT.value:
            return error.extensions.get("redirect", [])
    return None

class GraphQLError(Exception):
    def __init__(self, body):
        self.body = body
        self.errors = body["errors"]
        self.data = body.get("data")

    def has(self, code: Union[str, Error], path: list[str]=None):
        if isinstance(code, Error):
            code = code.value
        """
        Returns true if the given standard error code is present.

        If path is not None, only tests the specified path.
        """
        for err in self.errors:
            present = err.get("extensions", {}).get("code") == code
            if not present:
                continue
            if path is None:
                return True
            if err["path"] == path:
                return True
        return False
