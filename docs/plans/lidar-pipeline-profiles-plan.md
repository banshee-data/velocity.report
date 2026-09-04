# LiDAR pipeline profiles (v0.5.2)

- **Status:** Implemented (v0.5.2); CI baselines outstanding
- **Layers:** LiDAR pipeline (L3–L6), tuning config, perf-regression CI
- **Target:** v0.5.2; the perf gate cannot be re-armed until a baseline can say which workload it measured
- **Companion plans:** [lidar-performance-measurement-harness-plan](lidar-performance-measurement-harness-plan.md), [lidar-clock-abstraction-and-time-domain-model-plan](lidar-clock-abstraction-and-time-domain-model-plan.md) <!-- link-ignore -->
- **Canonical:** [performance-regression-testing.md](../lidar/operations/performance-regression-testing.md)

## Motivation

The nightly perf gate failed on 2026-09-01 reporting a 7028% heap regression. The
regression was real but the comparison was not: the baseline it measured against
recorded a run in which L4, L5 and L6 never executed. Before #555 the benchmark's
background grid never finished settling — settling required 30 s of wall-clock
elapsed time and `lidar-bench` replays the whole capture in about 8 s — so
foreground extraction returned nothing and clustering, tracking and classification
were never entered. The baseline's `cluster_time_ms`, `tracking_time_ms` and
`classify_time_ms` are all zero, and the comparator skips any metric whose baseline
is zero, so the two fields that would have said "clustering is not running" were the
two it ignored. That state persisted for three months.

The structural fault is that a baseline records **hardware** (`goos`, `goarch`,
`num_cpu`, `go_version`, `commit_hash`) and **nothing about the workload**. Two runs
with different layer depth, different engines, or different warm-up settings produce
incomparable numbers, and the file cannot tell you which one you are holding. Until
a baseline can name its workload, regenerating it only resets the clock on the same
failure.

Naming workloads also has a payoff beyond CI: a profile is a supported way to run
the pipeline with less of it switched on, for constrained hardware and for
diagnosis.

## Current state

The selection machinery mostly exists and is unwired.

| Component                 | Location                                                                       | State                                                                                      |
| ------------------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------ |
| Per-layer engine selector | `l3.engine`, `l4.engine`, `l5.engine` in `config/tuning.defaults.json`         | Present, parsed, validated                                                                 |
| Engine registry           | [tuning.go:191](../../internal/config/tuning.go)                               | 8 engines declared across L3/L4/L5                                                         |
| Selected-block codec      | [tuning_codec.go](../../internal/config/tuning_codec.go)                       | `decodeSelectedEngineBlock[T]`, strict                                                     |
| Engine accessors          | [tuning_accessors.go](../../internal/config/tuning_accessors.go)               | `ActiveConfig()` / `ActiveCommon()` per layer                                              |
| Stage contracts           | [tracking_pipeline.go:100](../../internal/lidar/pipeline/tracking_pipeline.go) | `ForegroundStage`, `PerceptionStage`, `TrackingStage`, `ObjectStage`                       |
| Stage boundaries          | [tracking_pipeline.go](../../internal/lidar/pipeline/tracking_pipeline.go)     | Stages 1–7, each with an early-return path that calls `emitTiming` and `publishEmptyFrame` |
| Benchmark config input    | `lidar-bench -config`                                                          | Already accepts an arbitrary tuning file                                                   |

Three gaps:

1. **Nothing dispatches on the engine selector at runtime.** `DefaultDBSCANParams()`
   hardcodes `cfg.L4.DbscanXyV1`. Setting `l4.engine: hdbscan_adaptive_v1` parses,
   validates, and then silently runs DBSCAN. Five of the eight registered engines
   (`ema_track_assist_v2`, `two_stage_mahalanobis_v2`, `hdbscan_adaptive_v1`,
   `imm_cv_ca_v2`, `imm_cv_ca_rts_eval_v2`) have no implementation anywhere in
   `internal/lidar/` — they are config schema only.
2. **There is no depth concept.** The selector chooses _which algorithm_ runs at a
   layer, never _how far up the stack_ the pipeline goes. Depth is what delivers
   reduced load, and it is what the failed baseline accidentally exercised.
3. **Baselines carry no workload identity**, and the comparator has no way to refuse
   a mismatched comparison.

## Findings

