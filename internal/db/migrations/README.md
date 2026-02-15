# Database Migrations Guide

This folder contains SQL migration scripts for the velocity.report SQLite database.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Migration Commands](#migration-commands)
- [Migration History](#migration-history)
- [Creating New Migrations](#creating-new-migrations)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)
- [Production Deployment](#production-deployment)
- [Architecture](#architecture)

## Overview

velocity.report uses [golang-migrate](https://github.com/golang-migrate/migrate) for database schema management with the pure-Go SQLite driver (`modernc.org/sqlite`).

**Key Features:**

- Tracks applied migrations in a `schema_migrations` table
- Supports forward (up) and rollback (down) migrations
- Pure-Go implementation (no CGO dependencies)
- Automatic schema version detection for legacy databases
- Built-in migration commands in `velocity-report` binary

**Migration File Format:**

Each migration consists of two files:

- `{version}_{description}.up.sql` - Forward migration (apply changes)
- `{version}_{description}.down.sql` - Rollback migration (undo changes)

Example:

```
000001_initial_schema.up.sql
000001_initial_schema.down.sql
```

## Quick Start

### New Database Setup

When you create a new database, velocity.report automatically:

1. Runs `schema.sql` to create all tables
2. Applies essential PRAGMAs (WAL mode, busy_timeout, etc.)
3. Baselines the database at version 8 (latest)
4. Marks all migrations as applied

**No manual migration needed for fresh installations!**

### Check Migration Status

```bash
velocity-report migrate status
```

Example output:

```
=== Migration Status ===
Current version: 8
Dirty: false
Schema migrations table exists: true
```

### Apply Pending Migrations

```bash
velocity-report migrate up
```

### Existing Database Migration

If you have an existing database:

1. **Backup first:**

   ```bash
   cp sensor_data.db sensor_data.db.$(date +%Y%m%d_%H%M%S)
   ```

2. **Check current state:**

   ```bash
   velocity-report migrate status
   ```

3. **Apply migrations:**
   ```bash
   velocity-report migrate up
   ```

## Migration Commands

### `migrate status`

Show current migration state.

```bash
velocity-report migrate status
```

### `migrate up`

Apply all pending migrations to bring database to latest version.

```bash
velocity-report migrate up
```

Example output:

```
[migrate] 1/u initial_schema (4.5ms)
[migrate] 2/u rename_tables (12.3ms)
✓ All migrations applied successfully
```

### `migrate down`

Rollback the most recent migration.

```bash
velocity-report migrate down
```

**⚠️ Warning:** Rollbacks may lose data. Always backup first!

### `migrate detect`

Detect schema version of legacy databases (without `schema_migrations` table).

```bash
velocity-report migrate detect
```

This command:

- Analyses databases without migration tracking
- Compares current schema against all known migration points
- Calculates similarity score (0-100%)
- Suggests baseline version

Example output:

```
=== Schema Detection Results ===
Best match: version 3
Similarity: 100%
Latest available: 8

✓ Perfect match found!

To baseline and apply remaining migrations:
  1. velocity-report migrate baseline 3
  2. velocity-report migrate up
```

### `migrate version <N>`

Migrate to specific version (up or down).

```bash
velocity-report migrate version 3
```

### `migrate baseline <N>`

Set migration version without running migrations (for databases with existing schema).

```bash
velocity-report migrate baseline 8
```

### `migrate force <N>`

Force migration version (emergency recovery only).

```bash
velocity-report migrate force 7
```

**⚠️ WARNING:** Use only to recover from failed migrations. Does not run any SQL

```bash
velocity-report migrate down
```

### Migrate to Specific Version

```bash
velocity-report migrate version 3
```

### Baseline Database (Legacy Upgrade)

```bash
# Set starting version without running migrations
velocity-report migrate baseline 3
```

### Force Version (Recovery)

```bash
# Use only to recover from failed migrations
velocity-report migrate force 2
```

## Safety Checklist

**Always before applying migrations:**

1. **Backup the database:**

   ```bash
   cp sensor_data.db sensor_data.db.$(date +%Y%m%d_%H%M%S)
   ```

2. **Stop the service** (if running in production):

   ```bash
   sudo systemctl stop velocity-report.service
   ```

3. **Apply migrations:**

   ```bash
   velocity-report migrate up
   ```

4. **Verify the migration:**

   ```bash
   velocity-report migrate status
   sqlite3 sensor_data.db ".tables"
   ```

5. **Restart the service:**
   ```bash
   sudo systemctl start velocity-report.service
   ```

## Manual Migration (Advanced)

Migrations can still be applied manually if needed:

```bash
# Apply a single up migration
sqlite3 sensor_data.db < internal/db/migrations/000001_rename_tables_column.up.sql

# Apply a single down migration (rollback)
sqlite3 sensor_data.db < internal/db/migrations/000001_rename_tables_column.down.sql
```

**Note:** Manual application bypasses the migration tracking system. Use the CLI commands instead.

## Baselining Existing Databases

If you have an existing database that already has all migrations applied (through manual application), you can baseline it:

```bash
# Baseline at version 8 (assuming all migrations are applied)
velocity-report migrate baseline 8
```

This sets the migration version without re-running migrations.

## Creating New Migrations

### Step 1: Determine Next Version

The next migration should be **000026** (current latest is 000025).

```bash
ls -1 internal/db/migrations/*.up.sql | tail -1
# Shows: 000025_add_label_source.up.sql
# Next: 000026
```

### Step 2: Create Migration Files

```bash
touch internal/db/migrations/000026_your_change.up.sql
touch internal/db/migrations/000026_your_change.down.sql
```

### Step 3: Write the SQL

**000026_your_change.up.sql:**

```sql
-- Migration: Brief description
-- Date: YYYY-MM-DD
-- Description: Detailed explanation
--
-- Note: Essential PRAGMAs (journal_mode=WAL, busy_timeout, etc.) are applied
-- by the Go code in db.go/applyPragmas() rather than in migrations.

CREATE TABLE IF NOT EXISTS new_table (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_new_table_name ON new_table(name);
```

**000026_your_change.down.sql:**

```sql
-- Rollback: Remove new_table

DROP INDEX IF EXISTS idx_new_table_name;
DROP TABLE IF EXISTS new_table;
```

### Step 4: Test Thoroughly

```bash
# Create test database
cp sensor_data.db test.db

# Test up migration
velocity-report --db-path test.db migrate up

# Verify tables created
sqlite3 test.db ".tables"

# Test down migration (rollback)
velocity-report --db-path test.db migrate down

# Verify tables removed
sqlite3 test.db ".tables"

# Test idempotency (up again)
velocity-report --db-path test.db migrate up

# Cleanup
rm test.db*
```

### Step 5: Update Documentation

Update the [Migration History](#migration-history) table in this README.

### Step 6: Commit Both Files

```bash
git add internal/db/migrations/000026_your_change.*.sql
git commit -m "[sql] add migration: your_change"
```

## Best Practices

### 1. Always Create Both Up and Down Migrations

Every migration needs both forward and rollback SQL:

```bash
# Always create BOTH files
touch internal/db/migrations/000026_change.up.sql
touch internal/db/migrations/000026_change.down.sql
```

### 2. Backup Before Production Migrations

For production systems:

```bash
# Stop service
sudo systemctl stop velocity-report.service

# Backup database
cp /var/lib/velocity-report/sensor_data.db /var/lib/velocity-report/sensor_data.db.backup

# Apply migrations
velocity-report migrate up --db-path /var/lib/velocity-report/sensor_data.db

# Restart service
sudo systemctl start velocity-report.service
```

### 3. Make Migrations Idempotent

Use `IF EXISTS` and `IF NOT EXISTS` to allow re-running migrations:

```sql
-- ✓ Good - can run multiple times
CREATE TABLE IF NOT EXISTS my_table (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

ALTER TABLE my_table ADD COLUMN IF NOT EXISTS new_col TEXT;

-- ✗ Bad - fails if run twice
CREATE TABLE my_table (id INTEGER PRIMARY KEY);
ALTER TABLE my_table ADD COLUMN new_col TEXT;
```

### 4. Test Rollbacks

Always verify your `.down.sql` works:

```bash
# Create test database
cp sensor_data.db test.db

# Test forward migration
velocity-report --db-path test.db migrate up

# Test rollback
velocity-report --db-path test.db migrate down

# Test forward again (idempotency)
velocity-report --db-path test.db migrate up

# Cleanup
rm test.db*
```

### 5. Keep Migrations Focused

One logical change per migration - don't combine unrelated schema changes.

**Good:**

- `000026_add_user_email.up.sql` - adds email column
- `000027_create_notifications_table.up.sql` - new table

**Bad:**

- `000009_various_changes.up.sql` - adds email, creates notifications, renames columns

### 6. Document Data Transformations

If your migration transforms data, document:

- What data is affected
- Whether rollback is safe (or lossy)
- Any data validation performed

```sql
-- Migration: Add speed_limit column with default value
-- Date: 2025-01-15
-- Description: Adds speed_limit to site_reports with default 25 mph.
--              Rollback is SAFE - drops column only.
--
-- Data impact: All existing rows get speed_limit=25
-- Rollback impact: Column removed, data discarded (not recoverable)

ALTER TABLE site_reports ADD COLUMN IF NOT EXISTS speed_limit INTEGER DEFAULT 25;
```

### 7. Don't Put PRAGMAs in Migrations

Essential SQLite PRAGMAs (journal_mode, busy_timeout, etc.) are applied in Go code (`internal/db/db.go/applyPragmas()`), not in migrations. This ensures consistency regardless of how the database is created.

### 8. Use DROP COLUMN Directly

As of `modernc.org/sqlite v1.44.3`, the bundled SQLite engine (v3.51.2) supports `ALTER TABLE DROP COLUMN` (available since SQLite 3.35.0). New migrations that need to remove columns should use this directly:

```sql
-- ✓ Preferred (new migrations)
ALTER TABLE my_table DROP COLUMN old_col;

-- ✗ Legacy workaround (used in migrations 000014-000019)
-- CREATE TABLE my_table_new (...); INSERT INTO ...; DROP TABLE my_table; ALTER TABLE RENAME ...
```

Older migrations in this repository still use the table-recreation workaround because they predate this capability. They are left as-is for safety.

## Troubleshooting

### "Dirty migration" Error

**Symptom:**

```
=== Migration Status ===
Current version: 5
Dirty: true
```

**Cause:** A migration failed mid-execution.

**Solution:**

1. Check database state:

   ```bash
   sqlite3 sensor_data.db
   .tables
   .schema
   ```

2. Fix issues manually if needed

3. Force to last known good version:

   ```bash
   velocity-report migrate force 4
   ```

4. Try migrating again:
   ```bash
   velocity-report migrate up
   ```

### Rollback Fails

**Symptom:** `migrate down` fails or warns about data loss.

**Cause:** Some migrations cannot be fully rolled back (e.g., data transformations with precision loss).

**Solution:** Check the `.down.sql` file for warnings. If data loss is expected, ensure you have backups before rolling back.

### Schema Drift

**Symptom:** Manual changes were made to the database schema outside migrations.

**Solution:**

1. **Document current state:**

   ```bash
   sqlite3 sensor_data.db .schema > current_schema.sql
   ```

2. **Create migration to formalize change:**

   ```bash
   touch internal/db/migrations/000026_fix_schema_drift.up.sql
   touch internal/db/migrations/000026_fix_schema_drift.down.sql
   # Add SQL to match current state
   ```

3. **Or restore from backup:**
   ```bash
   cp sensor_data.db.backup sensor_data.db
   velocity-report migrate up
   ```

### Version Mismatch on Startup

**Symptom:** Application exits with migration version mismatch error.

**Cause:** Database is from prior installation with older schema.

**Solution:**

```bash
# Check current version
velocity-report migrate status

# Apply pending migrations
velocity-report migrate up

# Verify
velocity-report migrate status
```

## Production Deployment

### Pre-Deployment Checklist

Before deploying migrations to production:

- [ ] Test migrations locally with production-like data
- [ ] Test rollback (`.down.sql`) works correctly
- [ ] Document data impact and rollback safety
- [ ] Backup production database
- [ ] Plan maintenance window if needed
- [ ] Test migration with `--db-path` pointing to backup copy

### Deployment Steps

1. **Stop the service:**

   ```bash
   sudo systemctl stop velocity-report.service
   ```

2. **Backup database:**

   ```bash
   sudo cp /var/lib/velocity-report/sensor_data.db \
           /var/lib/velocity-report/sensor_data.db.$(date +%Y%m%d_%H%M%S)
   ```

3. **Check current version:**

   ```bash
   velocity-report migrate status --db-path /var/lib/velocity-report/sensor_data.db
   ```

4. **Apply migrations:**

   ```bash
   velocity-report migrate up --db-path /var/lib/velocity-report/sensor_data.db
   ```

5. **Verify success:**

   ```bash
   velocity-report migrate status --db-path /var/lib/velocity-report/sensor_data.db
   ```

6. **Restart service:**

   ```bash
   sudo systemctl start velocity-report.service
   ```

7. **Monitor logs:**
   ```bash
   sudo journalctl -u velocity-report.service -f
   ```

### Rollback Procedure

If a migration causes issues:

1. **Stop service:**

   ```bash
   sudo systemctl stop velocity-report.service
   ```

2. **Rollback migration:**

   ```bash
   velocity-report migrate down --db-path /var/lib/velocity-report/sensor_data.db
   ```

3. **Or restore backup:**

   ```bash
   sudo cp /var/lib/velocity-report/sensor_data.db.20250115_120000 \
           /var/lib/velocity-report/sensor_data.db
   ```

4. **Restart service:**
   ```bash
   sudo systemctl start velocity-report.service
   ```

## Architecture

### Migration Framework

velocity.report uses [golang-migrate](https://github.com/golang-migrate/migrate) for database migrations. Key components:

- **Migration files:** SQL files embedded in binary via Go's `embed.FS`
- **Driver:** `modernc.org/sqlite v1.44.3` (pure-Go, no CGO) — bundles SQLite 3.51.2, which supports `ALTER TABLE DROP COLUMN`
- **Tracking:** `schema_migrations` table stores version and dirty state
- **Commands:** Exposed via `velocity-report migrate` CLI

### Migration File Format

Migrations follow sequential naming: `00000N_description.up.sql` and `00000N_description.down.sql`

- **Up migrations:** `000001_initial_schema.up.sql` - apply changes
- **Down migrations:** `000001_initial_schema.down.sql` - rollback changes

### Schema Tracking

The `schema_migrations` table tracks applied migrations:

```sql
CREATE TABLE schema_migrations (
    version BIGINT PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);
```

- **version:** Migration number (1-8 currently)
- **dirty:** True if migration failed mid-execution (needs manual recovery)

### PRAGMA Handling

Essential SQLite PRAGMAs are applied in Go code (`internal/db/db.go/applyPragmas()`), not in migration files:

- `journal_mode=WAL` - Write-Ahead Logging for concurrency
- `busy_timeout=5000` - Wait up to 5s for locks
- `synchronous=NORMAL` - Balance safety and performance
- `temp_store=MEMORY` - Use memory for temp tables

This ensures consistent PRAGMA application regardless of database creation method.

### Automatic Migration Detection

When starting with an existing database:

1. If `schema_migrations` table exists → check version
2. If version mismatch detected → display warning and exit
3. User must run `velocity-report migrate up` to update

For new databases:

1. Run `schema.sql` to create all tables
2. Apply PRAGMAs via `applyPragmas()`
3. Baseline at version 8 (latest)
4. No manual migration needed

## Legacy Migrations

**Historical Note:** Original migrations used `YYYYMMDD_` format:

- `20250826_rename_tables_column.sql`
- `20250827_migrate_ro_to_unix_timestamp.sql`
- `20250929_migrate_data_to_radar_data.sql`
- `20251014_create_site_table.sql`
- `20251016_create_site_reports.sql`
- `20251022_add_velocity_report_prefix.sql`

These have been converted to sequential `00000N_` format for golang-migrate compatibility. Original files are preserved in `archive/` for reference.

## References

- [golang-migrate documentation](https://github.com/golang-migrate/migrate)
- [SQLite ALTER TABLE documentation](https://www.sqlite.org/lang_altertable.html)
- [modernc.org/sqlite driver](https://pkg.go.dev/modernc.org/sqlite)
