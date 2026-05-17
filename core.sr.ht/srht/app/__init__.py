import locale
from srht.app.datetime import *
from srht.app.cors import *
from srht.app.csrf import *
from srht.app.session import *
from srht.app.pagination import *
from srht.app.profile import *
from srht.app.icons import *
from srht.app.flask import Flask

try:
    locale.setlocale(locale.LC_ALL, 'en_US')
except:
    pass
