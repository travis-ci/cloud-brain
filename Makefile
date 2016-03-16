PACKAGES := \
	github.com/travis-ci/cloud-brain/background \
	github.com/travis-ci/cloud-brain/cbcontext \
	github.com/travis-ci/cloud-brain/cloud \
	github.com/travis-ci/cloud-brain/cloudbrain \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-create-token \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-create-worker \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-http \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-refresh-worker \
	github.com/travis-ci/cloud-brain/database \
	github.com/travis-ci/cloud-brain/http

BINARY_NAMES := $(notdir $(wildcard cmd/*))
BINARIES := $(addprefix bin/,$(BINARY_NAMES))

.PHONY: test
test: deps
	go test $(PACKAGES)

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

deps: $(GOPATH)/bin/gvt vendor/.deps-fetched

$(GOPATH)/bin/gvt:
	go get github.com/FiloSottile/gvt

vendor/.deps-fetched:
	gvt rebuild
	touch $@

bin/%: cmd/% $(wildcard **/*.go)
	go build -o $@ ./$<
