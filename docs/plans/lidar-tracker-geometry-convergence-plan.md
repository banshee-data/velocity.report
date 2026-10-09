# Tracker geometry convergence: a heading that converges and a box that holds its points

<!-- ignore-style-length -->

The [solid-body alignment run](../lidar/operations/solid-body-physical-alignment-kirk0-2026-10.md)
corrected two trucks on kirk0 with four opt-in experiments and held the corpus's lateral tail.
It also showed that the corrections are patches on an estimator with two structural gaps: it has
no observation of heading, so a wrong heading persists for the life of a track, and nothing ties
the box it reports to the points it was measured from, so half its body-centre rows sit off
their own returns. This plan states both gaps with measurements, sets the levels the tracker
must reach, designs the two observation models that close the gaps, maps the work already
planned onto them, and schedules what remains.

- **Status:** Proposed, 2026-10-09. Measurements from the alignment run's evidence; no code
- **Target:** v0.5.2, Sprint 0.5.2.1 for the two observation models and the gates; v0.5.6 and v0.5.7 for the upstream and estimator work they expose
- **Layers:** L5 tracker (heading, solid body), L3 foreground and L4 clustering (as inputs), offline evaluation
- **Canonical:** [LiDAR pipeline reference](../lidar/architecture/lidar-pipeline-reference.md)
- **Design basis:** [state estimation plan](lidar-state-estimation-plan.md) §5.6, §9.2, §9.3; [visibility-aware review](../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md) §3.2, §4.1, §5.2, §7.1
- **Related:** [alignment report](../lidar/operations/solid-body-physical-alignment-kirk0-2026-10.md), [alignment plan](lidar-solid-body-physical-alignment-plan.md), [near-edge tracked state](lidar-near-edge-tracked-state-plan.md), [heading coherence sprint](lidar-heading-coherence-sprint-plan.md), [D2 implementation report](lidar-heading-d2-implementation-report.md), [facet registration experiment](lidar-facet-registration-experiment-plan.md), [physical-reference review](lidar-physical-reference-review-plan.md), [kirk0 pilot](../lidar/operations/physical-reference-pilot-kirk0-2026-10.md)

## 1. What occurred

The pilot froze 22 reviewed poses of three vehicles and found the solid body within 0.25 m of a
car and nowhere near two trucks. The alignment run traced that to three mechanisms inside the
solid body and two upstream of it, built an experiment for each inside mechanism, and scored
them. Read as symptoms of the two gaps above:

| Finding, from the alignment report                                               | Which gap                                                                                                       |
| -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| Truck 2's heading 76° to 126° off its course for its whole life                  | No heading observation. The PCA axis of a face strip is seeded at birth and nothing can turn it by 90°          |
| Car 8 at 0.8 to 1.7 m/s, two poses 87° and 90° off                               | The same, below the course speed where the one correction available does not act                                |
| Truck 2 0.625 m long from its first frame; truck 1 5.9 m of 10.6                 | A span is admitted as a dimension and the box reported at it, shorter than the points it was measured from      |
| Growth admission's lengths started end-face-only fixes that drifted 3.1 m across | A fix moves the body along a face's normal only; across it the box leaves the points and nothing brings it back |
| Centring's one-frame 0.8 m return at haight; the 1.5 m jump at side-face entry   | The box returns to the points only when a new face happens to constrain the direction it drifted in             |
| Identity under A2 moves by up to 39 switches with no physical change             | Every geometry change alters which cluster the body claims, because the box, not the points, gates association  |

The experiments that worked, the course heading and growth admission with centring, each put a
proxy in the place of a missing observation: the velocity in place of a heading measurement, a
face's own span in place of a containment constraint. They are kept as the candidate, and this
plan is what replaces them.

## 2. The two gaps, measured

All figures are from the alignment run's kirk0 evidence at `0011ff8dc` and `be79fbd53`, and the
23-site corpus passes, read from the retained 256-point samples joined to the solid-body rows.
"A2" is `near_edge_track`; "candidate" is A2 with the four kept experiments. Scripts live with the
run's other raw outputs, outside the repository.

### 2.1 Heading error does not fall with evidence

Axis error against the body's own course, folded to [0°, 90°], for rows moving at 2 m/s or more,
by track age. The course is not truth (§3.2 of the visibility-aware review). It is independent of the PCA
computation, not of the clusters: velocity and PCA come from the same associated clusters, and
under A2 the rows a wrong heading lost to association are absent from the table. Its own noise
floor is about 8.6° at 2 m/s (σ_v ≈ 0.3 m/s across the course), so a 5° median against it is an
upper bound on the fit's error, not a measurement of it. On kirk0 it agrees with the reviewed
yaw within 6° for the car and truck 1.

