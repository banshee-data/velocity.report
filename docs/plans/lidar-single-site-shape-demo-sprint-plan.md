# Three-day point annotation, shape, and trail demo

This sprint demonstrates one vehicle remaining one object as its visible surfaces change. It
pairs reviewed point masks with a small, inspectable shape model and compares the resulting
trail with the current tracker on one site.

- **Status:** Proposed; implementation not started by this plan
- **Layers:** Offline analysis, L4 Perception, L5 Tracks, L6 Objects, L10 Clients
- **Scope:** Three engineering days, one site, one annotation client, seeded rigid-vehicle
  tracking
- **Related:** [Annotation contract](lidar-point-annotation-and-object-dataset-plan.md),
  [Shape descriptors](lidar-shape-descriptors-plan.md),
  [State estimation](lidar-state-estimation-plan.md),
  [Geometry proposal D-04](../../data/maths/proposals/20260222-geometry-coherent-tracking.md),
  [Test corpus](lidar-test-corpus-plan.md)

**Mathematical contract:** Follow the
[visibility-aware review](../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md).
Low registration error is not pose certainty. The bounded tracker must test perturbed,
re-matched pose alternatives, identify prior-dominated directions, and avoid counting
cached points or a motion prior twice as independent evidence. Human membership masks do
not provide temporal point correspondences.

## 1. What will be demonstrable

An operator opens a frozen VRLOG excerpt, marks an object with a lasso and depth slab, and
corrects its membership in later keyframes. Replay shows the observed points, accumulated
object-local shape, estimated body box, and trail. A descriptor panel explains why a compact
JSON model favours a chosen shape class or abstains. Baseline and candidate can be compared at
the same capture time.

This is a research demo, not a three-day promise to solve classification, occlusion, and
tracking everywhere. Delivery is the working annotation-to-evaluation loop; an improved
held-out trajectory is a separate measured outcome. No large neural weights, production
rollout, or automatic make/model recognition is required.

## 2. Starting branch and dependencies

Planning re-inspected root branch `dd/docs/state-est` at `6a9b6ebf6`. Its heading-coherence
sprint now includes the committed headless PCAP replay harness and reports Day 1 A/B results.
Those results were read, not independently reproduced here. The mathematical review
separates reported proxy improvements from the remaining accuracy and identity gates.

The active execution document is `docs/plans/lidar-heading-coherence-sprint-plan.md` on that
branch; it is not present in this planning worktree. Integrate this plan there after
reconciling the live changes. Do not copy older runtime files over the active agent's work.

The sprint uses the existing renderer and playback controls, a local annotation pack, and an
offline tracker. It does not depend on finishing the full QC workbench, a new database schema,
all descriptor families, an IMM, or per-point velocity extraction. Do not broaden the live
tuning surface.

### Day-zero readiness, within the first two hours of Day 1

- Select one accessible site and pin a recording, source PCAP if available, config, and build
  digest.
- Candidate: run `baf20f02-075b-4041-9860-ff090754f94f`, sourced from
  `s2_sf_4_20260902153250_00003.pcap`. The PCAP was found in the root checkout's sibling
  `sensor_data/lidar/static` directory; do not rely on the unavailable `/Volumes/lidar` mount.
- Verify point-bearing frames, capture ordering, calibration, and annotation coverage. Use the
  source pack unchanged for A/B; reproduce settling and completion barriers for any PCAP rerun.
- Confirm the visualiser builds on the demo machine. Do not count an old app bundle as this
  build.
- Identify at least two actual shape families at the site. Box trucks, buses, and a verified
  vehicle platform are candidates, not a promise that the selected minute contains them.

If raw-scene retention is unavailable, use recorded foreground and label the demo accordingly.
If the selected site lacks two families, demonstrate tracking for one family plus unknown
distractors and mark multi-class validation deferred. Do not invent examples or substitute
another site silently.

## 3. Dataset and operator budget

