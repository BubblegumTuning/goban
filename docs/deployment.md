# Deployment

Preferred production layout:

```
/opt/goban/
├── bin/           # goban, goban-cli, goban-user-cli
├── config/        # goban.toml
├── public/        # index.html, login.html, go-board.html, app.js, js/, styles/
└── data/          # goban.db (SQLite)
```

Ticket/board/activity **GET** APIs are unauthenticated (trusted LAN). Do not bind the listen port to a public interface unless that is intended.

Root `deploy.sh` is **deprecated** (prints instructions and exits 1). Use `scripts/`.

## Build

```bash
make dev                            # same as ./build.sh — native + public symlink
make build                          # binaries only — not enough to serve the UI
make package                        # binaries + public/ + .fiber.gz
make release                        # goban-vX.Y.Z-linux-amd64.tar.gz
make tarball TARGET_OS=darwin TARGET_ARCH=arm64
```

Use `make package` or `make release` before deploying. `make build` omits frontend assets.

## Option 1: clone and build on the host

```bash
git clone <repo-url> ~/goban
cd ~/goban
./build.sh
sudo ./scripts/build-and-deploy-local.sh
```

## Option 2: release tarball (no compiler on the host)

```bash
wget <releases>/goban-v1.2.0-linux-amd64.tar.gz
sudo ./scripts/deploy-local.sh goban-v1.2.0-linux-amd64.tar.gz
# or an already-extracted directory
sudo ./scripts/deploy-local.sh /path/to/extracted/goban-v1.2.0-linux-amd64/
```

`scripts/deploy-local.sh` installs binaries, rsyncs `public/` (deletes stale files), regenerates `.fiber.gz`, chowns to `goban` if that user exists, restarts systemd, checks health.

## Option 3: remote from a development machine

```bash
./scripts/deploy.sh 192.168.88.30 goban
```

Builds locally, rsync over SSH, verifies.

Script cheat sheet: [`scripts/README.md`](../scripts/README.md).

## Gitea Actions release

```bash
git tag -a v1.2.0 -m "Release v1.2.0"
git push origin master --tags
```

`.gitea/workflows/release.yml`: checkout with `fetch-depth: 0`, Go 1.25, `make release TARGET_OS=linux TARGET_ARCH=amd64`, upload tarball.

Manual: `make release TARGET_OS=linux TARGET_ARCH=amd64` then upload the tarball.

## System user and unit

```bash
sudo useradd -r -s /usr/sbin/nologin -c "Goban Kanban Service" goban
sudo cp systemd/goban.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goban
sudo systemctl status goban
journalctl -u goban -f
```

The unit sets `GOBAN_CONFIG_PATH=/opt/goban/config/goban.toml`, `GOBAN_STATIC_PATH=/opt/goban/public`, `WorkingDirectory=/opt/goban`, `ReadWritePaths=/opt/goban/data`, plus `NoNewPrivileges` and `ProtectSystem=strict`.

## Security (ops)

- Never commit real passwords. Direct env override is `DB_PASSWORD`. `${GOBAN_DB_PASSWORD}` works only if that name appears in the TOML file.
- Change `jwt_secret` from the example value.
- Service user `goban`; `goban-user-cli` creates users via the database (filesystem perms). HTTP has no user-create route.
- CORS defaults to same-origin (`http://localhost:<port>`); set `cors_origins` / `GOBAN_CORS_ORIGINS` if needed.

## MCP on a host

`mcp_enabled = true` in TOML starts stdio MCP **beside** HTTP. Stdio is only useful when something launches the process as an MCP child; a systemd unit that expects HTTP still binds the port. See [mcp.md](mcp.md).
