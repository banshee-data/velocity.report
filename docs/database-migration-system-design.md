# Database Migration System Design

**Status**: Design Proposal
**Date**: November 10, 2025
**Author**: Agent Ictinus
**Goal**: Design a lightweight, maintainable database migration system for velocity.report

> **⚠️ CRITICAL: Pure-Go SQLite Driver Required**
>
> This design uses **`modernc.org/sqlite`** (pure-Go, no CGO) via golang-migrate's `database/sqlite` driver.
> The codebase already uses `modernc.org/sqlite` to avoid CGO dependencies and simplify cross-compilation
> for Raspberry Pi and other ARM64 targets.
>
> **DO NOT use `database/sqlite3`** which depends on `github.com/mattn/go-sqlite3` (requires CGO).

## Executive Summary

This document proposes a database migration system for velocity.report that:

- Tracks applied migrations via a metadata table in SQLite
- Supports forward (up) and rollback (down) migrations
- Works with existing `internal/db/migrations/` SQL files
- Minimizes maintenance burden
- Integrates with current deployment patterns (systemd service, manual setup)
- Preserves privacy-first, local-only architecture
- **Uses pure-Go SQLite driver (no CGO)** to match existing codebase

## Problem Statement

### Current State

The velocity.report project currently uses an **ad-hoc migration approach**:

1. **Manual SQL execution**: Users run migrations manually via `sqlite3 sensor_data.db < migration.sql`
2. **No state tracking**: No record of which migrations have been applied
3. **No rollback support**: Migrations are one-way only
4. **Documentation-based process**: Users must read `internal/db/migrations/README.md` to know what to run
5. **Schema.sql bootstrapping**: `NewDB()` runs full schema.sql, but doesn't help with incremental changes

**Existing migrations** (6 files in `internal/db/migrations/`):

- `20250826_rename_tables_column.sql` - Table/column renames
- `20250827_migrate_ro_to_unix_timestamp.sql` - Timestamp conversion
- `20250929_migrate_data_to_radar_data.sql` - Legacy table migration
- `20251014_create_site_table.sql` - Site configuration table
- `20251016_create_site_reports.sql` - Report tracking table
- `20251022_add_velocity_report_prefix.sql` - Filename standardization

### Problems with Current Approach

1. **No idempotency guarantee**: Users don't know if a migration was already applied
2. **Manual error-prone process**: Users must track state themselves
3. **No version visibility**: Can't query "what schema version am I on?"
4. **Deployment friction**: New deployments must manually apply all historical migrations
5. **No rollback path**: Breaking changes are permanent
6. **Testing complexity**: Hard to test migrations in CI/CD

### User Pain Points

**For developers:**

- "Did I already run this migration on my local DB?"
- "How do I test a new migration without polluting my database?"
- "What schema version is this database?"

**For operators:**

- "Which migrations do I need to apply after upgrading?"
- "How do I rollback if a migration breaks production?"
- "What happens if I accidentally run a migration twice?"

**For new deployments:**

- "Do I run schema.sql or all the migrations?"
- "What if schema.sql diverges from migrations?"

## Design Goals

### Functional Requirements

1. **Migration tracking**: Store metadata about applied migrations
2. **Idempotency**: Running a migration twice should be safe (no-op)
3. **Ordering**: Migrations must apply in deterministic order
4. **Rollback support**: Each migration should have a down/rollback script
5. **Version visibility**: Users can query current schema version
6. **Baseline support**: Existing databases can be baselined to current version
7. **File-based migrations**: Continue using SQL files in `internal/db/migrations/`
8. **Cross-platform**: Works on Raspberry Pi ARM64 and developer machines

### Non-Functional Requirements

1. **Minimal dependencies**: Avoid heavy frameworks (keep binary small)
2. **Simple maintenance**: Easy to understand and modify
3. **Local-only**: No cloud/external service dependencies (privacy principle)
4. **Backward compatible**: Don't break existing deployments
5. **CI/CD friendly**: Easy to test in automated pipelines
6. **Low cognitive overhead**: Developers shouldn't need to learn complex DSLs

### Anti-Goals (Out of Scope)

