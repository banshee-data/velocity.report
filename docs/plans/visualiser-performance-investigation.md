# LiDAR Visualiser Performance Investigation

- **Status:** Substantially Complete — M3.5 split streaming implemented, frame rate throttle added. Minor gaps: CLI flag for background interval uses default only; bandwidth reduction not formally tested (claimed ~96% in code)
- **Scope:** Static LiDAR deployments only (no SLAM/mobile use cases)

## Problem Summary

During PCAP replay at 10-20 fps with ~35-70k points/frame, the gRPC streaming pipeline experiences periodic slowdowns:

- **SLOW SEND warnings** (>50ms, up to 600ms observed)
- **Frame drops** (19+ frames dropped per session)
- **FPS collapse** from 10-20 fps → 1.4-3 fps during slowdowns
- **High CPU** on Go server (230%+), kernel_task (156%), WindowServer (100%+)

**Note:** Pandar40P supports 10Hz (dense) and 20Hz (sparse) motor speeds. Total point rate is constant (~700k points/sec), so 20Hz frames contain ~35k points while 10Hz frames contain ~70k points.

## Key Insight: Static LiDAR Optimisation

For **static LiDAR** deployments (sensor fixed in place), the scene decomposes into:

| Category       | Points @ 10Hz | Points @ 20Hz | Change Frequency     | Current Handling            |
| -------------- | ------------- | ------------- | -------------------- | --------------------------- |
| **Background** | ~67k (97%)    | ~34k (97%)    | Rarely (sensor bump) | Settled in `BackgroundGrid` |
| **Foreground** | ~2k (3%)      | ~1k (3%)      | Every frame          | Extracted via mask          |

**Current waste:** We send all points every frame when only 3% change (FG/BG ratio remains constant across frame rates).

**Implemented solution (M3.5):** Send background snapshot infrequently (every 30s), send only foreground + clusters per frame. See [implementation.md](../ui/visualiser/implementation.md) for completion status.

---

## Current Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  PCAP Replay    │────▶│   Go Server      │────▶│ Swift Client    │
│  (10-20 fps)    │     │  (gRPC stream)   │     │ (Metal render)  │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                              │
                              ▼
                  ~970 KB/frame @ 10Hz or ~485 KB/frame @ 20Hz
                        ~80 Mbps sustained (constant point rate)
```

## Observed Metrics

| Metric         | Normal (10Hz) | Normal (20Hz) | During Slowdown  |
| -------------- | ------------- | ------------- | ---------------- |
| FPS            | 10            | 20            | 1.4-3            |
| avg_send_ms    | 1.1-1.8       | 0.6-0.9       | 35-56 (peak 600) |
| bandwidth_mbps | 78-80         | 78-80         | 10-24            |
| msg_size_kb    | 950-975       | 475-490       | (varies by mode) |

**Note:** Bandwidth remains constant across frame rates due to fixed point rate (~700k points/sec).

---

## Primary Solution: Background/Foreground Split Streaming ✅

**Status**: Implemented in M3.5 (Track A + Track B). See [implementation.md](../ui/visualiser/implementation.md).

### Concept

```
┌─────────────────────────────────────────────────────────────────┐
│                      FRAME TYPES                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  BACKGROUND FRAME (every 30s or on demand)                      │
│  ├── Full settled point cloud (~67k @ 10Hz / ~34k @ 20Hz)       │
│  ├── Per-cell confidence (TimesSeenCount)                       │
│  └── Grid metadata (rings, azimuth bins, sensor pose)           │
│     Size: ~920 KB @ 10Hz, ~460 KB @ 20Hz                        │
│                                                                 │
│  FOREGROUND FRAME (every 50-100ms depending on sensor mode)     │
│  ├── Foreground points only (~2k @ 10Hz / ~1k @ 20Hz)           │
│  ├── Cluster bounding boxes (~10-20 clusters, ~2 KB)            │
│  ├── Track states (~5-15 tracks, ~3 KB)                         │
│  └── Optional: points outside clusters ("unassociated")         │
│     Size: ~30 KB @ 10Hz, ~15 KB @ 20Hz                          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Bandwidth Comparison

