# LiDAR classification evaluation harness and optional model training

- **Status:** Draft
- **Layers:** L6 Objects, L8 Analytics, sweep/evaluation platform
- **Target:** v0.5.2-v2.0; the measurement harness lands with the 052 maths run, the corpus contract with 053 data contracts, and candidate models stay parked at v2.0 until the scorecard exists.
- **Companion plans:** [lidar-maths-coherence-plan.md](lidar-maths-coherence-plan.md), [lidar-track-labelling-auto-aware-tuning-plan.md](lidar-track-labelling-auto-aware-tuning-plan.md), [lidar-test-corpus-plan.md](lidar-test-corpus-plan.md), [unpopulated-data-structures-remediation-plan.md](unpopulated-data-structures-remediation-plan.md), [platform-data-science-metrics-first-plan.md](platform-data-science-metrics-first-plan.md)
- **Canonical:** [data/maths/classification-maths.md](../../data/maths/classification-maths.md) (single source of truth)
- **Related:** [ML solver expansion](../lidar/architecture/ml-solver-expansion.md) (the optimisation platform this work reuses; **not** canonical for classification)

## Motivation

The project has a standing rule that classification must stay explainable, and a
standing question about whether that rule costs accuracy. Neither can be settled,
because **nothing in the repository measures classification accuracy**. The
ground-truth evaluator scores whether a track was detected; it never scores
whether the track was given the right class. There is no confusion matrix, no
per-class precision or recall, and no calibration measurement anywhere in the
tree.

The consequence is already visible in the runtime. Truck and motorcyclist
classification are commented out in
[internal/lidar/l6objects/classification.go](../../internal/lidar/l6objects/classification.go),
with source comments attributing the decision to insufficient labelled data
rather than to model capacity. Two classes were switched off because nobody could
demonstrate they worked. That is a measurement failure, not a modelling failure,
and it will repeat for every future class until a scorecard exists.

The previous version of this document was a policy statement rather than a plan:
guardrails and a promotion gate, with no phases, no metric definitions, and three
named implementation files that do not exist. It also declared
[ml-solver-expansion.md](../lidar/architecture/ml-solver-expansion.md) canonical,
which is a category error — that document specifies a _parameter-optimisation_
platform and never discusses object classification. This rewrite fixes the
pointer, defines the scorecard, and sequences the work.

## Current state

| Fact                                                                                                                                                        | Evidence                                                       |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| Classifier is a fixed-priority rule cascade, model string `rule-based-v1.2`                                                                                 | `l6objects/classification.go` `ClassifyFeatures`               |
| All ~20 class thresholds are Go `const`, not config; only `min_observations_for_classification` is tunable                                                  | `classification.go` const block; `config/tuning.defaults.json` |
| Confidence is 29 scattered `confidence += 0.05` / `+= 0.1` / `-= 0.15` statements with no governing equation                                                | `classification.go` confidence helpers                         |
| Truck and motorcyclist branches are commented out for lack of labelled data                                                                                 | `classification.go` rules 3 and 5                              |
| Live cascade consumes 11 features; `TrackFeatures` already computes ~20 (elongation, compactness, intensity std, heading variance)                          | `l6objects/features.go`                                        |
| Ground-truth evaluator scores detection, fragmentation, false positives; `DetectionRateByClass` is detection recall per reference class, not class accuracy | `internal/lidar/adapters/ground_truth.go`                      |
| No confusion matrix, macro-F1, or calibration measurement exists                                                                                            | No occurrences in `l8analytics/` or `adapters/`                |
| Labels live only in runtime SQLite (`lidar_run_tracks.user_label`); no versioned corpus artefact                                                            | `internal/db/migrations/`                                      |
| 1 of 5 planned test PCAPs captured                                                                                                                          | `docs/lidar/operations/test-corpus.md`                         |
| No training-data export path; spec parked at v0.8.0                                                                                                         | `unpopulated-data-structures-remediation-plan.md` Phase 5      |

## Findings

| Area                    | Current state                                                                                                    | Severity | Release view                                                   |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------- | -------- | -------------------------------------------------------------- |
| Canonical pointer       | Declares a parameter-optimisation doc canonical; that doc never mentions object classification                   | High     | Fix in this rewrite                                            |
| Scorecard definition    | Promotion gate demands "the agreed scorecard"; no scorecard is defined anywhere in the repository                | High     | Phase 1, v0.5.2                                                |
| Class accuracy metric   | Absent. Detection is measured, classification is not                                                             | High     | Phase 1, v0.5.2                                                |
| Confidence calibration  | Uncalibrated by construction; a reliability diagram over additive increments is meaningless                      | High     | Owned by [maths coherence plan](lidar-maths-coherence-plan.md) |
| Platform reuse          | Plan proposes a parallel Python stack; the Go sweep platform already provides versioning and score decomposition | Medium   | Phase 1 design decision                                        |
| Proposed language       | Names `tools/ml-training/*.py` in a repo that removed its Python PDF generator                                   | Medium   | Rejected in this rewrite                                       |
| Feature-set duplication | Restates the feature list in prose instead of referencing `TrackFeatures`                                        | Low      | Fixed in this rewrite                                          |
| Corpus availability     | Assumes fixed replay packs; 1 of 5 captured, labels unversioned                                                  | Medium   | Phase 2, v0.5.3                                                |

