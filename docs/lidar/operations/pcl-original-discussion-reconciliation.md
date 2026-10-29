# Original PCL discussion: reconciliation and one additional question

- **Status:** OQ-11 answered; OQ-13 added for merged-detection partitioning
- **Source:** [Compare PCL Features, owner-supplied shared discussion](https://chatgpt.com/share/6adff4d9-bf44-83e8-a2d9-9993e4c9c469)
- **Local baseline:** velocity.report `1762c9e13072657ee685b20f401592d63e20ec9b`, OQ-10 checkpoint
- **PCL Git SHA:** `3346843ea6b97697a1577293aa9cdf236b5c7ffb`, 1.15.1
- **Related:** [review](pcl-algorithm-review.md), [handover](pcl-astera-handover.md), [candidate plan](../../plans/lidar-pcl-algorithm-candidates-plan.md)

All thirteen review questions and the final branch pass are complete. Source findings and
validation below describe this report's recorded checkpoint; subsequent full-suite validation
and remaining implementation prerequisites are recorded in the
[current handover](pcl-astera-handover.md#resume-point). Closing a review question does not
complete or authorise its proposed campaign.

## What the discussion adds

The opening answer supplies the same five questions already used by the review: road/kerb
geometry, descriptors, footprints, static registration, and occlusion. It does not establish an
algorithmic gain. Its additional contribution is a measurement programme: independent references,
qualified uncertainty, opportunity denominators, layer ownership, and protection of held-out
sites. Most of that programme is already recorded in D-36, D-37, and the cohort-isolation plan.
Closing OQ-11 therefore does not restart the roadmap or turn PCL research into scheduled P0 work.

One specific suggestion needs a separate disposition: using richer geometry to distinguish two
objects temporarily merged by DBSCAN. The current PCL work covers joining fragments and scoring
whole-object descriptors, neither of which specifies how to partition one detection into two.
Record that as OQ-13, with the existing observation-revision plan as its owner context. No new
experiment, implementation, or release slot is added at this checkpoint.

## Source and reading boundary

The owner supplied the share URL during this run. Its title is **Compare PCL Features**. The
shared page exposed the earlier technical discussion in a browser after the thread reader had
returned only later PR-management messages. The web fetch failed with a cache miss; that failure
was not evidence that the discussion was absent. The temporary access-blocked draft was replaced
by this reconciliation before commit.

The relevant source passages, identified by visible headings and adjacent owner prompts, are:

| Source passage                          | Distinguishing request or content                                                                                                      | Reading in this checkpoint                                                                                                  |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Opening capability assessment           | Owner asks for a high-level PCL comparison while another agent performs the deep implementation review                                 | Capability hypotheses and the five questions; not a source-code audit                                                       |
| Strategic realignment answer            | Owner asks how to prioritise validated physical geometry, distances, and explicit uncertainty                                          | Six proposed epics, qualification states, external references, coverage and mobile transfer                                 |
| Roadmap/L7 follow-up                    | Owner asks whether L7 should hold this work and requests versioned releases, S-* workstreams, priorities, and a separate mobile series | Cross-layer validation, retained layer boundaries, L7/L8 responsibilities, pose dependencies                                |
| Accepted backlog changes                | Owner agrees with the points and requests priority-first/workstream-last backlog entries and a PR                                      | Historical adoption request, checked against current repository decisions; not a fresh instruction to rerun old Git actions |
| Labelling and regional rollout exchange | Owner asks about labels, held-out captures, published speeds, and the next three San Francisco S2 L10 regions                          | Separate reference truth, site/capture eligibility, independent references, and publication from development access         |
| Cohort enforcement exchange             | Owner asks what pipeline enforcement means and asks for an Ops / Pipeline item targeting v0.5.4                                        | A manifest/guard proposal and subsequent backlog work; estimates are not measured overhead                                  |

The L7 request is visible as turn `317e86c5-5462-4fea-82be-0f5b652f19b8`; its response is
`f3c3eb8c-6d9a-4e53-b9dc-df525fa4b02f`. The backlog-adoption request is
`05f888ea-7c87-4d35-b6a0-03cab6d8a759`; the detailed cohort response is
`c806417d-6516-4632-975a-e59f2347aa10`. These identify the retrieved passages; the shared page
is not a implementation source identified by a Git SHA. Unrelated conversation and historical PR-management
instructions are excluded from the engineering record. No raw conversation dump is committed.

## Claim-level reconciliation

| ID     | Discussion claim or proposal                                                                  | Evidence and disposition                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ------ | --------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C11-01 | Robust ground/surface estimation can improve safety measurements                              | Already Q0 and ground0006. [OQ-01](pcl-astera-handover.md#review-record) and [OQ-03](pcl-segmentation-contracts.md) show that orientation constraints, normals, and morphology do not establish road identity. Keep held-back surface validation and separate downstream metric evaluation                                                                                                                                                                                                                                                                       |
| C11-02 | Normals and shape descriptors can help classification and fragmentation                       | Classification maps to [OQ-10](pcl-classification-corpus-requirements.md); joining fragments maps to [OQ-05](pcl-joining-cap-controls.md). The additional merged-object suggestion is OQ-13 below. The thread's suggested priority does not bypass the Parked scorecard/corpus dependencies                                                                                                                                                                                                                                                                      |
| C11-03 | Per-track hulls improve physical footprint/distance estimates                                 | Already Q2 and [geom0013](../../../data/experiments/geom/geom0013.md). A hull bounds observed support; unseen endpoints remain an inference. [OQ-06](pcl-planar-registration-disposition.md) retains censored extent estimation and correlated-history limits                                                                                                                                                                                                                                                                                                    |
| C11-04 | Static landmarks and registration improve repeat surveys                                      | Already Q3 and [OQ-08](pcl-gerardus-registration-gap-review.md). Gerardus already has observability diagnostics; only the conditional initial-proposal comparison remains new. [OQ-09](pcl-background-cleanup-disposition.md) rejects blanket cleanup without fixed evaluation support                                                                                                                                                                                                                                                                           |
| C11-05 | Use range images and octree changes to reason about hidden versus departed objects            | Already Q4; a causal conclusion would exceed what these operators return. At the Git SHA, [new-voxel extraction](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/octree/include/pcl/octree/octree_pointcloud_changedetector.h#L87-L107) returns point indices from new leaf containers, subject to a count threshold. It has no track identity or explanation for missing returns. [OQ-02](pcl-organised-grid-compatibility.md) also blocks direct native-grid use; OQ-12 records the completed fate-label/tap comparison |
| C11-06 | PCL should be an offline comparator, without replacing the Kalman tracker or native viewer    | Already the candidate plan and [OQ-04 runbook](pcl-oracle-tooling.md), under D-26/D-30. Existing diagnostics are not complete campaign harnesses or runtime ports                                                                                                                                                                                                                                                                                                                                                                                                |
| C11-07 | Should physical validation belong in L7 as static mapping moves to Gerardus?                  | The owner posed a question; the response rejected putting the whole programme in L7. [D-36](../../DECISIONS.md#d-36--physical-measurement-qualification-and-gerardusl7-boundary) and [L7 revised ownership](../../plans/lidar-l7-scene-plan.md#01-revised-05x-ownership-and-delivery-d-36) already put body state in L5, relationships in L7, and measurement/validity in L8. Lower layers use injected neutral queries, not imports from L7                                                                                                                     |
| C11-08 | Independent truth, uncertainty calibration, suppression and coverage should govern promotion  | Already the [backlog promotion rollups](../../BACKLOG.md), [physical-reference plan](../../plans/lidar-physical-reference-review-plan.md), and [state-estimation plan](../../plans/lidar-state-estimation-plan.md). Return-fitted reference bounds are not external absolute truth; covariance is not automatically calibrated coverage. These are requirements, not passed gates                                                                                                                                                                                |
| C11-09 | Pose/frame contracts and accepted-pose writeback must precede world-relative claims           | Existing backlog rows and L7-A own both. [L7 current status](../../plans/lidar-l7-scene-plan.md#0-where-implementation-stands-october-25-2026) explicitly records the writeback gap. Preserve shared registration/calibration error in relative measurements; do not infer deployment accuracy from a successful PCL fit                                                                                                                                                                                                                                         |
| C11-10 | Reorganise 05x with priorities/workstreams and reserve 06x for mobile                         | Already D-36/D-37 and the backlog. The thread's old release counts, proposed dates, and intermediate PR state are historical, not current scheduling authority. No backlog migration is repeated here                                                                                                                                                                                                                                                                                                                                                            |
| C11-11 | Freeze whole sites/captures, protect independent labels, and keep historical published speeds | Already the [cohort-isolation plan](../../plans/lidar-qualification-cohort-isolation-plan.md). Canonical site identity is not removed to hide a test set. Production processing/publication is distinct from development diagnostic ingestion; publication does not make predictions reference truth                                                                                                                                                                                                                                                             |
| C11-12 | Existing manifests make cohort enforcement incremental and inexpensive                        | The cited [annotation splits](../../../internal/lidar/annotation/split.go), [capture manifests](../../../internal/lidar/jobs/manifest.go), and [dataset records](../../../internal/lidar/dataset/dataset.go) exist, but the cohort plan remains Proposed. A manifest is neither a shared eligibility guard nor storage access control. The 4–7-day estimate and negligible-runtime expectation have not been benchmarked here                                                                                                                                    |

This table checks the discussion against current contracts, not every implementation behind the
roadmap. It does not claim those proposed integrations have shipped. The reviewed octree method was
read directly; no new PCL executable or campaign was needed for this reconciliation.

## Counterexamples that bound the discussion

- **Two objects, one descriptor:** one ESF vector for a merged car/cyclist cluster does not assign
  its returns to two independent objects. A classifier label cannot substitute for a point
  partition or establish which track owns each return.
- **Absent does not mean departed:** a previously occupied voxel can lose returns because of an
  occluder, sampling, a pose change, or actual departure. Occupancy differencing alone cannot
  choose between them, and a new occupied voxel does not identify the returning track.
- **More precise does not mean less biased:** repeated returns from one face can shrink a fitted
  covariance while leaving the hidden bumper wrong. A smoother trajectory or lower registration
  residual does not supply independent physical truth.
- **A holdout label is not enforced isolation:** assigning a qualification role in a file cannot
  stop an unguarded exporter from loading it or a developer from inspecting residuals. The
  cohort plan requires both loader checks and separate access to protected reference feedback.
- **No label leak does not guarantee blindness:** inspecting public or developer-facing outputs
  from a qualification site and tuning in response can compromise the next claim even if sealed
  labels were never opened. Record feedback use and retire exposed cohorts as specified by the
  existing plan; do not promise perpetual blindness from a manifest alone.

## OQ-13: partitioning merged detections

Ask whether PCL local geometry or descriptors supply a useful, bounded partition proposal for two
physical objects merged into one L4 detection, distinct from fragment joining, class prediction,
and duplicate-track deletion. The discussion motivates asking this question; it does not answer it.

The [observation-revision plan](../../plans/lidar-cluster-observation-log-and-async-tracking-plan.md#association-within-the-window)
already considers cluster splits and requires exclusive point ownership. Start there, and with
its [no-double-counting contract](../../plans/lidar-cluster-observation-log-and-async-tracking-plan.md#separate-guarantees-from-measured-accuracy),
the W3 near-merge cases, and the existing PCL clustering contracts. Do not create a second tracker
or give a merged detection's full evidence to both tracks.

A later answer must either map a precise proposal into the existing owner with reviewed
multi-object membership, false-split controls, identity and resource gates, or drop the PCL arm
with a source-based reason. Two nearby objects versus one partially visible articulated/long body
is the critical counterexample. Whole-cluster ESF classification alone does not meet the contract.
This checkpoint adds the question only; it does not select an algorithm or edit l4clus0005 into a
splitting experiment.

**Subsequent disposition:** [OQ-13](pcl-merged-detection-partition-disposition.md) answered the
question by dropping a standalone PCL splitting arm. The reviewed graph/descriptor contracts do
not supply instance identity; MinCut's active unary costs ignore background-point seeds.
Point-level repair remains with the existing observation-revision owner, subject to the report's
membership, false-split, identity, and resource conditions. This later answer preserves the
original discussion's question and adds no experiment.

## Consequences for the current plan

The four proposed experiments retain their priorities. The merged-detection question was added
and answered as OQ-13, and the offline runbook now states the cohort dependency: PCL exports and oracle
inspection are development use, even though PCL is outside the product. While the guard is not
implemented, a diagnostic must name its known development input and cannot claim enforced
qualification isolation. A future campaign must record the cohort decision and provenance or
remain explicitly outside a qualification claim. The guard's implementation stays with its
existing P0 owner; no such guard was added in this documentation checkpoint.

OQ-11 is complete. OQ-12 subsequently [resolved the return-fate and border-label contract](pcl-return-fate-border-contract.md), and OQ-13 is answered above. The cohort plan still owns exact
region assignments, independent case counts, reference instruments, custodians, and numerical
qualification thresholds; OQ-10's classification screening counts do not settle those broader
measurement decisions.
