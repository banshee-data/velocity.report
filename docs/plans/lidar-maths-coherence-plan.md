# LiDAR maths coherence plan (v0.5.2)

- **Status:** Draft
- **Layers:** L1 Packets, L2 Frames, L3 Grid, L4 Perception, L5 Tracks, L6 Objects, L8 Analytics
- **Target:** v0.5.2; the 052 run is maths-led, and the individual paper-gap fixes scheduled for it land on top of a substrate that does not yet exist.
- **Companion plans:** [lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md), [go-runtime-pipeline-correctness-plan.md](go-runtime-pipeline-correctness-plan.md), [lidar-clock-abstraction-and-time-domain-model-plan.md](lidar-clock-abstraction-and-time-domain-model-plan.md), [lidar-architecture-foundations-fixit-plan.md](lidar-architecture-foundations-fixit-plan.md), [lidar-performance-measurement-harness-plan.md](lidar-performance-measurement-harness-plan.md)
- **Canonical:** [data/maths/MATHS.md](../../data/maths/MATHS.md) (single source of truth)
- **Related:** [LiDAR math foundations audit](../lidar/architecture/math-foundations-audit.md), [Paper-vs-implementation gap analysis](../../data/maths/paper-implementation-gap-analysis.md)

## Motivation

`data/maths/` holds roughly 5,700 lines of careful specification across nine
documents and seven proposals. The Go implementation holds the mathematics those
documents describe. **The two are not connected in either direction that a
compiler or a reviewer can follow.** No Go file in the repository references a
`data/maths/` document. The citation is one-way: the maths notes point at code,
and the code points nowhere. An engineer editing the covariance update has no
breadcrumb saying which specification governs it, which paper it derives from, or
which known deviation is deliberate.

That is the connective-tissue problem. There is also a substantive one: the least
principled mathematics in the pipeline is the L6 confidence channel, which is
assembled from 29 scattered additive increments and has no governing equation in
any document. It is the one numerical surface in the tree that no maths note
covers, and it blocks the classification scorecard downstream.

v0.5.2 schedules six paper-gap fixes (K1, K2, B1, M1, S2, S3). Landing them
against the current substrate means six more numerical changes with no in-code
provenance, and in M1's case a metric with no maths note to live in. This plan
builds the substrate first so the 052 fixes land documented rather than merely
correct.

## Current state

| Fact                                                                                                                | Evidence                                                                                            |
| ------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| Zero Go files cite a `data/maths/` document                                                                         | Repository-wide search over `internal/` and `cmd/` returns no matches                               |
| Layer `doc.go` comments cite only `LIDAR_ARCHITECTURE.md`, never the maths notes                                    | `l3grid/doc.go`, `l4perception/doc.go`, `l5tracks/doc.go`, `l6objects/doc.go`, `l8analytics/doc.go` |
| L6 confidence is 29 additive increments with no equation in any document                                            | `l6objects/classification.go` confidence helpers                                                    |
| L1, L2, and L8 have no maths note; L8 is where MOTA/MOTP must live                                                  | `data/maths/` contents                                                                              |
| `MATHS.md` names "four math-heavy layers" and omits L6 despite `classification-maths.md` existing                   | `data/maths/MATHS.md`                                                                               |
| `MAGIC_NUMBERS.md` omits L3 EMA constants, DBSCAN defaults, Kalman noise defaults, and all L6 confidence increments | `MAGIC_NUMBERS.md`                                                                                  |
| Config-maths parity CI check runs with `continue-on-error: true`                                                    | `.github/workflows/config-order-ci.yml`                                                             |
| That workflow's path trigger references `internal/lidar/monitor/webserver.go`, which no longer exists               | `.github/workflows/config-order-ci.yml`                                                             |
| No fuzz targets and no property-based tests anywhere under `internal/lidar/`                                        | No `func Fuzz` or `testing/quick` occurrences                                                       |
| None of K1, K2, B1, M1, S2, S3 has landed                                                                           | Verified against `l5tracks/`, `l3grid/`, `l8analytics/`                                             |

## Ownership boundaries

The paper-gap items are already agreed backlog work owned by
[paper-implementation-gap-analysis.md](../../data/maths/paper-implementation-gap-analysis.md).
This plan does not absorb them. It owns the substrate they land on, which no
current document owns.

