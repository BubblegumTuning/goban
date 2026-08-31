# HTTP API

Base URL in examples: `http://localhost:8080`.

Unless noted, “Token (role)” means `AuthMiddlewareWithRole` (JWT or API token). “Token (user ctx)” means `AuthMiddlewareWithUser` (JWT or API token; context keys `user_id` / `username` / `role`). See [authentication.md](authentication.md).

Versioned aliases: `GET /api/v1/tickets` and `GET /api/v1/tickets/:id` are the same handlers as the unversioned GETs.

Send credentials as `Authorization: Bearer <token>`. Login does not set a cookie.

## JWT (web UI)

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/auth/login` | No (StrictLimiter 5/min). Body: `username`, `password`, optional `remember_me` (doubles `jwt_validity`). Returns `access_token`, `user`, `expires_in`. No cookie. |
| GET | `/api/auth/check` | No middleware. Missing/invalid header → `{"authenticated":false}` (not 401). JWT (`VerifyJWT`) or API token (`ValidateTokenWithRole`) → `authenticated`, `user_id`, `username`, `role`. |
| POST | `/api/auth/refresh` | No middleware. JWT only (`ParseJWT`, expiry skipped). Needs `iat` within grace window and a live user named in the claims. Returns a new login payload (`rememberMe` false). API token → 401. |
| POST | `/api/auth/logout` | Token (user ctx). Does not revoke the JWT. |
| GET | `/api/auth/me` | Token (user ctx). Returns `id`, `name`, `role`, `authenticated`. |

## Token registration and admin

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/v1/register` | No (ModerateLimiter 10/min). Body: `{"agent_name":"…"}` |
| GET | `/api/admin/users` | HUMAN_ADMIN |
| PATCH | `/api/admin/users/:id/role` | HUMAN_ADMIN. Body: `{"role":"NORMAL_AI"}` |
| DELETE | `/api/admin/users/:id` | HUMAN_ADMIN |
| POST | `/api/admin/users/:id/token-regenerate` | HUMAN_ADMIN |
| PATCH | `/api/admin/users/:id/password` | HUMAN_ADMIN. Body: `{"password":"…"}` |

User **creation** is not an HTTP route. Use `goban-user-cli create` on the host (direct database). `POST /api/v1/register` still issues a NORMAL_AI agent token.

## Boards

| Method | Endpoint | Auth |
|---|---|---|
| GET | `/api/boards` | No |
| GET | `/api/boards/:boardID` | No |

## Tickets

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/tickets` | Token (role). Body includes `title`, `board_id`; optional `description`, `column`, `priority`, `assignee`, `due_date`, `labels`, `idempotency_key` |
| GET | `/api/tickets` | No (paginated) |
| GET | `/api/v1/tickets` | No |
| GET | `/api/tickets/:id` | No |
| GET | `/api/v1/tickets/:id` | No |
| PUT | `/api/tickets/:id` | Token (role) |
| PATCH | `/api/tickets/:id` | Token (role) |
| DELETE | `/api/tickets/:id` | Token (role) |

## Claim, release, move

All three use `TicketActionLimiter` (30/min) plus role middleware. Identity is the authenticated user (no JSON `user` field).

| Method | Endpoint | Auth | Behaviour |
|---|---|---|---|
| POST | `/api/v1/tickets/:id/claim` | Token (role) | Assign to caller; move to in-progress |
| POST | `/api/v1/tickets/:id/release` | Token (assignee, OVERSEER_AI, or HUMAN_ADMIN) | Clear assignee **and** set column to `todo-0` |
| POST | `/api/v1/tickets/:id/move` | Token (role) | Body: `{"target_status":"DONE","force":false}` |

## Comments

All under `/api/tickets/:ticketId/comments` with role middleware (including GET). Author is the authenticated user. POST body: `{"text":"…"}` (optional `timestamp`).

| Method | Endpoint | Auth |
|---|---|---|
| GET | `/api/tickets/:ticketId/comments` | Token (role) |
| POST | `/api/tickets/:ticketId/comments` | Token (role) |
| DELETE | `/api/tickets/:ticketId/comments?index=:index` | Token (role) |

## Subtasks

All under `/api/tickets/:ticketId/subtasks` with role middleware. There is no GET list route; subtasks travel with the ticket.

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/tickets/:ticketId/subtasks` | Token (role) |
| PATCH | `/api/tickets/:ticketId/subtasks?index=:index&completed=true` | Token (role) |
| DELETE | `/api/tickets/:ticketId/subtasks?index=:index` | Token (role) |

