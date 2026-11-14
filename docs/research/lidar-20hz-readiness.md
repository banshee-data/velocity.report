# Pandar40P at 20 Hz: pipeline readiness and expected gains

A 20 Hz deployment requires a fresh rotation every 50 ms, complete processing capacity, and
observable loss and latency through the deployed path. This source audit identifies the work
needed for a proposed 48 ms processing budget and separates that target from demonstrated
performance. It changes no code, tuning values, sensor settings or production behaviour.

- **Status:** Findings and proposed qualification work; no benchmark or physical experiment run.
- **Canonical:** [LiDAR architecture](../lidar/architecture/LIDAR_ARCHITECTURE.md).
- **Related:** [Performance gate](../lidar/operations/performance-regression-testing.md),
  [time domains](../lidar/architecture/time-domain-model.md),
  [observation and async tracking plan](../plans/lidar-cluster-observation-log-and-async-tracking-plan.md),
  [SRAC maths PR](https://github.com/banshee-data/velocity.report/pull/1023).
- **References:** `HesaiPandar40P` in [the bibliography](../../data/maths/references.bib).
- **Provenance:** Source inspected at `436a5413a573e78354fbee9da944195a355d531e`.
  External specification: Hesai manual 402-en-250410, sections 1.5 and 4.2.
  Timing and rate calculations below are derived, not measured on a Raspberry Pi.

## Findings

The sensor supports 20 Hz. The current live pipeline has no identified universal 5 Hz throttle:
its replay catch-up throttle explicitly bypasses live input and analysis mode. The important
5 Hz references describe successful observations in an older cohort and decimated scene exports.
Neither establishes the current end-to-end rate. There are nevertheless frame-count assumptions,
export retention, latency accounting and qualification gaps to address before adopting 20 Hz.

Two metrics also hard-code 10 Hz: `SpatialCoverage` divides observations by duration ×10 and
clamps at one; `MaxOcclusionFrames` converts gaps using 100 ms. Correct those contracts before
using them to judge a 20 Hz deployment. The former can report full coverage at only about half
the new cadence, including in downstream quality summaries.

The configured processing budget is **98 ms**, appropriate to the current 10 Hz target, not 48 ms.
Changing that field changes a performance acceptance threshold; it does not change the motor
speed, scheduler or processing capacity. The two existing timing paths cover different work.
Neither alone proves that the production pipeline, storage and clients meet the proposed deadline.

Recommend retaining a 20 Hz acquisition/processing goal and treating 48 ms as a proposed service
budget. First instrument and qualify the actual target device and workload. Decide separately
whether behaviour analysis, future SRAC evaluation, persisted products and visual refresh each
need a fresh result every 50 ms. Optional consumers must not silently lower observation coverage.

## Evidence

### Sensor cadence and point density

The [manufacturer manual](https://www.hesaitech.com/wp-content/uploads/2025/04/Pandar40P_User_Manual_402-en-250410.pdf)
(section 1.5, printed pages 14–15) specifies 10 or 20 Hz, horizontal resolution 0.2° or 0.4°
respectively, and 720,000 valid points/s for single return or 1,440,000 for dual return.
The corresponding point-cloud data rates are 18.84 and 37.68 Mbit/s. These are device specifications,
not measured useful foreground rates or application throughput.

Dividing the specified single-return rate by rotation rate gives the following idealised counts.
Clipping, invalid returns and packet loss reduce retained points; dual returns are not independent
views and do not restore the missing horizontal directions.

| Quantity                                              | 10 Hz     | 20 Hz     | Consequence                                                            |
| ----------------------------------------------------- | --------- | --------- | ---------------------------------------------------------------------- |
| Rotation period                                       | 100 ms    | 50 ms     | Twice as many completed-rotation opportunities                         |
| Ideal single-return points per rotation               | 72,000    | 36,000    | Roughly half the points per individual object snapshot                 |
| Horizontal spacing at 50 m, small-angle approximation | 0.175 m   | 0.349 m   | Narrow/distant objects need renewed detection and extent qualification |
| Ideal input points per second, same return mode       | Unchanged | Unchanged | Packet/point work need not double; per-frame work may                  |

Confirm the actual device configuration and timestamp-derived rotation spacing in a capture.
Replaying a 10 Hz capture twice as fast is a wall-clock stress test, not a 20 Hz sensor experiment:
it preserves 10 Hz capture intervals and spatial sampling. Dropping every other 20 Hz rotation
is a useful temporal comparator, but does not reproduce the denser 10 Hz scan.

### Rate and latency audit

Paths below refer to the frozen source revision above; function/setting names identify the
review points without relying on line numbers that will drift.

| Area and source                                                                                                                        | Current behaviour                                                                                                                                                                            | Required follow-up                                                                                                                                                                                                                                                         |
| -------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Config budget](../../internal/config/profile.go), [defaults](../../config/tuning.defaults.json)                                       | `DefaultFrameBudgetMs` and `pipeline.frame_budget_ms` are 98                                                                                                                                 | Add a checked-in 48 ms experimental configuration; recapture matching baselines when the fingerprint changes. Do not claim the threshold creates capacity                                                                                                                  |
| [Live/replay wiring](../../internal/cmd/server/radar.go), [callback](../../internal/lidar/pipeline/tracking_pipeline.go)               | `MaxFrameRate: 25` is replay catch-up only; `shouldThrottleFrame` bypasses live and analysis. L3 runs before that throttle                                                                   | Preserve this distinction. Count assembled, processed and throttled rotations per mode; do not “fix” live cadence by raising this value                                                                                                                                    |
| [Frame assembly](../../internal/lidar/l2frames/frame_builder.go), [completion](../../internal/lidar/l2frames/frame_builder_cleanup.go) | Rotation-driven completion; 10,000-point completion threshold; configured minimum 1,000; server timeout 500 ms, cleanup 250 ms, backfill field defaults to 100 ms but has no production read | Validate half-density scans, return modes, partial rotations and packet disorder. Measure wrap-to-ready latency and expiry fallback; timeout/cleanup are not a universal 4 or 5 Hz processing loop; resolve the inert backfill contract before treating it as a wait bound |
| Frame callback queue in the same files                                                                                                 | One serial callback worker; bounded channel; live drops on a full channel, analysis can block                                                                                                | Measure queue residence and drops, not just callback duration. Preserve ordered L5 updates if stages are separated. Larger buffers absorb bursts but increase stale-data age                                                                                               |
| [Offline benchmark](../../internal/lidar/lidarbench/lidarbench.go), `analysisFrameBuilder.processCurrentFrame`                         | Per-frame timer surrounds L3 through configured L4–L6 work, on its own offline path. Aggregate parse/wall timers are separate                                                                | Keep this useful algorithm gate; add deployed-path timing for ingest, assembly, queueing, persistence, L8 and publication under concurrent load                                                                                                                            |
| [Live stage timer](../../internal/lidar/pipeline/frame_timer.go) and callback                                                          | Enabled in benchmark mode; starts after L3 and replay throttle; total sums recorded stages; slow alert is fixed at 50 ms                                                                     | Label its coverage. Align alert and budget semantics; add a full callback/service timer and acquisition-to-result age; do not compare this partial total directly with offline whole-frame totals                                                                          |
| [Track defaults](../../config/tuning.defaults.json), [covariance](../../internal/lidar/l5tracks/tracking_covariance.go)                | Hits/misses, histories and heading alpha include frame-based choices. `addProcessNoise` already multiplies shipped noise by elapsed `dt`                                                     | Preserve time-scaled noise. Audit the frame-based values and observation-dependent updates; avoid a blanket doubling of all tuning values                                                                                                                                  |
| [Smoother](../../internal/lidar/l5tracks/smoother.go)                                                                                  | Capture-lag capacity explicitly allows 20 Hz; frame-count lag is also supported                                                                                                              | Verify 20 Hz, jitter and gaps with the selected lag contract. A count-lag halves its duration; a seconds-lag retains latency and roughly doubles stored steps                                                                                                              |
| [Observation capture policy](../../internal/cmd/server/lidar_observations.go)                                                          | Upstream queue age uses channel capacity ×100 ms; shedding threshold is 50 ms                                                                                                                | Revisit the declared age/loss budget against measured 20 Hz residence. A period-times-capacity calculation is not a service-delay bound under stalls                                                                                                                       |
| [VRLOG policy](../../internal/lidar/storage/vrlog/policy.go)                                                                           | Maximum frame-rate assumption already 20 Hz; batch timing is a durability policy                                                                                                             | Verify actual writer throughput, flush stalls and declared crash-loss bounds; batching frequency is not observation frequency                                                                                                                                              |
| [Publisher](../../internal/lidar/l9endpoints/publisher.go)                                                                             | Bounded publication/client queues expose drop counters; periodic background snapshots every 300 rotations by default                                                                         | Qualify slow clients and recording independently. 300 rotations become 15 s at 20 Hz versus 30 s at 10 Hz; choose intended time or count semantics                                                                                                                         |
| [Scene CLI](../../internal/cmd/scene/scene.go), [publisher script](../../tools/s2-archive/publish-scenes.py)                           | CLI stride defaults to 1, but publishing script sets `STRIDE = "2"`                                                                                                                          | Explicitly choose retention by product. Script output is nominally 5 Hz from 10 Hz and 10 Hz from 20 Hz. Use stride 1 for a complete 20 Hz rotation product                                                                                                                |
| [Track gap metric](../../internal/lidar/l5tracks/tracking_metrics.go)                                                                  | Gap threshold is 200 ms; `MaxOcclusionFrames` divides by 100 ms; `SpatialCoverage` uses duration ×10 and clamps at 1                                                                         | Replace hard-coded count/coverage denominators with a versioned cadence/opportunity contract. Decide whether 200 ms means physical duration; preserve actual missing counts and do not hide losses through clamping                                                        |

The [behaviour plan's observation budget](../plans/lidar-behaviour-analytics-plan.md#3-the-observation-budget)
reports roughly 5 Hz effective observations from a historical 10 Hz cohort. The tracked object
can miss a sensor rotation through occlusion, detection or association loss even when processing
keeps up. Instrument that funnel before assigning blame to a timer. Doubling acquisition does
not guarantee 20 successful updates/s for each actor, especially with fewer points per rotation.

Older [playback-quality findings](../lidar/operations/playback-speed-vs-track-quality.md) describe
throttling behaviour from their historical revision. Current `shouldThrottleFrame` semantics
and tests are the authority for this audit; those older observations do not prove a present
analysis-mode or live cap. Synthetic gRPC defaults to 10 Hz in
[synthetic.go](../../internal/lidar/l9endpoints/synthetic.go), which is a test/demo source, not a
physical-input limit. A fast UI refresh likewise cannot establish new sensor observations.

## The 48 ms budget and the missing measurements

At 20 Hz the inter-arrival period is 50 ms. A 48 ms serial service time leaves only 2 ms, or 4%,
per period for variability. Define the proposed budget around complete per-rotation processing
from dequeue to its required analytical output, including synchronous storage/publication work.
Also measure assembly, queue residence and asynchronous completion independently. If L3 is moved
onto another worker, its time still belongs in end-to-end age and its throughput needs its own gate.

Before any finite-buffer drops, for one serial worker with service times Sₙ and regular 50 ms arrivals, waiting time obeys
Wₙ₊₁=max(0,Wₙ+Sₙ−0.05), in seconds. This derived recurrence shows why a p99 below 48 ms or a small
fraction of frames over budget cannot alone bound backlog. A few large stalls can dominate it.
The existing percentage-over-budget gate is a useful statistical acceptance check, not a
worst-case execution-time proof. Require mean capacity with headroom, tails, maximum age,
consecutive misses, queue depth, drops and recovery after bursts. Set allowable miss/drop/age
limits before qualification; no values have been measured or adopted by this report.

Publish distinct timestamps and measures:

1. First and last point acquisition, frame ready/enqueued, worker start, analytical result ready,
   durable write acknowledgement where applicable, and client receipt.
2. Capture-time span, assembly wait, queue residence, service duration, persistence delay and
   capture-to-result age. Use monotonic time for local durations and an explicit mapping/error
   budget when comparing sensor time with host time.
3. Counts from packets and rotations through valid frames, L3/L4 outputs, assigned observations,
   predicted-only states, persisted frames and consumer drops. Include empty, invalid, skipped and
   timed-out frames in the denominator; report completed-work latency separately from loss.
4. CPU per stage and total, allocation/GC, resident memory, disk/WAL/writer queues, network loss,
   clients, thermal state and frequency on the target Pi. Freeze enabled features and consumers.

A full-rotation product already spans about 50 ms at 20 Hz. First-point-to-result age can therefore
exceed 50 ms even with zero queueing and a service time below 48 ms. Late packet policy and smoothing
add more. A frame finishing its computation every 50 ms is throughput, not a promise that every
state is less than 50 ms old. Record which reference timestamp each estimate describes.

Do not divide 48 ms into invented measured stage allocations. Profile the production path first,
then assign budgets whose serial critical path fits the envelope and whose parallel stages each
sustain arrivals. Prioritise measured hot paths, bounded allocation and removal of synchronous
I/O stalls; preserve ordered state updates. The
[async tracking plan](../plans/lidar-cluster-observation-log-and-async-tracking-plan.md) offers an
architectural direction, not evidence that asynchronous separation has already met this deadline.

## Preserve physical time when cadence changes

The following arithmetic assumes regular successful updates. Real track observation gaps require
capture timestamps, not replacement of all counts by a guessed sensor period.

| Current default or contract                                              | At 10 Hz                   | At 20 Hz unchanged | Decision needed                                                                                                                  |
| ------------------------------------------------------------------------ | -------------------------- | ------------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| L3 warm-up minimum 50 frames                                             | Nominal count span 5 s     | 2.5 s              | Preserve minimum evidence and elapsed settling requirements independently                                                        |
| Confirmation 4 hits                                                      | First-to-fourth hit 0.30 s | 0.15 s             | Decide whether count evidence or elapsed persistence governs confirmation                                                        |
| Confirmed-track miss limit 25                                            | 25 intervals = 2.5 s       | 1.25 s             | Audit actual lifecycle comparison and capture-time continuity rules; these are count-window durations, not promised expiry times |
| Track history 200 / speed history 100 entries                            | Nominal 20 / 10 s          | 10 / 5 s           | Size by required time support and memory; gaps shorten evidence density                                                          |
| OBB heading alpha 0.08 per update                                        | Time constant about 1.20 s | About 0.60 s       | If preserving this constant, use time-aware alpha; qualify response versus noise                                                 |
| Box extent window 50 spans                                               | Nominal 5 s                | 2.5 s              | Confirm what creates a span and preserve the intended evidence window                                                            |
| Stationary median 400 observed states                                    | Nominal 40 s               | 20 s               | Preserve the dwell policy's intended support; shorter memory can hide slow creep                                                 |
| Sustained heading lock 5 updates / classification minimum 5 observations | About 0.5 s of intervals   | About 0.25 s       | Distinguish count evidence from elapsed persistence                                                                              |
| Background snapshot every 300 rotations                                  | 30 s                       | 15 s               | Choose cadence explicitly and account for writer work                                                                            |

For a per-update exponential filter, a time constant T gives α(Δt)=1−exp(−Δt/T).
Preserving the constant of α=0.08 at 0.1 s gives α≈0.04083 at 0.05 s. This is a derivation,
not a recommended untested config change. L3 has its own update semantics and must be audited
per parameter; do not assume every coefficient is per rotation. The [stationary-window and heading-lock constants](../../internal/lidar/l5tracks/tracking.go)
also need this audit: the 400-state median is deliberately related to the dwell policy's 40 s
window. The shipped Kalman process-noise
terms already scale by `dt`, so doubling them would change the physical uncertainty model.
The additional [occlusion inflation](../../internal/lidar/l5tracks/continuity.go) is different:
with the default capture-time option off, it adds 0.5 to position covariance per missed update.
For a continuously coasting track this contributes nominally 5 m²/s at 10 Hz and 10 m²/s at
20 Hz before lifecycle limits and covariance caps. Qualify time-based inflation or retuning
separately from Kalman process noise.
Measurement covariance still needs requalification under the altered point density. The server
also constructs a 1,800-bin azimuth background grid: verify cell occupancy, freshness and warm-up
at the coarser 20 Hz angular sampling instead of assuming every bin gets an equivalent update.

The 127-interval future grid proposed for SRAC is an evaluation buffer, not the track's past-history
buffer. For a 6.35 s horizon use 127 intervals and 128 states including now. Current speed-history
capacity 100 is not by itself an obstacle to future prediction, but is only about 5 s of regular
20 Hz observations if a later method requires that much past support.

## Expected gains and their limits

| Quantity, assuming successful observations            | 5 Hz effective | 10 Hz  | 20 Hz |
| ----------------------------------------------------- | -------------- | ------ | ----- |
| Time between observations                             | 200 ms         | 100 ms | 50 ms |
| Travel at 10 m/s between observations                 | 2 m            | 1 m    | 0.5 m |
| Travel at 20 m/s between observations                 | 4 m            | 2 m    | 1 m   |
| Mean wait to next observation for uniform event phase | 100 ms         | 50 ms  | 25 ms |

These calculations suggest finer event localisation, smaller association displacements and more
opportunities to observe short interactions. They do not include assembly or estimator latency.
More temporal support may improve following-gap continuity and reduce missed short conflicts;
that must be measured with physical references and complete opportunity accounting. Doubling the
rate does not extend the observed corridor, qualify unseen vehicle geometry, or turn predicted
future occupancy into fact. SRAC's 6.35 s support and upper-bound requirements remain separate.

There is also a noise tradeoff. For equally spaced positions with independent scalar error
variance σ², simple first and second differences have noise variance 2σ²/Δ² and 6σ²/Δ⁴.
Halving Δ doubles velocity noise standard deviation and quadruples acceleration noise standard
deviation under those assumptions. Real estimator errors are correlated, and point thinning can
change σ. Evaluate derivatives with time-based windows, uncertainty and lag; do not sell 20 Hz as
an automatic fourfold improvement in acceleration or jerk quality.

At the same return mode, packet/point volume is broadly unchanged while rotation headers,
per-frame clustering setup, tracking updates and stored state rows may grow. Actual VRLOG, database
and exported sizes depend on representation, retention and occupancy. Measure them rather than
assuming either all storage doubles or none of it does.

## Proposed implementation and qualification sequence

These are follow-up slices, not changes adopted by this PR. Before an experiment, create each
supported arm in `config/experiments/`, document changed keys and fingerprint, and retain raw
outputs locally. Keep device motor/return settings in frozen run provenance as well: tuning JSON
alone does not configure the sensor. Unsupported instrumentation/schema changes precede the arms.

| Order | Work                                                                                                  | Acceptance evidence                                                                                                                                                                    |
| ----- | ----------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | Add deployed-path timestamps, loss funnel and full service/age metrics                                | Trace one rotation through ingest, storage and client; reconcile counts including empty/lost frames; document timer coverage                                                           |
| 2     | Capture genuine 10 and 20 Hz workloads on the deployment Pi and storage, in each intended return mode | Device settings, build/config fingerprints, capture hashes, density/range/occupancy distributions and physical reference protocol; no speed-up replay substitution                     |
| 3     | Establish 48 ms experimental threshold and workload-matched baselines                                 | Matching config identity; per-stage and end-to-end tails, backlog, drops and thermal soak with required concurrent consumers                                                           |
| 4     | Correct time/count contracts, coverage/gap denominators and export retention                          | Paired time-support, lifecycle, density, identity and smoothing checks; a 20 Hz stream with half its observations missing must not be reported as full coverage                        |
| 5     | Optimise measured critical paths; isolate optional consumers with bounded queues if needed            | Service capacity and result-age criteria hold under dense scenes, disk stalls, slow clients, packet disorder and startup/background work                                               |
| 6     | Assess analytical benefit on held-out physical references                                             | Detection/association coverage, identity stability, gap and motion errors, exclusions and supported-horizon yield; acceptable uncertainty and regression criteria fixed before scoring |
| 7     | Adopt device/config defaults only if both timing and quality gates pass                               | Written per-case findings, limitations and rollback conditions; otherwise retain 10 Hz or restrict the supported workload explicitly                                                   |

Separate software comparisons from hardware cadence: replay the same native capture for algorithm
arms; use matched controlled physical episodes for 10-versus-20 Hz quality comparisons. Include a
20 Hz capture decimated to 10 Hz as a temporal-only diagnostic, labelled as such. Stratify by
range, class, occlusion, return mode, traffic density and adverse packet/disk conditions. Benchmark
all required pipeline depths, but only the deployed full workload can justify full-pipeline claims.
Do not treat the one-frame 20 Hz fixture described in the
[Pi benchmark runbook](../lidar/operations/pi-benchmark-runbook.md) as a sustained qualification run.

Report numerical per-case summaries and unresolved failures in an authored report linked from
this document, the relevant plan and a devlog fragment. No current benchmark result in this audit
establishes a Pi can sustain the proposed workload or that 20 Hz improves physical metric accuracy.

## Open questions

- Which Pi model, return mode, disk and concurrent consumers define the supported deployment?
- Does 48 ms mean a hard service ceiling or a statistical target, and what result-age, loss and
  recovery limits must accompany it? A hard guarantee requires bounded worst-case work/resources.
- Which windows preserve seconds, which preserve independent evidence, and which require both?
- Does the sparser 20 Hz snapshot retain enough distant/small-object support to improve effective
  observation rate? The historical 5 Hz cohort cannot answer this.
- Which outputs require 20 fresh results/s? A future SRAC solver needs its own bounded workload
  and deadline contract rather than consuming the 2 ms nominal service margin by assumption.
