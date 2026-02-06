# Implementation Plan

This document defines an incremental, API-first implementation plan with explicit milestones and acceptance criteria.

**Current Status** (February 2026):

- ✅ **M0: Schema + Synthetic** — Complete
- ✅ **M1: Recorder/Replayer** — Complete
- ✅ **M2: Real Point Clouds** — Complete
- ✅ **M3: Canonical Model + Adapters** — Complete
- ✅ **M3.5: Split Streaming** — Complete (Track A + Track B)
- ✅ **M4: Tracking Interface Refactor** — Complete (Track A + Track B)
- ✅ **M5: Algorithm Upgrades** — Complete (Track B)
- ✅ **M6: Debug + Labelling** — Complete (Track B)
- 🔶 **M7** — In progress (7.1 and 7.2 complete)

**Checkbox Legend**:

- `[x]` — Completed
- `[ ]` — Not started
- `[~]` — Skipped / Won't do

---

## 1. Milestone Overview

```
 M0: Schema + Synthetic            ──▶ Visualiser renders synthetic data     ✅ DONE
 M1: Recorder/Replayer             ──▶ Deterministic playback works          ✅ DONE
 M2: Real Point Clouds             ──▶ Pipeline emits live points via gRPC   ✅ DONE
 M3: Canonical Model + Adapters    ──▶ LidarView + gRPC from same source     ✅ DONE
 M3.5: Split Streaming             ──▶ BG/FG separation, 96% bandwidth cut   ✅ DONE
 M4: Tracking Interface Refactor   ──▶ Golden replay tests pass              ✅ DONE
 M5: Algorithm Upgrades            ──▶ Improved tracking quality             ✅ DONE
 M6: Debug + Labelling             ──▶ Full debug overlays + label export    ✅ DONE
 M7: Performance Hardening         ──▶ Production-ready performance
```

---

## 2. Detailed Milestones

### M0: Protobuf Schema + gRPC Stub + Synthetic Publisher + macOS Viewer ✅

**Status**: Complete

**Goal**: Visualiser renders synthetic point clouds, boxes, and trails. Validates end-to-end pipeline without real tracking.

**Track A (Visualiser)**:

- [x] SwiftUI app shell with window management
- [x] `MTKView` integration for Metal rendering
- [x] Point cloud renderer (point sprites)
- [x] Instanced box renderer (AABB)
- [x] Trail renderer (fading polylines)
- [x] gRPC client connects to localhost:50051
- [x] Decode `FrameBundle` from stream
- [x] Basic UI: connect/disconnect, overlay toggles

**Track B (Pipeline)**:

- [x] `proto/velocity_visualiser/v1/visualiser.proto` schema
- [~] `buf.gen.yaml` for Go + Swift codegen
- [x] `Makefile` target: `make proto-gen`
- [x] Synthetic data generator (rotating points, moving boxes)
- [x] gRPC server stub with `StreamFrames` RPC
- [x] Serves synthetic `FrameBundle` at 10-20 Hz (configurable)

**Acceptance Criteria**:

- [x] Visualiser connects to Go server
- [x] Renders 10,000+ synthetic points at 30fps
- [x] Shows 10 synthetic boxes moving in circles
- [x] Trails fade over 2 seconds
- [x] No crashes on disconnect/reconnect

**Estimated Dev-Days**: 10 (5 Track A + 5 Track B)

---

### M1: Recorder/Replayer with Deterministic Playback ✅

**Status**: Complete

**Goal**: Record synthetic frames to `.vrlog`, replay with identical output.

**Implementation Notes**:

- Frame index-based seeking (not frame ID) for accurate navigation
- `seekOccurred` flag ensures timing reset after seek operations
- `sendOneFrame` flag enables stepping while paused
- Rate control supports discrete values: 0.5x, 1x, 2x, 4x, 8x, 16x, 32x, 64x
- Race condition fix in adapter.go for history iteration

**Track A (Visualiser)**:

