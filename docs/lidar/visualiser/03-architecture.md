# Architecture

This document describes the system architecture for the macOS LiDAR visualiser and the supporting pipeline refactor.

---

## Industry Standards Alignment

This architecture aligns with industry-standard LiDAR perception formats:

| Standard                        | Implementation                           | Reference                                                                                |
| ------------------------------- | ---------------------------------------- | ---------------------------------------------------------------------------------------- |
| **7-DOF Bounding Box**          | `OrientedBoundingBox` in protobuf schema | [av-lidar-integration-plan.md](../future/av-lidar-integration-plan.md)                   |
| **Coordinate Frame Convention** | ENU (East-North-Up) world frame          | [static-pose-alignment-plan.md](../future/static-pose-alignment-plan.md)                 |
| **Background Grid**             | Polar range image with VTK export option | [lidar-background-grid-standards.md](../architecture/lidar-background-grid-standards.md) |

The `OrientedBoundingBox` message in `visualiser.proto` uses the same field layout as `BoundingBox7DOF` from the AV integration spec, enabling direct conversion for AV dataset import/export.

---

## 1. Split Plan: Two Parallel Tracks

### Track A: Visualiser (Primary)

The macOS application that renders point clouds, tracks, and debug overlays.

**Scope**:

- SwiftUI application shell
- gRPC client (grpc-swift)
- Metal renderer for point clouds and geometry
- UI controls (connection, playback, overlays)
- Labelling workflow and export

**Unblocked by**: Synthetic data generators and recorded logs. Track A can progress **before** Track B completes the pipeline refactor.

### Track B: Pipeline API + Tracking Refactor (Supporting)

The Go server-side changes to emit a stable API for the visualiser.

**Scope**:

- Define canonical internal model
- gRPC/protobuf publisher
- Recorder/replayer for logs
- Debug artifact emission
- Adapter layer for LidarView (preserve existing behaviour)
- Label REST API endpoints (`/api/lidar/labels`) with SQLite persistence

**Linked by**: The protobuf schema (`visualiser.proto`) is the **only coupling** between tracks.

