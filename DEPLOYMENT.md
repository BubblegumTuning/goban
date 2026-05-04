# Goban Deployment Guide

## Directory Layout (Production)

```
/opt/goban/
├── bin/           # Compiled binary
│   └── goban
├── config/        # Configuration files
│   └── goban.toml
├── static/        # Frontend assets (HTML/CSS/JS)
│   ├── index.html
│   └── styles/
│   └── tailwind.min.css
└── data/          # Database and runtime data
    └── goban.db
```

## Configuration Priority

1. **Environment variables** (highest priority)
   - `GOBAN_CONFIG_PATH` - Path to config file
   - `GOBAN_STATIC_PATH` - Path to static files directory
   - `GOBAN_PORT` - Server port
   - `DB_PATH`, `DB_TYPE`, etc.

2. **Config file** (`/opt/goban/config/goban.toml`)
   ```toml
   port = "8080"
   db_path = "/opt/goban/data/goban.db"
   static_path = "/opt/goban/static"
   ```

3. **Hardcoded defaults** (lowest priority)
   - Falls back to binary location for dev mode

## Development Mode

Build and run locally:
```bash
cd ~/goban
./build.sh
cd bin && ./goban
```

This uses symlinks so static files are found in `../public`.

## Production Deployment

### Manual Deploy
```bash
./scripts/deploy.sh [hostname]
```

On the server:
```bash
sudo systemctl enable goban
sudo systemctl start goban
sudo systemctl status goban
```

## Creating the Goban System User

Before deploying, create the service user on the target host:

```bash
# Create system user (no login shell, no home directory)
sudo useradd -r -s /usr/sbin/nologin -c "Goban Kanban Service" goban

# Verify creation
id goban
```

## Systemd Service

The service file (`systemd/goban.service`) configures:
- Working directory: `/opt/goban`
- Environment variables for paths
- Automatic restart on failure
- Security hardening (NoNewPrivileges, ProtectSystem)

## Static File Resolution Order

1. `GOBAN_STATIC_PATH` env var
2. `static_path` in config file
3. Fallback: `/opt/goban/static`
4. Dev fallback: `<binary-dir>/public`
