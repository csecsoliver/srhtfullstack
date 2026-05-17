import base64
import hashlib
import hmac
import struct
import time

def totp(secret, token):
    tm = int(time.time() / 30)
    key = base64.b32decode(secret)

    for ix in range(-2, 3):
        b = struct.pack(">q", tm + ix)
        hm = hmac.HMAC(key, b, hashlib.sha1).digest()
        offset = hm[-1] & 0x0F
        truncatedHash = hm[offset:offset + 4]
        code = struct.unpack(">L", truncatedHash)[0]
        code &= 0x7FFFFFFF
        code %= 1000000
        if token == code:
            return True

    return False
