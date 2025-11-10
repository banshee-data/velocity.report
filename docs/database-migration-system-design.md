# Database Migration System Design

**Status**: Design Proposal  
**Date**: November 10, 2025  
**Author**: Agent Ictinus  
**Goal**: Design a lightweight, maintainable database migration system for velocity.report

## Executive Summary

This document proposes a database migration system for velocity.report that:
- Tracks applied migrations via a metadata table in SQLite
- Supports forward (up) and rollback (down) migrations
- Works with existing `data/migrations/` SQL files
- Minimizes maintenance burden
- Integrates with current deployment patterns (systemd service, manual setup)
- Preserves privacy-first, local-only architecture

## Problem Statement

### Current State

The velocity.report project currently uses an **ad-hoc migration approach**:

1. **Manual SQL execution**: Users run migrations manually via `sqlite3 sensor_data.db < migration.sql`
2. **No state tracking**: No record of which migrations have been applied
3. **No rollback support**: Migrations are one-way only
4. **Documentation-based process**: Users must read `data/migrations/README.md` to know what to run
5. **Schema.sql bootstrapping**: `NewDB()` runs full schema.sql, but doesn't help with incremental changes

**Existing migrations** (6 files in `data/migrations/`):
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
7. **File-based migrations**: Continue using SQL files in `data/migrations/`
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

## Migration System Design Options

### Option 1: golang-migrate/migrate (Popular Go Library)

**Overview**: Industry-standard migration library for Go applications.

**Repository**: https://github.com/golang-migrate/migrate  
**Stars**: ~15k | **License**: MIT | **Language**: Go

**SQLite Support**: ✅ Yes, via `sqlite3` driver using `github.com/mattn/go-sqlite3` (cgo)
- Automatic transaction wrapping for each migration
- Uses standard `schema_migrations` table
- Well-documented SQLite-specific behavior

**How it works:**
```go
import "github.com/golang-migrate/migrate/v4"
import _ "github.com/golang-migrate/migrate/v4/database/sqlite3"
import _ "github.com/golang-migrate/migrate/v4/source/file"

m, err := migrate.New(
    "file://data/migrations",
    "sqlite3:///var/lib/velocity-report/sensor_data.db")
m.Up() // Apply pending migrations
```

**File structure:**
```
data/migrations/
├── 000001_initial_schema.up.sql
├── 000001_initial_schema.down.sql
├── 000002_add_site_table.up.sql
├── 000002_add_site_table.down.sql
```

**Pros:**
- ✅ Mature, battle-tested (used by thousands of projects)
- ✅ **SQLite support via `sqlite3` driver** (first-class citizen)
- ✅ CLI tool + Go library (flexible usage)
- ✅ Automatic version tracking table (`schema_migrations`)
- ✅ Dirty state detection (failed migrations flagged)
- ✅ Up/down migrations with rollback support
- ✅ Force version (manual override for recovery)
- ✅ **Works with SQLite's transactional DDL** (atomic migrations)
- ✅ No ORM required (pure SQL)

**Cons:**
- ❌ Requires renaming existing migration files (`.up.sql`, `.down.sql` suffix)
- ❌ Adds ~2MB to binary size (full library)
- ❌ Opinionated file naming convention
- ❌ More complex than needed for simple use case
- ❌ Need to write down migrations for existing files retroactively

**Integration effort**: Medium
- Rename 6 existing migrations (create `.up.sql` + `.down.sql` variants)
- Add `golang-migrate` to `go.mod`
- Create CLI command or integrate into `NewDB()`
- Update documentation

**Recommendation**: ⭐⭐⭐⭐ (4/5)  
Best choice if we want industry-standard tooling and don't mind the file renaming work.

---

### Option 2: pressly/goose (Lightweight Go Library)

**Overview**: Simple database migration tool focused on ease of use.

**Repository**: https://github.com/pressly/goose  
**Stars**: ~7k | **License**: MIT | **Language**: Go

