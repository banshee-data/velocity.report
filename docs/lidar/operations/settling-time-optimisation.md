# LiDAR Background Settling Time Optimisation

- **Status:** Phase 3 Complete (February 2026)

This document proposes two complementary approaches to address the loss of ~30 seconds of data at the start of PCAP file analysis due to the LiDAR background regions settling period.

## Implementation Summary

- ✅ Phase 1: Background Grid Restoration - Not implemented (regions-only approach used instead)
- ✅ Phase 2: Region Persistence - **COMPLETE** (see implementation details below)
- ✅ Phase 3: Settling Evaluation Tool - **COMPLETE** (see implementation details below)
- 🔲 Phase 4: Adaptive Settling Mode - Not started

**Current Capability**: Region data is persisted with scene hash and automatically restored when processing PCAPs from the same location, skipping the ~30 second settling period entirely.

**Cross-reference**: The sweep runner (`internal/lidar/sweep/runner.go`) implements a `SettleMode` field with two options: `once` (settle once, keep grid across combinations) and `per_combo` (re-settle per combination). This uses region persistence for efficient parameter sweeps. See also [`auto-tuning.md`](auto-tuning.md).

## Overview

This document proposes two complementary approaches to address the loss of ~30 seconds of data at the start of PCAP file analysis due to the LiDAR background regions settling period. The current implementation requires 100-300 frames (5-30 seconds at 10-20 Hz) of settling before foreground identification can begin, causing valuable data to be discarded.

## Problem Statement

### Current Behaviour

When processing PCAP files or starting a new LiDAR session:

1. **Background Grid Initialisation**: A fresh `BackgroundGrid` is created with empty cells
2. **Settling Period**: The system processes `WarmupMinFrames` (default: 100) or `WarmupDuration` (default: 30s) before marking `SettlingComplete = true`
3. **Region Identification**: Adaptive regions are identified based on variance collected during settling
4. **Foreground Suppression**: All foreground detections are suppressed until settling completes

**Impact**: For a 5-minute PCAP capture, the first 30 seconds (10% of data) is effectively lost for foreground analysis.

### Root Cause

The settling period serves two critical purposes:

1. **Background Model Seeding**: Cells need sufficient observations to establish reliable `AverageRangeMeters` and `RangeSpreadMeters` values
2. **Region Variance Collection**: The `RegionManager` needs variance samples to classify cells into stable/variable/volatile regions

Both processes currently run only during live data collection and are not persisted in a reusable form.

## Proposed Solutions

### Option A: Background Grid and Region Persistence

**Concept**: Save the settled background grid and identified regions to the database, then restore them when processing PCAPs from the same sensor/location.

#### Database Schema Changes

Extend `lidar_bg_snapshot` table or create a new table for region metadata:

> **Source:** Migration `000017_create_lidar_bg_regions.up.sql`. Option A.1 adds a `regions_json` column to `lidar_bg_snapshot`. Option A.2 (preferred) creates a separate `lidar_bg_regions` table with columns: region_set_id, snapshot_id (FK), sensor_id, created_unix_nanos, region_count, regions_json, variance_data_json, settling_frames, grid_hash, UNIQUE(snapshot_id). Indexed on sensor_id.

#### Implementation Components

1. **RegionManager Serialisation** (`internal/lidar/background.go`):

   > **Source:** `internal/lidar/background.go`. `RegionSnapshot` struct with fields: Regions, CellToRegionID, FramesSampled, IdentifiedAt. `RegionManager` exposes `ToSnapshot()` and `RestoreFromSnapshot()` methods for persistence round-trips.

2. **BackgroundManager Restoration** (`internal/lidar/background.go`):

   > **Source:** Same file. `BackgroundManager.RestoreFromSnapshot()` takes a grid snapshot and optional region snapshot. Deserialises grid cells, restores `RegionManager` state, and marks `SettlingComplete = true` when regions are present.

3. **BgStore Interface Extension** (`internal/lidar/background.go`):

   > **Source:** Same file. `BgStore` interface adds `InsertRegionSnapshot()` and `GetLatestRegionSnapshot()` alongside the existing `InsertBgSnapshot()`.

