#!/bin/bash
# install.sh - Standalone installation script for Goban release tarballs.
#
# Usage:
#   sudo ./deploy/install.sh
#
# This script installs Goban from a pre-built release package into /opt/goban.
# It assumes the tarball has been extracted and contains bin/, public/, deploy/.
#
# Run as root (or with sudo).

set -euo pipefail

INSTALL_PREFIX="/opt/goban"
SERVICE_NAME="goban"
SERVICE_USER="${SERVICE_USER:-goban}"
SERVICE_GROUP="${SERVICE_GROUP:-goban}"
PORT="${GOBAN_PORT:-8080}"

# Detect script location (works whether called from repo root or extracted tarball)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RELEASE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"  # parent of deploy/

BIN_DIR="$INSTALL_PREFIX/bin"
CONFIG_DIR="$INSTALL_PREFIX/config"
DATA_DIR="$INSTALL_PREFIX/data"
PUBLIC_DIR="$INSTALL_PREFIX/public"

# ---------------------------------------------------------------------------
# Checks
# ---------------------------------------------------------------------------

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        echo "ERROR: This script must be run as root (use sudo)." >&2
        exit 1
    fi
}

check_source() {
    if [ ! -d "$RELEASE_DIR/bin" ] || [ ! -f "$RELEASE_DIR/bin/goban" ]; then
        echo "ERROR: Cannot find bin/goban in $RELEASE_DIR" >&2
        echo "This script should be run from inside an extracted release tarball." >&2
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Installation steps
# ---------------------------------------------------------------------------

create_system_user() {
    if id "$SERVICE_USER" &>/dev/null; then
        echo "[SKIP] User '$SERVICE_USER' already exists."
    else
        groupadd -r "$SERVICE_GROUP" 2>/dev/null || true
        useradd -r -g "$SERVICE_GROUP" -d "$INSTALL_PREFIX" \
            -s /usr/sbin/nologin -c "Goban Kanban Service" "$SERVICE_USER"
        echo "[OK] Created system user/group '$SERVICE_USER'."
    fi
}

setup_directories() {
    mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$DATA_DIR" "$PUBLIC_DIR"
    chown "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_PREFIX" "$BIN_DIR" \
        "$CONFIG_DIR" "$DATA_DIR" "$PUBLIC_DIR"
    echo "[OK] Directory structure created under $INSTALL_PREFIX."
}

install_binaries() {
    if command -v rsync &>/dev/null; then
        rsync -a --delete "$RELEASE_DIR/bin/" "$BIN_DIR/"
    else
        cp -f "$RELEASE_DIR/bin/"* "$BIN_DIR/"
        chmod 750 "$BIN_DIR"/*
    fi
    chown -R "$SERVICE_USER:$SERVICE_GROUP" "$BIN_DIR"
    echo "[OK] Binaries installed to $BIN_DIR."
}

install_assets() {
    if [ -d "$RELEASE_DIR/public" ]; then
        if command -v rsync &>/dev/null; then
            rsync -a --delete "$RELEASE_DIR/public/" "$PUBLIC_DIR/"
        else
            cp -rf "$RELEASE_DIR/public/"* "$PUBLIC_DIR/"
        fi
        chown -R "$SERVICE_USER:$SERVICE_GROUP" "$PUBLIC_DIR"
        echo "[OK] Frontend assets installed to $PUBLIC_DIR."
    fi
}

install_config() {
    local example="$SCRIPT_DIR/goban.toml.example"
    local target="$CONFIG_DIR/goban.toml"

    if [ -f "$target" ]; then
        echo "[SKIP] Config already exists at $target (not overwriting)."
    elif [ -f "$example" ]; then
        cp "$example" "$target"

        # Generate a JWT secret automatically
        local jwt_secret
        jwt_secret="$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)"
        sed -i "s/jwt_secret = \"CHANGE_ME\"/jwt_secret = \"$jwt_secret\"/" "$target"

        # Set the port from env
        sed -i "s/port = .*/port = \"$PORT\"/" "$target"

        chown "$SERVICE_USER:$SERVICE_GROUP" "$target"
        echo "[OK] Config created at $target (generated JWT secret)."
    else
        echo "[WARN] No example config found. Please create $target manually."
    fi
}

install_service() {
    local service_file="$SCRIPT_DIR/${SERVICE_NAME}.service"
    local target="/etc/systemd/system/${SERVICE_NAME}.service"

    if [ ! -f "$service_file" ]; then
        echo "[WARN] No systemd unit file found. Skipping service installation."
        return
    fi

    cp "$service_file" "$target"
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    echo "[OK] Systemd service installed and enabled."
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    require_root
    check_source

    echo "=== Goban Installer ==="
    echo "Release dir : $RELEASE_DIR"
    echo "Install dir : $INSTALL_PREFIX"
    echo "Service user: $SERVICE_USER"
    echo "Port        : $PORT"
    echo ""

    create_system_user
    setup_directories
    install_binaries
    install_assets
    install_config
    install_service

    echo ""
    echo "=== Installation complete ==="
    echo ""
    echo "To start the service:"
    echo "  sudo systemctl start $SERVICE_NAME"
    echo ""
    echo "To check status:"
    echo "  sudo systemctl status $SERVICE_NAME"
    echo ""
    echo "Web UI: http://<hostname>:$PORT"
}

main "$@"
