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

TANTIVY_VERSION := v1.0.4
TANTIVY_LIB_PATH ?= dist/tantivy
CGO_LDFLAGS := -L$(TANTIVY_LIB_PATH)

GOLANGCI_LINT_VERSION := v2.7.2

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

download-tantivy: ## Download tantivy library for current platform
	@if [ ! -f "$(TANTIVY_LIB_PATH)/libtantivy_go.a" ]; then \
		echo "Downloading tantivy library $(TANTIVY_VERSION) for $(GOOS)/$(GOARCH)..."; \
		mkdir -p $(TANTIVY_LIB_PATH); \
		if [ "$(GOOS)" = "darwin" ]; then \
			if [ "$(GOARCH)" = "amd64" ]; then \
				curl -L "https://github.com/anyproto/tantivy-go/releases/download/$(TANTIVY_VERSION)/darwin-amd64.tar.gz" | tar xz -C $(TANTIVY_LIB_PATH); \
			elif [ "$(GOARCH)" = "arm64" ]; then \
				curl -L "https://github.com/anyproto/tantivy-go/releases/download/$(TANTIVY_VERSION)/darwin-arm64.tar.gz" | tar xz -C $(TANTIVY_LIB_PATH); \
			else \
				echo "Unsupported architecture: $(GOARCH) for macOS"; \
				exit 1; \
			fi; \
		elif [ "$(GOOS)" = "linux" ]; then \
			if [ "$(GOARCH)" = "amd64" ]; then \
				curl -L "https://github.com/anyproto/tantivy-go/releases/download/$(TANTIVY_VERSION)/linux-amd64-musl.tar.gz" | tar xz -C $(TANTIVY_LIB_PATH); \
			elif [ "$(GOARCH)" = "arm64" ]; then \
				curl -L "https://github.com/anyproto/tantivy-go/releases/download/$(TANTIVY_VERSION)/linux-arm64-musl.tar.gz" | tar xz -C $(TANTIVY_LIB_PATH); \
			else \
				echo "Unsupported architecture: $(GOARCH) for Linux"; \
				exit 1; \
			fi; \
		elif [ "$(GOOS)" = "windows" ]; then \
			if [ "$(GOARCH)" = "amd64" ]; then \
				curl -L "https://github.com/anyproto/tantivy-go/releases/download/$(TANTIVY_VERSION)/windows-amd64.tar.gz" | tar xz -C $(TANTIVY_LIB_PATH); \
			else \
				echo "Unsupported architecture: $(GOARCH) for Windows"; \
				exit 1; \
			fi; \
		else \
			echo "Unsupported OS: $(GOOS)"; \
			exit 1; \
		fi; \
		echo "Tantivy library downloaded successfully"; \
	else \
		echo "Tantivy library already exists"; \
	fi

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