# The rectangle heading on kirk0 and the corpus: the W1b screen

<!-- ignore-style-length -->

The [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md) found
that the solid body's heading does not converge, because nothing observes it: the faces are
found along the believed axis, and the believed axis is a PCA heading seeded at birth. W1a
built a per-frame rectangle fit that reads the body's axis within 2.6° of kirk0's reviewed
yaw. This report scores W1b's first build, `solid_body_rectangle_heading`, which makes that fit
the heading observation, on the frozen kirk0 split and the 23-site corpus, against the levels
the [criteria record](geometry-convergence-criteria.md) predeclared.

- **Status:** Complete, 2026-10-09; the σ scale on the full arm added 2026-10-09 late. kirk0 scored, corpus screened, cost measured
- **Layers:** L4 rectangle fit, L5 solid body
- **Related:** [convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md#41-an-orientation-observation-from-the-cluster) (W1b), [criteria record](geometry-convergence-criteria.md), [containment screen](solid-body-containment-corpus-2026-10.md) (W2), [alignment report](solid-body-physical-alignment-kirk0-2026-10.md), [kirk0 pilot](physical-reference-pilot-kirk0-2026-10.md)

## Summary

- **The heading converges.** On kirk0 the share of rows more than 30° off the course after two
  seconds falls from 0.56 and 0.41 to 0.04, inside G3's 0.05; the axis is within the reviewed
  yaw's bound at the median and 1.3° beyond it at p90 (G1), and truck 2's heading goes from
  75.8° to 1.1° off. On the 23-site corpus the rectangle heading beats A2 at every site at both
  ages, from 0.23 and 0.26 to 0.040 and 0.043 for the full arm with course fusion.
- **A right axis fixes what the alignment patches were reaching for.** Truck 1 grows from 5.9 to
  8.6 m without growth admission, and containment's floor brackets the true axis, so the reported
  length is short of the frame's span in 1 % of rows rather than 3.6 %. The full arm, the
  rectangle heading in the course heading's place beside containment, growth, centring and the
  vehicle floor, is the best geometry scored on kirk0: mean box IoU 0.549, 12 of 14 centres
  within bound, 14 of 15 lengths, body-centre lateral p99 0.099 m against A2's 0.291.
- **The fit adds a lateral tail, as the plan warned.** Against A2 with containment the
  rectangle heading raises the body-centre p99 by 0.029 m at the corpus median (17 sites worse),
  and on the windows both arms score the p99 rises while p95 and the median do not. Fusing the
  course as a second axis observation at speed, built during this run, cuts that to 0.008 m.
- **On the full arm, σ × 1.5 puts the lateral tail below the alignment candidate's.** On the
  15,166 windows both score its p99 is 0.162 m against 0.175, and it is better per site on the
  body-centre and all-window p99 at 15 of 23; the fused G3 is 0.026 and 0.029. This is the
  setting the report recommends carrying.
- **Weighing each fit at twice its σ removes most of the tail that remains.** On the matched
  windows the rise against A2 with containment falls from 0.023 to 0.005 m at p99, and G3 goes
  to 0.025 and 0.031; on kirk0 the axis lags more (G1 p90 5.0° against 2.7°), inside its level.
- **The full arm with fusion is level with the alignment candidate on lateral** (the same
  matched-window p99, 0.171 m; all-window p99 0.015 m lower at 15 of 23 sites) and ahead of it
  on held share at 21 sites; on kirk0 ahead on heading (G1 p90 1.2° against 5.4°) and box.
- **A W1a diagnostic was wrong on the corpus and is corrected.** The summary compared the fit's
  axis with the course modulo 180° where the fit is defined modulo 90°, so it read most corpus
  rows as far off; kirk0's traffic directions hid it. Re-read, the fit's own axis is over 30°
  off on about 6 % of rows after two seconds, close to the level.
- **Still open:** car 8's one pose under 2 m/s on a 0.3 s-old track has its axis right and its
  label 90° off, the low-speed label not built here; identity under A2 moves with any change
  (switches 65 to 118, IDF1 down on association), so the route is shadow-first; and the cost
  is in the table below.

## Question

Does a heading taken from each frame's rectangle fit, filtered and labelled, converge where the
tracked heading does not, and what does a right heading do to the centre, extents, lateral
residual, identity and cost, alone and with containment?

## What the experiment does

`solid_body_rectangle_heading` (`SolidBodyOptions.RectangleHeading`, in `solid_body_axis.go`):

- **The axis is a filtered state.** Each associated frame's `l4perception.FitRectangle`
  observes the axis modulo 90°, with the fit's own σ (3° floor) and an abstention when the
  cluster has no edges. The axis is a scalar Kalman filter on the quarter-turn circle: a random
  walk at (0.1 rad)² per second, about 1.8° per frame, updated by every fit that does not
  abstain, the innovation taken modulo 90° so a fit at 89° and a state at 1° are 2° apart. A fit
  beyond 3σ of the prediction is refused, and three refusals in a row restart the axis from the
  fit, so a wrong first fit cannot hold a track. The process noise is a reasoned choice, not a
  tuned one.
- **The label picks one of four headings.** The axis says which way the sides run, not which is
  the length or the front. Of the four headings axis + k·90°, the label takes the one nearest the
  course at 2 m/s or more, which resolves length and front together; below 2 m/s, the one
  nearest the last heading, keeping its ambiguity; with no last heading, the one nearest the
  tracked heading, unresolved.
- **Faces and spans along it.** The near-edge faces are found along the labelled heading rather
  than the course (`solid_body_course_faces` is overridden while the axis is known), and spans
  are admitted along it without the course check while its σ is within 5°.
- **Until the axis is observed** the heading is what it would be without the option. The row
  records the frame's fit, computed once per frame.

A second option, `solid_body_rectangle_course_fusion` (`RectangleCourseFusion`), was built
during this run after the first corpus sites showed the lateral residual rising (see below). At
2 m/s or more it takes the course as a second observation of the axis, modulo 90°, with the
course's own variance plus a 3° sideslip allowance, gated like a fit; a course beyond the gate
is a turn or a manoeuvre and is left out, never counting toward a restart, and the course never
starts an axis on its own. It also holds the label through a turn: a resolved body keeps its
quadrant while the course sits more than 35° from the nearest candidate heading, as a velocity
lagging a turn does, and takes only the front from the course. With the course fused into the
axis, G3 (axis against the course) is no longer an independent reading, and G1 against the
references is the check.

Not built here, from the plan's W1b: the label weight from the extent belief's aspect below
2 m/s, the ambiguous row with its span-floored face update, and retiring the tracked heading's
guards. The experiment leaves the tracked filter's own heading alone; under A2 the solid body's
faces move the tracked state, so identity can move.

## Method

- **Arms.** `a2`: `solid_body`, `solid_body_face_hysteresis`, `solid_body_course_faces`,
  `solid_body_full_members`, `near_edge_track`. `a2-rh`: with `solid_body_rectangle_heading`.
  `a2-rh-cont`: with that and `solid_body_containment`. `a2-new`: `a2-rh-cont` with three of the
  alignment candidate's four experiments (`solid_body_extent_growth`,
  `solid_body_end_face_centring_open_prior`, `solid_body_vehicle_extent_floor`), the rectangle
  heading taking the course heading's place. `a2-allo-cont`: the alignment candidate with
  containment, the arm `a2-new` is meant to replace. The `-cf` arms add
  `solid_body_rectangle_course_fusion`. On kirk0 also the shadow (`sh`, the same without
  `near_edge_track`), where identity cannot move.
