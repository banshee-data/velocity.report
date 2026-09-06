# Heading coherence: D2 readiness and handoff

- **Status:** Implementation slice delivered; candidate acceptance remains open
- **Scope:** Root checkout `dd/docs/state-est`; original inspection at `9b5525ab3`
- **Related:** [Heading sprint](lidar-heading-coherence-sprint-plan.md), [Visibility-aware
  maths][research], [Single-site demo](lidar-single-site-shape-demo-sprint-plan.md)

[research]: ../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md

## Current implementation declaration

The sections below preserve the pre-implementation review. D2.5's recorded-output fixture
and replay boundary, D2.3's diagnostic/objective plumbing, and the default-off D2.1/D1.4
candidate are now implemented on top of `4d3bc0fe5`. The
[implementation and experiment report](lidar-heading-d2-implementation-report.md) supersedes
statements below that these mechanisms are absent. D2.2 association work and D2.4 UI remain
outstanding. The first warmed A/B is mixed; it does not justify enabling the candidate.

## 1. Decision

Proceed with a bounded axis-coherence experiment, not a full geometry estimator. First repair
the evaluation contract: warm-up, lock episodes, comparison fields, and a meaningful reference
fixture. Then implement D2.1 with an explicit ambiguous outcome. Do not equate always choosing
an axis with always having evidence for that axis.

The other agent completed D1.1, D1.2, D1.3, D1.5, and D1.6. D1.4 remains deferred. It also
committed the headless replay harness, execution-parameter provenance, blocking frame delivery,
point-derived DBSCAN subsampling, sorted association order, three-way lock outcomes, and a
recorded `released` heading source. These are useful prerequisites. Code inspection finds no
D2.1 axis selector, D2.2 general shape cost, D2.3 revised objective, D2.4 run panel, or D2.5
named-car fixture.

The sprint reports a 60-second A/B improvement from 40.8 to 29.7 degrees of median per-track
course error, with fragmentation rising from 0.301 to 0.313. Its later five-minute window
reports 45 never-recovered tracks, two classified as relocked, 55 released, and 130
forced-release frames. These are reported experiments, not reruns in this review. Their windows
and populations differ; do not combine them into one before/after result.

## 2. Gaps to close before judging D2

### 2.1 Warm-up is not implemented by the documented start offset

[The replay harness](../../internal/lidar/replayeval/replayeval.go) creates a fresh background
manager for each run and passes `StartSeconds` directly to
[the packet reader](../../internal/lidar/l1packets/network/pcap.go). That reader skips earlier
packets before parsing. There is no predecessor input or background restore in this harness
configuration. Running a predecessor in a separate invocation therefore does not warm the next
invocation; concatenating files and skipping the first 300 seconds does not warm it either.

Define separate processing and scoring windows. Either process the static predecessor and
target in one continuous pipeline, then score only the target, or explicitly warm and restore
compatible background state before processing the target. State whether tracker history crosses
the boundary; reset or retain it consistently in both arms. Validate sensor pose, calibration,
timestamps, and settling before calling a window warmed.

The sprint's reported warm-up improvement needs its exact execution recipe retained and
checked. This inspection disproves the documented recipe, not every possible experiment that
could have produced the reported values.

### 2.2 Lock outcomes are lifetime predicates, not episode outcomes

[Live counters](../../internal/lidar/l5tracks/tracking_update.go) and
[offline reconstruction](../../internal/lidar/analysis/report.go) latch both recovery booleans.
Reading their state transitions gives these counterexamples:

| Recorded sequence                      | Current outcome | What the label fails to establish       |
| -------------------------------------- | --------------- | --------------------------------------- |
| 10 locked, 5 unlocked, 10 locked       | `released`      | The final sustained lock is unrecovered |
| 10 locked, 1 unlocked, then track ends | `relocked`      | No second lock occurred                 |

These are code-derived cases, not newly executed production fixtures. Existing tests pass with
the current lifetime semantics; passing them does not resolve the naming mismatch.

Preserve lifetime statistics, but add episode start/end, last-episode recovery, and a
right-censored outcome for tracks ending before the release window can be observed. Decide
explicitly whether any re-lock or only the terminal episode defines a failure. Mirror tests
between live and offline paths, including clean release followed by a new lock, one unlocked
frame at track end, deleted frames, unknown sources, and irregular cadence. Record durations in
capture time as well as frame counts.

The single `locked` source does not identify Guard 1 versus Guard 2 versus Guard 3. Persist a
reason or bitmask before attributing the remaining population to a specific guard. A
never-recovered heading can reflect insufficient evidence, not necessarily a failed
rejection-counter release. The `released` event is useful but does not measure whether the
snapped heading was physically correct.

### 2.3 The comparison and objective do not yet support the proposed gate

[QualityDelta](../../internal/lidar/analysis/types.go) contains fragmentation, mean
observations, and mean occlusions, but not course, lock, or co-location deltas.
[CompareReports](../../internal/lidar/analysis/compare.go) filters to final `confirmed` tracks
and matches using temporal overlap. That can exclude completed vehicles and pair different
vehicles that coexist. It is not an identity reference for the split-car case.

