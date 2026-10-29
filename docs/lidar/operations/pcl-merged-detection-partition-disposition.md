# Merged detections: no standalone PCL splitting arm

- **Status:** OQ-13 answered; standalone PCL partition arm dropped
- **Local baseline:** velocity.report `d914a4c26d0bb04a9c2cffd44b0999de252e3207`
- **PCL Git SHA:** `3346843ea6b97697a1577293aa9cdf236b5c7ffb`, 1.15.1
- **Related:** [review](pcl-algorithm-review.md), [original discussion](pcl-original-discussion-reconciliation.md#oq-13-partitioning-merged-detections), [observation-revision owner](../../plans/lidar-cluster-observation-log-and-async-tracking-plan.md#association-within-the-window), [joining experiment](../../../data/experiments/l4clus/l4clus0005.md)

All thirteen review questions and the final branch pass are complete. Source findings and
validation below describe this report's recorded checkpoint; subsequent full-suite validation
and remaining implementation prerequisites are recorded in the
[current handover](pcl-astera-handover.md#resume-point). Closing a review question does not
complete or authorise its proposed campaign.

## Decision

Do not add a standalone PCL experiment for splitting one merged L4 detection into physical
objects. ESF produces a whole-cloud signature. Radius and normal-based segmentation can propose
subsets, but their connectivity rules do not establish how many objects are present or which
track owns each return. The closest additional candidate, `MinCutSegmentation`, supplies an
object-versus-rest cut with a foreground-distance prior; at this Git SHA its background-point setter
does not affect the active unary calculation. It is not a two-track seed contract.

This is a scope decision, not proof that PCL can never contribute a useful proposal. A conditional
predicate or externally seeded cut could help on a discriminating case, but the predicate, seed
provenance, rejection rule, and temporal validation would carry the new hypothesis. None is
defined or supported by the original descriptor suggestion. Keep point-level repair with the
existing bounded observation-revision owner, where alternatives, uncertainty, exclusive ownership,
and later disambiguating observations already belong. Preserve l4clus0005's joining experiment.

The historical W3 kirk0 baseline has zero measured reviewed merges and did not measure near-merge
distances. It provides a false-merge control, not positive evidence that a splitter can repair a
merge. That statement is limited to its recorded run and sampled membership; it is not a fresh
inventory of all captures or an assertion that kirk0 can never contain a merge.

## Claim-level source evidence

PCL links are immutable at the Git SHA above. Local ranges refer to the stated baseline. These are
source findings and prior-report interpretation; no new segmentation execution occurred.

| ID     | Claim tested                                                                | Verdict and evidence                                                                                                                                                                                                                                                                                                                                                                 | Disposition                                                                                                                                                                                              |
| ------ | --------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C13-01 | Whole-cluster ESF supplies per-object membership                            | Rejected. [computeFeature, lines 534–549][esf] scales the supplied surface, computes a histogram, and emits one signature                                                                                                                                                                                                                                                            | Retain OQ-10 classification scope. A later classifier cannot recover a point partition merely by assigning that signature a class                                                                        |
| C13-02 | Euclidean extraction can split an unchanged connected bridge                | Rejected for unchanged input, metric, and radius. [extractEuclideanClusters, lines 62–115][euclidean] grows the reachable component and then applies size limits                                                                                                                                                                                                                     | A smaller radius changes the hypothesis and can fragment one body. Rejecting an oversized component does not split it into valid children                                                                |
| C13-03 | Conditional clustering already provides the required split rule             | Not established. [segment, lines 79–138][conditional] accepts caller-defined pairwise conditions, grows transitive components, and routes components by size                                                                                                                                                                                                                         | A condition can remove cross-object edges, but it must be specified and tested against within-object edges. l4clus0005's symmetric height condition remains a joining comparison, not an instance oracle |
| C13-04 | Normal regions are physical objects                                         | Rejected. [findPointNeighbours, lines 324–348][rg-neighbours] stores K-nearest neighbours without a spatial-distance gate. [validatePoint, lines 461–512][rg-validation] checks normal agreement; curvature/residual failures suppress propagation rather than membership                                                                                                            | Coplanar vehicles can join; one vehicle's differently oriented faces can split. OQ-03's earlier fixtures establish the distance/seed distinction, not traffic-instance accuracy                          |
| C13-05 | MinCut honours foreground and background seeds as opposing object evidence  | Corrected. [setBackgroundPoints, lines 188–195][mincut-setter] stores the points, but [calculateUnaryPotential, lines 348–399][mincut-unary] returns before the commented-out background calculation                                                                                                                                                                                 | Do not feed track B to this setter and claim that B constrained a cut around A. Reopening needs an implemented and verified seed contract                                                                |
| C13-06 | MinCut uses one consistent 3-D object-distance model                        | Corrected. [setRadius, lines 100–106][mincut-radius] stores the squared radius. The active [unary calculation][mincut-unary] uses XY distance only and constant source weight; [binary potential, lines 430–440][mincut-binary] uses squared XYZ distance scaled by inverse sigma squared                                                                                            | Record coordinates and all priors explicitly. Changing only seed z cannot alter unary costs. The radius is not a maximum graph-edge distance                                                             |
| C13-07 | Two cut outputs identify two physical objects                               | Rejected. [extract, lines 255–263][mincut-flow] calls max-flow and assembles labels; [assembleLabels, lines 515–538][mincut-labels] creates two index lists from source-edge residuals                                                                                                                                                                                               | One invocation assigns selected point indices to one of two lists, but the rest can contain several objects or noise. Separate invocations do not enforce joint ownership or track identity              |
| C13-08 | A small selected subset guarantees bounded MinCut memory and latency        | Rejected as a resource guarantee. [buildGraph, lines 284–343][mincut-graph] allocates vertices and edge markers from the full input-cloud size, then connects selected points through K-nearest search; [extract][mincut-flow] invokes max-flow without a wall-time budget                                                                                                           | Any future prototype needs compact, ordinal-mapped input, explicit point/edge/candidate bounds, and measured time/memory with a safe abstention path. No Pi cost is inferred here                        |
| C13-09 | The existing owner can accept a repaired merge without duplicating evidence | Contract confirmed, implementation not certified here. [observation-revision plan](../../plans/lidar-cluster-observation-log-and-async-tracking-plan.md), lines 267–274 and 303–319, requires exclusive measurement ownership, bounded alternatives, and an unchanged observation record. Lines 397–406 distinguish complete rich evidence from summaries and sampled/reduced points | Keep any later repair inside that owner, with an explicit residual/unassigned set and source lineage. Do not give the whole parent observation to both child tracks                                      |
| C13-10 | W3's near-merge requirement supplies positive splitter cases                | Rejected for the cited evidence. [W3 report](w3-retention-fragmentation-kirk0-2026-10.md), lines 27–30, records no measured merges; lines 45–54 document sampling and omitted instants, and lines 120–122 say near-merge distances were not measured                                                                                                                                 | Require independently reviewed positive merges and single-body false-split controls before a benefit claim. Zero positive cases means repair recall is not evaluable                                     |
| C13-11 | Existing merge/split flags already perform point-level repair               | Rejected for the cited path. [Tracker.Update](../../../internal/lidar/l5tracks/tracking.go), lines 564–597, sets flags from associated-cluster versus historical area and then updates the solid body. [WorldCluster](../../../internal/lidar/l4perception/types.go), lines 97–108, distinguishes sampled retained points from opt-in complete members                               | Flags can inform eligibility and extent safeguards, but this path does not emit exclusive child memberships. Do not treat a candidate flag or retained sample as repaired evidence                       |

For C13-06, the active unary capacities are `source_weight` and `d_xy / R`, with `d_xy` the
distance to the nearest supplied foreground point and `R` the configured radius. The pairwise
capacity is `exp(-d_xyz² / sigma²)`. These are source-derived expressions, not fitted parameters
or a proposed experiment configuration. Foreground points define soft spatial costs rather than
a general hard, mutually exclusive multi-object labelling contract.

## Counterexamples

These are analytical constructions, not measured traffic or new compiled fixtures.

1. **A locally valid bridge:** returns at x = 0, 0.6, and 1.2 m, with equal y and z, form one
   component at radius 0.8 m. All pairwise height conditions on the chain pass. Labelling the
   endpoints as different vehicles does not alter the graph. Removing the connector changes
   sampling; it does not validate a new identity rule.
2. **One long body versus two nearby bodies:** an occluded articulated vehicle and two aligned
   vehicles can present the same visible patches in one frame. Identical observed input provides
   no evidence for choosing between those physical accounts, even if a method samples alternative
   partitions. More normal or descriptor dimensions do not supply missing returns.
3. **An ignored opposing seed:** keep the cloud, foreground points, and parameters fixed, and move
   only the background seed from one vehicle to another. The active unary function has identical
   inputs and costs. The setter's name does not make either vehicle excluded evidence.
4. **Vertical ambiguity:** two points with the same XY location and different z have equal unary
   costs relative to a given foreground seed. Their pairwise costs may differ, but the unary
   prior itself does not assign them to different vertically separated objects.
5. **Object versus rest is not A versus B:** a cut around A can leave B, road leakage, and noise in
   the rest. Repeating a foreground extraction around B may overlap A's extraction. Assigning
   each entire output to a track double-counts shared source returns unless a joint ownership
   rule rejects or partitions them. Two output slots also do not guarantee two non-empty bodies.
6. **A favourable denominator:** evaluating only known two-body merges rewards always splitting
   into two. Evaluating only W3's zero-merge baseline cannot measure repair recall at all. A
   partial cluster sample can also miss the small second body, making an apparent clean split
   or clean baseline an artefact of retained evidence.

## Existing owner and conditions for reconsideration

The [observation-revision plan](../../plans/lidar-cluster-observation-log-and-async-tracking-plan.md#association-within-the-window)
continues to own merged-cluster repair. A later proposal must supply all of the following before
it becomes an experiment; this checkpoint selects no algorithm, arm, threshold, or release slot.

| Requirement                    | Evidence needed                                                                                                                                                                                                                                                                                                    |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Independent case selection     | Reviewed multi-object memberships before/during/after merges, plus one-body long/articulated/partially visible controls and non-merged neighbours. Preserve unknown memberships; record class, range, aspect, site, and capture eligibility                                                                        |
| Fixed evidence and ownership   | Freeze parent cluster and complete selected source membership, with voxel/cap lineage. Every selected source return belongs to at most one child or the explicit residual set; their union accounts for the parent selection. Never count both the parent and its children as independent measurements             |
| A specified proposal           | Define the geometric predicate or seed source, coordinate frame, ambiguous/unavailable-seed handling, and no-split/abstain path. Labels score proposals; they cannot supply production seeds. State whether future observations are used and charge their capture-time delay                                       |
| Matched controls               | Compare no repair and the owner's simplest motion/geometry assignment on identical members and temporal support. Keep the same candidate budget and include single-body controls; do not claim a PCL gain from extra look-ahead, samples, or privileged seeds                                                      |
| Decision rule before execution | Freeze numerical limits for false splits, lost/contaminated child membership, repaired merges, identity switches/duplicates, retention, ambiguity/abstention, and latency/memory. Report by independent episode and relevant strata. OQ-10's classification sample counts do not set this experiment's corpus size |
| Bounded integration            | Fit inside the owner's candidate/time limits, preserve immutable observations and revision lineage, and abstain on resource exhaustion. Use checked-in parameter sets and an eligible development corpus; qualify held-out accuracy and Pi cost separately                                                         |

An empty positive-merge set stops a claimed repair-benefit comparison. Synthetic fixtures can
check ownership, bridge behaviour, seeds, and failure handling, but do not replace reviewed
physical cases. If a future custom graph supplies competing motion/geometry unaries, report it
as that new hypothesis; the stock PCL MinCut name cannot stand in for its implementation.

## Changes and validation limits

The canonical review, candidate plan, oracle runbook, and observation-revision owner now record
this disposition. l4clus0005 gains a scope clarification only; its six arms, parameter files,
numerical rules, and proposed status remain unchanged. The original-discussion record links the
later answer without rewriting its earlier findings. No backlog priority or runtime behaviour
changes, and no splitter harness is added.

The reviewed source ranges and local evidence were checked, along with documentation lint, links,
experiment validation, the unchanged l4clus0005 arms/rules, and the no-results guard. No MinCut
execution, replay, new benchmark, or physical validation ran. Earlier OQ-03 fixture results remain
attributed to that checkpoint; at OQ-13, the latest full-suite run was OQ-08 and it was not repeated for that
documentation-only change. The later final branch pass reran the full suite. OQ-01 through OQ-13 are answered; implementation and qualification
dependencies remain open work, not unanswered review rows.

[esf]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/esf.hpp#L534-L549
[euclidean]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/extract_clusters.hpp#L62-L115
[conditional]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/conditional_euclidean_clustering.hpp#L79-L138
[rg-neighbours]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/region_growing.hpp#L324-L348
[rg-validation]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/region_growing.hpp#L461-L512
[mincut-setter]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/min_cut_segmentation.hpp#L188-L195
[mincut-unary]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/min_cut_segmentation.hpp#L348-L399
[mincut-radius]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/min_cut_segmentation.hpp#L100-L106
[mincut-binary]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/min_cut_segmentation.hpp#L430-L440
[mincut-flow]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/min_cut_segmentation.hpp#L255-L263
[mincut-labels]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/min_cut_segmentation.hpp#L515-L538
[mincut-graph]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/min_cut_segmentation.hpp#L284-L343
