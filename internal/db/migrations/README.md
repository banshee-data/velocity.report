# Database Migrations

This folder contains SQL migration scripts for the velocity.report SQLite database using [golang-migrate](https://github.com/golang-migrate/migrate).

## Migration System

**Status:** Active - Using golang-migrate for automated migration management

Migrations are managed using golang-migrate with the pure-Go SQLite driver (`modernc.org/sqlite`). Each migration consists of two files:

- `{version}_{description}.up.sql` - Forward migration
- `{version}_{description}.down.sql` - Rollback migration

Example:

```
000001_rename_tables_column.up.sql
000001_rename_tables_column.down.sql
```

## Using the Migration CLI

The `velocity-report` binary includes built-in migration commands:

### Check Migration Status

```bash
velocity-report migrate status
```

### Detect Schema Version (Legacy Databases)

```bash
velocity-report migrate detect
```

Use this command to:

- Identify the schema version of databases without `schema_migrations` table
- Compare current schema against all known migration points
- Get recommendations for baselining and upgrading legacy databases

### Apply All Pending Migrations

```bash
velocity-report migrate up
```

### Rollback One Migration

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

## Migration History

Current migrations (in order of application):

1. **000001_initial_schema** - Create initial database schema (4 core tables)
2. **000002_create_site_table** - Add site configuration table
3. **000003_create_site_reports** - Add site_reports tracking table
4. **000004_add_velocity_report_prefix** - Standardize report filename prefixes
5. **000005_create_radar_data_transits** - Add persisted sessionization table
6. **000006_create_radar_transit_links** - Add join table for transits
7. **000007_create_lidar_bg_snapshot** - Add LiDAR background snapshot table

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
# Baseline at version 6 (assuming all 7 migrations are applied)
velocity-report migrate baseline 7
```

This sets the migration version without re-running migrations.

## Creating New Migrations

When adding new migrations:

1. **Determine next version number:**

   ```bash
   ls -1 internal/db/migrations/*.up.sql | tail -1
   # Look at the highest number, add 1
   ```

2. **Create both files:**

   ```bash
   touch internal/db/migrations/000007_your_migration_name.up.sql
   touch internal/db/migrations/000007_your_migration_name.down.sql
   ```

3. **Write the SQL:**

   - `.up.sql` - Forward migration (what changes)
   - `.down.sql` - Rollback migration (how to undo)

4. **Test thoroughly:**

   ```bash
   # Test up
   velocity-report migrate up

   # Test down
   velocity-report migrate down

   # Test up again (idempotency)
   velocity-report migrate up
   ```

5. **Commit both files together**

## Troubleshooting

### "Dirty migration" error

If a migration fails mid-execution:

```bash
# Check status
velocity-report migrate status

# Manually inspect database state
sqlite3 sensor_data.db

# Force to last known good version (use with caution)
velocity-report migrate force 5
```

### Rollback not working

Some migrations cannot be fully rolled back (e.g., data transformations with precision loss). Check the `.down.sql` file for warnings.

### Schema drift

If manual changes were made to the database:

1. Create a new migration to formalize the change
2. Or restore from backup and re-apply migrations

## Legacy Migrations

**Historical Note:** Original migrations used `YYYYMMDD_` format and have been converted to sequential `00000N_` format for golang-migrate compatibility. Original files are preserved with their date-based names for reference.

## References

- [golang-migrate documentation](https://github.com/golang-migrate/migrate)
- [SQLite ALTER TABLE documentation](https://www.sqlite.org/lang_altertable.html)
- [Design document](../../docs/database-migration-system-design.md)
