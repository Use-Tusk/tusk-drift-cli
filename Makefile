GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=tusk
BINARY_UNIX=$(BINARY_NAME)_unix


.PHONY: all build build-ci build-linux test test-ci clean deps install-buf install-lint-tools setup setup-ci run fmt lint help

all: build

build:
	@echo "🔨 Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BINARY_NAME) -v .

build-ci:
	@echo "🏗️  CI: Building $(BINARY_NAME) with version info..."
	$(eval VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev"))
	$(eval BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ'))
	$(eval GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown"))
	$(GOBUILD) -ldflags "-X github.com/Use-Tusk/tusk-cli/internal/version.Version=$(VERSION) -X github.com/Use-Tusk/tusk-cli/internal/version.BuildTime=$(BUILD_TIME) -X github.com/Use-Tusk/tusk-cli/internal/version.GitCommit=$(GIT_COMMIT)" -o $(BINARY_NAME) -v .

test: 
	@echo "🧪 Running tests..."
	$(GOTEST) -v .

test-ci:
	@echo "🧪 CI: Running tests..."
	$(GOTEST) -v ./...

clean:
	@echo "🧹 Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

deps:
	@echo "📦 Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

build-linux:
	@echo "🐧 Building for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v .

install-buf:
	@echo "🔧 Installing buf..."
	@command -v buf >/dev/null 2>&1 || go install github.com/bufbuild/buf/cmd/buf@latest
	@echo "✅ buf is available"

install-lint-tools:
	@echo "📦 Installing linting tools..."
	GOTOOLCHAIN=local go install mvdan.cc/gofumpt@latest
	GOTOOLCHAIN=local go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✅ Linting tools installed"

setup: install-buf deps install-lint-tools
	@echo "✅ Development environment ready"

setup-ci: deps install-lint-tools
	@echo "✅ CI environment ready"

run: build
	./$(BINARY_NAME)

fmt:
	@echo "📝 Formatting code..."
	gofumpt -w .

lint:
	@echo "🔍 Linting code..."
	golangci-lint run --allow-parallel-runners


help:
	@echo "Available targets:"
	@echo "  all                - build (default)"
	@echo "  build              - Build the binary"
	@echo "  build-ci           - Build for CI with version info"
	@echo "  build-linux        - Build for Linux"
	@echo "  test               - Run tests"
	@echo "  test-ci            - Run tests for CI"
	@echo "  clean              - Clean build artifacts"
	@echo "  deps               - Download dependencies"
	@echo "  install-buf        - Check if buf is installed"
	@echo "  install-lint-tools - Install linting tools"
	@echo "  setup              - Setup development environment"
	@echo "  setup-ci           - Setup CI environment"
	@echo "  run                - Build and run"
	@echo "  fmt                - Format code"
	@echo "  lint               - Lint code"
	@echo "  help               - Show this help"