4. **PCAP Analysis Integration** (`cmd/tools/pcap-analyse/main.go`):

   > **Source:** `cmd/tools/pcap-analyse/main.go`. When `--restore-background` is set, loads the latest grid and region snapshots from the DB and calls `RestoreFromSnapshot()` — foreground detection begins immediately without settling.

#### Scene Similarity Detection (Optional Enhancement)

For multi-location deployments, detect whether a saved background matches the current scene:

> **Source:** `internal/lidar/background.go`. `SceneSignature()` returns a hash based on cell range distribution, coverage pattern, and variance distribution. `IsSceneCompatible()` compares a saved signature against the current scene within a configurable threshold.

#### Pros

- **Eliminates settling delay entirely** for subsequent sessions at the same location
- **Preserves tuned region parameters** across restarts
- **Natural fit** with existing snapshot persistence architecture
- **Enables "warm start"** for live deployments after service restart

#### Cons

- **Scene dependency**: Saved backgrounds are invalid if the physical scene changes
- **Storage growth**: Additional database storage for region metadata
- **Complexity**: Need to handle version compatibility and schema migrations

### Option B: Adaptive Settling Time Evaluation

**Concept**: Create a test harness that evaluates when the background model has "stabilised enough" based on convergence metrics, rather than using fixed frame/time thresholds.

#### Convergence Metrics

Define metrics to determine when settling can end early:

1. **Cell Coverage Rate**: Percentage of cells with `TimesSeenCount > 0`
2. **Spread Convergence**: Rate of change in `RangeSpreadMeters` values
3. **Region Stability**: Variance in region classification across consecutive evaluations
4. **Background Model Confidence**: Aggregate `TimesSeenCount` across all cells

> **Source:** `internal/lidar/l3grid/settling_eval.go`. `SettlingMetrics` struct tracks CoverageRate, SpreadDeltaRate, RegionStability, MeanConfidence, EvaluatedAt, and FrameNumber. `BackgroundManager.EvaluateSettling()` computes the metrics; `SettlingMetrics.IsConverged()` checks them against `SettlingThresholds`.

#### Test Harness Tool

Create `cmd/tools/settling-eval/main.go`:

> **Source:** `cmd/tools/settling-eval/main.go`. `SettlingEvaluation` struct with fields: PcapFile, SensorID, TotalFrames, MetricsHistory (per-frame convergence snapshots), RecommendedFrame, RecommendedTime, and Rationale. The tool processes all frames with settling suppressed, computes convergence metrics at each, and reports the optimal settling point.

#### Dynamic Settling Mode

Modify `ProcessFramePolar` to support adaptive settling:

> **Source:** `internal/lidar/config.go` (when implemented). `SettlingMode` enum (`Fixed`, `Adaptive`). `BackgroundConfig` gains SettlingMode plus three adaptive thresholds: MinCoverageForSettling (e.g. 0.8), MaxSpreadDeltaForSettling (e.g. 0.001), and MinConfidenceForSettling (e.g. 10).

#### Pros

- **Data-driven optimisation**: Settling time adapts to actual scene complexity
- **No scene dependency**: Works for any location without prior data
- **Diagnostic value**: Metrics help tune parameters for different environments
- **Lower storage**: No additional database tables required

#### Cons

- **Not instant**: Still requires some settling, just potentially less
- **Complexity**: Convergence detection adds processing overhead
- **Tuning required**: Thresholds need calibration per deployment environment

## Recommended Approach: Hybrid Implementation

Implement both options in phases:

### Phase 1: Background Grid Restoration (Option A Core)

1. Add `RestoreFromSnapshot()` to `BackgroundManager`
2. Implement grid cell deserialisation from `BgSnapshot.GridBlob`
3. Add `--restore-background` flag to `pcap-analyse`
4. Mark `SettlingComplete = true` when restoring a valid snapshot

**Outcome**: Immediate settling skip for sensors with existing snapshots.

