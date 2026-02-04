# Repo Inspection Notes

This document summarises the existing LiDAR pipeline architecture identified during the design phase for the macOS visualiser project.

## 1. LiDAR Ingestion

### Files and Functions

| File                                 | Key Components                                   |
| ------------------------------------ | ------------------------------------------------ |
| `internal/lidar/network/listener.go` | `UDPListener`, `Start()`, `handlePacket()`       |
| `internal/lidar/parse/extract.go`    | `Pandar40PParser`, `ParsePacket()`               |
| `internal/lidar/frame_builder.go`    | `FrameBuilder`, `AddPointsPolar()`, `GetFrame()` |

### Data Flow

```
UDP packets (port 2368)
    → UDPListener.Start()
    → handlePacket()
    → parser.ParsePacket() → []PointPolar
    → frameBuilder.AddPointsPolar()
    → LiDARFrame (360° rotation)
```

### Key Observations

- Hesai Pandar40P packets are 1262 bytes (see [packet_analysis_results.md](../reference/packet_analysis_results.md) for protocol details)
- Motor speed drives frame duration: 10-20 Hz (600-1200 RPM)
  - Sensor supports two speed modes with variable packet rates during transitions
  - Frame completion time: ~50-100ms depending on motor speed
- Points stored in polar coordinates initially (`PointPolar`)
- Converted to Cartesian via `SphericalToCartesian()` in `transform.go`

---

## 2. Foreground Extraction

### Files and Functions

| File                           | Key Components                                             |
| ------------------------------ | ---------------------------------------------------------- |
| `internal/lidar/background.go` | `BackgroundManager`, `BackgroundGrid`, `BackgroundCell`    |
| `internal/lidar/foreground.go` | `ProcessFramePolarWithMask()`, `ExtractForegroundPoints()` |

### Data Flow

```
LiDARFrame (polar)
    → BackgroundManager.UpdateFromFrame()
    → Per-cell EMA update
    → ProcessFramePolarWithMask()
    → foregroundMask []bool
    → ExtractForegroundPoints()
    → []WorldPoint (world frame)
```

### Key Observations

- Background model: polar grid (40 rings × 1800 azimuth bins)
- EMA-based learning with configurable alpha
- Warmup scaling prevents false positives during settling
- Foreground = points with range deviation > threshold

---

## 3. Clustering

### Files and Functions

| File                           | Key Components                             |
| ------------------------------ | ------------------------------------------ |
| `internal/lidar/clustering.go` | `DBSCAN()`, `WorldCluster`, `SpatialIndex` |

### Data Flow

```
[]WorldPoint (foreground, world frame)
    → DBSCAN(points, eps=0.6, minPts=12)
    → []WorldCluster
    → WorldCluster{CentroidX/Y/Z, BoundingBox*, PointsCount, ...}
```

### Key Observations

- DBSCAN operates in world frame (after pose transform)
- Spatial index accelerates neighbour queries
- Cluster features computed: centroid, AABB, height_p95, intensity_mean

---

## 4. Tracking

### Files and Functions

| File                                  | Key Components                                   |
| ------------------------------------- | ------------------------------------------------ |
| `internal/lidar/tracking.go`          | `Tracker`, `TrackedObject`, `TrackState`         |
| `internal/lidar/tracking_pipeline.go` | `TrackingPipelineConfig`, callback orchestration |

### Data Flow

```
[]WorldCluster
    → Tracker.Update(clusters, timestamp)
    → predict() - Kalman predict step
    → associate() - greedy nearest-neighbour with Mahalanobis gating
    → update() - Kalman update step
    → createNewTracks() - spawn from unassigned clusters
    → cleanup() - delete stale tracks
    → TrackedObject{TrackID, State, X, Y, VX, VY, P, ...}
```

### Key Observations

- Constant velocity (CV) Kalman filter, 4-state: [x, y, vx, vy]
- Lifecycle: Tentative → Confirmed (5 hits) → Deleted (3 misses)
- Mahalanobis distance gating (squared threshold = 25 m²)
- Rule-based classification: pedestrian, car, bird, other

