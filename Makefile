BINARY ?= rivulet
BACKEND := ./apps/backend
PKG := $(BACKEND)/...

.PHONY: run api test test-manual lint build daemon-build api-build

run:
	go run $(BACKEND)/cmd/rivulet server

api:
	go run $(BACKEND)/cmd/api

test:
	go test $(PKG) -race -count=1

test-manual:
	go run $(BACKEND)/cmd/rivulet run --file data/workflows/n8n_workflow.json

lint:
	@golangci-lint run ./apps/backend/... || echo "Install golangci-lint for linting"

build:
	go build -o bin/$(BINARY) $(BACKEND)/cmd/rivulet

daemon-build:
	go build -o bin/flowd $(BACKEND)/cmd/flowd

api-build:
	go build -o bin/rivulet-api $(BACKEND)/cmd/api
