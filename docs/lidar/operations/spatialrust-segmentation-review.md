# SpatialRust segmentation and clustering review

- **Status:** Source review complete; experiments and implementation slices proposed
- **Scope:** SpatialRust segmentation, supporting search and GPU paths, and applicability to
  velocity.report L3/L4. This is not an audit of the whole SpatialRust workspace.
- **Upstream revision:** `6f563321ad7b9801f49b45f52f4392141411f54c`
- **Local baseline:** `a71d5fc4df1cc4141f6504f4766dea990f3cd5ba`
- **Related:** [L4 experiment area](../../../data/experiments/l4clus/README.md),
  [clustering plan](../../plans/lidar-clustering-observability-and-benchmark-plan.md),
  [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md)

SpatialRust is useful as a catalogue of working geometric algorithms and as an example of
explicit execution accounting. Its strongest contribution here is a way to compare alternative
connectivity rules with independent correctness references. It does not establish that a Rust
port, GPU backend, or three-dimensional clustering would improve roadside tracking on a Pi.

The first priority is to act on the now-answered l4clus0003 density experiment: keep the core
threshold at 8 in Stage B and investigate joining fragments. Establish a common membership
comparison, then test Euclidean connectivity with the same foreground input where a sparse
radius-graph path exists. Ground and surface methods follow this fragmentation work.

## What is implemented

The [roadmap][roadmap] is much broader than segmentation. The actual exports and feature gates in
[the segmentation crate][features] establish the following capabilities.

| Capability                   | Implementation                                                                                                   | Relevance and limits                                                                                                                                                                    |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| DBSCAN                       | XYZ KD-tree radius queries, core and border expansion, explicit noise labels, configurable radius and core count | A useful alternative reference. CPU implementation; no DBSCAN GPU feature in this crate                                                                                                 |
| Euclidean clusters           | XYZ KD-tree breadth-first connected components, followed by minimum and maximum component-size filters           | Removes the density-core requirement; can recover sparse chains but also connect separate objects through sparse bridges                                                                |
| GPU Euclidean clusters       | Sparse cell construction, sorting and compaction, then device union/find and minimum-index roots                 | Actual GPU computation. Exact point-distance checks within neighbouring cells, not simply connected occupied voxels. Final size filtering and labelled-cloud construction remain on CPU |
| Normal region growing        | k-nearest neighbours, normal-angle agreement, optional curvature ordering and propagation threshold              | Surface patches such as road, wall, and body panels; these are not automatically whole vehicles                                                                                         |
| Ground segmentation          | Minimum height in each planar grid cell, neighbourhood erosion, height threshold, selectable up axis             | Simple local terrain baseline. No temporal background model or road confidence in this segmenter                                                                                        |
| Plane RANSAC                 | Seeded three-point hypotheses, inlier counting, covariance/eigenvector refinement; optional GPU scoring          | Useful for robust local surface fitting. Dominant plane is not necessarily road                                                                                                         |
| Multiple planes              | Repeated plane extraction on the remaining points, mapped back to original input labels                          | Candidate tool for static scene geometry and spatial priors                                                                                                                             |
| Sphere and cylinder RANSAC   | Sample-based geometric primitive fits                                                                            | Potential offline infrastructure extraction; low priority for traffic clusters                                                                                                          |
| Label and attribute handling | Labelled cloud copies and mask/index extraction preserve fields and metadata                                     | Useful ownership discipline, but full-cloud copies have a cost                                                                                                                          |

Plane and Euclidean segmentation are default crate features. DBSCAN, ground, multiple planes,
primitives, region growing, and GPU paths are optional. The Python bindings expose several of
these algorithms and offer an offline comparison route.

No HDBSCAN, OPTICS, or range-adaptive DBSCAN implementation was found in this segmentation
crate. These segmenters do not provide temporal object identity, velocity estimation, or
traffic-class validation. Broader semantic and robotics crates should not be mistaken for those
capabilities in the clustering API. The roadmap explicitly excludes cross-chunk global
clustering and cross-chunk normals from its 1.2 streaming scope. A chunked reader alone does
not solve cluster connectivity across chunk boundaries.