| Age      | Corpus A2 median / share over 30° | Corpus candidate | kirk0 A2 median: believed / raw PCA | kirk0 rectangle fit (§4.1), mod 90 |
| -------- | --------------------------------: | ---------------: | ----------------------------------: | ---------------------------------: |
| 0 to 1 s |                        33° / 52 % |        6° / 13 % |                           28° / 23° |                                 5° |
| 1 to 2 s |                        16° / 33 % |         5° / 5 % |                           14° / 17° |                                 5° |
| 2 to 4 s |                        13° / 23 % |         5° / 4 % |                           35° / 49° |                                 5° |
| over 4 s |                        15° / 27 % |         6° / 6 % |                           23° / 52° |                                 5° |

Three things to read from it:

- **The tracker's heading does not converge.** After two seconds a quarter of the corpus's
  moving rows are still more than 30° off, and on kirk0 the median error is no better at four
  seconds than at one. The raw PCA input gets worse with age, because far clusters are thin.
- **The estimator has no mechanism that could make it converge.** Per-frame headings are the
  PCA axis of the cluster, smoothed by an EMA (α = 0.08) behind three guards. The aspect lock
  passes a face strip (aspect 0.84 against a 0.25 threshold). The velocity and flip rules act
  modulo π, so neither can turn an axis by 90°. The jump guard rejects exactly the 90°
  correction, and its five-rejection release is reset by any accepted frame in between. In the
  corpus 1.4 % of consecutive rows turn by more than 60°: the guard releasing, late and all at
  once. The solid body takes this heading as its orientation; the near-edge faces are then
  searched along it, so the faces carry no orientation information back
  ([measurement_model.go:194-206, 253](../../internal/lidar/l5tracks/measurement_model.go)).
  `minimumAxisSpan` searches ±10° for the tightest span and discards the angle it found
  ([solid_body_nearedge.go:1313](../../internal/lidar/l5tracks/solid_body_nearedge.go)).
- **The candidate's 5° is the course measuring itself.** Under `solid_body_course_heading` the
  orientation is the velocity, so its agreement with the course is nearly tautological; where
  the velocity is itself wrong, at an end-face-only fix, the heading confirms it (the
  california-leon-baker drift). The 6 % of rows still over 30° are below 4 m/s, where the
  blend is partial, and in turns.

Against the 21 reviewed poses with a yaw, the believed heading on A2 is 7.5° off at the median
and 89° at p90.

### 2.2 The reported box does not hold its points

For every body-centre row, project the frame's retained points into the reported box
(position, heading, believed length and width) and count those outside it by more than
0.15 m.

| Rows referenced to the body centre      | Shadow |     A2 | Candidate |
| --------------------------------------- | -----: | -----: | --------: |
| More than 10 % of points outside        |   50 % |   54 % |      42 % |
| More than half the points outside       |   20 % |   19 % |       9 % |
| Median of the farthest point's distance | 0.59 m | 0.62 m |    0.42 m |
| p90 of the farthest point's distance    | 2.09 m | 1.60 m |    1.18 m |
| Cluster's own box centre inside the box |   85 % |   84 % |      93 % |

Rows still referenced to the cluster medoid are centred on their points by construction, yet
a quarter of them have a tenth of their points outside: the believed box is smaller than the
cluster.

Where the box sits, by what fixed it (A2; the candidate in brackets):

| Fix                   | Rows | Observed span along the axis exceeds the believed length by over 0.25 m | Median shortfall when it does | Median offset of the cluster's across-midpoint from the box centre |
| --------------------- | ---: | ----------------------------------------------------------------------: | ----------------------------: | -----------------------------------------------------------------: |
| Corner (end and side) |  603 |                                                             34 % (12 %) |                 0.41 (0.20) m |                                                      0.24 (0.25) m |
| End face alone        |  110 |                                                             66 % (25 %) |                 1.08 (0.25) m |                                                      0.15 (0.09) m |
| Side face alone       |  167 |                                                             53 % (36 %) |                 1.40 (0.70) m |                                                      0.14 (0.28) m |

Two mechanisms, both by design:

- **A fix constrains one direction per face.** The near-edge update is one scalar update along
  each face's normal, the plan's invariant 2 ("a rank-one frame moves nothing across its
  normal"). The visibility-aware review §5.2 is right that a flat side observes nothing along
  itself. But the _points_ observe something a plane does not: the body cannot be where they are
  not. A box whose far edge lies 1.4 m inside the cluster's span is contradicted by the frame,
  and today nothing says so.
- **A dimension is the third-largest admitted span, reported as the body.** The extent belief
  is a corroborated maximum (0.25 m bins, three observations), and the box is drawn at it. Until
  three frames have seen longer, the box is shorter than this frame's cluster: 66 % of end-face
  fixes on A2. The state plan §9.2 says a partial span is a lower bound; the reported box does
  not honour the bound it has just measured.

What a containment constraint would do on A2's body-centre rows (points trimmed 2 %, +0.15 m):

| Axis, fix              | Already consistent | Would shift the centre (median, p90) | Would grow the reported extent (median, p90) |
| ---------------------- | -----------------: | -----------------------------------: | -------------------------------------------: |
| Across, corner         |               10 % |                  66 % (0.13, 0.57 m) |                          24 % (0.42, 1.03 m) |
| Along, corner          |               15 % |                  45 % (0.14, 0.45 m) |                          41 % (0.71, 2.93 m) |
| Along, end face alone  |                5 % |                  20 % (0.14, 0.64 m) |                          75 % (1.11, 2.08 m) |
| Along, side face alone |               19 % |                  26 % (0.59, 0.82 m) |                          55 % (1.41, 1.57 m) |

It would act on nearly every row. That is the measure of the gap, and also two warnings. A
constraint that fires every frame is an observation model, not a clamp, and must enter the
filter as one or it will fight the prediction and show up as lateral jitter. And a span measured
along a wrong axis overstates: across an axis θ off, the observed width is W cos θ + L sin θ,
0.87 m extra for 5° on a 10 m truck, so part of the corner rows' "grow" is the heading error of
§2.1 read as an extent, and the floor in §4.2 must use the window-minimum span, which understates
but cannot overstate.

### 2.3 The orientation is in the data every frame

A rectangle fitted to each frame's retained points, searching the axis over [0°, 90°) in 1°
steps for the orientation that puts the most points nearest an edge (the closeness criterion of
`Zhang2017` §III, the L-shape fit the facet maths review §7 item 6 proposes for corners), gives
the body axis modulo 90°:

| Against                               | Rectangle fit, axis | Raw PCA, axis | Believed heading, directed |
| ------------------------------------- | ------------------: | ------------: | -------------------------: |
| Course, kirk0, all ages, median / p90 |            5° / 20° |     10° / 33° |      10 to 17° / 27 to 39° |
| Reviewed yaw, 21 poses, median / p90  |         2.6° / 4.1° |   10.7° / 15° |               7.5° / 89.4° |

It holds on truck 2's end-on strips (0.4 to 0.8 m deep, 27 to 121 points: axis within 0.4° to
4.8°) and on car 8 at 2 m/s. It fails where it should abstain: car 8 at 0.8 m/s, a 1.6 × 1.4 m
view, 31° off. Four caveats belong beside the 2.6°. The effective sample is three vehicles, one
of them end on throughout, not 21 independent poses; the p90 is the 19th of 21 values (19 of 21
under 4.1°). The references' own yaw bound is 3.7° at the mean, so the fit is within the
references' resolution rather than measurably better than it. The references are fits to the
same reviewed returns the tracker reads, so this compares a fit with a fit; W7's references must
be operator-keyframed to break that. And §4.1 calibrates the fit's variance on these poses, so
nothing here is held out. What the fit does not give is which of the two axes is the length. For truck 2's
strip the longer span runs across the truck. That labelling is the one ambiguity, and it is the
job the D2.1 axial selector already does with the extent belief's aspect, on an input (the PCA
box) whose axis is wrong by 10°.

## 3. What acceptable looks like

No numeric physical-heading threshold exists in any plan today, and G-GEO-1 has no heading or
containment row. These are proposed levels, to be frozen as part of the predeclared physical
scoring criteria (Sprint 0.5.2.0) before any held-out score, and amended only in writing before
that score. "Established" is the lifecycle state of state plan §5.6; "reference bound" is the
pilot's per-pose bound, which is the resolution of the score: a level tighter than it cannot be
read. Axis error is modulo 90° and no labelling touches it; directed error is modulo 360° and
includes the label and the front.

