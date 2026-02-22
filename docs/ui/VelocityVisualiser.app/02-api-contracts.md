# API Contracts

Status: Active (contains planned sections)

This is the **most critical document** for the visualiser project. It defines the canonical data model and communication protocol between the Go pipeline (server) and the macOS visualiser (client).

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

```protobuf
message CoordinateFrameInfo {
  string frame_id = 1;           // e.g., "site/hesai-01"
  string reference_frame = 2;    // e.g., "ENU" or "sensor"
  double origin_lat = 3;         // optional, for georeferencing
  double origin_lon = 4;
  double origin_alt = 5;
  float rotation_deg = 6;        // rotation of X-axis from East (ENU)
}
```

### 1.2 Point Clouds

Points are emitted in world frame. Full fidelity or downsampled modes are supported.

```protobuf
message PointCloudFrame {
  uint64 frame_id = 1;
  int64 timestamp_ns = 2;
  string sensor_id = 3;

  // Compact encoding: arrays of equal length
  repeated float x = 4 [packed = true];      // world frame X (metres)
  repeated float y = 5 [packed = true];      // world frame Y (metres)
  repeated float z = 6 [packed = true];      // world frame Z (metres)
  repeated uint32 intensity = 7 [packed = true];  // 0-255

  // Optional: per-point classification (background=0, foreground=1)
  repeated uint32 classification = 8 [packed = true];

  // Decimation info
  DecimationMode decimation_mode = 9;
  float decimation_ratio = 10;   // e.g., 0.5 = half the points
}

enum DecimationMode {
  DECIMATION_NONE = 0;
  DECIMATION_UNIFORM = 1;
  DECIMATION_VOXEL = 2;
  DECIMATION_FOREGROUND_ONLY = 3;  // only foreground points, no background
}
```

### 1.3 Clusters (Foreground Objects)

Clusters are detected objects before tracking association.

```protobuf
message Cluster {
  int64 cluster_id = 1;          // unique within frame
  string sensor_id = 2;
  int64 timestamp_ns = 3;

  // Centroid in world frame
  float centroid_x = 4;
  float centroid_y = 5;
  float centroid_z = 6;

  // Axis-aligned bounding box
  float aabb_length = 7;         // X extent (metres)
  float aabb_width = 8;          // Y extent (metres)
  float aabb_height = 9;         // Z extent (metres)

  // Oriented bounding box (if computed)
  OrientedBoundingBox obb = 10;

  // Features
  int32 points_count = 11;
  float height_p95 = 12;
  float intensity_mean = 13;

  // Optional: sample points for debug rendering
  repeated float sample_points = 14 [packed = true];  // xyz interleaved
}

// OrientedBoundingBox conforms to the industry-standard 7-DOF format.
// See: docs/plans/lidar-av-lidar-integration-plan.md for full specification.
// This matches BoundingBox7DOF from the AV integration spec:
//   - center_x/y/z: Centre position in metres (world frame)
//   - length/width/height: Box extents in metres
//   - heading_rad: Yaw angle around Z-axis in radians [-π, π]
message OrientedBoundingBox {
  float center_x = 1;
  float center_y = 2;
  float center_z = 3;
  float length = 4;              // along heading direction (metres)
  float width = 5;               // perpendicular to heading (metres)
  float height = 6;              // Z extent (metres)
  float heading_rad = 7;         // rotation around Z-axis (radians, [-π, π])
}

message ClusterSet {
  uint64 frame_id = 1;
  int64 timestamp_ns = 2;
  repeated Cluster clusters = 3;
}
```

### 1.4 Tracks (State, Velocity, Lifecycle)

Tracks are persistent object identities across frames.

```protobuf
message Track {
  string track_id = 1;           // e.g., "track_42"
  string sensor_id = 2;

  // Lifecycle
  TrackState state = 3;
  int32 hits = 4;                // consecutive successful associations
  int32 misses = 5;              // consecutive missed associations
  int32 observation_count = 6;   // total observations

  // Timestamps
  int64 first_seen_ns = 7;
  int64 last_seen_ns = 8;

  // Current position (world frame)
  float x = 9;
  float y = 10;
  float z = 11;

  // Current velocity (world frame)
  float vx = 12;
  float vy = 13;
  float vz = 14;                 // typically 0 for ground-plane tracking

  // Derived kinematics
  float speed_mps = 15;
  float heading_rad = 16;

  // Uncertainty (optional)
  repeated float covariance_4x4 = 17 [packed = true];  // row-major

  // Bounding box (running average)
  float bbox_length_avg = 18;
  float bbox_width_avg = 19;
  float bbox_height_avg = 20;

  // Features
  float height_p95_max = 21;
  float intensity_mean_avg = 22;
  float avg_speed_mps = 23;
  float peak_speed_mps = 24;

  // Classification
  string class_label = 25;       // "pedestrian", "car", "cyclist", "bird", "other"
  float class_confidence = 26;   // 0.0 - 1.0

  // Quality metrics
  float track_length_metres = 28;
  float track_duration_secs = 28;
  int32 occlusion_count = 29;
}

enum TrackState {
  TRACK_STATE_UNKNOWN = 0;
  TRACK_STATE_TENTATIVE = 1;     // new track, needs confirmation
  TRACK_STATE_CONFIRMED = 2;     // stable track
  TRACK_STATE_DELETED = 3;       // track marked for removal
}

message TrackTrail {
  string track_id = 1;
  repeated TrackPoint points = 2;
}

message TrackPoint {
  float x = 1;
  float y = 2;
  int64 timestamp_ns = 3;
}

message TrackSet {
  uint64 frame_id = 1;
  int64 timestamp_ns = 2;
  repeated Track tracks = 3;
  repeated TrackTrail trails = 4;  // historical positions for rendering
}
```

