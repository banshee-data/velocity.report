# Database Migration Guide

This guide explains how to use the database migration system in velocity.report.

## Overview

velocity.report uses [golang-migrate](https://github.com/golang-migrate/migrate) for database schema management. The system:

- Tracks applied migrations in a `schema_migrations` table
- Supports forward (up) and rollback (down) migrations
- Works with existing `data/migrations/` SQL files
- Uses pure-Go SQLite driver (no CGO dependencies)
- Integrates with the `velocity-report` binary CLI

## Quick Start

### Check Current Status

```bash
velocity-report migrate status
```

Output:
```
=== Migration Status ===
Current version: 6
Dirty: false
Schema migrations table exists: true
```

### Apply All Pending Migrations

```bash
velocity-report migrate up
```

### Rollback One Migration

```bash
velocity-report migrate down
```

## New Database Setup

When you create a new database, velocity.report automatically:

1. Runs `schema.sql` to create all tables
2. Baselines the database at version 7
3. Marks all existing migrations as applied

**No manual migration needed for fresh installations!**

## Automatic Migration Detection

**New in this version:** When starting the application with an existing database, velocity.report automatically detects and handles different migration scenarios.

### Database with schema_migrations Table

If a version mismatch is detected (e.g., when upgrading with a database from a prior installation):

1. The application will **display a warning** with migration details
2. The application will **exit with an error** message
3. You'll be prompted to run `velocity-report migrate up` to apply outstanding migrations

Example output:
```
⚠️  Database schema version mismatch detected!
   Current database version: 3
   Latest available version: 7
   Outstanding migrations: 4

This database appears to be from a prior installation.
To apply the outstanding migrations, run:
   velocity-report migrate up

To see migration status, run:
   velocity-report migrate status

Error: database schema is out of date (version 3, need 7). Please run migrations
```

### Legacy Database without schema_migrations Table

If you have an older database from before the migration system was implemented (no `schema_migrations` table), velocity.report will:

1. **Automatically detect the schema version** by comparing the current schema against all known migration points
2. **Baseline at the detected version** if it's a perfect match (100% similarity)
3. **Prompt for manual baselining** if the schema doesn't match exactly

Example output for a perfect match:
```
⚠️  Database exists but has no schema_migrations table!
   Attempting to detect schema version...
   Schema detection results:
   - Best match: version 3 (score: 100%)
   - Perfect match! Baselining at version 3

   Database has been baselined at version 3
   There are 4 additional migrations available (up to version 7)

   To apply remaining migrations, run:
      velocity-report migrate up
```

Example output for an imperfect match:
```
⚠️  Database exists but has no schema_migrations table!
   Attempting to detect schema version...
   Schema detection results:
   - Best match: version 3 (score: 85%)
   - No perfect match found (best: 85%)

   Schema differences from version 3:
     + Extra in current: custom_index
     ~ Modified: radar_data

   The current schema does not exactly match any known migration version.
   Closest match is version 3 with 85% similarity.

   Options:
   1. Baseline at version 3 and apply remaining migrations:
      velocity-report migrate baseline 3
      velocity-report migrate up

   2. Manually inspect the differences and adjust your schema
```

This intelligent detection ensures smooth upgrades from any version, even very old databases.

## Existing Database Migration

If you have an existing database that needs migration:

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

4. **Verify:**
   ```bash
   sqlite3 sensor_data.db ".tables"
   velocity-report migrate status
   ```

## Available Commands

### `migrate up`

Apply all pending migrations to bring the database to the latest version.

```bash
velocity-report migrate up
```

Example output:
```
Running migrations from ./data/migrations...
[migrate] 1/u rename_tables_column (4.5ms)
[migrate] 2/u migrate_ro_to_unix_timestamp (12.3ms)
✓ All migrations applied successfully
Current version: 6 (dirty: false)
```

### `migrate down`

Rollback the most recent migration.

```bash
velocity-report migrate down
```

**Warning:** Rollbacks may lose data, especially for data transformation migrations. Always backup first!

### `migrate status`

Show the current migration state.

```bash
velocity-report migrate status
```

### `migrate version <N>`

Migrate to a specific version (can go up or down).

```bash
# Migrate to version 3
velocity-report migrate version 3
```

### `migrate force <N>`

Force the migration version (recovery only). Use this when a migration fails and leaves the database in a "dirty" state.

```bash
# Force to version 2
velocity-report migrate force 2
```

**⚠️  WARNING:** This command is for emergency recovery only. It doesn't run any migrations, just sets the version number.

### `migrate baseline <N>`

Set the migration version without running migrations. Use this for databases that already have the schema but no migration tracking.

```bash
# Baseline at version 6
velocity-report migrate baseline 7
```

## Makefile Shortcuts

For convenience, you can use Makefile targets:

```bash
make migrate-status              # Check status
make migrate-up                  # Apply all migrations
make migrate-down                # Rollback one migration
make migrate-version VERSION=3   # Migrate to version 3
make migrate-force VERSION=2     # Force to version 2
make migrate-baseline VERSION=7  # Baseline at version 6
```

## Migration History

Current migrations (in order):

| Version | Name | Description |
|---------|------|-------------|
| 000001 | initial_schema | Create initial database schema (4 core tables) |
| 000002 | create_site_table | Add site configuration table |
| 000003 | create_site_reports | Add site_reports tracking table |
| 000004 | add_velocity_report_prefix | Standardize report filename prefixes |
| 000005 | create_radar_data_transits | Add persisted sessionization table |
| 000006 | create_radar_transit_links | Add join table for transits |
| 000007 | create_lidar_bg_snapshot | Add LiDAR background snapshot table |

## Creating New Migrations

When you need to add a new migration:

1. **Determine next version:**
   ```bash
   ls -1 data/migrations/*.up.sql | tail -1
   # Current highest: 000007_create_lidar_bg_snapshot.up.sql
   # Next: 000008
   ```

2. **Create migration files:**
   ```bash
   touch data/migrations/000008_your_change.up.sql
   touch data/migrations/000008_your_change.down.sql
   ```

3. **Write the SQL:**

   **000008_your_change.up.sql:**
   ```sql
   -- Migration: Brief description
   -- Date: YYYY-MM-DD
   -- Description: Detailed explanation
   
   CREATE TABLE IF NOT EXISTS new_table (
       id INTEGER PRIMARY KEY AUTOINCREMENT,
       name TEXT NOT NULL
   );
   ```

   **000008_your_change.down.sql:**
   ```sql
   -- Rollback: Remove new_table
   
   DROP TABLE IF EXISTS new_table;
   ```

4. **Test the migration:**
   ```bash
   # Create a test database copy
   cp sensor_data.db test.db
   
   # Test up
   velocity-report --db-path test.db migrate up
   
   # Test down
   velocity-report --db-path test.db migrate down
   
   # Test up again (idempotency)
   velocity-report --db-path test.db migrate up
   
   # Cleanup
   rm test.db*
   ```

5. **Commit both files:**
   ```bash
   git add data/migrations/000008_your_change.*.sql
   git commit -m "Add migration: your_change"
   ```

## Migration Best Practices

### 1. Always Backup Before Migrating

```bash
cp sensor_data.db sensor_data.db.$(date +%Y%m%d_%H%M%S)
```

### 2. Stop Services Before Migration

For production systems:

```bash
sudo systemctl stop velocity-report.service
velocity-report migrate up --db-path /var/lib/velocity-report/sensor_data.db
sudo systemctl start velocity-report.service
```

### 3. Make Migrations Idempotent

Use `IF EXISTS` and `IF NOT EXISTS`:

```sql
-- Good
CREATE TABLE IF NOT EXISTS my_table (...);
ALTER TABLE my_table ADD COLUMN IF NOT EXISTS new_col TEXT;

-- Bad (fails if run twice)
CREATE TABLE my_table (...);
ALTER TABLE my_table ADD COLUMN new_col TEXT;
```

### 4. Test Rollbacks

Always test that your `.down.sql` migration works:

```bash
velocity-report --db-path test.db migrate up
velocity-report --db-path test.db migrate down
```

### 5. Keep Migrations Small

One logical change per migration. Don't combine unrelated schema changes.

### 6. Document Data Transformations

If your migration transforms data, document:
- What data is affected
- Whether rollback is safe (or lossy)
- Any data validation performed

## Troubleshooting

### "Dirty migration" Error

**Symptom:**
```
=== Migration Status ===
Current version: 3
Dirty: true
```

**Cause:** A migration failed mid-execution.

**Solution:**

1. Check what went wrong:
   ```bash
   sqlite3 sensor_data.db
   .tables
   .schema
   ```

2. Fix the database manually if needed

3. Force to a known good version:
   ```bash
   velocity-report migrate force 2
   ```

4. Try migrating again:
   ```bash
   velocity-report migrate up
   ```

### Schema Drift

**Symptom:** Manual changes were made to the database schema.

**Solution:** Create a new migration to formalize the change:

```bash
# Document the current state
sqlite3 sensor_data.db .schema > current_schema.sql

# Create migration to match
touch data/migrations/000008_fix_schema_drift.up.sql
# Add SQL to match current state
```

### Rollback Fails

**Symptom:** `migrate down` fails or loses data.

**Cause:** Some migrations cannot be fully rolled back.

**Solution:**

- Check the `.down.sql` file for warnings
- Restore from backup if needed
- Consider rollback lossy (document in migration)

### Migration Runs Multiple Times

**Symptom:** Migration applied twice somehow.

**Prevention:** Always use:
```sql
CREATE TABLE IF NOT EXISTS ...
ALTER TABLE ... ADD COLUMN IF NOT EXISTS ...
```

## Production Deployment Checklist

Before deploying migrations to production:

- [ ] **Backup database**
  ```bash
  cp /var/lib/velocity-report/sensor_data.db /var/lib/velocity-report/sensor_data.db.backup
  ```

- [ ] **Check current version**
  ```bash
  velocity-report --db-path /var/lib/velocity-report/sensor_data.db migrate status
  ```

- [ ] **Test on production copy first**
  ```bash
  cp /var/lib/velocity-report/sensor_data.db test.db
  velocity-report --db-path test.db migrate up
  # Verify success
  rm test.db*
  ```

- [ ] **Stop service**
  ```bash
  sudo systemctl stop velocity-report.service
  ```

- [ ] **Apply migrations**
  ```bash
  velocity-report --db-path /var/lib/velocity-report/sensor_data.db migrate up
  ```

- [ ] **Verify**
  ```bash
  velocity-report --db-path /var/lib/velocity-report/sensor_data.db migrate status
  ```

- [ ] **Start service**
  ```bash
  sudo systemctl start velocity-report.service
  ```

- [ ] **Monitor logs**
  ```bash
  sudo journalctl -u velocity-report.service -f
  ```

- [ ] **Keep backup for 7 days**

## Architecture Details

### Pure-Go SQLite Driver

velocity.report uses `modernc.org/sqlite` (pure-Go, no CGO) via golang-migrate's `database/sqlite` driver.

**Benefits:**
- No CGO dependencies
- Simpler cross-compilation (Raspberry Pi ARM64)
- Matches existing codebase design

### Schema Migrations Table

```sql
CREATE TABLE schema_migrations (
    version INTEGER NOT NULL,
    dirty INTEGER NOT NULL  -- 0=false, 1=true
);
```

- `version`: Current migration version
- `dirty`: Whether a migration failed mid-execution

### Transaction Safety

Each migration runs in a transaction. If any SQL fails, the entire migration rolls back automatically.

## References

- [golang-migrate documentation](https://github.com/golang-migrate/migrate)
- [Migration file format](../data/migrations/README.md)
- [Design document](database-migration-system-design.md)
- [SQLite ALTER TABLE](https://www.sqlite.org/lang_altertable.html)

## Support

For issues or questions:

1. Check this documentation
2. Review `data/migrations/README.md`
3. Check migration design doc: `docs/database-migration-system-design.md`
4. Open an issue on GitHub
