from enum import Enum, IntEnum
from markupsafe import escape, Markup
from urllib import parse
import json

class ValidationError:
    def __init__(self, field, message):
        self.field = field
        self.message = escape(message)

    def json(self):
        j = dict()
        if self.field:
            j['field'] = self.field
        if self.message:
            j['reason'] = self.message
        return j

class ValidCatch:
    def __init__(self, valid, exc_type, *args, **kwargs):
        self.valid = valid
        self.exc_type = exc_type
        self.args = args
        self.kwargs = kwargs

    def __enter__(self):
        return self.valid

    def __exit__(self, exc_type, exc_value, traceback):
        if exc_type is None:
            return
        if exc_type != self.exc_type:
            raise exc_value
        self.valid.error(*self.args, **self.kwargs)
        return True

class Validation:
    def __init__(self, request):
        self.files = dict()
        self.errors = []
        self.status = 400
        if isinstance(request, dict):
            self.source = request
        else:
            contentType = request.headers.get("Content-Type")
            if contentType and contentType == "application/json":
                try:
                    self.source = json.loads(request.data.decode('utf-8'))
                    if not isinstance(self.source, dict):
                        self.error("Expected JSON dictionary")
                        self.source = {}
                except json.JSONDecodeError:
                    self.error("Invalid JSON provided")
                    self.source = {}
            else:
                self.source = request.form
                self.files = request.files
            self.request = request
        self._kwargs = {
            "valid": self,
            **self.source,
        }

    @property
    def ok(self):
        return len(self.errors) == 0

    def cls(self, name):
        return 'is-invalid' if any([
            e for e in self.errors if e.field == name
        ]) else ""

    def summary(self, name=None):
        errors = [e.message for e in self.errors if e.field == name or name == '@all']
        if len(errors) == 0:
            return ''
        if name is None:
            return Markup('<div class="alert alert-danger">{}</div>'
                    .format('<br />'.join(errors)))
        else:
            return Markup('<div class="invalid-feedback">{}</div>'
                    .format('<br />'.join(errors)))

    @property
    def response(self):
        return { "errors": [ e.json() for e in self.errors ] }, self.status

    @property
    def kwargs(self):
        return self._kwargs

    def catch(self, exc_type, *args, **kwargs):
        """
        Returns a context manager that, in the event the block raises exc_type,
        will catch the exception and pass *args and **kwargs to valid.error.
        """
        return ValidCatch(self, exc_type, *args, **kwargs)

    def error(self, message, field=None, status=None):
        self.errors.append(ValidationError(field, message))
        if status:
            self.status = status
        return self.response

    def optional(self, name, default=None, cls=None, max_file_size=-1):
        value = self.source.get(name)
        if value is None:
            value = self.files.get(name)
            if value and value.filename:
                if max_file_size >= 0:
                    fbytes = value.read(max_file_size + 1)
                    if len(fbytes) == max_file_size + 1:
                        self.error('{} is too large'.format(name), field=name)
                        return None
                    else:
                        value = fbytes
                else:
                    value = value.read()
        if value is None:
            if name in self.source:
                self.error('{} may not be null'.format(name), field=name)
                return None
            else:
                value = default
        if cls and value is not None:
            if cls and issubclass(cls, IntEnum):
                if not isinstance(value, int):
                    self.error('{} should be an int'.format(name), field=name)
                    return None
                else:
                    try:
                        value = cls(value)
                    except ValueError:
                        self.error('{} is not a valid {}'.format(
                            value, cls.__name__), field=name)
                        return None
            elif issubclass(cls, Enum):
                if not isinstance(value, str):
                    self.error("{} should be an str".format(name), field=name)
                else:
                    if value not in cls:
                        self.error("{} should be a valid {}".format(name, cls.__name__),
                                field=name)
                        return None
                    else:
                        try:
                            value = cls(value)
                        except ValueError:
                            self.error('{} is not a valid {}'.format(
                                value, cls.__name__), field=name)
            elif not isinstance(value, cls):
                self.error('{} should be a {}'.format(name, cls.__name__), field=name)
                return None
        return value

    def require(self, name, cls=None, friendly_name=None):
        value = self.optional(name, None, cls)
        if not friendly_name:
            friendly_name = name
        if not isinstance(value, (bool, Enum)) and not value:
            self.error('{} is required'.format(friendly_name), field=name)
        return value

    def expect(self, condition, message, field=None, **kwargs):
        if not condition:
            self.error(message, field, **kwargs)

    def copy(self, valid, field=None):
        for err in self.errors:
            valid.error(err.message, field + "." + err.field)

    def error_for(self, *fields):
        for error in self.errors:
            if error.field in fields:
                return True
        return False

    def __contains__(self, value):
        return value in self.source or value in self.files

    def __enter__(self):
        pass

    def __exit__(self, exc_type, exc_value, traceback):
        if exc_type is None:
            return

        # XXX: This is hacky. We should figure out a way to centralize the
        # exceptions used by Ariadne-generated code so we don't have to munge
        # strings here
        is_gql = exc_type.__name__ == "GraphQLClientGraphQLMultiError"

        if not is_gql:
            raise exc_value

        for err in exc_value.errors:
            msg = err.message
            ext = err.extensions
            if ext:
                field = ext.get("field")
            else:
                field = None
            self.error(escape(msg), field=field)

        return True

def valid_url(url):
    allowed_schemes = ('http', 'https', 'gemini', 'gopher')
    try:
        u = parse.urlparse(url)
        return bool(u.scheme and u.netloc and u.scheme in allowed_schemes)
    except:
        return False
