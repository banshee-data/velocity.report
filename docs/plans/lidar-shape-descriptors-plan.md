# LiDAR shape descriptors and cluster point retention

- **Status:** Draft
- **Layers:** L4 Perception, L6 Objects, L9 Endpoints, storage
- **Target:** v0.5.2-v0.5.4; point retention and the descriptor set are prerequisites for the classification scorecard's feature work and for any candidate model above the current bbox cascade.
- **Companion plans:** [lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md), [lidar-maths-coherence-plan.md](lidar-maths-coherence-plan.md), [lidar-av-lidar-integration-plan.md](lidar-av-lidar-integration-plan.md), [lidar-test-corpus-plan.md](lidar-test-corpus-plan.md)
- **Canonical:** [data/maths/clustering-maths.md](../../data/maths/clustering-maths.md) (single source of truth)
- **Related:** [Classification maths](../../data/maths/classification-maths.md), [Paper-vs-implementation gap analysis](../../data/maths/paper-implementation-gap-analysis.md)

## Motivation

The classifier decides between eight classes using a bounding box and a speed. It
never sees the shape of the object it is classifying, because the points that
describe that shape are discarded before the cluster leaves L4.

That limit is already visible in the runtime: truck and motorcyclist are commented
out of the cascade in
[internal/lidar/l6objects/classification.go](../../internal/lidar/l6objects/classification.go),
and both are classes a bounding box genuinely cannot separate. A truck and a long
car differ in where their mass sits, not in their extents. A motorcyclist and a
cyclist differ in base density. Those are structural distinctions, and the current
feature set cannot express them at any threshold.

The fix is not a learned representation over raw points. It is a set of named
geometric descriptors — eigenvalue ratios, vertical mass distribution, ground
footprint, return character — each a scalar with a physical reading, computed in
L4 and consumed as ordinary features. Descriptors preserve the inspectable,
tuneable, explainable property the pipeline requires, and they are the established
alternative to opaque point-cloud networks (Weinmann 2015; West 2004; Osada 2002).

Nothing can be built until points survive clustering, which is why point retention
leads this plan.

## Current state

| Fact                                                                                                                            | Evidence                                                             |
| ------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Cluster member points are discarded inside `computeClusterMetrics`, one call after DBSCAN produces them                         | `l4perception/cluster.go`, `buildClusters` → `computeClusterMetrics` |
| `WorldCluster.SamplePoints` is declared and never assigned outside tests                                                        | `l4perception/types.go`                                              |
| proto `Cluster.sample_points` is declared and never assigned                                                                    | `proto/velocity_visualiser/v1/visualiser.proto`                      |
| `ExtractClusterFeatures(cluster, points)` is never called in production; the live path uses `ExtractTrackFeatures`              | `l6objects/features.go`, `pipeline/tracking_pipeline.go`             |
| `IntensityStd` and `VerticalSpread` are therefore always zero in shipped output                                                 | `l6objects/features.go`                                              |
| `ExportTrackPointCloud` returns an empty slice: _"PolarPoints will be populated when point cloud storage is integrated"_        | `adapters/track_export.go`                                           |
| `SummarizeTrainingDataset.TotalPoints` is never populated: _"TODO: Add point count when point cloud storage is integrated"_     | `l6objects/quality.go`                                               |
| No table in any migration holds point data; the only BLOBs are the background grid and a site map SVG                           | `internal/db/migrations/`                                            |
| PCA is 2D over X-Y only; the smaller eigenvalue is a commented-out line and neither eigenvalue is retained                      | `l4perception/obb.go`                                                |
| No linearity, planarity, sphericity, omnivariance, anisotropy, eigenentropy or curvature computation exists anywhere            | Repository-wide search                                               |
| VRLOG can carry points but is foreground-only under split streaming, records no cluster id per point, and is off for sweep runs | `l9endpoints/adapter.go`, `sweep/runner.go`                          |

## Findings