- **kirk0.** Frozen split `kirk0-ad8b9438-tuning-r1`, episode `following-472-530` for physical
  scoring and `whole-pack` for identity, the `d1` arms replayed at `be8ace881` and the
  course-fusion `d4` arms on a build of the working tree that became `d5ff96fb6`, stamped
  `cf-wip`; the control `d1-a2` reproduces A2's summary at every earlier build exactly.
- **Corpus.** The D2 first-segment corpus, 23 tuning and screen sites (`embarcadero-folsom`
  held out), 200 s scored after a 70 s warm-up: pass 5 of the alignment run's harness, `a2-rh`,
  `a2-rh-cont` and `a2-new` at `be8ace881`, and pass 6, `a2-rh-cf-cont` and `a2-new-cf` at
  `d5ff96fb6`, run side by side, against `a2`, `a2-cont` and `a2-allo-cont` from passes 1, 4 and
  3, whose A2 control is identical across those builds. Pass 1's `a2` predates the W0 measures
  and has no axis-by-age table, so `a2-cont` (pass 4), which leaves the heading alone, is A2's G3
  baseline.
- **The levels** are the criteria record's as written. G3 reads the axis column of
  `axis_by_age`, the reported heading folded to an axis so that no label touches it, with the
  directed column beside it as a labelling check. G1 against kirk0's references compares a fit
  with a fit (the references were fitted to the same returns, within 3.7° of resolution), which
  the record calls necessary, not sufficient.

## Findings on kirk0

**Per vehicle**, medians over scored poses (truck 1 10.63 m, car 8 4.26 m, truck 2 9.63 m long):

| Arm                    | Truck 1 centre / yaw / length | Car 8 centre / yaw / length | Truck 2 centre / yaw / length | Mean box IoU |
| ---------------------- | ----------------------------- | --------------------------- | ----------------------------- | ------------ |
| A2                     | 2.19 m / 6.7° / 5.88 m        | 0.25 m / 6.0° / 3.88 m      | 2.00 m / 75.8° / 0.63 m       | 0.413        |
| + rect heading         | 0.97 / 2.2 / 8.63             | 0.19 / 2.3 / 3.88           | 1.93 / 1.1 / 0.63             | 0.461        |
| + containment          | 2.40 / 2.4 / 0.20 (fragment)  | 0.14 / 2.3 / 3.99           | 1.92 / 1.1 / 0.75             | 0.309        |
| `a2-new`               | 0.79 / 2.1 / 9.13             | 0.13 / 2.3 / 3.99           | 1.92 / 1.1 / 0.75             | 0.549        |
| + course fusion        | 0.97 / 3.2 / 8.63             | 0.19 / 2.3 / 3.88           | 1.94 / 1.0 / 0.63             | 0.487        |
| + fusion + containment | 1.00 / 3.2 / 8.63             | 0.13 / 2.3 / 3.99           | 1.92 / 1.2 / 0.75             | 0.511        |
| `a2-new-cf`            | 0.82 / 1.8 / 9.13             | 0.13 / 2.2 / 3.98           | 1.92 / 1.3 / 0.75             | 0.536        |
| `a2-allo-cont`         | 0.91 / 4.4 / 8.88             | 0.23 / 6.0 / 3.91           | 1.93 / 2.9 / 0.75             | 0.522        |
| Shadow                 | 2.62 / 85.0 / 0.38            | 0.24 / 6.0 / 3.88           | 1.99 / 125.9 / 0.63           | 0.258        |
| + rect heading         | 2.47 / 1.9 / 0.29             | 0.24 / 2.3 / 3.88           | 1.92 / 1.1 / 0.63             | 0.310        |

