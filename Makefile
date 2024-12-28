# Keep VERSION for manual releases
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -X github.com/gnomegl/gitslurp/internal/utils.Version=$(VERSION)

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" .

.PHONY: install
install:
	go install -ldflags "$(LDFLAGS)" .

.PHONY: release
release:
	git tag -a v$(VERSION) -m "Release v$(VERSION)"
	git push origin v$(VERSION)