# Formae Plugin Makefile
#
# Targets:
#   build   - Build the plugin binary
#   test    - Run tests
#   lint    - Run linter
#   clean   - Remove build artifacts
#   install - Build and install plugin locally (binary + schema + manifest)

# Plugin metadata - extracted from formae-plugin.pkl
PLUGIN_NAME := $(shell pkl eval -x 'name' formae-plugin.pkl 2>/dev/null || echo "example")
PLUGIN_VERSION := $(shell pkl eval -x 'version' formae-plugin.pkl 2>/dev/null || echo "0.0.0")
PLUGIN_NAMESPACE := $(shell pkl eval -x 'namespace' formae-plugin.pkl 2>/dev/null || echo "EXAMPLE")

# Build settings
GO := go
GOFLAGS := -trimpath
BINARY := $(PLUGIN_NAME)

# Installation paths
# NOTE: Directory structure will change from <namespace> to <name> in a future version
PLUGIN_BASE_DIR := $(HOME)/.pel/formae/plugins
INSTALL_DIR := $(PLUGIN_BASE_DIR)/$(PLUGIN_NAME)/v$(PLUGIN_VERSION)

.PHONY: all build test test-unit test-integration lint clean install help setup-credentials clean-environment conformance-test conformance-test-crud conformance-test-discovery

all: build