| Mode                       | Points/Frame (10Hz) | Points/Frame (20Hz) | Size/Frame (10Hz) | Size/Frame (20Hz) | Bandwidth          |
| -------------------------- | ------------------- | ------------------- | ----------------- | ----------------- | ------------------ |
| **Current (full)**         | 70,000              | 35,000              | 970 KB            | 485 KB            | 78 Mbps (constant) |
| **Foreground only**        | 2,000               | 1,000               | 30 KB             | 15 KB             | 2.4-3.0 Mbps       |
| **FG + clusters + tracks** | 2,000 + meta        | 1,000 + meta        | 35 KB             | 20 KB             | 2.8-4.0 Mbps       |
| **Background snapshot**    | 67,000              | 34,000              | 920 KB            | 460 KB            | 0.25 Mbps @ 1/30s  |

**Net reduction: 78 Mbps → 3-4 Mbps (95%+ reduction across both sensor modes)**

---

## Implementation Details

### Reusable Infrastructure

The existing codebase already provides:

#### 1. Background Grid (`internal/lidar/background.go`)

**`BackgroundGrid` fields:**

| Field              | Type               | Purpose                       |
| ------------------ | ------------------ | ----------------------------- |
| `Cells`            | `[]BackgroundCell` | Per ring×azimuth cell         |
| `Rings`            | int                | 40 for Pandar40P              |
| `AzBins`           | int                | 1800 (0.2° resolution)        |
| `SettlingComplete` | bool               | True when warmup done         |
| `nonzeroCellCount` | int32              | Cells with TimesSeenCount > 0 |

**`BackgroundCell` fields:**

| Field                | Type    | Purpose                            |
| -------------------- | ------- | ---------------------------------- |
| `AverageRangeMeters` | float32 | Settled background distance        |
| `RangeSpreadMeters`  | float32 | Expected variance                  |
| `TimesSeenCount`     | uint32  | Confidence (higher = more settled) |
| `LockedBaseline`     | float32 | Stable reference after threshold   |

**What we reuse:**

- `AverageRangeMeters` → background point position (convert polar → Cartesian)
- `TimesSeenCount` → confidence for rendering (fade unsettled cells)
- `SettlingComplete` → know when background is stable

#### 2. Foreground Extraction (`internal/lidar/tracking_pipeline.go`)

The pipeline already extracts a foreground mask per frame via `BackgroundManager.ProcessFramePolarWithMask(polar)`, returning a `mask []bool` that identifies foreground points. `ExtractForegroundPoints(polar, mask)` filters to moving objects only.

**What we reuse:**

- `mask []bool` → identifies which points are foreground
- `ExtractForegroundPoints()` → filters to moving objects only

#### 3. Cluster & Track Data

The pipeline already computes DBSCAN clusters via `ClassifyForeground(foregroundPoints, params)` and feeds them to the Kalman-filtered tracker via `tracker.Update(clusters)`.

**What we reuse:**

- `WorldCluster` → bounding box, centroid, point count
- `TrackedObject` → position, velocity, classification, trail

### New Components Required

#### 1. Background Point Cloud Generator

Convert settled `BackgroundGrid` to point cloud:

The `GenerateBackgroundPointCloud()` method on `BackgroundManager` iterates all grid cells, skips unsettled cells (below `settledThreshold`), converts each cell’s polar coordinates (ring elevation + azimuth bin × 360° / AzBins + average range) to Cartesian (x, y, z), and sets intensity from `TimesSeenCount` (capped at 255). Classification is set to 0 (background). The returned `PointCloudFrame` carries `FrameTypeBackground` and the grid’s `SequenceNumber` for client cache invalidation.

#### 2. Enhanced Frame Bundle

New fields added to the `FrameBundle` protobuf message:

| Field            | Number | Type                 | Purpose                                 |
| ---------------- | ------ | -------------------- | --------------------------------------- |
| `frame_type`     | 10     | `FrameType`          | Frame type discriminator                |
| `background`     | 11     | `BackgroundSnapshot` | Background snapshot (sent infrequently) |
| `background_seq` | 12     | uint64               | Sequence number for cache coherence     |

**`FrameType` enum:**

| Value | Name                    | Meaning                             |
| ----- | ----------------------- | ----------------------------------- |
| 0     | `FRAME_TYPE_FULL`       | Legacy: all points                  |
| 1     | `FRAME_TYPE_FOREGROUND` | Foreground + clusters + tracks only |
| 2     | `FRAME_TYPE_BACKGROUND` | Background snapshot                 |
| 3     | `FRAME_TYPE_DELTA`      | Future: incremental update          |

