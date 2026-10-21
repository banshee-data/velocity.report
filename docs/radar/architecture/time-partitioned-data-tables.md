# Time-Partitioned raw data tables

- **Status:** Proposed (D-15): the owner's storage choices are settled; crash recovery and the other [implementation blockers](#open-implementation-problems) remain open. Was: Draft

Design for partitioning the raw radar data tables by time period to keep individual SQLite databases small enough for reliable operation on Raspberry Pi storage.

---

## Overview

This specification proposes a time-based partitioning strategy for raw sensor data tables in velocity.report to enable sustainable long-term data growth. On the 2nd of each month at 00:00:00 UTC the system checks which earlier months can close. A month stays writable in the main database until every session touching it has closed and its transit processing has finished; only then can its raw data and derived records move to a read-only archive. Configuration tables remain in the main database, and history queries read main once and each required archived month once.

**Key Benefits:**

- **Manageable Growth:** Prevents single-file database from growing unbounded
- **Performance:** Smaller active database improves write performance and vacuum operations
- **Archival:** Easy to back up or move old partitions to slower, cheaper local storage
- **Privacy:** Simple for the operator to delete old months without complex WHERE clauses
- **Recovery:** Corruption isolated to single partition instead of entire dataset

**Trade-offs:**

- Increased complexity in query planning and database management
- Additional disk I/O for queries spanning multiple partitions
- Need for partition-aware backup and monitoring strategies

