import humanize
import json
import stat
from pastesrht import filters
from pastesrht.graphql import Visibility
from pastesrht.types import User
from srht.app import Flask
from srht.config import cfg
from srht.database import DbSession

db = DbSession(cfg("paste.sr.ht", "connection-string"))
db.init()

class PasteApp(Flask):
    def __init__(self):
        super().__init__("paste.sr.ht", __name__, user_class=User)

        self.url_map.strict_slashes = False

        from pastesrht.blueprints.public import public
        from srht.graphql import gql_blueprint

        self.register_blueprint(public)
        self.register_blueprint(gql_blueprint)
        self.add_template_filter(filters.render_markdown)

        @self.context_processor
        def inject():
            return {
                "humanize": humanize,
                "json": json.dumps,
                "stat": stat,
                "Visibility": Visibility,
            }


app = PasteApp()
