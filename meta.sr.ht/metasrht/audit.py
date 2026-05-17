from datetime import datetime, timedelta
from flask import request
from ipaddress import ip_address
from metasrht.email import send_email
from metasrht.types import AuditLogEntry
from srht.config import cfg
from srht.database import db
from srht.oauth import current_user
from string import Template

owner_name = cfg("sr.ht", "owner-name")
site_name = cfg("sr.ht", "site-name")

def audit_log(event_type, details=None, user=None,
        email=False, subject=None, email_details=None):
    if not user:
        user = current_user
    if not user:
        return
    addr = request.access_route[-1]
    event = AuditLogEntry(user.id, event_type, ip_address(addr), details)
    db.session.add(event)
    if email:
        tmpl = Template("""Subject: $subject
Reply-To: $reply_to

~$username,

This email was sent to inform you that the following security-sensitive
event has taken place for your account on $site_name:

$email_details

If you did not expect this to occur, please reply to this email urgently
to contact support. Otherwise, no action is required.

-- 
$owner_name
""")
        reply_to =f"{cfg('sr.ht', 'owner-name')} <{cfg('sr.ht', 'owner-email')}>"
        reply_to = cfg("sr.ht", "security-address", default=reply_to)
        rendered = tmpl.substitute(**{
                'subject': subject,
                'reply_to': reply_to,
                'username': user.username,
                'site_name': site_name,
                'email_details': email_details,
                'owner_name': owner_name
            })
        send_email(user.email, rendered)

def expire_audit_logs():
    cutoff = datetime.now() - timedelta(days=14)
    AuditLogEntry.query.filter(AuditLogEntry.created <= cutoff) .delete()
    db.session.commit()
