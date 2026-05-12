#!/bin/bash
# DEPRECATED: Replaced by scripts/build-and-deploy-local.sh.
# This script has been consolidated into the unified deployment workflow.
#
# Usage (new):
#   sudo ./scripts/build-and-deploy-local.sh [source_dir]

echo "DEPRECATED: scripts/deploy-local.sh is no longer maintained."
echo ""
echo "Please use:"
echo "  sudo ./scripts/build-and-deploy-local.sh    # Local deployment"
echo "  ./scripts/deploy.sh <host> [user]           # Remote deployment"
exit 1
