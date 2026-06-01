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
build-all: build-linux-amd64 build-linux-arm64 ## Build for all default targets (amd64 + arm64). armv7 still buildable via `make build-linux-armv7`.

.PHONY: release-agents
release-agents: ## Cross-build agent binaries + stage install.sh for hub /install endpoint
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-agent-linux-amd64 ./cmd/lumen-agent
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/lumen-agent-linux-arm64 ./cmd/lumen-agent
	cp scripts/install-agent.sh $(DIST)/install.sh

# Tarball recipe shared by per-arch release-hub-tarball targets below.
# $(1) = arch suffix (amd64, arm64, armv7), $(2) = GOARCH, $(3) = GOARM
#
# PREREQUISITE: run `make build-web` first so internal/hub/web/dist/ has
# the embedded UI. The recipe fails fast with a helpful message if not.
define HUB_TARBALL_RECIPE
	@test -f internal/hub/web/dist/index.html || { \
	  echo "ERR: web bundle not staged at internal/hub/web/dist/index.html."; \
	  echo "     Run \`make build-web\` first (needs pnpm + node)."; \
	  exit 1; }
	@echo "==> Packaging lumen-hub-linux-$(1).tar.gz"
	mkdir -p $(DIST)/.hub-$(1)/lumen-hub-linux-$(1)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(2) $(3) $(GO) build -trimpath -ldflags '$(LDFLAGS)' \
		-o $(DIST)/.hub-$(1)/lumen-hub-linux-$(1)/lumen-hub ./cmd/lumen-hub
	cp scripts/install-hub.sh                  $(DIST)/.hub-$(1)/lumen-hub-linux-$(1)/install-hub.sh
	cp deploy/systemd/lumen-hub.service         $(DIST)/.hub-$(1)/lumen-hub-linux-$(1)/lumen-hub.service
	cp deploy/systemd/hub.env.example           $(DIST)/.hub-$(1)/lumen-hub-linux-$(1)/hub.env.example
	cp scripts/release-tarball-README.md        $(DIST)/.hub-$(1)/lumen-hub-linux-$(1)/README.md
	chmod +x $(DIST)/.hub-$(1)/lumen-hub-linux-$(1)/install-hub.sh
	tar -czf $(DIST)/lumen-hub-linux-$(1).tar.gz -C $(DIST)/.hub-$(1) lumen-hub-linux-$(1)
	rm -rf $(DIST)/.hub-$(1)
	@echo "    $(DIST)/lumen-hub-linux-$(1).tar.gz"
endef

.PHONY: release-hub-tarball-amd64 release-hub-tarball-arm64 release-hub-tarball-armv7
release-hub-tarball-amd64: ## Tarball lumen-hub for linux/amd64 (build-web prereq)
	$(call HUB_TARBALL_RECIPE,amd64,amd64,)
release-hub-tarball-arm64: ## Tarball lumen-hub for linux/arm64 — Pi 4/5, Ampere (build-web prereq)
	$(call HUB_TARBALL_RECIPE,arm64,arm64,)
release-hub-tarball-armv7: ## Tarball lumen-hub for linux/armv7 — Pi 2/3 (build-web prereq)
	$(call HUB_TARBALL_RECIPE,armv7,arm,GOARM=7)

.PHONY: release-hub-tarballs
release-hub-tarballs: release-hub-tarball-amd64 release-hub-tarball-arm64 ## All hub tarballs (amd64 + arm64). armv7 available via `make release-hub-tarball-armv7`.

# ============================================================
# Docker
# ============================================================

.PHONY: docker-hub
docker-hub: ## Build hub Docker image
	docker build -f deploy/docker/Dockerfile.hub -t lumenhq/lumen-hub:$(VERSION) .

.PHONY: docker-agent
docker-agent: ## Build agent Docker image
	docker build -f deploy/docker/Dockerfile.agent -t lumenhq/lumen-agent:$(VERSION) .

.PHONY: compose-up
compose-up: ## Start hub via Docker Compose, build images on first run
	docker compose -f deploy/docker/docker-compose.yml up --build

.PHONY: compose-down
compose-down: ## Stop the Compose stack and remove the local images
	docker compose -f deploy/docker/docker-compose.yml down --rmi local

# ============================================================
# Clean
# ============================================================

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(DIST) coverage.out coverage.html
	rm -rf web/dist docs/dist docs/.astro