## Lessons to adopt

### Separate the connectivity rule from its implementation

[Euclidean clustering][euclidean] computes connected components of the radius graph. DBSCAN
expands only through sufficiently dense core points; border points can join without propagating.
Consequently, replacing DBSCAN with Euclidean clustering is a behavioural change even with an
identical radius and a final minimum component size.

Our production neighbourhood uses squared XY distance in
[`cluster.go`](../../../internal/lidar/l4perception/cluster.go); SpatialRust uses XYZ. Test the
connectivity rule in XY first. Test XYZ or an explicit vertical gate in a separate arm. Otherwise
a result mixes density semantics with vertical separation, which is especially consequential for
Pandar40P beam spacing and vehicles seen as several panels.

Our `foreground_min_cluster_points` maps to `DBSCANParams.MinPts`, the local core threshold.
`buildClusters` subsequently filters diameter and aspect ratio; it does not implement an
independent minimum component-count setting. Describe experiments using that distinction.
SpatialRust's Euclidean `min_cluster_size` is a final component-size filter, not the equivalent
of this DBSCAN parameter.

### Preserve point identity and explain rejection

[SpatialRust's results][dbscan] expose labels, cluster sizes, and noise counts. We already have
`DBSCANWithTrace`, source ordinals, and the observation tap. Extend those contracts for alternative
algorithms rather than adding another evidence path. Require one trace back to original returns
through height filtering, voxelisation, input capping, clustering, and shape rejection.

The missing useful distinction is core versus border versus density noise, with neighbourhood
counts where affordable. Existing `cluster_noise` can also cover a group that never meets the
density threshold. A shared outcome should preserve raw membership and final acceptance reasons.
Compare partitions by source ordinal, not numeric label: deterministic minimum-index roots still
change their labels when the input is permuted.

### Make performance claims cover completed work

The [GPU component implementation note][gpu-note] distinguishes CPU work, device work, transfer
bytes, frozen old code, independent references, and completed root-API timing. It also preserves
adverse cases. Its first survey reports these medians:

| Input                       | CPU KD-tree/BFS | GPU root API | Meaning                                             |
| --------------------------- | --------------: | -----------: | --------------------------------------------------- |
| 4,096-point lattice         |        8.011 ms |     3.294 ms | GPU wins this dense shape                           |
| 4,096-point reversed chain  |        1.081 ms |     3.017 ms | CPU wins this sparse shape                          |
| 16,384-point reversed chain |        4.742 ms |     8.611 ms | Point count alone still does not predict the winner |

These are upstream measurements on Windows with a discrete Vulkan GPU, not our measurements.
They include root computation and transfers but exclude high-level size filtering, label
remapping, and cloud assembly. They establish neither Pi performance nor end-to-end tracking
benefit. The API's automatic point-count threshold should not become our deployment policy.

For us, measure search construction, neighbourhood queries, component expansion, output
assembly, and the complete frame callback. Record allocations, candidate pairs, dense-cell
occupancy, cap frequency, and p95/p99 latency. GPU transfer accounting is worth borrowing if a
backend is eventually justified; GPU adoption is not a prerequisite for these improvements.

### Use independent references and difficult geometry

The GPU tests compare minimum-root components with brute-force graph traversal, a frozen prior
implementation, and analytic expectations. Cases include long chains, duplicates, boundary
distances, partial dispatches, invalid capacity, and concurrency. Apply the same discipline to a
Go clustering change: the baseline must not call the candidate's own neighbour search.

Add sparse bridges between two dense bodies, side-by-side vehicles, disconnected coplanar panels,
vertical overlap, empty input, and dense near-field frames. Test partition equality for exact
backend changes. For DBSCAN, separately specify the assignment of a border point reachable from
two components; ambiguous border assignment is not a reason to conflate the core components.

## What to improve before copying an approach

These are source-level observations and derived failure cases, not reproduced runtime failures.

1. **DBSCAN queue growth.** SpatialRust checks `visited` when enqueueing, but a point may be queued
   repeatedly before its first visit. Dense neighbourhoods can therefore accumulate quadratic
   queue entries. Our `expandCluster` already has a queued bitmap and reusable query scratch.
   Preserve those improvements; the upstream code is not automatically a faster replacement.
2. **Euclidean output assembly.** `finalize_euclidean_clusters` scans all input roots once for
   every accepted component, giving O(N × K) work for N points and K components. Our single-pass
   bucketing already avoids that pattern. Upstream root-API timings exclude this step.
3. **Region-growing distance.** The [region grower][regions] has a k-neighbour rule without a
   maximum spatial distance. Two widely separated patches with matching normals can connect
   when k exceeds the local patch support. Use a bounded radius or an explicit distance gate,
   finite unit-normal validation, and curvature confidence before trying this on sparse returns.
4. **Ground is a lower-envelope heuristic.** The [ground code][ground] performs erosion only,
   despite the module comment calling it a morphological opening. A roof patch larger than the
   erosion window, with no observed road underneath, can seed its own interior as ground.
   A steep slope can exceed the height threshold relative to an eroded neighbour. The dense
   bounding grid also has no explicit cell-budget check before allocating `cols * rows`.
   Require bounded sparse storage or checked extents, slope-aware tests, and unknown-support
   handling; do not erase low vehicle returns based on this heuristic alone.
5. **Plane model and membership.** [Plane finalisation][plane] refines the model but returns the
   pre-refinement inlier mask. Threshold membership can therefore disagree with the returned
   plane. Reclassify after refinement, or explicitly preserve both consensus and refined
   memberships. The multi-plane path does reclassify remaining original points.
6. **Road constraints.** Unconstrained RANSAC can select a wall or vehicle side. Road extraction
   needs an up/gradient prior, support and residual checks, and an explicit unsupported result.
   Do not fit ground using only the foreground objects being classified.

The shared `PointCloudSegmenter` trait exposes only a name. It is not a complete interchangeable
segmentation execution contract. Likewise, our `ClustererInterface` is not currently the
production seam: `tracking_pipeline.go` calls DBSCAN directly. A new algorithm needs deliberate
wiring and trace parity, not just another implementation of the existing interface.

## Interpret our existing evidence before proposing another algorithm

| Evidence already in this repository                                                                                                                                                   | Implication                                                                                                           |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| [Range-adaptive epsilon](l4-range-adaptive-eps-2026-10.md): 20–35 m fragmentation stayed at 54%; 35–50 m stayed around 46%, under the then-shipped L3                                 | Do not repeat a wider-radius experiment merely because another library uses radius search                             |
| [CMR input cap](l4-dbscan-input-cap-2026-10.md): 8,000 → 16,000 points raised W3 retention 0.869 → 0.972, but fragmentation stayed at 17% and HOTA moved 0.533 → 0.534                | Recovering points is valuable but does not establish recovered object connectivity                                    |
| Same cap study: p99 frame time 48 → 129 ms on a shared Mac                                                                                                                            | Uncapping is not an established solution to the Pi budget; these are not Pi timings                                   |
| [Stage B return fate](w3-return-fate-stage-b-kirk0-2026-10.md): under CMR, 72% of mid-range gap returns were DBSCAN noise and 13% L3 background                                       | Density/connectivity is a specific L4 hypothesis worth testing under that arm                                         |
| [l4clus0003](l4-min-cluster-size-kirk-2026-10.md): core threshold 8 → 5 raised 20–35 m fragmentation 35% → 36%, unlabelled clusters 0.52 → 1.43/frame, and lowered HOTA 0.533 → 0.515 | Every acceptance clause failed; keep 8 in Stage B and prioritise a joining rule over another minimum-threshold retune |
| [L3 consolidation](replay-l3-consolidation-2026-10.md): new defaults use closeness 2.0, margin 0.30 m, and neighbour confirmation 5                                                   | Historical shipped-L3 findings are not measurements of today's default; include a fresh current-default control       |

CMR is an experimental retune, not the current default. Its mid-range failure attribution should
not silently become an attribution for all sites or the new default. The two reviewed kirk0
trucks do not provide an adequate side-by-side merge test. W3 point masks are available on
kirk0 in these reports; kirk1 reference coverage is not interchangeable with point-level W3.

We already implement regional ground fitting from settled background and optional
`SurfaceHeightFilter`, with a separately usable blended surface for tracker boxes. SpatialRust
should supply challenger estimators and test cases, not prompt a duplicate ground subsystem.

## Proposed experiments and coding sequence

Row 1 incorporates an answered experiment; the subsequent rows are proposals, not scheduled
runs or new claimed results. Complete the parameter files and experiment specs before replaying
them. New selectors need config validation first; do not encode an unavailable algorithm as an
ad hoc command-line patch.

| Order | Question and arms                                                                                                                                                           | Coding work                                                                                                                             | Decision evidence                                                                                                                                                      |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | Prioritise [l4clus0003's answer](l4-min-cluster-size-kirk-2026-10.md): keep 8 in Stage B; no `cr` arm added to l3bg0009                                                     | No new replay or threshold implementation; carry the result into the next joining spec                                                  | Failed every clause on kirk0; extra detections became extra fragments; held-out and Pi claims remain open                                                              |
| 2     | Do the reviewed fragments have a sparse XY radius-graph path that the density rule cannot traverse?                                                                         | Independent brute-force XY oracle, core/border diagnostics, membership comparison; retain current-default and CMR controls              | Establish whether exact Euclidean connectivity can join the target fragments at the existing radius; absent paths direct the work to motion-supported fragment joining |
| 3     | Are surviving sparse chains rejected by the core rule? Compare DBSCAN XY with Euclidean XY at the same radius, input set, cap, and final shape filters                      | Named experimental L4 selector, exact radius edges, deterministic components, separate component-size control, shared membership result | W3 by range/class, reviewed merge rate, false clusters, HOTA/IDF1, and full callback cost; genuine joining distinguished from extra small clusters                     |
| 4     | Does a vertical gate prevent bridges without splitting vehicle panels? Compare winning XY rule with one fixed vertical constraint; XYZ is another separate arm              | Distance-policy option and boundary fixtures; retain original point ordinals                                                            | Side-by-side and vertically overlapping cases, truck fragmentation, pedestrian retention, no improvement claimed from cluster count alone                              |
| 5     | Can bounded spatial reduction retain more object support than uniform capping? Existing cap versus provenance-preserving voxel representatives at matched processing budget | Original-to-representative mapping, raw occupancy and retained-support accounting; explicitly define density weighting                  | W3 on original returns, partition distortion, OBB/extent effects, false merges, Pi p99; representative counts are not raw return counts                                |
| 6     | Does robust local ground fitting improve sloped-road filtering? Current regional surface versus bounded local RANSAC; eroded minima as a diagnostic baseline                | Reuse settled L3 input and `HeightSurface`; constrain slope, return confidence/unknown, reclassify refined inliers                      | Road leakage and wheel/low-body loss, kerb and steep-grade cases, surface error against independent references, downstream tracking                                    |
| 7     | Can local normal patches improve offline scene priors or explain body-panel splits?                                                                                         | Radius-bounded region grower on settled scene geometry or cluster-local points; normal confidence                                       | Surface-patch quality first; only then test any proposal to group patches into an object                                                                               |

A contiguous grid is a later optimisation proposal, not a claim that the current hash grid is
the bottleneck. Profile after the joining question is answered. Existing voxelisation and
density-preserving cap options must be included in the inventory before implementing substitutes. A representative-based algorithm
changes sampling and potentially density semantics; it is not an exact backend replacement.

For order 3, preregister a proposed promotion rule before running: at least five percentage
points less fragmentation in the target band, no increase in reviewed cross-object merges,
HOTA and IDF1 each within 0.01 of control, and unlabelled clusters no more than 1.5 times control.
Report denominators and paired uncertainty by episode/site; correlated frames are not independent
replicates. Passing kirk tuning data is only a screen. Require a frozen labelled holdout with
close vehicles and the target Pi's full-frame budget, nominally 100 ms at 10 Hz, before a default
change. Record p95, p99, maxima, and overruns; a mean below budget is insufficient.

Do not treat lower fragmentation as success if it comes from missing an object or merging two
objects. Keep retained share, main-component share, misses, splits, merges, and density/shape
rejection visible together. Changes to extents also need the geometry/following gates; good
segmentation scores do not physically validate vehicle length, speed, or following distance.

## Changes to existing plans

- Prioritise [l4clus0003's answered result](l4-min-cluster-size-kirk-2026-10.md): keep 8 in Stage B
  and leave `cr` out of l3bg0009. Its parameter changes core density, not final component size.
  Preserve its predeclared rule and kirk0-only scope. Register the fragment-joining follow-up
  ahead of cost-only, ground, and GPU work once its schema and case set exist.
- Refresh the [clustering observability plan](../../plans/lidar-clustering-observability-and-benchmark-plan.md)
  against implemented timers, observation taps, deterministic subsampling, and performance gates.
  Add only missing core/border diagnostics and alternative-algorithm comparisons. Its parked
  release status is not changed by this review.
- Feed ground-estimator comparisons into the existing geometry convergence and ground-plane
  work. Feed normal patches and multi-plane extraction into spatial priors, not a new runtime
  segmentation platform.
- Defer GPU integration, HDBSCAN, and a Rust runtime dependency until the simpler experiment
  identifies an accuracy or cost limitation they would address. An offline SpatialRust adapter
  can remain a development tool, with pinned source and identical point sets.

Every executed arm must live in `config/experiments/`, list changed keys in its README, and record
its path and fingerprint. Freeze the code revision, capture digest, label-pack version, split,
warm-up, sampling policy, input ordering, and machine. Keep raw outputs local; publish edited
per-case findings and limitations in a report linked from the experiment, plan, and devlog.

## Review method and limits

Read the pinned roadmap and feature matrix, segmentation source and tests, KD-tree query paths,
GPU component shader and measurement note, and local production L3/L4 orchestration and recent
experiment reports. Algorithm and complexity observations above come from source inspection.
Numerical results are attributed to upstream or existing local reports; no new replay or physical
validation was performed. Rust/Cargo is unavailable in this environment, so the upstream test
suite was not rerun. Synthetic upstream tests establish limited algorithm behaviour, not
roadside detection accuracy. GPU and broad roadmap completion claims outside the inspected
paths were not independently verified.

[roadmap]: https://github.com/rsasaki0109/SpatialRust/blob/6f563321ad7b9801f49b45f52f4392141411f54c/docs/ROADMAP.md
[features]: https://github.com/rsasaki0109/SpatialRust/blob/6f563321ad7b9801f49b45f52f4392141411f54c/crates/spatialrust-segmentation/src/lib.rs
[dbscan]: https://github.com/rsasaki0109/SpatialRust/blob/6f563321ad7b9801f49b45f52f4392141411f54c/crates/spatialrust-segmentation/src/dbscan.rs
[euclidean]: https://github.com/rsasaki0109/SpatialRust/blob/6f563321ad7b9801f49b45f52f4392141411f54c/crates/spatialrust-segmentation/src/cluster.rs
[regions]: https://github.com/rsasaki0109/SpatialRust/blob/6f563321ad7b9801f49b45f52f4392141411f54c/crates/spatialrust-segmentation/src/region_growing.rs
[ground]: https://github.com/rsasaki0109/SpatialRust/blob/6f563321ad7b9801f49b45f52f4392141411f54c/crates/spatialrust-segmentation/src/ground.rs
[plane]: https://github.com/rsasaki0109/SpatialRust/blob/6f563321ad7b9801f49b45f52f4392141411f54c/crates/spatialrust-segmentation/src/plane.rs
[gpu-note]: https://github.com/rsasaki0109/SpatialRust/blob/6f563321ad7b9801f49b45f52f4392141411f54c/notes/2026-10-12_gpu_cluster_union.md
