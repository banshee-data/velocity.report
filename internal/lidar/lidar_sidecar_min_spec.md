# LiDAR Sidecar — Minimal Implementation Spec (no Prometheus, no fusion, no gRPC)

**Audience:** another engineer (Claude Sonnet) implementing the first working sidecar.  
**Scope:** Ingest Hesai UDP → parse to points → range‑image background subtraction (sensor frame) → cluster foreground → transform to world frame → track → expose simple HTTP JSON endpoints for health and recent tracks.  
**Out of scope (for now):** Prometheus metrics, radar fusion/association, gRPC/WS streaming, DB persistence (the main app may persist via existing APIs later).

---

## 1) High‑level data flow

```
[UDP Listener] -> [Parser] -> [Frame Assembler] -> [Range-Image BG (sensor frame)]
                                                       |
                                              [Foreground mask]
                                                       |
                                                 [Clustering]
                                                       |
                                              [World Transform]
                                                       |
                                                   [Tracking]
                                                       |
                                             [HTTP JSON Endpoints]
```

- **Sensor frame** = coordinates fixed to the LiDAR device. Used only for the **background subtractor**.
- **World (site) frame** = stable site/map axes. Used for **clusters, tracking, visualization**.

---

## 2) Modules / files (suggested)

```
cmd/lidar/main.go                  # wire flags, goroutines, HTTP
internal/lidar/listener/           # UDP socket and packet channel
internal/lidar/parser/             # Pandar40P packet -> []Point (sensor frame)
internal/lidar/stagecraft/         # BG subtractor, clustering, world transform, tracking
internal/lidar/pose/               # Pose cache + SE(3) helpers
internal/lidar/debug/              # HTTP handlers: /health /fg /tracks/recent /track/:id
internal/lidar/cfg/                # config structs + flag parsing
pcap/                               # offline replay utilities (optional)
```

---

## 3) Core in‑memory data structures (Go)

> **Background** lives in the **sensor frame**. Everything else is **world frame**.

```go
// Frames & poses --------------------------------------------------------------
type FrameID string

type Pose struct {
    PoseID         int64
    SensorID       string
    FromFrame      FrameID // "sensor/hesai-01"
    ToFrame        FrameID // "site/main-st-001"
    T              [16]float64 // 4x4 row-major
    ValidFromNanos int64
    ValidToNanos   *int64
    Method         string  // "tape+square","plane-fit",...
    RMSEm          float32
}

type PoseCache struct {
    BySensorID map[string]*Pose
    WorldFrame FrameID
}

// Background mask (sensor frame) ---------------------------------------------
type BackgroundParams struct {
    BackgroundUpdateFraction       float32 // e.g., 0.02
    ClosenessSensitivityMultiplier float32 // e.g., 3.0
    SafetyMarginMeters             float32 // e.g., 0.5
    FreezeDurationNanos            int64   // e.g., 5e9
    NeighborConfirmationCount      int     // e.g., 5
}

type BackgroundCell struct {
    AverageRangeMeters   float32
    RangeSpreadMeters    float32
    TimesSeenCount       uint32
    LastUpdateUnixNanos  int64
    FrozenUntilUnixNanos int64
}

type BackgroundGrid struct {
    SensorID    string
    SensorFrame FrameID
    Rings       int
    AzimuthBins int
    Cells       []BackgroundCell // len=Rings*AzimuthBins
    Params      BackgroundParams
    // Telemetry
    ForegroundCount int64
    BackgroundCount int64
}

func (g *BackgroundGrid) Idx(ring, azBin int) int { return ring*g.AzimuthBins + azBin }

// World-space entities --------------------------------------------------------
type WorldCluster struct {
    ClusterID         int64
    SensorID          string
    WorldFrame        FrameID
    PoseID            int64
    TSUnixNanos       int64
    CentroidX, CentroidY, CentroidZ float32
    BBoxL, BBoxW, BBoxH float32
    PointsCount       int
    HeightP95         float32
    IntensityMean     float32
    // Optional debug hints (from sensor frame):
    SensorRingHint    int
    SensorAzDegHint   float32
}

type TrackState2D struct {
    X, Y   float32
    VX, VY float32
    Cov    [16]float32 // row-major 4x4
}

type Track struct {
    TrackID        string
    SensorID       string
    WorldFrame     FrameID
    PoseID         int64
    FirstUnixNanos int64
    LastUnixNanos  int64
    State          TrackState2D
    BBoxLAvg, BBoxWAvg, BBoxHAvg float32
    ObsCount       int
    AvgSpeedMps    float32
    PeakSpeedMps   float32
    HeightP95Max   float32
    IntensityMeanAvg float32
    ClassLabel     string
    ClassConfidence float32
    Misses         int
}

type TrackObs struct {
    TrackID     string
    UnixNanos   int64
    WorldFrame  FrameID
    PoseID      int64
    X, Y, Z     float32
    VX, VY, VZ  float32
    SpeedMps    float32
    HeadingRad  float32
    BBoxL, BBoxW, BBoxH float32
    HeightP95   float32
    IntensityMean float32
}

// Sidecar state ---------------------------------------------------------------
type SidecarState struct {
    Poses   *PoseCache
    BG      map[string]*BackgroundGrid
    Tracks  map[string]*Track               // by TrackID
    RecentClusters []*WorldCluster          // ring buffer for UI/debug
}
```

