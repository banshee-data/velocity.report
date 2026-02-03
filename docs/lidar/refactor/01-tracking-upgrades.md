# Tracking Upgrades

This document proposes concrete improvements to the LiDAR tracking pipeline for street scenes, mapping each proposal to existing code and new API outputs.

---

## 1. Current State

### 1.1 Existing Implementation

| Component | File | Key Functions/Types |
|-----------|------|---------------------|
| **Background Model** | `internal/lidar/background.go` | `BackgroundManager`, `BackgroundGrid`, `BackgroundCell` |
| **Foreground Extraction** | `internal/lidar/foreground.go` | `ProcessFramePolarWithMask()`, `ExtractForegroundPoints()` |
| **Clustering** | `internal/lidar/clustering.go` | `DBSCAN()`, `WorldCluster`, `SpatialIndex` |
| **Tracking** | `internal/lidar/tracking.go` | `Tracker`, `TrackedObject`, `TrackState` |
| **Pipeline** | `internal/lidar/tracking_pipeline.go` | `TrackingPipelineConfig`, `NewFrameCallback()` |
| **Transform** | `internal/lidar/transform.go` | `SphericalToCartesian()`, `TransformToWorld()` |

### 1.2 Current Algorithm

```
Raw Points (polar)
    │
    ▼
Background Model (EMA grid, 40 rings × 1800 azimuth bins)
    │
    ▼
Foreground Mask (per-point classification)
    │
    ▼
World Transform (spherical → Cartesian → world frame)
    │
    ▼
DBSCAN Clustering (eps=0.6m, minPts=12)
    │
    ▼
Nearest-Neighbor Association (Mahalanobis gating)
    │
    ▼
Kalman Update (constant velocity model)
    │
    ▼
Lifecycle Management (tentative → confirmed → deleted)
```

### 1.3 Known Limitations

| Issue | Impact | Cause |
|-------|--------|-------|
| Ground points leak into foreground | False clusters near sensor | Height-based filtering missing |
| Clusters split on large vehicles | Track fragmentation | DBSCAN eps too small |
| Tracks merge when objects cross | ID swaps | Greedy association |
| Heading estimation noisy | OBB rotation jitter | No temporal smoothing |
| No occlusion handling | Tracks deleted during occlusion | Fixed miss threshold |

---

## 2. Proposed Upgrades

### 2.1 Ground/Background Removal

**Current**: Polar-grid EMA model classifies points as foreground/background based on range deviation.

**Proposed**: Add explicit **ground plane removal** before clustering.

**Options**:

| Method | Pros | Cons | Recommendation |
|--------|------|------|----------------|
| Height threshold | Simple, fast | Assumes flat ground | Use as baseline |
| RANSAC plane fit | Handles slope | More compute | Use for accuracy |
| Ring-based gradient | Uses sensor geometry | Complex | Deferred |

**Implementation**:

```go
// internal/lidar/ground.go (NEW)

type GroundRemover interface {
    RemoveGround(points []WorldPoint) []WorldPoint
}

// Simple height threshold
type HeightThresholdRemover struct {
    MinHeight float32  // e.g., 0.2m (wheel height)
    MaxHeight float32  // e.g., 3.0m (truck height)
}

func (r *HeightThresholdRemover) RemoveGround(points []WorldPoint) []WorldPoint {
    filtered := make([]WorldPoint, 0, len(points))
    for _, p := range points {
        if p.Z >= float64(r.MinHeight) && p.Z <= float64(r.MaxHeight) {
            filtered = append(filtered, p)
        }
    }
    return filtered
}
```

**API Output**: Add `ground_removed: bool` flag to `PointCloudFrame`.

**Repo Location**: New file `internal/lidar/ground.go`, called from `tracking_pipeline.go` after `TransformToWorld()`.

---

### 2.2 Clustering Improvements

**Current**: DBSCAN with fixed `eps=0.6m`, `minPts=12`.

**Proposed**: Adaptive parameters + voxel grid preprocessing.

#### 2.2.1 Voxel Grid Downsampling

Reduce point count before clustering for large clouds.

```go
// internal/lidar/voxel.go (NEW)

type VoxelGrid struct {
    Resolution float64  // e.g., 0.1m
}

func (v *VoxelGrid) Downsample(points []WorldPoint) []WorldPoint {
    // Hash points to voxel cells
    // Return one representative per cell
}
```

**Tradeoff**: Voxel grid loses density information but improves clustering speed.

#### 2.2.2 Connected Components Alternative

For dense point clouds, connected components on voxel grid may be faster than DBSCAN.

```go
// internal/lidar/clustering.go (extension)

func ConnectedComponents(points []WorldPoint, resolution float64) []WorldCluster {
    // 1. Build 3D voxel grid
    // 2. Find connected components via flood fill
    // 3. Build clusters from components
}
```

