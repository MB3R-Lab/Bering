SHELL := /bin/sh

GO ?= go
IMAGE ?= bering:dev

.PHONY: help lint test build run-checks docker-build clean

help:
	@echo "Targets:"
	@echo "  lint        Run gofmt and go vet"
	@echo "  test        Run unit and integration tests"
	@echo "  build       Build bering binary"
	@echo "  run-checks  Run lint + test + build"
	@echo "  docker-build Build CLI image"
	@echo "  clean       Remove generated binaries"

lint:
	@fmt_out="$$(gofmt -l .)"; \
	if [ -n "$$fmt_out" ]; then \
		echo "gofmt required for:"; \
		echo "$$fmt_out"; \
		exit 1; \
	fi
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	mkdir -p bin
	$(GO) build -o bin/bering ./cmd/bering

run-checks: lint test build

docker-build:
	docker build -f build/Dockerfile -t $(IMAGE) .

clean:
	rm -rf bin out dist

