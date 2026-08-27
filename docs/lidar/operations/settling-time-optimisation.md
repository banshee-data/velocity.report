# LiDAR background settling time optimisation

- **Status:** Phase 4 Complete (August 2026)

This document proposes two complementary approaches to address the loss of ~30 seconds of data at the start of PCAP file analysis due to the LiDAR background regions settling period.

## Implementation summary

- ✅ Phase 1: Background Grid Restoration - Not implemented (regions-only approach used instead)
- ✅ Phase 2: Region Persistence - **COMPLETE** (see implementation details below)
- ✅ Phase 3: Settling Evaluation Tool - **COMPLETE** (see implementation details below)
- ✅ Phase 4: Adaptive Settling Mode - **COMPLETE** (see implementation details below)

**Current Capability**: Region data is persisted with scene hash and automatically restored when processing PCAPs from the same location, skipping the ~30 second settling period entirely. Where no snapshot matches, settling now ends on measured convergence instead of waiting out the fixed duration — a quiet scene settles in about six seconds rather than thirty.

**Cross-reference**: The sweep runner ([internal/lidar/sweep/runner.go](../../../internal/lidar/sweep/runner.go)) implements a `SettleMode` field with two options: `once` (settle once, keep grid across combinations) and `per_combo` (re-settle per combination). This uses region persistence for efficient parameter sweeps. See also [`auto-tuning.md`](auto-tuning.md).

## Overview

This document proposes two complementary approaches to address the loss of ~30 seconds of data at the start of PCAP file analysis due to the LiDAR background regions settling period. The current implementation requires 100-300 frames (5-30 seconds at 10-20 Hz) of settling before foreground identification can begin, causing valuable data to be discarded.

## Problem statement

### Current behaviour

When processing PCAP files or starting a new LiDAR session:

1. **Background Grid Initialisation**: A fresh `BackgroundGrid` is created with empty cells
2. **Settling Period**: The system processes `WarmupMinFrames` (default: 100) or `WarmupDuration` (default: 30s) before marking `SettlingComplete = true`
3. **Region Identification**: Adaptive regions are identified based on variance collected during settling
4. **Foreground Suppression**: All foreground detections are suppressed until settling completes

**Impact**: For a 5-minute PCAP capture, the first 30 seconds (10% of data) is effectively lost for foreground analysis.

### Root cause

The settling period serves two critical purposes:

1. **Background Model Seeding**: Cells need sufficient observations to establish reliable `AverageRangeMeters` and `RangeSpreadMeters` values
2. **Region Variance Collection**: The `RegionManager` needs variance samples to classify cells into stable/variable/volatile regions

Both processes currently run only during live data collection and are not persisted in a reusable form.

## Proposed solutions

### Option a: background grid and region persistence

**Concept**: Save the settled background grid and identified regions to the database, then restore them when processing PCAPs from the same sensor/location.

#### Database schema changes

Extend `lidar_bg_snapshot` table or create a new table for region metadata:

> **Source:** Migration `000017_create_lidar_bg_regions.up.sql`. Option A.1 adds a `regions_json` column to `lidar_bg_snapshot`. Option A.2 (preferred) creates a separate `lidar_bg_regions` table with columns: region_set_id, snapshot_id (FK), sensor_id, created_unix_nanos, region_count, regions_json, variance_data_json, settling_frames, grid_hash, UNIQUE(snapshot_id). Indexed on sensor_id.

#### Implementation components

1. **RegionManager Serialisation** (`internal/lidar/background.go`):

   > **Source:** `internal/lidar/background.go`. `RegionSnapshot` struct with fields: Regions, CellToRegionID, FramesSampled, IdentifiedAt. `RegionManager` exposes `ToSnapshot()` and `RestoreFromSnapshot()` methods for persistence round-trips.

2. **BackgroundManager Restoration** (`internal/lidar/background.go`):

   > **Source:** Same file. `BackgroundManager.RestoreFromSnapshot()` takes a grid snapshot and optional region snapshot. Deserialises grid cells, restores `RegionManager` state, and marks `SettlingComplete = true` when regions are present.

3. **BgStore Interface Extension** (`internal/lidar/background.go`):

   > **Source:** Same file. `BgStore` interface adds `InsertRegionSnapshot()` and `GetLatestRegionSnapshot()` alongside the existing `InsertBgSnapshot()`.

4. **PCAP Analysis Integration** ([internal/lidar/lidarbench/lidarbench.go](../../../internal/lidar/lidarbench/lidarbench.go)):

   > **Source:** [internal/lidar/lidarbench/lidarbench.go](../../../internal/lidar/lidarbench/lidarbench.go). When `--restore-background` is set, loads the latest grid and region snapshots from the DB and calls `RestoreFromSnapshot()`; foreground detection begins immediately without settling.

#### Scene similarity detection (optional enhancement)

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

### Option b: adaptive settling time evaluation

