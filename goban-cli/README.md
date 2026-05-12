# goban-cli

Command-line interface for interacting with the Goban Kanban board API. Designed primarily for AI agents and automated workflows.

## Building

```bash
cd ~/goban
./build.sh          # Builds server, CLI, and user-CLI into bin/
```

Or build only the CLI:

```bash
cd goban-cli
go build -o ../bin/goban-cli .
```

## Installation

### Quick Install (Recommended)

Copy the binary to your personal bin directory:

```bash
mkdir -p ~/bin
cp ~/goban/bin/goban-cli ~/bin/
chmod +x ~/bin/goban-cli
```

This places it in `~/bin` which is typically already in your PATH. The deploy script (`../deploy.sh`) also handles this automatically.

### Alternative: System-Wide Installation

If you have sudo access and want to install system-wide:

```bash
sudo cp ~/goban/bin/goban-cli /usr/local/bin/
sudo chmod +x /usr/local/bin/goban-cli
```

### Verify Installation

```bash
goban-cli --help
goban-cli list-boards  # Should show available boards
```

## Configuration

The CLI looks for configuration in `~/.goban/config.yaml` (persistent between builds).
If not found, it uses defaults or reads from environment variables.

### Default Config Location

```bash
# Create config directory and copy example:
mkdir -p ~/.goban
cp goban-cli/config.yaml.example ~/.goban/config.yaml
```

Edit `~/.goban/config.yaml` with your settings:

```yaml
api:
  base_url: "http://your-server-ip:8080"
  api_token: ""     # Optional, for authenticated endpoints
  timeout: 30       # seconds

output:
  format: "line"    # or "json", "table"
  colorize: true

retry:
  max_attempts: 3
  initial_delay: 1   # seconds
  backoff_multiplier: 2
```

### Environment Variable Override

You can also set settings via environment variables (takes precedence over config file):

```bash
export GOBAN_API_BASE_URL="http://your-server-ip:8080"
export GOBAN_API_TOKEN="***"
export GOBAN_BOARD="human-to-ai"
export GOBAN_USER="yourname"
export GOBAN_OUTPUT_FORMAT="json"   # or "line", "table"
```

### Command-Line Flag Overrides (highest priority)

Most commands support these flags:

```bash
--server <url>       Override API server URL
--board <id>         Override default board ID
--user <name>        Override username for claiming
--format <line|json> Override output format
```

## Usage

### List all boards

```bash
goban-cli list-boards
```

### View ticket details

```bash
goban-cli view abc123
# Without arguments, shows the session's active ticket
goban-cli view
```

### Create a new ticket

```bash
goban-cli create \
  --board human-to-ai \
  --title "Fix bug #123" \
  --description "Details here..." \
  --column todo
```

### Claim a ticket (moves to In Progress)

```bash
goban-cli claim abc123 --user yourname
```

### Release a ticket back to TODO

```bash
goban-cli release abc123
# Without arguments, releases the session's active ticket
goban-cli release
```

### Move a ticket between columns

```bash
goban-cli move abc123 done
# Columns: backlog, todo, inprogress, review, done, cancelled
```

### Update ticket description

```bash
goban-cli update-description abc123 --description "Updated with new findings"
```

### List tickets

```bash
# Default view: TODO, IN_PROGRESS, REVIEW columns
goban-cli list-tickets

# Include BACKLOG column in results
goban-cli list-tickets --backlog

# Show ALL columns including DONE and CANCELLED
goban-cli list-tickets --full

# List tickets available to claim (not yet claimed)
goban-cli list-available

# View your currently claimed tickets
goban-cli my-tickets
```

### Add a comment

```bash
goban-cli comment abc123 --message "Investigating..."
```

### List comments on a ticket

```bash
goban-cli list-comments abc123
```

### Delete a ticket (with confirmation)

```bash
goban-cli delete abc123
# Requires typing the ticket ID to confirm
```

### Batch operations

```bash
# Move multiple tickets to DONE
goban-cli batch-done abc123 def456 ghi789

# Cancel multiple tickets
goban-cli batch-cancel abc123 def456
```

### Session-based workflow

Set an active ticket, then advance it without specifying the ID each time:

```bash
# Move active ticket to DONE
goban-cli done

# Move active ticket to REVIEW
goban-cli review

# Return active ticket to TODO
goban-cli todo

# Return active ticket to BACKLOG
goban-cli backlog

# Cancel active ticket
goban-cli cancel
```

### Regenerate your API token

If your token has been compromised, regenerate it:

```bash
goban-cli regenerate-token
# For a different user ID (requires admin privileges)
goban-cli regenerate-token --user-id 2
```

### Task links (parent-child dependencies)

Link tickets to establish parent-child dependency relationships:

```bash
# Create a link between two tickets
goban-cli link <parent_id> <child_id>

# Remove an existing link
goban-cli unlink <parent_id> <child_id>
```

### Run history (execution tracking)

Track execution attempts against tickets:

```bash
# View run history for a ticket
goban-cli runs <ticket-id>

# Start a new run on a ticket
goban-cli start <ticket-id>

# Finish the active run on a ticket
goban-cli finish <ticket-id>
```

## Authentication

Commands that modify state (claim, move, release, create, delete, update) require an API token. Set it via one of these methods (in order of priority):

1. Environment variable: `GOBAN_API_TOKEN=***`
2. Config file: `api.api_token` in `~/.goban/config.yaml`

## AI Agent Integration Example

For use in shell scripts or AI agent workflows:

```bash
#!/bin/bash
export GOBAN_API_BASE_URL="http://localhost:8080"
export GOBAN_API_TOKEN="***"
export GOBAN_BOARD="human-to-ai"
export GOBAN_USER="AI-Agent-01"

# Create a task for human review
goban-cli create \
  --title "Review deployment logs" \
  --description "$(cat /var/log/app.log | tail -50)" \
  --column todo

# Claim and work on the most recent ticket
TICKET_ID=$(goban-cli list-available --format json | jq -r '.tickets[0].id')
goban-cli claim $TICKET_ID --user AI-Agent-01

# Add progress updates
goban-cli comment $TICKET_ID --message "Analysis complete"

# Move to review when done
goban-cli move $TICKET_ID review
```

## API Endpoints Used

This CLI wraps the following Goban REST API endpoints:

| Command | Endpoint | Method |
|---------|----------|--------|
| `list-boards` | `/api/boards` | GET |
| `view <id>` | `/api/v1/tickets/:id` | GET |
| `create` | `/api/tickets` | POST (auth) |
| `claim <id>` | `/api/v1/tickets/:id/claim` | POST (auth) |
| `release <id>` | `/api/v1/tickets/:id/release` | POST (auth) |
| `move <id> <col>` | `/api/v1/tickets/:id/move` | POST (auth) |
| `update-description` | `/api/tickets/:id` | PATCH (auth) |
| `delete <id>` | `/api/tickets/:id` | DELETE (auth) |
| `list-tickets` | `/api/v1/tickets` | GET |
| `list-available` | `/api/boards/:boardID` (filter unassigned) | GET |
| `my-tickets` | `/api/boards/:boardID` (filter assigned to user) | GET |
| `comment <ticket-id>` | `/api/tickets/:id/comments` | POST (auth) |
| `list-comments <ticket-id>` | `/api/tickets/:id/comments` | GET (auth) |
| `batch-done <ids...>` | `/api/v1/tickets/:id/move` x N | POST (auth) |
| `batch-cancel <ids...>` | `/api/v1/tickets/:id/move` x N | POST (auth) |
| `done/review/todo/backlog/cancel` | `/api/v1/tickets/:id/move` (session ticket) | POST (auth) |
| `link <parent> <child>` | `/api/tickets/<child>/links` | POST (auth) |
| `unlink <parent> <child>` | `/api/tickets/<child>/links?parent=<parent>` | DELETE (auth) |
| `runs <ticket-id>` | `/api/tickets/:id/runs` | GET |
| `start <ticket-id>` | `/api/tickets/:id/runs` | POST (auth) |
| `finish <ticket-id>` | `/api/tickets/:id/runs?run_id=<id>` | PUT (auth) |
| `regenerate-token` | `/api/admin/users/:id/token-regenerate` | POST (token + role-based) |
