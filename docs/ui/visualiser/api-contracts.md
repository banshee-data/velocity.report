# API Contracts

Canonical data model and communication protocol between the Go pipeline (server) and the macOS visualiser (client).

---

## Industry Standards Alignment

This API is designed to align with the **7-DOF industry standard** for 3D bounding boxes used in autonomous vehicle (AV) perception systems:

| Standard Element          | Implementation                                      | Reference                                                                        |
| ------------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------- |
| **7-DOF Bounding Box**    | `OrientedBoundingBox` message                       | [av-lidar-integration-plan.md](../../plans/lidar-av-lidar-integration-plan.md)   |
| **Coordinate Convention** | ENU: +X East, +Y North, +Z Up (world frame)         | [static-pose-alignment-plan.md](../../plans/lidar-static-pose-alignment-plan.md) |
| **Heading Convention**    | Radians, [-π, π], rotation around Z-axis            | AV industry standard                                                             |
| **Units**                 | Metres for positions/dimensions, radians for angles | SI units                                                                         |

**Key alignment points:**

- `OrientedBoundingBox` matches `BoundingBox7DOF` from `av-lidar-integration-plan.md`
- Future AV dataset import can use the same data structures
- Compatible with the 28-class AV taxonomy (via extensible `class_label` field)

---

## 1. Canonical Conceptual Model

The visualiser consumes a **frame-oriented data stream** from the pipeline. Each frame represents one complete LiDAR rotation (~50-100ms at 10-20 Hz, depending on motor speed) and contains:

```
┌─────────────────────────────────────────────────────────────────┐
│                         FrameBundle                             │
├─────────────────────────────────────────────────────────────────┤
│  Metadata:                                                      │
│    - frame_id (uint64, monotonic)                               │
│    - timestamp_ns (int64, capture time)                         │
│    - sensor_id (string)                                         │
│    - coordinate_frame (CoordinateFrameInfo)                     │
├─────────────────────────────────────────────────────────────────┤
│  Point Cloud:                                                   │
│    - PointCloudFrame (optional, may be downsampled)             │
├─────────────────────────────────────────────────────────────────┤
│  Perception:                                                    │
│    - ClusterSet (foreground objects)                            │
│    - TrackSet (tracked objects with state)                      │
├─────────────────────────────────────────────────────────────────┤
│  Debug:                                                         │
│    - DebugOverlaySet (optional, toggleable)                     │
└─────────────────────────────────────────────────────────────────┘
```

### 1.1 Frame Timebase and Coordinate Frames

**Timebase**:

- All timestamps are **Unix nanoseconds** (int64)
- Frame timestamp is the **capture time of the first point** in the rotation
- Monotonically increasing frame IDs (uint64)

**Coordinate Frames**:

- **Sensor frame**: Origin at sensor, X=right, Y=forward, Z=up (matches `transform.go`)
- **World frame**: Site-level coordinates, typically `site/<sensor_id>`
- The pipeline emits data in **world frame** after applying sensor pose

`CoordinateFrameInfo` is a 6-field message: `frame_id`, `reference_frame`, optional `origin_lat`/`origin_lon`/`origin_alt`, and `rotation_deg` (X-axis rotation from East in ENU). See [`visualiser.proto`](../../../proto/velocity_visualiser/v1/visualiser.proto).

### 1.2 Point Clouds

Points are emitted in world frame. Full fidelity or downsampled modes are supported.

`PointCloudFrame` carries one frame of point cloud data. Key fields:

| Field              | Type             | Description                                           |
| ------------------ | ---------------- | ----------------------------------------------------- |
| `x`, `y`, `z`      | packed float[]   | World-frame coordinates (metres)                      |
| `intensity`        | packed uint32[]  | Per-point intensity (0-255)                           |
| `classification`   | packed uint32[]  | Optional per-point label (0=background, 1=foreground) |
| `decimation_mode`  | `DecimationMode` | NONE, UNIFORM, VOXEL, or FOREGROUND_ONLY              |
| `decimation_ratio` | float            | Fraction of points retained (e.g. 0.5)                |

See [`visualiser.proto`](../../../proto/velocity_visualiser/v1/visualiser.proto).

### 1.3 Clusters (Foreground Objects)

Clusters are detected objects before tracking association.

`Cluster` represents a detected foreground object before tracking. `ClusterSet` wraps a frame's worth of clusters. Key fields:

| Field                      | Type                  | Description                                                           |
| -------------------------- | --------------------- | --------------------------------------------------------------------- |
| `centroid_x/y/z`           | float                 | Centroid position in world frame (metres)                             |
| `aabb_length/width/height` | float                 | Axis-aligned bounding box extents (metres)                            |
| `obb`                      | `OrientedBoundingBox` | Optional 7-DOF OBB (center xyz + length/width/height + `heading_rad`) |
| `points_count`             | int32                 | Number of points in cluster                                           |
| `sample_points`            | packed float[]        | Optional xyz-interleaved sample points for debug rendering            |

`OrientedBoundingBox` conforms to the industry-standard 7-DOF format matching `BoundingBox7DOF` from the AV integration spec: centre position (xyz), box extents (length/width/height), and heading (radians, [-pi, pi] around Z-axis).

See [`visualiser.proto`](../../../proto/velocity_visualiser/v1/visualiser.proto).

### 1.4 Tracks (State, Velocity, Lifecycle)

Tracks are persistent object identities across frames.

`Track` is a persistent object identity across frames (35 fields). `TrackSet` wraps a frame's tracks plus `TrackTrail` historical positions for rendering. Key field groups:

| Group              | Fields                                                                        | Description                                  |
| ------------------ | ----------------------------------------------------------------------------- | -------------------------------------------- |
| Lifecycle          | `state` (TENTATIVE/CONFIRMED/DELETED), `hits`, `misses`, `observation_count`  | Association and confirmation state           |
| Position/velocity  | `x/y/z`, `vx/vy/vz`                                                           | Current state in world frame (metres, m/s)   |
| Derived kinematics | `speed_mps`, `heading_rad`                                                    | Scalar speed and heading                     |
| Uncertainty        | `covariance_4x4`                                                              | Optional 4x4 packed float, row-major         |
| Bounding box       | `bbox_length/width/height`                                                    | Per-frame cluster dimensions from DBSCAN OBB |
| Features           | `height_p95_max`, `intensity_mean_avg`, `avg_speed_mps`, `max_speed_mps`      | Accumulated track features                   |
| Classification     | `object_class`, `class_confidence`                                            | Classifier output or user label              |
| Quality            | `track_length_metres`, `track_duration_secs`, `occlusion_count`, `confidence` | Track quality metrics                        |

Track speed contract is non-percentile. See [`visualiser.proto`](../../../proto/velocity_visualiser/v1/visualiser.proto).

### 1.5 Debug Overlays

Optional debug artifacts for algorithm tuning.

`DebugOverlaySet` carries four types of optional debug artifact per frame: **AssociationCandidate** (cluster-to-track pairing with Mahalanobis distance and accept/reject), **GatingEllipse** (Mahalanobis gate ellipse geometry per track), **InnovationResidual** (Kalman filter predicted-vs-measured position and residual magnitude), and **StatePrediction** (predicted position and velocity for each track). See [`visualiser.proto`](../../../proto/velocity_visualiser/v1/visualiser.proto).

### 1.5.1 Planned: Background Debug Surfaces for Swift Frontend

**Status:** Planned (docs-only), not implemented in current protobuf/API yet.

Goal: let the Swift frontend inspect background model behaviour directly in both
native polar representation and Cartesian rendering form, including region map
assignment state.

Proposed debug payloads:

Four proposed messages (not yet in `.proto`): `BackgroundPointPolarDebug` (ring/azimuth/range/spread/confidence/region/settle-state), `BackgroundPointCartesianDebug` (xyz + confidence + source ring/azimuth + region/settle-state), `RegionMapCellDebug` (ring/azimuth/region/surface-class/settle-state), and `BackgroundDebugBundle` wrapping all three per frame. Three new `StreamRequest` toggle fields (`include_bg_debug_polar`, `include_bg_debug_cartesian`, `include_bg_region_map`) would control inclusion.

Planned frontend modes:

- `Polar`: ring/azimuth grid inspection for settling diagnostics
- `Cartesian`: world/sensor-frame point overlay for geometric inspection
- `Region Map`: colourised region IDs and lifecycle state overlay

### 1.6 Labels (User Annotations)

Labels are created by the user in the visualiser and stored in the Go backend SQLite database via REST API.

**REST API Endpoints:**

