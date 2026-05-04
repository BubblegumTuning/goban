# Goban

Lightweight Kanban board designed for Human + AI collaboration.

## Features

- ✅ Go backend with Fiber framework
- ✅ SQLite and PostgreSQL database support  
- ✅ Multiple boards (Human → AI, AI → Human)
- ✅ Drag-and-drop ticket management
- ✅ CLI tool for scripting/automation (`goban-cli`)
- ✅ User admin CLI for local server administration (`goban-user-cli`)
- ✅ Secure config with environment variable expansion (`${VAR}` syntax)
- ✅ Production-ready systemd service deployment to `/opt/goban`
- ✅ Bearer token authentication with role-based access control (RBAC)
- ✅ Activity logging / audit trail for all ticket operations
- ✅ Comments system on tickets
- ✅ Subtasks (checklist items within tickets)
- ✅ Archive/soft-delete functionality
- ✅ Priority levels, labels, and due dates

## Quick Start

### Build from source

```bash
./build.sh          # Builds both server and CLI into bin/ directory
cd bin && ./goban   # Start the server (default: port 8080, SQLite)
```

Binaries are placed in `bin/` which is git-ignored.

### Access the UI

Open your browser to: http://localhost:8080

## Authentication & Authorization

Goban uses two authentication systems:
- **Bearer token auth** for API/CLI access (token-based, role-controlled)
- **JWT session auth** for the web UI (cookie-based login/logout)

### Middleware Architecture

| Component | Location | Purpose | Status |
|-----------|----------|---------|--------|
| `AuthMiddleware` | `auth/auth.go` | Bearer token validation + user lookup | Active -- used by claim/move/release routes |
| `AuthMiddlewareWithRole` | `handlers/claim.go` | Token validation with role-based permissions | Active -- used by ticket CRUD, claim, move, release |
| `AuthMiddlewareWithUser` | `auth/auth.go` | JWT Bearer token + user context | Active -- used by archive routes |
| RequestID middleware | `middleware/request_id.go` | Per-request unique ID injection | Active -- registered in main.go |
| StrictLimiter | `middleware/rate_limiter.go` | 5 req/min brute-force protection on `/api/auth/login` | Active |
| ModerateLimiter | `middleware/rate_limiter.go` | 10 req/min on `/api/v1/register` and claim routes | Active |
| GameLimiter | `middleware/rate_limiter.go` | 60 req/min on `/api/v1/go/*` endpoints | Active |

### Roles

| Role | Permissions |
|------|-------------|
| `HUMAN_ADMIN` | Full control: create users, set privileges, force any operation |
| `OVERSEER_AI` | Can claim/move/release ANY ticket across all boards |
| `NORMAL_AI` | Can only act on tickets they own (assigned to them) |

### Registering a Token

Create your first token via the registration endpoint:

```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "agent_name": "usernamehere"
  }'
```

Response includes your token (shown once):
```json
{
  "id": 1,
  "agent_name": "usernamehere",
  "token_name": "goban-abc123def456",
  "token": "***",
  "user_id": 1,
  "created_at": "2024-01-15T10:30:00Z"
}
```

**Important:** Save the token immediately. It is never returned again for security.

### Using Your Token

Include it in the `Authorization` header for authenticated endpoints:

```bash
export MY_TOKEN="***"
curl -H "Authorization: Bearer $MY_TOKEN" http://localhost:8080/api/v1/tickets/abc123/claim \
  -d '{"user":"usernamehere"}'
```

### Admin Operations (Human Only)

Admin endpoints require `HUMAN_ADMIN` role and are under `/api/admin`:

```bash
# Create a new user with OVERSEER_AI role
curl -X POST http://localhost:8080/api/admin/users \
  -H "Authorization: Bearer ***" \
  -H "Content-Type: application/json" \
  -d '{"username": "overseer-bot", "role": "OVERSEER_AI"}'

# List all users
curl -H "Authorization: Bearer ***" http://localhost:8080/api/admin/users

# Update a user's role (user_id=2)
curl -X PATCH http://localhost:8080/api/admin/users/2/role \
  -H "Authorization: Bearer ***" \
  -d '{"role": "NORMAL_AI"}'

# Delete a user
curl -X DELETE http://localhost:8080/api/admin/users/2 \
  -H "Authorization: Bearer ***"

# Regenerate a token (if compromised)
curl -X POST http://localhost:8080/api/admin/users/1/token-regenerate \
  -H "Authorization: Bearer ***"
```

