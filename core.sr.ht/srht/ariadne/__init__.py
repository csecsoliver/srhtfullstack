import ast
import string
from ariadne_codegen.utils import str_to_snake_case
from ariadne_codegen.plugins.base import Plugin
from collections import namedtuple
from graphql import OperationDefinitionNode
from graphql.language import print_ast
from typing import Union

Operation = namedtuple('Operation', ["name", "op"])

_imports_tmpl = """
import httpx
from flask import request, has_request_context
from srht.config import cfg, get_api
from srht.graphql import InternalAuth
"""

_init_tmpl = string.Template('''
def __init__(self, auth=None, headers=None, client=None, **kwargs):
    origin = get_api("$service")
    if auth is None:
        auth = InternalAuth()
    headers = {
        **auth.headers,
        "User-Agent": "$service frontend",
    }
    if has_request_context():
        headers["X-Forwarded-For"] = ", ".join(request.access_route)
    if not client:
        backend_timeout = cfg("$service", "backend-timeout", default=3)
        client = httpx.Client(headers=headers, timeout=httpx.Timeout(int(backend_timeout)))
    super().__init__(origin + "/query", headers, client, **kwargs)
''')

_query_tmpl = string.Template("$name = $value")

class AriadnePlugin(Plugin):
    """
    ariadne-codegen plugin that amends the client class with a more convenient
    constructor for use with core.sr.ht.
    """

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.queries = []

    def generate_client_module(self, module: ast.Module) -> ast.Module:
        module.body.insert(0, ast.parse(_imports_tmpl))
        return module

    def generate_client_class(self, class_def: ast.ClassDef) -> ast.ClassDef:
        service = (
            self.config_dict.get("tool")
            .get("ariadne-codegen")
            .get("sourcehut")
            .get("service_name")
        )
        class_def.body.insert(0, ast.parse(_init_tmpl.substitute({
            "service": service,
        })))
        for query in self.queries:
            qname = str_to_snake_case(query.name) + "_query"
            value = ast.unparse(ast.Constant(value=query.op))
            class_def.body.append(ast.parse(_query_tmpl.substitute({
                "name": qname,
                "value": value,
            })))
        return class_def

    def generate_client_method(
            self,
            method_def: Union[ast.FunctionDef, ast.AsyncFunctionDef],
            operation_definition: OperationDefinitionNode,
        ) -> Union[ast.FunctionDef, ast.AsyncFunctionDef]:
        op = print_ast(operation_definition)
        name = operation_definition.name.value
        self.queries.append(Operation(name, op))
        return method_def
