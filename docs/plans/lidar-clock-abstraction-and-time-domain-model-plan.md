# Clock Abstraction Adoption and Time-Domain Model

- **Status:** Proposed
- **Layers:** Cross-cutting (L1 Packets, L2 Frames, Pipeline, L9 Endpoints)
- **Canonical:** [multi-model-ingestion-and-configuration.md](../lidar/architecture/multi-model-ingestion-and-configuration.md)
- **Related:** [`timeutil/clock.go`](../../internal/timeutil/clock.go), [`lidar-data-layer-model.md`](../lidar/architecture/lidar-data-layer-model.md)

Adopt the existing `timeutil.Clock` interface into the four critical-path subsystems where wall-time coupling prevents testability, and formalise the sensor-time vs wall-time boundary before multi-sensor support lands.

## Problem Statement

The `timeutil.Clock` interface (`internal/timeutil/clock.go`) provides a complete
clock abstraction with `RealClock`, `MockClock`, `MockTimer`, and `MockTicker` —
333 lines of production-ready code. It is used in exactly **three test call sites**
(`internal/lidar/logutil/tagged_logger_test.go`). Meanwhile, **81 non-test call
sites** use `time.Now()` directly, and key subsystems (pipeline throttle, frame
cleanup, replay pacing, benchmark timing) are untestable without waiting for
real wall-clock intervals.

Separately, the pipeline conflates two distinct time domains — sensor timestamps
from device packets and host wall-clock timestamps — which works today with a
single Hesai Pandar40P in `TimestampModeSystemTime`, but will break under GPS/PTP
modes or multi-sensor configurations.

## Findings

### F1: Clock Abstraction Exists but Is Almost Entirely Unused

81 non-test `time.Now()` call sites exist. Key hotspots:

| Location | Category | Risk |
|----------|----------|------|
| `l1packets/parse/extract.go:226,236,467,498` | Parser boot-time and packet timestamping | Medium — ties internal timestamp mode to init-time wall clock |
| `l2frames/frame_builder.go:163` | `time.AfterFunc` for cleanup timers | Medium — cleanup logic untestable |
| `l3grid/` (17 calls) | Background model timestamps | Low — audit/diagnostic timestamps |
| `pipeline/tracking_pipeline.go:215` | Frame-rate throttle state | High — throttle logic untestable |
| `pipeline/frame_timer.go:36,46,62` | Benchmark stopwatch | Medium — timing overhead unmockable |
| `l9endpoints/replay.go:98` | Replay pacing via `time.Sleep` | High — replay timing untestable |
| `serialmux/serialmux.go:109,115` | Radar device clock sync | Low — one-shot initialisation |

### F2: Two Time Domains Are Conflated

**Sensor time** — timestamps embedded in LiDAR packets (`CombinedTimestamp`,
`TimestampMode` in `extract.go`) or radar serial data. Source: device clock
(GPS, PTP, internal, or native LiDAR DateTime+Timestamp).

**Wall time** — timestamps from the host's `time.Now()`. Used for ingest timing
(`LiDARFrame.StartWallTime/EndWallTime`), throttle decisions, and DB audit fields.

Conflation points:

- `extract.go:226` — `bootTime: time.Now()` initialises device-internal offset from wall clock
- `extract.go:467,498` — falls back to `packetTime = time.Now()` when sensor timestamp unavailable
- `tracking.go:192` — `dt` computed from `timestamp.UnixNano()`, which may be sensor or wall time depending on upstream `TimestampMode`

This works today because `TimestampModeSystemTime` is the default and all timestamps are effectively wall time. Under GPS/PTP modes, or in replay, the tracker's `dt` and the pipeline throttle would use different time bases.

### F3: Frame-Rate Throttle Is Wall-Time Coupled

`pipeline/tracking_pipeline.go:215-219`:

```go
var lastProcessedTime time.Time
now := time.Now()
if !lastProcessedTime.IsZero() && now.Sub(lastProcessedTime) < minFrameInterval {
    // skip expensive path
}
lastProcessedTime = now
```

Cannot unit-test without waiting real milliseconds. During PCAP replay, throttle decisions are wall-time based, not replay-time based.

### F4: FrameBuilder Uses `time.AfterFunc` Directly

`l2frames/frame_builder.go:163` and `frame_builder_cleanup.go:315`:

