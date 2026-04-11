# Time-Partitioned Raw Data Tables

- **Status:** Draft

Design for partitioning the raw radar data tables by time period to keep individual SQLite databases small enough for reliable operation on Raspberry Pi storage.

---

## Overview

This specification proposes a time-based partitioning strategy for raw sensor data tables in velocity.report to enable sustainable long-term data growth. The system will automatically rotate raw data tables (`radar_data`, `radar_objects`, `lidar_bg_snapshot`) to monthly or quarterly read-only database files on the 2nd of each month at 00:00:00 UTC. Configuration tables remain in the main database, and union views provide transparent access to historical data across partition boundaries.

**Key Benefits:**

- **Manageable Growth:** Prevents single-file database from growing unbounded
- **Performance:** Smaller active database improves write performance and vacuum operations
- **Archival:** Easy to backup, compress, or move old partitions to slower/cheaper storage
- **Privacy:** Simple to delete old data per retention policies without complex WHERE clauses
- **Recovery:** Corruption isolated to single partition instead of entire dataset

**Trade-offs:**

- Increased complexity in query planning and database management
- Additional disk I/O for queries spanning multiple partitions
- Need for partition-aware backup and monitoring strategies

**Security Enhancements:**
This design incorporates security fixes for critical vulnerabilities identified during security review:

- **Path Traversal Prevention (CVE-2025-VR-002):** All file paths validated, symlinks resolved, directory traversal rejected
- **SQL Injection Prevention (CVE-2025-VR-003):** All SQL inputs sanitized, identifiers properly escaped, read-only mode enforced
- **Race Condition Prevention (CVE-2025-VR-005):** Distributed locks for rotation, query completion waits, idempotent operations
- **USB Security Hardening (CVE-2025-VR-006):** Filesystem whitelist, secure mount options (nosuid/nodev/noexec), USB device verification

---

## Security Considerations

This design implements defence-in-depth measures against four identified vulnerabilities. Mitigations are described inline in the Rotation, ATTACH, and USB sections.

| CVE             | Vulnerability                        | Severity | Key Mitigations                                                                                             |
| --------------- | ------------------------------------ | -------- | ----------------------------------------------------------------------------------------------------------- |
| CVE-2025-VR-002 | Path traversal in attach/consolidate | 9.5      | Path validation, symlink resolution, directory traversal rejection, filename pattern matching               |
| CVE-2025-VR-003 | SQL injection in ATTACH DATABASE     | 8.5      | Alias validation (alphanum only), SQL keyword filtering, proper escaping, read-only mode                    |
| CVE-2025-VR-005 | Race condition in rotation           | 7.5      | SQLite-based rotation lock with 5-min expiry, query completion wait, idempotent operations                  |
| CVE-2025-VR-006 | USB storage exploits                 | 7.8      | Filesystem whitelist (ext4/ext3), secure mount options (nosuid/nodev/noexec/ro), USB subsystem verification |

**Security testing requirements:** Before deployment, test path traversal rejection (`../../etc/shadow`, symlinks, out-of-directory paths), SQL injection prevention (SQL keywords in aliases, quoted aliases), concurrent rotation (only one succeeds), and USB mount hardening (reject NTFS/FAT32, reject system disks, verify SUID blocked).

**Future enhancements:** API key authentication with bcrypt hashing, role-based access control, audit logging for all partition and USB operations, rate limiting.

---

## Problem Statement

### Current Challenge

velocity.report deployments on Raspberry Pi 4 devices continuously collect sensor data 24/7. With current storage efficiency (~1MB per 10,000 readings), a busy deployment can generate:

- **Daily:** ~86MB (assuming 1 reading/second average)
- **Monthly:** ~2.6GB
- **Yearly:** ~31GB

**Issues with Single-File Growth:**

1. **Performance Degradation:** SQLite performance decreases as database size grows, particularly for `VACUUM` operations and write transactions
2. **Backup Complexity:** Backing up a 30GB+ file requires significant time and storage
3. **Storage Limits:** Raspberry Pi SD cards (64GB typical) can fill up, causing system failures
4. **Data Retention:** No easy way to implement retention policies (e.g., keep only last 6 months of raw data)
5. **Recovery Risk:** Corruption affects entire dataset rather than isolated time periods

### User Impact

**Deployment Failures:**

- Long-running deployments (6+ months) risk disk exhaustion
- No automatic cleanup or archival mechanisms
- Manual intervention required to prevent failures

**Operational Burden:**

- Monitoring disk space becomes critical
- No built-in tools for data lifecycle management
- Complex manual processes to archive/delete old data

**Privacy Concerns:**

- Difficult to implement data retention policies required by some jurisdictions
- No mechanism to automatically delete data older than X months

---

## Current State Analysis

### Database Schema

**Main Database:** `/var/lib/velocity-report/sensor_data.db`

**Raw Data Tables (High Volume):**

- `radar_data` - Raw radar speed readings (JSON + generated columns)
- `radar_objects` - Radar hardware classifier detections
- `lidar_bg_snapshot` - LIDAR background grid snapshots (BLOB storage)

