#!/bin/bash
# Deploy goban binaries to system locations
# Usage: ./deploy.sh [--dry-run] [--skip-symlinks] [source_dir]
#   --dry-run       Show what would be deployed without making changes
#   --skip-symlinks Don't create symlinks in /usr/local/bin
#   source_dir      Directory containing built binaries (default: current dir)

set -e

DRY_RUN=false
SKIP_SYMLINKS=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --skip-symlinks)
            SKIP_SYMLINKS=true
            shift
            ;;
        *)
            if [ -z "$SOURCE_DIR" ]; then
                SOURCE_DIR="$1"
            fi
            shift
            ;;
    esac
done

# Default to current directory if not specified
if [ -z "$SOURCE_DIR" ]; then
    SOURCE_DIR="."
fi

SERVER_DEST="/opt/goban"
CLI_DEST="/bin/goban-cli"

# Helper function for dry-run aware commands
run_cmd() {
    if [ "$DRY_RUN" = true ]; then
        echo "[dry-run] Would execute: $*"
    else
        eval "$@" || return $?
    fi
}

echo "=== Goban Deployment Script ==="
[ "$DRY_RUN" = true ] && echo "(DRY RUN MODE - no changes will be made)"
echo "Source directory: $(cd "$SOURCE_DIR" && pwd)"
echo ""

# Check for required binaries (now in bin/ directory)
if [ ! -f "$SOURCE_DIR/bin/goban" ]; then
    echo "ERROR: goban binary not found in $SOURCE_DIR/bin/"
    echo "       Run './build.sh' first to compile the server."
    exit 1
fi

if [ ! -f "$SOURCE_DIR/bin/goban-cli" ]; then
    echo "ERROR: goban-cli binary not found in $SOURCE_DIR/bin/"
    echo "       Build it with './build.sh' first."
    exit 1
fi

echo "[+] Found server binary: $(ls -lh "$SOURCE_DIR/bin/goban" | awk '{print $5}')"
echo "[+] Found CLI binary:   $(ls -lh "$SOURCE_DIR/bin/goban-cli" | awk '{print $5}')"
echo ""

# Check if root is needed for /bin and /opt
NEEDS_ROOT=0
if [ ! -w "/opt" ]; then
    echo "[!] Cannot write to /opt - will require sudo"
    NEEDS_ROOT=1
fi
if [ ! -w "/bin" ]; then
    echo "[!] Cannot write to /bin - will require sudo"
    NEEDS_ROOT=1
fi

# Check if passwordless sudo is available (for non-interactive use)
if [ $NEEDS_ROOT -eq 1 ] && [ "$DRY_RUN" = false ]; then
    if ! sudo -n true 2>/dev/null; then
        echo ""
        echo "[!] Sudo requires a password. You have two options:"
        echo "    1) Run with: sudo ./deploy.sh"
        echo "    2) Configure passwordless sudo for this user"
        echo ""
        exit 1
    fi
fi

# Determine command prefix
if [ $NEEDS_ROOT -eq 1 ] && [ "$DRY_RUN" = false ]; then
    CMD_PREFIX="sudo"
else
    CMD_PREFIX=""
fi

echo ""
echo "[*] Creating destination directory: $SERVER_DEST..."
run_cmd "$CMD_PREFIX mkdir -p \"$SERVER_DEST\""
[ "$DRY_RUN" = false ] && echo "[+] Created/verified: $SERVER_DEST"

echo ""
echo "[*] Deploying server binary to $SERVER_DEST/goban..."
run_cmd "$CMD_PREFIX cp \"$SOURCE_DIR/bin/goban\" \"$SERVER_DEST/\""
run_cmd "$CMD_PREFIX chmod +x \"$SERVER_DEST/goban\""
[ "$DRY_RUN" = false ] && echo "[+] Server deployed: $(ls -lh "$SERVER_DEST/goban")"

# Create symlink for easy access (optional)
if [ "$SKIP_SYMLINKS" != true ]; then
    if [ ! -L /usr/local/bin/goban ] || [ "$FORCE_LINKS" = true ]; then
        echo ""
        echo "[*] Creating symlink: /usr/local/bin/goban -> $SERVER_DEST/goban..."
        run_cmd "sudo ln -sf \"$SERVER_DEST/goban\" /usr/local/bin/goban"
    fi
fi

echo ""
echo "[*] Deploying CLI to $CLI_DEST..."
run_cmd "$CMD_PREFIX cp \"$SOURCE_DIR/bin/goban-cli\" \"$CLI_DEST\""
run_cmd "$CMD_PREFIX chmod +x \"$CLI_DEST\""
[ "$DRY_RUN" = false ] && echo "[+] CLI deployed: $(ls -lh "$CLI_DEST")"

# Create symlink if not exists (optional)  
if [ "$SKIP_SYMLINKS" != true ]; then
    if [ ! -L /usr/local/bin/goban-cli ] || [ "$FORCE_LINKS" = true ]; then
        echo ""
        echo "[*] Creating symlink: /usr/local/bin/goban-cli -> $CLI_DEST..."
        run_cmd "sudo ln -sf \"$CLI_DEST\" /usr/local/bin/goban-cli"
    fi
fi

# Create ~/.goban directory for persistent config and copy example if it doesn't exist
echo ""
HOME_GOBAN_DIR="$HOME/.goban"
if [ ! -d "$HOME_GOBAN_DIR" ]; then
    echo "[*] Creating persistent config directory: $HOME_GOBAN_DIR..."
    run_cmd "mkdir -p \\\"$HOME_GOBAN_DIR\\\""
    [ "$DRY_RUN" = false ] && echo "[+] Created: $HOME_GOBAN_DIR"
else
    echo "[*] Config directory already exists: $HOME_GOBAN_DIR"
fi

# Copy config example if no config exists yet
if [ ! -f "$HOME_GOBAN_DIR/config.yaml" ] && [ -f "$SOURCE_DIR/goban-cli/config.yaml.example" ]; then
    echo "[*] Copying config template to $HOME_GOBAN_DIR/config.yaml..."
    run_cmd "cp \"$SOURCE_DIR/goban-cli/config.yaml.example\" \"$HOME_GOBAN_DIR/config.yaml\""
    [ "$DRY_RUN" = false ] && echo "[+] Config template created (edit with your settings)"
fi

echo ""
if [ "$DRY_RUN" = true ]; then
    echo "=== Dry Run Complete ==="
else
    echo "=== Deployment Complete ==="
fi
echo ""
echo "Server:   $SERVER_DEST/goban"
echo "CLI:      $CLI_DEST"
echo "Config:   $HOME_GOBAN_DIR/config.yaml"
echo ""
echo "To start the server:"
echo "  $SERVER_DEST/goban -c /path/to/config.yaml"
echo ""
echo "To use the CLI (now available system-wide):"
echo "  goban-cli list | goban-cli create | goban-cli move ..."
