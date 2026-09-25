# LiDAR time-domain model

Which clock each part of the LiDAR pipeline runs on, why the estimator runs on capture time alone,
and what that means for replay, transport gaps and more than one sensor.

- **Status:** Implemented for the estimator boundary (L5 and the timestamps that feed it). Clock
  injection into runtime code is the separate clock-abstraction remainder.
- **Layers:** L1 Packets, L2 Frames, L3 Grid, L5 Tracks, pipeline, offline replay
- **Plan:** [clock abstraction and time-domain model plan](../../plans/lidar-clock-abstraction-and-time-domain-model-plan.md)
- **Related:** [state-estimation plan](../../plans/lidar-state-estimation-plan.md) (Section 5.6, question Q3),
  [portable capture timing](portable-capture-timing.md), [LIDAR_ARCHITECTURE.md](LIDAR_ARCHITECTURE.md),
  [PCAP analysis mode](../operations/pcap-analysis-mode.md)

## The rule

Every quantity that estimates motion, or decides whether a track still exists, is derived from
**capture time**: the timestamps carried by the data. The host's **wall clock** serves operator
and runtime concerns only: pacing a replay, throttling it, ageing out stalled frames, timing
stages, pruning the database and stamping logs.

Wall-clock concerns may decide _which_ frames reach the estimator. They never decide _how much
time_ the estimator believes has passed. That is what makes a replay reproduce its tracks at any
pace, on any machine.

## Two clocks

| Clock        | Origin                                                                  | Carried by                                                                                     | Reproducible on replay |
| ------------ | ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | ---------------------- |
| Capture time | L1 packet time, resolved per packet by the parser (see the table below) | `PointPolar.Timestamp`, `LiDARFrame.StartTimestamp`/`EndTimestamp`, `WorldCluster.TSUnixNanos` | Yes                    |
| Wall time    | The host's `time.Now()` at the moment code runs                         | `LiDARFrame.StartWallTime`/`EndWallTime`, logs, audit columns                                  | No                     |

`LiDARFrame.StartTimestamp` is the frame's earliest point timestamp. The pipeline passes it to
`Tracker.Update`, so it is the tracker's frame clock. `WorldCluster.TSUnixNanos` is the
acquisition time of the cluster's first member point, between the frame start and roughly one
rotation (100 ms at 10 Hz) later.

## What capture time is, per source

The parser resolves one packet time per packet; every point in it takes that time plus its
channel's firetime offset. The table is the audit of `resolvePacketTime` (plan item F1). The
tests named in the last column pin each row, packet by packet and end to end through the real
frame builder into the tracker.

| Source and mode                   | Packet time                                                                                             | Tracker interval                                                                                                     | Tests                                                                                                  |
| --------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| PCAP replay, any mode             | The packet's capture timestamp                                                                          | The capture interval: host reception spacing at capture, not the sensor clock                                        | `TestCaptureTimeOverridesEveryMode`, `TestTrackerIntervalFollowsCaptureTimeOnReplay`                   |
| Live, `system` (the default)      | Host `time.Now()` during parsing                                                                        | Arrival spacing, including socket, queue and parser latency; moves with any host clock step                          | `TestSystemTimeModeIsHostArrivalTime`, `TestTrackerIntervalIsHostArrivalInSystemTimeMode`              |
| Live, `lidar` (native)            | Sensor `DateTime` plus the microsecond field                                                            | The sensor's interval exactly, monotonic across UTC seconds                                                          | `TestLiDARModeFollowsTheSensorClockAcrossTheSecond`, `TestTrackerIntervalFollowsSensorTimeInLiDARMode` |
| Live, `gps`, `internal`, PTP enum | Parser boot time plus the microsecond field                                                             | The sensor's interval inside one second; at each second boundary a step back of about 0.9 s, then one short interval | `TestBootOffsetModesStepBackAtEverySecond`, `TestBootOffsetModesStepBackAndTheTrackerAbsorbsIt`        |
| Live, PTP or GPS, field stalled   | Host `time.Now()` once the field has repeated more than 10 times, and for the rest of the parser's life | Switches from sensor-derived to arrival spacing mid-stream                                                           | `TestPTPStaticFallbackIsPermanent`                                                                     |