> **Note:** Token self-registration is available at `POST /api/v1/register`. Admin token regeneration via `/api/admin/users/:id/token-regenerate` is the only other token management endpoint.

### Web UI Authentication (JWT)

The web UI uses JWT-based session authentication for browser access:

```bash
# Login with username and password
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "mypassword"}'

# Check auth status (returns user info or null)
curl http://localhost:8080/api/auth/check

# Get current user profile (requires JWT cookie)
curl -b cookies.txt http://localhost:8080/api/auth/me

# Logout
curl -X POST -b cookies.txt http://localhost:8080/api/auth/logout
```

## CLI Tools

### goban-cli (Ticket Management)

The `goban-cli` tool allows programmatic ticket management via the API:

#### Setup

First, configure the CLI (create `~/.goban/config.yaml`):

```yaml
api:
  base_url: "http://localhost:8080"
  api_token: ""
  timeout: 30

output:
  format: "line"   # or "json", "table"
  colorize: true

retry:
  max_attempts: 3
  initial_delay: 1
  backoff_multiplier: 2
```

Or use environment variables directly:

```bash
export GOBAN_API_BASE_URL="http://localhost:8080"
export GOBAN_BOARD="human-to-ai"
export GOBAN_USER="yourname"
export GOBAN_API_TOKEN="***"
```

#### Common Commands

```bash
# List available boards
./bin/goban-cli list-boards

# Create a ticket
./bin/goban-cli create --board human-to-ai \
  --title "Fix bug #123" \
  --description "Details here..." \
  --column todo

# Claim a ticket (moves to In Progress)
./bin/goban-cli claim <ticket-id> --user yourname

# Release a ticket back to TODO
./bin/goban-cli release <ticket-id>

# Move ticket between columns
./bin/goban-cli move <ticket-id> done

# List tickets in a column
./bin/goban-cli list-tickets --status todo

# List tickets available to claim (not yet claimed)
./bin/goban-cli list-available

# View all your currently claimed tickets
./bin/goban-cli my-tickets

# Update a ticket's description
./bin/goban-cli update-description <ticket-id> "New description"

# View ticket details
./bin/goban-cli view <ticket-id>

# Delete a ticket
./bin/goban-cli delete <ticket-id>

# Batch move tickets to DONE or CANCELLED
./bin/goban-cli batch-done abc123 def456 ghi789
./bin/goban-cli batch-cancel abc123 def456

# Session-based workflow (set active ticket, then advance it)
./bin/goban-cli done        # Move active ticket to DONE
./bin/goban-cli review      # Move active ticket to REVIEW
./bin/goban-cli todo         # Return active ticket to TODO
./bin/goban-cli backlog     # Return active ticket to BACKLOG
./bin/goban-cli cancel      # Move active ticket to CANCELLED
```

#### CLI Commands Requiring Token

For operations that require authentication (claim, move, ticket CRUD), set your token:

```bash
export GOBAN_API_TOKEN="***"

# List all tickets (requires auth)
./bin/goban-cli list-tickets

# Claim a ticket
./bin/goban-cli claim abc123 --user usernamehere

# Move a ticket between columns  
./bin/goban-cli move abc123 done

# Add comments
./bin/goban-cli comment abc123 --message "Investigating..."

# List comments on a ticket
./bin/goban-cli list-comments abc123

# Regenerate your API token (if compromised)
./bin/goban-cli regenerate-token

# Manage subtasks (via API directly)
curl -X POST http://localhost:8080/api/tickets/abc123/subtasks \
  -H "Authorization: Bearer ***" \
  -d '{"title": "Step 1"}'

curl -X PATCH "http://localhost:8080/api/tickets/abc123/subtasks?index=0&completed=true" \
  -H "Authorization: Bearer ***"
```

---

### goban-user-cli (User Administration)

The `goban-user-cli` tool provides **direct database access** for user administration. Unlike `goban-cli`, it does NOT require API authentication tokens — it operates with filesystem/database permissions only. This makes it ideal for local server administration where the web interface may not be accessible.

#### Key Differences from goban-cli

