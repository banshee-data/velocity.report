# VRLOG Run Comparison Report — 2026-03-10

**Branch:** `dd/vrlog-2`
**Source PCAP:** `kirk1.pcapng` (Hesai Pandar40P, ~178s, ~2012 frames)
**Tuning:** identical across all runs (`0ff580...` hash)
**Runs compared:** 5 analysis runs at different playback speeds

## 1. Run Summary

| Run        | Format | Speed | Frames | Confirmed | Tentative | Deleted | Total | Frag  |
| ---------- | ------ | ----- | ------ | --------- | --------- | ------- | ----- | ----- |
| `54ba7296` | 1.0    | n/a   | 2012   | 15        | 194       | 235     | 444   | 0.437 |
| `60a4774c` | 0.5    | 0.1x  | 1832   | 6         | 154       | 200     | 360   | 0.428 |
| `d8f87151` | 0.5    | 0.5x  | 1434   | 5         | 132       | 167     | 304   | 0.434 |
| `8802b52b` | 0.5    | 1.0x  | 743    | 4         | 101       | 127     | 232   | 0.435 |
| `8099c80b` | 0.5    | 1.0x  | 349    | 3         | 64        | 54      | 121   | 0.529 |

## 2. Critical Findings

### 2.1 Frame dropping scales with playback speed

Higher playback speed means fewer frames make it into the VRLOG:

| Speed | Frames | % of source |
| ----- | ------ | ----------- |
| n/a   | 2012   | 100%        |
| 0.1x  | 1832   | 91%         |
| 0.5x  | 1434   | 71%         |
| 1.0x  | 743    | 37%         |
| 1.0x  | 349    | 17%         |

The frame pipeline cannot keep up with real-time playback. Even at 0.1x,
9% of frames are lost. The two 1.0x runs show wildly different frame
counts (743 vs 349), indicating **non-deterministic frame loss** that
depends on OS scheduling, CPU load, or backpressure in the processing
pipeline.

**Root cause hypothesis:** The PCAP reader pushes frames faster than the
processing + recording pipeline can consume. There is no explicit
flow-control mechanism that ensures every PCAP frame is processed. This
is the single biggest issue: analysis mode and replay mode produce
fundamentally different results because they see different subsets of the
same source data.

### 2.2 Timestamp inversion bug

Three of five runs have `start_ns` from the wall clock (March 2026) and
`end_ns` from the PCAP data (December 2025), producing:

- `duration_secs = 0.0` (clamped from negative)
- `frame_interval_ms.min = -8.1 billion ms` (timestamp jumps backward)
- `inferred_replay_speed = null` (cannot compute from negative duration)

**Root cause:** The recorder captures the timestamp from the first
`FrameBundle` it receives. When playback starts, the first frame may carry
a wall-clock timestamp (from the gRPC handshake, initialization, or a
race between PCAP timestamp injection and the start of recording).
Subsequent frames carry PCAP timestamps, so `end_ns` is correct but
`start_ns` is wrong.

The two runs that avoid this (54ba7296 and 60a4774c) both ran at slow
speeds (possibly allowing the PCAP timestamp source to stabilize before
the first frame was recorded).

| Run        | `start_ns` origin | `end_ns` origin   |
| ---------- | ----------------- | ----------------- |
| `54ba7296` | PCAP (2025-12-05) | PCAP (2025-12-05) |
| `60a4774c` | PCAP (2025-12-05) | PCAP (2025-12-05) |
| `d8f87151` | Wall (2026-03-10) | PCAP (2025-12-05) |
| `8802b52b` | Wall (2026-03-10) | PCAP (2025-12-05) |
| `8099c80b` | Wall (2026-03-10) | PCAP (2025-12-05) |

### 2.3 Very low confirmed-track ratio

Across all runs, confirmed tracks are 2.5–3.4% of total tracks. The
remaining 97%+ are tentative or deleted. From the best run (54ba7296):

- 15 confirmed out of 444 total (3.4%)
- 194 tentative (43.7%) — tracks that never reached confirmation threshold
- 235 deleted (52.9%) — tracks that were created and then pruned

The confirmed tracks are mostly low-speed objects: birds (6 tracks),
dynamic (5), plus 2 cars, 1 pedestrian, and 1 motorcyclist. All have
`max_speed_mps = 0.0`, which indicates the `max_speed` field is not being
populated in the FrameBundle (separate bug).

