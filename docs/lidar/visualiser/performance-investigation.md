# LiDAR Visualiser Performance Investigation

**Status:** Active Investigation
**Date:** 2026-02-04
**Authors:** David, Copilot

## Problem Summary

During PCAP replay at 10 fps with ~69k points/frame, the gRPC streaming pipeline experiences periodic slowdowns causing:

- **SLOW SEND warnings** (>50ms, up to 600ms observed)
- **Frame drops** (19+ frames dropped per session)
- **FPS collapse** from 10 fps → 1.4-3 fps during slowdowns
- **High CPU** on Go server (230%+), kernel_task (156%), WindowServer (100%+)

## Current Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  PCAP Replay    │────▶│   Go Server      │────▶│ Swift Client    │
│  (10 fps)       │     │  (gRPC stream)   │     │ (Metal render)  │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                              │
                              ▼
                        ~970 KB/frame
                        ~80 Mbps sustained
```

## Observed Metrics

| Metric         | Normal  | During Slowdown     |
| -------------- | ------- | ------------------- |
| FPS            | 10      | 1.4-3               |
| avg_send_ms    | 1.1-1.8 | 35-56 (peak 600)    |
| bandwidth_mbps | 78-80   | 10-24               |
| msg_size_kb    | 950-975 | 950-975 (unchanged) |
| avg_adapt_ms   | 0.5-0.7 | 7.9-10.3            |

**Key observation:** Message size is constant—the bottleneck is client receive throughput, not serialisation.

## Root Cause Analysis

### Primary Cause: Client Backpressure

When the Swift client's Metal rendering or AsyncStream processing can't keep up:

1. TCP receive buffer fills
2. TCP flow control kicks in (window shrinks)
3. `stream.Send()` blocks waiting for ACKs
4. Go server goroutine stalls
5. Frame queue fills → drops

### Contributing Factors

1. **Large message size** (~1 MB per frame)
2. **No client-side decimation** (full point cloud sent)
3. **Synchronous Metal rendering** may block receive thread
4. **GC pressure** from 69k × 5 slice allocations per frame (partially addressed with sync.Pool)

---

## Potential Fixes

### 1. Server-Side Point Cloud Decimation

**Impact: High | Complexity: Low**

Reduce point count before transmission. Options:

#### 1a. Uniform Decimation

Keep every Nth point based on target ratio.

```go
// Example: 50% decimation = 69k → 35k points = ~485 KB
bundle.PointCloud.ApplyDecimation(DecimationUniform, 0.5)
```

**Pros:** Simple, predictable reduction
**Cons:** May lose detail in sparse regions

#### 1b. Foreground-Only Mode

Send only foreground (moving object) points.

```go
bundle.PointCloud.ApplyDecimation(DecimationForegroundOnly, 0)
```

**Pros:** Massive reduction (69k → 500-2000 points typically)
**Cons:** Loses static scene context

#### 1c. Voxel Grid Decimation

Divide space into voxels, keep one point per voxel.

**Pros:** Spatially uniform, preserves coverage
**Cons:** More complex, requires tuning voxel size

#### 1d. Adaptive Decimation

Adjust ratio based on client feedback or queue depth.

```go
if queueDepth > 5 || consecutiveSlowSends > 2 {
    decimationRatio *= 0.8 // Reduce points by 20%
}
```

**Pros:** Self-tuning to client capability
**Cons:** Variable quality, complexity

### 2. Multi-Resolution Streaming

**Impact: High | Complexity: Medium**

Stream at multiple detail levels:

```protobuf
message StreamRequest {
  DetailLevel detail_level = 5;
  enum DetailLevel {
    FULL = 0;        // All points (~69k)
    HIGH = 1;        // 50% (~35k)
    MEDIUM = 2;      // 25% (~17k)
    LOW = 3;         // 10% (~7k)
    CLUSTERS_ONLY = 4; // No points, just clusters/tracks
  }
}
```

**Client can dynamically request lower detail when struggling.**

### 3. Delta/Differential Encoding

**Impact: Medium | Complexity: High**

Only send points that changed since last frame.

For static LiDAR:

- Background points rarely change
- Only foreground (moving) points need full updates
- Could reduce bandwidth by 90%+ for static scenes

**Implementation:**

```go
type DeltaFrame struct {
    BaseFrameID uint64
    AddedPoints []Point
    RemovedPointIndices []uint32
    ModifiedPoints []IndexedPoint
}
```

**Cons:** Requires client-side state reconstruction, complexity, error accumulation

### 4. Binary Protocol Optimisation

**Impact: Medium | Complexity: Medium**

Current protobuf overhead: ~14 bytes/point (X, Y, Z as float32 + intensity + classification + varint overhead)

#### 4a. Packed Binary Format

```
[header: 16 bytes]
[points: N × 13 bytes] // 3×float32 + uint8 + uint8, no protobuf framing
```

Saves ~1 byte/point = ~69 KB/frame (7% reduction)

#### 4b. Half-Precision Floats (float16)

```
[points: N × 7 bytes] // 3×float16 + uint8 + uint8
```

Saves 6 bytes/point = ~414 KB/frame (43% reduction)

**Cons:** Precision loss (±0.1% for typical LiDAR ranges)

#### 4c. Quantised Integers

Encode X/Y/Z as int16 with known scale factor:

```
int16 x = (int16)(point.X * 1000) // mm precision, ±32m range
```

Saves 6 bytes/point = ~414 KB/frame

### 5. Client-Side Improvements (Swift)

**Impact: High | Complexity: Medium**

#### 5a. Async Receive Processing

Ensure gRPC receive doesn't block on Metal rendering:

```swift
// Current (potentially blocking)
for try await frame in stream {
    await renderer.render(frame) // Blocks if Metal is slow
}

