# Radiance-field scene-model review for the LiDAR pipeline

- **Status:** Review note (decision); v1.0
- **Layers:** L3 Grid, L4 Perception, L5 Tracks, L7 Scene
- **Decision posture:** Evaluates radiance-field-style models against the
  already-designed L7 + vector-scene-map path.
- **Canonical:** [LIDAR_ARCHITECTURE.md](../lidar/architecture/LIDAR_ARCHITECTURE.md),
  [vector-scene-map.md](../lidar/architecture/vector-scene-map.md),
  [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md),
  [lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md)

## 1. Summary

We evaluated whether radiance-field-style ML methods — NeRF, 3D Gaussian
splatting, neural occupancy fields, neural SDFs, dynamic scene fields, and
hybrid explicit+learned scene maps — should play a role in the
velocity.report LiDAR pipeline.

**Recommendation:** Do not adopt radiance-field methods in the production
pipeline. The already-planned classical L7 Scene layer (Bayesian evidence
grid with log-odds updates, Welford-refined canonical objects, Mahalanobis
cross-sensor association, Procrustes prior alignment) plus the vector
scene map (explicit polygons, plane equations, bounding volumes, LOD 0–3)
**supersedes** the use cases a radiance field would address, while
preserving privacy, explainability, audit-grade replay, and Raspberry Pi
compute economics.

A narrow, opt-in, **offline-only** research lane is permitted for
visualisation and reconstruction QA of recorded sessions. It must produce
advisory overlays — never metrics, never civic-report content, never
inputs to any production layer (L4, L5, L6, L7, L8).