```
POST   /api/lidar/labels              Create new label
GET    /api/lidar/labels              List all labels (with filters)
GET    /api/lidar/labels/:id          Get specific label
PUT    /api/lidar/labels/:id          Update label
DELETE /api/lidar/labels/:id          Delete label
GET    /api/lidar/labels/export       Export labels as JSON
```

**Label JSON Schema:**

```json
{
  "label_id": "uuid-string",
  "track_id": "track_42",
  "class_label": "pedestrian",
  "start_timestamp_ns": 1234567890000000,
  "end_timestamp_ns": 1234567891000000,
  "confidence": 0.95,
  "created_by": "username",
  "created_at_ns": 1234567890000000,
  "notes": "optional notes"
}
```

**Database Schema (SQLite):** Table `lidar_labels` with columns: `label_id` (TEXT PK), `track_id` (TEXT FK to `lidar_tracks`), `class_label` (TEXT), `start_timestamp_ns`/`end_timestamp_ns` (INTEGER), `confidence` (REAL), `created_by` (TEXT), `created_at_ns`/`updated_at_ns` (INTEGER), `notes` (TEXT). Indexed on `track_id` and timestamp range.

---

## 2. Protobuf Schema

### 2.1 File Location

```
proto/
└── velocity_visualiser/
    └── v1/
        └── visualiser.proto
```

### 2.2 Full Schema

See [visualiser.proto](../../../proto/velocity_visualiser/v1/visualiser.proto) for the complete protobuf definition.

### 2.3 Versioning Policy

- Schema version: `v1`
- Package: `velocity.visualiser.v1`
- **Backward compatibility**: New fields are optional; old clients ignore unknown fields
- **Forward compatibility**: Old servers respond with subset of fields
- **Breaking changes**: Bump to `v2` with new package name

### 2.4 Field Semantics

| Field        | Type  | Units       | Convention               |
| ------------ | ----- | ----------- | ------------------------ |
| `*_ns`       | int64 | nanoseconds | Unix epoch               |
| `*_mps`      | float | m/s         | speed magnitude          |
| `*_rad`      | float | radians     | angle, CCW from +X       |
| `x, y, z`    | float | metres      | world frame              |
| `vx, vy, vz` | float | m/s         | world frame              |
| `*_length`   | float | metres      | along heading            |
| `*_width`    | float | metres      | perpendicular to heading |
| `*_height`   | float | metres      | Z extent                 |

---

## 3. gRPC Service Design

### 3.1 Service Definition

`VisualiserService` defines 9 RPCs: `StreamFrames` (server-streaming frame bundles), `Pause`/`Play`/`Seek`/`SetRate` (playback control, all return `PlaybackStatus`), `SetOverlayModes` (toggle debug overlays), `GetCapabilities` (query server features), and `StartRecording`/`StopRecording` (live capture control). See [`visualiser.proto`](../../../proto/velocity_visualiser/v1/visualiser.proto).

### 3.2 Message Definitions

| Message                | Purpose                            | Key fields                                                                                                      |
| ---------------------- | ---------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `StreamRequest`        | Client subscription config         | `sensor_id`, `include_points/clusters/tracks/debug`, `point_decimation`, `decimation_ratio`                     |
| `FrameBundle`          | Top-level per-frame envelope       | `frame_id`, `timestamp_ns`, nested `PointCloudFrame`/`ClusterSet`/`TrackSet`/`DebugOverlaySet`, `playback_info` |
| `PlaybackInfo`         | Replay metadata within FrameBundle | `is_live`, `log_start_ns`/`log_end_ns`, `playback_rate`, `paused`                                               |
| `PlaybackStatus`       | Response to playback RPCs          | `paused`, `rate`, `current_timestamp_ns`, `current_frame_id`                                                    |
| `SeekRequest`          | Seek target (oneof)                | `timestamp_ns` or `frame_id`                                                                                    |
| `SetRateRequest`       | Playback speed                     | `rate` (e.g. 0.5, 1.0, 2.0)                                                                                     |
| `OverlayModeRequest`   | Toggle 8 overlay layers            | `show_points/clusters/tracks/trails/velocity/gating/association/residuals`                                      |
| `CapabilitiesResponse` | Server feature flags               | `supports_points/clusters/tracks/debug/replay/recording`, `available_sensors`                                   |
| `RecordingStatus`      | Recording state                    | `recording`, `output_path`, `frames_recorded`                                                                   |