1. **Database branching/merging**: Single linear migration history
2. **Schema diffs**: No automatic migration generation
3. **ORM integration**: Migrations are pure SQL
4. **Multi-database support**: SQLite only
5. **Distributed migrations**: Single-node only (no coordination)

## Current Architecture Context

### Database Usage Pattern

```
┌─────────────────────────────────────────────────────────────┐
│  Go Server (cmd/radar/)                                      │
│    ├── internal/db.NewDB(path)                               │
│    │     └── Runs schema.sql on new databases                │
│    └── Systemd service: /var/lib/velocity-report/            │
├─────────────────────────────────────────────────────────────┤
│  Python PDF Generator (tools/pdf-generator/)                 │
│    └── Direct SQLite access (read-only queries)              │
├─────────────────────────────────────────────────────────────┤
│  Web Frontend (web/)                                         │
│    └── HTTP API to Go server (no direct DB access)           │
├─────────────────────────────────────────────────────────────┤
│  SQLite Database                                             │
│    Location: /var/lib/velocity-report/sensor_data.db         │
│    Tables: radar_data, site, site_reports, lidar_*, etc.     │
└─────────────────────────────────────────────────────────────┘
```

### Key Constraints

1. **Raspberry Pi 4 target**: Limited CPU/memory, ARM64 architecture
2. **Single SQLite file**: No clustering, local-only storage
3. **Systemd service**: Runs as `velocity` user with specific WorkingDirectory
4. **Multiple entry points**: Binary, Python tools, manual sqlite3 access
5. **Privacy by design**: No external network calls for migrations

## Selected Solution: golang-migrate

**golang-migrate** is an industry-standard migration library for Go applications with first-class pure-Go SQLite support.

**Repository**: https://github.com/golang-migrate/migrate
**License**: MIT | **Language**: Go | **Community**: ~15k stars

### Why golang-migrate?

1. **Separate up/down files for manual execution**

   - Each migration has distinct `.up.sql` and `.down.sql` files
   - Can run migrations manually with `sqlite3` command-line tool without the framework
   - Clear separation makes rollback operations explicit
   - No special markers or syntax—pure SQL

2. **Pure-Go SQLite support (no CGO)**

   - Uses `modernc.org/sqlite` via `database/sqlite` driver
   - Matches existing codebase (already uses `modernc.org/sqlite`)
   - No CGO dependency = simpler cross-compilation for Raspberry Pi ARM64
   - Automatic transaction wrapping per migration

3. **Battle-tested and mature**

   - Industry-standard solution used by thousands of projects
   - Extensive community support and documentation
   - Dirty state detection (failed migrations flagged)
   - Force version capability for manual override

4. **Operational reliability**
   - Clear file structure reduces confusion
   - Manual fallback option important for production
   - Pure SQL files can be version-controlled
   - Emergency recovery via direct `sqlite3` execution

### Integration Example

```go
import "github.com/golang-migrate/migrate/v4"
import _ "github.com/golang-migrate/migrate/v4/database/sqlite"  // Pure-Go driver
import _ "github.com/golang-migrate/migrate/v4/source/file"

func (db *DB) RunMigrations(migrationsDir string) error {
    m, err := migrate.New(
        "file://"+migrationsDir,
        "sqlite://"+db.path)  // Note: sqlite:// not sqlite3://
    if err != nil {
        return err
    }
    return m.Up()
}
```

**File structure:**

```
internal/db/migrations/
├── 000001_initial_schema.up.sql
├── 000001_initial_schema.down.sql
├── 000002_add_site_table.up.sql
├── 000002_add_site_table.down.sql
```

### Trade-offs

**Costs:**

- Requires renaming existing 6 migration files to sequential format
- Adds ~2MB to binary size
- More opinionated file naming (sequential numbers)

**Benefits:**

- Separate files can be run manually (critical for emergency recovery)
- Pure-Go implementation matches existing codebase
- Industry-standard with extensive community support
- Well-documented SQLite-specific behavior

#### Phase 1: Foundation (Week 1)

**Tasks:**