**Against the levels**, A2 and its arms. G1 is the axis error beyond the pose's yaw bound
(median / p90; truck 2's median apart), G2 the poses directed within 10° beyond the bound (truck
2 apart), G3 the share of rows over 30° off the course at 2 to 4 s and after 4 s, G5 the
body-centre rows holding 98 % of their points, G6 the rows whose reported length / width is short
of the frame's span, G7 centres within bound, G8 lengths within 15 % (car) or 20 % (trucks), G9
gaps within bound with the mean gap error, G10 the body-centre lateral p99 with its window count:

| Arm                    | G1            | G2         | G3                  | G5   | G6            | G7    | G8    | G9          | G10         | Lapses | G11 switches; HOTA; IDF1 |
| ---------------------- | ------------- | ---------- | ------------------- | ---- | ------------- | ----- | ----- | ----------- | ----------- | ------ | ------------------------ |
| A2                     | 2.7 / 5.6; 13 | 13/15; 0/6 | 0.56 / 0.41         | 0.18 | 0.45 / 0.20   | 5/13  | 5/15  | 3/7; 1.15 m | 0.291 (220) | 49     | 100; 0.310; 0.285        |
| + rect heading         | 0 / 1.3; 0.3  | 14/16; 6/6 | 0.04 / 0.04         | 0.38 | 0.21 / 0.21   | 10/13 | 11/16 | 3/7; 1.68   | 0.266 (177) | 46     | 65; 0.304; 0.273         |
| + containment          | 0 / 1.3; 0.3  | 13/15; 6/6 | 0.04 / 0.04         | 0.48 | 0.010 / 0.023 | 5/9   | 6/15  | 0/7; 4.64   | 0.200 (237) | 31     | 108; 0.306; 0.303        |
| `a2-new`               | 0 / 1.3; 0.3  | 14/15; 6/6 | 0.05 / 0.05         | 0.59 | 0.007 / 0.016 | 12/14 | 14/15 | 3/7; 1.58   | 0.099 (183) | 26     | 99; 0.306; 0.242         |
| + course fusion        | 0 / 1.4; 0.1  | 15/16; 6/6 | 0 / 0 (fused)       | 0.37 | 0.21 / 0.21   | 10/12 | 11/16 | 3/7; 1.69   | 0.379 (236) | 49     | 66; 0.304; 0.282         |
| + fusion + containment | 0 / 2.7; 0.4  | 15/16; 6/6 | 0 / 0 (fused)       | 0.48 | 0.011 / 0.027 | 10/13 | 14/16 | 3/7; 1.60   | 0.178 (255) | 18     | 90; 0.305; 0.270         |
| `a2-new-cf`            | 0 / 1.2; 0.4  | 14/15; 6/6 | 0.02 / 0.03 (fused) | 0.58 | 0.012 / 0.014 | 11/13 | 13/15 | 3/7; 1.58   | 0.136 (205) | 20     | 110; 0.306; 0.259        |
| `a2-allo-cont`         | 0 / 5.4; 1.5  | 13/15; 6/6 | 0.05 / 0.19         | 0.40 | 0.037 / 0.048 | 11/14 | 14/15 | 3/7; 1.56   | 0.156 (176) | 27     | 118; 0.310; 0.272        |

- **The heading converges.** The share of rows more than 30° off the course after two seconds
  falls from 0.56 and 0.41 to 0.04 at both ages, inside G3's 0.05, and stays there under every
  arm that carries the rectangle heading; the axis median is 3 to 5° at every age where A2's is
  14 to 35°. The old candidate's course heading reaches 0.05 at 2 to 4 s and 0.19 after four,
  where its blend toward the course lets a turning or slowing body's heading go.
- **Truck 2 is headed along its axis.** Its directed error falls from 75.8° to 1.1° and all six
  poses pass G2. Its length stays the end face's depth and its centre 1.9 m off: its cab never
  reaches the tracker, which no heading can fix (W3).
- **Truck 1 grows without growth admission.** With the faces found along the right axis, the
  spans admitted along it are the truck's: length 5.88 to 8.63 m, centre error 2.19 to 0.97 m.
  The alignment run needed growth admission to reach 8.6 m on the old axis.
- **The one body G2 failure is the predicted one.** Under every rectangle-heading arm car 8 at
  sample 491 is 89° off directed with the axis within 1.1° of the reference: the label, carried
  from a PCA heading on a track 0.3 s old at 0.8 to 1.6 m/s, picked the perpendicular. That is
  the plan's low-speed case, the part of W1b not built here. Car 8's other slow pose, 495, is now
  right. (The second failure under `a2-rh` alone is truck 1's cab fragment, matched at sample
  494 because the body track's centre was outside the scorer's match there.)
- **A course rule for slow bodies does not reach it.** A variant that labelled by the course
  below 2 m/s whenever the course was known to 15° and the speed was at least 0.5 m/s changed no
  kirk0 row's score: on a track that young the velocity's variance is still the seed's, about
  1 m/s, so the course is known to 35° at best. It was not kept. What reaches that pose is either
  the label from the accumulated extents' aspect, as the plan has it, or reporting the row as
  unresolved until a cue arrives.
- **The full arm is the best geometry yet scored on kirk0.** `a2-new` takes the mean box IoU
  to 0.549 (0.522 for the old candidate with containment), 12 of 14 centres within bound, 14 of
  15 lengths, and the body-centre lateral p99 to 0.099 m from A2's 0.291; its switches are 99
  against 100. IDF1 falls from 0.285 to 0.242, which the record reports but does not gate: the
  matches (1,250 against 1,242) and DetA (0.222 against 0.219) are unchanged and AssA falls from
  0.514 to 0.484, so which track holds which vehicle changed, not how many frames are found. On
  the shadow, where the solid body cannot move association, IDF1 stays at 0.330 in every arm.