`PauseRequest`, `PlayRequest`, `CapabilitiesRequest`, `RecordingRequest`, and `OverlayModeResponse` are empty or single-field messages. See [`visualiser.proto`](../../../proto/velocity_visualiser/v1/visualiser.proto).

---

## 4. Recording/Replay Format

### 4.1 Log Format

Logs are stored as **chunked protobuf streams** with an index for efficient seeking.

```
<log_file>.vrlog
├── header.json                 # metadata
├── index.bin                   # frame index for seeking
└── frames/
    ├── chunk_0000.pb          # frames 0-999
    ├── chunk_0001.pb          # frames 1000-1999
    └── ...
```

**Header (JSON):** Contains `version`, `created_ns`, `sensor_id`, `total_frames`, `start_ns`/`end_ns` time range, and nested `coordinate_frame` (frame_id + reference_frame).

**Index (binary)**:

```
[frame_id: uint64][timestamp_ns: int64][chunk_id: uint32][offset: uint32]
... repeated for each frame
```

**Chunks (protobuf)**:

- Each chunk contains up to 1000 `FrameBundle` messages
- Length-prefixed format: `[4-byte length][FrameBundle proto bytes]`

### 4.2 Determinism Rules

For reproducible replay:

1. **No runtime randomness** in tracking pipeline
2. **Seeded RNG** if any randomness is required (e.g., for sampling)
3. **Timestamp-based ordering**: Frames processed in capture order
4. **Stable IDs**: Track IDs generated deterministically from initial cluster + timestamp

---

## 5. LidarView Adapter

The existing LidarView forwarding path is **preserved unchanged**.

### 5.1 Adapter Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Tracking Pipeline                           │
│  (ingest → foreground → cluster → track → canonical model)      │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
                    ┌───────────────┐
                    │ Canonical     │
                    │ Internal      │
                    │ Model         │
                    └───────┬───────┘
                            │
            ┌───────────────┼───────────────┐
            ▼               ▼               ▼
    ┌───────────────┐ ┌───────────────┐ ┌───────────────┐
    │ LidarView     │ │ gRPC          │ │ Recorder      │
    │ Adapter       │ │ Publisher     │ │               │
    │ (port 2370)   │ │ (port 50051)  │ │ (.vrlog)      │
    └───────────────┘ └───────────────┘ └───────────────┘
```

### 5.2 Adapter Implementation

The LidarView adapter (`internal/lidar/network/foreground_forwarder.go`) continues to:

1. Receive foreground `PointPolar` from pipeline
2. Encode as Pandar40P packets
3. Forward to port 2370

**No changes required** to the adapter. It consumes polar points directly from `TrackingPipelineConfig.FgForwarder`.

### 5.3 Comparison Workflow

For regression testing:

1. Run replay from `.vrlog` file
2. Pipeline emits to both LidarView adapter and gRPC publisher
3. LidarView shows packet-level view
4. macOS visualiser shows semantic view (tracks, clusters)
5. Compare: track counts, speed distributions, cluster counts

---

## 6. Bandwidth/Performance Modes

### 6.1 Full Mode

All data at full fidelity:

- Points: 70,000 per frame × 16 bytes = ~1.1 MB/frame
- Clusters: ~50 × 100 bytes = ~5 KB/frame
- Tracks: ~20 × 200 bytes = ~4 KB/frame
- **Total**: ~1.1 MB/frame × 10-20 Hz = ~11-22 MB/s

### 6.2 Foreground-Only Mode

Only foreground points (typically 5-10% of total):

- Points: 7,000 per frame × 16 bytes = ~112 KB/frame
- **Total**: ~120 KB/frame × 10-20 Hz = ~1.2-2.4 MB/s

### 6.3 Tracks-Only Mode

No point cloud, clusters/tracks only:

- **Total**: ~10 KB/frame × 10-20 Hz = ~100-200 KB/s

### 6.4 Overlay Toggles

Client can request specific overlays to reduce payload:

Example: set `show_clusters`, `show_tracks`, `show_trails` to true; disable `show_points` and `show_gating` (expensive debug overlay) — see `OverlayModeRequest` in section 3.2.

---

## 7. Related Documents

- [velocity-visualiser-architecture.md](./architecture.md) – System architecture (includes problem statement)
- [velocity-visualiser-implementation.md](./implementation.md) – Implementation milestones
- [01-tracking-upgrades.md](../../lidar/troubleshooting/01-tracking-upgrades.md) – Tracking improvements