1. Add `github.com/golang-migrate/migrate/v4` to `go.mod`
2. Create `internal/db/migrate.go` wrapper:

   ```go
   import "github.com/golang-migrate/migrate/v4"
   import _ "github.com/golang-migrate/migrate/v4/database/sqlite"  // Pure-Go driver
   import _ "github.com/golang-migrate/migrate/v4/source/file"

   func (db *DB) RunMigrations(migrationsDir string) error {
       m, err := migrate.New(
           "file://"+migrationsDir,
           "sqlite://"+db.path)  // Note: sqlite:// not sqlite3://
       if err != nil {
           return err
       }
       return m.Up()
   }
   ```

   **Important**: Use `database/sqlite` (pure-Go, `modernc.org/sqlite`) NOT `database/sqlite3` (CGO).
   This matches the existing codebase and avoids CGO dependencies for cross-compilation.

3. Create `schema_migrations` baseline for existing databases
4. Add integration test (`internal/db/migrate_test.go`)

**Deliverables:**

**Note:** Mark checkboxes as tasks are completed during implementation.

- [ ] Migration runner in `internal/db/migrate.go`
- [ ] Unit tests with coverage >80%
- [ ] Documentation: `docs/database-migrations.md`

#### Phase 2: Migration Files (Week 1-2)

**Tasks:**

1. Rename and split 6 existing migrations to golang-migrate format:

   ```
   20250826_rename_tables_column.sql
     → 000001_rename_tables_column.up.sql
     → 000001_rename_tables_column.down.sql

   20250827_migrate_ro_to_unix_timestamp.sql
     → 000002_migrate_ro_to_unix_timestamp.up.sql
     → 000002_migrate_ro_to_unix_timestamp.down.sql

   ... (and so on for all 6 migrations in chronological order)
   ```

2. Write down migrations for each file:

   - Analyze each migration's changes
   - Create reverse operations (down migrations)
   - Test on copy of production database
   - Verify manual execution: `sqlite3 db.db < 000001_rename_tables_column.up.sql`

3. Document manual fallback procedures

**Deliverables:**

- [ ] 12 migration files (6 up + 6 down) in sequential format
- [ ] Migration testing guide
- [ ] Manual execution documentation
- [ ] Rollback procedures documented

#### Phase 3: CLI Integration (Week 2)

**Tasks:**

1. Add migration commands to radar binary:
   ```bash
   velocity-report migrate up
   velocity-report migrate down
   velocity-report migrate status
   velocity-report migrate create <name>
   ```
2. Update `cmd/radar/radar.go` to run migrations on startup (optional flag)
3. Add Makefile targets:

   ```makefile
   migrate-up:
       velocity-report migrate up

   migrate-down:
       velocity-report migrate down

   migrate-status:
       velocity-report migrate status
   ```

**Deliverables:**

- [ ] CLI commands in radar binary
- [ ] Makefile targets
- [ ] Help text and examples

#### Phase 4: Deployment Integration (Week 3)

**Tasks:**

1. Update `scripts/setup-radar-host.sh` to run migrations:
   ```bash
   log_info "Running database migrations..."
   /usr/local/bin/velocity-report migrate up --db ${DATA_DIR}/sensor_data.db
   ```
2. Add migration step to CI/CD (GitHub Actions):
   ```yaml
   - name: Test migrations
     run: make test-migrations
   ```
3. Update systemd service to auto-migrate on startup (optional):
   ```ini
   ExecStartPre=/usr/local/bin/velocity-report migrate up --db /var/lib/velocity-report/sensor_data.db
   ```
4. Create migration troubleshooting guide

**Deliverables:**

- [ ] Updated setup script
- [ ] CI/CD integration
- [ ] Troubleshooting documentation
- [ ] Operator runbook

#### Phase 5: Documentation & Training (Week 3-4)

**Tasks:**

1. Create comprehensive migration guide:
   - How to create new migrations
   - Testing procedures
   - Rollback processes
   - Common issues and solutions
2. Update `internal/db/migrations/README.md`
3. Add migration examples to documentation site
4. Record demo video (optional)

**Deliverables:**

- [ ] Migration developer guide
- [ ] Updated README files
- [ ] Example migrations
- [ ] Training materials

### Migration File Standards

#### Naming Convention

**golang-migrate format** (sequential numbering):

```
{version}_{description}.up.sql
{version}_{description}.down.sql

Examples:
000001_initial_schema.up.sql
000001_initial_schema.down.sql
000002_add_site_table.up.sql
000002_add_site_table.down.sql
000003_add_user_preferences_table.up.sql
000003_add_user_preferences_table.down.sql
```

