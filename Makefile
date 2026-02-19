.PHONY: help build test test-verbose clean run docker-build docker-run install lint fmt

# Default target
help:
	@echo "FideX Edge Node - Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  make build          - Build the FideX node binary"
	@echo "  make test           - Run all tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make run            - Build and run the node"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make install        - Install dependencies"
	@echo "  make lint           - Run linters"
	@echo "  make fmt            - Format code"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-run     - Run in Docker"

# Build the binary
build:
	@echo "Building FideX Edge Node..."
	@go build -o fidex-node ./cmd/fidex-node
	@echo "✓ Build complete: ./fidex-node"

# Run all tests
test:
	@echo "Running tests..."
	@go test ./... -cover

# Run tests with verbose output
test-verbose:
	@echo "Running tests (verbose)..."
	@go test ./... -v -cover

# Run tests for crypto package only
test-crypto:
	@echo "Running crypto tests..."
	@go test ./internal/crypto -v

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f fidex-node
	@rm -f fidex_local.db fidex_local.db-shm fidex_local.db-wal
	@rm -rf fidex/
	@go clean -cache -testcache
	@echo "✓ Clean complete"

# Build and run
run: build
	@echo "Starting FideX Edge Node..."
	@./fidex-node

# Install dependencies
install:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies installed"

# Run linter
lint:
	@echo "Running linters..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		go vet ./...; \
	fi

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Code formatted"

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build -t fidex-node:latest .
	@echo "✓ Docker image built"

# Run in Docker
docker-run:
	@echo "Running FideX Node in Docker..."
	@docker run -p 8080:8080 -p 8443:8443 -v $(PWD)/fidex:/app/fidex fidex-node:latest

# Development mode (watch and rebuild)
dev:
	@echo "Starting development mode..."
	@while true; do \
		make build; \
		./fidex-node & \
		PID=$$!; \
		inotifywait -e modify -r ./cmd ./internal; \
		kill $$PID; \
	done
