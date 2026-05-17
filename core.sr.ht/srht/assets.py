import hashlib
import os.path
from flask import Blueprint, current_app, send_from_directory
from os import getcwd
from srht.config import cfg

_assets = cfg("sr.ht", "assets", default="/usr/share/sourcehut")
_static_cache = {}
static_blueprint = Blueprint('srht_static', __name__)

@static_blueprint.route("/static/<path:filename>")
def static_asset(filename):
    filename = os.path.normpath(filename)

    if current_app.debug:
        dev_path = os.path.join(getcwd(), 'static', current_app.site)
        if os.path.exists(os.path.join(dev_path, filename)):
            return send_from_directory(dev_path, filename)

    site_assets = os.path.join(_assets, 'static', current_app.site)
    if os.path.exists(os.path.join(site_assets, filename)):
        return send_from_directory(site_assets, filename)

    shared_assets = os.path.join(_assets, 'static')
    return send_from_directory(shared_assets, filename)

def static_resource(name, minify=False):
    """
    Given "example.css", returns "/static/$site/example.$hash.css".
    """
    if name in _static_cache:
        return _static_cache[name]

    site = current_app.site
    if not current_app.debug:
        path, ext = os.path.splitext(name)
        if minify:
            minify = ".min"
        else:
            minify = ""
        name = f"{path}{minify}{ext}"

        sha256 = hashlib.sha256()
        with open(os.path.join(_assets, 'static', site, name), "rb") as f:
            sha256.update(f.read())
        url = f"/static/{site}/{path}{minify}.{sha256.hexdigest()[:8]}{ext}"
    else:
        url = f"/static/{site}/{name}"

    _static_cache[name] = url
    return url
