# =============================================================================
# mc-server-manager - Makefile
# =============================================================================

# --- Variables ---------------------------------------------------------------

BINARY_NAME  := mc
BUILD_DIR    := bin
INSTALL_DIR  := /usr/local/bin
SYSTEMD_DIR  := /etc/systemd/system
CONFIG_DIR   := /etc/mc

# Version: use exact tag on tagged commits, otherwise v0.0.0-dev.<short SHA>.
# Appends -dirty if the working tree has uncommitted changes.
GIT_SHA     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_TAG     := $(shell git describe --tags --exact-match 2>/dev/null)
GIT_DIRTY   := $(shell git diff --quiet 2>/dev/null || echo "-dirty")
VERSION     := $(if $(GIT_TAG),$(GIT_TAG)$(GIT_DIRTY),v0.0.0-dev.$(GIT_SHA)$(GIT_DIRTY))

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

## install: Build, install binary + service + polkit + env file + config
install: build
	@echo "==> Installing $(BINARY_NAME) to $(INSTALL_DIR) ..."
	install -d $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "==> Installing default config (if not present) ..."
	install -d -m 755 $(CONFIG_DIR)
	@test -f $(CONFIG_DIR)/config.toml || install -m 640 configs/mc.toml.example $(CONFIG_DIR)/config.toml
	@echo "==> Installing systemd service + polkit + env file ..."
	$(INSTALL_DIR)/$(BINARY_NAME) service install
	@echo ""
	@echo "Install complete. No sudo needed for mc start/stop/restart."

## uninstall: Remove binary + systemd service + polkit (keeps config and world)
uninstall:
	@echo "==> Stopping service (if running) ..."
	-systemctl stop minecraft 2>/dev/null || true
	-systemctl disable minecraft 2>/dev/null || true
	@echo "==> Removing binary ..."
	rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "==> Removing systemd service ..."
	rm -f $(SYSTEMD_DIR)/minecraft.service
	@echo "==> Removing environment file ..."
	rm -f $(CONFIG_DIR)/minecraft.env
	@echo "==> Removing polkit rules ..."
	rm -f /etc/polkit-1/rules.d/10-mc-minecraft.rules
	rm -f /etc/polkit-1/localauthority/50-local.d/10-mc-minecraft.pkla
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

## lint: Run go vet and staticcheck
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
