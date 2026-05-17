from srht.config import cfg, cfgi
import argparse

def build_parser(app):
    parser = argparse.ArgumentParser(
        description='Development server for %s' % app.site)
    return parser

def run_app(app):
    cfg_section = app.site
    app.run(host=cfg(cfg_section, "debug-host"),
            port=cfgi(cfg_section, "debug-port"),
            debug=True)

def run_service(app):
    parser = build_parser(app)
    args = parser.parse_args()
    run_app(app)
