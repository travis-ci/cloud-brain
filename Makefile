PACKAGES := \
	github.com/travis-ci/cloud-brain/cbcontext \
	github.com/travis-ci/cloud-brain/cloud \
	github.com/travis-ci/cloud-brain/cloudbrain \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-create-token \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-create-worker \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-http \
	github.com/travis-ci/cloud-brain/cmd/cloudbrain-refresh-worker \
	github.com/travis-ci/cloud-brain/database \
	github.com/travis-ci/cloud-brain/http \
	github.com/travis-ci/cloud-brain/worker

.PHONY: test
test: deps
	go test $(PACKAGES)

deps: vendor/.deps-fetched

vendor/.deps-fetched:
	gvt rebuild
	touch $@

.PHONY: list-deps
list-deps:
	go list -f '{{ join .Imports "\n" }}' $(PACKAGES) | sort | uniq
