# Goban Deployment Scripts

## Quick Reference

| What you want to do | Command |
|---|---|
| Local development build | `make dev` (or `./build.sh`) |
| Build + package for deployment | `make package` |
| Deploy locally (this machine) | `sudo ./scripts/build-and-deploy-local.sh` |
| Deploy remotely via SSH/rsync | `./scripts/deploy.sh <host> [user]` |
| Create distributable tarball | `make release` |

## Workflow

### Local Deployment (Production Machine)

1. **Clone/fetch latest code** (manual step):
   ```bash
   cd ~/goban
   git pull origin master
   ```

2. **Build and deploy in one command**:
   ```bash
   sudo ./scripts/build-and-deploy-local.sh
   ```

This single command:
- Validates source has both `bin/` AND `public/` (runs `make package` if needed)
- Creates `/opt/goban/{bin,config,public/styles,data}` directories
- Deploys binaries + frontend assets (rsync --delete removes stale files)
- Regenerates `.fiber.gz` companion files on target
- Restarts systemd service
- Verifies health check and all critical assets return HTTP 200

### Remote Deployment (from development machine)

```bash
./scripts/deploy.sh 192.168.88.30 goban
```

This builds locally, then deploys to the remote host via SSH/rsync with full verification.

## Build Targets (Makefile)

| Target | Description |
|---|---|
| `make build` | Compile binaries only (`bin/goban`, `bin/goban-cli`, `bin/goban-user-cli`) |
| `make package` | Full deployable artifact: binaries + public/ + .fiber.gz files |
| `make release` | Create distributable `.tar.gz` from packaged output (CI uses this) |
| `make dev` | Local development build with symlink to source `public/` |
| `make clean` | Remove all build artifacts |

**Important:** Use `make package`, not `make build`, when preparing for deployment. The `build` target only compiles Go binaries and does not include frontend assets. Deploying without frontend assets will result in a server that cannot serve the UI.

## Directory Structure (Production)

```
/opt/goban/
├── bin/
│   ├── goban           # Server binary
│   ├── goban-cli       # Admin CLI
│   └── goban-user-cli  # User CLI
├── config/
│   └── goban.toml      # Production configuration
├── data/               # Database files (SQLite)
└── public/             # Frontend assets
    ├── index.html
    ├── app.js
    ├── go-board.html
    ├── login.html
    └── styles/
        ├── tailwind.min.css
        ├── font-awesome.min.css
        └── *.fiber.gz  # Compressed companions
```

## Troubleshooting

**"public/ directory is missing or empty"** - You ran `make build` instead of `make package`. The deploy scripts require both binaries and frontend assets. Run `make package` first, then retry deployment.

**"Service failed to start"** - Check systemd logs:
```bash
sudo journalctl -u goban -n 20 --no-pager
```

**Frontend assets return 404 after deployment** - Verify the `static_path` in `/opt/goban/config/goban.toml` points to `/opt/goban/public`. The server reads this from config before falling back to the binary's directory.