**Concept**: Create a test harness that evaluates when the background model has "stabilised enough" based on convergence metrics, rather than using fixed frame/time thresholds.

#### Convergence metrics

Define metrics to determine when settling can end early:

1. **Cell Coverage Rate**: Percentage of cells with `TimesSeenCount > 0`
2. **Spread Convergence**: Rate of change in `RangeSpreadMeters` values
3. **Region Stability**: Variance in region classification across consecutive evaluations
4. **Background Model Confidence**: Aggregate `TimesSeenCount` across all cells

> **Source:** [internal/lidar/l3grid/settling_eval.go](../../../internal/lidar/l3grid/settling_eval.go). `SettlingMetrics` struct tracks CoverageRate, SpreadDeltaRate, RegionStability, MeanConfidence, EvaluatedAt, and FrameNumber. `BackgroundManager.EvaluateSettling()` computes the metrics; `SettlingMetrics.IsConverged()` checks them against `SettlingThresholds`.

#### Test harness tool

Create [cmd/tools/settling-eval/main.go](../../../cmd/tools/settling-eval/main.go):

> **Source:** [cmd/tools/settling-eval/main.go](../../../cmd/tools/settling-eval/main.go). `SettlingEvaluation` struct with fields: PcapFile, SensorID, TotalFrames, MetricsHistory (per-frame convergence snapshots), RecommendedFrame, RecommendedTime, and Rationale. The tool processes all frames with settling suppressed, computes convergence metrics at each, and reports the optimal settling point.

#### Dynamic settling mode

Modify `ProcessFramePolar` to support adaptive settling:

> **Source:** delivered without a `SettlingMode` enum. `BackgroundParams` carries `SettlingMinCoverage`, `SettlingMaxSpreadDelta`, `SettlingMinRegionStability`, and `SettlingMinConfidence`; convergence engages when all four are set. See Phase 4 below.

#### Pros

- **Data-driven optimisation**: Settling time adapts to actual scene complexity
- **No scene dependency**: Works for any location without prior data
- **Diagnostic value**: Metrics help tune parameters for different environments
- **Lower storage**: No additional database tables required

#### Cons

- **Not instant**: Still requires some settling, just potentially less
- **Complexity**: Convergence detection adds processing overhead
- **Tuning required**: Thresholds need calibration per deployment environment

## Recommended approach: hybrid implementation

Implement both options in phases:

### Phase 1: background grid restoration (option a core)

1. Add `RestoreFromSnapshot()` to `BackgroundManager`
2. Implement grid cell deserialisation from `BgSnapshot.GridBlob`
3. Add `--restore-background` flag to `settling-eval`
4. Mark `SettlingComplete = true` when restoring a valid snapshot

**Outcome**: Immediate settling skip for sensors with existing snapshots.

### Phase 2: region persistence ✅ COMPLETE

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

**DB Methods** ([internal/db/db.go](../../../internal/db/db.go)):

- `InsertRegionSnapshot()`
- `GetRegionSnapshotByGridHash()`
- `GetLatestRegionSnapshot()`

**Outcome**: Full state restoration including region-specific parameters. Settling period can be skipped entirely when scene hash matches a previous run.

### Phase 3: settling evaluation tool (option b) ✅ COMPLETE

**Status**: Implemented February 2026

**Implementation**:

1. ✅ Create `settling-eval` CLI tool
   - Location: [cmd/tools/settling-eval/main.go](../../../cmd/tools/settling-eval/main.go)
   - Connects to running server via `/api/lidar/settling_eval` endpoint
   - Polls convergence metrics at configurable interval
   - Outputs JSON evaluation with recommended `WarmupMinFrames`
   - Usage: `settling-eval --server http://localhost:8080 --sensor hesai-01 [--output report.json]`

2. ✅ Implement convergence metrics computation
   - `SettlingMetrics` struct: `CoverageRate`, `SpreadDeltaRate`, `RegionStability`, `MeanConfidence`
   - `SettlingThresholds` struct with `DefaultSettlingThresholds()`
   - `EvaluateSettling(frameNumber)` method on `BackgroundManager`
   - `IsConverged(thresholds)` method on `SettlingMetrics`
   - Location: [internal/lidar/l3grid/settling_eval.go](../../../internal/lidar/l3grid/settling_eval.go)

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

### Phase 4: adaptive settling mode ✅ COMPLETE

Settling ends when the grid has demonstrably converged, rather than when a fixed
duration expires. `WarmupDurationNanos` becomes a **ceiling** for scenes that
never converge — a busy junction, or a sensor watching moving traffic — instead
of a toll every scene pays.

**Measured result**: a quiet scene settles in **5.9 s against a 30 s ceiling**
(2026-08-27). Until settling completes, foreground extraction yields nothing, so
this is 24 seconds of otherwise-empty scene recovered on every cold start.

#### What arms it

