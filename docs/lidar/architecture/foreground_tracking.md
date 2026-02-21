# LIDAR Foreground Extraction and Tracking Implementation Plan

Status: Active

**Status:** Implementation Complete through Phase 3.7
**Date:** January 2026
**Version:** 7.1 - Warmup sensitivity scaling added

---

## Executive Summary

This document provides a comprehensive implementation plan for LIDAR-based object detection and tracking with **explicit separation between polar-frame background processing and world-frame clustering/tracking**.

**Key Architectural Principle:** Background subtraction operates purely in sensor-centric polar coordinates (azimuth/elevation/range). Only after foreground extraction are points transformed to world-frame Cartesian coordinates for clustering, tracking, and persistence.

**Implementation Phases:**

- **Phase 2.9:** ✅ Foreground mask generation (polar frame)
- **Phase 3.0:** ✅ Polar → World coordinate transformation
- **Phase 3.1:** ✅ DBSCAN clustering (world frame)
- **Phase 3.2:** ✅ Kalman filter tracking (world frame)
- **Phase 3.3:** ✅ SQL schema & database persistence
- **Phase 3.4:** ✅ Track-level classification
- **Phase 3.5:** ✅ REST API endpoints
- **Phase 3.6:** ✅ PCAP Analysis Tool for ML data extraction
- **Phase 3.7:** ✅ Analysis Run Infrastructure (params JSON, run comparison)

**Next Phases:** See [ML Pipeline Roadmap](../ROADMAP.md) for Phases 4.0-4.3.

---

## Table of Contents

