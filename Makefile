.PHONY: all build install uninstall clean clean-tantivy download-tantivy lint lint-fix install-linter

all: download-tantivy build

VERSION ?= $(shell git describe --tags 2>/dev/null)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d %H:%M:%S')
GIT_STATE ?= $(shell git diff --quiet 2>/dev/null && echo "clean" || echo "dirty")

LDFLAGS := -s -w \
           -X 'github.com/anyproto/anytype-cli/core.Version=$(VERSION)' \
           -X 'github.com/anyproto/anytype-cli/core.Commit=$(COMMIT)' \
           -X 'github.com/anyproto/anytype-cli/core.BuildTime=$(BUILD_TIME)' \
           -X 'github.com/anyproto/anytype-cli/core.GitState=$(GIT_STATE)'

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
OUTPUT ?= dist/anytype

TANTIVY_VERSION := $(shell cat go.mod | grep github.com/anyproto/tantivy-go | cut -d' ' -f2)
TANTIVY_LIB_PATH ?= dist/tantivy
TANTIVY_MARKER = $(TANTIVY_LIB_PATH)/.version-$(TANTIVY_VERSION)
TANTIVY_ASSET_darwin_amd64 := darwin-amd64
TANTIVY_ASSET_darwin_arm64 := darwin-arm64
TANTIVY_ASSET_linux_amd64 := linux-amd64-musl
TANTIVY_ASSET_linux_arm64 := linux-arm64-musl
TANTIVY_ASSET_windows_amd64 := windows-amd64
TANTIVY_ASSET = $(TANTIVY_ASSET_$(GOOS)_$(GOARCH))
TANTIVY_URL = https://github.com/anyproto/tantivy-go/releases/download/$(TANTIVY_VERSION)/$(TANTIVY_ASSET).tar.gz
CGO_LDFLAGS := -L$(TANTIVY_LIB_PATH)

GOLANGCI_LINT_VERSION := v2.12.2

##@ Build

build: download-tantivy ## Build the cli binary
	@echo "Building anytype-cli with embedded anytype-heart server..."
	@CGO_ENABLED=1 CGO_LDFLAGS="$(CGO_LDFLAGS)" GOOS=$(GOOS) GOARCH=$(GOARCH) go build -tags "$(BUILD_TAGS)" -ldflags "$(LDFLAGS) $(EXTRA_LDFLAGS)" -o $(OUTPUT)
	@echo "Built successfully: $(OUTPUT)"

cross-compile: ## Build for all platforms
	@echo "Cross-compiling anytype-cli for all platforms..."
	@$(MAKE) build-darwin-amd64
	@$(MAKE) build-darwin-arm64
	@$(MAKE) build-windows-amd64
	@$(MAKE) build-linux-amd64
	@$(MAKE) build-linux-arm64
	@echo "All platforms built successfully!"

build-darwin-amd64:
	@GOOS=darwin GOARCH=amd64 TANTIVY_LIB_PATH=dist/tantivy-darwin-amd64 OUTPUT=dist/anytype-darwin-amd64 $(MAKE) build

build-darwin-arm64:
	@GOOS=darwin GOARCH=arm64 TANTIVY_LIB_PATH=dist/tantivy-darwin-arm64 OUTPUT=dist/anytype-darwin-arm64 $(MAKE) build

build-windows-amd64:
	@GOOS=windows GOARCH=amd64 TANTIVY_LIB_PATH=dist/tantivy-windows-amd64 BUILD_TAGS=noheic CC=x86_64-w64-mingw32-gcc OUTPUT=dist/anytype-windows-amd64.exe $(MAKE) build

build-linux-amd64:
	@GOOS=linux GOARCH=amd64 TANTIVY_LIB_PATH=dist/tantivy-linux-amd64 BUILD_TAGS=noheic CC=x86_64-linux-musl-gcc EXTRA_LDFLAGS="-linkmode external -extldflags '-static'" OUTPUT=dist/anytype-linux-amd64 $(MAKE) build

build-linux-arm64:
	@GOOS=linux GOARCH=arm64 TANTIVY_LIB_PATH=dist/tantivy-linux-arm64 BUILD_TAGS=noheic CC=aarch64-linux-musl-gcc EXTRA_LDFLAGS="-linkmode external -extldflags '-static'" OUTPUT=dist/anytype-linux-arm64 $(MAKE) build

download-tantivy: $(TANTIVY_MARKER) ## Download tantivy library for current platform

$(TANTIVY_MARKER):
	@if [ -z "$(TANTIVY_ASSET)" ]; then \
		echo "Unsupported platform: $(GOOS)/$(GOARCH)"; \
		exit 1; \
	fi
	@rm -rf $(TANTIVY_LIB_PATH) $(TANTIVY_LIB_PATH).tar.gz
	@mkdir -p $(TANTIVY_LIB_PATH)
	@echo "Downloading tantivy library $(TANTIVY_VERSION) for $(GOOS)/$(GOARCH)..."
	@curl -fsSL --retry 5 --retry-all-errors \
		-o $(TANTIVY_LIB_PATH).tar.gz "$(TANTIVY_URL)"
	@tar xzf $(TANTIVY_LIB_PATH).tar.gz -C $(TANTIVY_LIB_PATH)
	@rm -f $(TANTIVY_LIB_PATH).tar.gz
	@if [ ! -f $(TANTIVY_LIB_PATH)/libtantivy_go.a ]; then \
		echo "Tantivy archive did not contain libtantivy_go.a"; \
		exit 1; \
	fi
	@touch $@
	@echo "Tantivy library $(TANTIVY_VERSION) downloaded successfully"

##@ Installation

install: build ## Install to ~/.local/bin (user installation)
	@echo "Installing anytype-cli..."
	@mkdir -p $$HOME/.local/bin
	@cp dist/anytype $$HOME/.local/bin/anytype
	@ln -sf $$HOME/.local/bin/anytype $$HOME/.local/bin/any
	@echo "Installed to $$HOME/.local/bin/ (available as 'anytype' and 'any')"
	@echo "Make sure $$HOME/.local/bin is in your PATH"
	@echo ""
	@echo "Usage:"
	@echo "  anytype serve              # Run server in foreground"
	@echo "  anytype service install    # Install as user service"

uninstall: ## Uninstall from ~/.local/bin
	@echo "Uninstalling anytype-cli..."
	@rm -f $$HOME/.local/bin/anytype
	@rm -f $$HOME/.local/bin/any
	@echo "Uninstalled from $$HOME/.local/bin/"

##@ Development

lint: ## Run linters
	@golangci-lint run ./...

lint-fix: ## Run linters with auto-fix
	@golangci-lint run --fix ./...

install-linter: ## Install golangci-lint
	@echo "Installing golangci-lint..."
	@go install github.com/daixiang0/gci@latest
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "golangci-lint installed successfully"

test: download-tantivy ## Run tests
	@echo "Running tests..."
	@CGO_ENABLED=1 CGO_LDFLAGS="$(CGO_LDFLAGS)" go test github.com/anyproto/anytype-cli/...
	@echo "Tests completed"

##@ Cleanup

clean: clean-tantivy ## Clean all build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf dist/
	@echo "Build artifacts cleaned"

clean-tantivy: ## Clean tantivy libraries
	@echo "Cleaning tantivy libraries..."
	@rm -rf $(TANTIVY_LIB_PATH)
	@echo "Tantivy libraries cleaned"

##@ Other

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)