---

## 5. LidarView Forwarding

### Files and Functions

| File                                             | Key Components                               |
| ------------------------------------------------ | -------------------------------------------- |
| `internal/lidar/network/foreground_forwarder.go` | `ForegroundForwarder`, `ForwardForeground()` |
| `internal/lidar/network/forwarder.go`            | `PacketForwarder`, `ForwardAsync()`          |

### Data Flow

```
Foreground []PointPolar
    → ForegroundForwarder.ForwardForeground()
    → Encode to Pandar40P packet format
    → UDP send to 127.0.0.1:2370
    → LidarView receives and renders
```

### Key Observations

- Preserves `RawBlockAzimuth` for packet compatibility
- Encodes polar points back to Pandar40P format
- Outputs to port 2370 (separate from ingestion port 2368)
- **Must remain unchanged** as regression oracle

---

## 6. Coordinate Frames and Transforms

### Files and Functions

| File                          | Key Components                          |
| ----------------------------- | --------------------------------------- |
| `internal/lidar/transform.go` | `SphericalToCartesian()`, `ApplyPose()` |
| `internal/lidar/arena.go`     | `Pose`, `PoseCache`, `FrameID`          |

### Coordinate Convention

- **Sensor frame**: X=right, Y=forward, Z=up
- **World frame**: Site-level ENU (East-North-Up)
- Pose stored as 4×4 row-major matrix

### Key Observations

- `SphericalToCartesian(distance, azimuth, elevation)` → (x, y, z)
- `ApplyPose(x, y, z, T)` → (wx, wy, wz)
- Poses stored in DB with validity windows (`valid_from_ns`, `valid_to_ns`)

---

## 7. Database Schema (Relevant Tables)

| Table               | Purpose                                     |
| ------------------- | ------------------------------------------- |
| `lidar_clusters`    | Persisted cluster detections                |
| `lidar_tracks`      | Track summaries (start/end, classification) |
| `lidar_track_obs`   | Per-frame track observations                |
| `lidar_bg_snapshot` | Background grid snapshots                   |
| `sensor_poses`      | Sensor-to-world transforms                  |
| `lidar_labels`      | User annotations for tracks (to be added)   |

---

## 8. API Endpoints (Existing)

| Endpoint                            | Purpose               |
| ----------------------------------- | --------------------- |
| `GET /api/lidar/tracks`             | List tracks           |
| `GET /api/lidar/tracks/active`      | Active tracks         |
| `GET /api/lidar/clusters`           | List clusters         |
| `GET /api/lidar/observations`       | Track observations    |
| `GET /api/lidar/background/params`  | Background parameters |
| `POST /api/lidar/background/params` | Set parameters        |

## 9. API Endpoints (To Be Added)

| Endpoint                       | Purpose               |
| ------------------------------ | --------------------- |
| `POST /api/lidar/labels`       | Create label          |
| `GET /api/lidar/labels`        | List labels           |
| `GET /api/lidar/labels/:id`    | Get label by ID       |
| `PUT /api/lidar/labels/:id`    | Update label          |
| `DELETE /api/lidar/labels/:id` | Delete label          |
| `GET /api/lidar/labels/export` | Export labels as JSON |

---

## Summary

The existing pipeline is well-structured with clear separation:

1. **Ingestion** → UDP + parsing
2. **Preprocessing** → Background model + foreground extraction
3. **Perception** → DBSCAN clustering + Kalman tracking
4. **Output** → LidarView forwarding + REST API + SQLite persistence

The visualiser integration uses two transport channels:

- **gRPC** (port 50051): Point cloud streaming for real-time rendering
- **REST API** (port 8080): Label CRUD operations, shared with web UI

This separation ensures labeling data is centrally stored in the Go backend SQLite database and accessible from both the macOS visualiser and web interface.