| Measure                                 | Level                                                                                                                                                                                                                                                            | Today (A2 / candidate, kirk0)                             |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| Axis, established rows at 2 m/s or more | Error beyond the reference's yaw bound: zero at the median, ≤ 10° at p90; the bound (3.7° mean) printed beside it. The course candidate already meets this on kirk0's straight passes, so the discriminating cases are under 2 m/s, in turns and at rest         | 7.5° / 89° directed; axis not scored today                |
| Directed heading, the label and front   | ≤ 10° on at least 90 % of poses, scored separately from the axis so a label error (a right axis with the wrong length) is seen as one                                                                                                                            | Truck 2 76° on A2                                         |
| Heading convergence, label-free         | Axis error against the course, rows older than 2 s: more than 30° off ≤ 5 %, with the course's noise floor stated; the directed error reported beside it as a labelling check only                                                                               | 23 to 27 % corpus (directed); axis not reported           |
| Heading ambiguity                       | A row whose label is unresolved says so and reports the observed spans, never a labelled body                                                                                                                                                                    | Reported resolved with weight 0                           |
| Containment, invariant check            | Every body-centre row holds ≥ 98 % of its retained points within 0.15 m, stratified by range (ring spacing exceeds 0.15 m beyond about 40 m); rows that cannot are flagged. The constraint meets this by construction, so it is a check that it ran, not a score | 46 % / 58 %                                               |
| Reported extent                         | Never below the frame's window-minimum observed span along the reported axes; the belief's extent scored separately against the reference, not the floored value                                                                                                 | Below the span in 34 to 66 % of fixes                     |
| Centre                                  | ≥ 80 % of scored poses within the reference's own centre bound, truck 2 scored separately until its cab reaches the tracker (W3)                                                                                                                                 | 26 % / 52 %                                               |
| Length, established cars; trucks        | Belief within 15 %; within 20 % of the reference on ≥ 80 % of poses, truck 2 separately                                                                                                                                                                          | Car 3.88 of 4.26 m; trucks 8.6 of 10.6, 0.6 of 9.6 m      |
| Following gap, both bodies established  | Within the reference's own gap bound (about 1.2 m on kirk0) on every scored gap; p95 cannot be read from six gaps                                                                                                                                                | 1.15 / 1.60 m mean                                        |
| Lateral, G-GEO-1                        | Criteria 1 to 4 scored as written; W1 and W2 are not expected to pass criterion 1 (a 50 % p99 cut) on their own and must not raise p99; criterion 3 (fragmentation) is what W3's fix moves                                                                       | Candidate −10 % p99 at the corpus median                  |
| Identity on the frozen split, A2 only   | ID switches within 10 % of the arm's baseline above a measured no-op noise floor, predeclared in W0; HOTA and IDF1 reported but not the gate, since 39 switches moved HOTA under 0.005                                                                           | Switches 78 to 117                                        |
| Cost                                    | `Tracker.Update` p99 within 10 % of the arm's baseline; the fit's operation count stated and its time measured on the Mac and on a Pi, per candidate pair under A2                                                                                               | Candidate −25 % p99, +19 % median on a Mac; Pi unmeasured |

Three things are deliberately not here. A rate for the deployed system needs the held-out split
that physical scoring still refuses, and a second capture with an end-on truck (W7). The
slope-aware ground surface (P11) and any later L3 or L4 change move G-GEO-1's inputs, so the
final W1 and W2 score comes after P11 and is reopened by W3's fix. And nothing above is a
promotion gate on its own: the chain G-GEO-1, G-UNC-1, G-SMO-1 stands, and G-UNC-1's
calibration table is refrozen after W5, since W1 and W2 change its residuals.

## 4. What is needed

Two observation models, each default-off behind an experiment name, each tested on the synthetic
passes, scored on the frozen kirk0 split and screened on the corpus like the alignment
experiments were. Then the estimator and upstream work they expose.

### 4.1 An orientation observation from the cluster

Every frame, fit the rectangle orientation to the associated cluster's retained points and
treat it as an observation of the body axis.

- **The fit.** Search θ over [0°, 90°) at 1° then 0.25° around the best, for the closeness
  criterion of `Zhang2017` §III; report θ, the two edge spans s₁ and s₂ with the support n₁ and n₂
  on each, and the plateau: the set of θ within a stated fraction of the best score. The score is
  a clamped sum of reciprocals with a non-smooth maximum, so its curvature is not a variance; the
  plateau is the per-frame spread. Cost: two passes over at most 256 points at each of about 100
  orientations, some 50,000 point operations plus the refinement, nearer 0.3 ms on a Mac and
  several milliseconds on a Pi, per candidate pair under A2; measured, not assumed.
- **Variance.** σ_θ² = max(σ_floor², c · 12σ_r² / Σ_k n_k s_k²): the noise-only line-fit floor
  summed over the edges (for a 2 m edge of 27 points at σ_r = 0.05 m, about 1°), under a shape
  floor of 2° to 3° for what a vehicle is not (bumpers, mirrors, a bonnet seen from 2.3 m up, the
  across-face band from ring spacing), with c calibrated on the 21 poses and the corpus courses
  and stated as a bounded scale, not a posterior. Consecutive frames see the same vehicle at
  nearly the same aspect, so the heading filter carries a variance floor or process noise, or N
  correlated frames would claim σ/√N.
- **The state is the axis.** What is observed is the axis modulo 90°, so that is what is
  filtered: one continuous state θ with the residual r = ¼ · atan2(sin 4Δ, cos 4Δ), updated every
  frame the fit does not abstain. The label (which of θ and θ + 90° is the length) and the front
  (± 180°) are two discrete weights beside it, and the reported yaw is ψ = θ + k · 90° + m · 180°.
  That is state plan §5.6's bimodal belief with one more bit, and the D2.1 selector's structure
  on a measured axis. A modulo-180° Gaussian fed a modulo-90° observation would need the label
  chosen first, and a wrong choice gives a 90° residual that is either gated (the jump guard
  reborn) or drags the belief toward 45°, which review §4.1 forbids. Two hypotheses are two
  weights on one filter, not two filters: fed the same θ they could never diverge.
