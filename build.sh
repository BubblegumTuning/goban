#!/bin/bash
# Build Goban binaries with version injection
# Usage: ./build.sh

set -e

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

echo "[4/4] Dev symlink (local only)... "
ln -sf ../public bin/public 2>/dev/null || true

# Regenerate .fiber.gz companion files for all non-.gz assets in public/ (recursive)
echo "=== Refreshing compressed assets ==="
find public/ -type f ! -name '*.gz' | while read -r f; do gzip -n -c "$f" > "${f}.fiber.gz"; done 2>/dev/null || true

echo "=== Build complete ==="
echo "Local dev ready. Run ./bin/goban --version to verify."
