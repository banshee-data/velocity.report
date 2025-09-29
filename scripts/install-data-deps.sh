#!/usr/bin/env bash
# Install repository pinned dependencies into the repo venv and then install
# any per-data-project requirements files using that venv.
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VENV="$ROOT_DIR/.venv"

if [ ! -d "$VENV" ]; then
  echo "Virtualenv not found. Run: $ROOT_DIR/scripts/venv-init.sh" >&2
  exit 1
fi

source "$VENV/bin/activate"

if [ ! -f "$ROOT_DIR/requirements.txt" ]; then
  echo "requirements.txt not found. Generate with: pip install pip-tools && pip-compile requirements.in" >&2
  exit 1
fi

echo "Installing repo pinned requirements into venv..."
pip install -r "$ROOT_DIR/requirements.txt"

echo "Installing data project requirements (if present)..."
for proj in "$ROOT_DIR"/data/*; do
  req="$proj/requirements.txt"
  if [ -f "$req" ]; then
    echo "Installing for project: $(basename "$proj") -> $req"
    pip install -r "$req"
  fi
done

echo "Done. Use 'source $VENV/bin/activate' to enter the environment."