```
┌─────────────────────────────────────────────────────────────────────┐
│                          TRACK A                                    │
│                       (Visualiser)                                  │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  macOS App (Swift)                                          │    │
│  │  ┌─────────┐  ┌─────────────┐  ┌──────────────┐             │    │
│  │  │ gRPC    │→ │ Decode +    │→ │ Render Graph │             │    │
│  │  │ Client  │  │ Transform   │  │ (Metal)      │             │    │
│  │  └─────────┘  └─────────────┘  └──────────────┘             │    │
│  │       ↓                              ↓                      │    │
│  │  ┌─────────────────────────────────────────────────────┐    │    │
│  │  │ UI Layer (SwiftUI)                                  │    │    │
│  │  │ - Connection panel    - Playback controls           │    │    │
│  │  │ - Overlay toggles     - Label panel                 │    │    │
│  │  │ - Detail inspector    - Export menu                 │    │    │
│  │  └─────────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                              ▲
                              │ gRPC (localhost:50051) - Point cloud streaming
                              │ REST (localhost:8080)  - Labels, metadata
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          TRACK B                                    │
│                   (Pipeline + Refactor)                             │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  Go Server                                                  │    │
│  │                                                             │    │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────────────┐ │    │
│  │  │ Ingest  │→ │ PreProc │→ │ Cluster │→ │ Track           │ │    │
│  │  │ (UDP)   │  │ (BG/FG) │  │ (DBSCAN)│  │ (Kalman)        │ │    │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────────────┘ │    │
│  │                                                 ↓           │    │
│  │                                         ┌─────────────────┐ │    │
│  │                                         │ Canonical Model │ │    │
│  │                                         │ (FrameBundle)   │ │    │
│  │                                         └─────────┬───────┘ │    │
│  │                                                   │         │    │
│  │              ┌───────────────┬────────────────────┼         │    │
│  │              ▼               ▼                    ▼         │    │
│  │      ┌───────────────┐ ┌───────────────┐ ┌───────────────┐  │    │
│  │      │ LidarView     │ │ gRPC          │ │ Recorder      │  │    │
│  │      │ Adapter       │ │ Publisher     │ │ (.vrlog)      │  │    │
│  │      │ (port 2370)   │ │ (port 50051)  │ │               │  │    │
│  │      └───────────────┘ └───────────────┘ └───────────────┘  │    │
│  └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. Module Diagrams

### 2.1 Visualiser Modules (Track A)

```
tools/visualiser-macos/
├── VelocityVisualiser/
│   ├── App/
│   │   ├── VelocityVisualiserApp.swift     # Entry point
│   │   └── AppState.swift                   # Global state + M3.5 cache status
│   │
│   ├── gRPC/
│   │   ├── VisualiserClient.swift          # gRPC client wrapper
│   │   ├── FrameDecoder.swift              # Proto → Swift models
│   │   └── Generated/                       # grpc-swift codegen
│   │
│   ├── Rendering/
│   │   ├── MetalRenderer.swift             # Main render coordinator (M4 cluster boxes)
│   │   ├── CompositePointCloudRenderer.swift # M3.5: dual BG/FG buffer renderer
│   │   ├── PointCloudRenderer.swift        # Point sprites/shading
│   │   ├── BoxRenderer.swift               # Instanced box rendering
│   │   ├── TrailRenderer.swift             # Polyline trails
│   │   ├── OverlayRenderer.swift           # Debug overlays
│   │   ├── Shaders/
│   │   │   ├── PointCloud.metal            # Point shaders
│   │   │   ├── Box.metal                   # Box shaders
│   │   │   └── Trail.metal                 # Trail shaders
│   │   └── Camera.swift                    # 3D camera controls
│   │
│   ├── UI/
│   │   ├── ContentView.swift               # Main window (M3.5 cache indicator, M4 cluster toggle)
│   │   ├── ConnectionPanel.swift           # Connect/disconnect
│   │   ├── PlaybackControls.swift          # Pause/play/seek
│   │   ├── OverlayToggles.swift            # Visibility toggles
│   │   ├── TrackInspector.swift            # Track detail view
│   │   └── LabelPanel.swift                # Labelling UI
│   │
│   ├── Labelling/
│   │   ├── LabelAPIClient.swift            # REST API client for labels
│   │   ├── LabelExporter.swift             # JSON export (via API)
│   │   └── LabelEvent.swift                # Data model
│   │
│   └── Models/
│       ├── FrameBundle.swift               # Swift representation (M3.5 frame types)
│       ├── Track.swift
│       ├── Cluster.swift
│       └── PointCloud.swift
│
├── VelocityVisualiser.xcodeproj/
└── README.md
```

### 2.2 Pipeline Modules (Track B)

```
internal/
├── lidar/
│   ├── ... (existing files)
│   │
│   ├── tracker_interface.go             # M4: TrackerInterface (6 methods)
│   ├── clusterer_interface.go            # M4: ClustererInterface (3 methods)
│   ├── dbscan_clusterer.go              # M4: Deterministic DBSCAN wrapper
│   ├── golden_replay_test.go            # M4: Golden determinism tests
│   ├── background_snapshot_test.go       # M3.5: Background snapshot tests
│   │
│   ├── visualiser/                          # gRPC publisher (M2–M3.5)
│   │   ├── publisher.go                    # gRPC streaming server + BG scheduling
│   │   ├── model.go                        # Canonical FrameBundle + M3.5 types
│   │   ├── adapter.go                      # Pipeline → FrameBundle (M4: TrackerInterface)
│   │   ├── grpc_server.go                  # frameBundleToProto() + M3.5 conversion
│   │   ├── config.go                       # Feature flags
│   │   └── publisher_m35_test.go            # M3.5: Publisher background tests
│   │
│   ├── labels/                              # Label persistence (future M6)
│   │   ├── store.go                        # Label CRUD operations (SQLite)
│   │   ├── models.go                       # Label data models
│   │   └── api.go                          # REST API handlers (/api/lidar/labels)
│   │
│   ├── recorder/                            # Log recording (M1)
│   │   ├── recorder.go                     # Write .vrlog files
│   │   ├── replayer.go                     # Read + stream logs
│   │   ├── index.go                        # Seek index
│   │   └── format.go                       # Chunk format
│   │
│   └── debug/                               # Debug artifacts (future M6)
│       ├── overlays.go                     # Gating, association
│       └── collector.go                    # Per-frame collection
│
proto/
└── velocity_visualiser/
    └── v1/
        ├── visualiser.proto                 # Schema (M3.5: FrameType, BackgroundSnapshot)
        └── buf.gen.yaml                     # Codegen config
