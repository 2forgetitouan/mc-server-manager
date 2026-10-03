#!/usr/bin/env bash
# =============================================================================
# mc-server-manager — uninstall script
# =============================================================================
# Removes the mc binary and systemd service. Config and world data are
# preserved unless --purge is passed.
#
# Usage:
#   sudo ./scripts/uninstall.sh            # keep config
#   sudo ./scripts/uninstall.sh --purge    # also remove /etc/mc/
# =============================================================================

set -euo pipefail

# --- Configuration -----------------------------------------------------------

BINARY_NAME="mc"
INSTALL_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
CONFIG_DIR="/etc/mc"
SERVICE_FILE="minecraft.service"

# --- Flags -------------------------------------------------------------------

PURGE=false
for arg in "$@"; do
    case "$arg" in
        --purge) PURGE=true ;;
        *)
            echo "Unknown option: $arg" >&2
            echo "Usage: $0 [--purge]" >&2
            exit 1
            ;;
    esac
done

# --- Helpers -----------------------------------------------------------------

info()  { echo "[INFO]  $*"; }
warn()  { echo "[WARN]  $*"; }
error() { echo "[ERROR] $*" >&2; exit 1; }

# --- Pre-flight checks -------------------------------------------------------

if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root (try: sudo $0)"
fi

# --- Stop and disable service ------------------------------------------------

if systemctl is-active --quiet minecraft 2>/dev/null; then
    info "Stopping minecraft service ..."
    systemctl stop minecraft
    info "Service stopped."
else
    info "Service is not running."
fi

if systemctl is-enabled --quiet minecraft 2>/dev/null; then
    info "Disabling minecraft service ..."
    systemctl disable minecraft
    info "Service disabled."
else
    info "Service is not enabled."
fi

# --- Remove binary -----------------------------------------------------------

if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
    info "Removing ${INSTALL_DIR}/${BINARY_NAME} ..."
    rm -f "${INSTALL_DIR}/${BINARY_NAME}"
    info "Binary removed."
else
    info "Binary not found at ${INSTALL_DIR}/${BINARY_NAME} — skipping."
fi

# --- Remove systemd service --------------------------------------------------

if [[ -f "${SYSTEMD_DIR}/${SERVICE_FILE}" ]]; then
    info "Removing ${SYSTEMD_DIR}/${SERVICE_FILE} ..."
    rm -f "${SYSTEMD_DIR}/${SERVICE_FILE}"
    info "Service file removed."
else
    info "Service file not found at ${SYSTEMD_DIR}/${SERVICE_FILE} — skipping."
fi

# --- Remove environment file -------------------------------------------------

if [[ -f "${CONFIG_DIR}/minecraft.env" ]]; then
    info "Removing ${CONFIG_DIR}/minecraft.env ..."
    rm -f "${CONFIG_DIR}/minecraft.env"
    info "Environment file removed."
fi

# --- Remove polkit rules -----------------------------------------------------

POLKIT_RULES="/etc/polkit-1/rules.d/10-mc-minecraft.rules"
POLKIT_PKLA="/etc/polkit-1/localauthority/50-local.d/10-mc-minecraft.pkla"

if [[ -f "${POLKIT_RULES}" ]]; then
    info "Removing polkit rule ${POLKIT_RULES} ..."
    rm -f "${POLKIT_RULES}"
fi
if [[ -f "${POLKIT_PKLA}" ]]; then
    info "Removing polkit rule ${POLKIT_PKLA} ..."
    rm -f "${POLKIT_PKLA}"
fi

info "Reloading systemd daemon ..."
systemctl daemon-reload

# --- Optionally purge config -------------------------------------------------

if [[ "$PURGE" == true ]]; then
    if [[ -d "${CONFIG_DIR}" ]]; then
        info "Purging config directory ${CONFIG_DIR} ..."
        rm -rf "${CONFIG_DIR}"
        info "Config directory removed."
    else
        info "Config directory not found at ${CONFIG_DIR} — skipping."
    fi
else
    info "Config directory ${CONFIG_DIR} was NOT removed (use --purge to remove)."
fi

# --- Summary -----------------------------------------------------------------

echo ""
echo "============================================"
echo "  mc-server-manager uninstalled"
echo "============================================"
echo ""
echo "Removed:"
echo "  - ${INSTALL_DIR}/${BINARY_NAME}"
echo "  - ${SYSTEMD_DIR}/${SERVICE_FILE}"
echo "  - ${CONFIG_DIR}/minecraft.env"
echo "  - polkit rules (if present)"
if [[ "$PURGE" == true ]]; then
    echo "  - ${CONFIG_DIR}/ (purged)"
fi
echo ""
echo "Kept:"
if [[ "$PURGE" != true ]]; then
    echo "  - ${CONFIG_DIR}/ (config files)"
fi
echo "  - /home/ubuntu/minecraft/ (server + world data)"
echo ""