---

## 4) Algorithms (concise)

### 4.1 UDP ingest & parse
- Create a UDP socket on `-udp_addr` (default `:2368`).
- Set `SetReadBuffer(4<<20)`; reuse a 1500‑byte buffer.
- For each 1262‑byte packet, call `parser.ParsePacket(data)` → `[]Point` in sensor frame with per‑point timestamps.
- Append to a **FrameBuilder** using either:
  - **Time window** (e.g., 100 ms per frame), or
  - **Spin wrap** (detect azimuth wrap-around).

### 4.2 Background subtractor (sensor frame)
- Bin each point into `(ring, azimuthBin)`; `azimuthBin = floor(azimuth_deg / bin_size_deg)` (e.g., 0.2° → 1800 bins).
- Decision rule per bin:
  ```
  motion_threshold = average_range
                   - closeness_sensitivity_multiplier * range_spread
                   - safety_margin
  is_foreground = (current_range < motion_threshold)
  ```
- Apply **3×3 neighbor vote**; require ≥ `neighbor_confirmation_count` neighbors.
- If a cell is foreground, **freeze** updates for `freeze_duration` (prevent absorbing stopped cars immediately).
- Else (not foreground, not frozen) update `average_range` and `range_spread` by slow EMA using `background_update_fraction`.

**Outputs:** boolean foreground mask per bin + counters.

### 4.3 Foreground selection & clustering
- Keep only points whose bins are foreground (after neighbor vote).
- (Optional) light ground prior; not required because BG already suppresses static planes.
- Run Euclidean clustering:
  - Parameters: `eps ≈ 0.6 m`, `minPts ≈ 12`.
  - Compute per‑cluster: centroid, PCA bbox (L/W/H), `pointsCount`, `heightP95`, `intensityMean`.
- Create `WorldCluster` for each cluster **after** transform (next step).

### 4.4 World transform
- Lookup `Pose` from `PoseCache` for `sensor_id`.
- Apply `T_world_sensor` to cluster centroid (and optionally to points if needed for bbox); stamp `PoseID` and `WorldFrame`.

### 4.5 Tracking (world frame)
- State vector `[x,y,vx,vy]`, constant‑velocity KF.
- Process noise `Q` (start): diag(0.5, 0.5, 1.0, 1.0). Measurement noise `R`: diag(0.3, 0.3).
- Per tick:
  1. **Predict** all tracks to current frame time.
  2. **Associate** clusters → tracks using Mahalanobis distance on position; greedy assignment is acceptable initially.
  3. **Update** matched tracks with cluster centroid; update running bbox and rollups (avg/peak speed, height).
  4. **Birth** new tracks from unmatched clusters.
  5. **Delete** tracks missing for > 0.5 s (configurable).
- Produce **recent track list** for HTTP endpoints.

---

## 5) HTTP (no gRPC)

