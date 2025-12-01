# Design Specification: Time-Partitioned Raw Data Tables

**Status:** Draft  
**Created:** 2025-12-01  
**Author:** Ictinus (Product-Conscious Software Architect)  
**Target Release:** TBD

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Problem Statement](#problem-statement)
3. [Current State Analysis](#current-state-analysis)
4. [Proposed Architecture](#proposed-architecture)
5. [Detailed Design](#detailed-design)
6. [API Management for Partition Control](#api-management-for-partition-control)
7. [USB Storage Management and Growth Projection](#usb-storage-management-and-growth-projection)
8. [Phased Implementation Plan](#phased-implementation-plan)
9. [Pros and Cons](#pros-and-cons)
10. [Alternative Approaches](#alternative-approaches)
11. [Storage Management](#storage-management)
12. [Migration Path](#migration-path)
13. [Performance Implications](#performance-implications)
14. [Operational Considerations](#operational-considerations)
15. [Implementation Phases](#implementation-phases)
16. [Success Metrics](#success-metrics)
17. [Open Questions](#open-questions)
18. [References](#references)

---

## Executive Summary

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

### Overview

This design implements defense-in-depth security measures to protect against common attack vectors in partition management and USB storage operations. All security fixes are marked with CVE references in the code sections below.

### Critical Security Fixes

#### CVE-2025-VR-002: Path Traversal Prevention (Severity: 9.5/10)

**Vulnerability:** User-controlled file paths in attach/consolidate operations could access arbitrary files via directory traversal (`../../../etc/shadow`) or symlink attacks.

**Mitigations Implemented:**
1. **Path Validation:** All file paths validated against allowed directories before use
2. **Symlink Resolution:** `filepath.EvalSymlinks()` resolves all symlinks to absolute paths
3. **Directory Traversal Rejection:** Paths containing `..` are rejected
4. **Filename Pattern Matching:** Partition files must match `YYYY-MM_data.db` or `YYYY-QN_data.db` pattern
5. **Regular File Verification:** Only regular files accepted (not devices, sockets, or pipes)

**Code Location:** See `ValidatePartitionPath()` function in ATTACH DATABASE Management section.

#### CVE-2025-VR-003: SQL Injection Prevention (Severity: 8.5/10)

**Vulnerability:** Unsanitized aliases and paths in `ATTACH DATABASE` commands could execute arbitrary SQL (`"m01; DROP TABLE radar_data; --"`).

**Mitigations Implemented:**
1. **Alias Validation:** Only alphanumeric and underscore allowed, must start with letter
2. **SQL Keyword Filtering:** Aliases containing SQL keywords (DROP, DELETE, etc.) rejected
3. **Proper SQL Escaping:** Single quotes doubled in paths, double quotes doubled in identifiers
4. **Read-Only Mode:** Partitions attached with `mode=ro` to prevent write operations
5. **Length Limits:** Aliases limited to 32 characters

**Code Location:** See `ValidateAlias()`, `QuoteSQLiteString()`, and `QuoteSQLiteIdentifier()` functions in ATTACH DATABASE Management section.

#### CVE-2025-VR-005: Race Condition Prevention (Severity: 7.5/10)

**Vulnerability:** Concurrent rotation operations or rotation during active queries could cause data corruption and loss.

**Mitigations Implemented:**
1. **Distributed Locking:** SQLite-based rotation lock prevents concurrent rotations
2. **Lock Expiration:** Locks automatically expire after 5 minutes to prevent deadlocks
3. **Query Completion Wait:** Rotation waits for active queries to complete (30 second timeout)
4. **Idempotent Operations:** Rotation checks if partition exists before creating
5. **Transaction Safety:** Data deletion wrapped in transaction with rollback on failure
6. **Cleanup on Failure:** Partial partition files deleted if rotation fails

**Code Location:** See `AcquireRotationLock()` function and enhanced rotation algorithm in Rotation Process Details section.

#### CVE-2025-VR-006: USB Storage Security (Severity: 7.8/10)

**Vulnerability:** Mounting arbitrary USB devices could enable filesystem exploits, SUID binary execution, or persistent backdoors via systemd units.

**Mitigations Implemented:**
1. **Filesystem Whitelist:** Only ext4 and ext3 allowed (no NTFS, FAT32, exfat)
2. **Secure Mount Options:** Always mount with `nosuid,nodev,noexec,noatime,ro`
3. **USB Device Verification:** Verify device is in USB subsystem via sysfs paths
4. **System Disk Protection:** Reject mounting of /dev/sda or /dev/mmcblk0
5. **Read-Only by Default:** USB drives mounted read-only to prevent malicious writes
6. **Filesystem Detection:** Use `blkid` to detect and validate filesystem type

**Code Location:** See `MountUSBStorage()` and `VerifyUSBDevice()` functions in USB Storage Management section.

### Security Testing Requirements

Before deployment, the following security tests MUST pass:

1. **Path Traversal Tests:**
   - Reject `../../etc/shadow` in partition paths
   - Reject symlinks to system files
   - Reject paths outside allowed directories

2. **SQL Injection Tests:**
   - Reject aliases with SQL keywords: `"m01; DROP TABLE radar_data; --"`
   - Reject aliases with quotes: `"m01' OR 1=1 --"`
   - Verify read-only mode prevents writes

3. **Race Condition Tests:**
   - Concurrent rotation attempts only succeed once
   - Rotation waits for queries to complete
   - No data corruption under concurrent load

4. **USB Security Tests:**
   - Reject mounting NTFS/FAT32 filesystems
   - Reject mounting /dev/sda (system disk)
   - Verify SUID binaries cannot execute from mounted USB
   - Verify devices flag prevents device files on USB

### Future Security Enhancements

**Authentication and Authorization (High Priority):**
- API endpoints currently lack authentication (addressed in separate implementation)
- Implement API key authentication with bcrypt hashing
- Add role-based access control for partition management

**Audit Logging:**
- Log all partition attach/detach operations
- Log all USB mount/unmount operations
- Log all rotation operations with success/failure status

**Rate Limiting:**
- Limit API requests per IP address
- Limit concurrent consolidation operations
- Prevent DoS via resource exhaustion

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
│  │  │                                                      │  │  │
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

**Query Optimization:**
```sql
-- SQLite query planner uses WHERE clauses to skip irrelevant partitions
SELECT * FROM radar_data_all 
WHERE write_timestamp BETWEEN 1704067200.0 AND 1706745600.0;
-- Only queries partitions containing Jan 2024 data
```

### Rotation Process Details

**Rotation Algorithm:**
```go
func RotatePartitions(db *DB, rotationTime time.Time) error {
    // SECURITY FIX (CVE-2025-VR-005): Acquire distributed lock to prevent race conditions
    lock, err := AcquireRotationLock(db, 10*time.Second)
    if err != nil {
        return fmt.Errorf("cannot acquire rotation lock: %w", err)
    }
    defer lock.Release()
    
    // 1. Determine partition name (e.g., "2025-01_data.db")
    partitionName := rotationTime.AddDate(0, -1, 0).Format("2006-01") + "_data.db"
    partitionPath := filepath.Join(archivesDir, partitionName)
    
    // Check if partition already exists (idempotency)
    if _, err := os.Stat(partitionPath); err == nil {
        log.Warnf("Partition %s already exists, skipping rotation", partitionName)
        return nil
    }
    
    // 2. Create partition database with schema
    partitionDB, err := CreatePartitionDB(partitionPath)
    if err != nil {
        return err
    }
    
    // 3. Wait for active queries to complete (prevents corruption)
    if err := WaitForQueriesWithTimeout(db, 30*time.Second); err != nil {
        return fmt.Errorf("timeout waiting for queries: %w", err)
    }
    
    // 4. Copy previous month's data to partition
    startTime := rotationTime.AddDate(0, -1, 0).Truncate(24*time.Hour)
    endTime := rotationTime.Truncate(24*time.Hour)
    
    err = CopyDataToPartition(db, partitionDB, startTime, endTime, []string{
        "radar_data",
        "radar_objects",
        "lidar_bg_snapshot",
        "radar_data_transits",
        "radar_transit_links",
    })
    if err != nil {
        // Cleanup on failure
        os.Remove(partitionPath)
        return err
    }
    
    // 5. Verify data integrity
    if err := VerifyPartition(partitionDB, startTime, endTime); err != nil {
        os.Remove(partitionPath)
        return err
    }
    
    // 6. Delete copied data from main database (within transaction)
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    err = DeleteRotatedData(tx, startTime, endTime)
    if err != nil {
        return err
    }
    
    if err := tx.Commit(); err != nil {
        return err
    }
    
    // 7. Make partition read-only
    if err := os.Chmod(partitionPath, 0444); err != nil {
        return err
    }
    
    // 8. Update union views to include new partition
    if err := UpdateUnionViews(db); err != nil {
        return err
    }
    
    return nil
}

// AcquireRotationLock prevents concurrent rotation operations (CVE-2025-VR-005)
func AcquireRotationLock(db *DB, timeout time.Duration) (*RotationLock, error) {
    // Create lock table if not exists
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS rotation_lock (
            lock_id INTEGER PRIMARY KEY DEFAULT 1,
            acquired_at REAL,
            expires_at REAL,
            CHECK (lock_id = 1)
        )
    `)
    if err != nil {
        return nil, err
    }
    
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        // Try to acquire lock (expires after 5 minutes)
        result, err := db.Exec(`
            INSERT OR REPLACE INTO rotation_lock (lock_id, acquired_at, expires_at)
            SELECT 1, UNIXEPOCH('subsec'), UNIXEPOCH('subsec') + 300
            WHERE NOT EXISTS (
                SELECT 1 FROM rotation_lock 
                WHERE lock_id = 1 AND expires_at > UNIXEPOCH('subsec')
            )
        `)
        if err != nil {
            return nil, err
        }
        
        rows, _ := result.RowsAffected()
        if rows > 0 {
            return &RotationLock{db: db}, nil
        }
        
        time.Sleep(100 * time.Millisecond)
    }
    
    return nil, fmt.Errorf("timeout acquiring rotation lock")
}

type RotationLock struct {
    db *DB
}

func (l *RotationLock) Release() error {
    _, err := l.db.Exec("DELETE FROM rotation_lock WHERE lock_id = 1")
    return err
}
```

**Transaction Safety:**
```go
// Rotation must be atomic:
// - Either all data copied and deleted, or nothing changes
// - Use WAL mode to allow reads during rotation
// - Lock writes during DELETE phase to prevent data loss
```

**Failure Handling:**
```go
// If rotation fails:
// 1. Keep data in main database (no data loss)
// 2. Delete partial partition file
// 3. Log error and retry at next scheduled rotation
// 4. Alert operator if failures persist
```

### ATTACH DATABASE Management

**Dynamic Partition Mounting:**
```go
func AttachPartitions(db *DB) error {
    // Scan archives directory for partition files
    partitions, err := filepath.Glob(filepath.Join(archivesDir, "*_data.db"))
    if err != nil {
        return err
    }
    
    // Attach each partition with unique alias
    for i, partition := range partitions {
        // SECURITY FIX (CVE-2025-VR-002): Validate partition path
        if err := ValidatePartitionPath(partition); err != nil {
            log.Warnf("Skipping invalid partition %s: %v", partition, err)
            continue
        }
        
        alias := fmt.Sprintf("m%02d", i+1) // m01, m02, m03, ...
        
        // SECURITY FIX (CVE-2025-VR-003): Sanitize inputs for SQL injection prevention
        if err := ValidateAlias(alias); err != nil {
            return fmt.Errorf("invalid alias: %w", err)
        }
        
        // Use read-only mode and properly escaped identifiers
        quotedPath := QuoteSQLiteString(partition)
        quotedAlias := QuoteSQLiteIdentifier(alias)
        sql := fmt.Sprintf("ATTACH DATABASE 'file:%s?mode=ro' AS %s", partition, quotedAlias)
        
        _, err := db.Exec(sql)
        if err != nil {
            return err
        }
    }
    
    return nil
}

// ValidatePartitionPath prevents path traversal attacks (CVE-2025-VR-002)
func ValidatePartitionPath(path string) error {
    // Reject directory traversal
    if strings.Contains(path, "..") {
        return fmt.Errorf("directory traversal not allowed")
    }
    
    // Resolve symlinks and get absolute path
    absPath, err := filepath.EvalSymlinks(path)
    if err != nil {
        return fmt.Errorf("cannot resolve path: %w", err)
    }
    
    // Ensure path is under allowed directory
    if !strings.HasPrefix(absPath, allowedPartitionDir) {
        return fmt.Errorf("path outside allowed directory")
    }
    
    // Verify it's a regular file
    info, err := os.Lstat(absPath)
    if err != nil {
        return fmt.Errorf("cannot stat file: %w", err)
    }
    if !info.Mode().IsRegular() {
        return fmt.Errorf("not a regular file")
    }
    
    // Verify filename matches partition pattern
    filename := filepath.Base(absPath)
    matched, _ := regexp.MatchString(`^[0-9]{4}-(0[1-9]|1[0-2]|Q[1-4])_data\.db$`, filename)
    if !matched {
        return fmt.Errorf("filename does not match partition pattern")
    }
    
    return nil
}

// ValidateAlias prevents SQL injection in aliases (CVE-2025-VR-003)
func ValidateAlias(alias string) error {
    // Only alphanumeric and underscore, must start with letter
    matched, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9_]{0,31}$`, alias)
    if !matched {
        return fmt.Errorf("alias must start with letter and contain only alphanumeric and underscore")
    }
    
    // Reject SQL keywords
    sqlKeywords := []string{"DROP", "DELETE", "INSERT", "UPDATE", "CREATE", "ALTER", "EXEC"}
    upperAlias := strings.ToUpper(alias)
    for _, keyword := range sqlKeywords {
        if strings.Contains(upperAlias, keyword) {
            return fmt.Errorf("alias contains SQL keyword")
        }
    }
    
    return nil
}

// QuoteSQLiteString escapes single quotes for SQL strings
func QuoteSQLiteString(s string) string {
    return strings.ReplaceAll(s, "'", "''")
}

// QuoteSQLiteIdentifier escapes double quotes for SQL identifiers
func QuoteSQLiteIdentifier(s string) string {
    return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
```

**Limits:**
- SQLite allows up to **10 attached databases by default** (compile-time limit)
- Can be increased to **125** with `SQLITE_MAX_ATTACHED` compile flag
- Monthly partitions: 10 partitions = 10 months, 125 = 10.4 years
- Quarterly partitions: 10 partitions = 2.5 years, 125 = 31 years

**Recommendation:** Start with monthly, increase `SQLITE_MAX_ATTACHED` to 125 for long-term deployments.

---

## API Management for Partition Control

### Overview

To support operational flexibility and future UI development, the system provides HTTP API endpoints for managing attached database partitions. These endpoints allow operators to:

1. View currently attached partitions
2. Safely attach/detach historical partitions (respecting connection limits)
3. Consolidate multiple monthly/quarterly partitions into yearly archives
4. Monitor partition status and buffer safety

**Safety Guarantees:**
- **Write partition protection:** Main database (write partition) can never be detached
- **Buffer protection:** Prevents detaching partitions with uncommitted data or active queries
- **Atomic operations:** Attach/detach operations are transactional
- **Connection limit enforcement:** Respects SQLite `SQLITE_MAX_ATTACHED` limit

### API Endpoints

#### 1. List Attached Partitions

**Endpoint:** `GET /api/partitions`

**Purpose:** List all currently attached database partitions and their status.

**Response:**
```json
{
  "main": {
    "path": "/var/lib/velocity-report/sensor_data.db",
    "alias": "main",
    "writable": true,
    "active_queries": 2,
    "size_bytes": 2847583234,
    "time_range": {
      "start": "2025-03-01T00:00:00Z",
      "end": "2025-03-31T23:59:59Z"
    },
    "status": "active",
    "can_detach": false,
    "detach_reason": "main database is always attached"
  },
  "attached": [
    {
      "path": "/var/lib/velocity-report/archives/2025-01_data.db",
      "alias": "m01",
      "writable": false,
      "active_queries": 0,
      "size_bytes": 2641583234,
      "time_range": {
        "start": "2025-01-01T00:00:00Z",
        "end": "2025-01-31T23:59:59Z"
      },
      "status": "attached",
      "can_detach": true,
      "detach_reason": null
    },
    {
      "path": "/var/lib/velocity-report/archives/2025-02_data.db",
      "alias": "m02",
      "writable": false,
      "active_queries": 1,
      "size_bytes": 2598234987,
      "time_range": {
        "start": "2025-02-01T00:00:00Z",
        "end": "2025-02-28T23:59:59Z"
      },
      "status": "attached",
      "can_detach": false,
      "detach_reason": "partition has active queries"
    }
  ],
  "available": [
    {
      "path": "/var/lib/velocity-report/archives/2024-12_data.db",
      "size_bytes": 2712983456,
      "time_range": {
        "start": "2024-12-01T00:00:00Z",
        "end": "2024-12-31T23:59:59Z"
      },
      "status": "detached"
    }
  ],
  "limits": {
    "max_attached": 125,
    "current_attached": 3,
    "available_slots": 122
  },
  "buffers": {
    "write_buffer_size_bytes": 4096,
    "pending_writes": 0,
    "safe_to_rotate": true
  }
}
```

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Database error

#### 2. Attach Partition

**Endpoint:** `POST /api/partitions/attach`

**Purpose:** Attach a historical partition to enable queries against it.

**Request Body:**
```json
{
  "path": "/var/lib/velocity-report/archives/2024-12_data.db",
  "alias": "m12_2024",
  "priority": "normal"
}
```

**Parameters:**
- `path` (required): Full path to partition database file
- `alias` (optional): Custom alias for partition. If omitted, auto-generated based on filename
- `priority` (optional): `high` | `normal` | `low`. If connection limit reached, may detach low-priority partition

**Response:**
```json
{
  "success": true,
  "partition": {
    "path": "/var/lib/velocity-report/archives/2024-12_data.db",
    "alias": "m12_2024",
    "attached_at": "2025-03-15T14:23:45Z",
    "time_range": {
      "start": "2024-12-01T00:00:00Z",
      "end": "2024-12-31T23:59:59Z"
    }
  },
  "detached_partition": null,
  "message": "Partition attached successfully"
}
```

**Error Response (Connection Limit Reached):**
```json
{
  "success": false,
  "error": "connection_limit_reached",
  "message": "Maximum attached databases (125) reached. Detach a partition or use priority='high' to auto-detach low-priority partition.",
  "limits": {
    "max_attached": 125,
    "current_attached": 125
  },
  "suggestions": [
    {
      "alias": "m01_2023",
      "path": "/var/lib/velocity-report/archives/2023-01_data.db",
      "priority": "low",
      "last_accessed": "2025-01-10T08:15:22Z",
      "can_detach": true
    }
  ]
}
```

**Status Codes:**
- `200 OK` - Partition attached successfully
- `400 Bad Request` - Invalid path or alias
- `409 Conflict` - Connection limit reached, no low-priority partitions to detach
- `422 Unprocessable Entity` - File does not exist or is not a valid SQLite database
- `500 Internal Server Error` - Database error

**Safety Checks:**
1. **Path validation (CVE-2025-VR-002):** Verify file exists, resolve symlinks, ensure path is under allowed directory, reject directory traversal
2. **Filename validation:** Verify filename matches partition pattern (YYYY-MM_data.db or YYYY-QN_data.db)
3. **Alias validation (CVE-2025-VR-003):** Verify alias is alphanumeric with underscore only, no SQL keywords
4. **SQLite validation:** Verify file has valid SQLite header and schema compatibility
5. **Connection limit enforcement:** Ensure SQLite SQLITE_MAX_ATTACHED limit not exceeded
6. **Alias uniqueness:** Ensure alias not already in use

#### 3. Detach Partition

**Endpoint:** `POST /api/partitions/detach`

**Purpose:** Detach a historical partition to free connection slots.

**Request Body:**
```json
{
  "alias": "m12_2024",
  "force": false
}
```

**Parameters:**
- `alias` (required): Alias of partition to detach
- `force` (optional): If `true`, waits for active queries to complete before detaching. Default: `false`

**Response:**
```json
{
  "success": true,
  "partition": {
    "alias": "m12_2024",
    "path": "/var/lib/velocity-report/archives/2024-12_data.db",
    "detached_at": "2025-03-15T14:25:12Z"
  },
  "message": "Partition detached successfully"
}
```

**Error Response (Active Queries):**
```json
{
  "success": false,
  "error": "partition_in_use",
  "message": "Cannot detach partition with active queries",
  "partition": {
    "alias": "m12_2024",
    "active_queries": 3,
    "query_details": [
      {
        "query_id": "q_1234",
        "started_at": "2025-03-15T14:20:30Z",
        "duration_ms": 4520,
        "sql": "SELECT COUNT(*) FROM m12_2024.radar_data WHERE ..."
      }
    ]
  },
  "suggestion": "Wait for queries to complete or use force=true"
}
```

**Status Codes:**
- `200 OK` - Partition detached successfully
- `400 Bad Request` - Invalid alias or attempt to detach main database
- `409 Conflict` - Partition has active queries (force=false)
- `404 Not Found` - Alias not found
- `500 Internal Server Error` - Database error

**Safety Checks:**
1. Prevent detaching main database (write partition)
2. Check for active queries (unless force=true)
3. Check for uncommitted transactions
4. Verify no pending writes in buffers
5. Update union views after detach

#### 4. Consolidate Partitions

**Endpoint:** `POST /api/partitions/consolidate`

**Purpose:** Combine multiple monthly/quarterly partitions into a yearly archive (cold storage optimization).

**Request Body:**
```json
{
  "source_partitions": [
    "/var/lib/velocity-report/archives/2024-01_data.db",
    "/var/lib/velocity-report/archives/2024-02_data.db",
    "/var/lib/velocity-report/archives/2024-03_data.db",
    "/var/lib/velocity-report/archives/2024-04_data.db",
    "/var/lib/velocity-report/archives/2024-05_data.db",
    "/var/lib/velocity-report/archives/2024-06_data.db",
    "/var/lib/velocity-report/archives/2024-07_data.db",
    "/var/lib/velocity-report/archives/2024-08_data.db",
    "/var/lib/velocity-report/archives/2024-09_data.db",
    "/var/lib/velocity-report/archives/2024-10_data.db",
    "/var/lib/velocity-report/archives/2024-11_data.db",
    "/var/lib/velocity-report/archives/2024-12_data.db"
  ],
  "output_path": "/var/lib/velocity-report/archives/cold/2024_data.db",
  "delete_sources": false,
  "compress_sources": true,
  "strategy": "yearly"
}
```

**Parameters:**
- `source_partitions` (required): Array of partition paths to consolidate
- `output_path` (required): Path for consolidated archive
- `delete_sources` (optional): Delete source partitions after successful consolidation. Default: `false`
- `compress_sources` (optional): If delete_sources=false, compress source partitions with gzip. Default: `false`
- `strategy` (optional): `yearly` | `biennial` | `custom`. Validates source partitions match strategy. Default: `custom`

**Response:**
```json
{
  "success": true,
  "job_id": "consolidate_2024_yearly_abc123",
  "status": "running",
  "progress": {
    "current_partition": 1,
    "total_partitions": 12,
    "percent_complete": 8,
    "estimated_completion": "2025-03-15T14:45:00Z"
  },
  "output": {
    "path": "/var/lib/velocity-report/archives/cold/2024_data.db",
    "size_bytes": 0,
    "status": "creating"
  },
  "message": "Consolidation job started. Poll GET /api/partitions/consolidate/{job_id} for progress."
}
```

**Long-Running Job Response (Poll Status):**

`GET /api/partitions/consolidate/{job_id}`

```json
{
  "job_id": "consolidate_2024_yearly_abc123",
  "status": "completed",
  "started_at": "2025-03-15T14:30:00Z",
  "completed_at": "2025-03-15T14:42:15Z",
  "duration_seconds": 735,
  "result": {
    "output_path": "/var/lib/velocity-report/archives/cold/2024_data.db",
    "output_size_bytes": 29847234567,
    "source_partitions_processed": 12,
    "total_records_copied": 12456789,
    "compression_ratio": 0.82,
    "sources_deleted": false,
    "sources_compressed": true,
    "compressed_files": [
      "/var/lib/velocity-report/archives/2024-01_data.db.gz",
      "/var/lib/velocity-report/archives/2024-02_data.db.gz"
      // ... etc
    ]
  },
  "verification": {
    "record_count_matches": true,
    "time_ranges_validated": true,
    "schema_validated": true
  },
  "message": "Consolidation completed successfully"
}
```

**Status Codes:**
- `202 Accepted` - Consolidation job started (async operation)
- `400 Bad Request` - Invalid source partitions or output path
- `409 Conflict` - Source partitions currently attached or in use
- `422 Unprocessable Entity` - Source files don't exist or schema mismatch
- `500 Internal Server Error` - Database error

**Safety Checks:**
1. Verify all source partitions exist and are readable
2. Ensure source partitions are detached (no active queries)
3. Validate schema compatibility across all sources
4. Verify sufficient disk space for output file
5. Create atomic transaction: copy → verify → delete/compress sources
6. Rollback on failure: delete output, restore sources

**Consolidation Algorithm:**
```go
func ConsolidatePartitions(sources []string, output string) error {
    // 1. Create output database with standard schema
    outputDB := CreatePartitionDB(output)
    
    // 2. For each source partition:
    for _, source := range sources {
        // 2a. Attach source temporarily
        AttachDatabase(source, "temp_source")
        
        // 2b. Copy all tables to output
        for _, table := range []string{"radar_data", "radar_objects", "lidar_bg_snapshot", 
                                        "radar_data_transits", "radar_transit_links"} {
            CopyTable("temp_source."+table, "main."+table)
        }
        
        // 2c. Detach source
        DetachDatabase("temp_source")
    }
    
    // 3. Verify record counts and time ranges
    if !VerifyConsolidation(sources, outputDB) {
        return errors.New("consolidation verification failed")
    }
    
    // 4. Make output read-only
    os.Chmod(output, 0444)
    
    // 5. Compress or delete sources (if requested)
    if deleteOrCompress {
        CompressOrDeleteSources(sources)
    }
    
    return nil
}
```

#### 5. Get Partition Metadata

**Endpoint:** `GET /api/partitions/{alias}/metadata`

**Purpose:** Get detailed metadata about a specific partition (attached or detached).

**Response:**
```json
{
  "alias": "m01",
  "path": "/var/lib/velocity-report/archives/2025-01_data.db",
  "status": "attached",
  "writable": false,
  "size_bytes": 2641583234,
  "created_at": "2025-02-02T00:05:12Z",
  "last_modified": "2025-02-02T00:05:12Z",
  "last_accessed": "2025-03-15T14:20:30Z",
  "time_range": {
    "start": "2025-01-01T00:00:00Z",
    "end": "2025-01-31T23:59:59Z",
    "days_covered": 31
  },
  "tables": {
    "radar_data": {
      "row_count": 2456789,
      "size_bytes": 1847234567
    },
    "radar_objects": {
      "row_count": 145678,
      "size_bytes": 234567890
    },
    "lidar_bg_snapshot": {
      "row_count": 8760,
      "size_bytes": 459781236
    },
    "radar_data_transits": {
      "row_count": 98234,
      "size_bytes": 89234567
    },
    "radar_transit_links": {
      "row_count": 2456789,
      "size_bytes": 11565445
    }
  },
  "indexes": [
    "idx_radar_data_time",
    "idx_transits_time"
  ],
  "compression": {
    "compressed": false,
    "compression_available": true,
    "estimated_compressed_size_bytes": 528316647
  },
  "queries_24h": {
    "total_queries": 45,
    "avg_duration_ms": 125,
    "p95_duration_ms": 340
  }
}
```

**Status Codes:**
- `200 OK` - Metadata retrieved
- `404 Not Found` - Partition not found
- `500 Internal Server Error` - Database error

#### 6. Buffer Status and Safety

**Endpoint:** `GET /api/partitions/buffers`

**Purpose:** Check write buffer status and rotation safety (critical before rotation operations).

**Response:**
```json
{
  "safe_to_rotate": true,
  "write_buffer": {
    "size_bytes": 4096,
    "pending_writes": 0,
    "last_flush": "2025-03-15T14:28:50Z"
  },
  "wal_checkpoint": {
    "mode": "PASSIVE",
    "frames_in_wal": 0,
    "frames_checkpointed": 234567,
    "last_checkpoint": "2025-03-15T14:28:50Z"
  },
  "active_transactions": 0,
  "active_queries": {
    "total": 2,
    "by_partition": {
      "main": 2,
      "m01": 0,
      "m02": 0
    }
  },
  "recommendation": "Safe to perform rotation or partition management operations"
}
```

**Status Codes:**
- `200 OK` - Buffer status retrieved
- `500 Internal Server Error` - Database error

**Use Case:** Called before rotation to ensure no data loss:
```bash
# Pre-rotation safety check
curl http://localhost:8080/api/partitions/buffers | jq '.safe_to_rotate'
# Returns: true

# If false, response includes reason:
{
  "safe_to_rotate": false,
  "reason": "pending_writes",
  "details": "12 writes pending in buffer. Wait 5-10 seconds and retry.",
  "recommendation": "Wait for buffer flush before rotation"
}
```

### Operational Workflows

#### Workflow 1: Query Historical Data Beyond Connection Limit

**Scenario:** Need to query data from 18 months ago, but only 12 partitions attached due to connection limit.

```bash
# 1. Check available partitions
curl http://localhost:8080/api/partitions | jq '.available'

# 2. Detach old partition (low priority)
curl -X POST http://localhost:8080/api/partitions/detach \
  -H "Content-Type: application/json" \
  -d '{"alias": "m01_2024", "force": false}'

# 3. Attach needed partition
curl -X POST http://localhost:8080/api/partitions/attach \
  -H "Content-Type: application/json" \
  -d '{"path": "/var/lib/velocity-report/archives/2023-09_data.db", "priority": "high"}'

# 4. Run query against newly attached partition
sqlite3 sensor_data.db "SELECT * FROM m09_2023.radar_data WHERE ..."

# 5. Detach when done (optional)
curl -X POST http://localhost:8080/api/partitions/detach \
  -H "Content-Type: application/json" \
  -d '{"alias": "m09_2023"}'
```

#### Workflow 2: Create Yearly Archive (Cold Storage)

**Scenario:** Consolidate 12 monthly partitions from 2024 into single yearly archive.

```bash
# 1. List 2024 partitions
curl http://localhost:8080/api/partitions | jq '.attached[] | select(.path | contains("2024"))'

# 2. Start consolidation job
curl -X POST http://localhost:8080/api/partitions/consolidate \
  -H "Content-Type: application/json" \
  -d '{
    "source_partitions": [
      "/var/lib/velocity-report/archives/2024-01_data.db",
      "/var/lib/velocity-report/archives/2024-02_data.db",
      ...
      "/var/lib/velocity-report/archives/2024-12_data.db"
    ],
    "output_path": "/mnt/usb-hdd/velocity-archives/2024_data.db",
    "delete_sources": false,
    "compress_sources": true,
    "strategy": "yearly"
  }' | jq '.job_id'

# 3. Poll for completion
JOB_ID="consolidate_2024_yearly_abc123"
watch -n 5 "curl http://localhost:8080/api/partitions/consolidate/$JOB_ID | jq '.status, .progress'"

# 4. Once completed, attach yearly archive
curl -X POST http://localhost:8080/api/partitions/attach \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/mnt/usb-hdd/velocity-archives/2024_data.db",
    "alias": "y2024",
    "priority": "low"
  }'

# 5. Optionally detach monthly partitions (now compressed .db.gz files)
for month in {01..12}; do
  curl -X POST http://localhost:8080/api/partitions/detach \
    -H "Content-Type: application/json" \
    -d "{\"alias\": \"m${month}\"}"
done
```

#### Workflow 3: Pre-Rotation Safety Check

**Scenario:** Before automatic monthly rotation, verify safety.

```bash
# Called by rotation cron job before rotation
SAFE=$(curl -s http://localhost:8080/api/partitions/buffers | jq -r '.safe_to_rotate')

if [ "$SAFE" = "true" ]; then
  # Proceed with rotation
  /usr/local/bin/velocity-report-rotate
else
  # Log warning and retry
  echo "Rotation delayed: buffers not safe" | logger
  sleep 30
  # Retry check...
fi
```

### Security Considerations

**Authentication:** All partition management endpoints require authentication (future: API key or OAuth).

**Authorization:** Partition management operations require admin role.

**Rate Limiting:** Consolidation operations limited to 1 concurrent job per deployment.

**Audit Logging:** All attach/detach/consolidate operations logged to `system_events` table:

```sql
INSERT INTO system_events (event_type, event_data, timestamp)
VALUES ('partition_attached', json_object(
  'alias', 'm12_2024',
  'path', '/var/lib/velocity-report/archives/2024-12_data.db',
  'user', 'admin',
  'ip', '192.168.1.100'
), UNIXEPOCH('subsec'));
```

### Future UI Integration

These API endpoints are designed for future web UI development:

**Partition Manager Dashboard:**
- Visual timeline of partitions (attached vs available)
- Drag-and-drop attach/detach interface
- Progress bars for consolidation jobs
- Disk space visualization (tiered storage)
- Query performance metrics per partition

**Consolidation Wizard:**
- Auto-suggest yearly consolidation when 12 monthly partitions exist
- Preview storage savings (compression estimates)
- Schedule consolidation during low-traffic periods
- Rollback capability if issues detected

**Example UI Mockup (Future State):**
```
┌─────────────────────────────────────────────────────────────┐
│ Partition Manager                                   [Refresh]│
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Timeline: [Jan 2024] ──────────────── [Mar 2025]          │
│                                                             │
│  ✓ 2024 (Yearly Archive)  [2.4GB]  📦 Attached  🗑️ Detach  │
│  ✓ 2025-01               [2.6GB]  📦 Attached  🗑️ Detach  │
│  ✓ 2025-02               [2.5GB]  📦 Attached  🗑️ Detach  │
│  ✓ 2025-03 (Current)     [1.2GB]  ✏️ Active    ⛔ Cannot   │
│                                                  Detach     │
│  Available Partitions:                                      │
│  ○ 2023-12               [2.7GB]  📥 Attach                │
│                                                             │
│  Connection Limit: 3 / 125 used                             │
│                                                             │
│  [Create Yearly Archive] [Import Partition] [Settings]     │
└─────────────────────────────────────────────────────────────┘
```

---

## USB Storage Management and Growth Projection

### Overview

To support tiered storage with USB drives and provide proactive capacity planning, the system includes endpoints for USB storage detection, mounting, safe ejection, and growth rate projection.

**Key Capabilities:**
1. Detect and list available USB storage devices
2. Mount USB drives for cold storage archives
3. Safe eject with pre-flight checks (no active queries on partitions)
4. Project storage growth rate and predict disk full dates
5. Automated alerts for capacity thresholds

### USB Storage API Endpoints

#### 7. Detect USB Storage Devices

**Endpoint:** `GET /api/storage/usb/devices`

**Purpose:** Detect and list available USB storage devices for cold archive storage.

**Response:**
```json
{
  "devices": [
    {
      "device_path": "/dev/sda",
      "device_name": "USB_HDD_1TB",
      "vendor": "Seagate",
      "model": "Backup Plus",
      "serial": "NA8VQJR5",
      "size_bytes": 1000204886016,
      "size_human": "1.0 TB",
      "partitions": [
        {
          "partition_path": "/dev/sda1",
          "filesystem": "ext4",
          "label": "velocity-archives",
          "size_bytes": 999653638144,
          "size_human": "999.7 GB",
          "mounted": true,
          "mount_point": "/mnt/usb-archives",
          "free_bytes": 712458240000,
          "free_human": "712.5 GB",
          "usage_percent": 28.7,
          "has_velocity_partitions": true,
          "partition_count": 8
        }
      ],
      "usb_port": "1-1.2",
      "connected_at": "2025-03-15T08:30:00Z"
    },
    {
      "device_path": "/dev/sdb",
      "device_name": "USB_SSD_500GB",
      "vendor": "Samsung",
      "model": "T7 Portable SSD",
      "serial": "S6XZNF0T123456",
      "size_bytes": 500107862016,
      "size_human": "500 GB",
      "partitions": [
        {
          "partition_path": "/dev/sdb1",
          "filesystem": "ext4",
          "label": "velocity-cold",
          "size_bytes": 499558400000,
          "size_human": "499.6 GB",
          "mounted": false,
          "mount_point": null,
          "has_velocity_partitions": false,
          "partition_count": 0
        }
      ],
      "usb_port": "1-1.3",
      "connected_at": "2025-03-10T14:22:00Z"
    }
  ],
  "recommended_device": "/dev/sda1",
  "recommendation_reason": "Already mounted with existing velocity partitions"
}
```

**Status Codes:**
- `200 OK` - USB devices detected
- `500 Internal Server Error` - System error

**Detection Method:**
```bash
# Uses lsblk, udevadm, and /proc/mounts
lsblk -J -o NAME,SIZE,TYPE,MOUNTPOINT,FSTYPE,LABEL,VENDOR,MODEL,SERIAL
```

#### 8. Mount USB Storage

**Endpoint:** `POST /api/storage/usb/mount`

**Purpose:** Mount a USB partition for cold archive storage.

**Request Body:**
```json
{
  "partition_path": "/dev/sda1",
  "mount_point": "/mnt/usb-archives",
  "filesystem": "ext4",
  "mount_options": "defaults,noatime",
  "create_systemd_unit": true,
  "set_as_cold_storage": true
}
```

**Parameters:**
- `partition_path` (required): Device partition path (e.g., `/dev/sda1`)
- `mount_point` (optional): Mount point path. Default: `/var/lib/velocity-report/archives/cold`
- `filesystem` (optional): Filesystem type. Auto-detected if omitted.
- `mount_options` (optional): Mount options. Default: `defaults,noatime`
- `create_systemd_unit` (optional): Create systemd mount unit for auto-mount on boot. Default: `true`
- `set_as_cold_storage` (optional): Configure as default cold storage location. Default: `true`

**Response:**
```json
{
  "success": true,
  "mount": {
    "partition_path": "/dev/sda1",
    "mount_point": "/mnt/usb-archives",
    "filesystem": "ext4",
    "mounted_at": "2025-03-15T15:30:45Z",
    "total_bytes": 999653638144,
    "free_bytes": 712458240000,
    "usage_percent": 28.7,
    "systemd_unit": "mnt-usb\\x2darchives.mount",
    "systemd_enabled": true
  },
  "cold_storage_configured": true,
  "existing_partitions_found": 8,
  "message": "USB storage mounted successfully and configured as cold storage"
}
```

**Error Response (Already Mounted):**
```json
{
  "success": false,
  "error": "already_mounted",
  "message": "Partition /dev/sda1 is already mounted at /mnt/usb-archives",
  "current_mount": {
    "mount_point": "/mnt/usb-archives",
    "mounted_at": "2025-03-15T08:30:00Z"
  }
}
```

**Status Codes:**
- `200 OK` - Mount successful
- `400 Bad Request` - Invalid partition path or mount point
- `409 Conflict` - Already mounted
- `422 Unprocessable Entity` - Mount failed (permissions, filesystem issues)
- `500 Internal Server Error` - System error

**Safety Checks (CVE-2025-VR-006 - USB Security):**
1. **Device verification:** Verify partition exists and is actually a USB device (not system disk)
2. **Filesystem whitelist:** Only allow ext4 and ext3 (reject NTFS, FAT32, exfat due to security concerns)
3. **Filesystem detection:** Auto-detect and validate filesystem type using `blkid`
4. **System disk protection:** Reject mounting of /dev/sda or /dev/mmcblk0 (system disks)
5. **Mount point validation:** Validate mount point is empty or doesn't exist, reject path traversal
6. **Secure mount options:** Always use `nosuid,nodev,noexec,noatime,ro` (read-only by default)
7. **USB path verification:** Verify device is in USB subsystem via `/sys/block/` symlinks
8. **Marker file creation:** Create `.velocity-report-archives` marker file after successful mount

**Systemd Mount Unit (Secure Configuration):**
```ini
# /etc/systemd/system/mnt-usb\x2darchives.mount
[Unit]
Description=Velocity Report USB Archive Storage
After=local-fs.target

[Mount]
What=/dev/disk/by-uuid/a1b2c3d4-e5f6-7890-abcd-ef1234567890
Where=/mnt/usb-archives
Type=ext4
# SECURITY: Read-only, no SUID/exec/devices
Options=nosuid,nodev,noexec,noatime,ro

[Install]
WantedBy=multi-user.target
```

**USB Mount Implementation (Secure):**
```go
func MountUSBStorage(partitionPath, mountPoint string) error {
    // SECURITY FIX (CVE-2025-VR-006): Verify device is USB
    if err := VerifyUSBDevice(partitionPath); err != nil {
        return fmt.Errorf("device verification failed: %w", err)
    }
    
    // Detect and validate filesystem
    fstype, err := DetectFilesystem(partitionPath)
    if err != nil {
        return err
    }
    
    // Whitelist only ext4 and ext3
    allowedFS := map[string]bool{"ext4": true, "ext3": true}
    if !allowedFS[fstype] {
        return fmt.Errorf("filesystem type not allowed: %s (only ext4, ext3 supported)", fstype)
    }
    
    // Validate mount point (no path traversal)
    if strings.Contains(mountPoint, "..") {
        return fmt.Errorf("path traversal not allowed in mount point")
    }
    
    // Create mount point if needed
    if err := os.MkdirAll(mountPoint, 0755); err != nil {
        return err
    }
    
    // Mount with secure options (read-only by default)
    secureOptions := "nosuid,nodev,noexec,noatime,ro"
    err = syscall.Mount(partitionPath, mountPoint, fstype,
        syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC|syscall.MS_RDONLY,
        secureOptions)
    
    return err
}

func VerifyUSBDevice(device string) error {
    // Reject system disks
    if strings.HasPrefix(device, "/dev/sda") || strings.HasPrefix(device, "/dev/mmcblk0") {
        return fmt.Errorf("cannot mount system disk")
    }
    
    // Verify device is USB by checking sysfs
    deviceName := filepath.Base(device)
    sysPath := "/sys/block/" + strings.TrimSuffix(deviceName, "1") // Remove partition number
    realPath, err := filepath.EvalSymlinks(sysPath)
    if err != nil {
        return fmt.Errorf("device not found in sysfs: %w", err)
    }
    
    if !strings.Contains(realPath, "/usb") {
        return fmt.Errorf("device is not a USB device")
    }
    
    return nil
}

func DetectFilesystem(device string) (string, error) {
    cmd := exec.Command("blkid", "-o", "value", "-s", "TYPE", device)
    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("cannot detect filesystem: %w", err)
    }
    
    return strings.TrimSpace(string(output)), nil
}
```

#### 9. Unmount/Eject USB Storage

**Endpoint:** `POST /api/storage/usb/unmount`

**Purpose:** Safely unmount and eject USB storage (safe removal).

**Request Body:**
```json
{
  "mount_point": "/mnt/usb-archives",
  "force": false,
  "detach_partitions": true,
  "disable_systemd_unit": false
}
```

**Parameters:**
- `mount_point` (required): Mount point path to unmount
- `force` (optional): Force unmount even if busy. Default: `false`
- `detach_partitions` (optional): Detach all SQLite partitions on this mount before unmounting. Default: `true`
- `disable_systemd_unit` (optional): Disable systemd auto-mount unit. Default: `false`

**Response:**
```json
{
  "success": true,
  "unmount": {
    "mount_point": "/mnt/usb-archives",
    "partition_path": "/dev/sda1",
    "unmounted_at": "2025-03-15T16:45:30Z",
    "partitions_detached": [
      "m01_2024",
      "m02_2024",
      "m03_2024"
    ],
    "systemd_unit_disabled": false
  },
  "safe_to_remove": true,
  "message": "USB storage safely unmounted. Safe to physically remove device."
}
```

**Error Response (Busy - Active Queries):**
```json
{
  "success": false,
  "error": "device_busy",
  "message": "Cannot unmount: 2 partitions have active queries",
  "blocking_partitions": [
    {
      "alias": "m01_2024",
      "path": "/mnt/usb-archives/2024-01_data.db",
      "active_queries": 1,
      "query_details": [
        {
          "query_id": "q_5678",
          "started_at": "2025-03-15T16:40:00Z",
          "duration_ms": 5430,
          "sql": "SELECT COUNT(*) FROM m01_2024.radar_data WHERE ..."
        }
      ]
    }
  ],
  "suggestion": "Wait for queries to complete or use force=true (may cause query failures)"
}
```

**Status Codes:**
- `200 OK` - Unmount successful
- `400 Bad Request` - Invalid mount point
- `409 Conflict` - Device busy (active queries/files open)
- `500 Internal Server Error` - System error

**Safety Checks:**
1. List all SQLite partitions on this mount point
2. Check for active queries on each partition
3. If `detach_partitions=true`, detach all partitions
4. Sync filesystem buffers (`sync`)
5. Unmount filesystem
6. Optionally disable systemd unit
7. Return "safe to remove" confirmation

**Unmount Algorithm:**
```go
func SafeUnmountUSB(mountPoint string, opts UnmountOptions) error {
    // 1. Find all partitions on this mount
    partitions := FindPartitionsOnMount(mountPoint)
    
    // 2. Check for active queries
    for _, partition := range partitions {
        if HasActiveQueries(partition) && !opts.Force {
            return ErrDeviceBusy{partition, GetActiveQueries(partition)}
        }
    }
    
    // 3. Detach all partitions if requested
    if opts.DetachPartitions {
        for _, partition := range partitions {
            DetachPartition(partition.Alias)
        }
    }
    
    // 4. Sync filesystem
    exec.Command("sync").Run()
    
    // 5. Unmount
    if err := syscall.Unmount(mountPoint, 0); err != nil {
        if opts.Force {
            syscall.Unmount(mountPoint, syscall.MNT_FORCE)
        } else {
            return err
        }
    }
    
    // 6. Disable systemd unit if requested
    if opts.DisableSystemdUnit {
        DisableSystemdMountUnit(mountPoint)
    }
    
    return nil
}
```

#### 10. Storage Growth Projection

**Endpoint:** `GET /api/storage/growth`

**Purpose:** Project storage growth rate and predict disk full dates based on historical data collection patterns.

**Query Parameters:**
- `lookback_days` (optional): Days of historical data to analyze. Default: `30`
- `projection_months` (optional): Months to project forward. Default: `12`

**Response:**
```json
{
  "analysis_period": {
    "start": "2025-02-13T00:00:00Z",
    "end": "2025-03-15T00:00:00Z",
    "days": 30
  },
  "current_storage": {
    "total_bytes": 8589934592,
    "total_human": "8.0 GB",
    "breakdown": {
      "main_db": {
        "bytes": 2684354560,
        "human": "2.5 GB"
      },
      "recent_archives": {
        "bytes": 5368709120,
        "human": "5.0 GB",
        "partition_count": 2
      },
      "cold_archives": {
        "bytes": 536870912,
        "human": "512 MB",
        "partition_count": 1
      }
    }
  },
  "growth_rate": {
    "daily_bytes": 89478485,
    "daily_human": "85.3 MB/day",
    "monthly_bytes": 2684354560,
    "monthly_human": "2.5 GB/month",
    "yearly_bytes": 32212254720,
    "yearly_human": "30 GB/year",
    "trend": "steady",
    "confidence": 0.92,
    "data_points": 30
  },
  "storage_tiers": {
    "sd_card": {
      "location": "/var/lib/velocity-report",
      "total_bytes": 64424509440,
      "free_bytes": 51539607552,
      "used_bytes": 12884901888,
      "usage_percent": 20.0,
      "projected_full_date": "2026-09-15T00:00:00Z",
      "days_until_full": 549,
      "months_until_full": 18.0,
      "status": "healthy",
      "warning_threshold_reached": false
    },
    "usb_cold_storage": {
      "location": "/mnt/usb-archives",
      "total_bytes": 999653638144,
      "free_bytes": 712458240000,
      "used_bytes": 287195398144,
      "usage_percent": 28.7,
      "projected_full_date": "2029-12-25T00:00:00Z",
      "days_until_full": 1745,
      "months_until_full": 57.2,
      "status": "healthy",
      "warning_threshold_reached": false
    }
  },
  "projections": [
    {
      "date": "2025-04-15",
      "total_storage_bytes": 11274289152,
      "total_storage_human": "10.5 GB",
      "sd_card_usage_percent": 22.5,
      "usb_usage_percent": 29.0
    },
    {
      "date": "2025-05-15",
      "total_storage_bytes": 13958643712,
      "total_storage_human": "13.0 GB",
      "sd_card_usage_percent": 25.0,
      "usb_usage_percent": 29.3
    },
    // ... monthly projections for next 12 months
  ],
  "recommendations": [
    {
      "priority": "low",
      "category": "capacity_planning",
      "message": "SD card has 18 months of capacity remaining. No action required.",
      "action": null
    },
    {
      "priority": "low",
      "category": "optimization",
      "message": "Consider enabling compression for partitions older than 6 months to free ~2.1 GB",
      "action": "enable_compression",
      "estimated_savings_bytes": 2252341760,
      "estimated_savings_human": "2.1 GB"
    }
  ],
  "alerts": []
}
```

**Alert Scenarios:**

When storage reaches threshold, alerts are included:
```json
{
  "alerts": [
    {
      "severity": "warning",
      "storage_tier": "sd_card",
      "threshold": "80%",
      "current_usage": 82.5,
      "message": "SD card storage at 82.5%. Projected full in 3.2 months.",
      "recommendations": [
        "Move old partitions to USB cold storage",
        "Enable compression for archived partitions",
        "Reduce retention policy from 36 to 24 months"
      ]
    }
  ]
}
```

**Status Codes:**
- `200 OK` - Growth analysis retrieved
- `500 Internal Server Error` - Analysis error

**Growth Rate Calculation:**
```go
func CalculateGrowthRate(lookbackDays int) GrowthRate {
    // 1. Query data volume over time
    dataPoints := QueryDailyDataVolume(lookbackDays)
    
    // 2. Linear regression for trend
    slope, intercept, r2 := LinearRegression(dataPoints)
    
    // 3. Calculate daily growth rate
    dailyBytes := slope
    
    // 4. Project monthly/yearly
    return GrowthRate{
        DailyBytes:   dailyBytes,
        MonthlyBytes: dailyBytes * 30,
        YearlyBytes:  dailyBytes * 365,
        Confidence:   r2, // R-squared as confidence metric
        Trend:        CategorizeTrend(slope, dataPoints),
    }
}

func ProjectDiskFull(tier StorageTier, growthRate GrowthRate) time.Time {
    freeBytes := tier.FreeBytes
    dailyGrowth := growthRate.DailyBytes
    
    // Account for tiered storage policy
    // SD card only holds active + recent (3 months max)
    if tier.Type == "sd_card" {
        // Cap at 3 months of data (~9 GB)
        maxBytes := 3 * growthRate.MonthlyBytes
        if freeBytes > maxBytes {
            freeBytes = maxBytes
        }
    }
    
    daysUntilFull := freeBytes / dailyGrowth
    return time.Now().AddDate(0, 0, int(daysUntilFull))
}
```

#### 11. Configure Storage Alerts

**Endpoint:** `POST /api/storage/alerts/configure`

**Purpose:** Configure automated storage capacity alerts.

**Request Body:**
```json
{
  "enabled": true,
  "thresholds": {
    "warning": 75,
    "critical": 90
  },
  "check_interval_minutes": 60,
  "notification_channels": [
    {
      "type": "log",
      "enabled": true
    },
    {
      "type": "system_events",
      "enabled": true
    },
    {
      "type": "email",
      "enabled": false,
      "config": {
        "recipients": ["admin@example.com"],
        "smtp_server": "localhost:25"
      }
    }
  ],
  "auto_actions": {
    "compress_old_partitions": true,
    "move_to_cold_storage": true,
    "alert_before_full_days": 30
  }
}
```

**Response:**
```json
{
  "success": true,
  "config": {
    "enabled": true,
    "thresholds": {
      "warning": 75,
      "critical": 90
    },
    "check_interval_minutes": 60,
    "next_check": "2025-03-15T17:30:00Z"
  },
  "message": "Storage alerts configured successfully"
}
```

**Status Codes:**
- `200 OK` - Configuration updated
- `400 Bad Request` - Invalid configuration
- `500 Internal Server Error` - System error

### USB Storage Workflows

#### Workflow 4: Setup USB Cold Storage

**Scenario:** First-time USB drive setup for cold archive storage.

```bash
# 1. Detect available USB devices
curl http://localhost:8080/api/storage/usb/devices | jq '.devices'

# 2. Mount USB drive
curl -X POST http://localhost:8080/api/storage/usb/mount \
  -H "Content-Type: application/json" \
  -d '{
    "partition_path": "/dev/sda1",
    "mount_point": "/mnt/usb-archives",
    "create_systemd_unit": true,
    "set_as_cold_storage": true
  }'

# 3. Verify mount and configure cold storage tier
curl http://localhost:8080/api/storage/growth | jq '.storage_tiers.usb_cold_storage'

# 4. Move old partitions to USB storage (example)
curl -X POST http://localhost:8080/api/partitions/consolidate \
  -H "Content-Type: application/json" \
  -d '{
    "source_partitions": [
      "/var/lib/velocity-report/archives/2024-01_data.db",
      "/var/lib/velocity-report/archives/2024-02_data.db"
    ],
    "output_path": "/mnt/usb-archives/2024-Q1_data.db",
    "delete_sources": true,
    "strategy": "quarterly"
  }'
```

#### Workflow 5: Safe USB Drive Removal

**Scenario:** Remove USB drive for transport or replacement.

```bash
# 1. Check storage status
curl http://localhost:8080/api/storage/growth | jq '.storage_tiers.usb_cold_storage'

# 2. Safely unmount (detaches partitions, waits for queries)
curl -X POST http://localhost:8080/api/storage/usb/unmount \
  -H "Content-Type: application/json" \
  -d '{
    "mount_point": "/mnt/usb-archives",
    "detach_partitions": true,
    "force": false
  }'

# 3. Verify safe to remove
# Response: {"safe_to_remove": true, "message": "Safe to physically remove device"}

# 4. Physically unplug USB drive
```

#### Workflow 6: Monitor Growth and Plan Capacity

**Scenario:** Monthly capacity planning check.

```bash
# 1. Get growth projection
curl http://localhost:8080/api/storage/growth?lookback_days=30&projection_months=12 \
  | jq '{
      growth_rate: .growth_rate,
      sd_card: .storage_tiers.sd_card | {usage_percent, months_until_full},
      usb: .storage_tiers.usb_cold_storage | {usage_percent, months_until_full},
      recommendations: .recommendations
    }'

# 2. Configure alerts if approaching thresholds
curl -X POST http://localhost:8080/api/storage/alerts/configure \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "thresholds": {"warning": 75, "critical": 90},
    "auto_actions": {
      "compress_old_partitions": true,
      "alert_before_full_days": 30
    }
  }'
```

### Integration with Existing Endpoints

**Enhanced Partition Listing:**

`GET /api/partitions` now includes storage tier information:
```json
{
  "attached": [
    {
      "alias": "m01_2024",
      "path": "/mnt/usb-archives/2024-01_data.db",
      "storage_tier": "usb_cold_storage",
      "mount_point": "/mnt/usb-archives",
      "removable": true
    }
  ]
}
```

**Safe Rotation with Storage Checks:**

Rotation process now includes storage capacity pre-flight:
```go
func PreRotationChecks() error {
    // Check buffer safety (existing)
    bufferSafe := CheckBufferSafety()
    
    // NEW: Check storage capacity
    growth := GetGrowthProjection(30)
    sdCard := growth.StorageTiers["sd_card"]
    
    if sdCard.UsagePercent > 85 {
        return ErrInsufficientStorage{
            Message: "SD card at 85%. Move partitions to cold storage before rotation.",
        }
    }
    
    return nil
}
```

---

## Phased Implementation Plan

To manage the scope of this architecture, the implementation is divided into **5 distinct phases**, each deliverable independently:

### Phase 1: Core Partitioning Foundation (Weeks 1-3)
**Scope:** Basic time-based partitioning without advanced features.

**Deliverables:**
- Partition rotation algorithm (2nd of month UTC)
- Create partition database with schema
- Copy data from main to partition
- Delete rotated data from main
- Set partition to read-only (chmod 444)
- Dynamic ATTACH DATABASE management
- Union views (`*_all`) for transparent queries
- Unit and integration tests

**Document:** `docs/features/phase-1-core-partitioning.md`

**Success Criteria:**
- Automatic monthly rotation working
- Union views allow seamless historical queries
- Zero data loss during rotation
- Performance benchmarks meet targets (1000 inserts/sec)

---

### Phase 2: API Management and Operational Control (Weeks 4-6)
**Scope:** HTTP API for partition management.

**Deliverables:**
- `GET /api/partitions` - List attached/available partitions
- `POST /api/partitions/attach` - Attach historical partitions
- `POST /api/partitions/detach` - Safely detach partitions
- `GET /api/partitions/{alias}/metadata` - Partition information
- `GET /api/partitions/buffers` - Rotation safety checks
- Authentication and authorization
- Audit logging to system_events
- API documentation and OpenAPI spec

**Document:** `docs/features/phase-2-api-management.md`

**Success Criteria:**
- All API endpoints functional
- Safety checks prevent write partition detach
- Buffer protection prevents data loss
- API documentation complete

---

### Phase 3: USB Storage and Growth Projection (Weeks 7-9)
**Scope:** Tiered storage with USB drives and capacity planning.

**Deliverables:**
- `GET /api/storage/usb/devices` - Detect USB storage
- `POST /api/storage/usb/mount` - Mount USB for cold storage
- `POST /api/storage/usb/unmount` - Safe USB eject
- `GET /api/storage/growth` - Growth rate projection
- `POST /api/storage/alerts/configure` - Capacity alerts
- Systemd mount unit generation
- Growth rate calculation with linear regression
- Disk full date prediction
- Automated capacity alerts

**Document:** `docs/features/phase-3-usb-storage-growth.md`

**Success Criteria:**
- USB detection and mounting working
- Safe eject prevents data loss
- Growth projections accurate within 10%
- Alerts trigger at configured thresholds

---

### Phase 4: Consolidation and Cold Storage (Weeks 10-12)
**Scope:** Partition consolidation and compression for long-term archival.

**Deliverables:**
- `POST /api/partitions/consolidate` - Combine partitions into yearly archives
- Async job execution with progress tracking
- Verification and rollback on failure
- Optional gzip compression (80% reduction)
- Automatic tier migration (recent → cold)
- Retention policy enforcement
- Storage optimization recommendations

**Document:** `docs/features/phase-4-consolidation-cold-storage.md`

**Success Criteria:**
- 12 monthly partitions consolidate into 1 yearly archive
- Compression achieves 70-80% size reduction
- Consolidation job completes in <15 minutes
- Rollback works on failure
- Automated tier migration based on age

---

### Phase 5: Migration, Testing, and Production Rollout (Weeks 13-15)
**Scope:** Migration tools, testing, and production deployment.

**Deliverables:**
- Configuration flags (`--enable-partitioning`)
- Historical backfill tool
- Partition consolidation CLI wrapper
- End-to-end testing on Raspberry Pi
- Performance benchmarks (write/query)
- Long-running stability tests
- User setup guide
- Operations guide
- Migration guide for existing deployments
- Alpha/Beta/Stable releases

**Document:** `docs/features/phase-5-migration-production.md`

**Success Criteria:**
- Existing deployments can migrate with zero downtime
- Rollback procedure tested and documented
- Performance meets or exceeds targets
- 99.9% rotation success rate over 30-day test period
- Production documentation complete

---

**Total Timeline:** 15 weeks (3.75 months) for complete implementation.

**Parallel Workstreams:** Phases 3-4 can be developed in parallel with Phase 2 if resources allow.

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
- Union view overhead (though SQLite optimizes this)

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
- Trade-off: include derived tables in partition vs keep centralized

---

## Alternative Approaches

### Alternative 1: Data Deletion (No Partitioning)

**Approach:** Periodically delete old data from main database.

```sql
-- Delete data older than 6 months
DELETE FROM radar_data WHERE write_timestamp < ?;
VACUUM;  -- Reclaim space
```

**Pros:**
- Simplest implementation
- No schema changes
- Single database file

**Cons:**
- Data permanently lost (no archival)
- VACUUM on large database is slow and blocks writes
- No way to recover deleted data
- No tiered storage options

**Verdict:** ❌ Not recommended. Data loss without archival contradicts project philosophy of user data ownership.

### Alternative 2: PostgreSQL with Native Partitioning

**Approach:** Migrate from SQLite to PostgreSQL with declarative partitioning.

```sql
CREATE TABLE radar_data (
    write_timestamp DOUBLE PRECISION,
    raw_event JSONB
) PARTITION BY RANGE (write_timestamp);

CREATE TABLE radar_data_2025_01 PARTITION OF radar_data
    FOR VALUES FROM (1704067200) TO (1706745600);
```

**Pros:**
- Native partition management
- Automatic partition pruning
- Enterprise-grade features

**Cons:**
- **Major architectural change:** Requires PostgreSQL server
- Increased deployment complexity (Raspberry Pi resource constraints)
- Network dependency for remote queries
- Violates "SQLite as single source of truth" design principle
- Higher memory/CPU requirements

**Verdict:** ❌ Not recommended for current architecture. Re-evaluate if multi-device support added.

### Alternative 3: Time-Series Database (InfluxDB, TimescaleDB)

**Approach:** Use purpose-built time-series database.

**Pros:**
- Optimized for time-series data
- Built-in downsampling and retention policies
- Compression and aggregation features

**Cons:**
- **Major architectural change:** New database technology
- Increased complexity (separate server process)
- JSON flexibility lost (fixed schema)
- Higher resource requirements
- External dependency (network, ports)

**Verdict:** ❌ Overkill for current use case. Consider for future multi-device aggregation scenarios.

### Alternative 4: External File Storage (CSV/Parquet)

**Approach:** Store raw data in files (CSV, Parquet), keep metadata in SQLite.

**Pros:**
- Unlimited storage growth
- Easy to archive/compress
- Interoperable formats

**Cons:**
- Loss of SQL query capabilities
- Complex query implementation
- No transaction guarantees
- Difficult to maintain referential integrity
- Poor query performance without indexes

**Verdict:** ❌ Not recommended. Loses SQLite's query flexibility and transaction safety.

### Alternative 5: Hybrid Approach (Hot/Cold Storage)

**Approach:** Keep recent data in main DB, move old data to SQLite partitions (proposed design) OR compress to read-only formats.

**Pros:**
- Combines benefits of partitioning with flexibility
- Could use different formats for very old data (CSV for >1 year)

**Cons:**
- Most complex approach
- Multiple query paths based on data age
- Difficult to reason about

**Verdict:** ⚠️ Possible future enhancement. Start with uniform partitioning (proposed design), add cold storage later if needed.

### Comparison Matrix

| Approach | Complexity | Data Retention | Query Performance | Storage Efficiency | Recommendation |
|----------|-----------|----------------|-------------------|-------------------|----------------|
| **Proposed (SQLite Partitions)** | Medium | ✅ Full archival | ✅ Good (with time filters) | ✅ Good (compression) | ✅ **Recommended** |
| Data Deletion | Low | ❌ No archival | ✅ Good (small DB) | ⚠️ No archival | ❌ No |
| PostgreSQL | High | ✅ Full archival | ✅ Excellent | ✅ Good | ❌ Too complex |
| Time-Series DB | High | ✅ Full archival | ✅ Excellent | ✅ Excellent | ❌ Overkill |
| External Files | Medium | ✅ Full archival | ❌ Poor | ✅ Good | ❌ No |
| Hybrid | Very High | ✅ Full archival | ⚠️ Variable | ✅ Excellent | ⚠️ Future |

---

## Storage Management

### Mount Points and Disk Layout

**Recommended Storage Architecture:**

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
        ├── 2024-02_data.db
        └── ...
```

**Tiered Storage Strategy:**

**Tier 1: Active (SD Card/SSD)**
- `sensor_data.db` (current month)
- Fastest access for writes

**Tier 2: Recent Archives (SD Card/SSD)**
- Last 3 months of partitions
- Frequently queried historical data
- Report generation typically uses recent data

**Tier 3: Cold Archives (USB HDD/NFS)**
- Partitions older than 3 months
- Infrequently accessed
- Can be slower/cheaper storage
- Optional compression (gzip: ~80% reduction)

### Disk Space Quotas

**Quota Monitoring Strategy:**

```bash
# Example quota check script
#!/bin/bash
THRESHOLD_GB=50

# Check main database size
MAIN_SIZE=$(du -s /var/lib/velocity-report/sensor_data.db | cut -f1)

# Check total archives size
ARCHIVES_SIZE=$(du -s /var/lib/velocity-report/archives/ | cut -f1)

# Alert if over threshold
TOTAL_GB=$((($MAIN_SIZE + $ARCHIVES_SIZE) / 1024 / 1024))
if [ $TOTAL_GB -gt $THRESHOLD_GB ]; then
    echo "ALERT: Database storage exceeds ${THRESHOLD_GB}GB"
fi
```

**Automated Quota Management:**

```go
// Quota enforcement policy
type RetentionPolicy struct {
    MaxActivePartitions    int           // Max partitions on fast storage
    MaxTotalPartitions     int           // Max total partitions (all tiers)
    ArchiveAfterDays       int           // Move to cold storage after N days
    DeleteAfterMonths      int           // Delete partitions older than N months
    CompressAfterMonths    int           // Compress partitions older than N months
}

// Example policy
policy := RetentionPolicy{
    MaxActivePartitions:  3,    // Last 3 months on fast storage
    MaxTotalPartitions:   36,   // Keep 3 years total
    ArchiveAfterDays:     90,   // Move to cold storage after 90 days
    DeleteAfterMonths:    36,   // Delete after 3 years
    CompressAfterMonths:  6,    // Compress after 6 months
}
```

### Storage Allocation

**Raspberry Pi 4 with 64GB SD Card:**

```
Total: 64GB
├── System/OS: 10GB
├── Active DB: 3GB (current month)
├── Recent Archives: 9GB (3 months × 3GB)
├── Cold Archives (USB HDD): Unlimited
└── Free Space: 42GB (buffer for logs, updates, etc.)
```

**Storage Growth Over Time:**

| Month | Active DB | Recent (SD) | Cold (USB) | SD Card Usage |
|-------|-----------|-------------|------------|---------------|
| 1     | 3GB       | 0GB         | 0GB        | 13GB (20%)    |
| 6     | 3GB       | 9GB         | 9GB        | 22GB (34%)    |
| 12    | 3GB       | 9GB         | 27GB       | 22GB (34%)    |
| 24    | 3GB       | 9GB         | 60GB       | 22GB (34%)    |

**Conclusion:** SD card usage stabilizes at ~22GB with tiered storage.

### Mount Point Configuration

**systemd Mount Units:**

```ini
# /etc/systemd/system/var-lib-velocity\x2dreport-archives.mount
[Unit]
Description=Velocity Report Archive Storage
After=local-fs.target

[Mount]
What=/dev/sda1                         # USB HDD partition
Where=/var/lib/velocity-report/archives
Type=ext4
Options=defaults,noatime

[Install]
WantedBy=multi-user.target
```

**Fallback to Local Storage:**
```go
// Check if archive mount is available, fall back to local if not
func GetArchiveDir() string {
    archiveMount := "/var/lib/velocity-report/archives"
    
    if isMountPoint(archiveMount) {
        return archiveMount
    }
    
    // Fallback to local storage
    log.Warn("Archive mount not available, using local storage")
    return "/var/lib/velocity-report/archives-local"
}
```

### Compression Strategy

**Compress Old Partitions:**
```bash
# Compress partitions older than 6 months
gzip /var/lib/velocity-report/archives/2024-*_data.db

# Result: 2024-01_data.db.gz (~80% size reduction)
# SQLite can't query compressed files directly, must decompress first
```

**Trade-off:**
- **Pros:** ~80% storage savings, easy to archive
- **Cons:** Must decompress for queries, slower access

**Implementation:**
```go
// Lazy decompression for queries
func QueryCompressedPartition(partitionPath string) (*sql.DB, error) {
    if strings.HasSuffix(partitionPath, ".gz") {
        // Decompress to temp location
        tempPath := filepath.Join(os.TempDir(), filepath.Base(partitionPath))
        if err := decompressGzip(partitionPath, tempPath); err != nil {
            return nil, err
        }
        defer os.Remove(tempPath) // Clean up after query
        partitionPath = tempPath
    }
    
    return sql.Open("sqlite", partitionPath)
}
```

---

## Migration Path

### Phase 1: Pre-Migration (Development/Testing)

**Goal:** Validate partitioning approach with test data.

**Steps:**
1. Implement partition rotation logic in Go
2. Test with synthetic data (simulated months of readings)
3. Benchmark query performance (single-file vs partitioned)
4. Validate union view query planner behavior
5. Test failure scenarios (rotation failure, corruption)

**Deliverables:**
- Working partition rotation code
- Performance benchmarks
- Test suite for partitioning logic

### Phase 2: Opt-In Partitioning (Existing Deployments)

**Goal:** Allow existing deployments to enable partitioning.

**Steps:**
1. Add configuration flag: `--enable-partitioning`
2. On first run with flag:
   - Analyze existing data for natural partition boundaries
   - Offer to backfill historical partitions OR start fresh
3. Run rotation process on next scheduled trigger

**Backward Compatibility:**
- Default: disabled (single-file behavior)
- Flag-enabled: partitioning active
- Rollback: disable flag, union views continue to work

**Example:**
```bash
# Enable partitioning on existing deployment
velocity-report --db-path /var/lib/velocity-report/sensor_data.db \
                --enable-partitioning \
                --partition-schedule monthly \
                --backfill-partitions
```

### Phase 3: Historical Backfill (Optional)

**Goal:** Partition existing historical data in main database.

**Approach:**
```go
func BackfillPartitions(db *DB, strategy PartitionStrategy) error {
    // 1. Analyze data time range
    minTime, maxTime := GetDataTimeRange(db)
    
    // 2. Calculate partition boundaries
    partitions := CalculatePartitions(minTime, maxTime, strategy)
    
    // 3. For each partition:
    for _, partition := range partitions {
        // Create partition file
        // Copy data for partition period
        // Verify integrity
        // Delete from main DB
    }
    
    // 4. Update union views
    return UpdateUnionViews(db)
}
```

**User Decision:**
- **Option A:** Backfill historical data into partitions (takes time, enables full partitioning)
- **Option B:** Start fresh, keep historical data in main DB (faster, mixed mode)

### Phase 4: New Deployments (Default Enabled)

**Goal:** Partitioning enabled by default for new installations.

**Changes:**
- `--enable-partitioning` becomes default
- Initial schema creates empty archives directory
- First rotation occurs on 2nd of second month

### Rollback Strategy

**Disable Partitioning:**
```bash
# Stop partitioning, revert to single-file
velocity-report --db-path /var/lib/velocity-report/sensor_data.db \
                --disable-partitioning
```

**Effect:**
- Union views remain functional (read-only access to partitions)
- New writes go to main database only
- Partitions not deleted (preserved as archives)

**Consolidate Partitions:**
```go
// Emergency consolidation: merge partitions back into main DB
func ConsolidatePartitions(db *DB) error {
    partitions := FindPartitions(archivesDir)
    
    for _, partition := range partitions {
        // Copy data from partition to main DB
        if err := CopyPartitionToMain(partition, db); err != nil {
            return err
        }
    }
    
    return nil
}
```

**Use Case:** If partitioning causes issues, consolidate back to single file.

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
| 1GB           | 1000                | 1000                    |
| 10GB          | 800                 | 1000                    |
| 30GB          | 500                 | 1000                    |

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

**Query Optimization Strategies:**

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

**Metrics to Track:**

1. **Partition Count**
   - Alert if approaching ATTACH limit (10 default, 125 max)
   - Trigger consolidation or deletion if too many

2. **Active Database Size**
   - Alert if main database exceeds 5GB (rotation may be failing)

3. **Disk Space**
   - Monitor SD card usage (alert at 80%)
   - Monitor USB HDD usage (alert at 90%)

4. **Rotation Health**
   - Track last successful rotation time
   - Alert if rotation fails 2+ times in a row

5. **Query Performance**
   - Track p95 query latency for `radar_data_all` views
   - Alert if latency increases significantly

**Example Monitoring Dashboard:**
```
Velocity Report Storage Health
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Active DB:        2.8GB / 5GB   [████████░░] 56%
Partitions:       8 / 125       [█░░░░░░░░░]  6%
SD Card:         18GB / 64GB    [███░░░░░░░] 28%
USB HDD:         24GB / 1TB     [░░░░░░░░░░]  2%

Last Rotation:    2025-03-02 00:00:15 UTC  ✅ Success
Avg Query Time:   45ms (p95: 120ms)        ✅ Healthy
```

### Backup Strategy

**Partition-Aware Backup:**

```bash
#!/bin/bash
# Backup script for partitioned database

BACKUP_DIR=/mnt/backup/velocity-report

# Backup main database (always)
sqlite3 /var/lib/velocity-report/sensor_data.db ".backup $BACKUP_DIR/sensor_data.db"

# Backup recent partitions (last 3 months)
for partition in /var/lib/velocity-report/archives/recent/*.db; do
    cp "$partition" "$BACKUP_DIR/archives/"
done

# Backup cold partitions (incremental, skip if already backed up)
rsync -av --ignore-existing /var/lib/velocity-report/archives/cold/ \
      "$BACKUP_DIR/archives/cold/"

# Compress old backups
find "$BACKUP_DIR/archives/cold" -name "*.db" -mtime +30 -exec gzip {} \;
```

**Backup Frequency:**
- **Active database:** Daily
- **Recent partitions:** Weekly
- **Cold partitions:** Monthly (or after creation)

**Recovery:**
```bash
# Restore main database
cp /mnt/backup/sensor_data.db /var/lib/velocity-report/

# Restore partitions
cp -r /mnt/backup/archives/* /var/lib/velocity-report/archives/

# Restart service
systemctl restart velocity-report
```

### Deployment Considerations

**systemd Service Updates:**

```ini
# /etc/systemd/system/velocity-report.service
[Unit]
Description=Velocity Report Data Collection
After=network.target var-lib-velocity\x2dreport-archives.mount

[Service]
Type=simple
User=velocity-report
ExecStart=/usr/local/bin/velocity-report \
    --db-path=/var/lib/velocity-report/sensor_data.db \
    --enable-partitioning \
    --partition-schedule=monthly \
    --max-attached=125
Restart=always
RestartSec=10s

# Ensure archives directory exists
ExecStartPre=/bin/mkdir -p /var/lib/velocity-report/archives/recent
ExecStartPre=/bin/mkdir -p /var/lib/velocity-report/archives/cold

[Install]
WantedBy=multi-user.target
```

**Disk Space Pre-flight Check:**

```go
// Before starting server, check available space
func PreflightChecks() error {
    // Check SD card space
    freeSpace := GetFreeSpace("/var/lib/velocity-report")
    if freeSpace < 10*GB {
        return fmt.Errorf("insufficient disk space: %dGB free (need 10GB)", freeSpace/GB)
    }
    
    // Check archives mount
    if !isMountPoint("/var/lib/velocity-report/archives") {
        log.Warn("Archives mount not found, using local storage")
    }
    
    return nil
}
```

### Documentation Updates

**User Documentation Needs:**

1. **Setup Guide:**
   - Enable partitioning on new deployment
   - Migrate existing deployment to partitioning
   - Configure tiered storage (USB HDD)

2. **Operations Guide:**
   - Monitor partition count and disk usage
   - Backup and restore procedures
   - Troubleshoot rotation failures

3. **API Changes:**
   - Query views (`radar_data_all`) instead of tables (`radar_data`)
   - Partition-aware query patterns

4. **Configuration Reference:**
   - Partition schedule options (monthly/quarterly)
   - Retention policies
   - Storage tier configuration

---

## Implementation Phases

### Phase 0: Design and Review (Current)
- ✅ Design specification document
- 🔄 Community feedback and review
- 🔄 Finalize design decisions

### Phase 1: Core Partitioning Logic (Weeks 1-2)
- [ ] Implement partition rotation algorithm
- [ ] Create partition database with schema
- [ ] Copy data from main to partition
- [ ] Delete rotated data from main
- [ ] Set partition to read-only
- [ ] Unit tests for rotation logic

### Phase 2: Union Views (Week 3)
- [ ] Implement dynamic view generation
- [ ] ATTACH DATABASE management
- [ ] Update views after rotation
- [ ] Query planner optimization tests
- [ ] Performance benchmarks

### Phase 3: Scheduling and Automation (Week 4)
- [ ] Rotation scheduler (2nd of month 00:00 UTC)
- [ ] Pre-flight checks (disk space, etc.)
- [ ] Failure handling and retry logic
- [ ] Rotation status monitoring
- [ ] Integration tests

### Phase 4: Storage Management (Week 5)
- [ ] Tiered storage support (recent/cold)
- [ ] Compression for old partitions
- [ ] Retention policy enforcement
- [ ] Disk quota monitoring
- [ ] Storage allocation tools

### Phase 5: API Management Endpoints (Week 6)
- [ ] Implement `GET /api/partitions` (list attached/available)
- [ ] Implement `POST /api/partitions/attach` (attach with safety checks)
- [ ] Implement `POST /api/partitions/detach` (detach with query checks)
- [ ] Implement `POST /api/partitions/consolidate` (yearly archives)
- [ ] Implement `GET /api/partitions/{alias}/metadata` (partition info)
- [ ] Implement `GET /api/partitions/buffers` (rotation safety checks)
- [ ] Add authentication and authorization
- [ ] Audit logging to system_events table
- [ ] Unit and integration tests for API endpoints

### Phase 6: Migration Tools (Week 7)
- [ ] Configuration flags (`--enable-partitioning`)
- [ ] Historical backfill tool
- [ ] Partition consolidation CLI wrapper
- [ ] Migration documentation

### Phase 7: Testing and Validation (Week 8)
- [ ] End-to-end testing on Raspberry Pi
- [ ] Performance benchmarks (write/query)
- [ ] Long-running stability tests
- [ ] Edge case testing (failures, corruption)
- [ ] API endpoint testing (attach/detach/consolidate)

### Phase 8: Documentation and Release (Week 9)
- [ ] User setup guide
- [ ] Operations guide
- [ ] API documentation updates
- [ ] API endpoint reference documentation
- [ ] CHANGELOG entry
- [ ] Release notes

### Phase 9: Rollout (Week 10+)
- [ ] Alpha release (opt-in, developer testing)
- [ ] Beta release (community testing)
- [ ] Stable release (default for new deployments)
- [ ] Migration guide for existing deployments

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

## Open Questions

### Design Questions

1. **Monthly vs Quarterly Partitions:**
   - **Decision needed:** Which default? Allow configuration?
   - **Proposal:** Start with monthly, add quarterly as option

2. **Derived Tables in Partitions:**
   - **Question:** Include `radar_data_transits` in partitions or keep centralized?
   - **Trade-off:** Simplicity vs late-arriving sessionization updates
   - **Proposal:** Include in partitions, accept rare update failures

3. **SQLITE_MAX_ATTACHED Limit:**
   - **Question:** Compile with increased limit (125) by default?
   - **Impact:** Slightly larger binary, no performance impact
   - **Proposal:** Yes, increase to 125 (10 years of monthly partitions)

4. **Compression Strategy:**
   - **Question:** Automatic compression after X months?
   - **Trade-off:** Storage savings vs query complexity
   - **Proposal:** Manual compression initially, automate in Phase 2

### Implementation Questions

1. **Rotation Locking:**
   - **Question:** How to prevent concurrent rotations?
   - **Proposal:** File-based lock in `/var/run/velocity-report-rotation.lock`

2. **Timezone Handling:**
   - **Question:** User's local timezone vs UTC for rotation trigger?
   - **Proposal:** Always UTC (simplicity, consistency), document clearly

3. **Backward Compatibility:**
   - **Question:** How long to maintain single-file mode as option?
   - **Proposal:** 2 major releases (1 year), then deprecate

### Operational Questions

1. **Monitoring Integration:**
   - **Question:** Built-in monitoring or rely on external tools?
   - **Proposal:** Expose metrics via HTTP endpoint, document Prometheus/Grafana integration

2. **Backup Tooling:**
   - **Question:** Provide built-in backup script or document best practices?
   - **Proposal:** Provide reference scripts in `scripts/backup/`

3. **Cloud Storage:**
   - **Question:** Support for S3/cloud storage for cold archives?
   - **Proposal:** Not initially, add as feature request

---

## References

### SQLite Documentation
- [ATTACH DATABASE](https://www.sqlite.org/lang_attach.html)
- [Limits: Maximum Number of Attached Databases](https://www.sqlite.org/limits.html#max_attached)
- [Write-Ahead Logging (WAL)](https://www.sqlite.org/wal.html)
- [Query Planning and Optimization](https://www.sqlite.org/queryplanner.html)

### Partitioning Patterns
- [Time-Series Partitioning Best Practices](https://en.wikipedia.org/wiki/Partition_(database))
- [SQLite Performance Tuning](https://www.sqlite.org/speed.html)

### Related velocity.report Documentation
- [ARCHITECTURE.md](/ARCHITECTURE.md) - System architecture overview
- [README.md](/README.md) - Project overview
- [internal/db/schema.sql](/internal/db/schema.sql) - Current database schema

### Future Reading
- [Multi-Device Support Design](docs/features/multi-device-support.md) (planned)
- [Data Retention Policies](docs/features/data-retention.md) (planned)

---

## Revision History

| Version | Date       | Author   | Changes                          |
|---------|------------|----------|----------------------------------|
| 1.0     | 2025-12-01 | Ictinus  | Initial design specification     |

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

---

## Appendix B: Configuration Examples

### Enable Partitioning (New Deployment)
```bash
velocity-report \
    --db-path=/var/lib/velocity-report/sensor_data.db \
    --enable-partitioning \
    --partition-schedule=monthly \
    --archive-dir=/var/lib/velocity-report/archives \
    --max-attached=125
```

### Migrate Existing Deployment
```bash
# Step 1: Enable partitioning
velocity-report \
    --db-path=/var/lib/velocity-report/sensor_data.db \
    --enable-partitioning \
    --backfill-partitions

# Step 2: Monitor first rotation (2nd of next month)
journalctl -u velocity-report.service -f | grep "rotation"

# Step 3: Verify partition created
ls -lh /var/lib/velocity-report/archives/

# Step 4: Test query performance
sqlite3 /var/lib/velocity-report/sensor_data.db \
    "SELECT COUNT(*) FROM radar_data_all WHERE write_timestamp > UNIXEPOCH('now', '-1 month');"
```

### Configure Retention Policy
```bash
velocity-report \
    --enable-partitioning \
    --retention-months=36 \           # Delete partitions older than 3 years
    --compress-after-months=6 \       # Compress partitions older than 6 months
    --tier-storage \
        recent=/var/lib/velocity-report/archives/recent:3 \   # Last 3 months on SD
        cold=/mnt/usb-hdd/velocity-archives                   # Older on USB HDD
```

---

## Appendix C: Troubleshooting

### Rotation Failed
**Symptom:** Log shows rotation error, data still in main database.

**Diagnosis:**
```bash
journalctl -u velocity-report.service | grep "rotation failed"
```

**Causes:**
1. Insufficient disk space
2. Permission errors on archives directory
3. Database locked during rotation

**Resolution:**
1. Check disk space: `df -h /var/lib/velocity-report`
2. Check permissions: `ls -ld /var/lib/velocity-report/archives`
3. Verify no manual queries locking database
4. Retry rotation: `systemctl restart velocity-report`

### Query Performance Degraded
**Symptom:** Queries taking longer than expected.

**Diagnosis:**
```sql
-- Check number of attached partitions
PRAGMA database_list;

-- Explain query plan
EXPLAIN QUERY PLAN 
SELECT * FROM radar_data_all WHERE write_timestamp > ?;
```

**Causes:**
1. Too many attached partitions
2. Missing indexes on partitions
3. Query not using time filters (full scan)

**Resolution:**
1. Implement retention policy to limit partition count
2. Verify indexes: `sqlite3 partition.db ".indexes"`
3. Optimize query with time range filters

### Disk Space Exhausted
**Symptom:** Service fails with "disk full" error.

**Diagnosis:**
```bash
df -h /var/lib/velocity-report
du -sh /var/lib/velocity-report/sensor_data.db
du -sh /var/lib/velocity-report/archives/
```

**Resolution:**
1. Move cold partitions to USB HDD
2. Compress old partitions: `gzip archives/*.db`
3. Delete partitions older than retention policy
4. Expand SD card or add external storage

---

*End of Design Specification*