**Version numbering:**

- Sequential integers (000001, 000002, 000003, ...)
- Padded with zeros for proper sorting
- Must be unique across all migrations
- Version extracted from filename determines execution order

**Note**: Existing migrations in `internal/db/migrations/` use YYYYMMDD format. During implementation, these will need to be renamed to sequential format (e.g., `20250826_rename_tables_column.sql` → `000001_rename_tables_column.up.sql`).

#### File Format (golang-migrate)

**Separate up and down files:**

```sql
-- File: 000001_create_site_table.up.sql
-- Migration: Create site table for location and configuration data
-- Author: Developer Name
-- Date: YYYY-MM-DD

CREATE TABLE IF NOT EXISTS site (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    location TEXT NOT NULL,
    speed_limit INTEGER DEFAULT 25
);

CREATE INDEX IF NOT EXISTS idx_site_name ON site(name);
```

```sql
-- File: 000001_create_site_table.down.sql
-- Rollback: Remove site table

DROP INDEX IF EXISTS idx_site_name;
DROP TABLE IF EXISTS site;
```

**Key points:**

- No special markers or syntax required (pure SQL)
- Each file can be run manually: `sqlite3 db.db < 000001_create_site_table.up.sql`
- Comments are optional but recommended for clarity
- golang-migrate automatically wraps each file in a transaction (unless `x-no-tx-wrap` is set)

#### Best Practices

1. **Atomic changes**: Each migration should do one logical thing
2. **Idempotent SQL**: Use `IF EXISTS` / `IF NOT EXISTS` where possible
3. **Data migrations**: Separate DDL (schema) from DML (data) migrations
4. **Testing**: Always test down migration after up migration
5. **Backward compatibility**: Consider running code during migration
6. **Comments**: Explain non-obvious changes
7. **Transactions**: SQLite supports transactional DDL (use it!)

### Metadata Table Schema

**golang-migrate's actual schema** (recommended approach):

````sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER NOT NULL,
    dirty INTEGER NOT NULL  -- Boolean: 0=false, 1=true
);

CREATE UNIQUE INDEX IF NOT EXISTS version_unique ON schema_migrations (version);

**Note**: SQLite stores these as INTEGER types. The `dirty` field uses 0 for false and 1 for true.
**Field descriptions:**
- `version`: Migration version number (e.g., 000001, 000002)
- `dirty`: Boolean flag indicating if a migration failed mid-execution
  - `true` = migration started but didn't complete successfully
  - `false` = migration completed successfully
  - Used for detecting and preventing running migrations on corrupted state

**Note**: golang-migrate uses a minimal schema by design. The version number comes from the filename (e.g., `000001_initial_schema.up.sql` → version 1). The `dirty` flag is critical for detecting failed migrations and preventing further migrations until the issue is resolved.

### Rollback Strategy

#### When to Rollback

- ❌ **Don't rollback** for data migrations (may lose data)
- ✅ **Do rollback** for schema changes with no data loss
- ✅ **Do rollback** for failed migrations in development
- ⚠️ **Carefully rollback** in production (test first!)

#### Rollback Types

1. **Automatic rollback** (transaction failure):
   - Migration fails mid-execution
   - SQLite rolls back entire transaction
   - `schema_migrations` not updated

2. **Manual rollback** (operator initiated):
   ```bash
   # Rollback one migration
   velocity-report migrate down

   # Or force to specific version
   velocity-report migrate force 7
````

- Runs down script
- Removes entry from `schema_migrations`
- Validates database state

3. **Emergency rollback** (dirty state):
   ```bash
   velocity-report migrate force 6
   ```
   - Manually sets version
   - Use only when automated rollback fails
   - Requires manual database inspection

### Testing Strategy

#### Unit Tests

```go
// internal/db/migrate_test.go
func TestMigrations_UpDown(t *testing.T) {
    db := setupTestDB(t)

    // Create migrate instance (use pure-Go sqlite driver)
    m, err := migrate.New(
        "file://migrations",
        "sqlite://"+db.path)  // Note: sqlite:// not sqlite3://
    if err != nil {
        t.Fatal(err)
    }

    // Apply all migrations
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        t.Fatal(err)
    }

    // Check version (should be 6 after applying all 6 existing migrations)
    // Note: This assumes 1:1 mapping of existing migrations to sequential versions (1-6).
    version, dirty, _ := m.Version()
    if version != 6 {
        t.Errorf("expected version 6, got %d", version)
    }
    if dirty {
        t.Error("database should not be dirty")
    }

    // Test rollback
    if err := m.Down(); err != nil && err != migrate.ErrNoChange {
        t.Fatal(err)
    }

    // Verify tables removed
    // ... table existence checks
}