All figures: `kirk0.pcapng`, 832 frames, Apple M1 Pro, at `1af38e9b1` (the two
allocation fixes from #565 applied). Depth gating was applied as temporary
instrumentation in `analysisFrameBuilder.processCurrentFrame` to size the profiles;
that instrumentation is not part of this plan's deliverable.

| Workload                      | Wall clock | Frame avg | Frame p95 |   DBSCAN | Live heap | Total alloc |
| ----------------------------- | ---------: | --------: | --------: | -------: | --------: | ----------: |
| `unsettled` (pre-#555 config) |   5 024 ms |   3.06 ms |   4.11 ms |     0 ms |  11.9 MiB |    15.7 GiB |
| `l3-only`                     |   5 228 ms |   3.40 ms |   3.67 ms |     0 ms |  17.0 MiB |    15.8 GiB |
| `detect`                      |   9 615 ms |   8.69 ms |  37.18 ms | 4 287 ms |  17.0 MiB |    16.4 GiB |
| `track`                       |   9 623 ms |   8.98 ms |  37.65 ms | 4 381 ms |  17.0 MiB |    16.4 GiB |
| `full`                        |   9 644 ms |   8.87 ms |  37.40 ms | 4 326 ms |  17.0 MiB |    16.5 GiB |

Live heap is measured after an explicit `runtime.GC()`; frame-time distributions come
from a companion run of the same builds without it. Wall clock is sampled before
either, so both sets are comparable.

### F1 — The existing baseline is not a profile

`unsettled` is not `l3-only`. A grid that never settles skips region identification
entirely and stays in warm-up alpha for the whole capture; `l3-only` settles at frame
~50, runs `IdentifyRegions` once, and applies region-aware parameters thereafter. The
gap is +4% wall clock, +11% frame average, and **+43% live heap** (11.9 → 17.0 MiB).

The heap delta reconciles: 50 surviving regions × 72 000 cells = 3.6 MB of masks,
plus `prevSpreads` (72 000 × 4 B) and `prevRegionIDs` (72 000 × 8 B) = 0.86 MB of
settling-evaluation state. That is 4.5 MB against a measured 5.1 MiB delta.

"Never settles" is a degenerate state, not a tier. It produces no foreground, so the
visualiser shows an empty scene and no vehicle is ever detected — the condition
`SettlingStatus` was added in #555 to make visible. Blessing it as a supported
profile would enshrine a bug as a product feature.

**Severity: high. Release view: the baseline must be cut and redone, not renamed.**

The historical numbers are not lost. Setting the four `settling_*` thresholds to zero
and `warmup_min_frames` back to 100 reproduces the old measurement from current code
— 5 024 ms, `cluster=0`, 15.7 GiB allocated, against the pre-#555 build's 5 184 ms /
`cluster=0` / 15.7 GiB. Anything needing the old numbers can regenerate them.

### F2 — There are two performance tiers, not four

L5 costs 48–51 ms and L6 58 ms out of ~9 640 ms: **1.1% combined**. `detect`, `track`
and `full` are indistinguishable in wall clock, frame time and allocation; the spread
between them (9 615 / 9 623 / 9 644 ms) is inside the ±4% run-to-run noise measured
on `full` over three repeats.

The only measurable step is L3 → L4, at 1.85×. Gating four profiles would produce
four sets of numbers, three of which move together and explain nothing.

**Severity: medium. Release view: gate two profiles; keep any others diagnostic.**

### F3 — `heap_alloc_bytes` is currently noise, and one line fixes it

`HeapAllocBytes` is `runtime.MemStats.HeapAlloc` read with no preceding collection —
whatever the heap happened to be mid-cycle. Across the four profiles it read 15.3,
18.2, 19.6, 35.1 and 35.4 MiB on workloads whose true live heap is identical. With a
`runtime.GC()` inserted before `ReadMemStats` it reads **17.0 MiB on every profile,
on every run**, and `TotalAlloc` becomes stable to three significant figures.

This is the single highest-value change in the plan: it converts the metric that
produced the 7028% headline into the most reliable number in the set. It also means
live heap does _not_ discriminate between profiles, so profile budgets must be set on
time and allocation.

**Severity: high. Release view: land independently of everything else here.**

### F4 — Per-layer timing would have caught this

A baseline whose `l4_perception` mean is 0.0 ms is self-evidently wrong. Item 1 of
the [performance measurement harness plan](lidar-performance-measurement-harness-plan.md)
already proposes exactly that instrumentation. This plan supplies workload identity;
that plan supplies within-run attribution. Neither substitutes for the other, and the
two together close the gap that let a no-op pipeline pass as a baseline for three
months.

**Severity: medium. Release view: sequence this plan's schema work with that plan's.**

### F5 — Settling depth depends on wall clock

Whether the grid settles at all depends on elapsed wall-clock time versus replay
speed, so the workload varies with how fast the runner happens to be. The CI runner
takes 15 168 ms for what takes 9 644 ms locally; a slower or more heavily loaded
runner could cross the 30 s ceiling mid-run and settle at a different frame. This is
the same sensor-time-versus-wall-time boundary the
[clock abstraction plan](lidar-clock-abstraction-and-time-domain-model-plan.md)
exists to formalise, and its backlog entry already names making perf-harness results
reproducible as the reason.

**Severity: medium. Release view: gated on the clock abstraction work.**

## Design / approach

A **profile** is a single closed enum naming how far up the layer stack the pipeline
runs. It is deliberately not a cross-product of per-layer switches: three independent
depth toggles would be eight combinations, most of them meaningless, and every one of
them a support surface. Per-layer `engine` selectors stay exactly as they are —
orthogonal, and each with one implementation today.

### The profiles

Superseded by the implementation note below: these are depths derived from the
per-layer engine selectors, not values of a `pipeline.profile` key.

| Profile   | Runs                                                   | Measured cost | Rationale                                                                                  |
| --------- | ------------------------------------------------------ | ------------: | ------------------------------------------------------------------------------------------ |
| `l3-only` | L3 background model, settling, region identification   |      5 228 ms | Background and settling tuning in isolation; sensor health; thermally-constrained hardware |
| `detect`  | + L4 world transform, ground removal, clustering       |      9 615 ms | Cluster-level tuning without tracker state or DB writes                                    |
| `full`    | + L5 tracking, L6 classification, persistence, publish |      9 644 ms | What ships. Default.                                                                       |

Three names, two performance tiers. `detect` is retained not for CPU — F2 shows it
saves 0.3% — but because it holds no Kalman state and performs no track persistence.
Neither shows up in an 83-second benchmark; both matter to a Pi running for weeks,
where `max_tracks` state and per-frame DB writes accumulate. That rationale should be
stated in the docs so nobody later "optimises" `detect` away on the benchmark
evidence alone.

`track` is dropped. It is 0.1% cheaper than `full`, holds the same tracker state, and
its only distinction is skipping classification — which is not a load characteristic
anyone would choose a deployment on.

### Where it plugs in

The pipeline already early-returns between stages when a dependency is nil:

```go
// Stage 4: Track update
if cfg.Tracker == nil {
    if emitTiming != nil { emitTiming(len(foregroundPoints), len(clusters), 0) }
    publishEmptyFrame(frame)
    return
}
```

Every boundary already carries its `emitTiming` and `publishEmptyFrame` handling. A
depth profile is the same early return, chosen by configuration instead of by an
accidental nil. That makes this a small change at the existing seams rather than a
restructuring, and it is why the profile axis is depth rather than something new.

### Baseline identity

`BenchmarkResult` gains `profile` and a tuning fingerprint. The comparator **refuses**
to compare across differing profiles or fingerprints and exits non-zero with a message
naming both sides, rather than emitting a delta. The zero-baseline skip
(`if m.baseline == 0 { continue }`) is removed: a metric moving from non-zero to zero
is the strongest possible regression signal and is currently the one case that is
silently ignored.

Baselines become `baseline-<pcap>-<profile>-ci.json`, composing with the
hardware-suffix scheme the harness plan defines (`-ci`, `-pi`, bare for local).

### Nightly gating

Gate `full` and `l3-only`. `l3-only` costs ~5 s and isolates background-model
regressions from clustering noise, which is real value for near-zero runtime.
`detect` is a diagnostic tool, run on demand, not gated — an unexercised gated
profile is a set of numbers nobody can explain when it moves.

## Scope

### Item 1: Stabilise the heap metric

**Summary:** Insert an explicit collection before reading memory statistics so the
gate's memory metrics measure live heap rather than GC phase.

**Steps:**

1. Call `runtime.GC()` before the closing `runtime.ReadMemStats` in
   `lidarbench.runBenchmark`.
2. Note in [performance-regression-testing.md](../lidar/operations/performance-regression-testing.md)
   that `heap_alloc_bytes` is live heap after a forced collection.
3. Add a benchmark-level test asserting two runs over the same capture report the
   same `heap_alloc_bytes`.

**Milestone:** v0.5.2. Independent of every other item here.

### Item 2: Workload identity in the baseline

**Summary:** Record which workload produced a benchmark, and refuse comparisons
across workloads.

**Steps:**

1. Add `profile` and `tuning_fingerprint` (hash of the resolved tuning config) to
   `BenchmarkResult`.
2. Refuse comparison on mismatch: exit non-zero naming both sides.
3. Remove the zero-baseline skip; treat non-zero → zero as a regression.
4. Treat a baseline with no `profile` field as `unknown` and refuse to compare
   against it, so the current file cannot be used by accident.

**Milestone:** v0.5.2. Depends on Item 3 for the profile vocabulary.

### Item 3: The profile enum

**Summary:** Add `pipeline.profile` to the tuning config as a closed set, and gate the
existing stage boundaries on it.

**Steps:**

1. Add `Profile string` to `PipelineConfig` with a `Validate()` case list, following
   the `L3Config.Validate()` pattern.
2. Default to `full` when absent, so every existing config keeps working.
3. Gate the stage boundaries in `tracking_pipeline.go` on the resolved profile, using
   the existing `emitTiming` / `publishEmptyFrame` early-return paths.
4. Mirror the gating in `analysisFrameBuilder.processCurrentFrame` so `lidar-bench`
   and the live pipeline agree on what a profile means.
5. Add a test per profile asserting which stage counters remain zero.

**Milestone:** v0.5.2.

### Item 4: Cut the new baselines

**Summary:** Generate `full` and `l3-only` CI baselines from scratch on runner
hardware.

**Steps:**

1. Land #565 first; a baseline generated before it captures the region-mask and
   expansion defects as normal.
2. Land Items 1–3 so the new files carry stable memory figures and a profile name.
3. Generate both baselines on the CI runner via a `workflow_dispatch` run.
4. Retire `baseline-kirk0-ci.json`. Do not rename it — per F1 it measures a
   degenerate state, not a profile.
5. Extend the nightly to gate both profiles.

**Milestone:** v0.5.2.

### Item 5: Document the profiles

**Summary:** State what each profile runs, what it costs, and why `detect` exists.

**Steps:**

1. Add a profiles section to
   [performance-regression-testing.md](../lidar/operations/performance-regression-testing.md).
2. Record the F2 finding — that `detect` is justified by state and I/O, not CPU — so
   the rationale survives the next optimisation pass.
3. Cross-reference from [config-param-tuning.md](../lidar/operations/config-param-tuning.md).

**Milestone:** v0.5.2.

## Dependencies

| Dependency                                                                            | Relationship                                                                                                                              |
| ------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| #565 (region mask + DBSCAN expansion fixes)                                           | Must land before Item 4; baselines cut before it would enshrine both defects                                                              |
| [Performance measurement harness plan](lidar-performance-measurement-harness-plan.md) | Item 2's schema work should land with that plan's per-layer fields, not twice                                                             |
| [Clock abstraction plan](lidar-clock-abstraction-and-time-domain-model-plan.md)       | F5: settling depth is wall-clock dependent, so profile boundaries are not fully reproducible until the time-domain boundary is formalised |

## Risks

| Risk                                                                    | Likelihood | Impact | Mitigation                                                                                                                       |
| ----------------------------------------------------------------------- | ---------- | ------ | -------------------------------------------------------------------------------------------------------------------------------- |
| Profiles multiply into a config matrix                                  | Medium     | High   | Single closed enum, validated against a case list; per-layer engine selectors stay orthogonal and unwired                        |
| `detect` rots unexercised                                               | Medium     | Medium | Not gated in CI; documented as diagnostic; its state/IO rationale recorded so benchmark evidence alone does not condemn it       |
| New baselines cut on an atypical runner                                 | Medium     | Medium | Generate via `workflow_dispatch` on the same runner class as the nightly; re-cut if the first nightly disagrees beyond threshold |
| Profile gating diverges between `lidar-bench` and the live pipeline     | Medium     | High   | Item 3 step 4 mirrors the gating; per-profile stage-counter tests in both                                                        |
| Wall-clock-dependent settling makes profile boundaries non-reproducible | High       | Medium | F5; gated on the clock abstraction plan. Until then, pin `warmup_duration_nanos` in the benchmark configs                        |

## Checklist

### Complete

- [x] Establish that the existing baseline measures a degenerate state, not a profile (F1)
- [x] Size all four candidate profiles against `kirk0` (F2)
- [x] Establish that `heap_alloc_bytes` is GC-phase noise and that one collection fixes it (F3)
- [x] Confirm the engine selector exists but is unwired, and that 5 of 8 engines are config-only

- [x] Item 1: stabilise the heap metric — `runtime.GC()` before the closing read;
      `heap_alloc_bytes` now reads 17.0 MiB on every run of the full profile
- [x] Item 2: workload identity in the baseline — `profile`, `tuning_fingerprint`,
      `metrics.work` and platform, with the comparator refusing rather than
      emitting a delta; the zero-baseline skip removed
- [x] Item 3: the profile enum and stage gating — gated in both
      `tracking_pipeline.go` and `analysisFrameBuilder`, with a per-profile
      stage-counter test in each
- [x] Item 5: document the profiles

### Outstanding

- [ ] Item 4: cut the CI baselines via the 📏 Capture Perf Baseline workflow and
      commit them (`S`). Local baselines for `full` and `l3-only` are committed;
      until the `-ci` files exist the nightly runs the absolute frame-budget check
      alone, which is a real gate rather than a skip

### Deferred

- [ ] Runtime dispatch on the per-layer `engine` selector: five registered engines have no implementation, and wiring dispatch for absent algorithms would invent a support surface rather than use one
- [ ] Pi-hardware profile baselines: covered by the hardware-baseline scheme in [lidar-performance-measurement-harness-plan](lidar-performance-measurement-harness-plan.md) <!-- link-ignore -->

### Implementation notes

**The profile is derived, not stored.** This plan proposed `pipeline.profile` as a
closed enum. That shipped first and was then removed, because it was a second
mechanism answering a question the config already had one for: `l4.engine` says which
clustering algorithm runs, and `pipeline.profile` said whether clustering happens at
all. Neither could be read without the other, the depth lived in a Go lookup table
rather than in the file, and an `l3-only` config still carried — and validation still
_required_ — a fully populated `l4.dbscan_xy_v1` block whose nine parameters had no
effect.

What shipped instead: `engine: "none"` is a legal value of the selector each layer
already has. A disabled layer carries no parameter block, the codec rejects one if
present, and `TuningConfig.Profile()` reads the depth off the selectors. The label
still exists for baseline filenames and gate lists, but it is computed, so it cannot
disagree with what runs.

The closed set survives as a validation rule rather than an enumeration: disabled
layers must form a suffix, so `l4.engine: "none"` with a live L5 is rejected at load.
Tracking cannot consume clusters that were never produced.

**Three profiles, not four.** `track` needs an `l6.engine` selector to be expressible,
and L6 has no config block at all — its one parameter squats in `l5.cv_kf_v1`. Adding
required keys breaks the strict decoder, so that is a schema v3 change, tracked in the
backlog alongside giving L2 a block and emptying the `pipeline` junk drawer. `track`
falls out of that work for free. The evidence never argued for it anyway: 0.1% cheaper
than `full` with identical tracker state.

**Cost of the redesign.** Roughly thirty accessors dereference `ActiveCommon()`
directly, so a disabled layer returning nil would panic at startup. `ActiveCommon()`
returns a zero-valued block instead: a layer that does not run has no meaningful
parameter values, and the stage that would read them never executes.

The 98 ms frame budget arrived alongside this work rather than in the harness plan.
It is the answer to a question no relative gate can address — whether the pipeline
keeps up with a 10 Hz sensor at all — and `kirk0` is exactly 10.0 Hz (83.183 s,
832 frames), so the budget is 2 ms inside the frame interval.

### Accepted residuals (no action planned)

- [ ] Work counters are not bit-exact across runs: L3 settling is wall-clock
      dependent (F5), so counts drift with replay speed. Measured at under 0.01%
      across five repeats, against a 10% identity tolerance
- [ ] Wall-clock variance on `full` (±4% across repeats): inherent to a wall-clock throughput metric, and the reason the 30% gate threshold is not tightened