| Scope                                         | Owner                                                                                                                                         | Reason                                                                                        |
| --------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| K1, K2, B1, M1, S2, S3 fixes                  | Gap analysis + backlog P1a/P1b                                                                                                                | Individually specified with test criteria; this plan sequences them but does not restate them |
| Citation loop, doc coverage, hub corrections  | **This plan**                                                                                                                                 | Connective tissue; no existing document owns it                                               |
| L6 confidence equation                        | **This plan**                                                                                                                                 | A mathematical defect with no gap ID; blocks the classification scorecard                     |
| L8 analytics maths note                       | **This plan**                                                                                                                                 | Prerequisite for M1, which currently has nowhere to live                                      |
| Magic-number registry completeness            | **This plan**                                                                                                                                 | `MAGIC_NUMBERS.md` scope claim is currently untrue for L3, L5, L6                             |
| Config-parity CI enforcement                  | **This plan**                                                                                                                                 | The gate exists but is masked; foundations fix-it owns the key wiring, not the gate           |
| Runtime config key parity (10 remaining keys) | [Foundations fix-it](lidar-architecture-foundations-fixit-plan.md) Phase 2                                                                    | Already scheduled there                                                                       |
| Throttle and clock injection                  | [Clock abstraction](lidar-clock-abstraction-and-time-domain-model-plan.md) and [runtime correctness](go-runtime-pipeline-correctness-plan.md) | Both rewrite the same block; see Sequencing                                                   |

## Design / approach

### Citations are bidirectional and function-local

A package-level pointer is not enough, because the deviation is function-local.
Two levels:

1. Each layer's `doc.go` names the governing maths note.
2. Each function implementing a specified equation carries a one-line reference
   naming the document section and, where a deviation is deliberate, the gap ID
   that records it.

The second level is what makes the gap analysis self-enforcing: a reader of
`tracking_update.go` learns from the source that the covariance form is the naive
one and that K2 tracks the Joseph-form replacement.

### The L6 confidence channel becomes a stated model

The current behaviour is an unnormalised additive score clamped to a fixed band.
It is not a probability, and reporting it as `confidence` alongside a class is
misleading. The work is to state what it actually is, name its terms, and make it
measurable — not to replace it with a fitted model, which is Phase 3 of the
classifier plan.

1. Express the cascade's confidence as one documented additive-evidence model
   with named, registered terms rather than inline literals.
2. Record it as an equation in `classification-maths.md` with the evidence terms
   tabulated per class.
3. Register the term values in `MAGIC_NUMBERS.md`.
4. State explicitly in the note whether the output is calibrated. It is not,
   today; saying so is what lets the classifier plan measure the gap.

### Write the L8 note before implementing M1

MOTA and MOTP are the highest-impact gap items for tuning validation and the only
`L`-effort entry in the gap analysis. L8 has no maths note, so there is no
specification to implement against and no place to record the CLEAR MOT matching
rule. The note precedes the code, and shares its matching rule with the
classification scorecard so the two metric layers agree.

## Scope

### Item 1: close the citation loop

**Summary:** Make the maths-to-code reference bidirectional and function-local.

**Steps:**

1. Add the governing maths note reference to each layer `doc.go` for L3, L4, L5,
   L6, L8.
2. Add function-local references at each implemented equation: EMA baseline and
   spread update, DBSCAN neighbourhood and expansion, PCA and OBB extraction,
   Kalman predict and update, Mahalanobis gating, Hungarian cost construction.
3. At each known deviation, name the gap ID in the source comment.
4. Add a link-check pass so a referenced maths section that disappears fails CI
   the way other relative links already do.

**Milestone:** v0.5.2

### Item 2: state the L6 confidence model

**Summary:** Replace 29 inline increments with one documented, named model.

**Steps:**

1. Extract the increments into named evidence terms grouped by class rule.
2. Document the model as an equation in `classification-maths.md`, with an
   evidence-term table per class.
3. Register the term values in `MAGIC_NUMBERS.md`.
4. State the calibration status explicitly in the note.
5. Add a test pinning the confidence output per class so the extraction is
   provably behaviour-preserving.

**Milestone:** v0.5.2

### Item 3: L8 analytics maths note

**Summary:** Give MOTA, MOTP, and the classification scorecard a specification to
implement against.

**Steps:**

1. Write `data/maths/l8-analytics-maths.md` covering the CLEAR MOT definitions,
   the matching rule, temporal IoU as currently implemented, and the aggregation
   and percentile conventions the project uses.
2. Reconcile the matching rule with `computeTemporalIoU` in
   `internal/lidar/adapters/ground_truth.go` and with the classification
   scorecard so all three agree.
3. Add the note to the `MATHS.md` index.

**Milestone:** v0.5.2

### Item 4: L1 and L2 maths note

**Summary:** Document the timing and geometry mathematics that the clock and
performance work depends on.

**Steps:**

1. Write a combined note covering packet timestamp extraction, the sensor-time
   versus wall-time boundary, and polar-to-Cartesian frame geometry.
2. Cross-reference the time-domain model from the
   [clock abstraction plan](lidar-clock-abstraction-and-time-domain-model-plan.md)
   rather than restating it.

**Milestone:** v0.5.2

### Item 5: hub and registry corrections

**Summary:** Make the index documents true.

**Steps:**

1. Correct `MATHS.md` to include L6 among the mathematically significant layers
   and add the L8 and L1/L2 notes to the index.