**SQLite Support**: ✅ Yes, first-class support
- SQLite3 listed equally with Postgres and MySQL in documentation
- Dedicated CLI command: `goose sqlite3 ./database.db`
- Examples prominently feature SQLite usage
- No caveats or TODOs for SQLite support

**How it works:**
```go
import "github.com/pressly/goose/v3"

db, _ := sql.Open("sqlite3", "/var/lib/velocity-report/sensor_data.db")
goose.Up(db, "data/migrations")
```

**File structure:**
```
data/migrations/
├── 20250826_rename_tables_column.sql
├── 20250929_migrate_data_to_radar_data.sql
```

**SQL file format:**
```sql
-- +goose Up
CREATE TABLE site (...);

-- +goose Down
DROP TABLE site;
```

**Pros:**
- ✅ Lightweight (~500KB binary increase)
- ✅ Flexible file naming (supports `YYYYMMDD_*` pattern)
- ✅ Up/down in same file (via `-- +goose Up/Down` markers)
- ✅ Can embed migrations in binary with `//go:embed`
- ✅ CLI + library modes
- ✅ Hybrid SQL + Go migrations supported
- ✅ Active maintenance

**Cons:**
- ❌ Requires modifying existing SQL files (add `-- +goose` markers)
- ❌ Less popular than golang-migrate (fewer examples)
- ❌ Must write down migrations for existing files
- ❌ Marker syntax may confuse users running files manually

**Integration effort**: Medium-Low
- Add `-- +goose Up/Down` markers to 6 existing migrations
- Add `goose` to `go.mod`
- Create migration CLI command
- Minimal doc updates

**Recommendation**: ⭐⭐⭐⭐ (4/5)  
Good balance of simplicity and features. Better file naming flexibility than golang-migrate.

---

### Option 3: Custom Lightweight Go Implementation

**Overview**: Build a minimal migration system tailored to velocity.report needs.

**Core Components:**

1. **Metadata table** (`schema_migrations`):
   ```sql
   CREATE TABLE IF NOT EXISTS schema_migrations (
       version TEXT PRIMARY KEY,
       applied_at INTEGER NOT NULL,
       description TEXT,
       checksum TEXT,
       success INTEGER NOT NULL DEFAULT 1
   );
   ```

2. **Migration file format** (keep existing naming):
   ```
   data/migrations/
   ├── 20250826_rename_tables_column.sql      (up migration)
   ├── 20250826_rename_tables_column.down.sql (down migration)
   ```

3. **Migration runner** (`internal/db/migrate.go`):
   ```go
   func (db *DB) ApplyMigrations(migrationsDir string) error
   func (db *DB) Rollback(version string) error
   func (db *DB) CurrentVersion() (string, error)
   ```

4. **CLI tool** (`cmd/migrate/main.go` or add to existing binary):
   ```bash
   velocity-report migrate up
   velocity-report migrate down [version]
   velocity-report migrate status
   ```

**Implementation (~200 lines of Go):**

```go
package db

import (
    "crypto/sha256"
    "database/sql"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "time"
)

type Migration struct {
    Version     string
    Description string
    UpSQL       string
    DownSQL     string
    Checksum    string
}

func (db *DB) ensureMigrationsTable() error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version TEXT PRIMARY KEY,
            applied_at INTEGER NOT NULL,
            description TEXT,
            checksum TEXT,
            success INTEGER NOT NULL DEFAULT 1
        )
    `)
    return err
}

func (db *DB) loadMigrations(dir string) ([]*Migration, error) {
    files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
    if err != nil {
        return nil, err
    }
    
    migrations := make(map[string]*Migration)
    for _, f := range files {
        name := filepath.Base(f)
        if strings.HasSuffix(name, ".down.sql") {
            continue // Process down migrations separately
        }
        
        // Extract version (YYYYMMDD) and description
        parts := strings.SplitN(name, "_", 2)
        if len(parts) < 2 {
            continue
        }
        version := parts[0]
        desc := strings.TrimSuffix(parts[1], ".sql")
        
        upSQL, _ := os.ReadFile(f)
        downSQL, _ := os.ReadFile(strings.Replace(f, ".sql", ".down.sql", 1))
        
        hash := sha256.Sum256(upSQL)
        
        migrations[version] = &Migration{
            Version:     version,
            Description: desc,
            UpSQL:       string(upSQL),
            DownSQL:     string(downSQL),
            Checksum:    fmt.Sprintf("%x", hash),
        }
    }
    
    // Sort by version
    sorted := make([]*Migration, 0, len(migrations))
    for _, m := range migrations {
        sorted = append(sorted, m)
    }
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].Version < sorted[j].Version
    })
    
    return sorted, nil
}

