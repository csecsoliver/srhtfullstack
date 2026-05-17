from flask import Blueprint, render_template, abort, request
from srht.app import paginate_query
from srht.search import search_by
from srht.oauth import current_user
from mansrht.types import User, Wiki, Visibility

public = Blueprint('public', __name__)

@public.route("/~<username>")
@public.route("/~<username>/")
def user_index(username):
    user = User.query.filter(User.username == username).first()
    if not user:
        abort(404)
    terms = request.args.get("search")
    wikis = Wiki.query.filter(Wiki.owner_id == user.id)
    if current_user and current_user.id != user.id:
        wikis = wikis.filter(Wiki.visibility == Visibility.PUBLIC)
    elif not current_user:
        wikis = wikis.filter(Wiki.visibility == Visibility.PUBLIC)

    search_error = None
    try:
        wikis = search_by(wikis, terms, [Wiki.name])
    except ValueError as ex:
        search_error = str(ex)

    wikis = wikis.order_by(Wiki.updated.desc())
    wikis, pagination = paginate_query(wikis)

    return render_template("user.html", user=user, wikis=wikis,
        search=terms, search_error=search_error, **pagination)
