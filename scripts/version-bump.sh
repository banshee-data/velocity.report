#!/usr/bin/env bash
# version-bump.sh — Increment the pre-release or patch version across all sources,
# stash uncommitted work, commit the bump, then pop the stash.
#
# Versioning rules:
#   0.5.1-pre3  →  0.5.1-pre4      (increment pre-release counter)
#   0.5.1       →  0.5.2-pre1       (stable → next patch pre-release)
#
# Usage: ./scripts/version-bump.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colour codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}✓${NC} $1"; }
log_warn()  { echo -e "${YELLOW}⚠${NC} $1"; }
log_error() { echo -e "${RED}✗${NC} $1"; }

# ---------------------------------------------------------------------------
# 1. Read current version from the Makefile
# ---------------------------------------------------------------------------
cd "$REPO_ROOT"

CURRENT=$(grep -E '^VERSION := ' Makefile | sed 's/VERSION := //')
if [[ -z "$CURRENT" ]]; then
    log_error "Could not read VERSION from Makefile"
    exit 1
fi

echo "Current version: $CURRENT"

# ---------------------------------------------------------------------------
# 2. Compute the next version
# ---------------------------------------------------------------------------
if [[ "$CURRENT" =~ ^([0-9]+\.[0-9]+\.[0-9]+)-pre([0-9]+)$ ]]; then
    # Pre-release: increment the counter
    BASE="${BASH_REMATCH[1]}"
    PRE="${BASH_REMATCH[2]}"
    NEXT="${BASE}-pre$((PRE + 1))"
elif [[ "$CURRENT" =~ ^[0-9]+\.[0-9]+\.([0-9]+)$ ]]; then
    # Stable release: bump patch, start pre1
    MAJOR_MINOR="${CURRENT%.*}"
    PATCH="${BASH_REMATCH[1]}"
    NEXT="${MAJOR_MINOR}.$((PATCH + 1))-pre1"
else
    log_error "Unrecognised version format: $CURRENT"
    log_error "Expected X.Y.Z or X.Y.Z-preN"
    exit 1
fi

echo "Next version:    $NEXT"
echo ""

# ---------------------------------------------------------------------------
# 3. Guard against bad git states
# ---------------------------------------------------------------------------
if [[ -d .git/rebase-merge ]] || [[ -d .git/rebase-apply ]]; then
    log_error "Rebase in progress — finish or abort it first"
    exit 1
fi

if [[ -f .git/MERGE_HEAD ]]; then
    log_error "Merge in progress — finish or abort it first"
    exit 1
fi

if [[ -f .git/CHERRY_PICK_HEAD ]]; then
    log_error "Cherry-pick in progress — finish or abort it first"
    exit 1
fi

if [[ -f .git/BISECT_LOG ]]; then
    log_error "Bisect in progress — finish or abort it first"
    exit 1
fi

# ---------------------------------------------------------------------------
# 4. Stash uncommitted changes (if any)
# ---------------------------------------------------------------------------
STASH_NEEDED=0
if ! git diff --quiet || ! git diff --cached --quiet; then
    STASH_NEEDED=1
    log_warn "Stashing uncommitted changes..."
    git stash push -m "version-bump: auto-stash before $NEXT"
fi

# Ensure we pop the stash on any exit path
cleanup() {
    if [[ "$STASH_NEEDED" -eq 1 ]]; then
        log_info "Restoring stashed changes..."
        git stash pop --quiet
    fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 5. Apply the version bump across all sources
# ---------------------------------------------------------------------------
"$SCRIPT_DIR/set-version.sh" "$NEXT" --all

# ---------------------------------------------------------------------------
# 6. Stage and commit
# ---------------------------------------------------------------------------
git add -A
git commit -m "[ver] bump version to $NEXT across all relevant files"

log_info "Committed version bump to $NEXT (not pushed)"
