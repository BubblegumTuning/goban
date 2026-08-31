# Goban documentation

Guides for learning each part of the system. The root [README](../README.md) is only a quick start.

| Document | What it covers |
|---|---|
| [architecture.md](architecture.md) | Packages, binaries, frontend, what lives where |
| [authentication.md](authentication.md) | API tokens vs JWT, roles, middleware, rate limits |
| [api.md](api.md) | REST routes, auth column, activity log, SSE, Go game |
| [cli.md](cli.md) | `goban-cli` config, commands, flags, endpoints used |
| [user-cli.md](user-cli.md) | `goban-user-cli` (direct DB; creates users) |
| [release-notes.md](release-notes.md) | v1.2.0 / v1.1.0 |
| [configuration.md](configuration.md) | Search path, TOML keys, environment overrides |
| [deployment.md](deployment.md) | Build, tarball, `/opt/goban`, systemd, scripts |
| [mcp.md](mcp.md) | Optional MCP stdio server and tools |

Internal audits (findings, not a tutorial): [`../internal_docs/`](../internal_docs/).