- **Containment works better on a right axis.** With the rectangle heading the reported length
  is short of the frame's span in 1 % of rows rather than 3.6 %, and the held share is 0.48 to
  0.59 rather than 0.40: the floor searches 10° either side of the believed axis, which now
  brackets the true one.
- **Gaps.** Car 8 behind truck 1 improves (0.27 to 0.17 or 0.18 m mean error); truck 2 behind
  car 8 worsens (1.80 to 2.6 to 2.8 m), the missing cab's length. The heading across the truck
  had partly cancelled that upstream error, and a right heading exposes it.

**Rectangle heading with containment loses truck 1 to identity.** In that arm alone, truck 1's
reviewed instants match only the cab fragment (0.2 m long, 2.4 m off): an older truck 1 track
survives under containment where, with the rectangle heading alone, the tracker re-acquired it
2.5 s before the episode as a fresh track that grew to 8.6 m. The surviving track's length belief
stays 5.9 m, the merge test refusing the trailer's length as its side comes into view (the
alignment report's third mechanism), and its centre sits 4.6 m off, outside the scorer's match.
With growth admission (`a2-new`) the same track is 9.1 m long and 0.79 m off. It is one track on
one capture under A2's identity sensitivity, not a property of either option; it says the
rectangle heading needs a merge test that admits a growing view, which is W4's.

**Course fusion on kirk0** keeps the heading (G1 p90 1.2 to 2.7°, truck 2 within 1.3°) and, with
containment, keeps truck 1 whole (1.00 m off, 8.63 m long; IoU 0.511 against 0.309 without the
fusion), with the fewest lapses of any arm (18) and the body-centre p99 at 0.178 m. Alone its p99
is 0.379 m on 236 windows, and the full arm with it is 0.136 m against 0.099 without: on kirk0's
five to seven scored moving tracks a p99 is two or three windows, and the corpus decides it.
Car 8's sample 491 still fails G2 in every fusion arm, unresolved below 2 m/s where the fusion
does not act; the second failure of `a2-rh`, truck 1's cab fragment, does not recur.

**The shadow** shows the heading change without identity moving: truck 1's yaw 85.0° to 1.9°,
truck 2's 125.9° to 1.1°, car 8's 6.0° to 2.3°; G2 6 of 15 to 10 of 13; switches 96 to 94. With
the rectangle heading alone the shadow's truck 1 is still a fragment, so its centre and length
do not move. The full arm on the shadow (`d1-sh-new`, without fusion) takes the mean box IoU
from 0.258 to 0.458, against 0.357 for the alignment candidate with containment on the shadow,
centres within bound from 4 of 11 to 11 of 14, and truck 1 to 1.05 m off and 8.63 m long, with
switches 97 and IDF1 0.330 against 96 and 0.330.

## Findings on the corpus

**Heading convergence (G3)**, the share of rows more than 30° off the course, per site:

| Arm                     | 2 to 4 s, median (range) | After 4 s, median (range) | Sites at or under 0.05 at both |
| ----------------------- | ------------------------ | ------------------------- | ------------------------------ |
| A2 (`a2-cont`)          | 0.231 (0.110 to 0.392)   | 0.262 (0.042 to 0.494)    | 0                              |
| Rectangle heading       | 0.062 (0.026 to 0.250)   | 0.073 (0 to 0.324)        | 3                              |
| + containment           | 0.060 (0.013 to 0.153)   | 0.067 (0 to 0.260)        | 4                              |
| + fusion + containment  | 0.037 (0.007 to 0.177)   | 0.051 (0 to 0.220)        | 7                              |
| `a2-new`                | 0.058 (0.008 to 0.188)   | 0.056 (0 to 0.242)        | 5                              |
| `a2-new-cf`             | 0.040 (0.008 to 0.246)   | 0.043 (0 to 0.185)        | 10                             |
| `a2-allo-cont` (course) | 0.041 (0.009 to 0.093)   | 0.048 (0 to 0.095)        | 9                              |

The rectangle heading is better than A2 at every one of the 23 sites at both ages, with and
without fusion. Without fusion it is the independent reading: an axis filtered from fits alone,
within 30° of the course on 93 to 94 % of rows after two seconds, short of the level. With fusion the course is half the observation and the reading is partly the
course's agreement with itself, as the old candidate's always was.

**Lateral and the guards**, median per-site change against the arm without the rectangle
heading (sites worse / better):

| Measure                               | Rect. heading vs A2 | + containment vs A2 + containment | + fusion vs without | + fusion + containment vs A2 + containment | `a2-new` vs `a2-allo-cont` | `a2-new-cf` vs `a2-allo-cont` |
| ------------------------------------- | ------------------- | --------------------------------- | ------------------- | ------------------------------------------ | -------------------------- | ----------------------------- |
| Body-centre p99 (G10 as written)      | +0.020 m (14 / 9)   | +0.029 (17 / 6)                   | −0.016 (7 / 16)     | +0.008 (16 / 7)                            | +0.010 (12 / 11)           | +0.009 (12 / 11)              |
| All-window p95                        | +0.018 (16 / 7)     | +0.019 (18 / 5)                   | −0.007 (7 / 16)     | +0.017 (17 / 6)                            | +0.002 (12 / 11)           | −0.002 (11 / 12)              |
| All-window p99                        | +0.006 (13 / 10)    | +0.005 (12 / 11)                  | −0.005 (10 / 13)    | −0.005 (7 / 13)                            | −0.014 (9 / 14)            | −0.015 (8 / 15)               |
| Tracks with an excursion, all windows | +0.013 (14 / 6)     | +0.027 (13 / 9)                   | −0.002 (6 / 12)     | −0.005 (6 / 15)                            | −0.009 (7 / 13)            | 0 (8 / 11)                    |
| Lapses                                | +6 (15 / 8)         | +2 (12 / 9)                       | −1 (7 / 14)         | +1 (12 / 10)                               | +6 (16 / 7)                | +5 (16 / 7)                   |
| Near-edge fix share                   | +0.029 (1 / 22)     | +0.032 (2 / 21)                   | 0 (13 / 10)         | +0.029 (1 / 22)                            | +0.031 (5 / 18)            | +0.036 (4 / 19)               |
| Held share (G5)                       |                     | +0.072 (2 / 21)                   | +0.001 (10 / 13)    | +0.068 (2 / 21)                            | +0.045 (2 / 21)            | +0.044 (2 / 21)               |