| Area                      | Current state                                                                         | Severity | Release view      |
| ------------------------- | ------------------------------------------------------------------------------------- | -------- | ----------------- |
| Point retention           | Points destroyed at the earliest possible stage; three declared fields never assigned | High     | Phase 1, v0.5.2   |
| Dead point-dependent code | Two feature fields permanently zero; two exported helpers unreachable                 | High     | Phase 1, v0.5.2   |
| 3D shape description      | Absent entirely; only a 2D heading survives from the covariance                       | High     | Phase 2, v0.5.3   |
| Retroactive recovery      | VRLOG is the only source and needs offline re-clustering to attribute points          | Medium   | Accepted residual |
| Range envelope            | Undefined; DBSCAN `MinPts` is 5 and returns fall below that beyond ~40 m              | High     | Phase 4, v0.5.3   |
| Hot-path cost             | Retention adds allocation at 10 Hz on constrained hardware                            | Medium   | Phase 1 gate      |

## Design / approach

### Retain a bounded sample, not the full cluster

Full retention is unnecessary and unaffordable on the hot path. Descriptors are
statistics: they converge well before a cluster's full point count. Retain a capped,
deterministic subsample per cluster, with the cap exposed as a tuning key under the
active L4 engine block so it can be raised for offline analysis and lowered for
constrained deployment.

Determinism matters more than sample size: replay must reproduce descriptors
exactly. The existing `uniformSubsample` in `l4perception/cluster.go` **cannot be
reused** for this. It seeds from `time.Now().UnixNano()` mixed with a monotonic
counter, deliberately, so that "consecutive calls within the same nanosecond still
produce distinct subsamples". Retention therefore needs a content-derived seed —
frame id, cluster id and sensor id hashed together — so the same input reproduces
the same sample on every replay.

That choice exposes a pre-existing problem rather than creating one: the same
non-deterministic subsample already runs ahead of DBSCAN whenever a frame exceeds
`foreground_max_input_points` (default 8000), which makes clustering itself
irreproducible on busy frames. Recorded as a separate item below; this plan must
not inherit the behaviour.

### Descriptors in SQLite, points in VRLOG

Two tiers, chosen by what each consumer needs:

| Tier   | Holds                                     | Rationale                                                                         |
| ------ | ----------------------------------------- | --------------------------------------------------------------------------------- |
| SQLite | Descriptor scalars, per cluster and track | Small, queryable, and exactly what the classifier and the corpus export consume   |
| VRLOG  | Sampled points, cluster-tagged            | Large; needed only to recompute or audit descriptors, which is an offline concern |

Following the repository's JSON-first convention, descriptors persist as a
`shape_features_json` column with generated columns for the fields worth indexing,
rather than twenty scalar columns.

### Preserve the existing heading computation

The 2D X-Y covariance in `l4perception/obb.go` produces the OBB heading, and that
heading feeds tracker guards and rendering. Changing it to a 3D decomposition would
shift track output and break replay determinism. The 3D covariance is therefore
**additive and separate**: a new computation for descriptors, leaving the heading
path untouched. `TestGoldenReplay_Determinism` is the gate on that claim.

### Descriptor families

Four families. Each descriptor is a named scalar with a physical reading, recorded
with its formula and source in `data/maths/clustering-maths.md`.

**Eigenvalue features.** From the 3×3 covariance of the retained points, with
eigenvalues λ₁≥λ₂≥λ₃ normalised so that e_i = λ_i/Σλ and Σe = 1:

| Descriptor   | Definition       | Reads as                       |
| ------------ | ---------------- | ------------------------------ |
| Linearity    | (e₁−e₂)/e₁       | pole-like, thin, upright       |
| Planarity    | (e₂−e₃)/e₁       | sheet-like, vehicle side panel |
| Sphericity   | e₃/e₁            | blob-like, vegetation, torso   |
| Anisotropy   | (e₁−e₃)/e₁       | directional versus isotropic   |
| Omnivariance | (e₁·e₂·e₃)^(1/3) | volumetric spread              |
| Eigenentropy | −Σ e_i·ln(e_i)   | structural disorder            |