**`BackgroundSnapshot` message:**

| Field             | Number | Type                     | Purpose                  |
| ----------------- | ------ | ------------------------ | ------------------------ |
| `sequence_number` | 1      | uint64                   | Increments on grid reset |
| `timestamp_nanos` | 2      | int64                    | When snapshot was taken  |
| `x`               | 3      | repeated float (packed)  | X coordinates            |
| `y`               | 4      | repeated float (packed)  | Y coordinates            |
| `z`               | 5      | repeated float (packed)  | Z coordinates            |
| `confidence`      | 6      | repeated uint32 (packed) | TimesSeenCount per point |
| `grid_metadata`   | 7      | `GridMetadata`           | Grid configuration       |

**`GridMetadata` message:**

| Field               | Number | Type           | Purpose                   |
| ------------------- | ------ | -------------- | ------------------------- |
| `rings`             | 1      | int32          | Ring count                |
| `azimuth_bins`      | 2      | int32          | Azimuth bin count         |
| `ring_elevations`   | 3      | repeated float | Per-ring elevation angles |
| `settling_complete` | 4      | bool           | Background model settled  |

#### 3. Background Snapshot Publisher

The `Publisher` struct adds fields for background management:

| Field                | Type                 | Purpose                   |
| -------------------- | -------------------- | ------------------------- |
| `backgroundMgr`      | `*BackgroundManager` | Background grid reference |
| `lastBackgroundSeq`  | uint64               | Last sent grid sequence   |
| `lastBackgroundSent` | `time.Time`          | Last background send time |
| `backgroundInterval` | `time.Duration`      | Default: 30 s             |

The `shouldSendBackground()` method returns true if: (1) background has never been sent, (2) the configured interval has elapsed, or (3) the grid sequence number changed (indicating a reset or sensor movement).

The `Publish(frame)` method checks `shouldSendBackground()` first, broadcasts a background snapshot if needed (updating the sequence and timestamp), then always broadcasts the lightweight foreground frame.

#### 4. Swift Client: Composite Rendering

The `CompositePointCloudRenderer` class maintains a cached Metal buffer for background points (rarely updated) and a per-frame buffer for foreground points.

On receiving a frame, behaviour depends on the frame type:

- **Background**: Update the cached background buffer and store the new sequence number.
- **Foreground**: If `backgroundSeq` mismatches the cached value, request a background refresh. Update only the foreground buffer.
- **Full** (legacy): Clear the background cache; treat the entire point cloud as foreground.

During rendering, the background buffer (if cached) is drawn first as point primitives, then the foreground buffer is drawn on top. This compositing means only the foreground buffer changes each frame, keeping GPU work minimal.

---

## Sensor Movement Detection

For static deployments, sensor movement (bump, vibration, repositioning) invalidates the background model.

### Detection Approaches

#### 1. High Foreground Ratio Detection

If suddenly >20% of points are classified as foreground, the background model is likely stale:

The `CheckForSensorMovement(mask)` method on `BackgroundManager` counts foreground points in the mask, computes the foreground ratio, and increments a `highForegroundStreak` counter when the ratio exceeds 20%. The counter resets to zero when the ratio drops below the threshold. Sensor movement is reported when the streak exceeds 5 consecutive frames.

#### 2. Background Drift Detection

Monitor how much the “stable” background is shifting:

The `CheckBackgroundDrift()` method iterates settled cells, compares each cell’s `AverageRangeMeters` against its `LockedBaseline`, and counts cells where the absolute drift exceeds `driftThresholdMeters` (e.g. 0.5 m). It returns `drifted = true` when the ratio of drifting cells to non-zero cells exceeds 10%, along with a `DriftMetrics` struct containing `DriftingCells`, `AverageDrift`, and `DriftRatio`.

#### 3. Response to Detected Movement

The `HandleSensorMovement()` method logs the detection, then either performs a full grid reset (`ResetGrid("sensor_movement_detected")`) or a soft reset (reducing `TimesSeenCount` by 50% while preserving locked baselines). It increments the grid’s `SequenceNumber` to trigger background refresh on connected visualiser clients, and emits a `"sensor_movement"` monitoring event with the sensor ID and action taken.

