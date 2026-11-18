GO ?= $(HOME)/sdk/go1.27.3/bin/go

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o todo ./cmd/todo

test:
	$(GO) vet ./... && $(GO) test ./...

.PHONY: build test
