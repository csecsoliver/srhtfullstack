from hubsrht.types import User
from srht.app import Flask
from srht.config import cfg
from srht.database import DbSession

db = DbSession(cfg("hub.sr.ht", "connection-string"))
db.init()

class HubApp(Flask):
    def __init__(self):
        super().__init__("hub.sr.ht", __name__, user_class=User)

        from hubsrht.blueprints.mailing_lists import mailing_lists
        from hubsrht.blueprints.projects import projects
        from hubsrht.blueprints.public import public
        from hubsrht.blueprints.sources import sources
        from hubsrht.blueprints.trackers import trackers
        from hubsrht.blueprints.users import users
        from hubsrht.blueprints.webhooks import webhooks
        from srht.graphql import gql_blueprint

        self.register_blueprint(mailing_lists)
        self.register_blueprint(projects)
        self.register_blueprint(public)
        self.register_blueprint(sources)
        self.register_blueprint(trackers)
        self.register_blueprint(users)
        self.register_blueprint(webhooks)
        self.register_blueprint(gql_blueprint)

        self.url_map.strict_slashes = False

app = HubApp()
