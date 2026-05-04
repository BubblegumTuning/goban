#!/bin/bash
# Build Goban binaries with version injection and deploy
# Usage: ./scripts/deploy.sh [remote-host] (optional deploy to remote)

set -e

REMOTE_HOST="${1:-}"
DEPLOY_USER="goban"

echo "=== Building Goban ==="

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-s -w -X goban/version.Version=${VERSION} -X goban/version.BuildTime=${BUILD_TIME}"

echo "  Version: ${VERSION}"
echo "  Build time: ${BUILD_TIME}"

mkdir -p bin

echo "[1/4] Building server..."
CGO_ENABLED=1 go build -ldflags="$LDFLAGS" -o bin/goban *.go
echo "      ✓ Server built"

echo "[2/4] Building CLI..."
cd goban-cli && go build -ldflags="-s -w -X goban/version.Version=${VERSION}" -o ../bin/goban-cli . || echo "      ⚠ CLI skipped"
cd ..

echo "[3/4] Building user CLI..."
cd goban-user-cli && go build -ldflags="-s -w -X goban/version.Version=${VERSION}" -o ../bin/goban-user-cli . || echo "      ⚠ User CLI skipped"
cd ..

if [ -z "$REMOTE_HOST" ]; then
  echo "[4/4] Dev symlink (local only)..."
  ln -sf ../public bin/public 2>/dev/null || true
else
  echo "[4/4] Prod mode - no dev symlink"
fi

echo "=== Build complete ==="

if [ -n "$REMOTE_HOST" ]; then
  echo "=== Deploying to $REMOTE_HOST ==="

  # Prepare remote directories and clean stale artifacts
  ssh ${DEPLOY_USER}@${REMOTE_HOST} "
    sudo mkdir -p /opt/goban/{bin,config,public/styles,data}
    sudo rm -f /opt/goban/public/public /opt/goban/public/static 2>/dev/null || true
    sudo rm -rf /opt/goban/public/styles/styles/ 2>/dev/null || true
    # Remove stale root-level CSS copies (should only live in styles/)
    sudo rm -f /opt/goban/public/font-awesome.min.css /opt/goban/public/font-awesome.min.css.fiber.gz
    sudo rm -f /opt/goban/public/tailwind.min.css /opt/goban/public/tailwind.min.css.fiber.gz
  "

  echo "[1/3] Deploying binary..."
  scp bin/goban ${DEPLOY_USER}@${REMOTE_HOST}:/opt/goban/bin/
  ssh ${DEPLOY_USER}@${REMOTE_HOST} "sudo chown goban:goban /opt/goban/bin/goban && sudo chmod +x /opt/goban/bin/goban"

  echo "[2/3] Deploying frontend assets..."
  # rsync handles incremental sync, deletions of stale files, and preserves structure
  # --no-dereference avoids following symlinks (e.g., public/public -> ../public)
  rsync -avz --delete --no-dereference public/ ${DEPLOY_USER}@${REMOTE_HOST}:/opt/goban/public/

  ssh ${DEPLOY_USER}@${REMOTE_HOST} "sudo chown -R goban:goban /opt/goban/public && sudo chmod -R 755 /opt/goban/public"

  echo "[3/3] Restarting service..."
  ssh ${DEPLOY_USER}@${REMOTE_HOST} "sudo systemctl restart goban && sleep 2"

  echo "=== Verification ==="
  ERRORS=0
  # Verify critical frontend assets are served correctly
  for asset in /app.js /index.html /go-board.html /styles/tailwind.min.css; do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://${REMOTE_HOST}:8080${asset})
    if [ "$STATUS" = "200" ]; then
      echo "  ✓ ${asset} (HTTP ${STATUS})"
    else
      echo "  ✗ ${asset} FAILED (HTTP ${STATUS})"
      ERRORS=$((ERRORS + 1))
    fi
  done

  # Verify API is responding with boards and columns
  BOARD_COUNT=$(curl -s http://${REMOTE_HOST}:8080/api/boards | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "?")
  if [ "$BOARD_COUNT" != "?" ] && [ "$BOARD_COUNT" -gt 0 ]; then
    echo "  ✓ /api/boards returns ${BOARD_COUNT} board(s)"
  else
    echo "  ✗ /api/boards returned unexpected result"
    ERRORS=$((ERRORS + 1))
  fi

  # Verify app.js contains critical functions (not empty/stale)
  APP_JS_SIZE=$(curl -s http://${REMOTE_HOST}:8080/app.js | wc -c)
  if [ "$APP_JS_SIZE" -gt 1000 ]; then
    echo "  ✓ app.js is ${APP_JS_SIZE} bytes (contains frontend logic)"
  else
    echo "  ✗ app.js appears empty or stale (${APP_JS_SIZE} bytes)"
    ERRORS=$((ERRORS + 1))
  fi

  if [ "$ERRORS" -gt 0 ]; then
    echo ""
    echo "=== Deployment completed with ${ERRORS} verification error(s) ==="
    exit 1
  else
    echo "=== All verifications passed — deployment successful ==="
  fi
else
  echo "Local dev ready. Run ./bin/goban --version to verify."
fi
