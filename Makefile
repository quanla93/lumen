# Lumen Makefile
# Run `make help` for the list of targets.

SHELL := bash

GO            ?= go
PNPM          ?= pnpm
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS       := -s -w -X main.Version=$(VERSION)
DIST          := dist

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ============================================================
# Dev
# ============================================================

.PHONY: dev-hub
dev-hub: ## Run hub in dev mode (reads .env from CWD; see .env.example)
	$(GO) run ./cmd/lumen-hub

.PHONY: dev-agent
dev-agent: ## Run agent in dev mode (reads .env from CWD; see .env.example)
	$(GO) run ./cmd/lumen-agent

.PHONY: dev-web
dev-web: ## Run Vite dev server for the web UI
	$(PNPM) --filter web dev

.PHONY: dev-docs
dev-docs: ## Run Starlight dev server for docs
	$(PNPM) --filter docs dev

# ============================================================
# Lint & test
# ============================================================

.PHONY: lint
lint: lint-go lint-web ## Lint everything

.PHONY: lint-go
lint-go: ## Lint Go code
	golangci-lint run ./...

.PHONY: lint-web
lint-web: ## Lint web + docs
	$(PNPM) run lint
	$(PNPM) run typecheck

.PHONY: test
test: ## Run unit tests
	$(GO) test -race -coverprofile=coverage.out ./...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests (slow)
	$(GO) test -tags=e2e -race -timeout=10m ./test/e2e/...

.PHONY: benchmark
benchmark: ## Run benchmarks (RAM + throughput)
	$(GO) test -bench=. -benchmem -run=^$$ ./internal/hub/...

# ============================================================
# Build
# ============================================================

.PHONY: build
build: build-web build-hub build-agent ## Build hub + agent for current platform

.PHONY: build-web
build-web: ## Build web UI bundle and stage it for hub embed
	$(PNPM) --filter web build
	rm -rf internal/hub/web/dist
	mkdir -p internal/hub/web/dist
	cp -r web/dist/. internal/hub/web/dist/
	touch internal/hub/web/dist/.gitkeep

.PHONY: build-hub
build-hub: ## Build hub binary
	mkdir -p $(DIST)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-hub ./cmd/lumen-hub

.PHONY: build-agent
build-agent: ## Build agent binary
	mkdir -p $(DIST)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-agent ./cmd/lumen-agent

.PHONY: build-linux-amd64
build-linux-amd64: build-web
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-hub-linux-amd64 ./cmd/lumen-hub
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-agent-linux-amd64 ./cmd/lumen-agent

.PHONY: build-linux-arm64
build-linux-arm64: build-web
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-hub-linux-arm64 ./cmd/lumen-hub
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-agent-linux-arm64 ./cmd/lumen-agent

.PHONY: build-linux-armv7
build-linux-armv7: build-web
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-hub-linux-armv7 ./cmd/lumen-hub
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-agent-linux-armv7 ./cmd/lumen-agent

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-linux-armv7 ## Build for all target platforms

# ============================================================
# Docker
# ============================================================

.PHONY: docker-hub
docker-hub: ## Build hub Docker image
	docker build -f deploy/docker/Dockerfile.hub -t lumenhq/lumen-hub:$(VERSION) .

.PHONY: docker-agent
docker-agent: ## Build agent Docker image
	docker build -f deploy/docker/Dockerfile.agent -t lumenhq/lumen-agent:$(VERSION) .

# ============================================================
# Clean
# ============================================================

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(DIST) coverage.out coverage.html
	rm -rf web/dist docs/dist docs/.astro