func TestMigrations_Idempotency(t *testing.T) {
    db := setupTestDB(t)

    m, _ := migrate.New(
        "file://migrations",
        "sqlite://"+db.path)  // Note: sqlite:// not sqlite3://

    // Apply migrations twice
    m.Up()
    err := m.Up()

    // Second up should return ErrNoChange
    if err != nil && err != migrate.ErrNoChange {
        t.Fatal("second up should be no-op or return ErrNoChange")
    }
}
```

#### Integration Tests

```bash
# scripts/test-migrations.sh
#!/usr/bin/env bash
set -euo pipefail

# Create test database
TEST_DB=$(mktemp)
cp /var/lib/velocity-report/sensor_data.db $TEST_DB

# Run migrations up
./velocity-report migrate up --db $TEST_DB

# Verify schema
sqlite3 $TEST_DB "SELECT COUNT(*) FROM schema_migrations"

# Run migrations down
./velocity-report migrate down --db $TEST_DB

# Cleanup
rm $TEST_DB
```

#### CI/CD Tests

```yaml
# .github/workflows/test-migrations.yml
name: Test Migrations
on: [pull_request]

jobs:
  test-migrations:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: "1.25"

      - name: Run migration tests
        run: |
          make build-radar-local
          make test-migrations

      - name: Test migration idempotency
        run: |
          velocity-report migrate up --db test.db
          velocity-report migrate up --db test.db
          [ $? -eq 0 ] || exit 1