1. [Current State Assessment](#current-state-assessment)
2. [Architecture: Polar vs World Frame](#architecture-polar-vs-world-frame)
3. [Phase 2.9: Foreground Mask Generation (Polar)](#phase-29-foreground-mask-generation-polar)
4. [Phase 3.0: Polar → World Transform](#phase-30-polar--world-transform)
5. [Phase 3.1: DBSCAN Clustering (World Frame)](#phase-31-dbscan-clustering-world-frame)
6. [Phase 3.2: Kalman Tracking (World Frame)](#phase-32-kalman-tracking-world-frame)
7. [Phase 3.3: SQL Schema & REST APIs](#phase-33-sql-schema--rest-apis)
8. [Phase 3.4: Track Classification](#phase-34-track-classification)
9. [Phase 3.5: REST API Endpoints](#phase-35-rest-api-endpoints)
10. [Phase 3.6: PCAP Analysis Tool](#phase-36-pcap-analysis-tool)
11. [Phase 3.7: Analysis Run Infrastructure](#phase-37-analysis-run-infrastructure)
12. [Performance & Concurrency](#performance--concurrency)
13. [Testing Strategy](#testing-strategy)
14. [Implementation Roadmap](#implementation-roadmap)
15. [Appendix](#appendix)
    - [A. Data Structures](#a-data-structures)
    - [B. Configuration Parameters](#b-configuration-parameters)
    - [C. Related Documentation](#c-related-documentation)
    - [D. ML Training Data Storage](#d-ml-training-data-storage)
    - [E. Future Work: Pose Validation](#e-future-work-pose-validation)

---

## Current State Assessment

### ✅ Completed (Phase 1 & 2)

#### Background Grid Infrastructure (Polar Frame)

- **Grid Structure:** 40 rings × 1800 azimuth bins (0.2° resolution) = 72,000 cells
- **Coordinate System:** **Purely polar** (ring index, azimuth bin, range in meters)
- **Learning Algorithm:** Exponential Moving Average (EMA) for range/spread tracking
- **Classification:** Distance-adaptive threshold with same-ring neighbor voting
- **Persistence:** Automatic snapshots to `lidar_bg_snapshot` table
- **Location:** `internal/lidar/background.go`

**Critical Constraint:** Background grid **never** stores or uses Cartesian/world coordinates. All EMA updates, neighbor voting, and classification occur in polar space.

#### Current Capabilities

- ✅ UDP packet ingestion (Hesai Pandar40P)
- ✅ Frame assembly (360° rotations)
- ✅ Background learning (EMA-based grid)
- ✅ Foreground/background classification (per-point in polar)
- ✅ PCAP replay with parameter tuning
- ✅ HTTP APIs for monitoring and control

### ✅ Completed (Phases 2.9 - 3.2)

#### Phase 2.9: Foreground Mask Generation (Polar Frame)

- **Implementation:** `internal/lidar/foreground.go`
- ✅ `ProcessFramePolarWithMask()` - per-point foreground/background classification returning mask
- ✅ **Warmup Sensitivity Scaling** - dynamic threshold multiplier during cell initialisation (4x→1x over 100 observations)
- ✅ `ExtractForegroundPoints()` - helper to filter foreground points from mask
- ✅ `ComputeFrameMetrics()` - frame-level statistics (total, foreground, background counts)
- ✅ Unit tests in `internal/lidar/foreground_test.go` and `internal/lidar/foreground_warmup_test.go`

#### Phase 3.0: Polar → World Transform

- **Implementation:** `internal/lidar/clustering.go`
- ✅ `WorldPoint` struct for world-frame coordinates
- ✅ `TransformToWorld()` - converts polar points to world frame
- ✅ `TransformPointsToWorld()` - convenience function for pre-computed Cartesian points
- ✅ Identity transform used by default (sensor frame = world frame)
- ✅ Unit tests for coordinate transformation accuracy

> **Note:** Pose-based transformations are deferred to a future phase. Currently, sensor frame coordinates are used directly as world frame coordinates.

#### Phase 3.1: DBSCAN Clustering (World Frame)

- **Implementation:** `internal/lidar/clustering.go`
- ✅ `SpatialIndex` struct with grid-based indexing using Szudzik pairing
- ✅ `DBSCAN()` - density-based clustering with spatial index
- ✅ `computeClusterMetrics()` - centroid, bounding box, height P95, intensity mean
- ✅ `WorldCluster` struct with all required features
- ✅ Unit tests in `internal/lidar/clustering_test.go`

#### Phase 3.2: Kalman Tracking (World Frame)

- **Implementation:** `internal/lidar/tracking.go`
- ✅ `TrackState` lifecycle: Tentative → Confirmed → Deleted
- ✅ `TrackedObject` struct with Kalman state and aggregated features
- ✅ `Tracker` with configurable parameters via `TrackerConfig`
- ✅ Mahalanobis distance gating for association
- ✅ Kalman predict/update with constant velocity model
- ✅ Track lifecycle management (hits/misses, promotion, deletion)
- ✅ Speed statistics (average, peak, history for percentiles)
- ✅ Unit tests in `internal/lidar/tracking_test.go`

#### ML Training Data Support

- **Implementation:** `internal/lidar/training_data.go`
- ✅ `ForegroundFrame` struct for exporting foreground points
- ✅ `EncodeForegroundBlob()`/`DecodeForegroundBlob()` - compact binary encoding (8 bytes/point)
- ✅ `TrainingDataFilter` for filtering training data
- ✅ Unit tests for training data encoding

> **Future Work:** Pose validation and quality assessment will be implemented in a future phase.
> The current implementation stores training data in polar (sensor) frame, which is pose-independent.
> This allows training data to remain valid even if the sensor pose changes.

### ✅ Completed (Phases 3.3 - 3.5)

#### Phase 3.3: SQL Schema & Database Persistence

- **Implementation:** `internal/db/migrations/000009_create_lidar_tracks.up.sql`, `internal/lidar/track_store.go`
- ✅ `lidar_clusters` table for DBSCAN cluster persistence
- ✅ `lidar_tracks` table for track lifecycle and aggregated features
- ✅ `lidar_track_obs` table for per-observation tracking data
- ✅ `InsertCluster()`, `InsertTrack()`, `UpdateTrack()` database functions
- ✅ `GetActiveTracks()`, `GetTrackObservations()`, `GetRecentClusters()` query functions
- ✅ Unit tests in `internal/lidar/track_store_test.go`

#### Phase 3.4: Track Classification

- **Implementation:** `internal/lidar/classification.go`
- ✅ `TrackClassifier` with rule-based classification
- ✅ Object classes: pedestrian, car, bird, other
- ✅ Classification features: height, length, width, speed, duration
- ✅ Configurable thresholds for each class
- ✅ `ClassifyAndUpdate()` for track classification integration
- ✅ `ComputeSpeedPercentiles()` for P50/P85/P95 speed computation
- ✅ Unit tests in `internal/lidar/classification_test.go`

#### Phase 3.5: REST API Endpoints

- **Implementation:** `internal/lidar/monitor/track_api.go`
- ✅ `TrackAPI` struct with HTTP handlers for track/cluster queries
- ✅ `GET /api/lidar/tracks` - List tracks with optional state filter
- ✅ `GET /api/lidar/tracks/active` - Active tracks (real-time from memory or DB)
- ✅ `GET /api/lidar/tracks/{track_id}` - Get specific track details
- ✅ `PUT /api/lidar/tracks/{track_id}` - Update track metadata (class, confidence)
- ✅ `GET /api/lidar/tracks/{track_id}/observations` - Get track trajectory
- ✅ `GET /api/lidar/tracks/summary` - Aggregated statistics by class/state
- ✅ `GET /api/lidar/clusters` - Recent clusters by time range
- ✅ Unit tests in `internal/lidar/monitor/track_api_test.go`
- ✅ **UI visualisation** - Track history playback with MapPane, TrackList, TimelinePane components

### 📋 Remaining Components

1. **Track Labeling UI** - Manual annotation interface for ML training (Phase 4.0)

---

## Architecture: Polar vs World Frame

### Coordinate System Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│                    POLAR FRAME (Sensor-Centric)                 │
│                                                                 │
│  • Background Grid (40 rings × 1800 azimuth bins)               │
│  • EMA Learning (range, spread per cell)                        │
│  • Foreground/Background Classification                         │
│  • Neighbor Voting (same-ring only)                             │
│                                                                 │
│  Coordinates: (ring, azimuth_deg, range_m)                      │
│                                                                 │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         │ Phase 3.0: Transform
                         │ Input: Foreground polar points + Pose
                         │ Output: World Cartesian points
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                   WORLD FRAME (Site-Centric)                    │
│                                                                 │
│  • DBSCAN Clustering (Euclidean distance)                       │
│  • Kalman Tracking (position & velocity)                        │
│  • Track Classification (object type)                           │
│  • Database Persistence (clusters, tracks, observations)        │
│  • REST APIs (JSON responses)                                   │
│  • Web UI (visualization)                                       │
│                                                                 │
│  Coordinates: (x, y, z) meters in site frame                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow Pipeline

```
UDP Packets → Frame Builder → ProcessFramePolar → [Foreground Mask]
                                                         ↓
                                            Extract Foreground Points (polar)
                                                         ↓
                                              Transform to World Frame
                                                         ↓
                                                   DBSCAN Clustering
                                                         ↓
                                                  Kalman Tracking
                                                         ↓
                                              Track Classification
                                                         ↓
                                            Persist to Database (world)
                                                         ↓
                                              HTTP API / Web UI
```

### Key Design Decisions

1. **Background in Polar:** Stable sensor geometry, efficient ring-based neighbor queries
2. **Clustering in World:** Consistent Euclidean distances, stable reference frame
3. **Tracking in World:** Velocity estimation requires fixed coordinate system
4. **No Reverse Transform:** World frame components never convert back to polar

---

## Phase 2.9: Foreground Mask Generation (Polar)

### Objective

Generate per-point foreground/background classification mask in polar coordinates without extracting points.

### Changes to ProcessFramePolar

**Current Contract:**

```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) error
```

**New Contract:**

```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) (foregroundMask []bool, err error)
```

**Implementation:**

```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) ([]bool, error) {
    if bm == nil || bm.Grid == nil {
        return nil, fmt.Errorf("background manager or grid nil")
    }

    g := bm.Grid
    g.mu.Lock()
    defer g.mu.Unlock()

    // Allocate mask for all points
    foregroundMask := make([]bool, len(points))

    for i, p := range points {
        // Calculate ring and azimuth bin (polar coordinates only)
        ring := p.Ring
        azBin := int(p.Azimuth / 0.2) % 1800

        // Get cell from background grid
        cell := g.Cells[g.Idx(ring, azBin)]

        // Classify in polar space
        cellDiff := math.Abs(float64(p.Distance) - float64(cell.AverageRangeMeters))
        closenessThreshold := g.Params.ClosenessSensitivityMultiplier *
            (cell.RangeSpreadMeters + g.Params.NoiseRelativeFraction * p.Distance + 0.01) +
            g.Params.SafetyMarginMeters

        // Same-ring neighbor voting
        neighborConfirm := countSameRingNeighbors(g, ring, azBin, p.Distance)

        isBackground := (cellDiff <= float64(closenessThreshold)) ||
                       (neighborConfirm >= g.Params.NeighborConfirmationCount)

        foregroundMask[i] = !isBackground

        if isBackground {
            g.BackgroundCount++  // Protected by mutex, no atomic needed
            // Update EMA for background cells only
            updateCellEMA(cell, p.Distance)
        } else {
            g.ForegroundCount++  // Protected by mutex, no atomic needed
        }
    }

    return foregroundMask, nil
}
```

### Foreground Point Extraction (Outside Lock)

**After releasing background lock:**

```go
func extractForegroundPoints(allPoints []PointPolar, mask []bool) []PointPolar {
    foregroundPoints := make([]PointPolar, 0, len(allPoints)/10)

    for i, isForeground := range mask {
        if isForeground {
            foregroundPoints = append(foregroundPoints, allPoints[i])
        }
    }

    return foregroundPoints
}
```

### Frame Processing Callback

```go
func onFrameComplete(frame *LidarFrame) {
    startTime := time.Now()
    polarPoints := frame.GetPolarPoints()

    // Step 1: Classify in polar (with background lock)
    foregroundMask, err := backgroundManager.ProcessFramePolar(polarPoints)
    if err != nil {
        log.Printf("[ERROR] ProcessFramePolar: %v", err)
        return
    }

    // Step 2: Extract foreground points (outside lock)
    foregroundPolar := extractForegroundPoints(polarPoints, foregroundMask)

    // Step 3: Emit metrics
    foregroundCount := len(foregroundPolar)
    totalCount := len(polarPoints)
    foregroundFraction := float64(foregroundCount) / float64(totalCount)

    emitFrameMetrics(FrameMetrics{
        TotalPoints:         totalCount,
        ForegroundPoints:    foregroundCount,
        ForegroundFraction:  foregroundFraction,
        ProcessingTimeUs:    time.Since(startTime).Microseconds(),
    })

    // Step 4: Pass to transform stage (Phase 3.0)
    if len(foregroundPolar) > 0 {
        worldPoints := transformToWorld(foregroundPolar, currentPose)
        clusteringPipeline.Process(worldPoints)
    }
}
```

### Monitoring & Metrics

Add per-frame foreground metrics to HTTP status:

```go
type FrameMetrics struct {
    TotalPoints        int     `json:"total_points"`
    ForegroundPoints   int     `json:"foreground_points"`
    BackgroundPoints   int     `json:"background_points"`
    ForegroundFraction float64 `json:"foreground_fraction"`
    ProcessingTimeUs   int64   `json:"processing_time_us"`
}

// GET /api/lidar/frame_metrics?sensor_id=<id>
```

---

## Phase 3.0: Polar → World Transform

### Objective

Explicit coordinate transformation stage converting foreground polar points to world-frame Cartesian coordinates.

### Transform Stage Design

**Input:** `[]PointPolar` (foreground only) + `Pose` (sensor → world transform)
**Output:** `[]WorldPoint` (Cartesian coordinates in site frame)

**Responsibilities:**

1. Convert polar (distance, azimuth, elevation) → sensor Cartesian (x, y, z)
2. Apply pose transform: sensor frame → world frame
3. Attach metadata (timestamp, sensor_id, intensity)

**Does NOT:**

- Update background grid
- Perform clustering or tracking
- Store polar coordinates in output

### Implementation

```go
// WorldPoint represents a point in Cartesian world coordinates
type WorldPoint struct {
    X, Y, Z       float64   // World frame position (meters)
    Intensity     uint8     // Laser return intensity
    Timestamp     time.Time // Acquisition time
    SensorID      string    // Source sensor
}

// TransformToWorld converts foreground polar points to world frame
func TransformToWorld(polarPoints []PointPolar, pose *Pose) []WorldPoint {
    worldPoints := make([]WorldPoint, len(polarPoints))

    for i, p := range polarPoints {
        // Step 1: Polar → Sensor Cartesian
        cosElev := math.Cos(p.Elevation * math.Pi / 180)
        sensorX := p.Distance * math.Cos(p.Azimuth*math.Pi/180) * cosElev
        sensorY := p.Distance * math.Sin(p.Azimuth*math.Pi/180) * cosElev
        sensorZ := p.Distance * math.Sin(p.Elevation*math.Pi/180)

        // Step 2: Apply 4x4 homogeneous transform (sensor → world)
        // pose.T is row-major: [m00 m01 m02 m03, m10 m11 m12 m13, m20 m21 m22 m23, m30 m31 m32 m33]
        worldX := pose.T[0]*sensorX + pose.T[1]*sensorY + pose.T[2]*sensorZ + pose.T[3]
        worldY := pose.T[4]*sensorX + pose.T[5]*sensorY + pose.T[6]*sensorZ + pose.T[7]
        worldZ := pose.T[8]*sensorX + pose.T[9]*sensorY + pose.T[10]*sensorZ + pose.T[11]

        worldPoints[i] = WorldPoint{
            X:         worldX,
            Y:         worldY,
            Z:         worldZ,
            Intensity: p.Intensity,
            Timestamp: p.Timestamp,
            SensorID:  pose.SensorID,
        }
    }

    return worldPoints
}
```

### Testing Requirements

1. **Unit test:** Verify transform accuracy with known poses
2. **Integration test:** Compare transformed points against ground truth
3. **Validation:** Ensure no polar coordinates leak into world-frame structures

---

## Phase 3.1: DBSCAN Clustering (World Frame)

### Objective

Spatial clustering of foreground world points to detect distinct objects.

### Algorithm: DBSCAN with Required Spatial Index

**Parameters:**

- `eps = 0.6` meters (neighborhood radius)
- `minPts = 12` (minimum points per cluster)
- **Dimensionality:** 2D (x, y) clustering, with z used only for cluster features

**Rationale for 2D:**

- Ground-plane objects (vehicles, pedestrians) primarily distinguished by lateral position
- Vertical separation (z) used for height features after clustering
- Simplifies spatial index and reduces computational cost

**Spatial Index:** **Required** (not optional)

- Implementation: Regular grid with cell size ≈ `eps` (0.6m)
- Region queries examine only current cell + 8 neighbors (2D) or 26 neighbors (3D)
- Replaces O(n²) brute-force neighbor search

### Implementation

#### Spatial Index (Required)

```go
type SpatialIndex struct {
    CellSize float64
    Grid     map[int64][]int // Cell ID → point indices
}

func NewSpatialIndex(cellSize float64) *SpatialIndex {
    return &SpatialIndex{
        CellSize: cellSize,
        Grid:     make(map[int64][]int),
    }
}

func (si *SpatialIndex) Build(points []WorldPoint) {
    si.Grid = make(map[int64][]int, len(points)/4)

    for i, p := range points {
        cellID := si.getCellID(p.X, p.Y)
        si.Grid[cellID] = append(si.Grid[cellID], i)
    }
}

func (si *SpatialIndex) getCellID(x, y float64) int64 {
    cellX := int64(math.Floor(x / si.CellSize))
    cellY := int64(math.Floor(y / si.CellSize))
    // Szudzik's pairing function for signed integers
    // Maps signed integers to non-negative before pairing
    var a, b int64
    if cellX >= 0 {
        a = 2 * cellX
    } else {
        a = -2*cellX - 1
    }
    if cellY >= 0 {
        b = 2 * cellY
    } else {
        b = -2*cellY - 1
    }
    var pair int64
    if a >= b {
        pair = a*a + a + b
    } else {
        pair = a + b*b
    }
    // Ensure unique mapping for sign combinations
    if (cellX < 0) != (cellY < 0) {
        pair = -pair - 1
    }
    return pair
}

func (si *SpatialIndex) RegionQuery(points []WorldPoint, idx int, eps float64) []int {
    p := points[idx]
    neighbors := []int{}

    // Get neighboring cells (3x3 grid)
    cellX := int64(math.Floor(p.X / si.CellSize))
    cellY := int64(math.Floor(p.Y / si.CellSize))

    for dx := int64(-1); dx <= 1; dx++ {
        for dy := int64(-1); dy <= 1; dy++ {
            // Calculate neighbor cell world coordinates and get cell ID
            neighborX := float64(cellX+dx) * si.CellSize
            neighborY := float64(cellY+dy) * si.CellSize
            neighborCellID := si.getCellID(neighborX, neighborY)

            for _, candidateIdx := range si.Grid[neighborCellID] {
                candidate := points[candidateIdx]
                dist := math.Sqrt((candidate.X-p.X)*(candidate.X-p.X) +
                                 (candidate.Y-p.Y)*(candidate.Y-p.Y))

                if dist <= eps {
                    neighbors = append(neighbors, candidateIdx)
                }
            }
        }
    }

    return neighbors
}
```

#### DBSCAN Algorithm

```go
func DBSCAN(points []WorldPoint, eps float64, minPts int) []WorldCluster {
    n := len(points)
    labels := make([]int, n) // 0=unvisited, -1=noise, >0=clusterID
    clusterID := 0

    // Build spatial index (required for performance)
    spatialIndex := NewSpatialIndex(eps)
    spatialIndex.Build(points)

    for i := 0; i < n; i++ {
        if labels[i] != 0 {
            continue
        }

        neighbors := spatialIndex.RegionQuery(points, i, eps)

        if len(neighbors) < minPts {
            labels[i] = -1 // Noise
            continue
        }

        clusterID++
        expandCluster(points, spatialIndex, labels, i, neighbors, clusterID, eps, minPts)
    }

    return buildClusters(points, labels, clusterID)
}

func expandCluster(points []WorldPoint, si *SpatialIndex, labels []int,
                   seedIdx int, neighbors []int, clusterID int, eps float64, minPts int) {
    labels[seedIdx] = clusterID

    for j := 0; j < len(neighbors); j++ {
        idx := neighbors[j]

        if labels[idx] == -1 {
            labels[idx] = clusterID // Noise → border point
        }

        if labels[idx] != 0 {
            continue
        }

        labels[idx] = clusterID
        newNeighbors := si.RegionQuery(points, idx, eps)

        if len(newNeighbors) >= minPts {
            neighbors = append(neighbors, newNeighbors...)
        }
    }
}
```

#### Cluster Metrics Computation

```go
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

func computeClusterMetrics(points []WorldPoint) WorldCluster {
    n := float32(len(points))

    // Centroid (x, y, z)
    var sumX, sumY, sumZ float64
    for _, p := range points {
        sumX += p.X
        sumY += p.Y
        sumZ += p.Z
    }
    centroidX := float32(sumX / float64(n))
    centroidY := float32(sumY / float64(n))
    centroidZ := float32(sumZ / float64(n))

    // Axis-aligned bounding box
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
    p95Idx := int(0.95 * float64(len(heights)))
    if p95Idx >= len(heights) {
        p95Idx = len(heights) - 1
    }

    return WorldCluster{
        TSUnixNanos:       points[0].Timestamp.UnixNano(),
        SensorID:          points[0].SensorID,
        CentroidX:         centroidX,
        CentroidY:         centroidY,
        CentroidZ:         centroidZ,
        BoundingBoxLength: float32(maxX - minX),
        BoundingBoxWidth:  float32(maxY - minY),
        BoundingBoxHeight: float32(maxZ - minZ),
        PointsCount:       len(points),
        HeightP95:         float32(heights[p95Idx]),
        IntensityMean:     float32(sumIntensity / uint64(len(points))),
    }
}
```

---

## Phase 3.2: Kalman Tracking (World Frame)

### Objective

Multi-object tracking with explicit lifecycle states and world-frame state estimation.

### Track Lifecycle States

```
Tentative → Confirmed → Deleted
```

**State Transitions:**

- **Birth:** New cluster creates Tentative track
- **Tentative → Confirmed:** After N consecutive associations (N=3)
- **Confirmed → Deleted:** After MaxMisses frames without association (MaxMisses=3)
- **Tentative → Deleted:** After MaxMisses frames without association

### Track State (World Frame Only)

**State Vector:**

```
x = [x, y, vx, vy]^T
```

- `x, y`: Position in world frame (meters)
- `vx, vy`: Velocity in world frame (m/s)

**Motion Model (Constant Velocity):**

```
x_k+1 = F * x_k + w_k

F = [1  0  dt  0 ]
    [0  1  0  dt ]
    [0  0  1   0 ]
    [0  0  0   1 ]

w_k ~ N(0, Q)
```

**Measurement Model:**

```
z_k = H * x_k + v_k

H = [1  0  0  0]
    [0  1  0  0]

v_k ~ N(0, R)
```

### Implementation

```go
type TrackState string

const (
    TrackTentative TrackState = "tentative"
    TrackConfirmed TrackState = "confirmed"
    TrackDeleted   TrackState = "deleted"
)

type Track struct {
    // Identity
    TrackID    string
    SensorID   string
    WorldFrame FrameID
    PoseID     int64
    State      TrackState

    // Lifecycle
    FirstUnixNanos int64
    LastUnixNanos  int64
    Hits           int // Consecutive successful associations
    Misses         int // Consecutive missed associations

    // Kalman state (world frame only)
    KalmanState TrackState2D

    // Aggregated features
    ObservationCount     int
    BoundingBoxLengthAvg float32
    BoundingBoxWidthAvg  float32
    BoundingBoxHeightAvg float32
    AvgSpeedMps          float32
    PeakSpeedMps         float32

    // Classification (Phase 3.4)
    ObjectClass      string  // "pedestrian", "car", "bird", etc.
    ObjectConfidence float32
}

type TrackState2D struct {
    X, Y                 float32
    VelocityX, VelocityY float32
    CovarianceMatrix     [16]float32 // Row-major 4x4
}
```

### Tracker Implementation

```go
type Tracker struct {
    Tracks                map[string]*Track
    NextTrackID           int64
    MaxTracks             int     // 100
    MaxMisses             int     // 3
    HitsToConfirm         int     // 3
    GatingDistanceSquared float32 // 25.0 (5.0^2 meters squared)
    ProcessNoise          [4]float32
    MeasurementNoise      [2]float32
    mu                    sync.RWMutex
}

func NewTracker() *Tracker {
    return &Tracker{
        Tracks:                make(map[string]*Track),
        NextTrackID:           1,
        MaxTracks:             100,
        MaxMisses:             3,
        HitsToConfirm:         3,
        GatingDistanceSquared: 25.0, // 5.0 meters squared
        ProcessNoise:          [4]float32{0.1, 0.1, 0.5, 0.5},
        MeasurementNoise:      [2]float32{0.2, 0.2},
    }
}

func (t *Tracker) Update(clusters []WorldCluster, timestamp time.Time) {
    t.mu.Lock()
    defer t.mu.Unlock()

    // Predict all tracks to current time
    for _, track := range t.Tracks {
        if track.State != TrackDeleted {
            t.predict(track, timestamp)
        }
    }

    // Associate clusters to tracks (Mahalanobis distance with gating)
    associations := t.associate(clusters)

    // Update matched tracks
    matchedTracks := make(map[string]bool)
    for clusterIdx, trackID := range associations {
        if trackID != "" {
            track := t.Tracks[trackID]
            t.update(track, clusters[clusterIdx])
            track.Hits++
            track.Misses = 0
            matchedTracks[trackID] = true

            // Promote tentative → confirmed
            if track.State == TrackTentative && track.Hits >= t.HitsToConfirm {
                track.State = TrackConfirmed
            }
        }
    }

    // Handle unmatched tracks
    for trackID, track := range t.Tracks {
        if !matchedTracks[trackID] && track.State != TrackDeleted {
            track.Misses++
            track.Hits = 0

            if track.Misses >= t.MaxMisses {
                track.State = TrackDeleted
                track.LastUnixNanos = timestamp.UnixNano()
            }
        }
    }

    // Initialise new tracks from unassociated clusters
    for clusterIdx, trackID := range associations {
        if trackID == "" && len(t.Tracks) < t.MaxTracks {
            t.initTrack(clusters[clusterIdx])
        }
    }

    // Cleanup deleted tracks (after grace period)
    t.cleanupDeletedTracks(timestamp)
}
```

### Gating Distance (Mahalanobis)

**Definition:** Gating uses Mahalanobis distance in world coordinates to reject unlikely associations.

**Formula:**

```
d² = (z - Hx)^T * S^-1 * (z - Hx)

where:
  z = measurement (cluster centroid x, y)
  Hx = predicted measurement (track position x, y)
  S = innovation covariance (H*P*H^T + R)
```

**Threshold:** `GatingDistanceSquared = 25.0` (i.e., 5.0 meters)

- We threshold on **squared distance** to avoid square root computation
- Threshold tuned empirically for typical vehicle/pedestrian speeds

```go
func (t *Tracker) mahalanobisDistanceSquared(track *Track, cluster WorldCluster) float32 {
    // Innovation: difference between measurement and prediction
    dx := cluster.CentroidX - track.KalmanState.X
    dy := cluster.CentroidY - track.KalmanState.Y

    // Innovation covariance S (2x2)
    S := t.computeInnovationCovariance(track)

    // Mahalanobis distance squared: d² = [dx dy] * S^-1 * [dx dy]^T
    det := S[0]*S[3] - S[1]*S[2]
    if det == 0 {
        return 1e9 // Singular covariance
    }

    invS00 := S[3] / det
    invS01 := -S[1] / det
    invS11 := S[0] / det

    dist2 := dx*dx*invS00 + 2*dx*dy*invS01 + dy*dy*invS11

    return dist2
}
```

---

## Phase 3.3: SQL Schema & REST APIs

### SQL Schema (World Frame Only)

**Critical:** All tables store **world-frame coordinates only**. Polar coordinates and background grid data are **never** persisted to SQLite.

> **Note:** Pose ID columns have been removed. Data is stored in sensor frame coordinates (identity transform). Pose-based transformations are planned for a future phase.

#### Migration: 000009_create_lidar_tracks.up.sql

```sql
-- Clusters detected via DBSCAN (world frame)
CREATE TABLE IF NOT EXISTS lidar_clusters (
    lidar_cluster_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,

    -- World frame position (meters)
    centroid_x REAL,
    centroid_y REAL,
    centroid_z REAL,

    -- Bounding box (world frame, meters)
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL,

    -- Cluster features
    points_count INTEGER,
    height_p95 REAL,
    intensity_mean REAL
);

CREATE INDEX idx_clusters_sensor_time ON lidar_clusters(sensor_id, ts_unix_nanos);

-- Tracks (world frame)
CREATE TABLE IF NOT EXISTS lidar_tracks (
    track_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    track_state TEXT NOT NULL, -- 'tentative', 'confirmed', 'deleted'

    -- Lifecycle
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,

    -- Kinematics (world frame)
    avg_speed_mps REAL,
    peak_speed_mps REAL,
    p50_speed_mps REAL,  -- Median speed
    p85_speed_mps REAL,  -- 85th percentile
    p95_speed_mps REAL,  -- 95th percentile

    -- Shape features (world frame averages)
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,
    intensity_mean_avg REAL,

    -- Classification (Phase 3.4)
    object_class TEXT,           -- 'pedestrian', 'car', 'bird', 'other'
    object_confidence REAL,
    classification_model TEXT    -- Model version used for classification
);

CREATE INDEX idx_tracks_sensor ON lidar_tracks(sensor_id);
CREATE INDEX idx_tracks_state ON lidar_tracks(track_state);
CREATE INDEX idx_tracks_time ON lidar_tracks(start_unix_nanos, end_unix_nanos);
CREATE INDEX idx_tracks_class ON lidar_tracks(object_class);

-- Track observations (world frame)
CREATE TABLE IF NOT EXISTS lidar_track_obs (
    track_id TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    world_frame TEXT NOT NULL,

    -- Position (world frame, meters)
    x REAL,
    y REAL,
    z REAL,

    -- Velocity (world frame, m/s)
    velocity_x REAL,
    velocity_y REAL,
    speed_mps REAL,
    heading_rad REAL,

    -- Shape (world frame)
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL,
    height_p95 REAL,
    intensity_mean REAL,

    PRIMARY KEY (track_id, ts_unix_nanos),
    FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id) ON DELETE CASCADE
);

CREATE INDEX idx_track_obs_track ON lidar_track_obs(track_id);
CREATE INDEX idx_track_obs_time ON lidar_track_obs(ts_unix_nanos);
```

### REST API Endpoints

#### Existing Endpoints (Background/Polar)

```
GET  /api/lidar/params              - Background parameters
POST /api/lidar/params              - Update parameters
GET  /api/lidar/acceptance          - Acceptance metrics (polar)
GET  /api/lidar/grid_status         - Grid status (polar)
GET  /api/lidar/grid_heatmap        - Spatial heatmap (polar)
```

#### New Endpoints (World Frame)

```
# Clusters
GET /api/lidar/clusters?sensor_id=<id>&start=<unix>&end=<unix>
  - Returns recent clusters (world frame)
  - Response: Array of cluster objects with centroid, bbox, timestamp

# Active tracks
GET /api/lidar/tracks/active?sensor_id=<id>&state=<confirmed|tentative|all>
  - Returns currently active tracks (world frame)
  - Response: Array of track summaries with position, velocity, class

# Track history
GET /api/lidar/tracks/:track_id
  - Returns full track details
  - Response: Track object with all observations

# Track observations (trajectory)
GET /api/lidar/tracks/:track_id/observations
  - Returns trajectory points for visualization
  - Response: Array of (x, y, timestamp) tuples

# Track summaries (aggregated by class)
GET /api/lidar/tracks/summary?sensor_id=<id>&start=<unix>&end=<unix>&group_by=object_class
  - Returns aggregated statistics by object class
  - Response: Per-class counts, speed distributions, etc.
```

#### Example Response

```json
GET /api/lidar/tracks/active?sensor_id=hesai-01

{
  "tracks": [
    {
      "track_id": "track_1234",
      "sensor_id": "hesai-01",
      "track_state": "confirmed",
      "position": {"x": 12.5, "y": 3.2, "z": 0.5},
      "velocity": {"vx": 5.2, "vy": -0.3},
      "speed_mps": 5.21,
      "heading_rad": -0.057,
      "object_class": "car",
      "object_confidence": 0.89,
      "observation_count": 24,
      "age_seconds": 2.4
    }
  ],
  "count": 1,
  "timestamp": "2025-11-22T05:30:00Z"
}
```

---

## Phase 3.4: Track Classification

### Objective

Classify tracks by object type (pedestrian, car, bird, other) using world-frame features.

### Classification Features (World Frame)

**Spatial Features:**

- Bounding box dimensions (length, width, height) in meters
- Height p95 (95th percentile Z coordinate)
- Point density (points per cubic meter)

**Kinematic Features:**

- Average speed (p50_speed_mps)
- Peak speed (p95_speed_mps)
- Speed variance
- Acceleration magnitude

**Temporal Features:**

- Track duration
- Observation count
- Consistency score (ratio of observations to expected frames)

### Classification Logic

```go
type TrackClassifier struct {
    // Simple rule-based classifier (can be replaced with ML model later)
}

func (tc *TrackClassifier) Classify(track *Track) (class string, confidence float32) {
    // Extract features from track
    avgLength := track.BoundingBoxLengthAvg
    avgWidth := track.BoundingBoxWidthAvg
    avgHeight := track.BoundingBoxHeightAvg
    avgSpeed := track.AvgSpeedMps
    peakSpeed := track.PeakSpeedMps

    // Rule-based classification
    if avgHeight < 0.5 && avgSpeed < 1.0 {
        return "bird", 0.7
    } else if avgHeight > 1.2 && avgLength > 3.0 && avgSpeed > 5.0 {
        return "car", 0.85
    } else if avgHeight > 1.0 && avgHeight < 2.0 && avgSpeed < 3.0 {
        return "pedestrian", 0.75
    } else {
        return "other", 0.5
    }
}

// Called after track becomes confirmed
func (t *Tracker) classifyTrack(track *Track) {
    if track.ObservationCount < 10 {
        return // Not enough observations
    }

    class, confidence := t.classifier.Classify(track)
    track.ObjectClass = class
    track.ObjectConfidence = confidence
}
```

### Future Enhancement: ML-Based Classification

- Train model on labeled track features
- Export features to CSV for model training
- Deploy model as inference endpoint
- Update `classification_model` field with model version

---

## Performance & Concurrency

### Locking Boundaries

**Background Lock (RWMutex):**

- **Holds lock:** Only during `ProcessFramePolar()` classification
- **Releases before:** Foreground point extraction, transform, clustering, tracking

**Clear Separation:**

```
[Background Lock Held]
  - Classify points in polar space
  - Update EMA for background cells
  - Generate foreground mask

[Background Lock Released]
  - Extract foreground polar points
  - Transform polar → world
  - DBSCAN clustering
  - Kalman tracking
  - Database writes
  - API/UI updates
```

### Latency Budget (Per Stage)

Target: **<100ms end-to-end** at 10 Hz (10,000-20,000 points per frame)

| Stage                             | Target Latency | Notes                   |
| --------------------------------- | -------------- | ----------------------- |
| Background classification (polar) | <5ms           | With background lock    |
| Foreground extraction             | <1ms           | Simple mask application |
| Polar → World transform           | <3ms           | Matrix multiplication   |
| DBSCAN clustering (world)         | <30ms          | With spatial index      |
| Kalman tracking (world)           | <10ms          | Association + update    |
| Database persistence              | <5ms           | Async batch writes      |
| API/UI update                     | <5ms           | Non-blocking            |
| **Total**                         | **<60ms**      | Safety margin for 10 Hz |

### Profiling Points

```go
// Instrumentation for latency measurement
type PipelineMetrics struct {
    BackgroundClassifyUs int64
    ForegroundExtractUs  int64
    TransformUs          int64
    ClusteringUs         int64
    TrackingUs           int64
    DatabaseUs           int64
    TotalUs              int64
}

// Emit per-frame for monitoring
emitPipelineMetrics(metrics)
```

---

## Testing Strategy

### Test Categories

#### 1. Polar Frame Tests (Phase 2.9)

**Test:** Foreground mask accuracy

```go
func TestProcessFramePolar_ForegroundMask(t *testing.T) {
    // Setup: Background grid with stable background at 10m
    bm := setupBackgroundManager(10.0)

    // Test: Points at 5m (foreground) and 10m (background)
    points := []PointPolar{
        {Ring: 0, Azimuth: 0, Distance: 5.0},  // Expect: foreground
        {Ring: 0, Azimuth: 0, Distance: 10.0}, // Expect: background
    }

    mask, err := bm.ProcessFramePolar(points)

    assert.NoError(t, err)
    assert.Equal(t, []bool{true, false}, mask)
}
```

#### 2. Transform Tests (Phase 3.0)

**Test:** Polar → World transform accuracy

```go
func TestTransformToWorld_Accuracy(t *testing.T) {
    // Known pose: identity transform
    identityPose := &Pose{
        T: [16]float64{
            1, 0, 0, 0,
            0, 1, 0, 0,
            0, 0, 1, 0,
            0, 0, 0, 1,
        },
    }

    // Point at (distance=10m, azimuth=0°, elevation=0°)
    // Should transform to (x=10, y=0, z=0)
    polar := []PointPolar{{Distance: 10.0, Azimuth: 0, Elevation: 0}}

    world := TransformToWorld(polar, identityPose)

    assert.InDelta(t, 10.0, world[0].X, 0.01)
    assert.InDelta(t, 0.0, world[0].Y, 0.01)
    assert.InDelta(t, 0.0, world[0].Z, 0.01)
}
```

#### 3. Clustering Tests (Phase 3.1)

**Test:** DBSCAN detects distinct clusters

```go
func TestDBSCAN_TwoSeparateClusters(t *testing.T) {
    // Create two clusters: one at origin, one at (10, 0)
    cluster1 := generateSphere(0, 0, 0, 0.3, 50)
    cluster2 := generateSphere(10, 0, 0, 0.3, 50)

    allPoints := append(cluster1, cluster2...)

    clusters := DBSCAN(allPoints, 0.6, 12)

    // Should detect exactly 2 clusters
    assert.Equal(t, 2, len(clusters))

    // Verify centroids are correct
    centroids := []float32{clusters[0].CentroidX, clusters[1].CentroidX}
    sort.Float32s(centroids)
    assert.InDelta(t, 0.0, centroids[0], 0.5)
    assert.InDelta(t, 10.0, centroids[1], 0.5)
}
```

#### 4. Tracking Tests (Phase 3.2)

**Test:** Track lifecycle (Tentative → Confirmed → Deleted)

```go
func TestTracking_Lifecycle(t *testing.T) {
    tracker := NewTracker()
    timestamp := time.Now()

    // Create cluster representing moving object
    cluster := WorldCluster{
        CentroidX: 0, CentroidY: 0,
        TSUnixNanos: timestamp.UnixNano(),
    }

    // Frame 1: Birth (Tentative)
    tracker.Update([]WorldCluster{cluster}, timestamp)
    assert.Equal(t, 1, len(tracker.Tracks))
    var track *Track
    for _, t := range tracker.Tracks {
        track = t
    }
    assert.Equal(t, TrackTentative, track.State)

    // Frames 2-4: Hits (Tentative → Confirmed after 3 hits)
    for i := 1; i <= 3; i++ {
        timestamp = timestamp.Add(100 * time.Millisecond)
        cluster.CentroidX = float32(i)
        tracker.Update([]WorldCluster{cluster}, timestamp)
    }
    assert.Equal(t, TrackConfirmed, track.State)

    // Frames 5-7: Misses (Confirmed → Deleted after 3 misses)
    for i := 0; i < 3; i++ {
        timestamp = timestamp.Add(100 * time.Millisecond)
        tracker.Update([]WorldCluster{}, timestamp)
    }
    assert.Equal(t, TrackDeleted, track.State)
}
```

#### 5. Integration Tests (End-to-End)

**Test:** Full pipeline with PCAP

```go
func TestPipeline_PCAPToTracks(t *testing.T) {
    // Load PCAP with known moving objects
    pcapPath := "testdata/cars.pcap"

    // Setup pipeline
    bm := NewBackgroundManager(...)
    tracker := NewTracker()

    // Process all frames
    processedFrames := 0
    finalTracks := 0

    err := processPCAP(pcapPath, func(frame *LidarFrame) {
        // Polar classification
        mask, _ := bm.ProcessFramePolar(frame.Points)
        foregroundPolar := extractForegroundPoints(frame.Points, mask)

        // Transform to world
        foregroundWorld := TransformToWorld(foregroundPolar, frame.Pose)

        // Cluster
        clusters := DBSCAN(foregroundWorld, 0.6, 12)

        // Track
        tracker.Update(clusters, frame.Timestamp)

        processedFrames++
        finalTracks = len(tracker.Tracks)
    })

    assert.NoError(t, err)
    assert.Greater(t, processedFrames, 100)
    assert.Greater(t, finalTracks, 0)
}
```

---

## Implementation Roadmap

### Phase Timeline

| Phase | Description              | Duration | Status      | Deliverables                                                                      |
| ----- | ------------------------ | -------- | ----------- | --------------------------------------------------------------------------------- |
| 2.9   | Foreground Mask (Polar)  | 1-2 days | ✅ Complete | `ProcessFramePolarWithMask`, `ExtractForegroundPoints`, `FrameMetrics`            |
| 3.0   | Transform (Polar→World)  | 1-2 days | ✅ Complete | `TransformToWorld`, `WorldPoint`, unit tests                                      |
| 3.1   | DBSCAN Clustering        | 3-4 days | ✅ Complete | `SpatialIndex`, `DBSCAN`, `computeClusterMetrics`, `WorldCluster`                 |
| 3.2   | Kalman Tracking          | 4-5 days | ✅ Complete | `Tracker`, `TrackedObject`, Mahalanobis gating, lifecycle management              |
| 3.3   | SQL Schema & Persistence | 3-4 days | ✅ Complete | `lidar_clusters`, `lidar_tracks`, `lidar_track_obs` tables, persistence functions |
| 3.4   | Classification           | 2-3 days | ✅ Complete | `TrackClassifier`, rule-based classification, object classes                      |
| 3.5   | REST API Endpoints       | 1-2 days | ✅ Complete | `TrackAPI` HTTP handlers, list/get/update tracks, cluster queries                 |
| 3.6   | PCAP Analysis Tool       | 1-2 days | ✅ Complete | `pcap-analyze` CLI tool for batch processing, ML data export                      |
| 3.8   | Track Visualisation UI   | 2-3 days | ✅ Complete | MapPane, TrackList, TimelinePane components, pagination, playback                 |
| Test  | Integration Testing      | 2-3 days | 📋 Planned  | End-to-end tests, performance validation                                          |

**Phases 2.9-3.8: Complete**
**Remaining: Integration Testing**

### Milestones

1. ✅ **Background Learning Complete** (Done - Phase 1-2)
2. ✅ **Foreground Masks Working** - `ProcessFramePolarWithMask()` outputs per-point masks
3. ✅ **World Transform Validated** - `TransformToWorld()` tests passing with identity and custom poses
4. ✅ **Clustering Operational** - `DBSCAN()` detecting clusters with spatial index
5. ✅ **Tracking Functional** - `Tracker` maintains tracks with Kalman filter and lifecycle management
6. ✅ **SQL Schema Ready** - Database persistence with `lidar_clusters`, `lidar_tracks`, `lidar_track_obs` tables
7. ✅ **Classification Active** - Rule-based classifier for pedestrian, car, bird, other
8. ✅ **REST Endpoints** - HTTP handlers for track/cluster API access
9. ✅ **PCAP Analysis Tool** - CLI tool for batch track categorisation and ML data export
10. ✅ **Track Visualisation UI** - SvelteKit components for track history playback
11. 📋 **Production Ready** - All tests passing, documented, deployed

### Implementation Files

| Phase   | File                                                       | Description                                      |
| ------- | ---------------------------------------------------------- | ------------------------------------------------ |
| 2.9     | `internal/lidar/foreground.go`                             | Foreground mask generation and extraction        |
| 2.9     | `internal/lidar/foreground_test.go`                        | Unit tests for foreground extraction             |
| 3.0-3.1 | `internal/lidar/clustering.go`                             | Transform and DBSCAN clustering                  |
| 3.0-3.1 | `internal/lidar/clustering_test.go`                        | Unit tests for transform and clustering          |
| 3.2     | `internal/lidar/tracking.go`                               | Kalman tracking with lifecycle management        |
| 3.2     | `internal/lidar/tracking_test.go`                          | Unit tests for tracking                          |
| 3.3     | `internal/db/migrations/000009_create_lidar_tracks.up.sql` | Database schema for clusters/tracks/observations |
| 3.3     | `internal/lidar/track_store.go`                            | Database persistence functions                   |
| 3.3     | `internal/lidar/track_store_test.go`                       | Unit tests for track persistence                 |
| 3.4     | `internal/lidar/classification.go`                         | Rule-based track classification                  |
| 3.4     | `internal/lidar/classification_test.go`                    | Unit tests for classification                    |
| 3.5     | `internal/lidar/monitor/track_api.go`                      | HTTP handlers for track/cluster queries          |
| 3.5     | `internal/lidar/monitor/track_api_test.go`                 | Unit tests for track API                         |
| 3.6     | `cmd/tools/pcap-analyze/main.go`                           | PCAP analysis tool for batch processing          |
| 3.8     | `web/src/lib/components/lidar/MapPane.svelte`              | Canvas-based track visualisation                 |
| 3.8     | `web/src/lib/components/lidar/TrackList.svelte`            | Track list with filters and pagination           |
| 3.8     | `web/src/lib/components/lidar/TimelinePane.svelte`         | Timeline with playback controls                  |
| 3.8     | `web/src/routes/lidar/tracks/+page.svelte`                 | Track history playback page                      |
| ML      | `internal/lidar/training_data.go`                          | Training data export and encoding                |
| ML      | `internal/lidar/training_data_test.go`                     | Unit tests for training data                     |

---

## Appendix

### A. Data Structures

**PointPolar (Input - Polar Frame):**

```go
type PointPolar struct {
    Distance  float64   // Range in meters
    Azimuth   float64   // Horizontal angle (degrees)
    Elevation float64   // Vertical angle (degrees)
    Intensity uint8     // Return intensity
    Timestamp time.Time
    Ring      int       // Laser ring index (0-39)
}
```

**WorldPoint (After Transform - World Frame):**

```go
type WorldPoint struct {
    X, Y, Z   float64   // Cartesian world coordinates (meters)
    Intensity uint8
    Timestamp time.Time
    SensorID  string
}
```

**WorldCluster (After Clustering - World Frame):**

```go
type WorldCluster struct {
    ClusterID         int64
    SensorID          string
    WorldFrame        FrameID
    PoseID            int64
    TSUnixNanos       int64
    CentroidX         float32  // World frame (meters)
    CentroidY         float32
    CentroidZ         float32
    BoundingBoxLength float32
    BoundingBoxWidth  float32
    BoundingBoxHeight float32
    PointsCount       int
    HeightP95         float32
    IntensityMean     float32
}
```

**Track (After Tracking - World Frame):**

```go
type Track struct {
    TrackID              string
    SensorID             string
    State                TrackState // tentative/confirmed/deleted
    FirstUnixNanos       int64
    LastUnixNanos        int64
    Hits                 int
    Misses               int
    KalmanState          TrackState2D // x, y, vx, vy in world frame
    ObservationCount     int
    AvgSpeedMps          float32
    PeakSpeedMps         float32
    BoundingBoxLengthAvg float32
    ObjectClass          string
    ObjectConfidence     float32
}
```

### B. Configuration Parameters

**Background (Polar):**

```go
BackgroundUpdateFraction       = 0.02
ClosenessSensitivityMultiplier = 3.0
SafetyMarginMeters             = 0.5
NeighborConfirmationCount      = 3
NoiseRelativeFraction          = 0.315
```

**Clustering (World):**

```go
Eps      = 0.6    // meters
MinPts   = 12     // points
CellSize = 0.6    // spatial index cell size (meters)
```

**Tracking (World):**

```go
MaxTracks             = 100
MaxMisses             = 3
HitsToConfirm         = 3
GatingDistanceSquared = 25.0  // 5.0 meters squared
ProcessNoise          = [0.1, 0.1, 0.5, 0.5]
MeasurementNoise      = [0.2, 0.2]
```

### C. Related Documentation

- `ARCHITECTURE.md` - System architecture overview
- `internal/lidar/docs/architecture/lidar_sidecar_overview.md` - LIDAR implementation details
- `internal/lidar/docs/devlog.md` - Development history
- `internal/lidar/background.go` - Background grid implementation
- `internal/lidar/arena.go` - Track data structures

### D. ML Training Data Storage

#### Storage Recommendation: Sensor Frame (Polar)

**Training data should be stored in sensor frame (polar coordinates)** for the following reasons:

1. **Pose Independence:** Polar data is independent of external calibration. If the pose changes (sensor moved, recalibrated), historical polar data remains valid and can be re-transformed.

2. **Reusability:** Training data collected from one installation can be reused when the pose is updated, without needing to recollect or retransform.

3. **Compact Representation:** Polar coordinates (distance, azimuth, elevation, ring) are a compact, lossless representation of sensor measurements.

4. **Transform on Demand:** World-frame data can always be regenerated from polar data + current pose at training time.

#### Training Data Schema

```sql
-- Foreground point cloud sequences for ML training
CREATE TABLE IF NOT EXISTS lidar_training_frames (
    frame_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    frame_sequence_id TEXT,          -- Group frames into sequences (e.g., "seq_20251130_001")

    -- Metadata
    total_points INTEGER,
    foreground_points INTEGER,
    background_points INTEGER,

    -- Polar point cloud (compressed blob)
    -- Each point: (distance_cm uint16, azimuth_centideg uint16, elevation_centideg int16,
    --              intensity uint8, ring uint8) = 8 bytes per point
    foreground_polar_blob BLOB,

    -- Labels (filled during annotation)
    annotation_status TEXT DEFAULT 'unlabeled',  -- 'unlabeled', 'in_progress', 'labeled'
    annotator TEXT,
    annotation_json TEXT              -- Track labels, object classes, etc.
);

CREATE INDEX idx_training_sensor_time ON lidar_training_frames(sensor_id, ts_unix_nanos);
CREATE INDEX idx_training_sequence ON lidar_training_frames(frame_sequence_id);
```

> **Note:** Pose columns have been removed. Data is stored in polar (sensor) frame, which is pose-independent. Pose-based quality filtering is planned for a future phase.

#### Export Functions

```go
// ForegroundFrame represents a single frame of foreground points for training
type ForegroundFrame struct {
    SensorID         string
    TSUnixNanos      int64
    SequenceID       string
    ForegroundPoints []PointPolar
    TotalPoints      int
    BackgroundPoints int
}

// ExportForegroundFrame exports foreground points in polar coordinates for ML training
func ExportForegroundFrame(polarPoints []PointPolar, mask []bool, sensorID string, ts time.Time) *ForegroundFrame {
    foreground := ExtractForegroundPoints(polarPoints, mask)

    return &ForegroundFrame{
        SensorID:         sensorID,
        TSUnixNanos:      ts.UnixNano(),
        ForegroundPoints: foreground,
        TotalPoints:      len(polarPoints),
        BackgroundPoints: len(polarPoints) - len(foreground),
    }
}
```

### E. Future Work: Pose Validation

> **Note:** Pose validation and quality assessment have been deferred to a future phase. This section describes the planned functionality.

#### Planned Features

1. **Pose Validation:** Validate sensor calibration quality based on RMSE metrics
2. **Quality Assessment:** Categorize pose quality (Excellent/Good/Fair/Poor)
3. **Transform Gating:** Gate world-frame transformations by pose quality
4. **Training Data Filtering:** Filter ML training data by pose quality

#### Design Rationale

The current implementation stores all data in polar (sensor) frame, which is pose-independent. This design choice:

1. **Preserves Data Validity:** Training data remains valid even if the sensor pose changes
2. **Simplifies Schema:** No pose foreign keys required in the database
3. **Enables Future Enhancement:** Pose can be added later without data migration

#### Future Implementation Plan

When pose validation is implemented:

| RMSE (meters) | Quality   | Usage Recommendation                    |
| ------------- | --------- | --------------------------------------- |
| < 0.05        | Excellent | Use for all downstream processing       |
| 0.05 - 0.15   | Good      | Use for tracking and training           |
| 0.15 - 0.30   | Fair      | Use for tracking; exclude from training |
| > 0.30        | Poor      | Manual recalibration required           |

**Current Status:** Training data stored in polar (sensor) frame. World-frame transformations use identity transform (sensor frame = world frame).

---

**Document Status:** Implementation Complete through Phase 3.6
**Next Action:** Implement Phase 4.0 Analysis Run Infrastructure (see `../ROADMAP.md`)
**Last Updated:** December 1, 2025
**Contact:** Engineering Team

---

## Related Documentation

- **[ML Pipeline Roadmap](../ROADMAP.md)** - Complete architectural plan for Phase 4.0-4.4 (analysis runs, labeling UI, ML training, parameter tuning)
- **[Velocity-Coherent Foreground Extraction](../future/velocity-coherent-foreground-extraction.md)** - Alternative algorithm design for sparse-point tracking with velocity coherence
- **[LIDAR Foreground Tracking Status](../operations/lidar-foreground-tracking-status.md)** - Current issues, fixes, and enhancement roadmap
- **[LIDAR Sidecar Overview](lidar_sidecar_overview.md)** - Technical implementation overview and module structure
- **[Development Log](../../DEVLOG.md)** - Chronological implementation history