### 2.4 Confirmed track count scales directly with frame count

| Run        | Frames | Confirmed |
| ---------- | ------ | --------- |
| `54ba7296` | 2012   | 15        |
| `60a4774c` | 1832   | 6         |
| `d8f87151` | 1434   | 5         |
| `8802b52b` | 743    | 4         |
| `8099c80b` | 349    | 3         |

When we lose frames, we lose observations. Fewer observations mean tracks
don't accumulate enough hits to cross the confirmation threshold. This is
the direct mechanism by which frame dropping causes track loss.

### 2.5 Track matching across runs

Pairwise comparison of 54ba7296 (best) against each other run:

| Comparison          | Matched | A-only | B-only | Speed corr | Mean speed Δ |
| ------------------- | ------- | ------ | ------ | ---------- | ------------ |
| 54ba vs 60a4 (0.1x) | 6       | 9      | 0      | 0.319      | 1.87 m/s     |
| 54ba vs d8f8 (0.5x) | 5       | 10     | 0      | 0.998      | 0.13 m/s     |
| 54ba vs 8802 (1.0x) | 4       | 11     | 0      | 0.803      | 1.08 m/s     |
| 54ba vs 8099 (1.0x) | 3       | 12     | 0      | 0.999      | 0.77 m/s     |

Key observations:

- **B-only = 0 everywhere**: the fewer-frames run never finds tracks that
  the more-frames run misses. Frame loss only removes tracks, never adds.
- **54ba vs 60a4 has anomalously low speed correlation (0.319)** despite
  both having many frames. One matched pair has a 5.68 m/s speed delta,
  suggesting track ID fragmentation assigned different observation windows
  to what should be the same physical object.
- **Observation ratios drop with fewer frames**: e.g., for the matched car
  track, obs_ratio goes 0.99 → 0.78 → 0.35 → 0.09 as frames decrease.

### 2.6 100% foreground classification

Every frame in every run reports 100% foreground point ratio. Either:

1. The background model is not subtracting any points, or
2. The foreground percentage calculation only counts points already
   classified as foreground.

If (1), all points feed into clustering, which explains the enormous
tentative track count (hundreds of noise clusters being tracked as
tentative tracks).

### 2.7 Fragmentation is constant

Fragmentation ratio is ~0.43 across all runs except the smallest
(0.53 for 349 frames). This suggests fragmentation is an intrinsic
property of the scene/tuning, not of frame completeness.

## 3. Root Cause Summary

| Issue                     | Severity | Root cause                                           |
| ------------------------- | -------- | ---------------------------------------------------- |
| Frame loss at speed       | Critical | No backpressure / flow control in PCAP replay path   |
| Track loss at speed       | Critical | Direct consequence of frame loss                     |
| Timestamp inversion       | High     | First frame carries wall-clock, not PCAP timestamp   |
| max_speed_mps always 0    | Medium   | Field not populated in FrameBundle track data        |
| 100% foreground           | Medium   | Background model may not be subtracting in PCAP mode |
| High tentative count      | Low      | Expected with noisy foreground + aggressive tracking |
| Non-deterministic 1x runs | High     | OS scheduling / CPU load causes variable frame loss  |

## 4. Experiment Test Plan

### Experiment 1: Frame Integrity Test (blocking)

**Goal:** Verify that analysis mode processes every PCAP frame.

**Method:**

1. Count physical frames in `kirk1.pcapng` using a PCAP parser (count
   packets matching the LiDAR port).
2. Run pipeline in analysis mode at 0.01x speed with VRLOG recording.
3. Compare VRLOG `total_frames` to PCAP frame count.
4. If they differ, the pipeline has frame loss even at the slowest speed.

**Expected outcome:** Frame counts should match exactly when playback
speed is low enough. If they don't, there's a structural frame-loss bug
independent of speed.

**Fix direction:** Add a frame sequence counter to the PCAP reader. The
recorder should log warnings when gaps appear. Analysis mode should
either block until processing completes (synchronous mode) or buffer
frames until the pipeline drains.

### Experiment 2: Synchronous Analysis Mode

**Goal:** Implement and validate a frame-synchronous PCAP processing mode
where the next PCAP frame is read only after the current one finishes
processing.

**Method:**

1. Add `--sync` flag to analysis mode that blocks PCAP reads on pipeline
   completion.
