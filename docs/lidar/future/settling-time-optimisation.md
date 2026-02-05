# LiDAR Background Settling Time Optimisation

## Executive Summary

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

```sql
-- Option A.1: Add region data to existing snapshot
ALTER TABLE lidar_bg_snapshot ADD COLUMN regions_json TEXT;
-- regions_json contains serialised RegionManager state

-- Option A.2: Separate table (preferred for clarity)
CREATE TABLE IF NOT EXISTS lidar_bg_regions (
    region_set_id INTEGER PRIMARY KEY,
    snapshot_id INTEGER NOT NULL REFERENCES lidar_bg_snapshot(snapshot_id),
    sensor_id TEXT NOT NULL,
    created_unix_nanos INTEGER NOT NULL,
    region_count INTEGER NOT NULL,
    regions_json TEXT NOT NULL,  -- serialised []Region with CellToRegionID
    variance_data_json TEXT,     -- optional: SettlingMetrics for debugging
    settling_frames INTEGER,     -- frames used to identify regions
    scene_hash TEXT,             -- optional: hash for scene similarity detection
    UNIQUE(snapshot_id)
);

CREATE INDEX IF NOT EXISTS idx_bg_regions_sensor ON lidar_bg_regions(sensor_id);
```

#### Implementation Components

1. **RegionManager Serialisation** (`internal/lidar/background.go`):
   ```go
   // RegionSnapshot for database persistence
   type RegionSnapshot struct {
       Regions        []*Region `json:"regions"`
       CellToRegionID []int     `json:"cell_to_region_id"`
       FramesSampled  int       `json:"frames_sampled"`
       IdentifiedAt   time.Time `json:"identified_at"`
   }
   
   func (rm *RegionManager) ToSnapshot() *RegionSnapshot
   func (rm *RegionManager) RestoreFromSnapshot(snap *RegionSnapshot) error
   ```

2. **BackgroundManager Restoration** (`internal/lidar/background.go`):
   ```go
   // RestoreFromSnapshot restores grid state and regions from a database snapshot.
   // If the snapshot includes region data, settling is marked complete immediately.
   func (bm *BackgroundManager) RestoreFromSnapshot(snap *BgSnapshot, regionSnap *RegionSnapshot) error {
       // Deserialise grid cells from snap.GridBlob
       // Restore RegionManager from regionSnap if present
       // Set SettlingComplete = true if regions are restored
   }
   ```

3. **BgStore Interface Extension** (`internal/lidar/background.go`):
   ```go
   type BgStore interface {
       InsertBgSnapshot(snap *BgSnapshot) (int64, error)
       // New methods
       InsertRegionSnapshot(snap *RegionSnapshot, snapshotID int64) error
       GetLatestRegionSnapshot(sensorID string) (*RegionSnapshot, error)
   }
   ```

4. **PCAP Analysis Integration** (`cmd/tools/pcap-analyse/main.go`):
   ```go
   // Before processing PCAP, attempt to restore from database
   if *restoreBackground {
       snap, _ := store.GetLatestBgSnapshot(sensorID)
       regionSnap, _ := store.GetLatestRegionSnapshot(sensorID)
       if snap != nil && regionSnap != nil {
           bm.RestoreFromSnapshot(snap, regionSnap)
           // SettlingComplete is now true; foreground detection begins immediately
       }
   }
   ```

#### Scene Similarity Detection (Optional Enhancement)

For multi-location deployments, detect whether a saved background matches the current scene:

```go
// SceneSignature creates a hash representing the background characteristics
func (bm *BackgroundManager) SceneSignature() string {
    // Hash based on:
    // - Distribution of cell ranges (histogram buckets)
    // - Coverage pattern (which cells have data)
    // - Variance distribution
}

// IsSceneCompatible checks if a saved snapshot matches the current scene
func (bm *BackgroundManager) IsSceneCompatible(savedSignature string, threshold float64) bool
```

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

```go
// SettlingMetrics tracks convergence indicators during warmup
type SettlingMetrics struct {
    CoverageRate      float64   // Fraction of cells with data
    SpreadDeltaRate   float64   // Rate of change in spread values
    RegionStability   float64   // Consistency of region classification
    MeanConfidence    float64   // Average TimesSeenCount
    EvaluatedAt       time.Time
    FrameNumber       int
}

// EvaluateSettling checks if the background model has converged
func (bm *BackgroundManager) EvaluateSettling() SettlingMetrics

// SettlingComplete returns true if metrics indicate sufficient convergence
func (m SettlingMetrics) IsConverged(thresholds SettlingThresholds) bool
```