## Design / approach

### Reuse the optimisation platform; do not build a second one

Phase A of [ml-solver-expansion.md](../lidar/architecture/ml-solver-expansion.md)
shipped the substrate this work needs, and the previous plan did not know it
existed:

| Capability                               | Where it already lives                                                               |
| ---------------------------------------- | ------------------------------------------------------------------------------------ |
| Score component decomposition            | `ScoreComponents`, `ScoreExplanation`, `topContributors` in `sweep/score_explain.go` |
| Experiment schema versioning             | `SchemaVersion`, `RoundRecordSchemaVersion` in `sweep/schema_contracts.go`           |
| Objective version stamping               | `ObjectiveVersion` in `sweep/runner.go`, persisted per sweep                         |
| Explainability endpoint                  | `GET /api/lidar/sweep/explain/{sweep_id}` in `server/sweep_handlers.go`              |
| Label provenance + carry-over confidence | `sweep/hint.go`, HINT state                                                          |
| Class and temporal coverage gates        | HINT continue validation                                                             |

Versioned experiment schema, score decomposition, explainability, and coverage
gating are precisely the reproducibility contract the promotion gate requires.
Classification metrics become an additional scored component inside that
platform, in Go, and inherit persistence and explainability for free. The
`tools/ml-training/*.py` proposal is withdrawn.

### The candidate ladder, and the ceiling probe

Candidates are tiered by auditability. The final tier exists to answer the
open policy question with a number rather than an argument.

| Tier | Method                                    | Deployable | Purpose                                         |
| ---- | ----------------------------------------- | ---------- | ----------------------------------------------- |
| 0    | Current rule cascade                      | Shipped    | Permanent baseline                              |
| 1    | Data-derived threshold tables, same shape | Yes        | Fitted cuts on features already computed        |
| 2    | Logistic regression                       | Yes        | Coefficients are the explanation                |
| 3    | Decision tree, depth ≤ 4, emitted as Go   | Yes        | Ships as generated readable source, not weights |
| X    | Gradient-boosted trees or small MLP       | **Never**  | Research-only headroom probe                    |

Tier X is run offline, never wired to the pipeline, and never promoted. Its only
output is a number: the accuracy gap between the best transparent candidate and
an unconstrained one. If Tier 3 lands close to Tier X, the explainability rule is
demonstrably free and the question is closed on evidence. If the gap is large,
the project has a measured cost to weigh rather than a principle to defend. This
is Tenet 3 applied to the project's own methodology.

### Boundary with the maths coherence plan

The L6 confidence arithmetic is a mathematical defect, not a classification
feature, and is owned by
[lidar-maths-coherence-plan.md](lidar-maths-coherence-plan.md). This plan
consumes the corrected confidence channel. Phase 1 calibration measurement
therefore depends on that item landing first.

## Scope

### Phase 1: classification scorecard

**Summary:** Make classification accuracy measurable inside the existing
evaluation platform.

**Steps:**

1. Introduce a `Classifier` interface in `l6objects`; make the rule cascade the
   first implementation and the default. Additive, no behaviour change.
2. Add classification metrics to the ground-truth evaluation path: confusion
   matrix over the emitted class set, per-class precision, recall, and F1,
   macro-F1, and an unknown/`dynamic` fallthrough rate.
3. Add confidence calibration measurement (reliability curve and expected
   calibration error) against the corrected confidence channel.
4. Surface the new components through `ScoreComponents` so they persist and
   explain alongside the existing detection metrics; bump `ObjectiveVersion`.
5. Record the scorecard definition in `data/maths/classification-maths.md`:
   metric formulae, class set, and the matching rule inherited from
   `computeTemporalIoU`.

**Milestone:** v0.5.2

### Phase 2: corpus contract and feature export

**Summary:** Turn labels from runtime database rows into a versioned, replayable
artefact.

**Steps:**

1. Implement the training-export path in Go over `TrackFeatures` and
   `SortedFeatureNames`, pulling forward the endpoint spec from the
   [unpopulated data structures plan](unpopulated-data-structures-remediation-plan.md)
   Phase 5.
