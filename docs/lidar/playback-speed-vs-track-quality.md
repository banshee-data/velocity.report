# Playback Speed vs Track Quality

How PCAP replay speed affects tracking accuracy through the lidar pipeline.

## Pipeline Overview

Packets flow through a chain of stages, each with speed-sensitive behaviour:

```
PCAP Reader → FrameBuilder → Pipeline Throttle → Clustering → Tracker → Publisher → gRPC Client
```

At each handoff there is either a channel buffer, a timing gate, or both.
The speed mode determines which throttling mechanisms are active.

## PCAP Speed Modes

| Mode       | SpeedMultiplier | FrameBuilder             | Pacing                | Use Case                           |
| ---------- | --------------- | ------------------------ | --------------------- | ---------------------------------- |
| `analysis` | N/A             | **Blocking** (zero drop) | None — `ReadPCAPFile` | Analysis runs, every frame matters |
| `realtime` | 1.0             | Non-blocking (drop)      | Wall-clock timing     | Live forwarding, visualiser        |
| `scaled`   | User ratio      | Non-blocking (drop)      | Wall-clock timing     | Controlled-speed replay            |

Key code: `datasource_handlers.go:508–610`.

### Analysis mode

Calls `ReadPCAPFile` — no timing, no `RealtimeReplayConfig`. Frames fly through
as fast as the CPU allows. FrameBuilder channel (cap 32) is set to **blocking**
mode so every frame is delivered — zero drops. Back-pressure from the pipeline
callback naturally throttles the PCAP reader.
Best for analysis runs where every frame must be processed.

### Realtime / Scaled modes

Timing-paced via wall-clock comparison. FrameBuilder is non-blocking (drops when
channel full). The dynamic backoff system (see below) prevents catch-up bursts
from flooding the pipeline.

## Stage-by-Stage Quality Impact

### 1. PCAP Reader Pacing (`pcap_realtime.go`)

The reader compares wall-clock elapsed time against PCAP capture-time elapsed,
sleeping when ahead of schedule and applying dynamic backoff when behind.

**Death spiral (scaled):** Before the `cumulativeYield` fix, backoff sleep time
was counted as wall-clock time, making `behindBy` grow monotonically at sub-1x
speeds. The fix subtracts cumulative yield from elapsed time.

**Quality impact:** Pacing doesn't directly affect track quality — it controls
the rate at which packets enter the FrameBuilder. The downstream stages determine
whether frames are processed or dropped.

### 2. FrameBuilder (`frame_builder.go`)

Accumulates points into 360-degree rotational frames. Completed frames are
pushed to a channel (capacity **32**).

| Mode                   | Behaviour                          | Drop risk                 |
| ---------------------- | ---------------------------------- | ------------------------- |
| Non-blocking (default) | `select` on channel; drops if full | Yes — at burst speeds     |
| Blocking (`analysis`)  | Blocks until pipeline accepts      | None — full back-pressure |

**Quality impact:** Dropped frames = missed observations for the tracker.
A track with `MaxMisses=3` (tentative) can be deleted by just 3 consecutive
drops. In non-blocking mode at >1x speeds, burst drops are likely.

### 3. MaxFrameRate Throttle (`tracking_pipeline.go:323–343`)

Caps the rate at which frames proceed through the expensive downstream path
(clustering, tracking, serialisation). Default: **25 fps**.

**What runs on every frame regardless:** Background model update
(`ProcessFramePolarWithMask`) — keeps foreground extraction accurate even
during throttle.

**What is skipped during throttle:**

- Clustering (DBSCAN)
- Tracking (`Tracker.Update`)
- Serialisation / forwarding
- **`AdvanceMisses` is intentionally NOT called**

The deliberate skip of `AdvanceMisses` is critical: during PCAP catch-up bursts,
frames arrive faster than 25 fps. If each throttled frame incremented misses,
tentative tracks (`MaxMisses=3`) would die within ~120-300ms of burst. By
skipping miss advancement, tracks coast through the burst and resume tracking
when frames slow to processable rates.

**Quality impact at different speeds:**

| Speed                | Effective fps | Throttle active? | Effect                                                                   |
| -------------------- | ------------- | ---------------- | ------------------------------------------------------------------------ |
| 0.5x                 | ~5 fps        | No               | Every frame processed                                                    |
| 1.0x                 | 10-20 fps     | Rarely           | Normal operation                                                         |
| 2.0x                 | 20-40 fps     | Sometimes        | Some frames skip tracking                                                |
| Analysis (CPU-bound) | CPU-bound     | Heavily          | Most frames skip tracking, but blocking mode prevents FrameBuilder drops |

### 4. Kalman Filter dt Sensitivity (`tracking.go:370–381`)

The tracker computes `dt` from the nanosecond timestamps of successive frames:

```go
dt = float32(nowNanos - t.LastUpdateNanos) / 1e9
```

Clamped at `MaxPredictDt=0.5s` to prevent covariance explosion.

**How speed affects dt:**

- **Analysis mode + throttle:** Throttled frames don't call `Tracker.Update`, so
  `LastUpdateNanos` isn't advanced. When the next frame passes the throttle,
  `dt` = time since last processed frame. At 25fps cap, dt ~40ms regardless
  of replay speed.