- [x] 3D camera controls: orbit (rotate), pan, zoom
- [x] Mouse/trackpad gesture support for camera
- [x] Keyboard shortcuts for camera reset
- [x] Playback controls: pause/play/seek/rate
- [x] Timeline scrubber with frame timestamps
- [x] Frame stepping (previous/next)
- [x] Playback rate adjustment (0.5x - 64x)
- [x] Display playback position in UI

**Track B (Pipeline)**:

- [x] `.vrlog` file format (header + index + chunks)
- [x] `Recorder` writes streamed frames to disk
- [x] `Replayer` reads log and streams via gRPC
- [x] Seek to timestamp or frame ID
- [x] Rate control (wallclock vs playback time)
- [x] Control RPCs: `Pause`, `Play`, `Seek`, `SetRate`

**Acceptance Criteria**:

- [x] Record 60 seconds of synthetic data
- [x] Replay produces identical frames (byte-for-byte)
- [x] Seek to arbitrary timestamp < 500ms
- [x] Playback at 0.5x and 2x works correctly
- [x] Pause/resume maintains correct position

**Estimated Dev-Days**: 8 (4 Track A + 4 Track B)

---

### M2: Real Point Clouds via gRPC ✅

**Status**: Complete

**Goal**: Pipeline emits actual LiDAR point clouds via gRPC. Visualiser renders real data.

**Implementation Notes**:

- `FrameAdapter.AdaptFrame()` converts pipeline LiDARFrame → FrameBundle
- Point cloud adaptation includes foreground/background classification
- Decimation modes implemented: none, uniform, foreground-only, voxel (stub)
- Integration via `TrackingPipelineConfig.VisualiserPublisher` and `VisualiserAdapter`
- SwiftUI visualiser decodes classification field and renders accordingly

**Track B (Pipeline)**:

- [x] Wire `FrameBuilder` output to gRPC publisher
- [x] Convert `LiDARFrame` → `PointCloudFrame` proto
- [x] Foreground mask classification in point data
- [x] Decimation modes (full, uniform, foreground-only)
- [~] Feature flag: `--grpc-enabled` (not needed, uses optional adapters)

**Track A (Visualiser)**:

- [x] Handle 70,000+ points per frame
- [x] Colour by classification (foreground/background)
- [x] Colour by intensity
- [x] Point size adjustment

**Acceptance Criteria**:

- [x] Visualiser shows live point cloud from sensor
- [x] Foreground points highlighted in different colour
- [x] Frame rate ≥ 30 fps with full point cloud
- [x] Decimation reduces bandwidth as expected

**Estimated Dev-Days**: 6 (2 Track A + 4 Track B)

---

### M3: Canonical Internal Model + Adapters ✅

**Status**: Complete

**Goal**: Introduce canonical `FrameBundle` as single source of truth. Both LidarView and gRPC consume from same model.

**Implementation Notes**:

- `internal/lidar/visualiser/model.go` defines canonical FrameBundle model (311 lines)
- `adapter.go` implements FrameAdapter with AdaptFrame(), adaptPointCloud(), adaptClusters(), adaptTracks()
- `lidarview_adapter.go` implements LidarViewAdapter for UDP forwarding
- `publisher.go` implements Publisher for gRPC streaming (247 lines)
- Pipeline routes through interface checks: if both adapters present, publishes to both
- LidarView-only mode preserved when gRPC adapter is nil
- Dual-mode operation in `tracking_pipeline.go` Phase 6

**Track B (Pipeline)**:

- [x] Define `internal/lidar/visualiser/model.go` with Go structs
- [x] `Adapter` converts tracking output → `FrameBundle`
- [x] `LidarViewAdapter` consumes `FrameBundle` → Pandar40P packets
- [x] `GRPCPublisher` consumes `FrameBundle` → proto stream
- [x] Feature flag: `--forward-mode=lidarview|grpc|both` (implemented via optional interfaces)
- [x] Preserve existing LidarView behaviour unchanged

**Acceptance Criteria**:

- [x] `--forward-mode=lidarview` produces identical output to current
- [x] `--forward-mode=grpc` works for visualiser
- [x] `--forward-mode=both` runs simultaneously
- [x] No regression in LidarView packet format

**Estimated Dev-Days**: 5 (5 Track B)

---

### M3.5: Split Streaming for Static LiDAR ✅

**Status**: Complete

**Goal**: Reduce gRPC bandwidth by 96% by sending background snapshots infrequently and foreground-only frames per tick.

**Problem**: At 10 fps with 70k points/frame (970 KB), the pipeline sustains ~80 Mbps. For static LiDAR, 97% of points are background (unchanging). Sending all points every frame wastes bandwidth and causes client backpressure.

**Solution**: Send background snapshot every 30s (~920 KB), send foreground-only frames at 10 fps (~30 KB). Net bandwidth: ~3 Mbps.

See [performance-investigation.md](./performance-investigation.md) for detailed design.

**Implementation Notes**:

- `FrameType` enum added to protobuf: `FULL`, `FOREGROUND`, `BACKGROUND`, `DELTA`
- `BackgroundSnapshot` and `GridMetadata` messages added to protobuf schema
- `GenerateBackgroundSnapshot()` on `BackgroundManager` converts settled polar grid → Cartesian point cloud
- Publisher schedules background every 30s (configurable via `BackgroundInterval`)
- `CheckForSensorMovement()` detects >20% foreground ratio (configurable)
- `CheckBackgroundDrift()` monitors cell drift >0.5m across >10% of cells (configurable)
- Sequence number increments on grid reset for client cache coherence
- macOS `CompositePointCloudRenderer` maintains dual Metal buffers (background + foreground)
- Cache states: Empty → Cached(seq) → Refreshing
- UI indicator shows green/orange/grey dot for cache status

**Track B (Pipeline)**:

- [x] Add `FrameType` enum to protobuf (`FULL`, `FOREGROUND`, `BACKGROUND`, `DELTA`)
- [x] Add `BackgroundSnapshot` message to protobuf schema
- [x] Implement `GenerateBackgroundSnapshot()` on BackgroundManager
- [x] Add background snapshot scheduling to Publisher (30s default)
- [~] Add `--vis-background-interval` CLI flag (configured via `BackgroundInterval` field)
- [x] Implement foreground-only frame adaptation in FrameAdapter
- [x] Add sensor movement detection (`CheckForSensorMovement`)
- [x] Add background drift detection (`CheckBackgroundDrift`)
- [x] Handle grid reset → sequence number increment
- [x] Unit tests for background snapshot generation
- [x] Unit tests for movement detection

**Track A (Visualiser)**:

- [x] Update protobuf stubs for new message types
- [x] Implement `CompositePointCloudRenderer` with background cache
- [x] Handle `FrameType.background` → update cached buffer
- [x] Handle `FrameType.foreground` → render over cached background
- [x] Request background refresh when `backgroundSeq` mismatches
- [x] Add UI indicator for "Background: Cached" vs "Refreshing"
- [x] Performance test: verify <5 Mbps bandwidth achieved

**Acceptance Criteria**:

- [x] Background snapshot sent every 30s (configurable)
- [x] Foreground frames contain only moving points + metadata
- [x] Bandwidth reduced from ~80 Mbps to <5 Mbps
- [x] No visual difference from full-frame mode
- [x] Sensor movement triggers background refresh
- [x] Client handles reconnect with stale cache gracefully

**Estimated Dev-Days**: 8 (3 Track A + 5 Track B)

---

### M4: Tracking Interface Refactor ✅

**Status**: Complete

**Goal**: Wrap current tracking in interfaces without changing algorithms. Enable golden replay tests.

**Implementation Notes**:

- `TrackerInterface` (6 methods): `Update`, `GetActiveTracks`, `GetConfirmedTracks`, `GetTrack`, `GetTrackCount`, `GetAllTracks`
- `ClustererInterface` (3 methods): `Cluster`, `GetParams`, `SetParams`
- `DBSCANClusterer` wraps existing DBSCAN with deterministic output (clusters sorted by centroid X, then Y)
- `TrackingPipelineConfig.Tracker` changed from `*Tracker` to `TrackerInterface`
- Golden replay tests verify identical track IDs, states, positions, and velocities across runs
- Floating point tolerances: positions 1e-5, velocities 1e-4
- No algorithm changes — pure interface wrapping
- Cluster rendering on macOS: cyan wireframe boxes (RGBA 0.0, 0.8, 1.0, 0.7), toggle with 'C' key
- Track rendering: state-coloured boxes (green/yellow/red), toggle with 'B' key

**Track B (Pipeline)**:

- [x] Define `Tracker` interface abstracting current implementation (`tracker_interface.go`)
- [x] Define `Clusterer` interface for DBSCAN (`clusterer_interface.go`)
- [x] Inject interfaces via dependency injection (`tracking_pipeline.go`, `adapter.go`)
- [x] `FrameBundle` includes `ClusterSet` and `TrackSet`
- [x] Golden replay test: compare track IDs/states frame-by-frame (`golden_replay_test.go`)
- [x] Determinism: clusters sorted by centroid (X, Y) in `DBSCANClusterer`

**Track A (Visualiser)**:

- [x] Render `ClusterSet` as cyan boxes (`updateClusterInstances()` in `MetalRenderer.swift`)
- [x] Render `TrackSet` with IDs and state colours
- [x] Track trails from `TrackTrail` data

**Acceptance Criteria**:

- [x] Golden replay test passes (identical tracks each run)
- [x] Visualiser shows clusters + tracks correctly
- [x] Track IDs stable across replay
- [x] No algorithm changes (pure refactor)

**Test Coverage**:

- `internal/lidar`: 89.6% coverage
- `internal/lidar/visualiser`: 76.8% coverage
- 4 golden replay tests, 7 DBSCANClusterer tests, 7 TrackerInterface tests

**Estimated Dev-Days**: 8 (2 Track A + 6 Track B)

---

### M5: Algorithm Upgrades ✅

**Status**: Complete (Track A + Track B)

**Goal**: Improve tracking quality with refined algorithms.

**Implementation Notes**:

- `internal/lidar/ground.go` implements height-based ground removal (MinHeight 0.2m, MaxHeight 3.0m)
- `internal/lidar/obb.go` implements PCA-based oriented bounding box estimation
- OBB smoothing integrated into `TrackedObject` with exponential moving average (α=0.3)
- `internal/lidar/debug/collector.go` provides debug artifact collection framework

**Track B (Pipeline)**:

- [x] Improved ground removal (height threshold)
- [x] Voxel grid downsampling (`internal/lidar/voxel.go`)
- [x] OBB estimation from cluster PCA
- [x] Temporal OBB smoothing
- [x] Hungarian algorithm for association (`internal/lidar/hungarian.go`)
- [x] Occlusion handling (confirmed tracks coast 8 frames, covariance inflation)
- [x] Classification hooks (`internal/lidar/features.go`, periodic re-classification)

See [../refactor/01-tracking-upgrades.md](../refactor/01-tracking-upgrades.md) for detailed proposals.

**Track A (Visualiser)**:

- [x] Render OBB (oriented boxes)
- [x] Show OBB heading arrows

**Acceptance Criteria**:

- [x] OBB headings computed via PCA (align with vehicle shape)
- [x] Track continuity improved (Hungarian association, occlusion coasting)
- [x] Performance metrics maintained or improved

**Estimated Dev-Days**: 12 (2 Track A + 10 Track B)

---

### M6: Debug Overlays + Labelling Export ✅

**Status**: Complete (Track A + Track B)

**Goal**: Full debug visualisation and labelling workflow.

**Implementation Notes**:

- Debug collector in `internal/lidar/debug/collector.go` records association candidates, gating ellipses, innovation residuals, and state predictions
- Tracking integration captures debug data at predict(), associate(), mahalanobisDistanceSquared(), and update() steps
- Database migration 000016 adds `lidar_labels` table
- REST API in `internal/api/lidar_labels.go` provides full CRUD + export for labels
- `SetOverlayModes` gRPC RPC implemented in `grpc_server.go`

**Track B (Pipeline)**:

- [x] Emit `DebugOverlaySet` with association candidates
- [x] Emit gating ellipses from Mahalanobis distance
- [x] Emit innovation residuals
- [x] Emit state predictions
- [x] Toggle debug output via `SetOverlayModes` RPC

**Track A (Visualiser)**:

- [x] Render association lines (dashed, colour-coded)
- [x] Render gating ellipses
- [x] Render residual vectors
- [x] Track selection (click to select)
- [x] Track detail panel
- [x] Label assignment UI
- [x] REST API client for label CRUD operations
- [x] Label export to JSON (via API)

**Track B (Pipeline)**:

- [x] `lidar_labels` table schema migration
- [x] Label API endpoints (POST/GET/PUT/DELETE)
- [x] Label filtering by track_id, time range, class
- [x] JSON export endpoint for ML pipeline
- [~] Integration with existing `/api/lidar/tracks` endpoint (deferred)

**Acceptance Criteria**:

- [x] All debug overlays render correctly
- [x] Labels persist in SQLite database
- [x] Labels accessible from both visualiser and web UI
- [x] Export produces valid JSON for ML pipeline
- [x] Labelling workflow < 3 seconds per track

**Estimated Dev-Days**: 12 (8 Track A + 4 Track B)

---

### M7: Performance Hardening

**Goal**: Production-ready performance and stability.

**Track A (Visualiser)**:

- [x] GPU buffer pooling (avoid allocations per frame) — M7.1 implemented
- [ ] Triple buffering for smooth rendering
- [ ] Memory usage < 500 MB
- [ ] CPU profiling and optimisation
- [ ] GPU profiling (Metal System Trace)
- [x] Swift vertex buffer reuse (see §7.1 below) — M7.1 implemented

**Track B (Pipeline)**:

- [ ] gRPC streaming optimisation
- [ ] Protobuf arena allocators
- [ ] Decimation auto-adjustment based on bandwidth
- [ ] Memory profiling for 100+ track scale
- [x] PointCloudFrame memory pool with reference counting (see §7.2 below) — M7.2 implemented
- [x] Frame skipping with cooldown mechanism (see §7.3 below) — M7.3 implemented

**Acceptance Criteria**:

- [ ] 70,000 points at 30 fps sustained
- [ ] 200 tracks render without frame drops
- [ ] Memory stable over 1 hour session
- [ ] CPU usage < 30% on M1 MacBook
- [ ] No memory leaks from pooled allocations

**Estimated Dev-Days**: 8 (4 Track A + 4 Track B)

#### 7.1 Swift Buffer Pooling ✅

**Status**: Implemented (February 2026)

**Problem**: `MetalRenderer.updatePointBuffer()` allocates a new `vertices` array for every frame. At 10-20 fps with 70k points, this creates allocation pressure.

**Options**:

1. **Pre-allocated buffer**: Keep a single large `MTLBuffer` and reuse if point count hasn't changed significantly
2. **Buffer pool**: Similar to Go implementation, maintain pool of `MTLBuffer` objects by size class
3. **Ring buffer**: Triple buffer with fence synchronisation

**Recommendation**: Start with option 1 (simplest), benchmark, escalate to option 2 if needed.

**Implementation (February 2026)**: Option 1 implemented in both `MetalRenderer.swift` and `CompositePointCloudRenderer.swift`:

- Buffer capacity tracked separately from point count
- Reallocation only when capacity is insufficient or >4x larger than needed
- 50% growth margin to reduce reallocation frequency
- `getBufferStats()` method added for performance monitoring

#### 7.2 PointCloudFrame Memory Pool (Release() Strategy) ✅

**Status**: Implemented (February 2026) using Option A (Reference Counting)

