BINARY ?= rivulet
PKG := ./...

.PHONY: run test test-manual lint build

run:
	go run ./cmd/rivulet run --file data/workflows/n8n_workflow.json

test:
	go test $(PKG) -race -count=1

test-manual:
	go run ./cmd/rivulet run --file data/workflows/n8n_workflow.json

lint:
	@golangci-lint run ./... || echo "Install golangci-lint for linting"

build:
	go build -o bin/$(BINARY) ./cmd/rivulet