[DefaultObjectiveWeights](../../internal/lidar/sweep/objective.go) already leaves
`HeadingJitter` at zero; it remains available as an opt-in term. `ActiveTracks` still has the
positive 0.3 weight. D2.3 must audit effective tuning weights and runner plumbing, not describe
zeroing the default jitter weight as a new fix. The runner has no course aggregate yet. Course
should remain a diagnostic or constrained secondary objective, not body-yaw truth. A
track-count band needs a declared site/window reference population.

### 2.4 Reproducibility and provenance are useful but incomplete

The association sort uses random UUIDs. It removes within-run map-order churn but does not
define cross-run tie-breaking. Add a tied-cost fixture with canonical creation or observation
ordering. Repeated matching aggregates on one capture do not establish that all assignments are
reproducible, especially when overlapping tracks are the target case.

The replay repeatability test uses six seconds and compares frame count, track count, and
fragmentation. It does not require nonzero heading samples or the named turning car. Execution
provenance stores tuning parameters and their hash, source basename, and build stamps. Add
source-content identity, processing/scoring windows, warm-up provenance, calibration identity,
and effective replay options to the experiment manifest. A tuning hash is not a complete
experiment identity, and an unstamped build must be flagged.

## 3. Revised D2 sequence

| Order | Item                    | Bounded next delivery                                                                                                         | Exit evidence                                                                                  |
| ----- | ----------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| 1     | D2.5 first slice        | Pin the named-car excerpt, raw source identity, processing/scoring windows, and human object identity; add lock-episode cases | Nonempty eligible population; warm-up and live/offline semantics verified                      |
| 2     | D2.3 measurement slice  | Add comparison fields and runner aggregates; expose denominators and missing values; audit actual weights                     | No missing metric treated as perfect zero; no course-only success claim                        |
| 3     | D2.1                    | Compare aligned/swapped axial hypotheses against a frozen pre-update reference, retaining ambiguity                           | Swaps resolved when supported; partial, square, turn, reverse, and low-point cases can abstain |
| 4     | D1.4 with D2.1          | Publish a coherent observed envelope or explicitly estimated body box                                                         | Published centre, angle, and extents describe the same object representation                   |
| 5     | D2.2                    | Trial a bounded visible-support compatibility cost separately from dimension-update admissibility                             | Better identity evidence without rejecting legitimate partial views or multiplying fragments   |
| 6     | D2.4 and D2.5 close-out | Display and freeze the same metrics, labels, and before/after evidence                                                        | Human-reviewable baseline/candidate result and regression fixture                              |

### D2.1 implementation contract

- Retain Guard 1 or an equivalent validity test; minimum point count alone is not sufficient.
- Swap dimensions with the quarter-turn hypothesis and compare axial angles modulo pi. Do not
  infer body-front identity from the axial decision.
- Do not treat running means of partial raw extents as physical dimensions. If used as an
  interim descriptor, label them as observed-support heuristics and keep them separate.
- Compare against the pre-update reference. Today's raw averages update before the heading
  guards, so reading them later would partly score a candidate against itself.
- Require a declared evidence margin and minimum support; otherwise retain uncertainty and
  report `ambiguous` or prior-dominated. Neither a forced winner nor an indefinite confident
  lock is acceptable. Missing geometry and ambiguous geometry are distinct reasons.
- Keep the baseline path selectable for A/B. Do not add uncalibrated posterior-confidence
  claims or count-based sigma shrinkage to make the experiment appear more complete.

### D2.2 and the split car

A 0.11-metre fragment may belong to the car even though it cannot measure the car's full size.
Association acceptance and geometry-update acceptance therefore need separate tests. The
current narrow fragment guard leaves scraps available to seed new tracks; that can protect
dimensions while increasing duplicate identities. Measure both effects.

Use a physical-object annotation independent of predicted track UUIDs. Proximity is a
candidate-review signal, not proof of duplication: two real road users can be close. Do not
hard-reject ordinary partial views for disagreeing with full object dimensions. Test
pedestrians, cyclists, unknown dimensions, merges, and competing vehicles as well as the named
car. D2.2 is not yet the full face-aware tracker or learned class model.

## 4. Documentation precedence

This review supersedes the sprint's current instructions where they say that D2.1 must always
choose an axis, that a raw dimension mean is a physical shape belief, that the association
threshold is a six-metre radius, or that start-offset skipping warms the grid. It also
qualifies the earlier causal interpretations of fragmentation and lock recovery. Preserve
measured tables and their provenance; do not silently rewrite them as new results.

The three-day annotation/shape demo remains a separate bounded experiment. Its point masks,
body-local shape cache, and JSON descriptors can proceed on a frozen dataset while D2
stabilises the online comparison. Adaptive shape estimation is not required to finish D2.1.

## 5. Transfer and validation

The original documentation commit was already present in the root history. Seven remaining
documentation-file updates were merged from the `7c2f` worktree into the root checkout. Root
editorial changes and semantic-classification links were retained. The root's modified PCAP
scan script was left untouched; neither that script nor the scratch binary was copied over it.
The source worktree remains intact as a recovery copy.

Fresh validation at the inspected root commit passed the PCAP-tagged package suites for
`l4perception`, `l5tracks`, `analysis`, and `replayeval`; `sweep` passed from its test cache. A
second uncached `replayeval` run passed all 15 tests with no skips, including its capture
integration, repeatability, and provenance tests. These checks do not reproduce the long
reference-window experiment, validate physical heading truth, or test the proposed fixes. No
runtime source file was changed by this transfer or review.