- **Realtime/Scaled mode + drops:** If FrameBuilder drops frames, dt stretches.
  At 2x speed with occasional drops, dt might jump to 100-200ms, widening the
  gating ellipse and increasing the chance of mis-association.
- **Sub-1x speeds:** dt is smaller than real-time (e.g. ~100ms at 0.5x with
  10Hz sensor). This tightens gating — good for accuracy, but could miss
  legitimate fast-moving objects at the edge of the gate.

**The predict step uses dt:**

```
X += VX * dt    (position update)
P = F*P*F' + Q  (covariance growth proportional to dt^2)
```

Larger dt = wider gating = more permissive association = potential track swaps.

### 5. Speed Window (`speed_window.go`)

A purely **sample-based** ring buffer (`max_speed_history_length=100`) with
no time weighting or decay.

```
At 10 fps: 100 samples = 10 seconds of data
At 50 fps: 100 samples = 2 seconds of data
```

**Quality impact:** At higher effective frame rates, the speed window covers
a shorter wall-clock duration. P50 speed converges faster but is more volatile.
At lower frame rates, the window covers more time, providing smoother estimates
but slower response to actual speed changes.

For speed measurement accuracy, this is **not directly affected by replay speed**
because the MaxFrameRate throttle ensures the tracker sees ~25 fps max regardless
of how fast packets arrive. The practical speed window duration is ≥4 seconds.

### 6. Miss Counting (`tracking.go:439–492`)

| Track state | Max misses              | Effect of miss                                 |
| ----------- | ----------------------- | ---------------------------------------------- |
| Tentative   | `MaxMisses=3`           | Deleted after 3 consecutive misses             |
| Confirmed   | `MaxMissesConfirmed=15` | Deleted after 15 consecutive misses (coasting) |

Misses are incremented in `AdvanceMisses()` which is called from
`Tracker.Update()`. Throttled frames skip `Update`, so misses don't accumulate
during speed bursts (intentional).

**Where misses cause quality problems:**

- FrameBuilder drops (non-blocking mode): each dropped frame is invisible to
  the tracker — no miss is counted, but the track also gets no observation.
  Paradoxically, drops are **less damaging** to track continuity than processing
  empty frames (which would increment misses). The risk is that fast-moving
  objects move out of the gating radius during the unobserved gap.
- Slow pipeline: if the pipeline callback takes >100ms, the FrameBuilder
  channel fills, dropping frames. The tracker loses observations without
  counting misses.

### 7. Publisher / gRPC Forwarding (`publisher.go`, `grpc_server.go`)

Two-level buffering:

- **Publisher channel:** capacity 100, drops when full
- **Per-client channel:** capacity 10, drops when client is slow

**Quality impact on visualisation (not tracking):** Publisher drops don't
affect tracking — they only affect what the gRPC client (visualiser) sees.
A slow gRPC client will see stale frames. The gRPC cooldown system enters
"skip mode" after repeated slow sends, dropping frames at the server to
prevent backlog growth.

## Practical Recommendations

### For analysis runs (accuracy matters most)

Use **`analysis`** mode. Blocking FrameBuilder ensures zero frame drops. The
25fps throttle still applies, but `AdvanceMisses` is skipped during throttle,
preserving track continuity. Every frame gets background model processing.

### For live visualisation

Use **`realtime`** (1.0x). Timing-paced, matches sensor cadence. Dynamic
backoff prevents burst flooding. Some frames may drop under pipeline load
but confirmed tracks coast with `MaxMissesConfirmed=15`.

### For speed testing

Use **`scaled`** with ratios:

- **0.5x:** Tighter Kalman gating, fewer drops, best association accuracy.
  Good baseline for comparing against faster modes.
- **1.0x:** Normal operating conditions.
- **2.0x:** Tests pipeline resilience. Expect occasional FrameBuilder drops
  and wider Kalman dt. Watch for track breaks (tentative tracks dying) and
  ID swaps (gating too wide).

### Comparing quality across speeds

The `pcap-analyse` tool can run the same PCAP at different speeds and compare:

- Track count and track duration distribution
- Speed P50/P85 per track
- Miss ratio (misses / total frames per track)
- Track breaks (same object getting multiple track IDs)

## Summary Table

| Factor                | Sub-1x        | 1x          | 2x+                     |
| --------------------- | ------------- | ----------- | ----------------------- |
| FrameBuilder drops    | None          | Rare        | Likely                  |
| Pipeline throttle     | Inactive      | Inactive    | Active                  |
| AdvanceMisses         | Every frame   | Every frame | Skipped on throttle     |
| Kalman dt             | ~100ms (10Hz) | ~50-100ms   | ~40ms (throttle-capped) |
| Speed window duration | ~10s          | ~5-10s      | ~4s (throttle-capped)   |
| Background model      | Every frame   | Every frame | Every frame             |
| Track break risk      | Low           | Low         | Medium (drops)          |
| Gating accuracy       | Tight         | Normal      | Wide (if dt inflated)   |
