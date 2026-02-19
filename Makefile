.PHONY: build run dev clean test lint help

# Binary name
BINARY := mixed-script-detector

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GORUN := $(GOCMD) run
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt

# Build the binary
build:
	@echo "Building $(BINARY)..."
	$(GOBUILD) -o $(BINARY) -v

# Run the binary (interactive mode)
run: build
	@echo "Running $(BINARY)..."
	./$(BINARY)

# Development mode: build and run
dev: build
	./$(BINARY)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY)
	rm -f *_findings.txt
	rm -f *_findings.json

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOGET) ./...
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

# Run tests (if any)
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Lint (basic go vet)
lint:
	@echo "Linting..."
	$(GOCMD) vet ./...

# Full CI build
ci: deps lint test build
	@echo "CI build complete!"

# Help
help:
	@echo "Available targets:"
	@echo "  build     - Build the binary"
	@echo "  run       - Build and run interactively"
	@echo "  dev       - Development mode (build + run)"
	@echo "  clean     - Remove build artifacts"
	@echo "  deps      - Download dependencies"
	@echo "  fmt       - Format code with gofmt"
	@echo "  test      - Run tests"
	@echo "  lint      - Run linter (go vet)"
	@echo "  ci        - Full CI build (deps + lint + test + build)"
	@echo "  help      - Show this help"
