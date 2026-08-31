# Architecture

Goban is a single Go module. The HTTP server is `main.go`. Ticket writes go through `services/` (claim / move / release) with store transactions. The web UI is static files under `public/`.

## Binaries

| Binary | Source | Talks to |
|---|---|---|
| `goban` | `./` (`main.go`) | SQLite or PostgreSQL; serves API + `public/` |
| `goban-cli` | `goban-cli/` | HTTP API (Authorization header) |
| `goban-user-cli` | `goban-user-cli/` | Database files / Postgres directly |

Build all three with `./build.sh` or `make dev`. Output: `bin/` (git-ignored).

## Packages (server)

| Path | Role |
|---|---|
| `auth/` | API-token validation, JWT issue/verify |
| `config/` | TOML + env loading |
| `handlers/` | Fiber routes (tickets, boards, auth, admin, archive, comments, subtasks, links, runs, SSE, Go game) |
| `services/` | Claim / move / release / user; Go engine + scoring |
| `store/` | SQLite and PostgreSQL implementations, game memory store, fixtures |
| `middleware/` | Request ID and rate limiters |
| `sse/` | Server-Sent Events broadcaster |
| `mcp/` | Optional MCP stdio sidecar |
| `models/` | Ticket, user, activity, game types |
| `validation/` | Input limits and priority enum |
| `version/` | `version.Version` (ldflags at release) |

## Layout (repository)

```
goban/
├── main.go
├── goban.toml.example
├── Makefile, build.sh
├── docs/                 # these guides
├── public/               # index.html, login.html, go-board.html, app.js, styles/, js/
├── src/                  # Tailwind input (build pipeline)
├── systemd/goban.service
├── scripts/              # deploy + SQLite↔Postgres migration
├── deploy/               # example unit + toml + install.sh
├── migrations/           # extra SQL (startup still creates tables)
├── cmd/                  # create_test_user, db_check (not in main binary)
├── goban-cli/
├── goban-user-cli/
└── internal_docs/        # code-quality audits
```

`deploy.sh` at the repo root is deprecated (exits 1). Use `scripts/`.

## Request path

1. Fiber recover, CORS, request ID
2. Static files from `GOBAN_STATIC_PATH` / `static_path` / `<binary-dir>/public`
3. API routes (`handlers.RegisterRoutes`)
4. `GET /healthz`
5. SPA catch-all `GET /*` → `index.html` (skips `/api/`, `/styles/`, `/events`, `/healthz`)

If `mcp_enabled` is true, MCP starts in a goroutine; HTTP still listens.

## Dual URL prefixes

Ticket **reads** exist as both `/api/tickets` and `/api/v1/tickets`. Writes for claim/move/release are **only** `/api/v1/tickets/:id/{claim,move,release}`. Links and runs are registered twice (unversioned and `/api/v1/...`) with different GET auth — see [api.md](api.md).
