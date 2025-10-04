#!/usr/bin/env bash
# Helper to source the repository-level virtualenv from the data project
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
VENV="$ROOT_DIR/.venv"
if [ ! -d "$VENV" ]; then
  echo "Repository venv not found at $VENV. Run: $ROOT_DIR/scripts/venv-init.sh" >&2
  exit 1
fi
echo "Sourcing venv: $VENV"
# shellcheck disable=SC1091
source "$VENV/bin/activate"
exec "$SHELL"
