# LiDAR Foreground Tracking & Export Status

**Last Updated:** January 4, 2026
**Status:** Active Development — Core Pipeline Working
**Consolidates:** `lidar-tracking-enhancements.md`, `foreground-track-export-investigation-plan.md`, `port-2370-corruption-diagnosis.md`, `port-2370-foreground-streaming.md`

## Executive Summary

This document serves as the single source of truth for the ongoing LiDAR foreground tracking implementation and debugging efforts.

**Current Working Features:**

1.  ✅ **Foreground Feed (Port 2370):** Working — foreground points visible in LidarView, distinct from background.
2.  ✅ **Real-time Parameter Tuning:** Working — params can be edited via JSON textarea in status UI without server restart.
3.  ✅ **Background Subtraction:** Working — points correctly masked as foreground/background.

**Current Issues Under Investigation:**

1.  🚧 **Performance/Framerate:** Foreground feed on 2370 may have framerate/latency issues; CPU usage on M1 Mac is higher than expected.
2.  🚧 **Foreground Trails:** Points that should instantly settle back to background after an object passes are lingering as foreground for multiple frames.

**Implementation Status:**

- ✅ **Phase 3.7 (Analysis Run Infrastructure):** Completed.
- ✅ **Port 2370 Foreground Streaming:** Working (fixed packet reconstruction with RawBlockAzimuth preservation).
- 🚧 **Performance Optimization:** Active investigation.
- 🚧 **Trail Artifact Investigation:** Active investigation (see §2 below).

---

## 1. Resolved Issues

### Issue 1: Packet Corruption on Port 2370 — ✅ FIXED

**Symptom:** LidarView showed sparse rings and patchy arcs.
**Root Cause:** Forwarder was reconstructing packets with incorrect azimuth values due to binning logic that destroyed original packet timing.
**Fix Applied:** Rewrote `ForegroundForwarder` to preserve `RawBlockAzimuth` and `UDPSequence` from original packets. Points are now grouped by original packet structure rather than azimuth bins.
**Status:** Verified working — foreground points display correctly in LidarView on port 2370.

### Issue 2: Real-time Parameter Tuning — ✅ IMPLEMENTED

**Feature:** JSON textarea in status page allows editing all background params without server restart.
**Implementation:** POST form submission to `/api/lidar/params` with JSON body. Server applies changes immediately via `SetParams()` and redirects back.
**Status:** Verified working — changes take effect immediately.

---

## 2. Active Investigation: Foreground "Trails"

### Problem Statement

After an object (e.g., vehicle) passes through the sensor field of view, points in the cells it occupied are **still classified as foreground for several frames** instead of immediately returning to background.

**Expected Behavior:** When an object leaves a cell, the next observation should match the settled background average and be classified as background.

**Observed Behavior:** "Trails" of foreground points linger behind moving objects.

### Root Cause Analysis

The background masking algorithm in `ProcessFramePolarWithMask()` classifies a point as **foreground** when:

```go
cellDiff := math.Abs(float64(cell.AverageRangeMeters) - p.Distance)
closenessThreshold := closenessMultiplier*(float64(cell.RangeSpreadMeters)+noiseRel*p.Distance+0.01) + safety
isBackgroundLike := cellDiff <= closenessThreshold || neighborConfirmCount >= neighConfirm
```

**Potential Causes for Trail Artifacts:**

1.  **EMA Drift During Object Presence:**

    - While object is in cell, the object's distance is _rejected_ as foreground
    - Each rejection **decrements `TimesSeenCount`** (line 219-223 in foreground.go)
    - If count hits 0, cell is considered "empty" and `nonzeroCellCount` decrements
    - When object leaves, the cell may have reduced confidence or corrupted average

2.  **Cell Freeze Mechanism:**

    - Large deviations trigger a freeze: `cell.FrozenUntilUnixNanos = nowNanos + freezeDur`
    - **Frozen cells are always classified as foreground** (line 141-145 in foreground.go)
    - Default `FreezeDurationNanos` may be too long

3.  **Spread Inflation:**

    - If object was partially classified as background (e.g., via neighbor confirmation), it may have **inflated `RangeSpreadMeters`**
    - Larger spread → larger closeness threshold → harder for true background to match

4.  **Post-Object Average Shift:**
    - If EMA was updated during object presence, `AverageRangeMeters` may have shifted toward object distance
    - True background distance now exceeds threshold

### Debugging Strategy: Region Tracking

To diagnose which mechanism causes trails, we need to **track a specific cell's state over time**.