### 5.1 `GET /health`
Returns basic liveness and rates:
```json
{
  "udp_active": true,
  "last_packet_ns": 1699999999999999999,
  "frames_per_sec": 9.8,
  "bg_bins_frozen": 123,
  "foreground_points": 8421,
  "tracks_live": 6
}
```

### 5.2 `GET /fg`
Foreground/background counts for quick tuning:
```json
{ "foreground_points": 8421, "background_points": 99123, "bins_frozen": 321 }
```

### 5.3 `GET /tracks/recent?since_ns=...&limit=...`
Array of recent tracks (latest state per track). Minimal fields:
```json
[
  {
    "track_id":"t-123",
    "sensor_id":"hesai-01",
    "world_frame":"site/main-st-001",
    "pose_id":7,
    "unix_nanos":1699999999999999999,
    "x":12.3, "y":-3.4, "vx":8.1, "vy":0.2, "speed_mps":8.1, "heading_rad":1.57,
    "bbox_l":4.2, "bbox_w":1.9, "bbox_h":1.6,
    "points_count":86, "height_p95":1.5, "intensity_mean":42.0,
    "class_label":"", "class_conf":0.0
  }
]
```

### 5.4 `GET /track/:id`
Full time series for a single track (down‑sample if long). Use `TrackObs` fields.

### 5.5 (Optional) `GET /range-image.png`
Quick PNG of the current range image with foreground overlay for debugging.

---

## 6) Concurrency model

- `udpReader` goroutine: reads packets → bounded channel (drop with counter if full).
- `parser` goroutine: packet → points → frame builder.
- `frameLoop` goroutine: per frame → BG → clustering → transform → tracking → update recent lists.
- `httpServer` goroutine: serves endpoints; supports graceful shutdown.
- Use `errgroup.WithContext` in `main.go`; cancel all on first error; close channels; allow ≤ 1s drain.

---

## 7) Configuration (flags/env)

- `-sensor_id` (string), `-site_id` (string)
- `-udp_addr` (default `:2368`)
- `-model` (e.g., `Pandar40P`)
- `-pose_file` (JSON with 4x4 `T` and `pose_id`) **or** `-pose_db` (defer if not ready)
- BG dials:
  - `-bg.update_fraction` (default `0.02`)
  - `-bg.sensitivity_multiplier` (default `3.0`)
  - `-bg.safety_margin_m` (default `0.5`)
  - `-bg.freeze_duration_ms` (default `5000`)
  - `-bg.neighbor_votes` (default `5`)
- Rates:
  - `-frame_ms` (default `100`), **or** `-spin_mode`
- HTTP:
  - `-http` (default `:8080`)

---

## 8) Testing checklist

- **Offline replay**: feed `.pcap` → verify stable `frames_per_sec`, increasing foreground when a moving object enters.
- **BG sanity**: stationary scene yields foreground ratio near zero after warm‑up; moving box synthetic test is detected.
- **Tilt tolerance**: mount the “sensor frame only” BG; verify moving car shows as foreground regardless of tilt.
- **Tracking**: two movers crossing; greedy association stays stable; track IDs don’t flap; deletion after misses.
- **Pose swap**: hot‑reload pose file (optional) and verify world coords jump coherently and responses include new `pose_id`.
- **HTTP**: endpoints return within <10 ms and don’t allocate large buffers.

---

## 9) Performance targets (initial)

- End‑to‑end latency (packet → recent track JSON): **< 100 ms** typical.
- CPU: ~1–2 cores at **10–15 Hz**.
- Memory: **< 300 MB** with 40×1800 background and small ring buffers.

---

## 10) Milestones to ship

1. UDP→Parser→Frame builder + `/health` counters.
2. Background subtractor + `/fg` (tunable dials).
3. Clustering → world transform → `/tracks/recent` (static pose file).
4. Tracking + `/track/:id` (down‑sampled series).
5. (Optional) `/range-image.png` and pose cache hot‑reload.

---

### Notes

- Keep **canonical** positions/velocities in world frame and attach **PoseID** to all outputs.  
- Background learns slowly; set a **warm‑up** window (e.g., ignore outputs for first 10–30 s or start with higher threshold and taper down).  
- You can add DB persistence later in the main app using the schema we defined previously.
