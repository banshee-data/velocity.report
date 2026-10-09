# Solid body against physical references: heading and extent from one face

<!-- ignore-style-length -->

The [kirk0 physical-reference pilot](physical-reference-pilot-kirk0-2026-10.md) found the solid
body tracking a car to within 0.25 m and neither of two trucks. This is the run that followed
it: a diagnosis of why, seven default-off experiments, and their scores against the same frozen
split, with identity, a synthetic end-on truck and the 23-site corpus as guards.

- **Status:** Complete, 2026-10-09. Tuning evidence on one capture; no default changes
- **Layers:** L5 tracker (solid body), offline evaluation
- **Related:** [plan](../../plans/lidar-solid-body-physical-alignment-plan.md), [kirk0 pilot](physical-reference-pilot-kirk0-2026-10.md), [near-edge tracked state](../../plans/lidar-near-edge-tracked-state-plan.md), [F0 report](facet-f0-attribution-kirk0-report.md), [per-frame evaluation](per-frame-evaluation.md)
- **Code:** [solid_body_nearedge.go](../../../internal/lidar/l5tracks/solid_body_nearedge.go), [experiments](../../../internal/lidar/replayeval/experiments.go), [tests](../../../internal/lidar/l5tracks/solid_body_alignment_test.go)

## Summary

Three mechanisms in the solid body, each with a default-off correction, explain most of what the
pilot found wrong with kirk0's two trucks:

- **Heading.** The course heading takes truck 2 from 76° to 1° off on A2.
- **Length and centre.** Growth admission takes truck 1 from 5.9 m to 8.9 m long (10.6 m
  reviewed) and its centre error from 2.2 m to 0.9 m.
- **Car 8** is unchanged.

On the 23-site corpus that pair made the lateral tail worse at 17 to 19 sites. End-face-only
fixes, started sooner and steered by the course, drift across the body until a side is seen.
End-face centring removes the drift. With all four experiments, centring in its open-prior
setting:

| Measure                       | Change against A2                                  |
| ----------------------------- | -------------------------------------------------- |
| Lateral p99                   | Better at 17 sites; worse at 6, by at most 0.018 m |
| Near-edge fixes               | +1.4 %                                             |
| Lapses                        | +4 %                                               |
| kirk0 `Tracker.Update` median | +19 %                                              |
| kirk0 `Tracker.Update` p99    | Falls                                              |

What is left of the trucks' error is upstream of the solid body: truck 2's cab never reaches the
tracker, and truck 1's cab is two separate fragments. Nothing changes by default. The four are a
candidate for a held-out physical score, not a result.

## Question

The pilot scored 22 reviewed poses of three vehicles in kirk0's slow lane. Why are the trucks
wrong, and can the solid body be corrected for what it actually sees, without fitting it to
three vehicles?

## Why the trucks are wrong

Reading the solid body's rows frame by frame against the poses found three mechanisms in the
solid body and two causes upstream of it.

**In the solid body:**

1. **A face-only cluster's heading runs across the vehicle.** The heading is the tracked PCA axis,
   seeded at birth. Truck 2, approaching end on, was born 70° off its course and settled 85°
   off: its front face is a strip as wide as the truck and a few decimetres deep. Nothing
   corrects a heading that started wrong; velocity only flips it by π.
2. **A single face's depth replaces the class prior.** A front face admits the cluster's span
   along the axis as the length, which seen end on is the face's depth. The first such span
   replaces the 4.5 m prior, so truck 2 became 0.625 m long on its first front-face frame, and
   the corroborated maximum only grows when three frames see longer.
3. **A growing view reads as a merge.** The merge test compares a cluster's box with the mean of
   every earlier one. Truck 2's history was 30 frames of a 2.6 × 0.8 m face; when its side
   came into view (a 9.5 × 4 m box) the ratio stayed above 2.5 for 22 frames, and extent
   admission is refused while it does: the length was refused exactly when it was first
   visible. Truck 1's rose the same way.

**Upstream, in what the tracker is given:**