**On the same windows.** Matching each arm's five-point windows to the baseline's by frame time
and position across all 23 sites:

| Pair                                       | Matched windows | p95              | p99              | Windows only the arm scores, p99 |
| ------------------------------------------ | --------------- | ---------------- | ---------------- | -------------------------------- |
| + fusion + containment vs A2 + containment | 14,359          | 0.076 to 0.079 m | 0.174 to 0.197 m | 2,960 at 0.306 m                 |
| `a2-new-cf` vs `a2-allo-cont`              | 15,154          | 0.075 to 0.072   | 0.171 to 0.171   | 2,614 at 0.307                   |

On eight of the sites, before fusion was built, the rectangle heading with containment took the
matched p99 from 0.156 to 0.192 m and fusion took it to 0.172, with p95 unchanged and the paired
median difference zero in both. Unlike W2's, this rise is on the windows both arms score: the
tail of the estimate itself, not a change of population.

- **Against A2 the rectangle heading costs a lateral tail**, about 0.02 to 0.03 m at the
  body-centre p99 and in p95 over all windows, at two thirds of sites. Fusion halves it at the
  matched p99 and removes most of it from the body-centre figure.
- **Against the alignment candidate the full arm is level.** With fusion its matched windows
  have the same p99 and a lower p95, its all-window p99 is 0.015 m lower at 15 of 23 sites, and
  its body-centre p99 is 0.009 m higher at 12 of 23: inside noise, and not above the baseline's
  in the sense G10's population-stable form would read it. It holds its points better at 21
  sites and fixes more often at 19.
- **Against plain A2**, the full arm with fusion halves the lapses at 22 sites and takes the
  all-window p99 down 0.077 m at 19 of 23; its body-centre p99 is level (+0.001 m, 12 / 11).

## A diagnostic corrected

The axis-by-age table's rectangle columns, added in W1a to compare the fit's own axis with the
course, folded the error modulo 180 degrees as a heading's is. The fit reports its axis in
[0°, 90°) and names the sides, not which is the length, so a body whose course lay between 90°
and 180° modulo 180 read as 90° off: on the corpus that column said the fit was more than 30°
off on 55 to 60 % of rows after two seconds, while the axis filtered from the same fits was
within 30° on 94 %. kirk0's two traffic directions both fold to about 60°, which is why W1a's
kirk0 figures were right and the error went unseen. The column now folds modulo 90 (`cc28d59c6`,
with a test whose fit reads 87° on a course of 0°); nothing in the tracker read it.

Re-read with the fixed scorecard from the same evidence, with `-scoring-start-seconds 70`:

| Arm                 | Fit's own axis over 30°, 2 to 4 s / after 4 s (median over sites) | Fit's median error, 2 to 4 s | Fit abstains, 2 to 4 s | Filtered axis over 30° |
| ------------------- | ----------------------------------------------------------------- | ---------------------------- | ---------------------- | ---------------------- |
| kirk0, `d1-a2-rh`   | 0.028 / 0.011                                                     | 3.0°                         | 21 %                   | 0.041 / 0.042          |
| Corpus, `a2-rh`     | 0.063 (0.033 to 0.262) / 0.089 (0 to 0.293)                       | 5.0°                         | 10 %                   | 0.062 / 0.073          |
| Corpus, `a2-new-cf` | 0.056 (0.016 to 0.267) / 0.076 (0 to 0.193)                       | 4.5°                         | 11 %                   | 0.040 / 0.043 (fused)  |

The fit's own axis is the independent G3 reading the third proposed amendment asks for; on the
corpus it sits just above the level, as the unfused filtered axis does.

## The fit's σ scale, the first recommendation tried

`solid_body_rectangle_sigma_wide` (`RectangleSigmaScale` = 2, at `266d7edee`) weighs each fit at
twice its own σ in the axis filter, the plan's scale c, on the rectangle heading with fusion and
containment. The row keeps the fit's own σ.

- **kirk0.** The axis lags more: G1 0.7° median and 5.0° p90 beyond the bound against 0° and 2.7°
  at c = 1, truck 1's yaw 6.4° against 3.2°, truck 2 still within 0.1°. The body-centre p99 is
  0.175 m against 0.178 on the same 255 windows; mean box IoU 0.508 against 0.511; 87 switches
  against 90; 15 of 16 lengths within tolerance against 14.
