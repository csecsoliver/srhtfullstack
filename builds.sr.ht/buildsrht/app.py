from buildsrht.types import JobStatus, User
from datetime import datetime, timedelta
from flask import session
from humanize import naturalsize
from srht.app import Flask
from srht.config import cfg
from srht.database import DbSession

db = DbSession(cfg("builds.sr.ht", "connection-string"))
db.init()

class BuildApp(Flask):
    def __init__(self):
        super().__init__("builds.sr.ht", __name__, user_class=User)

        self.url_map.strict_slashes = False

        from buildsrht.blueprints.admin import admin
        from buildsrht.blueprints.jobs import jobs
        from buildsrht.blueprints.secrets import secrets
        from buildsrht.blueprints.settings import settings
        from srht.graphql import gql_blueprint

        self.register_blueprint(admin)
        self.register_blueprint(settings)
        self.register_blueprint(jobs)
        self.register_blueprint(secrets)
        self.register_blueprint(gql_blueprint)

        @self.context_processor
        def inject():
            return {
                "datetime": datetime,
                "timedelta": timedelta,
                "JobStatus": JobStatus,
                "naturalsize": naturalsize,
            }

app = BuildApp()
