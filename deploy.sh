#!/bin/bash
# DEPRECATED: This script has been retired.
# Use the unified deployment scripts in scripts/ instead:
#
#   Local deployment (this machine):  sudo ./scripts/build-and-deploy-local.sh
#   Remote deployment via SSH/rsync:  ./scripts/deploy.sh <host> [user]
#
# Old deploy.sh deployed binaries to /opt/goban/goban (wrong path) and did not
# handle frontend assets at all. See scripts/README.md for the current workflow.

echo "DEPRECATED: deploy.sh is no longer maintained."
echo ""
echo "Please use:"
echo "  sudo ./scripts/build-and-deploy-local.sh    # Local deployment"
echo "  ./scripts/deploy.sh <host> [user]           # Remote deployment"
exit 1
