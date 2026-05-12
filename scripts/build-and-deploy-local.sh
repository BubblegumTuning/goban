#!/bin/bash
# build-and-deploy-local.sh - Build and deploy Goban locally end-to-end.
#
# Usage:
#   ./scripts/build-and-deploy-local.sh [source_dir]
#
# Workflow:
#   1. Validates source has bin/ AND public/ (runs 'make package' if needed)
#   2. Creates /opt/goban directory structure
#   3. Deploys binaries + frontend assets (rsync --delete removes stale files)
#   4. Regenerates .fiber.gz companion files on target
#   5. Restarts systemd service
#   6. Verifies health check and critical assets return HTTP 200

set -euo pipefail

INSTALL_PREFIX="/opt/goban"
SERVICE_NAME="goban"
SERVER_PORT="${GOBAN_PORT:-8080}"

usage() {
    echo "Usage: sudo $0 [source_dir]"
    echo ""
    echo "Build and deploy Goban to /opt/goban on this machine."
    echo ""
    echo "  source_dir   - Directory containing bin/ and public/ (default: auto-detect)"
    echo "                 If not specified, runs 'make package' in the current repo."
    exit 1
}

# ---------------------------------------------------------------------------
# Determine source directory
# ---------------------------------------------------------------------------
SOURCE_DIR="${1:-}"

if [ -z "$SOURCE_DIR" ]; then
    # Auto-detect: look for make package output or build.sh output
    if [ -d "bin" ] && [ -f "bin/goban" ] && [ -d "public" ]; then
        SOURCE_DIR="."
        echo "[*] Using current directory (bin/ + public/ found)"
    elif [ -d "dist" ]; then
        LATEST=$(ls -1dt dist/*/ 2>/dev/null | head -1)
        if [ -n "$LATEST" ] && [ -f "${LATEST}bin/goban" ] && [ -d "${LATEST}public" ]; then
            SOURCE_DIR="$LATEST"
            echo "[*] Using packaged output: $SOURCE_DIR"
        fi
    fi

    if [ -z "$SOURCE_DIR" ]; then
        echo "[*] No ready source found. Running 'make package'..."
        make package || { echo "ERROR: Build failed."; exit 1; }
        LATEST=$(ls -1dt dist/*/ 2>/dev/null | head -1)
        if [ -n "$LATEST" ]; then
            SOURCE_DIR="$LATEST"
        fi
    fi
fi

# ---------------------------------------------------------------------------
# Validate source has required structure BEFORE touching anything on disk
# ---------------------------------------------------------------------------
echo ""
echo "=== Pre-flight Validation ==="
ERRORS=0

if [ ! -f "$SOURCE_DIR/bin/goban" ]; then
    echo "  ERROR: bin/goban not found in $SOURCE_DIR"
    ERRORS=$((ERRORS + 1))
fi

if [ ! -d "$SOURCE_DIR/public" ] || [ -z "$(ls -A "$SOURCE_DIR/public" 2>/dev/null)" ]; then
    echo "  ERROR: public/ directory is missing or empty in $SOURCE_DIR"
    echo "         Use 'make package' (not just 'make build') to include frontend assets."
    ERRORS=$((ERRORS + 1))
fi

# Check critical frontend files exist
for f in index.html app.js; do
    if [ ! -f "$SOURCE_DIR/public/$f" ]; then
        echo "  ERROR: public/$f not found in source"
        ERRORS=$((ERRORS + 1))
    fi
done

if [ $ERRORS -gt 0 ]; then
    echo ""
    echo "=== Validation failed with $ERRORS error(s). Aborting. ==="
    exit 1
fi

echo "  ✓ bin/goban present"
echo "  ✓ public/ directory complete"
echo "  ✓ Critical frontend files verified"

# ---------------------------------------------------------------------------
# Check root/sudo privileges
# ---------------------------------------------------------------------------
if [ "$(id -u)" -ne 0 ] && ! sudo -n true 2>/dev/null; then
    echo ""
    echo "ERROR: This script requires root privileges."
    echo "       Run with: sudo $0"
    exit 1
fi

# Determine sudo prefix (script may already be running as root)
if [ "$(id -u)" -ne 0 ]; then
    SUDO_CMD="sudo"
else
    SUDO_CMD=""
fi

run() {
    $SUDO_CMD "$@"
}

# ---------------------------------------------------------------------------
# Step 1: Create directory structure atomically (before any copies)
# ---------------------------------------------------------------------------
echo ""
echo "=== Creating Directory Structure ==="
for dir in bin config public/styles data; do
    run mkdir -p "${INSTALL_PREFIX}/${dir}"
done
echo "  ✓ /opt/goban/{bin,config,public/styles,data} ready"

# ---------------------------------------------------------------------------
# Step 2: Deploy binaries
# ---------------------------------------------------------------------------
echo ""
echo "=== Deploying Binaries ==="
run cp "$SOURCE_DIR/bin/goban" "${INSTALL_PREFIX}/bin/goban"
run chmod +x "${INSTALL_PREFIX}/bin/goban"
echo "  ✓ goban server binary deployed"

if [ -f "$SOURCE_DIR/bin/goban-cli" ]; then
    run cp "$SOURCE_DIR/bin/goban-cli" "${INSTALL_PREFIX}/bin/goban-cli"
    run chmod +x "${INSTALL_PREFIX}/bin/goban-cli"
    echo "  ✓ goban-cli deployed"
fi