```go
fb.cleanupTimer = time.AfterFunc(fb.cleanupInterval, fb.cleanupFrames)
```

The only way to test frame-expiry behaviour is to wait for real milliseconds. `FrameBuilderConfig` already has `CleanupInterval` but no way to inject a clock.

### F5: Radar Serial Path Is Fine as Is

`serialmux/serialmux.go:109,115` syncs the radar device clock to the host's wall time — a one-shot initialisation command. No clock abstraction needed.

### F6: Multi-Sensor Future Needs Timestamp Alignment

The [multi-model ingestion design](../lidar/architecture/multi-model-ingestion-and-configuration.md) describes supporting 3–10 LiDAR models with different packet formats but does not address cross-sensor timestamp alignment, variable spin rates across sensors, or the L7 Scene fusion time reference.

## Options Considered

### Clock adoption scope

| Option | Description | Effort | Risk |
|--------|-------------|--------|------|
| A. Do nothing | Leave 81 bare `time.Now()` calls | Zero | Accumulating test debt; throttle/replay untestable |
| B. Critical-path only | Wire `Clock` into FrameBuilder, pipeline throttle, replay server, frame_timer | M | Low — interfaces exist, DI patterns established |
| C. Full migration | Replace all 81 production `time.Now()` calls | L | Medium — unnecessary for logging/audit; large review surface |

**Decision: B.** The four critical-path subsystems account for most testability risk. Logging and DB audit timestamps do not need injection.

### Time-domain formalisation

| Option | Description | Effort | Risk |
|--------|-------------|--------|------|
| A. Document boundary | Add doc comments and architecture doc explaining sensor-time vs wall-time | S | Low — no behaviour change |
| B. Type-tag timestamps | Introduce `SensorTimestamp` / `WallTimestamp` type aliases; validate at boundaries | M | Medium — touches every timestamp-carrying struct |
| C. Alignment service | Compute wall↔sensor offset and maintain live | L | High — premature |

**Decision: A now, B later.** Conceptual clarity first, then optional type-level enforcement when multi-sensor lands.

## System Boundary Diagram

```
                      SENSOR TIME DOMAIN                         WALL TIME DOMAIN
                ┌──────────────────────────────┐       ┌──────────────────────────────────┐
                │                              │       │                                  │
 ┌──────────┐  │  ┌────────────────────┐       │       │  ┌────────────────────┐           │
 │ Hesai     │  │  │ L1 Pandar40PParser │       │       │  │ Pipeline Throttle  │           │
 │ Pandar40P │──┤  │ TimestampMode:     │       │       │  │ time.Now() → Clock │           │
 └──────────┘  │  │  · SystemTime ─────┼──wall─┼──────▶│  └────────┬───────────┘           │
               │  │  · GPS/PTP/LiDAR ──┼─sensor┼──┐    │           │                       │
               │  └────────┬───────────┘       │  │    │  ┌────────▼───────────┐           │
               │           │ PointPolar.Ts     │  │    │  │ Frame Cleanup      │           │
               │  ┌────────▼───────────┐       │  │    │  │ AfterFunc → Clock  │           │
               │  │ L2 FrameBuilder    │       │  │    │  └────────────────────┘           │
               │  │ StartTimestamp ─────┼─sensor│──┤    │                                  │
               │  │ StartWallTime ─────┼──wall─┼──┼───▶│  ┌────────────────────┐           │
               │  └────────┬───────────┘       │  │    │  │ Replay Pacing      │           │
               │  ┌────────▼───────────┐       │  │    │  │ Sleep → Clock      │           │
               │  │ L5 Tracker         │       │  │    │  └────────────────────┘           │
               │  │ dt = Δ(timestamp)  │◀──────┼──┘    │                                  │
 ┌──────────┐  │  └────────────────────┘       │       │  ┌────────────────────┐           │
 │ OmniPre- │  │                              │       │  │ DB Audit / Logging │           │
 │ Sense    │──┤  (radar serial: fine as-is)   │       │  │ time.Now() (stays) │           │
 └──────────┘  │                              │       │  └────────────────────┘           │
               └──────────────────────────────┘       └──────────────────────────────────┘
```

## Failure Registry