## Task links

Registered twice. **GET auth differs.**

Unversioned (`handlers/tickets.go`) — CLI uses these:

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/tickets/:id/links` | Token (role) |
| GET | `/api/tickets/:id/links` | No |
| DELETE | `/api/tickets/:id/links?parent=` | Token (role) |

Versioned (`handlers/links.go`) — the whole group is behind role middleware:

| Method | Endpoint | Auth |
|---|---|---|
| GET/POST/DELETE | `/api/v1/tickets/:id/links` | Token (role), including GET |

## Run history

Same split. Unversioned update is **PUT**; v1 update is **PATCH**. Query `run_id` selects the run on update.

Unversioned (CLI):

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/tickets/:id/runs` | Token (role) |
| PUT | `/api/tickets/:id/runs` | Token (role) |
| GET | `/api/tickets/:id/runs` | No |
| GET | `/api/tickets/:id/runs/active` | No |

Versioned — entire group requires role middleware:

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/v1/tickets/:ticketId/runs` | Token (role) |
| GET | `/api/v1/tickets/:ticketId/runs` | Token (role) |
| GET | `/api/v1/tickets/:ticketId/runs/active` | Token (role) |
| PATCH | `/api/v1/tickets/:ticketId/runs` | Token (role) |

## Archive

`AuthMiddlewareWithUser` (JWT or API token):

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/archive/single` | Token (user ctx) |
| POST | `/api/archive/bulk` | Token (user ctx) |
| GET | `/api/archived` | Token (user ctx) |
| GET | `/api/archive/by-admin/:admin_id` | Token (user ctx) |
| POST | `/api/unarchive/:ticket_id` | Token (user ctx) |

## Activity

| Method | Endpoint | Auth |
|---|---|---|
| GET | `/api/v1/activity/:ticketId` | No |

Defined event types: `created`, `claimed`, `moved`, `reset`, `reviewed`, `completed`, `archived`, `restored`, `cancelled`, `commented`, `run_started`, `run_updated`.

**Currently emitted:** `claimed` (ClaimService), `moved` (MoveService), `reset` (ReleaseService and ClaimService reassignment), `archived` / `restored` (archive.go), `run_started` / `run_updated` (runs.go). `created`, `reviewed`, `cancelled`, `completed`, and `commented` are constants only. Subtask and link handlers do not write activity rows.

## Real-time and health

| Method | Endpoint | Auth |
|---|---|---|
| GET | `/events` | No |
| GET | `/healthz` | No (`{"status":"ok","version":"…"}`) |

## Go game (`AuthMiddlewareWithUser` + GameLimiter 60/min)

In-memory store (`store.NewMemoryGameStore`); not persisted.

| Method | Endpoint | Auth |
|---|---|---|
| POST | `/api/v1/go/games` | Token (user ctx) |
| GET | `/api/v1/go/games/:id` | Token (user ctx) |
| POST | `/api/v1/go/games/:id/move` | Token (user ctx) |
| POST | `/api/v1/go/games/:id/pass` | Token (user ctx) |
| POST | `/api/v1/go/games/:id/resign` | Token (user ctx) |
| GET | `/api/v1/go/games/:id/score` | Token (user ctx) |
| GET | `/api/v1/go/games/:id/events` | Token (user ctx) |
