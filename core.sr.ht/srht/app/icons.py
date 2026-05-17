from flask import g
from importlib import resources
from markupsafe import Markup

_icon_cache = {}

def icon(i, cls=""):
    if i in _icon_cache:
        svg = _icon_cache[i]
        return Markup(f'<span class="icon icon-{i} {cls}" aria-hidden="true">{svg}</span>')
    fa_license = """<!--
        Font Awesome Free 5.3.1 by @fontawesome - https://fontawesome.com
        License - https://fontawesome.com/license/free (Icons: CC BY 4.0, Fonts: SIL OFL 1.1, Code: MIT License)
    -->"""
    # TODO once we can assume Python >= 3.13, use resources.read_text()
    svg = resources.files("srht").joinpath('icons', i + '.svg').read_text()
    _icon_cache[i] = svg
    if g and "fa_license" not in g:
        svg += fa_license
        g.fa_license = True
    return Markup(f'<span class="icon icon-{i} {cls}" aria-hidden="true">{svg}</span>')