func (db *DB) ApplyMigrations(dir string) error {
    if err := db.ensureMigrationsTable(); err != nil {
        return err
    }
    
    // Get applied migrations
    applied := make(map[string]bool)
    rows, err := db.Query("SELECT version FROM schema_migrations WHERE success = 1")
    if err != nil {
        return err
    }
    for rows.Next() {
        var v string
        rows.Scan(&v)
        applied[v] = true
    }
    rows.Close()
    
    // Load and apply pending migrations
    migrations, err := db.loadMigrations(dir)
    if err != nil {
        return err
    }
    
    for _, m := range migrations {
        if applied[m.Version] {
            continue // Already applied
        }
        
        log.Printf("Applying migration %s: %s", m.Version, m.Description)
        
        tx, err := db.Begin()
        if err != nil {
            return err
        }
        if _, err := tx.Exec(m.UpSQL); err != nil {
            tx.Rollback()
            // Mark as failed
            db.Exec(`INSERT INTO schema_migrations (version, applied_at, description, checksum, success) 
                     VALUES (?, ?, ?, ?, 0)`, m.Version, time.Now().Unix(), m.Description, m.Checksum)
            return fmt.Errorf("migration %s failed: %w", m.Version, err)
        }
        
        // Record success
        tx.Exec(`INSERT INTO schema_migrations (version, applied_at, description, checksum, success) 
                 VALUES (?, ?, ?, ?, 1)`, m.Version, time.Now().Unix(), m.Description, m.Checksum)
        tx.Commit()
    }
    
    return nil
}

func (db *DB) Rollback(targetVersion string) error {
    // Implementation: Load migrations, apply down scripts in reverse order
    // Similar structure to ApplyMigrations but in reverse
}

func (db *DB) CurrentVersion() (string, error) {
    var version string
    err := db.QueryRow(`SELECT version FROM schema_migrations 
                        WHERE success = 1 ORDER BY version DESC LIMIT 1`).Scan(&version)
    return version, err
}
```

**Pros:**
- ✅ Minimal dependencies (zero external libraries)
- ✅ Tailored to exact needs (no feature bloat)
- ✅ Full control over behavior
- ✅ Can keep existing file naming convention
- ✅ Easy to understand and modify (~200 LOC)
- ✅ No binary size increase
- ✅ Learning opportunity for team

**Cons:**
- ❌ Need to write and test from scratch
- ❌ No community support/examples
- ❌ Need to handle edge cases ourselves
- ❌ Still need to create `.down.sql` files for rollbacks
- ❌ Maintenance burden on team

**Integration effort**: High
- Write `internal/db/migrate.go` (~200 lines)
- Add CLI commands
- Write comprehensive tests
- Document behavior
- Handle edge cases (dirty migrations, checksums, etc.)

**Recommendation**: ⭐⭐⭐ (3/5)  
Best for learning and full control, but higher risk and maintenance burden.

---

### Option 4: Shell Script + SQLite Pragmas

**Overview**: Minimal shell-based migration runner using SQLite's built-in features.

**File structure:**
```
data/migrations/
├── 20250826_rename_tables_column.sql
├── 20250826_rename_tables_column.down.sql
scripts/
├── migrate-up.sh
├── migrate-down.sh
└── migrate-status.sh
```

**Implementation**: `scripts/migrate-up.sh`
```bash
#!/usr/bin/env bash
set -euo pipefail

