from datetime import datetime
from prometheus_client import Histogram
from sqlalchemy import create_engine, event
from sqlalchemy.orm import scoped_session, sessionmaker, declarative_base
from flask import request
from timeit import default_timer
from werkzeug.local import LocalProxy

Base = declarative_base()

_db = None
db = LocalProxy(lambda: _db)

_metrics = type("metrics", tuple(), {
    m.describe()[0].name: m
    for m in [
        Histogram("sql_query_duration", "Duration of SQL queries", ('route',)),
    ]
})

class DbSession():
    def __init__(self, connection_string, assign_global=True):
        global Base, _db
        self.engine = create_engine(connection_string, future=True)
        self.session = scoped_session(sessionmaker(
            autocommit=False,
            autoflush=False,
            bind=self.engine))
        Base.query = self.session.query_property()
        if assign_global:
            _db = self

    def init(self):
        @event.listens_for(Base, 'before_insert', propagate=True)
        def before_insert(mapper, connection, target):
            if hasattr(target, '_no_autoupdate'):
                return
            if hasattr(target, 'created'):
                target.created = datetime.utcnow()
            if hasattr(target, 'updated'):
                target.updated = datetime.utcnow()

        @event.listens_for(Base, 'before_update', propagate=True)
        def before_update(mapper, connection, target):
            if hasattr(target, '_no_autoupdate'):
                return
            if hasattr(target, 'updated'):
                target.updated = datetime.utcnow()

        @event.listens_for(self.engine, 'before_cursor_execute')
        def before_cursor_execute(conn, cursor, statement,
                    parameters, context, executemany):
            self._execute_start_time = default_timer()

        @event.listens_for(self.engine, 'after_cursor_execute')
        def after_cursor_execute(conn, cursor, statement,
                    parameters, context, executemany):
            route = ""
            if request:
                route = request.endpoint
            _metrics.sql_query_duration.labels(route=route).observe(
                max(default_timer() - self._execute_start_time, 0))

    def create(self):
        Base.metadata.create_all(bind=self.engine)
