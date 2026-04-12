# Tracking upgrades

- **Status:** 6 of 9 upgrades implemented (February 2026)

Implementation status of planned tracking pipeline improvements, from ground removal and OBB estimation through to future ML feature extraction.

| Upgrade                           | Status         | Implementation                                                                      |
| --------------------------------- | -------------- | ----------------------------------------------------------------------------------- |
| Ground removal (height threshold) | ✅ Implemented | `internal/lidar/ground.go` — `HeightBandFilter` with `FilterVertical()`             |
| OBB estimation (PCA)              | ✅ Implemented | `internal/lidar/obb.go` — `EstimateOBBFromCluster()`                                |
| OBB temporal smoothing            | ✅ Implemented | `internal/lidar/obb.go` — EMA heading (α=0.3)                                       |
| Hungarian association             | ✅ Implemented | `internal/lidar/hungarian.go` — Kuhn-Munkres solver                                 |
| Occlusion coasting                | ✅ Implemented | `internal/lidar/tracking.go` — `MaxMissesConfirmed=15`, `OcclusionCovInflation=0.5` |
| Debug artifacts                   | ✅ Implemented | `internal/lidar/debug/collector.go` — `DebugOverlaySet` via gRPC                    |
| Voxel grid preprocessing          | 📋 Planned     | —                                                                                   |
| Constant acceleration model       | 📋 Planned     | —                                                                                   |
| Feature extraction for ML         | 📋 Planned     | —                                                                                   |

This document proposes concrete improvements to the LiDAR tracking pipeline for street scenes, mapping each proposal to existing code and new API outputs.

---

## Industry standards reference

The tracking upgrades in this document are designed to align with the **7-DOF industry standard** for 3D bounding boxes:

| Specification                 | Document                                                                                 |
| ----------------------------- | ---------------------------------------------------------------------------------------- |
| **7-DOF Bounding Box Format** | [av-lidar-integration-plan.md](../../plans/lidar-av-lidar-integration-plan.md)           |
| **Pose Representation**       | [static-pose-alignment-plan.md](../../plans/lidar-static-pose-alignment-plan.md)         |
| **Background Grid Standards** | [lidar-background-grid-standards.md](../architecture/lidar-background-grid-standards.md) |

The `OrientedBoundingBox` output from OBB estimation (§2.6) conforms to `BoundingBox7DOF` from the AV spec.

---

## 1. Current state

### 1.1 Existing implementation

| Component                 | File                                  | Key Functions/Types                                        |
| ------------------------- | ------------------------------------- | ---------------------------------------------------------- |
| **Background Model**      | `internal/lidar/background.go`        | `BackgroundManager`, `BackgroundGrid`, `BackgroundCell`    |
| **Foreground Extraction** | `internal/lidar/foreground.go`        | `ProcessFramePolarWithMask()`, `ExtractForegroundPoints()` |
| **Clustering**            | `internal/lidar/clustering.go`        | `DBSCAN()`, `WorldCluster`, `SpatialIndex`                 |
| **Tracking**              | `internal/lidar/tracking.go`          | `Tracker`, `TrackedObject`, `TrackState`                   |
| **Pipeline**              | `internal/lidar/tracking_pipeline.go` | `TrackingPipelineConfig`, `NewFrameCallback()`             |
| **Transform**             | `internal/lidar/transform.go`         | `SphericalToCartesian()`, `TransformToWorld()`             |

### 1.2 Current algorithm

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

### 1.3 Known limitations

| Issue                              | Impact                          | Cause                          |
| ---------------------------------- | ------------------------------- | ------------------------------ |
| Ground points leak into foreground | False clusters near sensor      | Height-based filtering missing |
| Clusters split on large vehicles   | Track fragmentation             | DBSCAN eps too small           |
| Tracks merge when objects cross    | ID swaps                        | Greedy association             |
| Heading estimation noisy           | OBB rotation jitter             | No temporal smoothing          |
| No occlusion handling              | Tracks deleted during occlusion | Fixed miss threshold           |

---

## 2. Proposed upgrades

### 2.1 Ground/Background removal

**Current**: Polar-grid EMA model classifies points as foreground/background based on range deviation.

**Proposed**: Add explicit **ground plane removal** before clustering.

**Options**:

| Method              | Pros                 | Cons                | Recommendation   |
| ------------------- | -------------------- | ------------------- | ---------------- |
| Height threshold    | Simple, fast         | Assumes flat ground | Use as baseline  |
| RANSAC plane fit    | Handles slope        | More compute        | Use for accuracy |
| Ring-based gradient | Uses sensor geometry | Complex             | Deferred         |