| Feature | goban-cli | goban-user-cli |
|---------|-----------|----------------|
| Access Method | HTTP API calls | Direct database access |
| Authentication | Requires Bearer token | No auth required (DB permissions only) |
| Use Case | Remote/scripted ticket management | Local server user administration |
| Config File | `~/.goban/config.yaml` | None (uses env vars or discovers goban.toml) |

#### Configuration

No config file needed. The tool reads database settings from:

1. **Environment variables** (highest priority):
   ```bash
   export DB_TYPE=sqlite       # or "postgres"
   export DB_PATH=./goban.db   # SQLite path
   # For PostgreSQL:
   export DB_HOST=localhost
   export DB_PORT=5432
   export DB_USER=goban
   export DB_PASSWORD=***
   export DB_NAME=goban
   ```

2. **Auto-discovered goban.toml** (falls back to these locations):
   - `/opt/goban/config/goban.toml`
   - `/etc/goban/goban.toml`
   - `~/goban/goban.toml`
   - `./goban.toml`

3. **Defaults**: SQLite at `./goban.db`

#### Usage Examples

```bash
# List all users
./bin/goban-user-cli list

# Create a new user with role and password
./bin/goban-user-cli create --username=usernamehere-ai --role=NORMAL_AI --password="secret123"

# Update a user's role (user_id=1)
./bin/goban-user-cli update 1 --role=OVERSEER_AI

# Reset a user's password
./bin/goban-user-cli reset-password 1 --password="newpass"

# Regenerate a compromised token
./bin/goban-user-cli regenerate 1

# Delete a user
./bin/goban-user-cli delete 1

# Force delete even if user has tickets
./bin/goban-user-cli delete 1 --force
```

#### Role Options

- `HUMAN_ADMIN` — Full control over all users and tickets
- `OVERSEER_AI` — Can manage any ticket, read-only on users  
- `NORMAL_AI` — Can only claim/manage own tickets

#### Security Note

The tool requires write permissions to the database file. On production systems:
- Run as a privileged user or member of the `goban` group
- Ensure proper filesystem permissions (`chmod 640 goban.db`, owned by `goban:goban`)
- Never expose this tool over network — it bypasses all API authentication

---

## Configuration

### Environment Variables (Highest Priority)

```bash
export GOBAN_PORT=8081
export GOBAN_CONFIG_PATH=/etc/goban/goban.toml  # Optional config path override
export GOBAN_STATIC_PATH=/opt/goban/static      # Optional static files path
export DB_TYPE=postgres                          # or "sqlite"
export DB_HOST=localhost
export DB_USER=goban
export DB_PASSWORD=***    # Required for PostgreSQL
export DB_NAME=goban
export GOBAN_JWT_SECRET="your-secret-key"        # JWT signing secret (critical for production)
export LOG_LEVEL=info                             # debug, info, warn, error
export GOBAN_DEBUG=true                           # Enable verbose debug logging
./bin/goban
```

### Config File (goban.toml)

The config file supports `${VAR}` expansion:

```toml
port = "8080"
db_path = "./goban.db"
db_type = "sqlite"  # or "postgres"
static_path = "/opt/goban/static"
log_level = "info"   # debug, info, warn, error (controls log verbosity)
debug = false         # Enable verbose debug logging (gates DEBUG log.Printf calls)

# JWT signing secret for web UI authentication (critical - change in production!)
jwt_secret = "${GOBAN_JWT_SECRET}"

# PostgreSQL settings (when db_type = "postgres")
db_host = "${DB_HOST}"
db_port = 5432
db_user = "${DB_USER}"
db_password = "${GOBAN_DB_PASSWORD}"  # Use env var for security!
db_name = "goban"
```

**Configuration Priority:** Environment variables > TOML config file > Hardcoded defaults

The application uses two configuration resolution paths:

1. **`main.go getConfigPath()`** (used by default): Searches these locations in order:
   - `$GOBAN_CONFIG_PATH` environment variable
   - `/opt/goban/config/goban.toml` (production)
   - `/etc/goban/goban.toml` (system-wide)
   - `~/goban/goban.toml` or `./goban.toml` (development)

2. **`config.ResolveConfigPath()`** (alternative): Searches these locations:
   - `$GOBAN_CONFIG` environment variable
   - `~/.goban/goban.toml` (persistent user config)
   - `./goban.toml` (local fallback)

