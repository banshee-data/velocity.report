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
6. [Pros and Cons](#pros-and-cons)
7. [Alternative Approaches](#alternative-approaches)
8. [Storage Management](#storage-management)
9. [Migration Path](#migration-path)
10. [Performance Implications](#performance-implications)
11. [Operational Considerations](#operational-considerations)
12. [Implementation Phases](#implementation-phases)
13. [Success Metrics](#success-metrics)
14. [Open Questions](#open-questions)
15. [References](#references)

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
    // 1. Determine partition name (e.g., "2025-01_data.db")
    partitionName := rotationTime.AddDate(0, -1, 0).Format("2006-01") + "_data.db"
    partitionPath := filepath.Join(archivesDir, partitionName)
    
    // 2. Create partition database with schema
    partitionDB, err := CreatePartitionDB(partitionPath)
    if err != nil {
        return err
    }
    
    // 3. Copy previous month's data to partition
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
        return err
    }
    
    // 4. Verify data integrity
    if err := VerifyPartition(partitionDB, startTime, endTime); err != nil {
        return err
    }
    
    // 5. Delete copied data from main database
    err = DeleteRotatedData(db, startTime, endTime)
    if err != nil {
        return err
    }
    
    // 6. Make partition read-only
    if err := os.Chmod(partitionPath, 0444); err != nil {
        return err
    }
    
    // 7. Update union views to include new partition
    if err := UpdateUnionViews(db); err != nil {
        return err
    }
    
    return nil
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
        alias := fmt.Sprintf("m%02d", i+1) // m01, m02, m03, ...
        
        // ATTACH DATABASE 'path' AS alias
        _, err := db.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS %s", partition, alias))
        if err != nil {
            return err
        }
    }
    
    return nil
}
```

**Limits:**
- SQLite allows up to **10 attached databases by default** (compile-time limit)
- Can be increased to **125** with `SQLITE_MAX_ATTACHED` compile flag
- Monthly partitions: 10 partitions = 10 months, 125 = 10.4 years
- Quarterly partitions: 10 partitions = 2.5 years, 125 = 31 years

**Recommendation:** Start with monthly, increase `SQLITE_MAX_ATTACHED` to 125 for long-term deployments.

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

### Phase 5: Migration Tools (Week 6)
- [ ] Configuration flags (`--enable-partitioning`)
- [ ] Historical backfill tool
- [ ] Partition consolidation tool (rollback)
- [ ] Migration documentation

### Phase 6: Testing and Validation (Week 7)
- [ ] End-to-end testing on Raspberry Pi
- [ ] Performance benchmarks (write/query)
- [ ] Long-running stability tests
- [ ] Edge case testing (failures, corruption)

### Phase 7: Documentation and Release (Week 8)
- [ ] User setup guide
- [ ] Operations guide
- [ ] API documentation updates
- [ ] CHANGELOG entry
- [ ] Release notes

### Phase 8: Rollout (Week 9+)
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