- **Corpus**, pass 7 at `266d7edee`, against the same arm at c = 1 and against A2 with
  containment (median per-site change, sites worse / better):

  | Measure                  | c = 2 vs c = 1                                   | c = 2 vs A2 + containment | c = 1 vs A2 + containment |
  | ------------------------ | ------------------------------------------------ | ------------------------- | ------------------------- |
  | G3, 2 to 4 s / after 4 s | 0.025 / 0.031, 14 sites at or under 0.05 at both | from 0.231 / 0.262        | 0.037 / 0.051, 7 sites    |
  | Body-centre p99          | −0.007 m (9 / 14)                                | +0.016 (16 / 7)           | +0.008 (16 / 7)           |
  | All-window p95           | −0.004 (8 / 15)                                  | +0.006 (17 / 6)           | +0.017 (17 / 6)           |
  | Matched-window p99       |                                                  | 0.173 to 0.178 m (14,628) | 0.174 to 0.197 m (14,359) |
  | Held share               | +0.003 (7 / 16)                                  | +0.073 (1 / 22)           | +0.068 (2 / 21)           |

  On the windows both arms score, the scale takes the tail the rectangle heading added from
  +0.023 m to +0.005 m at p99, with p95 unchanged; the per-site body-centre median differs from
  c = 1 by less than the sites differ among themselves, and stays above A2's at two thirds of
  them. G3 improves too, though with the fit weighed less the fused course carries more of the
  axis, so the third amendment's reading applies. The price is kirk0's lag above.

- **Reading.** A scale of two is worth carrying: it removes most of the matched-window tail the
  fit added and keeps G1 within its level on kirk0 (5.0° at p90 against 10°). Whether 1.5 keeps
  the tail down with less lag, and what it does to the full arm, is the next run.

## The σ scale on the full arm

The run that settles the scale for the candidate: `a2-new-cf` at σ × 1, × 1.5
(`solid_body_rectangle_sigma_mid`, added at `24f76bf18`) and × 2, on kirk0 and the 23 sites,
pass 8 at `24f76bf18`, every site `ok`. The kirk0 control and `a2-new-cf` at σ × 1 reproduce
their earlier rows exactly at this build.

**kirk0:**

| Full arm | G1 median / p90; truck 2 | Truck 1 yaw | Mean box IoU | G7 / G8      | Body-centre p99 (windows) | G3, 2 to 4 s / after 4 s | Switches; IDF1 |
| -------- | ------------------------ | ----------- | ------------ | ------------ | ------------------------- | ------------------------ | -------------- |
| σ × 1    | 0° / 1.2°; 0.4°          | 1.8°        | 0.536        | 11/13; 13/15 | 0.136 m (205)             | 0.016 / 0.032            | 110; 0.259     |
| σ × 1.5  | 0° / 1.0°; 0.5°          | 2.7°        | 0.533        | 10/13; 13/15 | 0.158 (214)               | 0.017 / 0.031            | 124; 0.262     |
| σ × 2    | 0.1° / 1.3°; 0.7°        | 3.3°        | 0.534        | 10/13; 13/14 | 0.196 (197)               | 0.017 / 0.032            | 123; 0.270     |

On the full arm the scale costs kirk0 almost nothing in heading, unlike on the rectangle heading
with fusion and containment alone (G1 p90 5.0° at × 2), because growth, centring and the vehicle
floor hold the extents the faces are placed by. Truck 1's yaw grows with the scale. Switches at
× 1.5 and × 2 sit just outside the 81-to-119 band, where × 1 was inside it.

**Corpus.** G3 at the corpus median, 2 to 4 s / after 4 s, with the sites under 0.05 at both
ages: × 1 0.040 / 0.043 (10), × 1.5 0.026 / 0.029 (10), × 2 0.024 / 0.023 (13); the alignment
candidate 0.041 / 0.048 (9). Median per-site change (sites worse / better):

| Measure                  | × 1.5 vs × 1      | × 2 vs × 1       | × 1 vs `a2-allo-cont`                      | × 1.5 vs `a2-allo-cont`                  | × 2 vs `a2-allo-cont`                    |
| ------------------------ | ----------------- | ---------------- | ------------------------------------------ | ---------------------------------------- | ---------------------------------------- |
| Body-centre p99          | −0.005 m (9 / 14) | +0.002 (12 / 11) | +0.009 (12 / 11)                           | −0.004 (8 / 15)                          | −0.001 (10 / 13)                         |
| All-window p95           | 0 (12 / 11)       | −0.003 (9 / 14)  | −0.002 (11 / 12)                           | −0.002 (10 / 13)                         | −0.002 (10 / 13)                         |
| All-window p99           | −0.002 (10 / 13)  | −0.001 (11 / 12) | −0.015 (8 / 15)                            | −0.013 (8 / 15)                          | −0.002 (11 / 12)                         |
| Tracks with an excursion | 0 (4 / 9)         | 0 (10 / 6)       | 0 (8 / 11)                                 | −0.010 (8 / 13)                          | −0.003 (7 / 13)                          |
| Held share               | −0.003 (16 / 7)   | +0.002 (10 / 13) | +0.044 (2 / 21)                            | +0.042 (2 / 21)                          | +0.049 (1 / 22)                          |
| Matched-window p95 / p99 |                   |                  | 0.075 to 0.072 m / 0.171 to 0.171 (15,154) | 0.075 to 0.070 / 0.175 to 0.162 (15,166) | 0.076 to 0.070 / 0.179 to 0.166 (15,607) |

Against plain A2, σ × 1.5 halves the lapses at 22 sites and takes the all-window p99 down
0.081 m at 20 of 23 and the excursion share down at 19.

The fit's own axis, the third amendment's reading, is the same at every scale, 0.061 / 0.093
over 30° at × 1.5 and 0.063 / 0.079 at × 2 against 0.056 / 0.076 at × 1: the scale changes how
the filter weighs the fit, not the fit. The improvement in the fused axis's G3 is the course
carrying more of it.

