#!/usr/bin/env bash
# check-single-sqlite-driver.sh — enforce one SQLite driver across Go code
#
# velocity.report standardises on modernc.org/sqlite for both production and
# tests. This avoids split behaviour between CGO-backed and pure-Go drivers and
# keeps the build surface simpler before v0.5.0.
#
# Exit 0 if clean, 1 if violations found.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

module_violations="$(grep -n 'github.com/mattn/go-sqlite3' "$REPO_ROOT"/go.mod "$REPO_ROOT"/go.sum || true)"
import_violations="$(grep -RIn 'github.com/mattn/go-sqlite3' "$REPO_ROOT"/cmd "$REPO_ROOT"/internal --include='*.go' || true)"
driver_violations="$(grep -RIn 'sql.Open("sqlite3"' "$REPO_ROOT"/cmd "$REPO_ROOT"/internal --include='*.go' || true)"

if [ -n "$module_violations" ] || [ -n "$import_violations" ] || [ -n "$driver_violations" ]; then
    echo "ERROR: mixed SQLite drivers detected."
    echo ""
    echo "Use only modernc.org/sqlite and the \"sqlite\" driver name."
    echo ""
    if [ -n "$module_violations" ]; then
        echo "Module graph violations:"
        printf '%s\n' "$module_violations"
        echo ""
    fi
    if [ -n "$import_violations" ]; then
        echo "Go import violations:"
        printf '%s\n' "$import_violations"
        echo ""
    fi
    if [ -n "$driver_violations" ]; then
        echo "Driver name violations:"
        printf '%s\n' "$driver_violations"
    fi
    exit 1
fi

echo "OK — modernc.org/sqlite is the only SQLite driver in Go code"
