# Clock Abstraction Adoption and Time-Domain Model

- **Status:** Proposed
- **Layers:** Cross-cutting (L1 Packets, L2 Frames, Pipeline, L9 Endpoints)
- **Canonical:** [multi-model-ingestion-and-configuration.md](../lidar/architecture/multi-model-ingestion-and-configuration.md)
- **Related:** [`timeutil/clock.go`](../../internal/timeutil/clock.go),
  [`lidar-data-layer-model.md`](../lidar/architecture/lidar-data-layer-model.md),
  [`go-structured-logging-plan.md`](go-structured-logging-plan.md) (Item 3)

Adopt the existing `timeutil.Clock` interface into the critical-path
subsystems where wall-time coupling prevents testability, eliminate
`time.Sleep` synchronisation in tests, and formalise the sensor-time
vs wall-time boundary before multi-sensor support lands.

## Problem Statement

The `timeutil.Clock` interface (`internal/timeutil/clock.go`) provides a
complete clock abstraction with `RealClock`, `MockClock`, `MockTimer`,
and `MockTicker`. As of March 2026 it is used in only a handful of test
call sites (`internal/lidar/logutil/tagged_logger_test.go`). Meanwhile,
the majority of non-test call sites use `time.Now()` directly, and key
subsystems (pipeline throttle, frame cleanup, replay pacing, benchmark
timing) are untestable without waiting for real wall-clock intervals.

To reproduce current counts:

```sh
# Non-test time.Now() calls
rg 'time\.Now\(\)' internal/ cmd/ --type go -g '!*_test.go' -c
# Current Clock adoption sites
rg 'timeutil\.Clock|MockClock' internal/ --type go -c
```

Separately, the pipeline conflates two distinct time domains — sensor
timestamps from device packets and host wall-clock timestamps — which
works today with a single Hesai Pandar40P in `TimestampModeSystemTime`,
but will break under GPS/PTP modes or multi-sensor configurations.

## Findings

### F1: Clock Abstraction Exists but Is Almost Entirely Unused

Key hotspots where bare `time.Now()` creates testability risk:

| Location                                    | Category                     | Risk   |
| ------------------------------------------- | ---------------------------- | ------ |
| `l1packets/parse/extract.go:226,236,467,498`| Parser boot-time / timestamp | Medium |
| `l2frames/frame_builder.go:163`             | `time.AfterFunc` cleanup     | Medium |
| `l3grid/` (multiple calls)                  | Background model timestamps  | Low    |
| `pipeline/tracking_pipeline.go:215`         | Frame-rate throttle state    | High   |
| `pipeline/frame_timer.go:36,46,62`          | Benchmark stopwatch          | Medium |
| `l9endpoints/replay.go:98`                  | Replay `time.Sleep` pacing   | High   |
| `serialmux/serialmux.go:109,115`           | Radar device clock sync      | Low    |

### F2: Two Time Domains Are Conflated

**Sensor time** — timestamps embedded in LiDAR packets
(`CombinedTimestamp`, `TimestampMode` in `extract.go`) or radar
serial data. Source: device clock (GPS, PTP, internal, or native
LiDAR DateTime+Timestamp).

**Wall time** — timestamps from the host's `time.Now()`. Used for
ingest timing (`LiDARFrame.StartWallTime/EndWallTime`), throttle
decisions, and DB audit fields.

Conflation points:

- `extract.go:226` — `bootTime: time.Now()` initialises
  device-internal offset from wall clock
- `extract.go:467,498` — falls back to `packetTime = time.Now()`
  when sensor timestamp unavailable
- `l5tracks/tracking.go:192` — `dt` computed from
  `timestamp.UnixNano()`, which may be sensor or wall time
  depending on upstream `TimestampMode`

This works today because `TimestampModeSystemTime` is the default
and all timestamps are effectively wall time. Under GPS/PTP modes,
or in replay, the tracker's `dt` and the pipeline throttle would
use different time bases.

### F3: Frame-Rate Throttle Is Wall-Time Coupled

`pipeline/tracking_pipeline.go:215-219`:

```go
var lastProcessedTime time.Time
now := time.Now()
if !lastProcessedTime.IsZero() &&
    now.Sub(lastProcessedTime) < minFrameInterval {
    // skip expensive path
}
lastProcessedTime = now
```

Cannot unit-test without waiting real milliseconds. During PCAP
replay, throttle decisions are wall-time based, not replay-time
based.

### F4: FrameBuilder Uses `time.AfterFunc` Directly

`l2frames/frame_builder.go:163` and `frame_builder_cleanup.go:315`:

```go
fb.cleanupTimer = time.AfterFunc(
    fb.cleanupInterval, fb.cleanupFrames)
```

The only way to test frame-expiry behaviour is to wait for real
milliseconds. `FrameBuilderConfig` already has `CleanupInterval`
but no way to inject a clock.