2. Complete `MAGIC_NUMBERS.md` for L3 EMA constants, DBSCAN defaults, Kalman
   noise defaults, and L6 confidence terms, or narrow its stated scope to match
   what it actually covers.

**Milestone:** v0.5.2

### Item 6: unmask the config-parity gate

**Summary:** A masked gate is worse than no gate, because it reports success.

**Steps:**

1. Fix the stale path trigger in `.github/workflows/config-order-ci.yml`, which
   still references a moved file.
2. Remove `continue-on-error: true` once foundations fix-it Phase 2 wires the
   remaining runtime config keys; until then, make the masking visible in the job
   summary rather than silent.

**Milestone:** v0.5.2

## Sequencing

1. **Items 1, 3, 4, 5 first.** They are documentation and reference work with no
   runtime risk, and they give the numerical fixes somewhere to be recorded.
2. **Item 2 next.** It gates the classifier plan's Phase 1 calibration
   measurement.
3. **K1, K2, B1, S2, S3 after Item 1**, so each fix lands with its citation and
   its gap ID recorded in place. Re-validate every covariance change against
   `TestGoldenReplay_Determinism` in `internal/lidar/golden_replay_test.go`; its
   velocity tolerance is deliberately wide because Kalman estimates amplify
   floating-point differences, so a passing run is weaker evidence than it looks.
4. **M1 after Item 3.**
5. **Item 6 last**, gated on foundations fix-it Phase 2.

### Throttle collision warning

[Clock abstraction](lidar-clock-abstraction-and-time-domain-model-plan.md)
Phase A and [runtime correctness](go-runtime-pipeline-correctness-plan.md)
Phase 1 both rewrite the frame-rate throttle in
`internal/lidar/pipeline/tracking_pipeline.go`. The runtime-correctness plan
already flags the collision. Pin the current behaviour with tests before either
lands, and land them together. This plan does not own that code, but the
perf-harness numbers that validate the maths work depend on it, so the ordering
matters here too.

## Dependencies

- Foundations fix-it Phase 2 gates Item 6.
- The performance harness gates trustworthy before-and-after numbers for K1, K2,
  and S2/S3; its per-layer schema work is scheduled in the same release.
- The classification scorecard in
  [lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md)
  Phase 1 consumes Item 2 and shares a matching rule with Item 3.

## Risks

| Risk                                                                  | Likelihood | Impact | Mitigation                                                                                  |
| --------------------------------------------------------------------- | ---------- | ------ | ------------------------------------------------------------------------------------------- |
| Confidence extraction silently changes classifier output              | Medium     | High   | Behaviour-pinning test per class before extraction; no threshold changes in the same commit |
| Citations rot as code moves                                           | High       | Low    | Link-check pass in Item 1 step 4                                                            |
| L8 note is written to match the code rather than the papers           | Medium     | Medium | Write the CLEAR MOT definitions from the specification first, then record deviations        |
| Documentation-only items get deprioritised behind the numerical fixes | High       | Medium | Items 1 and 3 are prerequisites, not parallel work; sequence them first                     |
| Golden replay tolerance masks a covariance regression                 | Medium     | Medium | Compare covariance trace and symmetry directly, not just track positions                    |

## Checklist

### Complete

- [x] Gap analysis cross-referencing 24 papers against L3-L8 production code
- [x] Maths notes for L3, L4 ground, L4 clustering, L5, L6

### Outstanding

- [ ] Item 1: layer `doc.go` maths references (`S`)
- [ ] Item 1: function-local equation and gap-ID references (`M`)
- [ ] Item 1: maths-reference link check in CI (`S`)
- [ ] Item 2: extract confidence increments into named terms with pinning tests (`M`)
- [ ] Item 2: confidence equation and evidence tables in `classification-maths.md` (`S`)
- [ ] Item 3: `l8-analytics-maths.md` with CLEAR MOT definitions and matching rule (`M`)
- [ ] Item 4: L1/L2 timing and geometry note (`S`)
- [ ] Item 5: `MATHS.md` index and layer-list corrections (`S`)
- [ ] Item 5: `MAGIC_NUMBERS.md` completion for L3, L4, L5, L6 (`S`)
- [ ] Item 6: fix stale workflow path trigger (`S`)
- [ ] Item 6: unmask the config-parity gate (`S`)

### Deferred

- [ ] Property-based and fuzz coverage for the numerical kernels; valuable but
      broader than the 052 run and better scheduled with the coverage programme
- [ ] Paper acquisition for the eight paywall-blocked gap items, tracked by the
      gap analysis

### Accepted residuals (no action planned)

- [ ] L9 endpoints remain without a maths note; the layer is transport and
      serialisation, correctly out of scope
- [ ] L7 has no maths note because the layer is unimplemented; the scene maths
      belongs with the L7 plan when that work starts
