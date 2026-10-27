# Point Cloud Library review: tracking, facets, surface and occlusion

- **Status:** Source review complete; four experiment specs proposed, no implementation or measured result
- **Scope:** PCL's registration, tracking, sample consensus, keypoints, features, range image,
  segmentation, surface, filters, recognition and ml modules, read against velocity.report's L3 to
  L8 and the owner's five questions. Not an audit of PCL's I/O grabbers, visualisation or GPU code.
- **Upstream revision:** PCL `pcl-1.15.1`, commit `3346843ea6b97697a1577293aa9cdf236b5c7ffb`
  (August 26, 2025, the latest release in October 2026), BSD-3-Clause
- **Local baseline:** `5d3e2836` (main after #870 and #871)
- **Decisions:** [D-36](../../DECISIONS.md#d-36--physical-measurement-qualification-and-gerardusl7-boundary),
  [D-37](../../DECISIONS.md#d-37--priority-first-workstream-last-backlog-metadata), D-28, D-32
- **Plan:** [PCL candidates plan](../../plans/lidar-pcl-algorithm-candidates-plan.md)
- **Related:** [SpatialRust review](spatialrust-segmentation-review.md),
  [LiDAR-Processing-V2 review](lidar-processing-v2-segmentation-review.md),
  [facet registration maths review](../../../data/maths/proposals/20261008-facet-registration-maths-review.md),
  [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md),
  [static scene local fit plan](../../plans/lidar-static-scene-local-fit-plan.md)

PCL is a catalogue of mature reference implementations, with their papers and parameters in one
place. It cannot be a runtime dependency: the product is one static Go binary on a Raspberry Pi
(D-26, D-30), and PCL is C++ over Eigen, FLANN or nanoflann and OpenMP. It is useful to us in
three ways: as an offline oracle run on exported frames on the Mac, as a specification for a small
Go port once an experiment has shown a gain, and as a source of recommendations to Gerardus,
which owns the static scene under D-36.

The most promising runtime candidate is not a vehicle algorithm at all. It is PCL's range-image
border classification (obstacle border, shadow border, veil point), applied to the ring-by-azimuth
grid L2 and L3 already hold. It gives the facet plan its extent-end labeller, gives the occlusion
continuity options negative information, and removes the mixed-pixel returns that widen cluster
silhouettes. The second is a radius-graph joining rule with a geometric condition, for the 20 to
50 m fragments that a density retune did not join. Hulls, descriptors and registration have
narrower, measurable claims, set out below.

## The five questions, answered from the source

Each answer separates what PCL implements (cited by header), what the repository has measured, and
the gain we expect. Every gain below is a hypothesis with its basis, not a measurement.

### Q0. Can PCL improve road and kerb geometry enough to make safety metrics more reliable?

**Partly, and mostly not for following distance.** The following gap is measured along a shared
path between two bodies' longitudinal extremes; ground height enters it only through which returns
survive the vertical clip. Where ground geometry matters is the lower-body losses that W3 records,
the base the published box stands on (ground0004), lane and region membership for following and
PET, and lateral clearance to a kerb, which is a v0.7.10 metric.

What PCL offers that we lack:

| Need                                               | PCL                                                                                                                                             | Today                                                                           |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| A road plane that cannot tilt onto a wall or a van | `SampleConsensusModelPerpendicularPlane` (`setAxis`, `setEpsAngle`) under MSAC or MLESAC (`msac.h`, `mlesac.h`)                                 | `l3grid.FitGroundSurface`: lower quartile, least squares, one outlier refit     |
| Kerb faces separated from road                     | `SampleConsensusModelNormalPlane` (normal-weighted inlier distance); `DifferenceOfNormalsEstimation` (`don.h`), whose tutorial is a street kerb | No normals anywhere in L3 or L4                                                 |
| Planes and their boundaries per frame              | `OrganizedMultiPlaneSegmentation` with `GroundPlaneComparator` and `EdgeAwarePlaneComparator`                                                   | Regional planes, 10 m cells, global fallback                                    |
| A kerb line                                        | `SampleConsensusModelLine` and `SampleConsensusModelParallelLine` on the plane-boundary cells                                                   | None; Gerardus draws kerbs from 3DEP and DataSF                                 |
| A ground baseline independent of planes            | `ProgressiveMorphologicalFilter` (Zhang et al. 2003)                                                                                            | None; the SpatialRust review warns an erosion-only filter lifts roofs to ground |

The perpendicular-plane constraint is the road prior the SpatialRust review asked for, in one
setter: a plane whose normal lies within a few degrees of vertical cannot be a wall, and the
residual band can be the existing 0.3 m. MSAC scores inliers by distance rather than counting
them, which is the "better-scored consensus" thread the local-fit plan lists.

**Expected gain (hypothesis).** On level sites, none: the current fit already uses the settled
background, which is the stronger source. On graded and crowned sites (Marina 4.35 %, Columbus
0.88 %) a constrained robust fit and a kerb split should cut the surface error near kerbs and
islands, where the lower-quartile fit mixes heights. The measure is ground0001's surface residual
and W3's lower-body retention on those sites. We expect centimetres of surface error and a few
points of lower-body retention, not a change in following-gap error. The kerb line is a
recommendation to Gerardus, which owns kerbs; velocity.report consumes them through the L7
adapter (v0.5.8).

### Q1. Can richer geometric descriptors reduce classification errors and fragmentation?

**Classification: possibly, but it cannot be tested yet. Fragmentation: descriptors are the wrong
lever; the joining rule is the right one.**

PCL's global descriptors fall into two kinds. Those needing normals and a repeatable frame (VFH,
CVFH, OUR-CVFH, SHOT, `vfh.h`, `our_cvfh.h`, `shot.h`) assume dense, well-sampled surfaces; a
vehicle at 30 m has one or two faces of a few dozen returns, and a planar face leaves a local frame's
in-plane axes undetermined. ESF (`esf.h`, 640 bins of point-pair distance, angle and area
statistics) needs no normals and is the one global descriptor worth an offline comparison.
`MomentOfInertiaEstimation` gives eccentricities over 370 axis directions, an offline descriptor at
that cost. Everything else the shape-descriptors plan proposes (eigenvalue ratios, vertical mass
profile, footprint) is simpler than PCL and already named there.

The obstacle is not the descriptor. Nothing measures classification accuracy (the scorecard and
corpus are Parked P2 rows), and kirk0 has 20 reviewed road users. A descriptor experiment needs a
labelled corpus of trucks, long cars, motorcyclists and cyclists, aspect-binned, because a fixed
sensor sees each class at known aspects. Until then any claim is anecdote.

For fragmentation the repository's evidence already points away from density: lifting the input
cap recovered returns and joined no fragment, and lowering the DBSCAN core threshold made more
fragments ([l4clus0003](l4-min-cluster-size-kirk-2026-10.md)). PCL separates connectivity from
density cleanly. `EuclideanClusterExtraction` (`extract_clusters.h`) is a radius-graph connected
components rule with minimum and maximum size and no core-point requirement.
`ConditionalEuclideanClustering` (`conditional_euclidean_clustering.h`) joins two points only when a
user condition holds, for example vertical overlap, height-above-ground agreement or normal
agreement. That is the joining rule the SpatialRust review's order 2 and 3 called for, with the
bridge risk controlled by the condition rather than by density.

**Expected gain (hypothesis).** The fragment-joining oracle on kirk0's reviewed masks answers
whether a path exists between fragments at the current 0.8 m radius. If paths exist, a conditional
radius graph should lower 20 to 35 m fragmentation by the five points l4clus0003's rule asked for,
without the extra small clusters a lower density threshold made. If no path exists, the answer is
motion-supported joining (the lane-stitching thread), and no L4 rule will help. That is spec
[l4clus0005](../../../data/experiments/l4clus/l4clus0005.md).

### Q2. Can per-track hulls improve clearance and following distance over boxes?

**Over a box that holds its points, a hull of the visible returns adds little to following
distance and something to lateral clearance. The dominant error is the unseen extent, and no hull
of visible returns removes it.**

PCL's `ConvexHull` and `ConcaveHull` (`concave_hull.h`, alpha shapes) compute the visible
footprint. For a following gap the quantities are the leader's rear-most and the follower's
front-most points along the path. When the published box contains its returns (W2, the default
box model since geom0006) and the observed face is square to the path, the box face and the hull's
extreme coincide. They differ at oblique views and rounded corners, where a box corner can sit
ahead of every return. For lateral clearance to a cyclist, a hull follows mirrors, racks and
trailers that a rectangle squares off; the behaviour plan already notes clearance sigma is dominated
by width.

What a hull cannot do is see the far bumper. The facet maths review places a face plane to 7 mm
from eight returns while the half-extent prior carries 0.2 m; the kirk0 pilot's truck 2 gaps were
1.6 to 2.2 m long because its footprint ended short of its front. An accumulated body-local hull
over the pass is a different object: frame-to-frame planar registration of the cluster
(`TransformationEstimation2D`, or `TransformationEstimationLM` with `WarpPointRigid3D`, or
`NormalDistributionsTransform2D`, all x, y and yaw) stacks the faces seen at different aspects.
That is the facet plan's arm B, with PCL supplying the comparator, and it overlaps W4's extent
work.

**Expected gain (hypothesis).** Visible hull against the containment box: zero to a few
centimetres on the following gap for square views, up to about 0.3 m at oblique corners, and
measurable lateral gains on narrow or irregular bodies. The first experiment bounds the gain before
any build: it measures, on the pilot's reviewed poses and pairs, how far the box face lies from the
hull extreme along and across the path. If that distance is small against the extent error, hulls
stop here. That is spec [geom0013](../../../data/experiments/geom/geom0013.md).

### Q3. Can measured static-scene geometry make repeat surveys and multi-vantage registration more reliable?

**This is Gerardus's question under D-36, and PCL's main offer is diagnostics, not a better
aligner.** [pose0001](gerardus-static-registration-2026-10.md) registered 6 of 24 sites; hypothesis
separation failed at the other 18, and no search setting helped. The report locates the fault in
how separation compares the best pose with neighbours in its own basin.

PCL's registration module returns a transform, a converged flag and a fitness score (the mean of
squared nearest-neighbour distances). It returns no pose covariance and no observability analysis
(`registration.h`, `default_convergence_criteria.h`); converged means the step stopped changing,
not that the pose is right. So swapping ICP variants would not answer pose0001. What would help,
offered to Gerardus as recommendations:

| Recommendation                                                                                                                                  | PCL source                                                                                           | Why                                                                                                    |
| ----------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Report which pose directions the matched geometry constrains, from the eigenvalues of the point-to-plane normal matrix, before accepting a pose | `CovarianceSampling` (`covariance_sampling.h`, Gelfand et al. 2003)                                  | A long wall leaves translation along it free; separation tests cannot see that, a condition number can |
| Seed hypotheses from local descriptors instead of the eye-read position                                                                         | `FPFHEstimation` with `SampleConsensusPrerejective`, or `KFPCSInitialAlignment`                      | Distinct basins come from distinct correspondences, which is the "distinct outside the seed basin" fix |
| Score a repeat survey by change, not by residual                                                                                                | `OctreePointCloudChangeDetector`, `SegmentDifferences`                                               | A nudged sensor and a changed street look the same in a residual and different in a change map         |
| Strip veil points and isolated returns from the settled background before upload, and export normals                                            | `RangeImageBorderExtractor` veil class, `StatisticalOutlierRemoval`, `IntegralImageNormalEstimation` | ASC carries no normals; mixed pixels at building edges become false surfaces                           |

velocity.report's part is the last row: the background it exports and the track guard that keeps
vehicles out of it. The repository's spatial-priors plan already names Open3D, not PCL, as the
registration baseline; this review does not change that.

**Expected gain (hypothesis).** The observability report would turn some of the 18 failures from
"separation failed" into "yaw and along-street translation unconstrained", which is a correct
refusal rather than more passes. Descriptor seeding might add passing sites where the eye-read
seed is far. Both are scored by pose0001's own rule in Gerardus, not here.

### Q4. Can occlusion-aware analysis reduce track loss without more false associations?

**Yes, as a hypothesis worth the first experiment.** A fixed sensor knows its own rays, and L2
keeps every return's channel and azimuth, so a frame is an organised range image in all but type.
PCL's range-image tools turn that into three labels per cell.

`RangeImageBorderExtractor` (`range_image_border_extractor.h`) classifies cells as obstacle
borders (the outermost return still on the object), shadow borders (background beside an
occlusion) and veil points (mixed returns between a near edge and the background, which its
tutorial calls "a typical phenomenon in 3D range data obtained by lidars"). `NarfKeypoint`
excludes shadow borders and veil points by design. `CorrespondenceRejectionOrganizedBoundary`
rejects matches on depth boundaries by counting NaN neighbours and depth steps in a 5 by 5 window.
The recognition module's occlusion reasoning (`hv/occlusion_reasoning.h`) and `ObjRecRANSAC`'s
visibility fraction are the same idea applied to object hypotheses.

Three uses follow, in order of evidence needed:

1. **Extent-end labels.** Each end of a cluster's extent is a physical border, a shadow border or a
   field-of-view edge. This is the labeller the facet maths review requires before any
   repeatability test, and it feeds W2 containment and W4's censored extents.
2. **Negative information for coasting tracks.** For a coasting track, count the predicted
   footprint's cells that a nearer return shadows against the cells where background is seen
   through the body. Shadowed supports coasting; visible background argues for ending the track.
   That is Wyffels and Campbell's mechanism with the ray geometry we already hold, and it gives the
   v0.5.6 occlusion continuity options the evidence they lack.
3. **Veil removal.** Mixed returns at silhouettes inflate cluster width and reach DBSCAN as
   bridges between a vehicle and what lies behind it.

**Expected gain (hypothesis).** Longer correct coasting through true occlusions and earlier
expiry of ghosts, measured as l5id0007's link truth, fragments per reviewed object, identity
switches and duplicate pairs per hundred. Cost is a pass over the frame's cells and, per coasting
track, its footprint's cells; we expect well under a millisecond a frame on the M1, to be measured.
That is spec [l3bg0027](../../../data/experiments/l3bg/l3bg0027.md).

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
| 7    | Planar registration (`transformation_estimation_2D.h`, `warp_point_rigid_3d.h`, `ndt_2d.h`)      | Q2         | Offline comparator for arm B and W4   | Accumulated footprint for extent beyond one face                   | Extent error on the 22 reviewed poses              |
| 8    | Observability of a registration (`covariance_sampling.h`)                                        | Q3         | Gerardus recommendation               | Correct refusals at degenerate sites                               | pose0001's rule, in Gerardus                       |
| 9    | Descriptor seeding (`fpfh.h`, `sample_consensus_prerejective.h`, `ia_kfpcs.h`)                   | Q3         | Gerardus recommendation               | Distinct hypothesis basins                                         | pose0001's rule, in Gerardus                       |
| 10   | Organised crease edges (`organized_edge_detection.h`, `principal_curvatures.h`)                  | Q1, Q2     | Offline facet proposals               | Edge-segment facets for the annotation pilot                       | Operator acceptance of proposed facets             |
| 11   | ESF and moment-of-inertia descriptors (`esf.h`, `moment_of_inertia_estimation.h`)                | Q1         | Offline, after a class scorecard      | Truck against long car where length is unseen                      | Per-class recall on a labelled corpus              |
| 12   | Greedy and Papazov hypothesis verification (`hv/greedy_verification.h`, `hv/hv_papazov.h`)       | Q4         | Reference for duplicate resolution    | A support-then-conflict rule beside l5id0004's overlap end         | Duplicate pairs per hundred                        |

Considered and set aside: the particle filter tracker (its planar `ParticleXYR` state is the x-z
plane with rotation about y, and hundreds of hypotheses per track exceed the 3 ms tracker gate);
SHOT, PFH, VFH and CVFH (need dense normals and a repeatable frame); ISS, Harris 3D and SIFT
keypoints (fire only at corners on sparse planar faces, which the maths review shows add tangential
information only where a corner is resolved); supervoxels, LCCP and CPC (part segmentation of
dense objects); Poisson, greedy projection, marching cubes and moving least squares (need dense,
multi-view, oriented normals; a single-viewpoint vehicle has none of these, and static meshes are
Gerardus's); GPU modules (CUDA, no Pi path); LINEMOD (colour and camera geometry).

## Lessons to adopt whatever is ported

- **Keep the organised frame.** L2 and L3 hold rings by azimuth; L4 flattens them to a list.
  PCL's O(N) organised methods all need the grid. Pass the grid, or cell indices, to L4 rather than
  rebuilding neighbourhoods by hash.
- **Separate connectivity, condition and density.** PCL's clustering classes make the join rule a
  parameter. Our DBSCAN fixes it; a replay selector should name the rule.
- **Make rejection a stage.** PCL's registration chains correspondence estimation, then explicit
  rejectors (distance, one-to-one, boundary, normal), then estimation. Our association and our
  facet plans would benefit from the same explicit, logged rejection reasons.
- **Converged is not correct.** PCL reports convergence and a fitness score, never a covariance.
  Anything we port must add the uncertainty the state plan requires; PCL will not supply it.
- **Label what a boundary is.** PCL distinguishes obstacle, shadow and veil borders; our extents
  and facets do not yet. This is the one distinction every downstream gate in Q2 and Q4 rests on.

## What PCL does not give us

Pose covariance or observability from registration. Temporal identity or multi-object
association: the tracking module is single-object. Classes: the ml module is a toolkit, not a
model. Pi-ready code. A background model: our polar EMA stays. Physical validation: an oracle's
output is an independent algorithm, not a reference; the frozen physical references and label0001
remain the only truth.

## Open questions and how to continue

This run stopped short of its plan twice on usage limits. The table below is what it did not
reach, written so a later run can take one row, finish it, and record the answer here before
starting the next. Take rows in order unless the owner reorders them. Each row is sized to fit
one short run.

### What survives between runs

Committed: this review, the [plan](../../plans/lidar-pcl-algorithm-candidates-plan.md) and the
four specs. Not committed, and lost when the session ends: the PCL sparse checkout, the header
digests and the per-class catalogues (registration, tracking and sample consensus, 91 classes;
keypoints, features and range images, 55; recognition and ml). They are generated research
notes, not records, so AGENTS.md keeps them out of Git. Regenerate the checkout in a few minutes
with the command below; regenerate a catalogue only for the module a row needs, never all of them.

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

| ID    | Question                                                                                                                                                                                                                          | Why it is open                                                                                            | Read                                                                                                                                                                                 | Done when                                                                                                         | Status |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- | ------ |
| OQ-01 | Do the review's PCL claims survive a skeptic? One refuting pass per question against the headers, then a fix to any claim that fails                                                                                              | The planned adversarial verification never ran; claims were checked by hand for six classes only          | This review; the headers each claim names                                                                                                                                            | Every Q0 to Q4 claim is confirmed or corrected in place                                                           | Open   |
| OQ-02 | Does PCL's organised code accept a LiDAR grid: width as azimuth bins, height as rings, range as depth, non-uniform elevations? Run `IntegralImageNormalEstimation` and `RangeImageBorderExtractor` on one exported kirk0 frame    | Both assume a camera; the review's Q4 and normal ports rest on the answer                                 | `features/include/pcl/features/integral_image_normal.h`, `range_image_border_extractor.h`, `common/include/pcl/range_image/range_image.h`; plan Item 1                               | One frame's normals and border classes inspected; the l3bg0027 implement note corrected if the camera model fails | Open   |
| OQ-03 | Segmentation and surface claims at class level: does `RegionGrowing` bound neighbour distance, does `SACSegmentation` reclassify inliers after `setOptimizeCoefficients`, and is `ProgressiveMorphologicalFilter` a true opening? | These modules were read from digests only; the SpatialRust review found all three faults in its analogues | `segmentation/include/pcl/segmentation/{region_growing,sac_segmentation,progressive_morphological_filter}.h` and their `impl/*.hpp`; SpatialRust review items 3 to 5                 | Each answered with the line that shows it; ground0006's oracle note adjusted if membership is stale               | Open   |
| OQ-04 | Which PCL version and tools does the oracle use? Homebrew and Debian `pcl-tools` versions, and the `pcl_*` tools each spec calls                                                                                                  | Packaging was never verified; the specs name classes, not tools                                           | `tools/CMakeLists.txt` and the heads of the tools the specs need; package indexes                                                                                                    | Plan Item 1 names the install, its version and one tool per oracle step                                           | Open   |
| OQ-05 | Does l4clus0004's answer change l4clus0005? The cap bites at 10 of 23 sites and the joining oracle reads the capped input                                                                                                         | l4clus0004 was answered the day this review was written                                                   | [l4-subsampled-density-2026-10.md](l4-subsampled-density-2026-10.md); l4clus0005                                                                                                     | l4clus0005 either gains an uncapped arm (`config/experiments/l4-cap100k.json`) or says why not                    | Open   |
| OQ-06 | Accumulated body-local footprint (Q2): does a planar registration comparator (`TransformationEstimation2D`, `NormalDistributionsTransform2D`) add anything W4's extent work does not?                                             | Named as rank 7 with no spec; overlaps the Parked facet arm B                                             | Geometry convergence plan section 4 and W4 row; facet plan arm B; `registration/include/pcl/registration/ndt_2d.h`                                                                   | A one-paragraph verdict: a spec, a note added to arm B, or dropped                                                | Open   |
| OQ-07 | Duplicate tracks (Q4): is a support-then-conflict rule (`hv/greedy_verification.h`, `hv/hv_papazov.h`) a useful arm beside l5id0004's overlap end?                                                                                | Listed as rank 12 with no mapping                                                                         | [l5-duplicate-tracks-2026-10.md](l5-duplicate-tracks-2026-10.md); the two headers                                                                                                    | An arm proposed in an l5id spec, or dropped with the reason                                                       | Open   |
| OQ-08 | Q3 in Gerardus: does Gerardus already report registration observability or seed from descriptors?                                                                                                                                 | Gerardus is a separate repository and was not read                                                        | Gerardus `docs/REGISTRATION-STATUS.md`, its 2026-10-24 registration maths note                                                                                                       | The plan's Item 6 says which recommendations are new to Gerardus                                                  | Open   |
| OQ-09 | Settled background clean-up before upload: does moving least squares or statistical outlier removal improve pose0001's gates more than the track guard did?                                                                       | Not assessed; the review recommends veil removal only                                                     | [gerardus-static-registration-2026-10.md](gerardus-static-registration-2026-10.md); `surface/include/pcl/surface/mls.h`, `filters/include/pcl/filters/statistical_outlier_removal.h` | Folded into OQ-08's recommendation or dropped                                                                     | Open   |
| OQ-10 | Q1 classification: which labelled corpus would let ESF and moment-of-inertia descriptors be scored, by class and aspect?                                                                                                          | Blocked on the Parked classification scorecard and corpus rows                                            | Shape descriptors plan Phase 4; classifier training plan Phases 1 and 2                                                                                                              | Recorded as a dependency on those rows, with the minimum per-class counts                                         | Open   |
| OQ-11 | What did the owner's linked ChatGPT thread add?                                                                                                                                                                                   | chatgpt.com did not resolve from the cloud environment                                                    | The thread, pasted by the owner                                                                                                                                                      | Any new question becomes a row here                                                                               | Open   |

## Review method and limits

The PCL tree was read from a sparse checkout of the pinned tag: every module's headers, the
registration and surface sources, the 101 tutorial texts and `CHANGES.md`. Two subagents
catalogued registration, tracking and sample consensus (91 classes) and keypoints, features and
range images (55 classes) from headers and implementation files; a third catalogued recognition and
ml. Segmentation, surface and filters were read from a mechanical digest of their headers' class
comments and setters, and their claims above name only behaviour stated in those comments or the
tutorials. Template implementation bodies were read where a claim depends on them. PCL was not
built or run here, so no timing or accuracy in this review is measured. The owner's linked ChatGPT
thread could not be opened from this environment; the five questions are taken as the owner wrote
them. Repository numbers are quoted from the reports linked beside them.
