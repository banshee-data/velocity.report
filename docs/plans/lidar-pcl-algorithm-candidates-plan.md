# PCL algorithm candidates: experiments before ports (05x research, Parked P2)

This plan assesses Point Cloud Library candidates for geometry, descriptors, and tracking against the existing pipeline and physical-reference criteria.

- **Status:** Review and final branch pass complete; implementation proposed. Four experiment specs; diagnostic tools built, campaign oracles and runtime ports pending; nothing scheduled into a full 05x slot
- **Decisions:** D-36 (Gerardus owns static geometry; L5 owns bodies; L8 owns interaction metrics), D-37 (backlog metadata), D-32 (slot caps), D-26 and D-30 (one static Go binary: no C++ runtime dependency), D-28 (rectangle fit and range-adaptive eps context)
- **Layers:** L3 grid (border classes), L4 perception (joining rule, ground plane), L5 tracks (coasting evidence, extents), offline evaluation; Gerardus recommendations outside this repository
- **Target:** the experiments run alongside 05x as P2 research; a port enters a 05x slot only by displacing a row there, after its experiment passes
- **Canonical:** [PCL algorithm review](../lidar/operations/pcl-algorithm-review.md)
- **Handover:** [Astera open-question review](../lidar/operations/pcl-astera-handover.md)
- **Related:** [geometry convergence](lidar-tracker-geometry-convergence-plan.md), [facet registration](lidar-facet-registration-experiment-plan.md), [shape descriptors](lidar-shape-descriptors-plan.md), [static scene local fit](lidar-static-scene-local-fit-plan.md), [lane stitching](lidar-lane-stitching-plan.md), [L7 scene](lidar-l7-scene-plan.md)

## Motivation

The owner asked whether the Point Cloud Library can improve vehicle state tracking, facet and
keypoint segmentation, and surface geometry, framed by five questions: road and kerb geometry for
safety metrics, descriptors against classification error and fragmentation, hulls against boxes
for clearance and following distance, measured static-scene geometry for repeat surveys and
multi-vantage registration, and occlusion-aware analysis against track loss. The review answers
each from the reviewed source. This plan turns its shortlist into experiments with rules written
now, so that the answers arrive as measurements and a port is built only where one pays.

[OQ-11](../lidar/operations/pcl-original-discussion-reconciliation.md) reconciles the original
conversation with current decisions. [OQ-13](../lidar/operations/pcl-merged-detection-partition-disposition.md)
drops its additional standalone PCL splitting arm: geometric subsets are not physical instances,
and the reviewed MinCut does not use background-point seeds in its active cost. The existing
observation-revision plan retains bounded point repair and the conditions for a later proposal.
No splitting arm is added to l4clus0005 or scheduled as a new experiment.

Without it, PCL ideas would enter the backlog as unmeasured ports competing with P0 qualification
work, which is what D-36 and the 05x rubric exist to prevent.

## Current state

- No PCL product dependency exists. The offline `tools/pcl-review` diagnostics use an external
  reviewed source build; `ARCHITECTURE` cites PCL's VoxelGrid and Euclidean clustering as lineage.
- L2 keeps each return's channel and azimuth and L3's grid is rings by azimuth bins, but L4 receives
  a flat point list; no stage computes normals, curvature or border classes.
- Exports: settled backgrounds as ASC (`cmd/tools/bgasc`), frames as ASC (`l2frames/export.go`).
  OQ-02 added a diagnostic PCD frame exporter; the general exporter and background PCD option
  remain open. ASC carries no organised layout or normals.
- The evaluators these experiments need exist: `lidar-ground-truth-eval` (W3 retention and
  fragmentation, per-frame HOTA and IDF1, physical references), `lidar-box-eval`, `lidar-near-face-eval`.
  l5id0007's link measure is proposed, not built.
- Every 05x slot from v0.5.3 to v0.5.9 is at or near the twelve-row cap.

## Findings