```

---

## 3. Transport Choice

### 3.1 Why gRPC for Point Cloud Streaming

| Requirement         | gRPC Advantage                |
| ------------------- | ----------------------------- |
| **Structured data** | Native protobuf support       |
| **Streaming**       | Built-in server-streaming RPC |
| **Type safety**     | Generated Swift + Go stubs    |

### 3.2 Why REST for Labeling and Metadata

| Requirement               | REST Advantage                            |
| ------------------------- | ----------------------------------------- |
| **Shared with web UI**    | Same API for macOS app and browser        |
| **Persistent storage**    | Direct SQLite access from Go backend      |
| **CRUD operations**       | Standard HTTP verbs (GET/POST/PUT/DELETE) |
| **Simple caching**        | HTTP caching semantics                    |
| **Debugging**             | Easy to inspect with curl/Postman         |
| **Bidirectional control** | Control RPCs for playback                 |
| **Performance**           | HTTP/2 multiplexing, binary encoding      |
| **Future-proof**          | Easy to extend to remote access           |

> **IMPORTANT: Single Database Constraint**
>
> The macOS visualiser **MUST NOT** maintain its own SQLite database for labels or any other persistent data. All label storage is handled by the Go backend via REST API (`/api/lidar/labels`). This ensures:
>
> - **Single source of truth**: Labels are consistent across macOS app and web UI
> - **No data synchronisation issues**: No risk of divergent local databases
> - **Centralised backup**: All data lives in the Go server's SQLite database
> - **Shared access**: Web UI and visualiser can see each other's labels immediately
>
> The visualiser's `LabelAPIClient.swift` is a REST client only — it performs HTTP requests to the Go backend and does not access any local database.

### 3.3 Alternatives Considered

| Option       | Rejected Because                            |
| ------------ | ------------------------------------------- |
| Raw UDP      | No reliability, no structure, no control    |
| WebSocket    | Requires JSON or custom binary, web-centric |
| REST polling | High latency, inefficient for streaming     |
| Unix socket  | Less portable, harder tooling               |

### 3.3 Future Remote Access

Current scope: **localhost only** (127.0.0.1:50051).

Future option: Enable TLS + authentication for remote access from field laptops. Out of scope for MVP.

---

## 4. macOS Rendering Approach

### 4.1 Framework Evaluation

| Framework      | Point Count | Instancing | Custom Shaders | Verdict                  |
| -------------- | ----------- | ---------- | -------------- | ------------------------ |
| **SceneKit**   | ~50k        | Limited    | Possible       | Too slow for full clouds |
| **RealityKit** | ~100k       | Good       | Limited        | AR-focused, not ideal    |
| **Metal**      | 500k+       | Excellent  | Full control   | **Selected**             |

### 4.2 Metal Implementation Strategy

**Chosen approach**: Direct Metal with `MTKView` for maximum control.

```swift
// Core rendering pipeline
class MetalRenderer: NSObject, MTKViewDelegate {
    var device: MTLDevice
    var commandQueue: MTLCommandQueue
    var pointCloudPipeline: MTLRenderPipelineState
    var boxPipeline: MTLRenderPipelineState
    var trailPipeline: MTLRenderPipelineState

    // Per-frame data
    var pointBuffer: MTLBuffer?      // Interleaved xyz + intensity
    var boxInstances: MTLBuffer?     // Transform matrices
    var trailVertices: MTLBuffer?    // Polyline vertices