This recommendation is consistent with — and follows directly from — the
[ML classifier plan](lidar-ml-classifier-training-plan.md)'s explicit
guardrails ("opaque end-to-end models are out of scope"; "the decision
path remains explainable"). Radiance-field methods would violate those
guardrails at L7 just as they would at L6. §4.1 enumerates the conflict
clause by clause. §6.4 explains why more edge compute (Coral, Hailo,
Jetson) does not change the conclusion. §10.1 lists the concrete
offline use cases the permitted research lane is meant to address.

## 2. Non-goals

- Not a comparison of NeRF variants. We treat "radiance-field-style"
  broadly and pick the strongest plausible LiDAR-only formulation in
  each section.
- Not a benchmark study. We have no recorded multi-view dataset and no
  GPU on the target deployment platform.
- Not a critique of learned models in general. The ML classifier plan
  ([lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md))
  already defines the legitimate envelope for learned methods at L6.
- Not a graduation of L7. L7 remains in 📋 Planned per
  [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md). This document is a
  decision note about a different family of methods.

## 3. Current pipeline alignment

### 3.1 Where a scene model could plausibly attach

The frozen ten-layer model
([LIDAR_ARCHITECTURE.md](../lidar/architecture/LIDAR_ARCHITECTURE.md))
has five layers where a learned scene representation could in principle
be wired in:

| Layer         | Current role                                                        | Plausible radiance-field role                                            |
| ------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| L3 Grid       | Polar EMA background, foreground gating, per-ring variance          | Learned background density as a prior over EMA                           |
| L4 Perception | Height-band filter, voxel downsample, DBSCAN, OBB/PCA               | Learned occupancy field to guide foreground extraction or ground removal |
| L5 Tracks     | Hungarian + Kalman, OBB heading, coasting                           | Learned scene-conditioned occlusion prediction for track gap filling     |
| L6 Objects    | Feature aggregation, rule-based classifier                          | (Out of scope here; covered by the ML classifier plan)                   |
| **L7 Scene**  | **Reserved.** Planned as Bayesian evidence grid + canonical objects | **Natural home** if any learned scene representation were ever adopted   |

L7 is the _only_ layer where a persistent learned scene representation
fits the architecture. L3/L4/L5 are explicitly stateless or per-track and
must not depend on a back-end that requires cross-frame training or
GPU-resident parameter blobs. The
[L7 plan](lidar-l7-scene-plan.md) already places "persistent
evidence-accumulated world model" at L7.

### 3.2 What the L7 plan already commits to

The existing L7 plan is _not_ an empty slot waiting for a representation.
It commits to:

- **Bayesian evidence grid with log-odds updates** (the OctoMap
  formulation; Hornung et al., 2013) for static-geometry confidence
  accumulation — applied to _vector features_, not voxels
  ([lidar-l7-scene-plan.md §3.1](lidar-l7-scene-plan.md)).
- **Welford running statistics** for canonical-object dimension
  refinement ([§3.2](lidar-l7-scene-plan.md)).
- **Mahalanobis-gated cross-sensor association** for track handoff
  ([§3.3](lidar-l7-scene-plan.md)).
- **Procrustes rigid alignment** (SVD-based) for OSM prior registration
  ([§3.4](lidar-l7-scene-plan.md)).
- **Vector scene map** as the storage and query substrate
  ([vector-scene-map.md](../lidar/architecture/vector-scene-map.md)):
  polygons + plane equations + bounding volumes, LOD 0–3, ~35 KB
  compressed per 100 m × 100 m scene.

Every problem a radiance field would be invited to solve — persistent
scene model, occlusion reasoning, multi-frame fusion, multi-sensor
merging, scene-conditioned motion prediction — already has a designed,
explainable, low-compute classical answer in this plan.

### 3.3 What the current code looks like today

There is **no L7 package**. There are **no learned-model dependencies**
in `go.mod` (no ONNX, no torch, no tensorflow). Classification at
[internal/lidar/l6objects/](../../internal/lidar/l6objects/) is rule-based
and feature-driven. Replay is deterministic via VRLOG: every sensor frame
yields a frame-bundle entry — including empty frames — so a parameter
sweep or A/B compare can be run against immutable inputs
([internal/lidar/l9endpoints/](../../internal/lidar/l9endpoints/)).

The runtime envelope is a Raspberry Pi at ~10 Hz with no GPU. The
sweep harness ([internal/lidar/sweep/](../../internal/lidar/sweep/))
scores parameter sets via measurable signals (acceptance rate,
velocity-trail alignment, fragmentation, jitter) — all derived from
classical intermediate artefacts.

## 4. Explainability constraints

We start from the project's hard commitments. [TENETS.md](../../TENETS.md):
"Privacy above all", "Evidence over opinion", "Local-first, offline-capable",
"Simplicity and durability". [Vector scene map](../lidar/architecture/vector-scene-map.md)
§ Architecture principles: "Privacy-first: No camera data, no PII. Only
geometric features (polygons, planes, volumes)." ML classifier plan:
"Opaque end-to-end models are out of scope for this lane."

Eight explainability properties the current pipeline preserves:

| Property                              | Today                                                                                                                                   |
| ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| Deterministic replay                  | VRLOG 1:1 PCAP correspondence; param changes apply to _new_ runs                                                                        |
| Inspectable intermediate artifacts    | Polar grid, foreground points, clusters, OBBs, Kalman states, tracks                                                                    |
| Human-readable civic reporting        | PDF report ([internal/report/](../../internal/report/)) cites measured counts and speeds from `radar_data_transits` and tracked objects |
| Reproducible before/after comparisons | Sweep harness with versioned params + immutable inputs                                                                                  |
| Ability to explain a count            | Each track has an OBB history, classification feature vector, and quality metrics                                                       |
| Ability to debug a false positive     | Inspect cluster → tracks → background-cell variance at the relevant grid coordinate                                                     |
| Local, offline operation              | All compute on-device; SQLite store; no cloud calls                                                                                     |
| Auditable from stored data            | `lidar_run_tracks` + VRLOG snapshot is sufficient to reproduce L8 analytics                                                             |

Evaluation of each radiance-field option against these eight:

| Method                             | Determ. replay | Inspectable IA | Civic-report fit | Repro a/b   | Explain count | Debug FP/FN | Local-only   | Auditable from store |
| ---------------------------------- | -------------- | -------------- | ---------------- | ----------- | ------------- | ----------- | ------------ | -------------------- |
| NeRF (RGB)                         | weakens        | weakens        | breaks privacy   | weakens     | breaks        | weakens     | needs GPU    | weakens              |
| NeRF (LiDAR-only, e.g. LiDAR-NeRF) | weakens        | weakens        | weakens          | weakens     | breaks        | weakens     | needs GPU    | weakens              |
| 3D Gaussian splatting (RGB)        | breaks         | breaks         | breaks privacy   | breaks      | breaks        | breaks      | needs GPU    | breaks               |
| Gaussian splatting (LiDAR-only)    | weakens        | weakens        | weakens          | weakens     | breaks        | weakens     | needs GPU    | weakens              |
| Neural occupancy field             | weakens        | weakens        | weakens          | weakens     | weakens       | weakens     | possibly CPU | weakens              |
| Neural SDF                         | weakens        | weakens        | weakens          | weakens     | weakens       | weakens     | possibly CPU | weakens              |
| Dynamic-scene NeRF (D-NeRF)        | breaks         | breaks         | weakens          | breaks      | breaks        | breaks      | needs GPU    | breaks               |
| Hybrid explicit + learned          | preserves\*    | preserves\*    | preserves\*      | preserves\* | preserves\*   | preserves\* | possibly     | preserves\*          |

\* Only when the explicit side remains the source of truth and the
learned side is advisory (overlay-only, confidence-only, or hole-fill
that is _checked_ by classical geometry before exposure).

The pattern is clear: every option that **replaces** classical
geometry weakens or breaks at least one hard property. The only
configuration that survives is one where the learned component is an
_advisory overlay_ checked against classical L4/L5/L7 outputs — at
which point we may as well ask whether the learned component is
purchasing anything we don't already have.

### 4.1 Alignment with the ML classifier plan's guardrails

The [ML classifier plan](lidar-ml-classifier-training-plan.md) is the
project's canonical statement on which learned methods are permitted
inside the pipeline. It is written about L6 classification, but its
five guardrails encode the project's stance on runtime ML in general —
nothing in their wording is L6-specific. Radiance-field methods fail
multiple guardrails, not by accident but by category.

| #   | Guardrail (verbatim from classifier plan)                                                                  | Radiance-field methods                                                                                                                                                           |
| --- | ---------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| G1  | "The live pipeline keeps the current rule-based classifier as the default and fallback path."              | Fails by analogy. There is no classical fallback for a learned scene field — the field _is_ the representation. Hybrid (architecture C in §5) only narrowly survives.            |
| G2  | "Candidate methods must use documented, exportable feature vectors."                                       | Fails. NeRF, splat, and SDF parameters are dense network weights or millions of primitive parameters, not feature vectors. Per-cell density samples are not human-meaningful.    |
| G3  | "Benchmark wins must be demonstrated on fixed replay packs and labelled runs."                             | Conditionally passable but expensive. Per-scene retraining means a new model per replay pack; longitudinal comparisons require freezing model + weights + RNG seed.              |
| G4  | "Deployment is allowed only when metric gains are reproducible and the decision path remains explainable." | Fails on the second clause. "Explain why this cell was rendered occupied" cannot be answered from network weights without additional interpretability tooling we have not built. |
| G5  | "Opaque end-to-end models are out of scope for this lane."                                                 | Fails directly. NeRF and 3D Gaussian splatting are exactly this category.                                                                                                        |

The same plan defines a four-point promotion gate. Radiance-field
methods would have to:

1. **Beat the rule-based / classical baseline on the agreed scorecard.**
   _Hard._ The planned L7 evidence grid is already cheap and accurate
   for the static-scene problem; there is no scorecard a learned field
   could win on without changing what is being measured.
2. **Avoid regressions in critical classes or noise handling.**
   _Possible_, but the failure mode shifts from interpretable noise (a
   noisy cluster) to opaque hallucination (a confidently rendered
   non-existent surface). The latter is harder to detect and harder to
   defend.
3. **Be reproducible from versioned inputs and feature exports.**
   _Hard._ NeRF training is stochastic, weight files are large, and
   "feature exports" do not apply to dense implicit fields. Determinism
   requires freezing the entire training stack.
4. **Ship with enough metadata to audit why a class decision was made.**
   _Hard._ This is the core black-box problem. Saliency and attribution
   methods for NeRF/splatting are an active research area, not a
   shippable feature.

**Could the "L7 not L6" argument escape these guardrails?** No. The
guardrails are written about runtime ML, not about L6 specifically.
Their spirit — _no opaque end-to-end models in the live pipeline_ —
applies wherever a learned model would shape a measurement the system
later reports. L7 is squarely inside that envelope because L7
representations feed downstream decisions: L7 scene constraints
influence L5 prediction and L8 analytics
([lidar-l7-scene-plan.md §4](lidar-l7-scene-plan.md)). Placing the
learned model at L7 rather than L6 does not change its effect on the
chain of custody.

The classifier plan does explicitly permit learned methods _offline_
("research lane", "not on the critical path"). Architecture A in §5
sits inside that permission. Architectures B and C sit outside it.

## 5. Candidate architectures

We enumerate four integration patterns, in increasing intrusion.

### A. No production integration (offline research only)

- **Layer placement:** None. Lives in `tools/` (proposed
  `tools/scene-recon/`), reads VRLOG and PCAP.
- **Inputs:** Recorded VRLOG bundles, optional background snapshots.
- **Outputs:** Visualisation-only reconstructions (point density,
  surface, splat overlay) usable in the macOS visualiser or as
  rendered preview images for QA.
- **Storage:** Local cache only; nothing in SQLite; nothing in VRLOG.
- **Explainability:** Production pipeline unchanged. Reconstruction is
  labelled as "research overlay" with model version + input range.
- **Failure modes:** Bad reconstruction. No downstream impact because
  no production layer reads from it.
- **Privacy:** LiDAR-only. No camera ingestion. Reconstructions are
  geometric (no identifying features by construction).
- **Runtime cost:** Offline; can use desktop GPU if anyone has one.
  Never runs on the Raspberry Pi.
- **Fits repo boundaries:** Yes. Mirrors existing `tools/` style; no
  `internal/` changes; no `go.mod` additions to the radar binary.

### B. L7 scene prior (learned static background)

- **Layer placement:** L7, advisory. L3/L4/L5 unchanged.
- **Inputs:** Aggregated foreground-removed point cloud over many
  frames (the same data already feeding the planned vector scene map
  construction).
- **Outputs:** A learned density/occupancy estimate as a _secondary_
  scene feature, alongside the canonical vector polygons and Bayesian
  evidence grid.
- **Storage:** Additional blob in `lidar_bg_snapshot` or a new
  `lidar_scene_learned` table. Model weights ≪ 50 MB to fit RPi.
- **Explainability:** Weakens L7 audit story. Two sources of truth
  (classical Bayesian grid + learned field) require explicit
  reconciliation rules; the L7 plan's log-odds formulation is
  Bayesian-clean today and would become harder to defend.
- **Failure modes:** Drift between classical and learned views; users
  cannot tell which is "correct"; debugging requires retraining.
- **Privacy:** LiDAR-only is feasible; preserves the camera-free
  story.
- **Runtime cost:** Training offline. Inference on RPi for an
  implicit field is non-trivial; periodic query cost ≥ 50 ms/query
  with naive MLPs.
- **Fits repo boundaries:** Poorly. Introduces a learned runtime
  dependency to the radar binary; conflicts with the
  classifier-plan's "no opaque end-to-end models" rule even though
  the target layer is L7 rather than L6, because the spirit of that
  rule is runtime auditability.

### C. Hybrid explicit scene map with learned confidence

- **Layer placement:** L7, in the existing vector-scene-map data
  flow. Classical polygons remain the source of truth.
- **Inputs:** Same as B.
- **Outputs:** Learned confidence/hole-fill _annotations_ on existing
  polygons (e.g. "this kerb segment is likely continuous through the
  occluded zone, conf 0.8") that L7 can choose to surface or hide.
- **Storage:** Confidence floats per vector feature; no new tables.
- **Explainability:** Tolerable if the learned output is _only_ used
  to colour the visualiser overlay and _never_ to alter L4/L5/L8
  outputs.
- **Failure modes:** Scope creep. The temptation to let learned
  confidence influence track gap filling, classification, or report
  numbers is enormous. The architectural rule "advisory only" has to
  be enforced by review, not by code.
- **Privacy:** LiDAR-only; preserves camera-free story.
- **Runtime cost:** Smaller than B (annotation, not field query).
- **Fits repo boundaries:** Marginal. Strictly better than B but
  still introduces a runtime learned dependency.

### D. ML-assisted labelling and evaluation

- **Layer placement:** Outside the runtime pipeline; sibling to the
  existing ML classifier plan's `tools/ml-training/` workspace.
- **Inputs:** Recorded VRLOG + PCAP packs.
- **Outputs:** Generated training labels, synthetic viewpoints for
  evaluation, scene-difference reports for sensor-pose change
  detection.
- **Storage:** Labels in the existing labelling store; no production
  impact.
- **Explainability:** Production untouched. Used purely upstream of
  the classifier-plan's promotion gate.
- **Failure modes:** Bad labels; caught by the existing scorecard
  before any candidate model is promoted.
- **Privacy:** LiDAR-only path preserves story. RGB-fed NeRF for
  label generation would _not_ preserve the story even if used
  offline — the moment we ingest camera frames anywhere in the
  workflow, the project's "no cameras" claim becomes harder to
  defend in public.
- **Runtime cost:** Offline; one-off training and inference jobs.
- **Fits repo boundaries:** Yes for LiDAR-only label generation. No
  for any RGB-conditioned variant.

### Summary of architectures

| Option                                           | Privacy | Audit | Compute | Worth pursuing now?                  |
| ------------------------------------------------ | ------- | ----- | ------- | ------------------------------------ |
| A. Offline research only                         | ✅      | ✅    | offline | **Yes, narrow & opt-in**             |
| B. L7 scene prior (learned background)           | ✅      | ❌    | RPi-bad | No                                   |
| C. Hybrid + learned confidence                   | ✅      | ⚠️    | RPi-bad | No                                   |
| D. ML-assisted labelling (LiDAR-only)            | ✅      | ✅    | offline | Maybe; covered by ML classifier plan |
| D'. ML-assisted labelling (RGB-conditioned NeRF) | ❌      | ✅    | offline | No                                   |

## 6. Mathematical fit

### 6.1 What radiance fields are good at

NeRF and related implicit methods excel at _dense multi-view
reconstruction of largely static scenes from many overlapping camera
or LiDAR observations_. They learn a continuous function
$F_\theta : (x, y, z, \mathbf{d}) \to (\sigma, c)$ that emits density
and (for RGB variants) colour given position and view direction.
Gaussian splatting replaces the implicit field with explicit
oriented Gaussian primitives optimised by photometric loss. Neural
SDFs encode geometry as the zero level-set of a learned distance
function.

These methods reward: many overlapping views, dense per-frame
coverage, static or quasi-static geometry, photometric or
geometric loss signals strong enough to constrain the field.

### 6.2 What velocity.report actually has

The opposite, mostly.

- **Sparse 40-ring LiDAR (Hesai Pandar40P)** at 10 Hz: per-frame
  coverage is sparse, especially at range. A single sensor sees ~13°
  vertical FOV with non-uniform ring spacing. Per-frame point counts
  per cubic metre at 30 m range are in the tens to low hundreds,
  not the thousands NeRF training assumes.
- **Single-viewpoint per sensor**: each sensor is fixed. The
  multi-view geometry that gives NeRF its supervisory signal is
  absent.
- **Dynamic objects are the entire point**: vehicles, pedestrians,
  cyclists. Dynamic-scene NeRF variants (D-NeRF, NeRFlow) handle
  this poorly and at high training cost.
- **No cameras**: by tenet
  ([TENETS.md](../../TENETS.md)). Camera-conditioned variants are
  ruled out at the project level.
- **LiDAR-only NeRF / occupancy** (LiDAR-NeRF, Behley et al., Wu et
  al. occupancy networks) is research-grade. Training is per-scene
  and costly; inference is implicit and slow on CPU; the output is
  difficult to audit cell-by-cell.

### 6.3 Comparison with the existing maths proposals

| Existing method                                                                                                   | What it does                                                        | What a radiance field improves                                            | What a radiance field makes worse                                                                          |
| ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| EMA background settling ([background-grid-settling-maths.md](../../data/maths/background-grid-settling-maths.md)) | Per-cell exponential moving mean + Welford variance in polar coords | Nothing measurable. EMA is already accurate, cheap, replayable.           | Replaces a 4-line update with an MLP forward pass; loses the per-cell numerical audit trail.               |
| Foreground gating ([foreground-tracking.md](../lidar/architecture/foreground-tracking.md))                        | Statistical thresholding on per-cell distribution                   | Marginally improves edge cases at high variance cells                     | Introduces a learned threshold; harder to defend in a civic report.                                        |
| DBSCAN ([clustering-maths.md](../../data/maths/clustering-maths.md))                                              | Density-based spatial clustering in world frame                     | Possibly better cluster boundaries in cluttered scenes                    | Loses the rank-1 property that any cluster can be traced back to specific points and parameters.           |
| OBB / PCA geometry                                                                                                | Eigendecomposition of cluster covariance                            | Nothing. PCA is the right tool.                                           | N/A                                                                                                        |
| Kalman tracking + Hungarian ([tracking-maths.md](../../data/maths/tracking-maths.md))                             | Linear-Gaussian state estimation + optimal assignment               | Scene-conditioned occlusion gap filling (legitimate but small)            | Adds a learned motion model that depends on a learned scene; doubles the uncertainty surface.              |
| Heuristic classification ([classification-maths.md](../../data/maths/classification-maths.md))                    | Rule-based thresholds on feature vector                             | Out of scope per the ML classifier plan                                   | Out of scope per the ML classifier plan                                                                    |
| L7 Bayesian evidence grid (log-odds) ([lidar-l7-scene-plan.md §3.1](lidar-l7-scene-plan.md))                      | OctoMap-style per-feature log-odds confidence                       | **None.** Log-odds is the right tool for accumulating uncertain evidence. | Replaces a closed-form update with a learned representation. Loses the "we are doing Bayes" defensibility. |
| L7 canonical object refinement (Welford) ([§3.2](lidar-l7-scene-plan.md))                                         | Online sufficient statistics                                        | None. Welford is provably optimal for the moments we care about.          | Replaces a 4-line update with a trained model.                                                             |
| L7 cross-sensor association (Mahalanobis) ([§3.3](lidar-l7-scene-plan.md))                                        | $\chi^2$-gated nearest neighbour                                    | None. The classical gate is correct under the assumed measurement model.  | Adds a learned association model whose failure modes are opaque.                                           |

**Simpler classical alternatives that should be exhausted first** if
the planned L7 ever needs more than log-odds + Welford:

- **TSDF (truncated signed distance fields)**, Newcombe et al. 2011.
  Explicit, auditable, deterministic, well-studied for
  LiDAR. Already noted in
  [lidar-background-grid-standards.md](../lidar/architecture/lidar-background-grid-standards.md)
  as a candidate "deferred until we pursue multi-sensor fusion".
- **OctoMap voxels** for full 3D occupancy if the vector-feature
  formulation proves limiting. Same log-odds maths the L7 plan
  already commits to.
- **Surfel maps** (Behley & Stachniss, 2018). Explicit, fast on CPU,
  proven on automotive LiDAR.

Any of these would supersede a radiance-field approach on
explainability while matching or beating it on dynamic-scene
robustness.

### 6.4 Hardware sensitivity — what an edge accelerator would and wouldn't change

A natural follow-up question is: "would this decision change if the Pi
had a Coral USB TPU, a Hailo NPU, or were swapped for a Jetson Orin?"
The short answer is no. Compute is not the binding constraint. Privacy,
auditability, and civic-report defensibility do not improve with more
TOPS. We document this explicitly because the question will recur as
edge accelerators become cheaper.

| Accelerator                                 | What it enables                                                                                                                   | What it does not change                                                                                                                         |
| ------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| **None (current target: Pi 4)**             | Nothing new. Classical L3–L7 stack runs comfortably at 10 Hz on the existing hardware.                                            | Baseline. Sets the bar every learned alternative must beat on _privacy + audit + cost_, not only accuracy.                                      |
| **Coral USB TPU (Edge TPU, INT8, ~4 TOPS)** | Small fixed-point CNN inference. Plausibly enables a quantised CNN classifier at L6 _within_ the ML classifier plan's guardrails. | NeRF, splatting, and neural SDF do not lower well to INT8 and the op coverage is narrow. Does not unlock radiance fields in any useful form.    |
| **Hailo-8 or similar NPU (~26 TOPS)**       | Larger CNN inference budget. Some Instant-NGP-style hash-grid lookups become tractable; small implicit fields conceivable.        | Privacy story unchanged. Audit story unchanged. Determinism still hard (quantisation noise, vendor runtime opacity).                            |
| **Jetson Orin Nano (CUDA, ~40 TOPS)**       | LiDAR-only NeRF or neural occupancy inference at low resolution becomes feasible per site; training still offline.                | Still opaque, still per-site retraining, still no civic-report defensibility. BOM cost, power draw, and thermal envelope all change materially. |
| **Server-class GPU (offline workstation)**  | Full radiance-field capability for offline reconstruction. The right tool for architecture A in §5.                               | Does not change the production pipeline. Does not change civic-report defensibility. Does not change the privacy story.                         |

Two conclusions follow.

**First**, the production decision is robust against future hardware
upgrades. None of the binding constraints — privacy, audit-grade
replay, civic-report defensibility, "no opaque models in the live
chain" — is unlocked by adding compute to the edge device. If a Pi
gains a Coral TPU tomorrow, the right thing to spend that budget on
is the interpretable L6 classifier already permitted by the ML
classifier plan, not a learned scene field.

**Second**, the offline research lane (architecture A) is precisely
where a server-class GPU pays off. That lane already assumes a
desktop machine. A future contributor with a GPU can produce richer
visualisation overlays without changing anything in the deployed
binary. This is the only place compute meaningfully changes what is
possible, and it changes possibility in the _visualisation_ dimension
only — never in the _measurement_ dimension.

A third, narrower observation: if the Pi ever gains an accelerator,
the natural first occupant is the L6 CNN classifier work already
scoped in
[lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md),
not a learned scene field. That work has clear guardrails, a defined
promotion gate, and a baseline to beat. A radiance-field experiment
on the same accelerator would have none of those things.

## 7. Why this might be the wrong direction

A direct enumeration of the risks of pursuing radiance fields in
this project.

1. **Black-box risk.** Even a "white-box" NeRF (small MLP, public
   weights) is opaque relative to log-odds on a vector feature. A
   public report saying "the radiance field decided this region is
   road" is materially harder to defend than "the Bayesian evidence
   grid accumulated N consistent observations of a flat surface,
   here is its log-odds value and Welford-refined plane equation".
   See §4.1 for the explicit, clause-by-clause conflict with the ML
   classifier plan's guardrails.

2. **Public-report difficulty.** Civic reports cite measured speeds
   and counts. The chain of custody runs sensor → packet → frame →
   foreground → cluster → track → measurement. A learned scene
   representation in the middle of that chain makes the report
   defensibly _opinionated_ rather than _measured_. The project's
   "evidence over opinion" tenet
   ([TENETS.md](../../TENETS.md)) is incompatible with that drift.

3. **GPU/runtime cost.** The deployment target is a Raspberry Pi 4
   with no GPU. NeRF inference, even with Instant-NGP optimisations,
   is GPU-bound. Gaussian splatting demands a CUDA-class accelerator
   at any reasonable resolution. Neural occupancy at meaningful
   resolution is CPU-feasible but slow and trades determinism for
   accuracy. See §6.4 for what edge accelerators (Coral, Hailo,
   Jetson) would and would not change — the short answer is that
   they unlock CNN work permitted by the ML classifier plan, not
   radiance fields.

4. **Training data requirements.** Per-scene retraining is the
   normal NeRF workflow. velocity.report deployments are sited
   per-intersection with weeks-long capture campaigns; per-site
   retraining is operationally hostile.

5. **Brittleness across sensor mounting positions.** Per-site
   geometry varies. A learned scene representation trained at one
   site does not transfer; even moving the sensor a metre changes
   the visible scene. The classical L7 plan handles this naturally
   via per-sensor calibration and Procrustes alignment to priors.

6. **Poor fit for sparse 40-ring LiDAR.** NeRF and splatting were
   developed for dense camera or dense-LiDAR (HDL-64 / 128-beam)
   coverage. The 40-ring sensor's per-frame coverage is below the
   density these methods assume.

7. **Distraction risk.** Pursuing radiance fields would delay the
   already-planned L7 explicit-geometry work. The L7 plan is
   well-scoped, low-risk, and has known maths. A radiance-field
   detour would consume engineering capacity that should land
   classical L7 first.

8. **Privacy-story risk.** The civic argument for velocity.report is
   "measure velocity, not identity." Any flirtation with radiance
   fields invites the public-perception association with
   camera-based scene reconstruction even if the project uses only
   LiDAR. The simplest way to keep the privacy story credible is to
   not enter the territory at all.

## 8. How the planned classical path supersedes radiance fields

For each scene-level capability a radiance field would offer, the
already-planned classical L7 + vector-scene-map provides an
equivalent or stronger answer with better auditability and lower
compute.

| Capability                            | Radiance-field offer                  | Classical answer in this repo                                                                                                                                                                                                                          |
| ------------------------------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Persistent scene representation       | Learned density / SDF over the volume | Planned L7 vector scene map (polygons + planes + volumes) at LOD 0–3, ~35 KB compressed per 100 m × 100 m. Storage target `vector_scene_features` (when implemented; see [vector-scene-map.md §5.3](../lidar/architecture/vector-scene-map.md)).       |
| Confidence accumulation across frames | Implicit, via training loss           | Log-odds Bayesian update per scene feature, closed-form, per-frame ([lidar-l7-scene-plan.md §3.1](lidar-l7-scene-plan.md)).                                                                                                                            |
| Multi-frame fusion                    | Aggregated training set               | Welford running statistics on canonical object dimensions ([§3.2](lidar-l7-scene-plan.md)); incremental scene-feature evidence.                                                                                                                        |
| Multi-sensor fusion                   | Joint optimisation across sensors     | Per-sensor L1–L6 pipelines + L7 Mahalanobis-gated cross-sensor association ([§3.3](lidar-l7-scene-plan.md)).                                                                                                                                           |
| Occlusion reasoning                   | Learned visibility prediction         | Scene graph relations (`occluded_by`) on explicit structure features; per-track coasting in L5; structure footprints from vector scene map.                                                                                                            |
| Hole filling / completion             | Implicit field interpolation          | LOD fallback in the vector scene map (parent polygon answers when no child polygon covers); OSM S3DB structure priors aligned by Procrustes.                                                                                                           |
| Static background prior               | Learned scene representation          | Polar EMA background ([background-grid-settling-maths.md](../../data/maths/background-grid-settling-maths.md)) + vector scene map at L7.                                                                                                               |
| Cross-session scene memory            | Cached weights                        | Planned versioned `vector_scene_snapshots` keyed by snapshot hash (SHA256); deterministic restore. Today, `lidar_bg_regions.grid_hash` (renamed from `scene_hash` in migration 000030) provides the analogous restore path for the L3 background grid. |
| Visual reconstruction for QA          | NeRF render or Gaussian splat         | Allowed only as **offline research** per architecture A in §5; not in production.                                                                                                                                                                      |

The classical path beats the learned path on every column except
"visual reconstruction for QA", where we accept the use case as an
offline-only research lane and contain it strictly.

## 9. Recommendation

**Do not adopt radiance-field methods in the production pipeline.**
The already-planned L7 Scene + vector-scene-map work supersedes the
relevant capabilities while preserving every hard property the
project has committed to.

**Permit a narrow offline research lane (architecture A from §5)** for
visualisation and reconstruction QA of recorded sessions, with strict
guardrails (§10).

**Do not block** the planned L7 graduation on this decision. L7 should
proceed on its existing Bayesian-evidence-grid plan
([lidar-l7-scene-plan.md](lidar-l7-scene-plan.md)) without
incorporating any learned scene representation.

**Revisit** this decision only if all three of these become true:

1. Multi-sensor deployment is funded, sensors overlap with enough
   geometry to give NeRF-like methods a supervisory signal, _and_
   the classical L7 path has been built and measured.
2. A LiDAR-only, CPU-feasible, deterministic-replay-compatible
   variant has matured (e.g. an explicit occupancy network with a
   Bayesian wrapper).
3. The privacy story can be defended in writing for the new method
   without weakening the "measure velocity, not identity" claim.

## 10. Proposed offline research lane (architecture A)

If anyone wants to run a radiance-field experiment, it must look
like this. Otherwise it does not get integrated.

**Smallest useful experiment.** A single offline tool, e.g.
`tools/scene-recon/`, that reads one VRLOG file, runs a chosen
reconstruction method on the foreground-removed background points,
and emits an overlay asset usable in the macOS visualiser.

**Inputs (required):** VRLOG path, background snapshot range
(start/end frame), output path. Optional: model variant, parameter
set.

**Inputs (forbidden):** Camera frames. Anything sourced from outside
the recorded LiDAR.

**Success criteria.** The tool produces a reconstruction artefact
that, when overlaid on the visualiser, makes scene context easier to
interpret during manual QA. Concretely: at least one
already-difficult-to-debug recorded case becomes easier to debug.

**Failure criteria (any one kills the lane).**

- The reconstruction is used as input to L4, L5, L6, L7, or L8.
- Any output appears in a civic PDF report.
- Any output is described as a measurement rather than a visualisation.
- The tool requires a hardware accelerator the project cannot ship on
  the Raspberry Pi target.
- The tool ingests RGB or any other non-LiDAR sensor data.
- The tool's output is not deterministic given fixed inputs and
  fixed parameters.

**Dataset.** Existing recorded VRLOGs from a single sensor over a
representative window (e.g. one capture day). No synthetic
augmentation that hides classical limitations.

**Expected runtime cost.** Offline, single-machine, GPU-optional.
Training and inference both excluded from the radar binary and from
CI.

**Comparison with existing L3/L4/L5 outputs.** A "control" overlay
showing the classical foreground / cluster / track output side-by-side
with the learned reconstruction. The classical view is always the
labelled baseline.

**Audit artefacts that must be stored.**

- Model version (commit SHA + checkpoint hash).
- Parameters (full config).
- Input frame range.
- Output overlay (file + content hash).
- Reconstruction confidence statistics.
- Fallback behaviour: what was rendered when the model produced no
  output.

**Visualiser exposure.** The reconstruction is an _overlay_. The
point cloud, foreground, clusters, and tracks must remain visible
underneath. No view setting hides the classical pipeline outputs.

**What must never be hidden behind the model.**

- The raw LiDAR point cloud.
- The L3 foreground extraction.
- The L4 clusters and OBBs.
- The L5 track IDs and Kalman states.
- The L8 traffic measurements.

### 10.1 Concrete offline use cases this would unlock

The offline research lane is justified by specific deployment- and
debugging-time questions that the classical pipeline answers poorly
or not at all. Each use case below has identifiable inputs, outputs,
and a clear boundary against the measurement pipeline.

| Use case                                                     | Inputs                                                | Outputs                                                             | What it enables                                                                                                                       | What it must not become                                                       |
| ------------------------------------------------------------ | ----------------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| **Sensor-placement QA**                                      | VRLOG from a candidate mount position; site survey    | 3D scene reconstruction of the visible field of view                | Validates that a proposed mount captures the intended traffic corridor before committing to permanent installation.                   | A reason to skip the empirical L3/L4 acceptance tests at the new site.        |
| **Sparse-return background completion (visualisation)**      | Foreground-removed point cloud accumulated over hours | Surface or splat overlay rendering the implied static scene         | Lets a human inspector see the persistent scene that the polar EMA grid encodes implicitly; useful for sanity-checking grid settling. | Input to L3 foreground gating. The polar EMA stays canonical.                 |
| **Synthetic viewpoint generation for multi-sensor planning** | VRLOG from sensor A                                   | Rendered "what would a sensor at position B see" preview            | Lets a planner reason about coverage overlap and occlusion before deploying a second sensor.                                          | A substitute for actual measurement at position B.                            |
| **Sensor pose drift detection**                              | VRLOGs from the same sensor across multiple days      | Reconstruction diff (today vs. last week)                           | Flags sensors that have physically shifted (wind, knock, mount drift). Complements the planned `scene_hash` settling-state restore.   | A trigger for automatic recalibration. Drift detection notifies; humans act.  |
| **Cluttered-scene context rendering for L4 debug**           | VRLOG window around a confusing L4 cluster            | Rendered local 3D context around the cluster                        | Gives an engineer a visual reference for why DBSCAN cohered or fragmented the cluster.                                                | An override of the classical cluster decision.                                |
| **Calibration validation (ground tile vs. surface)**         | Settled L4 ground tiles + reconstructed surface       | Overlay showing per-tile residual against the reconstructed surface | Checks tile-fit quality across the scene; spots tiles that have settled to a locally wrong plane.                                     | An automatic tile-replacement mechanism.                                      |
| **Pre-rendered civic-report illustration assets**            | VRLOG + reconstruction model                          | Static 3D illustration of the deployment site (decorative)          | Helps non-technical readers picture the sensor's coverage in a community-facing report.                                               | A source of measurement claims in the same report. Captioned as illustration. |
| **Training-data augmentation for the L6 classifier**         | VRLOG of tracked objects                              | Synthetic per-track viewpoint variations                            | Feeds the ML classifier plan's existing training pipeline with more views per object, under that plan's guardrails.                   | A substitute for real labelled data. Must respect the classifier plan's gate. |

Each row above terminates in a clear "what it must not become." That
column is the boundary between the offline research lane and the
measurement pipeline. Crossing it converts the work from
visualisation into a production-ML proposal that has to clear the
full classifier-plan promotion gate (§4.1) on its own merits.

The lane is intentionally narrower than the union of every plausible
use case for radiance fields elsewhere in industry. Three deliberate
omissions:

- **No real-time on-device reconstruction.** Even on a future
  GPU-equipped Pi, real-time reconstruction would compete with the
  measurement pipeline for the same compute and would tempt anyone
  to surface the result in user-facing views. See §6.4.
- **No camera-conditioned variants.** Even for offline use. See §11.
- **No "advisory feedback" loop into the classical pipeline.** Any
  use case where the learned output influences a classical decision
  (e.g., "the reconstruction says this is road, so DBSCAN should
  cluster more aggressively here") becomes architecture C, not
  architecture A, and is rejected.

## 11. Do not do this

A short list of tempting but bad integrations.

- **Replace the L3 EMA background with a learned density.** Loses
  the per-cell variance audit trail, breaks deterministic settling,
  fails the civic-report defensibility test.
- **Train a NeRF on RGB from a borrowed camera "just for
  evaluation."** Undermines the privacy story. The moment a camera
  is in the pipeline, even offline, the public-facing claim "no
  cameras" requires footnotes.
- **Use a learned occupancy field to gap-fill L5 tracks through
  occlusions.** The current Kalman coasting + scene-graph
  `occluded_by` relations in the L7 plan already handle this with
  explainable parameters.
- **Let the macOS visualiser default to showing only the learned
  reconstruction.** Hides the measurement. Always show classical
  output underneath.
- **Cite a learned-model output in the PDF report.** The report
  cites measurements. ML-derived overlays are visualisation, not
  measurement.
- **Add a `--use-radiance-field` flag to the radar binary.**
  Anything that ships on the RPi must remain auditable; the offline
  research lane lives in `tools/`, never in `internal/cmd/server`.
- **Treat radiance fields as a substitute for the planned vector
  scene map.** They aren't. The vector scene map is the canonical
  L7 storage substrate per
  [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md).

## 12. Open questions

These are not blockers for this decision, but should be tracked.

1. If LiDAR-only NeRF or neural occupancy matures to deterministic
   replay and CPU inference, when do we revisit?
2. Does multi-sensor deployment change the picture enough to justify
   re-evaluation? (Probably not until _after_ classical L7 is
   shipped and measured.)
3. Should the offline research lane be permitted to use desktop
   GPUs, or restricted to CPU-only to keep the gap to production
   visible?
4. If a learned scene-conditioned motion model becomes valuable for
   L7's "scene-constrained physics" work
   ([lidar-l7-scene-plan.md §4](lidar-l7-scene-plan.md)), how
   does that interact with this decision? (Suggest: that's an L7
   motion-model question, not a scene-representation question;
   evaluate separately.)

## 13. References

### Internal documents

- [LIDAR_ARCHITECTURE.md](../lidar/architecture/LIDAR_ARCHITECTURE.md) — canonical L1–L10 model
- [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md) — L7 Bayesian evidence grid plan (supersedes this decision)
- [vector-scene-map.md](../lidar/architecture/vector-scene-map.md) — explicit polygon scene representation
- [lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md) — ML classifier guardrails
- [foreground-tracking.md](../lidar/architecture/foreground-tracking.md) — polar/world coordinate boundary
- [ground-plane-extraction.md](../lidar/architecture/ground-plane-extraction.md) — tiled ground model
- [lidar-background-grid-standards.md](../lidar/architecture/lidar-background-grid-standards.md) — TSDF / OctoMap evaluation
- [background-grid-settling-maths.md](../../data/maths/background-grid-settling-maths.md) — EMA settling maths
- [clustering-maths.md](../../data/maths/clustering-maths.md) — DBSCAN maths
- [tracking-maths.md](../../data/maths/tracking-maths.md) — Kalman + Hungarian maths
- [classification-maths.md](../../data/maths/classification-maths.md) — rule-based classifier maths
- [TENETS.md](../../TENETS.md) — project tenets (privacy, evidence, local-first)

### External references

- Mildenhall et al. (2020), _NeRF: Representing Scenes as Neural
  Radiance Fields for View Synthesis_, ECCV.
- Kerbl et al. (2023), _3D Gaussian Splatting for Real-Time Radiance
  Field Rendering_, SIGGRAPH.
- Hornung et al. (2013), _OctoMap: An efficient probabilistic 3D
  mapping framework based on octrees_, Autonomous Robots. (Already
  cited by the L7 plan.)
- Behley & Stachniss (2018), _Efficient Surfel-Based SLAM using 3D
  Laser Range Data in Urban Environments_, RSS.
- Newcombe et al. (2011), _KinectFusion: Real-Time Dense Surface
  Mapping and Tracking_, ISMAR. (TSDF.)
- Tao et al. (2023), _LiDAR-NeRF: Novel LiDAR View Synthesis via
  Neural Radiance Fields_, arXiv:2304.10406.
- Pumarola et al. (2021), _D-NeRF: Neural Radiance Fields for
  Dynamic Scenes_, CVPR.
