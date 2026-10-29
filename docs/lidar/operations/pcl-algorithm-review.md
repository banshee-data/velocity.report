# Point Cloud Library review: tracking, facets, surface and occlusion

- **Status:** Source review complete; four experiment specs proposed, no implementation or measured result
- **Scope:** PCL's registration, tracking, sample consensus, keypoints, features, range image,
  segmentation, surface, filters, recognition and ml modules, read against velocity.report's L3 to
  L8 and the owner's five questions. Not an audit of PCL's I/O grabbers, visualisation or GPU code.
- **Upstream revision:** PCL `pcl-1.15.1`, commit `3346843ea6b97697a1577293aa9cdf236b5c7ffb`
  (pinned source for this review; no current-release claim), BSD-3-Clause
- **Local baseline:** `5d3e2836` (main after #870 and #871)
- **Decisions:** [D-36](../../DECISIONS.md#d-36--physical-measurement-qualification-and-gerardusl7-boundary),
  [D-37](../../DECISIONS.md#d-37--priority-first-workstream-last-backlog-metadata), D-28, D-32
- **Plan:** [PCL candidates plan](../../plans/lidar-pcl-algorithm-candidates-plan.md)
- **Handover:** [Astera review scope, evidence contract, and resume point](pcl-astera-handover.md)
- **Related:** [SpatialRust review](spatialrust-segmentation-review.md),
  [LiDAR-Processing-V2 review](lidar-processing-v2-segmentation-review.md),
  [facet registration maths review](../../../data/maths/proposals/20261008-facet-registration-maths-review.md),
  [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md),
  [static scene local fit plan](../../plans/lidar-static-scene-local-fit-plan.md)

PCL is a catalogue of mature reference implementations, with their papers and parameters in one
place. It cannot be a runtime dependency: the product is one static Go binary on a Raspberry Pi
(D-26, D-30), and PCL is C++ over Eigen, FLANN or nanoflann and OpenMP. It is useful to us in
three ways: as an offline oracle run on exported frames on the Mac, as a specification for a small
Go port once an experiment has shown a gain, and as a source of recommendations to Gerardus, which
owns the static scene under D-36.

The most promising runtime candidate is not a vehicle algorithm at all. It is PCL's range-image
border classification (obstacle border, shadow border, veil point), applied to the ring-by-azimuth
grid L2 and L3 already hold. It could supply evidence for the facet plan's extent-end labeller and
the occlusion continuity options. Its border traits are geometric hypotheses, not verified
physical-end or mixed-pixel labels; the sensor mapping and the effect of withholding returns need
measurement. The second is a radius-graph joining rule with a geometric condition, for the 20 to
50 m fragments that a density retune did not join. Hulls, descriptors and registration have
narrower, measurable claims, set out below.

## The five questions, answered from the source

Each answer separates what PCL implements (cited by header), what the repository has measured, and
the gain we expect. Every gain below is a hypothesis with its basis, not a measurement.

### Q0. Can PCL improve road and kerb geometry enough to make safety metrics more reliable?

**Partly, and mostly not for following distance.** The following gap is measured along a shared
path between two bodies' longitudinal extremes; ground height can affect it indirectly through
clipping, clustering, and body estimation. Where ground geometry matters is the lower-body losses
that W3 records, the base the published box stands on (ground0004), lane and region membership for
following and PET, and lateral clearance to a kerb, which is a v0.7.10 metric.

What PCL offers that we lack:

| Need                                                   | PCL                                                                                                                                             | Today                                                                           |
| ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| A near-horizontal plane that rejects wall orientations | `SampleConsensusModelPerpendicularPlane` (`setAxis`, `setEpsAngle`) under MSAC or MLESAC (`msac.h`, `mlesac.h`)                                 | `l3grid.FitGroundSurface`: lower quartile, least squares, one outlier refit     |
| Kerb faces separated from road                         | `SampleConsensusModelNormalPlane` (normal-weighted inlier distance); `DifferenceOfNormalsEstimation` (`don.h`), whose tutorial is a street kerb | No normals anywhere in L3 or L4                                                 |
| Planes and their boundaries per frame                  | `OrganizedMultiPlaneSegmentation` with `GroundPlaneComparator` and `EdgeAwarePlaneComparator`                                                   | Regional planes, 10 m cells, global fallback                                    |
| A kerb line                                            | `SampleConsensusModelLine` and `SampleConsensusModelParallelLine` on the plane-boundary cells                                                   | None; Gerardus draws kerbs from 3DEP and DataSF                                 |
| A ground baseline independent of planes                | `ProgressiveMorphologicalFilter` (Zhang et al. 2003)                                                                                            | None; the SpatialRust review warns an erosion-only filter lifts roofs to ground |

The perpendicular-plane constraint is the road prior the SpatialRust review asked for, in one
constraint: set a non-zero axis and a positive angle tolerance in radians. A near-vertical normal
excludes walls, but also fits a horizontal roof or van floor; low-return selection and road
evidence remain necessary. ground0006 proposes a 0.05 m residual threshold. PCL's MSAC minimises
the sum of distances truncated at that threshold, including outliers, rather than maximising an
inlier count. SACSegmentation reselects inliers after coefficient optimisation.

The organised segmentation classes return regions and boundaries, but comparator names do not
guarantee their documented tests: GroundPlaneComparator's implemented comparison checks normal
angles only; its distance test is commented out. EdgeAwarePlaneComparator does check Euclidean and
plane distance, normals, curvature, and a supplied distance map. DoN measures a change of normal
across scales, not kerb identity: roofs, vegetation, and inconsistent normal orientation can also
respond. ProgressiveMorphologicalFilter calls an erosion followed by dilation; a flat elevated
patch can nevertheless survive without observed road. Plain RegionGrowing has no maximum
neighbour-distance gate with its default k-nearest search. Its curvature and residual thresholds
control whether an accepted point can seed further growth, not whether it belongs to the region.
[OQ-03's source evidence and fixtures](pcl-segmentation-contracts.md) reproduce these distinctions
and confirm SAC's refined membership.

**Expected gain (hypothesis).** A robust fit and a kerb split may reduce surface error where
regional fits mix road and kerb heights, including the graded Marina and Columbus sites. The
current settled-background fit may already suffice at some sites; neither a zero gain on level
sites nor a centimetre gain on graded sites follows from PCL. ground0006 measures held-back
surface residuals. Lower-body retention and downstream gap effects need their own comparison. Kerb
candidates go to Gerardus under D-36; the L7 adapter is the planned consumer boundary.

### Q1. Can richer geometric descriptors reduce classification errors and fragmentation?

**Classification needs a labelled scorecard. For fragmentation, the existing evidence motivates a
joining-rule experiment before adding descriptors.**

VFH and CVFH use normals; OUR-CVFH adds a reference-frame construction. SHOT is a local descriptor
with local reference frames, not a global descriptor of the same kind. Sparse or partial support
can make these unstable, but a repeatable in-plane frame is not a requirement shared by all of
them. Degenerate support can leave SHOT's or OUR-CVFH's frame ambiguous. ESF (`esf.h`, 640 bins of
point-pair distance, angle and area statistics) needs no normals and is a selected global
descriptor for an offline comparison. `MomentOfInertiaEstimation` gives eccentricities over 370
angular samples at its default 10° step (with repeated directions), an offline descriptor at that
cost. Everything else the shape-descriptors plan proposes (eigenvalue ratios, vertical mass
profile, footprint) is simpler than PCL and already named there.

The obstacle is not the descriptor. Nothing measures classification accuracy (the scorecard and
corpus are Parked P2 rows). The older F0 report scored 20 road users; the local label-set inventory
at revision 2233 has 21 reviewed road-user records, including no cyclists or motorcyclists.
[OQ-10](pcl-classification-corpus-requirements.md) records the provenance and specifies a minimum
screen of 100 independent held-out objects per target class/contrast and 30 per claimed
class × aspect × range cell. These are evaluation floors, not a training-size or adoption guarantee.
Long-car membership, independent yaw, untouched sites, and the class scorecard remain dependencies.
The [original discussion](pcl-original-discussion-reconciliation.md) also suggests distinguishing
objects temporarily merged by DBSCAN. That is a point-partition question, now OQ-13, separate
from joining fragments or assigning one class to a whole cluster.

Earlier kirk0 evidence found that lifting the input cap under CMR recovered returns without
joining fragments, and lowering the DBSCAN core threshold made more fragments
([l4clus0003](l4-min-cluster-size-kirk-2026-10.md)). The later l4clus0004 corpus result changes
track counts in both directions and leaves labelled quality open; it does not remove sampling
as a confounder. [OQ-05](pcl-joining-cap-controls.md) therefore adds matched capped/uncapped
controls and frozen inputs before comparing the joining rules. PCL separates connectivity from
density cleanly. `EuclideanClusterExtraction` (`extract_clusters.h`) is a radius-graph connected
components rule with minimum and maximum size and no core-point requirement.
`ConditionalEuclideanClustering` (`conditional_euclidean_clustering.h`) joins two points only when
a user condition holds, for example vertical overlap, height-above-ground agreement or normal
agreement. That is the joining rule the SpatialRust review's order 2 and 3 called for, with the
bridge risk tested through the condition rather than assumed solved by it. Both still grow
transitive components: a chain of locally acceptable edges can join two vehicles. The default
point search is XYZ; the proposed XY graph needs an explicit search representation. A symmetric
condition and defined height support are required for an order-independent graph.

**Expected gain (hypothesis).** The fragment-joining oracle on kirk0's reviewed masks answers
whether a path exists between fragments at the current 0.8 m radius. If paths exist, the
conditional graph can be tested against l4clus0003's five-percentage-point fragmentation threshold
and its merge and spurious-cluster bounds. Existence of paths alone does not predict a pass. If no
path exists on that input at that radius, this particular graph cannot join it. Uncapped input, a
different radius, or a different L4 rule remain possible; motion-supported joining is another
candidate, not a consequence proved by a failed fixed-radius test. That is spec
[l4clus0005](../../../data/experiments/l4clus/l4clus0005.md).

### Q2. Can per-track hulls improve clearance and following distance over boxes?

**The gain over a containing box needs measurement. A visible hull can change endpoints where the
box extends beyond its returns, but it cannot recover unseen extent.**

PCL's `ConvexHull` and `ConcaveHull` (`concave_hull.h`, alpha shapes) can compute a visible
footprint after explicit projection to XY; automatic dimensionality on a 3-D body is not that
contract. For a following gap the quantities are the leader's rear-most and the follower's
front-most points along the path. When the published box contains its returns (W2, the default box
model since geom0006) and its relevant face is tight to the current returns and square to the
path, the box face and the hull's extreme coincide. Containment alone does not imply tightness:
extent memory or a size floor can leave empty space even in a square view. They differ at oblique
views and rounded corners, where a box corner can sit ahead of every return. For lateral clearance
to a cyclist, a hull may better follow sampled mirrors, racks and trailers, but cannot retain
protrusions absent from its input; the behaviour plan already notes clearance sigma is dominated
by width.

What a hull cannot do is see the far bumper. The facet maths review places a face plane to 7 mm
from eight returns while the half-extent prior carries 0.2 m; the kirk0 pilot's truck 2 gaps were
1.6 to 2.2 m long because its footprint ended short of its front. An accumulated body-local hull
over the pass is a different object: frame-to-frame planar registration of the cluster
(`TransformationEstimationLM` with `WarpPointRigid3D`, or
`NormalDistributionsTransform2D`, for x, y and yaw). The public path of
`TransformationEstimation2D` clears centroid z before computing translation, so it estimates
XY translation and yaw, not a z offset. The surrounding search must still be explicitly XY.
[OQ-06](pcl-planar-registration-disposition.md) corrects the earlier claim with a compiled
regression and retains one bounded point-to-point comparator inside facet arm B. B's main robust
point-to-plane loss and W4's censored extent belief remain distinct; no separate experiment or
NDT2D variant is added. Aligning history can combine newly observed faces but cannot reveal an
unseen bumper.

**Expected gain (hypothesis).** The size of the gain is unmeasured; containment alone gives no
centimetre-scale bound. For any direction, the extrema of a point set and its convex hull are
identical. An alpha hull retaining those extremes cannot improve this endpoint statistic; an
apparent improvement after dropping extreme points is a support-selection effect to report
separately. Local boundary clearance is a different quantity from a global lateral extreme. The
first experiment bounds the gain before any build: it measures, on the pilot's reviewed poses and
pairs, how far the box face lies from the hull extreme along and across the path. If that distance
is small against the extent error, hulls stop here. That is spec
[geom0013](../../../data/experiments/geom/geom0013.md).

### Q3. Can measured static-scene geometry make repeat surveys and multi-vantage registration more reliable?

**This is Gerardus's question under D-36.** [pose0001](gerardus-static-registration-2026-10.md)
produced six CANDIDATE sites out of 24 and accepted no poses; hypothesis separation failed at the
other 18. That historical result is not a physical-registration success count.
[OQ-08's source review](pcl-gerardus-registration-gap-review.md) verifies Gerardus main at
`a2c2ac10`: it already reports matched-geometry eigenvalues, constraint rank, and weak directions,
and gates candidate acceptance on rank. The original broad observability recommendation is
therefore redundant.

PCL's base Registration API returns a transform, convergence state and fitness, not calibrated
pose covariance. DefaultConvergenceCriteria can accept an iteration limit, small increments, or
small MSE changes. None establishes correctness. Gerardus already goes beyond that API: it
analyses the undamped point-to-plane matrix with rotations scaled at 10 m, reports weak directions,
and separately tests rank, separation, and seed agreement. Its formal covariance still needs
calibration for correlated residuals; its October 24 maths note already identifies that limit.

| Candidate or reference                                     | PCL source                                                                | Disposition after checking Gerardus                                                                        |
| ---------------------------------------------------------- | ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Matched-geometry observability                             | CovarianceSampling                                                        | Already implemented in Gerardus, including scale and directional labels; no new generic report or PCL port |
| Initial proposals from descriptors or congruent point sets | FPFHEstimation with SampleConsensusPrerejective, or KFPCSInitialAlignment | New conditional comparator to the existing raster search, subject to a matched input/compute contract      |
| Registered change maps alongside residuals and uncertainty | OctreePointCloudChangeDetector, SegmentDifferences                        | Not closed by OQ-08; differences still cannot establish the cause of change                                |
| Cleanup before upload                                      | MovingLeastSquares, StatisticalOutlierRemoval                             | OQ-09 drops blanket export cleanup; no demonstrated gain beyond the guard                                  |

CovarianceSampling's condition-number primitive is not a reason to replace diagnostics already
computed from Gerardus's fitted correspondences. Gerardus searches every heading around the
position hint, fits multiple raster peaks, and locally perturbs the best fit to measure seed
agreement. FPFH/prerejective or KFPCS would change the initial proposal mechanism; KFPCS uses
congruent four-point sets, not descriptors. Its planned mobile Scan Context retrieval is a
separate loop-closure task. Neither proposal route establishes a correct or distinct basin.

**Expected gain (hypothesis).** One offline proposal comparator might recover useful basins where
the current raster search misses them. Compare the same reference coverage, preprocessing,
proposal budget, ICP, and refusal gates, with independent pose evidence wherever physical
correctness is claimed. Full rank and repeated local convergence can coexist with a wrong
repeated-facade match. No gain, new acceptance threshold, or implementation is established here.

velocity.report owns the exported background and the track guard. [pose0002](gerardus-background-window-2026-10.md)
adopted no shorter background window. [OQ-09](pcl-background-cleanup-disposition.md) drops blanket
MLS/SOR cleanup: neither has measured benefit beyond the guard, and changing the point set can
change the held-out split and residual population. MLS moves points even without upsampling.
Reconsider only with reviewed contamination and fixed original evaluation evidence. Border-label
experiments and validated normal estimation retain their separate contracts. The spatial-priors
plan's Open3D baseline remains unchanged.

### Q4. Can occlusion-aware analysis reduce track loss without more false associations?

**Yes, as a hypothesis worth the first experiment.** A fixed sensor knows its own rays, and L2
keeps every return's channel and azimuth, but an organised array is not yet PCL's RangeImage
geometry. The base RangeImage uses a spherical angular projection, including elevation-dependent
horizontal scaling, rather than a pinhole camera. Arbitrarily assigning non-uniform laser rings to
its rows is not equivalent. IntegralImageNormalEstimation computes XYZ covariance but uses z for
its depth-discontinuity map and optional smoothing. Substituting range for Cartesian z corrupts
the geometry. OQ-02 must test a declared projection or adapter while preserving Cartesian
coordinates.

`RangeImageBorderExtractor` (`range_image_border_extractor.h`) classifies cells as obstacle
borders (the outermost return still on the object), shadow borders (background beside an
occlusion) and veil points between associated obstacle and shadow borders. It marks directional
trait bits, not an exhaustive, mutually exclusive physical classification; a veil bit is not proof
that a return is a mixed-pixel artefact. `NarfKeypoint` excludes shadow borders and veil points by
design. `CorrespondenceRejectionOrganizedBoundary` rejects matches on depth boundaries by counting
NaN neighbours and depth steps in a default 5 by 5 target-cloud window, using Cartesian z as
depth. The recognition module's occlusion reasoning (`hv/occlusion_reasoning.h`) and
`ObjRecRANSAC`'s visibility fraction are references for visibility tests on object hypotheses; the
former explicitly uses focal length and x/z, y/z camera projection and is not a drop-in LiDAR ray
test.

Three uses follow, in order of evidence needed:

1. **Extent-end labels.** Each end needs a label for physical evidence, occlusion, field-of-view loss,
   or unknown support. Missing returns, clipping, and sparsity must not be forced into a physical
   border class. PCL traits are inputs to that labeller. This is the labeller the facet maths review requires before any
   repeatability test, and it feeds W2 containment and W4's censored extents.
2. **Negative information for coasting tracks.** For a coasting track, count the predicted
   footprint's cells that a nearer return shadows against the cells where background is seen
   through the body. Shadowed is consistent with coasting; background behind the predicted body argues against
   occupancy only when ray coverage, pose uncertainty, and expected detection are accounted for.
   A missing return supplies no positive free-space evidence.
   That is Wyffels and Campbell's mechanism with the ray geometry we already hold, and it gives the
   v0.5.6 occlusion continuity options the evidence they lack.
3. **Veil removal.** Mixed returns at silhouettes inflate cluster width and reach DBSCAN as
   bridges between a vehicle and what lies behind it.

**Expected gain (hypothesis).** Longer correct coasting through true occlusions and earlier expiry
of ghosts, measured as l5id0007's link truth, fragments per reviewed object, identity switches and
duplicate pairs per hundred. Cost requires measurement: PCL includes local surface estimation and
several neighbourhood passes. No sub-millisecond estimate is established by this source review.
That is spec [l3bg0028](../../../data/experiments/l3bg/l3bg0028.md).

The separate duplicate-track proposal is closed by [OQ-07](pcl-hypothesis-verification-disposition.md).
Greedy and Papazov verification compete for nearby scene-point support; neither supplies temporal
identity, expected-visible LiDAR support, or survivor history. Disjoint fragments can both pass,
and equal-support Papazov survival can depend on input order. Drop rank 12 as a standalone arm;
retain the concept as a reference for the existing identity and visibility work.

## Algorithms worth investigating, ranked

The plan carries the schedule. This table is the shortlist with each claim's measure.

| Rank | Algorithm (PCL header)                                                                           | Question   | Use                                   | Expected gain (hypothesis)                                         | Measure                                            |
| ---- | ------------------------------------------------------------------------------------------------ | ---------- | ------------------------------------- | ------------------------------------------------------------------ | -------------------------------------------------- |
| 1    | Range-image border classes (`range_image_border_extractor.h`)                                    | Q4, Q2     | Go port on the L3 grid, replay-only   | Correct coasting and ghost expiry; extent-end labels for W2 and W4 | l5id0007 links, fragments, switches, duplicates    |
| 2    | Conditional radius-graph clustering (`conditional_euclidean_clustering.h`, `extract_clusters.h`) | Q1         | Offline oracle, then L4 selector      | Five points less 20 to 35 m fragmentation if paths exist           | W3 fragmentation by range band, merges, HOTA, IDF1 |
| 3    | Visible convex and alpha hulls (`convex_hull.h`, `concave_hull.h`)                               | Q2         | Offline bound on the pilot references | Small on following gap; lateral gains on narrow bodies             | Box face to hull extreme, along and across path    |
| 4    | Constrained robust ground plane (`sac_model_perpendicular_plane.h`, `msac.h`)                    | Q0         | Offline oracle on settled backgrounds | Centimetres of surface error near kerbs on graded sites            | ground0001 residual, W3 lower-body retention       |
| 5    | Difference of normals, normal-weighted plane (`don.h`, `sac_model_normal_plane.h`)               | Q0         | Offline kerb split; Gerardus input    | Kerb faces out of the road fit                                     | Residual near kerbs; kerb line against Gerardus    |
| 6    | Integral-image normals (`integral_image_normal.h`)                                               | Q0, Q1, Q4 | Shared primitive, ported once         | Enables ranks 2, 5 and the crease edges below                      | Per-frame cost on the organised grid               |
| 7    | Planar registration (`transformation_estimation_2D.h`, `warp_point_rigid_3d.h`, `ndt_2d.h`)      | Q2         | Bounded comparator within facet arm B | Check alignment of identical visible history                       | Facet E2 physical errors, coverage, abstention     |
| 8    | Observability of a registration (`covariance_sampling.h`)                                        | Q3         | Existing in Gerardus                  | Use existing rank and weak directions                              | OQ-08 source audit; no new gain claimed            |
| 9    | Descriptor seeding (`fpfh.h`, `sample_consensus_prerejective.h`, `ia_kfpcs.h`)                   | Q3         | Conditional Gerardus comparator       | Useful distinct basins beyond raster search                        | Matched-budget pose and refusal comparison         |
| 10   | Organised crease edges (`organized_edge_detection.h`, `principal_curvatures.h`)                  | Q1, Q2     | Offline facet proposals               | Edge-segment facets for the annotation pilot                       | Operator acceptance of proposed facets             |
| 11   | ESF and moment-of-inertia descriptors (`esf.h`, `moment_of_inertia_estimation.h`)                | Q1         | Offline, after a class scorecard      | Truck against long car where length is unseen                      | Per-class recall on a labelled corpus              |
| 12   | Greedy and Papazov hypothesis verification (`hv/greedy_verification.h`, `hv/hv_papazov.h`)       | Q4         | Dropped as a standalone arm           | No distinct temporal evidence contract                             | Reopen only with identity and retention gates      |

Considered and set aside: the particle filter tracker (its planar `ParticleXYR` state is the x-z
plane with rotation about y, and hundreds of hypotheses per track exceed the 3 ms tracker gate);
SHOT, PFH, VFH and CVFH (need dense normals and a repeatable frame); ISS, Harris 3D and SIFT
keypoints (fire only at corners on sparse planar faces, which the maths review shows add
tangential information only where a corner is resolved); supervoxels, LCCP and CPC (part
segmentation of dense objects); Poisson, greedy projection, marching cubes and moving least
squares (need dense, multi-view, oriented normals; a single-viewpoint vehicle has none of these,
and static meshes are Gerardus's); GPU modules (CUDA, no Pi path); LINEMOD (colour and camera
geometry).

## Lessons to adopt whatever is ported

- **Keep the organised frame.** L2 and L3 hold rings by azimuth; L4 flattens them to a list.
  PCL's O(N) organised methods all need the grid. Pass the grid, or cell indices, to L4 rather than
  rebuilding neighbourhoods by hash.
- **Separate connectivity, condition and density.** PCL's clustering classes make the join rule a
  parameter. Our DBSCAN fixes it; a replay selector should name the rule.
- **Make rejection a stage.** PCL's registration chains correspondence estimation, then explicit
  rejectors (distance, one-to-one, boundary, normal), then estimation. Our association and our
  facet plans would benefit from the same explicit, logged rejection reasons.
- **Converged is not correct.** The base registration API reports convergence and a fitness score without calibrated pose covariance.
  Anything we port must add the uncertainty the state plan requires; PCL will not supply it.
- **Label what a boundary is.** PCL distinguishes obstacle, shadow and veil borders; our extents
  and facets do not yet. This is the one distinction every downstream gate in Q2 and Q4 rests on.

## What PCL does not give us

Calibrated pose covariance or a matched-geometry observability report from the base registration
API. Temporal identity or multi-object association: the tracking module is single-object. Classes:
the ml module is a toolkit, not a model. Pi-ready code. A background model: our polar EMA stays.
Physical validation: an oracle's output is an independent algorithm, not a reference; the frozen
physical references and label0001 remain the only truth.

## Open questions and how to continue

This run stopped short of its plan twice on usage limits. The table below is what it did not
reach, written so a later run can take one row, finish it, and record the answer here before
starting the next. Take rows in order unless the owner reorders them. Each row is sized to fit one
short run.

### What survives between runs

Committed: this review, the [plan](../../plans/lidar-pcl-algorithm-candidates-plan.md) and the
four specs. Not committed, and lost when the session ends: the PCL sparse checkout, the header
digests and the per-class catalogues (registration, tracking and sample consensus, 91 classes;
keypoints, features and range images, 55; recognition and ml). They are generated research notes,
not records, so AGENTS.md keeps them out of Git. Regenerate the checkout in a few minutes with the
command below; regenerate a catalogue only for the module a row needs, never all of them.

```bash
git clone --depth 1 --branch pcl-1.15.1 --filter=blob:none --sparse \
  https://github.com/PointCloudLibrary/pcl.git pcl
cd pcl && git sparse-checkout set --no-cone 'doc/tutorials/content/*.rst' CHANGES.md \
  '*/include/**' 'registration/src/*.cpp' 'surface/src/*.cpp' 'tools/*.cpp' tools/CMakeLists.txt
git rev-parse HEAD   # expect 3346843ea6b97697a1577293aa9cdf236b5c7ffb
```

pcl.readthedocs.io and pointclouds.org did not resolve from the cloud environment; GitHub did.
Read headers and tutorial sources from the checkout, not over the web.

### Rules for a continuing run

1. Read this section and the plan's checklist first; do not re-scope the five questions, which
   are answered above.
2. Take the first open row. Read only the files its "Read" cell names. Spend no agents on rows
   already answered.
3. Write the answer into the row's Status cell (answered, with a sentence and a link) and push
   before taking the next row. A run that stops mid-row leaves the row open and adds one line
   saying how far it got.
4. A row that changes a spec's rule edits the spec before any replay, as the experiment
   discipline requires.

### Open rows

| ID    | Question                                                                                                                                                                                                                          | Why it is open                                                                                                   | Read                                                                                                                                                                                 | Done when                                                                                                                                        | Status                                                                                                                                                    |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OQ-01 | Do the review's PCL claims survive a skeptic? One refuting pass per question against the headers, then a fix to any claim that fails                                                                                              | The planned adversarial verification never ran; claims were checked by hand for six classes only                 | This review; the headers each claim names                                                                                                                                            | Every Q0 to Q4 claim is confirmed or corrected in place                                                                                          | Answered: source claims corrected and execution limits retained; [claim record](pcl-astera-handover.md#review-record).                                    |
| OQ-02 | Does PCL's organised code accept a LiDAR grid: width as azimuth bins, height as rings, range as depth, non-uniform elevations? Run `IntegralImageNormalEstimation` and `RangeImageBorderExtractor` on one exported kirk0 frame    | Their depth and projection conventions differ from the native grid; OQ-01 identified the mismatch                | `features/include/pcl/features/integral_image_normal.h`, `range_image_border_extractor.h`, `common/include/pcl/range_image/range_image.h`; plan Item 1                               | One frame's normals and border classes inspected; the l3bg0028 implement note corrected if the camera model fails                                | Answered: direct mapping fails on one kirk0 frame; [measurements and adapter requirements](pcl-organised-grid-compatibility.md).                          |
| OQ-03 | Segmentation and surface claims at class level: does `RegionGrowing` bound neighbour distance, does `SACSegmentation` reclassify inliers after `setOptimizeCoefficients`, and is `ProgressiveMorphologicalFilter` a true opening? | These modules were read from digests only; the SpatialRust review found all three faults in its analogues        | `segmentation/include/pcl/segmentation/{region_growing,sac_segmentation,progressive_morphological_filter}.h` and their `impl/*.hpp`; SpatialRust review items 3 to 5                 | Each answered with the line that shows it; ground0006's oracle note adjusted if membership is stale                                              | Answered: no RegionGrowing distance bound; SAC reselects; PMF opens. [Evidence and counterexamples](pcl-segmentation-contracts.md).                       |
| OQ-04 | Which PCL version and tools does the oracle use? Homebrew and Debian `pcl-tools` versions, and the `pcl_*` tools each spec calls                                                                                                  | Packaging was never verified; the specs name classes, not tools                                                  | `tools/CMakeLists.txt` and the heads of the tools the specs need; package indexes                                                                                                    | Plan Item 1 names the install, its version and one tool per oracle step                                                                          | Answered: pinned source build selected; packaged CLI gaps and planned harnesses mapped in the [tooling runbook](pcl-oracle-tooling.md).                   |
| OQ-05 | Does l4clus0004's answer change l4clus0005? The cap bites at 10 of 23 sites and the joining oracle reads the capped input                                                                                                         | l4clus0004 was answered the day this review was written                                                          | [l4-subsampled-density-2026-10.md](l4-subsampled-density-2026-10.md); l4clus0005                                                                                                     | l4clus0005 either gains an uncapped arm (`config/experiments/l4-cap100k.json`) or says why not                                                   | Answered: six matched cap/rule arms, frozen-input oracle, and explicit stopping/acceptance comparisons; [design](pcl-joining-cap-controls.md).            |
| OQ-06 | Accumulated body-local footprint (Q2): does a planar registration comparator (`TransformationEstimation2D`, `NormalDistributionsTransform2D`) add anything W4's extent work does not?                                             | Named as rank 7 with no spec; overlaps the Parked facet arm B                                                    | Geometry convergence plan section 4 and W4 row; facet plan arm B; `registration/include/pcl/registration/ndt_2d.h`                                                                   | A one-paragraph verdict: a spec, a note added to arm B, or dropped                                                                               | Answered: one bounded comparator within facet arm B; no separate W4 spec. [Disposition and corrected z contract](pcl-planar-registration-disposition.md). |
| OQ-07 | Duplicate tracks (Q4): is a support-then-conflict rule (`hv/greedy_verification.h`, `hv/hv_papazov.h`) a useful arm beside l5id0004's overlap end?                                                                                | Listed as rank 12 with no mapping                                                                                | [l5-duplicate-tracks-2026-10.md](l5-duplicate-tracks-2026-10.md); the two headers                                                                                                    | An arm proposed in an l5id spec, or dropped with the reason                                                                                      | Answered: standalone arm dropped; [source evidence, counterexamples, and existing owners](pcl-hypothesis-verification-disposition.md).                    |
| OQ-08 | Q3 in Gerardus: does Gerardus already report registration observability or seed from descriptors?                                                                                                                                 | Gerardus is a separate repository and was not read                                                               | Gerardus `docs/REGISTRATION-STATUS.md`, its 2026-10-24 registration maths note                                                                                                       | The plan's Item 6 says which recommendations are new to Gerardus                                                                                 | Answered: observability already exists; descriptor/congruent-set proposals remain new. [Pinned audit](pcl-gerardus-registration-gap-review.md).           |
| OQ-09 | Settled background clean-up before upload: does moving least squares or statistical outlier removal improve pose0001's gates more than the track guard did?                                                                       | Not assessed; the review recommends veil removal only                                                            | [gerardus-static-registration-2026-10.md](gerardus-static-registration-2026-10.md); `surface/include/pcl/surface/mls.h`, `filters/include/pcl/filters/statistical_outlier_removal.h` | Folded into OQ-08's recommendation or dropped                                                                                                    | Answered: blanket MLS/SOR cleanup dropped; [source evidence, evaluation confounds, and reopening conditions](pcl-background-cleanup-disposition.md).      |
| OQ-10 | Q1 classification: which labelled corpus would let ESF and moment-of-inertia descriptors be scored, by class and aspect?                                                                                                          | Blocked on the Parked classification scorecard and corpus rows                                                   | Shape descriptors plan Phase 4; classifier training plan Phases 1 and 2                                                                                                              | Recorded as a dependency on those rows, with the minimum per-class counts                                                                        | Answered: [corpus dependency and count contract](pcl-classification-corpus-requirements.md); comparison remains parked                                    |
| OQ-11 | What did the owner's linked ChatGPT thread add?                                                                                                                                                                                   | Original review could not access the discussion; owner supplied a readable shared copy during OQ-11              | [Original discussion](https://chatgpt.com/share/6adff4d9-bf44-83e8-a2d9-9993e4c9c469), D-36/D-37, cohort-isolation plan                                                              | Any new question becomes a row here                                                                                                              | Answered: [reconciliation](pcl-original-discussion-reconciliation.md); existing owners mapped and OQ-13 added                                             |
| OQ-12 | Does l3bg0024's ring-by-ring fate of lower-body returns (absorbed, occluded, dropout, clipped) already give l3bg0028 its occlusion and dropout labels, so the border pass reuses that `FrameTap` instead of a new one?            | l3bg0024 was answered after this review, and main's l3bg0027 (held returns to L4) shares the lower-body question | [vehicle-lower-body-by-range-2026-10.md](vehicle-lower-body-by-range-2026-10.md); l3bg0027; l3bg0028                                                                                 | l3bg0028's implement note names the reused tap, or says why the fate classes differ from border classes                                          | Open                                                                                                                                                      |
| OQ-13 | Can local geometry or descriptors propose a defensible point partition when DBSCAN merges two physical objects into one detection?                                                                                                | The original discussion suggests this; fragment joining and whole-cluster classification do not specify a split  | [OQ-11 reconciliation](pcl-original-discussion-reconciliation.md#oq-13-partitioning-merged-detections), observation-revision plan, W3 near-merge cases, PCL clustering contracts     | Map a bounded partition arm to the existing owner with exclusive point ownership and false-split/identity gates, or drop it with source evidence | Open                                                                                                                                                      |

## Review method and limits

The PCL tree was read from a sparse checkout of the pinned tag: every module's headers, the
registration and surface sources, the 101 tutorial texts and `CHANGES.md`. Two subagents
catalogued registration, tracking and sample consensus (91 classes) and keypoints, features and
range images (55 classes) from headers and implementation files; a third catalogued recognition
and ml. Segmentation, surface and filters were read from a mechanical digest of their headers'
class comments and setters, and their claims above name only behaviour stated in those comments or
the tutorials. Template implementation bodies were read where a claim depends on them. PCL was not
built or run here, so no timing or accuracy in this review is measured. The owner's linked ChatGPT
thread could not be opened from this environment; the five questions are taken as the owner wrote
them. Repository numbers are quoted from the reports linked beside them.

OQ-01 was subsequently checked directly against the pinned implementation. The [claim
record](pcl-astera-handover.md#review-record) records corrections and counterexamples. Subsequent
[OQ-02 execution](pcl-organised-grid-compatibility.md) and [OQ-03 contract checks](pcl-segmentation-contracts.md)
record their own methods and limits; The [OQ-04 tooling inventory](pcl-oracle-tooling.md) selects the source build and maps missing
campaign harnesses. [OQ-05](pcl-joining-cap-controls.md) revises the sampling controls before
execution. [OQ-06](pcl-planar-registration-disposition.md) places planar registration within
facet arm B and corrects the earlier z-translation claim. [OQ-07](pcl-hypothesis-verification-disposition.md)
drops the standalone hypothesis-verification arm. [OQ-08](pcl-gerardus-registration-gap-review.md)
removes the redundant observability recommendation and narrows Gerardus follow-up.
[OQ-09](pcl-background-cleanup-disposition.md) drops blanket export cleanup and records the
fixed-evaluation requirement. [OQ-10](pcl-classification-corpus-requirements.md) inventories current
labels and records class/aspect/range count requirements.
[OQ-11](pcl-original-discussion-reconciliation.md) subsequently read the owner-supplied discussion,
mapped its measurement programme to existing decisions and owners, and added OQ-13 for merged
point partitions. OQ-12 and OQ-13 remain open.
