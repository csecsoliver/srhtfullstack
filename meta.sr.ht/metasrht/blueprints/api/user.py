from flask import Blueprint
from srht.oauth import oauth, current_token
from metasrht.webhooks import UserWebhook

user = Blueprint('api_user', __name__)

@user.route("/api/user/profile")
@oauth("profile:read")
def user_profile_GET():
    return current_token.user.to_dict(first_party=current_token.first_party)

UserWebhook.api_routes(blueprint=user, prefix="/api/user")
