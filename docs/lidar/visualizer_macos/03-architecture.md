# Architecture

This document describes the system architecture for the macOS LiDAR visualiser and the supporting pipeline refactor.

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

**Linked by**: The protobuf schema (`visualizer.proto`) is the **only coupling** between tracks.

```
┌─────────────────────────────────────────────────────────────────────┐
│                          TRACK A                                     │
│                       (Visualiser)                                   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  macOS App (Swift)                                           │   │
│  │  ┌─────────┐  ┌─────────────┐  ┌──────────────┐             │   │
│  │  │ gRPC    │→ │ Decode +    │→ │ Render Graph │             │   │
│  │  │ Client  │  │ Transform   │  │ (Metal)      │             │   │
│  │  └─────────┘  └─────────────┘  └──────────────┘             │   │
│  │       ↓                              ↓                        │   │
│  │  ┌─────────────────────────────────────────────────────┐    │   │
│  │  │ UI Layer (SwiftUI)                                    │    │   │
│  │  │ - Connection panel    - Playback controls             │    │   │
│  │  │ - Overlay toggles     - Label panel                   │    │   │
│  │  │ - Detail inspector    - Export menu                   │    │   │
│  │  └─────────────────────────────────────────────────────┘    │   │
│  │       ↓                                                       │   │
│  │  ┌─────────────────────────────────────────────────────┐    │   │
│  │  │ Labelling Store (SQLite / JSON)                       │    │   │
│  │  └─────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
                              ▲
                              │ gRPC (localhost:50051)
                              │ Protocol: visualizer.proto
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          TRACK B                                     │
│                   (Pipeline + Refactor)                              │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Go Server                                                    │   │
│  │                                                               │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────────────┐│   │
│  │  │ Ingest  │→ │ PreProc │→ │ Cluster │→ │ Track           ││   │
│  │  │ (UDP)   │  │ (BG/FG) │  │ (DBSCAN)│  │ (Kalman)        ││   │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────────────┘│   │
│  │                                               ↓              │   │
│  │                                    ┌───────────────────┐    │   │
│  │                                    │ Canonical Model   │    │   │
│  │                                    │ (FrameBundle)     │    │   │
│  │                                    └─────────┬─────────┘    │   │
│  │                                              │               │   │
│  │              ┌───────────────┬───────────────┼───────────────┐   │
│  │              ▼               ▼               ▼               │   │
│  │      ┌───────────────┐ ┌───────────────┐ ┌───────────────┐  │   │
│  │      │ LidarView     │ │ gRPC          │ │ Recorder      │  │   │
│  │      │ Adapter       │ │ Publisher     │ │ (.vrlog)      │  │   │
│  │      │ (port 2370)   │ │ (port 50051)  │ │               │  │   │
│  │      └───────────────┘ └───────────────┘ └───────────────┘  │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. Module Diagrams

### 2.1 Visualiser Modules (Track A)

```
tools/visualizer-macos/
├── VelocityVisualizer/
│   ├── App/
│   │   ├── VelocityVisualizerApp.swift     # Entry point
│   │   └── AppState.swift                   # Global state
│   │
│   ├── gRPC/
│   │   ├── VisualizerClient.swift          # gRPC client wrapper
│   │   ├── FrameDecoder.swift              # Proto → Swift models
│   │   └── Generated/                       # grpc-swift codegen
│   │
│   ├── Rendering/
│   │   ├── MetalRenderer.swift             # Main render coordinator
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
│   │   ├── ContentView.swift               # Main window
│   │   ├── ConnectionPanel.swift           # Connect/disconnect
│   │   ├── PlaybackControls.swift          # Pause/play/seek
│   │   ├── OverlayToggles.swift            # Visibility toggles
│   │   ├── TrackInspector.swift            # Track detail view
│   │   └── LabelPanel.swift                # Labelling UI
│   │
│   ├── Labelling/
│   │   ├── LabelStore.swift                # Local SQLite storage
│   │   ├── LabelExporter.swift             # JSON export
│   │   └── LabelEvent.swift                # Data model
│   │
│   └── Models/
│       ├── FrameBundle.swift               # Swift representation
│       ├── Track.swift
│       ├── Cluster.swift
│       └── PointCloud.swift
│
├── VelocityVisualizer.xcodeproj/
└── README.md
```

### 2.2 Pipeline Modules (Track B)

```
internal/
├── lidar/
│   ├── ... (existing files unchanged)
│   │
│   ├── visualizer/                          # NEW: gRPC publisher
│   │   ├── publisher.go                    # gRPC streaming server
│   │   ├── model.go                        # Canonical FrameBundle
│   │   ├── adapter.go                      # Pipeline → FrameBundle
│   │   └── config.go                       # Feature flags
│   │
│   ├── recorder/                            # NEW: Log recording
│   │   ├── recorder.go                     # Write .vrlog files
│   │   ├── replayer.go                     # Read + stream logs
│   │   ├── index.go                        # Seek index
│   │   └── format.go                       # Chunk format
│   │
│   └── debug/                               # NEW: Debug artifacts
│       ├── overlays.go                     # Gating, association
│       └── collector.go                    # Per-frame collection
│
proto/
└── velocity_visualizer/
    └── v1/
        ├── visualizer.proto                 # Schema definition
        └── buf.gen.yaml                     # Codegen config
