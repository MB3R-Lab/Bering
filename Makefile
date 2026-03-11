SHELL := /bin/sh

GO ?= go
GORELEASER ?= goreleaser
IMAGE ?= bering:dev
DIST_DIR ?= dist
VERSION ?= 0.0.0-dev
CHART_VERSION ?= $(VERSION)
BUILD_DATE ?= $(shell git show -s --format=%cI HEAD 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_SHA ?= $(shell git rev-parse --verify HEAD 2>/dev/null || echo unknown)
GIT_TAG ?= v$(VERSION)
IMAGE_REPOSITORY ?= ghcr.io/mb3r-lab/bering
CHART_OCI_REPOSITORY ?= oci://ghcr.io/mb3r-lab/charts
PUBLISH_OCI ?= 0
ALLOW_CHART_VERSION_MISMATCH ?= 0

BOOL_TRUE := 1 true TRUE yes YES

.PHONY: help lint test build run-checks docker-build goreleaser-release contracts-pack chart-package oci-image release-manifest validate-release release-dry-run release-local clean

help:
	@echo "Targets:"
	@echo "  lint              Run gofmt and go vet"
	@echo "  test              Run unit and integration tests"
	@echo "  build             Build the local bering binary"
	@echo "  run-checks        Run lint + test + build"
	@echo "  release-dry-run   Build the release payload locally without publishing"
	@echo "  release-local     Build the canonical release payload and optionally publish OCI artifacts"
	@echo "  chart-package     Package the Helm chart and optionally publish it to an OCI registry"
	@echo "  release-manifest  Generate release-manifest.json and supporting metadata"
	@echo "  validate-release  Validate generated release metadata"
	@echo "  docker-build      Build the local CLI image"
	@echo "  clean             Remove generated binaries and release artifacts"

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
	$(GO) build -trimpath -o bin/bering ./cmd/bering

run-checks: lint test build

docker-build:
	docker build -f build/Dockerfile -t $(IMAGE) .

goreleaser-release:
	rm -rf $(DIST_DIR)
	RELEASE_VERSION=$(VERSION) $(GORELEASER) release --snapshot --clean --skip=publish

contracts-pack:
	$(GO) run ./cmd/releasectl contracts-pack --repo-root . --dist-dir $(DIST_DIR) --app-version $(VERSION) --build-date $(BUILD_DATE)

chart-package:
	$(GO) run ./cmd/releasectl chart-package --repo-root . --dist-dir $(DIST_DIR) --chart-dir charts/bering --app-version $(VERSION) --chart-version $(CHART_VERSION) --oci-repository $(CHART_OCI_REPOSITORY) $(if $(filter $(BOOL_TRUE),$(PUBLISH_OCI)),--publish,) $(if $(filter $(BOOL_TRUE),$(ALLOW_CHART_VERSION_MISMATCH)),--allow-chart-version-mismatch,)

oci-image:
	$(GO) run ./cmd/releasectl oci-image --repo-root . --dist-dir $(DIST_DIR) --dockerfile build/Dockerfile --image-repository $(IMAGE_REPOSITORY) --app-version $(VERSION) --git-commit $(GIT_SHA) --build-date $(BUILD_DATE) $(if $(filter $(BOOL_TRUE),$(PUBLISH_OCI)),--publish,)

release-manifest:
	$(GO) run ./cmd/releasectl release-manifest --repo-root . --dist-dir $(DIST_DIR) --app-version $(VERSION) --git-commit $(GIT_SHA) --git-tag $(GIT_TAG) --build-date $(BUILD_DATE)

validate-release:
	$(GO) run ./cmd/releasectl validate --repo-root . --dist-dir $(DIST_DIR) --app-version $(VERSION) --build-date $(BUILD_DATE) $(if $(filter $(BOOL_TRUE),$(PUBLISH_OCI)),--require-published-oci,) $(if $(filter $(BOOL_TRUE),$(ALLOW_CHART_VERSION_MISMATCH)),--allow-chart-version-mismatch,)

release-dry-run: lint test goreleaser-release contracts-pack chart-package oci-image release-manifest validate-release

release-local: test goreleaser-release contracts-pack chart-package oci-image release-manifest validate-release

clean:
	rm -rf bin out dist
