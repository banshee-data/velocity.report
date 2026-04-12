# Transit deduplication plan

Documents the algorithm and safeguards for preventing duplicate transit records when overlapping cron runs or backfill operations process the same radar data windows.

## Problem statement

The transit worker can create duplicate transit records when:

1. Hourly cron runs overlap with full backfill runs
2. Multiple hourly runs process the same data due to the lookback window
3. Different model versions (e.g., "hourly-cron" vs "rebuild-full") are used

This can lead to double-counting of transits in statistics and reports.

## Current behaviour

### Transit worker ([internal/db/transit_worker.go](../../../internal/db/transit_worker.go))

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

## Proposed solution

### Strategy: delete-before-insert with model version priority

**Priority Order:**

1. Full backfill runs ("rebuild-full") take precedence over hourly runs
2. Newer hourly runs ("hourly-cron") replace older overlapping hourly data
3. When switching between model versions, explicitly remove old version data

### Implementation plan

#### Phase 1: pre-processing step in `RunRange()`

Before processing transits, delete existing transits with the same `model_version` that overlap the target time range. Uses a three-way overlap check (starts-in, ends-in, spans-entire) within a transaction. See [internal/db/transit_worker.go](../../../internal/db/transit_worker.go) `RunRange()`.

#### Phase 2: model version migration

`MigrateModelVersion()` deletes all transits with the old model version, then re-runs the worker over the full history with the new version. See [internal/db/transit_worker.go](../../../internal/db/transit_worker.go).

#### Phase 3: CLI commands

Added `--cleanup-transits`, `--migrate-transits-from`, and `--migrate-transits-to` flags to `cmd/radar/main.go`.

#### Phase 4: overlap analysis

`AnalyseTransitOverlaps()` uses a self-join on `radar_data_transits` to find cross-model-version overlapping transit pairs. Groups results by model version pair with counts and example timestamps.

## Migration path

### Step 1: deploy code with deduplication logic (non-breaking)

- Deploy new code with delete-before-insert in `RunRange()`
- This immediately prevents new duplicates from hourly runs
- Existing duplicates remain (cleaned up in Step 2)

### Step 2: one-time cleanup of existing duplicates

```bash
# Analyse current state
velocity-report analyse-transits

# Option A: Keep hourly-cron, remove any rebuild-full duplicates
sqlite3 sensor_data.db "DELETE FROM radar_data_transits WHERE model_version = 'rebuild-full'"

# Option B: Migrate from hourly-cron to rebuild-full (re-process all)
velocity-report --migrate-transits-from hourly-cron --migrate-transits-to rebuild-full
```

### Step 3: ongoing operations

- Hourly cron runs automatically deduplicate their own overlaps
- Full backfill runs (if ever needed) require manual model version migration

## Testing strategy

1. **Unit tests** (`internal/db/transit_worker_test.go`): overlapping window handling, same-model deduplication, cross-model scenarios.
2. **Integration test**: insert sample `radar_data` spanning 2 hours, run worker with 1 h window twice (overlapping), verify no duplicate transits and all data points linked exactly once.
3. **Manual verification**: self-join query on `radar_data_transits` checking for overlapping start/end timestamps across different transit IDs.

## Database impact

**Performance Considerations:**

- Delete query adds overhead to each worker run (~10-50ms for typical hourly data)
- Uses existing `idx_transits_time` index for efficient range deletion
- Cascade delete via foreign key automatically cleans up `radar_transit_links`

**Disk Space:**

- Initial cleanup may free significant space if duplicates exist
- Ongoing operations maintain single copy per time range

## Resolved design questions

| Question                                      | Resolution                                                                                                                 |
| --------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| Support multiple model versions concurrently? | No. Use the migration path (`MigrateModelVersion`) for algorithm changes. A/B testing would complicate statistics queries. |
| Grace period for overlapping windows?         | Keep the 5 min overlap (1 h interval, 1 h 5 min window). Deduplication handles late-arriving data.                         |
| Should hourly runs remove backfill data?      | No. Backfills are rare and intentional: use the explicit migration CLI.                                                    |

## Implementation priority

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