**Security Note:** Never commit config files with hardcoded passwords. See `.env.example` for required environment variables. The `jwt_secret` must be changed from its default value in production environments to prevent JWT forgery attacks.

## Database Setup

### SQLite (Default - No Setup Required)

Just run `./bin/goban` and data is stored in `goban.db`.

### PostgreSQL

1. Create database:
   ```sql
   CREATE DATABASE goban OWNER goban;
   ```

2. Set environment variables or update config file with DB credentials

3. Run migration (handled automatically on startup):
   ```bash
   ./bin/goban  # Will create tables if they don't exist
   ```

## Project Structure

```
goban/
├── main.go              # Application entry point (with path discovery)
├── middleware.go        # DebugLogger middleware (auth removed — see auth/)
│
├── middleware/          # HTTP middleware package
│   ├── rate_limiter.go  # Rate limiting (StrictLimiter on login, ModerateLimiter on register/claim, GameLimiter on Go endpoints)
│   └── request_id.go    # Per-request unique ID injection
├── go.mod/go.sum        # Go module files
├── goban.toml.example   # Example configuration file with default settings
├── build.sh             # Build script (outputs to bin/)
├── DEPLOYMENT.md        # Production deployment guide
│
├── auth/
│   └── auth.go          # Token validation, user lookup, JWT middleware
│
├── config/
│   ├── config.go           # Config loading with env var support
│   ├── .env.example        # Environment variable template
│   ├── prod-goban.toml     # Production config example
│   ├── prod-config.yaml    # Production YAML config alternative
│   ├── config.postgres.yaml # PostgreSQL-specific test config
│   └── sqlite_test.yaml    # SQLite test config
│
├── handlers/
│   ├── handlers.go      # Route registration, board state management
│   ├── tickets.go       # Ticket CRUD endpoints (requires auth)
│   ├── boards.go        # Board listing and retrieval
│   ├── claim.go         # Claim ticket endpoint + AuthMiddlewareWithRole definition
│   ├── release.go       # Release/unassign ticket endpoint
│   ├── moves.go         # Move ticket between columns (v1 with permissions)
│   ├── admin.go         # Admin user management endpoints
│   ├── comments.go      # Comment CRUD operations
│   ├── subtasks.go      # Subtask CRUD operations
│   ├── archive.go       # Archive/unarchive tickets (single + bulk)
│   ├── activity.go      # Activity log retrieval
│   ├── register.go      # Token self-registration endpoint
│   ├── auth.go          # JWT login/logout/me/check/refresh routes
│   ├── go_game.go       # Go game board HTTP handlers (/api/v1/go/* routes)
│   └── sse.go           # Server-sent events for real-time updates
│
├── validation/
│   └── validator.go     # Input validation helpers (length limits, priority enum)
│
├── models/
│   ├── ticket.go        # Ticket, Board, Column data structures
│   ├── types.go         # User, AgentToken, ActivityLog, Comment, Subtask
│   └── game.go          # Game state data structures for Go game feature
│
├── services/
│   ├── claim.go         # Claim service with atomic transactions
│   ├── move.go          # Move service with permission checks
│   ├── release.go       # Release service with validation
│   ├── user.go          # User CRUD operations
│   ├── errors.go        # Shared error definitions (e.g. ErrArchived)
│   ├── go_engine.go     # Go game engine logic (move validation, ko rules)
│   └── go_scoring.go    # Territory scoring implementation
│
├── store/
│   ├── interface.go     # Store interfaces (PaginatedStore, TicketStore)
│   ├── sqlite_store.go  # SQLite implementation
│   ├── postgres_store.go  # PostgreSQL implementation
│   ├── store.go         # Factory function
│   ├── helpers.go       # Utility functions
│   ├── game_store.go    # GameStore interface definition
│   └── game_memory_store.go # In-memory game store implementation
│
├── sse/
│   └── sse.go           # Server-Sent Events broadcaster
│
├── version/
│   └── version.go       # Version constants (--version flag support)
│
├── migrations/          # SQL migration files for schema changes
│   ├── 002_add_password_hash.sql
│   ├── 003_add_password_to_users.sql
│   ├── drop_tags_column.sql
│   └── normalize_column_values.sql
│
├── cmd/                 # Utility commands (not built into main binary)
│   ├── create_test_user/main.go  # Create test user for development/testing
│   └── db_check/main.go          # Database health check utility
│
├── testutil/
│   └── mock_store.go    # Mock store for testing
│
├── deploy.sh            # Quick deployment script (root-level)
│
├── scripts/             # Deployment & migration utilities
│   ├── README.md                     # Migration scripts guide
│   ├── deploy.sh                     # Manual deployment script
│   ├── migrate_sqlite_to_postgres.py # SQLite → PostgreSQL migration
│   └── rollback_postgres_to_sqlite.py # Rollback to SQLite
│
├── systemd/
│   └── goban.service    # Systemd service unit file
│
├── bin/                 # Build artifacts (git-ignored)
│   ├── goban            # Server binary
│   ├── goban-cli        # CLI tool binary
│   └── goban-user-cli   # User admin CLI binary
│
├── goban-cli/           # CLI source code (API-based)
│   ├── main.go
│   ├── README.md         # CLI-specific usage guide
│   ├── cmd/             # Command implementations
│   │   ├── batch.go       # Batch operations (batch-done, batch-cancel)
│   │   ├── claim.go       # Claim a ticket
│   │   ├── comment.go     # Add/list comments
│   │   ├── create.go      # Create new ticket
│   │   ├── delete.go      # Delete a ticket
│   │   ├── list_available.go# List tickets available to claim
│   │   ├── list_boards.go # List all boards
│   │   ├── list_tickets.go# List all tickets
│   │   ├── move.go        # Move ticket between columns
│   │   ├── my_ticket.go   # Show your claimed tickets
│   │   ├── release.go     # Release a ticket back to TODO
│   │   ├── session_commands.go # Session workflow (done/review/todo/backlog/cancel)
│   │   ├── regenerate_token.go # Regenerate API token command
│   │   ├── update_description.go # Update ticket description
│   │   └── view.go        # View full ticket details
│   └── internal/        # Internal packages (client, config)
│
├── goban-user-cli/      # User admin CLI source (direct DB access)
│   ├── main.go
│   ├── cmd/             # Command implementations
│   │   ├── create.go          # Create user with role/password
│   │   ├── delete.go          # Delete user (--force option)
│   │   ├── list.go            # List all users
│   │   ├── regenerate.go      # Regenerate agent token
│   │   ├── reset_password.go  # Reset user password (bcrypt)
│   │   └── update.go          # Update user role
│   └── internal/        # Internal packages (client, config, output)
│
├── public/              # Frontend assets (served as static files)
│   ├── index.html       # Main web UI (single-page app)
│   ├── login.html       # Login page
│   ├── go-board.html    # Go game board UI
│   ├── app.js           # Frontend JavaScript (SPA logic)
│   ├── js/              # Third-party JS libraries
│   │   └── sortable.min.js
│   └── styles/          # CSS files (Tailwind, Font Awesome)
│
├── src/                 # Frontend source files (CSS build pipeline)
│   └── input.css        # Tailwind CSS input file
│
├── static/css/          # Compiled frontend assets
│   └── tailwind.css     # Compiled Tailwind output
│
├── package.json         # Node.js dependencies (Tailwind CLI, etc.)
├── tailwind.config.js   # Tailwind CSS configuration
│
└── data/                # Local development database files (git-ignored)
    └── goban.db
```