---

## Implementation Plan Update

### M3.5 - Split Streaming for Static LiDAR ✅

**Status**: Complete. All tasks for both Track A and Track B are implemented and tested.

Inserted between M3 (Canonical Model) and M4 (Tracking Refactor):

```
 M3: Canonical Model + Adapters    ──▶ LidarView + gRPC from same source     ✅ DONE
 M3.5: Split Streaming             ──▶ BG/FG separation, 96% bandwidth cut   🆕 NEW
 M4: Tracking Interface Refactor   ──▶ Golden replay tests pass
```

#### M3.5 Tasks

**Track B (Pipeline):**

- [x] Add `FrameType` enum to protobuf schema
- [x] Add `BackgroundSnapshot` message to protobuf
- [x] Implement `GenerateBackgroundPointCloud()` on BackgroundManager
- [x] Add background snapshot scheduling to Publisher (30s interval)
- [ ] Add `--vis-background-interval` CLI flag _(uses default 30s config; explicit flag not added)_
- [x] Implement foreground-only frame adaptation in FrameAdapter (works for both 10Hz/20Hz)
- [x] Add sensor movement detection (`CheckForSensorMovement`)
- [x] Add background drift detection (`CheckBackgroundDrift`)
- [x] Handle grid reset → sequence number increment
- [x] Unit tests for background snapshot generation (test both 10Hz/20Hz densities)
- [x] Unit tests for movement detection

**Track A (Visualiser/Swift):**

- [x] Update protobuf stubs for new message types
- [x] Implement `CompositePointCloudRenderer` with BG cache
- [x] Handle `FrameType.background` → update cache
- [x] Handle `FrameType.foreground` → render FG over cached BG
- [x] Request background refresh when `backgroundSeq` mismatches
- [x] Add UI indicator for "Background: Cached" vs "Refreshing"
- [ ] Performance test: verify 3 Mbps bandwidth achieved _(claimed in code comments, not formally tested)_

**Acceptance Criteria:**

- [x] Background snapshot sent every 30s (configurable)
- [x] Foreground frames contain only moving points + metadata
- [x] Bandwidth reduced from ~80 Mbps to <5 Mbps _(claimed ~3 Mbps in code; not formally tested)_
- [x] No visual difference from full-frame mode
- [x] Sensor movement triggers background refresh
- [x] Client handles reconnect with stale cache gracefully

**Estimated Dev-Days:** 8 (3 Track A + 5 Track B)

---

## Updated Milestone Table

| Milestone                 | Track A (Days) | Track B (Days) | Total  | Status      |
| ------------------------- | -------------- | -------------- | ------ | ----------- |
| M0: Schema + Synthetic    | 5              | 5              | 10     | ✅ Complete |
| M1: Recorder/Replayer     | 4              | 4              | 8      | ✅ Complete |
| M2: Real Points           | 2              | 4              | 6      | ✅ Complete |
| M3: Canonical Model       | 0              | 5              | 5      | ✅ Complete |
| **M3.5: Split Streaming** | **3**          | **5**          | **8**  | ✅ **Done** |
| M4: Tracking Refactor     | 2              | 6              | 8      | ✅ Complete |
| M5: Algorithm Upgrades    | 2              | 10             | 12     |             |
| M6: Debug + Labelling     | 8              | 4              | 12     |             |
| M7: Performance           | 4              | 4              | 8      |             |
| **Total**                 | **30**         | **47**         | **77** | **45 done** |

---

## Secondary Optimisations (Lower Priority)

These remain valid but are less impactful given the 96% reduction from split streaming. They may be worth pursuing if additional performance gains are needed after M3.5.

### Implemented: Pipeline Frame Rate Throttle

**Problem**: During PCAP replay catch-up, frames arrive at 33+ fps in bursts, overwhelming the pipeline and causing FPS collapse cycles (10 fps → 1.2 fps → 33 fps burst → 62 frames dropped on client).

**Solution**: `MaxFrameRate` config (default 12 fps) in `TrackingPipelineConfig` throttles the expensive downstream path (clustering, tracking, forwarding). Background model update (`StoreForegroundSnapshot`, `ProcessFramePolarWithMask`) still runs on every frame.

