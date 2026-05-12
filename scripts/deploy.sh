#!/bin/bash
# deploy.sh - Build locally and deploy Goban to a remote host via SSH/rsync.
#
# Usage:
#   ./scripts/deploy.sh <remote-host> [deploy-user]
#
# Workflow:
#   1. Validates source has bin/ AND public/ (runs 'make package' if needed)
#   2. Creates /opt/goban directory structure on remote host
#   3. Deploys binaries via scp, frontend assets via rsync --delete
#   4. Regenerates .fiber.gz companion files on remote target
#   5. Restarts systemd service on remote host
#   6. Verifies health check and critical assets return HTTP 200

set -euo pipefail

INSTALL_PREFIX="/opt/goban"
SERVICE_NAME="goban"
DEPLOY_USER="${GOBAN_DEPLOY_USER:-goban}"

usage() {
    echo "Usage: $0 <remote-host> [deploy-user]"
    echo ""
    echo "Build locally and deploy Goban to a remote host via SSH/rsync."
    echo ""
    echo "  remote-host   - Remote server hostname or IP (e.g., 192.168.88.30)"
    echo "  deploy-user   - SSH user on remote (default: ${DEPLOY_USER})"
    exit 1
}

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
if [ $# -lt 1 ]; then
    usage
fi

REMOTE_HOST="$1"
if [ $# -ge 2 ]; then
    DEPLOY_USER="$2"
fi

SERVER_PORT="${GOBAN_PORT:-8080}"

echo "=== Remote Deployment Configuration ==="
echo "  Host:   ${DEPLOY_USER}@${REMOTE_HOST}"
echo "  Target: ${INSTALL_PREFIX}"
echo ""

# ---------------------------------------------------------------------------
# Ensure source has required structure (build if needed)
# ---------------------------------------------------------------------------
SOURCE_DIR=""

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

# ---------------------------------------------------------------------------
# Validate source has required structure BEFORE touching remote host
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
# Step 1: Prepare remote directories and clean stale artifacts
# ---------------------------------------------------------------------------
echo ""
echo "=== Preparing Remote Directories ==="
ssh ${DEPLOY_USER}@${REMOTE_HOST} \
    "sudo mkdir -p ${INSTALL_PREFIX}/{bin,config,public/styles,data}"

ssh ${DEPLOY_USER}@${REMOTE_HOST} "
    sudo rm -f ${INSTALL_PREFIX}/public/public 2>/dev/null || true
    sudo rm -rf ${INSTALL_PREFIX}/public/styles/styles/ 2>/dev/null || true
    sudo rm -f ${INSTALL_PREFIX}/public/font-awesome.min.css ${INSTALL_PREFIX}/public/font-awesome.min.css.fiber.gz
    sudo rm -f ${INSTALL_PREFIX}/public/tailwind.min.css ${INSTALL_PREFIX}/public/tailwind.min.css.fiber.gz
"
echo "  ✓ Remote directories ready, stale artifacts cleaned"

# ---------------------------------------------------------------------------
# Step 2: Deploy binaries via scp
# ---------------------------------------------------------------------------
echo ""
echo "=== Deploying Binaries ==="
scp "$SOURCE_DIR/bin/goban" ${DEPLOY_USER}@${REMOTE_HOST}:${INSTALL_PREFIX}/bin/
ssh ${DEPLOY_USER}@${REMOTE_HOST} \
    "sudo chown ${DEPLOY_USER}:${DEPLOY_USER} ${INSTALL_PREFIX}/bin/goban && sudo chmod +x ${INSTALL_PREFIX}/bin/goban"
echo "  ✓ goban server binary deployed"

if [ -f "$SOURCE_DIR/bin/goban-cli" ]; then
    scp "$SOURCE_DIR/bin/goban-cli" ${DEPLOY_USER}@${REMOTE_HOST}:${INSTALL_PREFIX}/bin/
    ssh ${DEPLOY_USER}@${REMOTE_HOST} \
        "sudo chown ${DEPLOY_USER}:${DEPLOY_USER} ${INSTALL_PREFIX}/bin/goban-cli && sudo chmod +x ${INSTALL_PREFIX}/bin/goban-cli"
    echo "  ✓ goban-cli deployed"
fi

if [ -f "$SOURCE_DIR/bin/goban-user-cli" ]; then
    scp "$SOURCE_DIR/bin/goban-user-cli" ${DEPLOY_USER}@${REMOTE_HOST}:${INSTALL_PREFIX}/bin/
    ssh ${DEPLOY_USER}@${REMOTE_HOST} \
        "sudo chown ${DEPLOY_USER}:${DEPLOY_USER} ${INSTALL_PREFIX}/bin/goban-user-cli && sudo chmod +x ${INSTALL_PREFIX}/bin/goban-user-cli"
    echo "  ✓ goban-user-cli deployed"
fi

# ---------------------------------------------------------------------------
# Step 3: Deploy frontend assets via rsync --delete (removes stale files)
# ---------------------------------------------------------------------------
echo ""
echo "=== Deploying Frontend Assets ==="
rsync -avz --delete \
    $SOURCE_DIR/public/ \
    ${DEPLOY_USER}@${REMOTE_HOST}:${INSTALL_PREFIX}/public/ > /dev/null 2>&1

ssh ${DEPLOY_USER}@${REMOTE_HOST} \
    "sudo chown -R ${DEPLOY_USER}:${DEPLOY_USER} ${INSTALL_PREFIX}/public && sudo chmod -R 755 ${INSTALL_PREFIX}/public"
echo "  ✓ Frontend assets deployed (rsync --delete)"

# ---------------------------------------------------------------------------
# Step 4: Regenerate .fiber.gz companion files on remote target
# ---------------------------------------------------------------------------
echo ""
echo "=== Refreshing Compressed Assets ==="
ssh ${DEPLOY_USER}@${REMOTE_HOST} \
    "find ${INSTALL_PREFIX}/public -type f ! -name '*.gz' | while read -r f; do gzip -n -c \"\$f\" > \"\${f}.fiber.gz\"; done 2>/dev/null || true"
echo "  ✓ .fiber.gz files regenerated on remote"

# ---------------------------------------------------------------------------
# Step 5: Restart systemd service on remote host
# ---------------------------------------------------------------------------
echo ""
echo "=== Restarting Service ==="
ssh ${DEPLOY_USER}@${REMOTE_HOST} \
    "sudo systemctl restart ${SERVICE_NAME} && sleep 2"

if ssh ${DEPLOY_USER}@${REMOTE_HOST} "sudo systemctl is-active --quiet ${SERVICE_NAME}"; then
    echo "  ✓ ${SERVICE_NAME} service started successfully on remote"
else
    echo "  ✗ Service failed to start. Check logs:"
    echo "      ssh ${DEPLOY_USER}@${REMOTE_HOST} 'sudo journalctl -u ${SERVICE_NAME} -n 20'"
    exit 1
fi

# ---------------------------------------------------------------------------
# Step 6: Verify deployment against remote host
# ---------------------------------------------------------------------------
echo ""
echo "=== Verification ==="
VERIFY_ERRORS=0

# Health check
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://${REMOTE_HOST}:${SERVER_PORT}/healthz 2>/dev/null || echo "000")
if [ "$HTTP_STATUS" = "200" ]; then
    echo "  ✓ /healthz (HTTP ${HTTP_STATUS})"
else
    echo "  ✗ /healthz FAILED (HTTP ${HTTP_STATUS})"
    VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
fi

# Critical frontend assets
for asset in "/index.html" "/app.js" "/go-board.html" "/styles/tailwind.min.css"; do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://${REMOTE_HOST}:${SERVER_PORT}${asset} 2>/dev/null || echo "000")
    if [ "$STATUS" = "200" ]; then
        echo "  ✓ ${asset} (HTTP ${STATUS})"
    else
        echo "  ✗ ${asset} FAILED (HTTP ${STATUS})"
        VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
    fi
done

# API check - boards endpoint returns data
BOARD_COUNT=$(curl -s http://${REMOTE_HOST}:${SERVER_PORT}/api/boards 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "?")
if [ "$BOARD_COUNT" != "?" ] && [ "$BOARD_COUNT" -gt 0 ]; then
    echo "  ✓ /api/boards returns ${BOARD_COUNT} board(s)"
else
    echo "  ! /api/boards returned unexpected result (may be normal on fresh start)"
fi

# App.js size check (not empty/stale)
APP_JS_SIZE=$(curl -s http://${REMOTE_HOST}:${SERVER_PORT}/app.js | wc -c)
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

VERSION_OUTPUT=$(ssh ${DEPLOY_USER}@${REMOTE_HOST} "${INSTALL_PREFIX}/bin/goban --version 2>/dev/null || echo '(unknown)'")
echo ""
echo "Version:        $VERSION_OUTPUT"
echo "Server binary:  ${INSTALL_PREFIX}/bin/goban (on ${REMOTE_HOST})"
echo "Frontend:       ${INSTALL_PREFIX}/public/ (on ${REMOTE_HOST})"