```

---

## 3. Transport Choice

### 3.1 Why gRPC over localhost

| Requirement               | gRPC Advantage                       |
| ------------------------- | ------------------------------------ |
| **Structured data**       | Native protobuf support              |
| **Streaming**             | Built-in server-streaming RPC        |
| **Type safety**           | Generated Swift + Go stubs           |
| **Bidirectional control** | Control RPCs for playback            |
| **Performance**           | HTTP/2 multiplexing, binary encoding |
| **Future-proof**          | Easy to extend to remote access      |

### 3.2 Alternatives Considered

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
│                    macOS Visualiser Threads                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐                                               │
│  │ Network      │  - gRPC client recv loop                      │
│  │ Queue        │  - Decode protobuf → Swift models              │
│  │ (Background) │  - Push to frame queue                        │
│  └──────┬───────┘                                               │
│         │                                                        │
│         ▼                                                        │
│  ┌──────────────┐                                               │
│  │ Frame        │  - Thread-safe queue                          │
│  │ Queue        │  - Bounded size (backpressure)                │
│  │              │  - Drop oldest if full                        │
│  └──────┬───────┘                                               │
│         │                                                        │
│         ▼                                                        │
│  ┌──────────────┐                                               │
│  │ Simulation   │  - Playback clock management                  │
│  │ Thread       │  - Seek handling                              │
│  │              │  - Rate control                               │
│  └──────┬───────┘                                               │
│         │                                                        │
│         ▼                                                        │
│  ┌──────────────┐                                               │
│  │ Render       │  - MTKView draw callback                      │
│  │ Thread       │  - GPU command encoding                       │
│  │ (Main/Metal) │  - Present to screen                          │
│  └──────┬───────┘                                               │
│         │                                                        │
│         ▼                                                        │
│  ┌──────────────┐                                               │
│  │ UI Thread    │  - SwiftUI state updates                      │
│  │ (Main)       │  - User input handling                        │
│  └──────────────┘                                               │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 5.2 Backpressure Strategy

- Frame queue bounded to **10 frames** (~1 second at 10 Hz)
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

| File                           | Purpose                              |
| ------------------------------ | ------------------------------------ |
| `internal/lidar/background.go` | Background model (polar grid)        |
| `internal/lidar/foreground.go` | Foreground/background classification |

### 7.3 Clustering and Tracking

| File                                  | Purpose                                 |
| ------------------------------------- | --------------------------------------- |
| `internal/lidar/clustering.go`        | DBSCAN clustering                       |
| `internal/lidar/tracking.go`          | Kalman tracker (Tracker, TrackedObject) |
| `internal/lidar/tracking_pipeline.go` | Pipeline orchestration                  |

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

## 8. Related Documents

- [01-problem-and-user-workflows.md](./01-problem-and-user-workflows.md) – Problem statement
- [02-api-contracts.md](./02-api-contracts.md) – API contract (protobuf schema)
- [04-implementation-plan.md](./04-implementation-plan.md) – Milestones and tasks
- [../refactor/01-tracking-upgrades.md](../refactor/01-tracking-upgrades.md) – Tracking improvements