**Security Enhancements:**
This design carries four security requirements identified during design review. They apply to
code not yet written: none describes a weakness in shipped code. The identifiers are internal,
from the [security checklist's register](../../../.github/knowledge/security-checklist.md#identifier-register);
no CVE has been requested or assigned.

- **Path Traversal Prevention (VR-SEC-002):** All file paths validated, symlinks resolved, directory traversal rejected
- **SQL Injection Prevention (VR-SEC-003):** All SQL inputs sanitized, identifiers properly escaped, read-only mode enforced
- **Race Condition Prevention (VR-SEC-005):** A file lock (flock) plus an in-process mutex for rotation, query completion waits, idempotent operations
- **USB Security Hardening (VR-SEC-006):** Filesystem whitelist, secure mount options (nosuid/nodev/noexec), partition files read-only and attached read-only, USB device verification

---

## Security considerations

The implementing pull request must meet four security requirements. Each is described inline in the Rotation, ATTACH and USB sections.

| Identifier | Requirement                       | Status                  | Gate          | Key Mitigations                                                                                                                                |
| ---------- | --------------------------------- | ----------------------- | ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| VR-SEC-002 | Path traversal in attach and move | design requirement      | CRITICAL      | Path validation, symlink resolution, directory traversal rejection, filename pattern matching                                                  |
| VR-SEC-003 | SQL injection in ATTACH DATABASE  | design requirement      | CRITICAL      | Alias validation (alphanum only), SQL keyword filtering, proper escaping, read-only mode                                                       |
| VR-SEC-005 | Race condition in rotation        | reliability requirement | INFORMATIONAL | flock on a lock file (released by the kernel if the process dies) plus an in-process mutex, query completion wait, idempotent operations       |
| VR-SEC-006 | USB storage exploits              | design requirement      | CRITICAL      | Filesystem whitelist (ext4/ext3), mount options nosuid/nodev/noexec, partition files `0444` and attached `mode=ro`, USB subsystem verification |

Relabelled on October 20, 2026 after a security review: the four were labelled `CVE-2025-VR-002`, `003`,
`005` and `006`, with bare scores and called fixes. VR-SEC-001 and 004 have no counterpart here
and stay reserved. The review's other points were settled with the owner on October 21, 2026,
and the sections below now say one thing each: six contradictions between sections (the rotation
lock, the attach limit, compression, quarterly partitions, NFS and a read-only USB mount that
partitions move onto), API authentication as a precondition for the storage actions, what the
debug surfaces see once partitions exist, and retention. The answers are in
[design decisions](#design-decisions-resolved).

**Security testing requirements:** Before deployment, test path traversal rejection (`../../etc/shadow`, symlinks, out-of-directory paths), SQL injection prevention (SQL keywords in aliases, quoted aliases), concurrent rotation (only one succeeds), a write through an attached partition (refused), and USB mount hardening (reject NTFS/FAT32, reject system disks, verify SUID blocked).

**Precondition:** API authentication lands before any storage action endpoint, as its own backlog row: an admin key, hashed with bcrypt, with every use written to `system_events`. Role-based access control and rate limiting remain future enhancements.

---

## Problem statement

### Current challenge

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

### User impact

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

## Current state analysis

### Database schema

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

### Data characteristics

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

### Storage patterns

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

## Proposed architecture

### Time-Based partitioning strategy

**Partition Scheme:** Monthly time-based partitions for raw data tables; the period is not configurable, and months are never consolidated into larger files.

**Rotation Schedule:**

- **First eligibility check:** 2nd of each month at 00:00:00 UTC
- **Closure lag:** The date alone does not close a month. Open sessions and unfinished transit processing delay closure; the main file can hold several unclosed months.
- **Retry:** Recheck a deferred month after session closure or worker progress, and on restart. Report its age and blocking work; never force-close a session to meet the calendar.

**Partition Naming Convention:**

```
/var/lib/velocity-report/
├── sensor_data.db                    # Main DB (current period + config)
├── archives/
│   ├── 2025-01_data.db              # Monthly partition (Jan 2025)
│   ├── 2025-02_data.db              # Monthly partition (Feb 2025)
│   └── ...
```

### Architecture diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                      Go Server Process                          │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Database Manager                                         │  │
│  │                                                           │  │
│  │  ┌─────────────────────────────────────────────────────┐  │  │
│  │  │  Main Database (sensor_data.db)                     │  │  │
│  │  │  • radar_data (unclosed months)                     │  │  │
│  │  │  • radar_objects (unclosed months)                  │  │  │
│  │  │  • lidar_bg_snapshot (unclosed months)              │  │  │
│  │  │  • radar_data_transits (unclosed months)            │  │  │
│  │  │  • radar_transit_links (unclosed months)            │  │  │
│  │  │  • site, site_reports (all config data)             │  │  │
│  │  └─────────────────────────────────────────────────────┘  │  │
│  │                                                           │  │
│  │  ┌─────────────────────────────────────────────────────┐  │  │
│  │  │  History query (its own connection, per query)      │  │  │
│  │  │                                                     │  │  │
│  │  │  ATTACH 'file:archives/2025-01_data.db?mode=ro'     │  │  │
│  │  │    AS m01   (only the months the range covers,      │  │  │
│  │  │             at most ten per connection)             │  │  │
│  │  │  CREATE TEMP VIEW radar_data_all AS                 │  │  │
│  │  │    SELECT * FROM main.radar_data                    │  │  │
│  │  │    UNION ALL SELECT * FROM m01.radar_data           │  │  │
│  │  │  ... run the query ...                              │  │  │
│  │  │  DROP VIEW; DETACH m01; return the connection       │  │  │
│  │  └─────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  From 2025-03-02T00:00:00Z, once February can close:             │
│  1. Create archives/2025-02_data.db                             │
│  2. Move Feb data from main → 2025-02_data.db                   │
│  3. Make 2025-02_data.db read-only (chmod 444)                  │
│  4. Keep current and other unclosed months writable in main    │
└─────────────────────────────────────────────────────────────────┘

Storage Layout:
/var/lib/velocity-report/
├── sensor_data.db          (unclosed months + config; grows while closure waits)
└── archives/
    ├── 2025-01_data.db     (read-only, ~2.6GB, can move to slower storage)
    ├── 2025-02_data.db     (read-only, ~2.6GB)
    └── ...
```

### Key design decisions

**1. Separate Config from Data**

Config tables (`site`, `site_reports`, etc.) remain in main database:

- **Reason:** Config is cross-cutting and needed for all queries
- **Size:** Negligible (<1MB)
- **Access:** Frequently joined with raw data queries

**2. Monthly Partitions Only**

Monthly, not configurable, and never consolidated (the owner, October 21, 2026):

- **Pros:** Smaller partition size (~2.6GB), finer granularity for deletion, one filename pattern
- **Cons:** More files; a history query longer than ten archived months reads in batches

Quarterly partitions and monthly-to-yearly consolidation were considered and dropped: each added a
second filename pattern and a second rotation or merge path to test, and attaching per query means
the attach limit never forces a merge.

**3. Immutable Partitions**

Once rotated, partitions become read-only:

- **Implementation:** `chmod 444` on archived database files
- **Benefit:** Prevents accidental writes, enables aggressive caching
- **Closure condition:** All sessions touching the month are closed, their derived writes are committed, and no queued ingest or transit work can still change its rows. Until then the month remains in main.
- **After sealing:** Neither ingest nor the transit worker writes to the archive. Reprocessing or correcting a sealed month requires a separate, explicit policy; it is an [open implementation problem](#open-implementation-problems).

**4. Derived Tables Included in Partitions**

`radar_data_transits` and `radar_transit_links` included in partitions:

- **Reason:** Transit data derived from raw data in same time period
- **Trade-off:** Closure waits for the sessionisation worker, including a session that spans midnight or the month boundary. A worker backlog or a session that never closes delays archival and raises a storage-health warning.

---

## Detailed design

### Partition lifecycle

**Phase 1: Unclosed Months (Main Database)**

The current month and any earlier month awaiting session closure or transit processing remain in the main database. Normal ingest and the worker can finish their writes there. The closed-month boundary, not the current date alone, decides whether a write is allowed.

**Phase 2: Closure and Rotation (Eligible from the 2nd of the Next Month)**

The eligibility check requires all sessions touching the month to be closed, the worker to have processed through the month's end and committed the derived records and links, and queued writes affecting it to be drained. If any condition is false, leave the whole month in main and expose the reason. The implementation must make this check and the exclusion of later writes one coordinated operation.

Once eligible, the required rotation stages are:

1. Copy the month into an unpublished staging file and verify it
2. Durably record and publish the archive under the recovery protocol
3. Delete only the verified source rows and record completion
4. Seal the archive and keep later months writable in main

These are requirements, not a completed crash-recovery protocol; see [open implementation problems](#open-implementation-problems).

**Phase 3: Archived Partition (Read-Only)**

A history query runs through one helper that attaches the months its time range covers and defines `radar_data_all` on that connection (see [history queries](#history-queries)). A query such as `SELECT * FROM radar_data_all WHERE write_timestamp BETWEEN ? AND ?` then reads only those months.

### Schema consistency

**Each Partition Database Contains:**

Same schema as main database for raw data tables:

| Table / Index         | Purpose                                |
| --------------------- | -------------------------------------- |
| `radar_data`          | Raw radar speed readings               |
| `radar_objects`       | Radar hardware classifier detections   |
| `lidar_bg_snapshot`   | LiDAR background grid snapshots        |
| `radar_data_transits` | Sessionised vehicle transits           |
| `radar_transit_links` | Many-to-many transit links             |
| `idx_radar_data_time` | Index on `radar_data(write_timestamp)` |

**Main Database Retains:**

The main database retains all unclosed months' raw and derived records and all configuration tables (`site`, `site_reports`). Delayed closure can leave more than one month's records there.

### History queries

**Attached per query, on its own connection.** A history query takes a dedicated connection from
the pool (`sql.Conn`), lists the archives directory, and attaches only the months its time range
covers, each with `file:…?mode=ro`. On that connection it defines `radar_data_all` as a `TEMP`
view: a `UNION ALL` across `main.radar_data` and each attached month's `radar_data`, with a
`partition_source` column naming the origin. `radar_objects_all`, `lidar_bg_snapshot_all` and
`radar_data_transits_all` follow the same pattern. Before the connection returns to the pool the
views are dropped and every month detached, so no pooled connection holds an attached schema and
no global view needs maintaining.

**At most ten archived months per connection.** The driver, `modernc.org/sqlite`, is compiled
with `SQLITE_MAX_ATTACHED=10`. A longer range reads the archives in disjoint batches of at most
ten. Include main in the first batch only, even when it holds several unclosed months or
unpartitioned historical data. Every later batch's view contains only its archived months.
Never repeat main when merging the batches.

**One result over the whole range.** Apply the same time filter to every source. Merge counts
by addition, means from sums and counts, and minima and maxima over every batch. Compute
percentiles over the combined qualifying rows. Merge grouped results by their full group key:
a local-calendar day or hour can cross a UTC partition boundary. Apply global ordering and
pagination after the merge, with a stable tie-breaker. A month still in main must not also be
published as an archive visible to the query; coordinating that transition remains an
[implementation blocker](#open-implementation-problems).

**Acceptance cases:** Compare with a single-file oracle for zero, ten, eleven, and twenty-one
archived months, including main with two unclosed months. Check row identities and counts,
unequal group sizes for means, percentiles, local-day groups across a UTC month boundary, and
ordered pages. Every qualifying row must occur exactly once.

**Query Optimisation:**

The helper attaches only the months the range covers, and within each month the
`write_timestamp` index serves the range.

### Rotation process details

> **Source:** `RotatePartitions()` and `AcquireRotationLock()`; implementation in `internal/db/partitions/` (when implemented)

**Required rotation stages (recovery protocol still open):**

1. Acquire rotation lock (VR-SEC-005: `flock` on `archives/.rotation.lock`, plus an in-process mutex for goroutines; a second rotation fails fast rather than waiting, and the kernel releases the lock if the process dies, so nothing goes stale)
2. Select the oldest eligible unclosed month, whose sessions and transit processing have finished; leave it in main and report the blocker if they have not
3. Consult durable completion state; an existing filename alone is not proof of successful rotation
4. Create an unpublished staging database with matching schema
5. Coordinate query admission and drain existing readers before the publication/source-deletion transition (30 s timeout; a timeout defers rotation)
6. Copy that month's data (`radar_data`, `radar_objects`, `lidar_bg_snapshot`, `radar_data_transits`, `radar_transit_links`)
7. Verify data integrity in partition
8. Durably publish the verified archive and delete only its copied source rows, under the recovery protocol that the implementing design must define
9. Set partition file to read-only (`0444`) and record completion before admitting history readers again
10. Release the lock

Before source deletion, a failed unpublished copy can be removed while the source remains in main. After source deletion has committed, never remove the archive as generic failure cleanup. On restart, recover from the recorded stage rather than skipping an existing filename. The durable stages and recovery actions are still to be designed and tested.

**Transaction safety:** WAL transactions are atomic within each database file, not across the main file and an attached archive. A file lock only excludes another storage operation. Neither establishes crash-safe copy-and-delete or prevents a newly admitted query from seeing both copies. The implementation cannot claim zero-loss, exactly-once rotation until its recovery and reader-admission protocol passes the fault tests below.

### ATTACH DATABASE management

> **Source:** `AttachPartitions()`, `ValidatePartitionPath()`, `ValidateAlias()`; implementation in `internal/db/partitions/` (when implemented)

**Attach algorithm:** For one history query, on its own connection: list the archives directory, choose the `YYYY-MM_data.db` files the time range covers, validate each path, assign aliases (`m01`, `m02`, …) and attach each read-only via `ATTACH DATABASE 'file:…?mode=ro'`. Detach every alias before the connection returns to the pool.

**Security checks (per partition):**

- **Path traversal prevention (VR-SEC-002):** Reject `..`, resolve symlinks, verify path is under the allowed directory, verify regular file, match filename against the `YYYY-MM_data.db` pattern.
- **SQL injection prevention (VR-SEC-003):** Alias must match `^[a-zA-Z][a-zA-Z0-9_]{0,31}$`, must not contain SQL keywords. Identifiers and strings are quoted with proper escaping.

**Limits:** Ten attached databases per connection, the limit `modernc.org/sqlite` is compiled
with. Longer ranges read in batches; see [history queries](#history-queries).

---

## API management for partition control

HTTP endpoints for partition status and the storage actions. Every action endpoint requires API authentication, which lands as its own backlog row before them: an admin key, hashed with bcrypt, with every use written to `system_events`. The read-only endpoints need no key. There are no attach or detach endpoints: history queries attach months themselves, so there is no standing set of attached partitions to manage.

> **Source:** Implementation in `internal/api/partitions/` (when implemented)

### Partition endpoints

| Endpoint                           | Method | Purpose                                     | Key Parameters    |
| ---------------------------------- | ------ | ------------------------------------------- | ----------------- |
| `/api/partitions`                  | GET    | List months with tier, size and time range  | -                 |
| `/api/partitions/{month}/metadata` | GET    | Size, time range, row counts, query metrics | -                 |
| `/api/partitions/buffers`          | GET    | Write buffer status and rotation safety     | -                 |
| `/api/partitions/rotate`           | POST   | Run the rotation now (admin key)            | -                 |
| `/api/partitions/{month}/move`     | POST   | Move a month to another tier (admin key)    | `tier` (required) |
| `/api/partitions/{month}`          | DELETE | Delete a month (admin key, operator only)   | -                 |

**Status codes (all endpoints):** `200 OK`, `400 Bad Request`, `401 Unauthorized` (action without a valid key), `404 Not Found`, `409 Conflict` (rotation lock held or active queries), `422 Unprocessable Entity`, `500 Internal Server Error`.

**Attach safety checks (inside the history helper):** Path exists and resolves under allowed directory (no symlinks, no `..`); filename matches `YYYY-MM_data.db`; alias alphanumeric + underscore only, no SQL keywords; valid SQLite header; at most ten attached on the connection.

**Move and delete safety:** Both take the rotation lock, wait for history queries reading the month (blocking unless `force=true`), and never touch any unclosed month in the main database. A move copies, verifies, then removes the source. No deletion happens unless the operator asks for it.

**Example response (`GET /api/partitions`):**

| Field                    | Type   | Description                                                                                                       |
| ------------------------ | ------ | ----------------------------------------------------------------------------------------------------------------- |
| `main.alias`             | string | `"main"`                                                                                                          |
| `main.writable`          | bool   | `true`                                                                                                            |
| `main.size_bytes`        | int    | Active database size                                                                                              |
| `main.status`            | string | `"active"`                                                                                                        |
| `partitions[]`           | array  | Archived months, each with `month`, `tier`, `path`, `size_bytes`, first and last `write_timestamp` and row counts |
| `max_attached_per_query` | int    | Ten, the driver's compiled limit; longer ranges read in batches                                                   |

### Partition workflows

**Query more than ten months of history:** No operator step: the history helper reads in batches of ten.

**Free space:** `GET /api/storage/growth` shows the projected fill date; the operator moves months to the USB tier (`POST /api/partitions/{month}/move`) or deletes the ones they no longer want (`DELETE /api/partitions/{month}`).

**Pre-rotation safety check:** `GET /api/partitions/buffers` returns `safe_to_rotate`, `pending_writes`, `active_transactions`, and each deferred month's open-session count, worker progress, and closure lag. The scheduler uses it for visibility; the rotation operation rechecks eligibility under its coordination mechanism. A stale preflight response never authorises sealing.

### Security notes

- Every action endpoint (rotate, move, delete, USB mount and unmount, alert configuration) requires the admin key; the key's row lands before this design's endpoints
- Every action logged to the `system_events` table with its outcome
- One rotation, move or delete at a time: all take the rotation lock

---

## USB storage management and growth projection

Tiered storage support: USB drives for cold archives, growth projection, and capacity alerts.

> **Source:** USB mount/unmount/verify functions; implementation in `internal/db/partitions/` (when implemented)

### USB storage endpoints

| Endpoint                        | Method | Purpose                                  | Key Parameters                                      |
| ------------------------------- | ------ | ---------------------------------------- | --------------------------------------------------- |
| `/api/storage/usb/devices`      | GET    | Detect available USB storage             | -                                                   |
| `/api/storage/usb/mount`        | POST   | Mount USB storage securely (admin key)   | `device_path`, `mount_point`, `label`               |
| `/api/storage/usb/unmount`      | POST   | Safely unmount USB storage (admin key)   | `mount_point`, `force`                              |
| `/api/storage/growth`           | GET    | Growth projection and disk-full estimate | `lookback_days`                                     |
| `/api/storage/alerts/configure` | POST   | Set threshold alerts (admin key)         | `sd_card_percent`, `usb_percent`, `days_until_full` |

### USB mount security (VR-SEC-006)

**Mount algorithm:** Verify device is USB (reject system disks `/dev/sda`, `/dev/mmcblk0`; check sysfs `/usb` path). Detect filesystem (whitelist: ext4, ext3 only). Validate mount point (no path traversal). Mount with secure options: `nosuid,nodev,noexec,noatime`. The drive is writable so that a move and an operator's delete can write to it; each partition on it is `0444` and is only ever attached `mode=ro`.

**Unmount algorithm:** Find all partitions on mount → wait for history queries reading them (block unless `force`) → sync filesystem → unmount → optionally disable systemd unit.

**systemd integration:** Generate persistent mount units via `systemd-mount` for USB drives that should survive reboot. The unit file at `/etc/systemd/system/mnt-usb\x2darchives.mount` specifies `What=/dev/sdb1`, `Where=/mnt/usb-archives`, `Type=ext4`, and secure mount options: `nosuid,nodev,noexec,noatime`.

### Growth projection

**Algorithm:** Query daily data volume over the lookback period, run linear regression for trend, calculate daily/monthly/yearly growth rates with R² confidence. Project disk-full date per storage tier, accounting for the target of three recent closed months on SD card and all unclosed months still retained in main.

**Storage tiers:** SD card (active + recent, alert at 80%), USB HDD (cold archives, alert at 90%). Configurable alert thresholds with notification support.

### USB storage workflows

**Setup USB cold storage:** Detect device (`GET /api/storage/usb/devices`) → mount (`POST /api/storage/usb/mount`) → verify and configure cold storage tier → move old partitions.

**Safe USB removal:** Check storage status → safely unmount (waits for history queries reading its months) → verify safe to remove → physically unplug.

**Monitor growth:** `GET /api/storage/growth` returns per-tier projections, days until full, and recommendations. Configure alerts via `POST /api/storage/alerts/configure`.

---

---

## Phased implementation plan

| Phase | Scope                | Weeks | Key Deliverables                                                                                                                           |
| ----- | -------------------- | ----- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| 1     | Core partitioning    | 1–3   | Rotation algorithm, schema creation, data copy/delete, read-only partitions, the per-query attach helper, tests                            |
| 2     | API management       | 4–6   | Partition list/metadata/buffer endpoints, rotate/move/delete behind the admin key (whose own row lands first), audit logging, OpenAPI spec |
| 3     | USB storage & growth | 7–9   | USB detect/mount/unmount, growth projection (linear regression), capacity alerts, systemd units                                            |
| 4     | Cold tier            | 10–12 | Tier migration to a local USB disk, operator-run deletion, fill-date warnings                                                              |
| 5     | Migration & rollout  | 13–15 | `--enable-partitioning` flag, backfill tool, RPi integration testing, alpha/beta/stable releases                                           |

**Total timeline:** 15 weeks. Phases 3–4 can run in parallel with Phase 2.

**Success criteria per phase:** Zero data loss during rotation, the history helper reads any time range, all API endpoints pass safety checks, USB mount/unmount handles active queries gracefully, existing deployments migrate with zero downtime, 99.9% rotation success over 30-day test.

---

## Pros and cons

### Advantages

**✅ Bounded Active Database Size**

- Main database stays small (~2-3GB max)
- Faster writes, faster VACUUM operations
- Predictable performance characteristics

**✅ Simple Archival and Backup**

- Individual partition files easy to backup
- Move to slower/cheaper storage (USB HDD) without affecting active queries

**✅ Operator-Run Deletion**

- The operator deletes a month by deleting one file; nothing is deleted automatically
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
- Archives on a slower local USB disk
- Tiered storage strategy possible

### Disadvantages

**❌ Increased Complexity**

- More files to manage
- History queries go through an attach helper
- Partition rotation logic required

**❌ Query Performance (Cross-Partition)**

- Queries spanning multiple months touch multiple files
- More disk I/O than single-file approach
- `UNION ALL` overhead across the attached months

**❌ ATTACH DATABASE Limits**

- Ten attached databases per connection, the driver's compiled limit
- A history query longer than ten archived months reads in batches and merges in Go

**❌ Operational Overhead**

- Monitoring partition count and disk usage
- Backup strategy needs partition awareness
- Debugging spans multiple files

**❌ Derived Table Challenges**

- `radar_data_transits` sessionization may span partition boundaries
- Late-arriving sessionization updates need handling
- Trade-off: include derived tables in partition vs keep centralised

---

## Alternative approaches

Five alternatives were evaluated against the proposed SQLite partition approach:

**Alternative 1: Data deletion**; Periodically `DELETE` + `VACUUM` on main database. Simplest, but data permanently lost with no archival. Contradicts user data ownership principle. **Verdict: not recommended.**

**Alternative 2: PostgreSQL with native partitioning**; Declarative `PARTITION BY RANGE`. Enterprise-grade features, but requires PostgreSQL server, increases RPi resource requirements, violates "SQLite as single source of truth" principle. **Verdict: not recommended** for current architecture. Re-evaluate for multi-device.

**Alternative 3: Time-series database** (InfluxDB, TimescaleDB); Built-in downsampling and retention. Overkill for current use case; adds complexity, loses JSON flexibility, requires separate server. **Verdict: not recommended.**

**Alternative 4: External file storage** (CSV/Parquet); Unlimited growth, interoperable formats, but loses SQL query capabilities, transactions, and referential integrity. **Verdict: not recommended.**

**Alternative 5: Hybrid hot/cold storage**; Partitions for recent data, compressed/alternative formats for old data. Most flexible but most complex. **Verdict: possible future enhancement**; start with uniform partitioning, add cold storage later.

### Comparison matrix

| Approach                         | Complexity | Data Retention | Query Performance        | Storage Efficiency | Recommendation  |
| -------------------------------- | ---------- | -------------- | ------------------------ | ------------------ | --------------- |
| **Proposed (SQLite partitions)** | Medium     | Full archival  | Good (with time filters) | Good               | **Recommended** |
| Data deletion                    | Low        | No archival    | Good (small DB)          | No archival        | No              |
| PostgreSQL                       | High       | Full archival  | Excellent                | Good               | Too complex     |
| Time-series DB                   | High       | Full archival  | Excellent                | Excellent          | Overkill        |
| External files                   | Medium     | Full archival  | Poor                     | Good               | No              |
| Hybrid                           | Very high  | Full archival  | Variable                 | Excellent          | Future          |

---

## Storage management

### Mount points and disk layout

```
/var/lib/velocity-report/         (Fast storage: SD card or SSD)
├── sensor_data.db                (Active database, ~2-3GB)
└── archives/                     (Can be symlink to slower storage)
    ├── recent/                   (Last 3 months, fast storage)
    │   ├── 2025-01_data.db
    │   ├── 2025-02_data.db
    │   └── 2025-03_data.db
    └── cold/                     (>3 months, local USB disk)
        ├── 2024-01_data.db
        └── ...
```

**Storage tiers:** Active (SD card, every unclosed month) → Recent (closed monthly archives on SD card) → Cold (closed archives on a local USB disk). Partitions live on local disks only: no NFS or other network filesystem, whose locking SQLite does not trust and whose hangs would stall every query that attaches a month. Copies on a NAS are backups, never attached.

### Disk space quotas

| Policy            | Value         | Purpose                                                       |
| ----------------- | ------------- | ------------------------------------------------------------- |
| Recent archives   | 3 months      | Target for closed months retained on fast storage             |
| Move to cold tier | After 90 days | Only after the month has closed; closure lag takes precedence |
| Delete            | Operator only | Nothing is deleted automatically (October 21, 2026)           |

### Storage growth (Raspberry Pi 4, 64GB SD)

| Month | Active DB | Recent (SD) | Cold (USB) | SD Usage    |
| ----- | --------- | ----------- | ---------- | ----------- |
| 1     | 3 GB      | 0 GB        | 0 GB       | 13 GB (20%) |
| 6     | 3 GB      | 9 GB        | 9 GB       | 22 GB (34%) |
| 12    | 3 GB      | 9 GB        | 27 GB      | 22 GB (34%) |
| 24    | 3 GB      | 9 GB        | 60 GB      | 22 GB (34%) |

These are planning estimates assuming prompt month closure, not measurements on a Pi. Under
those assumptions SD card usage is about 22 GB, including about 10 GB for the system. An open
session or stalled worker can retain earlier months in main and exceed this estimate; storage
alerts must show that lag rather than force archival.

### Compression

None (October 21, 2026). Partitions stay plain SQLite files, readable in place. Decompressing a
month before each query would need temporary space the size of the partition (about 2.6 GB) on the
card the scheme exists to save.

### systemd mount units

Archives directory can be a systemd mount pointing to a USB HDD. If the mount is unavailable at startup, the service falls back to local storage with a warning.

---

## Migration path

### Phase 1: pre-migration (development)

Validate partitioning with test data: implement rotation, test with synthetic months, benchmark single-file vs partitioned, test failure scenarios.

### Phase 2: opt-in partitioning (existing deployments)

Add `--enable-partitioning` flag (default: disabled). On first run: analyse data for partition boundaries, offer backfill or start fresh. Backward compatible; disable flag returns to single-file behaviour; union views continue working.

### Phase 3: historical backfill (optional)

> **Source:** `BackfillPartitions()`; implementation in `internal/db/partitions/` (when implemented)

User chooses: (A) backfill historical data into partitions (slow, enables full partitioning), or (B) start fresh, keep historical data in main DB (fast, mixed mode).

### Phase 4: new deployments (default enabled)

`--enable-partitioning` becomes default. Initial schema creates empty archives directory. First closure check on the 2nd of the second month; rotation waits for session and worker completion.

### Rollback strategy

Disable partitioning: `velocity-report --disable-partitioning`. History queries still attach archived months read-only. New writes go to main database only. Partitions preserved as archives.

**Merge back:** Emergency rollback merges all partition data back into the main database. Iterates over all partition files, copies each into main via `CopyPartitionToMain()`, then the partitions can be removed. Use if partitioning causes issues.

---

## Performance implications

The figures in this section are inherited planning estimates without a recorded benchmark run.
They establish no physical-device performance claim; measure them on the device of record before
setting release acceptance from them. Closure lag can also increase the main file beyond these
examples.

### Write performance

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
| ------------- | -------------------- | ------------------------ |
| 1GB           | 1000                 | 1000                     |
| 10GB          | 800                  | 1000                     |
| 30GB          | 500                  | 1000                     |

**Conclusion:** Partitioning maintains consistent write performance.

### Query performance

**Scenario 1: Single-Month Query (Most Common)**

A query filtering the current month’s data (e.g. `WHERE write_timestamp BETWEEN <current_month_start> AND <now>`) touches only the active partition.

**Current:** Query touches entire database (slower)
**Partitioned:** ✅ Query touches only active partition (faster)

**Scenario 2: Multi-Month Query**

A query spanning the last 3 months (e.g. `WHERE write_timestamp BETWEEN <3_months_ago> AND <now>`) touches 3 partitions.

**Current:** Single database (faster)
**Partitioned:** ⚠️ Queries 3 partitions (slower due to UNION)

**Scenario 3: Historical Query (6+ months)**

A query spanning the last year (e.g. `WHERE write_timestamp BETWEEN <1_year_ago> AND <now>`) touches 12 partitions.

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

### Rotation overhead

**Rotation Process Duration:**

- ~30-60 seconds for 2.6GB partition (Raspberry Pi 4)
- Runs once per month at low-traffic time (00:00 UTC)

**Impact:**

- Brief write pause during DELETE phase (~5-10 seconds)
- Read queries continue normally (WAL mode)
- Acceptable monthly maintenance window

---

## Operational considerations

### Monitoring and alerting

| Metric               | Alert Threshold                        | Action                                                                    |
| -------------------- | -------------------------------------- | ------------------------------------------------------------------------- |
| Active database size | >5 GB                                  | Rotation may be failing                                                   |
| SD card usage        | >80%                                   | Move partitions to USB                                                    |
| USB HDD usage        | >90%                                   | Warn with the projected fill date; the operator decides                   |
| Rotation health      | 2+ consecutive failures                | Investigate and alert                                                     |
| Month closure lag    | Month deferred at an eligibility check | Show open sessions, worker backlog, and retained bytes; never force-close |
| Query latency (p95)  | Significant increase                   | Review partition count                                                    |

```
velocity.report Storage Health
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Active DB:        2.8GB / 5GB   [████████░░] 56%
Partitions:       8 months
SD Card:         18GB / 64GB    [███░░░░░░░] 28%
USB HDD:         24GB / 1TB     [░░░░░░░░░░]  2%

Last Rotation:    2025-03-02T00:00:15Z     ✅ Success
Avg Query Time:   45ms (p95: 120ms)        ✅ Healthy
```

### Backup strategy

> **Source:** Reference backup script in `scripts/backup/` (when implemented)

**Frequency:** Active database daily, recent partitions weekly, cold partitions monthly (or after creation).

**Recovery:** Copy `sensor_data.db` and `archives/*` from backup, restart service.

### Debug surfaces

- **SQL console (`/debug/tailsql/`):** stays on its own read-only handle to the main file. The
  page says that it excludes archives and can include earlier unclosed months or retained
  pre-partitioning history. It does not claim to show only the current month.
- **Database stats (`/debug/db-stats`):** lists every partition with its month, tier and size
  beside the main file.
- **Backup route:** with partitioning on, includes the archives with the main file, or refuses and
  names the reference backup script. It never returns a backup of the main file alone without
  saying so.

### Deployment

**systemd service** adds `After=` dependency on the archives mount, `ExecStartPre` ensures archive directories exist, and flags include `--enable-partitioning`.

> **Source:** `PreflightChecks()`; implementation in `internal/db/partitions/` (when implemented). Checks SD card free space (minimum 10 GB) and archives mount availability.

---

## Success metrics

These are acceptance targets for an implementation, not completed checks or measured results.

### Technical metrics

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

### Operational metrics

**Usability:**

- ✅ Setup time ≤10 minutes for new deployment
- ✅ Migration time ≤1 hour for existing deployment
- ✅ Zero manual intervention for rotation (automated)

**Maintainability:**

- ✅ Clear error messages for rotation failures
- ✅ Automated monitoring alerts
- ✅ Documented rollback procedure

### User satisfaction

**Feedback Targets:**

- ✅ Positive feedback from beta testers (3+ deployments)
- ✅ No reported data loss incidents
- ✅ <5% request for rollback to single-file mode

---

## Design decisions (resolved)

| Decision                  | Resolution                                                                                                                                                     |
| ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Monthly vs quarterly      | Monthly only, not configurable; no consolidation into larger files (October 21, 2026)                                                                          |
| Derived tables (transits) | The month waits for all touching sessions to close and transit processing to finish; cross-month record/link placement remains an implementation blocker       |
| Month closure             | The 2nd is the first eligibility check, not forced closure; delayed months stay writable in main, with lag and blocking work reported                          |
| SQLITE_MAX_ATTACHED       | Ten, the driver's compiled limit: each history query attaches the months it covers on its own connection, and longer ranges read in batches (October 21, 2026) |
| Compression               | None (October 21, 2026)                                                                                                                                        |
| Rotation locking          | `flock` on a lock file plus an in-process mutex (October 21, 2026)                                                                                             |
| Timezone                  | UTC for rotation triggers                                                                                                                                      |
| Single-file mode          | Keep indefinitely as default: partitioning is opt-in                                                                                                           |
| Monitoring                | Built-in disk usage on /api/status                                                                                                                             |
| Backup                    | Provide reference scripts in `scripts/backup/`                                                                                                                 |
| Cloud storage             | Out of scope: local-first principle; backup to USB only                                                                                                        |
| Network filesystems       | None: partitions on local disks only; NAS copies are backups, never attached (October 21, 2026)                                                                |
| USB mount                 | Writable with `nosuid,nodev,noexec,noatime`; each partition `0444` and attached `mode=ro` (October 21, 2026)                                                   |
| Storage actions           | Behind API authentication (an admin key), which lands first as its own backlog row (October 21, 2026)                                                          |
| Debug surfaces            | Console on main only, including unclosed months; stats list every partition; the backup route includes the archives or refuses (October 21, 2026)              |
| Retention                 | No automatic deletion: the operator deletes months; disk alerts warn with a projected fill date (October 21, 2026)                                             |

The owner settled the storage choices from the October 20 security review. Review then made
month closure depend on finished sessions and transit processing, and made query batches
disjoint. These choices do not close the implementation problems below.

## Open implementation problems

The design remains Proposed. Resolve these in the implementing design and tests before enabling
partitioning; the calendar, a file lock, and an archive filename do not resolve them.

| Problem                              | Required resolution and evidence                                                                                                                                                                                                                                                                                                |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Crash-safe rotation                  | Define durable staging, publication, source-deletion, and completion states, including filesystem sync ordering. Inject termination at every boundary and prove restart neither loses nor counts rows twice. Never skip a partial archive because its final filename exists.                                                    |
| Closure lag and cross-month sessions | Define the worker progress record and how closure eligibility prevents later writes. Close a session crossing midnight and month-end, drain its work, and prove the month remains in main until then. Define transit and link placement across months without orphaning readings or duplicating transits.                       |
| Queries during storage changes       | Define reader admission and archive discovery across recent and USB tiers. Test rotation, move, delete, and unmount against newly starting and batched queries, including cancellation and failed detach. Missing or unavailable archives must produce an explicit incomplete-history error, not an apparently complete result. |
| Corrections after sealing            | Define how an operator reprocesses old readings or repairs derived transits. Normal ingest and the worker must refuse sealed-month writes; no read-write exception to the archive policy is implied.                                                                                                                            |

The synthetic acceptance cases above are planned tests, not measurements of implemented code.

## References

### SQLite documentation

- [ATTACH DATABASE](https://www.sqlite.org/lang_attach.html)
- [Limits: Maximum Number of Attached Databases](https://www.sqlite.org/limits.html#max_attached)
- [Write-Ahead Logging (WAL)](https://www.sqlite.org/wal.html)
- [Query Planning and Optimisation](https://www.sqlite.org/queryplanner.html)

### Partitioning patterns

- [Time-Series Partitioning Best Practices](<https://en.wikipedia.org/wiki/Partition_(database)>)
- [SQLite Performance Tuning](https://www.sqlite.org/speed.html)

### Related velocity.report documentation

- [ARCHITECTURE.md](../../../ARCHITECTURE.md) - System architecture overview
- [README.md](../../../README.md) - Project overview
- [`internal/db/schema.sql`](../../../internal/db/schema.sql) - Current database schema

### Future reading

- Multi-device support design (planned, doc not written yet)

---

## Revision history

| Version | Date       | Author  | Changes                                                                                                                                                                                                                     |
| ------- | ---------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1.0     | 2025-12-01 | Ictinus | Initial design specification                                                                                                                                                                                                |
| 1.1     | 2026-10-21 | Owner   | Security review settled: monthly partitions attached per query, flock rotation, no compression, local disks only, writable hardened USB mount, authentication before storage actions, debug surfaces, no automatic deletion |

---

## Appendix a: example queries

These run through the history helper, which defines `radar_data_all` on the query's own
connection. For a batched range, main appears in its first view only; the helper merges all
batches before applying final grouping, ordering, and pagination.

### Query current month data

```sql
-- Fast: Only touches active partition
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN UNIXEPOCH('now', 'start of month') AND UNIXEPOCH('now');
```

### Query last 3 months

```sql
-- Moderate: Touches 3 partitions
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN UNIXEPOCH('now', '-3 months') AND UNIXEPOCH('now')
ORDER BY write_timestamp DESC;
```

### Query specific month (historical)

```sql
-- Fast: Only touches one archived partition
SELECT * FROM radar_data_all
WHERE write_timestamp BETWEEN 1704067200.0 AND 1706745600.0  -- Jan 2024
ORDER BY write_timestamp;
```

### Aggregation across all time

```sql
-- The helper reads disjoint batches, includes main once, and merges by date.
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
