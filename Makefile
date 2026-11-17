GO ?= $(HOME)/sdk/go1.27.3/bin/go

build:
	$(GO) build -o todo ./cmd/todo

test:
	$(GO) vet ./... && $(GO) test ./...

.PHONY: build test
