# Refining the published box: what moved, what did not, and the box model by default

<!-- ignore-style-length -->

The box model of [geom0005](published-box-rotation-2026-10.md) holds every return and turns
within what a vehicle can do, but it trails a turn by a few degrees, can draw a slow car across
itself, costs a rectangle fit per track a frame, and draws kirk0's two trucks short. This report
is one unattended run against those four: two pre-registered experiments, a check of the box
model under the L3 retune, and the change that makes the box model the default.

- **Status:** Complete, 2026-10-17. One unattended run; experiments [geom0006](../../../data/experiments/geom/geom0006.md) and [geom0007](../../../data/experiments/geom/geom0007.md)
- **Layers:** L5 (the published box), the tuning defaults, the perf gate
- **Related:** [published box rotation](published-box-rotation-2026-10.md), [L3 region override attribution](l3-region-override-attribution-kirk-2026-10.md), [performance regression testing](performance-regression-testing.md)
- **Parameter sets:** the box model is in [tuning.defaults.json](../../../config/tuning.defaults.json); [box-model-off.json](../../../config/experiments/box-model-off.json) draws the legacy box

## Summary

- **The box model is on by default.** No refinement passed its rule, so the default is geom0005's
  box model as it was scored: the defaults and every parameter set carry its block, and the old
  `box-model.json` (equal to the defaults) is gone. On kirk0 through the perf gate it costs 1 to
  3 % of the full pipeline's wall time and 0.1 ms of mean frame time; the 95th and 99th
  percentile frame times move within the repeat spread. The Mac baselines were recaptured for
  the new fingerprint. The Pi timing geom0005 asked for before the change is still owed.
- **Turn lag is not the course's to fix.** Leading the course by its own rate cut the median
  turn lag from 3.8° to 3.5° at best (0.4 s), short of the 1° the rule asked; weighing a turning
  course less did nothing. Most of what remains is the Kalman velocity's lag behind a centred
  course, which the box inherits whichever way the course is weighed.
- **Displacement labels did not hold out.** Labelling a box from 1 m of straight displacement cut
  turning at rest on the tuning set from 3.6 % to 2.1 % of rest pairs, but on the fourth segment
  of 20 sites, read for the first time, only from 2.6 % to 2.2 %: 0.45 points against the 0.75
  the rule required. It also drew more slow vehicles across themselves there (6.5 % of slow
  frames against 3.6 %). It stays an option, off.
- **The minimum-area axis is cheaper and worse.** It halves the cost per update but moves the
  straight-road error's p90 from 6.1° to 8.7°, turning at rest from 3.6 % to 4.8 %, and kirk0's
  reviewed axis p90 from 8.4° to 12.9°.
- **Under the L3 retune the box is better still.** With the retune's foreground (CMR, the
  candidate L3 defaults), the box model meets all 44 of kirk0's reviewed keyframes (39 under the
  defaults) and its axis is 1.3° from them at the median and 3.5° at p90, with none over 10°
  (2.1°, 8.4° and four under the defaults). Turning at rest falls from 3.6 % to 0.7 %. No return
  is outside on any vehicle frame. The box model adds nothing against adopting the retune.