DB_PATH="${1:-/var/lib/velocity-report/sensor_data.db}"
MIGRATIONS_DIR="$(dirname "$0")/../data/migrations"

# Create migrations table if not exists
sqlite3 "$DB_PATH" <<SQL
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at INTEGER NOT NULL,
    description TEXT
);
SQL

# Get applied migrations
APPLIED=$(sqlite3 "$DB_PATH" "SELECT version FROM schema_migrations ORDER BY version")

# Apply pending migrations
for migration in "$MIGRATIONS_DIR"/*.sql; do
    [[ "$migration" == *.down.sql ]] && continue
    
    VERSION=$(basename "$migration" | cut -d_ -f1)
    DESC=$(basename "$migration" .sql | cut -d_ -f2-)
    
    if echo "$APPLIED" | grep -q "^$VERSION$"; then
        echo "⏭ Skipping $VERSION (already applied)"
        continue
    fi
    
    echo "▶ Applying $VERSION: $DESC"
    
    sqlite3 "$DB_PATH" <<SQL
BEGIN TRANSACTION;
$(cat "$migration")
INSERT INTO schema_migrations (version, applied_at, description) 
VALUES ('$VERSION', strftime('%s', 'now'), '$DESC');
COMMIT;
SQL
    
    if [ $? -eq 0 ]; then
        echo "✓ Migration $VERSION applied successfully"
    else
        echo "✗ Migration $VERSION failed"
        exit 1
    fi
done

echo "✓ All migrations applied"
```

**Pros:**
- ✅ Zero dependencies (shell + sqlite3)
- ✅ Extremely simple to understand
- ✅ No binary changes needed
- ✅ Easy to debug (plain SQL and bash)
- ✅ Works anywhere (Raspberry Pi, CI, local)
- ✅ Can run manually or from systemd

**Cons:**
- ❌ No integration with Go code
- ❌ No checksum validation
- ❌ Limited error handling
- ❌ Hard to test migrations in Go unit tests
- ❌ No automatic rollback (must run down scripts manually)
- ❌ Bash-specific (Windows compatibility issues)

**Integration effort**: Low
- Write 3 shell scripts (~150 lines total)
- Create `.down.sql` files for existing migrations
- Update documentation
- Add to Makefile targets

**Recommendation**: ⭐⭐⭐ (3/5)  
Good for quick MVP, but limited features and hard to integrate with Go application lifecycle.

---

### Option 5: Python Migration Tool (Custom)

**Overview**: Python-based migration runner that can be used by PDF generator and as standalone tool.

**Why Python?**
- PDF generator already has SQLite access
- Python is available on all deployment environments
- Simple scripting for file I/O and SQL execution

**File structure:**
```
tools/db-migrate/
├── migrate.py
├── __init__.py
└── README.md
```

**Implementation**: `tools/db-migrate/migrate.py`
```python
#!/usr/bin/env python3
import sqlite3
import hashlib
import sys
from pathlib import Path
from datetime import datetime

class MigrationRunner:
    def __init__(self, db_path, migrations_dir):
        self.db_path = Path(db_path)
        self.migrations_dir = Path(migrations_dir)
        self.conn = sqlite3.connect(db_path)
        self._ensure_migrations_table()
    
    def _ensure_migrations_table(self):
        self.conn.execute("""
            CREATE TABLE IF NOT EXISTS schema_migrations (
                version TEXT PRIMARY KEY,
                applied_at INTEGER NOT NULL,
                description TEXT,
                checksum TEXT
            )
        """)
        self.conn.commit()
    
    def get_applied_migrations(self):
        cursor = self.conn.execute("SELECT version FROM schema_migrations ORDER BY version")
        return {row[0] for row in cursor}
    
    def load_migrations(self):
        migrations = []
        for f in sorted(self.migrations_dir.glob("*.sql")):
            if f.name.endswith(".down.sql"):
                continue
            
            version = f.name.split("_")[0]
            desc = "_".join(f.name.split("_")[1:]).replace(".sql", "")
            up_sql = f.read_text()
            
            down_file = f.with_suffix("").with_suffix(".down.sql")
            down_sql = down_file.read_text() if down_file.exists() else None
            
            checksum = hashlib.sha256(up_sql.encode()).hexdigest()
            
            migrations.append({
                "version": version,
                "description": desc,
                "up_sql": up_sql,
                "down_sql": down_sql,
                "checksum": checksum,
            })
        
        return sorted(migrations, key=lambda m: m["version"])
    
    def apply_migrations(self):
        applied = self.get_applied_migrations()
        migrations = self.load_migrations()
        
        for m in migrations:
            if m["version"] in applied:
                print(f"⏭ Skipping {m['version']} (already applied)")
                continue
            
            print(f"▶ Applying {m['version']}: {m['description']}")
            
            try:
                cursor = self.conn.cursor()
                cursor.execute("BEGIN")
                cursor.executescript(m["up_sql"])
                cursor.execute(
                    "INSERT INTO schema_migrations (version, applied_at, description, checksum) VALUES (?, ?, ?, ?)",
                    (m["version"], int(datetime.now().timestamp()), m["description"], m["checksum"])
                )
                cursor.execute("COMMIT")
                print(f"✓ Migration {m['version']} applied successfully")
            except Exception as e:
                self.conn.rollback()
                print(f"✗ Migration {m['version']} failed: {e}")
                return False
        
        print("✓ All migrations applied")
        return True
    
    def current_version(self):
        cursor = self.conn.execute("SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1")
        row = cursor.fetchone()
        return row[0] if row else None

if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Database migration tool")
    parser.add_argument("--db", default="/var/lib/velocity-report/sensor_data.db", help="Database path")
    parser.add_argument("--migrations", default="data/migrations", help="Migrations directory")
    parser.add_argument("command", choices=["up", "status"], help="Command to run")
    args = parser.parse_args()
    
    runner = MigrationRunner(args.db, args.migrations)
    
    if args.command == "up":
        runner.apply_migrations()
    elif args.command == "status":
        version = runner.current_version()
        print(f"Current version: {version or 'none'}")
```

**Usage:**
```bash
python tools/db-migrate/migrate.py --db sensor_data.db up
python tools/db-migrate/migrate.py --db sensor_data.db status
```

**Pros:**
- ✅ Python already available on all systems
- ✅ Easy to integrate with PDF generator
- ✅ Simple, readable code (~150 lines)
- ✅ Cross-platform (Windows, macOS, Linux)
- ✅ No Go dependencies needed
- ✅ Can use from virtual environment

**Cons:**
- ❌ Not integrated with Go server startup
- ❌ Requires Python to be installed/configured
- ❌ Separate tool from main binary
- ❌ Can't easily call from Go unit tests
- ❌ Another language/ecosystem to maintain

**Integration effort**: Low-Medium
- Write Python migration tool (~150 lines)
- Add to Makefile targets
- Create `.down.sql` files
- Update documentation

**Recommendation**: ⭐⭐⭐ (3/5)  
Good if Python is already a first-class citizen. Reasonable for mixed Go/Python codebase.

## Recommended Approach

### SQLite as First-Class Concern

**Critical Requirement**: Any migration framework must have first-class SQLite support, not just "supports multiple databases including SQLite."

**Both golang-migrate and goose meet this requirement:**

| Criterion | golang-migrate | goose |
|-----------|---------------|-------|
| SQLite Documentation | Dedicated driver docs | Prominent in examples |
| CLI Support | ✅ `migrate -database sqlite3://` | ✅ `goose sqlite3` |
| Production Usage | ✅ Widely used | ✅ Well-established |
| Transaction Handling | ✅ Auto-wraps migrations | ✅ Standard SQL transactions |
| Caveats/TODOs | None for main driver | None |

Both are suitable for velocity.report's SQLite-only architecture.

### Primary Recommendation: **Option 1 - golang-migrate/migrate**

**Why golang-migrate?**

1. **Separate up/down files for manual execution** (per user requirement)
   - Each migration has distinct `.up.sql` and `.down.sql` files
   - Can run migrations manually with `sqlite3` without framework
   - Clear separation makes rollback operations explicit
   - No special markers or syntax in SQL files—pure SQL

2. **SQLite support is production-ready**
   - Uses `github.com/mattn/go-sqlite3` (industry standard)
   - Automatic transaction wrapping per migration
   - Configurable via `x-no-tx-wrap` parameter if needed
   - Well-documented SQLite-specific behavior

3. **Battle-tested and mature**
   - Industry-standard solution used by thousands of projects
   - Extensive community support and documentation
   - Dirty state detection (failed migrations flagged)
   - Force version capability for manual override

4. **Operational reliability**
   - Clear file structure reduces confusion
   - Manual fallback option important for production
   - Pure SQL files can be version-controlled separately
   - Emergency recovery via direct `sqlite3` execution

**Trade-offs:**
- ❌ Larger binary size (~2MB vs ~500KB for goose)
- ❌ Requires renaming existing 6 migration files
- ❌ More opinionated file naming convention (sequential numbers)

**Why separate files matter**: As noted by project maintainer, separate `.up.sql` and `.down.sql` files can be run directly without the framework if needed—critical for emergency scenarios and production debugging.

### Alternative Recommendation: **Option 2 - pressly/goose** (If binary size is critical)

**Also excellent SQLite support**, choose if:
- Binary size is a critical constraint for Raspberry Pi (~1.5MB smaller)
- Combined up/down in single file is acceptable
- Flexible file naming (keeps existing YYYYMMDD pattern)
- Team is comfortable with marker syntax (`-- +goose Up/Down`)

**Note**: goose files with markers can still be run manually, but requires editing out markers or running entire file (both up and down sections execute).

### Implementation Roadmap (golang-migrate)

#### Phase 1: Foundation (Week 1)

**Tasks:**
1. Add `github.com/golang-migrate/migrate/v4` to `go.mod`
2. Create `internal/db/migrate.go` wrapper:
   ```go
   import "github.com/golang-migrate/migrate/v4"
   import _ "github.com/golang-migrate/migrate/v4/database/sqlite3"
   import _ "github.com/golang-migrate/migrate/v4/source/file"
   
   func (db *DB) RunMigrations(migrationsDir string) error {
       m, err := migrate.New(
           "file://"+migrationsDir,
           "sqlite3://"+db.path)
       if err != nil {
           return err
       }
       return m.Up()
   }
   ```
3. Create `schema_migrations` baseline for existing databases
4. Add integration test (`internal/db/migrate_test.go`)

**Deliverables:**
- [ ] Migration runner in `internal/db/migrate.go`
- [ ] Unit tests with coverage >80%
- [ ] Documentation: `docs/database-migrations.md`

#### Phase 2: Migration Files (Week 1-2)

**Tasks:**
1. Split 6 existing migrations into separate `.up.sql` and `.down.sql` files:
   ```
   20250826_rename_tables_column.sql
     → 20250826_rename_tables_column.up.sql
     → 20250826_rename_tables_column.down.sql
   ```
2. Write down migrations for each file:
   - Analyze each migration's changes
   - Create reverse operations (down migrations)
   - Test on copy of production database
   - Verify manual execution: `sqlite3 db.db < migration.up.sql`
3. Document manual fallback procedures

**Deliverables:**
- [ ] 12 migration files (6 up + 6 down)
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
       ./app-radar migrate up
   
   migrate-down:
       ./app-radar migrate down
   
   migrate-status:
       ./app-radar migrate status
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
2. Update `data/migrations/README.md`
3. Add migration examples to documentation site
4. Record demo video (optional)

**Deliverables:**
- [ ] Migration developer guide
- [ ] Updated README files
- [ ] Example migrations
- [ ] Training materials

### Migration File Standards

#### Naming Convention

```
YYYYMMDD_descriptive_name.sql

Examples:
20250826_rename_tables_column.sql
20251110_add_user_preferences_table.sql
20251115_add_index_on_radar_data_speed.sql
```

#### File Format (with goose)

```sql
-- Migration: Brief description of what this migration does
-- Author: Developer Name
-- Date: YYYY-MM-DD
-- Ticket: GH-123 (if applicable)

-- +goose Up
-- Description of forward migration
CREATE TABLE new_table (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE INDEX idx_new_table_name ON new_table(name);

-- +goose Down
-- Description of rollback
DROP INDEX IF EXISTS idx_new_table_name;
DROP TABLE IF EXISTS new_table;
```

#### Best Practices

1. **Atomic changes**: Each migration should do one logical thing
2. **Idempotent SQL**: Use `IF EXISTS` / `IF NOT EXISTS` where possible
3. **Data migrations**: Separate DDL (schema) from DML (data) migrations
4. **Testing**: Always test down migration after up migration
5. **Backward compatibility**: Consider running code during migration
6. **Comments**: Explain non-obvious changes
7. **Transactions**: SQLite supports transactional DDL (use it!)

### Metadata Table Schema

```sql
CREATE TABLE schema_migrations (
    version_id INTEGER PRIMARY KEY AUTOINCREMENT,
    version TEXT UNIQUE NOT NULL,      -- e.g., "20250826"
    applied_at TIMESTAMP NOT NULL,     -- Unix timestamp
    description TEXT,                   -- Human-readable description
    checksum TEXT,                      -- SHA256 of migration SQL
    execution_time_ms INTEGER,         -- Performance tracking
    success BOOLEAN NOT NULL DEFAULT 1  -- 1 = success, 0 = failed/dirty
);

CREATE INDEX idx_schema_migrations_version ON schema_migrations(version);
CREATE INDEX idx_schema_migrations_applied_at ON schema_migrations(applied_at DESC);
```

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
   velocity-report migrate down 20251110
   ```
   - Runs down script
   - Removes entry from `schema_migrations`
   - Validates database state

3. **Emergency rollback** (dirty state):
   ```bash
   velocity-report migrate force 20251109
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
    
    // Apply all migrations
    if err := goose.Up(db.DB, "../../data/migrations"); err != nil {
        t.Fatal(err)
    }
    
    // Check version
    version, _ := goose.GetDBVersion(db.DB)
    if version != 20251022 {
        t.Errorf("expected version 20251022, got %d", version)
    }
    
    // Test rollback
    if err := goose.Down(db.DB, "../../data/migrations"); err != nil {
        t.Fatal(err)
    }
    
    // Verify tables removed
    // ... table existence checks
}

func TestMigrations_Idempotency(t *testing.T) {
    db := setupTestDB(t)
    
    // Apply migrations twice
    goose.Up(db.DB, "../../data/migrations")
    err := goose.Up(db.DB, "../../data/migrations")
    
    if err != nil {
        t.Fatal("second up should be no-op")
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
          go-version: '1.25'
      
      - name: Run migration tests
        run: |
          make build-radar-local
          make test-migrations
      
      - name: Test migration idempotency
        run: |
          ./app-radar migrate up --db test.db
          ./app-radar migrate up --db test.db
          [ $? -eq 0 ] || exit 1
```

## Migration Checklist for Developers

### Creating a New Migration

- [ ] **Plan changes**: Document what tables/columns will change
- [ ] **Create migration file**: `YYYYMMDD_descriptive_name.sql`
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
- ✅ Date-based naming (YYYYMMDD) naturally orders
- ✅ PR reviews catch conflicts
- ✅ CI/CD tests detect migration issues
- ✅ Clear merge conflict resolution process

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

## Appendix A: Comparison Matrix

| Feature | golang-migrate | goose | Custom Go | Shell Script | Python |
|---------|---------------|-------|-----------|--------------|--------|
| **SQLite Support** | ✅ First-class | ✅ First-class | ✅ Custom | ✅ Native | ✅ Native |
| **Separate Up/Down Files** | ✅ Yes | ❌ Single file | ✅ Configurable | ✅ Yes | ✅ Yes |
| **Manual Execution** | ✅ Pure SQL | ⚠️ Needs markers | ✅ Pure SQL | ✅ Pure SQL | ✅ Pure SQL |
| Binary Size Impact | +2MB | +500KB | +0KB | +0KB | N/A |
| Setup Complexity | Medium | Low | High | Very Low | Low |
| Maintenance Burden | Low | Low | High | Medium | Medium |
| Community Support | High | Medium | None | Low | Low |
| Up/Down Migrations | ✅ | ✅ | ✅ | ✅ | ✅ |
| Dirty State Detection | ✅ | ✅ | ❌ | ❌ | ❌ |
| Embedded Migrations | ✅ | ✅ | ✅ | ❌ | ❌ |
| CLI Tool | ✅ | ✅ | Custom | ✅ | ✅ |
| Go Library | ✅ | ✅ | ✅ | ❌ | ❌ |
| Checksum Validation | ✅ | ✅ | Custom | ❌ | ✅ |
| Cross-Platform | ✅ | ✅ | ✅ | ⚠️ | ✅ |
| Learning Curve | Low | Low | High | Very Low | Low |
| **Recommendation** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |

## Appendix B: Example Migration Workflows

### Workflow 1: Add New Table

```sql
-- 20251110_add_user_preferences.sql
-- +goose Up
CREATE TABLE IF NOT EXISTS user_preferences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL UNIQUE,
    theme TEXT DEFAULT 'light',
    timezone TEXT DEFAULT 'UTC',
    created_at INTEGER NOT NULL DEFAULT (UNIXEPOCH('subsec')),
    updated_at INTEGER NOT NULL DEFAULT (UNIXEPOCH('subsec'))
);

CREATE INDEX IF NOT EXISTS idx_user_prefs_user_id ON user_preferences(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_user_prefs_user_id;
DROP TABLE IF EXISTS user_preferences;
```

### Workflow 2: Add Column (Backward Compatible)

```sql
-- 20251111_add_radar_data_confidence.sql
-- +goose Up
ALTER TABLE radar_data ADD COLUMN confidence REAL DEFAULT 1.0;

-- Update existing rows (optional)
UPDATE radar_data SET confidence = 0.8 WHERE magnitude < 100;

-- +goose Down
-- SQLite doesn't support DROP COLUMN in older versions
-- So we create a new table without the column and copy data

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

```sql
-- 20251112_normalize_speed_units.sql
-- Migration: Convert all speeds from mph to m/s
-- NOTE: This is a data migration - down migration will lose precision!

-- +goose Up
UPDATE radar_data 
SET raw_event = json_set(
    raw_event, 
    '$.speed', 
    CAST(json_extract(raw_event, '$.speed') AS REAL) * 0.44704
)
WHERE json_extract(raw_event, '$.speed_unit') = 'mph';

UPDATE radar_data
SET raw_event = json_set(raw_event, '$.speed_unit', 'mps');

-- +goose Down
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
⏭ Skipping 20251110 (already applied)
```

**Solution**: This is normal. The migration was already applied. Check status:
```bash
velocity-report migrate status
```

### Problem: "Dirty migration" Error

```bash
$ velocity-report migrate up
Error: migration 20251110 failed: dirty state detected
```

**Cause**: Previous migration failed mid-execution.

**Solution**:
1. Check database state: `sqlite3 sensor_data.db ".tables"`
2. Manual inspection: Determine if migration partially applied
3. Fix manually or force version:
   ```bash
   velocity-report migrate force 20251109  # Go back to last known good
   velocity-report migrate up              # Try again
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

**Solution**: Document in migration file:
```sql
-- +goose Down
-- WARNING: This migration cannot be safely rolled back
-- Data has been transformed and cannot be restored to original state
-- Manual recovery required if rollback needed
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

**Document Version**: 1.0  
**Last Updated**: November 10, 2025  
**Review Date**: December 10, 2025  
**Approvers**: [TBD]