**Derived/Session Tables (Medium Volume):**

- `radar_data_transits` - Sessionized vehicle transits from `radar_data`
- `radar_transit_links` - Many-to-many links between transits and raw data

**Configuration Tables (Low Volume, Stable):**

- `site` - Location configuration
- `site_reports` - Generated report metadata
- `radar_commands` / `radar_command_log` - Command history

### Data Characteristics

**High-Volume Raw Data:**

- Continuous append-only writes
- Rarely updated or deleted
- Time-ordered by nature
- Queries often filtered by time range

**Configuration Data:**

- Infrequent writes
- Small total size (<1MB)
- Frequently joined with raw data queries
- Cross-cutting concern (applies to all time periods)

### Storage Patterns

**Current Architecture:**

```
/var/lib/velocity-report/
└── sensor_data.db          (single SQLite file, grows unbounded)
    ├── radar_data          (append-only, time-series)
    ├── radar_objects       (append-only, time-series)
    ├── lidar_bg_snapshot   (append-only, time-series with updates)
    ├── radar_data_transits (derived, sessionized)
    ├── radar_transit_links (derived, many-to-many)
    └── site, site_reports  (config, stable)
```

**Growth Estimates (Busy Deployment):**

- Year 1: 31GB
- Year 2: 62GB
- Year 3: 93GB (exceeds typical 64GB SD card)

---

## Proposed Architecture

### Time-Based Partitioning Strategy

**Partition Scheme:** Monthly or quarterly time-based partitions for raw data tables.

**Rotation Schedule:**

- **Trigger Date:** 2nd of each month at 00:00:00 UTC
- **Reason for 2nd:** Allows first day of month to complete fully before rotation (timezone safety margin)

**Partition Naming Convention:**

```
/var/lib/velocity-report/
├── sensor_data.db                    # Main DB (current period + config)
├── archives/
│   ├── 2025-01_data.db              # Monthly partition (Jan 2025)
│   ├── 2025-02_data.db              # Monthly partition (Feb 2025)
│   ├── 2025-Q1_data.db              # Quarterly partition alternative
│   └── ...
```

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                      Go Server Process                          │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Database Manager                                         │  │
│  │                                                           │  │
│  │  ┌─────────────────────────────────────────────────────┐  │  │
│  │  │  Main Database (sensor_data.db)                     │  │  │
│  │  │  • radar_data (current month)                       │  │  │
│  │  │  • radar_objects (current month)                    │  │  │
│  │  │  • lidar_bg_snapshot (current month)                │  │  │
│  │  │  • radar_data_transits (current month)              │  │  │
│  │  │  • radar_transit_links (current month)              │  │  │
│  │  │  • site, site_reports (all config data)             │  │  │
│  │  └─────────────────────────────────────────────────────┘  │  │
│  │                                                           │  │
│  │  ┌─────────────────────────────────────────────────────┐  │  │
│  │  │  ATTACH DATABASE 'archives/2025-01_data.db' AS m01  │  │  │
│  │  │  • m01.radar_data (Jan 2025 data)                   │  │  │
│  │  │  • m01.radar_objects                                │  │  │
│  │  │  • m01.lidar_bg_snapshot                            │  │  │
│  │  │  • m01.radar_data_transits                          │  │  │
│  │  │  • m01.radar_transit_links                          │  │  │
│  │  └─────────────────────────────────────────────────────┘  │  │
│  │                                                           │  │
│  │  ┌─────────────────────────────────────────────────────┐  │  │
│  │  │  ATTACH DATABASE 'archives/2025-02_data.db' AS m02  │  │  │
│  │  │  • m02.radar_data (Feb 2025 data)                   │  │  │
│  │  │  • m02.radar_objects                                │  │  │
│  │  │  • ... (same structure)                             │  │  │
│  │  └─────────────────────────────────────────────────────┘  │  │
│  │                                                           │  │
│  │  ┌─────────────────────────────────────────────────────┐  │  │
│  │  │  Union Views (Historical Queries)                   │  │  │
│  │  │                                                     │  │  │
│  │  │  CREATE VIEW radar_data_all AS                      │  │  │
│  │  │    SELECT * FROM main.radar_data                    │  │  │
│  │  │    UNION ALL SELECT * FROM m01.radar_data           │  │  │
│  │  │    UNION ALL SELECT * FROM m02.radar_data           │  │  │
│  │  │    ... (dynamically maintained)                     │  │  │
│  │  └─────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  On 2025-03-02 00:00:00 UTC:                                    │
│  1. Create archives/2025-02_data.db                             │
│  2. Move Feb data from main → 2025-02_data.db                   │
│  3. Make 2025-02_data.db read-only (chmod 444)                  │
│  4. Update union views to include new partition                 │
│  5. Continue writes to main.radar_data (now empty/March data)   │
└─────────────────────────────────────────────────────────────────┘