    func draw(in view: MTKView) {
        // 1. Update buffers from latest frame
        // 2. Encode point cloud draw call
        // 3. Encode instanced boxes
        // 4. Encode trails
        // 5. Encode 2D overlays
        // 6. Present
    }
}
```

### 4.3 Rendering Techniques

**Point Sprites / Point Shading**:

```metal
vertex PointOutput pointVertex(
    uint vid [[vertex_id]],
    constant float4 *points [[buffer(0)]],
    constant Uniforms &uniforms [[buffer(1)]]
) {
    float4 pos = points[vid];
    PointOutput out;
    out.position = uniforms.mvp * float4(pos.xyz, 1.0);
    out.pointSize = uniforms.pointSize / out.position.w;
    out.intensity = pos.w;
    return out;
}
```

**Instanced Boxes**:

```metal
vertex BoxOutput boxVertex(
    uint vid [[vertex_id]],
    uint iid [[instance_id]],
    constant float3 *boxVerts [[buffer(0)]],
    constant BoxInstance *instances [[buffer(1)]],
    constant Uniforms &uniforms [[buffer(2)]]
) {
    BoxInstance inst = instances[iid];
    float3 worldPos = inst.transform * boxVerts[vid];
    BoxOutput out;
    out.position = uniforms.mvp * float4(worldPos, 1.0);
    out.color = inst.color;
    return out;
}
```

**Trail Rendering**:

- Polylines as triangle strips with varying alpha
- Fade based on age: `alpha = 1.0 - (age / max_trail_age)`

**Text/Labels**:

- 2D overlay using Core Graphics
- Billboards in 3D: position in world, render as sprite

---

## 5. Threading Model

### 5.1 Visualiser Threading

```
┌─────────────────────────────────────────────────────────────────┐
│                    macOS Visualiser Threads                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐                                               │
│  │ Network      │  - gRPC client recv loop                      │
│  │ Queue        │  - Decode protobuf → Swift models             │
│  │ (Background) │  - Push to frame queue                        │
│  └──────┬───────┘                                               │
│         │                                                       │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │ Frame        │  - Thread-safe queue                          │
│  │ Queue        │  - Bounded size (backpressure)                │
│  │              │  - Drop oldest if full                        │
│  └──────┬───────┘                                               │
│         │                                                       │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │ Simulation   │  - Playback clock management                  │
│  │ Thread       │  - Seek handling                              │
│  │              │  - Rate control                               │
│  └──────┬───────┘                                               │
│         │                                                       │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │ Render       │  - MTKView draw callback                      │
│  │ Thread       │  - GPU command encoding                       │
│  │ (Main/Metal) │  - Present to screen                          │
│  └──────┬───────┘                                               │
│         │                                                       │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │ UI Thread    │  - SwiftUI state updates                      │
│  │ (Main)       │  - User input handling                        │
│  └──────────────┘                                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 5.2 Backpressure Strategy

- Frame queue bounded to **10 frames** (~0.5-1 second at 10-20 Hz)
- If queue is full, **drop oldest frame**
- UI shows "frame drop" indicator
- Client can request reduced decimation to lower bandwidth

---

## 6. Comparison / A-B Workflow

### 6.1 Parallel Output

During development and testing:

```bash
# Start pipeline with both outputs enabled
velocity-report --lidar-forward-enabled --grpc-enabled

# LidarView receives Pandar40P packets on port 2370
# Visualiser receives FrameBundles on port 50051
```

### 6.2 Comparison Metrics

Since LidarView shows raw points and the visualiser shows semantic data, direct visual comparison isn't possible. Instead, compare:

| Metric            | LidarView                   | Visualiser                  | Comparison                      |
| ----------------- | --------------------------- | --------------------------- | ------------------------------- |
| Point count/frame | Packet analysis             | `PointCloudFrame.x.len()`   | Should match (if no decimation) |
| Foreground count  | N/A (all points same color) | Foreground classification   | N/A                             |
| Track count       | N/A                         | `TrackSet.tracks.len()`     | Compare with DB                 |
| Cluster count     | N/A                         | `ClusterSet.clusters.len()` | Compare with DB                 |

### 6.3 Regression Testing

1. Record known-good log with track IDs and timestamps
2. Replay through pipeline
3. Compare output tracks:
   - Same track IDs created at same frames
   - Same lifecycle transitions
   - Same final statistics
4. Any delta is a regression

---

## 7. Key File References (Existing Code)

### 7.1 LiDAR Ingestion

| File                                 | Purpose                  |
| ------------------------------------ | ------------------------ |
| `internal/lidar/network/listener.go` | UDP packet reception     |
| `internal/lidar/parse/extract.go`    | Pandar40P packet parsing |
| `internal/lidar/frame_builder.go`    | Rotation accumulation    |

### 7.2 Foreground Extraction

| File                           | Purpose                                       |
| ------------------------------ | --------------------------------------------- |
| `internal/lidar/background.go` | Background model (polar grid) + M3.5 snapshot |
| `internal/lidar/foreground.go` | Foreground/background classification          |

### 7.3 Clustering and Tracking

| File                                    | Purpose                                 |
| --------------------------------------- | --------------------------------------- |
| `internal/lidar/clustering.go`          | DBSCAN clustering                       |
| `internal/lidar/clusterer_interface.go` | M4: ClustererInterface                  |
| `internal/lidar/dbscan_clusterer.go`    | M4: Deterministic DBSCAN wrapper        |
| `internal/lidar/tracking.go`            | Kalman tracker (Tracker, TrackedObject) |
| `internal/lidar/tracker_interface.go`   | M4: TrackerInterface                    |
| `internal/lidar/tracking_pipeline.go`   | Pipeline orchestration + frame throttle |
| `internal/lidar/golden_replay_test.go`  | M4: Golden determinism tests            |

