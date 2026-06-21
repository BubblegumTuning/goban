# Goban Deployment Guide

## Directory Layout (Production)

```
/opt/goban/
├── bin/           # Compiled binaries
│   ├── goban          # Server binary
│   ├── goban-cli      # CLI tool
│   └── goban-user-cli # User management CLI
├── config/        # Configuration files
│   └── goban.toml
├── public/        # Frontend assets (HTML/CSS/JS)
│   ├── app.js     # Main frontend logic
│   ├── go-board.html
│   ├── index.html
│   ├── login.html
│   ├── js/
│   │   └── sortable.min.js
│   └── styles/
│       ├── font-awesome.min.css
│       └── tailwind.min.css
└── data/          # Database and runtime data
    └── goban.db
```

## Configuration Priority

1. **Environment variables** (highest priority)
   - `GOBAN_CONFIG_PATH` - Path to config file
   - `GOBAN_STATIC_PATH` - Path to static files directory
   - `GOBAN_PORT` - Server port
   - `DB_PATH`, `DB_TYPE`, etc.

2. **Config file** (`/opt/goban/config/goban.toml`)
   ```toml
   port = "8080"
   db_path = "/opt/goban/data/goban.db"
   static_path = "/opt/goban/public"
   ```

3. **Hardcoded defaults** (lowest priority)
   - Falls back to binary location for dev mode

## Deployment Options

### Option 1: Clone + Build on Target Host

The traditional approach — clone the repository and build directly on the server.

```bash
# On target host
git clone <repo-url> ~/goban
cd ~/goban
./build.sh                          # Native build with version injection
sudo ./scripts/deploy.sh            # Deploy to /opt/goban/
```

### Option 2: Makefile Build (Local or Remote)

Use the Makefile for cross-compilation and release tarball generation.

```bash
# Build for local platform
make dev                            # Same as ./build.sh — native build with dev symlink

# Cross-compile for linux/amd64 (default target)
make release                        # Produces goban-vX.Y.Z-linux-amd64.tar.gz

# Custom target
make tarball TARGET_OS=darwin TARGET_ARCH=arm64
```

The `make release` target:
1. Compiles server, CLI, and user-CLI binaries with version injection (`git describe --tags`)
2. Copies all frontend assets from `public/`
3. Generates `.fiber.gz` compressed companion files for SendFile optimization
4. Packages everything into a tarball

### Option 3: Release Tarball + Local Deploy Script (RECOMMENDED)

Download a prebuilt release and deploy it using the local deploy script. This is the fastest path to production with no build tools required on the target host.

```bash
# Download release tarball from Gitea Releases page
wget https://gitea.example.com/goban/releases/download/v1.2.3/goban-v1.2.3-linux-amd64.tar.gz

# Deploy (requires sudo)
sudo ./scripts/deploy-local.sh goban-v1.2.3-linux-amd64.tar.gz
```

The `deploy-local.sh` script:
1. Extracts the tarball to a temporary directory
2. Installs binaries (`goban`, `goban-cli`, `goban-user-cli`) to `/opt/goban/bin/`
3. Copies frontend assets to `/opt/goban/public/` (with stale file cleanup via rsync)
4. Regenerates `.fiber.gz` companion files
5. Sets ownership to the `goban` system user (if it exists)
6. Restarts the systemd service and verifies health

```bash
# Deploy from an already-extracted directory
sudo ./scripts/deploy-local.sh /path/to/extracted/goban-v1.2.3-linux-amd64/

# Dry run equivalent — verify source structure without deploying
tar -tzf goban-*.tar.gz | head -20
```

## Creating Releases

### Automated via Gitea Actions (Recommended)

Push an annotated tag to trigger the CI/CD pipeline:

```bash
# Tag and push — Gitea Actions builds + creates release automatically
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin master --tags
```

The workflow (`.gitea/workflows/release.yml`) will:
1. Checkout code with full history (`fetch-depth: 0` for version detection)
2. Set up Go 1.23 and build dependencies
3. Run `make release TARGET_OS=linux TARGET_ARCH=amd64`
4. Upload the resulting tarball to Gitea Releases

### Manual Release (Fallback)

```bash
# Build locally
make release TARGET_OS=linux TARGET_ARCH=amd64

# The tarball is in the current directory:
ls -lh goban-*-linux-amd64.tar.gz

# Upload manually via Gitea web UI or API
```

## Development Mode

Build and run locally:

```bash
cd ~/goban
make dev                            # Or: ./build.sh
cd bin && ./goban                   # Uses symlinks so public/ is found at ../public
```

## Production Deployment (Post-Deploy)

After deploying binaries to `/opt/goban/bin/`:

```bash
sudo systemctl enable goban
sudo systemctl start goban
sudo systemctl status goban
```

## Creating the Goban System User

Before deploying, create the service user on the target host:

```bash
# Create system user (no login shell, no home directory)
sudo useradd -r -s /usr/sbin/nologin -c "Goban Kanban Service" goban

# Verify creation
id goban
```

## Systemd Service

The service file (`systemd/goban.service`) configures:
- Working directory: `/opt/goban`
- Environment variables for paths
- Automatic restart on failure
- Security hardening (NoNewPrivileges, ProtectSystem)

Install it:
```bash
sudo cp systemd/goban.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable goban
```

## Static File Resolution Order

1. `GOBAN_STATIC_PATH` env var
2. `static_path` in config file
3. Fallback: `/opt/goban/public`
4. Dev fallback: `<binary-dir>/public`

## MCP Server

Set `mcp_enabled = true` (default) and `mcp_transport` in goban.toml. Stdio recommended for local agents.

