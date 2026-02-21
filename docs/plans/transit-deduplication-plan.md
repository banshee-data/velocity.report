# Transit Deduplication Plan

Status: Planned

## Problem Statement

The transit worker can create duplicate transit records when:

1. Hourly cron runs overlap with full backfill runs
2. Multiple hourly runs process the same data due to the lookback window
3. Different model versions (e.g., "hourly-cron" vs "rebuild-full") are used

This can lead to double-counting of transits in statistics and reports.

## Current Behaviour

### Transit Worker (`internal/db/transit_worker.go`)

**Data Model:**

- `radar_data_transits` table stores sessionised vehicle transits
- Each transit has a unique `transit_key` (SHA1 hash of start_time, threshold, model_version)
- `model_version` field identifies which run created the transit (e.g., "hourly-cron", "rebuild-full")
- Transits are linked to raw radar_data via `radar_transit_links` table

**Current Upsert Logic:**

```sql
INSERT INTO radar_data_transits (...)
VALUES (...)
ON CONFLICT(transit_key) DO UPDATE SET
  transit_end_unix = excluded.transit_end_unix,
  ...
```

**Issues:**

1. Same transit can exist with different model_versions (different keys)
2. No cleanup of older/overlapping transits before inserting new ones
3. Lookback window (1h5m for 1h interval) creates intentional overlap but doesn't deduplicate

## Proposed Solution

### Strategy: Delete-Before-Insert with Model Version Priority

**Priority Order:**

1. Full backfill runs ("rebuild-full") take precedence over hourly runs
2. Newer hourly runs ("hourly-cron") replace older overlapping hourly data
3. When switching between model versions, explicitly remove old version data

### Implementation Plan

#### Phase 1: Add Pre-Processing Step to `RunRange()`

Before processing transits in a time range, delete existing transits that would overlap:

```go
func (w *TransitWorker) RunRange(ctx context.Context, start, end float64) error {
    tx, err := w.DB.BeginTx(ctx, &sql.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // NEW: Delete overlapping transits with the same model_version
    // This handles hourly re-runs and window overlaps
    deleteQuery := `
        DELETE FROM radar_data_transits
        WHERE model_version = ?
          AND (
              (transit_start_unix BETWEEN ? AND ?)
              OR (transit_end_unix BETWEEN ? AND ?)
              OR (transit_start_unix <= ? AND transit_end_unix >= ?)
          )
    `
    result, err := tx.ExecContext(ctx, deleteQuery,
        w.ModelVersion,
        start, end,  // transit starts in range
        start, end,  // transit ends in range
        start, end,  // transit spans entire range
    )
    if err != nil {
        return fmt.Errorf("failed to delete overlapping transits: %w", err)
    }

    deleted, _ := result.RowsAffected()
    if deleted > 0 {
        log.Printf("Transit worker: deleted %d overlapping %s transits in range [%v, %v]",
            deleted, w.ModelVersion, start, end)
    }

    // Continue with existing clustering logic...
}
```

#### Phase 2: Add Model Version Migration Support

Add a function to switch between model versions cleanly:

```go
// MigrateModelVersion replaces all transits from oldVersion with newVersion
// by re-running the transit worker over the entire historical range.
func (w *TransitWorker) MigrateModelVersion(ctx context.Context, oldVersion string) error {
    if oldVersion == w.ModelVersion {
        return fmt.Errorf("old and new model versions must differ")
    }

    log.Printf("Transit worker: migrating from %s to %s", oldVersion, w.ModelVersion)

    // Delete all old version transits
    result, err := w.DB.ExecContext(ctx,
        `DELETE FROM radar_data_transits WHERE model_version = ?`,
        oldVersion,
    )
    if err != nil {
        return fmt.Errorf("failed to delete old version transits: %w", err)
    }

    deleted, _ := result.RowsAffected()
    log.Printf("Transit worker: deleted %d %s transits", deleted, oldVersion)

    // Re-run over full history with new version
    return w.RunFullHistory(ctx)
}
```

#### Phase 3: Add CLI Command for Manual Cleanup

Add command in `cmd/radar/main.go`:

```go
// Add flags:
var (
    cleanupTransits    bool
    migrateModelFrom   string
    migrateModelTo     string
)

// In init():
flag.BoolVar(&cleanupTransits, "cleanup-transits", false,
    "Remove duplicate transits across all model versions (keeps newest)")
flag.StringVar(&migrateModelFrom, "migrate-transits-from", "",
    "Migrate transits from this model version")
flag.StringVar(&migrateModelTo, "migrate-transits-to", "",
    "Migrate transits to this model version")

// In main():
if cleanupTransits {
    if err := db.CleanupDuplicateTransits(); err != nil {
        log.Fatalf("Failed to cleanup transits: %v", err)
    }
    log.Println("Transit cleanup complete")
    return
}

if migrateModelFrom != "" {
    worker := db.NewTransitWorker(db, transitWorkerThreshold, migrateModelTo)
    worker.Interval = transitWorkerInterval
    worker.Window = transitWorkerWindow
    if err := worker.MigrateModelVersion(context.Background(), migrateModelFrom); err != nil {
        log.Fatalf("Failed to migrate model version: %v", err)
    }
    log.Println("Model version migration complete")
    return
}
```

