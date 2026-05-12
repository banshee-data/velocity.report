#!/usr/bin/env bash
# Publish the LOC + coverage chart to the orphan `stats` branch.
#
# Usage:
#   scripts/publish-stats-branch.sh <path-to-loc-coverage.svg>
#
# Idempotent: exits 0 without pushing if the SVG content is unchanged.
# Creates the orphan `stats` branch on first run.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <path-to-loc-coverage.svg>" >&2
  exit 2
fi

src=$1
if [[ ! -f "$src" ]]; then
  echo "$src does not exist" >&2
  exit 1
fi

# Resolve to absolute path before any branch switch.
src=$(cd "$(dirname "$src")" && pwd)/$(basename "$src")

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

# Stash any local changes so checkout is clean (CI working tree has the
# generated SVG, build artefacts, etc.).
work_tree=$(mktemp -d)
trap 'rm -rf "$work_tree"' EXIT

if git ls-remote --exit-code --heads origin stats >/dev/null 2>&1; then
  git fetch origin stats
  git worktree add -B stats "$work_tree" origin/stats
else
  git worktree add --detach "$work_tree"
  git -C "$work_tree" checkout --orphan stats
  git -C "$work_tree" rm -rf . >/dev/null 2>&1 || true
fi

cp "$src" "$work_tree/loc-coverage.svg"
git -C "$work_tree" add loc-coverage.svg

if git -C "$work_tree" diff --cached --quiet; then
  echo "stats: loc-coverage.svg unchanged; skipping push."
  git worktree remove --force "$work_tree"
  exit 0
fi

git -C "$work_tree" commit -m "stats: refresh loc-coverage chart"
git -C "$work_tree" push origin stats
git worktree remove --force "$work_tree"
echo "stats: pushed updated loc-coverage.svg"