Change-of-curvature, λ₃/(λ₁+λ₂+λ₃), reduces to e₃ under this normalisation and is
therefore not stored separately — six independent values, not seven. Record that
reduction in the maths note so it is not rediscovered as a bug.

**Vertical profile.** Point fraction per horizontal band, measured against the
height-band floor already used by `l4perception/ground.go` rather than a new ground
reference. Band count is a tuning key. This is the family that expresses mass
distribution, and the one a bounding box cannot approximate.

**Footprint.** Convex hull area and perimeter of the X-Y projection, fill ratio
(points per hull area), convexity (hull area over OBB footprint area), and hull
elongation.

**Return character.** Intensity mean and standard deviation — populating the two
fields that are currently always zero — high-intensity fraction, and observed point
density against range-expected density. The last is an occlusion and hollowness
signal, since return density falls roughly as the inverse square of range.

### Degenerate inputs

Clusters below four points cannot support a 3D covariance, and near-collinear or
near-coplanar clusters produce eigenvalues at or below floating-point noise. Every
descriptor carries an explicit validity flag rather than a silently wrong number,
following the `obbCovarianceEpsilon` precedent already in `obb.go`. Consumers treat
an invalid descriptor set as absent, not as zero.

### Range stratification is a design constraint, not a caveat

DBSCAN `MinPts` is 5, the sparse-track floor is 3 points, and `data/QUESTIONS.md`
Q5 records fewer than five returns per object beyond 40 m. Eigenvalues over eight
points are noise.

Three consequences, all binding:

1. Every descriptor set is stored with its point count and range.
2. Any scorecard over descriptors is reported **stratified by range band**, never
   pooled — a pooled figure hides the fact that the descriptors stopped working.
3. An explicit insufficient-points path falls back to the existing
   bbox-plus-kinematics rules. That fallback is part of the design.

## Scope

### Phase 1: cluster point retention

**Summary:** Make points survive L4 under a bounded, deterministic cap.

**Steps:**

1. Add a `max_sample_points` tuning key to the active L4 engine block; wire it
   through `l4perception` config loading alongside the existing DBSCAN parameters.
2. Populate `WorldCluster.SamplePoints` in `computeClusterMetrics`, applying the cap
   using a content-derived seed (frame, cluster and sensor id), **not** the
   time-seeded `uniformSubsample` already in the file.
3. Populate `Cluster.SamplePoints` in `l9endpoints/adapter.go` (`adaptClusters` and
   `adaptUnassociatedClusters`) so VRLOG recordings carry cluster-tagged points.
4. Call `ExtractClusterFeatures` from the live path so `IntensityStd` and
   `VerticalSpread` carry real values.
5. Benchmark against `internal/lidar/perf/baseline/baseline-kirk0-full.json`. A cap that
   drops sustained frame rate below 10 Hz on target hardware is rejected; if the
   hot-path cost cannot be met, retention becomes analysis-and-replay-only, where
   throughput is not real-time bound.

**Milestone:** v0.5.2

### Phase 2: descriptor computation

**Summary:** Compute the four families in L4 as an additive stage.

**Steps:**

1. Add `ShapeFeatures` and its computation in a new `l4perception/shape.go`, taking
   the retained points and returning the descriptor set with validity flags.
2. Use the analytic symmetric 3×3 eigenvalue solution rather than adding a linear
   algebra dependency; it is stable for the positive semi-definite covariance case
   and keeps the computation auditable.
3. Leave `obb.go`'s 2D heading path untouched.
4. Document every descriptor in `data/maths/clustering-maths.md` with formula,
   source, physical reading, and the e₃ reduction noted above. Register band counts,
   caps and epsilons in `MAGIC_NUMBERS.md`.

**Milestone:** v0.5.3

### Phase 3: persistence and export

**Summary:** Make descriptors durable and queryable.

**Steps:**

1. Migration adding `shape_features_json` to the cluster and run-track tables, with
   generated columns for the fields worth indexing.
2. Aggregate per-track descriptors across observations — median and variance per
   descriptor. **Descriptor variance across a track is itself a feature**: a rigid
   object holds its shape between sweeps, vegetation re-scatters, and that
   distinction is the natural noise discriminator.