#### Test Harness Tool

Create `cmd/tools/settling-eval/main.go`:

```go
// settling-eval --pcap <file> [--sensor <id>] [--output <json>]
//
// Evaluates settling time for a PCAP file by:
// 1. Processing all frames with settling suppressed
// 2. Computing convergence metrics at each frame
// 3. Reporting optimal settling point

type SettlingEvaluation struct {
    PcapFile        string            `json:"pcap_file"`
    SensorID        string            `json:"sensor_id"`
    TotalFrames     int               `json:"total_frames"`
    MetricsHistory  []SettlingMetrics `json:"metrics_history"`
    RecommendedFrame int              `json:"recommended_settling_frame"`
    RecommendedTime  time.Duration    `json:"recommended_settling_time"`
    Rationale       string            `json:"rationale"`
}
```

#### Dynamic Settling Mode

Modify `ProcessFramePolar` to support adaptive settling:

```go
type SettlingMode int

const (
    SettlingModeFixed    SettlingMode = iota // Current: fixed frames/duration
    SettlingModeAdaptive                     // New: convergence-based
)

// BackgroundConfig additions
type BackgroundConfig struct {
    // ... existing fields ...
    
    // Adaptive settling thresholds
    SettlingMode              SettlingMode
    MinCoverageForSettling    float64 // e.g., 0.8 (80% of cells have data)
    MaxSpreadDeltaForSettling float64 // e.g., 0.001 (spread changes < 0.1%/frame)
    MinConfidenceForSettling  uint32  // e.g., 10 (average TimesSeenCount)
}
```

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

### Phase 2: Region Persistence

1. Add `lidar_bg_regions` table
2. Implement `RegionManager.ToSnapshot()` and `RestoreFromSnapshot()`
3. Persist regions alongside grid snapshots
4. Restore regions to enable immediate adaptive parameter application

**Outcome**: Full state restoration including region-specific parameters.

### Phase 3: Settling Evaluation Tool (Option B)

1. Create `settling-eval` CLI tool
2. Implement convergence metrics computation
3. Generate recommendations for `WarmupMinFrames` tuning
4. Document recommended settings per scene type

**Outcome**: Data-driven guidance for tuning settling parameters.

### Phase 4: Adaptive Settling Mode (Optional)

1. Add `SettlingModeAdaptive` option
2. Implement convergence-based settling termination
3. Add runtime API to switch settling modes

**Outcome**: Self-tuning settling for new deployments without prior data.

## Implementation Priority

| Phase | Effort | Value | Priority |
|-------|--------|-------|----------|
| Phase 1 | Medium | High | **P0** - Immediate benefit for existing deployments |
| Phase 2 | Low | Medium | P1 - Completes the restoration story |
| Phase 3 | Medium | Medium | P1 - Provides tuning guidance |
| Phase 4 | High | Low | P2 - Nice-to-have for edge cases |

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
- [PCAP Split Tool](pcap-split-tool.md) (future: auto-segment for settling)
- [LiDAR Background Grid Standards](../architecture/lidar-background-grid-standards.md)

## References

- `internal/lidar/background.go`: `BackgroundManager`, `BackgroundGrid`, `RegionManager`
- `internal/lidar/config.go`: `BackgroundConfig`, settling parameters
- `internal/db/db.go`: `GetLatestBgSnapshot`, `InsertBgSnapshot`
- `cmd/tools/pcap-analyse/main.go`: PCAP analysis tool

## Appendix: Current Settling Parameters

From `config.go`:

```go
DefaultBackgroundConfig() returns:
    WarmupDuration:  30 * time.Second
    WarmupMinFrames: 100
    SettlingPeriod:  5 * time.Minute  // for first snapshot
```

At 10 Hz (Hesai P40), 100 frames = 10 seconds.
At 20 Hz, 100 frames = 5 seconds.
`WarmupDuration` of 30s ensures both conditions are met.

## Changelog

- **2026-02-05**: Initial design document created
