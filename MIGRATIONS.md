# Database Migrations

This repository uses manual SQL migrations stored in `data/migrations/`.

## Current Status

**Migration System**: Manual (in progress to automated)  
**Migrations Directory**: `data/migrations/`  
**Database**: SQLite (`/var/lib/velocity-report/sensor_data.db`)

### Existing Migrations

6 migrations currently exist:
1. `20250826_rename_tables_column.sql` - Table/column renames
2. `20250827_migrate_ro_to_unix_timestamp.sql` - Timestamp conversion
3. `20250929_migrate_data_to_radar_data.sql` - Legacy table migration
4. `20251014_create_site_table.sql` - Site configuration table
5. `20251016_create_site_reports.sql` - Report tracking table
6. `20251022_add_velocity_report_prefix.sql` - Filename standardization

## Quick Usage (Current Manual Process)

### Running Migrations

```bash
# Always backup first!
cp sensor_data.db sensor_data.db.bak

# Apply a migration
sqlite3 sensor_data.db < data/migrations/20250929_migrate_data_to_radar_data.sql

# For production database
sqlite3 /var/lib/velocity-report/sensor_data.db < data/migrations/YYYYMMDD_migration_name.sql
```

### Safety Checklist

- [ ] Backup database before migration
- [ ] Stop velocity-report service
- [ ] Run migration
- [ ] Inspect results
- [ ] Restart service
- [ ] Monitor for issues

See `data/migrations/README.md` for detailed instructions.

## Future: Automated Migration System

A comprehensive **automated migration system** has been designed to replace the manual process.

**📄 Full Design Document**: [`docs/database-migration-system-design.md`](./docs/database-migration-system-design.md)

### Key Features (Planned)

- ✅ **State tracking** via `schema_migrations` metadata table
- ✅ **Idempotent migrations** - safe to run multiple times
- ✅ **Up/down migrations** - rollback support
- ✅ **Version visibility** - query current schema version
- ✅ **CLI integration** - `velocity-report migrate up/down/status`
- ✅ **Automated testing** - CI/CD integration
- ✅ **Baseline support** - existing databases can adopt system

### Recommended Tool: golang-migrate

After evaluating 5 approaches (golang-migrate, goose, custom Go, shell scripts, Python), **golang-migrate** was selected for:

- **Separate `.up.sql` and `.down.sql` files** - can run manually without framework
- **First-class SQLite support** - production-ready driver with auto-transaction wrapping
- Battle-tested industry standard (15k+ stars)
- Pure SQL files with no special markers
- Clear separation of forward/rollback operations
- Critical for emergency recovery scenarios

### Migration File Format (Future)

Separate up/down files for manual execution:

```bash
data/migrations/
├── 000001_create_site_table.up.sql      # Forward migration
├── 000001_create_site_table.down.sql    # Rollback migration
├── 000002_add_reports.up.sql
├── 000002_add_reports.down.sql
```

**Up migration** (`000001_create_site_table.up.sql`):
```sql
CREATE TABLE IF NOT EXISTS site (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    ...
);
```

**Down migration** (`000001_create_site_table.down.sql`):
```sql
DROP TABLE IF EXISTS site;
```

Files can be run manually: `sqlite3 sensor_data.db < 000001_create_site_table.up.sql`

### Implementation Timeline

- **Phase 1** (Week 1): Foundation - Add goose, create migration runner
- **Phase 2** (Week 1-2): Update existing migration files with up/down markers
- **Phase 3** (Week 2): CLI integration - Add commands to velocity-report binary
- **Phase 4** (Week 3): Deployment integration - Update setup scripts, CI/CD
- **Phase 5** (Week 3-4): Documentation and training

## Contributing Migrations

When creating new migrations:

1. **Name pattern**: `YYYYMMDD_descriptive_name.sql`
2. **Atomic changes**: One logical change per migration
3. **Test locally**: On copy of production database
4. **Document**: Add comments explaining non-obvious changes
5. **Backup strategy**: Always include rollback instructions

See design doc for comprehensive developer checklist.

## Resources

- **Current migration docs**: [`data/migrations/README.md`](./data/migrations/README.md)
- **Full design document**: [`docs/database-migration-system-design.md`](./docs/database-migration-system-design.md)
- **Schema definition**: [`internal/db/schema.sql`](./internal/db/schema.sql)
- **Database initialization**: [`internal/db/db.go`](./internal/db/db.go)

## Questions?

- Review the [design document](./docs/database-migration-system-design.md) for detailed analysis
- Check [data/migrations/README.md](./data/migrations/README.md) for current manual process
- See [ARCHITECTURE.md](./ARCHITECTURE.md) for overall system design