| Question | Answer from the review                                                                                            | Candidate experiment                                      | Workstream, gate      | Serves P0 rows in                          |
| -------- | ----------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | --------------------- | ------------------------------------------ |
| Q0       | A constrained robust plane and a kerb split can tighten the runtime surface on graded sites; kerbs are Gerardus's | [ground0006](../../data/experiments/ground/ground0006.md) | `S-OBS`, G-OBS        | v0.5.4 ground rows                         |
| Q1       | Fragmentation needs a joining rule, not a descriptor; descriptors wait on a class scorecard and labelled corpus   | [l4clus0005](../../data/experiments/l4clus/l4clus0005.md) | `S-OBS`, G-OBS        | v0.5.4 W3 fix                              |
| Q2       | Measure the visible-support endpoint gain over the containing box before building                                 | [geom0013](../../data/experiments/geom/geom0013.md)       | `S-GEO`, G-GEO, G-MET | v0.5.5 W2, W4; v0.5.8 endpoints            |
| Q3       | Gerardus's question; PCL offers ingredients for geometry diagnostics and alternative pose seeds                   | Recommendations to Gerardus                               | `S-SCENE`             | pose0001's follow-up, in Gerardus          |
| Q4       | Border traits are candidate evidence for extent ends, coasting and return filtering                               | [l3bg0028](../../data/experiments/l3bg/l3bg0028.md)       | `S-GEO`, G-TEMP       | v0.5.6 occlusion continuity, reacquisition |

## Design / approach

**PCL is an oracle, never a dependency.** The experiments target PCL 1.15.1 offline on the Mac,
using a verified installation or reviewed source build against exported frames and backgrounds.
The [OQ-04 runbook](../lidar/operations/pcl-oracle-tooling.md) records package versions, the verified
source build, and executable coverage. Each report records the build actually used. Raw outputs stay in a local `results/` directory per AGENTS.md. A port
to Go happens only after a passing experiment, behind a named replay experiment and then a tuning
key, with the oracle kept as its independent test reference. This is the discipline the
SpatialRust review set: the reference must not call the candidate's own neighbour search.

**One exporter, two formats.** A small Go writer emits PCD v0.7 with the organised layout (width
equal to azimuth bins, height to rings, NaN for no return) and each return's source ordinal as a
field, so an oracle's labels join back to the pipeline's returns by ordinal, never by index order.
The same writer serves all four experiments. Normals for a Gerardus upload require a separately
validated estimator; changing the file format does not compute them. OQ-02 must also specify the
mapping from the sensor grid to PCL RangeImage, including resampling and source identity.

**Ports stay inside their layer.** Border classes and normals belong to L3 (they are properties of
the grid); the joining rule to L4; coasting evidence is read by L5 from an L3 product passed
through `pipeline/`. L4 and L5 never import L7 (D-36); kerbs reach L5 and L8 only through the L7
adapter.

**Organised normals are the shared primitive.** If two experiments pass that need normals (the
kerb split and a normal-conditioned join), port `IntegralImageNormalEstimation`'s covariance
method once, on ring and azimuth neighbours with Cartesian XYZ preserved. OQ-02 must establish the
depth-discontinuity and neighbourhood adaptation for LiDAR rays; replacing z with range is not
valid covariance input.

## Scope

### Item 1: PCD export and the oracle at the recorded Git SHA

**Summary:** An organised PCD writer for frames and settled backgrounds, with source ordinals, and
a documented oracle setup.

**Steps:**

1. Writer in `l2frames` beside the ASC export, with a round-trip test against a fixture read by a
   minimal parser.
2. `bgasc` gains a `-format pcd` option; the replay tools gain a frame-window export.
3. Use the [OQ-04 runbook](../lidar/operations/pcl-oracle-tooling.md): PCL 1.15.1 at
   `3346843ea6b97697a1577293aa9cdf236b5c7ffb`, external Release source build with Eigen 3.4.1.
   Homebrew lists 1.15.1 revision 8; Debian trixie/bookworm list 1.15.0/1.13.0, respectively.
   Those packages are not the recorded source installation.
