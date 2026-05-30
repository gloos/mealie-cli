BINARY  := mealie
PKG     := github.com/gloos/mealie-cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/internal/buildinfo.Version=$(VERSION) \
	-X $(PKG)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(PKG)/internal/buildinfo.Date=$(DATE)

SPEC := api/specs/mealie/v3.19.2/openapi.json

.PHONY: all build install test cover vet fmt fmt-check lint tidy clean snapshot release-check spec

all: fmt-check vet test build

build: ## Build the binary into bin/
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/mealie

install: ## Install the binary into GOBIN
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/mealie

test: ## Run all tests
	go test ./...

cover: ## Run tests with coverage
	go test -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -func=coverage.txt | tail -1

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go files
	gofmt -w .

fmt-check: ## Fail if any file is not gofmt-clean
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

tidy: ## Tidy go.mod/go.sum
	go mod tidy

clean: ## Remove build artefacts
	rm -rf bin dist coverage.txt coverage.html

snapshot: ## Build a local snapshot release with GoReleaser
	goreleaser release --snapshot --clean

release-check: ## Validate the GoReleaser configuration
	goreleaser check

spec: ## Refresh the pinned Mealie OpenAPI spec from a Docker container
	docker rm -f mealie-spec >/dev/null 2>&1 || true
	docker run -d --name mealie-spec -p 9925:9000 ghcr.io/mealie-recipes/mealie:v3.19.2 >/dev/null
	@echo "waiting for Mealie to start..."; \
	for i in $$(seq 1 90); do \
		if curl -fsS http://localhost:9925/openapi.json -o $(SPEC) 2>/dev/null; then echo "spec saved to $(SPEC)"; break; fi; \
		sleep 2; \
	done; \
	docker rm -f mealie-spec >/dev/null 2>&1 || true
