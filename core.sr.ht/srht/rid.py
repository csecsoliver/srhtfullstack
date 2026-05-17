import base64
import uuid

_rid_alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
_std_alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

_decode_trans = str.maketrans(_rid_alphabet, _std_alphabet)
_encode_trans = str.maketrans(_std_alphabet, _rid_alphabet)

def to_rid(u: uuid.UUID) -> str:
    """
    Returns the resource-id representation of a UUID.
    """
    b32 = base64.b32encode(u.bytes).decode().rstrip("=")
    return b32.translate(_encode_trans)

def from_rid(rid: str) -> uuid.UUID:
    """
    Returns the resource-id representation of a UUID.
    """
    std = rid.translate(_decode_trans)
    data = base64.b32decode(std + "======")
    return uuid.UUID(bytes=data)
