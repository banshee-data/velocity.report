# Extent bound accrual on kirk0: why truck 2 got shorter at #989, and what accruing the bounds restores

<!-- ignore-style-length -->

The November benchmark bisected a 1.64 m loss in truck 2's solid-body length on kirk0 to #989.
This report traces that loss to its cause and reads a default-off fix,
`solid_body_extent_accrual`, against a rule committed before any scored replay. It answers
[geom0018](../../../data/experiments/geom/geom0018.md).

- **Status:** Measured on kirk0, 2026-11-09. The rule fails in both reads. Nothing adopted, no
  default changed
- **Spec:** [geom0018](../../../data/experiments/geom/geom0018.md); rule at `016ebacc`, restated
  for the second read at `f6ce2843`, each before its replays
- **Layers:** L5, the shadow solid body's extent state, and the published box through
  `box_extent_solid_body`; scored against L10 physical references
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`. Every option is a replay experiment, not a tuning key
- **Related:** [state plan §9.2.4](../../plans/lidar-state-estimation-plan.md#924-extents-in-the-state-and-censored-extent-evidence-delivered-default-off),
  [geom0015's bias baseline](d2-frozen-split-bias-baseline-2026-11.md),
  [face-state maths review §3.1](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md#31-the-half-extent-is-a-state-not-a-constant);
  the benchmark is l5id0019 on PR #994, not yet on main

## Answer

- **The 6.24 m that #989 lost was an artefact, not evidence.** Before #989, truck 2's bounds came
  at under 2 m/s, along a face axis 40 to 50 degrees from its course. They were at most 3.95 m
  long and 3.3 m across. The moment-matched truncation raised the half-length from 2.20 to
  3.14 m in five frames, which is a 6.3 m length from spans no longer than 3.95 m. That is the
  overshoot #989 removed. On the six reviewed poses no bound is admitted at main. The truck is
  then a merge candidate or its face axis disagrees with its course, so the length stays near the
  class prior.
- **The fix does not meet the brief's target.** In the second read the option takes truck 2's
  median length over its six poses from 4.60 to 4.83 m. The target was 6.24 m, so R1 fails. The
  last two poses read 8.01 and 8.56 m against 9.63 m, where main reads 4.50 and 4.38 m. On those
  poses the centre error falls from 2.70 and 2.66 m to 1.04 and 0.53 m. No honest bound longer
  than the state exists before the cluster grows at 75.0 s.
- **Widths improve where they were raised off an unsettled heading.** Truck 2's width goes from
  1.90 to 2.88 m against 2.84 m, and truck 1's from 3.82 to 3.13 m against 2.91 m. Car 8 does not
  move: 2.45 to 2.44 m against 1.73 m. The solid body's median centre error over the 22 poses
  falls from 1.028 to 0.771 m.
- **Two other gates fail narrowly.** Car 8's length holds (R2). Rows at the half-extent floor
  rise from 14 to 16 (R3). The published box under `box_extent_solid_body` is not non-inferior to
  `a0`'s (R4): median 0.280 m against 0.190 m. Main's `box-extent` fails the same thresholds at
  0.281 m.
- **The first version failed worse, for a reason worth keeping.** It refused every bound off an
  axis not believed within 5 degrees. With no width bound, the faces carried widths to the floor
  on 106 rows against 9. With no length bound, truck 1 and car 8 read shorter. Refusing a bound
  is not neutral. The bound is what holds the half-extent against the faces.

## Question and rule

The rule is in the [spec](../../../data/experiments/geom/geom0018.md). R0 is binding: `face-state`
reproduces l5id0019's solid-body rows, `a0` its box rows, and the option leaves the tracks
byte-identical. R1: truck 2's solid-body length is at least 6.24 m, and its error is below #989's
5.03 m. R2: car 8's length error is no worse than `face-state`'s plus 0.005 m. R3: no more rows
at the floor than `face-state`. R4: `box-accrual`'s box is non-inferior to `a0`'s by l5id0019's R3
thresholds. Those thresholds are 0.05 m overall, 0.25 m per vehicle and 0.5 m on truck length,
with no fewer matches. The fix holds when R1 to R4 all hold.

## Root cause

Two label-free probe replays of the `face-state` arm on kirk0 came before the rule, one at this
branch's base and one at #989's parent (`50c7790b`). Each logged truck 2's extent state frame by
frame. Truck 2 is track `seq-000049`.

| Time on kirk0                     | Speed (m/s) | Face axis against course | Bounds admitted (L, W, m) | hL at #989's parent |   hL at main |
| --------------------------------- | ----------: | ------------------------ | ------------------------- | ------------------: | -----------: |
| 60.1 to 68.6 s                    |   4.4 → 1.9 | 47 to 60°, variance 1.3  | none (course contradicts) |         2.25 → 2.20 |  2.25 → 2.20 |
| 68.9 to 72.1 s                    |  1.5 to 2.1 | 40 to 50°, variance 1.2  | up to 3.95, up to 3.3     |         2.20 → 3.14 |  2.20 → 2.06 |
| 72.2 to 75.0 s (poses 491 to 502) |   2.3 → 6.2 | 17 to 37°                | none (course contradicts) |                 3.1 | 2.05 to 2.42 |
| 75.1 to 77.3 s (poses 504, 509)   |   6.4 → 7.3 | 5 to 15°                 | none (merge candidate)    |                 3.1 |   2.3 → 2.14 |
| 77.5 s on                         |         7.3 | under 10°                | 8.5 to 9.0, 2.5           |                4.66 |         4.48 |

The bounds of 68.9 to 72.1 s are taken with the predicted speed under 2 m/s. `spanAxisBelieved`
makes no course check there, so it admits any axis. Against a half-length of 2.20 m with a sigma
near 1.17 m, the first bound's moment-matched truncation at `l = 1.78 m` moves the mean by
`σλ(α)` with `α = −0.36`, which is 0.68 m: the 2.20 to 2.88 m the trace shows. Repeating that
every frame, as #989 found, gives a 6.3 m length from spans no longer than 3.95 m. The same
mechanism took the width from 3.0 to 5.0 m. After #989 the projection leaves the half-length at
2.20 m, because the bounds sit below it. The faces then hold it near the prior through the
reviewed poses.

Along the course the spans are 3.4 to 3.6 m until 75.0 s, and 8.2 to 8.8 m from 75.1 s. The
longer spans arrive with the merge flag up. That flag compares the cluster's area with the
track's running box (`tracking.go`), so a truck whose view grows from its end to its side raises
it. The course's sigma reaches 5 degrees only at 75.7 s, after pose 509 at 75.6 s.

## What the option does

`SolidBodyOptions.ExtentBoundAccrual` runs under `ExtentState` with `CensoredExtent` or
`SpanBoundsBelief`. It changes the bounds and nothing else
([`solid_body_extent_accrual.go`](../../../internal/lidar/l5tracks/solid_body_extent_accrual.go)):

- **A record per half-extent.** It keeps the three largest admitted bounds. The supported bound
  is the third: the largest value three frames reached. That is the extent belief's corroborated
  maximum (Section 9.2.3) without its bins. Each frame holds the half-extent at or above it by
  #989's projection, before the faces and again after them. P is never narrowed.
- **A window as wide as the axis is uncertain** (second version). The bound is the window-minimum
  span about the face axis or, where its variance is smaller, the course. The half-window is twice
  the standard deviation, between 10 degrees and a quarter-turn. At 10 degrees it is main's
  computation. At a quarter-turn both bounds are the returns' least width, which no axis error
  can carry past the body's width. The first version refused the bound instead.
- **A merge candidate's length goes to the record only.** It counts when the cluster is no wider
  than one body, using `growingNotMerging`'s lateral test. It is never the frame's own bound and
  never a width.

## kirk0: the solid body on geom0015's 22 poses

Medians over each vehicle's reviewed poses (length, absolute length error, width), and the median
centre error over all 22. References: truck 1 10.63 × 2.91 m, car 8 4.26 × 1.73 m, truck 2
9.63 × 2.84 m.

| Arm and build                         | Median centre | Truck 1: L, \|err\|, W | Car 8: L, \|err\|, W | Truck 2: L, \|err\|, W | Rows at the floor (L, W) |
| ------------------------------------- | ------------: | ---------------------- | -------------------- | ---------------------- | -----------------------: |
| `face-state` (main), `f6ce2843`       |       1.028 m | 9.73, 0.90, 3.82       | 3.90, 0.36, 2.45     | 4.60, 5.03, 1.90       |                14 (5, 9) |
| `accrual`, first version, `016ebacc`  |       1.496 m | 8.01, 2.62, 2.66       | 3.43, 0.83, 2.26     | 4.58, 5.05, 1.24       |             113 (7, 106) |
| `accrual`, second version, `f6ce2843` |       0.771 m | 9.62, 1.01, 3.13       | 3.92, 0.34, 2.44     | 4.83, 4.80, 2.88       |               16 (2, 14) |

`face-state` at `016ebacc` gave the same rows as at `f6ce2843`, which is l5id0019's figures.
`box-accrual` reads the same solid body as `accrual` to the centimetre in both versions.

Truck 2's six poses, length (centre error), in metres:

| Pose | Main        | First version | Second version |
| ---: | ----------- | ------------- | -------------- |
|  491 | 4.84 (2.66) | 4.85          | 4.85 (2.41)    |
|  495 | 4.80 (2.67) | 4.50          | 4.80 (2.49)    |
|  500 | 4.64 (2.63) | 4.69          | 4.64 (2.50)    |
|  502 | 4.56 (2.65) | 4.61          | 4.57 (2.52)    |
|  504 | 4.50 (2.70) | 4.55          | 8.01 (1.04)    |
|  509 | 4.38 (2.66) | 4.42          | 8.56 (0.53)    |

After the reviewed poses, from 76 s on, truck 2 reads 8.97 m under the second version and
9.01 m under main. Both get there from 77.5 s, when the first bound along the course is admitted.

**The floor rows** (second version) belong to different tracks than main's. Main's 14 are a car's
width on one row, one unknown-class track's length on three, another's width on eight, and a
pedestrian's length on two. Under the option only the pedestrian's two remain. They are joined by
14 width rows on one pedestrian 83 m out, `seq 56`, between 67.0 and 74.6 s, where the bound's
noise is 0.31 m. Inferred: the least width of that sparse cluster over its window falls below the
noise, so the frame gives no width bound and the faces carry the width down, where main's
across-span off the wrong axis cleared it.

## kirk0: the published box

`box_extent_solid_body` draws the box model's length and width from the extent state. Medians
over geom0015's 22 poses (gate 3 m):

| Arm                   | Matched |  Median | Truck 1       | Car 8        | Truck 2      | Box IoU | R4 against `a0`          |
| --------------------- | ------: | ------: | ------------- | ------------ | ------------ | ------: | ------------------------ |
| `a0`                  | 22 / 22 | 0.190 m | 0.19; L 10.60 | 0.04; L 4.40 | 1.49; L 6.59 |   0.887 |                          |
| `box-extent` (main)   | 22 / 22 | 0.281 m | 0.19; L 10.62 | 0.29; L 4.40 | 1.49; L 6.59 |   0.802 | fails: median +0.091     |
| `box-accrual`, first  | 22 / 22 | 0.227 m | 0.19; L 10.60 | 0.23; L 4.40 | 1.49; L 6.59 |   0.810 | holds                    |
| `box-accrual`, second | 22 / 22 | 0.280 m | 0.19; L 10.60 | 0.32; L 4.40 | 1.49; L 6.59 |   0.810 | fails: +0.090, car +0.28 |

No arm moves a published length. The box's own length already exceeds the extent state's on every
pose. What moves is car 8's width, and with it the centre across the body. The first version
narrowed car 8's extent-state width to 2.26 m, and its box with it. The second leaves it at
2.44 m, as main does.

**Tracks.** `accrual`'s tracking baseline equals `a0`'s and `face-state`'s byte for byte
(`79d9abff…`), in both versions and both passes. `box-accrual` (`298081ae…`) and `box-extent`
(`186d30d4…`) change the tracks through the L3 track guard. Each repeats its own baseline.

## Against the rule

| Gate                      | First version (`016ebacc`) | Second version (`f6ce2843`)                |
| ------------------------- | -------------------------- | ------------------------------------------ |
| R0, binding               | holds                      | holds                                      |
| R1, truck 2 ≥ 6.24 m      | fails: 4.58 m              | fails: 4.83 m (error 4.80 m, below 5.03 m) |
| R2, car 8 ≤ 0.365 m error | fails: 0.83 m              | holds: 0.34 m                              |
| R3, floor rows ≤ 14       | fails: 113                 | fails: 16                                  |
| R4, box non-inferior      | holds                      | fails: median +0.090 m, car 8 +0.28 m      |
| Verdict                   | not held                   | not held                                   |

## Interpretation

Each reading is marked measured or inferred.

**The brief's target was set by the overshoot.** Measured: the 6.24 m comes from a truncation of
bounds no longer than 3.95 m, and no bound on or before the reviewed poses exceeds 4 m along any
axis the course allows. Inferred: no estimator that honours the bounds as lower bounds can read
truck 2 at 6.24 m on poses 491 to 502 from this evidence. A test that it does would reward the
defect #989 fixed. The physical shortfall there is length the sensor has not seen. That is the
maths review's §3.1 case, which needs the far end, and this capture shows the far end only from
75 s.

**The record works where the evidence exists.** Measured: on poses 504 and 509 truck 2 reads 8.0
and 8.6 m, with its centre 1.0 and 0.5 m off instead of 2.7 m. Inferred: only the record can raise
it there. Those frames are merge candidates, which take no frame bound, and their spans along the
course are 8.2 to 8.8 m. The probe did not run on this version, so the frame-by-frame path is not
shown. The lateral test is the only merge guard on those spans, and kirk0 has no labelled merge to
show whether it is enough.

**Refusing a bound is not the same as not taking it.** Measured: the first version's refusals
sent 106 width rows to the floor and shortened truck 1 by 1.7 m. Inferred: under `ExtentState`
the frame's bound is the only thing a face's pull on `h` meets. The face has a null direction
between `n·c` and `h`, so without a bound the prior's variance lets it carry `h` as far as the
plane moved. A conservative bound is better than none.

**Car 8's width is not the heading's fault.** Measured: car 8's width is 2.45 m under main, 2.44 m
under the second version and 2.26 m under the first. The axis-uncertain window does not narrow
it. Inferred: car 8's width is raised by bounds along a believed axis, or by its faces, not by
bounds off an unsettled heading. The first version narrowed it only by taking no width bound at
all. The published box's car 8 regression under `box_extent_solid_body` therefore has a cause
this option does not reach. That is the benchmark's fix 5, which takes only evidenced extent into
the box.

## What this does not establish

- **Anything beyond kirk0.** One tuning capture, three reviewed vehicles, and six truck 2 poses
  within two seconds.
- **A merge reference.** The merge-candidate rule is untested against a real merge.
- **Revisability.** The record keeps no frame identities, so a frame later found to be a merge
  cannot be removed (Section 9.2.3). This is a named debt of the option.
- **Cost.** The replays did not run `-campaign-metrics`. The widened window searches up to 91
  angles on 256 returns per axis where main searches 21, so Tracker.Update is not measured here.
- **The parameter hash tells the versions apart.** It does not: both versions of
  `solid_body_extent_accrual` hash to `f0d816ce7ec9`, because the hash folds the experiment's
  name and not its code. The build sha is what distinguishes the two reads.

## Recommendations

1. Keep `solid_body_extent_accrual` default-off, in its second version. It is the better extent
   state on truck 2 and truck 1's widths and on the solid body's centre (median 0.77 m against
   1.03 m). It does not meet the brief's target, and the target cannot be met from the evidence.
2. Restate the truck-length check for the extent state without the overshoot's figure. The
   reference to beat on poses 491 to 502 is the span the sensor saw, not 6.24 m. Score the far-end
   case where the far end is seen, on poses after 75 s, which need reviewed physical references.
3. Before the option can be named for a held-out read, add a range-aware width floor for small
   classes, so a far pedestrian's width is not left to the faces. That is the R3 regression.
4. Take car 8's width through the benchmark's fix 5 (evidenced extent only into the box). This
   option does not reach it.
5. Give the replay manifest a code identity for experiment options, so two versions of one name
   cannot share a parameter hash.

## Provenance

| Item          | Value                                                                                                                                                                                                                                                                               |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base          | main `704b8e46`                                                                                                                                                                                                                                                                     |
| Commits       | option `6d81ea8f`; rule `016ebacc`; second version `181459be`; restated rule `f6ce2843`                                                                                                                                                                                             |
| Tools         | Makefile LDFLAGS and the `pcap` tag, `0.5.1-pre52`. First read at `016ebacc`: `lidar-state-estimation-baseline` `74b97ce0…`, `lidar-ground-truth-eval` `044f9029…`. Second read at `f6ce2843`: `fe395c23…`, `e5a2ee40…`. Every header stamps the build sha                          |
| Replay        | `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024`, evidence per case, every arm repeat-verified                                                                                                                                |
| Arm hashes    | `a0` `ab0c4035780d`; `face-state` `80e04c6ea08e`; `accrual` `f0d816ce7ec9`; `box-extent` `7c8a88e85dc1`; `box-accrual` `1f17a5f00037`                                                                                                                                               |
| Capture       | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`                                                                                                                                                                                                    |
| Labels        | the reconstructed pack `ds_67842cb1b22b70e7` (`sha256:f0ed3414…`) and physical references revision 77 (`sha256:189ecf65…`, re-pinned to the reconstructed digest as l5id0019 did); split `kirk0-tuning` drafted (`7194bb61…`), reference digest `a784e69d…`, `obj_788622b5` ignored |
| Scoring       | `perframe -physical-reference`, gate 3 m, online estimates as declared baselines; `-a-boxes -b-boxes` and `-a-boxes -b-solid-body`; floor rows from `lidar_track_solid_bodies` at 0.2001 m                                                                                          |
| Probe replays | scratch instrumentation of `stepNearEdge`, not committed, at the base and at `50c7790b`, label-free                                                                                                                                                                                 |