**Reading.** Carry σ × 1.5. On the windows both score, the full arm at × 1.5 has a p99 0.013 m
below the alignment candidate's where × 1 only matched it, and per site it beats that candidate
by the widest margins: the body-centre p99 at 15 of 23 sites, the all-window p99 at 15 and the
excursion share at 13 (× 2: 13, 12 and 13), while holding its points better at 21. Its
fused G3 is 0.026 and 0.029, and on kirk0 its axis is within 1.0° of the bound at p90. × 2 is
level with it on the matched windows and a little better on G3, with narrower per-site margins,
and lags truck 1 more. Neither settles identity: 123 and 124 switches on kirk0, just outside the band,
which is the shadow-first route's question rather than the scale's.

## Cost

`Tracker.Update` on kirk0, balanced order (each arm forward then reverse, twice), median of
eight runs each, nothing else running, at `d5ff96fb6` with the faster closeness loop:

| Arm                        | p50          | p99             | Frame p99 |
| -------------------------- | ------------ | --------------- | --------- |
| Shadow                     | 0.170 ms     | 3.058 ms        | 53.6 ms   |
| Shadow + rectangle heading | 0.460 (2.7×) | 3.374 (+10.3 %) | 53.7      |
| A2                         | 0.131        | 4.246           | 54.0      |
| A2 + rectangle heading     | 0.364 (2.8×) | 4.707 (+10.9 %) | 54.9      |
| `a2-new-cf`                | 0.646 (4.9×) | 4.184 (−1.5 %)  | 55.0      |

- **The fit is every associated cluster's, every frame.** At 110 µs per 256-point fit, after
  the closeness loop's `math.Min` and `math.Max` became comparisons (587 µs before, every kirk0
  row identical to the bit), a few associated clusters a frame nearly triple the median update.
  The p99, which the level reads, is set by frames with many tracks and rises about a tenth.
- **G12.** The full arm with fusion is inside the level (−1.5 % at p99); the rectangle heading
  alone is just outside it, +10.9 % on A2 and +10.3 % on the shadow. A frame's p99 rises 0.2 to
  1.9 %, and a CPU profile of the full arm puts the whole tracker at 1.2 % of a replay and the
  fit at 0.4 %: clustering and the background model are the replay's cost, not this.
- **Levers if the p99 must come down:** the fit's 256-point cap, a coarser first search, or
  fitting only when the axis's variance has grown past the fit's floor. None is tried here,
  and the Pi, where the plan expected several milliseconds per fit before the loop was made
  faster, is unmeasured.

## Against the predeclared levels

Read for the candidate this report recommends carrying, the full arm with fusion
(`a2-new-cf`), and for the rectangle heading with fusion and containment on its own:

| Id  | Level                            | `a2-new-cf`                                                                                 | Rectangle heading + fusion + containment             | Reading                                                    |
| --- | -------------------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------- | ---------------------------------------------------------- |
| G1  | Median 0°, p90 ≤ 10°             | 0° / 1.2°; truck 2 0.4°                                                                     | 0° / 2.7°; truck 2 0.4°                              | Met on kirk0, a fit scored against fits                    |
| G2  | ≤ 10° on 90 % of poses           | 14 of 15 (93 %); truck 2 6 of 6                                                             | 15 of 16; 6 of 6                                     | Met; car 8's slow pose the one miss                        |
| G3  | ≤ 0.05 at the corpus median      | 0.040 / 0.043; kirk0 0.02 / 0.03                                                            | 0.037 / 0.051                                        | Met at both ages for `a2-new-cf`, partly self-measured     |
| G5  | ≥ 0.95                           | kirk0 0.58; corpus 0.70                                                                     | kirk0 0.48; corpus 0.68                              | Not met; amendment 2 stands                                |
| G6  | 0 short                          | 0.012 / 0.014                                                                               | 0.011 / 0.027                                        | Nearer than W2 alone; amendment 2                          |
| G7  | ≥ 80 % within bound              | 11 of 13 (85 %)                                                                             | 10 of 13                                             | Met by the full arm on kirk0                               |
| G8  | ≥ 80 % within 15 % / 20 %        | 13 of 15 (87 %)                                                                             | 14 of 16                                             | Met on kirk0                                               |
| G9  | Every gap within bound           | 3 of 7                                                                                      | 3 of 7                                               | Not met: truck 2's missing cab (W3)                        |
| G10 | p99 not above the baseline's     | kirk0 0.136 vs 0.291; corpus +0.001 m vs A2, +0.009 vs the old candidate, matched p99 level | +0.008 m vs A2 + containment, matched p99 +0.023     | Level for the full arm; a small tail for the heading alone |
| G11 | Within 19 switches of 100        | 110                                                                                         | 90                                                   | Met, both                                                  |
| G12 | `Tracker.Update` p99 within 10 % | −1.5 % at p99, p50 4.9×                                                                     | Not timed; the heading alone +10.9 %, shadow +10.3 % | Met by the full arm; the heading alone just outside        |

## Interpretation

- **The heading converges because it is now observed.** Nothing about the rectangle heading is
  tuned to the corpus or the references: a fit to each frame's returns, filtered, and labelled
  by the course. That it holds G3 at every age on kirk0 and takes the corpus median from about
  a quarter of rows over 30° to near the level is what the plan said an observation would do,
  and the tracked heading's guards and the course blend were standing in for it.
- **A right axis fixes extents and centres the alignment patches were reaching for.** Truck 1
  grows to 8.6 m without growth admission, because spans are admitted along the axis they were
  measured on; containment's floor brackets the true axis, so its shortfall falls to about 1 %.
  On kirk0 the full arm with the rectangle heading in the course heading's place is the best
  geometry scored, and on the shadow, where identity is fixed, it more than doubles the control's
  mean box IoU.
- **The fit is a new jitter source, as the plan warned.** On the windows both arms score the
  body-centre p95 does not move and the paired median difference is zero, but the p99 grows:
  the fit's frame-to-frame noise reaches the centre through the faces, half a length times the
  angle, and shows in the tail. Fusing the course halves that growth, since the filtered
  velocity is steadier than any one frame's returns. What remains is the fit's σ being trusted
  further than it should be at speed, which is the scale c the plan said to calibrate and this
  build left at one.
