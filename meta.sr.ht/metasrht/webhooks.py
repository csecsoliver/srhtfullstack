from srht.config import cfg
from srht.database import DbSession, db
if not hasattr(db, "session"):
    # Initialize the database if not already configured (for running daemon)
    db = DbSession(cfg("meta.sr.ht", "connection-string"))
    import metasrht.types
    db.init()
from srht.webhook import Event
from srht.webhook.celery import CeleryWebhook, make_worker
from srht.metrics import RedisQueueCollector

webhook_broker = cfg("meta.sr.ht", "webhooks", "redis://")
worker = make_worker(broker=webhook_broker)
webhook_metrics_collector = RedisQueueCollector(webhook_broker, "srht_webhooks", "Webhook queue length")

class UserWebhook(CeleryWebhook):
    events = [
        Event("profile:update", "profile:read"),
        Event("ssh-key:add", "keys:read"),
        Event("ssh-key:remove", "keys:read"),
        Event("pgp-key:add", "keys:read"),
        Event("pgp-key:remove", "keys:read"),
    ]

def deliver_profile_update(user):
    """
    Delivers a profile update to subscribers, accounting for first-party
    clients receiving more details.
    """
    from metasrht.types import OAuthClient, OAuthToken
    event = UserWebhook.Events.profile_update
    third_party_subs = (UserWebhook.Subscription.query
        .join(OAuthToken, UserWebhook.Subscription.token_id == OAuthToken.id)
        .join(OAuthClient, OAuthToken.client_id == OAuthClient.id)
        .filter(UserWebhook.Subscription.user_id == user.id)
        .filter(not OAuthClient.preauthorized)
        .filter(UserWebhook.Subscription._events.ilike('%' + event.value + '%'))
    ).all()
    for sub in third_party_subs:
        if event in sub.events:
            UserWebhook.notify(sub, event, user.to_dict(first_party=False))

    # God this is a mess
    first_party_subs = (UserWebhook.Subscription.query
        .join(OAuthToken, UserWebhook.Subscription.token_id == OAuthToken.id)
        .filter(OAuthToken.client_id == None)
        .filter(UserWebhook.Subscription.user_id == user.id)
        .filter(UserWebhook.Subscription._events.ilike('%' + event.value + '%'))
    ).all()
    for sub in first_party_subs:
        if event in sub.events:
            UserWebhook.notify(sub, event, user.to_dict(first_party=True))

    legacy_first_party_subs = (UserWebhook.Subscription.query
        .join(OAuthToken, UserWebhook.Subscription.token_id == OAuthToken.id)
        .join(OAuthClient, OAuthToken.client_id == OAuthClient.id)
        .filter(UserWebhook.Subscription.user_id == user.id)
        .filter(OAuthClient.preauthorized)
        .filter(UserWebhook.Subscription._events.ilike('%' + event.value + '%'))
    ).all()
    for sub in legacy_first_party_subs:
        if event in sub.events:
            UserWebhook.notify(sub, event, user.to_dict(first_party=True))