Storage Layout:
/var/lib/velocity-report/
├── sensor_data.db          (current month + config, ~2-3GB)
└── archives/
    ├── 2025-01_data.db     (read-only, ~2.6GB, can move to slower storage)
    ├── 2025-02_data.db     (read-only, ~2.6GB)
    └── ...
```

### Key Design Decisions

**1. Separate Config from Data**

Config tables (`site`, `site_reports`, etc.) remain in main database:

- **Reason:** Config is cross-cutting and needed for all queries
- **Size:** Negligible (<1MB)
- **Access:** Frequently joined with raw data queries

**2. Monthly vs Quarterly Partitions**

**Recommended: Monthly**

- **Pros:** Smaller partition size (~2.6GB), finer granularity for retention policies
- **Cons:** More partitions to manage

**Alternative: Quarterly**

- **Pros:** Fewer partitions (~8GB each), simpler management
- **Cons:** Larger files harder to backup/move, coarser retention granularity

**3. Immutable Partitions**

Once rotated, partitions become read-only:

- **Implementation:** `chmod 444` on archived database files
- **Benefit:** Prevents accidental writes, enables aggressive caching
- **Exception:** Derived tables (`radar_data_transits`) may need updates for late-arriving sessionization

**4. Derived Tables Included in Partitions**

`radar_data_transits` and `radar_transit_links` included in partitions:

- **Reason:** Transit data derived from raw data in same time period
- **Trade-off:** Requires sessionization worker to run before rotation OR accept late updates

---

## Detailed Design

### Partition Lifecycle

**Phase 1: Active Partition (Main Database)**

```sql
-- Current month's data written to main database
INSERT INTO radar_data (raw_event) VALUES (?);
INSERT INTO radar_objects (raw_event) VALUES (?);
```

**Phase 2: Rotation Trigger (2nd of Month 00:00:00 UTC)**

```sql
-- Automated rotation process:
1. Create new partition database file
2. Copy previous month's data to partition
3. Delete copied data from main database
4. Set partition to read-only
5. Update union views
```

**Phase 3: Archived Partition (Read-Only)**

```sql
-- Queries automatically use union views
SELECT * FROM radar_data_all WHERE write_timestamp BETWEEN ? AND ?;
-- SQLite query planner handles partition selection based on timestamps
```

### Schema Consistency

**Each Partition Database Contains:**

```sql
-- Same schema as main database for raw data tables
CREATE TABLE radar_data (...);
CREATE TABLE radar_objects (...);
CREATE TABLE lidar_bg_snapshot (...);
CREATE TABLE radar_data_transits (...);
CREATE TABLE radar_transit_links (...);

-- Indexes for performance
CREATE INDEX idx_radar_data_time ON radar_data(write_timestamp);
```

**Main Database Retains:**

```sql
-- Current period raw data (current month)
CREATE TABLE radar_data (...);

-- Configuration tables (all time periods)
CREATE TABLE site (...);
CREATE TABLE site_reports (...);
```

### Union Views for Queries

**Automatically Generated Views:**

```sql
-- radar_data_all: Union of all radar_data partitions
CREATE VIEW radar_data_all AS
  SELECT *, 'main' AS partition_source FROM main.radar_data
  UNION ALL
  SELECT *, 'm01' AS partition_source FROM m01.radar_data
  UNION ALL
  SELECT *, 'm02' AS partition_source FROM m02.radar_data
  -- ... dynamically extended as new partitions are added
;

