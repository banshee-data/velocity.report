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
- **Canonical:** [state estimation plan](lidar-state-estimation-plan.md) §5.6, §9.2, §9.3; [visibility-aware review](../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md) §3.2, §4.1, §5.2, §7.1
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
by track age. The course is not truth (§3.2 of the visibility-aware review) but it is
independent of the PCA heading, and on kirk0 it agrees with the reviewed yaw within 6° for the
car and truck 1.

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

It would act on nearly every row. That is the measure of the gap, and also the warning: a
constraint that fires every frame is an observation model, not a clamp, and must enter the
filter as one or it will fight the prediction and show up as lateral jitter.

### 2.3 The orientation is in the data every frame

A rectangle fitted to each frame's retained points, searching the axis over [0°, 90°) in 1°
steps for the orientation that puts the most points nearest an edge (the closeness criterion of
Zhang, Lu and Wang, 2017; facet maths review §7, item 6), gives the body axis modulo 90°:

| Against                               | Rectangle fit, axis | Raw PCA, axis | Believed heading, directed |
| ------------------------------------- | ------------------: | ------------: | -------------------------: |
| Course, kirk0, all ages, median / p90 |            5° / 20° |     10° / 33° |      10 to 17° / 27 to 39° |
| Reviewed yaw, 21 poses, median / p90  |         2.6° / 4.1° |   10.7° / 15° |               7.5° / 89.4° |

It holds on truck 2's end-on strips (0.4 to 0.8 m deep, 27 to 121 points: axis within 0.4° to
4.8°) and on car 8 at 2 m/s. It fails where it should abstain: car 8 at 0.8 m/s, a 1.6 × 1.4 m
view, 31° off. What the fit does not give is which of the two axes is the length. For truck 2's
strip the longer span runs across the truck. That labelling is the one ambiguity, and it is the
job the D2.1 axial selector already does with the extent belief's aspect, on an input (the PCA
box) whose axis is wrong by 10°.

## 3. What acceptable looks like

No numeric physical-heading threshold exists in any plan today, and G-GEO-1 has no heading or
containment row. These are proposed levels, to be frozen as part of the predeclared physical
scoring criteria (Sprint 0.5.2.0) before any held-out score, and amended only in writing before
that score. "Established" is the lifecycle state of state plan §5.6; "reference bound" is the
pilot's per-pose bound.

