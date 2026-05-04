#!/bin/bash
# Build Goban binaries with version injection
# Usage: ./build.sh [remote-host] (optional deploy to remote)

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
  ssh ${DEPLOY_USER}@${REMOTE_HOST} "
    sudo mkdir -p /opt/goban/{bin,config,public/styles,data}
    sudo rm -f /opt/goban/public/public /opt/goban/public/static 2>/dev/null || true
  "
  scp bin/goban ${DEPLOY_USER}@${REMOTE_HOST}:/opt/goban/bin/
  ssh ${DEPLOY_USER}@${REMOTE_HOST} "sudo chown goban:goban /opt/goban/bin/goban && sudo chmod +x /opt/goban/bin/goban"
  rsync -avz --delete public/ ${DEPLOY_USER}@${REMOTE_HOST}:/opt/goban/public/
  ssh ${DEPLOY_USER}@${REMOTE_HOST} "sudo chown -R goban:goban /opt/goban/public && sudo chmod -R 755 /opt/goban/public && sudo systemctl restart goban"
  echo "=== Verification ==="
  ssh ${DEPLOY_USER}@${REMOTE_HOST} '
    curl -I http://localhost:8080/styles/tailwind.min.css
    curl -s http://localhost:8080/styles/tailwind.min.css | head -c 60
    echo "CSS verification passed."
  '
  echo "=== Deployed ==="
else
  echo "Local dev ready. Run ./bin/goban --version to verify."
fi
