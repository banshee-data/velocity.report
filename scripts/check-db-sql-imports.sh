#!/usr/bin/env bash
# check-db-sql-imports.sh — enforce database/sql import boundary
#
# Only two package trees may import "database/sql":
#   1. internal/db/           — the primary database abstraction layer
#   2. internal/lidar/storage/ — the LiDAR storage layer (SQLite repositories)
#
# Everything else (API handlers, pipeline, monitor, etc.) must use the
# types and sentinels exported by those packages instead of importing
# database/sql directly. This keeps the abstraction boundary intact so
# that changes to connection pooling, query tracing, or context
# propagation can be applied in one place.
#
# Test files (*_test.go) are exempt — they often open in-memory databases
# for fixture setup.
#
# Tools under cmd/tools/ are exempt — standalone CLI utilities that
# operate on raw databases outside the main server.
#
# Exit 0 if clean, 1 if violations found.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Find non-test Go files that import "database/sql" outside allowed paths.
violations=""
while IFS= read -r file; do
    # Make path relative for readability.
    rel="${file#"$REPO_ROOT"/}"

    # Allowed: internal/db/
    case "$rel" in
        internal/db/*) continue ;;
    esac

    # Allowed: internal/lidar/storage/
    case "$rel" in
        internal/lidar/storage/*) continue ;;
    esac

    # Allowed: cmd/tools/ (standalone CLI utilities)
    case "$rel" in
        cmd/tools/*) continue ;;
    esac

    violations="${violations}  ${rel}\n"
done < <(grep -rl '"database/sql"' "$REPO_ROOT" \
    --include='*.go' \
    --exclude='*_test.go' \
    --exclude-dir=vendor \
    --exclude-dir=.git \
    || true)

if [ -n "$violations" ]; then
    echo "ERROR: database/sql imported outside allowed packages."
    echo ""
    echo "Only internal/db/ and internal/lidar/storage/ may import database/sql."
    echo "Use the types and sentinels from those packages instead:"
    echo "  - sqlite.SQLDB, sqlite.SQLTx  (type aliases)"
    echo "  - sqlite.ErrNotFound           (sentinel for sql.ErrNoRows)"
    echo ""
    echo "Violations:"
    echo -e "$violations"
    exit 1
fi

echo "OK — no database/sql imports outside allowed packages"
