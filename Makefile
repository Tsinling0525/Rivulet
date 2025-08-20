BINARY ?= rivulet
PKG := ./...

.PHONY: run api test lint build

run:
	go run cmd/flowd/main.go

api:
	go run cmd/api/main.go

test:
	go test $(PKG) -race -count=1

lint:
	@golangci-lint run || echo "Install golangci-lint for linting"

build:
	go build -o bin/$(BINARY) cmd/flowd/main.go


