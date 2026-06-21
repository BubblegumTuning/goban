# Goban Makefile
# Build, package, and deploy targets.
#
# Targets:
#   make build          - Compile binaries only (used by CI/docker)
#   make package        - Full deployable artifact: bin/ + public/ + .fiber.gz
#   make tarball        - Create distributable .tar.gz from packaged output
#   make release        - Alias for tarball (CI workflow uses this)
#   make dev            - Local development build (runs ./build.sh)
#   make clean          - Remove all build artifacts

GO         ?= go
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS_SERVER  = -s -w -X goban/version.Version=$(VERSION) -X goban/version.BuildTime=$(BUILD_TIME)
LDFLAGS_CLI     = -s -w -X goban/version.Version=$(VERSION)

# Target OS/arch for cross-compilation (override via make TARGET_OS=linux TARGET_ARCH=amd64 ...)
TARGET_OS   ?= linux
TARGET_ARCH ?= amd64

RELEASE_DIR  := dist/$(VERSION)-$(TARGET_OS)-$(TARGET_ARCH)
TARBALL_NAME := goban-$(VERSION)-$(TARGET_OS)-$(TARGET_ARCH).tar.gz

.PHONY: all build package tarball release dev clean verify

all: package

# ---------------------------------------------------------------------------
# Build - compile binaries only (no frontend assets)
# Use 'make package' for a deployable artifact.
# ---------------------------------------------------------------------------

build: $(RELEASE_DIR)/bin/goban $(RELEASE_DIR)/bin/goban-cli $(RELEASE_DIR)/bin/goban-user-cli

$(RELEASE_DIR)/bin/goban: *.go go.mod go.sum
	@echo "[BUILD] Server binary ($(TARGET_OS)/$(TARGET_ARCH)) - v$(VERSION)"
	mkdir -p $(RELEASE_DIR)/bin
	GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) CGO_ENABLED=1 \
		$(GO) build -ldflags="$(LDFLAGS_SERVER)" -o $@ *.go

$(RELEASE_DIR)/bin/goban-cli: goban-cli/go.mod
	@echo "[BUILD] CLI binary ($(TARGET_OS)/$(TARGET_ARCH))"
	mkdir -p $(RELEASE_DIR)/bin
	(cd goban-cli && GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) \
		$(GO) build -ldflags="$(LDFLAGS_CLI)" -o ../$(RELEASE_DIR)/bin/goban-cli .)

$(RELEASE_DIR)/bin/goban-user-cli: goban-user-cli/go.mod
	@echo "[BUILD] User CLI binary ($(TARGET_OS)/$(TARGET_ARCH))"
	mkdir -p $(RELEASE_DIR)/bin
	(cd goban-user-cli && GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) \
		$(GO) build -ldflags="$(LDFLAGS_CLI)" -o ../$(RELEASE_DIR)/bin/goban-user-cli .)

# ---------------------------------------------------------------------------
# Package - full deployable artifact (binaries + frontend assets + .fiber.gz)
# This is the correct target to produce a complete deployment unit.
# ---------------------------------------------------------------------------

package: $(RELEASE_DIR)/.fiber-gz

$(RELEASE_DIR)/public: | build
	@echo "[COPY] Frontend assets"
	cp -r public/ $(RELEASE_DIR)/public/ 2>/dev/null || true

$(RELEASE_DIR)/deploy: | build
	@echo "[COPY] Deploy scripts"
	cp -r deploy/ $(RELEASE_DIR)/deploy/ 2>/dev/null || true

# Generate .fiber.gz companion files for all non-.gz assets in public/
$(RELEASE_DIR)/.fiber-gz: $(RELEASE_DIR)/bin/goban $(RELEASE_DIR)/public $(RELEASE_DIR)/deploy
	@echo "[COMPRESS] Generating .fiber.gz files"
	find $(RELEASE_DIR)/public -type f ! -name '*.gz' | while read -r f; do \
		gzip -n -c "$$f" > "$${f}.fiber.gz"; \
	done 2>/dev/null || true
	touch $@

# ---------------------------------------------------------------------------
# Tarball - distributable archive from packaged output
# ---------------------------------------------------------------------------

tarball: $(RELEASE_DIR)/.fiber-gz
	@echo "[TARBALL] Creating $(RELEASE_DIR)/$(TARBALL_NAME)"
	tar -czf $(RELEASE_DIR)/$(TARBALL_NAME) \
		-C $(RELEASE_DIR) \
		bin/ public/ deploy/
	@ls -lh $(RELEASE_DIR)/$(TARBALL_NAME)

release: tarball

# ---------------------------------------------------------------------------
# Local development build (native platform, with dev symlink)
# ---------------------------------------------------------------------------

dev:
	@./build.sh

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------

clean:
	rm -rf dist/ bin/goban bin/goban-cli bin/goban-user-cli goban goban-server *.tar.gz