**Implementation**: `internal/lidar/l4perception/ground.go` — `HeightBandFilter` implementing a `GroundRemover` interface. Filters points outside a configurable height band (default min 0.2 m wheel height, max 3.0 m truck height). Called from `tracking_pipeline.go` after `TransformToWorld()`.

**API Output**: `ground_removed: bool` flag in `PointCloudFrame`.

---

### 2.2 Clustering improvements

**Current**: DBSCAN with fixed `eps=0.6m`, `minPts=12`.

**Proposed**: Adaptive parameters + voxel grid preprocessing.

#### 2.2.1 Voxel grid downsampling

Reduce point count before clustering by hashing points to voxel cells (e.g. 0.1 m resolution) and returning one representative per cell. New file `internal/lidar/l4perception/voxel.go`.

**Tradeoff**: Voxel grid loses density information but improves clustering speed.

#### 2.2.2 Connected components alternative

For dense point clouds, connected components on voxel grid via flood fill may be faster than DBSCAN. Add as an extension to `internal/lidar/l4perception/cluster.go`.

**Recommendation**: Keep DBSCAN as default, add voxel grid preprocessing option.

**API Output**: `ClusterSet.clustering_method` field (enum: `DBSCAN`, `CONNECTED_COMPONENTS`).

---

### 2.3 Association upgrades

**Current**: Greedy nearest-neighbour with Mahalanobis gating.

**Proposed**: Optimal assignment via Hungarian algorithm.

#### 2.3.1 Hungarian (jonker-volgenant) algorithm

Define an `Associator` interface in `internal/lidar/l5tracks/hungarian.go` with `Associate(tracks, clusters, dt) → []Assignment` where each `Assignment` carries track index, cluster index, and cost. Two implementations: `GreedyAssociator` (current nearest-neighbour) and `HungarianAssociator` (Kuhn-Munkres solver with max-cost rejection).

**Mahalanobis Gating**: Current implementation at `tracking.go:317-360` is preserved. Hungarian operates on the cost matrix after gating.

**API Output**: Debug overlay `AssociationCandidate` shows accepted/rejected pairs.

---

### 2.4 Filter model improvements

**Current**: Constant velocity (CV) Kalman filter, 4-state: `[x, y, vx, vy]`.

**Proposed**: Options for enhanced models.

#### 2.4.1 Constant acceleration (CA) model

6-state: `[x, y, vx, vy, ax, ay]`

Better for accelerating/braking vehicles.

#### 2.4.2 Interacting multiple model (IMM)

Blend CV + CA based on motion likelihood.

**Recommendation**: Keep CV as default. Add CA as configuration option. IMM is future work.

Add a `MotionModel` string enum (`cv`, `ca`) to `TrackerConfig` in `internal/lidar/l5tracks/tracking.go`. The CA model extends the Kalman state from 4-state `[x, y, vx, vy]` to 6-state `[x, y, vx, vy, ax, ay]`.

**API Output**: `Track.motion_model` field in proto.

---

### 2.5 Lifecycle and occlusion handling

**Current**: Fixed `MaxMisses=3` before deletion. No occlusion awareness.

**Proposed**: Adaptive lifecycle based on track confidence and occlusion detection.

#### 2.5.1 Confidence-based lifecycle

Extend `TrackedObject` in `internal/lidar/l5tracks/tracking.go` with `Confidence float32` (0–1, based on hit ratio, observation count, track age, covariance magnitude) and `OcclusionState` enum (`none`, `partial`, `full`). `MaxMisses` adapts based on confidence: confirmed high-confidence tracks coast longer.

#### 2.5.2 Occlusion detection

Detect when a track is likely occluded by casting a ray from sensor origin to each track and checking whether another track's bounding box intersects the ray. Implemented as `Tracker.DetectOcclusions()` in `internal/lidar/l5tracks/tracking.go`.

**API Output**: `Track.occlusion_state` field, `Track.confidence` field.

---

### 2.6 OBB estimation and smoothing

**Current**: Only AABB (axis-aligned) bounding boxes. No heading estimation from shape.

**Proposed**: Oriented bounding box (OBB) via PCA + temporal smoothing.

#### 2.6.1 PCA-based OBB

`EstimateOBBFromCluster()` in `internal/lidar/l4perception/obb.go` computes an OBB with fields: centre (x, y, z), extents (length, width, height), and heading (radians around Z). Algorithm: compute centroid → build 2D covariance matrix → eigen decomposition → rotate to principal frame → compute extents → heading from atan2 of first eigenvector.

