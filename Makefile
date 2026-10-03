# =============================================================================
# mc-server-manager — Makefile
# =============================================================================

# --- Variables ---------------------------------------------------------------

BINARY_NAME  := mc
BUILD_DIR    := bin
INSTALL_DIR  := /usr/local/bin
SYSTEMD_DIR  := /etc/systemd/system
CONFIG_DIR   := /etc/mc

# Git-derived version, falls back to "dev" if no tags exist.
VERSION      := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Go build flags.
LDFLAGS      := -ldflags "-X main.version=$(VERSION)"

# Entry point for the binary.
CMD_PKG      := ./cmd/mc

# =============================================================================
# Targets
# =============================================================================

.PHONY: build build-linux-arm64 build-linux-amd64 build-all \
        install uninstall test test-verbose lint fmt clean help

## build: Compile for the current platform
build:
	@echo "==> Building $(BINARY_NAME) $(VERSION) ..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PKG)
	@echo "    $(BUILD_DIR)/$(BINARY_NAME)"

## build-linux-arm64: Cross-compile for linux/arm64
build-linux-arm64:
	@echo "==> Building $(BINARY_NAME) $(VERSION) for linux/arm64 ..."
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_PKG)
	@echo "    $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64"

## build-linux-amd64: Cross-compile for linux/amd64
build-linux-amd64:
	@echo "==> Building $(BINARY_NAME) $(VERSION) for linux/amd64 ..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PKG)
	@echo "    $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

## build-all: Build for both linux/arm64 and linux/amd64
build-all: build-linux-arm64 build-linux-amd64

## install: Build, install binary + systemd service + config, daemon-reload
install: build
	@echo "==> Installing $(BINARY_NAME) to $(INSTALL_DIR) ..."
	install -d $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "==> Installing systemd service ..."
	install -d $(SYSTEMD_DIR)
	install -m 644 systemd/minecraft.service $(SYSTEMD_DIR)/minecraft.service
	@echo "==> Installing default config (if not present) ..."
	install -d -m 755 $(CONFIG_DIR)
	@test -f $(CONFIG_DIR)/config.toml || install -m 640 configs/mc.toml.example $(CONFIG_DIR)/config.toml
	@echo "==> Reloading systemd daemon ..."
	systemctl daemon-reload
	@echo ""
	@echo "Install complete."
	@echo "  Binary:  $(INSTALL_DIR)/$(BINARY_NAME)"
	@echo "  Service: $(SYSTEMD_DIR)/minecraft.service"
	@echo "  Config:  $(CONFIG_DIR)/config.toml"
	@echo ""
	@echo "Next steps:"
	@echo "  sudo systemctl enable minecraft"
	@echo "  sudo systemctl start minecraft"

## uninstall: Remove binary + systemd service (keeps config and world data)
uninstall:
	@echo "==> Stopping service (if running) ..."
	-systemctl stop minecraft 2>/dev/null || true
	-systemctl disable minecraft 2>/dev/null || true
	@echo "==> Removing binary ..."
	rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "==> Removing systemd service ..."
	rm -f $(SYSTEMD_DIR)/minecraft.service
	@echo "==> Reloading systemd daemon ..."
	systemctl daemon-reload
	@echo ""
	@echo "Uninstall complete."
	@echo "  Config directory $(CONFIG_DIR) was NOT removed."
	@echo "  Minecraft world data was NOT removed."

## test: Run all tests
test:
	go test ./...

## test-verbose: Run all tests with verbose output
test-verbose:
	go test -v ./...

## lint: Run go vet (and staticcheck if available)
lint:
	@echo "==> go vet ..."
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then \
		echo "==> staticcheck ..."; \
		staticcheck ./...; \
	else \
		echo "    staticcheck not found, skipping (install: go install honnef.co/go/tools/cmd/staticcheck@latest)"; \
	fi

## fmt: Format all Go source files
fmt:
	gofmt -w .

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## help: Show available targets
help:
	@echo "mc-server-manager $(VERSION)"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