## build: Build the plugin binary and update manifest
build:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY) .
	@MIN_VERSION=$$($(GO) list -m -f '{{.Dir}}' github.com/platform-engineering-labs/formae/pkg/plugin 2>/dev/null | xargs -I{} grep 'MinFormaeVersion' {}/version.go 2>/dev/null | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"'); \
	if [ -n "$$MIN_VERSION" ]; then \
		echo "Updating minFormaeVersion to $$MIN_VERSION"; \
		if [ "$$(uname)" = "Darwin" ]; then \
			sed -i '' 's/^minFormaeVersion = .*/minFormaeVersion = "'"$$MIN_VERSION"'"/' formae-plugin.pkl; \
		else \
			sed -i 's/^minFormaeVersion = .*/minFormaeVersion = "'"$$MIN_VERSION"'"/' formae-plugin.pkl; \
		fi; \
	fi

## test: Run all tests
test:
	$(GO) test -v ./...

## test-unit: Run unit tests only (tests with //go:build unit tag)
test-unit:
	$(GO) test -v -tags=unit ./...

## test-integration: Run integration tests (requires cloud credentials)
test-integration:
	$(GO) test -v -tags=integration ./...

## lint: Run golangci-lint
lint:
	golangci-lint run

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/

## verify-schema: Verify plugin schema files against PKL spec
verify-schema:
	go run github.com/platform-engineering-labs/formae/pkg/plugin/testutil/cmd/verify-schema ./schema/pkl

## schema-docs: Generate documentation for plugin schema in markdown format
schema-docs:
	go run github.com/platform-engineering-labs/formae/pkg/plugin/testutil/cmd/schema-docs --format markdown ./schema/pkl

## install: Build and install plugin locally (binary + schema + manifest)
## Installs to ~/.pel/formae/plugins/<namespace>/v<version>/
## Removes any existing versions of the plugin first to ensure clean state.
install: build
	@echo "Installing $(PLUGIN_NAME) v$(PLUGIN_VERSION) (namespace: $(PLUGIN_NAMESPACE))..."
	@rm -rf $(PLUGIN_BASE_DIR)/$(PLUGIN_NAME)
	@mkdir -p $(INSTALL_DIR)/schema/pkl
	@cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@cp -r schema/pkl/* $(INSTALL_DIR)/schema/pkl/
	@if [ -f schema/Config.pkl ]; then cp schema/Config.pkl $(INSTALL_DIR)/schema/; fi
	@cp formae-plugin.pkl $(INSTALL_DIR)/
	@echo "Installed to $(INSTALL_DIR)"
	@echo "  - Binary: $(INSTALL_DIR)/$(BINARY)"
	@echo "  - Schema: $(INSTALL_DIR)/schema/"
	@echo "  - Manifest: $(INSTALL_DIR)/formae-plugin.pkl"

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## setup-credentials: Provision cloud provider credentials
## Edit scripts/ci/setup-credentials.sh to configure for your provider.
setup-credentials:
	@./scripts/ci/setup-credentials.sh

## clean-environment: Clean up test resources in cloud environment
## Called before and after conformance tests. Edit scripts/ci/clean-environment.sh
## to configure for your provider.
clean-environment:
	@./scripts/ci/clean-environment.sh

# Normalize TIMEOUT: bare digits (legacy minutes form) get "m" appended.
TEST_TIMEOUT := $(if $(TIMEOUT),$(if $(shell echo $(TIMEOUT) | grep -E '^[0-9]+$$'),$(TIMEOUT)m,$(TIMEOUT)),30m)

# OVH managed Kubernetes is slow:
#   - cluster create normally takes 2-3 min, but US-EAST-VA-1 has been observed
#     to keep a fresh nodepool in INSTALLING for 10-15 min under load
#   - cluster delete tears the control plane down over a similar window
#   - the OOB-delete phase recreates a cluster *and* exercises Delete on it, so
#     two of those windows back-to-back fit inside one OOB plugin RPC
#
# FORMAE_TIMEOUT: bounds the framework's per-operation wait
# (PollStatus / WaitForResourceCompletion). Default 15 min — kube cluster
# create+nodepool-installing has been observed at 10-12 min on US-EAST-VA-1.
# OOB_TIMEOUT: bounds a single OOB Create/Delete plugin RPC. 15 min is enough
# headroom for a slow OVH region without hiding genuine plugin hangs.
# OOB_DELETE_TIMEOUT: bounds the post-sync inventory tombstone wait. The plugin
# Delete has already returned by this point; what we wait for here is OVH's GET
# eventually reflecting the deletion — for kube clusters that is 5-10 min.
FORMAE_TIMEOUT ?= 15
OOB_TIMEOUT ?= 15
OOB_DELETE_TIMEOUT ?= 15
KUBE_TEST_ENV := FORMAE_TEST_TIMEOUT=$(FORMAE_TIMEOUT) \
	$(if $(OOB_TIMEOUT),FORMAE_TEST_OOB_TIMEOUT=$(OOB_TIMEOUT)) \
	$(if $(OOB_DELETE_TIMEOUT),FORMAE_TEST_OOB_DELETE_TIMEOUT=$(OOB_DELETE_TIMEOUT)) \
	FORMAE_LOG_PLUGINS=debug

## conformance-test: Run all conformance tests (CRUD + discovery)
## Usage: make conformance-test [TEST=s3-bucket] [TIMEOUT=30m] [OOB_TIMEOUT=30] [OOB_DELETE_TIMEOUT=2]
## Calls clean-environment before and after tests.
conformance-test: conformance-test-crud conformance-test-discovery

## conformance-test-crud: Run only CRUD lifecycle tests
## Usage: make conformance-test-crud [TEST=s3-bucket] [TIMEOUT=30m] [OOB_TIMEOUT=30] [OOB_DELETE_TIMEOUT=2]
conformance-test-crud: install setup-credentials
	@echo "Pre-test cleanup..."
	@./scripts/ci/clean-environment.sh || true
	@echo ""
	@echo "Running CRUD conformance tests..."
	@FORMAE_TEST_FILTER="$(TEST)" FORMAE_TEST_TYPE=crud $(KUBE_TEST_ENV) \
		$(GO) test -tags=conformance -v -timeout $(TEST_TIMEOUT) ./...; \
	TEST_EXIT=$$?; \
	echo ""; \
	echo "Post-test cleanup..."; \
	./scripts/ci/clean-environment.sh || true; \
	exit $$TEST_EXIT

## conformance-test-discovery: Run only discovery tests
## Usage: make conformance-test-discovery [TEST=s3-bucket] [TIMEOUT=30m] [OOB_TIMEOUT=30]
conformance-test-discovery: install setup-credentials
	@echo "Pre-test cleanup..."
	@./scripts/ci/clean-environment.sh || true
	@echo ""
	@echo "Running discovery conformance tests..."
	@FORMAE_TEST_FILTER="$(TEST)" FORMAE_TEST_TYPE=discovery $(KUBE_TEST_ENV) \
		$(GO) test -tags=conformance -v -timeout $(TEST_TIMEOUT) ./...; \
	TEST_EXIT=$$?; \
	echo ""; \
	echo "Post-test cleanup..."; \
	./scripts/ci/clean-environment.sh || true; \
	exit $$TEST_EXIT

## conformance-test-crud-run: Run only CRUD lifecycle tests (no cleanup)
## Used by CI matrix jobs where cleanup is managed separately.
conformance-test-crud-run:
	@echo "Running CRUD conformance tests..."
	@FORMAE_TEST_FILTER="$(TEST)" FORMAE_TEST_TYPE=crud $(KUBE_TEST_ENV) \
		$(GO) test -tags=conformance -v -timeout $(TEST_TIMEOUT) ./...

## conformance-test-discovery-run: Run only discovery tests (no cleanup)
## Used by CI matrix jobs where cleanup is managed separately.
conformance-test-discovery-run:
	@echo "Running discovery conformance tests..."
	@FORMAE_TEST_FILTER="$(TEST)" FORMAE_TEST_TYPE=discovery $(KUBE_TEST_ENV) \
		$(GO) test -tags=conformance -v -timeout $(TEST_TIMEOUT) ./...