### Phase 2: Region Persistence ✅ COMPLETE

**Status**: Implemented February 2026

**Implementation**:

1. ✅ Add `lidar_bg_regions` table
   - Migration: `000017_create_lidar_bg_regions.up.sql`
   - Schema: `region_set_id`, `snapshot_id`, `sensor_id`, `created_unix_nanos`, `region_count`, `regions_json`, `variance_data_json`, `settling_frames`, `grid_hash`
   - Indexes: `idx_bg_regions_sensor`, `idx_bg_regions_grid_hash`

2. ✅ Implement `RegionManager.ToSnapshot()` and `RestoreFromSnapshot()`
   - `ToSnapshot()`: Serialises regions to `RegionSnapshot` with JSON-encoded `RegionData`
   - `RestoreFromSnapshot()`: Rebuilds `RegionManager` state from snapshot
   - Location: `internal/lidar/background.go` (lines 667-749)

3. ✅ Persist regions alongside grid snapshots
   - `BackgroundManager.persistRegionsOnSettleLocked()`: Persists regions when settling completes (background.go:1453-1483)
   - `BackgroundGrid.sceneSignatureUnlocked()`: Computes scene hash from range/spread distribution histogram (background.go:249-310)
   - `Persist()` extended to persist regions via `RegionStore` interface when settling completes

4. ✅ Restore regions to enable immediate adaptive parameter application
   - `BackgroundManager.tryRestoreRegionsFromStoreLocked()`: Attempts restoration after ~10 warmup frames (background.go:1407-1446)
   - `regionRestoreMinFrames = 10`: Enough frames to build stable scene signature
   - `regionRestoreAttempted` flag: Ensures DB lookup happens only once per settling cycle
   - Reset by `ResetGrid()` on PCAP start

**Key Features**:

- **Scene Hash Matching**: SHA256 hash of range distribution (6 buckets) + spread distribution (4 buckets) + coverage count
- **Early Restoration**: Attempts restore after 10 frames (vs. 100-300 for full settling)
- **Automatic Persistence**: Regions saved when settling completes, independent of periodic background flusher
- **Lock-Safe**: Uses `sceneSignatureUnlocked()` for use within locked sections

**DB Methods** (`internal/db/db.go`):

- `InsertRegionSnapshot()`
- `GetRegionSnapshotByGridHash()`
- `GetLatestRegionSnapshot()`

**Outcome**: Full state restoration including region-specific parameters. Settling period can be skipped entirely when scene hash matches a previous run.

### Phase 3: Settling Evaluation Tool (Option B) ✅ COMPLETE

**Status**: Implemented February 2026

**Implementation**:

1. ✅ Create `settling-eval` CLI tool
   - Location: `cmd/tools/settling-eval/main.go`
   - Connects to running server via `/api/lidar/settling_eval` endpoint
   - Polls convergence metrics at configurable interval
   - Outputs JSON evaluation with recommended `WarmupMinFrames`
   - Usage: `settling-eval --server http://localhost:8080 --sensor hesai-01 [--output report.json]`

2. ✅ Implement convergence metrics computation
   - `SettlingMetrics` struct: `CoverageRate`, `SpreadDeltaRate`, `RegionStability`, `MeanConfidence`
   - `SettlingThresholds` struct with `DefaultSettlingThresholds()`
   - `EvaluateSettling(frameNumber)` method on `BackgroundManager`
   - `IsConverged(thresholds)` method on `SettlingMetrics`
   - Location: `internal/lidar/l3grid/settling_eval.go`

3. ✅ Generate recommendations for `WarmupMinFrames` tuning
   - CLI outputs recommended frame count and duration
   - Includes 20% safety margin in recommendation
   - Provides rationale explaining convergence status

4. ✅ Document recommended settings per scene type
   - Default thresholds: coverage ≥ 80%, spread delta ≤ 0.001, region stability ≥ 95%, confidence ≥ 10
   - Thresholds suitable for typical outdoor LiDAR scenes

**API Endpoint**: `GET /api/lidar/settling_eval?sensor_id=<id>`