4. The existing `pcl-grid-check` and `pcl-segmentation-check` are diagnostics. Implement the
   runbook's named campaign contracts: `pcl-border-oracle` for l3bg0028, `pcl-join-oracle` for
   l4clus0005, `pcl-hull-oracle` for geom0013, and `pcl-ground-oracle` for the constrained fit,
   two-scale normals, DoN, and lines. These names are planned targets, not available commands.
   Stock `pcl_*` tools do not satisfy the complete contracts. Raw outputs stay outside Git.

**Milestone:** first step of ground0006 and l3bg0028; `S`.

OQ-02's [one-frame check](../lidar/operations/pcl-organised-grid-compatibility.md) rejects direct
native-grid use: normal validity changes under coordinate rotation and PCL's base RangeImage
projection does not match the assigned cells. Item 1 remains open for a calibrated adapter or
explicit spherical resampling with source membership; the diagnostic exporter is not its completion.

### Item 2: Q4, border classes on the L3 grid (l3bg0028)

**Summary:** Directional obstacle, shadow and veil traits per cell, evaluated separately against
reviewed extent ends and predicted-body visibility, replay-only.

[OQ-12](../lidar/operations/pcl-return-fate-border-contract.md) reuses `replayeval.FrameTap` for
offline export and diagnostics. Lower-body fates do not supply border labels or a coasting
population. The publication observer runs too late for same-frame interventions: veil withholding
belongs before L4 input selection, and coast evidence before the lifecycle decision. Preserve
unknown coverage and source membership; do not mutate the tap or treat veil as mixed-return truth.

**Steps:**

1. Oracle: PCL's `RangeImageBorderExtractor` on exported kirk0 frames; the Go replay pass compared
   cell by cell.
2. Replay experiment `border_classes`: veil returns withheld from L4; each coasting track's predicted
   footprint read for shadowed against seen-through cells, recorded beside the existing support
   tokens without changing a decision.
3. A second arm lets the evidence end or extend coasting under the v0.5.6 options.

**Milestone:** reports before the v0.5.6 occlusion continuity acceptance row chooses values; `M`.

### Item 3: Q1, the joining rule (l4clus0005)

**Summary:** Whether kirk0's 20 to 50 m fragments are joined by a radius graph, and by one with a
vertical-overlap condition, at the current radius.

**Steps:**

1. Oracle: freeze one baseline's post-voxel pre-cap set and its actual capped subset. Compare
   DBSCAN, PCL EuclideanClusterExtraction, and ConditionalEuclideanClustering on both sets by
   source ordinal. [OQ-05](../lidar/operations/pcl-joining-cap-controls.md) specifies six arms,
   the fixed fragmented-instant denominator, and a per-treatment one-third stop gate.
2. Reuse `config/experiments/l4-cap100k.json` for all three high-cap arms, verifying it never
   caps warm-up or scoring input. Keep the three default-cap arms and count exposure in both
   phases. No replay selector is implemented by this design amendment.
3. Only eligible graph/cap combinations proceed to replay, with both DBSCAN controls. Every
   candidate meets the original thresholds against a0; uncapped graphs additionally meet them
   against nocap DBSCAN. Treat end-to-end feedback separately from frozen-input connectivity.
   A negative result rules out only these fixed-radius treatments on this input.

**Milestone:** reports into the v0.5.4 W3 fix row; `M`.

### Item 4: Q2, the hull bound (geom0013)

**Summary:** How far the published box's faces sit from the visible hull's extremes, along and
across the path, on the pilot's reviewed poses and pairs.

**Steps:**

1. Visible XY convex hull of each observation's members (PCL oracle and the Go hull already
   computed by `MinAreaRectangle`), checked against direct directional extrema. Alpha hulls
   are optional support-selection diagnostics; concavity alone cannot change an extreme.
