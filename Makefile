BINARY ?= rivulet
PKG := ./...
GO ?= go

.PHONY: run examples test lint vet build agent

run:
	$(GO) run ./cmd/rivulet run --file examples/hello.dify.yml --input name=world

examples:
	@for file in examples/*.dify.yml; do \
		echo "== $$file"; \
		$(GO) run ./cmd/rivulet validate --file "$$file" || exit 1; \
	done

agent:
	$(GO) run ./cmd/rivulet agent

test:
	$(GO) test $(PKG) -race -count=1

lint:
	@golangci-lint run ./... || echo "Install golangci-lint for linting"

vet:
	$(GO) vet ./...

build:
	$(GO) build -o bin/$(BINARY) ./cmd/rivulet
