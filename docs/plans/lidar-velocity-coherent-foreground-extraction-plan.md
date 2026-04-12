# Velocity-Coherent foreground extraction

- **Status:** Implementation In Progress (Core Phases 1–5 Simplified; Phases 0, 6–7 Pending)
- **Layers:** L3 Grid, L4 Perception
- **Plan Version:** 2.0
- **Canonical:** [velocity-foreground-extraction.md](../lidar/architecture/velocity-foreground-extraction.md)

- **Note:** This is the living design document and implementation checklist. The active foreground extractor is `ProcessFramePolarWithMask` in [internal/lidar/l3grid/foreground.go](../../internal/lidar/l3grid/foreground.go); the active clustering is DBSCAN in [internal/lidar/l4perception/cluster.go](../../internal/lidar/l4perception/cluster.go). No `VelocityCoherentTracker` exists yet in the codebase. Core phases 1–5 have prototype implementations with simplifications; see [Implementation Notes](#implementation-notes-january-2026) for detail.
  > The mathematical model and parameter tradeoffs are also documented in:
  > [`data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md`](../../data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md)

---

> **Problem statement, core concept, algorithm overview, and acceptance metrics:** see [velocity-foreground-extraction.md](../lidar/architecture/velocity-foreground-extraction.md).

## Scope

### In scope

- Per-point velocity estimation across frames
- Position+velocity clustering (6D metric behaviour)
- Long-tail lifecycle states (pre-tail, post-tail)
- Sparse continuation down to 3 points with stricter velocity checks
- Fragment merge heuristics for split tracks
- Dual-source storage/API for side-by-side evaluation

### Out of scope (for this plan)

- Replacing the existing background-subtraction pipeline
- New ML model training
- New sensor calibration procedures

---

## Phase 0: instrumentation and fixtures

**Goal:** Establish repeatable evaluation before algorithm changes so that improvements can be measured objectively.

### Checklist

- [ ] Define evaluation datasets (PCAP segments: dense traffic, sparse/distant traffic, occlusion-heavy)
- [ ] Add baseline metrics capture for current pipeline
- [ ] Add benchmark harness for frame throughput and memory
- [ ] Lock acceptance report format for side-by-side comparisons

### Exit criteria

- [ ] Reproducible baseline report generated from one command
- [ ] Baseline includes precision/recall proxy, track duration, fragmentation rate, and throughput

---

## Phase 1: point-level velocity estimation

**Goal:** Compute velocity vectors and confidence for points with stable frame-to-frame correspondence.

### Objective by finding correspondences between consecutive frames.

### Algorithm: nearest-neighbour with velocity constraint

**`PointVelocity` struct** — a point with estimated velocity:

| Field                   | Type    | Notes                                |
| ----------------------- | ------- | ------------------------------------ |
| `X, Y, Z`               | float64 | Position (world frame)               |
| `VX, VY, VZ`            | float64 | Estimated velocity (m/s)             |
| `VelocityConfidence`    | float32 | Confidence [0, 1]                    |
| `CorrespondingPointIdx` | int     | Index in previous frame (−1 if none) |
| `TimestampNanos`        | int64   | Point timestamp                      |

**`EstimatePointVelocities` function** — computes per-point velocities from frame correspondence. Accepts `currentFrame`, `previousFrame` (both `[]WorldPoint`), `prevVelocities` (`[]PointVelocity`), `dtSeconds` (float64), and `config` (VelocityEstimationConfig). Returns `[]PointVelocity`.

Algorithm:

1. Build 3D spatial index for previous frame using `config.SearchRadius`
2. For each current point, optionally back-project search position using median local velocity from `prevVelocities`
3. Query previous-frame candidates within `SearchRadius`
4. If no candidates: confidence 0, index −1
5. Otherwise, `selectBestCorrespondence` by combined position + velocity consistency
6. Compute velocity as `(curr − prev) / dt`, assign confidence from match score

### Velocity confidence scoring

Velocity confidence is computed based on:

1. **Spatial distance**: Closer correspondences are more confident
2. **Velocity consistency**: Similar velocity to neighbours increases confidence
3. **Magnitude plausibility**: Reject physically impossible velocities (>50 m/s for vehicles)

**`computeVelocityConfidence` function** — combines three factors:

1. **Spatial proximity score**: $e^{-d / r}$ where $d$ is spatial distance and $r$ is `SearchRadius`
2. **Velocity plausibility score**: $1 - v / v_{\max}$; returns 0 if $v > v_{\max}$
3. **Neighbour consistency score**: $e^{-\sigma^2 / \tau}$ where $\sigma^2$ is neighbour velocity variance and $\tau$ is `VelocityVarianceThreshold`

Final confidence is the product of all three scores, cast to float32.

### Configuration parameters

**`VelocityEstimationConfig` struct:**

| Field                       | Type    | Default | Purpose                                   |
| --------------------------- | ------- | ------- | ----------------------------------------- |
| `SearchRadius`              | float64 | 2.0 m   | Max correspondence search distance        |
| `MaxVelocityMps`            | float64 | 50.0    | Vehicle max (~180 km/h)                   |
| `VelocityVarianceThreshold` | float64 | 2.0 m/s | Variance threshold for consistency score  |
| `MinConfidence`             | float32 | 0.3     | Minimum confidence to accept velocity     |
| `NeighborRadius`            | float64 | 1.0 m   | Radius for local velocity context         |
| `MinNeighborsForContext`    | int     | 3       | Minimum neighbours for context estimation |

### Checklist

- [ ] Create `internal/lidar/velocity_estimation.go`
- [ ] Implement correspondence search with configurable radius and plausibility gates
- [ ] Implement velocity confidence scoring
- [ ] Add `internal/lidar/velocity_estimation_test.go` with synthetic and replayed edge cases
- [ ] Add config wiring for velocity estimation parameters

### Exit criteria

- [ ] Velocity output generated for >95% of matchable points on validation segments
- [ ] Implausible velocity rates bounded by configured threshold
- [ ] Unit tests cover no-match, ambiguous-match, and high-noise cases

---

## Phase 2: velocity-coherent clustering

**Goal:** Cluster points using position+velocity coherence and support `MinPts=3` mode.

### Objective

Group points that are both **spatially close** and **moving together** (similar velocity vectors).

### Algorithm: 6D DBSCAN

Standard DBSCAN operates in 3D (x, y, z). We extend to 6D: (x, y, z, vx, vy, vz).

The key insight is that **two points belong to the same object if they are close in position AND have similar velocities**.

**`VelocityCoherentCluster` struct:**

| Field                              | Type    | Notes                            |
| ---------------------------------- | ------- | -------------------------------- |
| `ClusterID`                        | int64   | Unique cluster identifier        |
| `CentroidX, CentroidY, CentroidZ`  | float64 | World-frame centroid             |
| `VelocityX, VelocityY, VelocityZ`  | float64 | World-frame velocity (m/s)       |
| `PointCount`                       | int     | Number of points                 |
| `VelocityConfidence`               | float32 | Average confidence across points |
| `PointIndices`                     | []int   | Indices into source array        |
| `BoundingBoxLength, Width, Height` | float32 | OBB dimensions                   |
| `HeightP95`                        | float32 | 95th percentile height           |
| `IntensityMean`                    | float32 | Mean intensity                   |
| `TSUnixNanos`                      | int64   | Timestamp                        |

**`DBSCAN6D` function** — clusters points in position-velocity space. Accepts `[]PointVelocity` and `Clustering6DConfig`, returns `[]VelocityCoherentCluster`. Algorithm:

1. Build 6D spatial index with separate `PositionEps` and `VelocityEps`
2. For each unvisited point, run 6D region query (position + velocity)
3. If neighbours ≥ `MinPts` (default 3, reduced from 12): start new cluster, expand
4. Otherwise mark as noise
5. Build `VelocityCoherentCluster` records from labels

### 6D distance metric

**`Distance6D` function** — computes weighted distance in position-velocity space. Calculates Euclidean 3D position distance and Euclidean 3D velocity distance separately, then returns `positionWeight * positionDist + velocityWeight * velocityDist` (typically weights 1.0 and 2.0 respectively, making velocity more important).

### Minimum cluster size reduction

**Critical change**: Reduce `MinPts` from 12 to **3** for velocity-coherent clustering.

Justification:

- Velocity coherence provides strong confirmation (points moving together)
- Human eye can identify objects with 3 consistent points
- Distant/sparse objects produce fewer returns but still have coherent motion

**`Clustering6DConfig` struct:**

| Field                   | Type    | Default | Purpose                                      |
| ----------------------- | ------- | ------- | -------------------------------------------- |
| `PositionEps`           | float64 | 0.6 m   | Spatial clustering radius                    |
| `VelocityEps`           | float64 | 1.0 m/s | Velocity clustering radius                   |
| `MinPts`                | int     | 3       | Reduced from 12 (velocity confirms identity) |
| `PositionWeight`        | float64 | 1.0     | Weight for position in distance metric       |
| `VelocityWeight`        | float64 | 2.0     | Weight for velocity in distance metric       |
| `MinVelocityConfidence` | float32 | 0.3     | Minimum confidence filter                    |

### Checklist

- [ ] Create `internal/lidar/clustering_6d.go`
- [ ] Implement 6D neighbourhood metric (position + velocity weighting)
- [ ] Implement minimum-point behaviour with sparse guardrails
- [ ] Add `internal/lidar/clustering_6d_test.go`
- [ ] Validate cluster stability versus existing DBSCAN on replay data

### Exit criteria

- [ ] 3-point sparse clusters are accepted only with velocity coherence
- [ ] False-positive growth stays within agreed threshold versus baseline
- [ ] Runtime impact is measured and documented

---

## Phase 3: long-tail track management

**Goal:** Extend track continuity at object entry and exit boundaries.

### Objective

Extend track lifetimes to capture:

- **Pre-tail**: Objects entering the sensor field (before full clustering)
- **Post-tail**: Objects exiting the sensor field (after point density drops)

### Pre-Tail detection: velocity-predicted entry

When a new cluster appears, we check if it matches the predicted position of an object that should be entering the field of view based on its extrapolated trajectory from previous sparse observations.

**`PredictedEntryZone` struct** — fields: `PredictedX, PredictedY` (extrapolated position), `VelocityX, VelocityY` (velocity vector), `UncertaintyRadius` (grows with time), `SourceTrackID`, `PredictionTimeNanos`, `FramesSinceObservation`.

**`PreTailDetector` struct** — maintains `EntryZones` ([]PredictedEntryZone), `FieldOfViewBoundary` (PolygonBoundary), and `Config` (PreTailConfig).

**`PreTailDetector.Update` method** — for each entry zone, searches new clusters near the predicted entry point. If distance < `UncertaintyRadius` and velocity matches above `MinVelocityMatchScore`, creates a `TrackAssociation` with type `AssociationPreTail` linking the cluster to the source track.

### Post-Tail continuation: prediction window

Instead of deleting tracks after `MaxMisses` frames, we continue predicting their position and attempt to recover them when points reappear.

**`PostTailConfig` struct:**

| Field                   | Type    | Default | Purpose                                           |
| ----------------------- | ------- | ------- | ------------------------------------------------- |
| `MaxPredictionFrames`   | int     | 30      | Max frames to continue prediction (~3 s at 10 Hz) |
| `MaxUncertaintyRadius`  | float64 | 10.0 m  | Abandon track beyond this radius                  |
| `MinRecoveryConfidence` | float32 | 0.5     | Minimum confidence to recover                     |

**`ContinuePostTail` method** — on `VelocityCoherentTracker`. For deleted/missing tracks with known velocity:

1. Compute frames since last observation; return nil if > `MaxPredictionFrames`
2. Predict current position: $x_{pred} = x_{last} + v_x \cdot \Delta t$
3. Grow uncertainty: $r = r_{base} + n_{frames} \cdot r_{growth}$; return nil if > `MaxUncertaintyRadius`
4. Return `PredictedPosition` with track ID, predicted coordinates, velocity, uncertainty, and frame gap

### Extended track state machine

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                                                         │
                    │  ┌─────────────┐                                        │
    New sparse      │  │             │                                        │
    observation ───►│  │  PRE_TAIL   │◄───────────────────────────────────┐   │
                    │  │  (≥3 pts)   │                                    │   │
                    │  │             │                                    │   │
                    │  └──────┬──────┘                                    │   │
                    │         │                                           │   │
                    │         │ Velocity confirmed                        │   │
                    │         │ over 3+ frames                            │   │
                    │         ▼                                           │   │
                    │  ┌─────────────┐                                    │   │
                    │  │             │                                    │   │
                    │  │ TENTATIVE   │                                    │   │
                    │  │             │                                    │   │
                    │  └──────┬──────┘                                    │   │
                    │         │                                           │   │
                    │         │ HitsToConfirm                             │   │
                    │         │ (3 consecutive)                           │   │
                    │         ▼                                           │   │
                    │  ┌─────────────┐                                    │   │
                    │  │             │                                    │   │
                    │  │ CONFIRMED   │◄───────────────────────────────┐   │   │
                    │  │             │                                │   │   │
                    │  └──────┬──────┘                                │   │   │
                    │         │                                       │   │   │
                    │         │ MaxMisses                             │   │   │
                    │         │ (but velocity known)                  │   │   │
                    │         ▼                                       │   │   │
                    │  ┌─────────────┐                                │   │   │
                    │  │             │  Recovery with     ────────────┘   │   │
                    │  │  POST_TAIL  │  velocity match                    │   │
                    │  │ (predicted) │                                    │   │
                    │  │             │  Pre-entry match   ────────────────┘   │
                    │  └──────┬──────┘                                        │
                    │         │                                               │
                    │         │ MaxPredictionFrames                           │
                    │         │ or MaxUncertainty                             │
                    │         ▼                                               │
                    │  ┌─────────────┐                                        │
                    │  │             │                                        │
                    │  │  DELETED    │                                        │
                    │  │             │                                        │
                    │  └─────────────┘                                        │
                    │                                                         │
                    └─────────────────────────────────────────────────────────┘
```

### Checklist

- [ ] Create `internal/lidar/long_tail.go`
- [ ] Add pre-tail predicted entry association logic
- [ ] Add post-tail prediction window and uncertainty growth logic
- [ ] Extend track states and transitions
- [ ] Add `internal/lidar/long_tail_test.go`

### Exit criteria

- [ ] Mean track duration increases on boundary-entry/exit scenarios
- [ ] Recovery after brief occlusions improves without large precision drop
- [ ] State machine transitions are fully test-covered

---

## Phase 4: sparse continuation logic

**Goal:** Preserve track identity through low-point-count observations.

### Objective

Maintain track identity even when point count drops to ~3 points, using velocity coherence as the primary confirmation signal.

### Sparse track criteria

**`SparseTrackConfig` struct:**

| Field                            | Type    | Default | Purpose                                    |
| -------------------------------- | ------- | ------- | ------------------------------------------ |
| `MinPointsAbsolute`              | int     | 3       | Floor for track maintenance                |
| `MinVelocityConfidenceForSparse` | float32 | 0.6     | Higher confidence required when sparse     |
| `MaxVelocityVarianceForSparse`   | float64 | 0.5 m/s | Velocity must closely match existing track |
| `MaxSpatialSpreadForSparse`      | float64 | 2.0 m   | Max bounding box dimension                 |

**`IsSparseTrackValid` function** — checks whether a sparse cluster can maintain track identity. Returns `(valid bool, confidence float32)`. Validation gates (all must pass):

1. Point count ≥ `MinPointsAbsolute`
2. Velocity confidence ≥ threshold
3. Velocity difference from existing track ≤ `MaxVelocityVarianceForSparse`
4. Spatial spread ≤ threshold

Confidence score: `velocityMatchScore × pointScore × velocityConfidence` where `pointScore` scales 3–10 points to 0.3–1.0.

### Graceful degradation strategy

As point count decreases, we progressively tighten velocity constraints:

| Point Count | Velocity Tolerance | Spatial Tolerance | Notes                          |
| ----------- | ------------------ | ----------------- | ------------------------------ |
| ≥12         | ±2.0 m/s           | ±1.0 m            | Standard DBSCAN clustering     |
| 6-11        | ±1.5 m/s           | ±0.8 m            | Reduced tolerance              |
| 3-5         | ±0.5 m/s           | ±0.5 m            | Strict velocity match required |
| <3          | N/A                | N/A               | Rely on prediction only        |

**`adaptiveTolerances` method** — returns `(velTol, spatialTol float64)` based on point count, matching the table above. Returns (0, 0) for <3 points (prediction only).

### Checklist

- [ ] Create `internal/lidar/sparse_continuation.go`
- [ ] Implement adaptive tolerances by point count
- [ ] Enforce confidence and variance gates for 3–5 point frames
- [ ] Integrate sparse continuation decisions into tracker updates
- [ ] Add targeted tests for 3-point continuation and failure boundaries

### Exit criteria

- [ ] Sparse tracks are maintained when motion is coherent
- [ ] No significant increase in ID switches in sparse scenes
- [ ] Parameter sensitivity documented for tuning

---

## Phase 5: track fragment merging

**Goal:** Merge split track fragments when kinematics are consistent.

### Objective

Reconnect track fragments that were split due to:

- Occlusion gaps exceeding MaxMisses
- Sensor noise causing temporary point loss
- Objects passing through blind spots

### Fragment detection

**`TrackFragment` struct:**

| Field             | Type            | Notes                               |
| ----------------- | --------------- | ----------------------------------- |
| `Track`           | \*TrackedObject | Source track                        |
| `EntryPoint`      | [2]float32      | Position where track first appeared |
| `ExitPoint`       | [2]float32      | Position where track last appeared  |
| `EntryVelocity`   | [2]float32      | Velocity at entry                   |
| `ExitVelocity`    | [2]float32      | Velocity at exit                    |
| `StartNanos`      | int64           | First timestamp                     |
| `EndNanos`        | int64           | Last timestamp                      |
| `HasNaturalEntry` | bool            | Started from sensor boundary        |
| `HasNaturalExit`  | bool            | Ended at sensor boundary            |

**`DetectFragments` function** — iterates tracks with ≥2 history points. Computes entry/exit velocities from first/last two history points. Checks if entry/exit positions are near the `sensorBoundary` (within 2.0 m) to set `HasNaturalEntry`/`HasNaturalExit` flags. Returns `[]TrackFragment`.

### Fragment matching algorithm

**`MergeConfig` struct:**

| Field                     | Type    | Default | Purpose                         |
| ------------------------- | ------- | ------- | ------------------------------- |
| `MaxTimeGapSeconds`       | float64 | 5.0     | Max gap between fragments       |
| `MaxPositionErrorMeters`  | float64 | 3.0     | Predicted vs actual entry error |
| `MaxVelocityDifferenceMs` | float64 | 2.0     | Velocity difference at junction |
| `MinAlignmentScore`       | float32 | 0.7     | Minimum overall score           |

**`MergeCandidatePair` struct** — links two `*TrackFragment` records with `PositionScore`, `VelocityScore`, `TrajectoryScore`, and `OverallScore` (all float32).

**`FindMergeCandidates` function** — sorts fragments by start time, then for each pair where the earlier track has a non-natural exit and the later track has a non-natural entry:

1. Check time gap is in range (0, `MaxTimeGapSeconds`]
2. Predict earlier track’s position at `later.StartNanos` using exit velocity
3. Check position error ≤ `MaxPositionErrorMeters`
4. Check velocity difference ≤ `MaxVelocityDifferenceMs`
5. Compute scores: `posScore = 1 − posError/maxError`, `velScore = 1 − velDiff/maxDiff`, `trajectoryScore` from alignment function
6. `overallScore = (posScore + velScore + trajectoryScore) / 3`; accept if ≥ `MinAlignmentScore`

### Merge execution

**`MergeTrackFragments` function** — combines two `*TrackedObject` records into one:

- Keeps earlier track’s ID; lifecycle spans both fragments (`FirstUnixNanos` from earlier, `LastUnixNanos` from later)
- Kalman state (position, velocity, covariance) taken from the later track (most recent)
- Aggregate statistics summed: `Hits`, `ObservationCount`
- History arrays concatenated; if gap is >0 and <5 s, interpolated points are inserted
- `ComputeQualityMetrics()` and `recomputeAggregatedFeatures()` called on merged result

### Checklist

- [ ] Create `internal/lidar/track_merge.go`
- [ ] Implement candidate generation using time/position/velocity gates
- [ ] Implement merge scoring and deterministic tie-breaking
- [ ] Add `internal/lidar/track_merge_test.go`
- [ ] Record merge decisions for audit/debug

### Exit criteria

- [ ] Fragmentation rate decreases on occlusion-heavy validation runs
- [ ] Incorrect merge rate remains below agreed threshold
- [ ] Merge audit trail is queryable

---

## Phase 6: pipeline, storage, and API integration

**Goal:** Run current and velocity-coherent paths in parallel and expose both results via API and storage.

### Checklist

- [ ] Create `internal/lidar/velocity_coherent_tracker.go`
- [ ] Create dual extraction orchestration path (parallel source processing)
- [ ] Add storage schema for velocity-coherent clusters/tracks (see [Database Schema Extensions](#database-schema-extensions))
- [ ] Add API source selector (`background_subtraction`, `velocity_coherent`, `all`)
- [ ] Add migration and rollback notes

### Exit criteria

- [ ] Both sources can be queried independently and jointly
- [ ] Dashboard comparison can be generated from stored results
- [ ] No regression in existing source behaviour

---

## Phase 7: validation and rollout

**Goal:** Decide production readiness from measured outcomes.

### Checklist

- [ ] Run full replay evaluation across selected PCAP suites
- [ ] Compare against baseline using agreed metrics
- [ ] Document default parameter set and safe bounds
- [ ] Add ops runbook (alerts, fallbacks, troubleshooting)
- [ ] Stage rollout behind feature flag

### Exit criteria

- [ ] Acceptance thresholds met on continuity and sparse-object capture
- [ ] Throughput and memory remain within service budget
- [ ] Rollback path verified in staging

---

## Data structures

### Core types

**`VelocityCoherentTrackerConfig` struct** — nests all per-phase config structs:

| Field                | Type                     | Phase    |
| -------------------- | ------------------------ | -------- |
| `VelocityEstimation` | VelocityEstimationConfig | 1        |
| `Clustering`         | Clustering6DConfig       | 2        |
| `PreTail`            | PreTailConfig            | 3        |
| `PostTail`           | PostTailConfig           | 3        |
| `SparseContinuation` | SparseTrackConfig        | 4        |
| `Merge`              | MergeConfig              | 5        |
| `Tracking`           | TrackerConfig            | Existing |

**`DefaultVelocityCoherentConfig` defaults:**

| Sub-config         | Field                           | Default             |
| ------------------ | ------------------------------- | ------------------- |
| VelocityEstimation | SearchRadius                    | 2.0                 |
| VelocityEstimation | MaxVelocityMps                  | 50.0                |
| VelocityEstimation | VelocityVarianceThreshold       | 2.0                 |
| VelocityEstimation | MinConfidence                   | 0.3                 |
| VelocityEstimation | NeighborRadius                  | 1.0                 |
| VelocityEstimation | MinNeighborsForContext          | 3                   |
| Clustering         | PositionEps                     | 0.6                 |
| Clustering         | VelocityEps                     | 1.0                 |
| Clustering         | MinPts                          | 3 (reduced from 12) |
| Clustering         | PositionWeight                  | 1.0                 |
| Clustering         | VelocityWeight                  | 2.0                 |
| Clustering         | MinVelocityConfidence           | 0.3                 |
| PreTail            | EntryPredictionWindow           | 30 frames           |
| PreTail            | MinVelocityMatchScore           | 0.6                 |
| PreTail            | BoundaryMarginMeters            | 2.0                 |
| PostTail           | MaxPredictionFrames             | 30                  |
| PostTail           | MaxUncertaintyRadius            | 10.0                |
| PostTail           | MinRecoveryConfidence           | 0.5                 |
| SparseContinuation | MinPointsAbsolute               | 3                   |
| SparseContinuation | MinVelocityConfidenceForSparse  | 0.6                 |
| SparseContinuation | MaxVelocityVarianceForSparse    | 0.5                 |
| SparseContinuation | MaxSpatialSpreadForSparse       | 2.0                 |
| Merge              | MaxTimeGapSeconds               | 5.0                 |
| Merge              | MaxPositionErrorMeters          | 3.0                 |
| Merge              | MaxVelocityDifferenceMs         | 2.0                 |
| Merge              | MinAlignmentScore               | 0.7                 |
| Tracking           | _(from DefaultTrackerConfig())_ | —                   |

### Point history ring buffer

For efficient frame-to-frame correspondence:

**`FrameHistory`** — ring buffer of recent frames (fields: `Frames []PointVelocityFrame`, `Capacity int`, `WriteIndex int`). `Add` overwrites at `WriteIndex` and advances mod `Capacity`. `Previous(offset)` returns the frame `offset` steps before the most recent, or nil if out of range.

**`PointVelocityFrame`** — holds `Points []PointVelocity`, `Timestamp time.Time`, and `SpatialIndex *SpatialIndex6D` for efficient neighbourhood queries.

---

## Integration with existing system

### Dual-Source architecture (matching radar pattern)

Just as the radar system maintains two independent transit sources:

- **`radar_objects`**: Hardware classifier from OPS243 sensor
- **`radar_data_transits`**: Software sessionisation algorithm

The LIDAR system will maintain two independent track sources:

- **`lidar_tracks`**: Existing background-subtraction + DBSCAN clustering (MinPts=12)
- **`lidar_velocity_coherent_tracks`**: New velocity-coherent extraction (MinPts=3)

Both track sources are:

1. **Stored independently** in separate database tables
2. **Queryable via API** with a `source` parameter to select which algorithm
3. **Comparable in dashboards** for performance evaluation
4. **Compatible with the same downstream analysis** (speed summaries, classification, etc.)

The API accepts a `source` query parameter: `GET /api/lidar/tracks?source=background_subtraction`, `GET /api/lidar/tracks?source=velocity_coherent`, or `GET /api/lidar/tracks?source=all` (returns both with source labels).

`TrackSource` is a string constant: either `"background_subtraction"` or `"velocity_coherent"`. `TrackWithSource` wraps a `TrackedObject` with its `Source` label.

### Parallel processing path

The velocity-coherent extraction runs **alongside** the existing background-subtraction system, not replacing it:

**`DualExtractionPipeline.ProcessFrame`** runs three parallel paths:

1. **Path 1 — Background subtraction:** Apply background mask to polar points → extract foreground → transform to world frame → standard DBSCAN (MinPts=12)
2. **Path 2 — Velocity-coherent extraction:** Transform _all_ polar points to world frame (no background filter) → estimate per-point velocities → 6D DBSCAN (MinPts=3)
3. **Path 3 — Merge:** Take union of both cluster sets using `mergeClusterSets` with a configurable merge threshold

The tracker is updated with the merged cluster set. The returned `FrameResult` includes `BackgroundClusters`, `VelocityCoherentClusters`, `MergedClusters`, and `ActiveTracks`.

### REST API extensions

**Additional REST endpoints:**

| Method | Path                                            | Purpose                                       |
| ------ | ----------------------------------------------- | --------------------------------------------- |
| GET    | `/api/lidar/velocity-tracks/active`             | Active tracks with velocity confidence scores |
| GET    | `/api/lidar/tracks/{track_id}/velocity-profile` | Velocity history for a track                  |
| GET    | `/api/lidar/merge-candidates`                   | Detected fragment merge opportunities         |
| POST   | `/api/lidar/merge-tracks`                       | Manually merge two track fragments            |

The `POST /api/lidar/merge-tracks` body contains `earlier_track_id` and `later_track_id` (both strings).

---

## Database schema extensions

**`lidar_velocity_coherent_clusters` table** — 6D DBSCAN output:

| Column                                     | Type    | Notes                      |
| ------------------------------------------ | ------- | -------------------------- |
| `cluster_id`                               | INTEGER | Primary key                |
| `sensor_id`                                | TEXT    | Not null                   |
| `ts_unix_nanos`                            | INTEGER | Not null                   |
| `centroid_x`, `centroid_y`, `centroid_z`   | REAL    | World-frame position       |
| `velocity_x`, `velocity_y`, `velocity_z`   | REAL    | World-frame velocity (m/s) |
| `velocity_confidence`                      | REAL    | —                          |
| `points_count`                             | INTEGER | —                          |
| `bounding_box_length`, `_width`, `_height` | REAL    | —                          |

**`lidar_velocity_coherent_tracks` table** — parallel to `lidar_tracks`:

| Column                                                 | Type    | Notes                                                  |
| ------------------------------------------------------ | ------- | ------------------------------------------------------ |
| `track_id`                                             | TEXT    | Primary key                                            |
| `sensor_id`                                            | TEXT    | Not null                                               |
| `track_state`                                          | TEXT    | pre_tail / tentative / confirmed / post_tail / deleted |
| `start_unix_nanos`                                     | INTEGER | Not null                                               |
| `end_unix_nanos`                                       | INTEGER | —                                                      |
| `observation_count`                                    | INTEGER | —                                                      |
| `avg_speed_mps`, `peak_speed_mps`                      | REAL    | Kinematics                                             |
| `avg_velocity_confidence`                              | REAL    | Estimation quality                                     |
| `velocity_consistency_score`                           | REAL    | Stability across observations                          |
| `bounding_box_length_avg`, `_width_avg`, `_height_avg` | REAL    | Shape features                                         |
| `height_p95_max`                                       | REAL    | —                                                      |
| `min_points_observed`                                  | INTEGER | Sparse tracking metrics                                |
| `sparse_frame_count`                                   | INTEGER | Frames with <12 points                                 |
| `object_class`                                         | TEXT    | Classification                                         |
| `object_confidence`                                    | REAL    | —                                                      |
| `classification_model`                                 | TEXT    | —                                                      |

**`lidar_track_merges` table** — merge audit trail:

| Column                                                                  | Type    | Notes         |
| ----------------------------------------------------------------------- | ------- | ------------- |
| `merge_id`                                                              | INTEGER | Primary key   |
| `merged_at`                                                             | INTEGER | Not null      |
| `earlier_track_id`, `later_track_id`                                    | TEXT    | Not null      |
| `result_track_id`                                                       | TEXT    | Not null      |
| `position_score`, `velocity_score`, `trajectory_score`, `overall_score` | REAL    | Merge quality |
| `gap_seconds`                                                           | REAL    | —             |
| `interpolated_points`                                                   | INTEGER | —             |

**Indexes:** `idx_vc_tracks_sensor` on sensor_id, `idx_vc_tracks_state` on track_state, `idx_vc_tracks_time` on (start_unix_nanos, end_unix_nanos), `idx_velocity_coherent_clusters_time` on ts_unix_nanos, `idx_track_merges_result` on result_track_id.

---

## Implementation roadmap

### Phase timeline

| Phase | Description                     | Duration  | Priority | Dependencies |
| ----- | ------------------------------- | --------- | -------- | ------------ |
| 0     | Instrumentation and Fixtures    | 1 week    | P0       | None         |
| 1     | Point-Level Velocity Estimation | 1–2 weeks | P0       | Phase 0      |
| 2     | 6D DBSCAN Clustering            | 1 week    | P0       | Phase 1      |
| 3     | Long-Tail Track Management      | 1–2 weeks | P1       | Phase 2      |
| 4     | Sparse Continuation Logic       | 1 week    | P1       | Phase 2      |
| 5     | Track Fragment Merging          | 1–2 weeks | P2       | Phases 3, 4  |
| 6     | Pipeline, Storage, and API      | 1 week    | P1       | All phases   |
| 7     | Validation and Rollout          | 1–2 weeks | P1       | Phase 6      |

### Implementation files

| Phase | File                                          | Description                    |
| ----- | --------------------------------------------- | ------------------------------ |
| 1     | `internal/lidar/velocity_estimation.go`       | Per-point velocity computation |
| 1     | `internal/lidar/velocity_estimation_test.go`  | Unit tests                     |
| 2     | `internal/lidar/clustering_6d.go`             | 6D DBSCAN implementation       |
| 2     | `internal/lidar/clustering_6d_test.go`        | Unit tests                     |
| 3     | `internal/lidar/long_tail.go`                 | Pre-tail and post-tail logic   |
| 3     | `internal/lidar/long_tail_test.go`            | Unit tests                     |
| 4     | `internal/lidar/sparse_continuation.go`       | Sparse track validation        |
| 5     | `internal/lidar/track_merge.go`               | Fragment detection and merging |
| 5     | `internal/lidar/track_merge_test.go`          | Unit tests                     |
| 6     | `internal/lidar/velocity_coherent_tracker.go` | Combined pipeline              |
| 6     | `internal/lidar/monitor/velocity_api.go`      | REST endpoints                 |

---

## Acceptance metrics

These targets are hypotheses to validate against measured outcomes, not committed production guarantees.

| Metric                                              | Target                |
| --------------------------------------------------- | --------------------- |
| Sparse-object recall (3–11 points)                  | +20% to +40% relative |
| Track fragmentation rate                            | −10% to −25% relative |
| Median track duration for boundary-crossing objects | +10% to +30% relative |
| Additional false positives vs baseline              | <+10%                 |
| Throughput regression at target frame rate          | <20%                  |

---

## Risks and mitigations

| Risk                                                     | Mitigation                                                                  |
| -------------------------------------------------------- | --------------------------------------------------------------------------- |
| Low `MinPts` increases noise clusters                    | Strict velocity-confidence and variance gates in sparse mode                |
| Over-aggressive post-tail prediction causes ghost tracks | Uncertainty growth caps and hard prediction timeout                         |
| Incorrect fragment merges corrupt track statistics       | Conservative merge threshold + auditable merge logs in `lidar_track_merges` |
| Runtime overhead from added matching/clustering          | Bounded neighbourhood queries and benchmark gates in CI                     |

---

## Dependencies

- Stable world-frame transform quality from existing pose pipeline
- Representative replay datasets with known corner cases (dense traffic, sparse/distant traffic, occlusion-heavy)
- Dashboard/query support for dual-source comparison

---

## Milestones

| Milestone | Completion Criteria                                                         |
| --------- | --------------------------------------------------------------------------- |
| M0        | Reproducible baseline report generated from one command                     |
| M1        | Phase 1 complete: per-point velocities with confidence scores validated     |
| M2        | Phase 2 complete: stable sparse clustering with MinPts=3                    |
| M3        | Phases 3–4 complete: long-tail states and sparse continuation working       |
| M4        | Phase 5 complete: audited fragment merging with queryable merge trail       |
| M5        | Phases 6–7 complete: dual-source API, storage, validation, rollout decision |

---

## Appendix: mathematical formulation

### A. point correspondence optimisation

Given frames $F_{n-1}$ and $F_n$, find optimal point correspondences $C: F_{n-1} \to F_n$ that minimise:

$$L(C) = \sum_{i \in F_n} \left[ w_{\text{pos}} \cdot d_{\text{pos}}(i, C(i)) + w_{\text{vel}} \cdot d_{\text{vel}}(i, C(i)) \right]$$

Where:

- $d_{\text{pos}}(i, j)$ = Euclidean distance between points $i$ and $j$
- $d_{\text{vel}}(i, j)$ = Velocity consistency with local neighbourhood
- $w_{\text{pos}}, w_{\text{vel}}$ = Weighting factors

### B. 6D distance metric

For points $p = (x, y, z, v_x, v_y, v_z)$ and $q = (x', y', z', v_x', v_y', v_z')$:

$$D_{6D}(p, q) = \sqrt{ \alpha(\Delta x^2 + \Delta y^2 + \Delta z^2) + \beta(\Delta v_x^2 + \Delta v_y^2 + \Delta v_z^2) }$$

Where:

- $\alpha$ = position weight (default 1.0)
- $\beta$ = velocity weight (default 2.0)
- Higher $\beta$ emphasises velocity coherence over spatial proximity

### C. merge trajectory alignment score

For fragments $A$ (ending at $t_1$) and $B$ (starting at $t_2$), compute trajectory alignment:

$$S_{\text{trajectory}} = \cos(\theta_{\text{exit}}, \theta_{\text{entry}}) \cdot \exp\!\left(\frac{-|v_{\text{exit}} - v_{\text{entry}}|}{\sigma_v}\right)$$

Where:

- $\theta_{\text{exit}}$ = heading angle at $A$'s exit
- $\theta_{\text{entry}}$ = heading angle at $B$'s entry
- $v_{\text{exit}}, v_{\text{entry}}$ = speed magnitudes
- $\sigma_v$ = velocity tolerance parameter

---

## Implementation notes (January 2026)

### Simplifications applied vs. original design

The prototype implementation applies practical simplifications for the traffic monitoring use case:

| Design Section          | Original Spec                          | Implementation                           | Rationale                      |
| ----------------------- | -------------------------------------- | ---------------------------------------- | ------------------------------ |
| **Tracking Model**      | 6D (x,y,z,vx,vy,vz)                    | 2D+velocity (x,y,vx,vy)                  | Ground-plane assumption valid  |
| **6D DBSCAN**           | Full 6D spatial index                  | Sequential 3D position + velocity filter | Simpler, reuses existing index |
| **MinPts**              | 3                                      | 3 ✅                                     | Implemented as designed        |
| **Velocity Estimation** | Frame correspondence + back-projection | Frame correspondence ✅                  | Implemented as designed        |
| **Long-Tail Tracking**  | Pre-tail + post-tail states            | Implemented ✅                           | Working as designed            |
| **Fragment Merging**    | Full kinematic matching                | Basic implementation ✅                  | Simplified merge criteria      |
| **Sparse Continuation** | Adaptive tolerances by point count     | Implemented ✅                           | Working as designed            |

### Key implementation files (prototype)

| Phase       | Design Section | Implementation File                                |
| ----------- | -------------- | -------------------------------------------------- |
| Phase 1     | §8 above       | `velocity_estimation.go`                           |
| Phase 2     | §9 above       | `velocity_coherent_clustering.go`                  |
| Phase 3     | §10 above      | `velocity_coherent_tracking.go`                    |
| Phase 4     | §11 above      | `velocity_coherent_tracking.go`                    |
| Phase 5     | §12 above      | `velocity_coherent_merging.go`                     |
| Integration | §13–14 above   | `dual_pipeline.go`, `velocity_coherent_tracker.go` |

### What was NOT implemented (deferred)

These features from the original design are deferred to future work:

1. **Full 6D Spatial Index**: Simpler approach of 3D clustering + velocity validation used instead
2. **Heading Estimation**: Not needed; velocity vector provides implicit heading
3. **Z-axis Tracking**: Height stored as statistic, not tracked position
4. **Track Quality Scoring**: Basic quality metrics only
5. **Batch Mode Processing**: Real-time mode only

### Performance observations (from PCAP replay testing)

- **MinPts=3** successfully captures sparse distant objects missed by MinPts=12
- **Velocity coherence** significantly reduces false positives from background noise
- **Fragment merging** recovers ~10–15% of tracks split by occlusion gaps
- **Post-tail prediction** extends track duration by 1–3 seconds on average

### Recommended next steps

1. **Add truck/cyclist classes**: Currently only car/pedestrian/bird/other
2. **Tune velocity tolerances**: May need per-class velocity limits
3. **Evaluate sparse track quality**: 3-point tracks may have elevated position noise
4. **Complete Phase 0**: Formal baseline metrics are needed to confirm improvement claims

---

## Related documentation

- [`data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md`](../../data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md): Mathematical model and parameter tradeoffs
- [`docs/lidar/architecture/vector-vs-velocity-workstreams.md`](../lidar/architecture/vector-vs-velocity-workstreams.md): Workstream separation rationale
- [`docs/plans/lidar-static-pose-alignment-plan.md`](./lidar-static-pose-alignment-plan.md): Pose pipeline dependency
- [`docs/plans/lidar-motion-capture-architecture-plan.md`](./lidar-motion-capture-architecture-plan.md): Motion capture architecture
