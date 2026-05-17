from redis import from_url as from_redis_url, Sentinel
from urllib.parse import parse_qsl, urlparse
from srht.config import cfg


def from_sentinel_urls(urls):
    # Connection parameters like username and password MUST be the same for all
    # sentinels, so just take those from the first URL...
    u = urls[0]

    kwargs = dict(parse_qsl(u.query))
    if u.scheme == 'rediss+sentinel':
        kwargs['ssl'] = True
    if u.username:
        kwargs['username'] = u.username
    if u.password:
        kwargs['password'] = u.password

    master = u.path[1:]
    if '/' in master:
        master, s, db = master.partition('/')
        kwargs['db'] = db

    sentinels = list(map(lambda u: (u.hostname, u.port), urls))
    sentinel = Sentinel(sentinels, sentinel_kwargs=kwargs, **kwargs)
    return sentinel.master_for(master)

def from_url(url):
    urls = list(map(lambda u: urlparse(u), url.split(',')))
    enforce_uniform = {
        'scheme': list(set(map(lambda u: u.scheme, urls))),
        'username': list(set(map(lambda u: u.username, urls))),
        'password': list(set(map(lambda u: u.password, urls)))
    }

    if len(enforce_uniform['scheme']) == 0 or len(urls) == 0:
        raise Exception("Invalid redis connection URL")
    for part in enforce_uniform:
        if len(enforce_uniform[part]) > 1:
            raise Exception(f"Multiple redis connection URLs must have uniform {part}")

    if len(urls) == 1:
        u = urls[0]
        if u.scheme in ['redis', 'rediss', 'unix']:
            return from_redis_url(url)
        elif u.scheme in ['redis+sentinel', 'rediss+sentinel']:
            return from_sentinel_urls(urls)
        else:
            raise Exception("Unknown redis connection URL scheme")
    else:
        if not enforce_uniform['scheme'][0] in ['redis+sentinel', 'rediss+sentinel']:
            raise Exception("Multiple redis connection URLs only supported for '+sentinel' schemes")
        return from_sentinel_urls(urls)

redis = from_url(cfg("sr.ht", "redis-host", "redis://localhost"))
