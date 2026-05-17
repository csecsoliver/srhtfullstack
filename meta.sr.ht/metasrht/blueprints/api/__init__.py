from flask import Blueprint
from metasrht.blueprints.api.user import user
from srht.app import csrf_bypass

def register_api(app):
    app.register_blueprint(user)

    csrf_bypass(user)

    @app.route("/api/version")
    def version():
        return { "version": "deprecated" }