- **Identity under A2 still moves with any solid-body change.** The switches run from 65 to 118
  across the arms here, inside and outside the band, and IDF1 falls on association rather than
  detection. The shadow keeps both fixed, which is the case for W5's shadow-first delivery.

## What this cannot establish

- A rate: three vehicles on one tuning capture, and a corpus with no references. G1 compares a
  fit with references fitted to the same returns; W7's operator-keyframed references are what
  break that.
- Whether the process noise and gate are right: both are reasoned choices, and the corpus's
  turns are the only test of them here.
- The low-speed label: car 8's pose is one case, and the rule that would fix it is not built.
- Identity: A2's switches move with any solid-body change; the arms here sit at 65 to 118
  against a band of 81 to 119.

## Recommendations

- **Carry the rectangle heading with course fusion forward as W1b's heading source**, under
  both names until W1b's remaining pieces are built: it meets G1 and G3 on kirk0 and the corpus,
  and fusion halves the lateral tail the fit alone adds.
- **Carry the full arm with the fit's σ at 1.5** (`a2-new-cf` with
  `solid_body_rectangle_sigma_mid`): on the corpus its matched-window p99 is below the
  alignment candidate's and it is better per site at 15 of 23, with G1 within 1.0° on kirk0.
  A turn-rate state remains the alternative if the intersection sites need less lag.
- **Build the low-speed label and the ambiguous row**, the parts of §4.1 not built here; car
  8's pose at 491 is their test, and a course rule cannot reach it on a track that young.
- **Score the full arm with fusion as the candidate that replaces the alignment candidate's
  course heading**, and retire `solid_body_course_heading` under W8 once the low-speed label
  exists: on the corpus the two are level on lateral, and on kirk0 the rectangle heading is
  ahead on heading, box and held share.
- **Bring W4's merge test forward of any A2 promotion.** Truck 1's loss under the rectangle
  heading with containment is the merge test refusing a growing view on a track identity kept,
  the alignment report's third mechanism, and growth admission is only its first version.
- **Deliver shadow-first (W5).** The geometry gains hold on the shadow with switches and IDF1
  unchanged; A2 adds an identity question this run cannot settle.

## Provenance

- **Code.** Branch `claude/geometry-convergence`: the experiment at `be8ace881`, the faster
  closeness loop at `f5fd9007e` (every kirk0 row identical to the bit). Every kirk0 arm and corpus
  pass 5 ran on `bin/baseline-be8ace881` and were scored with `bin/gt-eval-be8ace881`, SHA-256 in
  `bin/sha256.txt`.
- **kirk0 arms** `d1-a2`, `d1-a2-rh`, `d1-a2-rh-cont`, `d1-a2-new`, `d1-a2-allo-cont`, `d1-sh`,
  `d1-sh-rh`, `d1-sh-rh-cont`, `d1-sh-new` and `d1-sh-allo-cont` at `be8ace881`; `d4-a2-rh-cf`,
  `d4-a2-rh-cf-cont` and `d4-a2-new-cf` on the `cf-wip` build of the tree committed as
  `d5ff96fb6` (that binary was not kept); the dropped slow-label variant as `d3-*`, its patch
  kept as `slow-label-tried.patch`. All scored as `x-*` against `d1-a2` or `d1-sh` with
  `bin/gt-eval-be8ace881`, under `~/vr-scratch/physical-align`; levels read by `levels.py`.
- **Timing** `timing4.sh` at `d5ff96fb6`, under `timing-d5ff96fb6`; the profile under
  `prof-d5ff96fb6`. The corrected diagnostic re-read with `bin/scorecard-cc28d59c6` into
  `rect-fix/`.
- **Corpus pass 7.** 2026-10-09 22:49 to 23:20 UTC, `a2-rh-cf-cont-wide` at `266d7edee`, all 23
  sites replayed with their summaries and evidence; each site logged `ok=0` because no scorecard
  tool was built for that commit, so pass 7 has no `scorecard.json`, which nothing here reads.
  kirk0 arm `d5-a2-rh-cf-cont-wide`, scored with `bin/gt-eval-266d7edee`.
- **Corpus pass 5.** 2026-10-09 20:30 to 22:16 UTC, `a2-rh`, `a2-rh-cont` and `a2-new` at
  `be8ace881`, all 23 sites `ok`; pass 6, 20:51 to 22:05 UTC, `a2-rh-cf-cont` and `a2-new-cf` at
  `d5ff96fb6`, all 23 `ok`, the two passes run side by side. Evidence under
  `work/velocity-campaign/physical-align-corpus-20261009/<site>/<arm>` on the LiDAR volume;
  scripts `corpus/heading-summary.py`, `corpus/g3-sites.py` and `corpus/paired.py` under
  `~/vr-scratch/physical-align`.
- **Pass 8.** 2026-10-10 05:07 to 06:05 UTC, `a2-new-cf-mid` and `a2-new-cf-wide` at
  `24f76bf18`, all 23 sites `ok` with scorecards; kirk0 arms `d6-a2`, `d6-a2-new-cf`,
  `d6-a2-new-cf-mid`, `d6-a2-new-cf-wide` and `d6-a2-allo-cont` at the same build, scored with
  `bin/gt-eval-24f76bf18`; matched windows in `corpus/paired23-a2-new-cf-*.txt`.
- **References.** Pack `ad8b9438-c1d4-40f0-bd0a-f1852c984569`, split `kirk0-ad8b9438-tuning-r1`.