| Component | Failure Mode | Recovery |
|-----------|-------------|----------|
| `Clock` injection into `TrackingPipelineConfig` | Nil clock → panic on first frame | Default to `RealClock{}` if nil (defensive constructor) |
| `Clock` injection into `FrameBuilderConfig` | Nil clock → cleanup timer never fires | Default to `RealClock{}` if nil |
| `MockClock.Advance()` in tests | Forget to advance → timers never fire → test hangs | Document pattern; use test timeout assertions |
| Sensor/wall time mismatch | Tracker dt from sensor timestamps drifts from wall time | `TimestampModeSystemTime` is production default; GPS/PTP are specialist |

## Implementation Plan

### Phase 1: Wire Clock into Critical-Path Code

- [ ] **1a.** Add `Clock timeutil.Clock` to `FrameBuilderConfig`. Default to `RealClock{}` in `NewFrameBuilder` if nil. Replace `time.AfterFunc` calls with `clock.NewTimer()`.
- [ ] **1b.** Add `Clock timeutil.Clock` to `TrackingPipelineConfig`. Default to `RealClock{}` in `NewFrameCallback` if nil. Replace `time.Now()` in throttle logic with `clock.Now()`.
- [ ] **1c.** Add `Clock` parameter to `newFrameTimer()` in `pipeline/frame_timer.go`. Replace three bare `time.Now()` / `time.Since()` calls.
- [ ] **1d.** Add `Clock` to `ReplayServer` / `streamFromReader`. Replace `time.Sleep()` and `time.Time{}` pacing with clock-driven intervals.
- [ ] **1e.** Write tests for throttle behaviour using `MockClock.Advance()` — verify frames within `minFrameInterval` are skipped without waiting real time.
- [ ] **1f.** Write tests for FrameBuilder cleanup using `MockClock` — verify stale frames are cleaned up after advancing mock time past `CleanupInterval`.

### Phase 2: Formalise Time-Domain Boundary

- [ ] **2a.** Add doc comment block to `LiDARFrame` in `l2frames/types.go` explaining the two timestamp domains: `StartTimestamp/EndTimestamp` (sensor, used for dt and Kalman) vs `StartWallTime/EndWallTime` (host, used for rate control and diagnostics).
- [ ] **2b.** Create `docs/lidar/architecture/time-domain-model.md` explaining the sensor-time vs wall-time boundary, when each is appropriate, and implications for multi-sensor fusion.
- [ ] **2c.** Add a note to `multi-model-ingestion-and-configuration.md` referencing the time-domain model and identifying cross-sensor timestamp alignment as an open question for L7 Scene.

### Phase 3: Validate and Harden

- [ ] **3a.** Audit all five `TimestampMode` code paths in `extract.go` (lines 460–510). Verify that the `dt` computed in the tracker is monotonically increasing for each mode. Add a unit test per mode.
- [ ] **3b.** Add a regression test: replay a VRLOG file with `MockClock`, verify tracker `dt` values match original sensor timestamp intervals (not wall-clock replay intervals).
- [ ] **3c.** Verify `time.AfterFunc` replacement in FrameBuilder does not regress race tests (`toasc_race_test.go`).

### Phase 4: Multi-Sensor Preparation (Deferred)

- [ ] **4a.** When L7 Scene is designed, define a `TimestampAligner` interface accepting frames from N sensors with independent clocks producing a unified timeline.
- [ ] **4b.** When multi-model ingestion lands, ensure each `FrameBuilder` instance carries its own `Clock` for independent spin rates and frame timing per sensor.
- [ ] **4c.** Evaluate whether radar serial timestamps need alignment with LiDAR timestamps for sensor-fusion use cases.

## Size Estimates

| Phase | Scope | Effort |
|-------|-------|--------|
| 1 | Clock injection into four subsystems + tests | M (2–3 days) |
| 2 | Documentation and type comments | S (½ day) |
| 3 | Timestamp mode audit and regression tests | S (½ day) |
| 4 | Multi-sensor preparation | Deferred |

**Size key:** S = ½ day, M = 1–3 days, L = 3+ days

## Open Questions

1. Should `frame_timer.go` accept a `Clock` or remain wall-time-only since it measures real CPU-time performance? (Recommendation: inject Clock for consistency; benchmarks can pass `RealClock`.)
2. Should `l3grid` background timestamps (17 calls) migrate to `Clock`? (Recommendation: defer — these are diagnostic/audit timestamps with low testability risk.)
3. When multi-sensor lands, should each sensor's `FrameBuilder` have an independent `Clock` or share one? (Recommendation: independent — sensors have different spin rates.)