**Recommendation**: Keep DBSCAN as default, add voxel grid preprocessing option.

**API Output**: `ClusterSet.clustering_method` field (enum: `DBSCAN`, `CONNECTED_COMPONENTS`).

---

### 2.3 Association Upgrades

**Current**: Greedy nearest-neighbor with Mahalanobis gating.

**Proposed**: Optimal assignment via Hungarian algorithm.

#### 2.3.1 Hungarian (Jonker-Volgenant) Algorithm

```go
// internal/lidar/association.go (NEW)

type Associator interface {
    Associate(tracks []*TrackedObject, clusters []WorldCluster, dt float32) []Assignment
}

type Assignment struct {
    TrackIdx   int
    ClusterIdx int
    Cost       float32
}

// Greedy (current)
type GreedyAssociator struct {
    GatingThreshold float32
}

// Optimal (new)
type HungarianAssociator struct {
    GatingThreshold float32
    MaxCost         float32  // reject if cost > threshold
}
```

**Mahalanobis Gating**: Current implementation at `tracking.go:317-360` is preserved. Hungarian operates on the cost matrix after gating.

**API Output**: Debug overlay `AssociationCandidate` shows accepted/rejected pairs.

---

### 2.4 Filter Model Improvements

**Current**: Constant velocity (CV) Kalman filter, 4-state: `[x, y, vx, vy]`.

**Proposed**: Options for enhanced models.

#### 2.4.1 Constant Acceleration (CA) Model

6-state: `[x, y, vx, vy, ax, ay]`

Better for accelerating/braking vehicles.

#### 2.4.2 Interacting Multiple Model (IMM)

Blend CV + CA based on motion likelihood.

**Recommendation**: Keep CV as default. Add CA as configuration option. IMM is future work.

```go
// internal/lidar/tracking.go (extension)

type MotionModel string

const (
    MotionModelCV MotionModel = "cv"  // constant velocity (current)
    MotionModelCA MotionModel = "ca"  // constant acceleration
)

type TrackerConfig struct {
    // ... existing fields ...
    MotionModel MotionModel  // NEW
}
```

**API Output**: `Track.motion_model` field in proto.

---

### 2.5 Lifecycle and Occlusion Handling

**Current**: Fixed `MaxMisses=3` before deletion. No occlusion awareness.

**Proposed**: Adaptive lifecycle based on track confidence and occlusion detection.

#### 2.5.1 Confidence-Based Lifecycle

```go
// internal/lidar/tracking.go (extension)

type TrackedObject struct {
    // ... existing fields ...
    
    // NEW: Quality metrics
    Confidence      float32  // 0.0 - 1.0, based on observation history
    OcclusionState  OcclusionState
}

type OcclusionState string

const (
    OcclusionNone     OcclusionState = "none"
    OcclusionPartial  OcclusionState = "partial"
    OcclusionFull     OcclusionState = "full"
)

func (t *TrackedObject) ComputeConfidence() float32 {
    // Factors:
    // - hits / (hits + misses)
    // - observation count
    // - track age
    // - covariance magnitude
}
```

#### 2.5.2 Occlusion Detection

Detect when a track is likely occluded (another track between sensor and target).

```go
func (t *Tracker) DetectOcclusions(tracks []*TrackedObject, sensorPos [2]float32) {
    // For each track:
    // - Cast ray from sensor to track
    // - Check if another track intersects ray
    // - Mark as occluded if blocked
}
```

**API Output**: `Track.occlusion_state` field, `Track.confidence` field.

---

### 2.6 OBB Estimation and Smoothing

**Current**: Only AABB (axis-aligned) bounding boxes. No heading estimation from shape.

**Proposed**: Oriented bounding box (OBB) via PCA + temporal smoothing.

#### 2.6.1 PCA-Based OBB

```go
// internal/lidar/obb.go (NEW)

type OBB struct {
    CenterX, CenterY, CenterZ float32
    Length, Width, Height     float32
    HeadingRad                float32  // rotation around Z
}

func ComputeOBB(points []WorldPoint) OBB {
    // 1. Compute centroid
    // 2. Build covariance matrix (2D, x-y plane)
    // 3. Eigen decomposition → principal axes
    // 4. Rotate points to principal frame
    // 5. Compute extents in principal frame
    // 6. HeadingRad = atan2(eigenvector[0].y, eigenvector[0].x)
}
```

#### 2.6.2 Temporal Smoothing

Smooth OBB heading to reduce jitter.

```go
func (t *TrackedObject) SmoothOBB(newOBB OBB, alpha float32) {
    // Exponential moving average on heading
    // Handle angle wraparound (-π to π)
    t.OBBHeading = circularEMA(t.OBBHeading, newOBB.HeadingRad, alpha)
}
```

**API Output**: `OrientedBoundingBox` message in `Cluster` proto.

---