Returns `{ sensor_id, metrics, thresholds, converged, settling_complete }`

**Makefile**: `make build-settling-eval`

**Outcome**: Data-driven guidance for tuning settling parameters.

### Phase 4: Adaptive Settling Mode (Optional)

1. Add `SettlingModeAdaptive` option
2. Implement convergence-based settling termination
3. Add runtime API to switch settling modes

**Outcome**: Self-tuning settling for new deployments without prior data.

## Implementation Priority

| Phase   | Effort | Value  | Priority                                            |
| ------- | ------ | ------ | --------------------------------------------------- |
| Phase 1 | Medium | High   | **P0** - Immediate benefit for existing deployments |
| Phase 2 | Low    | Medium | P1 - Completes the restoration story                |
| Phase 3 | Medium | Medium | P1 - Provides tuning guidance                       |
| Phase 4 | High   | Low    | P2 - Nice-to-have for edge cases                    |

## API Changes

### New CLI Flags for `pcap-analyse`

```bash
pcap-analyse --pcap file.pcap \
    --restore-background         # Restore from latest database snapshot
    --restore-background-id 123  # Restore from specific snapshot ID
    --save-background            # Save final state to database (existing)
    --settling-mode adaptive     # Use convergence-based settling
```

### New HTTP API Endpoints

```bash
# Get region snapshot for a sensor
GET /api/lidar/background/regions?sensor_id=hesai-01

# Restore background from snapshot
POST /api/lidar/background/restore
{
    "sensor_id": "hesai-01",
    "snapshot_id": 123,  // optional, defaults to latest
    "include_regions": true
}

# Evaluate current settling status
GET /api/lidar/background/settling-status?sensor_id=hesai-01
{
    "settling_complete": false,
    "frames_processed": 45,
    "target_frames": 100,
    "coverage_rate": 0.72,
    "mean_confidence": 8.3,
    "estimated_completion_sec": 2.75
}
```

## Testing Strategy

1. **Unit Tests**: `internal/lidar/background_restore_test.go`
   - Test grid restoration from valid/invalid snapshots
   - Test region restoration with various configurations
   - Test convergence metric calculations

2. **Integration Tests**: `internal/lidar/integration_restore_test.go`
   - Test full restore → process → detect cycle
   - Verify foreground detection quality matches non-restored baseline

3. **Benchmark Tests**: `internal/lidar/settling_benchmark_test.go`
   - Measure settling time reduction with restoration
   - Profile memory usage during restoration

4. **Manual Validation**:
   - Process same PCAP with and without restoration
   - Compare foreground detection results
   - Verify no regression in detection quality

## Security Considerations

- **Snapshot Integrity**: Validate grid dimensions match before restoration
- **Version Compatibility**: Check snapshot format version before deserialisation
- **Path Traversal**: Ensure snapshot IDs are numeric, not paths

## Related Documentation

- [Adaptive Region Parameters](../operations/adaptive-region-parameters.md)
- [PCAP Split Tool](../../plans/pcap-split-tool-plan.md) (future: auto-segment for settling)
- [LiDAR Background Grid Standards](../architecture/lidar-background-grid-standards.md)

## References

- `internal/lidar/background.go`: `BackgroundManager`, `BackgroundGrid`, `RegionManager`
- `internal/lidar/config.go`: `BackgroundConfig`, settling parameters
- `internal/db/db.go`: `GetLatestBgSnapshot`, `InsertBgSnapshot`
- `cmd/tools/pcap-analyse/main.go`: PCAP analysis tool

## Appendix: Current Settling Parameters

From `config.go`:

> **Source:** `internal/lidar/config.go`. `DefaultBackgroundConfig()` returns WarmupDuration 30 s, WarmupMinFrames 100, SettlingPeriod 5 min (for first snapshot).

At 10 Hz (Hesai P40), 100 frames = 10 seconds.
At 20 Hz, 100 frames = 5 seconds.
`WarmupDuration` of 30s ensures both conditions are met.

## Changelog

- **2026-02-05**: Initial design document created