Consequences:

- **Only `lidar` mode follows the sensor clock.** Live runs in the default `system` mode are not
  temporally reproducible: arrival jitter enters every prediction interval.
- **Replay reproduces the capture, not the sensor.** A PCAP's timestamps are the capturing host's
  reception times, and they take precedence over every mode. Selecting `lidar` mode for a replay,
  as the server and `lidar-bench` do, changes nothing while the reader supplies capture times.
- **The boot-offset modes cannot guarantee a monotonic clock.** The Pandar40P microsecond field is
  the fraction of the UTC second, not time since boot. With a stream starting at .780 s the frame
  straddling the second steps back 0.88 s; the tracker predicts across zero, re-anchors, and sees
  0.08 s before the 0.1 s period resumes. Each second boundary costs between one and two
  rotations of prediction, depending on where the wrap falls in the frame: 0.12 s here.
- **The PTP and GPS static fallback latches.** Its counter never resets, so a stream that stalls
  once stays on host arrival time.
- `LIDAR_TIMESTAMP_MODE` accepts `system`, `gps`, `internal` and `lidar`; there is no `ptp` value.
  After a PCAP replay the server restores its shared parser to `system`, whatever the variable
  selected.

The last three are recorded rather than changed here; see [what remains](#what-remains).

## Which quantities use which clock

| Quantity                                                    | Clock                                                                       | Owner                                                                      |
| ----------------------------------------------------------- | --------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| Prediction interval `dt`                                    | Capture: frame clock, or measurement time under `MeasurementTimePrediction` | `l5tracks` `frameInterval`, `trackInterval`                                |
| Association implied-speed check                             | Capture: the same `dt`                                                      | `l5tracks` `associate`                                                     |
| Coast age, longest unobserved interval, capture-time expiry | Capture                                                                     | `l5tracks` `time_domain.go`                                                |
| Miss-count expiry (`max_misses`, `max_misses_confirmed`)    | Neither: counts frames that reached the tracker                             | `l5tracks` `Update`, `AdvanceMisses`                                       |
| Deleted-track grace and render fade                         | Capture                                                                     | `cleanupDeletedTracks`; the L9 adapter passes the frame timestamp          |
| Track history (trail) timestamps                            | Capture: acquisition time for observed points, frame time for coasted ones  | `l5tracks` `update`, `Update`                                              |
| `LastMeasurementUnixNanos`, persisted `ts_unix_nanos`       | Capture: the cluster's acquisition time, falling back to the frame time     | `l5tracks` `measurementForMode`                                            |
| Track duration, heading episodes                            | Capture                                                                     | `l5tracks`                                                                 |
| OBB heading smoothing, extent beliefs                       | Neither: indexed by observation, not by time                                | `l5tracks` `update`, `heading_extent.go`                                   |
| L3 warm-up, freeze and thaw                                 | Capture on the pipeline path (`ProcessFramePolarWithMaskAt`)                | `l3grid`; the settling-eval tool's `ProcessFramePolar` path uses wall time |
| L3 settling progress and elapsed, as displayed              | Wall                                                                        | `l3grid` `SettlingStatus`                                                  |
| Replay throttle                                             | Wall; replays only, never analysis mode                                     | `pipeline` `shouldThrottleFrame`                                           |
| Replay pacing and backoff                                   | Wall                                                                        | `l1packets/network` `ReadPCAPFileRealtime`                                 |
| Stalled-frame cleanup in the live frame builder             | Wall (`EndWallTime`); disabled for offline replay                           | `l2frames` cleanup                                                         |
| Stage timing, database pruning, logs, audit columns         | Wall                                                                        | `pipeline`, storage                                                        |

Heading and extent smoothing being observation-indexed means they cannot see wall time, but it
also means they are frame-rate dependent: a 20 Hz sensor smooths twice as fast in time as a 10 Hz
one. Any time-based smoother (retrospective refinement, G-SMO-1) must take its intervals from the
capture-time history timestamps.

## The estimator boundary in code

[internal/lidar/l5tracks/time_domain.go](../../../internal/lidar/l5tracks/time_domain.go) states
the boundary. Nothing in the package's non-test sources reads the wall clock:
`TestL5TracksDoesNotReadTheWallClock` parses them and fails on `time.Now`, `Since`, `Until`,
`Sleep`, `After`, `AfterFunc`, `Tick`, timers and tickers, on a dot-import of `time`, and on an
import of `timeutil`, whose `Clock` exists for runtime code and would hand the tracker a clock of
its own. A companion test proves the scan can fail.

The tracker keeps two capture clocks, deliberately distinct:

- **Frame clock**, `Tracker.LastUpdateNanos`: the timestamp of the last `Update`.
- **State clock**, `TrackedObject.StateUnixNanos`: the capture time the track's Kalman state
  refers to. By default it equals the frame clock after every frame.

Per track, `LastObservedUnixNanos` is the state time at the last accepted observation,
`CoastAgeSecs` is the capture time since then (zero on an observed frame) and `MaxCoastAgeSecs`
is the longest unobserved interval the track has closed. `LastMeasurementUnixNanos` remains the
evidence field: the cluster's acquisition time.

### Non-monotonic and duplicate timestamps

A frame stamped with the previous frame's time predicts across zero, as it always did, and is
counted. A frame stamped earlier than the previous frame used to become a negative interval: the
state ran backwards and process noise was subtracted from the covariance, which can leave it
indefinite. It now predicts across zero, the step and its size are counted, and the frame clock
re-anchors at the new timestamp, as it always did, so a stepped clock resumes ordinary intervals
on the next frame. Re-anchoring rather than holding a high-water mark was chosen because the
common causes are clock steps and the boot-offset wrap above, where holding would freeze
prediction for up to a second, or for as long as an NTP correction. The cost is that a single
out-of-order frame over-predicts the following interval by at most one frame, bounded by
`max_predict_dt`.

### Gaps

The tracker is called once per frame that produced clusters. A capture-time gap between calls
has four sources:

| Source                  | When                                                            | Miss count advanced |
| ----------------------- | --------------------------------------------------------------- | ------------------- |
| Frames with no clusters | Any quiet moment: the pipeline returns before `Tracker.Update`  | No                  |
| Throttled frames        | Non-analysis replays faster than `MaxFrameRate`                 | No                  |
| Capture joins           | A revolution straddling a join between capture files is dropped | No                  |
| Transport loss          | Packets or frames lost between sensor and pipeline              | No                  |

The first is the common one. Over the whole of kirk0 the tracker received 706 of 832 frames and
saw five gaps longer than `max_predict_dt`, the longest 2.0 s. Because none of these advance the
miss count, the frame-count expiry rule measures frames with clusters, not elapsed time: a track
whose object has left an otherwise empty scene is neither aged nor predicted until something else
appears.

By default a gap longer than `max_predict_dt` (0.5 s) is clamped, so a coasting track is predicted
only part of the way. The unclamped gap is now recorded: `TimeDomainStats` holds the last and
largest gap and the number clamped, and replay writes them to `replay_manifest.json` as
`time_domain`.

## Default-off options

All are Go-level `TrackerConfig` options, deliberately not tuning keys, so the tuning fingerprint
and the committed perf baselines do not move. The replay harness reaches two of them by name.

| Option                                           | Replay experiment     | Effect                                                                                                                                                                                                                |
| ------------------------------------------------ | --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MaxCoastSecsTentative`, `MaxCoastSecsConfirmed` | None yet              | Expire a track once its capture-time coast age reaches the bound, alongside the miss count. Checked before association, so an observation arriving after the bound seeds a new track instead of reviving the old one. |
| `CaptureGapPrediction`                           | `capture_gap_predict` | Predict across the whole gap in steps of at most `max_predict_dt`, so the covariance cap applies per step as it would across ordinary frames. Bounded at 1,200 steps; the remainder is counted, not predicted.        |
| `MeasurementTimePrediction`                      | `measurement_time`    | Predict each associated track from the frame time to its cluster's acquisition time before the update (question Q3). Gating and assignment still use the frame-time prediction.                                       |

The expiry bounds have no replay experiment because no value is yet justified by evidence;
wiring one should wait for bounds chosen against held-out occlusion scenes.

## Measurement time versus frame time (Q3)

By default every cluster in a frame is treated as observed at the frame's start. An object scanned
late in the sweep was measured up to one rotation later than the filter assumes; near the azimuth
wrap that offset changes abruptly between consecutive frames and reads as position noise.
`MeasurementTimePrediction` moves the update to the cluster's own time. On a synthetic object at
5 m/s whose scan offset alternates between 0 and 90 ms, mean innovation falls from 0.28 m to under
a millimetre.

The kirk0 smoke run (`TestTimeDomainExperimentsOnKirk0`, capture seconds 20 to 28) confirms the
wiring end to end and logs the residual bands. Its moving band holds 15 samples, which is not
evidence either way; the question is answered on the corpus:

```bash
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline -pcap-root "$LIDAR_PCAP_DIR" \
  -out /tmp/q3-default
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline -pcap-root "$LIDAR_PCAP_DIR" \
  -out /tmp/q3-measurement-time -experiment measurement_time
```

Compare each case's `tracking_baseline.json` between the two outputs, with attention to the
moving speed bands, and read each case's `time_domain` summary before trusting a temporal result.

## Replay implications

- **The estimator is replay-deterministic.** `TestReplayIsInvariantToWallClockPacing` replays a
  kirk0 window with moving vehicles unpaced, paced at 1x and paced at 2x: the per-frame track
  decisions (3,148 track-frames, IDs canonicalised) and the schema-2 tracking baseline are
  identical. Pipeline tests show wall-clock fields that jump, stall and run backwards leave the
  tracker's inputs and tracks unchanged, and that a capture gap under a regular wall clock is seen.
- **Default behaviour did not move.** A full kirk0 replay on this change is byte-identical to its
  base commit: tracking baseline and every frame's track decisions, 831 frames. kirk0 has no
  backward or duplicate frame timestamps, so the new guard never engages on it.
- **Wall-clock dependences that remain in replay** change which data reach the estimator, never
  the time it believes has passed:
  - the throttle, in non-analysis replays, drops frames by wall-clock spacing;
  - the paced reader, once more than 30 s behind, forgives the deficit and in doing so skips the
    packet it was handling;
  - the live frame builder finalises stalled frames by wall time (disabled offline).

  Analysis replays and `replayeval` run unpaced, unthrottled and without cleanup timers, which is
  why their output is reproducible.

## More than one sensor

Each sensor's capture clock is its own. Two sensors in `system` mode share the host clock but not
their latency; in `lidar` mode they share UTC only as well as their time sources agree. A fused
estimate therefore needs every observation mapped onto one timeline before prediction, which is a
property of L7 Scene, not of one sensor's L5. The per-track state clock is the hook: a track
predicted to each observation's own time, as `MeasurementTimePrediction` does within one sensor,
can accept observations from a second sensor once their times are aligned. Cross-sensor alignment
is an open question for L7; see
[multi-model ingestion](multi-model-ingestion-and-configuration.md#open-questions).

## What remains

| Item                                                                                                                                              | Owner                                                 |
| ------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Choose capture-time expiry bounds from held-out occlusion scenes; then wire an experiment                                                         | State-estimation plan, Sprint 0.5.2.2 continuity work |
| Answer Q3 on the corpus with `-experiment measurement_time`                                                                                       | State-estimation plan, question Q3                    |
| Measure `capture_gap_predict` on reacquisition across gaps                                                                                        | State-estimation plan, Sprint 0.5.2.2                 |
| Inject `timeutil.Clock` into the throttle, replay pacing and cleanup; stop the paced reader skipping the packet it forgives on                    | Clock plan, phases A and B (v0.5.4 remainder)         |
| Boot-offset modes: treat the microsecond field as the fraction of a second, release the static fallback, restore the configured mode after replay | Clock plan, item C2                                   |
