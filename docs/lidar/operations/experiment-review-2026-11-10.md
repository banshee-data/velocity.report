# Experiment review, November 10, 2026: the body and the motion model after the November fixes

<!-- ignore-style-length -->

Between November 5 and 9 the queue answered fifteen experiments, landed a dozen default-off
options in the tracker and the solid body, and adopted none. This review reads what those results
say about four things the owner asked: which experiments are worth running again at other
parameter values, which new directions the evidence opens, where a person's review is needed, and
where labelling buys the most. It then reads the briefings, the maths notes and the gap analysis
for what to implement and evaluate next in solid-body estimation (extents, facets, accumulated
shape) and in tracker accuracy (CV, CA, the multiple-model filter). Three specs are proposed with
their rules written now; the owner approves them.

- **Status:** Review, 2026-11-10; no measurement was run and no default changed
- **Goal:** a vehicle's body and motion the platform can state: one track a vehicle, an extent
  that is the vehicle's, and a speed with a known error
- **Inputs:** the [November 5 review](experiment-review-2026-11-05.md); the reports since
  ([kirk0 in the cloud](cloud-run-kirk0-labelled-2026-11.md),
  [the November fixes benchmarked](tracking-geometry-fixes-benchmark-2026-11.md),
  [robust birth gate](robust-birth-gate-kirk0-2026-11.md),
  [near-edge identity](near-edge-identity-kirk0-2026-11.md),
  [extent bound accrual](extent-bound-accrual-kirk0-2026-11.md),
  [G-EST-1 on face residuals](gest1-face-state-kirk0-2026-11.md),
  [two-point guards](two-point-init-guards-2026-11.md),
  [shared orientation](shared-orientation-duplicates-2026-11.md),
  [truck membership](w3-truck-membership-extent-kirk0-2026-11.md),
  [D2 and the bias baseline](d2-frozen-split-bias-baseline-2026-11.md),
  [containment and cardinality](containment-cardinality-guard-cloud-2026-11.md)); the
  [face state and CV/CA maths review](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md);
  the [gap analysis](../../../data/maths/paper-implementation-gap-analysis.md); the briefings of
  October 9 to November 9; the open queue
- **Method:** the [experiment-review skill](../../../.claude/skills/experiment-review/SKILL.md)
- **Related:** [state estimation plan](../../plans/lidar-state-estimation-plan.md),
  [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md),
  [label0003](../../../data/experiments/label/label0003.md),
  [experiment queue plan](../../plans/platform-experiment-queue-plan.md)

