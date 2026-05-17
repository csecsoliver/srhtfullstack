import urllib.parse

def get_clone_urls(origin_url, owner, repo, ssh_format):
    """ Gets a standard list of clone URLs to show on a repository's
        summary page (read-only https, read/write ssh).
    """
    # Remove the scheme and port from the origin URL.
    parsed_origin_url = urllib.parse.urlparse(origin_url)
    base = parsed_origin_url.hostname

    clone_urls = [
        {'desc': "read-only", 'url': "https://{origin}/{user}/{repo}",
            'link': True},
        {'desc': "read/write", 'url': ssh_format},
    ]
    for curl in clone_urls:
        curl['url'] = curl['url'].format(
            origin=base, user=owner.canonical_name, repo=repo.name)
    return clone_urls
