# Open research questions

Exploratory research topics for data scientists, ML engineers,
and perception researchers interested in contributing to
velocity.report. Each question needs evidence, not opinion.

This file is the canonical index of unanswered questions across
the pipeline. Questions graduate to [docs/DECISIONS.md](../docs/DECISIONS.md)
when they have a recorded answer with artefact provenance.

**Contribution protocol:** include the question being answered,
the observed result, the exact parameter bundle, the validation
date, and the replay artefacts used (`.pcap`, `.vrlog`, scene
IDs, run IDs, baselines, and any LFS-backed files). Claims
without artefacts are anecdotes.

**Constraint:** every algorithm must remain inspectable,
tuneable, and explainable. A model that performs well but
cannot explain itself is not welcome on the critical path.
See [CONTRIBUTING.md §Data Scientist](../CONTRIBUTING.md#data-scientist) and
[platform-data-science-metrics-first-plan.md](../docs/plans/platform-data-science-metrics-first-plan.md).

---

## 1. Tracking geometry and bounding box stability

The most visible user-facing problem: bounding boxes that spin,
change shape, or fail to capture all cluster points.

### Q1. do the OBB heading fixes hold up without geometry-coherent tracking?

The [2026-02-22 OBB review](maths/proposals/20260222-obb-heading-stability-review.md)
applied Guards B, C, G. Do they survive replay on multiple
sites, or does the [geometry-coherent model](maths/proposals/20260222-geometry-coherent-tracking.md)
remain necessary to stop axis flipping, dimension swap, and
heading rotation?

- **Evidence needed:** Dimension stability σ, heading drift
  rate, 90° jump frequency on kirk0 + at least two further
  PCAPs.
- **Decision:** [D-04](../docs/DECISIONS.md) schedules P1 for
  v0.6. This question validates whether the simpler guards
  are a viable intermediate step.

### Q2. how should the PCA axis ambiguity be resolved for near-square clusters?

Pedestrians and slow vehicles produce near-square clusters
where eigenvalue near-equality causes the principal axes to
flip between frames. The geometry-coherent proposal uses a
Bayesian prior with Mahalanobis residual selection. Is this
sufficient, or do near-square objects need a different
disambiguation model (e.g. motion-only heading)?

- **Evidence needed:** Axis-flip frequency by object class on
  labelled tracks. Comparison of Bayesian disambiguation vs
  motion-heading fallback for aspect ratios < 1.5.

### Q3. what convergence time does the bayesian geometry model need?

The proposal claims 5–10 frames vs 15–20 for reactive guards.
Validate on labelled tracks across vehicle, cyclist, and
pedestrian classes.

- **Evidence needed:** Per-class convergence curves (dimension
  σ vs frame count) on at least three sites.
- **Reference:** [geometry-coherent-tracking.md §convergence](maths/proposals/20260222-geometry-coherent-tracking.md)

---

## 2. Foreground extraction and motion models

Separating moving objects from the static scene: the
foundation that clustering and tracking depend on.

### Q4. does velocity-coherent extraction beat the baseline?

The [velocity-coherent proposal](maths/proposals/20260220-velocity-coherent-foreground-extraction.md)
hypothesises 20–40% improvement in sparse object recall and
10–25% reduction in fragmentation. These claims need benchmark
evidence before the baseline is replaced.

- **Acceptance gates:**
  - Track completeness (temporal IoU ≥ 0.5): ≥ 10% absolute
    improvement.
  - Fragmentation: < 1.2 tracks per ground-truth vehicle,
    < 1.5 for pedestrians.
  - Speed RMSE: no regression > 5%.
- **Protocol:** Same PCAP, same downstream parameters, scored
  via `GroundTruthEvaluator`.
- **Decision:** [D-05](../docs/DECISIONS.md) sequences P2
  after P1.

### Q5. can sparse long-range points be reliably promoted to foreground?

At ranges > 40 m the LiDAR returns < 5 points per object per
frame. Motion-aware background promotion (using predicted track
positions to rescue near-threshold points) could improve recall
but risks false positives.

- **Evidence needed:** False-positive rate of motion-aware
  promotion at 30 m, 40 m, 50 m ranges. Comparison with and
  without track-assisted promotion on sparse-vehicle PCAPs.

### Q6. which clustering engine performs best?

DBSCAN is the current production path. HDBSCAN may handle
variable-density scenes better. The deterministic
`DBSCANClusterer` variant avoids subsampling non-determinism
but may be slower.

- **Evidence needed:** Clustering quality (ARI, completeness)
  and throughput (ms/frame) for DBSCAN, HDBSCAN, and
  voxel-grid-preprocessed variants on the standard PCAP pack.

---

## 3. Ground plane and scene geometry

The terrain under the sensor: getting this right means better
foreground separation, fewer phantom objects, and the
foundation for corridor-aware tracking.

### Q7. when does the height-band filter stop being good enough?

The current filter is O(n), zero-parameter, and works well on
flat roads. Three failure modes are documented: sloped roads
(1.5 m height error at 30 m range on a 3° gradient), kerb
boundaries (5–15% false foreground), and long-running drift.

- **Evidence needed:** Replay comparison (height-band vs
  tile-plane PCA) on sloped-road and kerbed-pavement PCAPs.
  Quantify false-foreground rate and track completeness
  change.
- **Decision:** [pipeline-review Q1](maths/pipeline-review-open-questions.md)
  recommends implementing tile-plane fitting.

### Q8. how should tile-plane boundaries be set?

Fixed 1 m tiles, adaptive tiles, or region-based? The
streaming PCA approach assumes fixed tiles, but kerbs and
boundaries may need sub-tile resolution or curved fits.

- **Evidence needed:** Plane-fit residual distribution for
  0.5 m, 1.0 m, 2.0 m tiles on mixed-geometry scenes.
  Curvature-detection false-positive rate at tile boundaries.

### Q9. how should observed geometry align with OSM priors?

Observed tile-plane polygons need to be diffed, reviewed, and
exported against OpenStreetMap without weakening provenance.
The alignment model uses rigid transforms (translation +
rotation) with feature-matched confidence.

- **Evidence needed:** Translation/rotation residual
  distribution on sites with known OSM coverage. False-edit
  rate for changeset proposals.
- **Reference:** [pipeline-review Q2](maths/pipeline-review-open-questions.md),
  [vector-scene-map.md](../docs/lidar/architecture/vector-scene-map.md)

---

## 4. Sensor fusion and multi-sensor architecture

Combining radar velocity with LiDAR spatial data and
eventually merging overlapping fields of view.

### Q10. should radar + LiDAR fusion happen at L5 or L7?

Current design associates radar speed at the per-track level
(L5). The alternative is scene-level fusion at L7 where
canonical objects from multiple sensors are merged.

- **Evidence needed:** Speed accuracy comparison (L5
  per-track association vs L7 canonical-object fusion) on
  scenes with both radar and LiDAR coverage. Latency and
  complexity comparison.
- **Reference:** [tracking-maths.md §fusion](maths/tracking-maths.md),
  [lidar-l7-scene-plan.md](../docs/plans/lidar-l7-scene-plan.md)

### Q11. how should conflicting multi-sensor observations be resolved?

When radar and LiDAR disagree on speed, heading, or
classification, the conflict resolution strategy must be
principled and produce auditable confidence scores.

- **Evidence needed:** Frequency and magnitude of
  radar/LiDAR disagreements on dual-sensor captures.
  Comparison of voting, weighted-mean, and
  highest-confidence-wins strategies.

---

## 5. Classification and labelling

Turning tracked objects into named categories: currently a
rule-based cascade, with explicit questions about whether
interpretable ML could do better.

### Q12. should the rule-based classifier be augmented with interpretable ML?

The current 7-tier cascade uses height, width, speed, and
observation count. An interpretable model (logistic regression,
shallow decision tree, gradient-boosted stumps) could improve
boundary decisions while remaining auditable.

- **Constraint:** Must remain explainable. No black-box
  models on the critical path.
- **Evidence needed:** Classification accuracy (per-class
  precision/recall) of rule cascade vs interpretable ML on
  labelled tracks. Feature importance analysis to validate
  that ML uses physically meaningful features.
- **Reference:** [classification-maths.md](maths/classification-maths.md),
  [lidar-ml-classifier-training-plan.md](../docs/plans/lidar-ml-classifier-training-plan.md)

### Q13. what evidence justifies re-enabling truck and motorcyclist classes?

The v0.5.0 classifier disables truck (> 5.5 m) and
motorcyclist (5–30 m/s, narrow) classes. What labelled data
and false-positive rates are needed before re-activation?

- **Evidence needed:** Per-class false-positive rate on
  labelled PCAPs with known truck and motorcyclist presence.
  Confusion matrix showing that re-enabled classes do not
  degrade car/pedestrian/cyclist accuracy.

### Q14. how should quality labels be aggregated into tuning scores?

Track-level quality labels (`perfect`, `good`, `truncated`,
`noisy_velocity`, `stopped_recovered`) need a scoring function
that auto-tuning can optimise against.

- **Evidence needed:** Correlation between label categories
  and downstream speed accuracy. Proposed weighting scheme
  validated on labelled runs.
- **Reference:** [track-labelling plan](../docs/plans/lidar-track-labelling-auto-aware-tuning-plan.md)

---

## 6. Parameter tuning and benchmark coverage

Every default has provenance: or it should. Roughly eight
config keys are provisional (tuned on kirk0 only).

### Q15. which defaults survive multi-site validation?

Provisional defaults were tuned against one PCAP at one site.
The five-PCAP test corpus plan requires validation across flat
urban, sloped residential, school zone, multi-lane junction,
and rural road captures.

- **Evidence needed:** Per-key performance (metric vs default
  value) across all five sites. Keys that diverge > 10%
  across sites need adaptive or site-profile defaults.
- **Reference:** [config/README.maths.md §6](../config/README.maths.md),
  [pipeline-review Q7](maths/pipeline-review-open-questions.md),
  [pipeline-review Q11](maths/pipeline-review-open-questions.md)

### Q16. what does the auto-tuning objective function look like?

Should the objective optimise detection rate, fragmentation,
velocity noise, or a weighted multi-objective scorecard? How
are objectives weighted and rebalanced across site classes?

- **Evidence needed:** Pareto frontier analysis of detection
  vs fragmentation vs speed accuracy on multi-site sweeps.
- **Reference:** [auto-tuning.md](../docs/lidar/operations/auto-tuning.md),
  [parameter-tuning plan](../docs/plans/lidar-parameter-tuning-optimisation-plan.md)

### Q17. does kirk0 overfit?

All provisional defaults were tuned against a single capture
at one site with one sensor model. How degraded are results
on different road geometries, traffic mixes, and weather
conditions?

- **Evidence needed:** Cross-site validation matrix. Run
  kirk0-optimal parameters on planned PCAPs #2–#5 and
  report per-metric degradation.
- **Reference:** [pipeline-review Q11](maths/pipeline-review-open-questions.md)

---

## 7. Kinematic model extensions

The current constant-velocity Kalman filter fragments tracks
when vehicles brake, accelerate, or turn.

### Q18. does adding acceleration states reduce fragmentation?

A CA (constant-acceleration) model extends the state vector
from 4 to 6 dimensions. The pipeline-review estimates +1 ms
per frame cost.

- **Evidence needed:** Fragmentation rate (tracks per
  ground-truth object) for CV vs CA on captures with known
  braking events. Speed accuracy comparison.

### Q19. is IMM worth the complexity?

Interacting Multiple Model blending (CV + CA) adapts per track
per frame. Estimated +3 ms. Is the improvement over CA alone
justifiable on edge hardware?

- **Evidence needed:** Fragmentation and speed accuracy for
  CA vs IMM on manoeuvring-vehicle PCAPs. Per-frame timing on
  Raspberry Pi 4.

### Q20. are L5 kinematic fixes future-forward, or will L7 corridors supersede them?

The [pipeline-review Q5](maths/pipeline-review-open-questions.md)
argues that CTRV and ad-hoc sparse-cluster linking at L5 are
throwaway work because L7 corridors will handle turns and
occlusion more robustly.

- **Evidence needed:** Comparison of L5-only CA/IMM vs
  L7-corridor-constrained CV on turning-vehicle PCAPs.
  Identify which L5 investments survive L7 adoption.

---

## 8. Pose stability and static anchors

The sensor is stationary but not perfectly still: thermal
expansion, wind, and mast flexion produce micro-movements
that degrade long-running accuracy.

### Q21. can reflective signs serve as reliable pose anchors?

Retroreflective signs return intensity 50–200× background.
The [pose-anchor proposal](maths/proposals/20260310-reflective-sign-pose-anchor-maths.md)
claims σ_translation < 1 mm after 50 frames.

- **Evidence needed:** Measured sign detection rate and pose
  residual across varying ranges, angles of incidence, and
  weather conditions.

### Q22. what fallback hierarchy works in sign-poor scenes?

When signs are absent: walls (σ < 5 mm perpendicular only),
ground support (σ_z < 10 mm). Is the combined fallback
sufficient for shake diagnostics?

- **Evidence needed:** Pose stability comparison for scenes
  with signs vs wall-only vs ground-only anchors.

---

## 9. Analytics and traffic engineering metrics

The numbers this project produces go to community meetings
and council chambers. They must withstand scrutiny.

### Q23. how should speed percentiles be aggregated?

Percentiles do not aggregate: you cannot average p85 values
across time bins.
The [speed-percentile plan](../docs/plans/speed-percentile-aggregation-alignment-plan.md)
reserves p50/p85/p98 for grouped/report metrics.

- **Evidence needed:** Error magnitude of naive percentile
  averaging vs correct re-computation from raw data, across
  typical 15-minute and hourly bins.
- **Decision:** [D-18](../docs/DECISIONS.md)

### Q24. what sample size is needed for defensible p85 reporting?

Traffic engineers use the 85th percentile speed as a standard
metric. What minimum sample size per time bin produces a p85
estimate with ≤ 2 km/h confidence interval?

- **Evidence needed:** Bootstrap analysis of p85 confidence
  interval vs sample size, stratified by traffic density and
  speed distribution shape.

### Q25. are speed distributions normal, bimodal, or something else?

Many statistical methods assume normality. Urban speed
distributions are often bimodal (free-flow vs platooned) or
right-skewed. What distribution families fit observed data?

- **Evidence needed:** Goodness-of-fit tests (Shapiro–Wilk,
  Anderson–Darling) on per-site, per-hour speed distributions.
  Comparison of parametric vs non-parametric confidence
  intervals.

---

## 10. Edge hardware and performance

All algorithms run on a Raspberry Pi 4 at 10 Hz. The current
pipeline uses 23% of the 100 ms frame budget.

### Q26. do all proposed improvements fit within the frame budget?

The pipeline-review estimates all proposals combined add
12–17 ms, leaving ~40% headroom. Validate with measured
per-layer timing on Pi 4 hardware.

- **Evidence needed:** Per-layer benchmarks on ARM
  Cortex-A72. Identify any proposal that exceeds its
  estimated budget.
- **Reference:** [pipeline-review Q10](maths/pipeline-review-open-questions.md)

---

## Cross-cutting: experimental infrastructure

Several questions above depend on infrastructure that does
not yet exist or is incomplete.

### I1. five-site PCAP test corpus

Most questions require validation beyond kirk0. Four
additional captures are planned but not yet recorded.
See [lidar-test-corpus-plan.md](../docs/plans/lidar-test-corpus-plan.md).

### I2. ground-truth labelling pipeline

Track-level labels are needed for classification accuracy,
fragmentation measurement, and tuning validation. The macOS
visualiser Phase 9 provides the labelling UI.
See [track-labelling plan](../docs/plans/lidar-track-labelling-auto-aware-tuning-plan.md).

### I3. parameter sweep framework

Multi-key sweeps across multiple sites require automated
experiment runners with provenance tracking.
See [parameter-tuning plan](../docs/plans/lidar-parameter-tuning-optimisation-plan.md).

### I4. performance measurement harness

Per-layer timing on Pi 4 hardware for regression detection.
See [performance-harness plan](../docs/plans/lidar-performance-measurement-harness-plan.md).

---

## Index by pipeline layer

| Layer         | Questions                          |
| ------------- | ---------------------------------- |
| L3 Grid       | Q4, Q5, Q7, Q15, Q17               |
| L4 Perception | Q6, Q7, Q8, Q12                    |
| L5 Tracks     | Q1, Q2, Q3, Q4, Q10, Q18, Q19, Q20 |
| L6 Objects    | Q12, Q13, Q14                      |
| L7 Scene      | Q9, Q10, Q11, Q20, Q21, Q22        |
| L8 Analytics  | Q23, Q24, Q25                      |
| Cross-cutting | Q15, Q16, Q17, Q26, I1–I4          |
