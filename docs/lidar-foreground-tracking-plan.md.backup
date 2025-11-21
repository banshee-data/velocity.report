# LIDAR Foreground Extraction and Tracking Implementation Plan

**Status:** Analysis Complete - Ready for Phase 2.9 Implementation  
**Date:** November 21, 2025  
**Author:** Ictinus (Product Architecture Agent)

---

## Executive Summary

This document analyzes the current state of the LIDAR project and provides a comprehensive implementation plan for:
- **Foreground point extraction** - Filtering classified points for clustering
- **Spatial clustering** - DBSCAN-based object detection
- **Multi-object tracking** - Kalman filter-based trajectory management
- **UI visualization** - Real-time track display in web frontend
- **SQL schema** - Persistent storage for tracks and observations

**Key Finding:** The infrastructure is production-ready. Background learning and classification are complete. The next phase requires implementing the point extraction pipeline that feeds into clustering and tracking algorithms.

---

## Table of Contents

1. [Current State Assessment](#current-state-assessment)
2. [Architecture Analysis](#architecture-analysis)
3. [Phase 2.9: Foreground Point Extraction](#phase-29-foreground-point-extraction)
4. [Phase 3.1: Spatial Clustering](#phase-31-spatial-clustering)
5. [Phase 3.2: Multi-Object Tracking](#phase-32-multi-object-tracking)
6. [Phase 3.3: UI & SQL Integration](#phase-33-ui--sql-integration)
7. [Implementation Roadmap](#implementation-roadmap)
8. [Performance Considerations](#performance-considerations)
9. [Testing Strategy](#testing-strategy)
10. [Appendix](#appendix)

---

## Current State Assessment

### ✅ Completed Components (Phase 1 & 2)

#### Background Grid Infrastructure
- **Grid Structure:** 40 rings × 1800 azimuth bins (0.2° resolution) = 72,000 cells
- **Learning Algorithm:** Exponential Moving Average (EMA) for range/spread tracking
- **Persistence:** Automatic snapshots to `lidar_bg_snapshot` table with versioning
- **Location:** `internal/lidar/background.go` (~900 lines)

#### Foreground/Background Classification
- **Algorithm:** Distance-adaptive threshold with neighbor voting
- **Formula:** `closeness_threshold = closeness_multiplier * (range_spread + noise_relative * observation_mean + 0.01) + safety_margin`
- **Spatial Filtering:** Same-ring neighbor confirmation (configurable count)
- **Temporal Filtering:** Cell freezing after large divergence detection
- **Status:** **Classification complete but foreground points not yet extracted**

#### PCAP Replay & Parameter Tuning
- **PCAP Support:** File-based replay with BPF filtering
- **Runtime Switching:** Live UDP ↔ PCAP via API (no restart needed)
- **Sweep Tools:** `bg-sweep` and `bg-multisweep` for parameter optimization
- **Visualization:** Grid heatmaps showing fill/settle rates by spatial region

#### HTTP API & Monitoring
```
GET  /api/lidar/params              - Get background parameters
POST /api/lidar/params              - Update parameters at runtime
GET  /api/lidar/acceptance          - Acceptance metrics by range bucket
GET  /api/lidar/grid_status         - Grid statistics and settling status
GET  /api/lidar/grid_heatmap        - Spatial bucket aggregation
POST /api/lidar/grid_reset          - Reset grid for testing
POST /api/lidar/pcap/start          - Start PCAP replay
GET  /api/lidar/pcap/stop           - Stop replay, return to live UDP
GET  /api/lidar/snapshot            - Retrieve latest background snapshot
POST /api/lidar/snapshot/persist    - Force immediate snapshot
```

### ❌ Missing Components

#### 1. Foreground Point Extraction
**Current State:** `ProcessFramePolar()` classifies each point as foreground or background but **does not extract/filter** the foreground points.

**Location:** `internal/lidar/background.go:ProcessFramePolar()`

**What Exists:**
```go
// Classification happens here
isBackground := (cellDiff <= closenessThreshold) || (neighborConfirm >= requiredNeighbors)

if isBackground {
    atomic.AddInt64(&g.BackgroundCount, 1)
    // Update cell EMA...
} else {
    atomic.AddInt64(&g.ForegroundCount, 1)
    // BUT: Point is NOT extracted or passed to clustering
}
```

**What's Missing:** A collection mechanism to gather foreground points and pass them to the clustering stage.

#### 2. Spatial Clustering
**Status:** Not implemented  
**Design:** DBSCAN algorithm planned (eps ~0.6m, minPts ~12)  
**Location:** `internal/lidar/arena.go` has data structures but no clustering logic

#### 3. Multi-Object Tracking
**Status:** Not implemented  
**Design:** Constant-velocity Kalman filter with Mahalanobis association  
**Location:** `internal/lidar/arena.go` has `Track` and `TrackState2D` structures defined

#### 4. Track Visualization UI
**Status:** Not implemented  
**Current Web UI:** `web/src/` has radar stats visualization but no LIDAR track components

#### 5. Track Storage Schema
**Status:** Designed but not migrated  
**Tables Needed:** `lidar_clusters`, `lidar_tracks`, `lidar_track_obs`  
**Current Schema:** Only `lidar_bg_snapshot` table exists (from migration 000003)

---

## Architecture Analysis

### Data Flow (Current vs. Planned)

#### Current Pipeline
```
UDP Packets (Hesai P40)
    ↓
Packet Decoder (parse/extract.go)
    ↓
FrameBuilder (frame_builder.go) - Accumulates 360° rotation
    ↓
ProcessFramePolar (background.go) - Classifies each point
    ↓
[STOPS HERE - No foreground extraction]
```

#### Planned Complete Pipeline
```
UDP Packets
    ↓
Packet Decoder
    ↓
FrameBuilder → Complete Frame (PointPolar[])
    ↓
ProcessFramePolar → Foreground Mask (bool[])
    ↓
[NEW] ExtractForegroundPoints → Filtered PointPolar[]
    ↓
[NEW] TransformToWorld → Point[] (Cartesian world frame)
    ↓
[NEW] DBSCAN Clustering → WorldCluster[]
    ↓
[NEW] Kalman Tracking → Track[] + TrackObs[]
    ↓
[NEW] Persist to DB → lidar_clusters, lidar_tracks, lidar_track_obs
    ↓
[NEW] HTTP API → JSON responses
    ↓
[NEW] Web UI → Real-time visualization
```

### Key Design Decisions

#### 1. Sensor Frame vs. World Frame Processing
- **Background subtraction:** Sensor frame (stable geometry)
- **Clustering:** World frame (Cartesian coordinates, consistent scale)
- **Tracking:** World frame (stable reference for velocity estimation)

**Rationale:** Background grid is sensor-centric (polar coordinates), but clustering and tracking require Euclidean distances in a fixed reference frame.

#### 2. Performance Targets
- **Latency:** <100ms end-to-end (packet → track update)
- **Throughput:** 10 Hz LIDAR frame rate (typical Pandar40P)
- **Capacity:** 100 concurrent tracks maximum
- **Memory:** <300MB with full track history

#### 3. Thread Safety
- **Background Grid:** RWMutex for concurrent access during persistence
- **SidecarState:** RWMutex for track map and ring buffers
- **Ring Buffers:** Individual mutexes for cluster/observation queues

---

## Phase 2.9: Foreground Point Extraction

### Objective
Extract points classified as foreground from `ProcessFramePolar()` and prepare them for clustering.

### Implementation Overview

The current `ProcessFramePolar()` function classifies each point but doesn't collect the foreground results. We need to:

1. Modify the function to return foreground points
2. Update the frame processing callback to handle foreground points
3. Add foreground statistics to monitoring

### Detailed Implementation Steps

#### Step 1: Modify ProcessFramePolar Signature

**File:** `internal/lidar/background.go`

**Current:**
```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) error
```

**New:**
```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) (foregroundPoints []PointPolar, err error)
```

**Implementation Changes:**
```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) ([]PointPolar, error) {
    if bm == nil || bm.Grid == nil {
        return nil, fmt.Errorf("background manager or grid nil")
    }
    
    g := bm.Grid
    g.mu.Lock()
    defer g.mu.Unlock()
    
    // Pre-allocate with estimated 10% foreground rate
    foregroundPoints := make([]PointPolar, 0, len(points)/10)
    
    for _, p := range points {
        // ... existing ring/azimuth bin calculation ...
        
        // ... existing classification logic ...
        isBackground := (cellDiff <= closenessThreshold) || (neighborConfirm >= requiredNeighbors)
        
        if !isBackground {
            atomic.AddInt64(&g.ForegroundCount, 1)
            foregroundPoints = append(foregroundPoints, p)
        } else {
            atomic.AddInt64(&g.BackgroundCount, 1)
            // ... existing EMA update logic ...
        }
    }
    
    return foregroundPoints, nil
}
```

#### Step 2: Update Frame Processing Callback

The frame callback needs to be modified to:
1. Call `ProcessFramePolar()` and receive foreground points
2. Pass foreground points to the clustering pipeline
3. Update frame statistics

**Location:** Wherever the FrameBuilder callback is registered (likely in `cmd/radar/radar.go` or a lidar initialization function)

**Example:**
```go
func onFrameComplete(frame *LidarFrame) {
    startTime := time.Now()
    
    // Extract polar points from frame
    polarPoints := frame.GetPolarPoints()
    
    // Process through background manager
    foregroundPoints, err := backgroundManager.ProcessFramePolar(polarPoints)
    if err != nil {
        log.Printf("[ERROR] ProcessFramePolar failed: %v", err)
        return
    }
    
    // Update frame statistics
    frameStats := &FrameStats{
        TSUnixNanos:      frame.Timestamp.UnixNano(),
        PacketsReceived:  frame.PacketCount,
        PointsTotal:      len(polarPoints),
        ForegroundPoints: len(foregroundPoints),
        ProcessingTimeUs: time.Since(startTime).Microseconds(),
    }
    
    // TODO: Pass foreground points to clustering stage (Phase 3.1)
    if len(foregroundPoints) > 0 {
        // clusteringPipeline.ProcessPoints(foregroundPoints)
        log.Printf("[DEBUG] Frame has %d foreground points", len(foregroundPoints))
    }
    
    // Emit frame stats to monitoring/database
    emitFrameStats(frameStats)
}
```

#### Step 3: Add Monitoring and Diagnostics

Update the HTTP monitoring endpoints to include foreground extraction metrics:

**File:** `internal/lidar/monitor/webserver.go`

Add foreground extraction stats to the status page:
```go
type StatusResponse struct {
    // ... existing fields ...
    ForegroundPointsLastFrame int   `json:"foreground_points_last_frame"`
    ForegroundRatePercent     float64 `json:"foreground_rate_percent"`
}
```

### Testing Requirements

#### Unit Tests
**File:** `internal/lidar/background_test.go`

```go
func TestProcessFramePolar_ForegroundExtraction(t *testing.T) {
    // Setup: Create background manager with known grid
    bm := NewBackgroundManager(...)
    
    // Populate grid with stable background at 10m distance
    seedGridWithBackground(bm.Grid, 10.0)
    
    // Test: Add points at 5m (foreground) and 10m (background)
    points := []PointPolar{
        {Distance: 5.0, Azimuth: 0, Ring: 0},  // Foreground
        {Distance: 10.0, Azimuth: 0, Ring: 0}, // Background
    }
    
    foreground, err := bm.ProcessFramePolar(points)
    
    // Verify: Should extract only the 5m point
    assert.NoError(t, err)
    assert.Equal(t, 1, len(foreground))
    assert.Equal(t, 5.0, foreground[0].Distance)
}
```

#### Integration Tests
**File:** `internal/lidar/integration_test.go`

Use existing PCAP files to verify foreground extraction:
```go
func TestForegroundExtraction_PCAP(t *testing.T) {
    // Load PCAP with known moving objects
    pcapPath := "testdata/cars.pcap"
    
    // Process frames and collect foreground points
    totalForeground := 0
    frameCallback := func(frame *LidarFrame) {
        fg, _ := bm.ProcessFramePolar(frame.GetPolarPoints())
        totalForeground += len(fg)
    }
    
    // Process PCAP
    processPCAP(pcapPath, frameCallback)
    
    // Verify: Should detect foreground points (exact count depends on PCAP)
    assert.Greater(t, totalForeground, 0)
}
```

### Performance Validation

Add benchmarks to ensure <1ms overhead:

```go
func BenchmarkProcessFramePolar_ForegroundExtraction(b *testing.B) {
    bm := setupBenchmarkManager()
    points := generateTestPoints(10000) // Typical frame size
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = bm.ProcessFramePolar(points)
    }
}
```

**Expected Result:** <1ms per frame on typical hardware

---

## Phase 3.1: Spatial Clustering

### Objective
Implement DBSCAN clustering on foreground points to detect distinct objects.

### Algorithm: DBSCAN (Density-Based Spatial Clustering)

#### Parameters
- **eps (epsilon):** 0.6 meters - maximum distance between neighbors
- **minPts:** 12 points - minimum points to form a cluster
- **Distance Metric:** Euclidean distance in world frame

#### Why DBSCAN?
- **Handles arbitrary shapes:** Unlike k-means, works for non-spherical objects
- **Automatic outlier detection:** Noise points not assigned to clusters
- **No predefined cluster count:** Discovers number of objects automatically
- **Efficient for spatial data:** O(n log n) with spatial indexing

### Implementation Steps

#### Step 1: Transform Points to World Frame

**New File:** `internal/lidar/clustering.go`

```go
package lidar

import (
    "math"
    "time"
)

// WorldPoint represents a point in Cartesian world coordinates
type WorldPoint struct {
    X, Y, Z       float64
    Intensity     uint8
    Timestamp     time.Time
    OriginalIndex int
}

// TransformToWorld converts polar sensor frame to Cartesian world frame
func TransformToWorld(polarPoints []PointPolar, pose *Pose) []WorldPoint {
    worldPoints := make([]WorldPoint, len(polarPoints))
    
    for i, p := range polarPoints {
        // Convert polar to Cartesian (sensor frame)
        cosElev := math.Cos(p.Elevation * math.Pi / 180)
        sensorX := p.Distance * math.Cos(p.Azimuth*math.Pi/180) * cosElev
        sensorY := p.Distance * math.Sin(p.Azimuth*math.Pi/180) * cosElev
        sensorZ := p.Distance * math.Sin(p.Elevation*math.Pi/180)
        
        // Apply pose transform (4x4 homogeneous matrix)
        // pose.T is row-major: [m00 m01 m02 m03, m10 m11 m12 m13, ...]
        worldX := pose.T[0]*sensorX + pose.T[1]*sensorY + pose.T[2]*sensorZ + pose.T[3]
        worldY := pose.T[4]*sensorX + pose.T[5]*sensorY + pose.T[6]*sensorZ + pose.T[7]
        worldZ := pose.T[8]*sensorX + pose.T[9]*sensorY + pose.T[10]*sensorZ + pose.T[11]
        
        worldPoints[i] = WorldPoint{
            X:             worldX,
            Y:             worldY,
            Z:             worldZ,
            Intensity:     p.Intensity,
            Timestamp:     p.Timestamp,
            OriginalIndex: i,
        }
    }
    
    return worldPoints
}
```

#### Step 2: Implement DBSCAN

**File:** `internal/lidar/clustering.go` (continued)

```go
// DBSCAN performs density-based clustering
func DBSCAN(points []WorldPoint, eps float64, minPts int) []WorldCluster {
    n := len(points)
    labels := make([]int, n) // 0=unvisited, -1=noise, >0=clusterID
    clusterID := 0
    
    for i := 0; i < n; i++ {
        if labels[i] != 0 {
            continue
        }
        
        neighbors := regionQuery(points, i, eps)
        
        if len(neighbors) < minPts {
            labels[i] = -1 // Noise
            continue
        }
        
        clusterID++
        expandCluster(points, labels, i, neighbors, clusterID, eps, minPts)
    }
    
    return buildClusters(points, labels, clusterID)
}

// regionQuery finds all neighbors within eps distance
func regionQuery(points []WorldPoint, i int, eps float64) []int {
    neighbors := []int{}
    p := points[i]
    eps2 := eps * eps // Compare squared distances
    
    for j := range points {
        dx := points[j].X - p.X
        dy := points[j].Y - p.Y
        dz := points[j].Z - p.Z
        dist2 := dx*dx + dy*dy + dz*dz
        
        if dist2 <= eps2 {
            neighbors = append(neighbors, j)
        }
    }
    
    return neighbors
}

// expandCluster grows cluster via density-reachability
func expandCluster(points []WorldPoint, labels []int, i int, neighbors []int, 
                   clusterID int, eps float64, minPts int) {
    labels[i] = clusterID
    
    for j := 0; j < len(neighbors); j++ {
        idx := neighbors[j]
        
        if labels[idx] == -1 {
            labels[idx] = clusterID // Noise → border point
        }
        
        if labels[idx] != 0 {
            continue
        }
        
        labels[idx] = clusterID
        newNeighbors := regionQuery(points, idx, eps)
        
        if len(newNeighbors) >= minPts {
            neighbors = append(neighbors, newNeighbors...)
        }
    }
}
```

#### Step 3: Compute Cluster Metrics

**File:** `internal/lidar/clustering.go` (continued)

```go
// buildClusters creates WorldCluster objects with metrics
func buildClusters(points []WorldPoint, labels []int, maxClusterID int) []WorldCluster {
    clusters := make([]WorldCluster, 0, maxClusterID)
    
    for cid := 1; cid <= maxClusterID; cid++ {
        clusterPoints := []WorldPoint{}
        for i, label := range labels {
            if label == cid {
                clusterPoints = append(clusterPoints, points[i])
            }
        }
        
        if len(clusterPoints) == 0 {
            continue
        }
        
        cluster := computeClusterMetrics(clusterPoints)
        clusters = append(clusters, cluster)
    }
    
    return clusters
}

// computeClusterMetrics calculates centroid, bbox, height, intensity
func computeClusterMetrics(points []WorldPoint) WorldCluster {
    n := float32(len(points))
    
    // Centroid
    var sumX, sumY, sumZ float64
    for _, p := range points {
        sumX += p.X
        sumY += p.Y
        sumZ += p.Z
    }
    
    // Bounding box
    minX, maxX := points[0].X, points[0].X
    minY, maxY := points[0].Y, points[0].Y
    minZ, maxZ := points[0].Z, points[0].Z
    
    var sumIntensity uint64
    heights := make([]float64, len(points))
    
    for i, p := range points {
        if p.X < minX { minX = p.X }
        if p.X > maxX { maxX = p.X }
        if p.Y < minY { minY = p.Y }
        if p.Y > maxY { maxY = p.Y }
        if p.Z < minZ { minZ = p.Z }
        if p.Z > maxZ { maxZ = p.Z }
        sumIntensity += uint64(p.Intensity)
        heights[i] = p.Z
    }
    
    // P95 height
    sort.Float64s(heights)
    p95Index := int(0.95 * float64(len(heights)))
    if p95Index >= len(heights) {
        p95Index = len(heights) - 1
    }
    
    return WorldCluster{
        TSUnixNanos:       points[0].Timestamp.UnixNano(),
        CentroidX:         float32(sumX / float64(n)),
        CentroidY:         float32(sumY / float64(n)),
        CentroidZ:         float32(sumZ / float64(n)),
        BoundingBoxLength: float32(maxX - minX),
        BoundingBoxWidth:  float32(maxY - minY),
        BoundingBoxHeight: float32(maxZ - minZ),
        PointsCount:       len(points),
        HeightP95:         float32(heights[p95Index]),
        IntensityMean:     float32(sumIntensity / uint64(len(points))),
    }
}
```

### Performance Optimization

For better performance, implement spatial indexing:

**File:** `internal/lidar/spatial_index.go` (optional)

```go
// GridIndex provides O(1) neighbor lookup for DBSCAN
type GridIndex struct {
    CellSize float64
    Grid     map[int64][]int // Grid cell → point indices
}

// Build creates spatial index
func (gi *GridIndex) Build(points []WorldPoint) {
    gi.Grid = make(map[int64][]int)
    
    for i, p := range points {
        cellID := gi.getCellID(p.X, p.Y, p.Z)
        gi.Grid[cellID] = append(gi.Grid[cellID], i)
    }
}

// Query returns points within eps distance (uses neighboring cells)
func (gi *GridIndex) Query(points []WorldPoint, i int, eps float64) []int {
    // Implementation omitted for brevity
    // Query 3x3x3 neighboring cells and filter by distance
}
```

### Testing Requirements

#### Unit Tests
```go
func TestDBSCAN_SyntheticClusters(t *testing.T) {
    // Create 2 distinct clusters
    cluster1 := generateSphere(0, 0, 0, 0.5, 50) // 50 points at origin
    cluster2 := generateSphere(5, 0, 0, 0.5, 50) // 50 points at x=5m
    
    allPoints := append(cluster1, cluster2...)
    
    clusters := DBSCAN(allPoints, 0.6, 12)
    
    // Should detect exactly 2 clusters
    assert.Equal(t, 2, len(clusters))
}
```

---

## Phase 3.2: Multi-Object Tracking

### Objective
Implement Kalman filter-based tracking to maintain object identity across frames.

### Algorithm: Constant-Velocity Kalman Filter

#### State Vector
```
x = [x, y, vx, vy]^T
```

#### Motion Model (Prediction)
```
x_k+1 = F * x_k + w_k

F = [1  0  dt  0 ]
    [0  1  0  dt ]
    [0  0  1   0 ]
    [0  0  0   1 ]
```

#### Measurement Model (Update)
```
z_k = H * x_k + v_k

H = [1  0  0  0]
    [0  1  0  0]
```

### Implementation Steps

#### Step 1: Tracker Initialization

**New File:** `internal/lidar/tracking.go`

```go
package lidar

import (
    "fmt"
    "math"
    "sync"
    "time"
)

// Tracker manages all active tracks
type Tracker struct {
    Tracks             map[string]*Track
    NextTrackID        int64
    MaxTracks          int
    MaxMisses          int
    GatingDistance     float32
    ProcessNoise       [4]float32
    MeasurementNoise   [2]float32
    mu                 sync.RWMutex
}

// NewTracker creates tracker with default parameters
func NewTracker() *Tracker {
    return &Tracker{
        Tracks:           make(map[string]*Track),
        NextTrackID:      1,
        MaxTracks:        100,
        MaxMisses:        3,
        GatingDistance:   5.0,
        ProcessNoise:     [4]float32{0.1, 0.1, 0.5, 0.5},
        MeasurementNoise: [2]float32{0.2, 0.2},
    }
}

// Update processes new clusters
func (t *Tracker) Update(clusters []WorldCluster, timestamp time.Time) {
    t.mu.Lock()
    defer t.mu.Unlock()
    
    // Predict all tracks
    for _, track := range t.Tracks {
        t.predict(track, timestamp)
    }
    
    // Associate clusters to tracks
    associations := t.associate(clusters)
    
    // Update matched tracks
    for clusterIdx, trackID := range associations {
        if trackID != "" {
            track := t.Tracks[trackID]
            t.update(track, clusters[clusterIdx])
            track.Misses = 0
        }
    }
    
    // Handle unmatched tracks
    for trackID, track := range t.Tracks {
        if !trackWasMatched(trackID, associations) {
            track.Misses++
            if track.Misses >= t.MaxMisses {
                delete(t.Tracks, trackID)
            }
        }
    }
    
    // Initialize new tracks
    for clusterIdx, trackID := range associations {
        if trackID == "" && len(t.Tracks) < t.MaxTracks {
            t.initTrack(clusters[clusterIdx])
        }
    }
}

// predict propagates state forward
func (t *Tracker) predict(track *Track, timestamp time.Time) {
    dt := float32(timestamp.Sub(time.Unix(0, track.LastUnixNanos)).Seconds())
    if dt <= 0 {
        return
    }
    
    // x' = F * x
    track.State.X += track.State.VelocityX * dt
    track.State.Y += track.State.VelocityY * dt
    
    // P' = F * P * F^T + Q
    t.predictCovariance(track, dt)
}

// update corrects state with measurement
func (t *Tracker) update(track *Track, cluster WorldCluster) {
    // Innovation
    innovX := cluster.CentroidX - track.State.X
    innovY := cluster.CentroidY - track.State.Y
    
    // Kalman gain
    K := t.computeKalmanGain(track)
    
    // State update
    track.State.X += K[0] * innovX
    track.State.Y += K[1] * innovY
    track.State.VelocityX += K[2] * innovX
    track.State.VelocityY += K[3] * innovY
    
    // Covariance update
    t.updateCovariance(track, K)
    
    track.LastUnixNanos = cluster.TSUnixNanos
    track.ObservationCount++
}
```

#### Step 2: Data Association

**File:** `internal/lidar/tracking.go` (continued)

```go
// associate matches clusters to tracks using Mahalanobis distance
func (t *Tracker) associate(clusters []WorldCluster) map[int]string {
    // Build cost matrix
    costMatrix := t.buildCostMatrix(clusters)
    
    // Solve assignment (greedy or Hungarian)
    assignments := greedyAssignment(costMatrix)
    
    return assignments
}

// buildCostMatrix computes Mahalanobis distances
func (t *Tracker) buildCostMatrix(clusters []WorldCluster) [][]float32 {
    nClusters := len(clusters)
    trackIDs := make([]string, 0, len(t.Tracks))
    for id := range t.Tracks {
        trackIDs = append(trackIDs, id)
    }
    nTracks := len(trackIDs)
    
    matrix := make([][]float32, nClusters)
    for i, cluster := range clusters {
        matrix[i] = make([]float32, nTracks)
        for j, trackID := range trackIDs {
            track := t.Tracks[trackID]
            dist := t.mahalanobisDistance(track, cluster)
            
            if dist > t.GatingDistance {
                matrix[i][j] = 1e9 // Gating
            } else {
                matrix[i][j] = dist
            }
        }
    }
    
    return matrix
}
```

### Testing Requirements

Test Kalman filter with synthetic trajectories:

```go
func TestTracking_StraightLine(t *testing.T) {
    tracker := NewTracker()
    
    // Simulate object moving at constant velocity
    for i := 0; i < 10; i++ {
        cluster := WorldCluster{
            CentroidX: float32(i),
            CentroidY: 0,
            TSUnixNanos: int64(i * 1e8),
        }
        
        tracker.Update([]WorldCluster{cluster}, time.Unix(0, cluster.TSUnixNanos))
    }
    
    // Should have 1 track
    assert.Equal(t, 1, len(tracker.Tracks))
    
    // Velocity should be ~1 m/s
    for _, track := range tracker.Tracks {
        assert.InDelta(t, 1.0, track.State.VelocityX, 0.2)
    }
}
```

---

## Phase 3.3: UI & SQL Integration

### SQL Schema

#### Migration: 000009_create_lidar_tracks.up.sql

```sql
CREATE TABLE IF NOT EXISTS lidar_clusters (
    lidar_cluster_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    pose_id INTEGER NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    centroid_x REAL,
    centroid_y REAL,
    centroid_z REAL,
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL,
    points_count INTEGER,
    height_p95 REAL,
    intensity_mean REAL
);

CREATE INDEX idx_clusters_sensor_time ON lidar_clusters(sensor_id, ts_unix_nanos);

CREATE TABLE IF NOT EXISTS lidar_tracks (
    track_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    pose_id INTEGER NOT NULL,
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,
    avg_speed_mps REAL,
    peak_speed_mps REAL
);

CREATE INDEX idx_tracks_sensor ON lidar_tracks(sensor_id);

CREATE TABLE IF NOT EXISTS lidar_track_obs (
    track_id TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    x REAL,
    y REAL,
    velocity_x REAL,
    velocity_y REAL,
    speed_mps REAL,
    PRIMARY KEY (track_id, ts_unix_nanos),
    FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id)
);

CREATE INDEX idx_track_obs_track ON lidar_track_obs(track_id);
```

### REST API Endpoints

```go
// GET /api/lidar/tracks?sensor_id=<id>&active=true
func HandleGetTracks(w http.ResponseWriter, r *http.Request) {
    // Query active or historical tracks
}

// GET /api/lidar/tracks/:track_id/observations
func HandleGetTrackObservations(w http.ResponseWriter, r *http.Request) {
    // Return trajectory for visualization
}
```

### Web UI Components

**New File:** `web/src/lib/components/TrackVisualization.svelte`

```svelte
<script lang="ts">
    import { onMount } from 'svelte';
    
    let tracks = [];
    
    onMount(async () => {
        // Poll for active tracks
        setInterval(async () => {
            const res = await fetch('/api/lidar/tracks?active=true');
            tracks = await res.json();
        }, 1000);
    });
</script>

<div class="track-map">
    {#each tracks as track}
        <div class="track" style="left: {track.x}px; top: {track.y}px">
            Track {track.track_id}
        </div>
    {/each}
</div>
```

---

## Implementation Roadmap

### Timeline Estimate

| Phase | Task | Duration | Dependencies |
|-------|------|----------|--------------|
| 2.9   | Foreground Extraction | 1-2 days | None |
| 3.1   | Clustering | 3-5 days | Phase 2.9 |
| 3.2   | Tracking | 5-7 days | Phase 3.1 |
| 3.3   | UI & SQL | 4-6 days | Phase 3.2 |
| Test  | Integration Testing | 2-3 days | All phases |

**Total: 15-23 days**

### Milestones

1. ✅ **Background Learning Complete** (Already done)
2. 🎯 **Foreground Extraction Working** - Can extract and count foreground points
3. 🎯 **Clustering Operational** - Can detect distinct objects in scene
4. 🎯 **Tracking Functional** - Can maintain track identity across frames
5. 🎯 **UI Live** - Can visualize tracks in real-time
6. 🎯 **Production Ready** - All tests passing, documented, deployed

---

## Performance Considerations

### Target Metrics
- **End-to-end latency:** <100ms (95th percentile)
- **Frame rate:** 10 Hz sustained
- **Memory:** <300MB with 100 tracks
- **CPU:** <20% on Raspberry Pi 4

### Optimization Strategies
- Spatial indexing for DBSCAN (k-d tree or grid)
- Parallel frame processing
- Ring buffers for observations
- Async database persistence

---

## Testing Strategy

### Test Pyramid
1. **Unit Tests** (80%+)  - Individual functions
2. **Integration Tests** (PCAP) - Full pipeline
3. **Performance Tests** - Latency/throughput
4. **Manual Testing** - Live sensor validation

### Acceptance Criteria
- ✅ Foreground extraction: >95% precision
- ✅ Clustering: Detect objects with <10% false positives
- ✅ Tracking: Maintain identity for >90% of trajectory
- ✅ Latency: <100ms end-to-end

---

## Appendix

### A. Key Data Structures

See `internal/lidar/arena.go` for complete definitions:
- `PointPolar` - Polar sensor measurements
- `WorldPoint` - Cartesian world coordinates  
- `WorldCluster` - Detected objects
- `Track` - Tracked object state
- `TrackObs` - Individual observations

### B. Configuration Parameters

**Clustering:**
- `EpsilonMeters = 0.6`
- `MinPoints = 12`

**Tracking:**
- `MaxTracks = 100`
- `MaxMisses = 3`
- `GatingDistance = 5.0`

### C. Related Documentation

- `ARCHITECTURE.md` - System overview
- `internal/lidar/docs/lidar_sidecar_overview.md` - LIDAR details
- `internal/lidar/docs/devlog.md` - Development history

---

**Document Status:** Complete  
**Next Action:** Begin Phase 2.9 Implementation  
**Contact:** Engineering Team
