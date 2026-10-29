# Classification descriptors need independent labelled objects

- **Status:** OQ-10 answered as a dependency contract; descriptor comparison remains parked
- **Related:** [candidate Item 7](../../plans/lidar-pcl-algorithm-candidates-plan.md#item-7-q1-descriptors-deferred), [classifier Phases 1–2](../../plans/lidar-ml-classifier-training-plan.md#scope), [shape Phase 4](../../plans/lidar-shape-descriptors-plan.md#phase-4-range-envelope-characterisation)
- **PCL Git SHA:** `3346843ea6b97697a1577293aa9cdf236b5c7ffb`, 1.15.1
- **Local baseline:** velocity.report `b349e0f46`, the OQ-09 checkpoint

All thirteen review questions and the final branch pass are complete. Source findings and
validation below describe this report's recorded checkpoint; subsequent full-suite validation
and remaining implementation prerequisites are recorded in the
[current handover](pcl-astera-handover.md#resume-point). Closing a review question does not
complete or authorise its proposed campaign.

## Decision

Keep ESF and moment-of-inertia as offline classification comparators, blocked on the Parked P2
classification scorecard and corpus-contract rows. The minimum evaluation design is **100
independent held-out objects per target class or contrast group**, plus **30 independent objects
in every class × aspect × range cell claimed as supported**. These are proposed screening floors,
not measured coverage, training-set sizes, or a promotion rule. Missing cells remain unsupported;
do not pool them away. A smaller pilot can check plumbing and report descriptive results.

The four primary contrast groups are truck, long car, motorcyclist, and cyclist: at least 400
independent held-out objects before the two contrasts meet this coarse class-level screening floor.
This alone does not establish whole-classifier precision or safety. Other cars, buses, pedestrians,
and birds need coverage before claims about their respective classes. The minimums, splits, and
meaning of “supported” are specified below. No capture, training, descriptor run, or runtime change
was performed for this checkpoint.

## What labels are actually available

The old five-PCAP plan is a capture proposal, not an inventory of everything available now.
The [W7 archive audit](end-on-truck-window-spike-2026-10.md) records 23 replay sites and 77 scored
minutes, and 24 archive sites totalling 514 minutes. Its truck windows are label-free candidates
selected partly by inferred length. They cannot be counted as reviewed trucks or independent
classification examples. [label0001](../../../data/experiments/label/label0001.md) and
[label0002](../../../data/experiments/label/label0002.md) remain proposed in this checkout; their
held-out membership and physical-reference work is a dependency, not delivered class coverage.

A read-only inventory of the locally mounted dataset found one label set under `labels/`:
`20251205T1417_80858-75c_kirkham-9th/kirk0-reference`. Counting reviewed object records with at
least one reviewed mask gives:

| Reference class | Reviewed object records | OQ-10 meaning                                                                       |
| --------------- | ----------------------- | ----------------------------------------------------------------------------------- |
| car             | 13                      | Long-car membership and physical length have not been established by this inventory |
| truck           | 3                       | Tuning examples at one site; not 100 independent held-out examples                  |
| bus             | 1                       | Insufficient control coverage                                                       |
| pedestrian      | 4                       | Insufficient control coverage                                                       |
| cyclist         | 0                       | Missing from this local set                                                         |
| motorcyclist    | 0                       | Missing from this local set                                                         |
| bird            | 0                       | Missing from this local set                                                         |
| noise           | 7                       | Reviewed annotation category, not a road-user class                                 |
| ground          | 2                       | Reviewed annotation category, not a road-user class                                 |

All 30 object records and 4,102 masks have status `reviewed`; 21 records are road users. There
are no correspondence entries. This is a record count, not an audit proving distinct physical
objects, class correctness, full-scene completeness, or useful aspect coverage. No object record
has an aspect, range, or long-car field. Mask completeness, domains, physical poses, and deduplication
still need checking when constructing the classifier corpus. This local clone is not asserted to
be the latest remote dataset or the complete owner's archive. No raw labels were copied to Git.

The earlier [F0 report](facet-f0-attribution-kirk0-report.md) scored 20 road users from revision 1974. The inventory here reads revision 2233, so both numbers can be correct at their respective
label-set revisions. Preserve the old result; do not silently relabel its denominator as 21. Neither population is
a fresh held-out sample: kirk0 is already tuning evidence.

### Frozen provenance for the inventory

| Field                            | Value                                                              |
| -------------------------------- | ------------------------------------------------------------------ |
| Dataset-relative label directory | `labels/20251205T1417_80858-75c_kirkham-9th/kirk0-reference`       |
| Label set                        | `dd6579b7-4705-4a95-a2a3-2a4c12bef749`, schema 2, revision 2233    |
| Declared state digest            | `12b47778fdf9119e83d18e9e8e0ebeadaea1b52b5ba32b35441c83d8e8edfad8` |
| Capture SHA-256                  | `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a` |
| `labels.json` SHA-256            | `d0fc60acb8feacb601c465b2b15239fc67554cffea625a4c8ea886da0a56ea5e` |
| `objects.json` SHA-256           | `b38bf300529ac2d0f6af5671cd4024841407c08cb5c2d7d8e8d7b5d78a3991ec` |

File hashes were calculated during this review. The state digest is read from the header, not
recomputed from the packet table and every canonical file. The method parsed the object list and
mask chunks, counted classes/statuses, and checked which reviewed objects had reviewed masks.
It did not run a replay, a class prediction, or a physical-reference validation.

## Claim-level evidence and consequences

| Claim                                                                                                | Evidence at the reviewed implementation or local baseline                                                                                                                                                                                                                                                                                                                                                                                                    | Consequence                                                                                                                                                                                    |
| ---------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C10-01: ESF removes absolute scale before forming its histogram                                      | [unit-sphere scaling](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/esf.hpp#L481-L501) subtracts the centroid and scales by the maximum radius                                                                                                                                                                                                                                   | Keep dimensions and kinematics in the shared baseline. ESF alone cannot recover size removed by normalisation                                                                                  |
| C10-02: stock ESF sampling is stochastic and retries rejected triples                                | [sampling loop](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/esf.hpp#L50-L114) seeds `rand` from wall time and asks for 20,000 accepted triples                                                                                                                                                                                                                                 | Record seed-control limitations and repeatability; use a bounded worker and explicit invalid result for degenerate clouds. Setting a seed outside this function does not control its reseeding |
| C10-03: normalised moment of inertia is still point-count dependent                                  | [mass normalisation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/moment_of_inertia_estimation.hpp#L129-L180) uses `1/N²`; [moment calculation](https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/features/include/pcl/features/impl/moment_of_inertia_estimation.hpp#L473-L492) multiplies the sum of squared axis distances by that mass | Freeze sampling/caps and mass settings; stratify by retained and original point count. “Normalised” does not mean invariant to density                                                         |
| C10-04: the current cascade cannot demonstrate truck or motorcyclist recall by emitting those labels | [classification.go](../../../internal/lidar/l6objects/classification.go), `ClassifyFeatures`, rules 3 and 5 are disabled                                                                                                                                                                                                                                                                                                                                     | Include those true classes in the confusion matrix even when a baseline predicts none. Descriptor feature comparisons need the same downstream classifier and class vocabulary                 |
| C10-05: detection by class is not classification accuracy                                            | [ground_truth.go](../../../internal/lidar/adapters/ground_truth.go), `DetectionRateByClass`, counts matched reference tracks by `UserLabel`                                                                                                                                                                                                                                                                                                                  | Phase 1 must compare true and predicted classes, separately from detection and association failures                                                                                            |
| C10-06: labels are no longer confined to runtime SQLite                                              | [label-set format](../../../internal/lidar/annotation/labelset/README.md) records identity, revision, and state digest; [operator workflow](label-sets.md) stores capture-bound labels in the dataset; the inventory above reads one                                                                                                                                                                                                                         | Phase 2 should consume these references alongside legacy track labels. It must not create a second incompatible truth store                                                                    |

The PCL claims were checked against the local reviewed source. These are implementation readings,
not measured classifier results. Source-scale and density arguments are analytical counterexamples:

1. **Scaled copies:** clouds `X` and `2X` have the same normalised ESF input in exact arithmetic.
   A large and small geometrically similar body therefore cannot be separated by absolute size
   from ESF alone. Partial views can remove the distinctive geometry as well.
2. **Duplicated points:** duplicating every point gives the same mean and covariance directions,
   doubles the squared-distance sum, and quadruples `N²`. At a fixed axis, default moment values
   halve even though the measured shape is unchanged. Eccentricity is a separate output; this
   example concerns the moment values, not every output of the estimator.
3. **Degenerate support:** two points never supply three distinct indices; collinear triples
   never pass ESF's area test. The retry loop has no attempt limit. Validity checks and a worker
   time limit must precede an oracle campaign; “at least three points” alone is insufficient.
4. **Repeated frames:** a truck visible in 500 frames still contributes one independent object.
   A frame-level split can put the same geometry into training and test, and a frame-weighted
   score can let that truck dominate many briefly visible cars. Neither creates class coverage.
5. **Aspect confounding:** if all trucks are end-on and all cars are broadside, a descriptor can
   recognise aspect while appearing to recognise class. A pooled score and large frame count
   cannot repair the missing class × aspect cells.

## Minimum counts and what they justify

Use a binary correctness outcome per independent object for each recall estimate. For a proposed
screening precision of about ten percentage points, 100 objects is a useful rounded floor.
From the [Wilson interval formula](https://www.itl.nist.gov/div898/handbook/prc/section2/prc241.htm),
the largest two-sided 95% half-width is `z / (2 sqrt(n + z²))`, at observed proportion 0.5,
with `z = 1.959963984540054`. Substitution gives:

| Independent denominator | Maximum half-width      | Interpretation for this design                                |
| ----------------------- | ----------------------- | ------------------------------------------------------------- |
| 20                      | 20.07 percentage points | Small descriptive pilot                                       |
| 30                      | 16.85 percentage points | Coarse cell screen only                                       |
| 100                     | 9.62 percentage points  | Minimum class/contrast screen                                 |
| 384                     | 4.98 percentage points  | Approximate five-point precision if a later decision needs it |

The formula and arithmetic concern one binomial proportion under independent sampling. These
are not simultaneous confidence bounds across cells, power calculations for an improvement, F1
intervals, or protection against site correlation. For paired model deltas, retain per-object
outcomes and use paired uncertainty estimates; plan the confirmatory sample size from the minimum
useful effect and pilot discordance before opening held-out results. No five-point gain can be
promised detectable from 100 objects. Fitting a 640-bin ESF model may need substantially more
training objects; determine that separately with development learning curves.

| Ground-truth group | Minimum independent held-out objects | Role                                                                           |
| ------------------ | ------------------------------------ | ------------------------------------------------------------------------------ |
| Truck              | 100                                  | Primary contrast with long car                                                 |
| Long car           | 100                                  | Reviewed slice of `car`, not a new emitted class                               |
| Cyclist            | 100                                  | Primary contrast with motorcyclist                                             |
| Motorcyclist       | 100                                  | Primary contrast with cyclist                                                  |
| Other car          | 100                                  | Required for a general car-class claim beyond the long-car slice               |
| Bus                | 100                                  | Required for a bus-class claim and truck/bus confusion assessment              |
| Pedestrian         | 100                                  | Required for a pedestrian-class claim and vulnerable-user confusion assessment |
| Bird               | 100                                  | Required if claiming the full seven named classes                              |

Thus the two contrasts need at least 400 test objects, a six-road-user-class screen needs at
least 700 including the two car slices, and a full seven-class screen needs at least 800.
Training and validation objects are additional and disjoint. Noise, ground, uncertain truth,
missed detections, and `dynamic` predictions remain visible in appropriate rejection/coverage
counts; do not invent an eighth semantic truth class by treating `dynamic` as known identity.
These denominators concern recall. Precision uses predicted positives and also needs at least
100 independent predictions of a class for the same coarse precision claim; otherwise report
its interval and mark it underpowered. A balanced contrast pack does not estimate deployment
precision: also score traffic-selected complete windows with their observed class prevalence.

Long-car membership must use reviewed physical length independent of predicted boxes, with a
threshold frozen by the corpus owner before descriptor outputs are inspected. Unknown length
stays unknown. No existing reviewed long-car count was found; this threshold and the class-label
adjudication rubric are still owner inputs, not facts inferred from the 13 car records.

### Aspect, range, and independence

Use sensor-to-object line of sight and independently reviewed body yaw to define folded aspect
`alpha` in `[0°, 90°]`: end-on `[0°, 30°)`, oblique `[30°, 60°)`, broadside `[60°, 90°]`.
Keep approach/recede and visibility/occlusion as additional fields. Body yaw from the candidate
tracker cannot be the reference. Unresolved yaw is an explicit unknown-aspect bucket, not dropped.
For pedestrians and birds without a defensible body axis, retain unknown and do not claim an
aspect envelope. This does not block their class-level control score.

Use range bands `[0, 20)`, `[20, 40)`, `[40, 60)`, and `[60, infinity)` metres, measured from
sensor origin to reviewed object reference position. Publish point count and retained fraction
beside range. These proposed bins interrogate the sparse tail; they are not declarations that
40 m or any particular point count is a universal descriptor limit.

For each primary contrast group, require 30 independent objects in every aspect × range cell
being claimed. That is 12 cells and 360 object-cell memberships per group for the full envelope;
it does not necessarily require 360 different objects, since an object can traverse several
cells. Count each object once within each cell and once in the class-level score. Never add
cell memberships and call the sum independent objects. Choose one observation per object/cell
with a frozen time-based selection rule, independent of success or descriptor validity. Keep
invalid/missing descriptors in the denominator, with the shared fallback, and report their rate.
Within-object repeated observations are useful for stability, not extra independent samples.

Freeze sites before extraction: as a practical first design, at least two development sites,
one validation site, and two untouched test sites, with complete captures kept in one split.
For each claimed class/contrast, at least 25 of its 100 test objects must come from each of two
test sites; each claimed cell needs support at both sites, with at least ten objects at each.
These are diversity floors chosen for this screen, not a proof of generalisation to all sites.
Report per-site scores; two test sites cannot support a precise population-of-sites interval.
If coverage is unavailable, narrow the declared envelope before scoring or collect more labels.

Use capture-bound object identity, reviewed correspondences, and split manifests to keep fragments,
replays, clips, augmentations, and the same object episode in one partition. Do not identify people
or vehicles across unrelated captures to satisfy this rule. Select non-overlapping capture sessions
and record the residual possibility of repeated traffic. Kirk0 stays in development. label0001 is
a useful route to a new window, but one three-minute window is not evidence that these quotas exist.

## Dependency ownership and execution boundary

| Existing owner/workstream                              | Deliverable before the descriptor comparison                                                                                                                                                                   |
| ------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Parked P2 classification scorecard, classifier Phase 1 | Frozen truth/emitted vocabulary, object and observation aggregation, matching rule, per-class P/R/F1, macro-F1, calibration, fallthrough, invalid-descriptor coverage, paired comparison and precision targets |
| Parked P2 corpus contract, classifier Phase 2          | Export from specified label-set revisions and legacy labels with explicit provenance; object/capture/site split manifest, adjudication rubric, long-car threshold, class × aspect × range counts and unknowns  |
| Capture/reference owner, including label0001/0002      | Reviewed class, independent identity, yaw, length and visibility where knowable; missing cyclist/motorcyclist coverage; untouched sites and traffic-selected windows                                           |
| Shape-descriptor Phase 4                               | Usable envelope and stability by range/point count, with the above object counts; explicit fallback and missing-cell table                                                                                     |
| Later offline harness implementer                      | Matched retained inputs, safe ESF execution, repeatability record, PCL Git SHA and fixed settings, feature export and the same downstream model family/tuning budget across feature arms                       |

The initial comparison should include the shipped rule baseline, a fitted shared scalar baseline,
and that same fitted baseline plus ESF or moment/eccentricity features separately. Otherwise a
new model or re-enabled class can receive credit incorrectly assigned to a descriptor. Hold L3/L4,
association, sampling, splits, and evaluation membership fixed. Separate a diagnostic on reviewed
object point sets from the end-to-end score on predicted clusters; using reviewed masks as input
would hide the fragmentation and membership errors the deployed classifier faces.

No campaign is authorised by closing this question. When scheduled, define each parameter arm in
`config/experiments/`, predeclare its learning and comparison protocol, and keep captures, feature
rows, predictions, and metric dumps outside the checkout. Commit human-readable reports, reusable
code, hand-authored fixtures, and parameter sets. The classification plan's old instruction to
check generated corpus exports into this repository is corrected in this checkpoint.

## Validation and resume boundary

Validated the source paths, local inventory arithmetic, independent-count arithmetic, and document
links. Fresh `make lint` and `make check-no-results` passed. The full suite passed at OQ-08 and
was not repeated for the subsequent documentation-only changes. No classifier accuracy, descriptor
latency, Pi budget, or physical envelope was measured.
The result closes the dependency question, not the blocked implementation or labelling work.
OQ-11 subsequently [reconciled the original discussion](pcl-original-discussion-reconciliation.md).
The remaining corpus and implementation dependencies do not authorise a descriptor campaign
or turn the minimum counts into an adoption decision.