2. Define the corpus artefact: feature vectors plus labels plus provenance
   (run id, objective version, schema version, labeller source). Geometry and
   kinematics only — no PII by construction, so the artefact is repository-safe.
3. Freeze a baseline corpus from the captured kirk0 scene and check it in.
4. Add a regression test asserting the rule baseline's scorecard against the
   frozen corpus, so classifier changes surface as scorecard deltas.

**Milestone:** v0.5.3

### Phase 3: transparent candidate ladder

**Summary:** Fit Tiers 1-3 and benchmark them against the baseline.

**Steps:**

1. Fit data-derived threshold tables; compare against hand-set constants.
2. Fit logistic regression over the documented feature set; persist coefficients
   as the model artefact.
3. Fit a depth-limited decision tree; emit it as generated Go source so review
   and diff apply to the decision path itself.
4. Re-evaluate truck and motorcyclist as live classes once per-class recall on
   the frozen corpus supports it.

**Milestone:** v2.0

### Phase 4: headroom probe

**Summary:** Measure what the explainability constraint costs.

**Steps:**

1. Train an unconstrained model offline against the frozen corpus.
2. Report the macro-F1 gap against the best transparent tier as a published
   scorecard figure.
3. Record the result as a decision entry. No runtime integration, in any outcome.

**Milestone:** v2.0

## Dependencies

- L6 confidence equation from
  [lidar-maths-coherence-plan.md](lidar-maths-coherence-plan.md) gates Phase 1
  calibration measurement.
- MOTA/MOTP (gap ID M1) and the L8 analytics maths note share the metric
  substrate with Phase 1; sequence them together to avoid two metric layers.
- Phase 2 corpus breadth is limited by
  [lidar-test-corpus-plan.md](lidar-test-corpus-plan.md): 1 of 5 scenes captured.
  Phases 1-2 are valid on one scene; Phase 3 should not conclude on one scene.
- Labelling throughput from the
  [track labelling plan](lidar-track-labelling-auto-aware-tuning-plan.md)
  Phases 1-5, which are complete.

## Risks

| Risk                                                           | Likelihood | Impact | Mitigation                                                                          |
| -------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------- |
| Single-scene corpus overfits the scorecard                     | High       | Medium | Phases 1-2 only claim per-scene validity; Phase 3 gated on additional captures      |
| Scorecard becomes a second metric layer beside MOTA/MOTP       | Medium     | Medium | Share the matching rule and the `ScoreComponents` channel; sequence with M1         |
| Tier X result is used to argue for deployment                  | Low        | High   | Tier X is non-deployable by plan; promotion gate unchanged                          |
| Class-set drift between classifier output and label vocabulary | Medium     | Medium | Scorecard fixes the class set explicitly; reuses the display/selectable label split |
| Adding the interface invites premature model wiring            | Low        | Medium | Rule cascade stays the default and the fallback; no config switch until Phase 3     |

## Checklist

### Complete

- [x] Guardrails and promotion gate defined (carried forward from the previous revision)

### Outstanding

- [ ] Correct the canonical pointer and companion links; this revision (`S`)
- [ ] Phase 1: `Classifier` interface, rule cascade as first implementation (`S`)
- [ ] Phase 1: confusion matrix, per-class P/R/F1, macro-F1, fallthrough rate (`M`)
- [ ] Phase 1: calibration curve and expected calibration error (`S`)
- [ ] Phase 1: wire metrics into `ScoreComponents`, bump `ObjectiveVersion` (`S`)
- [ ] Phase 1: scorecard definition recorded in `classification-maths.md` (`S`)
- [ ] Phase 2: Go training-export path over `TrackFeatures` (`M`)
- [ ] Phase 2: corpus artefact format and frozen kirk0 baseline (`M`)
- [ ] Phase 2: baseline scorecard regression test (`S`)
- [ ] Phase 3: Tiers 1-3 fitted and benchmarked (`L`)
- [ ] Phase 3: truck and motorcyclist re-evaluation (`S`)
- [ ] Phase 4: headroom probe and published gap figure (`M`)

### Deferred

- [ ] Rename the plan file to match its scope; deferred to avoid breaking the
      backlog link mid-cycle
- [ ] Exposing L6 thresholds as config, tracked by the config restructure
      Phase 3 backlog item; Phase 3 of this plan supersedes the need if fitted
      tables land first

### Accepted residuals (no action planned)

- [ ] The rule cascade remains order-dependent by design; the scorecard measures
      the cascade as shipped rather than each rule in isolation
- [ ] Tier X carries no reproducibility guarantee beyond its published gap figure,
      because it is never promoted