**Replacement pattern:** extend `timeutil.Clock` with an
`AfterFunc(d time.Duration, f func()) Timer` method that returns
a stoppable/resettable `Timer`. `MockClock` fires the callback
synchronously on `Advance()`. This preserves the callback-based
semantics of `time.AfterFunc` while making the schedule
controllable in tests. Implementation reference: the existing
`MockTimer.checkAndFire` in `clock.go` already fires on
`Advance()` — `AfterFunc` follows the same pattern but invokes
`f()` instead of sending on a channel.

### F5: Radar Serial Path Is Fine as Is

`serialmux/serialmux.go:109,115` syncs the radar device clock to
the host's wall time — a one-shot initialisation command. No clock
abstraction needed.

### F6: Multi-Sensor Future Needs Timestamp Alignment

The [multi-model ingestion design](../lidar/architecture/multi-model-ingestion-and-configuration.md)
describes supporting 3–10 LiDAR models with different packet
formats but does not address cross-sensor timestamp alignment,
variable spin rates across sensors, or the L7 Scene fusion time
reference.

## Decisions

### Clock adoption scope

| Option | Description         | Effort | Risk   |
| ------ | ------------------- | ------ | ------ |
| A      | Do nothing          | Zero   | High   |
| B      | Critical-path only  | M      | Low    |
| C      | Full migration      | L      | Medium |

**Decision: B (Phases A–B), with C as future work.** The four
critical-path subsystems and `time.Sleep` test sites account for
most testability risk. Logging and DB audit timestamps do not need
injection. The remaining ~60 `time.Now()` call sites in `l3grid`,
`serialmux`, `cmd/`, etc. are catalogued as Phase C future work.

### Time-domain formalisation

| Option | Description            | Effort | Risk   |
| ------ | ---------------------- | ------ | ------ |
| A      | Document boundary      | S      | Low    |
| B      | Type-tag timestamps    | M      | Medium |
| C      | Alignment service      | L      | High   |

**Decision: A now, B later.** Conceptual clarity first, then
optional type-level enforcement when multi-sensor lands.

### Open questions — resolved

1. **Should `pipeline/frame_timer.go` accept a `Clock`?**
   Yes. Inject `Clock` for consistency; benchmarks pass
   `RealClock{}`.

2. **Should `l3grid` background timestamps migrate to `Clock`?**
   Phase C (future work) — diagnostic/audit timestamps with low
   testability risk.

3. **Should each sensor's `FrameBuilder` have an independent
   `Clock` or share one?**
   Independent by default — sensors have different spin rates.
   However, the API should accept an optional shared `Clock` for
   sensors that require frame-rate synchronisation. Each sensor
   gets its own `Clock` instance unless explicitly wired to
   another sensor's clock.

## System Boundary Diagram

```
              SENSOR TIME DOMAIN              WALL TIME DOMAIN
        ┌──────────────────────────┐   ┌──────────────────────────┐
        │                          │   │                          │
 Hesai  │  L1 Pandar40PParser     │   │  Pipeline Throttle       │
 40P ───┤  TimestampMode:         │   │  time.Now() → Clock      │
        │   · SystemTime ────wall──┼──▶│  ┌──────────────────┐    │
        │   · GPS/PTP/LiDAR ─sens─┼─┐ │  │ Frame Cleanup    │    │
        │  └──────────────────┘    │ │ │  │ AfterFunc → Clock│    │
        │         │ PointPolar.Ts  │ │ │  └──────────────────┘    │
        │  L2 FrameBuilder        │ │ │                          │
        │  StartTimestamp ───sens──┼─┤ │  Replay Pacing           │
        │  StartWallTime ───wall──┼─┼▶│  Sleep → Clock           │
        │  └──────────────────┘    │ │ │  └──────────────────┘    │
        │  L5 Tracker             │ │ │                          │
        │  dt = Δ(timestamp) ◀────┼─┘ │  DB Audit / Logging      │
 Omni   │                          │   │  time.Now() (stays)      │
 PreS ──┤  (radar serial: fine)    │   │                          │
        └──────────────────────────┘   └──────────────────────────┘
```

## Failure Registry

| Component         | Failure Mode            | Recovery                |
| ----------------- | ----------------------- | ----------------------- |
| Clock → Pipeline  | Nil clock → panic       | Default `RealClock{}`   |
| Clock → FrameBld  | Nil → timer never fires | Default `RealClock{}`   |
| MockClock.Advance | Forget → test hangs     | Document; use timeouts  |
| Time-domain drift | Tracker dt vs wall time | SystemTime is default   |

## Implementation Plan

### Phase A: Wire Clock into Critical-Path Code

- [ ] **A1.** Add `Clock timeutil.Clock` to `FrameBuilderConfig`.
  Default to `RealClock{}` in `NewFrameBuilder` if nil. Add
  `AfterFunc` method to `Clock` interface. Replace
  `time.AfterFunc` calls with `clock.AfterFunc()`.
- [ ] **A2.** Add `Clock timeutil.Clock` to
  `TrackingPipelineConfig`. Default to `RealClock{}` in
  `NewFrameCallback` if nil. Replace `time.Now()` in throttle
  logic with `clock.Now()`.
- [ ] **A3.** Add `Clock` parameter to `newFrameTimer()` in
  `pipeline/frame_timer.go`. Replace three bare `time.Now()` /
  `time.Since()` calls.
