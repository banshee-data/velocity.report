# LiDAR time-domain model

Which clock each part of the LiDAR pipeline runs on, why the estimator runs on capture time alone,
and what that means for replay, transport gaps and more than one sensor.

- **Status:** Implemented for the estimator boundary (L5 and the timestamps that feed it), with
  the occlusion-continuity primitives built on it (default-off). Clock injection into runtime code
  is the separate clock-abstraction remainder.
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
and the committed perf baselines do not move. The replay harness reaches most of them by name.

| Option                                           | Replay experiment     | Effect                                                                                                                                                                                                                |
| ------------------------------------------------ | --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MaxCoastSecsTentative`, `MaxCoastSecsConfirmed` | None yet              | Expire a track once its capture-time coast age reaches the bound, alongside the miss count. Checked before association, so an observation arriving after the bound seeds a new track instead of reviving the old one. |
| `CaptureGapPrediction`                           | `capture_gap_predict` | Predict across the whole gap in steps of at most `max_predict_dt`, so the covariance cap applies per step as it would across ordinary frames. Bounded at 1,200 steps; the remainder is counted, not predicted.        |
| `MeasurementTimePrediction`                      | `measurement_time`    | Predict each associated track from the frame time to its cluster's acquisition time before the update (question Q3). Gating and assignment still use the frame-time prediction.                                       |
| `OcclusionContinuity`                            | See below             | Absence explanation, capture-time coast uncertainty, per-class capture-time coast bounds and the reacquisition guard: [coast, existence and expiry](#coast-existence-and-expiry).                                     |

The simple expiry bounds have no replay experiment because no value is yet justified by
evidence. The per-class bounds below are wired, with stated starting values, so that the evidence
can be gathered.

## Coast, existence and expiry

Sprint 0.5.2.2 asks the tracker to keep an object's existence hypothesis through a bounded
unobserved interval without mistaking it for observation. The code is
[continuity.go](../../../internal/lidar/l5tracks/continuity.go).

### Existence is not observation

Every instant a live track is updated carries a support token, recorded on its history point
(`TrackPoint.Support`) and as `TrackedObject.LastSupport`. The tokens are the closed vocabulary of
the [behaviour plan's Section 7.3](../../plans/lidar-behaviour-analytics-plan.md#73-observation-support),
identical to `l8behaviour.SupportState`; a test reads them from the plan. L5 cannot import L8, so
a consumer maps by token, never by numeric value. This is always on and changes no estimate.

| Token                             | When l5tracks records it                                                                                  |
| --------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `observed`                        | A cluster was associated at this instant                                                                  |
| `coasted`                         | Unobserved, and absence explanation is off (the default)                                                  |
| `out_of_fov`                      | Unobserved, and the prediction lies outside the configured `SensorCoverage`                               |
| `occluded_inferred`               | Unobserved, and this frame's clusters wholly nearer the sensor cover half the predicted footprint's angle |
| `missed_unknown`                  | Unobserved with neither explanation                                                                       |
| `cluster_merged`, `cluster_split` | Declared for the vocabulary; not claimed. The merge and split flags are size ratios, not ownership        |

`TrackedObject.Existence` is a third lifecycle, beside `TrackState` (should the track exist?) and
the solid-body `EstimationState` (is the pose believed?): `observed`, `coasting`,
`coasting_explained` (`occluded_inferred` or `out_of_fov`), `coasting_unexplained` and `expired`.
Every deletion records an `ExpiryReason`: `misses`, `coast_age`, `missed_unknown`,
`occluded_inferred`, `out_of_fov` or `non_finite`. `SolidBodyEstimate.Support.Instant` carries the
token to the estimate contract.

`Tracker.ContinuityStats` counts, per window, support by track-instant, expiries by reason, coast
age at expiry and at reacquisition, tracks born and those of them confirmed, and the guard's
refusals. The window restarts with `BeginTrackingBaseline`, so a replay's figures cover its scoring
window; `replay_manifest.json` carries them as `continuity`.

### Options

`TrackerConfig.OcclusionContinuity`; its zero value is the shipped tracker, and
`DefaultOcclusionContinuity` switches everything on with starting values.

| Switch                 | Replay experiment      | Effect                                                                                                                                                                            |
| ---------------------- | ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ExplainAbsence`       | `coast_support`        | Classify every unobserved instant. Diagnostic only: tracks and baseline are byte-identical to the default replay's                                                                |
| `CaptureTimeInflation` | `coast_time_inflation` | Replace the per-missed-frame `occlusion_cov_inflation` with the class rate times the unobserved capture time the frame added. Rate zero is pure CV growth                         |
| `ClassCoastBounds`     | `class_coast_bounds`   | Replace the miss count with capture-time bounds: tentative, else the class's unexplained or, while `occluded_inferred`, explained allowance. Implies explanation                  |
| `ReacquisitionGuard`   | `reacquisition_guard`  | A coasting track may not reclaim a cluster whose extent does not fit its believed body, nor choose between ambiguous candidates; its implied-speed check divides by its coast age |
| All four               | `occlusion_continuity` | The bundle. It does not imply `capture_gap_predict`; name both for prediction in capture time as well                                                                             |

