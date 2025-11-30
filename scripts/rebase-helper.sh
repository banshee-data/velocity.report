#!/bin/bash
# rebase-helper.sh - Automates conflict resolution when rebasing this branch onto main
#
# This script helps resolve the known conflicts that occur when rebasing
# the serial-configuration feature branch onto main after PR #121
# (golang-migrate migration system) was merged.
#
# Usage:
#   1. Start the rebase: git rebase origin/main
#   2. When a conflict occurs, run: ./scripts/rebase-helper.sh
#   3. The script will detect and resolve the conflict automatically
#   4. If successful, the rebase will continue
#
# Known conflicts this script handles:
#   - Migration files moved from data/migrations/ to internal/db/migrations/
#   - pnpm-lock.yaml dependency conflicts
#   - Migration format change from date-based (20251106_*) to sequential (000009_*)

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== Rebase Helper ===${NC}"

# Check if we're in a rebase
if [ ! -d ".git/rebase-merge" ] && [ ! -d ".git/rebase-apply" ]; then
    echo -e "${RED}Error: Not in the middle of a rebase.${NC}"
    echo "Start a rebase first with: git rebase origin/main"
    exit 1
fi

# Get the current conflict status
conflicts=$(git diff --name-only --diff-filter=U 2>/dev/null || true)

if [ -z "$conflicts" ]; then
    echo -e "${GREEN}No conflicts detected.${NC}"
    echo "Run 'git rebase --continue' to proceed."
    exit 0
fi

echo -e "Conflicts found:\n$conflicts"
echo ""

# Handle migration conflicts
if echo "$conflicts" | grep -q "internal/db/migrations/20251106_create_radar_serial_config.sql"; then
    echo -e "${YELLOW}Resolving: Migration file location conflict (Phase 1)${NC}"
    git rm internal/db/migrations/20251106_create_radar_serial_config.sql 2>/dev/null || true
    echo -e "${GREEN}✓ Removed old-format migration from internal/db/migrations/${NC}"
fi

if echo "$conflicts" | grep -q "data/migrations/20251106_create_radar_serial_config.sql"; then
    echo -e "${YELLOW}Resolving: Migration file in old location${NC}"
    git rm data/migrations/20251106_create_radar_serial_config.sql 2>/dev/null || true
    echo -e "${GREEN}✓ Removed migration from data/migrations/${NC}"
fi

if echo "$conflicts" | grep -q "data/migrations/"; then
    echo -e "${YELLOW}Resolving: Old migration files conflict (SQL lint)${NC}"
    # Only remove specific date-format migration files (YYYYMMDD_*.sql)
    for f in data/migrations/20[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql; do
        if [ -f "$f" ]; then
            git rm "$f" 2>/dev/null || true
        fi
    done
    echo -e "${GREEN}✓ Removed all old-format migrations${NC}"
fi

# Handle pnpm-lock.yaml conflict
if echo "$conflicts" | grep -q "web/pnpm-lock.yaml"; then
    echo -e "${YELLOW}Resolving: pnpm-lock.yaml conflict${NC}"
    git checkout --theirs web/pnpm-lock.yaml
    
    # Try to regenerate lock file
    if command -v pnpm &> /dev/null; then
        echo "Regenerating with pnpm..."
        (cd web && pnpm install --lockfile-only)
    elif command -v npm &> /dev/null; then
        echo "Regenerating with npm..."
        (cd web && npm install --package-lock-only)
    fi
    
    git add web/pnpm-lock.yaml
    echo -e "${GREEN}✓ Resolved pnpm-lock.yaml${NC}"
fi

# Handle final migration conversion commit conflict
if echo "$conflicts" | grep -q "internal/db/migrations/000009_create_radar_serial_config.up.sql"; then
    echo -e "${YELLOW}Resolving: Final migration format conversion${NC}"
    
    # Check if the file exists from a previous resolution
    if [ ! -f "internal/db/migrations/000009_create_radar_serial_config.up.sql" ]; then
        # Try to restore from HEAD first (most reliable), then fall back to ORIG_HEAD
        git show HEAD:internal/db/migrations/000009_create_radar_serial_config.up.sql > \
            internal/db/migrations/000009_create_radar_serial_config.up.sql 2>/dev/null || \
        git show ORIG_HEAD:internal/db/migrations/000009_create_radar_serial_config.up.sql > \
            internal/db/migrations/000009_create_radar_serial_config.up.sql 2>/dev/null || true
    fi
    
    if [ -f "internal/db/migrations/000009_create_radar_serial_config.up.sql" ]; then
        git add internal/db/migrations/000009_create_radar_serial_config.up.sql
        echo -e "${GREEN}✓ Added 000009 migration${NC}"
    else
        echo -e "${RED}Warning: Could not find migration file to restore${NC}"
    fi
fi

# Handle deleted-by-them conflicts (main's migrations)
if echo "$conflicts" | grep -qE "internal/db/migrations/00000[678]_"; then
    echo -e "${YELLOW}Resolving: Main's migrations deleted by our changes${NC}"
    for f in internal/db/migrations/00000[678]_*.up.sql; do
        if [ -f "$f" ]; then
            git checkout --ours "$f" 2>/dev/null || true
            git add "$f" 2>/dev/null || true
        fi
    done
    echo -e "${GREEN}✓ Kept main's existing migrations${NC}"
fi

echo ""
echo -e "${GREEN}Conflict resolution complete.${NC}"
echo ""
echo "Next steps:"
echo "  1. Review the changes: git status"
echo "  2. Continue the rebase: GIT_EDITOR=true git rebase --continue"
echo "     (or just: git rebase --continue)"
echo ""