There is no `SettlingMode` enum. Convergence engages when all four thresholds in
[`SettlingThresholds`](../../../internal/lidar/l3grid/settling_eval.go) are
configured, and the existing frame-and-duration rule applies untouched when they
are not. All four are required: a partial set would let a grid settle on
whichever dimensions happened to be filled in, which is a weaker guarantee than
the duration it replaces.

The thresholds already existed in `config/tuning.defaults.json` and were already
consumed by the offline `settlingeval` tool (Phase 3). Phase 4 is largely the
work of consulting them while the grid is actually settling.

#### The frame minimum still applies

`warmup_min_frames` gates the convergence check entirely — it is not consulted
until the frames are in — so it sets the floor under any settling time
convergence can reach. It was lowered from 100 to **50** (five seconds at 10 Hz)
to match. Convergence measured over a handful of frames is not evidence of
anything, which is why the gate stays.

Evaluation walks every cell, so it runs on an interval
(`SettlingCheckInterval`, default 10 frames) rather than per frame.

#### One decision, two callers

The settling decision previously existed twice: once in the background update
path and once in foreground extraction, as two copies of the same test. They
drifted — convergence was added to one while the live pipeline ran the other, so
the feature was unreachable in production despite working under test. Both now
call `settlingCompleteLocked`, and a test asserts neither path reimplements it.

#### Observability

Settling reports itself at diag level:

```text
Settling started for sensor=X: 50 frames minimum, 30s ceiling, convergence armed (…)
Settling for sensor=X: 25 of 50 warm-up frames remaining, 2s of 30s elapsed
Settling complete for sensor=X after 5.918s on convergence (ceiling was 30s)
```

An unarmed grid says so explicitly, naming the missing thresholds. A convergence
check that fails names the unmet criterion with its value, so "still settling"
is never reported without something to act on.

Settling state also reaches operators live: `GET /api/lidar/data_source` carries
`settling` and `settling_progress`, and proto `PlaybackInfo` carries `settling`,
`settling_progress`, and `settling_elapsed_seconds`, which the macOS visualiser
shows as a `SETTLING 5.9s` badge. Until settling completes the scene renders
empty, which is otherwise indistinguishable from a dead sensor.

**Outcome**: Self-tuning settling for new deployments without prior data.

## Implementation priority

| Phase   | Effort | Value  | Priority                                            |
| ------- | ------ | ------ | --------------------------------------------------- |
| Phase 1 | Medium | High   | **P0** - Immediate benefit for existing deployments |
| Phase 2 | Low    | Medium | P1 - Completes the restoration story                |
| Phase 3 | Medium | Medium | P1 - Provides tuning guidance                       |
| Phase 4 | High   | Low    | ✅ Complete - delivered August 2026                 |

## API changes

### New CLI flags for `settling-eval`

```bash
settling-eval --pcap file.pcap \
    --restore-background         # Restore from latest database snapshot
    --restore-background-id 123  # Restore from specific snapshot ID
    --save-background            # Save final state to database (existing)
    --settling-mode adaptive     # Use convergence-based settling
```

### New HTTP API endpoints

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

## Testing strategy

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

## Security considerations

- **Snapshot Integrity**: Validate grid dimensions match before restoration
- **Version Compatibility**: Check snapshot format version before deserialisation
- **Path Traversal**: Ensure snapshot IDs are numeric, not paths

## Related documentation

- [Adaptive Region Parameters](../operations/adaptive-region-parameters.md)
- [PCAP Split Tool](../../plans/pcap-split-tool-plan.md) (future: auto-segment for settling)
- [LiDAR Background Grid Standards](../architecture/lidar-background-grid-standards.md)

## References

- `internal/lidar/background.go`: `BackgroundManager`, `BackgroundGrid`, `RegionManager`
- `internal/lidar/config.go`: `BackgroundConfig`, settling parameters
- [internal/db/db.go](../../../internal/db/db.go): `GetLatestBgSnapshot`, `InsertBgSnapshot`
- [internal/lidar/lidarbench/lidarbench.go](../../../internal/lidar/lidarbench/lidarbench.go): PCAP analysis tool

## Appendix: current settling parameters

From `config.go`:

> **Source:** `config/tuning.defaults.json`, active L3 profile. `warmup_duration_nanos` 30 s, `warmup_min_frames` 50, `settling_period` 5 min (for first snapshot), plus the four `settling_*` convergence thresholds.

At 10 Hz (Hesai P40), 50 frames = 5 seconds.
At 20 Hz, 50 frames = 2.5 seconds.

Since Phase 4 the 30 s duration is a ceiling, not the expected wait: a grid that
converges settles as soon as it does, and the frame minimum sets the floor.

## Changelog

- **2026-08-27**: Phase 4 complete — convergence-based settling termination; `warmup_min_frames` lowered 100 → 50; settling state surfaced on the HTTP API, the gRPC wire, and the visualiser badge
- **2026-02-05**: Initial design document created