Target 12 distinct objects across two supported families, with at least six per family. Assign
three per family to model fitting, one to tuning, and two to held-out evaluation: 6/2/4 objects
in total. This is a demo sample, not enough for a general accuracy claim. Freeze the split
before fitting.

Review about eight keyframes per object, roughly 96 masks, prioritising approach, closest pass,
departure, turns, sparse views, and overlap with another object. Include the reported split
vehicle as a regression case, with both predicted IDs mapped to one human identity only after
inspection. Reserve about four hours of operator review in addition to the engineering budget;
record actual annotation time. If one person does both jobs, cut stretch work rather than skip
review.

Each object needs a reviewed seed box with a stated physical-extent uncertainty. Later masks
remain evaluation-only during unassisted tracking. Annotate pose at a smaller set of reviewable
keyframes for trail/heading checks; record unobservable front/rear direction instead of
guessing it.

## 4. A small model, not a hidden checkpoint

Ship a versioned JSON parameter file, with a proposed 64 KiB limit per shape family. It
contains named values only. Point clouds, per-track shape caches, masks, and descriptors are
dataset/runtime state stored separately; small model parameters do not make those data
disappear.

| Parameter group   | Contents                                                                                       |
| ----------------- | ---------------------------------------------------------------------------------------------- |
| Identity          | Schema/model version, units, body origin, supported semantic label, and research subtype       |
| Shape             | One cuboid initially; optional cab/body parts only as stretch work                             |
| Priors            | Broad physical dimension ranges, uncertainty, and provenance of supplied dimensions            |
| Descriptor score  | Feature names, robust centres/scales, weights, missing-feature rules, and abstention threshold |
| Tracking          | Crop margin, voxel size, robust-loss scale, iteration limit, motion weight, and overlap floor  |
| Validity envelope | Point support, range, viewpoint support, and expected rigid-body assumptions                   |
| Reproducibility   | Training object IDs, split digest, feature formula version, and fitting tool version           |

Fit descriptor centres and scales on fitting objects only; tune thresholds on the tuning
partition. Use a small robust distance score over valid features, normalised by available
feature weight. Abstain on insufficient support or poor fit. Expose each feature's contribution
in the inspector. Class scores are not calibrated probabilities unless calibration is
separately demonstrated.

Initial physical dimensions come from reviewed seed priors, not means of partially visible
extents. Class scoring and single-object tracking are independent: a seeded vehicle can be
tracked while its class remains unknown. No object is rejected solely because a template says
it should look different.

## 5. Descriptors and evidence to add

The existing eigenvalue and height-distribution plan helps class scoring. Registration also
needs local correspondences and observability; a global descriptor cannot replace the point
cloud.

| Evidence/descriptor                                                          | Purpose                                                          | Sprint priority                                               |
| ---------------------------------------------------------------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------- |
| Point count, range, density, clipping, and valid-feature mask                | Explain when a score is unsupported                              | Required                                                      |
| Sorted covariance eigenvalues, planarity, linearity, and eigenvalue gap      | Describe surface support and axis ambiguity                      | Required                                                      |
| Ground-relative height quantiles and a small vertical occupancy profile      | Distinguish flat roof/body support from low or irregular objects | Required where ground reference is valid                      |
| Object-local longitudinal height profile                                     | Compare cab/body proportions or roof equipment                   | One simple profile if pose is credible                        |
| Registration inlier fraction, residual quantiles, overlap, and yaw ambiguity | Detect a wrong correspondence or an unobservable motion          | Required tracking diagnostics                                 |
| Viewpoint bins and newly observed surface support                            | Prevent repeated side views creating false dimension confidence  | Required metadata; simple bins                                |
| Local normals, plane offsets, extents, support, and fit residual             | Enable point-to-plane fitting and persistent face interpretation | Next increment; prototype only if time remains                |
| Partial-overlap-aware local shape cache                                      | Preserve useful recent surface evidence without unlimited growth | Required bounded tracking state                               |
| Free-space/ray consistency                                                   | Reject geometry where rays actually establish empty space        | Later; requires reliable ray provenance, not inferred absence |
| FPFH local descriptors                                                       | Candidate correspondence/reacquisition aid                       | Later benchmark, not a required SOTracker input               |