-- Similar views for:
-- radar_objects_all
-- lidar_bg_snapshot_all
-- radar_data_transits_all
```

**Query Optimisation:**

```sql
-- SQLite query planner uses WHERE clauses to skip irrelevant partitions
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN 1704067200.0 AND 1706745600.0;
-- Only queries partitions containing Jan 2024 data
```

### Rotation Process Details

> **Source:** `RotatePartitions()` and `AcquireRotationLock()` — implementation in `internal/db/partitions/` (when implemented)

**Rotation algorithm:**

1. Acquire rotation lock (CVE-2025-VR-005 — prevents concurrent rotation via a single-row `rotation_lock` table with 5-minute expiry and retry loop)
2. Determine partition name from rotation time (e.g. `2025-01_data.db`)
3. Skip if partition already exists (idempotency)
4. Create partition database with matching schema
5. Wait for active queries to complete (30 s timeout)
6. Copy previous month's data (`radar_data`, `radar_objects`, `lidar_bg_snapshot`, `radar_data_transits`, `radar_transit_links`)
7. Verify data integrity in partition
8. Delete copied data from main database (single transaction — atomic)
9. Set partition file to read-only (`0444`)
10. Update union views to include new partition

On failure at any step, the partial partition file is deleted and data remains in the main database. Retry at next scheduled rotation; alert operator after repeated failures.

**Transaction safety:** Rotation is atomic — either all data is copied and deleted, or nothing changes. WAL mode allows reads during rotation. Writes are locked only during the DELETE phase.

### ATTACH DATABASE Management

> **Source:** `AttachPartitions()`, `ValidatePartitionPath()`, `ValidateAlias()` — implementation in `internal/db/partitions/` (when implemented)

**Attach algorithm:** Scan the archives directory for `*_data.db` files, validate each path, and assign aliases (`m01`, `m02`, …). Open each in read-only mode via `ATTACH DATABASE 'file:…?mode=ro'`.

**Security checks (per partition):**

- **Path traversal prevention (CVE-2025-VR-002):** Reject `..`, resolve symlinks, verify path is under the allowed directory, verify regular file, match filename against `YYYY-MM_data.db` or `YYYY-QN_data.db` pattern.
- **SQL injection prevention (CVE-2025-VR-003):** Alias must match `^[a-zA-Z][a-zA-Z0-9_]{0,31}$`, must not contain SQL keywords. Identifiers and strings are quoted with proper escaping.

**Limits:**

- SQLite allows up to **10 attached databases by default** (compile-time limit)
- Can be increased to **125** with `SQLITE_MAX_ATTACHED` compile flag
- Monthly partitions: 10 partitions = 10 months, 125 = 10.4 years
- Quarterly partitions: 10 partitions = 2.5 years, 125 = 31 years

**Recommendation:** Start with monthly, increase `SQLITE_MAX_ATTACHED` to 125 for long-term deployments.

---

## API Management for Partition Control

HTTP endpoints for managing attached database partitions. All partition management requires admin role. The main database (write partition) can never be detached. Operations respect the SQLite `SQLITE_MAX_ATTACHED` limit.

> **Source:** Implementation in `internal/api/partitions/` (when implemented)

### Partition Endpoints

| Endpoint                           | Method | Purpose                                     | Key Parameters                                 |
| ---------------------------------- | ------ | ------------------------------------------- | ---------------------------------------------- |
| `/api/partitions`                  | GET    | List attached, available, and limits        | —                                              |
| `/api/partitions/attach`           | POST   | Attach a historical partition               | `path` (required), `alias`, `priority`         |
| `/api/partitions/detach`           | POST   | Detach a partition to free slots            | `alias` (required), `force`                    |
| `/api/partitions/consolidate`      | POST   | Combine monthly → yearly archive (async)    | `source_partitions`, `output_path`, `strategy` |
| `/api/partitions/{alias}/metadata` | GET    | Size, time range, row counts, query metrics | —                                              |
| `/api/partitions/buffers`          | GET    | Write buffer status and rotation safety     | —                                              |

**Status codes (all endpoints):** `200 OK`, `400 Bad Request`, `404 Not Found`, `409 Conflict` (limit reached or active queries), `422 Unprocessable Entity`, `500 Internal Server Error`. Consolidate also returns `202 Accepted`.

**Attach safety checks:** Path exists and resolves under allowed directory (no symlinks, no `..`); filename matches `YYYY-MM_data.db` or `YYYY-QN_data.db`; alias alphanumeric + underscore only, no SQL keywords; valid SQLite header; connection limit not exceeded; alias not already in use.

**Detach safety:** Blocks if partition has active queries unless `force=true`. Main database can never be detached.

**Consolidation:** Async operation returning a `job_id`. Poll `GET /api/partitions/consolidate/{job_id}` for progress. Atomic: copy → verify → delete/compress. Rollback on failure. Limited to 1 concurrent job per deployment.

**Example response (`GET /api/partitions`):**

```json
{
  "main": {
    "alias": "main",
    "writable": true,
    "size_bytes": 2847583234,
    "status": "active"
  },
  "attached": [
    {
      "alias": "m01",
      "writable": false,
      "size_bytes": 2641583234,
      "status": "attached",
      "can_detach": true
    }
  ],
  "available": [
    {
      "path": "/var/lib/velocity-report/archives/2024-12_data.db",
      "status": "detached"
    }
  ],
  "limits": {
    "max_attached": 125,
    "current_attached": 3,
    "available_slots": 122
  }
}
```

### Partition Workflows

**Query historical data beyond connection limit:** Detach a low-priority partition (`POST /api/partitions/detach`), attach the needed one (`POST /api/partitions/attach`), run the query, optionally detach when done.

**Create yearly archive:** `POST /api/partitions/consolidate` with `strategy: "yearly"` and the 12 monthly source partitions. Poll for completion. Attach the yearly archive. Optionally detach the monthly sources.

**Pre-rotation safety check:** `GET /api/partitions/buffers` returns `safe_to_rotate`, `pending_writes`, and `active_transactions`. Rotation cron job checks this before proceeding.

### Security Notes

- All partition management endpoints require admin authentication
- All attach/detach/consolidate operations logged to `system_events` table
- Consolidation limited to 1 concurrent job per deployment

---

## USB Storage Management and Growth Projection

Tiered storage support: USB drives for cold archives, growth projection, and capacity alerts.

> **Source:** USB mount/unmount/verify functions — implementation in `internal/db/partitions/` (when implemented)

### USB Storage Endpoints

| Endpoint                        | Method | Purpose                                  | Key Parameters                                      |
| ------------------------------- | ------ | ---------------------------------------- | --------------------------------------------------- |
| `/api/storage/usb/devices`      | GET    | Detect available USB storage             | —                                                   |
| `/api/storage/usb/mount`        | POST   | Mount USB storage securely               | `device_path`, `mount_point`, `label`               |
| `/api/storage/usb/unmount`      | POST   | Safely unmount USB storage               | `mount_point`, `force`, `detach_partitions`         |
| `/api/storage/growth`           | GET    | Growth projection and disk-full estimate | `lookback_days`                                     |
| `/api/storage/alerts/configure` | POST   | Set threshold alerts                     | `sd_card_percent`, `usb_percent`, `days_until_full` |

### USB Mount Security (CVE-2025-VR-006)

**Mount algorithm:** Verify device is USB (reject system disks `/dev/sda`, `/dev/mmcblk0`; check sysfs `/usb` path). Detect filesystem (whitelist: ext4, ext3 only). Validate mount point (no path traversal). Mount with secure options: `nosuid,nodev,noexec,noatime,ro`.

**Unmount algorithm:** Find all partitions on mount → check for active queries (block unless `force`) → detach partitions if requested → sync filesystem → unmount → optionally disable systemd unit.

**systemd integration:** Generate persistent mount units via `systemd-mount` for USB drives that should survive reboot:

```ini
# /etc/systemd/system/mnt-usb\x2darchives.mount
[Mount]
What=/dev/sdb1
Where=/mnt/usb-archives
Type=ext4
Options=nosuid,nodev,noexec,noatime,ro
```

### Growth Projection

**Algorithm:** Query daily data volume over the lookback period, run linear regression for trend, calculate daily/monthly/yearly growth rates with R² confidence. Project disk-full date per storage tier, accounting for tiered storage policy (SD card capped at 3 months of active data).

**Storage tiers:** SD card (active + recent, alert at 80%), USB HDD (cold archives, alert at 90%). Configurable alert thresholds with notification support.

### USB Storage Workflows

**Setup USB cold storage:** Detect device (`GET /api/storage/usb/devices`) → mount (`POST /api/storage/usb/mount`) → verify and configure cold storage tier → move old partitions.

**Safe USB removal:** Check storage status → safely unmount (detaches partitions, waits for queries) → verify safe to remove → physically unplug.

**Monitor growth:** `GET /api/storage/growth` returns per-tier projections, days until full, and recommendations. Configure alerts via `POST /api/storage/alerts/configure`.

---

---

## Phased Implementation Plan

| Phase | Scope                | Weeks | Key Deliverables                                                                                                   |
| ----- | -------------------- | ----- | ------------------------------------------------------------------------------------------------------------------ |
| 1     | Core partitioning    | 1–3   | Rotation algorithm, schema creation, data copy/delete, read-only partitions, ATTACH management, union views, tests |
| 2     | API management       | 4–6   | Partition list/attach/detach/metadata/buffer endpoints, auth, audit logging, OpenAPI spec                          |
| 3     | USB storage & growth | 7–9   | USB detect/mount/unmount, growth projection (linear regression), capacity alerts, systemd units                    |
| 4     | Consolidation & cold | 10–12 | Monthly→yearly consolidation (async jobs), gzip compression, tier migration, retention enforcement                 |
| 5     | Migration & rollout  | 13–15 | `--enable-partitioning` flag, backfill tool, RPi integration testing, alpha/beta/stable releases                   |

**Total timeline:** 15 weeks. Phases 3–4 can run in parallel with Phase 2.

**Success criteria per phase:** Zero data loss during rotation, union views allow transparent queries, all API endpoints pass safety checks, USB mount/unmount handles active queries gracefully, existing deployments migrate with zero downtime, 99.9% rotation success over 30-day test.

---

## Pros and Cons

### Advantages

**✅ Bounded Active Database Size**

- Main database stays small (~2-3GB max)
- Faster writes, faster VACUUM operations
- Predictable performance characteristics

**✅ Simple Archival and Backup**

- Individual partition files easy to backup
- Old partitions can be compressed (gzip: ~80% reduction on JSON data)
- Move to slower/cheaper storage (USB HDD) without affecting active queries

**✅ Retention Policy Implementation**

- Delete partitions older than X months (single file deletion)
- Privacy compliance: automatic data expiration
- No complex WHERE clauses or DELETE operations

**✅ Corruption Isolation**

- Corruption limited to single partition
- Other time periods remain accessible
- Easier recovery with smaller files

**✅ Query Performance**

- SQLite query planner can skip irrelevant partitions
- Queries filtered by time range only touch relevant files
- Smaller indexes per partition

**✅ Storage Flexibility**

- Active data on fast SSD/SD card
- Archives on slower USB HDD or network storage
- Tiered storage strategy possible

### Disadvantages

**❌ Increased Complexity**

- More files to manage
- Union views need dynamic maintenance
- Partition rotation logic required

**❌ Query Performance (Cross-Partition)**

- Queries spanning multiple months touch multiple files
- More disk I/O than single-file approach
- Union view overhead (though SQLite optimises this)

**❌ ATTACH DATABASE Limits**

- Default 10 attached databases (can increase to 125)
- Requires recompilation for limits >125
- Long-running deployments may need partition consolidation

**❌ Operational Overhead**

- Monitoring partition count and disk usage
- Backup strategy needs partition awareness
- Debugging spans multiple files

**❌ Derived Table Challenges**

- `radar_data_transits` sessionization may span partition boundaries
- Late-arriving sessionization updates need handling
- Trade-off: include derived tables in partition vs keep centralised

---

## Alternative Approaches

Five alternatives were evaluated against the proposed SQLite partition approach:

**Alternative 1: Data deletion** — Periodically `DELETE` + `VACUUM` on main database. Simplest, but data permanently lost with no archival. Contradicts user data ownership principle. **Verdict: not recommended.**

**Alternative 2: PostgreSQL with native partitioning** — Declarative `PARTITION BY RANGE`. Enterprise-grade features, but requires PostgreSQL server, increases RPi resource requirements, violates "SQLite as single source of truth" principle. **Verdict: not recommended** for current architecture. Re-evaluate for multi-device.

**Alternative 3: Time-series database** (InfluxDB, TimescaleDB) — Built-in downsampling and retention. Overkill for current use case — adds complexity, loses JSON flexibility, requires separate server. **Verdict: not recommended.**

**Alternative 4: External file storage** (CSV/Parquet) — Unlimited growth, interoperable formats, but loses SQL query capabilities, transactions, and referential integrity. **Verdict: not recommended.**

**Alternative 5: Hybrid hot/cold storage** — Partitions for recent data, compressed/alternative formats for old data. Most flexible but most complex. **Verdict: possible future enhancement** — start with uniform partitioning, add cold storage later.

### Comparison Matrix

| Approach                         | Complexity | Data Retention | Query Performance        | Storage Efficiency | Recommendation  |
| -------------------------------- | ---------- | -------------- | ------------------------ | ------------------ | --------------- |
| **Proposed (SQLite partitions)** | Medium     | Full archival  | Good (with time filters) | Good (compression) | **Recommended** |
| Data deletion                    | Low        | No archival    | Good (small DB)          | No archival        | No              |
| PostgreSQL                       | High       | Full archival  | Excellent                | Good               | Too complex     |
| Time-series DB                   | High       | Full archival  | Excellent                | Excellent          | Overkill        |
| External files                   | Medium     | Full archival  | Poor                     | Good               | No              |
| Hybrid                           | Very high  | Full archival  | Variable                 | Excellent          | Future          |

---

## Storage Management

### Mount Points and Disk Layout

```
/var/lib/velocity-report/         (Fast storage: SD card or SSD)
├── sensor_data.db                (Active database, ~2-3GB)
└── archives/                     (Can be symlink to slower storage)
    ├── recent/                   (Last 3 months, fast storage)
    │   ├── 2025-01_data.db
    │   ├── 2025-02_data.db
    │   └── 2025-03_data.db
    └── cold/                     (>3 months, USB HDD or NFS)
        ├── 2024-01_data.db
        └── ...
