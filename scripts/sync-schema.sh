#!/usr/bin/env bash
#
# sync-schema.sh - Regenerate schema.sql from latest migrations
#
# This script creates a temporary database, applies all migrations,
# exports the schema and fixture data, and updates internal/db/schema.sql.
#
# Usage:
#   ./scripts/sync-schema.sh
#
# The script automatically:
# 1. Creates a temporary database by applying all migrations via sqlite3
# 2. Exports the DDL using SQLite's .schema command
# 3. Extracts fixture data (INSERT rows) from non-empty tables
# 4. Combines DDL + fixtures into internal/db/schema.sql
# 5. Cleans up temporary files
#
# Migrations are the single source of truth for both schema and seed data.
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
TEMP_FIXTURES="$PROJECT_ROOT/.tmp_fixtures.sql"

# Cleanup function
cleanup() {
    rm -f "$TEMP_DB" "$TEMP_DB-shm" "$TEMP_DB-wal" "$TEMP_SCHEMA" \
          "$PROJECT_ROOT/.tmp_ordered_schema.sql" "$TEMP_FIXTURES"
}

# Register cleanup on exit
trap cleanup EXIT

echo -e "${GREEN}Syncing schema.sql from migrations...${NC}"
echo ""

# Check if sqlite3 is available
if ! command -v sqlite3 &> /dev/null; then
    echo -e "${RED}sqlite3 not found.${NC}"
    echo "Install it:"
    echo "  macOS:  brew install sqlite"
    echo "  Ubuntu: sudo apt-get install sqlite3"
    exit 1
fi

# Step 1: Create temporary database and apply migrations via sqlite3
echo "1. Creating temporary database and applying migrations..."
cd "$PROJECT_ROOT"

# Remove old temp db if it exists
rm -f "$TEMP_DB" "$TEMP_DB-shm" "$TEMP_DB-wal"

# Apply migrations directly via sqlite3 — the single source of truth for both
# DDL and fixture data. This bypasses the Go binary (which uses schema.sql for
# fresh databases, creating a circular dependency).

# Create schema_migrations table first (golang-migrate creates this; including it
# keeps schema.sql consistent with a Go-initialised database).
sqlite3 "$TEMP_DB" "CREATE TABLE schema_migrations (version uint64 NOT NULL, dirty bool NOT NULL); CREATE UNIQUE INDEX version_unique ON schema_migrations (version);"

# Apply all up-migrations in order
# PRAGMA legacy_alter_table = OFF matches the default for modernc.org/sqlite (and the
# C API generally). Without it the sqlite3 CLI does not update FK references in other
# tables during ALTER TABLE RENAME, producing a schema that diverges from the Go path.
MIGRATION_COUNT=0
while IFS= read -r -d '' migration; do
    if ! { echo "PRAGMA legacy_alter_table = OFF;"; cat "$migration"; } | sqlite3 "$TEMP_DB" 2>&1; then
        echo -e "${RED}Error: Failed to apply migration: $(basename "$migration")${NC}"
        exit 1
    fi
    MIGRATION_COUNT=$((MIGRATION_COUNT + 1))
done < <(find "$PROJECT_ROOT/internal/db/migrations" -name "*.up.sql" -print0 | sort -z)

echo "   Applied $MIGRATION_COUNT migrations (sqlite3 $(sqlite3 ':memory:' 'SELECT sqlite_version();'))"
echo -e "${GREEN}✓ Migrations applied successfully${NC}"
echo ""

# Step 2: Export schema from migrated database
echo "2. Exporting schema from migrated database..."
if ! sqlite3 "$TEMP_DB" ".schema" > "$TEMP_SCHEMA"; then
    echo -e "${RED}Could not export schema.${NC}"
    exit 1
fi

# Filter out sqlite_sequence table (internal SQLite table, auto-created with AUTOINCREMENT)
# This table cannot be imported and causes errors when trying to recreate the schema
if grep -q "CREATE TABLE sqlite_sequence" "$TEMP_SCHEMA"; then
    echo "   Filtering out sqlite_sequence table..."
    sed -i.bak '/CREATE TABLE sqlite_sequence/d' "$TEMP_SCHEMA"
    rm -f "$TEMP_SCHEMA.bak"
fi

echo -e "${GREEN}✓ Schema exported.${NC}"
echo ""

# Step 2.5: Reorder tables by foreign key dependencies
echo "2.5. Reordering tables by foreign key dependencies..."
TEMP_ORDERED="$PROJECT_ROOT/.tmp_ordered_schema.sql"
if ! python3 "$SCRIPT_DIR/order-schema-tables.py" "$TEMP_SCHEMA" > "$TEMP_ORDERED"; then
    echo -e "${RED}Could not reorder tables.${NC}"
    exit 1
fi
mv "$TEMP_ORDERED" "$TEMP_SCHEMA"
echo -e "${GREEN}✓ Tables reordered.${NC}"
echo ""

# Step 2.6: Extract fixture data from migration-derived database
echo "2.6. Extracting fixture data from migrations..."
: > "$TEMP_FIXTURES"  # truncate