- **Labelling.** The label weight moves on the extent belief's aspect when it is evidence, then
  on the course at 2 m/s or more; the front on the course or an asymmetric cue, as now. W1 owns
  this rule and D2.1 consumes it, not the reverse: D2.1 is v0.5.7 work that failed a four-site
  A/B on its PCA input. Below 2 m/s the course from displacement and the class prior's aspect
  are the only cues, and a near-square view has none: the weights stay split, the row is marked
  ambiguous, and it reports the observed spans. This is what car 8 at 0.8 m/s should have been.
- **The position update under ambiguity.** The two labels put the implied centre (L − W) / 2
  apart along the near face's normal, about 4 m for a truck. While the label is unresolved the
  face update uses the span-floored, label-free half-extents of §4.2, and the row says so. Without
  that rule an ambiguous frame has no position update at all.
- **Abstention, two decisions.** The axis abstains when σ_θ exceeds about 10° or the plateau is
  wider than about 15°, after the score is normalised by n and the clamp; the label abstains
  when the aspect is under 1.25 whatever the spans, since a 2.5 × 2.2 m view is as square as
  1.6 × 1.4. A partial corner (1.8 m of front, 2 m of side) has a sharp axis from two edges and
  no label; it must not abstain on the axis.
- **Faces along the observed axis.** The near-edge faces are found along this frame's θ, not the
  previous belief, so their normals agree with the points they are fitted to. For the solid body
  the EMA and the three guards retire under the experiment. Heading error then decays at the
  filter's rate, which is the convergence the first table lacks, and extent admission below 2 m/s
  has an axis it can trust.

### 4.2 A containment constraint: the box holds the points

The frame's points are known to lie on the body. The reported box must contain them, and the
estimate should be pulled toward boxes that do. The two are separate rules, and keeping them
separate is what makes the second one sound.

- **The reported box is floored along the hull, and the state is not.** The persisted and drawn
  length and width are never below this frame's window-minimum observed span (`minimumAxisSpan`,
  which cannot overstate along a wrong axis); the belief keeps its corroboration rule and never
  reads the floor, since a floor that fed the belief would corroborate a merge. This alone
  removes the rows whose box is shorter than their cluster, and it changes no estimate.
- **The interval is a one-sided bound with the belief's upper tail.** For a body axis with
  belief D and upper tail D_upper, the points' trimmed span [lo, hi] bounds the centre to
  [hi − D_upper / 2, lo + D_upper / 2]. Where D is below the span the frame is extent evidence,
  not a centre measurement: an interval built from a floored half-extent collapses to the
  cluster midpoint with a confident variance, which is the biased seed §5.6 calls unrecoverable
  and the reconstructed centre invariant 2 forbids. The bound's variance is the trimmed extreme's
  sampling spread (point spacing at range, 0.1 m across azimuth at 30 m, more across rings) plus
  projected range noise plus ((D_upper − span) / 2)², the censoring term of review §7.1 and the
  shape `endFaceCentringTerm` already uses; `measurement_noise` (0.05 m² default, 0.15 m²
  optimised) is the medoid's variance and not a stand-in for it.
- **The update is a truncation, not a gain.** Treating an inequality as a one-sided scalar
  measurement toward the nearer bound is the pseudo-measurement form of a constrained filter:
  information only when violated, covariance shrunk as if measured, an innovation that is
  one-signed and not chi-square (the code already keeps loose terms out of NIS). The better
  founded form is density truncation (`Simon2010`): truncate the Gaussian marginal along the
  axis to the interval widened by the bound's σ, and take its mean and variance. It has no gain
  to tune, moves the state by at most its own σ, and is a no-op well inside the interval; the
  censored likelihood of review §7.1 is the matching form for the extent (`Xia2021` is the
  truncated-Gaussian precedent in the references).
- **Position only.** `scalarPositionUpdate` reaches velocity through P's cross terms, so a
  repeated one-sided pull would turn the across velocity and, under course labelling, the
  heading. The truncation applies to position with the velocity rows zeroed, on invariant 3's
  "translation, not innovation" pattern, and records the across-velocity change it did not make.
  The five-point residual registers a per-frame move above about 0.1 m, and §2.2's median shift
  is 0.13 m: the bound by the state's own σ is what keeps the constraint off that residual.
- **Lapse within the hull.** A body-centre claim no longer lapses to the medoid while the box
  can be held on the points; it lapses when there are no points. When it must re-seed, it seeds
  inside the interval, not at the biased medoid.
- **The invariant and its metric.** Every persisted solid-body row carries its containment
  share; the solid-body summary reports the share of rows under 98 %, and the scorecard carries
  it as a check that the constraint ran, stratified by range. A large box satisfies it
  trivially, so it is a guard beside the extent level, never a score. The near-edge plan's
  invariant 2 should read: a plane observes nothing along itself, and a one-sided containment
  bound with its own provenance is admitted, under admitted membership and a resolved axis only.

The two models are one change in practice: containment along a wrongly oriented axis would pull
the box the wrong way, and an orientation observation without containment leaves the box off
the points along the open direction. Score W2 alone first, then with W1, as the alignment run
scored its pairs.