| Measure                                    | Level                                                                                                   | Today (A2 / candidate, kirk0)                         |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Heading, established rows at 2 m/s or more | Axis error against reviewed yaw: median ≤ 5°, p90 ≤ 15°; directed error ≤ 10° on at least 90 % of poses | 7.5° / 89°; 33° / 12° mean                            |
| Heading convergence, label-free            | Rows older than 2 s more than 30° off their course: ≤ 5 %, ambiguous rows excluded and counted          | 23 to 27 % corpus (candidate 4 to 6 %, self-measured) |
| Heading ambiguity                          | A row whose axis label is unresolved says so and reports the observed spans, never a labelled body      | Reported resolved with weight 0                       |
| Containment                                | ≥ 95 % of body-centre rows hold ≥ 98 % of their retained points within 0.15 m; the rest are flagged     | 46 % / 58 %                                           |
| Reported extent                            | Never below the frame's observed span along the reported axis                                           | Below it in 34 to 66 % of fixes                       |
| Centre                                     | ≥ 80 % of scored poses within the reference's own centre bound                                          | 26 % / 52 %                                           |
| Length, established cars; trucks           | Within 15 %; within 20 % of the reference on ≥ 80 % of poses                                            | Car 3.88 of 4.26 m; trucks 8.6 of 10.6, 0.6 of 9.6 m  |
| Following gap, both bodies established     | p95 absolute error ≤ 0.5 m (joins the following gate's predeclaration)                                  | 1.15 / 1.60 m mean                                    |
| Lateral, G-GEO-1                           | Criteria 1, 2 and 4 unchanged: no new jitter from the constraint, manoeuvres ≥ 90 % of magnitude        | Not scored on a held-out split                        |
| Identity on the frozen split               | HOTA and IDF1 within 0.01, ID switches within 10 % of the arm's baseline                                | HOTA within 0.005; switches 78 to 117                 |
| Cost                                       | `Tracker.Update` p99 within 10 % of the arm's baseline; the fit itself budgeted and measured on the Pi  | Candidate −25 % p99, +19 % median on a Mac            |

Two levels are deliberately not here. A rate for the deployed system needs the held-out split
that physical scoring still refuses, and a second capture with an end-on truck. And nothing
above is a promotion gate on its own: the chain G-GEO-1, G-UNC-1, G-SMO-1 stands.

## 4. What is needed

Two observation models, each default-off behind an experiment name, each tested on the synthetic
passes, scored on the frozen kirk0 split and screened on the corpus like the alignment
experiments were. Then the estimator and upstream work they expose.

### 4.1 An orientation observation from the cluster

Every frame, fit the rectangle orientation to the associated cluster's retained points and
treat it as an observation of the body axis.

- **The fit.** Search θ over [0°, 90°) at 1° (coarse) then 0.25° (around the best) for the
  closeness criterion; report θ, the two edge spans, the support on each edge, and the margin of
  the best score over the best score more than 10° away. Abstain when the margin is small or
  both spans are under 2 m with aspect under 1.25 (the near-square case). Cost: 90 orientations
  by at most 256 points, about 23,000 projections, under 0.1 ms in Go; measure it.
- **Variance.** σ_θ from the fit, not a constant: the edge support and spans bound it (a 0.4 m
  deep strip 2 m wide fixes its axis to about 2°; two short edges do not). Calibrate on the 21
  reviewed poses and the corpus courses before the experiment is scored; report it as a bounded
  scale, not a posterior, per the D2 report's own caveat.
- **Labelling.** The fit gives two axes. Which is the length is decided as D2.1 decides it: by the
  extent belief's aspect when it is evidence, then by the course at 2 m/s or more, else held as
  two hypotheses per state plan §5.6 and review §4.1. An unresolved row reports the observed
  spans and is marked ambiguous. This is what car 8 at 0.8 m/s should have been: a box on the
  right axis with no claim about its front.
- **The state.** Heading becomes a filtered quantity with its own variance, updated by θ each
  frame (mod π, with the π direction from the course or an asymmetric cue, as now). For the solid
  body the EMA and the three guards retire under the experiment; the near-edge faces are found
  along the observed axis of the frame, not the previous belief, so their normals agree with the
  points they are fitted to. Heading error then decays at the filter's rate, which is the
  convergence the first table lacks.
- **Low speed.** The axis is observable at any speed; only the label waits. Extent admission
  below 2 m/s then has an axis it can trust, which removes the reason the lifecycle holds a slow
  body in `initialising`.

### 4.2 A containment constraint: the box holds the points

The frame's points are known to lie on the body. The reported box must contain them, and the
estimate should be pulled toward boxes that do.

- **Reported extent floor.** The reported length and width are never below this frame's trimmed
  observed span along the reported axes. The belief keeps its corroboration rule; what is drawn
  and persisted honours the lower bound the frame has just measured (state plan §9.2.1, "admit
  as uncertain lower bound"). This alone removes the 24 to 75 % of rows whose box is shorter
  than their cluster.
- **Position interval.** For each body axis, the points' trimmed span [lo, hi] and the reported
  half-extent h give the feasible interval for the centre, [hi − h, lo + h]. A predicted centre
  outside it is updated toward the nearer bound by one scalar update with variance R plus the
  square of half the extent's uncertainty, recorded as a constraint application with its own
  provenance. It is the end-face centring term generalised: that term used one face's own span
  at one fix type; this uses every axis the fix left open, at every fix type, including the
  side-face-only fix that T5 excluded by design. A side face still observes nothing along
  itself; the points do.
- **Lapse within the hull.** A body-centre claim no longer lapses to the medoid while the box
  can be held on the points; it lapses when there are no points. When it must re-seed, it seeds
  inside the feasible interval, not at the biased medoid.
- **The invariant and its metric.** Every persisted solid-body row carries its containment
  share; the solid-body summary reports the share of rows under 98 %, and the scorecard carries
  it as a label-free guard beside the lateral fit. A row that cannot be made consistent is
  flagged, never silently reported.

The two models are one change in practice: containment along a wrongly oriented axis would pull
the box the wrong way, and an orientation observation without containment leaves the box off
the points along the open direction. Score them together and each alone, as the alignment run
did.

### 4.3 What they expose

- **Extents per §9.2.** With the reported box floored at the observed span, the belief's job is
  the unseen remainder. The corroborated maximum stays as a guard against a one-frame merge,
  but the merge test itself should become containment-consistent: a cluster whose across-span
  fits the believed width plus a margin is one body, however large its area ratio against the
  running mean. Growth admission is the first version of that rule. The censored likelihood and
  the revisable admitted-frame record of §9.2.2 and §9.2.3 are the estimator; they are not
  changed here.
- **Upstream retention and fragmentation.** Truck 2's cab never reached the tracker and truck
  1's cab was two separate clusters. No observation model can place a body on points it was not
  given. The reviewed pack can score this today: for each reviewed mask, the share of its
  returns in the associated cluster, and the number of clusters its returns fall in. Those two
  numbers are an L3 and an L4 gate, and the cases they find are the levers' evidence: the
  region-override window (at 20 m about 5 m, so returns within a fifth of the background range
  are background), the deadlock breaker that absorbs a slow body, and DBSCAN's single `eps`
  with no gap tolerance across a dropped band of returns.
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

| Planned item                                                                            | Where                                                                                                                              | What it covers                                                                            | What it leaves                                                                                    |
| --------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Heading candidate acceptance: D2.1 axial selector, D2.4 run panel                       | [BACKLOG v0.5.7](../BACKLOG.md#v057---tracker-correctness--robustness-057), [D2 report](lidar-heading-d2-implementation-report.md) | Labelling the axis by aspect against the extent belief, abstention, the observed envelope | Its input is the PCA box, 10° off at the axis; it does not observe heading. §4.1 feeds it the fit |
| Solid-body extent accumulation                                                          | [BACKLOG 0.5.2.1](../BACKLOG.md#sprint-0521-physical-body-and-corrected-geometry)                                                  | Growth admission and the vehicle floor as the opt-in candidate                            | The reported box still falls below the observed span; §4.2's floor                                |
| Phase 2 corrected measurement, G-GEO-1                                                  | BACKLOG 0.5.2.1, [state plan §9.3](lidar-state-estimation-plan.md#93-decision-gate-g-geo-1)                                        | The lateral criteria the constraint must not break                                        | No heading or containment row; §3 proposes them                                                   |
| Face-transition remedy, S2.1 (gap 1.9 against 1.25, unchosen)                           | [near-edge plan](lidar-near-edge-tracked-state-plan.md#face-transitions)                                                           | The 1.5 m jump at side-face entry                                                         | §4.2's interval is a candidate remedy: the jump is the across position returning late             |
| Physical scoring criteria and held-out acceptance                                       | BACKLOG 0.5.2.0                                                                                                                    | Predeclared limits on the tuning references                                               | Takes §3's heading and containment rows                                                           |
| Facet registration E1 to E3, arm B (17 to 29 days)                                      | [facet plan](lidar-facet-registration-experiment-plan.md), BACKLOG 0.5.2.1                                                         | Pose from point registration: the structural successor to faces                           | The F0 report asked for the cheaper step first; §4.1 and §4.2 are it                              |
| Geometry-coherent tracking, D-04 (#391)                                                 | BACKLOG v0.5.7                                                                                                                     | Persistent geometry state in association; targets drift under 0.5°/s, 90° jumps under 1 % | Waits for a corrected-body baseline; §4.1 is what would reach its targets                         |
| Live cluster point retention                                                            | BACKLOG v0.5.6                                                                                                                     | Points on the live path                                                                   | Critical path for live parity of everything here                                                  |
| Long-range foreground sensitivity; D3 near-merge and adaptive `eps`; slope-aware ground | BACKLOG v0.5.7, 0.5.2.1                                                                                                            | The L3 window and the L4 split, each from its own evidence                                | No measure against reviewed masks; §4.3's gates supply one and truck 2 is its case                |
| Association cost S3; reacquisition and identity acceptance                              | BACKLOG 0.5.2.2                                                                                                                    | Like-with-like association under A2                                                       | Take the containment share as an input                                                            |
| Option A to B: heading into the state (deferred gate)                                   | [state plan §7.3](lidar-state-estimation-plan.md#73-recommendation-and-gate)                                                       | Waits for orientation variance to be shown as the limiting term                           | §2 shows it: an end-on truck at 76° has a box IoU of 0.05 whatever its position                   |
| L-shape fitting                                                                         | [AV integration plan](lidar-av-lidar-integration-plan.md) Phase 6, deferred                                                        | Listed as a future bounding-box method                                                    | Brought forward as the orientation observation, not a box method                                  |

## 6. Work to scope and schedule

| Item | Work                                                                                                                                                                                                                                                                                                       | Size              | Sprint                      | Depends on                     |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------- | --------------------------- | ------------------------------ |
| W0   | Predeclare the geometry levels of §3 with the physical scoring criteria; add the containment share and the heading-by-age table to the solid-body summary and the scorecard, so every arm reports them                                                                                                     | S                 | 0.5.2.0                     | Nothing                        |
| W1   | Orientation observation (§4.1): the fit, its variance, labelling and hypotheses, the filtered heading, faces along the observed axis; experiment `solid_body_rectangle_heading`; unit tests on strip, corner, square, sparse and reversing passes; scored on the frozen split and the corpus               | M {math}          | 0.5.2.1                     | W0 for the score               |
| W2   | Containment (§4.2): the reported-extent floor, the position interval as a scalar update with provenance, lapse within the hull; experiment `solid_body_containment`; the same scoring                                                                                                                      | M {math}          | 0.5.2.1                     | W0; scored with and without W1 |
| W3   | Retention and fragmentation gates (§4.3): per reviewed mask, the retained share and cluster count per frame in `lidar-ground-truth-eval`; the kirk0 cases listed; truck 2's cab traced to the L3 rule or the L4 split that lost it                                                                         | M                 | 0.5.2.1 measure, v0.5.6 fix | The frozen pack                |
| W4   | Extents per §9.2 on the floored box: containment-consistent merge admission replacing the area-ratio refusal, the censored likelihood and the revisable record                                                                                                                                             | M {math}          | v0.5.7                      | W1, W2 scored                  |
| W5   | Shadow-first delivery and the identity gate: W1 and W2 on the shadow, physical score, then A2 against the §3 identity level; the containment share into the S3 and reacquisition decisions                                                                                                                 | S                 | 0.5.2.1 / 0.5.2.2           | W1, W2                         |
| W6   | Low-speed labelling: the course from displacement under 2 m/s and the class prior's aspect as the label below it; car 8's two poses as the case                                                                                                                                                            | S                 | 0.5.2.1, after W1           | W1                             |
| W7   | A second capture with an end-on truck, reviewed and frozen, and the held-out physical score the chain needs                                                                                                                                                                                                | M (operator time) | 0.5.2.1                     | The review window              |
| W8   | Retire `solid_body_course_heading`, `solid_body_extent_growth`, `solid_body_end_face_centring` (both settings) and `solid_body_vehicle_extent_floor` once W1 and W2 meet their levels, and `solid_body_extent_prior_floor` and `solid_body_face_plane_spans` now; the D2.4 panel shows the new diagnostics | S                 | v0.5.7                      | W1, W2 promoted                |

Sequence: W0, then W1 and W2 in parallel with W3's measurement, each about three to four days to
a scored result; W5 as the scoring is read; W6 and W4 after; W7 whenever the review window has
an operator, since it gates every promotion. About three weeks to a scored shadow candidate, two
more to A2 with the identity gate, not counting W7.

## 7. Risks

- **The fit is a new jitter source.** A rectangle on sparse far points can swing between frames.
  The variance must carry that, and G-GEO-1's criterion 1 is the check: if the five-point
  lateral residual rises with W1 on, the fit is being trusted too far.
- **Containment pulls toward merged clusters.** A box made to hold two cars' points is a box on
  neither. The constraint applies only to clusters the merge test admits; W4 is where the two
  rules are reconciled, and the corpus's merge-heavy sites (lombard-laguna, 1st-mission) are the
  screen.
- **Labelling errors look like heading errors.** A right axis with the wrong length label is a
  90° directed error in every score. The ambiguity must be reported, and the directed and axis
  errors scored separately, as §3 does.
- **Identity under A2 moves anyway.** Shadow-first measures the geometry without it; the A2
  decision then rests on the identity level alone.
- **Upstream bounds the result.** Truck 2's length and gap cannot be right until its cab is in
  the foreground. W3 is measurement; the fix is v0.5.6 work and may move the L3 fingerprint.
- **Live has no points.** None of this reaches a device until live retention ships; the Pi cost
  of the fit is unmeasured.

## 8. What this does not change

- The default tracker, the tuning fingerprint and the perf baselines. Everything is an
  experiment until the §3 levels are met on a held-out split.
- The state plan's estimator design (§9.2) or the facet registration experiment. W1 and W2 are
  the cheaper step the F0 report recommended before either; if they meet the levels, arm D's
  case weakens; if they do not, the residual error is the attribution arm D needs.
- The pilot's references or the frozen split.

## Checklist

- [ ] W0 levels predeclared in the physical scoring criteria; containment share and heading-by-age in the summary and scorecard
- [ ] W1 `solid_body_rectangle_heading` built, unit-tested, scored on the frozen split and the corpus
- [ ] W2 `solid_body_containment` built, unit-tested, scored alone and with W1
- [ ] W3 retention and fragmentation scored per reviewed mask; truck 2's cab attributed
- [ ] W5 shadow-first physical score; A2 against the identity level
- [ ] W6 low-speed labelling; car 8's two poses re-scored
- [ ] W7 second end-on truck reviewed and frozen; held-out score
- [ ] W4 extents per §9.2 on the floored box
- [ ] W8 superseded experiments retired; D2.4 panel shows the new diagnostics
