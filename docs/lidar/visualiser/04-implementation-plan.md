# Implementation Plan

This document defines an incremental, API-first implementation plan with explicit milestones and acceptance criteria.

**Current Status** (February 2026):

- ✅ **M0: Schema + Synthetic** — Complete
- ✅ **M1: Recorder/Replayer** — Complete
- ✅ **M2: Real Point Clouds** — Complete
- ✅ **M3: Canonical Model + Adapters** — Complete
- 🔲 **M4–M7** — Not started

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
 M4: Tracking Interface Refactor   ──▶ Golden replay tests pass
 M5: Algorithm Upgrades            ──▶ Improved tracking quality
 M6: Debug + Labelling             ──▶ Full debug overlays + label export
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

### M4: Tracking Refactor Behind Interfaces

**Goal**: Wrap current tracking in interfaces without changing algorithms. Enable golden replay tests.

**Track B (Pipeline)**:

- [ ] Define `Tracker` interface abstracting current implementation
- [ ] Define `Clusterer` interface for DBSCAN
- [ ] Inject interfaces via dependency injection
- [ ] `FrameBundle` includes `ClusterSet` and `TrackSet`
- [ ] Golden replay test: compare track IDs/states frame-by-frame
- [ ] Determinism: seed any RNG, sort clusters by centroid

**Track A (Visualiser)**:

- [ ] Render `ClusterSet` as boxes
- [ ] Render `TrackSet` with IDs and colours
- [ ] Track trails from `TrackTrail` data

**Acceptance Criteria**:

- [ ] Golden replay test passes (identical tracks each run)
- [ ] Visualiser shows clusters + tracks correctly
- [ ] Track IDs stable across replay
- [ ] No algorithm changes (pure refactor)

**Estimated Dev-Days**: 8 (2 Track A + 6 Track B)

---

### M5: Algorithm Upgrades

**Goal**: Improve tracking quality with refined algorithms.

**Track B (Pipeline)**:

- [ ] Improved ground removal (RANSAC or height threshold)
- [ ] Voxel grid downsampling option
- [ ] OBB estimation from cluster PCA
- [ ] Temporal OBB smoothing
- [ ] Hungarian algorithm for association (optional upgrade)
- [ ] Occlusion handling improvements
- [ ] Classification hooks (feature extraction)

See [../refactor/01-tracking-upgrades.md](../refactor/01-tracking-upgrades.md) for detailed proposals.

**Track A (Visualiser)**:

- [ ] Render OBB (oriented boxes)
- [ ] Show OBB heading arrows

**Acceptance Criteria**:

- [ ] OBB headings align with vehicle direction
- [ ] Track continuity improved (fewer splits/merges)
- [ ] Performance metrics maintained or improved

**Estimated Dev-Days**: 12 (2 Track A + 10 Track B)

---

### M6: Debug Overlays + Labelling Export

**Goal**: Full debug visualisation and labelling workflow.

**Track B (Pipeline)**:

- [ ] Emit `DebugOverlaySet` with association candidates
- [ ] Emit gating ellipses from Mahalanobis distance
- [ ] Emit innovation residuals
- [ ] Emit state predictions
- [ ] Toggle debug output via `SetOverlayModes` RPC

**Track A (Visualiser)**:

- [ ] Render association lines (dashed, colour-coded)
- [ ] Render gating ellipses
- [ ] Render residual vectors
- [ ] Track selection (click to select)
- [ ] Track detail panel
- [ ] Label assignment UI
- [ ] REST API client for label CRUD operations
- [ ] Label export to JSON (via API)

**Track B (Pipeline)**:

- [ ] `lidar_labels` table schema migration
- [ ] Label API endpoints (POST/GET/PUT/DELETE)
- [ ] Label filtering by track_id, time range, class
- [ ] JSON export endpoint for ML pipeline
- [ ] Integration with existing `/api/lidar/tracks` endpoint

**Acceptance Criteria**:

- [ ] All debug overlays render correctly
- [ ] Labels persist in SQLite database
- [ ] Labels accessible from both visualiser and web UI
- [ ] Export produces valid JSON for ML pipeline
- [ ] Labelling workflow < 3 seconds per track

**Estimated Dev-Days**: 12 (8 Track A + 4 Track B)

---

### M7: Performance Hardening

**Goal**: Production-ready performance and stability.

**Track A (Visualiser)**:

- [ ] GPU buffer pooling (avoid allocations per frame)
- [ ] Triple buffering for smooth rendering
- [ ] Memory usage < 500 MB
- [ ] CPU profiling and optimisation
- [ ] GPU profiling (Metal System Trace)

**Track B (Pipeline)**:

- [ ] gRPC streaming optimisation
- [ ] Protobuf arena allocators
- [ ] Decimation auto-adjustment based on bandwidth
- [ ] Memory profiling for 100+ track scale

**Acceptance Criteria**:

- [ ] 70,000 points at 30 fps sustained
- [ ] 200 tracks render without frame drops
- [ ] Memory stable over 1 hour session
- [ ] CPU usage < 30% on M1 MacBook

**Estimated Dev-Days**: 8 (4 Track A + 4 Track B)

---

## 3. Task Breakdown Summary

| Milestone              | Track A (Days) | Track B (Days) | Total (Days) | Status      |
| ---------------------- | -------------- | -------------- | ------------ | ----------- |
| M0: Schema + Synthetic | 5              | 5              | 10           | ✅ Complete |
| M1: Recorder/Replayer  | 4              | 4              | 8            | ✅ Complete |
| M2: Real Points        | 2              | 4              | 6            | ✅ Complete |
| M3: Canonical Model    | 0              | 5              | 5            | ✅ Complete |
| M4: Tracking Refactor  | 2              | 6              | 8            |
| M5: Algorithm Upgrades | 2              | 10             | 12           |             |
| M6: Debug + Labelling  | 8              | 4              | 12           |             |
| M7: Performance        | 4              | 4              | 8            |             |
| **Total**              | **27**         | **42**         | **69**       | **29 done** |

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

| Milestone | Stop Point                               | Status      |
| --------- | ---------------------------------------- | ----------- |
| M0        | Synthetic visualisation works end-to-end | ✅ Complete |
| M1        | Replay with seek/pause works             | ✅ Complete |
| M2        | Real point clouds render                 | ✅ Complete |
| M3        | Both outputs work from same model        | ✅ Complete |
| M4        | Golden replay tests pass                 |             |
| M5        | Improved tracking quality validated      |             |
| M6        | Labelling workflow complete              |             |
| M7        | Performance targets met                  |             |

**MVP = M0 + M1 + M2**: Visualiser shows real data with basic playback. ✅ **ACHIEVED**

**V1.0 = M0 - M6**: Full debug + labelling capability.

**V1.1 = M7**: Production-ready performance.

---

## 6. Related Documents

- [01-problem-and-user-workflows.md](./01-problem-and-user-workflows.md) – Problem statement
- [02-api-contracts.md](./02-api-contracts.md) – API contract
- [03-architecture.md](./03-architecture.md) – System architecture
- [../refactor/01-tracking-upgrades.md](../refactor/01-tracking-upgrades.md) – Tracking improvements
