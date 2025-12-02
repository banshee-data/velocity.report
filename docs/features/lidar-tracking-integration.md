# LiDAR Tracking Integration Status

## Current State

The LiDAR tracking infrastructure exists but is **not yet integrated** into the data processing pipeline:

### ✅ Implemented Components

1. **Database Schema** - Tables created via migration 000010:

   - `lidar_tracks` - Track lifecycle and statistics
   - `lidar_track_obs` - Per-frame track observations
   - `lidar_clusters` - Detected foreground clusters
   - `lidar_analysis_runs` - Analysis run metadata
   - `lidar_run_tracks` - Track-to-run associations

2. **Tracking Algorithm** (`internal/lidar/tracking.go`):

   - Kalman filter-based tracker
   - Gating and data association
   - Track lifecycle management (tentative → confirmed → deleted)
   - Speed statistics and feature aggregation

3. **Track Storage** (`internal/lidar/track_store.go`):

   - `InsertTrack()` - Save new tracks
   - `UpdateTrack()` - Update existing tracks
   - `InsertTrackObservation()` - Save per-frame observations
   - `InsertCluster()` - Save detected clusters

4. **Background Subtraction** (`internal/lidar/background.go`):
   - Background grid accumulation
   - Foreground point extraction
   - PCAP analysis mode with grid preservation

### ❌ Missing Integration

The tracking pipeline is **not connected** to the frame processing flow:

```
Current Flow:
  UDP/PCAP → Parser → FrameBuilder → BackgroundManager
                                          ↓
                                     (stops here)

Needed Flow:
  UDP/PCAP → Parser → FrameBuilder → BackgroundManager
                                          ↓
                                     Foreground Extraction
                                          ↓
                                     Clustering
                                          ↓
                                     Tracker → DB (tracks/observations)
```

## Why Tracks Are Empty

When you run PCAP analysis mode (`analysis_mode=true`), the system:

1. ✅ Processes packets and builds frames
2. ✅ Feeds frames to `BackgroundManager`
3. ✅ Builds background grid (preserved in analysis mode)
4. ❌ Does NOT extract foreground points
5. ❌ Does NOT cluster foreground points
6. ❌ Does NOT track clusters
7. ❌ Does NOT save tracks/observations to database

**Result:** `lidar_tracks` and `lidar_track_obs` remain empty.

## Required Integration Work

### Phase 1: Foreground Extraction

Add foreground extraction to `BackgroundManager.ProcessFramePolar()`:

```go
// internal/lidar/background.go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) ForegroundResult {
    // Existing: Update background grid
    bm.updateBackgroundCells(points)

    // NEW: Extract foreground points
    foregroundPoints := bm.extractForeground(points)

    return ForegroundResult{
        Timestamp:        time.Now(),
        ForegroundPoints: foregroundPoints,
        Stats:            /* ... */,
    }
}
```

### Phase 2: Clustering

Add clustering module to group foreground points:

```go
// internal/lidar/clustering.go (NEW FILE)
func ClusterForegroundPoints(points []lidar.Point, config ClusterConfig) []WorldCluster {
    // DBSCAN or Euclidean clustering
    // Convert point groups → WorldCluster structs
    // Return clusters for tracking
}
```

### Phase 3: Tracker Integration

Instantiate and wire tracker in `cmd/radar/radar.go`:

```go
// cmd/radar/radar.go (in lidar initialization section)
tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

frameCallback := func(frame *lidar.LiDARFrame) {
    // Existing: Feed to BackgroundManager
    foreground := backgroundManager.ProcessFramePolar(polar)

    // NEW: Cluster foreground
    clusters := lidar.ClusterForegroundPoints(foreground.ForegroundPoints, clusterConfig)

    // NEW: Update tracker
    tracker.Update(clusters, frame.Timestamp)

    // NEW: Persist tracks
    for _, track := range tracker.GetActiveTracks() {
        if track.State == lidar.TrackConfirmed {
            lidar.InsertTrack(lidarDB, track, worldFrame)
            obs := lidar.TrackObservationFromTrack(track)
            lidar.InsertTrackObservation(lidarDB, obs)
        }
    }
}
```