### 4.3 What they expose

- **Extents per §9.2.** With the reported box floored at the observed span, the belief's job is
  the unseen remainder. The corroborated maximum stays as the guard against a one-frame merge,
  and the merge test reads the belief before the floor, never after it: otherwise a merge that
  persists three frames corroborates the width, the floored width admits the merge, and the
  interval pulls to the merged midpoint. Beyond that, the test should become
  containment-consistent: a cluster whose window-minimum across-span fits the belief's upper tail
  is one body, however large its area ratio against the running mean; growth admission is the
  first version of that rule. The censored likelihood and the revisable admitted-frame record of
  §9.2.2 and §9.2.3 are the estimator; they are not changed here.
- **Upstream retention and fragmentation.** Truck 2's cab never reached the tracker and truck
  1's cab was two separate clusters. No observation model can place a body on points it was not
  given. The reviewed pack can score this today: for each reviewed mask, the share of its
  returns in the associated cluster, and the number of clusters its returns fall in. Those two
  numbers are an L3 and an L4 gate, and the cases they find are the levers' evidence: the
  region-override window (at 20 m about 5 m, so returns within a fifth of the background range
  are background), the deadlock breaker that absorbs a slow body, and DBSCAN's single `eps`
  with no gap tolerance across a dropped band of returns. D3's near-merge cases are the same
  measure read the other way, and belong in it.
- **Identity under A2.** A2 gates association on the body's prediction, so every geometry change
  moves identity. Deliver both models on the shadow first, where identity cannot move, and score
  the physical gain there; then A2, with the identity level above as its own gate. The
  association cost item (S3) and the reacquisition acceptance item are where like-with-like
  gets decided; they should take the containment share as an input, since a cluster the box
  cannot hold is a cluster the body should not claim.
- **Live parity.** The live pipeline retains no points (`max_sample_points` 0, `KeepMembers`
  false), so no face, no fit and no containment exist on the device today. The v0.5.6 live
  retention item is on the critical path for any of this to leave replay.

## 5. Upcoming work that addresses these

