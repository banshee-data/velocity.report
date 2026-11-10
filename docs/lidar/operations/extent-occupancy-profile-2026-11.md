# A body-frame occupancy profile as the extent estimator on kirk0 and two placements

<!-- ignore-style-length -->

The extent state is the weakest part of the shadow solid body. This report reads a default-off
shadow of it, `solid_body_extent_profile`, against a rule committed before the option existed. The
option keeps two one-dimensional occupancy grids per body, one along its believed axis and one
across it, after Schäfer, Alrifaee and Hashemi 2026. It answers
[geom0019](../../../data/experiments/geom/geom0019.md).

- **Status:** Measured on kirk0 and on the first five-minute segments of ashbury-downey and
  haight-divisadero, 2026-11-10. The rule fails on all four clauses. Nothing adopted, no default
  changed
- **Spec:** [geom0019](../../../data/experiments/geom/geom0019.md); rule at `ab19e51`, before the
  option existed; constants fixed at `54111f9` and the admission's axis at `882ab9a`, each before
  any scored replay
- **Layers:** L5, the shadow solid body and its new profile block; scored against L10 physical
  references on kirk0 and label-free on the placements
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`. Every option is a replay experiment, not a tuning key
- **Related:** [extent bound accrual (geom0018)](extent-bound-accrual-kirk0-2026-11.md),
  [geom0015's bias baseline](d2-frozen-split-bias-baseline-2026-11.md),
  [face-state maths review §3](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md),
  [state plan §9.2.4](../../plans/lidar-state-estimation-plan.md#924-extents-in-the-state-and-censored-extent-evidence-delivered-default-off)

## Answer

- **The profile is not a candidate.** P1, P2, P3 and P4 all fail. The spec's reading for a P1
  fail with P3 and P4 holding does not apply, because P3 and P4 fail too.
- **On kirk0's reviewed poses the profile almost never exists.** It updates only along an axis
  believed within 5 degrees. On the three reviewed vehicles the solid body's orientation has a
  standard deviation of 33 to 44 degrees at the median, and the course 9 to 15 degrees, at a
  median 2 to 4 m/s. Truck 1 and car 8 admit no frame before or during their poses, so P1's truck 1 clause and
  all of P2 have no profile to read. Truck 2's course settles at 75.2 s, and the profile locks at
  75.3 s, so only its last pose, 509, has a profile.
- **Where it exists, it measures truck 2's length.** On pose 509 the profile reads 9.50 by 3.00 m
  against the reference's 9.63 by 2.84 m, with all four ends observed. Its centre is 0.28 m from
  the reference, where the extent state reads 4.38 m and its centre is 2.66 m off. Over the next
  two seconds it reads 9.6 to 9.9 m. That is one pose, and P1 asks for the median of six.
- **It drifts with its own origin.** The grid sits on the solid body's centre, and that centre
  moves whenever the extent state moves. At 77.4 s truck 2's extent state takes its first bound
  and its half-length goes from 2.14 to 4.48 m. The centre moves about 2.3 m along the body, and
  the profile, which cannot take its locked cells back, reads 12.3 to 12.5 m from then on. The
  same mechanism makes the falls P3 counts. On ashbury-downey a car's profile grows from 4.2 to
  6.1 m as its reference slides, then free cells trim it back to 5.1 m.
- **Label-free, it is steadier in length on one placement and not on the other, and holds fewer
  withheld returns than the box.** P3: on ashbury-downey its within-track length spread is 0.60 m
  against the box's 0.95 m, but 7 of 15 tracks shorten by more than 0.5 m after lock-on. On
  haight-divisadero no track shortens, but its spread of 0.20 m exceeds the box's 0.00 m, since the
  box holds its length. P4: the profile holds 97.1 % and 94.9 % of the withheld frames' returns,
  where the box holds 100 % on both. The area ratio holds, at 1.13 and 1.06.

## Question and rule

The rule is in the [spec](../../../data/experiments/geom/geom0019.md), written before the option
existed:

- **P1, long bodies.** Truck 2's median absolute length error over its six poses is at least
  0.5 m under the published box's 3.04 m. Truck 1's over its nine poses is no more than 0.25 m
  over the box's.
- **P2, cars.** Car 8's median absolute length error over its seven poses is at most 0.365 m, and
  its median width is under 2.0 m (reference 1.73 m).
- **P3, steadiness, label-free.** On each placement segment, over confirmed vehicle tracks of
  3 s or more, the median within-track spread of the profile's length after the first second
  (p90 less p10) is no more than the box's. The share of tracks whose length falls by more than
  0.5 m after lock-on is under 5 %.
- **P4, withheld frames, label-free.** Every fifth admitted frame is withheld from the update. On
  those frames the profile's extent at the tracked pose, grown by 0.25 m, holds at least the box's
  share of the frame's returns less two points. The profile's area is no more than 1.15 times the
  box's at the median.
- **Verdict.** A candidate when P1 to P4 hold.

The implement note fixed the measurement details before any replay. P3 and P4 are read per
segment, and each must hold on both. P3's spread is taken over the frames after a track's first
scored second on which the profile is known, with the box's spread on the same frames. A track
with fewer than two such frames is counted apart. P4 reads all confirmed tracks' withheld frames
with both grids locked. The box's share is the published box grown by the same 0.25 m, over the
same returns, and each share is the mean over frames. The returns-weighted share is reported
beside it. A reviewed pose without a profile cannot meet P1 or P2: the option reports nothing
there.

## What the option does

`SolidBodyOptions.ExtentProfile`, replay name `solid_body_extent_profile`
([`solid_body_extent_profile.go`](../../../internal/lidar/l5tracks/solid_body_extent_profile.go)),
reads the solid body after its update and the frame's returns. It writes only its own grids and the
reading's `Profile` block. No track, no solid-body state and no published box changes.

- **Grids.** Two one-dimensional grids of 0.1 m cells in the body frame, origin at the solid
  body's reference (the body centre it believes), from −15 to +15 m along the axis and −5 to +5 m
  across it. Each cell holds a binary Bayes log-odds, prior 0, clamped to −2.0 and +3.5. The axis
  keeps its sense across a direction flip.
- **Occupied.** A cell takes +0.85 (p 0.70) on a frame where a return in the height band falls in
  it. Along cells use each return's coordinate along the axis and across cells its coordinate
  across, each a marginal of the other. The band is 0.2 to 4.0 m above the cluster's lowest
  return, which leaves out the lowest decimetres, where ground returns and spray survive foreground
  extraction.
- **Free.** The ray model is two-dimensional, from the sensor origin, in azimuth columns of
  0.4 degrees, about two of the Pandar40P's firings at 10 Hz. Each column holds the range of the
  nearest in-band return of any of the frame's clusters, so a vehicle in front shadows the one
  behind. A body-frame point is crossed without a return when it lies 0.3 m or more short of its
  column's nearest return. It is also crossed when its column is empty and lies outside the
  bearings of the track's cluster and of the profile's locked footprint. A cell is free, −0.40
  (p 0.40), when a crossed point lies in it within the other grid's extent, sampled 0.3 m inside
  that extent's ends. A cell occupied on the frame is not also freed. The model takes the sensor's
  vertical fan to cross the band at every range the grid covers. It does not see static
  background, so a parked car shadowing an end reads as free.
- **Height band and a low bonnet.** A ray over a low bonnet does not free the bonnet's cell while
  any in-band return in its column lies nearer, at any height. A column inside the body's bearings
  without a return is unknown, not free. Glass, dark paint and a grazing side too sparse for DBSCAN
  all leave such gaps.
- **Admission.** A frame updates the grids only while the body is referenced to its centre, past
  initialising, and has a believed axis. That is geom0018's rule: its orientation with a standard
  deviation of at most 5 degrees that the course at speed does not contradict, otherwise the
  course with its own standard deviation within 5 degrees. Other frames are refused and counted,
  the heading's refusals apart.
- **The reading.** The extent along each axis is the span from the lowest to the highest cell at
  or above the lock-on log-odds of 1.5. Each end is observed when a cell within 0.3 m beyond it
  reads −0.4 or less, and censored otherwise. The profile is known once both grids have a locked
  cell. Every fifth admitted frame is withheld from the update and marked.
- **Output.** The block is carried on the reading and not persisted: the solid-body table has no
  column for it, and adding one needs a migration. `lidar-box-eval -rows` writes the solid body's
  pose and extents, the profile's extents, ends, pose and counters, and on a withheld frame the P4
  shares and area ratio
  ([`extent_profile.go`](../../../cmd/tools/lidar-box-eval/extent_profile.go)).
- **Memory and cost.** 400 float32 per track, allocated on the track's first associated frame
  under the option. The free test samples only cells within 2 m of the extent, at 0.1 m along each
  cell's strip.

**Before the scored replays.** Two changes were made after the option existed and before any
figure of the rule was read. Both are stated in the spec's implement note. The synthetic
through-truck unit test showed a receding truck's grazing side, dropped by DBSCAN, free its whole
length in one frame. So the locked footprint's bearings count as the body's, and the strips are
sampled inside their ends (`e11c400`). The first build then admitted a frame only on the
orientation's own variance. Under the face-state options that is the tracked heading's jitter.
Label-free replays of the two placement segments at `c820d05` admitted 17 frames and refused
8,052, so no profile locked and no rule figure could be computed. The course was added as geom0018's believed
axis (`0449284`). Every figure below is at `0449284`.

## kirk0: the existing arms reproduce geom0018

All four arms ran `lidar-state-estimation-baseline` on kirk0 whole, repeat-verified. Every arm's
tracking baseline is `79d9abff…`, first and repeat, so the face-state options, the accrual and the
profile change no track. The solid-body rows below are `perframe -b-solid-body` on each arm's
baseline evidence, gate 3 m, on geom0015's 22 poses. They are geom0018's second-version rows.

| Arm          | Matched | Median centre | Truck 1: L, \|err\|, W | Car 8: L, \|err\|, W | Truck 2: L, \|err\|, W | Geom0018 (second version)               |
| ------------ | ------: | ------------: | ---------------------- | -------------------- | ---------------------- | --------------------------------------- |
| `a0` box     | 22 / 22 |       0.190 m | 10.60, 0.12, 2.94      | 4.40, 0.14, 1.87     | 6.59, 3.04, 2.84       | 0.190 m; truck 2 L 6.59                 |
| `face-state` | 22 / 22 |       1.028 m | 9.73, 0.90, 3.82       | 3.90, 0.35, 2.45     | 4.60, 5.03, 1.90       | 1.028 m; 9.73, 3.90, 4.60               |
| `accrual`    | 22 / 22 |       0.771 m | 9.62, 1.01, 3.13       | 3.92, 0.34, 2.44     | 4.83, 4.81, 2.88       | 0.771 m; 9.62, 3.92, 4.83               |
| `profile`    | 22 / 22 |       1.028 m | 9.73, 0.90, 3.82       | 3.90, 0.35, 2.45     | 4.60, 5.03, 1.90       | (new; its solid body is `face-state`'s) |

Car 8's error is 0.3548 m, which geom0018 rounded to 0.36 m: the same rows. The same arms through
`lidar-box-eval` with an evidence database give the same rows to the centimetre. Their
published-box rows are equal across all four arms, column for column with the track identifiers
set aside, and so are the placements' between `a0` and `profile`.

## kirk0: the profile on the reviewed poses

The profile is read from the profile arm's `lidar-box-eval -rows`, joined to each pose's
solid-body row by frame time and position (to 0.01 m).

| Vehicle | Poses | With a profile | Frames admitted by the last pose | Refused by then (heading alone) | Profile L, W           | Box L, \|err\| |
| ------- | ----: | -------------: | -------------------------------: | ------------------------------: | ---------------------- | -------------- |
| Truck 1 |     9 |              0 |                                0 |                       300 (285) | none                   | 10.60, 0.12    |
| Car 8   |     7 |              0 |                                0 |                       120 (108) | none                   | 4.40, 0.14     |
| Truck 2 |     6 |              1 |                                5 |                        111 (81) | 9.50, 3.00 on pose 509 | 6.59, 3.04     |

Why nothing is admitted, from the profile arm's solid-body rows over each track's life:

| Vehicle | Speed, p10 / median / p90 (m/s) | Orientation sigma, p10 / median | Course sigma, p10 / median | Frames with course sigma ≤ 5° |
| ------- | ------------------------------- | ------------------------------- | -------------------------- | ----------------------------: |
| Truck 1 | 0.9 / 2.2 / 5.6                 | 28.8° / 44.0°                   | 5.9° / 14.7°               |               10, from 91.9 s |
| Car 8   | 0.9 / 3.7 / 5.3                 | 33.2° / 44.1°                   | 5.4° / 8.5°                |                             0 |
| Truck 2 | 1.3 / 2.9 / 7.0                 | 25.3° / 33.1°                   | 4.7° / 11.1°               |               48, from 75.2 s |

Truck 2's six poses, in metres (the profile's centre is its extents' midpoint placed at its
pose):

| Pose | Time   | Box L (centre error) | Extent state L (centre error) | Profile L, W (centre error) | Ends (rear, front, right, left) |
| ---: | ------ | -------------------- | ----------------------------- | --------------------------- | ------------------------------- |
|  491 | 73.8 s | 4.10 (2.95)          | 4.84 (2.66)                   | none                        |                                 |
|  495 | 74.2 s | 2.78 (2.83)          | 4.80 (2.67)                   | none                        |                                 |
|  500 | 74.7 s | 3.59 (2.50)          | 4.64 (2.63)                   | none                        |                                 |
|  502 | 74.9 s | 9.42 (0.30)          | 4.56 (2.65)                   | none                        |                                 |
|  504 | 75.1 s | 9.09 (0.49)          | 4.50 (2.70)                   | none                        |                                 |
|  509 | 75.6 s | 9.49 (0.21)          | 4.38 (2.66)                   | 9.50, 3.00 (0.28)           | observed on all four            |

The box's own median over the six poses is 6.59 m because its first three read 2.8 to 4.1 m.
Measured against the reference's 9.63 m, the profile's one pose has a length error of 0.13 m.
The box's error is 3.04 m at the median of six, and 0.14 m on that pose.

**After the poses** (label-free). The profile reads 9.5 m at 75.3 s, then 9.6 to 9.9 m to 77.3 s,
with all four ends observed on every frame. At 77.4 s the extent state takes its first bound
along the course and goes from 4.28 to 8.96 m. The solid body's centre, the grid's origin, moves
about 2.3 m along the body. From 77.5 s the profile reads 12.3 to 12.5 m, and its width 3.2 to
3.9 m. The withheld frames of 75.6 to 79.6 s hold every return under both the profile and the box.

**kirk0 overall** (label-free). 329 frames were admitted of 6,798, and 6,469 refused, 4,114 of
them by the heading alone; 11 of 79 confirmed tracks locked. P3's measures on kirk0 read a spread
of 0.61 m against the box's 0.63 m over 8 tracks, with 3 of 8 falling. P4's read a share of
97.5 % against 100 % over 60 withheld frames, with an area ratio of 1.17. Neither is a gate on
kirk0.

## The placements

`lidar-box-eval -variants none -rows -require-settled=false -sample-points 1024` with an evidence
database, 40 s warm-up and 120 s scored, `a0` and `profile`. The published boxes of the two arms
are equal row for row.

| Segment              | Admitted / refused (heading) | Tracks locked | Vehicle tracks ≥ 3 s (scored) | Profile spread | Box spread | Falls > 0.5 m | Withheld frames | Profile share (pooled) | Box share | Area ratio |
| -------------------- | ---------------------------- | ------------: | ----------------------------: | -------------: | ---------: | ------------- | --------------: | ---------------------- | --------: | ---------: |
| ashbury-downey-s1    | 944 / 2,108 (948)            |       16 / 40 |                       18 (15) |         0.60 m |     0.95 m | 7 / 15, 47 %  |             183 | 97.1 % (98.1 %)        |   100.0 % |       1.13 |
| haight-divisadero-s1 | 339 / 4,678 (2,791)          |       21 / 56 |                       19 (17) |         0.20 m |     0.00 m | 0 / 17, 0 %   |              59 | 94.9 % (93.5 %)        |   100.0 % |       1.06 |

Over the frames where the profile is known, the end marks read (observed, censored): on
ashbury-downey rear 884, 186; front 720, 350; right 923, 147; left 1,012, 58. On
haight-divisadero rear 315, 23; front 203, 135; right 291, 47; left 290, 48. The front is the end
most often censored, as a vehicle approaching shows its front and a receding one hides it.

## Against the rule

| Clause                | Figure                                                                                           | Result |
| --------------------- | ------------------------------------------------------------------------------------------------ | ------ |
| P1, truck 2           | profile on 1 of 6 poses (0.13 m on pose 509); no median of six. Box 3.04 m, so the bar is 2.54 m | fails  |
| P1, truck 1           | no profile on any of 9 poses; box 0.12 m                                                         | fails  |
| P2, car 8             | no profile on any of 7 poses                                                                     | fails  |
| P3, ashbury-downey    | spread 0.60 m against 0.95 m holds; falls 47 % against under 5 % fails                           | fails  |
| P3, haight-divisadero | spread 0.20 m against 0.00 m fails; falls 0 % holds                                              | fails  |
| P4, ashbury-downey    | share 97.1 % against 98.0 % (box 100.0 % less two points) fails; area 1.13 against 1.15 holds    | fails  |
| P4, haight-divisadero | share 94.9 % against 98.0 % fails; area 1.06 holds                                               | fails  |
| Verdict               | not a candidate; the P1-only branch does not apply                                               |        |

P1 fails however a missing pose is treated. Filled with the extent state's length, truck 2's
poses would read a median error near 4.9 m against the bar of 2.54 m, and truck 1's 0.90 m against
0.37 m. On ashbury-downey P4's pooled share of 98.1 % would pass where the mean of 97.1 % fails.
The mean was fixed as the measure before the replays, and haight-divisadero fails either way.

## Interpretation

Each reading is marked measured or inferred.

**The admission, not the grid, decides kirk0.** Measured: none of the three reviewed vehicles has
an orientation within 5 degrees at any point, and their course is within 5 degrees on 0 to 48
frames, all after or at the end of the poses. Inferred: P1's truck 1 clause and P2 fail because
the profile is never asked, not because it answers wrongly. The spec warned that a wrong heading
smears occupancy into width, and the gate that guards against that excludes these slow,
manoeuvring vehicles altogether. This is not the observation limit the spec's verdict names, a
body seen from one end. Truck 2 is seen side-on from 75 s, and the profile measures it within
0.13 m as soon as the course settles.

**A grid on a moving origin accumulates the origin's motion.** Measured: truck 2's profile reads
9.5 to 9.9 m until the extent state's first bound moves the solid body's centre by about 2.3 m,
then 12.3 to 12.5 m. On ashbury-downey a car's rear end walks from −1.7 to −3.3 m while its front
end stays between 2.2 and 2.8 m, and free cells later trim it back. Inferred: the falls P3 counts are the
grid correcting a smear its own origin made, and the length gains before them are the same smear.
The solid body's centre is a point the extent state places, not one fixed to the body. Schäfer
et al. place their grids on an extended Kalman filter's pose from an L-shape box; this shadow
places them on the centre of the estimator it is meant to replace, so the profile inherits that
estimator's errors of position along the body.

**The box holds every withheld return by construction.** Measured: the published box grown by
0.25 m holds 100.0 % of the withheld frames' returns on both placements. Inferred: the box is drawn
to contain its cluster, so P4's bar is the box's containment less two points. The profile misses
3 to 5 % because its extent placed at the solid body's pose is offset by the origin drift, and
because a locked extent cannot follow a frame whose returns lie beyond it. Its area stays within
1.15 of the box's, so it is not too large; it is in the wrong place.

**A length that never moves is steady.** Measured: on haight-divisadero the box's length spread is
0.00 m at the median; its length belief is held once established. Inferred: P3's spread clause
there compares the profile with a constant. On ashbury-downey, where the box's length moves, the
profile is steadier, 0.60 m against 0.95 m.

## Recommendations

1. Keep `solid_body_extent_profile` default-off, as a shadow. It does not pass its rule, and the
   two-dimensional successor the verdict names is not specified.
2. Anchor the grid to a point the body carries, not to the solid body's centre. Shift the grid by
   the centre's change along the axis whenever the extent state moves the centre, or anchor it to
   the observed near end, so that a change of the extent state is not read as occupancy. The
   kirk0 trace above is the test: truck 2's profile must hold near 9.6 m across 77.4 s.
3. Before restating P1 or P2, decide the admission. A 5-degree believed axis admits nothing on
   kirk0's reviewed poses. A gate on the axis's sigma as a function of the body's length, the
   smear a heading error makes being the half-length times its sine, would admit truck 2 earlier
   and still refuse a car at 30 degrees. That is a new rule, not this one, and needs its own spec.
4. Restate P4's bar against a containment the box does not hold by construction. One choice is
   the solid body's own extent at the same pose, which is the extent this shadow competes with.
5. If a column is added for the profile, give the replay manifest a code identity for experiment
   options first. geom0018's recommendation 5 applies here too: the parameter hash folds the
   option's name, so the two builds of this option that ran share one hash.

## What this does not establish

- **Anything beyond kirk0's three reviewed vehicles and two five-minute segments.** Truck 2's
  profile rests on one pose.
- **Physical accuracy of the placements' figures.** P3 and P4 are label-free. They measure what
  the pipeline drew against its own returns, not where the vehicles were.
- **The free-space model's errors.** It does not see static background, and a parked car shadowing
  an end reads as free; no reviewed pose tests that. It takes the vertical fan to cross the band at
  every range, which is not checked ring by ring.
- **Merges.** kirk0 has no cluster holding two reviewed objects. A merge writes occupancy the grid
  cannot take back, as the accrual record cannot.
- **Cost.** The kirk0 replays ran beside two other agents' jobs on a four-core host (load average
  10 to 13). `Tracker.Update` read p50 4.49 ms and p99 69.9 ms under `face-state` against 4.80 ms
  and 63.9 ms under `profile` on the first pass, and 7.64 / 97.4 ms against 7.87 / 122.9 ms on the
  repeat; `a0` read 0.75 / 16.8 ms. The profile's own share is within that noise and is not
  measured here.
- **Revisability and consistency.** The grids keep no frame identities.

## Provenance

| Item            | Value                                                                                                                                                                                                                                                                                            |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Base            | `ab19e51` (the November 10 review and approved specs)                                                                                                                                                                                                                                            |
| Commits         | constants `54111f9`; option `e11c400`; free-space note `c820d05`; believed axis `0449284`; its note `882ab9a`                                                                                                                                                                                    |
| Tools           | built at `0449284` by `make` with the Makefile's LDFLAGS and the `pcap` tag, `0.5.1-pre52`, `GitSHA` `04492840655611a3436436aead3f792c49049adc`: `lidar-state-estimation-baseline` `c37919a9…`, `lidar-box-eval` `6cafdd18…`, `lidar-ground-truth-eval` `c76a0e79…`. Every header stamps the sha |
| Replay          | `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case, every arm repeat-verified; the kirk0 corpus, index and cohort manifest; a scratch use ledger at migration 71 per arm                                |
| Box replays     | `lidar-box-eval -variants none -rows -require-settled=false -sample-points 1024` with an evidence database: kirk0 `-warmup 6 -duration 0`, every arm; placements `-warmup 40 -duration 120`, `a0` and `profile`                                                                                  |
| Arm hashes      | `a0` `ab0c4035780d`; `face-state` `80e04c6ea08e`; `accrual` `f0d816ce7ec9`; `profile` `43c2e3673c92`. Tuning `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`                                                                                                 |
| Tracks          | tracking baseline `79d9abff…` for every arm, first and repeat; source manifest `sha256:d7e33e0d…`                                                                                                                                                                                                |
| Capture         | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`                                                                                                                                                                                                                 |
| Segments        | first five minutes, cut by the scratch pcapng window filter (l5id0019's method): ashbury-downey `971e093e…`, haight-divisadero `2afd5be4…`                                                                                                                                                       |
| Labels          | the reconstructed pack `ds_67842cb1b22b70e7` (`sha256:f0ed3414…`) and physical references revision 77 (`sha256:189ecf65…`, re-pinned copy `fef9f97f…`); split `kirk0-tuning` drafted (`7194bb61…`), reference digest `a784e69d…`, `obj_788622b5` ignored                                         |
| Scoring         | `perframe -physical-reference -physical-reference-revision 77 -physical-gate-metres 3`, online estimates as declared baselines, `-a-boxes -b-solid-body`; the profile joined from the box-eval rows by frame time and solid-body position; P3 and P4 by a scratch script over the rows           |
| Discarded build | `c820d05`: two label-free placement replays of the profile arm, 17 frames admitted and 8,052 refused, no figure computed                                                                                                                                                                         |
| Host            | Claude Code cloud container, 4 cores shared with two other agents; raw output in the session scratch, not kept                                                                                                                                                                                   |
