# PCL open questions: Astera review handover

- **Status:** OQ-01 to OQ-09 complete; OQ-10 is the next checkpoint
- **Scope:** Adversarial source review first, followed by bounded compatibility checks
- **Baseline:** velocity.report `7e35b3705e1287f9789a8b19df57ed3bb8af6fe9` (main, #881)
- **Upstream target:** PCL `pcl-1.15.1`, `3346843ea6b97697a1577293aa9cdf236b5c7ffb`, as recorded in the review; verify the checkout before citing it
- **Canonical:** [PCL review and open questions](pcl-algorithm-review.md#open-questions-and-how-to-continue)
- **Plan:** [PCL algorithm candidates](../../plans/lidar-pcl-algorithm-candidates-plan.md)

## Purpose and starting state

Test whether the proposed PCL experiments can answer their questions before building ports. Astera
owns the adversarial algorithm review and experimental-design critique. The returning
implementation agent owns reproducible checks, repository integration, and measured follow-up.
These are roles in this session; this document does not start another agent or select a model.

The baseline was pulled from origin/main and the worktree was clean before this handover was
written. It is a detached checkout. The preparation adds local documentation changes: preserve
them when taking over. Check the actual HEAD and diff before editing, and record any source drift.
The original review's `5d3e2836` baseline describes its earlier analysis, not this handover's
HEAD.

All OQ-01 to OQ-12 were open at preparation. OQ-01 is now answered in the review record below. PCL
has not been built or run in this preparation. At preparation there was no verified local PCL
checkout, package installation, exported organised frame, Gerardus checkout, or original ChatGPT
thread. The review record now identifies the PCL source obtained during OQ-01; the other inputs
remain unchecked. Check availability rather than inferring access from a path in an earlier
report.

## First run: finish OQ-01

Read AGENTS.md and the coding standards, then this document, the canonical review, and the plan's
checklist. Take OQ-01 first. Inventory the substantive PCL claims under Q0 to Q4, including the
qualifications and recommendations, and try to refute each against the pinned implementation. Read
linked repository reports only where needed to check a claim's local premise. Do not regenerate
catalogues for every PCL module or expand the original five questions.

Use the review's sparse-checkout recipe, but put the checkout outside the repository, for example
under `/private/tmp/velocity-pcl-review/`. Preserve the pin even if a newer release is available.
Read template implementation bodies when setters or class comments do not establish behaviour.

Check these assumptions explicitly. They are questions to test, not established defects:

| Area             | Adversarial check                                                                                                                                                    |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Q0, ground       | Does the selected model constrain the plane normal as claimed, in the stated coordinate frame and angle units? What membership does the optimised model return?      |
| Q1, connectivity | Does the oracle actually search XY rather than XYZ? What height interval belongs to a point after voxelisation, and can the conditional rule bridge separate bodies? |
| Q2, hulls        | Which box-to-hull offsets can change a directional endpoint? What does an alpha hull add over a convex hull for that quantity, on the same member points?            |
| Q3, registration | Which directions are observable from the matched geometry? Separate that question from convergence, residual quality, and distinct hypothesis basins.                |
| Q4, visibility   | Which PCL classes require projective geometry or a particular range-image construction? Separate a grid's storage layout from its ray geometry.                      |
| All questions    | Are numerical gains explicitly hypotheses, and are the proposed measurements capable of supporting them? An independent algorithm is not physical truth.             |

Record the findings in this document's review record and correct supported errors in the canonical
review. Update any affected plan or spec before a replay. OQ-01 is answered only when every
substantive Q0 to Q4 claim has a disposition. A partial pass leaves it open with an exact resume
point. Do not mark a runtime assumption verified from source inspection alone.

Stop after this checkpoint for the owner to assess the next run. OQ-02 is next; this first pass
does not authorise a replay campaign, runtime port, default change, commit, push, or PR. The
review's continuing-run instruction to push each row is a publication workflow, not publication
authorisation for this local preparation. Save the edited checkpoint locally for this session.

## Follow-up ownership and reading map

The canonical table owns question wording, order, and closure conditions. This table assigns work;
it does not change those conditions. Later runs take one row at a time after the first checkpoint.

| Row   | Astera's contribution                                                                  | Implementation agent's contribution                                                                |
| ----- | -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| OQ-01 | Refuting pass across Q0 to Q4, exact source evidence, and supported corrections        | Verify findings and reconcile dependent documents                                                  |
| OQ-02 | Camera and spherical geometry assumptions; define decisive compatibility checks        | Minimal organised export and harness; execute and inspect one kirk0 frame                          |
| OQ-03 | Verify RegionGrowing distance bounds, SAC inlier membership, and morphological opening | Small reproductions where needed; correct ground0006's oracle contract                             |
| OQ-04 | Review any algorithmic compromise in the available tools                               | Verify package versions, pinning, and actual executables; specify a harness where no tool suffices |
| OQ-05 | Review how the controls separate lost samples from connectivity                        | Reconcile l4clus0004 and l4clus0005; add an uncapped arm or a supported reason to omit it          |
| OQ-06 | Decide what planar registration adds beyond W4 and facet arm B                         | Record one disposition: comparator within arm B, separate spec, or removal                         |
| OQ-07 | Decide whether support-then-conflict adds evidence beyond l5id0004                     | Map the rule to observed returns and prepare a bounded arm if justified                            |
| OQ-08 | Critique observability and descriptor-seeding recommendations                          | Inspect current Gerardus sources and identify recommendations that are actually new                |
| OQ-09 | Identify what cleanup could change, and what it cannot repair                          | Compare against guarded backgrounds under pose0001's gates if a new arm is justified               |
| OQ-10 | Define class, aspect, and range coverage; justify minimum independent-object counts    | Inventory labels, scorecard dependencies, and capture gaps                                         |
| OQ-11 | Assess additional arguments once the original thread is supplied                       | Reconcile supplied content and retain missing access as an explicit dependency                     |
| OQ-12 | Check semantic equivalence of return fates and proposed boundary labels                | Trace and reuse replayeval.FrameTap where its contract fits; name missing evidence                 |

Start with the four proposed specs when a finding affects an experiment:
[l3bg0028](../../../data/experiments/l3bg/l3bg0028.md),
[l4clus0005](../../../data/experiments/l4clus/l4clus0005.md),
[geom0013](../../../data/experiments/geom/geom0013.md), and
[ground0006](../../../data/experiments/ground/ground0006.md). For later rows, read only the files
named in the canonical table, plus necessary implementation bodies. The following recent evidence
matters when those rows are reached:

- **OQ-05:** [subsampled density report](l4-subsampled-density-2026-10.md). The cap affects ten of 23 sites, but uncapping does not establish a net track-quality gain. Avoid treating track counts as labelled recall.
- **OQ-06:** [geometry convergence](../../plans/lidar-tracker-geometry-convergence-plan.md) and [facet arm B](../../plans/lidar-facet-registration-experiment-plan.md#e2-isolate-the-registration-benefit). Preserve the distinction between visible support, inferred unseen extent, and correlated history.
- **OQ-07:** [duplicate tracks report](l5-duplicate-tracks-2026-10.md). Drawn-box overlap wrongly ends queued cars; l5id0004 uses observed returns and remains default-off pending held-out identity evidence.
- **OQ-08 and OQ-09:** [registration report](gerardus-static-registration-2026-10.md), including its later pose0002 resolution. Do not reopen the background-window sweep as if it were unmeasured.
- **OQ-10:** [shape descriptor Phase 4](../../plans/lidar-shape-descriptors-plan.md#phase-4-range-envelope-characterisation) and [classifier Phases 1 and 2](../../plans/lidar-ml-classifier-training-plan.md#scope). Repeated frames from one object do not provide independent class examples.
- **OQ-12:** [lower-body fate report](vehicle-lower-body-by-range-2026-10.md) and [held returns to L4](../../../data/experiments/l3bg/l3bg0027.md). Trace `internal/lidar/replayeval/frame_tap.go` and its consumers. Reusing the tap does not by itself establish obstacle, shadow, or veil labels.

## Evidence and return contract

Keep edited findings here; keep generated catalogues, checkout contents, logs, exported frames,
and row-level outputs outside Git. A finding must remain understandable without local output. For
each claim, record:

1. **Claim:** source question, document heading, and the precise statement being tested.
2. **Verdict:** confirmed, corrected, requires execution, deferred, or dropped.
3. **Evidence:** revision, file, symbol, and verified line range; for PCL, use a permalink at the
   pinned commit. For a local premise, name the repository report or implementation.
4. **Counterexample:** the strongest failure case considered and whether the implementation
   handles it. Label inference separately from source behaviour and measured output.
5. **Impact:** exact review, plan, spec, or implementation contract affected; state any rule
   change before running the arm. Record justified no-change dispositions too.
6. **Remaining work:** smallest decisive check, owner, acceptance condition, and missing access.

When execution is needed, specify the input identity and fingerprint, coordinate conventions,
parameters, expected failure signal, and decision rule. Use checked-in experiment parameter sets
as AGENTS.md requires. Do not substitute PCL agreement for reviewed truth, a kirk0 tuning pass for
held-out qualification, or Mac timing for Pi timing. Keep PCL offline and preserve the D-36
boundary: Gerardus owns static scene geometry.

## Review record

OQ-01 is complete as a source review. The pinned checkout was obtained and its HEAD verified at
`3346843ea6b97697a1577293aa9cdf236b5c7ffb`; the local analysis baseline remains
`7e35b3705e1287f9789a8b19df57ed3bb8af6fe9`. No PCL build, exported-frame run, or timing
measurement was performed. The entries below cover Q0 to Q4; execution-dependent claims are
explicitly retained as hypotheses rather than marked empirically verified. No acceptance threshold
was changed. Oracle contracts were corrected before any execution.

The evidence links are immutable source references. Counterexamples below are analytical checks,
not claims that a compiled reproduction ran. The first-run scope excludes a new audit of every
algorithm in the secondary ranked/set-aside catalogue.

### Q0-01: Plane orientation is constrained, road identity is not

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/sample_consensus/include/pcl/sample_consensus/impl/sac_model_perpendicular_plane.hpp#L93-L119).

isModelValid checks the unsigned normal/axis angle only when eps_angle is positive. A horizontal
roof passes exactly the same test as a road; a zero tolerance disables the check. The review and
ground0006 now require a gravity-aligned axis, radians, and separate low-return evidence. The
implementation agent must test a roof and a wall before the ground oracle is accepted.

### Q0-02: MSAC scoring and refined membership

**Verdict: Confirmed with precision.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/sample_consensus/include/pcl/sample_consensus/impl/msac.hpp#L89-L118).

The objective sums min(distance, threshold) over points, including outliers. It is not simply an
inlier count or a squared-loss formula imported from a paper. SACSegmentation.segment,
impl/sac_segmentation.hpp lines 114–125, optimises coefficients then calls selectWithinDistance
again. The review and ground0006 record this contract; OQ-03 still owns its full three-part
closure.

### Q0-03: Normals and DoN supply candidates, not kerb labels

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/don.hpp#L44-L100).

computeFeature takes half the difference of the two supplied normal vectors. Inconsistent
orientations on a flat patch can create a large response. SampleConsensusModelNormalPlane blends
angular and Euclidean residuals with curvature-dependent weighting
(impl/sac_model_normal_plane.hpp lines 80–94). Neither implements semantic kerb separation. The
review qualifies the claim and ground0006 requires consistent orientation; kerb agreement remains
measured follow-up.

### Q0-04: Organised regions exist; comparator comments overpromise

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/ground_plane_comparator.h#L206-L233).

compare checks the desired road normal and adjacent normal agreement, while distance checks are
commented out. Two displaced parallel patches are a counterexample to trusting its distance
setter. EdgeAwarePlaneComparator.compare (edge_aware_plane_comparator.h lines 169–200) does check
distance, plane offset, curvature, and a distance map. OrganizedMultiPlaneSegmentation.segment
(impl/organized_multi_plane_segmentation.hpp lines 196–228) emits boundary-bearing regions. The
review now distinguishes these behaviours. Sensor-grid suitability remains OQ-02.

### Q0-05: Line fits and morphological ground baseline

**Verdict: Confirmed, limited.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/sample_consensus/include/pcl/sample_consensus/impl/sac_model_parallel_line.hpp#L91-L115).

The parallel-line model enforces a positive angular tolerance against an axis; neither it nor a
generic line model identifies a kerb. ProgressiveMorphologicalFilter.extract
(impl/progressive_morphological_filter.hpp lines 108–130) calls MORPH_OPEN;
filters/impl/morphological_filter.hpp lines 114–190 implement the two passes. Thus the
erosion-only objection does not apply to this PCL path. A non-road low surface can still survive.
Keep these as offline candidates; OQ-03 retains its broader checkpoint.

### Q0-06: Road-error gains and safety consequences

**Verdict: Requires execution.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/sample_consensus/include/pcl/sample_consensus/impl/sac_model_perpendicular_plane.hpp#L93-L119).

No plane constraint establishes centimetre gains, unchanged level-site performance, or downstream
gap accuracy. Clipping and clustering can propagate into extent and identity estimates. The review
now describes these as unmeasured effects. ground0006 measures held-back surface residuals;
lower-body and physical-metric validation remain separate. Local context: ground_surface.go and
the linked ground0001 and W3 reports, rather than PCL, define today’s baseline.

### Q1-01: Descriptor categories and frame requirements

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/vfh.hpp#L95-L190).

VFH uses centroid/normal and pair features without SHOT’s local-frame contract. SHOT is local and
uses FeatureWithLocalReferenceFrames (shot.h lines 75–120); OUR-CVFH exposes sgurf (our_cvfh.h
lines 130–145). A planar patch does not refute all these methods for the same reason. The review
distinguishes their contracts; sparse-support accuracy remains unmeasured and OQ-10 owns the
corpus.

### Q1-02: ESF and moment-of-inertia dimensions

**Verdict: Confirmed with precision.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/esf.h#L50-L75).

ESF derives from Feature, not FeatureFromNormals, and outputs ESFSignature640; impl/esf.hpp lines
272–301 concatenate ten histograms. MomentOfInertiaEstimation uses a default 10° step
(moment_of_inertia_estimation.h line 297) and inclusive theta 0–90°, phi 0–360° loops (impl lines
156–184): 370 samples, not 370 distinct directions. A repeated polar direction is the
counterexample. The wording is corrected; no classification utility or runtime cost is
established.

### Q1-03: Radius connectivity has no core-point test

**Verdict: Confirmed, qualified.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/extract_clusters.hpp#L62-L110).

The seed queue grows through radius neighbours and size limits apply to the completed component.
Conditional extraction calls its predicate per candidate edge
(impl/conditional_euclidean_clustering.hpp lines 94–119). A long chain of compatible edges can
bridge two vehicles. The review and l4clus0005 now require bridge cases and a symmetric condition;
a local predicate does not certify object identity.

### Q1-04: XY oracle needs an explicit representation

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/conditional_euclidean_clustering.hpp#L56-L68).

The default search is OrganizedNeighbor or KdTree; it is not automatically XY.
DefaultPointRepresentation caps the default representation at three floats
(common/include/pcl/point_representation.h lines 199–218). Vertically separated points at
identical XY expose the difference. l4clus0005 now requires an explicit XY search while preserving
z and ordinals, defines singleton height intervals, and requires membership-derived intervals for
aggregated samples. The implementation agent must verify search configuration and partition
invariance.

### Q1-05: A failed graph does not rule out L4

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/extract_clusters.hpp#L77-L100).

The traversal proves connectivity only for the supplied point set and tolerance. Removing a
sampling cap or changing radius can create a previously absent path. The review no longer claims
that no L4 rule can help. The reported cap and MinPts outcomes remain local evidence from
l4clus0002/0003; OQ-05 owns the new uncapped control. Five percentage points is a proposed
acceptance threshold, not a source-derived expected result.

### Q2-01: Footprints require projection; containment is not tightness

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/surface/include/pcl/surface/convex_hull.h#L145-L163).

ConvexHull accepts dimensions 2 or 3 and otherwise determines dimensionality. A full 3-D body is
not automatically its XY footprint. A historical size floor gives a containing box with empty
space even in a square view. geom0013 now projects to XY and measures box/hull support along the
path and across it; face-normal offsets are separate diagnostics. No numerical upper bound follows
from containment.

### Q2-02: Concavity cannot improve a retained directional extreme

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/surface/include/pcl/surface/impl/concave_hull.hpp#L398-L477).

Alpha reconstruction filters simplices by radius and retains their vertices; it may omit points or
return disconnected support. Mathematical check: max over conv(S) of u·x equals max over S of u·x,
since each convex combination cannot exceed the largest member projection. An alpha hull retaining
the extremal member has the same maximum; dropping it changes support. geom0013 uses direct
extrema as a cross-check and treats alpha output as optional selection diagnostics. Local boundary
clearance would need a different metric.

### Q2-03: The three planar registration options are not identical

**Verdict: Corrected again by OQ-06; the original z-translation finding was false.**
[Pinned implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/include/pcl/registration/impl/transformation_estimation_2D.hpp#L124-L182).

The earlier pass read the final centroid subtraction without following its inputs. The public
iterator path sets both centroid z components to zero at lines 139–141, so the returned transform
has XY translation and yaw, with zero z translation. OQ-06's executable cloud/correspondence tests
confirm that a 7 m target z offset does not produce z translation. XY matching still requires an
explicit search contract. WarpPointRigid3D and NDT2D also parameterise planar motion, but their
objectives are not automatically facet B's robust point-to-plane likelihood. See the
[OQ-06 disposition](pcl-planar-registration-disposition.md) for the bounded comparator and limits.

### Q2-04: Visible support cannot recover unseen extent

**Verdict: Confirmed, gains require execution.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/surface/include/pcl/surface/impl/concave_hull.hpp#L398-L477).

Two bodies with identical visible points and different hidden bumpers produce identical visible
hulls: this is an identifiability counterexample, not a PCL benchmark. The facet maths review’s
eight-return 7 mm face estimate and 0.2 m half-extent prior are an illustrative error budget, not
measured hull performance. Accumulation can add newly observed support only with correct motion
alignment. Remove the unsupported few-centimetre/0.3 m forecast; geom0013 and OQ-06 own
measurement and overlap.

### Q3-01: Fitness and convergence do not validate a pose

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/include/pcl/registration/impl/default_convergence_criteria.hpp#L49-L140).

hasConverged may return true at the iteration limit when failure_after_max_iter is false, as well
as for small transforms or MSE changes. Registration.getFitnessScore(max_range) averages accepted
squared nearest-neighbour distances (impl/registration.hpp lines 134–164); its cutoff changes the
population. A wrong local minimum or exhausted iteration budget can pass. The review narrows the
covariance claim to the base API and corrects convergence semantics.

### Q3-02: Geometry conditioning needs direction and scale

**Verdict: Confirmed as a recommendation, qualified.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/filters/include/pcl/filters/impl/covariance_sampling.hpp#L98-L126).

CovarianceSampling forms F from scaled-point cross normal and normal, computes FFᵀ, and exposes an
eigenvalue ratio. It does not inspect the actual registration correspondences for the proposed
acceptance gate. A long single wall leaves tangent translations unconstrained; a scalar condition
number cannot identify which direction failed. The review now requires matched-geometry
eigenvectors, eigenvalues, and meaningful scaling. OQ-08 subsequently confirmed that Gerardus
already computes scaled matched-geometry eigenvalues and named weak directions; the generic
recommendation is superseded by its [novelty audit](pcl-gerardus-registration-gap-review.md).

### Q3-03: Descriptor matching and KFPCS are different seeding routes

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/include/pcl/registration/ia_kfpcs.h#L43-L64).

KFPCS uses keypoint-based congruent four-point sets and keeps candidates; it is not FPFH
descriptor matching. SampleConsensusPrerejective finds feature correspondences and tests inlier
fraction/error (impl/sample_consensus_prerejective.hpp lines 243–288). Repeated facades can give
ambiguous correspondences in either route. The review now names both routes without promising
distinct correct basins. OQ-08 confirms these initial-proposal routes are absent from the
inspected Gerardus path; feasibility and benefit still require the bounded comparison.

### Q3-04: Change detection is not a cause classifier

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/octree/include/pcl/octree/octree_pointcloud_changedetector.h#L87-L105).

The octree returns points in new leaves. SegmentDifferences compares nearest-neighbour squared
distance with a threshold (impl/segment_differences.hpp lines 52–80). A rigid pose error produces
widespread apparent change, just as scene change can. The recommendation now calls for registered
change maps alongside pose uncertainty; it no longer claims these methods distinguish a sensor
nudge from a changed street.

### Q3-05: Cleanup and normals are conditional export work

**Verdict: Qualified.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/filters/include/pcl/filters/impl/statistical_outlier_removal.hpp#L76-L125).

SOR rejects mean-neighbour-distance outliers using a global mean plus standard-deviation
multiplier. A valid isolated edge or sparse distant surface may be removed. Normals require
estimation, not merely a PCD writer. The plan now states that dependency. pose0001 supplies the
6/24 and separation findings, not PCL; no improved registration gate or guaranteed rejection
diagnosis is established. OQ-09 subsequently drops blanket MLS/SOR export cleanup, with
[fixed-evaluation conditions](pcl-background-cleanup-disposition.md) before reconsideration.

### Q4-01: RangeImage is spherical, but not the native ring array

**Verdict: Corrected; execution remains open.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/common/include/pcl/range_image/impl/range_image.hpp#L355-L438).

getImagePoint computes angular coordinates and getImagePointFromAngles scales horizontal angle by
cos(elevation). createFromPointCloud uses specified angular resolutions (lines 110–137).
Non-uniform ring spacing cannot simply be assigned to these rows. The review, plan, and l3bg0028
now require a declared projection/adapter and identity mapping. OQ-02 must run the real frame;
this source result does not close it.

### Q4-02: Covariance XYZ and depth control are separate

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/integral_image_normal.hpp#L205-L250).

The covariance method forms a matrix from Cartesian XYZ sums. The depth-change map reads point.z
(lines 736–765), and depth-dependent smoothing also reads z (lines 875–895). A wall viewed at
changing azimuth exposes why z and radial range differ; replacing z changes the surface being
fitted. The plan and l3bg0028 preserve XYZ and require a separate depth-control adaptation, to be
tested under OQ-02.

### Q4-03: Border traits are not physical truth or exclusive labels

**Verdict: Corrected.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/src/range_image_border_extractor.cpp#L399-L460).

The code sets directional obstacle/shadow trait bits and marks intervening cells as veil. It does
not establish sensor mixed-pixel causation or supply a field-of-view/unknown physical-end
taxonomy. A valid thin surface between a foreground border and background is a counterexample to
blindly treating veil as noise. NARF does skip shadow/veil bits (keypoints/src/narf_keypoint.cpp
lines 298–332). l3bg0028 now preserves bits and unknown support; withholding remains an experiment
against reviewed returns.

### Q4-04: Boundary rejection and recognition visibility are camera-specific references

**Verdict: Confirmed with limits.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/src/correspondence_rejection_organized_boundary.cpp#L46-L85).

The rejector tests target organisation and counts invalid z or z steps in a window; the header
defaults to width 5 (lines 143–144). hv/occlusion_reasoning.h lines 72–119 projects x/z and y/z
using focal length. ObjRecRANSAC has a visibility parameter (ransac_based/obj_rec_ransac.h lines
458–460), but that is not calibrated coasting evidence. The review now names the camera and
adaptation limits.

### Q4-05: Coasting evidence and cost are proposed work

**Verdict: Requires execution.** [Pinned
implementation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/src/range_image_border_extractor.cpp#L101-L145).

PCL computes local surfaces before border scores, then multiple neighbourhood passes. Source
inspection supplies no sub-millisecond M1 timing. A missing return, sparse beam coverage, or
uncertain footprint cannot certify free space. The review removes the timing estimate; l3bg0028
requires declared coverage, detection, and uncertainty gates before the coast arm. Its acceptance
thresholds remain unchanged; labels, identity gains, and Pi performance are unmeasured.

### Checkpoint validation

Pinned source HEAD and all 25 primary source-link paths and line ranges were checked locally.
The changed documents pass Prettier, relative-link, British spelling, header metadata, geometry
wording, and diff-whitespace checks. The Git index passes `make check-no-results`. Prettier
parsed the four experiment files; a separate Python YAML check was unavailable because PyYAML
is not installed. No compiled PCL test, replay, accuracy measurement, or runtime benchmark ran.

## Return to the implementation agent

After the Astera checkpoint, verify its citations and proposed corrections against the recorded
revisions. Reconcile the canonical review, plan, and affected specs, preserving unresolved claims.
Summarise what source inspection established, what needs execution, and any owner decision.

For OQ-02, first verify installation and export availability, then prepare the smallest harness
that tests the camera assumptions. Do not build a Go port on an untested oracle representation. If
the one-frame result contradicts the design, revise the experiment before further replay. After
the open-question checkpoints, follow the plan's experiment sequence: border classes, joining
rule, hull bound, and constrained ground. Apply each experiment's stop rule before funding its
implementation. Publication and execution scope are agreed at those checkpoints.

## Continuation checkpoint

The owner authorised continuation and incremental local commits on
`codex/pcl-open-question-review`. OQ-01 is saved as `66a0cf558`. The earlier first-run stop and
no-commit instruction applied to that completed review phase; local commits are now authorised.
The owner subsequently authorised OQ-04 and OQ-05, incremental commits, and opening a PR.
Pushing this review branch and opening a draft PR are now authorised.

OQ-02 is answered by the [one-frame compatibility report](pcl-organised-grid-compatibility.md):
the native grid fails PCL projection consistency and normal rotation equivariance. The report,
exporter, parameter set, and C++ harness preserve the method; raw outputs remain local. A
calibrated adapter or explicit resampling contract is required before the border oracle.

OQ-03 is answered by the [segmentation contract report](pcl-segmentation-contracts.md).
Pinned source and executable fixtures establish: default RegionGrowing has no spatial distance
bound and residual failure affects seed eligibility, SAC reselects plane membership after
refinement, and PMF performs erosion then dilation. ground0006 retains its original experiment
rule and now makes the fit/held-back index separation explicit. No site campaign ran.

OQ-04 is answered by the [oracle tooling runbook](pcl-oracle-tooling.md): package versions
checked, source installation selected, and each oracle step assigned an existing diagnostic or
an explicitly planned campaign harness. Item 1 is not declared implemented.

OQ-05 is answered by the [joining-control design](pcl-joining-cap-controls.md). l4clus0005 now
has six matched cap/rule arms, a frozen-stream oracle, a per-treatment stop gate, and separate
comparisons against a0 and the matched-cap DBSCAN control. The existing high-cap config differs
from defaults only in the cap. No new campaign ran and the spec remains proposed.

OQ-06 retains one bounded comparator within facet arm B, with no separate W4 workstream. Its
[source and regression record](pcl-planar-registration-disposition.md) corrects OQ-01's false
z-translation claim. No iterative registration comparator or campaign was implemented.

OQ-07 drops the standalone Greedy/Papazov duplicate-track arm. The
[disposition](pcl-hypothesis-verification-disposition.md) maps six source claims and four
analytical counterexamples to observed support, visibility, temporal identity, and survivor
handling. l5id0004/0005's historical results remain unchanged; l5id0006, l5id0011, and l3bg0028
retain their existing scopes. No hypothesis-verification run or duplicate campaign was performed.

OQ-08 is answered by the [Gerardus novelty audit](pcl-gerardus-registration-gap-review.md).
Verified main `a2c2ac10` already supplies observability diagnostics and rank gating. Descriptor or
congruent-set initial proposals remain a conditional comparison to its multipeak raster search;
mobile Scan Context is a separate plan. Item 6 is narrowed accordingly, with no Gerardus changes,
external issue, campaign, or pose acceptance.

OQ-09 drops blanket MLS/SOR background cleanup from Item 6. Its
[source and evaluation record](pcl-background-cleanup-disposition.md) explains why density is
not temporal contamination, MLS moves points even without upsampling, and filtering can change
Gerardus's held-out set or lose confidence fields. The guard and pose0002 results are preserved;
no cleanup trial ran. Reopening requires reviewed contamination and fixed original evaluation
support, not merely lower residuals on a filtered cloud.

## Resume point

- **Active row:** OQ-10, not started; OQ-01 to OQ-09 complete
- **Next action:** Read shape-descriptor Phase 4, classifier training Phases 1/2, and the current labelled-corpus records. Identify the owner dependencies and justify minimum independent-object coverage by class, aspect, and range for scoring ESF and moment-of-inertia descriptors. Count independent objects/site splits rather than repeated frames; retain unavailable labels as an explicit dependency.
- **Preserve:** OQ-08 found existing Gerardus observability; OQ-09 dropped blanket MLS/SOR cleanup without running it. The descriptor-seeding comparator, border-label experiment, and classifier descriptor comparison are different questions.
- **Execution evidence:** OQ-02 one-frame check, OQ-03 segmentation fixtures, OQ-06 2-D estimator regression; OQ-07/08/09 are source analysis and prior-report synthesis. No descriptor or cleanup campaign.
- **Validation:** Full `make test` and `make lint` passed at OQ-08. OQ-09 changes documentation only; final lint, links, source ranges, arithmetic, and the no-results guard passed; the staged diff was reviewed. Gerardus tests were inspected, not rerun.
- **Outstanding inputs:** Original discussion for OQ-11; calibrated sensor-to-oracle adapter before l3bg0028; campaign harnesses from OQ-04; labelled classification coverage to inventory in OQ-10
- **Local source caches:** `/private/tmp/velocity-pcl-review/pcl` at the pinned PCL revision; `/private/tmp/velocity-pcl-review/gerardus-oq08` holds disposable main source snapshots. Gerardus main `a2c2ac10` was verified through the local GitHub CLI.
- **Checkpoint:** Draft PR #882; OQ-07 `342211be9`; OQ-08 `e1db67ddc`; OQ-09 saved in this checkpoint commit. The owner's continuation authorised completion of OQ-08 and OQ-09 and pushes to the same PR. Stop here; no PR merge is authorised.