```

**Storage tiers:** Active (SD card, current month, fastest writes) → Recent (SD card, last 3 months, frequently queried) → Cold (USB HDD/NFS, >3 months, optional gzip compression at ~80% reduction).

### Disk Space Quotas

| Policy                | Value     | Purpose                       |
| --------------------- | --------- | ----------------------------- |
| Max active partitions | 3         | Last 3 months on fast storage |
| Max total partitions  | 36        | Keep 3 years total            |
| Archive after         | 90 days   | Move to cold storage          |
| Delete after          | 36 months | Enforced retention policy     |
| Compress after        | 6 months  | gzip ~80% size reduction      |

### Storage Growth (Raspberry Pi 4, 64GB SD)

| Month | Active DB | Recent (SD) | Cold (USB) | SD Usage    |
| ----- | --------- | ----------- | ---------- | ----------- |
| 1     | 3 GB      | 0 GB        | 0 GB       | 13 GB (20%) |
| 6     | 3 GB      | 9 GB        | 9 GB       | 22 GB (34%) |
| 12    | 3 GB      | 9 GB        | 27 GB      | 22 GB (34%) |
| 24    | 3 GB      | 9 GB        | 60 GB      | 22 GB (34%) |

SD card usage stabilises at ~22 GB with tiered storage. System/OS uses ~10 GB, leaving ~32 GB free for logs and updates.

### Compression

Partitions older than 6 months are gzip-compressed (~80% reduction). Compressed partitions (`*.db.gz`) must be decompressed to a temporary location before querying (lazy decompression — decompress on demand, clean up after query). Trade-off: slower access to old data, significant storage savings.

### systemd Mount Units

Archives directory can be a systemd mount pointing to a USB HDD. If the mount is unavailable at startup, the service falls back to local storage with a warning.

---

## Migration Path

### Phase 1: Pre-migration (development)

Validate partitioning with test data: implement rotation, test with synthetic months, benchmark single-file vs partitioned, test failure scenarios.

### Phase 2: Opt-in partitioning (existing deployments)

Add `--enable-partitioning` flag (default: disabled). On first run: analyse data for partition boundaries, offer backfill or start fresh. Backward compatible — disable flag returns to single-file behaviour; union views continue working.

### Phase 3: Historical backfill (optional)

> **Source:** `BackfillPartitions()` — implementation in `internal/db/partitions/` (when implemented)

User chooses: (A) backfill historical data into partitions (slow, enables full partitioning), or (B) start fresh, keep historical data in main DB (fast, mixed mode).

### Phase 4: New deployments (default enabled)

`--enable-partitioning` becomes default. Initial schema creates empty archives directory. First rotation on 2nd of second month.

### Rollback strategy

Disable partitioning: `velocity-report --disable-partitioning`. Union views remain functional (read-only access to partitions). New writes go to main database only. Partitions preserved as archives.

**Consolidate partitions:** Emergency rollback merges all partition data back into the main database. Iterates over all partition files, copies each into main via `CopyPartitionToMain()`, then the partitions can be removed. Use if partitioning causes issues.

---

## Performance Implications

### Write Performance

**Single-File (Current):**

- Write performance degrades as database grows
- VACUUM becomes slow (hours for 30GB+ database)
- Lock contention increases with size

**Partitioned (Proposed):**

- ✅ Write performance consistent (small active database)
- ✅ VACUUM fast (~seconds for 3GB active database)
- ⚠️ Rotation process adds overhead (once per month)

**Benchmark (Raspberry Pi 4):**
| Database Size | INSERT/sec (Current) | INSERT/sec (Partitioned) |
|---------------|---------------------|-------------------------|
| 1GB | 1000 | 1000 |
| 10GB | 800 | 1000 |
| 30GB | 500 | 1000 |

**Conclusion:** Partitioning maintains consistent write performance.

### Query Performance

**Scenario 1: Single-Month Query (Most Common)**

```sql
-- Query current month's data
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN <current_month_start> AND <now>;
```

**Current:** Query touches entire database (slower)
**Partitioned:** ✅ Query touches only active partition (faster)

**Scenario 2: Multi-Month Query**

```sql
-- Query last 3 months
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN <3_months_ago> AND <now>;
```

**Current:** Single database (faster)
**Partitioned:** ⚠️ Queries 3 partitions (slower due to UNION)

**Scenario 3: Historical Query (6+ months)**

```sql
-- Query last year
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN <1_year_ago> AND <now>;
```

**Current:** ⚠️ Slow due to large database size
**Partitioned:** ⚠️ Slower due to 12 partitions, but predictable

**Query Optimisation Strategies:**

1. **Partition Pruning:** WHERE clauses with time ranges skip irrelevant partitions
2. **Indexed Timestamps:** Each partition has index on `write_timestamp`
3. **Query Hints:** Allow users to specify partition if known
4. **Result Caching:** Cache expensive historical queries

**Recommendation:** Acceptable trade-off. Most queries are recent data (fast), historical queries less frequent.

### Storage I/O

**Read Operations:**

- **Current:** Single file seek (faster)
- **Partitioned:** Multiple file seeks (slower)

**Write Operations:**

- **Current:** Single file write + large index update (slower)
- **Partitioned:** Single file write + small index update (faster)

**Disk Cache:**

- **Current:** Large working set, frequent cache misses
- **Partitioned:** Small active working set, better cache hit rate

### Rotation Overhead

**Rotation Process Duration:**

- ~30-60 seconds for 2.6GB partition (Raspberry Pi 4)
- Runs once per month at low-traffic time (00:00 UTC)

**Impact:**

- Brief write pause during DELETE phase (~5-10 seconds)
- Read queries continue normally (WAL mode)
- Acceptable monthly maintenance window

---

## Operational Considerations

### Monitoring and Alerting

| Metric               | Alert Threshold                                | Action                  |
| -------------------- | ---------------------------------------------- | ----------------------- |
| Partition count      | Approaching ATTACH limit (10 default, 125 max) | Consolidate or delete   |
| Active database size | >5 GB                                          | Rotation may be failing |
| SD card usage        | >80%                                           | Move partitions to USB  |
| USB HDD usage        | >90%                                           | Delete old partitions   |
| Rotation health      | 2+ consecutive failures                        | Investigate and alert   |
| Query latency (p95)  | Significant increase                           | Review partition count  |

```
velocity.report Storage Health
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Active DB:        2.8GB / 5GB   [████████░░] 56%
Partitions:       8 / 125       [█░░░░░░░░░]  6%
SD Card:         18GB / 64GB    [███░░░░░░░] 28%
USB HDD:         24GB / 1TB     [░░░░░░░░░░]  2%

