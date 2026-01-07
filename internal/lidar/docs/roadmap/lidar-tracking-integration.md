# LiDAR Tracking Integration Status

## Current State

**STATUS: ✅ INTEGRATED** (as of 2024-12-01)

The LiDAR tracking infrastructure is **fully integrated** into the data processing pipeline:

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

### ✅ Integrated Pipeline

The tracking pipeline is **fully connected** to the frame processing flow:

```
Complete Flow:
  UDP/PCAP → Parser → FrameBuilder → FrameCallback
                                          ↓
                                     BackgroundManager.ProcessFramePolarWithMask
                                          ↓
                                     Foreground Extraction (ExtractForegroundPoints)
                                          ↓
                                     Transform to World (TransformToWorld)
                                          ↓
                                     Clustering (DBSCAN)
                                          ↓
                                     Tracker.Update → Classify → DB (tracks/observations)
```

## How Tracking Works Now

When you run PCAP analysis mode (`analysis_mode=true`), the system:

1. ✅ Processes packets and builds frames
2. ✅ Feeds frames to `BackgroundManager.ProcessFramePolarWithMask`
3. ✅ Builds background grid (preserved in analysis mode)
4. ✅ Extracts foreground points using `ExtractForegroundPoints`
5. ✅ Transforms to world coordinates using `TransformToWorld`
6. ✅ Clusters foreground points using `DBSCAN`
7. ✅ Updates tracker with clusters using `Tracker.Update`
8. ✅ Classifies tracks using `TrackClassifier.ClassifyAndUpdate`
9. ✅ Saves tracks/observations to database

**Result:** `lidar_tracks` and `lidar_track_obs` will populate with detected objects.

## Implementation Details

### Integrated Components

#### Phase 1: Foreground Extraction (✅ COMPLETE)

Location: `cmd/radar/radar.go` lines 315-330

```go
// Phase 1: Foreground extraction
mask, err := backgroundManager.ProcessFramePolarWithMask(polar)
if err != nil || mask == nil {
    return
}

foregroundPoints := lidar.ExtractForegroundPoints(polar, mask)
if len(foregroundPoints) == 0 {
    return // No foreground detected
}
```

Uses:

- `BackgroundManager.ProcessFramePolarWithMask()` - per-point classification
- `lidar.ExtractForegroundPoints()` - extract foreground-marked points

#### Phase 2: World Transform & Clustering (✅ COMPLETE)

Location: `cmd/radar/radar.go` lines 335-345

```go
// Phase 2: Transform to world coordinates
worldPoints := lidar.TransformToWorld(foregroundPoints, nil, *lidarSensor)

// Phase 3: Clustering
clusters := lidar.DBSCAN(worldPoints, lidar.DefaultDBSCANParams())
if len(clusters) == 0 {
    return
}
```

Uses:

- `lidar.TransformToWorld()` - polar → world Cartesian conversion
- `lidar.DBSCAN()` - density-based clustering with default params

#### Phase 3: Tracker Integration (✅ COMPLETE)

Location: `cmd/radar/radar.go` lines 352-405

```go
// Phase 4: Track update
if tracker != nil {
    tracker.Update(clusters, frame.StartTimestamp)

    // Phase 5: Classify and persist confirmed tracks
    confirmedTracks := tracker.GetConfirmedTracks()
    for _, track := range confirmedTracks {
        // Classify if not already classified
        if track.ObjectClass == "" && track.ObservationCount >= 5 && classifier != nil {
            classifier.ClassifyAndUpdate(track)
        }

        // Persist track and observation to database
        worldFrame := fmt.Sprintf("site/%s", *lidarSensor)
        lidar.InsertTrack(lidarDB.DB, track, worldFrame)

        obs := &lidar.TrackObservation{ /* ... */ }
        lidar.InsertTrackObservation(lidarDB.DB, obs)
    }
}
```

Components initialized at startup (lines 257-260):

```go
tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())
classifier := lidar.NewTrackClassifier()
```

#### Phase 4: Analysis Run Management (⏸️ DEFERRED)

Analysis runs are not yet created for PCAP sessions. This is tracked as future work:

```go
// TODO: Create analysis run when starting PCAP analysis mode
// TODO: Update run status and track count on completion
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

## Integration Status

✅ **COMPLETED** (2024-12-01):

- Foreground Extraction: ✅
- World Transform: ✅
- Clustering (DBSCAN): ✅
- Tracker Integration: ✅
- Classification: ✅
- Database Persistence: ✅

⏸️ **DEFERRED** (future work):

- Analysis Run Management (metadata tracking for PCAP sessions)

## References

- Schema: `internal/db/migrations/000010_create_lidar_analysis_runs.up.sql`
- Tracking: `internal/lidar/tracking.go`
- Storage: `internal/lidar/track_store.go`
- Background: `internal/lidar/background.go`
- Pipeline: `cmd/radar/radar.go` (lines 280-320)
