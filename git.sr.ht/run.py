import logging
import os.path
import re
import subprocess
from srht.config import cfg
from werkzeug.exceptions import BadRequest
from werkzeug.wrappers import Request, Response
from werkzeug.wsgi import wrap_file

def configure_git_arguments(parser):
    parser.add_argument('--http-serve', action='store_true',
        help="Also serve the Git repositories for HTTP cloning.")

def configure_git_app(app, args):
    if not args.http_serve:
        return

    gitreposdir = cfg('git.sr.ht', 'repos')
    print("Serving git repos from {}".format(gitreposdir))
    app.wsgi_app = HttpGitRepos(app.wsgi_app, gitreposdir)

re_git1 = re.compile(
    r"^.*/objects/([0-9a-f]+/[0-9a-f]+|pack/pack-[0-9a-f]+.(pack|idx)).*$")
re_git2 = re.compile(
    r"^.*/(HEAD|info/refs|objects/info/.*|git-(upload|receive)-pack).*$")

logger = logging.getLogger('werkzeug')

class HttpGitRepos:
    def __init__(self, app, reposdir, ssl=None):
        self._app = app
        self._reposdir = reposdir
        self._ssl = None

    def __call__(self, environ, start_response):
        request = Request(environ)

        if re_git1.search(request.path):
            path = os.path.join(self._reposdir, request.path.lstrip('/'))
            if os.path.exists(path):
                f = wrap_file(environ, open(path))
                return Response(f, direct_passthrough=True)

        if re_git2.search(request.path):
            subenv = environ.copy()
            for k in list(subenv.keys()):
                if (k.startswith('wsgi') or k.startswith('werkzeug') or
                        type(subenv[k]) is not str):
                    del subenv[k]

            subenv['GIT_PROJECT_ROOT'] = self._reposdir
            subenv['GIT_HTTP_EXPORT_ALL'] = "1"
            p = subprocess.Popen(['git', 'http-backend'],
                    cwd=self._reposdir, env=subenv, stdin=subprocess.PIPE,
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            try:
                stdin = request.data
                stdout, stderr = p.communicate(input=stdin, timeout=30)
            except subprocess.TimeoutExpired:
                logger.warning("Git HTTP backend timed out:")
                logger.warning(stderr.decode())
                return BadRequest()(environ, start_response)

            sep = stdout.find(b'\r\n\r\n')
            headers = []
            body_start = 0
            if sep > 0:
                body_start = sep + 4
                raw_headers = stdout[:sep].decode()
                for i, line in enumerate(raw_headers.split('\r\n')):
                    sepidx = line.find(':')
                    if sepidx > 0:
                        headers.append((line[:sepidx], line[sepidx+1:].lstrip()))
                    else:
                        logger.warning("Skipping malformed header: %s" % line)

            if stderr:
                logger.warning("Errors while serving Git repo:")
                logger.warning(stderr.decode())
            body = stdout[body_start:]
            r = Response(body, headers=headers)
            return r(environ, start_response)

        return self._app(environ, start_response)

if __name__ == '__main__':
    from srht.debug import build_parser, run_app
    from gitsrht.app import app
    parser = build_parser(app)
    configure_git_arguments(parser)
    args = parser.parse_args()
    configure_git_app(app, args)
    run_app(app)
