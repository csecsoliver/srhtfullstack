from datetime import datetime, timedelta
from markupsafe import Markup
import humanize

DATE_FORMAT = "%Y-%m-%dT%H:%M:%S+00:00"

humanize.time._now = lambda: datetime.utcnow()

def date_handler(obj):
    if hasattr(obj, 'strftime'):
        return obj.strftime(DATE_FORMAT)
    if isinstance(obj, decimal.Decimal):
        return "{:.2f}".format(obj)
    if isinstance(obj, Enum):
        return obj.name
    return obj

def datef(d):
    if not d:
        return 'Never'
    if isinstance(d, timedelta):
        return Markup('<span title="{}">{}</span>'.format(
            f'{d.seconds} seconds', humanize.naturaldelta(d)))
    return Markup('<span title="{}">{}</span>'.format(
        d.strftime('%Y-%m-%d %H:%M:%S UTC'),
        humanize.naturaltime(d)))
