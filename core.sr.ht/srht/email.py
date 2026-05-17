from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from email.message import Message
from email.utils import formataddr, formatdate, make_msgid, parseaddr
from flask import request, has_request_context, has_app_context, current_app
from srht.crypto import encrypt_request_authorization
from srht.config import cfg, cfgi
import base64
import smtplib
import traceback

smtp_host = cfg("mail", "smtp-host", default=None)
smtp_port = cfgi("mail", "smtp-port", default=None)
smtp_user = cfg("mail", "smtp-user", default=None)
smtp_password = cfg("mail", "smtp-password", default=None)
smtp_from = cfg("mail", "smtp-from", default=None)
smtp_encryption = cfg("mail", "smtp-encryption", default=None)
error_to = cfg("mail", "error-to", default=None)
error_from = cfg("mail", "error-from", default=None)

def format_headers(**headers):
    headers['From'] = formataddr(parseaddr(headers['From']))
    headers['To'] = formataddr(parseaddr(headers['To']))
    if 'Reply-To' in headers:
        headers['Reply-To'] = formataddr(parseaddr(headers['Reply-To']))
    return headers

def prepare_email(body, to, subject, **headers):
    headers['Subject'] = subject
    headers.setdefault('From', smtp_from or smtp_user)
    headers.setdefault('To', to)
    headers.setdefault('Date', formatdate())
    headers.setdefault('Message-ID', make_msgid())
    headers = format_headers(**headers)

    text_part = MIMEText(body)
    multipart = MIMEMultipart()
    multipart.attach(text_part)

    for key in headers:
        multipart[key] = headers[key]
    return multipart

def start_smtp():
    if smtp_encryption == 'tls':
        smtp = smtplib.SMTP_SSL(smtp_host, smtp_port)
    else:
        smtp = smtplib.SMTP(smtp_host, smtp_port)
    smtp.ehlo()
    if smtp_encryption == 'starttls':
        smtp.starttls()
    if smtp_user and smtp_password:
        smtp.login(smtp_user, smtp_password)
    return smtp


def send_email(body, to, subject, **headers):
    message = prepare_email(body, to, subject, **headers)
    if not smtp_host:
        print("Not configured to send email. The email we tried to send was:")
        print(message)
        return
    smtp = start_smtp()
    smtp.send_message(message, smtp_from, [to])
    smtp.quit()

def mail_exception(ex, user=None, context=None):
    if not error_to or not error_from:
        print("Warning: no email configured for error emails")
        return
    if has_app_context() and has_request_context():
        data = request.get_data() or b"(no request body)"
    else:
        data = b"(no request body)"
    try:
        data = data.decode()
    except:
        data = base64.b64encode(data)
    if "password" in data:
        data = "(request body contains password)"
    if has_app_context() and has_request_context():
        headers = "\n".join(
            key + ": " + value for key, value in request.headers.items())
        body = f"""
Exception occured on {request.method} {request.url}

{traceback.format_exc()}

Request body:

{data}

Request headers:

{headers}

Current user:

{user}"""
    else:
        body = f"""
{traceback.format_exc()}"""
    if context:
        subject = f"[{context}] {ex.__class__.__name__}"
    elif has_app_context():
        subject = (f"[{current_app.site}] {ex.__class__.__name__} on " +
            f"{request.method} {request.url}")
    else:
        subject = f"{ex.__class__.__name__}"
    send_email(body, error_to, subject, **{"From": error_from})