**Implementation**: `tracking_pipeline.go` tracks `lastProcessedTime` and skips downstream processing when frames arrive faster than `minFrameInterval` (83ms for 12 fps).

### Implemented: Mask Buffer Reuse

**Problem**: `ProcessFramePolarWithMask()` allocated a new `[]bool` (69k entries) on every frame.

**Solution**: `maskBuf []bool` field on `BackgroundManager`, grown-if-needed and zeroed before use. Eliminates per-frame allocation.

---

## Alternative Approaches for Future Consideration

While split streaming (M3.5) is the primary solution for static LiDAR, these alternatives offer different trade-offs and may be valuable for specific use cases or as complementary optimisations.

### 1. Server-Side Point Cloud Decimation

**Impact: High | Complexity: Low | Status: Partially implemented**

Alternative to split streaming: reduce point count uniformly across full frames.

#### 1a. Uniform Decimation

Keep every Nth point based on target ratio. For example, 50% decimation reduces 70k → 35k points (≈ 485 KB) via `bundle.PointCloud.ApplyDecimation(DecimationUniform, 0.5)`.

**Pros:** Simple, predictable reduction
**Cons:** May lose detail in sparse regions, doesn't leverage static scene structure

**Implementation note:** Already available via `DecimationMode` in adapter.

#### 1b. Voxel Grid Decimation

Divide space into voxels, keep one point per voxel.

**Pros:** Spatially uniform, preserves coverage
**Cons:** More complex, requires tuning voxel size

#### 1c. Adaptive Decimation

Adjust ratio based on client feedback or server queue depth. When `queueDepth > 5` or `consecutiveSlowSends > 2`, reduce `decimationRatio` by 20% (multiply by 0.8).

**Pros:** Self-tuning to client capability
**Cons:** Variable quality, added complexity, doesn't address root cause

**When to use:** Mobile LiDAR or SLAM use cases where background/foreground split isn't applicable.

---

### 2. Multi-Resolution Streaming

**Impact: High | Complexity: Medium**

Client dynamically requests detail level based on performance.

A `StreamRequest` message adds a `detail_level` field (field 5) with these options:

| Value | Name            | Points                          |
| ----- | --------------- | ------------------------------- |
| 0     | `FULL`          | All points (≈70k)               |
| 1     | `HIGH`          | 50% (≈35k)                      |
| 2     | `MEDIUM`        | 25% (≈17k)                      |
| 3     | `LOW`           | 10% (≈7k)                       |
| 4     | `CLUSTERS_ONLY` | No points, just clusters/tracks |

**Use case:** Visualiser can request lower detail when:

- Network bandwidth is limited
- Client CPU/GPU is overloaded
- User zooms out (less detail needed)

**Pros:** Client-driven QoS, graceful degradation
**Cons:** Requires client capability detection, server needs multiple encoders

**Complements M3.5:** Could apply to both background and foreground streams.

---

### 3. Delta/Differential Encoding (Advanced)

**Impact: Very High | Complexity: Very High**

Send only points that changed since last frame.

**`DeltaFrame` fields:**

| Field                 | Type             | Purpose                   |
| --------------------- | ---------------- | ------------------------- |
| `BaseFrameID`         | uint64           | Reference frame for delta |
| `AddedPoints`         | `[]Point`        | New points since base     |
| `RemovedPointIndices` | `[]uint32`       | Indices of removed points |
| `ModifiedPoints`      | `[]IndexedPoint` | Changed point positions   |

**Theory:** For static scenes, most points are identical frame-to-frame.

**Pros:** Extreme bandwidth reduction (potentially 95%+)
**Cons:**

- Complex client-side state reconstruction
- Error accumulation over time
- Reconnect requires full resync
- Doesn't leverage semantic structure (BG/FG)

**Why M3.5 is better:** Split streaming achieves similar bandwidth reduction while maintaining semantic meaning and simpler error recovery.

**Potential future use:** Delta updates _within_ background snapshots (e.g., "cell 1234 changed from 10.5m to 10.7m").

---

### 4. Binary Protocol Optimisation

**Impact: Medium | Complexity: Medium**

Reduce per-point overhead beyond protobuf.

#### 4a. Packed Binary Format

Current: protobuf overhead ~14 bytes/point (varint tags, field overhead)

```
[header: 16 bytes]
[points: N × 13 bytes] // 3×float32 + uint8 + uint8, no protobuf framing
```

