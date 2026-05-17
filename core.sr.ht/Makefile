PREFIX?=/usr/local
BINDIR?=$(PREFIX)/bin
LIBDIR?=$(PREFIX)/lib
SHAREDIR?=$(PREFIX)/share

ASSETS=$(SHAREDIR)/sourcehut

all:

install:
	install -m755 -d "$(ASSETS)/static"
	install -m644 -t "$(ASSETS)/static" static/*
	install -m755 -d "$(ASSETS)/scss"
	install -m644 -t "$(ASSETS)/scss" scss/*.scss scss/*.css
	install -m755 -d "$(ASSETS)/scss/bootstrap"
	install -m644 scss/bootstrap/LICENSE \
	       	"$(ASSETS)/scss/bootstrap/LICENSE"
	install -m755 -t "$(BINDIR)" sourcehut-builddate
	install -m755 -t "$(BINDIR)" sourcehut-buildver
	cp -r scss/bootstrap/scss "$(ASSETS)/scss/bootstrap"
	find "$(ASSETS)/scss/bootstrap" -type f -exec chmod 0644 {} +