| Planned item                                                                            | Where                                                                                                                              | What it covers                                                                                       | What it leaves                                                                                                       |
| --------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Heading candidate acceptance: D2.1 axial selector, D2.4 run panel                       | [BACKLOG v0.5.7](../BACKLOG.md#v057---tracker-correctness--robustness-057), [D2 report](lidar-heading-d2-implementation-report.md) | Labelling the axis by aspect against the extent belief, abstention, the observed envelope            | Its input is the PCA box, 10° off at the axis, and it failed a four-site A/B; W1 owns the rule and it consumes W1    |
| Solid-body extent accumulation                                                          | [BACKLOG 0.5.2.1](../BACKLOG.md#sprint-0521-physical-body-and-corrected-geometry)                                                  | Growth admission and the vehicle floor as the opt-in candidate                                       | Superseded: the reported box is W2's floor, the belief is W4                                                         |
| Phase 2 corrected measurement, G-GEO-1                                                  | BACKLOG 0.5.2.1, [state plan §9.3](lidar-state-estimation-plan.md#93-decision-gate-g-geo-1)                                        | The lateral criteria the constraint must not break; criterion 3 is what W3's fix moves               | No heading or containment row; §3 proposes them. Criterion 1 is not reached by anything here                         |
| Face-transition remedy, S2.1 (gap 1.9 against 1.25, unchosen)                           | [near-edge plan](lidar-near-edge-tracked-state-plan.md#face-transitions)                                                           | The 1.5 m jump at side-face entry                                                                    | §4.2's interval is a candidate remedy: the jump is the across position returning late                                |
| Physical scoring criteria and held-out acceptance                                       | BACKLOG 0.5.2.0                                                                                                                    | Predeclared limits on the tuning references                                                          | Takes §3's rows (W0)                                                                                                 |
| Facet registration E1 to E3, arm B (17 to 29 days)                                      | [facet plan](lidar-facet-registration-experiment-plan.md), BACKLOG 0.5.2.1                                                         | Pose from point registration: the structural successor to faces, and the road to G-GEO-1 criterion 1 | The F0 report asked for the cheaper step first; W1 and W2 are it, and the facet gate reads W2's floored extent       |
| Geometry-coherent tracking, D-04 (#391)                                                 | BACKLOG v0.5.7                                                                                                                     | Persistent geometry state in association; targets drift under 0.5°/s, 90° jumps under 1 %            | Waits for a corrected-body baseline; W1 is what would reach its targets                                              |
| Live cluster point retention                                                            | BACKLOG v0.5.6                                                                                                                     | Points on the live path                                                                              | Critical path for live parity of everything here                                                                     |
| Long-range foreground sensitivity; D3 near-merge and adaptive `eps`; slope-aware ground | BACKLOG v0.5.7, 0.5.2.1                                                                                                            | The L3 window and the L4 split, each from its own evidence; P11 before the final score               | No measure against reviewed masks; §4.3's gates supply one, D3 folds into it, and truck 2 is its case                |
| Association cost S3; reacquisition and identity acceptance                              | BACKLOG 0.5.2.2                                                                                                                    | Like-with-like association under A2                                                                  | Take the containment share as an input (W5)                                                                          |
| Option A to B: heading into the state (deferred gate)                                   | [state plan §7.3](lidar-state-estimation-plan.md#73-recommendation-and-gate)                                                       | Waits for orientation variance to be shown as the limiting term                                      | §2 shows it: an end-on truck at 76° has a box IoU of 0.05 whatever its position; W1 keeps the axis a separate belief |
| L-shape fitting                                                                         | [AV integration plan](lidar-av-lidar-integration-plan.md) Phase 6, deferred                                                        | Listed as a future bounding-box method                                                               | Brought forward as the orientation observation, not a box method                                                     |
| 0.5.2 MVP exit B                                                                        | [MVP sprint plan](lidar-052-mvp-sprint-plan.md#evidence-and-promotion-ledger)                                                      | Qualified 0.5.2 result through G-GEO-1, G-UNC-1, G-SMO-1                                             | Exit A does not wait on this chain; exit B does, and the MVP plan now says so                                        |

## 6. Work to scope and schedule

| Item | Work                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | Size              | Sprint                          | Depends on        |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------- | ------------------------------- | ----------------- |
| W0   | Predeclare the §3 levels with the physical scoring criteria, including the identity band above a measured no-op noise floor; add the containment share, the axis-versus-course table by age and the reported-against-believed extent to the solid-body summary and the scorecard, so every arm reports them; committed to main before any W1 or W2 score                                                                                                                                                                         | S                 | 0.5.2.0                         | Nothing           |
| W1a  | The fit and its variance (§4.1): the closeness search, plateau, σ_θ with its floor and calibration, axis abstention; unit tests on strip, corner, square, sparse and reversing passes; the fit's cost measured on the Mac and a Pi                                                                                                                                                                                                                                                                                               | M {math}          | 0.5.2.1                         | W0 for the score  |
| W1b  | The axis state and labelling (§4.1): the modulo-90° filtered axis with its process noise, the label and front weights with their rules at and below 2 m/s, the ambiguous row and its span-floored face update, faces along the observed axis, the guards retired; experiment `solid_body_rectangle_heading`; scored on the frozen split and the corpus; car 8's two slow poses are the low-speed case                                                                                                                            | L {math}          | 0.5.2.1                         | W1a, W2's floor   |
| W2   | Containment (§4.2): the floored reported box, the truncation update with the belief's upper tail and position-only rows, lapse within the hull, the per-row share; experiment `solid_body_containment`; scored alone first, then with W1b                                                                                                                                                                                                                                                                                        | M {math}          | 0.5.2.1                         | W0                |
| W3   | Retention and fragmentation (§4.3), measurement: per reviewed mask, the retained share and cluster count per frame in `lidar-ground-truth-eval`, D3's near-merge cases included; kirk0's cases listed; truck 2's cab attributed to the L3 rule or the L4 split that lost it                                                                                                                                                                                                                                                      | M                 | 0.5.2.1 measure, v0.5.6 fix (L) | The frozen pack   |
| W4   | Extents per §9.2 on the floored box: the merge test reading the belief before the floor and admitting by containment, the censored likelihood, the revisable record                                                                                                                                                                                                                                                                                                                                                              | M {math}          | v0.5.7                          | W1b, W2 scored    |
| W5   | Shadow-first delivery and the identity gate: W1 and W2 on the shadow, physical score, then A2 against the §3 switch band; the containment share into the S3 and reacquisition decisions; G-UNC-1's table refrozen after                                                                                                                                                                                                                                                                                                          | S                 | 0.5.2.1, then 0.5.2.2           | W1b, W2           |
| W7   | The second capture: the [spike](../lidar/operations/end-on-truck-window-spike-2026-10.md) counted 33 clean end-on windows in the corpus and an archive 6.7 times its size, and names two tuning windows and a lot-drawn held-out segment; then one capture request covering an end-on truck, cars beyond 50 m and a side-to-end facet episode, selected by traffic or at random so the window is not chosen for failure, reviewed with operator-keyframed yaw and extents, frozen; tuning-only if it cannot be selected that way | M (operator time) | 0.5.2.1                         | The review window |
| W8   | Retire `solid_body_course_heading`, `solid_body_extent_growth`, `solid_body_end_face_centring` (both settings) and `solid_body_vehicle_extent_floor` once W1b and W2 meet their levels, and `solid_body_extent_prior_floor` and `solid_body_face_plane_spans` now; the D2.4 panel shows the heading source, the axis-by-age table and the containment share                                                                                                                                                                      | S                 | v0.5.7                          | W1b, W2 promoted  |

Sequence: W0 first, and the W7 spike in the same week, since every promotion waits on the
capture it finds. Then W2 alone and W1a in parallel with W3's measurement; W1b on both; W5 as
the scoring is read; W4 after. Each of W1a, W1b and W2 is one to two weeks to a scored result,
not days: the near-edge S2 line took the October campaign's fortnight for a smaller change, and
a corpus screen is about 1.7 hours per arm. About three to five weeks to a scored shadow
candidate, two more to A2 with the identity gate, not counting W7's review or the Pi
measurement, which no sprint holds today. W6's low-speed labelling is inside W1b. Sprint
0.5.2.1 then carries sixteen items; read it as two lanes, body geometry (W1, W2, W5, the extent
item they supersede) and measurement and capture (W3, P11, W7), with the facet items waiting on
W2's result as the facet plan itself asks.

## 7. Risks

- **The fit is a new jitter source.** A rectangle on sparse far points can swing between frames.
  σ_θ must carry that, the axis state has process noise, and G-GEO-1's criterion 1 is the check:
  if the five-point lateral residual rises with W1 on, the fit is being trusted too far.
- **Containment pulls toward merged clusters.** A box made to hold two cars' points is a box on
  neither. The floor never feeds the belief, the merge test reads the belief before the floor,
  the truncation applies only to clusters the merge test admits, and the corpus's merge-heavy
  sites (lombard-laguna, 1st-mission) are the screen. W4 is where the two rules are reconciled.
- **The constraint fights the filter.** Bounded by the state's own σ and applied to position
  only, it cannot move more than the prediction's uncertainty allows; the across-velocity change
  it would have made is recorded, and the five-point residual is the alarm.
- **Labelling errors look like heading errors.** A right axis with the wrong length label is a
  90° directed error in every score. The axis and directed errors are scored separately, and
  the ambiguous state is reported, not resolved by a guard.
- **The references are fits to the same returns.** kirk0's yaw and extents came from
  `fit:mask_v1` on the reviewed returns the tracker reads, within 3.7° of resolution. W7's
  references are operator-keyframed, and W1a's variance is calibrated on kirk0 and scored on W7.
- **Identity under A2 moves anyway.** Shadow-first measures the geometry without it; the A2
  decision then rests on the switch band above a measured noise floor.
- **Criterion 1 is not in reach of this plan.** The candidate cut the corpus's lateral p99 by
  10 % at the median; the gate asks 50 %. W1 and W2 make the box right, not tighter across the
  course; the next remedy is the face-transition remedy the near-edge plan left unchosen, then
  point-to-model residuals, and the exit-B date should be read with that.
- **Upstream bounds the result.** Truck 2's length and gap cannot be right until its cab is in
  the foreground. W3 is measurement; the fix is v0.5.6 work (L) that moves the L3 or L4
  fingerprint and reopens G-GEO-1, so the 0.5.2 envelope excludes partially retained trucks
  until it lands.
- **Live has no points.** None of this reaches a device until live retention ships; the Pi cost
  of the fit is unmeasured, and under A2 it runs per candidate pair.
- **Operator time.** W7 competes with the P3 pilot, the facet operator pilot (20 to 40 hours)
  and the facet capture for the one review window; one capture request serving all three, and a
  capped operator queue per sprint, is the mitigation.

## 8. What this does not change

- The default tracker, the tuning fingerprint and the perf baselines. Everything is an
  experiment until the §3 levels are met on a held-out split.
- The state plan's estimator design (§9.2) or the facet registration experiment. W1 and W2 are
  the cheaper step the F0 report recommended before either; if they meet the levels, arm D's
  case weakens; if they do not, the residual error is the attribution arm D needs.
- The pilot's references or the frozen split.
- The G-GEO-1, G-UNC-1 and G-SMO-1 thresholds. §3 adds rows for predeclaration; it substitutes
  for none of them.

## Checklist

- [ ] W0 levels predeclared, the identity noise floor measured, the band committed to main; containment share, axis-by-age table and reported-against-believed extent in the summary and scorecard
- [x] W7 spike: end-on truck windows counted across the corpus and archive ([spike](../lidar/operations/end-on-truck-window-spike-2026-10.md)); the capture request is its selection rule, to be raised with the operator
- [ ] W2 `solid_body_containment` built, unit-tested, scored alone on the frozen split and the corpus
- [ ] W1a fit, variance and abstention built and unit-tested; cost measured on the Mac and a Pi
- [ ] W1b `solid_body_rectangle_heading` built, scored alone and with W2; car 8's slow poses re-scored
- [ ] W3 retention and fragmentation scored per reviewed mask; truck 2's cab attributed
- [ ] W5 shadow-first physical score; A2 against the switch band; G-UNC-1's table refrozen
- [ ] W7 second capture reviewed with operator-keyframed references and frozen; held-out score
- [ ] W4 extents per §9.2 on the floored box
- [ ] W8 superseded experiments retired; D2.4 panel shows the new diagnostics