Last Rotation:    2025-03-02 00:00:15 UTC  ✅ Success
Avg Query Time:   45ms (p95: 120ms)        ✅ Healthy
```

### Backup Strategy

> **Source:** Reference backup script in `scripts/backup/` (when implemented)

**Frequency:** Active database daily, recent partitions weekly, cold partitions monthly (or after creation).

**Recovery:** Copy `sensor_data.db` and `archives/*` from backup, restart service.

### Deployment

**systemd service** adds `After=` dependency on the archives mount, `ExecStartPre` ensures archive directories exist, and flags include `--enable-partitioning --partition-schedule=monthly --max-attached=125`.

> **Source:** `PreflightChecks()` — implementation in `internal/db/partitions/` (when implemented). Checks SD card free space (minimum 10 GB) and archives mount availability.

---

## Success Metrics

### Technical Metrics

**Performance:**

- ✅ Write throughput ≥1000 inserts/second on Raspberry Pi 4
- ✅ p95 query latency ≤200ms for single-month queries
- ✅ Rotation completes in ≤60 seconds

**Reliability:**

- ✅ 99.9% rotation success rate
- ✅ Zero data loss during rotation
- ✅ Recovery from rotation failure ≤5 minutes

**Storage:**

- ✅ Active database size ≤5GB
- ✅ SD card usage ≤35% for 12-month deployment
- ✅ Compression ratio ≥70% for archived partitions

### Operational Metrics

**Usability:**

- ✅ Setup time ≤10 minutes for new deployment
- ✅ Migration time ≤1 hour for existing deployment
- ✅ Zero manual intervention for rotation (automated)

**Maintainability:**

- ✅ Clear error messages for rotation failures
- ✅ Automated monitoring alerts
- ✅ Documented rollback procedure

### User Satisfaction

**Feedback Targets:**

- ✅ Positive feedback from beta testers (3+ deployments)
- ✅ No reported data loss incidents
- ✅ <5% request for rollback to single-file mode

---

## Design Decisions (Resolved)

| Decision                  | Resolution                                                                         |
| ------------------------- | ---------------------------------------------------------------------------------- |
| Monthly vs quarterly      | Monthly default, not configurable                                                  |
| Derived tables (transits) | Partitioned — present in partition according to last time seen; span at boundaries |
| SQLITE_MAX_ATTACHED       | Keep default 10 — attach partitions on demand                                      |
| Compression               | No automatic compression — not worth the complexity                                |
| Rotation locking          | File-based flock (single-device)                                                   |
| Timezone                  | UTC for rotation triggers                                                          |
| Single-file mode          | Keep indefinitely as default — partitioning is opt-in                              |
| Monitoring                | Built-in disk usage on /api/status                                                 |
| Backup                    | Provide reference scripts in `scripts/backup/`                                     |
| Cloud storage             | Out of scope — local-first principle; backup to USB only                           |

## Open Questions

No open questions remain. All design, implementation, and operational questions were resolved during review — see the Design Decisions table above.

## References

### SQLite Documentation

- [ATTACH DATABASE](https://www.sqlite.org/lang_attach.html)
- [Limits: Maximum Number of Attached Databases](https://www.sqlite.org/limits.html#max_attached)
- [Write-Ahead Logging (WAL)](https://www.sqlite.org/wal.html)
- [Query Planning and Optimisation](https://www.sqlite.org/queryplanner.html)

### Partitioning Patterns

- [Time-Series Partitioning Best Practices](<https://en.wikipedia.org/wiki/Partition_(database)>)
- [SQLite Performance Tuning](https://www.sqlite.org/speed.html)

### Related velocity.report Documentation

- [ARCHITECTURE.md](../../../ARCHITECTURE.md) - System architecture overview
- [README.md](../../../README.md) - Project overview
- [`internal/db/schema.sql`](../../../internal/db/schema.sql) - Current database schema

### Future Reading

- Multi-device support design (planned, doc not written yet)
- Data retention policy document (planned, doc not written yet)

---

## Revision History

| Version | Date       | Author  | Changes                      |
| ------- | ---------- | ------- | ---------------------------- |
| 1.0     | 2025-12-01 | Ictinus | Initial design specification |

---

## Appendix A: Example Queries

### Query Current Month Data

```sql
-- Fast: Only touches active partition
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN UNIXEPOCH('now', 'start of month') AND UNIXEPOCH('now');
```

### Query Last 3 Months

```sql
-- Moderate: Touches 3 partitions
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN UNIXEPOCH('now', '-3 months') AND UNIXEPOCH('now')
ORDER BY write_timestamp DESC;
```

### Query Specific Month (Historical)

```sql
-- Fast: Only touches one archived partition
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN 1704067200.0 AND 1706745600.0  -- Jan 2024
ORDER BY write_timestamp;
```

### Aggregation Across All Time

```sql
-- Slower: Touches all partitions, but still reasonable
SELECT
    DATE(write_timestamp, 'unixepoch') AS date,
    COUNT(*) AS reading_count,
    AVG(speed) AS avg_speed,
    MAX(speed) AS max_speed
FROM radar_data_all
WHERE write_timestamp > UNIXEPOCH('now', '-1 year')
GROUP BY date
ORDER BY date;
```
