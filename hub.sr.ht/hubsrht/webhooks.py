from srht.database import db
from hubsrht.types import UserWebhooks

def get_user_webhooks(user):
    """
    Ensures that the database has a user_webhooks row for this user (and returns
    it to the caller).
    """
    if user.webhooks is not None:
        return user.webhooks
    wh = UserWebhooks()
    wh.user_id = user.id
    db.session.add(wh)
    db.session.commit()
    return wh