#### 2.6.2 Temporal smoothing

EMA smoothing on OBB heading (α = 0.3) with circular wraparound (−π to π). Implemented in `internal/lidar/l4perception/obb.go`.

**API Output**: `OrientedBoundingBox` message in `Cluster` proto.

---

### 2.7 Classification hooks

**Current**: Rule-based classifier in `internal/lidar/classification.go` (pedestrian, car, bird, other).

**Proposed**: Feature extraction hooks for ML classifier training.

#### 2.7.1 Feature vector

Two feature structs in `internal/lidar/l6objects/features.go`:

- **ClusterFeatures** (per-frame): point count, bbox extents (L/W/H), height p95, intensity mean/std, elongation (L/W), compactness (pts/volume), vertical spread (σ of Z).
- **TrackFeatures** (aggregated): embeds `ClusterFeatures` plus avg/max speed, speed variance, duration, track length, heading variance, occlusion ratio.

Extraction functions: `ExtractClusterFeatures(cluster, points)` and `ExtractTrackFeatures(track)`.

**API Output**: Optional `features` field in `Track` proto for offline research export.

---

### 2.8 Debug artifacts

**Current**: Limited debug logging via `Debugf()`.

**Proposed**: Structured debug artifacts for visualiser.

#### 2.8.1 Debug collector

`DebugCollector` in `internal/lidar/debug/collector.go` accumulates per-frame debug artifacts: association candidates, gating ellipses, innovation residuals, and state predictions. Four recording methods (`RecordAssociation`, `RecordGating`, `RecordResidual`, `RecordPrediction`) plus `Emit()` to flush the frame.

#### 2.8.2 Integration points

| Location                                   | What to Record                            |
| ------------------------------------------ | ----------------------------------------- |
| `tracking.go:associate()`                  | Association candidates, accepted/rejected |
| `tracking.go:mahalanobisDistanceSquared()` | Gating ellipse parameters                 |
| `tracking.go:update()`                     | Innovation residuals                      |
| `tracking.go:predict()`                    | State predictions                         |

**API Output**: `DebugOverlaySet` in `FrameBundle` proto.

---

## 3. Mapping to API outputs

| Upgrade        | New Proto Fields                            | Debug Overlays         |
| -------------- | ------------------------------------------- | ---------------------- |
| Ground removal | `PointCloudFrame.classification`            | N/A                    |
| Voxel grid     | `ClusterSet.clustering_method`              | N/A                    |
| Hungarian      | N/A                                         | `AssociationCandidate` |
| CA model       | `Track.motion_model`                        | `StatePrediction`      |
| Occlusion      | `Track.occlusion_state`, `Track.confidence` | N/A                    |
| OBB            | `Cluster.obb`, `Track.bbox_heading_rad`     | OBB visualisation      |
| Features       | `Track.features` (optional)                 | N/A                    |

---

## 4. Implementation priority

| Priority | Upgrade                           | Effort | Impact                        |
| -------- | --------------------------------- | ------ | ----------------------------- |
| **P0**   | Ground removal (height threshold) | Low    | High - reduces false clusters |
| **P0**   | OBB estimation                    | Medium | High - heading visualisation  |
| **P1**   | Debug artifacts                   | Medium | High - debugging workflow     |
| **P1**   | OBB temporal smoothing            | Low    | Medium - visual quality       |
| **P2**   | Hungarian association             | Medium | Medium - fewer ID swaps       |
| **P2**   | Occlusion detection               | Medium | Medium - track continuity     |
| **P3**   | Voxel grid preprocessing          | Low    | Low - performance             |
| **P3**   | CA motion model                   | Medium | Low - marginal accuracy       |
| **P3**   | Feature extraction                | Low    | Low - ML prep                 |

---

## 5. Testing strategy

### 5.1 Unit tests

Each upgrade includes unit tests for the new function.

### 5.2 Golden replay tests

- Record baseline tracks with current algorithm
- After upgrade, compare:
  - Track count (should be similar or improved)
  - Track duration (should be similar or improved)
  - ID stability (no regressions)

### 5.3 Visual validation

- Render before/after in visualiser
- Check for improvements in:
  - Ground point removal
  - OBB alignment
  - Track continuity

---

## 6. Related documents

- [architecture.md](../../ui/visualiser/architecture.md) – Architecture and problem statement
- [api-contracts.md](../../ui/visualiser/api-contracts.md) – API contract
- [implementation.md](../../ui/visualiser/implementation.md) – Milestones
