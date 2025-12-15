# LiDAR Foreground Tracking & Export Status

**Last Updated:** December 15, 2025
**Status:** Active Investigation & Implementation
**Consolidates:** `lidar-tracking-enhancements.md`, `foreground-track-export-investigation-plan.md`, `port-2370-corruption-diagnosis.md`, `port-2370-foreground-streaming.md`

## Executive Summary

This document serves as the single source of truth for the ongoing investigation into LiDAR foreground tracking, export corruption, and frame ordering issues. It consolidates root cause analysis, fix plans, and future enhancement roadmaps.

**Current Critical Issues:**

1.  **Packet Corruption (Port 2370):** Foreground stream packets are corrupt due to invalid BlockID/Channel mapping.
2.  **Frame Export Ordering:** `export_frame_sequence` returns frames out of order due to non-deterministic map iteration.
3.  **Low Foreground Count:** Background subtraction is too aggressive during PCAP replay due to lack of "warmup" and strict parameters.

**Implementation Status:**

- ✅ **Phase 3.7 (Analysis Run Infrastructure):** Completed.
- 🚧 **Phase 4.0 (Track Labeling UI):** In Progress.
- 🛑 **Foreground Export Fixes:** Pending (See Plan below).

---

## 1. Problem Statements & Root Cause Analysis

### Issue 1: Packet Corruption on Port 2370

**Symptom:** LidarView shows sparse rings and patchy arcs; tcpdump shows packets with mostly empty blocks.
**Root Cause:**

- **BlockID Mismatch:** `ForegroundForwarder` uses `PointPolar.BlockID` from the _original_ packet. When points are filtered, they retain original BlockIDs (e.g., Block 9). The encoder tries to fill blocks sequentially (0-9). If Block 0 has no points, it writes an empty block, even if points exist for Block 9.
- **Channel Mismatch:** Channel matching assumes BlockID correspondence, leading to further data loss.
- **Motor Speed Encoding:** Incorrectly encoded as `RPM * 60` instead of `RPM * 100` (0.01 RPM units).

### Issue 2: Out-of-Order Frame Export

**Symptom:** `export_frame_sequence` produces files like `frame_02.asc`, `frame_01.asc`, `frame_03.asc`.
**Root Cause:**

- **Map Iteration:** `FrameBuilder` stores frames in `map[string]*LiDARFrame`. Go map iteration is randomized. The finalization logic iterates this map, causing frames to be processed and exported in random order despite correct timestamps.

### Issue 3: Low Foreground Point Count (PCAP Replay)

**Symptom:** Foreground ratio is ~1.2% (expected 15-40%).
**Root Cause:**

- **No Warmup:** PCAP replay starts immediately. The background model is empty, so `SeedFromFirstObservation` (if false) or initial convergence takes time.
- **Aggressive Parameters:** Default `ClosenessSensitivityMultiplier=3.0` and `NeighborConfirmationCount=3` are too strict for the replay data.
- **Distance Overflow:** Points >130m cause `uint16` overflow in distance encoding.

---

## 2. Implementation Plan (Fixes)

### Phase 1: Frame Ordering Fix (URGENT)

**Goal:** Ensure deterministic export order.
**Files:** `internal/lidar/frame_builder.go`
**Tasks:**

1.  Modify `finalizeOldFrames` (or equivalent cleanup logic).
2.  Collect frames to finalize into a slice.
3.  Sort slice by `StartTimestamp`.
4.  Process sorted frames sequentially.

### Phase 2: Foreground Encoding & Corruption Fix (HIGH)

**Goal:** Fix Port 2370 stream and ASC export data integrity.
**Files:** `internal/lidar/network/foreground_forwarder.go`, `internal/lidar/track_export.go`
**Tasks:**

1.  **Rewrite Encoder:** Abandon `BlockID` reliance. Sort points by azimuth. Distribute into 10 azimuth buckets (36° each).
2.  **Fix Motor Speed:** Change encoding to `uint16(math.Round(RPM * 100))`.
3.  **Distance Clamping:** Clamp distances >130m to `0xFFFE`. Use `0xFFFF` only for no-return.
4.  **Validation:** Log warning if >50% of blocks are empty.

### Phase 3: Tuning & Replay Improvements (MEDIUM)

**Goal:** Restore foreground point density.
**Files:** `cmd/tools/pcap-analyze/main.go`, `internal/lidar/network/pcap_realtime.go`
**Tasks:**

1.  **Enable Seeding:** Set `SeedFromFirstObservation = true` for PCAP replay.
2.  **Relax Parameters:**
    - `ClosenessSensitivityMultiplier`: 3.0 → 2.0
    - `NeighborConfirmationCount`: 3 → 5
    - `SafetyMarginMeters`: 0.5 → 0.3
3.  **Warmup:** Process first 50-100 frames _without_ forwarding to seed the background grid.

### Phase 4: Track Point Cloud Export (FUTURE)

**Goal:** Export isolated tracks for ML training.
**Files:** `internal/lidar/track_point_cache.go`, `internal/lidar/monitor/webserver.go`
**Tasks:**

1.  Implement `TrackPointCache` to store polar points per track.
2.  Add REST endpoint `/api/lidar/export_track` to dump specific track points to ASC/PCAP.

---

## 3. Future Enhancements (Roadmap)

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

## 4. Troubleshooting Guide (Port 2370)

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