## Deployment

### Local Development

```bash
./build.sh
cd bin && ./goban  # Uses ../public for static files via symlink
```

### Production with Systemd (Recommended)

See [DEPLOYMENT.md](DEPLOYMENT.md) for full details.

**Directory Layout:**
```
/opt/goban/
├── bin/           # Compiled binary
│   └── goban
├── config/        # Configuration files
│   └── goban.toml
├── static/        # Frontend assets (HTML/CSS/JS)
│   ├── index.html
│   └── styles/
└── data/          # Database and runtime data
    └── goban.db
```

**Manual Deploy:**
```bash
./scripts/deploy.sh <hostname>

# On the server:
sudo systemctl enable --now goban
sudo systemctl status goban
journalctl -u goban -f  # View logs
```

## Security Considerations

- ⚠️ Never commit config files with real passwords
- ✅ Use environment variables: `${GOBAN_DB_PASSWORD}` in config files
- ✅ Set `DB_PASSWORD` or `GOBAN_DB_PASSWORD` env var before starting server
- ✅ `.gitignore` excludes config files and build artifacts
- ✅ Production service runs as unprivileged user `goban` (systemd)
- ✅ Systemd service includes security hardening (NoNewPrivileges, ProtectSystem)
- ✅ Bearer tokens are hashed (SHA256) in database; full token shown only once at creation
- ✅ Role-based access control prevents unauthorized ticket manipulation
- ⚠️ `goban-user-cli` bypasses API auth — ensure proper filesystem permissions on production