if [ -f "$SOURCE_DIR/bin/goban-user-cli" ]; then
    run cp "$SOURCE_DIR/bin/goban-user-cli" "${INSTALL_PREFIX}/bin/goban-user-cli"
    run chmod +x "${INSTALL_PREFIX}/bin/goban-user-cli"
    echo "  ✓ goban-user-cli deployed"
fi

# ---------------------------------------------------------------------------
# Step 3: Deploy frontend assets (rsync --delete removes stale files)
# ---------------------------------------------------------------------------
echo ""
echo "=== Deploying Frontend Assets ==="
if command -v rsync >/dev/null 2>&1; then
    run rsync -avz --delete "$SOURCE_DIR/public/" "${INSTALL_PREFIX}/public/" > /dev/null 2>&1
else
    echo "[!] rsync not found, using cp fallback (stale files may persist)"
    rm -rf "${INSTALL_PREFIX}/public/"*
    cp -r "$SOURCE_DIR/public/"* "${INSTALL_PREFIX}/public/"
fi

# Clean stale artifacts that have appeared in the past
run rm -f "${INSTALL_PREFIX}/public/public" 2>/dev/null || true
run rm -rf "${INSTALL_PREFIX}/public/styles/styles/" 2>/dev/null || true
echo "  ✓ Frontend assets deployed (rsync --delete)"

# ---------------------------------------------------------------------------
# Step 4: Regenerate .fiber.gz companion files on target
# ---------------------------------------------------------------------------
echo ""
echo "=== Refreshing Compressed Assets ==="
find "${INSTALL_PREFIX}/public" -type f ! -name '*.gz' | while read -r f; do
    gzip -n -c "$f" > "${f}.fiber.gz"
done 2>/dev/null || true
echo "  ✓ .fiber.gz files regenerated"

# ---------------------------------------------------------------------------
# Step 5: Set ownership (if goban user exists)
# ---------------------------------------------------------------------------
if id "goban" >/dev/null 2>&1; then
    echo ""
    echo "=== Setting Ownership ==="
    run chown -R goban:goban "${INSTALL_PREFIX}/bin"
    run chown -R goban:goban "${INSTALL_PREFIX}/public"
    echo "  ✓ goban:goban ownership set"
fi

# ---------------------------------------------------------------------------
# Step 6: Restart systemd service
# ---------------------------------------------------------------------------
echo ""
echo "=== Restarting Service ==="
if systemctl list-unit-files "${SERVICE_NAME}.service" >/dev/null 2>&1; then
    run systemctl restart "$SERVICE_NAME"
    sleep 2

    if run systemctl is-active --quiet "$SERVICE_NAME"; then
        echo "  ✓ ${SERVICE_NAME} service started successfully"
    else
        echo "  ✗ Service failed to start. Check logs:"
        echo "      sudo journalctl -u $SERVICE_NAME -n 20"
        exit 1
    fi
else
    echo "[!] systemd service '${SERVICE_NAME}' not found."
    echo "    To start manually: ${INSTALL_PREFIX}/bin/goban"
fi

# ---------------------------------------------------------------------------
# Step 7: Verify deployment
# ---------------------------------------------------------------------------
echo ""
echo "=== Verification ==="
VERIFY_ERRORS=0

# Health check
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:${SERVER_PORT}/healthz 2>/dev/null || echo "000")
if [ "$HTTP_STATUS" = "200" ]; then
    echo "  ✓ /healthz (HTTP ${HTTP_STATUS})"
else
    echo "  ✗ /healthz FAILED (HTTP ${HTTP_STATUS})"
    VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
fi

# Critical frontend assets
for asset in "/index.html" "/app.js" "/go-board.html" "/styles/tailwind.min.css"; do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:${SERVER_PORT}${asset} 2>/dev/null || echo "000")
    if [ "$STATUS" = "200" ]; then
        echo "  ✓ ${asset} (HTTP ${STATUS})"
    else
        echo "  ✗ ${asset} FAILED (HTTP ${STATUS})"
        VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
    fi
done

# API check - boards endpoint returns data
BOARD_COUNT=$(curl -s http://localhost:${SERVER_PORT}/api/boards 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "?")
if [ "$BOARD_COUNT" != "?" ] && [ "$BOARD_COUNT" -gt 0 ]; then
    echo "  ✓ /api/boards returns ${BOARD_COUNT} board(s)"
else
    echo "  ! /api/boards returned unexpected result (may be normal on fresh start)"
fi

# App.js size check (not empty/stale)
APP_JS_SIZE=$(curl -s http://localhost:${SERVER_PORT}/app.js | wc -c)
if [ "$APP_JS_SIZE" -gt 1000 ]; then
    echo "  ✓ app.js is ${APP_JS_SIZE} bytes"
else
    echo "  ✗ app.js appears empty or stale (${APP_JS_SIZE} bytes)"
    VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
if [ $VERIFY_ERRORS -gt 0 ]; then
    echo "=== Deployment completed with ${VERIFY_ERRORS} verification error(s) ==="
    exit 1
else
    echo "=== All verifications passed — deployment successful ==="
fi

# Show version if available
VERSION_OUTPUT=$(${INSTALL_PREFIX}/bin/goban --version 2>/dev/null || echo "(unknown)")
echo ""
echo "Version:        $VERSION_OUTPUT"
echo "Server binary:  ${INSTALL_PREFIX}/bin/goban"
echo "Frontend:       ${INSTALL_PREFIX}/public/"
echo "Config:         /opt/goban/config/goban.toml"
