PREFIX?=/usr/local
BINDIR?=$(PREFIX)/bin
LIBDIR?=$(PREFIX)/lib
SHAREDIR?=$(PREFIX)/share

ASSETS=$(SHAREDIR)/sourcehut

SERVICE=man.sr.ht
STATICDIR=$(ASSETS)/static/$(SERVICE)
MIGRATIONDIR=$(ASSETS)/migrations/$(SERVICE)

SASSC?=sassc
SASSC_INCLUDE=-I$(ASSETS)/scss/

ARIADNE_CODEGEN=ariadne-codegen

BINARIES=\
	$(SERVICE)-api

GO_LDFLAGS += -ldflags " \
              -X git.sr.ht/~sircmpwn/core-go/server.BuildVersion=$(shell sourcehut-buildver) \
              -X git.sr.ht/~sircmpwn/core-go/server.BuildDate=$(shell sourcehut-builddate)"

all: all-bin all-share all-python

install: install-bin install-share

clean: clean-bin clean-share clean-python

all-bin: $(BINARIES)

all-share: static/main.min.css

GIT_GRAPHQL_QUERIES != echo mansrht/git/*.graphql

ariadne/git.toml: ariadne/git.toml.in
	sed \
		-e 's:@ASSETS@:$(ASSETS):g' \
		< $< > $@

mansrht/git/__init__.py: ariadne/git.toml $(GIT_GRAPHQL_QUERIES)
	$(ARIADNE_CODEGEN) --config ariadne/git.toml

all-python: mansrht/git/__init__.py

install-bin: all-bin
	mkdir -p $(BINDIR)
	for bin in $(BINARIES); \
	do \
		install -Dm755 $$bin $(BINDIR)/; \
	done

install-share: all-share
	mkdir -p $(STATICDIR)
	mkdir -p $(MIGRATIONDIR)
	install -Dm644 static/*.css $(STATICDIR)
	install -Dm644 api/graph/schema.graphqls $(ASSETS)/$(SERVICE).graphqls
	install -Dm644 schema.sql $(ASSETS)/$(SERVICE).sql
	install -Dm644 migrations/*.sql $(MIGRATIONDIR)

clean-bin:
	rm -f $(BINARIES)

clean-share:
	rm -f static/main.min.css static/main.css

clean-python:
	rm -rf mansrht/git/*.py mansrht/git/__pycache__
	rm -f ariadne/*.toml

.PHONY: all all-bin all-share all-python
.PHONY: install install-bin install-share
.PHONY: clean clean-bin clean-share clean-python

static/main.css: scss/main.scss
	mkdir -p $(@D)
	$(SASSC) $(SASSC_INCLUDE) $< $@

static/main.min.css: static/main.css
	minify -o $@ $<
	cp $@ $(@D)/main.min.$$(sha256sum $@ | cut -c1-8).css

api/graph/api/generated.go: api/graph/schema.graphqls api/graph/generate.go go.sum
	cd api && go generate ./graph

$(SERVICE)-api: api/graph/api/generated.go
	go build -o $@ $(GO_LDFLAGS) ./cmd/api

# Always rebuild
.PHONY: $(BINARIES)