## Activity Logging / Audit Trail

All ticket operations are logged for audit purposes:

**Event Types (defined):** `created`, `claimed`, `moved`, `reset`, `reviewed`, `completed`, `archived`, `cancelled`, `commented`

**Currently logged:** `claimed` (via ClaimService), `moved` (via MoveService), `reset` (via ReleaseService), and `archived`/`restored` (directly in archive.go handlers). The remaining event types (`created`, `reviewed`, `completed`, `cancelled`, `commented`) are defined as constants but not yet emitted. Subtask operations (add/update/delete) do not generate activity logs.

**Retrieve Activity Log:**
```bash
# Get all activity for a ticket (note: /api/v1/activity/:ticketId)
curl http://localhost:8080/api/v1/activity/abc123

# Response includes actor, event type, previous/new state, timestamp, metadata
[
  {
    "id": 1,
    "ticket_id": "abc123",
    "event_type": "created",
    "actor": "usernamehere",
    "created_at": "2024-01-15T10:30:00Z"
  },
  {
    "id": 2,
    "ticket_id": "abc123", 
    "event_type": "claimed",
    "actor": "usernamehere",
    "new_state": "inprogress-0",
    "created_at": "2024-01-15T10:35:00Z"
  }
]
```

## API Endpoints

### JWT Authentication (Web UI)

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/auth/login` | Login with username/password, returns JWT cookie | No |
| GET | `/api/auth/check` | Check current auth status | No |
| POST | `/api/auth/refresh` | Refresh JWT within grace period window | Existing token |
| POST | `/api/auth/logout` | Logout (destroy session) | JWT |
| GET | `/api/auth/me` | Get current user profile | JWT |

### Token Registration & User Management

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/v1/register` | Register new agent token (creates user + token) | No |
| POST | `/api/admin/users` | Create new user with specified role | HUMAN_ADMIN |
| GET | `/api/admin/users` | List all users (no tokens exposed) | HUMAN_ADMIN |
| PATCH | `/api/admin/users/:id/role` | Update user's role | HUMAN_ADMIN |
| DELETE | `/api/admin/users/:id` | Delete user (cascades to tokens) | HUMAN_ADMIN |
| POST | `/api/admin/users/:id/token-regenerate` | Force token regeneration | HUMAN_ADMIN |

### Board Operations

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/boards` | List all boards | No |
| GET | `/api/boards/:boardID` | Get board state (all columns + tickets) | No |

### Ticket Operations

> **Note:** All ticket CRUD operations require a Bearer token with role-based access control when accessed through the authenticated route group (`/api/tickets` under `AuthMiddlewareWithRole`). However, production-compatible fallback routes exist at `/api/tickets`, `/api/tickets/:id` (DELETE/PATCH), and `/api/tickets/:id/release` that bypass authentication middleware entirely. These are intentional for web UI compatibility but represent a security consideration for deployments requiring strict auth enforcement.

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/tickets` | Create new ticket | Token (role-based) via authenticated group; unauthenticated fallback also exists |
| GET | `/api/tickets` | List all tickets (paginated) | Token (role-based) |
| GET | `/api/tickets/:id` | Get single ticket details | Token (role-based) |
| PUT | `/api/tickets/:id` | Update ticket fields (full replace) | Token (role-based) |
| PATCH | `/api/tickets/:id` | Update ticket fields (partial update) | Token (role-based) via authenticated group; unauthenticated fallback also exists |
| DELETE | `/api/tickets/:id` | Delete ticket permanently | Token (role-based) via authenticated group; unauthenticated fallback also exists |

### Claim, Release & Move Operations