```

## Migration Checklist for Developers

### Creating a New Migration

- [ ] **Plan changes**: Document what tables/columns will change
- [ ] **Create migration files**: `{next_version}_{descriptive_name}.up.sql` and `.down.sql`
- [ ] **Write up migration**: DDL/DML for forward change
- [ ] **Write down migration**: Reverse operations (can you?)
- [ ] **Test locally**:
  - [ ] Fresh database: `./velocity-report migrate up`
  - [ ] Existing database: Apply to copy of production DB
  - [ ] Rollback: `./velocity-report migrate down`
  - [ ] Re-apply: `./velocity-report migrate up` (idempotency)
- [ ] **Update tests**: Add schema validation if needed
- [ ] **Document**: Add to migration log in PR description
- [ ] **Review**: Get approval from another developer
- [ ] **Merge**: Squash migration file in single commit

### Applying Migrations in Production

- [ ] **Backup database**: `cp sensor_data.db sensor_data.db.$(date +%Y%m%d_%H%M%S)`
- [ ] **Check current version**: `velocity-report migrate status`
- [ ] **Review pending migrations**: Check what will be applied
- [ ] **Stop service**: `sudo systemctl stop velocity-report.service`
- [ ] **Apply migrations**: `velocity-report migrate up`
- [ ] **Verify**: Check logs for errors
- [ ] **Start service**: `sudo systemctl start velocity-report.service`
- [ ] **Monitor**: Watch logs for anomalies
- [ ] **Keep backup**: Hold for 7 days before deleting

## Risks & Mitigations

### Risk 1: Migration Breaks Production Database

**Likelihood**: Medium | **Impact**: High

**Mitigation:**

- ✅ Mandatory backup before applying migrations
- ✅ Test migrations on copy of production DB first
- ✅ Rollback procedures documented
- ✅ Down migrations tested
- ✅ Monitoring and alerts on service health

### Risk 2: Conflicting Migrations (Multiple Developers)

**Likelihood**: Low | **Impact**: Medium

**Mitigation:**

- ✅ Sequential numbering prevents conflicts (next available number)
- ✅ PR reviews catch conflicts
- ✅ CI/CD tests detect migration issues
- ✅ Clear merge conflict resolution process (renumber if needed)

### Risk 3: Schema.sql Diverges from Migrations

**Likelihood**: Medium | **Impact**: Medium

**Mitigation:**

- ✅ Generate schema.sql from fully-migrated database periodically
- ✅ Document which file is authoritative (schema.sql for new DBs, migrations for existing)
- ✅ Add test to verify schema.sql matches final migration state
- ✅ Make schema.sql regeneration part of release process

### Risk 4: Forgotten Down Migration

**Likelihood**: Medium | **Impact**: Low

**Mitigation:**

- ✅ PR template checklist includes "down migration exists"
- ✅ CI/CD fails if down migration missing
- ✅ Code review catches missing down migrations
- ✅ Some changes can't be rolled back (document as "no-op down")

### Risk 5: Slow Migration on Large Database

**Likelihood**: Low | **Impact**: Medium

**Mitigation:**

- ✅ Test migrations on production-sized datasets
- ✅ Add progress logging for long-running migrations
- ✅ Consider online migrations for large tables
- ✅ Schedule migrations during low-traffic windows
- ✅ Monitor execution time in metadata table

## Success Metrics

### Developer Experience

- ✅ Time to create new migration: <5 minutes
- ✅ Time to test migration locally: <2 minutes
- ✅ Migration failures caught in CI: 100%
- ✅ Developer confidence in migration process: High

### Operational Excellence

- ✅ Migration success rate: >99%
- ✅ Time to apply migrations in production: <5 minutes
- ✅ Rollback success rate: >95%
- ✅ Database corruption incidents: 0

### System Reliability

- ✅ Uptime during migrations: >99.9%
- ✅ Data loss incidents: 0
- ✅ Schema drift incidents: 0

## Future Enhancements (Post-MVP)

### Phase 2 Enhancements

1. **Automated schema.sql regeneration**: Generate from migrated database
2. **Migration conflict detection**: Warn on parallel migrations
3. **Online migrations**: Apply without downtime
4. **Dry-run mode**: Preview migration without applying
5. **Checksum validation**: Detect manual schema modifications
6. **Migration templates**: `velocity-report migrate create` with boilerplate

### Advanced Features (Optional)

1. **Multi-version jumps**: Skip to specific version
2. **Partial rollbacks**: Down to specific version
3. **Migration dependencies**: Explicit dependency graph
4. **Data validation**: Post-migration data integrity checks
5. **Performance profiling**: Track migration execution time trends
6. **Notifications**: Slack/email on migration events (careful: privacy!)

## Appendix B: Example Migration Workflows

### Workflow 1: Add New Table

**File: 000007_add_user_preferences.up.sql**

```sql
-- Migration: Add user preferences table
CREATE TABLE IF NOT EXISTS user_preferences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL UNIQUE,
    theme TEXT DEFAULT 'light',
    timezone TEXT DEFAULT 'UTC',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE INDEX IF NOT EXISTS idx_user_prefs_user_id ON user_preferences(user_id);
```

**File: 000007_add_user_preferences.down.sql**

```sql
-- Rollback: Remove user preferences table
DROP INDEX IF EXISTS idx_user_prefs_user_id;
DROP TABLE IF EXISTS user_preferences;
```

### Workflow 2: Add Column (Backward Compatible)

**File: 000008_add_radar_data_confidence.up.sql**

```sql
-- Migration: Add confidence score to radar data
ALTER TABLE radar_data ADD COLUMN confidence REAL DEFAULT 1.0;

-- Update existing rows (optional)
UPDATE radar_data SET confidence = 0.8 WHERE magnitude < 100;
```

**File: 000008_add_radar_data_confidence.down.sql**

```sql
-- Rollback: Remove confidence column
-- Note: SQLite supports DROP COLUMN since version 3.35.0 (March 2021).
-- For older SQLite versions, use this workaround: create a new table without the column and copy data.

CREATE TABLE radar_data_new (
    write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec')),
    raw_event JSON NOT NULL,
    uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED,
    magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED,
    speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
);

INSERT INTO radar_data_new (write_timestamp, raw_event)
SELECT write_timestamp, raw_event FROM radar_data;

