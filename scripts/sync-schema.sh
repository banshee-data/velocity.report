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
    rm -f "$TEMP_DB" "$TEMP_DB-shm" "$TEMP_DB-wal" "$TEMP_SCHEMA" "$PROJECT_ROOT/.tmp_ordered_schema.sql"
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

# Filter out sqlite_sequence table (internal SQLite table, auto-created with AUTOINCREMENT)
# This table cannot be imported and causes errors when trying to recreate the schema
if grep -q "CREATE TABLE sqlite_sequence" "$TEMP_SCHEMA"; then
    echo "   Filtering out sqlite_sequence table..."
    sed -i.bak '/CREATE TABLE sqlite_sequence/d' "$TEMP_SCHEMA"
    rm -f "$TEMP_SCHEMA.bak"
fi

echo -e "${GREEN}✓ Schema exported successfully${NC}"
echo ""

# Step 2.5: Reorder tables by foreign key dependencies
echo "2.5. Reordering tables by foreign key dependencies..."
TEMP_ORDERED="$PROJECT_ROOT/.tmp_ordered_schema.sql"
if ! python3 "$SCRIPT_DIR/order-schema-tables.py" "$TEMP_SCHEMA" > "$TEMP_ORDERED"; then
    echo -e "${RED}Error: Failed to reorder schema tables${NC}"
    exit 1
fi
mv "$TEMP_ORDERED" "$TEMP_SCHEMA"
echo -e "${GREEN}✓ Tables reordered successfully${NC}"
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

# Step 4: Format schema.sql with sql-formatter
echo "4. Formatting schema.sql..."
if command -v sql-formatter &> /dev/null; then
    if sql-formatter --fix -l sqlite -c "$PROJECT_ROOT/.sql-formatter.json" "$SCHEMA_FILE" 2>&1 > /dev/null; then
        echo -e "${GREEN}✓ Schema formatted successfully${NC}"
    else
        echo -e "${YELLOW}⚠ Failed to format schema (continuing anyway)${NC}"
    fi
else
    echo -e "${YELLOW}⚠ sql-formatter not found (skipping formatting)${NC}"
    echo "   Install with: npm install -g sql-formatter"
fi
echo ""

# Step 5: Show diff (if backup exists)
if [ -f "$SCHEMA_FILE.bak" ]; then
    echo "5. Changes made to schema.sql:"
    echo "----------------------------------------"
    if diff -u "$SCHEMA_FILE.bak" "$SCHEMA_FILE" || true; then
        echo -e "${YELLOW}No changes detected${NC}"
    fi
    echo "----------------------------------------"
    echo ""
fi

# Step 6: Verify consistency
echo "6. Verifying schema consistency..."
TEST_OUTPUT=$(mktemp)
if go test ./internal/db -run TestSchemaConsistency > "$TEST_OUTPUT" 2>&1; then
    # Test passed (exit code 0) - verify it's the expected output format
    if grep -qE "^ok[[:space:]]" "$TEST_OUTPUT" && grep -q "github.com/banshee-data/velocity.report/internal/db" "$TEST_OUTPUT"; then
        echo -e "${GREEN}✓ Schema consistency test passed${NC}"
    else
        # Unexpected output format
        echo -e "${YELLOW}⚠ Test completed but output format unexpected${NC}"
        echo "   Output: $(cat "$TEST_OUTPUT")"
        echo "   Run 'make test-go' to verify all tests pass."
    fi
else
    # Test failed (exit code non-zero)
    echo -e "${RED}✗ Schema consistency test failed${NC}"
    echo "   The migrated schema does not match schema.sql"
    echo "   This indicates migrations are out of sync with schema.sql"
    echo ""
    echo "   Test output:"
    cat "$TEST_OUTPUT" | tail -20
    echo ""
    echo "   Run 'make test-go' for full details."
fi
rm -f "$TEST_OUTPUT"

echo ""
echo -e "${GREEN}Schema sync complete!${NC}"
echo ""
echo "Next steps:"
echo "  1. Review changes: git diff internal/db/schema.sql"
echo "  2. Run tests: make test-go"
echo "  3. Commit: git add internal/db/schema.sql"
