# MCP server

Off by default (`mcp_enabled = false`). When enabled, `main` starts MCP in a **goroutine**; the HTTP server still listens.

```toml
mcp_enabled = false
mcp_transport = "stdio"
```

`mcp_transport = "http"` is not implemented and returns an error.

There is no environment override for `mcp_enabled`. The key must be present in TOML to set it (including to `false`).

Server name `goban`, version `version.Version` (v1.2.0) unless `version` is set in config. MCP ticket mutations emit SSE.

Write tools require `auth_token` (agent API token; looked up via SHA-256 hash). `list_tickets` / `list_boards` do not.

| Tool | Inputs | Purpose |
|---|---|---|
| `list_tickets` | optional `board_id` | Tickets from the store (not a stub) |
| `list_boards` | none | Boards from config |
| `create_ticket` | `auth_token`, `title`; optional `description`, `board_id`, `column`, `priority` | Create |
| `claim_ticket` | `auth_token`, `ticket_id` | Same claim service as HTTP |
| `move_ticket` | `auth_token`, `ticket_id`, `target_status`; optional `force` | `target_status`: BACKLOG / TODO / IN_PROGRESS / REVIEW / DONE / CANCELLED |
| `add_comment` | `auth_token`, `ticket_id`, `text` | Comment as the token’s user |

Code: `mcp/server.go`, `mcp/tools.go`.
