BINARY ?= rivulet
PKG := ./...
GO ?= go

.PHONY: run test lint vet build

run:
	$(GO) run ./cmd/rivulet agent

test:
	$(GO) test $(PKG) -race -count=1

lint:
	@golangci-lint run ./... || echo "Install golangci-lint for linting"

vet:
	$(GO) vet ./...

build:
	$(GO) build -o bin/$(BINARY) ./cmd/rivulet
