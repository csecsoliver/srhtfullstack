PREFIX?=/usr/local
BINDIR?=$(PREFIX)/bin
LIBDIR?=$(PREFIX)/lib
SHAREDIR?=$(PREFIX)/share

ASSETS=$(SHAREDIR)/sourcehut

SERVICE=hub.sr.ht
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

BUILDS_GRAPHQL_QUERIES != echo hubsrht/services/builds/*.graphql
GIT_GRAPHQL_QUERIES != echo hubsrht/services/git/*.graphql
HG_GRAPHQL_QUERIES != echo hubsrht/services/hg/*.graphql
LISTS_GRAPHQL_QUERIES != echo hubsrht/services/lists/*.graphql
TODO_GRAPHQL_QUERIES != echo hubsrht/services/todo/*.graphql

ariadne/%.toml: ariadne/%.toml.in
	sed -e 's:@ASSETS@:$(ASSETS):g' < $< > $@

hubsrht/services/builds/__init__.py: $(BUILDS_GRAPHQL_QUERIES) ariadne/builds.toml
	$(ARIADNE_CODEGEN) --config ariadne/builds.toml

hubsrht/services/git/__init__.py: $(GIT_GRAPHQL_QUERIES) ariadne/git.toml
	$(ARIADNE_CODEGEN) --config ariadne/git.toml

hubsrht/services/hg/__init__.py: $(HG_GRAPHQL_QUERIES) ariadne/hg.toml
	$(ARIADNE_CODEGEN) --config ariadne/hg.toml

hubsrht/services/lists/__init__.py: $(LISTS_GRAPHQL_QUERIES) ariadne/lists.toml
	$(ARIADNE_CODEGEN) --config ariadne/lists.toml

hubsrht/services/todo/__init__.py: $(TODO_GRAPHQL_QUERIES) ariadne/todo.toml
	$(ARIADNE_CODEGEN) --config ariadne/todo.toml

all-python: hubsrht/services/builds/__init__.py
all-python: hubsrht/services/git/__init__.py
all-python: hubsrht/services/hg/__init__.py
all-python: hubsrht/services/lists/__init__.py
all-python: hubsrht/services/todo/__init__.py

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
	./scripts/clean-python builds
	./scripts/clean-python git
	./scripts/clean-python hg
	./scripts/clean-python lists
	./scripts/clean-python todo
	# TODO: Replace the script with the following once legacy webhook
	# support is removed:
	# rm -rf hubsrht/services/builds/*.py hubsrht/services/builds/__pycache__
	# rm -rf hubsrht/services/git/*.py hubsrht/services/git/__pycache__
	# rm -rf hubsrht/services/hg/*.py hubsrht/services/hg/__pycache__
	# rm -rf hubsrht/services/lists/*.py hubsrht/services/lists/__pycache__
	# rm -rf hubsrht/services/todo/*.py hubsrht/services/todo/__pycache__
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

api/loaders/*_gen.go &: api/loaders/generate.go api/loaders/gen go.sum
	cd api && go generate ./loaders

api/graph/api/generated.go: api/graph/schema.graphqls api/graph/generate.go go.sum api/loaders/*_gen.go
	cd api && go generate ./graph

$(SERVICE)-api: api/graph/api/generated.go
	go build -o $@ $(GO_LDFLAGS) ./api

# Always rebuild
.PHONY: $(BINARIES)
