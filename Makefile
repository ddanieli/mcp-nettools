# MCP Network Tools Makefile

# Variables
BINARY_NAME=mcp-nettools
BIN_DIR=bin
CMD_DIR=cmd
GO_FILES=$(wildcard $(CMD_DIR)/*.go)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-s -w"

.PHONY: all build test clean help

# Default target
all: build

# Build the binary
build: $(BIN_DIR)/$(BINARY_NAME)

$(BIN_DIR)/$(BINARY_NAME): $(GO_FILES)
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)/*.go
	@echo "Build complete: $(BIN_DIR)/$(BINARY_NAME)"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -timeout 10s ./$(CMD_DIR)/...
	@echo ""
	@echo "Testing MCP server protocol..."
	@if [ -f $(BIN_DIR)/$(BINARY_NAME) ]; then \
		echo '{"jsonrpc": "2.0", "method": "tools/list", "id": 1}' | \
		./$(BIN_DIR)/$(BINARY_NAME) 2>/dev/null | \
		jq -e '.result.tools | length == 4' > /dev/null && \
		echo "✓ MCP server has 4 tools registered" || \
		(echo "✗ MCP server tool count mismatch" && exit 1); \
	else \
		echo "Binary not found. Run 'make build' first."; \
		exit 1; \
	fi
	@echo "All tests passed!"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BIN_DIR)
	@rm -f mcp-nettools
	@echo "Clean complete"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies updated"

# Run the binary (for testing)
run: build
	./$(BIN_DIR)/$(BINARY_NAME)

# Install to system
install: build
	@echo "Installing to /usr/local/bin..."
	@sudo cp $(BIN_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "Installed successfully"

# Help target
help:
	@echo "MCP Network Tools - Makefile targets:"
	@echo ""
	@echo "  make build    - Build the binary to $(BIN_DIR)/$(BINARY_NAME)"
	@echo "  make test     - Run all tests and validate MCP server"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make deps     - Download and tidy dependencies"
	@echo "  make run      - Build and run the binary"
	@echo "  make install  - Build and install to /usr/local/bin"
	@echo "  make help     - Show this help message"
	@echo ""
	@echo "The binary will be created at: $(BIN_DIR)/$(BINARY_NAME)"