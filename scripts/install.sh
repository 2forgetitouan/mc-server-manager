#!/usr/bin/env bash
# =============================================================================
# mc-server-manager — install script
# =============================================================================
# Compiles and installs the mc binary, systemd service, and default config.
# Safe to run multiple times (idempotent).
#
# Usage:
#   sudo ./scripts/install.sh
# =============================================================================

set -euo pipefail

# --- Configuration -----------------------------------------------------------

BINARY_NAME="mc"
INSTALL_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
CONFIG_DIR="/etc/mc"
SERVICE_FILE="minecraft.service"
MC_USER="ubuntu"

# Resolve the project root (parent directory of scripts/).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# --- Helpers -----------------------------------------------------------------

info()  { echo "[INFO]  $*"; }
warn()  { echo "[WARN]  $*"; }
error() { echo "[ERROR] $*" >&2; exit 1; }

# --- Pre-flight checks -------------------------------------------------------

if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root (try: sudo $0)"
fi

if ! command -v go &>/dev/null; then
    error "Go is not installed. Please install Go 1.21+ and try again."
fi

info "Go version: $(go version)"

# --- Build -------------------------------------------------------------------

info "Building ${BINARY_NAME} ..."
cd "${PROJECT_DIR}"

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo "dev")"
go build -ldflags "-X main.version=${VERSION}" -o "bin/${BINARY_NAME}" ./cmd/mc
info "Built bin/${BINARY_NAME} (version ${VERSION})"

# --- Install binary ----------------------------------------------------------

info "Installing binary to ${INSTALL_DIR}/${BINARY_NAME} ..."
install -d "${INSTALL_DIR}"
install -m 755 "bin/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"

# --- Install config ----------------------------------------------------------

info "Ensuring config directory ${CONFIG_DIR} exists ..."
install -d -m 755 "${CONFIG_DIR}"

if [[ -f "${CONFIG_DIR}/config.toml" ]]; then
    info "Config already exists at ${CONFIG_DIR}/config.toml — skipping."
else
    info "Installing default config to ${CONFIG_DIR}/config.toml ..."
    install -m 640 "${PROJECT_DIR}/configs/mc.toml.example" "${CONFIG_DIR}/config.toml"
fi

# Set ownership so the MC user can read the config.
if id "${MC_USER}" &>/dev/null; then
    chown "root:${MC_USER}" "${CONFIG_DIR}/config.toml"
    chmod 640 "${CONFIG_DIR}/config.toml"
    info "Config owned by root:${MC_USER} with mode 640."
else
    warn "User '${MC_USER}' does not exist yet. Config permissions not changed."
    warn "After creating the user, run: chown root:${MC_USER} ${CONFIG_DIR}/config.toml"
fi

# --- Install systemd service -------------------------------------------------

if [[ -f "${SYSTEMD_DIR}/${SERVICE_FILE}" ]]; then
    info "Service file already exists at ${SYSTEMD_DIR}/${SERVICE_FILE} — overwriting."
fi

info "Installing systemd service to ${SYSTEMD_DIR}/${SERVICE_FILE} ..."
install -m 644 "${PROJECT_DIR}/systemd/${SERVICE_FILE}" "${SYSTEMD_DIR}/${SERVICE_FILE}"

info "Reloading systemd daemon ..."
systemctl daemon-reload

info "Enabling ${SERVICE_FILE} (will start on next boot) ..."
systemctl enable "${SERVICE_FILE}"

# --- Done --------------------------------------------------------------------

echo ""
echo "============================================"
echo "  mc-server-manager installed successfully"
echo "============================================"
echo ""
echo "  Binary:  ${INSTALL_DIR}/${BINARY_NAME}"
echo "  Config:  ${CONFIG_DIR}/config.toml"
echo "  Service: ${SYSTEMD_DIR}/${SERVICE_FILE}"
echo ""
echo "Next steps:"
echo "  1. Edit ${CONFIG_DIR}/config.toml to match your setup."
echo "  2. Make sure the Minecraft server files are in /home/${MC_USER}/minecraft/"
echo "  3. Start the server:  sudo systemctl start minecraft"
echo "  4. Check status:      mc status"
echo "  5. View logs:         mc logs"
echo ""
