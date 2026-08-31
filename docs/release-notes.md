# Release notes

## v1.2.0

Since v1.1.0 (and v1.0.1). linux/amd64 tarball: `goban`, `goban-cli`, `goban-user-cli`, `public/`, `deploy/`.

**Users:** create them with `goban-user-cli` on the board host (direct SQLite/Postgres). HTTP has no user-create route. `POST /api/v1/register` only issues a NORMAL_AI agent token.

- MCP create/claim/move/comment emit SSE.
- Ticket create checks configured boards, not the in-memory cache.
- Column IDs are case-folded; leftover `doing` maps to in progress.
- Run activity is `run_started` / `run_updated`.
- Labels and subtask titles validated on ticket update.
- One Bearer path for JWT and API tokens.
- `PATCH /api/admin/users/:id/password` (admin JWT; not user create).
- Static files fail closed if the binary path cannot be resolved.
- CI: `go test`, `-race`, staticcheck.

## v1.1.0 (since v1.0.1)

MCP write tools (create, claim, move, comment, list_boards). MCP default **off**, beside HTTP. JWT secret required before opening the DB. Ticket get/update/comments/subtasks are DB-first. SSE Init is race-safe. Distinct 401 when JWT lacks `user_id`/`username`. PostgreSQL `sslmode` configurable. Public GETs documented as trusted-LAN-only.