# Find non-empty user tables (excluding sqlite internals and migration tracking)
FIXTURE_COUNT=0
for table in $(sqlite3 "$TEMP_DB" "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations' ORDER BY name;"); do
    count=$(sqlite3 "$TEMP_DB" "SELECT COUNT(*) FROM \"$table\";")
    if [ "$count" -gt 0 ]; then
        echo "   Found $count row(s) in $table"

        # Build column list excluding time-generating defaults — those columns should be
        # filled at runtime so the fixture data is deterministic across sync runs.
        # Covers UNIXEPOCH('subsec'), STRFTIME('%s', 'now'), and the SQLite keywords
        # CURRENT_TIMESTAMP, CURRENT_DATE, and CURRENT_TIME.
        COLS=$(sqlite3 "$TEMP_DB" "SELECT group_concat('\"' || name || '\"', ', ') FROM pragma_table_info('$table') WHERE dflt_value IS NULL OR (UPPER(dflt_value) NOT LIKE '%UNIXEPOCH%' AND UPPER(dflt_value) NOT LIKE '%STRFTIME%' AND UPPER(dflt_value) NOT LIKE 'CURRENT_%');")

        if [ -z "$COLS" ]; then
            echo "   Skipping $table (all columns have time-generating defaults)"
            continue
        fi

        # Generate INSERT OR IGNORE with explicit columns (so time defaults apply at runtime).
        # sqlite3 .mode insert omits quotes around the table name, so match unquoted.
        # Use awk for the substitution to avoid regex-escaping issues with sed.
        sqlite3 "$TEMP_DB" ".mode insert $table" "SELECT $COLS FROM \"$table\"" |
            awk -v tbl="$table" -v cols="$COLS" '{
                old = "INSERT INTO " tbl " VALUES"
                new = "INSERT OR IGNORE INTO \"" tbl "\" (" cols ") VALUES"
                idx = index($0, old)
                if (idx) print substr($0, 1, idx-1) new substr($0, idx+length(old))
                else print
            }' >> "$TEMP_FIXTURES"
        FIXTURE_COUNT=$((FIXTURE_COUNT + 1))
    fi
done

if [ "$FIXTURE_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ Extracted fixture data from $FIXTURE_COUNT table(s)${NC}"
else
    echo -e "${YELLOW}⚠ No fixture data found in migrations${NC}"
fi
echo ""

# Step 3: Update schema.sql
echo "3. Updating internal/db/schema.sql..."

# Backup old schema
if [ -f "$SCHEMA_FILE" ]; then
    cp "$SCHEMA_FILE" "$SCHEMA_FILE.bak"
    echo "   Backup created: $SCHEMA_FILE.bak"
fi

# Combine header + DDL + fixture data
HEADER="-- AUTO-GENERATED — do not edit by hand.
-- Regenerate with: ./scripts/sync-schema.sh
--"
{ echo "$HEADER"; cat "$TEMP_SCHEMA"; } > "$SCHEMA_FILE"

if [ -s "$TEMP_FIXTURES" ]; then
    {
        echo ""
        echo "-- Fixture data derived from migrations (do not edit — regenerate with make schema-sync)."
        echo ""
        cat "$TEMP_FIXTURES"
    } >> "$SCHEMA_FILE"
    echo "   Appended fixture data derived from migrations"
fi

echo -e "${GREEN}✓ schema.sql updated successfully${NC}"
echo ""

# Step 4: Format schema.sql with sql-formatter
echo "4. Formatting schema.sql..."
if command -v sql-formatter &> /dev/null; then
    if sql-formatter --fix -l sqlite -c "$PROJECT_ROOT/.sql-formatter.json" "$SCHEMA_FILE" 2>&1 > /dev/null; then
        # sql-formatter appends /* view_name(col,...) */ hints to VIEWs — strip them.
        # Use sed substitution (not line deletion) to preserve the semicolon
        # when the hint appears on the same line as the final SELECT column.
        # Then collapse any orphaned semicolon-only lines onto the previous line.
        sed -i.sed-tmp 's| */\*[^*]*\*/||g' "$SCHEMA_FILE"
        rm -f "$SCHEMA_FILE.sed-tmp"
        perl -i.sed-tmp -0777 -pe 's/\n\s*;$/;/mg' "$SCHEMA_FILE"
        rm -f "$SCHEMA_FILE.sed-tmp"
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
        echo -e "${GREEN}✓ Schema consistency check passed.${NC}"
    else
        # Unexpected output format
        echo -e "${YELLOW}⚠ Test ran but output looks odd.${NC}"
        echo "   Output: $(cat "$TEST_OUTPUT")"
        echo "   Run 'make test-go' to verify all tests pass."
    fi
else
    # Test failed (exit code non-zero)
    echo -e "${RED}✗ Schema consistency check failed.${NC}"
    echo "   Migrated schema does not match schema.sql."
    echo "   Migrations and schema.sql have diverged."
    echo ""
    echo "   Test output:"
    cat "$TEST_OUTPUT" | tail -20
    echo ""
    echo "   Run 'make test-go' for full details."
fi
rm -f "$TEST_OUTPUT"

echo ""
echo -e "${GREEN}Schema sync done.${NC}"
echo ""
echo "Review and commit:"
echo "  1. Review changes: git diff internal/db/schema.sql"
echo "  2. Run tests: make test-go"
echo "  3. Commit: git add internal/db/schema.sql"