The class is the L6 label the pipeline writes back through `UpdateClassification`, mapped by
`MotionClassForLabel`, at the strength of its confidence: each allowance and rate interpolates
linearly from `unknown`'s row toward the class's, per Section 5.5's first rule. A track not yet
classified is `unknown`. Starting values, not evidence-chosen:

| Class           | Unexplained | Explained | Inflation    |
| --------------- | ----------- | --------- | ------------ |
| `unknown`       | 1.0 s       | 2.0 s     | 3.0 m² / s   |
| `rigid_vehicle` | 1.5 s       | 3.0 s     | 3.0 m² / s   |
| `two_wheeler`   | 1.5 s       | 3.0 s     | 2.0 m² / s   |
| `pedestrian`    | 1.5 s       | 4.0 s     | 1.0 m² / s   |
| Tentative, any  | 0.3 s       | 0.3 s     | as its class |

The unexplained allowance is the shipped confirmed miss budget (15 frames) in capture time at
10 Hz. Every rate is below the 5 m²/s the shipped inflation adds at 10 Hz, so at the nominal rate
a coasting track's association discount (gap analysis S3) is never deeper than today; a test
holds that per class and per miss. The bound is judged before association against the last
instant's explanation and after association against this frame's, so an explanation that
disappears (the occluder moved on, or the prediction left its shadow, and nothing is there)
lapses the hypothesis on that instant.

Three findings shaped the guard. First, the shipped implied-speed check divides a pairing's
distance from the prediction by one frame interval, so at 10 Hz and 30 m/s no coasting track can
be reacquired more than 3 m from its prediction, however long it coasted. That binds before the
Mahalanobis gate and before the 5 m jump guard, and it made growing uncertainty pointless: in the
synthetic full-occlusion scene a car whose partial views on entry left its velocity 13 % low
re-emerged 2.9 m ahead of its prediction after 1.4 s and was refused. Under the guard a
reacquiring track is judged over its coast age. Second, the believed extent must not be the
running mean of spans, which is what `believedLongExtent` falls back to with the axis path off
(the default): partial views drag it down, and a whole view of the same car can then read as a
merge (a unit test pins the case). The guard keeps its own corroborated-maximum long-extent
belief, fed only while it is on.
Third, two fragments of one body are a split, not an identity question: two candidates closer
together than the believed extent are not treated as ambiguous.

Occlusion is explained from foreground clusters only. A parked vehicle absorbed into the
background, a building or a pole produces no cluster, so an object behind one reads as
`missed_unknown`. The sensor origin (`SensorX`, `SensorY`) is zero while the pipeline runs in
sensor coordinates with a nil pose; a site pose must set it. `SensorCoverage` is unset by default,
so `out_of_fov` is never claimed until a site configures it.

### Synthetic evidence

[continuity_scenario_test.go](../../../internal/lidar/l5tracks/continuity_scenario_test.go)
generates scenes by ray geometry with known truth: a sensor at the origin, the road user in a lane
12 m out, and a stationary occluder 6 m out sized to hide it. Seven scenes for each of a car
(10 m/s), cyclist (5 m/s) and pedestrian (1.4 m/s), under `DefaultOcclusionContinuity` with a
correct L6. Each asserts that support agrees with why the generator produced no cluster (an
occlusion is never claimed where there was none, and a real one is recognised on at least 80 % of
hidden instants), that history points carry their instant's token, the identity rule, that no
hypothesis is ever live beyond its instant's allowance, and that the truth lies inside the 99 %
position ellipse at 0.5 s and 1 s of coast. A companion test shows the checks catch the shipped
tracker's failures.

| Scene                 | Option                                                                                       | Shipped, same scene                                                    |
| --------------------- | -------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| Full occlusion        | Identity kept, all three; covered to the reacquisition                                       | Identity lost, all three: the car to the 3 m limit, the rest to misses |
| Hidden stop           | Expires with its reason (explained bound, or the prediction leaves the shadow); new identity | New identity                                                           |
| Turn during occlusion | Expires once the prediction leaves the shadow; the car's truth leaves the ellipse at 1.5 s   | Expires by misses                                                      |
| Sparse returns        | Identity kept; misses are `missed_unknown`, never an occlusion                               | Identity kept                                                          |
| Re-entry              | Expires `out_of_fov` within the unexplained bound; re-enters as a new identity               | New identity                                                           |
| Distractor            | Identity kept; never mixed with the distractor                                               | Car and pedestrian handed to the distractor's track                    |
| Departure             | Expires `missed_unknown` within the unexplained bound                                        | Expires by misses                                                      |

