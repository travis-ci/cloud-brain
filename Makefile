PACKAGES := \
	github.com/travis-ci/cloud-brain/background \
	github.com/travis-ci/cloud-brain/cbcontext \
	github.com/travis-ci/cloud-brain/cloud \
	github.com/travis-ci/cloud-brain/cloudbrain \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-create-token \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-create-worker \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-http \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-refresh-worker \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-remove-worker \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-show-provider \
	github.com/travis-ci/cloud-brain/database \
	github.com/travis-ci/cloud-brain/http

VERSION_VAR := github.com/travis-ci/cloud-brain/cloudbrain.VersionString
VERSION_VALUE ?= $(shell git describe --always --dirty --tags 2>/dev/null)
REV_VAR := github.com/travis-ci/cloud-brain/cloudbrain.RevisionString
REV_VALUE ?= $(shell git rev-parse HEAD 2>/dev/null || echo "???")
REV_URL_VAR := github.com/travis-ci/cloud-brain/cloudbrain.RevisionURLString
REV_URL_VALUE ?= https://github.com/travis-ci/cloud-brain/tree/$(REV_VALUE)
GENERATED_VAR := github.com/travis-ci/cloud-brain/cloudbrain.GeneratedString
GENERATED_VALUE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%S%z')
COPYRIGHT_VAR := github.com/travis-ci/cloud-brain/cloudbrain.CopyrightString
COPYRIGHT_VALUE ?= $(shell grep -i ^copyright LICENSE | sed 's/^[Cc]opyright //')

GOPATH := $(shell echo "$${GOPATH%%:*}")
GOBUILD_LDFLAGS ?= \
	-X '$(VERSION_VAR)=$(VERSION_VALUE)' \
	-X '$(REV_VAR)=$(REV_VALUE)' \
	-X '$(REV_URL_VAR)=$(REV_URL_VALUE)' \
	-X '$(GENERATED_VAR)=$(GENERATED_VALUE)' \
	-X '$(COPYRIGHT_VAR)=$(COPYRIGHT_VALUE)'

BINARY_NAMES := $(notdir $(wildcard cmd/*))
BINARIES := $(addprefix bin/,$(BINARY_NAMES))

.PHONY: test
test: deps
	go test -ldflags "$(GOBUILD_LDFLAGS)" $(PACKAGES)

.PHONY: list-deps
list-deps:
	go list -f '{{ join .Imports "\n" }}' $(PACKAGES) | sort | uniq

.PHONY: lint
lint:
	gometalinter --deadline=1m -Dstructcheck -Derrcheck -Dgotype -s vendor ./...

.PHONY: heroku
heroku:
	make -C $(GOPATH)/src/github.com/travis-ci/cloud-brain bin
	mkdir -p bin/
	cp $(GOPATH)/src/github.com/travis-ci/cloud-brain/bin/* bin/

.PHONY: bin
bin: deps $(BINARIES)

.PHONY: clean
clean:
	$(RM) $(BINARIES)

.PHONY: distclean
distclean: clean
	$(RM) vendor/.deps-fetched

deps: $(GOPATH)/bin/gvt vendor/.deps-fetched

$(GOPATH)/bin/gvt:
	go get github.com/FiloSottile/gvt

vendor/.deps-fetched:
	gvt rebuild
	touch $@

bin/%: cmd/% $(wildcard **/*.go)
	go build -ldflags "$(GOBUILD_LDFLAGS)" -o $@ ./$<