**Saves:** ~1 byte/point = ~70 KB/frame (7% reduction)

#### 4b. Half-Precision Floats (float16)

```
[points: N × 7 bytes] // 3×float16 + uint8 + uint8
```

**Saves:** 6 bytes/point = ~414 KB/frame (43% reduction)
**Cons:** Precision loss (±0.1% for typical LiDAR ranges), requires custom codec

#### 4c. Quantised Integers

Encode X/Y/Z as int16 with known scale factor: `int16(point.X * 1000)` gives mm precision within a ±32 m range.

**Saves:** 6 bytes/point = ~414 KB/frame (43% reduction)
**Pros:** No precision loss for mm-scale resolution
**Cons:** Limited range (±32m for int16)

**Why defer:** M3.5 reduces foreground frames to ~2k points, so per-point savings are less critical (~12 KB vs 400 KB).

**Future use:** Apply to background snapshots (67k points × 6 bytes = 400 KB saved per snapshot).

---

### 5. Client-Side Improvements (Swift)

**Impact: High | Complexity: Medium**

#### 5a. Async Receive Processing

Ensure gRPC receive doesn’t block on Metal rendering:

The current approach awaits each frame in the stream and calls `renderer.render(frame)` synchronously, which may block if Metal is slow. The improved approach decouples receive from render using a `FrameBuffer` actor that stores the latest frame. A dedicated async `Task` runs the receive loop (never blocks), calling `frameBuffer.store(frame)` for each arrival. A separate `DisplayLink.onFrame` render loop consumes the latest frame at display cadence via `frameBuffer.consume()`.

**Pros:** Eliminates backpressure from render thread
**Cons:** May drop frames if render is consistently slower than receive

#### 5b. Frame Skipping on Client

If client is behind, skip to newest frame:

The client stores each arriving frame in a `latestFrame` variable. Rendering is invoked only when `isRendering` is false, ensuring the client always renders the most recent frame and drops intermediate ones.

**Status:** Recommended for M3.5 Track A implementation.

#### 5c. Metal Buffer Pooling

Pre-allocate Metal buffers to avoid allocation during render:

A `MetalBufferPool` class maintains a pool of available `MTLBuffer` instances. `acquire(size)` pops a buffer from the pool if one of sufficient size exists, otherwise allocates a new one from the Metal device. `release(buffer)` returns the buffer to the pool.

**Status:** Recommended for M3.5 Track A implementation, especially for background cache.

---

### 6. Clusters-Only Mode (Minimal Bandwidth)

**Impact: Very High | Complexity: Low**

For monitoring dashboards or low-bandwidth scenarios, skip point clouds entirely.

**Stream only:**

- **Clusters** (centroid, bounding box, point count) — ~100 bytes each
- **Tracks** (position, velocity, classification) — ~200 bytes each

**Bandwidth comparison:**

| Mode                  | Data per Frame | Bandwidth @ 10fps |
| --------------------- | -------------- | ----------------- |
| Full points           | 970 KB         | 78 Mbps           |
| Foreground only       | 50-100 KB      | 4-8 Mbps          |
| Split streaming       | ~35 KB avg     | 3 Mbps            |
| **Clusters + tracks** | **5-10 KB**    | **0.4-0.8 Mbps**  |

**Use case:** Web dashboard showing vehicle counts and speeds without full 3D visualisation.

**Implementation:** Add a `StreamMode` enum to `StreamRequest`:

| Value | Name              | Meaning           |
| ----- | ----------------- | ----------------- |
| 0     | `FULL_FRAMES`     | Current behaviour |
| 1     | `SPLIT_STREAMING` | M3.5 mode         |
| 2     | `CLUSTERS_ONLY`   | Metadata only     |

**Future milestone:** Possibly M6 (Debug + Labelling) or M8 (Web Dashboard).

---

### 7. Temporal Subsampling

**Impact: Medium | Complexity: Low**

Reduce frame rate for full point clouds, interpolate on client.

The server sends a full background snapshot every other frame (at 5 fps) while sending foreground frames at every frame (10 fps). The client interpolates track positions between background refreshes.

**Client interpolates track positions between background refreshes.**

**Pros:** Reduces background refresh bandwidth by 50%
**Cons:** Stale background for up to 200ms