**Proposed Implementation:**

```go
// Add to BackgroundManager or as debug flag
type CellDebugConfig struct {
    Enabled  bool
    Ring     int  // Target ring to monitor (e.g., 20)
    AzBinMin int  // Azimuth bin range start (e.g., 450)
    AzBinMax int  // Azimuth bin range end (e.g., 460)
}

// In ProcessFramePolarWithMask, after classification:
if debugCfg.Enabled && ring >= debugCfg.Ring-1 && ring <= debugCfg.Ring+1 &&
   azBin >= debugCfg.AzBinMin && azBin <= debugCfg.AzBinMax {
    log.Printf("[CellDebug] ring=%d azbin=%d obs=%.3f avg=%.3f spread=%.3f seen=%d frozen=%v diff=%.3f thresh=%.3f isFg=%v",
        ring, azBin, p.Distance, cell.AverageRangeMeters, cell.RangeSpreadMeters,
        cell.TimesSeenCount, cell.FrozenUntilUnixNanos > nowNanos,
        cellDiff, closenessThreshold, foregroundMask[i])
}
```

**What to Look For:**

| Log Pattern                             | Indicates                         |
| --------------------------------------- | --------------------------------- |
| `seen=0` after object passes            | TimesSeenCount drained to zero    |
| `frozen=true` lingering                 | FreezeDuration too long           |
| `spread` much larger than before object | EMA spread inflation              |
| `avg` shifted toward object distance    | Background contaminated by object |
| `thresh` >> `diff` but still foreground | Neighbor confirmation failing     |

### Recommended Fixes (Ordered by Likelihood)

1.  **Reduce FreezeDuration:** Change from default (e.g., 500ms) to 100ms or disable entirely for this debugging phase.

2.  **Protect Background During Foreground:**

    ```go
    // Don't decrement TimesSeenCount below a minimum during foreground classification
    if cell.TimesSeenCount > 3 { // Keep minimum confidence
        cell.TimesSeenCount--
    }
    ```

3.  **Fast Re-acquisition:** When a cell goes from foreground back to matching background, boost its confidence faster than normal EMA.

4.  **Separate Spread Tracking:** Track background spread separately from observations during foreground periods.

---

## 3. Active Investigation: Performance/CPU

### Problem Statement

On macOS M1, the foreground processing pipeline shows higher CPU usage than expected. The foreground feed may have reduced framerate or latency compared to the raw 2368 stream.

### Metrics to Collect

- **CPU %** during foreground processing (Activity Monitor or `top`)
- **Frame latency**: Time from packet arrival to foreground packet emission
- **Point throughput**: Points/sec processed vs. points/sec forwarded
- **GC pressure**: `runtime.ReadMemStats` to check allocation rate

### Potential Causes

1.  **Per-frame allocations**: If `ProcessFramePolarWithMask` allocates large slices per frame, GC pressure increases.
2.  **Lock contention**: Background grid access may have mutex contention if multiple goroutines access it.
3.  **Unoptimized neighbor lookup**: Neighbor confirmation iterates neighboring cells; may be slow for dense grids.
4.  **Packet encoding overhead**: `ForegroundForwarder` may be inefficiently encoding/copying data.

### Profiling Plan

1.  **CPU Profile**: Run with `go tool pprof` during live capture to identify hot functions.
2.  **Trace Profile**: Use `runtime/trace` to visualize goroutine scheduling and GC pauses.
3.  **Memory Profile**: Check for per-frame allocations that could be pooled.

**Status:** Not yet investigated. Priority after trails bug is resolved.

---

## 4. Future Enhancements (Roadmap)

### Alternative Algorithm: Velocity-Coherent Foreground Extraction

**Design Document:** [velocity-coherent-foreground-extraction.md](velocity-coherent-foreground-extraction.md)

A new approach to address the limitations of background-subtraction:

- **Velocity-based clustering**: Associate points by kinematic coherence (6D DBSCAN)
- **Long-tail tracking**: Capture pre-entry and post-exit phases via velocity prediction
- **Sparse continuation**: Maintain track identity with as few as 3 points
- **Track merging**: Reconnect fragments split by occlusion or parameter sensitivity

**Key Innovation:** Reduce MinPts from 12 to 3 by using velocity coherence as confirmation signal, matching human visual perception capability.

### Phase 2: Training Data Preparation

- **Track Quality Metrics:** Add `OcclusionCount`, `SpatialCoverage`, `NoisePointRatio` to `TrackedObject`.
- **Training Data Filter:** Filter tracks by quality score (duration > 2s, length > 5m).
- **Export:** Generate labeled PCAP snippets for high-quality tracks.

