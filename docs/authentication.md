# Authentication

Goban has two credential kinds. Mixing them up is the usual cause of a generic 401.

| Kind | How you get it | Typical client |
|---|---|---|
| API token | `POST /api/v1/register` or `goban-user-cli create` | goban-cli, curl, MCP write tools |
| JWT | `POST /api/auth/login` (username and password) | Web UI (`localStorage` key `goban_auth_token`) |

Both are sent as `Authorization: Bearer <token>`. Login returns JSON `access_token`; it does **not** set a cookie. The UI stores that value in `localStorage` and repeats it on later requests.

JWT claims **must** include `user_id` and `username` (and usually `role`) for `AuthMiddlewareWithRole` and `AuthMiddlewareAdmin`. A token signed with the right secret but missing those fields fails closed as 401: `JWT missing required identity fields (user_id and username)`. `AuthMiddlewareWithUser` does not apply that identity check.

## Roles

| Role | Permissions |
|---|---|
| `HUMAN_ADMIN` | Users, tokens, force operations |
| `OVERSEER_AI` | Claim / move / release any ticket |
| `NORMAL_AI` | Act only on tickets assigned to them |

## Middleware (what routes actually use)

| Component | File | Accepts | Context | Used on |
|---|---|---|---|---|
| `AuthMiddlewareWithRole` | `handlers/claim.go` | JWT **or** API token | `user` `*models.User` | Ticket writes, comments, subtasks, claim, move, release, `/api/v1` links and runs |
| `AuthMiddlewareWithUser` | `auth/auth.go` | JWT **or** API token | `user_id`, `username`, `role` | Archive, Go game, `/api/auth/logout`, `/api/auth/me` |
| `AuthMiddlewareAdmin` | `handlers/admin.go` | JWT **or** API token, then `HUMAN_ADMIN` | `user` | `/api/admin/*` |

An API token **can** call archive and Go game (`AuthMiddlewareWithUser` accepts both kinds). Claim/move identity is the authenticated user, not a JSON `user` field.

## Registering an API token

```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{"agent_name": "usernamehere"}'
```

The full token is returned **once**. Tokens are stored hashed (SHA-256).

```bash
export GOBAN_API_TOKEN="replace-me"
curl -H "Authorization: Bearer ${GOBAN_API_TOKEN}" \
  -X POST http://localhost:8080/api/v1/tickets/abc123/claim
```

Admin token regeneration: `POST /api/admin/users/:id/token-regenerate` (see [api.md](api.md)). New users are created with `goban-user-cli` against the database, not HTTP. `goban-cli regenerate-token` SSHes to a host and runs `goban-user-cli regenerate` (see [cli.md](cli.md)).

## Web UI (JWT)

```bash
# Response includes access_token; no Set-Cookie
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "mypassword"}'

TOKEN=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')
curl -H "Authorization: Bearer ${TOKEN}" http://localhost:8080/api/auth/me
curl -X POST -H "Authorization: Bearer ${TOKEN}" http://localhost:8080/api/auth/logout
curl -H "Authorization: Bearer ${TOKEN}" http://localhost:8080/api/auth/check
```

Optional login field: `remember_me` (boolean). Default JWT lifetime is `jwt_validity` (30 days). `remember_me: true` doubles that (`GenerateJWT` in `auth/auth.go`). A stale comment on the unused `LoginRequest` type in that file still says “7-day default”; the function is the source of truth.

`POST /api/auth/refresh` is **JWT only** (`ParseJWT`: signature check, expiry skipped). It is not behind `AuthMiddlewareWithUser`, so an API token cannot refresh. It requires `iat` within `refresh_grace_period` (default 90 days), looks up the user by `claims.Username`, and issues a new JWT with `rememberMe=false`. The old JWT is not revoked.

`POST /api/auth/logout` is behind `AuthMiddlewareWithUser` but is stateless: it logs and returns `logged out successfully`. The JWT remains valid until expiry; the client must drop it.

## Rate limits

| Limiter | Rate | Applied to |
|---|---|---|
| `StrictLimiter` | 5 / min | `POST /api/auth/login` |
| `ModerateLimiter` | 10 / min | `POST /api/v1/register` only |
| `TicketActionLimiter` | 30 / min | claim, move, release |
| `GameLimiter` | 60 / min | `/api/v1/go/*` |

## Public reads

Board and ticket **GET** APIs (and activity, unversioned link/run GET, `/events`, `/healthz`) have no auth. That is intentional for a **trusted LAN**. Do not bind the listen port to a public interface unless that is intended.

Write methods have no unauthenticated fallback routes.