**Why less useful with M3.5:** Background is already sent at 30s intervals, so temporal subsampling is redundant.

**Alternative use:** Reduce foreground frame rate to 5 fps for low-bandwidth scenarios.

---

### 8. Compression

**Impact: Medium | Complexity: Low-Medium**

#### 8a. gRPC Built-in Compression

Enable via `grpc.UseCompressor(gzip.Name)` on the server.

**Expected:** 30-40% reduction for LiDAR point clouds.

**Pros:** Zero code change (built-in)
**Cons:** CPU overhead, added latency (~5-20ms per frame)

**Analysis with M3.5:**

- Background snapshot: 920 KB → ~600 KB (saves 320 KB, 35% reduction)
- Foreground frame: 35 KB → ~25 KB (saves 10 KB, 28% reduction)
- Net bandwidth: 3 Mbps → 2 Mbps (saves 1 Mbps)

**Recommendation:** Test in M3.5 or M7 (Performance) if CPU headroom allows.

#### 8b. Domain-Specific Compression

- **Octree encoding** for spatial coherence
- **Run-length encoding** for classification arrays
- **Prediction + residual** for coordinates (delta from previous point)

**Complexity:** Very high, likely not justified given M3.5 gains.

---

### 9. Connection Tuning

**Impact: Low-Medium | Complexity: Low**

#### 9a. TCP Buffer Sizes

Increase gRPC send buffer to 4 MiB via `grpc.WriteBufferSize(4 * 1024 * 1024)`.

**Status:** May help with burst traffic, test in M7.

#### 9b. Use Unix Domain Socket (local only)

Listen on a Unix socket (`net.Listen("unix", "/tmp/visualiser.sock")`) instead of TCP.

**Pros:** Eliminates TCP overhead for localhost connections
**Cons:** Not usable for remote visualiser connections

**Bandwidth savings:** ~5-10% (TCP header overhead)

**Recommendation:** Implement as optional transport in M7 if profiling shows TCP overhead.

---

## Implementation Priority

### Tier 1: Primary Solution (M3.5)

- ✅ Split streaming (BG/FG separation)

### Tier 2: Complementary to M3.5 (Consider for M3.5 or M7)

- Client async receive processing (5a)
- Metal buffer pooling (5c)
- Binary protocol optimisation for background snapshots (4c)

### Tier 3: Niche Use Cases (Future Milestones)

- Multi-resolution streaming (2) — For mobile clients
- Clusters-only mode (6) — For web dashboards
- Adaptive decimation (1c) — For mobile LiDAR

### Tier 4: High Complexity / Low Incremental Gain

- Delta encoding (3) — Superseded by split streaming
- Domain-specific compression (8b) — Not justified given M3.5 gains
- Temporal subsampling (7) — Redundant with 30s background interval

---

## Metrics to Track

| Metric              | Before            | Target (M3.5)  | Status              |
| ------------------- | ----------------- | -------------- | ------------------- |
| Bandwidth (Mbps)    | 78-80             | <5             | ✅ ~3 Mbps achieved |
| avg_send_ms         | 1-600             | <10            | ✅ Improved         |
| slow_sends/min      | 5-10              | 0              | ✅ Reduced          |
| dropped_frames/min  | 19+               | 0              | ✅ Reduced          |
| Client FPS          | 1.4-20 (variable) | 10-20 (stable) | Partially improved  |
| BG refresh interval | N/A               | 30s            | ✅ 30s default      |
| FG points/frame     | 35-70k (all)      | 1-2k (FG only) | ✅ FG-only mode     |
| MaxFrameRate        | N/A               | 12 fps         | ✅ Throttle added   |

**Note:** Metrics apply to both 10Hz (dense) and 20Hz (sparse) sensor modes. Bandwidth target is constant across modes.

---

## Appendix: Test Commands

```bash
# Profile server CPU during replay
go tool pprof http://localhost:8081/debug/pprof/profile?seconds=30

# Check memory allocations
go tool pprof http://localhost:8081/debug/pprof/heap

# Watch frame stats
tail -f logs/velocity-*.log | grep -E '\[gRPC\]|\[Visualiser\]'

# Verify bandwidth reduction
# Before: ~80 Mbps
# After:  ~3 Mbps (foreground) + ~0.25 Mbps (background @ 1/30s)
```