// Improved (decoupled)
for try await frame in stream {
    frameBuffer.store(frame) // Non-blocking
}

// Separate render loop
while true {
    if let frame = frameBuffer.latest {
        renderer.render(frame)
    }
}
```

#### 5b. Frame Skipping on Client

If client is behind, skip to newest frame:

```swift
var latestFrame: FrameBundle?
for try await frame in stream {
    latestFrame = frame
    if !isRendering {
        render(latestFrame)
    }
}
```

#### 5c. Metal Buffer Pooling

Pre-allocate Metal buffers to avoid allocation during render:

```swift
let bufferPool = MetalBufferPool(device: device, maxPoints: 100_000, poolSize: 3)

func render(frame: FrameBundle) {
    let buffer = bufferPool.acquire()
    defer { bufferPool.release(buffer) }
    // ...
}
```

### 6. Multi-Pass Object Identification

**Impact: High | Complexity: High**

Instead of streaming raw points, stream processed results:

#### Pass 1: Background Learning (Server)

- Build statistical background model
- Identify static vs dynamic regions
- Store in `lidar_bg_snapshot`

#### Pass 2: Foreground Extraction (Server)

- Extract moving objects only
- Cluster into objects
- Track over time

#### Pass 3: Object Streaming (to Client)

Stream only:

- **Clusters** (centroid, bounding box, point count) — ~100 bytes each
- **Tracks** (position, velocity, classification) — ~200 bytes each
- **Optional:** Representative points per cluster (~100 points/cluster)

**Bandwidth comparison:**
| Mode | Data per Frame | Bandwidth @ 10fps |
|------|----------------|-------------------|
| Full points | 970 KB | 78 Mbps |
| Foreground only | 50-100 KB | 4-8 Mbps |
| Clusters + tracks | 5-10 KB | 0.4-0.8 Mbps |

### 7. Temporal Subsampling

**Impact: Medium | Complexity: Low**

Reduce frame rate for full point clouds, interpolate on client:

```go
// Server: Send full frames at 5 fps, not 10 fps
if frameID % 2 == 0 {
    sendFullFrame(frame)
} else {
    sendClustersOnly(frame) // Lightweight update
}
```

Client interpolates positions between full frames.

### 8. Compression

**Impact: Medium | Complexity: Medium**

#### 8a. gRPC Built-in Compression

```go
grpc.UseCompressor(gzip.Name)
```

Typical LiDAR point clouds compress ~30-40% with gzip.

**Cons:** CPU overhead on both ends, latency increase

#### 8b. Domain-Specific Compression

- Octree encoding for spatial coherence
- Run-length encoding for classification arrays
- Prediction + residual for coordinates

### 9. Connection Tuning

**Impact: Low-Medium | Complexity: Low**

#### 9a. TCP Buffer Sizes

```go
// Increase gRPC send buffer
grpc.WriteBufferSize(4 * 1024 * 1024) // 4 MB
```

#### 9b. Disable Nagle's Algorithm

Already disabled by gRPC for streaming.

#### 9c. Use Unix Domain Socket (local only)

```go
listener, _ := net.Listen("unix", "/tmp/visualiser.sock")
```

Eliminates TCP overhead for localhost connections.

---

## Recommended Implementation Order

### Phase 1: Quick Wins (This Week)

1. **Add decimation CLI flag** — `--lidar-vis-decimation=0.5`
2. **Client frame skipping** — Skip to latest if behind
3. **Verify async receive** — Ensure Metal doesn't block gRPC

### Phase 2: Protocol Improvements (Next Sprint)

4. **Multi-resolution streaming** — Client requests detail level
5. **gRPC compression** — Enable gzip for 30% reduction
6. **Quantised coordinates** — int16 for 43% reduction

### Phase 3: Architecture Changes (Future)

7. **Clusters-only mode** — For monitoring dashboards
8. **Delta encoding** — For minimal bandwidth
9. **Multi-pass pipeline** — Full server-side processing

---

## Metrics to Track

| Metric                    | Target    | Current         |
| ------------------------- | --------- | --------------- |
| avg_send_ms               | <10ms     | 1-600ms         |
| slow_sends per minute     | 0         | 5-10            |
| dropped_frames per minute | 0         | 19+             |
| bandwidth_mbps            | <40       | 78-80           |
| client FPS                | 10 stable | 1.4-10 variable |

## Appendix: Test Commands

```bash
# Profile server CPU during replay
go tool pprof http://localhost:8081/debug/pprof/profile?seconds=30

# Check memory allocations
go tool pprof http://localhost:8081/debug/pprof/heap

# Watch frame stats
tail -f logs/velocity-*.log | grep -E '\[gRPC\]|\[Visualiser\]'
```