### Phase 4: Analysis Run Management

Create analysis runs for PCAP sessions:

```go
// When starting PCAP analysis mode
run := &lidar.AnalysisRun{
    RunID:      generateRunID(),
    SourceType: "pcap",
    SourcePath: pcapFile,
    Status:     "running",
    Params:     /* tracker + cluster config */,
}
analysisStore.InsertRun(run)

// After PCAP completion
run.Status = "completed"
run.TracksDetected = len(tracker.Tracks)
analysisStore.UpdateRun(run)
```

## Workaround for Testing

Until integration is complete, you can manually test the tracking components:

### Test 1: Track Storage

```go
// Test InsertTrack directly
track := &lidar.TrackedObject{
    TrackID:  "test_001",
    SensorID: "hesai-pandar40p",
    State:    lidar.TrackConfirmed,
    X:        10.5,
    Y:        -3.2,
    VX:       5.0,
    VY:       0.0,
    FirstUnixNanos: time.Now().UnixNano(),
    LastUnixNanos:  time.Now().UnixNano(),
}
err := lidar.InsertTrack(db, track, "site/main")
```

### Test 2: Tracker Algorithm

```go
// Test Tracker.Update with synthetic clusters
tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())
clusters := []lidar.WorldCluster{
    {
        SensorID:    "hesai-pandar40p",
        TSUnixNanos: time.Now().UnixNano(),
        CentroidX:   10.0,
        CentroidY:   -5.0,
        CentroidZ:   1.5,
        PointsCount: 150,
    },
}
tracker.Update(clusters, time.Now())
activeTracks := tracker.GetActiveTracks()
// activeTracks should have 1 tentative track
```

## Implementation Priority

**High Priority (Core Functionality):**

1. Foreground extraction from background manager
2. Basic clustering (Euclidean distance)
3. Tracker instantiation and update loop
4. Track/observation persistence

**Medium Priority (Analysis Features):**

1. Analysis run management
2. PCAP-specific run metadata
3. Track-to-run associations

**Low Priority (Enhancements):**

1. Advanced clustering (DBSCAN)
2. Track classification
3. Multi-sensor fusion
4. Feature extraction for ML

## Testing Plan

Once integrated:

1. **Unit Tests:**

   - Foreground extraction accuracy
   - Clustering quality metrics
   - Tracker state transitions

2. **Integration Tests:**

   - End-to-end PCAP → tracks pipeline
   - Database persistence verification
   - Analysis run lifecycle

3. **PCAP Analysis Workflow:**

   ```bash
   # Start analysis mode
   curl -X POST "http://localhost:8082/api/lidar/pcap/start?sensor_id=hesai-pandar40p" \
     -H "Content-Type: application/json" \
     -d '{"pcap_file":"break-80k.pcapng","analysis_mode":true}'

   # After completion, verify tracks
   sqlite3 sensor_data.db "SELECT COUNT(*) FROM lidar_tracks;"
   sqlite3 sensor_data.db "SELECT COUNT(*) FROM lidar_track_obs;"
   sqlite3 sensor_data.db "SELECT COUNT(*) FROM lidar_clusters;"
   ```

## Timeline Estimate

- **Foreground Extraction:** 4-6 hours
- **Clustering Module:** 6-8 hours
- **Tracker Integration:** 4-6 hours
- **Testing & Refinement:** 8-10 hours

**Total:** 22-30 hours for complete integration

## References

- Schema: `internal/db/migrations/000010_create_lidar_analysis_runs.up.sql`
- Tracking: `internal/lidar/tracking.go`
- Storage: `internal/lidar/track_store.go`
- Background: `internal/lidar/background.go`
- Pipeline: `cmd/radar/radar.go` (lines 280-320)