- [ ] **A4.** Add `Clock` to `ReplayServer` /
  `streamFromReader`. Replace `time.Sleep()` and `time.Time{}`
  pacing with clock-driven intervals.
- [ ] **A5.** Write tests for throttle behaviour using
  `MockClock.Advance()` — verify frames within
  `minFrameInterval` are skipped without waiting real time.
- [ ] **A6.** Write tests for FrameBuilder cleanup using
  `MockClock` — verify stale frames are cleaned up after
  advancing mock time past `CleanupInterval`.

### Phase B: Eliminate `time.Sleep` in Tests

Companion to [go-structured-logging-plan.md](go-structured-logging-plan.md)
Item 3 (flaky sleep elimination). That plan identifies the
problem and scope; this phase provides the `Clock`-based
mechanism.

- [ ] **B1.** Add `testutil.WaitFor(t, condition, timeout)`
  polling helper to `internal/testutil/`.
- [ ] **B2.** Audit all `time.Sleep` calls in `_test.go` files.
  Replace with `MockClock.Advance()` where a `Clock` is
  available, or `WaitFor` polling where it is not.
- [ ] **B3.** Replace production `time.Sleep` in
  `l9endpoints/replay.go` with `clock.Sleep()` (covered by A4
  above) — verify replay tests no longer depend on wall time.
- [ ] **B4.** Document the `MockClock.Advance()` test pattern
  and `WaitFor` helper in a short section in
  `docs/platform/architecture/structured-logging.md` or a
  dedicated test-patterns doc.

### Phase C: Full Migration (Future Work)

Migrate the remaining `time.Now()` call sites not covered by
Phase A. These are lower-risk diagnostic, audit, and logging
timestamps. Included here for completeness; schedule when
testability benefits justify the review surface.

- [ ] **C1.** `l3grid/` — ~17 background model timestamps
  (audit/diagnostic). Migrate to `Clock.Now()`.
- [ ] **C2.** `l1packets/parse/extract.go` — boot-time and
  fallback packet timestamping (4 calls). Requires careful
  handling of `TimestampMode` interactions.
- [ ] **C3.** `serialmux/serialmux.go` — radar clock sync
  (2 calls). Low priority; one-shot init.
- [ ] **C4.** `cmd/radar/` and `cmd/tools/` — startup/CLI
  timestamps. Low testability benefit.
- [ ] **C5.** `internal/db/` — DB audit timestamps. Low
  testability benefit but may be useful for deterministic
  test snapshots.

### Phase D: Multi-Sensor Preparation (Deferred)

- [ ] **D1.** When L7 Scene is designed, define a
  `TimestampAligner` interface accepting frames from N sensors
  with independent clocks producing a unified timeline.
- [ ] **D2.** When multi-model ingestion lands, ensure each
  `FrameBuilder` instance carries its own `Clock` for
  independent spin rates and frame timing per sensor. Provide
  an option to share a `Clock` across sensors that require
  frame-rate synchronisation.
- [ ] **D3.** Evaluate whether radar serial timestamps need
  alignment with LiDAR timestamps for sensor-fusion use cases.

### Phase E: Formalise Time-Domain Boundary

- [ ] **E1.** Add doc comment block to `LiDARFrame` in
  `l2frames/types.go` explaining the two timestamp domains:
  `StartTimestamp/EndTimestamp` (sensor, used for dt and
  Kalman) vs `StartWallTime/EndWallTime` (host, used for rate
  control and diagnostics).
- [ ] **E2.** Create
  `docs/lidar/architecture/time-domain-model.md` explaining the
  sensor-time vs wall-time boundary, when each is appropriate,
  and implications for multi-sensor fusion.
- [ ] **E3.** Add a note to
  `multi-model-ingestion-and-configuration.md` referencing the
  time-domain model and identifying cross-sensor timestamp
  alignment as an open question for L7 Scene.

### Phase F: Validate and Harden

- [ ] **F1.** Audit all five `TimestampMode` code paths in
  `extract.go` (lines 460–510). Verify that the `dt` computed
  in the tracker is monotonically increasing for each mode.
  Add a unit test per mode.
- [ ] **F2.** Add a regression test: replay a VRLOG file with
  `MockClock`, verify tracker `dt` values match original sensor
  timestamp intervals (not wall-clock replay intervals).
- [ ] **F3.** Verify `time.AfterFunc` replacement in
  FrameBuilder does not regress race tests
  (`toasc_race_test.go`).

## Size Estimates

| Phase | Scope                                    | Effort          |
| ----- | ---------------------------------------- | --------------- |
| A     | Clock injection into four subsystems     | M (2–3 days)    |
| B     | time.Sleep elimination in tests          | S–M (1–2 days)  |
| C     | Full remaining time.Now() migration      | M (future work) |
| D     | Multi-sensor preparation                 | Deferred        |
| E     | Documentation and type comments          | S (½ day)       |
| F     | Timestamp mode audit and regression      | S (½ day)       |

**Size key:** S = ½ day, M = 1–3 days, L = 3+ days
