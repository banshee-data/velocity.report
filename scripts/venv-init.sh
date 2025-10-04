#!/usr/bin/env bash
# Create and activate a repository-level Python virtualenv at .venv
# Usage: ./scripts/venv-init.sh
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VENV="$ROOT_DIR/.venv"

if [ ! -d "$VENV" ]; then
  echo "Creating virtualenv at $VENV"
  python3 -m venv "$VENV"
fi

echo "Activate it with: source $VENV/bin/activate"
echo "To install pinned deps after activation: pip install -r $ROOT_DIR/requirements.txt"
