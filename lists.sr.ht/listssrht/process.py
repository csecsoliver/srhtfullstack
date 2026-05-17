from srht.config import cfg, cfgi
from srht.database import DbSession, db
if not hasattr(db, "session"):
    db = DbSession(cfg("lists.sr.ht", "connection-string"))
    db.init()
from srht.email import start_smtp
from listssrht.types import Email

import email
import email.policy
from celery import Celery
from sqlalchemy import or_
from srht.email import mail_exception
from urllib.parse import quote

dispatch = Celery("lists.sr.ht", broker=cfg("lists.sr.ht", "redis"))

smtp_host = cfg("mail", "smtp-host", default=None)
smtp_port = cfgi("mail", "smtp-port", default=None)
smtp_user = cfg("mail", "smtp-user", default=None)
smtp_password = cfg("mail", "smtp-password", default=None)

policy = email.policy.SMTPUTF8.clone(max_line_length=998)

def task(func):
    def wrapper(*args, **kwargs):
        try:
            return func(*args, **kwargs)
        except Exception as ex:
            mail_exception(ex, context="lists.sr.ht-process")
            try:
                db.session.rollback()
            except:
                pass
            return
    wrapper.__name__ = func.__name__
    return dispatch.task(wrapper)

def _prep_mail(dest, mail):
    domain = cfg("lists.sr.ht", "posting-domain")
    list_name = "{}/{}".format(dest.owner.canonical_name, dest.name)
    archive_url = "{}/{}".format(cfg("lists.sr.ht", "origin"), list_name)
    list_unsubscribe = list_name + "+unsubscribe@" + domain
    list_subscribe = list_name + "+subscribe@" + domain
    for overwrite in ["List-Unsubscribe", "List-Subscribe", "List-Archive",
                "List-Post", "List-ID", "Sender"]:
        del mail[overwrite]

    mail["List-Unsubscribe"] = (
            "<mailto:{}?subject=unsubscribe>".format(list_unsubscribe))
    mail["List-Subscribe"] = (
            "<mailto:{}?subject=subscribe>".format(list_subscribe))
    mail["List-Archive"] = "<{}>".format(archive_url)
    mail["Archived-At"] = "<{}/{}>".format(archive_url, quote(mail["Message-ID"]))
    mail["List-Post"] = "<mailto:{}@{}>".format(list_name, domain)
    mail["List-ID"] = "{} <{}.{}>".format(list_name, list_name, domain)
    mail["Sender"] = "{} <{}@{}>".format(list_name, list_name, domain)
    return mail

@task
def forward_thread(list_id, thread_id, recipient):
    thread = (Email.query
            .filter(or_(Email.thread_id == thread_id, Email.id == thread_id))
            .filter(Email.list_id == list_id)
            .order_by(Email.id)).all()
    if not thread:
        return
    dest = thread[0].list

    smtp = start_smtp()
    for message in thread:
        mail = email.message_from_bytes(message.raw_message, policy=policy)
        mail = _prep_mail(dest, mail)
        try:
            smtp.send_message(mail, smtp_user, [recipient])
        except Exception as ex:
            print(ex)
            print("(continuing)")
            continue

    smtp.quit()