3. Extend the corpus export in
   [lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md)
   Phase 2 to carry descriptors alongside the existing feature set.

**Milestone:** v0.5.3

### Phase 4: range envelope characterisation

**Summary:** Establish empirically where descriptors stop being meaningful.

**Steps:**

1. Measure descriptor stability against point count and range over the captured
   corpus: for each descriptor, the variance across observations of the same track
   as a function of range band.
2. Publish the resulting usable envelope per class in the maths note, and set the
   insufficient-points threshold from that measurement rather than from assumption.
3. Feed the envelope into the scorecard's range stratification.

**Milestone:** v0.5.4

## Dependencies

- Phase 1 gates everything else in this plan and the descriptor portion of the
  classifier plan's candidate ladder.
- Phase 4 breadth is limited by [lidar-test-corpus-plan.md](lidar-test-corpus-plan.md):
  one of five scenes captured. A single-site envelope is provisional.
- The classification scorecard in
  [lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md)
  Phase 1 is independent of this plan and can proceed in parallel; it needs labels
  and predictions, not points.

## Risks

| Risk                                                     | Likelihood | Impact | Mitigation                                                                              |
| -------------------------------------------------------- | ---------- | ------ | --------------------------------------------------------------------------------------- |
| Point retention regresses hot-path throughput            | Medium     | High   | Configurable cap plus a benchmark gate; fall back to analysis-mode-only retention       |
| Descriptor change shifts OBB heading and breaks replay   | Low        | High   | 3D covariance is additive; heading path untouched; golden replay test is the gate       |
| Descriptors computed on too few points read as confident | High       | High   | Validity flags, explicit fallback path, mandatory range stratification                  |
| Storage growth from per-observation descriptor rows      | Medium     | Medium | Store aggregates on tracks; per-observation JSON only in analysis runs                  |
| Descriptor set grows without evidence any of it helps    | Medium     | Medium | Phase 4 measures per-descriptor separability; drop descriptors that do not earn a place |

## Checklist

### Outstanding

- [ ] Phase 1: `max_sample_points` tuning key and config wiring (`S`)
- [ ] Phase 1: populate `WorldCluster.SamplePoints` with a capped, content-seeded subsample (`M`)
- [ ] Phase 1: content-derived seed helper, replacing time-seeded selection on the retention path (`S`)
- [ ] Phase 1: populate proto `Cluster.sample_points` in the adapter (`S`)
- [ ] Phase 1: call `ExtractClusterFeatures` on the live path (`S`)
- [ ] Phase 1: throughput benchmark against the kirk0 baseline (`S`)
- [ ] Phase 2: `ShapeFeatures` and analytic 3×3 eigen solution in `l4perception/shape.go` (`M`)
- [ ] Phase 2: vertical profile, footprint and return-character families (`M`)
- [ ] Phase 2: descriptor documentation in `clustering-maths.md`; constants in `MAGIC_NUMBERS.md` (`S`)
- [ ] Phase 3: `shape_features_json` migration with generated columns (`M`)
- [ ] Phase 3: per-track descriptor aggregation including cross-observation variance (`M`)
- [ ] Phase 3: corpus export carries descriptors (`S`)
- [ ] Phase 4: descriptor stability versus range measurement and published envelope (`M`)

### Deferred

- [ ] Superquadric fitting (three scale plus two shape exponents) as an explicit
      primitive representation; fully interpretable but per-cluster nonlinear
      optimisation, and premature until the descriptor set is shown insufficient
- [ ] Dual-return and elongation features, blocked on the sensor limitation recorded
      in [av-range-image-format-alignment.md](../lidar/architecture/av-range-image-format-alignment.md)

### Accepted residuals (no action planned)

- [ ] Historical runs cannot be back-filled with descriptors; VRLOG recordings
      predating cluster-tagged points require offline re-clustering to attribute
      points, and sweep runs recorded nothing at all
- [ ] Descriptors remain unavailable beyond the range envelope; the bbox cascade
      is the permanent fallback there rather than a degraded shape estimate