### 2.7 Classification Hooks

**Current**: Rule-based classifier in `internal/lidar/classification.go` (pedestrian, car, bird, other).

**Proposed**: Feature extraction hooks for ML classifier training.

#### 2.7.1 Feature Vector

```go
// internal/lidar/features.go (NEW)

type ClusterFeatures struct {
    PointCount       int
    BBoxLength       float32
    BBoxWidth        float32
    BBoxHeight       float32
    HeightP95        float32
    IntensityMean    float32
    IntensityStd     float32
    Elongation       float32  // length / width
    Compactness      float32  // points / bbox_volume
    VerticalSpread   float32  // std of Z
}

type TrackFeatures struct {
    ClusterFeatures       // aggregated over observations
    AvgSpeedMps           float32
    PeakSpeedMps          float32
    SpeedVariance         float32
    TrackDurationSecs     float32
    TrackLengthMeters     float32
    HeadingVariance       float32
    OcclusionRatio        float32
}

func ExtractClusterFeatures(cluster WorldCluster, points []WorldPoint) ClusterFeatures
func ExtractTrackFeatures(track *TrackedObject) TrackFeatures
```

**API Output**: Optional `features` field in `Track` proto for training data export.

---

### 2.8 Debug Artifacts

**Current**: Limited debug logging via `Debugf()`.

**Proposed**: Structured debug artifacts for visualiser.

#### 2.8.1 Debug Collector

```go
// internal/lidar/debug/collector.go (NEW)

type DebugCollector struct {
    enabled bool
    frame   *DebugFrame
}

type DebugFrame struct {
    FrameID               uint64
    AssociationCandidates []AssociationCandidate
    GatingEllipses        []GatingEllipse
    Residuals             []InnovationResidual
    Predictions           []StatePrediction
}

func (c *DebugCollector) RecordAssociation(clusterIdx int, trackID string, dist float32, accepted bool)
func (c *DebugCollector) RecordGating(trackID string, ellipse GatingEllipse)
func (c *DebugCollector) RecordResidual(trackID string, predicted, measured [2]float32)
func (c *DebugCollector) Emit() *DebugFrame
```

#### 2.8.2 Integration Points

| Location | What to Record |
|----------|----------------|
| `tracking.go:associate()` | Association candidates, accepted/rejected |
| `tracking.go:mahalanobisDistanceSquared()` | Gating ellipse parameters |
| `tracking.go:update()` | Innovation residuals |
| `tracking.go:predict()` | State predictions |

**API Output**: `DebugOverlaySet` in `FrameBundle` proto.

---

## 3. Mapping to API Outputs

| Upgrade | New Proto Fields | Debug Overlays |
|---------|------------------|----------------|
| Ground removal | `PointCloudFrame.classification` | N/A |
| Voxel grid | `ClusterSet.clustering_method` | N/A |
| Hungarian | N/A | `AssociationCandidate` |
| CA model | `Track.motion_model` | `StatePrediction` |
| Occlusion | `Track.occlusion_state`, `Track.confidence` | N/A |
| OBB | `Cluster.obb`, `Track.bbox_heading_rad` | OBB visualisation |
| Features | `Track.features` (optional) | N/A |

---

## 4. Implementation Priority

| Priority | Upgrade | Effort | Impact |
|----------|---------|--------|--------|
| **P0** | Ground removal (height threshold) | Low | High - reduces false clusters |
| **P0** | OBB estimation | Medium | High - heading visualisation |
| **P1** | Debug artifacts | Medium | High - debugging workflow |
| **P1** | OBB temporal smoothing | Low | Medium - visual quality |
| **P2** | Hungarian association | Medium | Medium - fewer ID swaps |
| **P2** | Occlusion detection | Medium | Medium - track continuity |
| **P3** | Voxel grid preprocessing | Low | Low - performance |
| **P3** | CA motion model | Medium | Low - marginal accuracy |
| **P3** | Feature extraction | Low | Low - ML prep |

---

## 5. Testing Strategy

### 5.1 Unit Tests

Each upgrade includes unit tests for the new function.

### 5.2 Golden Replay Tests

- Record baseline tracks with current algorithm
- After upgrade, compare:
  - Track count (should be similar or improved)
  - Track duration (should be similar or improved)
  - ID stability (no regressions)

### 5.3 Visual Validation

- Render before/after in visualiser
- Check for improvements in:
  - Ground point removal
  - OBB alignment
  - Track continuity

---

## 6. Related Documents

- [../visualizer_macos/01-problem-and-user-workflows.md](../visualizer_macos/01-problem-and-user-workflows.md) – User workflows
- [../visualizer_macos/02-api-contracts.md](../visualizer_macos/02-api-contracts.md) – API contract
- [../visualizer_macos/04-implementation-plan.md](../visualizer_macos/04-implementation-plan.md) – Milestones