4. **Truck 1 is three tracks.** At sample 495 the body is one cluster of 632 points; the cab is
   two fragments of 40 and 15 points, about a metre beyond it. The control arm's score matched a
   cab fragment, which is its "0.38 m truck".
5. **Truck 2's cab never reaches the tracker.** Its tracked cluster lies 2.4 to 3.1 m behind the
   reviewed front at every gap sample: 62 of the mask's 285 points at sample 500, the face of
   the box behind the cab. Today's foreground keeps about half the returns the pack was cut
   with.

The first three are the solid body's to fix. The last two are not, and they bound what any
solid-body change can do here.

## Experiments

Each is a default-off `-experiment` name qualifying `solid_body`, so production B0, the shadow and
A2 are unchanged unless one is named. Constants are existing ones except the two stated.

| Experiment                                | Change                                                                                                                                                                                                                                                                    | Outcome on kirk0                    |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| `solid_body_course_heading`               | The orientation turns from the tracked axis toward the course as their disagreement grows from the span window (10°) to twice it and the speed from `CourseAlignmentMinSpeedMps` (2 m/s) to twice it; an axis within the window is kept, pointed the way the body travels | Truck 2 heading 76° to 1°           |
| `solid_body_extent_growth`                | A merge candidate no wider across the body than its believed width plus 1.0 m (new) still gives its length, not its width; a length span is searched for only when it could raise the belief                                                                              | Truck 1 length 5.9 to 8.9 m         |
| `solid_body_end_face_centring`            | At a fix by an end face alone, the position across the body is also updated from the midpoint of the face's own returns, with R plus the square of half its shortfall from the believed width; a face seen under half that width is refused as cut off                    | Corpus lateral tail; see below      |
| `solid_body_end_face_centring_open_prior` | End-face centring, refusing a narrow face only against a width measured on the body, not a class prior                                                                                                                                                                    | Unchanged; corpus, see below        |
| `solid_body_vehicle_extent_floor`         | A rigid vehicle, or a body of unknown class already 2.4 m long, is held at least 2.4 × 1.4 m (new: the smallest road cars)                                                                                                                                                | Truck 1 width 0.4 to 1.4 m (shadow) |
| `solid_body_face_plane_spans`             | A face admits the span along its own plane, not its depth                                                                                                                                                                                                                 | No effect                           |
| `solid_body_extent_prior_floor`           | A dimension below the class prior keeps the prior                                                                                                                                                                                                                         | Harmful                             |

- **Face-plane spans** changed nothing: a front strip also counts geometrically as showing a side
  face, and that face's plane span is the same strip depth.
- **The prior floor** took near-edge fixes from 942 to 382 and dropped car 8 from scoring: a
  dimension held at the prior is prior-only, and the fix excludes such faces by design. The
  vehicle floor keeps the dimension accumulated instead, so its faces stay in the fix, and does not
  lengthen a small car seen whole.
- **Growth admission** first admitted widths too, which held car 8 at 2.12 m wide (true 1.73 m)
  through the corroborated maximum; it now admits only length.
- **The course heading** first replaced every axis above 2 m/s, which cost car 8, accelerating
  from rest, two degrees; then kept an axis within the window and switched beyond it, which turns
  the orientation by the whole window in one frame. It now ramps.
- **The vehicle floor** first applied only to the rigid-vehicle class, and 88 % of kirk0's solid
  bodies, all three vehicles among them, are class unknown. A body already as long as the
  smallest car is now held at that car's width too.
- **End-face centring** came from the corpus, not kirk0: see below. T5 (`rank_one_medoid`), the
  existing remedy for the same drift, does not apply there, because a side face is found but not
  in the fix.
- **The open prior** came from the corpus too: with a width still at the unknown class's 2 m
  prior, plain centring refused any face seen under 1 m across, so a slow car seen obliquely had
  nothing across it until its width was measured. It is a second setting of centring, so the two
  names are refused together. On kirk0 it changes no scored pose and no identity figure.

## Method

