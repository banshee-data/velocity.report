#!/usr/bin/env bash
#
# sync-schema.sh - Regenerate schema.sql from latest migrations
#
# This script creates a temporary database, applies all migrations,
# exports the schema, and updates internal/db/schema.sql.
#
# Usage:
#   ./scripts/sync-schema.sh
#
# The script automatically:
# 1. Creates a temporary database
# 2. Applies all migrations using the Go migration system
# 3. Exports the schema using SQLite's .schema command
# 4. Updates internal/db/schema.sql with the new schema
# 5. Cleans up temporary files
#
# Exit codes:
#   0 - Success
#   1 - Error (migration failed, sqlite3 not found, etc.)

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Paths
SCHEMA_FILE="$PROJECT_ROOT/internal/db/schema.sql"
TEMP_DB="$PROJECT_ROOT/.tmp_schema_sync.db"
TEMP_SCHEMA="$PROJECT_ROOT/.tmp_schema.sql"

# Cleanup function
cleanup() {
    rm -f "$TEMP_DB" "$TEMP_DB-shm" "$TEMP_DB-wal" "$TEMP_SCHEMA"
}

# Register cleanup on exit
trap cleanup EXIT

echo -e "${GREEN}Syncing schema.sql with latest migrations...${NC}"
echo ""

# Check if sqlite3 is available
if ! command -v sqlite3 &> /dev/null; then
    echo -e "${RED}Error: sqlite3 command not found${NC}"
    echo "Please install SQLite3:"
    echo "  macOS:  brew install sqlite"
    echo "  Ubuntu: sudo apt-get install sqlite3"
    exit 1
fi

# Step 1: Create temporary database and apply migrations
echo "1. Creating temporary database and applying migrations..."
cd "$PROJECT_ROOT"

# Remove old temp db if it exists
rm -f "$TEMP_DB" "$TEMP_DB-shm" "$TEMP_DB-wal"

# Use the Go migration system to create a fresh database with all migrations
# The radar command with a non-existent database will trigger migration check
# which creates the database using schema.sql, then we'll migrate it properly
if ! go run ./cmd/radar --db-path="$TEMP_DB" migrate up 2>&1; then
    echo -e "${RED}Error: Failed to apply migrations${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Migrations applied successfully${NC}"
echo ""

# Step 2: Export schema from migrated database
echo "2. Exporting schema from migrated database..."
if ! sqlite3 "$TEMP_DB" ".schema" > "$TEMP_SCHEMA"; then
    echo -e "${RED}Error: Failed to export schema${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Schema exported successfully${NC}"
echo ""

# Step 3: Update schema.sql
echo "3. Updating internal/db/schema.sql..."

# Backup old schema
if [ -f "$SCHEMA_FILE" ]; then
    cp "$SCHEMA_FILE" "$SCHEMA_FILE.bak"
    echo "   Backup created: $SCHEMA_FILE.bak"
fi

# Copy new schema
cp "$TEMP_SCHEMA" "$SCHEMA_FILE"
echo -e "${GREEN}✓ schema.sql updated successfully${NC}"
echo ""

# Step 4: Show diff (if backup exists)
if [ -f "$SCHEMA_FILE.bak" ]; then
    echo "4. Changes made to schema.sql:"
    echo "----------------------------------------"
    if diff -u "$SCHEMA_FILE.bak" "$SCHEMA_FILE" || true; then
        echo -e "${YELLOW}No changes detected${NC}"
    fi
    echo "----------------------------------------"
    echo ""
fi

# Step 5: Verify consistency
echo "5. Verifying schema consistency..."
if go test -v ./internal/db -run TestSchemaConsistency 2>&1 | grep -q "PASS"; then
    echo -e "${GREEN}✓ Schema consistency test passed${NC}"
else
    echo -e "${YELLOW}⚠ Schema consistency test did not pass${NC}"
    echo "   This may be expected if you just added a new migration."
    echo "   Run 'make test-go' to verify all tests pass."
fi

echo ""
echo -e "${GREEN}Schema sync complete!${NC}"
echo ""
echo "Next steps:"
echo "  1. Review changes: git diff internal/db/schema.sql"
echo "  2. Run tests: make test-go"
echo "  3. Commit: git add internal/db/schema.sql"
