# Local entry points for the run view. CI runs `lint`, `test` and `package`, one
# step per target.
#
# The browser shell is plain ES modules with no build step, so its entry points
# are a syntax check and the Node test runner; both are part of `lint` and
# `test`. `package` is the build entry: the only way an installable artifact is
# produced. Nothing here publishes.

SHELL := /bin/sh
GO ?= go
NODE ?= node
ARTIFACT_DIR ?= dist

.PHONY: all build package verify fmt-check vet js-check lint test js-test acceptance clean

all: lint test

build:
	$(GO) build ./...

# package writes the extension package kos-sdk produces and its digest sidecar.
package:
	$(GO) run ./cmd/package -output $(ARTIFACT_DIR)

verify: fmt-check vet js-check

fmt-check:
	@unformatted=$$(gofmt -l $$(git ls-files '*.go')); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt required for:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	$(GO) vet ./...

js-check:
	@for file in $$(git ls-files '*.js'); do $(NODE) --check "$$file" || exit 1; done

lint: verify

test: js-test
	$(GO) test ./...

js-test:
	$(NODE) --test tests/shell/*.test.js tests/acceptance/*/*.test.js

acceptance:
	$(GO) test ./tests/... -count=1 -v

clean:
	rm -rf $(ARTIFACT_DIR)