Each statement is marked as **measured** (a report's figure), **inferred** by this review, or
something **nothing in the repository can tell yet**. Every number is a report's.

## Answer

The week found one lever and one wall. The lever is the association cost: scoring a cluster by
its likelihood (`ln det S` beside the Mahalanobis distance) takes kirk0's identity switches from
90 to 6 without ending a track, and a birth gate for the fragments it releases brings brief
tracks and clutter back inside the benchmark's bounds. It does what the overlap end did (87 to 5)
at a fraction of the noise, so the identity candidate changes and the overlap-end line of work
drops in rank. The wall is the extent. The solid body's centre sits a median 1.03 m from kirk0's
reviewed poses against the published box's 0.19 m, truck 2's believed length is 4.60 m against
9.63 m, and four successive forms of the extent estimator (the face state, span bounds, the
floor, bound accrual) have each moved that figure sideways. Everything downstream of the extent
is premature until it is fixed: the near-edge tracked filter loses identity, the box cannot take
the body's extents without widening cars, and CA "passes" on face residuals only because it
absorbs a metre-scale tail in the face measurement. Parameter reruns are mostly not the answer:
of ten options tried at one or two settings, three are worth another value and the rest
failed for structural reasons a sweep would not reach. Nothing here is held out. The single
action that unlocks the most is still a person's two and a quarter hours on label0003's spans,
whose window was drawn by lot and cut on November 8 and has no label on it.

For the two research questions: for the body, build a body-frame occupancy profile (Schäfer et
al. 2026's grid, adopted under D-28 and never built) as a shadow against the extent state, and
do not build a mesh, a contour tracker or a facet graph until it reports; the F0 report already
showed that removing the extent shortfall takes end-face error to −0.02 m, so edge localisation
is not what is wrong. For the tracker: ship no motion model. Read G-EST-1 on the corpus first,
restated to ask whether CV is over-dispersed along the track at all, because on kirk0 it is
under-dispersed and the gate's premise has not been met.

## What the experiments established

### A. What a rerun at other parameter values would and would not tell

Measured in every row; the last column is this review's inference.

| Option, and what was tried                                                                           | Result                                                                                                                                        | Rerun at other values?                                                                                                                                                                                              |
| ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `process_noise_vel` at 0.4 and 0.8 against 0.2 (l5est0003)                                           | Switches 90 to 43 and 24; ×4's HOTA 5th percentile below zero; truck 2 still 0.67 m/s low against a 0.40 bound                                | **Yes, but for speed, not identity.** The likelihood cost reaches 6 switches, so its identity case is taken. As a consistency lever it has never been read: l5est0007 reads 0.2, 0.4 and 0.8 on the corpus          |
| Robust birth gate: one margin (0.25 m plus sigma, cap 1 m), under the Student-t cost only (l5id0021) | Passes in sample; plain `likelihood_cost` scores as the robust cost does (HOTA 0.4360 against 0.4362, 6 switches each)                        | **Yes, the neighbouring arms, not the margin.** The gate under the plain likelihood and with tentative priority are the report's own next arms: l5id0023. Sweeping the margin on kirk0 would fit kirk0's 43 births  |
| Relative settling at 0.075 (l3bg0030's kirk0 clause)                                                 | kirk0's lowest ratio is 0.076                                                                                                                 | **Yes, on the corpus, with the trajectory recorded.** One threshold sitting 0.001 from one capture's floor says the criterion is being read at its knee; l3bg0030 should report the ratio's curve, not one crossing |
| `axis_sigma_scale` at 1.5 live; 1.0, 0.75, 0.5 read offline (shared orientation)                     | Mean heading NIS 0.73 to 1.52, median 0.10 to 0.20; no scale brings both near χ²₁; the prediction is a median 41 % of the innovation variance | **No.** A scale moves the mean and leaves the shape. The defect is in what the heading innovation is made of; read the tail's frames first                                                                          |
| Extent bound accrual: two code versions, each parameter at one value (geom0018)                      | Truck 2 4.58 and 4.83 m against 9.63 m; R1 fails in both; the 6.24 m target was itself an overshoot from bounds of at most 3.95 m             | **No.** The estimator has not seen the far end; no window or rank changes that. Restate the check on poses after 75 s, which need references                                                                        |
| Near-edge common gate: one form, 3 m bound kept (l5id0022)                                           | Births 183 to 143, duplicate pairs 23 to 16 (`a0` 10); HOTA still 0.34 against 0.43; AssA 0.87 to 0.70 with DetA level                        | **No.** The loss is in association accuracy and no gate value was shown to hold it. Per-object AssA first                                                                                                           |
| Two-point initialisation guards: one shadow margin, one slack cap (l5est0006)                        | Passes; declines 66 % and 53 % of pairs; no benefit shown; no vehicle above 14 m/s in either case                                             | **Not on these captures.** There is nothing fast to initialise. A capture with road users above 14 m/s is the precondition, or the class-range prior the benchmark names as the cheaper form                        |
| Track-conditioned joining: one width, two standard deviations (l4clus0010)                           | Truck fragmented instants 21.2 to 15.5 %; solid-body centre error worse, 1.09 to 1.22 m                                                       | **No, a corpus read.** The width is not what is unknown; whether it merges two vehicles side by side is, and kirk0 has no such pair                                                                                 |
| Containment claims with the cardinality guard (l5id0018)                                             | HOTA 0.4015 and 0.4071 against a floor of 0.4184                                                                                              | **No.** The report says so: a new lever, not a retune                                                                                                                                                               |
| The five-point fragmentation bar (l4clus0008, radius graph)                                          | `join` −0.8 points where `a0` holds 6.5 %                                                                                                     | **The bar, not the arm.** It was written against a 35 % baseline. The owner resets it or the arm can never pass                                                                                                     |

**A1. kirk0 cannot resolve most of what is being asked of it.** Measured: over 2,000 draws of
its 19 scored objects, `a0`'s HOTA runs 0.413 to 0.540 and its switches 4 to 242; half-widths
0.063 HOTA, 0.057 IDF1, 119 switches (l5id0015). Paired deltas are tighter: the robust cost's
HOTA delta runs +0.0023 to +0.0222. Inferred: an absolute kirk0 figure decides nothing and a
paired delta decides only large effects. Of this week's results, three survive that reading: the
likelihood cost's gain, the near-edge filter's loss (−0.116 to −0.021 HOTA), and the medoid's
loss against the mean (geom0015). Every clause written at 0.005 or 0.01 in an open spec
(l5id0008, l5id0010, l5id0011, l5id0014) should be restated against the paired interval before
it runs.

**A2. Options that are bit-for-bit no-ops were scored as arms.** Measured: `hygiene` and
`face-state` equal `a0` on every identity and noise figure in l5id0019, by construction (one is
latent at the shipped caps, the other a shadow). Inferred: a shadow should be scored only on the
quantity it writes. For the face state that is the solid body's centre and extents, where it
reads 1.03 m against the box's 0.19 m: it is not a no-op, it is behind.

### B. New directions the results open

**B1. The identity candidate is now the association cost.** Measured: `robust` 6 switches, 30
fragmentations, duplicate pair-frames 452 to 246 on kirk0, pooled duplicates 26 to 13 over three
captures; `robust-birth` brief tracks 14 against `a0`'s 12 and clutter 16 against 15. The overlap
end reached 5 switches with brief tracks 12 to 18 or 29 and a lost-track clause it could not
pass. Inferred: the two act on the same defect (a coasting track outbidding a fresh one, gap
analysis S3) and the likelihood removes it at the contest instead of after it. The overlap-end
chain (l5id0014, l5id0016, l5id0013's acceptance) is no longer the path to the held-out window;
it stays in the queue as the comparator.

**B2. Per-frame joining finally moved a labelled figure, through the track.** Measured:
`join-membership` gains IDF1 with a paired interval above zero (+0.0004 to +0.0182 for `join`)
and takes truck fragmented instants from 198 to 145 of 936, at zero noise cost on five cases.
Inferred: this is the pattern the November 5 review predicted, a temporal lever on a per-frame
problem. Its open risk is the one kirk0 cannot test: two vehicles side by side in one footprint.

**B3. The reference point matters more than the motion model, and it is not solved.** Measured:
on car 8 the mean measurement sits 0.35 m from the reference (0.27 m toward the sensor), the
medoid 0.47 m, the OBB centre 0.06 m; the OBB centre matches 22 of 22 poses at 0.56 m where the
mean matches 15 at 2.47 m; but as the tracked measurement it costs 125 switches (geom0015).
Inferred: the least biased point is the least stable one, because it moves with the visible
extent. A point that is both needs the extent, which is B4.

**B4. The extent is the wall.** Measured: believed length short of the labelled span in 83 % of
car and 97 % of truck instants (F0); solid-body centre 1.03 m against the box's 0.19 m; truck 2
at 4.60 m, having read 6.24 m before a defect was fixed; `box-extent` widens car 8 to 2.32 to
2.56 m against a 1.73 m reference; pooled box rows over 2.2 m wide rise from 24 % to 27 to 32 %.
Inferred: the estimator combines per-frame summaries (a span, a bound, a third-largest bin), and
each summary has discarded which cells were seen empty. An estimator that keeps occupancy and
free space per cell is a different kind of thing from another combination rule; that is
[geom0019](../../../data/experiments/geom/geom0019.md).

**B5. The motion-model gate's premise is unmet.** Measured: on kirk0, NIS per dimension is 0.43
on the medoid and 0.24 to 0.33 on faces; 1.75 % of values exceed the 95 % bound. In October the
gap analysis measured moving-track NIS of 1.7 to 3.2 with 7.7 to 17.1 % beyond the bound at the
sites. CA's along-track p99 ratio is 0.972 on the medoid (fails) and 0.615 on faces (passes),
where CV's own p99 is 4.06 m against the medoid's 1.27 m. Inferred: the two NIS readings differ
by a factor of four to seven and nothing reconciles them; candidates are the mean centre
(adopted November 1, after K9), the capture, or the population. Until that is read on the
corpus, "CV is over-dispersed" is October's finding about October's measurement.
[l5est0007](../../../data/experiments/l5est/l5est0007.md) reads it.

**B6. Reflections are a population, not noise.** Measured: all eight fast tracks under two-point
initialisation are thin clusters on a nearer car's bearing at 1.6 to 3.0 times its range and the
same multiple of its speed; the shipped tracker never confirms them only because it cannot hold
anything born above about 14 m/s. Inferred: any change that lets the tracker follow fast road
users (two-point, a looser process noise, CA) will also confirm these. They need a class at L4
before a faster filter ships, and they have no label.

### C. Where a person's review is needed

| What                                                                                                                                    | Why a machine cannot settle it                                                                                                                                                                                                            | Time     |
| --------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| The observation `speed_mps` fix made during the cloud run, and whether deployed rows are rewritten                                      | Rows read 0.38 to 0.72 of the reference speeds before it. It changes what the database holds; that is a decision about published history, not a replay                                                                                    | 30m      |
| The reconstructed kirk0 pack against the original                                                                                       | Its points digest differs and the retention floor was lowered to accept it. Every cloud score since reads it                                                                                                                              | 1h       |
| Truck 2 after 75 s, and the six unreviewed poses of `following-472-530`                                                                 | geom0018's restated check and fix 3's target both need references where the far end is seen; references are authored                                                                                                                      | 1h       |
| A merge reference: one pair of vehicles side by side                                                                                    | Three reports name it as the condition for a corpus read of joining and of the extent record. kirk0 has none                                                                                                                              | in masks |
| The join's five-point bar                                                                                                               | An outdated baseline; only the owner resets a rule                                                                                                                                                                                        | decision |
| The parameter hash that names two builds (`3184a2e6c676` for guarded and unguarded two-point; `f0d816ce7ec9` for both accrual versions) | Replay options are code, not tuning, so the fingerprint cannot tell versions apart. A manifest code identity is engineering; accepting past rows is not                                                                                   | decision |
| Whether embarcadero-folsom's window is unread for geometry                                                                              | label0003 says the window was never read label-free; the near-edge campaign scored the embarcadero-folsom case label-free for lateral geometry. It stands as held out for identity, noise and counts; for geometry the ledger must say so | 15m      |

## Where the metric stands

The chain from a capture to a body and a motion the platform can state:

1. **Clusters.** Truck instants fragmented 21.2 % under the defaults, 15.5 % with
   track-conditioned joining. Side-by-side merges untested. Waits on a merge reference.
2. **Identity.** 90 switches under the defaults, 6 under the likelihood cost with the birth
   gate; in sample. l5id0023, then label0003's spans.
3. **The reference point.** The mean, 0.35 m off on the one car with a reference. The less
   biased point costs identity. Waits on the extent.
4. **The extent.** The weakest link: median centre 1.03 m, truck 2 at 48 % of its length, cars
   too wide. geom0019.
5. **The published box.** 0.19 m at the median on 22 poses; truck 2 1.49 m, all of it extent.
   The best body the platform has, and it is the per-frame one.
6. **The motion model.** CV, lagging 0.71 m/s per 1 m/s² by computation; whether it is
   over-dispersed on the corpus is unread. l5est0007.
7. **Speed.** Two trucks read 0.77 and 0.86 m/s low against differenced poses. No reference off
   kirk0, none above 7.2 m/s. Waits on a radar beside the LiDAR (the November 5 review's E17) or
   the RTK probe (D-29, D-40).
8. **Truth.** One held-out window drawn and cut, unlabelled. Everything above is tuning data.

## The next seven days

| Rank | Spec                                                                       | Question                                                                                                          | Runs without a person                  | Machine | Agent implement | Unblocks                                                                                    |
| ---- | -------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | -------------------------------------- | ------- | --------------- | ------------------------------------------------------------------------------------------- |
| 1    | [l5id0023](../../../data/experiments/l5id/l5id0023.md) (new)               | The likelihood cost with the birth gate on two placements; which of three forms goes to the window                | Yes for the screen; K5 waits on rank 4 | 2h      | 30m             | The identity candidate for label0003; retires the overlap-end chain's claim on the window   |
| 2    | [l5est0007](../../../data/experiments/l5est/l5est0007.md) (new)            | Is CV over-dispersed along the track on the corpus, is it acceleration, and does a looser process noise answer it | Yes, label-free, needs the corpus host | 6h      | 35m             | Whether CA and the IMM are worked on at all; G-EST-1 criterion 1; the q value for l5est0004 |
| 3    | [geom0019](../../../data/experiments/geom/geom0019.md) (new)               | A body-frame occupancy profile as the extent estimator, as a shadow                                               | Yes; kirk0 in the cloud                | 50m     | 35m to 1h       | The extent arm for geom0016; whether a two-dimensional shape model has a case               |
| 4    | [label0003](../../../data/experiments/label/label0003.md)                  | Road-user spans on the drawn window                                                                               | No: 2h15m operator                     | none    | none            | Rank 1's held-out clause; counts with truth; the first evidence off kirk0                   |
| 5    | [l4clus0010](../../../data/experiments/l4clus/l4clus0010.md)'s corpus read | `fragment_join_track_membership` beside the join alone                                                            | Screen yes; decision waits on masks    | 2h      | none            | A second candidate for the window; needs the merge reference to decide                      |
| 6    | [l3bg0030](../../../data/experiments/l3bg/l3bg0030.md)                     | The relative settling criterion, reporting the ratio's trajectory                                                 | Yes                                    | 3h      | done            | Whether settling can decide anything; off this goal's chain, so last of the machine's       |
| 7    | [geom0016](../../../data/experiments/geom/geom0016.md)                     | The facet attribution gate, restated                                                                              | No: waits on label0001's masks         | 24m     | done            | Whether any facet arm has a case; reads `insufficient_evidence` until then                  |
| 8    | [l5est0004](../../../data/experiments/l5est/l5est0004.md)                  | A looser velocity process noise as an identity lever                                                              | Yes                                    | 2h38m   | none            | Little now: restate it after rank 2 with the likelihood arm as its baseline                 |

**Two overnight runs.** The first, in the cloud on kirk0 and the Hugging Face placements:
l5id0023's screen and geom0019 (each after its implementation step, both closed by an agent).
The second, on the host with the S2 corpus: l5est0007's 92 replays and l4clus0010's corpus read.

**What is read first.** l5est0007's M1. If CV is not over-dispersed along the track on the
corpus, the CA, IMM and smoother-horizon work all reorder behind the measurement noise.

**Held-out ledger, this round.** Nothing is claimed as held out label-free. l5id0023 reads the
first two segments of ashbury-downey and haight-divisadero as a screen (read by l5id0019 for
identity and noise) and label0003's spans once, for one arm; the window is tuning for identity
afterwards. l5est0007 is the first read of the first segments for the filter-consistency
family; the second segments stay unread for it. geom0019 reads kirk0 and two placement segments,
all tuning for geometry. embarcadero-folsom has been read label-free for lateral geometry by the
near-edge campaign and for nothing else.

## Labelling and annotation priorities

| Order | Task                                                                                                                                                                        | Person time | Tooling today                                                                                               | Serves                                                                                            |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| 1     | **label0003, spans** on `seg-955587a76c49fe5c6bd735c4` (embarcadero-folsom 575 to 755 s, drawn by lot November 8, cut, 1,800 samples)                                       | 2h15m       | The annotation window and label sets. No mask-free span record exists, so spans are entered as kirk1's were | l5id0023 K5; l4clus0010's candidate; a vehicle count with truth; every "named for the window" arm |
| 2     | **kirk0 references:** truck 2 on poses after 75 s, and the six unreviewed poses of `following-472-530`                                                                      | 1h          | Physical authoring in the window (`fit:mask_v1`)                                                            | geom0018 restated; geom0019 P1 on more than six poses within two seconds                          |
| 3     | **label0001, masks** on the same window, beginning with the long vehicles and any pair side by side, with keyframed yaw and extents on two or three of them under label0002 | 7h15m       | Point masks, checks as warnings, the browser review and freeze                                              | geom0016's decision read; the merge reference; near-face evaluation off kirk0; every adoption     |
| 4     | **Reflection clusters:** mark the eight fast tracks' clusters on kirk0 and ashbury-downey's first segment as a `reflection` class                                           | 45m         | Point masks; the class is a label-set addition                                                              | An L4 reflection filter's measure; the precondition for any filter that can hold a fast vehicle   |
| 5     | **A radar beside the LiDAR** for one 20-minute placement on a street with traffic above 14 m/s                                                                              | 2h, field   | Radar ingest and the transit worker                                                                         | The first speed reference; two-point initialisation's missing population; l5est0003's truck lag   |

About eleven and a quarter hours for items 1 to 4. The order within item 3 is the change from
November 5: masks on long vehicles and one adjacent pair first, because three of this week's
reports wait on exactly those and the rest of the window can follow. The rules hold: the window
was drawn by lot, and items 1 and 3 are one window.

Where labelling is **not** the constraint: l5est0007 and geom0019's P3 and P4 are label-free by
design, because consistency, steadiness of a rigid body's length and containment of withheld
frames are properties the data can be held to without a reference.

## Solid-body estimation: what to implement and evaluate

What the sources give, read against this week's measurements.

| Source                                                                                                               | What it offers                                                                                                                                                 | Verdict here                                                                                                                                                                                                |
| -------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Schäfer, Alrifaee and Hashemi 2026 (briefing 2025-01 to 2026-10; D-28)                                               | A one-dimensional grid per dimension, 0.1 m cells, log-odds, lock-on; dimension RMSE 0.46 m and pose RMSE 0.52 m against RTK over 19 passes                    | **Implement, as a shadow: geom0019.** Adopted October 10, never built. It is the only extent estimator in the reading with external truth behind it, and it keeps free space                                |
| [Maths review, November 8](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md), §3.1 to 3.4 | Extents in the state; a censored posterior; the frame's span as a bound; orientation variance in the face noise                                                | **Built, default-off, and not yet ahead of the box.** Items 1 to 3 were to be scored as one arm against geom0015's rows: that score is 1.03 m against 0.19 m. Keep as the baseline geom0019 must beat       |
| [Facet registration review, October 8](../../../data/maths/proposals/20261008-facet-registration-maths-review.md)    | A facet is rank one along its normal; plane sigma 7 mm at 8 returns against a 0.2 m half-extent prior; only a corner or a free-space-bounded edge adds tangent | **Do not build facet arms B to D yet.** F0: removing the extent shortfall takes end-face medians from −0.30 and −2.14 m to −0.02 m. Placement is fine; extent is wrong. geom0016 decides, on masks          |
| The same review, arm B (a body-local accumulated map, point-to-plane) and H5 (a mesh, 10 to 20 engineer-days)        | A persistent shape in the body frame, scored on views withheld from fusion                                                                                     | **Evaluate the measure now, the model later.** geom0019's P4 is the withheld-frame score in its cheapest form. A two-dimensional grid follows only if the one-dimensional profile passes; a mesh after that |
| Wahlström 2015, Kumru 2021 (Gaussian-process contours), random matrices                                              | A star-convex contour or an ellipse in the state                                                                                                               | **Leave as comparators**, as the facet review decided. A sensor that sees one or two faces cannot observe a closed contour, and the ellipse discards the corner                                             |
| Hong 2026; Zheng (NIF), briefing November 2                                                                          | Regularised and weighted least-squares rectangle fits; speed MAE 0.76 to 1.37 km/h against CAN bus                                                             | **Leave.** W1a's rectangle fit already reads 2.6° median against reviewed yaw. The fit is not the weak link                                                                                                 |
| Upganlawar, briefing November 9                                                                                      | The visible-surface centroid sits about 0.8 m from the box centre                                                                                              | **Already measured here**, smaller: 0.35 m on car 8 (geom0015). It is B3's bias and the reason the reference point waits on the extent                                                                      |
| [State plan §9](../../plans/lidar-state-estimation-plan.md)                                                          | A convex hull or surface residual is rejected until residual error above 0.1 m is attributable to edge localisation                                            | **The condition is not met**, by F0's figure. This is the plan's own gate against a mesh, and the evidence is on its side                                                                                   |

**The order for the body.**

1. **geom0019, the occupancy profile, as a shadow.** One change, default-off, no track moved.
   Its rule has two clauses against references and two that need none.
2. **Read the heading innovation before choosing a scale.** `P_ψψ` is a median 41 % of the
   innovation variance and the NIS is bimodal (median 0.10, p90 2.3). Bin the tail by speed,
   aspect and relabel events from the rows that exist. Engineering below; no spec until the
   tail is named.
3. **Gate widths on the far side being seen.** The benchmark's fix 5: the box takes `2hL` only
   with two-sided evidence and never `2hW` while the far side is unseen. The profile's
   observed-or-censored mark per end is the evidence that rule needs, which is why it follows 1.
4. **Only then the near-edge tracked filter.** It needs per-object AssA first (l5id0022), and a
   body whose extent is right.
5. **A two-dimensional body-frame grid, then a surface.** Conditional on 1 passing, with the
   withheld-frame score as the measure and a Pi cost bound stated before it is built: the
   solid body already takes Tracker.Update p99 from 1.7 to 2.2 ms up to 7.8 to 9.2 ms.

Throughout, the records describe measured extents and surfaces of a vehicle at a stated time, as
[the data-purpose note](../../geometry-priors-data-purpose.md) requires; the profile is kept in
memory for the track's life and nothing here changes what is persisted beyond the solid body's
existing row.

## Tracker accuracy: what to implement and evaluate

| Source                                                                                           | What it offers                                                                                                              | Verdict here                                                                                                                                                                                              |
| ------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Gap analysis S3 (likelihood cost against bare `d²`)                                              | A coasting track's `S` is 7.4, 20 and 34 times a fresh one's at 1, 3 and 5 misses, so it outbids it                         | **The measured win of the month.** 90 to 6 switches. Carried by l5id0023. Larger than any motion-model effect on record                                                                                   |
| Gap analysis K9 (innovation RMS 1.5 to 2.4 times expected along the track)                       | The case for a richer motion model                                                                                          | **Not reproduced on kirk0** (0.91, NIS 0.43 per dimension). Re-read on the corpus before anything is built on it: l5est0007 M1                                                                            |
| [Maths review §4.1](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md) | CV lags 0.71 m/s per 1 m/s² at q 0.2, 0.55 at 0.4, 0.43 at 0.8; velocity noise 0.13, 0.17, 0.23 m/s                         | **Evaluate q as the first alternative to CA.** l5est0007 M3 reads consistency at the three values; the trucks' 0.77 and 0.86 m/s shortfall is the size of this lag or of a sliding centre                 |
| The same review §4.3 to 4.5; offline CA, Singer coast rule and CV/CA IMM (built, #968)           | CA with no reversal while unobserved, τ 1.5 s; an IMM with dwell 6 s and 3 s, Student-t ν 4, `Π(Δt)` in capture time        | **Keep offline.** Every parameter is a prototype default. l5est0007 M3 names the one reading (IMM along-track ratio 0.8 or under on medoid residuals after q fails) that would make it the next increment |
| Schöller 2020 (gap analysis V2); G-EST-1 criterion 5                                             | CA is worst on final error when extrapolated                                                                                | **Held by the coast rule**, in simulation only. Reported in l5est0007; real gaps need occlusion scenes                                                                                                    |
| Gong 2025 (IMM for low-channel roadside LiDAR, 0.23 m position MAE)                              | An IMM result on a comparable sensor                                                                                        | **The one paper worth fetching this week**, for which modes it runs, what its truth was and whether its gain is on manoeuvres or everywhere. Unread; the briefing recorded it as "Confirm"                |
| Punzo et al. 2011 (briefing October 19)                                                          | Label-free trajectory checks: jerk above 15 m/s³, jerk sign changes a second, energy above about 1 Hz                       | **Implement as a reported measure** in `-gest1-report` (l5est0007's implementation step). It is the only check on over-fitting a looser filter that needs no reference                                    |
| Adari, ARC-SORT (briefing October 19)                                                            | Removing a hard Mahalanobis gate raised HOTA                                                                                | **Leave.** The gate here is 36, a tail of 1.5 × 10⁻⁸; it is already not a statistical gate (K10)                                                                                                          |
| State plan §7.3; pipeline review Q5                                                              | CTRV and CTRA deferred until turning exceeds 10 % of track-frames with residuals twice baseline                             | **Leave.** No report has measured the turning share; l5est0007's stratum table can carry it at no cost                                                                                                    |
| G-SMO-1 (fixed-lag RTS)                                                                          | Revision p99 0.63 m at every horizon against 0.3 m; whole-track RTS keeps 48 % of peak course rate at q 0.2 and 85 % at q 3 | **Wait for q.** Horizon and process noise are one decision, by the maths note's own words; l5est0002 should not run before l5est0007 reports                                                              |
| Cellina et al. (RTK-scored tracker, D-29); the November 5 review's E17                           | External truth for position and speed                                                                                       | **The only route to "accuracy".** Everything above measures consistency or agreement with fits to the sensor's own returns                                                                                |

**The order for the tracker.**

1. **l5id0023.** The association likelihood to its screen and its held-out read.
2. **l5est0007.** The corpus read that says whether there is a motion-model problem, and whether
   a process-noise value answers it.
3. **Initial velocity from a prior, not a pair.** The benchmark's restated fix 1: first-frame
   velocity variance from the class's speed range, `(v_max/2)²`. It has no reflection path and
   no reference-point slack. Worth building only with a capture that holds road users above
   14 m/s, so it waits on labelling item 5.
4. **CA or the IMM live** only on l5est0007's M3 reading, as one increment, with the coast rule
   and capture-gap prediction required.
5. **CTRV, a UKF, appearance re-identification:** nothing in the evidence asks for them.

## Superseded and revised work

| Item                                                                     | Change                                                                                                                        | Why                                                                               |
| ------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| Reacquisition and identity acceptance (backlog, v0.5.6)                  | Gains l5id0023 as the candidate that goes to label0003's spans; the overlap-end candidate becomes its comparator              | 6 switches at +2 brief tracks against 5 switches at +6 to +17                     |
| l5id0014 (birth gate on the live body), l5id0016 (overlap-end rescoring) | Reordered below l5id0023, not retired                                                                                         | They explain a lever that is no longer first. l5id0014's gate fired once on kirk0 |
| l5id0011 (likelihood cost with containment)                              | Its claim arms failed (l5id0018); its plain likelihood arm is l5id0023's. The spec should be restated or retired by the owner | Both guarded claim arms sit under the HOTA floor                                  |
| l5est0004 (looser process noise as an identity lever)                    | Restate after l5est0007, against the likelihood arm                                                                           | Its identity case was 90 to 43; the baseline it would now run beside reads 6      |
| Grid-map extent estimator as a shadow arm (backlog, W10, D-28)           | Points at geom0019, which carries it with a rule                                                                              | A backlog line since October 10 with no spec                                      |
| l5est0002 (trail from the whole-track smoother)                          | Waits on l5est0007                                                                                                            | Horizon and q are one decision                                                    |
| geom0012 (three centres a frame)                                         | Largely answered by geom0015's bias baseline; what remains is the tracked position's row. Owner to narrow or retire           | geom0015 measured the cluster and box centres against the same references         |
| Facet arms B to D, the facet operator pilot (D-32, parked)               | Stay parked, with a measured reason                                                                                           | F0's shortfall figure; 20 to 40 operator hours against a defect that is extent    |
| The near-edge tracked filter's promotion path (F8, perf0004)             | Blocked behind per-object AssA and the extent                                                                                 | l5id0022: no control recovers the loss                                            |

## Engineering work needed

| Work                                                                                                | Size     | Reads it                     | Why it matters                                                                       |
| --------------------------------------------------------------------------------------------------- | -------- | ---------------------------- | ------------------------------------------------------------------------------------ |
| `robust_birth_gate` accepts `likelihood_cost` as its prerequisite                                   | S        | l5id0023                     | The simpler arm cannot run without it                                                |
| A span reader in `lidar-ground-truth-eval` for a frozen span split: covered, one-track share, count | S        | l5id0023 K5                  | The first held-out read has no scorer                                                |
| A mask-free span record in the annotation window, frozen with a role                                | M        | label0003                    | Open since November 5; halves the first truth's cost                                 |
| `-gest1-report`: restated criterion 2, the stratum table, the jerk checks                           | S {math} | l5est0007                    | The screen's report asked for the restatement                                        |
| `SolidBodyOptions.ExtentProfile` and the withheld-frame score in `lidar-box-eval`                   | M {math} | geom0019                     | The extent estimator that keeps free space                                           |
| Per-object rows from `perframe` (AssA, switches, fragmentations by reviewed object)                 | S        | the next near-edge spec      | l5id0022's stated precondition; also shows which of 19 objects carry any kirk0 delta |
| A heading-innovation tail read from existing rows: speed, aspect, relabels, restarts                | S        | shared orientation           | Names the defect a scale cannot fix                                                  |
| A code identity for replay options in the manifest and the parameter hash                           | S        | every spec with a replay arm | Two builds of one option share a hash twice this week                                |
| A half-extent floor test for the zero-length rows `join` wrote                                      | S        | l4clus0010                   | Three 0.0 m rows reached the output                                                  |
| `HeadingNIS` persisted                                                                              | S        | G-UNC-1's aspect strata      | It is computed and dropped                                                           |
| A `reflection` class in the label set                                                               | S        | labelling item 4             | Eight tracks have an inferred cause and no label                                     |
| A range-aware width floor for small classes in the accrual record                                   | S        | geom0018 restated            | Only if geom0019 fails and accrual stays the extent arm; otherwise leave             |

## What the papers say to do, and what to leave

The two tables above are the reading. In one line each: the papers support the occupancy grid for
extents (Schäfer), give a label-free plausibility check for trajectories (Punzo), and leave the
IMM's case on this sensor to one unread source (Gong 2025). Fetch Gong 2025 this week, for its
modes and its truth. Nothing else should be read for this goal: the Gaussian-process and
random-matrix trackers, the rectangle fits and ARC-SORT have verdicts, and the evidence that
decides the rest is in the corpus, not in print.

## Decisions for the owner

| Decision                                                           | Recommendation                                                                                                                                          |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Approve l5id0023, l5est0007 and geom0019                           | Yes. Two are label-free; the third changes no track. No default is at risk                                                                              |
| Approve label0003 and do its 2h15m this week                       | Yes, first. Proposed November 5, window cut November 8; three specs now name it                                                                         |
| Order label0001's masks: long vehicles and one adjacent pair first | Yes. It turns seven hours into a first useful hour                                                                                                      |
| Reset the join's five-point bar                                    | To a share of what remains: a quarter of `a0`'s fragmented truck instants. `fix` would pass (198 to 145); `join` would not (198 to 182)                 |
| Ship any motion model, CA or IMM                                   | No, until l5est0007's M3                                                                                                                                |
| Ship the likelihood cost on its kirk0 pass                         | No. In sample by its own report; wait for K2 and K5                                                                                                     |
| Rewrite deployed observation speeds after the row fix              | The owner's; the review recommends recording the fix's commit in each affected run's provenance and leaving stored rows, until a speed reference exists |
| Restate or retire l5id0011 and geom0012                            | Restate l5id0011 to its plain-likelihood arm or retire it in l5id0023's favour; narrow geom0012 to the tracked position's row                           |
| Schedule the radar beside the LiDAR on a faster street             | Yes. It now serves three things: a speed bound, two-point initialisation's population, and the trucks' lag                                              |

## What this review does not establish

It ran nothing. Every figure is a report's, and the reports were read through digests for all
but the November 5 review, the maths review and the specs' answers, so a figure quoted here
should be checked against its report before it is quoted onward. The claim that the likelihood
cost supersedes the overlap end compares two in-sample kirk0 results on 19 objects; neither has
been read on truth off kirk0. The claim that the extent is the wall rests on three vehicles, two
of them trucks, with references that are fits to the sensor's own returns. The occupancy profile
is a prediction: no code exists, and its rule's P1 can be decided by six poses within two
seconds. The reading of K9 against l5est0005 as a contradiction may dissolve on populations: K9
read moving tracks at the sites, the screen read 43 tracks on a quiet street. Person times for
spans and reflection labels are estimates by analogy; no operator has timed either. The briefings
were read for verdicts, not the papers; Gong 2025 has not been read by anyone here.

## Provenance

- Repository at `b8ea18d` (main, 2026-11-09), branch `claude/serene-bardeen-16jxue`.
- `go run ./cmd/tools/experiments next` at that commit: 40 experiments open; approved are
  l4clus0008, l5id0011, l5id0013, l8fol0004, perf0001 and perf0004; label0001, label0002 and
  label0003 proposed.
- Read in full: the November 5 review, the November 8 maths review, the answers of l5id0015,
  l5id0017, l4clus0009, figure0002, l5est0003 and geom0014, and the specs l5est0005, l5est0006,
  l5id0019 and l5id0021.
- Read by digest (question, arms, figures, rule, recommendations, limitations): the eleven
  reports under Inputs, the F0 attribution report, the mean-centre report; the state estimation,
  geometry convergence, near-edge and sprint plans; the facet annotation spec; the labelling
  tool documents; the briefings of October 9 to November 9; the gap analysis, tracking maths and
  motion-models maths.
- Tuning fingerprints pinned in the new specs: defaults `ab0c4035780d`,
  `process-noise-vel-x2.json` `deb48e282fa2`, `process-noise-vel-x4.json` `e026f818d678`.
- No raw output was produced or read; nothing under a `results` directory was touched.