**Problem**: The Go `PointCloudFrame` uses `sync.Pool` for slice allocation via `getFloat32Slice()` and `getUint8Slice()`. A `Release()` method exists to return slices to the pool. However, in broadcast scenarios (Publisher sends same frame to multiple gRPC clients), calling `Release()` would corrupt data for other consumers.

**Options for Proper Pool Utilisation**:

| Option                            | Description                                                                                                                                        | Pros                                                 | Cons                                                   |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- | ------------------------------------------------------ |
| **A: Reference Counting** ✅      | Add `refCount` field to PointCloudFrame. Increment on broadcast to each client, decrement after protobuf conversion. Release when count hits zero. | Pool actually gets reused; memory-efficient at scale | Added complexity; must ensure all code paths decrement |
| **B: Copy-on-Broadcast**          | Each client receives a deep copy of the frame                                                                                                      | Simple ownership model; no shared state              | Defeats purpose of pooling; higher memory use          |
| **C: Single-Client Optimisation** | Only use pool in replay mode (single client). Live mode uses regular allocation.                                                                   | Works today without changes                          | Pool only helps replay; live mode still allocates      |
| **D: Remove Pooling**             | Delete pool code; use regular slices                                                                                                               | Simplest; fewer bugs                                 | Higher GC pressure at 70k points × 10 Hz               |

**Implementation (February 2026)**:

- `refCount atomic.Int32` field added to `PointCloudFrame` in `model.go`
- `Retain()` method increments reference count before sharing
- `RefCount()` method returns current count for testing/debugging
- `Release()` decrements count and only returns slices to pool when count reaches zero
- `broadcastLoop()` in `publisher.go` calls `Retain()` for each client, `Release()` on drop
- `streamFromPublisher()` in `grpc_server.go` calls `Release()` after protobuf conversion
- Skipped/paused frames properly release their references

**Original Implementation Sketch (Option A)** (preserved for reference):

```go
type PointCloudFrame struct {
    // ... existing fields ...
    refCount atomic.Int32
}

func (pc *PointCloudFrame) Retain() {
    pc.refCount.Add(1)
}

func (pc *PointCloudFrame) Release() {
    if pc.refCount.Add(-1) == 0 {
        putFloat32Slice(pc.X)
        // ... return other slices ...
    }
}

// In broadcastLoop:
for _, client := range p.clients {
    frame.PointCloud.Retain()
    select {
    case client.frameCh <- frame:
    default:
        frame.PointCloud.Release() // Wasn't sent
    }
}

// In streamFromPublisher after protobuf conversion:
frame.PointCloud.Release()
```

#### 7.3 Frame Skipping Cooldown ✅

**Status**: Implemented (July 2025)

**Problem**: The gRPC streaming code skips frames when `consecutiveSlowSends >= maxConsecutiveSlowSends`, but there's no cooldown after catching up. This could cause continued aggressive skipping even after the client recovers.

**Current Behaviour**: Skip frames while slow, reset counter on fast send.

**Proposed Enhancement**: After entering skip mode, require N consecutive fast sends before exiting skip mode (hysteresis). This prevents oscillation.

**Implementation (July 2025)**:

- Extracted `sendCooldown` struct in `grpc_server.go` with hysteresis logic
- `maxConsecutiveSlowSends = 3` — consecutive slow sends to enter skip mode
- `minConsecutiveFastSends = 5` — consecutive fast sends required to exit skip mode
- `recordSlow()` / `recordFast()` / `inSkipMode()` methods for clean separation
- A slow send during recovery resets the fast counter, preventing premature exit
- In normal mode, a fast send resets the slow counter (original behaviour preserved)
- 9 unit tests covering: entry, exit, interruption, return values, threshold edge cases

#### 7.4 Decimation Edge Cases

**Current Behaviour**: For very small ratios (e.g., 0.00001), `targetCount` becomes 1, and only the first point is kept.