2. Run `kirk1.pcapng` with `--sync` and compare output to the best
   existing run (54ba7296).
3. Frame count should equal PCAP frame count. Track count and
   classifications should be deterministic (identical across repeated
   runs).

**Expected outcome:** Deterministic, complete analysis. Every run of the
same PCAP with the same tuning produces identical tracks.

**Acceptance criteria:**

- `total_frames` equals PCAP frame count.
- Repeated runs produce identical `confirmed_tracks` count.
- Track IDs differ (UUID), but track matching IOU = 1.0 for all pairs.

### Experiment 3: Timestamp Source Audit

**Goal:** Identify where the first frame's timestamp comes from and fix
the wall-clock contamination.

**Method:**

1. Add logging at the PCAP reader, frame pipeline, and recorder entry
   points to trace `FrameBundle.TimestampNanos` origin.
2. Run at 1.0x speed and capture the first 5 frame timestamps.
3. Identify which component injects the wall-clock timestamp.

**Fix direction:** The recorder should reject or re-timestamp frames
where `TimestampNanos` is more than 1 hour away from the previous frame.
Alternatively, the PCAP reader should guarantee it only emits frames
with PCAP-sourced timestamps.

### Experiment 4: Frame Rate vs Confirmed Track Threshold

**Goal:** Determine the minimum frame rate (as % of source) needed to
confirm all real objects.

**Method:**

1. Using the synchronous analysis mode from Experiment 2, process the
   PCAP at full resolution (100% frames).
2. Downsample by processing every Nth frame (N=2,3,5,10) and re-run
   analysis.
3. For each N, compare confirmed track counts and matched track IOU.
4. Plot confirmed track count vs frame percentage.

**Expected outcome:** A monotonic curve showing the minimum frame density
needed for track confirmation. This informs whether the confirmation
threshold is too aggressive or whether frame loss is the sole cause.

### Experiment 5: Background Model Validation

**Goal:** Verify the background model is operational in PCAP mode.

**Method:**

1. Log `foreground_pct` per frame during a slow-speed analysis run.
2. Check whether the first N frames have 100% foreground (expected during
   background learning) and whether it drops after warmup.
3. If foreground stays at 100%, inspect whether the background update path
   is disabled or misconfigured in PCAP mode.

**Expected outcome:** After warmup (typically 50-100 frames), foreground
percentage should stabilize at a scene-dependent value (typically 1-15%
for outdoor traffic scenes).

**Impact:** If the background model isn't working, every point becomes a
foreground cluster candidate, which explains the hundreds of tentative
tracks from noise.

### Experiment 6: Determinism Test

**Goal:** Prove analysis mode is fully deterministic.

**Method:**

1. Run the synchronous analysis mode (Experiment 2) on `kirk1.pcapng`
   5 times.
2. Compare all 5 reports using `vrlog-analyse compare`.
3. All pairs should show: identical frame counts, identical track counts,
   track matching IOU = 1.0 for all confirmed tracks, identical speed
   values.

**Acceptance criteria:** All 5 runs are byte-identical in report output
(except for UUIDs and timestamps).

### Experiment 7: Track Confirmation Threshold Sensitivity

**Goal:** Evaluate whether the confirmation threshold is appropriate.

**Method:**

1. Using a deterministic full-frame analysis, count how many tentative
   tracks have observation counts close to the confirmation threshold.
2. Lower the confirmation threshold by 25% and re-run — how many more
   tracks get confirmed?
3. Raise the threshold by 25% — how many tracks are lost?

**Expected outcome:** Quantify the sensitivity of the confirmation
decision to the threshold parameter. If many tentative tracks are
1-2 observations short of confirmation, the threshold may be too
aggressive for the observed frame rate.

## 5. Priority Order

1. **Experiment 2 (Sync analysis mode)** — foundational; everything else
   depends on deterministic, complete frame processing.
2. **Experiment 1 (Frame integrity)** — validates that sync mode works.
3. **Experiment 3 (Timestamp audit)** — fixes the duration/interval
   corruption.
4. **Experiment 5 (Background model)** — explains 100% foreground and
   tentative track explosion.
5. **Experiment 6 (Determinism)** — validates the analysis pipeline is
   reliable.
6. **Experiment 4 (Frame rate threshold)** — informs graceful degradation
   design.
7. **Experiment 7 (Confirmation threshold)** — fine-tuning after
   fundamentals are fixed.
