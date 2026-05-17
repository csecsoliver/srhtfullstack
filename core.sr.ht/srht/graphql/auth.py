from abc import ABC, abstractmethod
from srht.crypto import encrypt_request_authorization, internal_anon

class ClientAuth(ABC):
    @property
    @abstractmethod
    def headers(self):
        pass

class InternalAuth(ClientAuth):
    """
    Authenticates a GraphQL client with internal authentication.
    """

    def __init__(self, user=None, client_id=None):
        self._headers = encrypt_request_authorization(user, client_id)

    @property
    def headers(self):
        return self._headers

    @staticmethod
    def anonymous():
        """
        Authenticates with a resolver that supports @anoninternal.
        """
        return InternalAuth(user=internal_anon)

class BearerAuth(ClientAuth):
    """
    Authenticates a GraphQL client with an OAuth2 Bearer token.
    """

    def __init__(self, oauth2_token):
        self._oauth2_token = oauth2_token

    @property
    def headers(self):
        return {
            "Authorization": f"Bearer {self._oauth2_token}"
        }
