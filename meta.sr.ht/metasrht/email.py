from metasrht.graphql import Client
from srht.crypto import internal_anon
from srht.graphql import InternalAuth

def send_email(address, msg):
    client = Client(InternalAuth.anonymous())
    client.send_email(address, msg)