### 1.5 Debug Overlays

Optional debug artifacts for algorithm tuning.

```protobuf
message DebugOverlaySet {
  uint64 frame_id = 1;
  int64 timestamp_ns = 2;

  // Association candidates
  repeated AssociationCandidate association_candidates = 3;

  // Gating ellipses (Mahalanobis distance thresholds)
  repeated GatingEllipse gating_ellipses = 4;

  // Innovation residuals (Kalman filter)
  repeated InnovationResidual residuals = 5;

  // Filtered state predictions
  repeated StatePrediction predictions = 6;
}

message AssociationCandidate {
  int64 cluster_id = 1;
  string track_id = 2;
  float distance = 3;            // Mahalanobis distance
  bool accepted = 4;             // whether association was accepted
}

message GatingEllipse {
  string track_id = 1;
  float center_x = 2;
  float center_y = 3;
  float semi_major = 4;          // metres
  float semi_minor = 5;          // metres
  float rotation_rad = 6;        // ellipse rotation
}

message InnovationResidual {
  string track_id = 1;
  float predicted_x = 2;
  float predicted_y = 3;
  float measured_x = 4;
  float measured_y = 5;
  float residual_magnitude = 6;
}

message StatePrediction {
  string track_id = 1;
  float x = 2;
  float y = 3;
  float vx = 4;
  float vy = 5;
}
```

### 1.5.1 Planned: Background Debug Surfaces for Swift Frontend

Status: **Planned (docs-only), not implemented in current protobuf/API yet**.

Goal: let the Swift frontend inspect background model behaviour directly in both
native polar representation and Cartesian rendering form, including region map
assignment state.

Proposed debug payloads:

```protobuf
message BackgroundPointPolarDebug {
  uint32 ring = 1;
  uint32 azimuth_bin = 2;
  float range_m = 3;
  float spread_m = 4;
  uint32 confidence = 5;   // TimesSeenCount-aligned
  uint32 region_id = 6;    // optional, 0 = unassigned
  string settle_state = 7; // learning/obs_stable/geom_stable/locked/frozen
}

message BackgroundPointCartesianDebug {
  float x = 1;
  float y = 2;
  float z = 3;
  uint32 confidence = 4;
  uint32 source_ring = 5;
  uint32 source_azimuth_bin = 6;
  uint32 region_id = 7;    // optional, 0 = unassigned
  string settle_state = 8;
}

message RegionMapCellDebug {
  uint32 ring = 1;
  uint32 azimuth_bin = 2;
  uint32 region_id = 3;
  string surface_class = 4; // ground/structure/volume/unknown
  string settle_state = 5;
}

message BackgroundDebugBundle {
  uint64 frame_id = 1;
  int64 timestamp_ns = 2;
  repeated BackgroundPointPolarDebug polar_points = 3;
  repeated BackgroundPointCartesianDebug cartesian_points = 4;
  repeated RegionMapCellDebug region_cells = 5;
}
```

Proposed stream request toggles:

```protobuf
message StreamRequest {
  // Existing fields...
  bool include_bg_debug_polar = 20;
  bool include_bg_debug_cartesian = 21;
  bool include_bg_region_map = 22;
}
```

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

**Database Schema (SQLite):**

```sql
CREATE TABLE lidar_labels (
    label_id TEXT PRIMARY KEY,
    track_id TEXT NOT NULL,
    class_label TEXT NOT NULL,
    start_timestamp_ns INTEGER NOT NULL,
    end_timestamp_ns INTEGER,
    confidence REAL,
    created_by TEXT,
    created_at_ns INTEGER NOT NULL,
    updated_at_ns INTEGER,
    notes TEXT,
    FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id)
);

CREATE INDEX idx_lidar_labels_track ON lidar_labels(track_id);
CREATE INDEX idx_lidar_labels_time ON lidar_labels(start_timestamp_ns, end_timestamp_ns);
```

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

