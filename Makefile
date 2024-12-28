VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -X main.version=$(VERSION)

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