2. Offsets binned by aspect and range; the gap each pilot pair would read from hull extremes.
3. Stop if the offset is small against the extent error; otherwise propose a hull endpoint source
   to the behaviour plan for passing clearance (v0.7.10).

**Milestone:** reports before v0.5.8's endpoint-source row is promoted; `S`.

[OQ-06](../lidar/operations/pcl-planar-registration-disposition.md) keeps accumulated alignment
as one bounded comparator within facet arm B. It does not extend geom0013's single-observation
hull bound or replace W4's censored extent estimator; no separate registration spec is added.

### Item 5: Q0, constrained ground and kerbs (ground0006)

**Summary:** A perpendicular-plane MSAC fit and a difference-of-normals kerb split on settled
backgrounds, against `RegionalGroundSurface`, on the graded sites.

**Steps:**

1. Oracle on the 23 first-segment backgrounds; residual per region against the current fit.
2. Kerb candidates as lines, offered to Gerardus for comparison with its kerbs.
3. If the surface error near kerbs falls, a replay estimator in `l3grid` behind the slope-aware
   ground block v0.5.4 adds.

[OQ-03's contract check](../lidar/operations/pcl-segmentation-contracts.md) confirms that the
oracle must use PCL's returned, reselected plane membership and keep held-back points separate.
Plain RegionGrowing has no default distance bound, and its residual test controls propagation
rather than membership. PMF opens correctly but does not establish road identity. Neither is
added as an extra arm; the 23-site comparison remains unrun.

**Milestone:** reports into the v0.5.4 slope-aware ground rows; `M`.

### Item 6: Q3, recommendations to Gerardus

**Summary:** [OQ-08](../lidar/operations/pcl-gerardus-registration-gap-review.md) checked Gerardus
main `a2c2ac10`. Matched-geometry eigenvalues, rank, weak directions, rank gating, and formal
covariance already exist. Do not request a new generic observability report. The remaining new
proposal is one offline initial-proposal comparator: FPFH/prerejective matching or KFPCS against
the current raster search. Planned mobile Scan Context is a different task.

**Steps:**

1. Gerardus owns selection and scheduling of that conditional comparator. Freeze scan/reference
   revisions, preprocessing, coverage and compute budget, distinct-basin handling, and the same
   ICP/refusal gates before scoring. Preserve pose0001's acceptance discipline and use independent
   pose evidence for correctness claims. The prepared recommendation is in the OQ-08 report;
   no external issue or campaign has been opened by this review.
2. Use the existing diagnostics when assessing the comparator. Local rank does not resolve global
   aliases; the distinct-alternative concern is already known to Gerardus. Do not change its
   acceptance thresholds on the strength of extra proposals.
3. [OQ-09](../lidar/operations/pcl-background-cleanup-disposition.md) drops blanket MLS/SOR
   export cleanup: there is no measured benefit beyond the guard, and deleting or moving points
   changes the evaluation population. Keep the guarded export. Reopen only with reviewed
   contamination and a frozen original evaluation set, preserving confidence and source identity;
   a comparison of each filtered file's own residual gates is insufficient. This adds no arm or
   tuning set. l3bg0028's border-label question and normal estimation retain their own contracts,
   including OQ-02's adapter constraint. Change-map attribution remains separate.

**Milestone:** unscheduled; Gerardus sets the order. This source-audit handoff is complete;
implementation, campaign design, and promotion remain unapproved here.

### Item 7: Q1, descriptors, deferred

**Summary:** ESF and moment-of-inertia descriptors as offline comparators to the shape-descriptor
plan's scalars, once a class scorecard and a labelled corpus exist.

**Milestone:** with the Parked classification scorecard and corpus rows. The
[OQ-10 dependency contract](../lidar/operations/pcl-classification-corpus-requirements.md) requires
100 independent held-out objects per target class/contrast and 30 per claimed class × aspect ×
range cell, with site-disjoint evaluation, unknown/invalid coverage, and a common downstream
classifier. These are screening floors, not promotion gates or training-set sizes. The current
local kirk0 labels lack cyclists, motorcyclists, and reviewed long-car/aspect coverage; no
comparison is scheduled by this contract.

## Dependencies

- label0001 or a frozen held-out window, for any promotion; every experiment here reads kirk0's
  tuning split and is a screen.
- Live cluster point retention (v0.5.4) for any runtime port that reads members.
- The qualification cohort manifest (v0.5.4): exports for oracles are development use and go through
  its eligibility guard once it exists.
- Gerardus for Item 6 and for kerbs.

## Risks

| Risk                                                                         | Likelihood | Impact | Mitigation                                                                       |
| ---------------------------------------------------------------------------- | ---------- | ------ | -------------------------------------------------------------------------------- |
| Oracle results read as references                                            | Medium     | High   | Reports name the oracle as an independent algorithm; truth is the reviewed packs |
| Camera-frame assumptions in PCL's organised code mislead on a spinning LiDAR | High       | Medium | Range as depth, ring and azimuth as the grid, non-uniform elevations stated      |
| Tuning on kirk0 becomes the result                                           | Medium     | High   | Rules written now; held-out window before any default change                     |
| A port competes with P0 work                                                 | Medium     | Medium | Ports enter a slot only by displacing a row, after a pass                        |
| Border classes cost more than estimated                                      | Low        | Medium | Full-callback p95 and p99 recorded in every replay arm                           |

## Checklist

### Complete

- [x] Source review of PCL 1.15.1 against the five questions: [review](../lidar/operations/pcl-algorithm-review.md)
- [x] Specs with rules: ground0006, l4clus0005, geom0013, l3bg0028

- [x] OQ-01 to OQ-13: source review, compatibility, contracts, tooling, sampling controls, registration/duplicate-track dispositions, Gerardus novelty audit, cleanup disposition, classification corpus requirements, original-discussion reconciliation, return-fate/border contracts, and merged-detection partition disposition

### Outstanding

- [ ] Organised PCD export and the oracle at the recorded Git SHA (`S`)
- [ ] l3bg0028 border classes (`M`)
- [ ] l4clus0005 joining rule (`M`)
- [ ] geom0013 hull bound (`S`)
- [ ] ground0006 constrained ground and kerbs (`M`)
- [ ] Gerardus initial-proposal comparator: owner scheduling and matched campaign design (`S` on this side)

All review rows are answered. These remaining items are proposed implementation and evaluation
work; closure of a source question does not pass their acceptance gates.

### Deferred

- [ ] Descriptor comparison (ESF, moment of inertia): waits on the Parked classification scorecard and corpus
- [ ] Organised normals port: only when two passing experiments need normals
- [ ] Planar registration comparator within facet arm B: OQ-06 contract agreed for the proposed experiment; implementation waits on its gates

### Accepted residuals (no action planned)

- [ ] PCL's particle filter, dense descriptors, corner keypoints, meshing and GPU modules: set aside in the review, with reasons

- [x] Greedy/Papazov standalone duplicate-track arm: [OQ-07 disposition](../lidar/operations/pcl-hypothesis-verification-disposition.md) records the missing visibility/temporal contract and counterexamples. Existing l5id0006, l5id0011, and l3bg0028 retain their own questions; no retrospective arm is added to l5id0004/0005.

- [x] Blanket MLS/SOR registration-export cleanup: [OQ-09 disposition](../lidar/operations/pcl-background-cleanup-disposition.md) records the missing benefit evidence, sparse-support counterexamples, and fixed-evaluation conditions for reconsideration.

- [x] Standalone PCL merged-detection splitting arm: [OQ-13 disposition](../lidar/operations/pcl-merged-detection-partition-disposition.md) keeps point repair with the existing observation-revision owner; reconsider only with specified seeds/predicates, complete membership, positive merges, false-split controls, and bounded identity/resource gates.