### Phase 3: Advanced Introspection

- **Dashboard:** Real-time charts for track quality and parameter sensitivity.
- **Sensitivity Analysis:** Automated parameter sweeps (varying `Eps`, `MinPts`) to optimize tracking.

### Phase 4: Split/Merge Correction

- **Detection:** Heuristics for spatial proximity and kinematic continuity to detect split tracks.
- **Correction UI:** Web interface to manually merge split tracks.

---

## 4. Simplification Decisions (3DOF/Bicycle Model)

**Date:** January 2026
**Context:** Review of over-engineered plans vs. practical implementation needs

### What Was Simplified

The implementation intentionally uses a **simpler 2D + velocity model** rather than the full 7DOF model described in `static-pose-alignment-plan.md` and `av-lidar-integration-plan.md`. This is the correct choice for traffic monitoring.

| Original Plan                   | Simplified Implementation                | Rationale                                  |
| ------------------------------- | ---------------------------------------- | ------------------------------------------ |
| 7DOF bounding box               | 2D + axis-aligned bbox                   | Ground-plane assumption valid for roadside |
| 6-state Kalman [x,y,z,vx,vy,vz] | EMA smoothing (x,y,vx,vy)                | Simpler, sufficient for tracking           |
| Heading via PCA                 | Heading from velocity atan2(vy,vx)       | Moving objects already have velocity       |
| Z-axis tracking                 | Single HeightP95Max stat                 | Vertical extent, not position tracking     |
| 28-class AV taxonomy            | 4 classes (car, pedestrian, bird, other) | Traffic monitoring needs                   |
| Oriented bounding boxes         | Axis-aligned boxes                       | Sufficient for counting/speed              |
| Shape completion (occlusion)    | Not implemented                          | AV-specific feature                        |
| Parquet/NLZ ingestion           | Not implemented                          | AV dataset feature                         |

### Deferred to "AV Integration" Phase

The following features from `av-lidar-integration-plan.md` are explicitly **out of scope** for traffic monitoring and should only be implemented if AV dataset integration is required:

- **Phase 6-7**: Clustering algorithms for occlusion, shape completion
- **28-class taxonomy**: Only needed for AV labeling compatibility
- **Parquet ingestion**: Only for importing AV training datasets
- **NLZ (No Label Zone)**: AV annotation concept

### Recommended Object Classes (Traffic Monitoring)

```go
// 6-class taxonomy (practical minimum)
const (
    ClassCar        = "car"        // P0: Core
    ClassTruck      = "truck"      // P0: Core (can split from car by size)
    ClassPedestrian = "pedestrian" // P0: Core
    ClassCyclist    = "cyclist"    // P1: Safety-relevant
    ClassBird       = "bird"       // P1: Filter false positives
    ClassOther      = "other"      // Catch-all
)
```

### Why Not Full 7DOF?

1. **Ground-plane assumption is valid**: Roadside sensors view objects from fixed position; vehicles travel on road surface
2. **Heading from velocity is sufficient**: atan2(vy, vx) gives heading for moving objects; stationary objects don't need heading
3. **Axis-aligned boxes work**: For counting and speed measurement, precise orientation isn't needed
4. **Complexity cost**: 7DOF requires PCA computation, oriented box fitting, and additional Kalman states

### Related Documents

- `static-pose-alignment-plan.md` - Full 7DOF plan (DEFERRED for AV integration)
- `av-lidar-integration-plan.md` - AV dataset compatibility (DEFERRED)
- `velocity-coherent-foreground-extraction.md` - Current implementation design

---

## 5. Troubleshooting Guide (Port 2370)

**Checklist if Port 2370 is silent:**

1.  **BackgroundManager:** Must be initialized and passed to `RealtimeReplayConfig`.
    - _Check:_ Logs should show "Foreground extraction: X/Y points".
2.  **ForegroundForwarder:** Must be created and `Start()` called.
    - _Check:_ Logs should show "Foreground forwarding started to ...:2370".
3.  **Build Tags:** Binary must be built with `-tags pcap`.
4.  **Firewall:** Ensure UDP port 2370 is open.
5.  **Data Density:** If logs show "0/X points (0%)", background parameters are too aggressive (see Fix Phase 3).

**Verification Commands:**

```bash
# Monitor traffic
sudo tcpdump -i any -n udp port 2370 -c 100

# Check listen status
sudo netstat -ulpn | grep 2370
```