All descriptors carry support count, range, frame/pose provenance, and validity. Compute shape
features from human masks for the oracle experiment and predicted masks for the actual demo.
Preserve both results. Do not use a smoothed predicted heading to define the reference heading.

FPFH describes local relationships using points and normals; it is a useful non-neural
candidate for difficult correspondence, but adds neighbourhood and normal-estimation costs.
Sparse or planar car returns may not distinguish positions along a face. Test it before adding
a native dependency.
[PCL documentation](https://pointclouds.org/documentation/tutorials/fpfh_estimation.html)

Intensity statistics remain optional and sensor-specific. Reflection, range, and incidence can
change them. Do not make operator identity or a vehicle subtype depend on uncalibrated
intensity.

## 6. Tracking comparator and trail contract

Mandatory candidate: a small offline, SOTracker-inspired baseline, not a claimed reproduction.
Use a supplied seed, a motion-seeded crop of surrounding points, bounded XY/yaw registration on
a declared locally planar road, a robust correspondence loss, and a bounded accumulated shape.
Hold seed dimensions as an uncertain prior for this demo; full visibility-aware dimension
learning is D-04 follow-on work. Keep vertical reference fixed or ground-conditioned and flag
grade violations.

Solve against the previous accepted surface and a bounded earlier shape cache. Do not align
only consecutive medoids. Reject ambiguous/low-overlap updates, coast with growing uncertainty,
and stop after a declared gap. One long planar side cannot fully constrain translation along
that side. The proposal path must show this uncertainty rather than inventing motion.

Update the cache only after the pose passes its quality gate. Proposed cap: 4,096 voxelled
points per object and a three-frame recent buffer, with configurable voxel size and
deterministic eviction. Retain contributing sample references so a bad update can be removed.
Keep sensor points and object-local points separate; test coordinate transforms and angle wrap
explicitly.

The reference SOTracker algorithm combines registration, accumulated shape, and motion priors
and uses an optimisation solver rather than a neural checkpoint. Its motion constraints and
sampling assumptions still need validation on turning, stationary-sensor data.
[Paper](https://arxiv.org/html/2103.06028v2)

The upstream implementation is an optional offline comparator. Time-box dependency, licence,
and custom-loader investigation to two hours within Day 2. Pin the source revision and record
any adaptations. If reuse terms or dependencies are unresolved, do not vendor or distribute it;
continue with the clearly named local baseline. Neither downloading Waymo data nor converting
our captures into a complete Waymo dataset is required by the documented custom-loader API.
[Upstream API](https://github.com/tusen-ai/LiDAR_SOT)

Trail samples carry capture time, physical reference point, body yaw, uncertainty/quality, and
observed versus predicted status. Draw gaps and rejected updates explicitly. The comparison
uses the same reference-point convention, or labels incompatible curves separately. No cosmetic
spline may count as tracking improvement. A retrospective view, if added later, must be
labelled separately from causal output and cannot read held-out annotations.

## 7. Three-day execution budget

One implementer, 24 engineering hours, plus the operator budget above. These are planning
allocations, not benchmarked implementation estimates. The minimum slice has no full paint
brush, server writes, production integration, or native upstream port.

| Day | Budget and work                                                                                                                         | Deliverable and gate                                                                                                 |
| --- | --------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| 1   | 2 h source/build readiness; 2 h frozen pack and sidecar; 4 h paused lasso, add/subtract, depth slab, save/reload                        | A person can label one object in several frames; exact membership round-trip and wrong-source rejection pass         |
| 2   | 2 h descriptors and JSON scorer; up to 2 h upstream feasibility; remaining 4 h or more on local seeded registration and diagnostics     | One track produces a candidate trail and bounded shape; ambiguous updates are visible; operator reviews corpus masks |
| 3   | 2 h synchronized baseline/candidate overlay and inspector; 3 h held-out evaluation; 2 h regression checks; 1 h demo notes and packaging | Reproducible one-site demo with all test objects, failures, intervention counts, and measured timings                |

If Day 1 misses its gate, Day 2 starts with fixing source identity/annotation, not tracker
tuning. Cut upstream execution first, then the longitudinal profile and proposal propagation.
The brush, multi-part templates, automatic face labels, and web parity are stretch work from
the outset. Do not cut exact mask persistence, split integrity, uncertainty flags, or held-out
evaluation.

### Implementation seams

- Export/validation: existing VRLOG reader and offline tools; reuse the active replay harness
  only after its completion/config parity is verified. Add synthetic point-bearing fixtures.
- Annotation: macOS `MetalRenderer`, point-cloud models, and labelling state; a local pack
  provider reuses the renderer without adding a second full viewer or mutating the live stream.
- Model/analysis: isolated offline package and fixture-backed feature calculator; no production
  L5 default change. Keep model fitting outside the held-out evaluator.
- Display: current trail renderer plus an imported candidate result layer keyed by sample
  identity. A toggle is sufficient; a second application or dashboard is not.

## 8. Acceptance scorecard

Thresholds below are proposed demo gates, fixed before opening test results. Failing a gate is
a reported result, not permission to relabel the test set or omit a difficult track.

| Area             | Gate or required report                                                                                                                                                 |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Source integrity | Exact mask round-trip; stale hashes, array mismatch, bad indices, and wrong coordinate contracts rejected                                                               |
| Human workflow   | Independent second-view review of every exported keyframe; unresolved membership is marked uncertain                                                                    |
| Selection UI     | Proposed p95 selection response below 200 ms on the named demo machine for a 100,000-point excerpt frame; otherwise lower the documented excerpt cap                    |
| Replay display   | At least 10 displayed frames/s from precomputed results on the demo machine; offline tracking wall time and peak memory reported separately                             |
| Point tracking   | Precision, recall, and IoU on reviewed keyframes, excluding uncertain points; report each object and range band                                                         |
| Demo improvement | Candidate median per-object point IoU at least 0.10 above the pinned baseline; if baseline is already above 0.90, require no regression plus better pose/trail evidence |
| Identity         | No unreported object switch; show losses, reseeds, and interventions, including the split-car regression                                                                |
| Pose/trail       | Position and axial-yaw errors only on sufficiently reviewed references; show per-object turn lag and gaps, not just aggregate jitter                                    |
| Class scoring    | Per-object confusion table and abstention rate for both oracle and predicted masks; no general accuracy claim from four test objects                                    |
| Shape            | Held-out observed-surface distance and view coverage; no complete-shape Chamfer score against a model fitted to the same frames                                         |
| Reproducibility  | Model/split/source/config hashes and a rerunnable export-to-report command sequence; identical selected memberships on rerun                                            |

Keep observed and predicted trail errors distinct. Bootstrap or uncertainty summaries, if
produced, resample whole objects, not adjacent frames. Include the entire test partition even
when a track fails early. No unlabelled region is scored as known background.

Define the baseline membership rule before scoring. Prefer retained cluster-to-point
associations; if unavailable, use a documented point-in-published-box proxy and call it that.
Apply the same recorded point domain and uncertain-point exclusions to both methods. Do not
present box containment as measured DBSCAN membership or compare candidate full-scene recall
against a foreground-only truth.

## 9. What follows the demo

Promote the annotation/source contract before expanding the algorithm. Then add depth-aware
brush selection, reviewed propagation, surface primitives, and a visibility-conditioned shape
likelihood. Extend the model from a cuboid to named body parts only when held-out surface
evidence supports it.

Reconcile D2.1's proposed running dimension mean with state-est's partial-extent treatment.
D2.2 must compare a fragment to expected visible support, not demand full-object dimensions in
every scan. D1.4 may update a displayed envelope but must not silently update physical shape
truth. Course alignment remains a diagnostic, not the objective that defines correct body
orientation.

The demo does not satisfy the multi-site corpus gate or prove real-time performance on a Pi.
Those remain separate decisions with separate measurements.