- **kirk0.** One replay per arm (`-warmup 20`, about 18 s), scored against
  `kirk0-ad8b9438-tuning-r1` with `lidar-ground-truth-eval perframe`: physical references on
  `following-472-530` alone (the split's episodes overlap), identity on `whole-pack`. Every arm and
  score is in a ledger; the head build reproduces the pilot's numbers exactly, and the control
  replay's summary is identical, timing aside, with the experiments compiled in.
- **Synthetic.** A 9.6 × 2.5 m truck approaching end on from 50 m, 4 m to the side, at 7 m/s
  from a 2.3 m mount, through the whole tracker (`TestAnEndOnTruckIsHeadedAlongItsCourse`).
- **Corpus.** The first segment of the 23 tuning and screen sites, 200 s after a 70 s warm-up,
  as the D2 refresh ran them, for A2 (`near_edge_track` on the shadow's base) and the kept
  experiments on it, in three passes as the experiments were refined; label-free:
  `lidar-track-scorecard` and the solid-body summary.
- **Cost.** `-campaign-metrics` on kirk0 with nothing else of this work running, conditions in a
  balanced order; and the synthetic truck as a Go benchmark.

## Findings on kirk0

### Per vehicle

Medians over scored poses at 0011ff8dc. Truck lengths against 10.63 and 9.63 m, car 8 against
4.26 m. "Heading + growth" is `solid_body_course_heading` with `solid_body_extent_growth`; "all
four" adds `solid_body_end_face_centring` and `solid_body_vehicle_extent_floor`. With centring's
open-prior setting instead, every figure in this section is the same.

| Vehicle | Arm                  | Poses | Centre, m | Heading, ° | Length, m | Width, m | Box IoU |
| ------- | -------------------- | ----: | --------: | ---------: | --------: | -------: | ------: |
| Truck 1 | shadow               |     6 |      2.62 |       85.0 |      0.38 |     0.50 |    0.01 |
| Truck 1 | shadow, all four     |     9 |      1.76 |        4.7 |      8.88 |     1.40 |    0.18 |
| Truck 1 | A2                   |     8 |      2.19 |        6.7 |      5.88 |     2.88 |    0.49 |
| Truck 1 | A2, heading + growth |     8 |      0.90 |        4.6 |      8.88 |     2.88 |    0.69 |
| Truck 1 | A2, all four         |     9 |      1.00 |        5.4 |      8.62 |     2.62 |    0.68 |
| Car 8   | shadow               |     5 |      0.24 |        6.0 |      3.88 |     1.62 |    0.76 |
| Car 8   | shadow, all four     |     6 |      0.25 |        6.0 |      3.88 |     1.62 |    0.74 |
| Car 8   | A2                   |     5 |      0.25 |        6.0 |      3.88 |     1.62 |    0.75 |
| Car 8   | A2, heading + growth |     6 |      0.23 |        6.0 |      3.88 |     1.62 |    0.75 |
| Car 8   | A2, all four         |     6 |      0.23 |        6.0 |      3.88 |     1.62 |    0.78 |
| Truck 2 | shadow               |     6 |      1.99 |      125.9 |      0.62 |     2.12 |    0.05 |
| Truck 2 | shadow, all four     |     6 |      1.93 |        3.2 |      0.62 |     2.12 |    0.05 |
| Truck 2 | A2                   |     6 |      2.00 |       75.8 |      0.62 |     2.12 |    0.05 |
| Truck 2 | A2, heading + growth |     6 |      2.00 |        1.0 |      0.62 |     2.12 |    0.05 |
| Truck 2 | A2, all four         |     6 |      1.93 |        2.9 |      0.62 |     2.12 |    0.05 |

- **Truck 1** is growth admission's: on A2 alone it takes the length from 5.9 to 8.9 m and the
  centre error from 2.2 to 0.9 m, and the course heading adds nothing to it. On the shadow its
  width stays a cab fragment's until the vehicle floor holds it at 1.4 m.
- **Truck 2** is the course heading's: 76° to 1° on A2 and 126° to 3° on the shadow. Its length and
  centre do not move in any arm, because the tracker never receives its cab (finding 5): the
  cluster it does get is a face 2.4 to 3.1 m behind the reviewed front.
- **Car 8** is unchanged within 0.03 m and 0.03 IoU. Arms with growth admission score one more
  pose: at sample 495 they give the car a body centre, where the others give none.

### Across components

Mean absolute errors over every scored pose of the three vehicles, and over the scored following
gaps. Brackets give poses within the reference's own bound, of those scored, where it is
informative.

| Arm                  | Centre, m    | Yaw, ° | Length, m | Width, m     | Front, m | Rear, m | Gap, m     |  IoU |
| -------------------- | ------------ | -----: | --------: | ------------ | -------: | ------: | ---------- | ---: |
| Shadow               | 1.67 (4/17)  |   70.6 |      6.19 | 1.03         |     2.37 |    4.44 | 2.65 (1/6) | 0.26 |
| Shadow, all four     | 1.40 (6/21)  |   13.7 |      4.16 | 0.90         |     2.09 |    2.94 | 1.50 (3/7) | 0.32 |
| A2                   | 1.61 (5/19)  |   33.2 |      4.46 | 0.27 (13/21) |     2.95 |    2.42 | 1.15 (3/7) | 0.41 |
| A2, heading + growth | 1.04 (11/20) |   12.4 |      3.46 | 0.28         |     1.69 |    2.37 | 1.56 (3/7) | 0.50 |
| A2, all four         | 1.06 (11/21) |   12.0 |      3.48 | 0.36 (5/22)  |     1.77 |    2.23 | 1.60 (3/7) | 0.52 |

- **Yaw** falls by nearly three times on A2 and five on the shadow. What remains is mostly two poses
  of car 8, at samples 491 and 495, which are 87° and 90° off in every arm, the controls included;
  without them the all-four mean is 4.4°. They are truck 2's mechanism below the course speed: car 8
  is accelerating from rest at 0.8 to 1.7 m/s, its new track is seeded from a rear-face strip at
  146°, across the car, and its believed box is 1.62 m long and 2.38 m wide until its side appears
  at sample 496 and the axis turns to 49°. The course heading does not act below 2 m/s, by design.
- **Width** within the bound falls from 13 to 5 poses with all four. Truck 1 is held at 2.62 m
  instead of 2.88 m (reviewed about 2.9 m), so its nine poses sit 0.28 m short, just outside a
  bound of about 0.24 m. The width belief comes from a different track history in that arm; it is
  the one physical cost of adding centring and the floor on kirk0.

### Following gaps

| Pair                 | Reviewed gaps |          Shadow | Shadow, all four |   A2 | A2, heading + growth | A2, all four |
| -------------------- | ------------: | --------------: | ---------------: | ---: | -------------------: | -----------: |
| Car 8 behind truck 1 |             3 | 4.11 (2 scored) |             0.17 | 0.27 |                 0.19 |         0.22 |
| Truck 2 behind car 8 |             4 |            1.93 |             2.50 | 1.80 |                 2.58 |         2.64 |

Car 8 behind truck 1 is now within 0.2 m, because truck 1's rear is where its body is. Truck 2
behind car 8 gets worse, and it is the right answer that makes it so: with truck 2 headed along
its course, its front is placed half a believed length ahead of a cluster that is itself behind
the cab, so the gap carries the missing cab directly. The wrong heading put the front across the
lane, nearer car 8 by chance.

### Single experiments and identity

On A2, each alone. Identity is over the whole pack (HOTA, IDF1, ID switches); A2 is 0.310, 0.285
and 100.

| Experiment alone     | Truck 1 centre, heading, length | Truck 2 heading |  HOTA |  IDF1 | IDSW |
| -------------------- | ------------------------------- | --------------: | ----: | ----: | ---: |
| Course heading       | unchanged                       |            1.0° | 0.309 | 0.281 |  103 |
| Extent growth        | 0.90 m, 4.6°, 8.88 m            |           75.8° | 0.305 | 0.282 |   78 |
| End-face centring    | 2.27 m, 15.9°, 3.62 m           |          120.2° | 0.305 | 0.267 |  117 |
| Vehicle extent floor | unchanged                       |           75.8° | 0.311 | 0.289 |   81 |
| Heading + growth     | 0.90 m, 4.6°, 8.88 m            |            1.0° | 0.305 | 0.280 |   97 |
| All four             | 1.00 m, 5.4°, 8.62 m            |            2.9° | 0.310 | 0.297 |   89 |

- **Identity moves by up to 39 switches with no physical change.** The vehicle floor alone
  changes nothing at the scored poses and takes the switches from 100 to 81. A2 runs the solid
  body on the tracked filter, so any change to it moves association somewhere in the pack. These
  are sensitivity, not improvement or harm; HOTA stays within 0.005. The shadow arms, where the
  solid body does not feed back, hold 0.356 to 0.358 HOTA and 96 to 97 switches.
- **End-face centring alone harms truck 1** (heading 16°, length 3.6 m, 117 switches), as T5 did
  alone. Combined with growth admission it does not: all four hold truck 1 at 1.00 m, 5.4° and
  8.62 m. The route by which centring alone reaches truck 1's heading was not traced.

### Synthetic end-on truck

The whole tracker on a 9.6 m truck approaching end on, with the corrections off and then with the
course heading and growth admission: its heading goes from 24.5° to 0.1° off its course and its
length from 0.20 m to 9.12 m. Centring leaves its worst error across the body at 0.13 m, as
without it, so it does not undo what the other two give an end-on truck.

### Cost

Wall time inside `Tracker.Update` and of each scored frame's pipeline callback, from
`-campaign-metrics` on kirk0 at be79fbd53, after the corpus passes, with nothing else of this
work running. Five conditions ran forward then in reverse, twice, so each has four runs; the
table gives their medians. "All four" here is the open-prior setting, which takes the same
code paths.

| Arm                  | Update p50, ms | Update p99, ms | Frame p99, ms |
| -------------------- | -------------: | -------------: | ------------: |
| Shadow               |          0.142 |           2.89 |          50.9 |
| Shadow, all four     |   0.179 (+26%) |    3.24 (+12%) |    52.5 (+3%) |
| A2                   |          0.109 |           4.07 |          51.6 |
| A2, heading + growth |   0.151 (+39%) |    3.09 (−24%) |    52.2 (+1%) |
| A2, all four         |   0.130 (+19%) |    3.07 (−25%) |    52.4 (+1%) |

Each condition's four runs agree within 0.02 ms at p50 and within 0.2 ms at p99, but for one shadow
run with all four at 3.60 ms. kirk0 scores 564 frames, so a p99 is the sixth slowest: a property of
a few frames, which on A2 are different frames once identity moves. The synthetic pass put most of
the added cost in growth admission; this table has no arm that separates it. The plan's guard,
`Tracker.Update` p99 within 10 % of the arm's baseline, holds on A2 and is missed by two points on
the shadow. A frame's p99 moves by 1 to 3 %.

The synthetic truck as a benchmark, idle machine, median of five: 2.16 ms per pass on the tracked
heading, 2.02 ms with the course heading, 4.82 ms with growth admission and 4.72 ms with both.
Growth admission searches a merge candidate's spans each frame, which an approaching truck
exercises; on a whole capture, kirk0, the four together add 19 % at the median.

## Corpus

The corpus has no labels. It asks whether the experiments that fix kirk0's trucks disturb what
the solid body already does well across 23 sites, measured by the five-point lateral fit: each
window of five consecutive body-centre rows of a moving track is fitted with a straight line, and
its residual is how far the middle row sits across that line. The tail of that residual is
jitter across the course; excursions are tracks with any window over 0.5 m.

### Heading and growth together (pass 1)

A2 with `solid_body_course_heading` and `solid_body_extent_growth`, against A2, at 17f2719e9
(the course heading before the blend). Medians over 23 sites of the per-site change, with the
sites that got worse and better.

| Metric                   | Change | Worse / better |
| ------------------------ | -----: | -------------: |
| Near-edge fix share      | +0.016 |         1 / 22 |
| Body-centre windows      |    +32 |         2 / 21 |
| Lateral residual p95, m  | +0.021 |         19 / 4 |
| Lateral residual p99, m  | +0.086 |         17 / 6 |
| Steady-run p99, m        | +0.089 |         19 / 4 |
| Tracks with an excursion | +0.012 |         12 / 3 |
| Lapses                   |     +6 |         15 / 8 |
| Tracks                   |     −1 |         6 / 13 |
| Median lifetime, s       |  0.000 |         11 / 9 |

More of the solid body runs on faces, and it runs worse across the course. A2's p95 is 0.02 to
0.15 m by site, so a 0.02 m median rise is about a fifth of it.

### Why: a course that confirms itself

The worst site, california-leon-baker (p99 0.13 to 0.54 m), is one car passing the sensor at
10 m/s. For most of 1.9 s its near-edge fixes come from its front face alone and then its rear
face alone. An end-face fix places the body along its axis and says nothing across it. Under the
course heading the axis is the filter's own velocity, which those fixes never correct across the
axis, so the course holds at 76.5° throughout against about 85° once corrected, and the body
moves 3.1 m across the lane along it, with one lapse partly resetting it. When the right face
enters the fix the body returns 2.8 m in one frame. In A2 the same car's axis came from its
clusters and ran across the car, so its fixes found two faces and constrained both directions:
it stays within 1.2 m across.

Marina showed the same drift in the earlier decomposition: the course heading alone moved
nothing at two sites, growth admission alone raised marina's p99 by 0.11 m, and the two together
by 0.13 m. Earlier lengths start end-face-only fixes sooner; the course heading removes the
cluster axis that used to correct them. `solid_body_rank_one_medoid` (T5), the existing remedy
for a lateral position no face constrains, does not act here, because a side face is found in
the cluster but is not in the fix.

`solid_body_end_face_centring` was written for this: at an end-face-only fix it adds a loose
measurement across the body at the midpoint of that face's own returns. On marina it took the
combination's p99 from 0.233 back to 0.121 m against A2's 0.107.

### All four, and each alone (pass 2)

At 0011ff8dc, against pass 1's A2: the control replays identically at both builds (see
[Provenance](#provenance)). "Heading alone" is the blended course heading; pass 1's combination
used the earlier switch.

| Metric, change from A2   | Heading + growth |       All four | Centring alone | Heading alone |
| ------------------------ | ---------------: | -------------: | -------------: | ------------: |
| Near-edge fix share      |    +0.016 (1/22) |  +0.014 (2/21) |  0.000 (11/12) | +0.002 (2/21) |
| Lateral residual p95, m  |    +0.021 (19/4) |  −0.005 (9/14) | +0.001 (12/10) |   0.000 (8/9) |
| Lateral residual p99, m  |    +0.086 (17/6) | −0.001 (10/12) |  −0.007 (8/14) |   0.000 (3/4) |
| Steady-run p99, m        |    +0.089 (19/4) | +0.001 (12/11) |  −0.007 (7/14) |   0.000 (8/3) |
| Tracks with an excursion |    +0.012 (12/3) |    0.000 (5/5) |    0.000 (2/4) |   0.000 (3/2) |
| Lapses                   |        +6 (15/8) |      +3 (15/8) |      +1 (12/7) |       0 (6/6) |
| Tracks                   |        −1 (6/13) |       0 (9/11) |        0 (5/8) |       0 (5/5) |

Medians of the per-site change (sites worse / better).

- **All four removes the regression at the median** and keeps the extra fixes: p95 and p99 fall
  slightly, the steady runs and excursions hold. It is not neutral everywhere. Three sites remain
  clearly worse at p99: haight-divisadero (+0.150 m), broadway-gough (+0.107 m) and howard-6th
  (+0.064 m); seven are better by more than 0.05 m.
- **Lapses rise with growth admission**, in either combination: pooled over the corpus, 4,707
  for A2, 4,929 with heading and growth and 4,931 with all four, about 23 per 1,000 solid-body rows
  against 22. Fixes rise more, from 91,280 to 94,278. A lapse is the body-centre claim giving way
  after the faceless limit, so more fixes leave more claims to lapse; this was not traced further.
- **Centring alone** is mildly good on A2's own drift (p99 better at 14 sites, worse at 8). On
  kirk0 it harmed truck 1 alone, so it is not a candidate on its own.
- **The blended heading alone** changes nothing on the corpus beyond fixes at 21 sites, which is
  the guard the plan asked for: the heading is only turned where the axis and the course disagree.

### Haight: the prior width

Haight's remaining tail under all four is a car turning at 1.4 to 3.5 m/s. Growth admission gives it
a length, so it is fixed by its front face alone for a second; its width is still the unknown
class's 2 m prior, the face, seen obliquely, is narrower than half that, and plain centring refuses
it as cut off. The body drifts 1.1 m across, and when a width is measured (1.4 m, the floor)
centring is accepted and pulls it back 0.8 m in one frame. A2 never fixed this car before its corner
was seen, because its length was still the prior. Refusing a narrow face only against a measured
width (`solid_body_end_face_centring_open_prior`) takes haight's p99 from 0.232 to 0.100 m and its
p95 from 0.140 to 0.060 m, against A2's 0.081 and 0.062.

### All four with the open prior (pass 3)

At be79fbd53, against pass 1's A2.

| Metric, change from A2   |       All four | All four, open prior |
| ------------------------ | -------------: | -------------------: |
| Near-edge fix share      |  +0.014 (2/21) |        +0.014 (3/20) |
| Lateral residual p95, m  |  −0.005 (9/14) |        −0.007 (7/16) |
| Lateral residual p99, m  | −0.001 (10/12) |        −0.024 (6/17) |
| Steady-run p99, m        | +0.001 (12/11) |        −0.010 (7/16) |
| Tracks with an excursion |    0.000 (5/5) |          0.000 (3/6) |
| Lapses                   |      +3 (15/8) |            +2 (14/9) |
| Tracks                   |       0 (9/11) |             0 (9/11) |

Pooled over the corpus:

| Arm                  | Tracks with an excursion | Lapses | Near-edge fixes |
| -------------------- | -----------------------: | -----: | --------------: |
| A2                   |                       15 |  4,707 |          91,280 |
| All four             |                       14 |  4,931 |          94,278 |
| All four, open prior |                       11 |  4,907 |          94,056 |

- **No site is more than 0.02 m worse at p99.** The six that are worse are haight-divisadero
  (+0.018 m), marina-webster-beach, pierce-haight, embarcadero-bryant, columbus-broadway and
  fulton-divisadero. Eight are better by more than 0.05 m, ashbury-downey by 0.33 m.
- **broadway-gough and howard-6th**, plain all four's other two outliers, are now 0.002 and
  0.008 m better than A2: the same prior-width refusal was behind them.
- **p95 is worse at seven sites**, by at most 0.026 m at pierce-haight, whose A2 p95 (0.018 m) is
  the corpus's lowest.
- **Lapses stay about 4 % above A2**, following the extra fixes; see the pass 2 note.

## Interpretation

- **The trucks' errors were mechanisms, not tuning.** Each traced to code that does what it was
  written to do on a view it was not written for: a heading seeded from an end face, a length
  replaced by one face's depth, a merge test that reads a growing view as two objects. None of
  the corrections fits a constant to the pilot's poses; the two new constants are a 1 m margin and
  the smallest road car.
- **kirk0 alone would have kept the wrong pair.** Heading and growth looked best on the 22 poses,
  and the drift they cause sits between the poses. The corpus found it, and the fix for it,
  centring, harms truck 1 when used alone. The four are one change: growth without centring
  drifts, centring without growth harms truck 1, and the heading alone changes nothing on
  the corpus.
- **Identity under A2 is sensitive, not improved.** Any change to the solid body moves
  association somewhere in the pack. kirk0's ID switches move between 78 and 117 with HOTA within
  0.005, and the corpus's track counts and lifetimes do not move at the median.
- **Truck 2's length, centre and gaps are not the solid body's to fix.** Its cab never reaches the
  tracker, so a correctly headed body is placed behind the cab, and the gap behind car 8 gets
  worse for the right reason.
- **Car 8's two worst poses** are truck 2's mechanism below the course speed. The course heading
  does not act below 2 m/s by design, and nothing here was changed for it.

## What this cannot establish

- A rate. Three vehicles on one tuning capture; physical scoring still refuses a held-out split.
- Physical truth. The references are bounded fits to reviewed returns, not measured vehicles.
- Final estimates. Every arm is online-stage output, scored as a declared baseline.
- Cost elsewhere. The timing is one capture of 564 frames, where a p99 is six frames.

## Recommendations

1. **Keep all seven experiments default-off**, and treat `solid_body_course_heading`,
   `solid_body_extent_growth`, `solid_body_end_face_centring_open_prior` and
   `solid_body_vehicle_extent_floor` as one candidate. Do not run growth admission without
   centring: on the corpus that pair is worse than A2 at most sites.
2. **Before any default, score the candidate on held-out physical references**, which physical
   scoring refuses today, and on a second capture with an end-on truck. Three vehicles on a
   tuning capture are the evidence here.
3. **Take truck 2's missing cab upstream.** The tracked cluster keeps about half the returns the
   pack was cut with; that is a foreground or clustering question at L3–L4, and it bounds every
   end-on truck.
4. **Trace the extra lapses** before promotion: whether they follow the extra fixes, as the
   counts suggest, or are a new failure.
5. **Retire `solid_body_extent_prior_floor` and `solid_body_face_plane_spans`** in a later
   clean-up unless a reason to rerun them appears. This report records their results.
6. **Leave the low-speed heading alone** until a course from a velocity under 2 m/s has its own
   evidence. Car 8's two poses are the only case here.

## Provenance

- **Code.** Branch `claude/physical-align`, stacked on `dd/lidar/annotate-108` (#716). kirk0 tables
  are at 0011ff8dc. Corpus pass 1 ran at 17f2719e9, before the blended heading; pass 2 at 0011ff8dc;
  pass 3, the open-prior kirk0 arms and the timing at be79fbd53. The evaluation tools were built at
  each SHA with `go build -tags pcap`, the commit stamped into `version.GitSHA`, and each arm
  records its binary's SHA-256.
- **Controls.** The A2 control replay of kirk0 gives identical summaries, timing aside, at
  d5e8bc7ca (#716's head), 17f2719e9, 93711e7bf, 0011ff8dc and be79fbd53, so later passes are
  compared with pass 1's A2.
- **References.** Pack `ad8b9438-c1d4-40f0-bd0a-f1852c984569`, frozen split
  `kirk0-ad8b9438-tuning-r1` (split `kirk0-tuning`), as the [pilot](physical-reference-pilot-kirk0-2026-10.md)
  froze it. Physical scoring on episode `following-472-530`; identity on `whole-pack`.
- **kirk0 replay.** `lidar-state-estimation-baseline` on the committed kirk0 corpus and index,
  `-warmup 20 -sample-points 256`, evidence per case; A2 is `solid_body`,
  `solid_body_face_hysteresis`, `solid_body_course_faces`, `solid_body_full_members` and
  `near_edge_track`, the shadow the same without `near_edge_track`.
- **Corpus.** The D2 refresh's first-segment corpus and site index (2026-10-05), every site but
  the held-out `embarcadero-folsom`, `-duration 200 -warmup 70`; scorecards with
  `-scoring-start-seconds 70`.
- **Raw outputs**, not in the repository: arms, scores and the ledger under
  `~/vr-scratch/physical-align` on the development Mac; corpus evidence under
  `work/velocity-campaign/physical-align-corpus-20261009` on the LiDAR volume.
