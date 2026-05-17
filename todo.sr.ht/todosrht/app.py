from srht.app import Flask
from srht.config import cfg
from srht.database import DbSession, db
from todosrht import urls, filters
from todosrht.graphql import Authenticity, TicketStatus, TicketResolution
from todosrht.types import User, TicketAccess, EventType

db = DbSession(cfg("todo.sr.ht", "connection-string"))
db.init()

class TodoApp(Flask):
    def __init__(self):
        super().__init__("todo.sr.ht", __name__, user_class=User)

        self.url_map.strict_slashes = False

        from todosrht.blueprints.html import html
        from todosrht.blueprints.tracker import tracker
        from todosrht.blueprints.ticket import ticket
        from todosrht.blueprints.settings import settings
        from srht.graphql import gql_blueprint

        self.register_blueprint(html)
        self.register_blueprint(tracker)
        self.register_blueprint(ticket)
        self.register_blueprint(settings)
        self.register_blueprint(gql_blueprint)

        self.add_template_filter(filters.label_badge)
        self.add_template_filter(filters.render_comment)
        self.add_template_filter(filters.render_ticket_description)
        self.add_template_filter(urls.label_add_url)
        self.add_template_filter(urls.label_edit_url)
        self.add_template_filter(urls.label_search_url)
        self.add_template_filter(urls.participant_url)
        self.add_template_filter(urls.ticket_assign_url)
        self.add_template_filter(urls.ticket_edit_url)
        self.add_template_filter(urls.ticket_delete_url)
        self.add_template_filter(urls.ticket_unassign_url)
        self.add_template_filter(urls.ticket_url)
        self.add_template_filter(urls.tracker_labels_url)
        self.add_template_filter(urls.tracker_url)
        self.add_template_filter(urls.user_url)

        @self.context_processor
        def inject():
            return {
                "Authenticity": Authenticity,
                "EventType": EventType,
                "TicketAccess": TicketAccess,
                "TicketStatus": TicketStatus,
                "TicketResolution": TicketResolution
            }

app = TodoApp()
