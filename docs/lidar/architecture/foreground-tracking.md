# LIDAR foreground extraction and tracking implementation plan

- **Status:** Implementation Complete through Phase 3.7
- **Version:** 7.1 - Warmup sensitivity scaling added

Full implementation of LiDAR foreground extraction and multi-object tracking, from polar-frame background subtraction through world-frame clustering, Kalman-filtered tracking, and database persistence.

---

## Overview

Implementation plan for LiDAR-based object detection and tracking with **explicit separation between polar-frame background processing and world-frame clustering/tracking**.

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

**Next Phases:** See [LiDAR Pipeline Reference](lidar-pipeline-reference.md) for Phases 4.0-4.3.

---

## Table of contents

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
    - [D. Classification Research Data Storage](#d-classification-research-data-storage)
    - [E. Future Work: Pose Validation](#e-future-work-pose-validation)

---

## Current state assessment

All phases through 3.7 are complete: background grid (polar), foreground mask generation, polar→world transform, DBSCAN clustering, Kalman tracking, SQL schema and persistence, track classification, REST API endpoints, PCAP analysis tool, analysis run infrastructure, and track visualisation UI. See the Implementation Files table below for the full file listing.

**Critical Constraint:** Background grid operates purely in polar space. All EMA updates, neighbour voting, and classification occur in polar coordinates.

### 📋 remaining components

1. **Track Labelling UI** — manual annotation interface for reproducible classification research and scorecards (Phase 4.0)

---

## Architecture: polar vs world frame

### Coordinate system boundaries

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

### Data flow pipeline

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

### Key design decisions

1. **Background in Polar:** Stable sensor geometry, efficient ring-based neighbour queries
2. **Clustering in World:** Consistent Euclidean distances, stable reference frame
3. **Tracking in World:** Velocity estimation requires fixed coordinate system
4. **No Reverse Transform:** World frame components never convert back to polar

---

## Phase 2.9: foreground mask generation (polar)

### Objective

Generate per-point foreground/background classification mask in polar coordinates without extracting points.

### Changes to processFramePolar

**Current Contract:**

```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) error
```

**New Contract:**

```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) (foregroundMask []bool, err error)
```

**Implementation:**

> **Source:** [`internal/lidar/l3grid/foreground.go`](../../../internal/lidar/l3grid/foreground.go) — `ProcessFramePolar()` classifies each point in polar space using EMA distance thresholds with same-ring neighbour voting and warmup sensitivity scaling.

### Foreground point extraction (outside lock)

**After releasing background lock:**

> **Source:** [`internal/lidar/l3grid/foreground.go`](../../../internal/lidar/l3grid/foreground.go) — `ExtractForegroundPoints()` filters polar points by mask outside the background lock.

### Frame processing callback

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

### Monitoring & metrics

Add per-frame foreground metrics to HTTP status:

> **Source:** [`internal/lidar/l8analytics/`](../../../internal/lidar/l8analytics/) — `FrameMetrics` struct tracks total, foreground, background counts, fraction, and processing time per frame.

---

## Phase 3.0: polar → world transform

### Objective

Explicit coordinate transformation stage converting foreground polar points to world-frame Cartesian coordinates.

### Transform stage design

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

> **Source:** [`internal/lidar/l4perception/cluster.go`](../../../internal/lidar/l4perception/cluster.go) — `WorldPoint` struct and `TransformToWorld()` converting polar to world-frame Cartesian via 4×4 homogeneous pose transform.

### Testing requirements

1. **Unit test:** Verify transform accuracy with known poses
2. **Integration test:** Compare transformed points against ground truth
3. **Validation:** Ensure no polar coordinates leak into world-frame structures

---

## Phase 3.1: DBSCAN clustering (world frame)

### Objective

Spatial clustering of foreground world points to detect distinct objects.

### Algorithm: DBSCAN with required spatial index

**Parameters:**

- `eps = 0.6` metres (neighbourhood radius)
- `minPts = 12` (minimum points per cluster)
- **Dimensionality:** 2D (x, y) clustering, with z used only for cluster features

**Rationale for 2D:**

- Ground-plane objects (vehicles, pedestrians) primarily distinguished by lateral position
- Vertical separation (z) used for height features after clustering
- Simplifies spatial index and reduces computational cost

**Spatial Index:** **Required** (not optional)

- Implementation: Regular grid with cell size ≈ `eps` (0.6m)
- Region queries examine only current cell + 8 neighbours (2D) or 26 neighbours (3D)
- Replaces O(n²) brute-force neighbour search

### Implementation

#### Spatial index (required)

> **Source:** [`internal/lidar/l4perception/cluster.go`](../../../internal/lidar/l4perception/cluster.go) — `SpatialIndex` with grid-based Szudzik pairing, `Build()`, `getCellID()`, and `RegionQuery()` examining current cell + 8 neighbours.

#### DBSCAN algorithm

> **Source:** [`internal/lidar/l4perception/cluster.go`](../../../internal/lidar/l4perception/cluster.go) — `DBSCAN()` with spatial index and `expandCluster()` for density-based neighbour expansion.

#### Cluster metrics computation

> **Source:** [`internal/lidar/l4perception/cluster.go`](../../../internal/lidar/l4perception/cluster.go) — `buildClusters()` and `computeClusterMetrics()` computing centroid, axis-aligned bounding box, height P95, and intensity mean per cluster.

---

## Phase 3.2: Kalman tracking (world frame)

### Objective

Multi-object tracking with explicit lifecycle states and world-frame state estimation.

### Track lifecycle states

```
Tentative → Confirmed → Deleted
```

**State Transitions:**

- **Birth:** New cluster creates Tentative track
- **Tentative → Confirmed:** After N consecutive associations (N=3)
- **Confirmed → Deleted:** After MaxMisses frames without association (MaxMisses=3)
- **Tentative → Deleted:** After MaxMisses frames without association

### Track state (world frame only)

**State Vector:**

```
x = [x, y, vx, vy]^T
```

- `x, y`: Position in world frame (metres)
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

> **Source:** [`internal/lidar/l5tracks/tracking.go`](../../../internal/lidar/l5tracks/tracking.go) — `Track` struct (identity, lifecycle, Kalman state, aggregated features, classification fields) and `TrackState2D` (position + velocity with 4×4 covariance).

### Tracker implementation

> **Source:** [`internal/lidar/l5tracks/tracking.go`](../../../internal/lidar/l5tracks/tracking.go) — `Tracker` struct with configurable gating, process/measurement noise, and `Update()` performing predict→associate→update→lifecycle management per frame.

### Gating distance (mahalanobis)

**Definition:** Gating uses Mahalanobis distance in world coordinates to reject unlikely associations.

**Formula:**

```
d² = (z - Hx)^T * S^-1 * (z - Hx)

where:
  z = measurement (cluster centroid x, y)
  Hx = predicted measurement (track position x, y)
  S = innovation covariance (H*P*H^T + R)
```

**Threshold:** `GatingDistanceSquared = 25.0` (i.e., 5.0 metres)

- We threshold on **squared distance** to avoid square root computation
- Threshold tuned empirically for typical vehicle/pedestrian speeds

> **Source:** [`internal/lidar/l5tracks/tracking.go`](../../../internal/lidar/l5tracks/tracking.go) — `mahalanobisDistanceSquared()` computes gating distance via 2×2 innovation covariance inversion.

---

## Phase 3.3: SQL schema & REST APIs

### SQL schema (world frame only)

**Critical:** All tables store **world-frame coordinates only**. Polar coordinates and background grid data are **never** persisted to SQLite.

> **Note:** Pose ID columns have been removed. Data is stored in sensor frame coordinates (identity transform). Pose-based transformations are planned for a future phase.

#### Migration: 000009_create_lidar_tracks.up.sql

> **Source:** [`internal/db/migrations/`](../../../internal/db/migrations/) — creates `lidar_clusters`, `lidar_tracks`, and `lidar_track_obs` tables with world-frame coordinates, lifecycle state, kinematics, classification fields, and time/sensor/class indices.

### REST API endpoints

#### Existing endpoints (background/Polar)

```
GET  /api/lidar/params              - Background parameters
POST /api/lidar/params              - Update parameters
GET  /api/lidar/acceptance          - Acceptance metrics (polar)
GET  /api/lidar/grid_status         - Grid status (polar)
GET  /api/lidar/grid_heatmap        - Spatial heatmap (polar)
```

#### New endpoints (world frame)

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

#### Example response

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

## Phase 3.4: track classification

### Objective

Classify tracks by object type (pedestrian, car, bird, other) using world-frame features.

### Classification features (world frame)

**Spatial Features:**

- Bounding box dimensions (length, width, height) in metres
- Height p95 (95th percentile Z coordinate)
- Point density (points per cubic metre)

**Kinematic Features:**

- Average speed (`avg_speed_mps`)
- Raw maximum speed (`max_speed_mps` in public contracts; `peak_speed_mps` in SQL)
- Speed variance
- Acceleration magnitude

**Temporal Features:**

- Track duration
- Observation count
- Consistency score (ratio of observations to expected frames)

### Classification logic

> **Source:** [`internal/lidar/l6objects/classification.go`](../../../internal/lidar/l6objects/classification.go) — `TrackClassifier.Classify()` implements rule-based classification using bounding box dimensions, speed, and height to distinguish pedestrian, car, bird, and other object classes.

### Future enhancement: ML-based classification

- Train model on labeled track features
- Export features to CSV for model training
- Deploy model as inference endpoint
- Update `classification_model` field with model version

---

## Performance & concurrency

### Locking boundaries

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

### Latency budget (per stage)

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

### Profiling points

> **Source:** `PipelineMetrics` struct in [`internal/lidar/l8analytics/`](../../../internal/lidar/l8analytics/) — per-frame latency measurements for each pipeline stage (background classify, foreground extract, transform, clustering, tracking, database, total).

---

## Testing strategy

### Test categories

#### 1. Polar frame tests (phase 2.9)

**Test:** Foreground mask accuracy — verify that `ProcessFramePolar` produces correct per-point foreground/background classification. Points closer than background (5 m vs 10 m stable background) should be marked foreground.

> **Source:** [`internal/lidar/l3grid/foreground_test.go`](../../../internal/lidar/l3grid/foreground_test.go)

#### 2. Transform tests (phase 3.0)

**Test:** Polar → World transform accuracy — verify that `TransformToWorld` converts polar coordinates to correct Cartesian world positions under identity and known custom poses. Point at (10 m, 0°, 0°) with identity pose should produce (x=10, y=0, z=0).

> **Source:** [`internal/lidar/l4perception/cluster_test.go`](../../../internal/lidar/l4perception/cluster_test.go)

#### 3. Clustering tests (phase 3.1)

**Test:** DBSCAN cluster detection — verify that two spatially separated point groups (e.g., origin and (10, 0)) produce exactly two clusters with correct centroids. Uses `eps=0.6`, `minPts=12`.

> **Source:** [`internal/lidar/l4perception/cluster_test.go`](../../../internal/lidar/l4perception/cluster_test.go)

#### 4. Tracking tests (phase 3.2)

**Test:** Track lifecycle transitions — verify Tentative → Confirmed (after 3 consecutive hits) and Confirmed → Deleted (after 3 consecutive misses). Uses `NewTracker()` with moving cluster input and empty-frame input for miss generation.

> **Source:** [`internal/lidar/l5tracks/tracking_test.go`](../../../internal/lidar/l5tracks/tracking_test.go)

#### 5. Integration tests (end-to-end)

**Test:** Full pipeline from PCAP to tracks — load a PCAP with known moving objects, run the full pipeline (polar classification → foreground extraction → world transform → DBSCAN clustering → Kalman tracking), verify that tracks are produced and frame count exceeds 100.

> **Source:** [`internal/lidar/l5tracks/tracking_test.go`](../../../internal/lidar/l5tracks/tracking_test.go)

---

## Implementation roadmap

### Phase timeline

| Phase | Description              | Duration | Status      | Deliverables                                                                      |
| ----- | ------------------------ | -------- | ----------- | --------------------------------------------------------------------------------- |
| 2.9   | Foreground Mask (Polar)  | 1-2 days | ✅ Complete | `ProcessFramePolarWithMask`, `ExtractForegroundPoints`, `FrameMetrics`            |
| 3.0   | Transform (Polar→World)  | 1-2 days | ✅ Complete | `TransformToWorld`, `WorldPoint`, unit tests                                      |
| 3.1   | DBSCAN Clustering        | 3-4 days | ✅ Complete | `SpatialIndex`, `DBSCAN`, `computeClusterMetrics`, `WorldCluster`                 |
| 3.2   | Kalman Tracking          | 4-5 days | ✅ Complete | `Tracker`, `TrackedObject`, Mahalanobis gating, lifecycle management              |
| 3.3   | SQL Schema & Persistence | 3-4 days | ✅ Complete | `lidar_clusters`, `lidar_tracks`, `lidar_track_obs` tables, persistence functions |
| 3.4   | Classification           | 2-3 days | ✅ Complete | `TrackClassifier`, rule-based classification, object classes                      |
| 3.5   | REST API Endpoints       | 1-2 days | ✅ Complete | `TrackAPI` HTTP handlers, list/get/update tracks, cluster queries                 |
| 3.6   | PCAP Analysis Tool       | 1-2 days | ✅ Complete | `pcap-analyze` CLI tool for batch processing and classification research export   |
| 3.8   | Track Visualisation UI   | 2-3 days | ✅ Complete | MapPane, TrackList, TimelinePane components, pagination, playback                 |
| Test  | Integration Testing      | 2-3 days | 📋 Planned  | Integration tests, performance validation                                         |

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
9. ✅ **PCAP Analysis Tool** - CLI tool for batch track categorisation and classification research export
10. ✅ **Track Visualisation UI** - SvelteKit components for track history playback
11. 📋 **Production Ready** - All tests passing, documented, deployed

### Implementation files

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
| ML      | `internal/lidar/training_data.go`                          | Classification research export and encoding      |
| ML      | `internal/lidar/training_data_test.go`                     | Unit tests for research export encoding          |

---

## Appendix

### A. data structures

> **Source:** [`internal/lidar/l2frames/`](../../../internal/lidar/l2frames/), [`internal/lidar/l4perception/cluster.go`](../../../internal/lidar/l4perception/cluster.go), [`internal/lidar/l5tracks/types.go`](../../../internal/lidar/l5tracks/types.go)

| Stage      | Type           | Frame | Key Fields                                                                                                 |
| ---------- | -------------- | ----- | ---------------------------------------------------------------------------------------------------------- |
| Input      | `PointPolar`   | Polar | distance, azimuth, elevation, intensity, ring (0–39)                                                       |
| Transform  | `WorldPoint`   | World | x, y, z (metres), intensity, timestamp, sensor_id                                                          |
| Clustering | `WorldCluster` | World | centroid (x, y, z), bounding box (L×W×H), point count, height P95, intensity mean                          |
| Tracking   | `Track`        | World | track ID, lifecycle state, Kalman state (x, y, vx, vy), hits/misses, speed stats, object class, confidence |

### B. configuration parameters

See [Configuration Reference](#configuration-reference) below for runtime parameter defaults. Additional clustering and tracking defaults:

| Domain     | Parameter               | Default              | Description                                |
| ---------- | ----------------------- | -------------------- | ------------------------------------------ |
| Clustering | `Eps`                   | 0.6 m                | Neighbourhood radius                       |
| Clustering | `MinPts`                | 12                   | Minimum points per cluster                 |
| Clustering | `CellSize`              | 0.6 m                | Spatial index cell size                    |
| Tracking   | `MaxTracks`             | 100                  | Maximum concurrent tracks                  |
| Tracking   | `MaxMisses`             | 3                    | Frames without association before deletion |
| Tracking   | `HitsToConfirm`         | 3                    | Consecutive hits to confirm                |
| Tracking   | `GatingDistanceSquared` | 25.0                 | Mahalanobis threshold (5.0 m²)             |
| Tracking   | `ProcessNoise`          | [0.1, 0.1, 0.5, 0.5] | Kalman Q diagonal                          |
| Tracking   | `MeasurementNoise`      | [0.2, 0.2]           | Kalman R diagonal                          |

### C. related documentation

- `ARCHITECTURE.md` - System architecture overview
- `docs/lidar/architecture/lidar-sidecar-overview.md` - LIDAR implementation details
- `docs/DEVLOG.md` - Development history
- `internal/lidar/l3grid/background.go` - Background grid implementation
- `internal/lidar/l5tracks/types.go` - Track data structures

### D. classification research data storage

#### Storage recommendation: sensor frame (polar)

**Classification research data should be stored in sensor frame (polar coordinates)** for the following reasons:

1. **Pose Independence:** Polar data is independent of external calibration. If the pose changes (sensor moved, recalibrated), historical polar data remains valid and can be re-transformed.

2. **Reusability:** Research data collected from one installation can be reused when the pose is updated, without needing to recollect or retransform.

3. **Compact Representation:** Polar coordinates (distance, azimuth, elevation, ring) are a compact, lossless representation of sensor measurements.

4. **Transform on Demand:** World-frame data can always be regenerated from polar data plus the current pose during offline analysis or model training.

#### Research data schema

```sql
-- Foreground point cloud sequences for classification research
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

#### Export functions

> **Source:** [`internal/lidar/l8analytics/`](../../../internal/lidar/l8analytics/) — `ForegroundFrame` struct and `ExportForegroundFrame()` for exporting foreground points in polar coordinates for classification research.

### E. future work: pose validation

> **Note:** Pose validation and quality assessment have been deferred to a future phase. This section describes the planned functionality.

#### Planned features

1. **Pose Validation:** Validate sensor calibration quality based on RMSE metrics
2. **Quality Assessment:** Categorise pose quality (Excellent/Good/Fair/Poor)
3. **Transform Gating:** Gate world-frame transformations by pose quality
4. **Research Data Filtering:** Filter classification research data by pose quality

#### Design rationale

The current implementation stores all data in polar (sensor) frame, which is pose-independent. This design choice:

1. **Preserves Data Validity:** Classification research data remains valid even if the sensor pose changes
2. **Simplifies Schema:** No pose foreign keys required in the database
3. **Enables Future Enhancement:** Pose can be added later without data migration

#### Future implementation plan

When pose validation is implemented:

| RMSE (metres) | Quality   | Usage Recommendation                  |
| ------------- | --------- | ------------------------------------- |
| < 0.05        | Excellent | Use for all downstream processing     |
| 0.05 - 0.15   | Good      | Use for tracking and research export  |
| 0.15 - 0.30   | Fair      | Use for tracking; exclude from export |
| > 0.30        | Poor      | Manual recalibration required         |

**Current Status:** Classification research data is stored in polar (sensor) frame. World-frame transformations use identity transform (sensor frame = world frame).

---

## Current operational status

### Working features

| Feature                     | Status     | Notes                                            |
| --------------------------- | ---------- | ------------------------------------------------ |
| Foreground Feed (Port 2370) | ✅ Working | Foreground points visible in LidarView           |
| Real-time Parameter Tuning  | ✅ Working | Edit params via JSON textarea without restart    |
| Background Subtraction      | ✅ Working | Points correctly masked as foreground/background |
| Warmup Sensitivity Scaling  | ✅ Working | Eliminates initialisation trails                 |
| PCAP Analysis Mode          | ✅ Working | Grid preserved for analysis workflows            |

### Resolved issues

**Packet Corruption on Port 2370** — Forwarder reconstructed packets with
incorrect azimuth values. Fixed by rewriting `ForegroundForwarder` to preserve
`RawBlockAzimuth` and `UDPSequence`.

**Foreground "Trails" After Object Pass** — Points lingered as foreground for
~30 seconds. Two root causes: (1) warmup variance underestimation — fixed with
sensitivity scaling in `ProcessFramePolarWithMask()` (4× → 1× over 100
observations); (2) `recFg` accumulation during freeze — fixed by not incrementing
during freeze and resetting to 0 on thaw. See
[TROUBLESHOOTING.md §Known Fixed Issues](../../../TROUBLESHOOTING.md#lidar-background-grid--warmup-trails-fixed-january-2026).

**Real-time Parameter Tuning** — POST to `/api/lidar/params` with JSON body;
changes apply immediately without restart.

### Known limitations

- **M1 performance** — CPU usage during foreground processing higher than
  expected. Investigate with `go tool pprof` (likely per-frame allocations, lock
  contention, or packet encoding overhead).
- **Runtime tuning schema parity** — `/api/lidar/params` supports core
  background/tracker keys but not full canonical tuning parity for all runtime
  keys. `max_tracks` POST support is wired.

### Configuration reference

| Parameter                        | Default | Description                                |
| -------------------------------- | ------- | ------------------------------------------ |
| `BackgroundUpdateFraction`       | 0.02    | EMA alpha for background learning          |
| `ClosenessSensitivityMultiplier` | 3.0     | Threshold multiplier for classification    |
| `SafetyMarginMeters`             | 0.1     | Fixed margin added to threshold            |
| `NoiseRelativeFraction`          | 0.01    | Distance-proportional noise allowance      |
| `NeighborConfirmationCount`      | 3       | Neighbours needed to confirm background    |
| `FreezeDurationNanos`            | 5e9     | Cell freeze duration after large deviation |
| `SeedFromFirstObservation`       | true    | Initialise cells from first observation    |

Warmup sensitivity: cells with `TimesSeenCount < 100` have their threshold
multiplied by `1.0 + 3.0 × (100 − count) / 100` (4× at count 0, 1× at 100+).

### API endpoints

| Endpoint                 | Method   | Description                       |
| ------------------------ | -------- | --------------------------------- |
| `/api/lidar/status`      | GET      | Current pipeline status           |
| `/api/lidar/params`      | GET/POST | View/update background parameters |
| `/api/lidar/grid_status` | GET      | Background grid statistics        |
| `/api/lidar/grid_reset`  | GET      | Reset background grid             |
| `/api/lidar/pcap/start`  | POST     | Start PCAP replay                 |
| `/api/lidar/pcap/stop`   | POST     | Stop PCAP replay                  |
| `/api/lidar/data_source` | GET      | Current data source (live/pcap)   |

---

## Related documentation

- **[LiDAR Pipeline Reference](lidar-pipeline-reference.md)** — Pipeline data flow, existing components, and deployment architecture
- **[Velocity-Coherent Foreground Extraction](../../plans/lidar-velocity-coherent-foreground-extraction-plan.md)** — Alternative algorithm design for sparse-point tracking with velocity coherence
- **[LIDAR Sidecar Overview](lidar-sidecar-overview.md)** — Technical implementation overview and module structure
- **[Development Log](../../DEVLOG.md)** — Chronological implementation history
