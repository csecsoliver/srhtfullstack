from datetime import datetime

DATE_FORMAT = "%Y-%m-%dT%H:%M:%SZ"

def gql_time(time):
    """
    Parses a timestamp from a GraphQL response.
    """
    # Python's strptime does not support nanoseconds, so that's cool.
    if "." in time:
        nanos = time.rindex(".")
        time = time[:nanos] + "Z"
    return datetime.strptime(time, DATE_FORMAT)