DROP TABLE radar_data;
ALTER TABLE radar_data_new RENAME TO radar_data;

-- Note: This is destructive! Carefully consider if rollback is necessary
-- Alternative: Document "Down migration not supported - schema change is backward compatible"
```

### Workflow 3: Data Migration

**File: 000009_normalize_speed_units.up.sql**

```sql
-- Migration: Convert all speeds from mph to m/s
-- NOTE: This is a data migration - down migration will lose precision!

UPDATE radar_data
SET raw_event = json_set(
    raw_event,
    '$.speed',
    CAST(json_extract(raw_event, '$.speed') AS REAL) * 0.44704
)
WHERE json_extract(raw_event, '$.speed_unit') = 'mph';

UPDATE radar_data
SET raw_event = json_set(raw_event, '$.speed_unit', 'mps');
```

**File: 000009_normalize_speed_units.down.sql**

```sql
-- Rollback: Convert speeds back from m/s to mph
-- WARNING: Converting back to mph will lose precision due to floating point

UPDATE radar_data
SET raw_event = json_set(
    raw_event,
    '$.speed',
    CAST(json_extract(raw_event, '$.speed') AS REAL) / 0.44704
)
WHERE json_extract(raw_event, '$.speed_unit') = 'mps';

UPDATE radar_data
SET raw_event = json_set(raw_event, '$.speed_unit', 'mph');
```

## Appendix C: Troubleshooting Guide

### Problem: "Migration already applied"

```bash
$ velocity-report migrate up
OK   000001_initial_schema.up.sql
OK   000002_add_site_table.up.sql
No change
```

**Solution**: This is normal. All migrations were already applied. Check status:

```bash
velocity-report migrate status
```

### Problem: "Dirty migration" Error

```bash
$ velocity-report migrate up
error: Dirty database version 7. Fix and force version.
```

**Cause**: Previous migration failed mid-execution.

**Solution**:

1. Check database state: `sqlite3 sensor_data.db ".tables"`
2. Manual inspection: Determine if migration partially applied
3. Fix manually or force version:
   ```bash
   velocity-report migrate force 6  # Go back to last known good version
   velocity-report migrate up       # Try again
   ```

### Problem: Schema.sql and Migrations Out of Sync

**Symptoms**: Fresh database has different schema than migrated database.

**Solution**:

1. Create fresh database: `rm test.db && velocity-report --db test.db migrate up`
2. Dump schema: `sqlite3 test.db .schema > schema_from_migrations.sql`
3. Compare: `diff internal/db/schema.sql schema_from_migrations.sql`
4. Update schema.sql to match or fix migrations

### Problem: Can't Rollback Data Migration

**Cause**: Down migration would lose data.

**Solution**: Document in down migration file:

```sql
-- File: 000009_normalize_speed_units.down.sql
-- WARNING: This migration cannot be safely rolled back
-- Data has been transformed and cannot be restored to original state
-- Manual recovery required if rollback needed

-- If you must rollback, this will convert back but lose precision:
-- (include lossy conversion code here)
```

## Appendix D: References

### Go Migration Libraries

- **golang-migrate**: https://github.com/golang-migrate/migrate
- **goose**: https://github.com/pressly/goose
- **sql-migrate**: https://github.com/rubenv/sql-migrate
- **dbmate**: https://github.com/amacneil/dbmate

### SQLite Resources

- **SQLite WAL Mode**: https://www.sqlite.org/wal.html
- **SQLite Transactions**: https://www.sqlite.org/lang_transaction.html
- **SQLite ALTER TABLE**: https://www.sqlite.org/lang_altertable.html

### Best Practices

- **Database Migration Best Practices**: https://www.prisma.io/dataguide/types/relational/migration-strategies
- **Zero-Downtime Migrations**: https://fly.io/blog/zero-downtime-postgres-migrations/

---

**Document Version**: 2.0
**Last Updated**: November 13, 2025
**Review Date**: December 13, 2025
**Approvers**: [TBD]

**Changelog**:

- v2.0 (Nov 13, 2025): Streamlined to focus solely on golang-migrate solution; removed alternative options
- v1.0 (Nov 10, 2025): Initial version with multiple solution options
