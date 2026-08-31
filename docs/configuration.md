# Configuration

Priority: **environment variables > TOML file > hardcoded defaults**.

The HTTP process refuses to start if `jwt_secret` / `GOBAN_JWT_SECRET` is empty (`config.RequireJWTSecret`).

## File search (`config.ResolveConfigPath`)

Used by `config.LoadConfig("")` from `main.go`:

1. `$GOBAN_CONFIG`
2. `$GOBAN_CONFIG_PATH`
3. `/opt/goban/config/goban.toml` (if present)
4. `/etc/goban/goban.toml` (if present)
5. `~/.goban/goban.toml` (if present)
6. `~/goban/goban.toml` (if present)
7. `./goban.toml`

A leading `~` is expanded.

There is a single resolver. Do not confuse CLI config (`~/.goban/goban-cli/config.yaml`) with the server TOML.

## TOML keys

See `goban.toml.example` at the repo root.

| Key | Default | Notes |
|---|---|---|
| `port` | `8080` | |
| `debug` | `false` | gates DEBUG `log.Printf`. TOML `debug = true` turns it on; env `GOBAN_DEBUG=true` also turns it on (cannot turn it off from env if TOML set it) |
| `log_level` | `info` | `debug`, `info`, `warn`, `error` |
| `db_path` | `./goban.db` | SQLite |
| `db_type` | `sqlite` | or `postgres` |
| `db_host` / `db_port` / `db_user` / `db_password` / `db_name` | localhost / 5432 / goban / empty / goban | Postgres |
| `db_sslmode` | empty (store default `disable`) | `GOBAN_DB_SSLMODE` or `DB_SSLMODE` |
| `static_path` | empty | used if `GOBAN_STATIC_PATH` is unset |
| `jwt_secret` | empty | **required** to listen |
| `jwt_validity` | 30 days | Go duration, e.g. `72h` |
| `refresh_grace_period` | 90 days | |
| `cors_origins` | empty → `http://localhost:<port>` | comma-separated; `*` warns unless debug |
| `mcp_enabled` | `false` | boolean merge uses “key present in TOML” |
| `mcp_transport` | `stdio` | `http` returns an error |
| `[[boards]]` | human-to-ai and ai-to-human | `id`, `title`, `desc`, `columns` |

`${VAR}` in the TOML file is expanded before parse.

`mcp_enabled = false` is honoured when the key is present. There is **no** `GOBAN_MCP_ENABLED` override.

## Environment

| Variable | Sets |
|---|---|
| `GOBAN_PORT` | port |
| `LOG_LEVEL` | log_level |
| `GOBAN_DEBUG` | debug if `true` or `1` |
| `GOBAN_CONFIG` / `GOBAN_CONFIG_PATH` | config file path |
| `GOBAN_STATIC_PATH` | static files |
| `DB_TYPE` `DB_PATH` `DB_HOST` `DB_USER` `DB_PASSWORD` `DB_NAME` | database. **No `DB_PORT` env** on the server (TOML `db_port` only). `GOBAN_DB_PASSWORD` works only via `${GOBAN_DB_PASSWORD}` in TOML, not as a direct override |
| `GOBAN_DB_SSLMODE` / `DB_SSLMODE` | sslmode |
| `GOBAN_JWT_SECRET` | jwt_secret |
| `GOBAN_JWT_VALIDITY` | duration string |
| `GOBAN_REFRESH_GRACE_PERIOD` | duration string |
| `GOBAN_CORS_ORIGINS` | CORS allow list |

Templates: `goban.toml.example`, `config/.env.example` (incomplete vs the table above — prefer this page).

## Database

SQLite: run `./bin/goban`; file from `db_path`.

PostgreSQL: create the database, set `db_type = "postgres"` and credentials. Tables are created on startup.

## Static files

1. `GOBAN_STATIC_PATH`
2. `static_path` in TOML
3. `<directory-of-binary>/public`
