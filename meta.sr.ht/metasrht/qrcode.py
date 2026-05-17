import base64
import io
import qrcode

def gen_qr(data):
    img = qrcode.make(data, border=3, box_size=5)
    arr = io.BytesIO()
    img.save(arr, format='PNG')
    encoded = base64.b64encode(arr.getvalue()).decode('utf-8')
    return 'data:image/png;base64,{}'.format(encoded)