```protobuf
service VisualiserService {
  // Live streaming of frame bundles (server-streaming)
  rpc StreamFrames(StreamRequest) returns (stream FrameBundle);

  // Control RPCs for playback (replay mode)
  rpc Pause(PauseRequest) returns (PlaybackStatus);
  rpc Play(PlayRequest) returns (PlaybackStatus);
  rpc Seek(SeekRequest) returns (PlaybackStatus);
  rpc SetRate(SetRateRequest) returns (PlaybackStatus);

  // Request specific overlay modes
  rpc SetOverlayModes(OverlayModeRequest) returns (OverlayModeResponse);

  // Server capabilities query
  rpc GetCapabilities(CapabilitiesRequest) returns (CapabilitiesResponse);

  // Recording control (live mode)
  rpc StartRecording(RecordingRequest) returns (RecordingStatus);
  rpc StopRecording(RecordingRequest) returns (RecordingStatus);
}
```

### 3.2 Message Definitions

```protobuf
message StreamRequest {
  string sensor_id = 1;          // which sensor to stream (or "all")
  bool include_points = 2;       // include full point cloud
  bool include_clusters = 3;     // include cluster set
  bool include_tracks = 4;       // include track set
  bool include_debug = 5;        // include debug overlays
  DecimationMode point_decimation = 6;
  float decimation_ratio = 7;    // 0.0-1.0
}

message FrameBundle {
  uint64 frame_id = 1;
  int64 timestamp_ns = 2;
  string sensor_id = 3;
  CoordinateFrameInfo coordinate_frame = 4;

  PointCloudFrame point_cloud = 5;
  ClusterSet clusters = 6;
  TrackSet tracks = 7;
  DebugOverlaySet debug = 8;

  // Playback metadata (replay mode only)
  PlaybackInfo playback_info = 9;
}

message PlaybackInfo {
  bool is_live = 1;              // true if live, false if replay
  int64 log_start_ns = 2;        // first frame timestamp in log
  int64 log_end_ns = 3;          // last frame timestamp in log
  float playback_rate = 4;       // 1.0 = real-time
  bool paused = 5;
}

message PlaybackStatus {
  bool paused = 1;
  float rate = 2;
  int64 current_timestamp_ns = 3;
  uint64 current_frame_id = 4;
}

message PauseRequest {}
message PlayRequest {}

message SeekRequest {
  oneof target {
    int64 timestamp_ns = 1;      // seek to timestamp
    uint64 frame_id = 2;         // seek to frame
  }
}

message SetRateRequest {
  float rate = 1;                // e.g., 0.5, 1.0, 2.0
}

message OverlayModeRequest {
  bool show_points = 1;
  bool show_clusters = 2;
  bool show_tracks = 3;
  bool show_trails = 4;
  bool show_velocity = 5;
  bool show_gating = 6;
  bool show_association = 7;
  bool show_residuals = 8;
}

message OverlayModeResponse {
  bool success = 1;
}

message CapabilitiesRequest {}

message CapabilitiesResponse {
  bool supports_points = 1;
  bool supports_clusters = 2;
  bool supports_tracks = 3;
  bool supports_debug = 4;
  bool supports_replay = 5;
  bool supports_recording = 6;
  repeated string available_sensors = 7;
}

message RecordingRequest {
  string output_path = 1;        // optional, server may generate
}

message RecordingStatus {
  bool recording = 1;
  string output_path = 2;
  uint64 frames_recorded = 3;
}
```

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

**Header (JSON)**:

```json
{
  "version": "1.0",
  "created_ns": 1706000000000000000,
  "sensor_id": "hesai-01",
  "total_frames": 12345,
  "start_ns": 1706000000000000000,
  "end_ns": 1706001234000000000,
  "coordinate_frame": {
    "frame_id": "site/hesai-01",
    "reference_frame": "ENU"
  }
}
```

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

```protobuf
OverlayModeRequest {
  show_points = false;
  show_clusters = true;
  show_tracks = true;
  show_trails = true;
  show_gating = false;    // expensive debug overlay
}
```

---

## 7. Related Documents

- [01-problem-and-user-workflows.md](./01-problem-and-user-workflows.md) – Problem statement and user workflows
- [03-architecture.md](./03-architecture.md) – System architecture
- [04-implementation-plan.md](./04-implementation-plan.md) – Implementation milestones
- [../../lidar/troubleshooting/01-tracking-upgrades.md](../../lidar/troubleshooting/01-tracking-upgrades.md) – Tracking improvements