All require Bearer token with role-based permissions:

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/v1/tickets/:id/claim` | Claim ticket (assign + move to inprogress) | Token (role-based) |
| POST | `/api/v1/tickets/:id/release` | Release ticket (unassign, keep column) | Token (owner/admin) |
| POST | `/api/v1/tickets/:id/move` | Move ticket between columns | Token (role-based) |

### Comments

All comment routes require Bearer token authentication (`AuthMiddlewareWithRole`):

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/tickets/:ticketId/comments` | List all comments on a ticket | Token (role-based) |
| POST | `/api/tickets/:ticketId/comments` | Add new comment to ticket | Token (role-based) |
| DELETE | `/api/tickets/:ticketId/comments?index=:index` | Delete comment by index | Token (role-based) |

### Subtasks

All subtask routes require Bearer token authentication (`AuthMiddlewareWithRole`):

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/tickets/:ticketId/subtasks` | Add new subtask to ticket | Token (role-based) |
| PATCH | `/api/tickets/:ticketId/subtasks?index=:index&completed=true` | Update subtask | Token (role-based) |
| DELETE | `/api/tickets/:ticketId/subtasks?index=:index` | Delete subtask by index | Token (role-based) |

### Archive Operations

All archive routes require JWT authentication (`AuthMiddlewareWithUser`):

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/archive/single` | Archive a single ticket | JWT |
| POST | `/api/archive/bulk` | Bulk archive multiple tickets | JWT |
| GET | `/api/archived` | List all archived tickets | JWT |
| GET | `/api/archive/by-admin/:admin_id` | Archived tickets by admin ID | JWT |
| POST | `/api/unarchive/:ticket_id` | Restore ticket from archive | JWT |

### Activity & Audit

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/v1/activity/:ticketId` | Get activity log for a ticket | No (public) |

### Real-Time Updates (SSE)

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/events` | Server-Sent Events stream for real-time updates | No |

### Health Check

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/healthz` | Service health check endpoint | No |

### Go Game Endpoints

All Go game routes have rate limiting (60 req/min via `GameLimiter`):

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/v1/go/games` | Create a new Go game | No |
| GET | `/api/v1/go/games/:id` | Get game state | No |
| POST | `/api/v1/go/games/:id/move` | Play a move | No |
| POST | `/api/v1/go/games/:id/pass` | Pass a turn | No |
| POST | `/api/v1/go/games/:id/resign` | Resign the game | No |
| GET | `/api/v1/go/games/:id/score` | Get territory score | No |
| GET | `/api/v1/go/games/:id/events` | SSE stream for game events | No |

## Changelog of Documentation Audits

- 2026-05-04: Removed dead code — deleted `handlers/nested.go` (empty stub, content migrated to comments.go/subtasks.go). Cleaned up `.gitignore` dead rules (`!.gobanrc`, `!goban-server/`). Removed "Known Dead Code" section from README (no longer needed).
- 2026-05-04: Cleaned up dead artifacts — removed root index.html (accidental dev build artifact, app serves public/index.html) and orphaned public/go-board.js.fiber.gz (source go-board.js does not exist). Fixed hardcoded username in goban.toml.example ("nanami" → "goban"). Added goban.toml.example to README project structure.
- 2026-05-04: Fixed rate limiter status (all three limiters are active, not dead code). Updated activity logging section to include archived/restored events. Fixed CLI comment flag examples (`--text --who` → `--message`). Added TODO marker to nested.go stub for future removal. Corrected DEPLOYMENT.md static file reference (`style.css` → `tailwind.min.css`). Removed references to deleted `handlers/tokens.go` and non-existent `handlers/index.go`. Added Go game API endpoints, models, services, and store files. Fixed comments/subtasks auth requirements (require token, not public). Added missing CLI commands (`regenerate-token`, `list-comments`). Added `/api/auth/refresh` endpoint. Removed Ansible deploy section (playbook not in repo). Rewrote goban-cli README to match actual commands.
- 2026-04-30: README audited against codebase. Added middleware architecture table, missing CLI commands (batch-done, batch-cancel, release, session workflow), validation/ package, config YAML files, services/errors.go, scripts/README.md, deploy.sh root-level script, goban-cli/README.md reference. Corrected token management dead code endpoints. Noted rate limiter as unused.