- **The trucks' length is not the box's memory.** Keeping the extent belief over 150 spans or
  the whole life changes none of kirk0's 44 reviewed boxes. Truck 1 is held by two tracks that
  take its cluster in alternate frames while it passes, so the 10.7 m spans of the close pass
  never enter the surviving track's belief; it leaves drawn at the 6.5 m it then shows. A 6 m
  floor for a tall body changes one keyframe. See [geom0007](#geom0007-the-length).
- **The box model draws into its neighbours.** Holding every return of its own, it holds five or
  more returns of another cluster on 6.0 % of vehicle rows (the legacy box 1.3 %), and overlaps
  another confirmed track's box on 17.6 % (10.7 %). The legacy figure is mostly two tracks on one
  vehicle; the difference is the box model's floor and held extents reaching into the next
  vehicle.

## Question

geom0005 ended with four open defects in the box it publishes: a lag of a few degrees in a turn,
slow vehicles drawn across themselves, the rectangle fit's cost, and trucks drawn short. Which of
them does a change to the box model fix, without undoing what it already does? And does the box
model belong in the defaults now?

## What was tried

Each option is a key in the `box_model` block, off at the defaults' value
([CONFIG.md](../../../config/CONFIG.md#l5-box-model)):

| Key                                              | What it does                                                                                                                                                                                  |
| ------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `course_lead_sec`                                | Moves the course forward by its own rate, measured over a 0.3 to 0.6 s baseline, times this lead: the Kalman velocity's lag in a turn                                                         |
| `course_rate_tau_sec`                            | Adds (tau·rate)² to a turning course's variance, so the shape's axis weighs more in a turn                                                                                                    |
| `displacement_label_metres`                      | Labels an unlabelled box from the track's own displacement over up to 2 s, once it is this long and nearly straight (straightness 0.8 or more over at least 0.8 s)                            |
| `axis_source: min_area`                          | The members' minimum-area rectangle's axis in place of the rectangle fit, abstaining when its aspect is under 1.2 ([min_area_rect.go](../../../internal/lidar/l4perception/min_area_rect.go)) |
| `extent_window_spans`                            | How many admitted spans the length and width are the third largest of: 50 as before, or the whole life at 0                                                                                   |
| `tall_height_metres`, `tall_length_floor_metres` | A labelled box of a vehicle's width whose corroborated vertical extent reaches the height is drawn at least the floor long                                                                    |

Observing the yaw rate from the course's rate was also built and removed: it moved none of the
lag, because the lag is in the course itself, not in the filter's rate.

## Method

**The scorer.** `lidar-box-eval` as in geom0005, which scores every variant beside the box model
on the same clusters and tracked states in one replay, and forces cluster members on whatever the
tuning says. It gains these measures, all on confirmed vehicle tracks:

| Measure             | Definition                                                                                                                                                                                                      |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Turn lag            | Against a centred course (the tracked position's direction over ±3 frames, at least 1 m apart), on frames at 3 m/s or more turning 10°/s or faster: the drawn axis's error in the turn's sense, positive behind |
| Straight-road error | The drawn axis against the centred course on frames at 3 m/s or more turning slower than 3°/s                                                                                                                   |
| Slow label error    | Frames at 0.5 to 3 m/s whose centred course over ±10 frames is at least 1 m long, with the drawn axis more than 60° from it: a slow vehicle drawn across itself                                                 |
| Foreign returns     | Share of rows whose box holds five or more returns of another cluster, each inside by 10 cm                                                                                                                     |
| Neighbour overlap   | Share of rows whose box shares more than a tenth of the smaller box's area with another confirmed track's box                                                                                                   |
| Overlong            | Share of rows whose box is longer than 13 m                                                                                                                                                                     |

The turn and straight figures are pooled from every frame's sample across sites, not medians of
site medians. Turning at rest, visible turn-rate violations and visible jumps are geom0005's.

**References.** kirk0's 44 reviewed keyframes with an observed yaw (truck 1 at 10.6 m, car 8 at
4.3 m and truck 2 at 9.6 m), from the frozen bundle at revision 76, matched as in geom0005. Besides
the axis, each match now gives the distance from the drawn centre to the reviewed body centre (a
face anchor moved half the reviewed length along the reviewed yaw) and the ratio of the drawn
length to the reviewed one.

**Captures.** Tuning: the first five-minute segment of 23 corpus sites, with kirk0 and kirk1's
first 270 s; 70 s of warm-up on the corpus and 20 s on kirk. Held out for geom0006: the fourth
segment of every site that has one, read for the first time (20 scored; Embarcadero at Bay and Van
Ness at Sacramento are shorter than the warm-up). Held out for geom0007: the fifth segment of the
11 sites that have one. Embarcadero at Folsom, the declared held-out site, is in no figure.

**Rules.** Each experiment's rule was committed before its replays
([geom0006](../../../data/experiments/geom/geom0006.md), [geom0007](../../../data/experiments/geom/geom0007.md)).
In short: a candidate must improve its own target by a stated amount on the tuning set, and leave
every other measure within the box model's × 1.1 + 0.1 points (shares) or 0.5° (straight error),
no return outside and kirk0's reviewed axis p90 within 1°; then hold on unread segments.

## Findings

### geom0006 round 1: the course, the label and the axis

Pooled over vehicle frames of the 23 first segments and kirk (4,977 turning frames, 14,365 rows):

| Variant           | \|lag\| p50 | \|lag\| p90 | Straight p50 | Straight p90 | Turning at rest | Slow label errors | kirk0 axis p90 | µs per update |
| ----------------- | ----------: | ----------: | -----------: | -----------: | --------------: | ----------------: | -------------: | ------------: |
| legacy box        |       14.9° |       62.2° |        11.7° |        56.3° |          31.5 % |           13.02 % |          79.3° |             – |
| `km` (box model)  |        3.8° |       13.4° |         1.7° |         6.1° |           3.6 % |            9.98 % |           8.4° |            37 |
| lead 0.2 s        |        3.6° |       13.2° |         1.6° |         5.8° |           3.6 % |            9.77 % |           8.4° |               |
| lead 0.4 s        |        3.5° |       13.1° |         1.5° |         5.9° |           3.6 % |            9.77 % |           7.5° |            26 |
| lead 0.6 s        |        3.5° |       13.2° |         1.7° |         6.3° |           3.6 % |           10.05 % |           8.1° |               |
| tau 0.4 s         |        3.8° |       13.7° |         1.8° |         6.4° |           3.6 % |            9.84 % |           8.4° |            27 |
| tau 0.8 s         |        3.8° |       14.0° |         1.9° |         6.9° |           3.6 % |            9.84 % |           8.4° |               |
| displacement 1 m  |        3.8° |       13.4° |         1.7° |         6.1° |           2.1 % |           10.05 % |           8.4° |            26 |
| displacement 2 m  |        3.8° |       13.4° |         1.7° |         6.1° |           2.1 % |           10.05 % |           8.4° |               |
| minimum-area axis |        5.0° |       17.0° |         2.6° |         8.7° |           4.8 % |           10.90 % |          12.9° |            19 |

No candidate passed. The best lead took 0.3° off the median lag where the rule asked for 1°; tau
changed nothing but the straight error, which it made worse; displacement labels left slow label
errors where they were (10.05 % against 9.98 %). The minimum-area axis met its cost target (19 µs
against 37 µs, about half) and failed four non-inferiority clauses. The per-update times are
measured beside two parallel replays and are comparable only within a pass.

The lag that remains is the course's own: against a centred course, the Kalman velocity's
direction trails a turn by about as much as the box does, and the box can only be steered by the
course it is given or turned by the shape it sees, which the yaw-rate bound and the axis noise
smooth. A better course in a turn would need a better velocity, which is the tracker's.

### geom0006 round 2: turning at rest

Displacement labels cut turning at rest (a box turning more than 1° between frames below
0.5 m/s), a measure they were not chosen for. A second rule, written before its replays, made
that the target: a fifth and 0.5 points off on the tuning set, then at least half as much on the
fourth segments.

| Variant                | Tuning: at rest | Tuning: kirk0 axis p90 | Verdict | Fourth segment: at rest | Slow label errors |
| ---------------------- | --------------: | ---------------------: | ------- | ----------------------: | ----------------: |
| `km`                   |          3.64 % |                   8.4° |         |                  2.63 % |            3.64 % |
| displacement 1 m       |          2.14 % |                   8.4° | pass    |                  2.18 % |            6.45 % |
| unlabelled floor 20°/s |          4.02 % |                  10.0° | fail    |                         |                   |
| both                   |          2.51 % |                  10.0° | fail    |                         |                   |

On 20 fourth segments (277 vehicle tracks, 10,617 rows) the label took turning at rest down by
0.45 points, against the 0.75 required, and nearly doubled the slow-label errors. It failed; it
stays an option at 0, and nothing from geom0006 enters the defaults.

The box model itself repeats on the fourth segments: no return outside (the legacy box 94.7 %),
visible turn-rate violations 0.9 % of pairs (3.8 %), visible jumps 0.25 % (1.14 %), the
straight-road error 1.4° at the median and 6.6° at p90 (11.5° and 63.6°), the median turn lag 4.1°
(15.2°).

### The box model under the L3 retune

The L3 retune (CMR, [l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json))
is the candidate L3 default: on kirk0 it raises recall from 0.59 to 0.89 and pedestrian recall
from 0.28 to 0.76, and its adoption waits on a held-out labelled window and the Pi
([l3bg0009](../../../data/experiments/l3bg/l3bg0009.md)). It changes what the box model is given:
more and longer vehicle tracks, and denser clusters. The 23 first segments and kirk under CMR,
against the same under the defaults:

| Tuning   | Variant    | Vehicle tracks |   Rows | Outside | Visible rate violations | At rest |  Jumps | Straight p50 / p90 | \|lag\| p50 / p90 | Slow label errors |
| -------- | ---------- | -------------: | -----: | ------: | ----------------------: | ------: | -----: | -----------------: | ----------------: | ----------------: |
| defaults | legacy box |            447 | 14,365 |  96.5 % |                   4.8 % |  31.5 % | 1.30 % |      11.7° / 56.3° |     14.9° / 62.2° |           13.02 % |
| defaults | box model  |            447 | 14,365 |   0.0 % |                   0.8 % |   3.6 % | 0.32 % |        1.7° / 6.1° |      3.8° / 13.4° |            9.98 % |
| CMR      | legacy box |            915 | 50,964 |  96.7 % |                   4.8 % |  16.7 % | 0.94 % |      14.0° / 54.1° |     14.0° / 52.8° |           14.54 % |
| CMR      | box model  |            915 | 50,964 |   0.0 % |                   0.5 % |   0.7 % | 0.19 % |        1.7° / 5.9° |      3.6° / 12.0° |            6.31 % |

CMR gives the box model twice the vehicle tracks and three and a half times the rows, and on them
it turns at rest a fifth as often and draws a slow vehicle across itself two thirds as often.

On kirk0's reviewed poses:

| Tuning   | Variant    | Matched | Axis p50 | Axis p90 | Over 10° | Over 45° | Centre p50 | Centre p90 | Length / reviewed |
| -------- | ---------- | ------: | -------: | -------: | -------: | -------: | ---------: | ---------: | ----------------: |
| defaults | legacy box |      42 |     8.4° |    79.3° |       19 |       15 |     3.26 m |     4.71 m |              0.61 |
| defaults | box model  |      39 |     2.1° |     8.4° |        4 |        2 |     2.14 m |     4.16 m |              0.62 |
| CMR      | legacy box |      44 |     9.2° |    36.9° |       18 |        2 |     3.23 m |     4.40 m |              0.64 |
| CMR      | box model  |      44 |     1.3° |     3.5° |        0 |        0 |     2.04 m |     3.45 m |              0.62 |

Under CMR every keyframe is matched, and the box model's axis is within 10° of every reviewed
pose. Car 8 is drawn 4.2 to 4.4 m long against its 4.26 m. Both trucks are still drawn short while
they approach and as they leave; the next section is why.

### geom0007: the length

On kirk0 under the defaults, truck 1 is drawn 3.8 m long (the vehicle floor) while it approaches
end-on, 10.3 to 10.7 m while it passes, and 6.4 to 6.6 m as it leaves; truck 2 similarly. The
centre is therefore 2.1 m from the reviewed body centre at the median, most of it along the axis.
geom0007 tested whether the extent belief's 50-span window was forgetting the full view, and
whether a length floor for a tall body would draw the approach nearer.

**The rule's target was not met by any candidate.** On kirk0's 44 reviewed keyframes every
variant matched the same 39, with the same medians: centre 2.14 m from the reviewed body centre
(p90 4.16 m), length 0.62 of the reviewed, axis 2.1° (p90 8.4°). The tall floor changed one
keyframe's length, from 5.6 to 6.0 m. All six variants met the non-inferiority clauses on the 23
first segments and kirk (447 vehicle tracks, 14,365 rows):

| Variant                | Foreign returns | Neighbour overlap | Over 13 m | Shrinking | At rest | Visible rate violations | Straight p90 |
| ---------------------- | --------------: | ----------------: | --------: | --------: | ------: | ----------------------: | -----------: |
| `km` (50 spans)        |          5.95 % |           17.58 % |    0.00 % |    9.10 % |   3.6 % |                   0.8 % |         6.1° |
| 150 spans              |          5.95 % |           17.76 % |    0.00 % |    8.08 % |   3.6 % |                   0.8 % |         6.1° |
| whole life             |          5.95 % |           17.76 % |    0.00 % |    6.97 % |   3.6 % |                   0.8 % |         6.1° |
| tall floor 6 m         |          6.33 % |           17.83 % |    0.00 % |    8.07 % |   3.6 % |                   0.8 % |         6.1° |
| 150 spans, tall floor  |          6.33 % |           18.00 % |    0.00 % |    7.01 % |   3.6 % |                   0.8 % |         6.1° |
| whole life, tall floor |          6.33 % |           18.00 % |    0.00 % |    6.74 % |   3.6 % |                   0.8 % |         6.1° |

"Shrinking" is the share of rows drawn under three quarters of the track's longest box so far, a
diagnostic outside the rule. A longer memory does what it says on the corpus (9.1 % of rows
shrinking to 7.0 %) without holding a merge's size, but on the reviewed trucks it has nothing to
remember. No candidate passed, so the fifth segments were not read and none of the options enters
the defaults.

**Why the memory has nothing to remember.** A diagnostic replay of kirk0 that writes the
tracker's merge flag and the cluster's vertical extent on every row shows what happens to truck 1:

- while it approaches, the tracker holds more than one track on it (three over the reviewed
  span, two of them on low clusters under 1 m tall), and the reviewed keyframes match the one
  nearest them, drawn at the 3.8 m vehicle floor;
- while it passes the sensor (11.9 to 14.9 s into the reviewed span) its returns form one
  cluster of up to 8,000 points, 10.3 to 10.7 m long, which the two tracks take in alternate
  frames. The tracker flags every one of those frames as a probable merge
  (`TrackedObject.MergeCandidate`), and the box model refuses a merge-flagged frame's spans as
  extent evidence, by design, so neither track's belief learns the length;
- as it leaves, the surviving track sees 6 to 6.6 m of it and is drawn that long.

The tall floor did not fire on the approach because the cluster's vertical extent at range is
1.3 m at the median and 2.6 m at p90, against the truck's 3.1 m: the sensor's beams are too
sparse there to see the whole height, so the corroborated height rarely reaches 2.6 m before the
truck is close enough to be seen whole.

The length the reviewers measured is in the data, on frames the tracker itself flags as two
tracks sharing a cluster. Admitting those frames as evidence would also admit two cars that merge
into one cluster; the fix is upstream, in the tracker holding one track per vehicle.

**What the box model costs its neighbours.** The same 23 first segments and kirk0 under the
defaults with the box model off give the legacy box the same measures (437 vehicle tracks, 14,127
rows; the tracks are identical either way):

| Box        | Outside | Foreign returns | Neighbour overlap | Wider than 2.8 m | Median length | Median width |
| ---------- | ------: | --------------: | ----------------: | ---------------: | ------------: | -----------: |
| legacy box |  96.5 % |          1.26 % |           10.74 % |            8.4 % |        3.92 m |       1.72 m |
| box model  |   0.0 % |          6.03 % |           17.56 % |            9.7 % |        4.22 m |       1.83 m |

Two boxes of distinct vehicles cannot overlap, so every overlap is a defect: two tracks on one
vehicle, which the legacy box shows on a tenth of vehicle rows, or a box drawn into its
neighbour. The box model holds every one of its own returns, but its larger boxes (the 3.8 m
floor and the unseen remainder drawn away from the sensor, the corroborated extents held after
the view shrinks) hold another cluster's returns on 6 % of vehicle rows against 1.3 %, and
overlap another track's box on 17.6 % against 10.7 %. geom0005 did not measure this.

## The box model in the defaults

geom0005 asked for the Pi timing before the box model became the default. The direction for this
run was to put promising results into the defaults, and the box model is the promising result of
the last two runs: it holds every return, its axis is within a few degrees of the reviewed poses,
and the tracks are byte-identical with it on. So the defaults now carry the block, with every
geom0006 and geom0007 option at its off value:

- every parameter set in `config/experiments/` carries the same block, so each still differs from
  the defaults only in its own keys; `box-model.json` equalled the defaults and was removed, and
  [box-model-off.json](../../../config/experiments/box-model-off.json) keeps the legacy box;
- the tracker's own default (`DefaultTrackerConfig`) reads the block from the embedded defaults;
- `lidar-bench` keeps cluster members whenever the tuning's box model reads them, so the perf
  gate measures the box model's real cost;
- the two Mac perf baselines were recaptured for the moved fingerprint from a shared clone at the
  final defaults: full 7,756 ms wall (spread 0.6 %), p95 frame 24.5 ms; l3-only 4,892 ms (2.9 %).
  The work is the same (25 tracks confirmed at once, 77 distinct). A first capture at the flip,
  before the geom0006 and geom0007 keys joined the block, had another fingerprint and was replaced.

Paired on the same clone and binary at the flip, five repeats each:

| Tuning             | Wall (median of 5) | Spread | Mean frame | p95 frame | p99 frame |
| ------------------ | -----------------: | -----: | ---------: | --------: | --------: |
| box model on       |           7,679 ms |  1.5 % |    6.18 ms |  24.42 ms |  50.05 ms |
| box model off      |           7,603 ms |  1.2 % |    6.08 ms |  23.51 ms |  48.96 ms |
| on (first capture) |           7,800 ms |  2.5 % |    6.20 ms |  23.59 ms |  49.43 ms |
| off                |           7,596 ms |  0.8 % |    6.07 ms |  23.59 ms |  49.93 ms |

The box model costs 1 to 3 % of the wall time and about 0.1 ms of mean frame time on kirk0; the
tails move within the spread of repeats. On the Pi the same fraction would be a few milliseconds
a frame at most, inside the 98 ms budget, but that is an extrapolation, not a measurement.

## Limitations

- **Three reviewed objects.** kirk0's references are two trucks and a car; every length and
  centre figure rests on them, and two of the three are the hardest case the box has.
- **No turn ground truth.** The turn lag is measured against a centred course from the tracked
  position, which is itself the tracker's estimate; a lag in the track would be invisible to it.
- **Per-update times are noisy.** The scorer's µs per update ran beside a second replay; only the
  perf-gate pairs are clean, and only on the Mac.
- **The Pi is not measured.** The box model is the default without the timing geom0005 asked for.
- **Corpus measures are proxies.** Foreign returns, neighbour overlap and overlong boxes stand in
  for merge harm on the corpus, which has no reviewed boxes.

## Provenance

| Item                        | Value                                                                                                                                                               |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Round 1                     | scorer `adbe94b54`, 23 first segments and kirk, defaults then without the block (variants carry their own box configuration)                                        |
| Round 2 and fourth segments | scorer `a8da1a8d6`; fourth segments of 20 sites, 70 s warm-up                                                                                                       |
| Round 3 (CMR)               | scorer `a8da1a8d6`; `l3-retune-cm2-mp8-floor.json` as at `6e0f5ba32` less the box block, SHA-256 `58ec690d5034664748d6ec3701202074c28c4b5d7b437e532095bd17dc924148` |
| geom0007                    | scorer `6533b390a`; defaults with the box model                                                                                                                     |
| Defaults at the change      | `config/tuning.defaults.json` at `6e0f5ba32`, SHA-256 `5b6a7ac0a933ad33e918bfc4c537d0e528168c8bcf8bf342ad1cf5e28f226923`                                            |
| Perf baselines              | `make perf-baseline-all` from a shared clone at `8f8ff9c9e`, fingerprints full `4c10f4e77702`, l3-only `9884a9eae446`; the on/off pairs at `6e0f5ba32`              |
| References                  | physical-reference bundle revision 76 (pack `ds_95be6fb22498b44a`), 44 keyframes with an observed yaw                                                               |
| Rules                       | geom0006 round 1 and round 2 in its spec; geom0007 committed at `168f2d517` before its replays                                                                      |
| Raw outputs                 | summaries, per-frame rows and logs under the run's scratch directory on the development Mac, archived on the results share; not in Git                              |

## Recommendations

1. **Time the box model on the Pi.** It is in the defaults now; the `pi` step of geom0005 is
   owed, and `axis_source: pca` is the fallback if the fit is too dear there.
2. **Adopt the L3 retune on its own evidence.** The box model is better under it, not worse;
   nothing here blocks l3bg0009.
3. **Stop the box at its neighbours.** The unseen part of a box, drawn from the length floor or
   a held extent, should end where another cluster's returns begin.
4. **Fix duplicate tracks on one vehicle before the box's length.** Truck 1 is held by two tracks
   while it passes; the box can only be as whole as the track it belongs to.
5. **Leave the course options off.** The lag is the velocity's; work on it belongs with the
   tracker's motion model, not the box.
6. **Republish the scene assets** from replays under the new defaults so velocity.report draws
   the box model.