#### Phase 4: Add Deduplication Analysis Function

For debugging and monitoring:

```go
// AnalyseTransitOverlaps returns statistics about overlapping transits
func (db *DB) AnalyseTransitOverlaps() (*TransitOverlapStats, error) {
    type overlap struct {
        ModelVersion1 string
        ModelVersion2 string
        Count        int64
        ExampleStart float64
        ExampleEnd   float64
    }

    query := `
        WITH overlaps AS (
            SELECT
                t1.model_version as mv1,
                t2.model_version as mv2,
                t1.transit_start_unix as start1,
                t1.transit_end_unix as end1,
                t2.transit_start_unix as start2,
                t2.transit_end_unix as end2
            FROM radar_data_transits t1
            JOIN radar_data_transits t2
                ON t1.transit_id < t2.transit_id
                AND t1.model_version != t2.model_version
                AND (
                    (t1.transit_start_unix BETWEEN t2.transit_start_unix AND t2.transit_end_unix)
                    OR (t1.transit_end_unix BETWEEN t2.transit_start_unix AND t2.transit_end_unix)
                    OR (t1.transit_start_unix <= t2.transit_start_unix
                        AND t1.transit_end_unix >= t2.transit_end_unix)
                )
        )
        SELECT mv1, mv2, COUNT(*), MIN(start1), MIN(end1)
        FROM overlaps
        GROUP BY mv1, mv2
    `

    // Execute query and return results...
}
```

## Migration Path

### Step 1: Deploy Code with Deduplication Logic (Non-Breaking)

- Deploy new code with delete-before-insert in `RunRange()`
- This immediately prevents new duplicates from hourly runs
- Existing duplicates remain (cleaned up in Step 2)

### Step 2: One-Time Cleanup of Existing Duplicates

```bash
# Analyse current state
velocity-report analyse-transits

# Option A: Keep hourly-cron, remove any rebuild-full duplicates
sqlite3 sensor_data.db "DELETE FROM radar_data_transits WHERE model_version = 'rebuild-full'"

# Option B: Migrate from hourly-cron to rebuild-full (re-process all)
velocity-report --migrate-transits-from hourly-cron --migrate-transits-to rebuild-full
```

### Step 3: Ongoing Operations

- Hourly cron runs automatically deduplicate their own overlaps
- Full backfill runs (if ever needed) require manual model version migration

## Testing Strategy

1. **Unit Tests** (`internal/db/transit_worker_test.go`):
   - Test overlapping window handling
   - Test same model version deduplication
   - Test cross-model version scenarios

2. **Integration Test**:

   ```go
   func TestTransitWorker_Deduplication(t *testing.T) {
       // Insert sample radar_data spanning 2 hours
       // Run worker with 1h window twice (overlapping)
       // Verify no duplicate transits exist
       // Verify all radar_data points are linked exactly once
   }
   ```

3. **Manual Verification**:
   ```sql
   -- Check for overlapping transits
   SELECT
       t1.transit_id as id1,
       t1.model_version as mv1,
       t1.transit_start_unix as start1,
       t2.transit_id as id2,
       t2.model_version as mv2,
       t2.transit_start_unix as start2
   FROM radar_data_transits t1
   JOIN radar_data_transits t2
       ON t1.transit_id < t2.transit_id
       AND t1.transit_start_unix < t2.transit_end_unix
       AND t1.transit_end_unix > t2.transit_start_unix
   LIMIT 10;
   ```

## Database Impact

**Performance Considerations:**

- Delete query adds overhead to each worker run (~10-50ms for typical hourly data)
- Uses existing `idx_transits_time` index for efficient range deletion
- Cascade delete via foreign key automatically cleans up `radar_transit_links`

**Disk Space:**

- Initial cleanup may free significant space if duplicates exist
- Ongoing operations maintain single copy per time range

## Open Questions

1. **Should we support multiple model versions concurrently?**
   - Pro: Allows A/B testing of different clustering algorithms
   - Con: Complicates statistics queries (which version to use?)
   - **Recommendation**: No. Use migration path for algorithm changes.

2. **What's the grace period for overlapping windows?**
   - Current: 1h interval with 1h5m window = 5m overlap
   - Overlap is intentional to catch late-arriving data
   - **Recommendation**: Keep current; deduplication handles it.

3. **Should hourly runs also check for and remove backfill data?**
   - This would allow hourly to "override" a previous full backfill
   - **Recommendation**: No. Backfills are rare and should be intentional migrations.

## Implementation Priority

1. ✅ **COMPLETED**: Fix model_version default to "hourly-cron" (already done)
2. ✅ **COMPLETED**: Implement delete-before-insert in `RunRange()` (Phase 1)
3. ✅ **COMPLETED**: Add CLI command for manual cleanup (Phase 3)
4. ✅ **COMPLETED**: Add model version migration support (Phase 2)
5. ✅ **COMPLETED**: Add overlap analysis function (Phase 4)

## Summary

The proposed solution ensures:

- ✅ No duplicate transits from overlapping hourly runs
- ✅ Clean model version transitions
- ✅ Minimal performance impact
- ✅ Backward compatible (existing data unaffected until cleanup)
- ✅ Observable via SQL queries and CLI tools