An unclassified pedestrian behind the same occluder is let go at `unknown`'s two seconds and seen
again under a new identity, where a classified one is held. The pedestrian ellipses are loose
(d² near 0.04 at 2 s of coast): coverage holds, calibration is not claimed, and G-UNC-1 remains
the gate for that.

### Replay evidence on kirk0

The whole of kirk0 after a 20 s warm-up (631 recorded frames), each arm the default replay with
experiments named. Label-free: `ContinuityStats` over the scoring window, and the recording's
confirmed tracks. The default arm is byte-identical to the base branch, and `coast_support` to the
default, in schema-2 baseline and every frame's track decisions.

| Arm                    | Born | Never confirmed | Reacquired | Expired by                     | Longest coast at expiry | ≥ 3 s | Confirmed tracks, median life |
| ---------------------- | ---- | --------------- | ---------- | ------------------------------ | ----------------------- | ----- | ----------------------------- |
| default                | 113  | 67 %            | 243        | misses 113                     | 6.4 s                   | 8     | 41, 5.8 s                     |
| `coast_time_inflation` | 113  | 65 %            | 200        | misses 114                     | 6.4 s                   | 8     | 45, 4.8 s                     |
| `class_coast_bounds`   | 122  | 65 %            | 262        | missed_unknown 82, occluded 40 | 2.9 s                   | 0     | 46, 4.2 s                     |
| `reacquisition_guard`  | 117  | 62 %            | 175        | misses 117                     | 6.3 s                   | 7     | 47, 4.4 s                     |
| `occlusion_continuity` | 124  | 61 %            | 125        | missed_unknown 87, occluded 37 | 2.8 s                   | 0     | 52, 3.5 s                     |

Reading it:

- **The miss count lets hypotheses outlive their evidence.** Frames with no clusters never reach
  the tracker, so by default eight tracks were still alive 3 to 6.4 s of capture time after their
  last observation. Capture-time bounds end every one by 2.9 s.
- **Most absences are unexplained.** `coast_support` splits the default's 1,263 coasted instants
  into 373 `occluded_inferred` (30 %) and 890 `missed_unknown`. Some of the latter are occlusion
  by static structure, which is not yet modelled.
- **The guard mostly refuses small hypotheses a vehicle's cluster.** About 60 % of the pairings
  it refused as too large (logged over the whole replay) were L6 `bird` or `dynamic` tracks
  believing in under a metre, 2 to 4 m from a vehicle-sized cluster; most of the rest look like
  merges (a 5.1 m belief against a 10.5 m cluster). Its
  failures are its belief's: one car track carried a merge-inflated 10.4 m belief and refused
  2.4 m views. The counters count forbidden pairings, not changed decisions.
- **`capture_gap_predict` adds nothing on top of the bundle here.** Every track spanning kirk0's
  long empty-frame gaps is expired by capture time before whole-gap prediction could act on it.
- **Direction is not established.** Fewer never-confirmed hypotheses and more confirmed tracks,
  with shorter lives and fewer reacquisitions, fit less identity mixing and more fragmentation
  of real objects equally. One capture without labels cannot separate them.

To compare on the S2 corpus (put `-out` on a different device from the captures):

```bash
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline -pcap-root "$LIDAR_PCAP_DIR" \
  -out /tmp/continuity-default
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline -pcap-root "$LIDAR_PCAP_DIR" \
  -out /tmp/continuity-option -experiment occlusion_continuity
```

Compare each case's `continuity` in `phase0-summary.json`, then attribute with the single
switches (`coast_support`, `coast_time_inflation`, `class_coast_bounds`, `reacquisition_guard`)
and with `occlusion_continuity,capture_gap_predict`. Judge identity against labelled tracks before
any value is chosen. The in-repo smoke run is `TestOcclusionContinuityExperimentsOnKirk0`.

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
| Choose the continuity values (class bounds and rates, `MaxCoastSecs*`) from held-out occlusion scenes, comparing `occlusion_continuity` on S2     | State-estimation plan, Sprint 0.5.2.2 continuity work |
| Explain occlusion by static structure from the L3 background range at the predicted azimuth                                                       | State-estimation plan, Sprint 0.5.2.2 continuity work |
| Carry the support token into VRLOG and the visualiser trail; fix association-cost bias (S3) and identity (K4/S2) separately                       | Visualiser trails plan; state-estimation plan         |
| Answer Q3 on the corpus with `-experiment measurement_time`                                                                                       | State-estimation plan, question Q3                    |
| Measure `capture_gap_predict` on reacquisition across gaps                                                                                        | State-estimation plan, Sprint 0.5.2.2                 |
| Inject `timeutil.Clock` into the throttle, replay pacing and cleanup; stop the paced reader skipping the packet it forgives on                    | Clock plan, phases A and B (v0.5.4 remainder)         |
| Boot-offset modes: treat the microsecond field as the fraction of a second, release the static fallback, restore the configured mode after replay | Clock plan, item C2                                   |
