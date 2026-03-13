.PHONY: build run clean test fmt vet start stop restart status watch install uninstall export backup insights dream-cycle mcp benchmark benchmark-quality setup-hooks lint

BINARY=mnemonic
BUILD_DIR=bin
VERSION=0.9.0 # x-release-please-version
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION)"
TAGS=-tags "sqlite_fts5"

build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build $(TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/mnemonic

run: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml serve

test:
	CGO_ENABLED=1 go test $(TAGS) ./... -v

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf $(BUILD_DIR)

tidy:
	go mod tidy

# Quick check: fmt + vet
check: fmt vet
	@echo "All checks passed"

# --- Daemon Management ---
start: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml start

stop:
	./$(BUILD_DIR)/$(BINARY) stop

restart: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml restart

# --- Monitoring ---
status: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml status

watch: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml watch

# --- Memory Commands ---
remember: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml remember "$(TEXT)"

recall: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml recall "$(QUERY)"

consolidate: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml consolidate

ingest: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml ingest $(DIR)

# --- Data Management ---
export: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml export $(ARGS)

backup: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml backup

insights: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml insights

dream-cycle: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml dream-cycle

meta-cycle: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml meta-cycle

# --- MCP Server ---
mcp: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml mcp

# --- Benchmark ---
benchmark:
	CGO_ENABLED=1 go build $(TAGS) -o $(BUILD_DIR)/benchmark ./cmd/benchmark
	@echo "Benchmark built. Run: ./$(BUILD_DIR)/benchmark (daemon must be running)"

benchmark-quality:
	CGO_ENABLED=1 go build $(TAGS) $(LDFLAGS) -o $(BUILD_DIR)/benchmark-quality ./cmd/benchmark-quality
	@echo "Quality benchmark built. Run: ./$(BUILD_DIR)/benchmark-quality"

# --- Lint ---
lint:
	golangci-lint run

# --- Setup ---
setup-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks configured to use .githooks/"

install: build
	./$(BUILD_DIR)/$(BINARY) --config config.yaml install

uninstall:
	./$(BUILD_DIR)/$(BINARY) uninstall

# --- Release ---
# Usage: make release NEW_VERSION=0.8.0
release:
ifndef NEW_VERSION
	$(error NEW_VERSION is required. Usage: make release NEW_VERSION=0.8.0)
endif
	@echo "Releasing v$(NEW_VERSION)..."
	@sed -i 's/^VERSION=.*/VERSION=$(NEW_VERSION)/' Makefile
	@echo "Updated Makefile VERSION to $(NEW_VERSION)"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Update CHANGELOG.md with changes for $(NEW_VERSION)"
	@echo "  2. git add Makefile CHANGELOG.md"
	@echo "  3. git commit -m 'Release v$(NEW_VERSION)'"
	@echo "  4. git tag v$(NEW_VERSION)"
	@echo "  5. git push origin main --tags"
	@echo ""
	@echo "Pushing the tag will trigger the GitHub Actions release workflow."
