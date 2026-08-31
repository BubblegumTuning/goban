# goban-cli

HTTP client for the Goban API. Built for agents and scripts. Source: `goban-cli/`. Install notes also live in [`goban-cli/README.md`](../goban-cli/README.md).

```bash
./build.sh                 # → bin/goban-cli
# or
(cd goban-cli && go build -o ../bin/goban-cli .)
```

## Configuration

Search order (`goban-cli/internal/config`):

1. `$GOBAN_CLI_CONFIG`
2. `~/.goban/goban-cli/config.yaml`
3. `~/.goban/config.yaml` (legacy)
4. `./config.yaml`

Example (`goban-cli/config.yaml.example`):

```yaml
api:
  base_url: "http://localhost:8080"
  api_token: ""
  timeout: 30
output:
  format: "line"   # line | json | compact
  colorize: true
retry:
  max_attempts: 3
  initial_delay: 1
  backoff_multiplier: 2
```

Environment (overrides file): `GOBAN_API_BASE_URL`, `GOBAN_API_TOKEN`, `GOBAN_BOARD`, `GOBAN_USER`, `GOBAN_OUTPUT_FORMAT`.

Persistent flags (highest priority): `--server`, `--board` / `-b`, `--user`, `--format`, `--verbose` / `-V`.

State-changing commands need `GOBAN_API_TOKEN` or `api.api_token`. Claim/move/release identity is that token’s user, not `--user`. `--user` is stored in the session file for later `done` / `comment` convenience.

## Commands

```bash
goban-cli list-boards
goban-cli create --board human-to-ai --title "Fix bug" --description "…" --column todo
# extra: --idempotency-key / -k, --parents / -p

goban-cli claim <ticket-id>            # assignee = token user
# --status / -s then moves (backlog, todo, inprogress, review, done, cancelled)
goban-cli release [ticket-id]          # unassign and move to TODO; session ticket if omitted
goban-cli move <ticket-id> <status>    # --force, --no-auto-claim
# statuses: backlog, todo, inprogress, review, done, cancelled

goban-cli list-tickets                 # TODO + IN_PROGRESS + REVIEW
goban-cli list-tickets --backlog
goban-cli list-tickets --full          # includes DONE and CANCELLED
goban-cli list-available
goban-cli my-tickets
goban-cli view [ticket-id]
goban-cli update-description <ticket-id> --description "…"
goban-cli delete <ticket-id>           # --force / -f skips confirm

goban-cli comment [ticket-id] --message "Investigating…"
goban-cli list-comments <ticket-id>    # needs API token (GET comments is authenticated)

goban-cli batch-done abc123 def456     # --file, --force
goban-cli batch-cancel abc123          # --file, --force

# session: operate on the ticket set by claim
goban-cli done      # --force
goban-cli review
goban-cli todo
goban-cli backlog
goban-cli cancel    # --force

goban-cli link <parent_id> <child_id>
goban-cli unlink <parent_id> <child_id>

goban-cli runs <ticket-id>
goban-cli start <ticket-id> --summary "…"
goban-cli finish <ticket-id> [--run-id N] [--outcome completed|released|blocked]

# NOT the HTTP admin regenerate endpoint:
# ssh kanban01 '… /opt/goban/bin/goban-user-cli regenerate <id>'
goban-cli regenerate-token --user-id 1 [--host kanban01]
```

## Endpoints used

| Command | Method | Path |
|---|---|---|
| `list-boards` | GET | `/api/boards` |
| `view` | GET | `/api/v1/tickets/:id` |
| `create` | POST | `/api/tickets` |
| `claim` | POST | `/api/v1/tickets/:id/claim` |
| `release` | POST | `/api/v1/tickets/:id/release` |
| `move` / session / batch | POST | `/api/v1/tickets/:id/move` |
| `update-description` | PATCH | `/api/tickets/:id` |
| `delete` | DELETE | `/api/tickets/:id` |
| `list-tickets` | GET | `/api/v1/tickets` |
| `list-available` / `my-tickets` | GET | `/api/boards/:boardID` (client filter) |
| `comment` | POST | `/api/tickets/:id/comments` |
| `list-comments` | GET | `/api/tickets/:id/comments` |
| `link` | POST | `/api/tickets/<child>/links` |
| `unlink` | DELETE | `/api/tickets/<child>/links?parent=<parent>` |
| `runs` | GET | `/api/tickets/:id/runs` |
| `start` | POST | `/api/tickets/:id/runs` |
| `finish` | PUT | `/api/tickets/:id/runs?run_id=` |

There is no CLI for subtasks; use curl against `/api/tickets/:id/subtasks`.