### 7.4 LidarView Forwarding

| File                                             | Purpose                       |
| ------------------------------------------------ | ----------------------------- |
| `internal/lidar/network/foreground_forwarder.go` | Encode + forward to port 2370 |
| `internal/lidar/network/forwarder.go`            | Raw packet forwarding         |

### 7.5 Transform and Types

| File                          | Purpose                                 |
| ----------------------------- | --------------------------------------- |
| `internal/lidar/transform.go` | Spherical ↔ Cartesian, pose transforms  |
| `internal/lidar/arena.go`     | Core data types (Point, Cluster, Track) |

---

## 8. Known Issues & Deferred Optimisations

This section documents known limitations and deferred work from the M2/M3/M3.5/M4 implementation that will be addressed in M7 (Performance Hardening).

### 8.1 Memory Pool Not Fully Utilised (Go)

**Issue**: The `PointCloudFrame` type uses `sync.Pool` for slice allocation (`getFloat32Slice()`, `getUint8Slice()`), but the `Release()` method is not called in broadcast scenarios.

**Reason**: In the Publisher's broadcast loop, the same frame is sent to multiple gRPC clients. Calling `Release()` after one client converts to protobuf would corrupt data for other clients still using the frame.

**Impact**: Pool slices are allocated but never returned; effectively defeats pooling benefit.

**Deferred Solution**: Reference counting (see [04-implementation-plan.md §7.2](./04-implementation-plan.md#72-pointcloudframe-memory-pool-release-strategy)).

### 8.2 Swift Buffer Allocation Per Frame

**Issue**: `MetalRenderer.updatePointBuffer()` allocates a new `vertices` array (350 KB for 70k points) on every frame.

**Impact**: At 10-20 fps, this creates ~5 MB/s allocation pressure, increasing GC load.

**Deferred Solution**: Buffer pooling or pre-allocation (see [04-implementation-plan.md §7.1](./04-implementation-plan.md#71-swift-buffer-pooling)).

### 8.3 Frame Skipping Lacks Cooldown

**Issue**: When gRPC streaming detects slow clients, it aggressively skips frames. However, there's no hysteresis to prevent oscillation between skip and normal modes.

**Impact**: Client may experience jittery frame delivery after recovering from backpressure.

**Deferred Solution**: Add cooldown counter (see [04-implementation-plan.md §7.3](./04-implementation-plan.md#73-frame-skipping-cooldown)).

### 8.4 Decimation Ratio Edge Cases

**Issue**: Very small decimation ratios (< 0.01) can result in only 1 point being kept.

**Workaround**: Callers should validate ratios. Minimum recommended: 0.01 (1%).

**Documentation**: Added to [04-implementation-plan.md §7.4](./04-implementation-plan.md#74-decimation-edge-cases).

### 8.5 Go 1.21+ Dependency

**Note**: The code uses the built-in `max()` function introduced in Go 1.21. This is compatible with the project's Go 1.21+ requirement (see `go.mod`). No action needed, but noted for reference.

### 8.6 PCAP Catch-Up Burst Processing (Partially Addressed)

**Issue**: During PCAP replay, when the pipeline blocks on a heavy frame (>16k foreground points), PCAP buffers packets. When the pipeline unblocks, PCAP dumps the backlog at 33+ fps, causing dropped frames on the client.

**Partial Fix**: `MaxFrameRate` throttle (default 12 fps) in `tracking_pipeline.go` skips expensive downstream processing (clustering, tracking, forwarding) when frames arrive faster than `minFrameInterval`. Background model update still runs on every frame to maintain accuracy.

**Partial Fix**: `maskBuf []bool` reuse in `ProcessFramePolarWithMask()` avoids per-frame allocation of the foreground mask buffer.

**Remaining**: Full solution would require PCAP replay rate control to prevent burst accumulation.

---

## 9. Related Documents

- [01-problem-and-user-workflows.md](./01-problem-and-user-workflows.md) – Problem statement
- [02-api-contracts.md](./02-api-contracts.md) – API contract (protobuf schema)
- [04-implementation-plan.md](./04-implementation-plan.md) – Milestones and tasks
- [../refactor/01-tracking-upgrades.md](../refactor/01-tracking-upgrades.md) – Tracking improvements
