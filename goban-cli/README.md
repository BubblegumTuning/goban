# goban-cli

Command-line client for the Goban HTTP API (agents and scripts).

**Full reference:** [docs/cli.md](../docs/cli.md)

## Build

```bash
cd ~/goban
./build.sh                 # server + CLIs → bin/
# CLI only:
(cd goban-cli && go build -o ../bin/goban-cli .)
```

## Install

```bash
mkdir -p ~/bin
cp ~/goban/bin/goban-cli ~/bin/
chmod +x ~/bin/goban-cli
goban-cli --help
```

System-wide: `sudo cp ~/goban/bin/goban-cli /usr/local/bin/`

## Config (short)

Copy `goban-cli/config.yaml.example` to `~/.goban/goban-cli/config.yaml`.

Search order: `$GOBAN_CLI_CONFIG`, `~/.goban/goban-cli/config.yaml`, `~/.goban/config.yaml`, `./config.yaml`.

```bash
export GOBAN_API_BASE_URL="http://localhost:8080"
export GOBAN_API_TOKEN="…"
export GOBAN_BOARD="human-to-ai"
export GOBAN_USER="yourname"
```

Writes need an API token. Command list, flags, and the endpoint table are in [docs/cli.md](../docs/cli.md).
