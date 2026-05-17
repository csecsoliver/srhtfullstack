from email.utils import parseaddr
from listssrht.filters import diffstat, format_body, post_address
from listssrht.graphql import Visibility
from listssrht.types import User, PatchsetStatus, ListAccess
from srht.app import Flask
from srht.config import cfg
from srht.database import DbSession
from urllib.parse import quote

db = DbSession(cfg("lists.sr.ht", "connection-string"))
db.init()

class ListsApp(Flask):
    def __init__(self):
        super().__init__("lists.sr.ht", __name__, user_class=User)

        self.url_map.strict_slashes = False

        from listssrht.blueprints.archives import archives
        from listssrht.blueprints.patches import patches
        from listssrht.blueprints.settings import settings
        from listssrht.blueprints.user import user
        from srht.graphql import gql_blueprint

        self.register_blueprint(archives)
        self.register_blueprint(patches)
        self.register_blueprint(settings)
        self.register_blueprint(user)
        self.register_blueprint(gql_blueprint)

        @self.context_processor
        def inject():
            return {
                "ListAccess": ListAccess,
                "Visibility": Visibility,
                "diffstat": diffstat,
                "format_body": format_body,
                "parseaddr": parseaddr,
                "PatchsetStatus": PatchsetStatus,
                "post_address": post_address,
                "quote": quote,
            }

app = ListsApp()