**Documented Behaviour**: This is intentional for extreme decimation. A minimum ratio of 0.01 (1%) is recommended for practical use. Callers should validate ratios before calling `ApplyDecimation()`.

---

## 3. Task Breakdown Summary

| Milestone              | Track A (Days) | Track B (Days) | Total (Days) | Status              |
| ---------------------- | -------------- | -------------- | ------------ | ------------------- |
| M0: Schema + Synthetic | 5              | 5              | 10           | ✅ Complete         |
| M1: Recorder/Replayer  | 4              | 4              | 8            | ✅ Complete         |
| M2: Real Points        | 2              | 4              | 6            | ✅ Complete         |
| M3: Canonical Model    | 0              | 5              | 5            | ✅ Complete         |
| M3.5: Split Streaming  | 3              | 5              | 8            | ✅ Complete         |
| M4: Tracking Refactor  | 2              | 6              | 8            | ✅ Complete         |
| M5: Algorithm Upgrades | 2              | 10             | 12           | ✅ Complete (B)     |
| M6: Debug + Labelling  | 8              | 4              | 12           | ✅ Complete (B)     |
| M7: Performance        | 4              | 4              | 8            |                     |
| **Total**              | **30**         | **47**         | **77**       | **59 Track B done** |

---

## 4. Risks and Mitigations

### 4.1 Protobuf Churn

**Risk**: Schema changes during development break client/server compatibility.

**Mitigation**:

- Freeze schema at M0 completion
- Use optional fields for new additions
- Version bump only for breaking changes

### 4.2 Performance Bottlenecks

**Risk**: 70k points × 10-20 Hz overwhelms bandwidth or GPU.

**Mitigation**:

- Implement decimation modes early (M0)
- Profile incrementally at each milestone
- Foreground-only mode as fallback

### 4.3 Determinism Failures

**Risk**: Replay produces different tracks due to floating-point or timing issues.

**Mitigation**:

- Seed all RNG with deterministic value
- Sort clusters by (x, y) before processing
- Use integer timestamps, not floating-point deltas
- Golden replay tests in CI

### 4.4 Coordinate Frame Bugs

**Risk**: Misaligned coordinates between pipeline and visualiser.

**Mitigation**:

- Document coordinate conventions clearly
- Include test frame with known geometry (axis marker)
- Validate with LidarView comparison

### 4.5 ID Stability

**Risk**: Track IDs change on replay due to association order.

**Mitigation**:

- Deterministic cluster sorting
- Deterministic track ID generation (hash of first cluster + timestamp)
- Golden replay tests validate ID stability

---

## 5. Stop Points

Each milestone has a **stop point** where functionality is complete and stable:

| Milestone | Stop Point                               | Status          |
| --------- | ---------------------------------------- | --------------- |
| M0        | Synthetic visualisation works end-to-end | ✅ Complete     |
| M1        | Replay with seek/pause works             | ✅ Complete     |
| M2        | Real point clouds render                 | ✅ Complete     |
| M3        | Both outputs work from same model        | ✅ Complete     |
| M3.5      | Bandwidth reduced to <5 Mbps             | ✅ Complete     |
| M4        | Golden replay tests pass                 | ✅ Complete     |
| M5        | Improved tracking quality validated      | ✅ Complete (B) |
| M6        | Labelling workflow complete              | ✅ Complete (B) |
| M7        | Performance targets met                  |                 |

**MVP = M0 + M1 + M2**: Visualiser shows real data with basic playback. ✅ **ACHIEVED**

**V1.0 = M0 - M6**: Full debug + labelling capability. ✅ **Track B ACHIEVED** (Track A pending)

**V1.1 = M7**: Production-ready performance.

---

## 6. Related Documents

- [01-problem-and-user-workflows.md](./01-problem-and-user-workflows.md) – Problem statement
- [02-api-contracts.md](./02-api-contracts.md) – API contract
- [03-architecture.md](./03-architecture.md) – System architecture
- [../refactor/01-tracking-upgrades.md](../refactor/01-tracking-upgrades.md) – Tracking improvements
