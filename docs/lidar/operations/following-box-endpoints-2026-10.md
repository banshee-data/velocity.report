# The published box as the following metric's endpoint source

<!-- ignore-style-length -->

On kirk0 the provisional following field run fitted 7 of 60 follower paths and found no valid
time, because its only physical endpoint source, the near-edge solid body, places a body centre
only after a near-edge fix. The published box is now the default and is recorded on every
observation. This report reads the box as the body, scores its gaps against kirk0's reference
gaps, and runs the field run with it on the first segment of 23 corpus sites. It answers
[l8fol0003](../../../data/experiments/l8fol/l8fol0003.md).

- **Status:** Rule passed, 2026-10-20. One unattended run (exp-20261020); no default changed, the
  field run stays provisional
- **Layers:** L5 (the published box), read by L8 following and scored against L10 physical
  references
- **Related:** [headway report oracle](headway-report-oracle.md#provisional-field-run),
  [physical reference pilot](physical-reference-pilot-kirk0-2026-10.md),
  [published box refinement](published-box-refinement-2026-10.md),
  centre against extent (#807),
  class evidence (#804),
  following opportunity (#805)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) (box, box-body-older)
  and [solid-body-a2](../../../config/experiments/solid-body-a2.json) (near-edge)

## Answer

**The box passes both clauses.**

1. **Reference gaps.** On kirk0's frozen tuning split the box scores 10 of the 11 reference gaps,
   all within the reference bound, with a mean absolute error of 0.18 m and a maximum of 0.48 m.
   The near-edge arm replayed at the same commit scores 4, 2 within the bound; the pilot's older
   build scored 7, 3 within. The eleventh gap's follower is unmatched under every arm.
2. **Paths on the corpus.** At the median of the 23 sites the field run fits a follower path for
   67 % of the vehicle-sized tracks under the box, against the rule's half; 21 sites are at or above
   half, the lowest are columbus-broadway (41 %) and pierce-haight (31 %).

With one track per vehicle (`box-body-older`) the median rises to 74 %, and the valid time below
to 290 s.

By the rule the box is worth carrying into the field run as its endpoint source, still labelled
provisional, with every endpoint temporally inferred.

**The first valid following time.** With the box, the field run finds 212 s of valid following
time on the corpus out of 1,593 s accounted, at 18 of the 23 sites, where kirk0 had none under any
reader. It is still suppressed far more than it is valid: the class gate takes 767 s, nearly half
of the accounted time. Of 2,399 vehicle-sized tracks under the box, 911 end classed car, truck or
bus and 1,367 end `dynamic`. The class gate is the next constraint, as
l8fol0002 (#804) found on kirk0; at faster sites it binds less.

**The trucks.** The spec asked for truck gaps apart because the box draws trucks short. On kirk0
every scored gap that involves a truck (five with truck 1 leading, five with truck 2 following) is
within the bound: each gap spans an end of each vehicle that the sensor sees, and the box holds
the seen end and draws the body short behind it
(centre against extent (#807)). A gap that needed a
truck's unseen end would be long by the missing length; none of kirk0's reference gaps does.

**The solid body cannot read half the corpus.** The near-edge arm's field run refused 11 of the 23
sites: the solid-body reader stops a case at the first solid body in the established state without
converged extents, a resolved heading and a physical reference. On the 12 sites it read, it fits
paths for 65 % of vehicle-sized tracks at the median (the box 73 % on the same sites) and finds 1 s
of valid time.

## Question

The spec's rule, written 2026-10-18 before the replays: the box source is worth carrying into the
field run (still provisional, no default changed) when (1) on kirk0's reference gaps it scores at
least as many gaps as the near-edge arm, and is within the reference bound on at least as many;
(2) on the corpus, follower paths fitted rise to at least half of the confirmed vehicle tracks at
the median site. Truck gaps are read apart.

**Fixed before the scored replays (2026-10-20).** Clause 1 is read against the near-edge arm at
this commit and the pilot's 7 of 11 (3 within) as a floor. Clause 2's "confirmed vehicle tracks" is
read as vehicle-sized tracks (lasting 2 s or more, p90 of the clusters' longer box side 3.0 m or
more; l8fol0001's definition), a path counting as fitted when the field run lists the track as a
follower with no path condition. Each corpus case replays 200 s after a 70 s warm-up;
embarcadero-folsom, label0001's lot, is not read.

## Method

**The reader.** `l8behaviour.TrajectoriesFromBoxes` reads each online estimate's published box
(migration 000062's `box_*` columns on `lidar_track_observations`) as the body: reference
`body_centre` at the box centre, heading from the box's axis, extents from its length and width,
every endpoint `temporally_inferred` because the box records no face; the front is named from the
course (an axis within 45° of a course of 0.5 m/s or more), held below that speed; class from the
track's label; extents converge at the track's third box; spreads are zero, so endpoint sigmas are
the filter's position sigma alone, a lower bound. `velocity report headway --db --boxes` runs the
field run with it, and the physical scorer reads it with `-a-boxes`/`-b-boxes`.

**The harness writes the box.** Before this change a replay's evidence database held no box: the
harness disables track persistence. It now writes each online estimate's `lidar_tracks` and
`lidar_track_observations` rows in the same transaction, using the server's own statements; the
replay baseline is byte-identical with and without the evidence.

**A storage key can lose a box.** `lidar_track_observations` is keyed by track and acquisition
time, and the statement replaces on conflict. When two consecutive frames' clusters start with
returns from the same firing at the frame seam, the second frame's row replaces the first, so one
estimate has no box. The reader counts such an estimate as a gap rather than refusing the case
(one in 23,667 estimates at bush-powell; four cases of 69 had one). The same key is the live
server's.

**Arms and scoring.**

| Arm            | Tuning                                  | Replay experiments                                       | Read as                    |
| -------------- | --------------------------------------- | -------------------------------------------------------- | -------------------------- |
| box            | `config/tuning.defaults.json`           | none                                                     | published boxes            |
| box-body-older | `config/tuning.defaults.json`           | `overlap_end_1`, `overlap_end_body`, `overlap_end_older` | published boxes            |
| near-edge      | `config/experiments/solid-body-a2.json` | none (the set enables the solid body)                    | solid bodies, online stage |

kirk0 was replayed whole (6 s warm-up, 1,024 sample points) and scored with
`lidar-ground-truth-eval perframe -physical-reference` on the frozen tuning split's following
episode (samples 472 to 530), each box arm against the near-edge arm. Each of the 23 corpus sites'
first segment was replayed per arm (256 sample points) and the field run read each case.

**Thirteen box-body-older cases did not repeat.** As in l8fol0001, the overlap end chose its
survivor in Go map order; the harness refused 13 of the 23 cases, which are read from the first
run's evidence. The fix is on its own branch
([determinism report](replay-determinism-2026-10.md#a-fourth-mechanism-the-overlap-ends-order-october-20)).

## Findings

### Clause 1: kirk0's reference gaps

Prediction minus reference, metres. Truck 1 is `obj_35c661db`, truck 2 `obj_8e829a06`, car 8
`obj_8c728991`.

| Sample | Follower → leader | Reference gap [bound]          | box                | box-body-older     | near-edge            |
| -----: | ----------------- | ------------------------------ | ------------------ | ------------------ | -------------------- |
|    491 | car 8 → truck 1   | 11.61 [8.98, 14.23]            | −0.17              | −0.17              | +0.23                |
|    491 | truck 2 → car 8   | 21.17 [17.31, 25.03], inferred | +0.48              | +0.42              | follower unmatched   |
|    495 | car 8 → truck 1   | 12.63 [9.98, 15.28]            | −0.08              | −0.07              | leader unmatched     |
|    495 | truck 2 → car 8   | 20.30 [19.15, 21.45], inferred | follower unmatched | follower unmatched | follower unmatched   |
|    500 | car 8 → truck 1   | 14.08 [13.11, 15.05]           | −0.15              | −0.14              | +7.89 (6.92 outside) |
|    500 | truck 2 → car 8   | 19.00 [18.07, 19.93]           | −0.19              | −0.16              | follower unmatched   |
|    502 | truck 2 → car 8   | 18.42 [17.27, 19.57]           | −0.12              | −0.11              | follower unmatched   |
|    504 | car 8 → truck 1   | 15.00 [13.16, 16.84]           | −0.14              | −0.16              | +6.64 (4.80 outside) |
|    504 | truck 2 → car 8   | 17.90 [16.79, 19.00]           | −0.07              | −0.07              | follower unmatched   |
|    509 | car 8 → truck 1   | 15.92 [14.00, 17.85]           | −0.24              | −0.24              | +0.27                |
|    509 | truck 2 → car 8   | 16.40 [15.49, 17.31]           | −0.18              | −0.18              | follower unmatched   |

| Arm            | Gaps scored of 11 | Within bound | Mean abs error | Max abs error |
| -------------- | ----------------: | -----------: | -------------: | ------------: |
| box            |                10 |           10 |         0.18 m |        0.48 m |
| box-body-older |                10 |           10 |         0.17 m |        0.42 m |
| near-edge      |                 4 |            2 |         3.76 m |        7.89 m |
| pilot floor    |                 7 |            3 |              – |             – |

The same scoring gives the box's other components against the near-edge arm: centre 1.54 m
against 1.67 m mean absolute error, yaw 0.04 against 0.28 rad, box IoU 0.64 against 0.29, and on
the MOT side HOTA 0.62 against 0.49 (0.65 with one track per vehicle).

### Clause 2: paths on the corpus

Share of vehicle-sized tracks with a fitted follower path, per site.

| Site                  | Vehicle-sized tracks |      box | box-body-older |           near-edge | Valid / accounted s (box) | Vehicle-sized tracks classed rigid (box) |
| --------------------- | -------------------: | -------: | -------------: | ------------------: | ------------------------: | ---------------------------------------: |
| 1st-mission           |                  124 |     0.51 |           0.49 |                   – |               0.3 / 114.0 |                                       26 |
| 3rd-folsom            |                   97 |     0.74 |           0.74 |                0.65 |                1.1 / 58.5 |                                       39 |
| ashbury-downey        |                   31 |     0.94 |           0.97 |                0.94 |               18.1 / 32.0 |                                       23 |
| broadway-gough        |                   94 |     0.74 |           0.83 |                   – |                0.4 / 34.7 |                                       35 |
| bush-powell           |                  114 |     0.59 |           0.61 |                   – |                0.8 / 82.0 |                                       35 |
| california-leon-baker |                   37 |     0.95 |           0.97 |                0.79 |                9.3 / 31.2 |                                       33 |
| columbus-broadway     |                  157 |     0.41 |           0.60 |                   – |               1.0 / 159.3 |                                       61 |
| columbus-north-point  |                   69 |     0.57 |           0.61 |                   – |                0.0 / 11.0 |                                       33 |
| embarcadero-bay       |                   89 |     0.64 |           0.84 |                   – |              24.9 / 148.8 |                                       40 |
| embarcadero-broadway  |                  159 |     0.67 |           0.74 |                0.62 |               37.9 / 99.4 |                                       46 |
| embarcadero-bryant    |                  186 |     0.55 |           0.62 |                   – |              47.3 / 134.8 |                                       74 |
| franklin-mcallister   |                  188 |     0.63 |           0.70 |                   – |                3.6 / 76.3 |                                       49 |
| fulton-divisadero     |                   76 |     0.78 |           0.81 |                0.64 |                0.0 / 21.2 |                                       24 |
| haight-divisadero     |                   45 |     0.67 |           0.79 |                   – |                0.0 / 35.0 |                                       19 |
| howard-6th            |                   59 |     0.64 |           0.54 |                0.52 |                0.0 / 58.9 |                                       11 |
| hyde-ofarrell         |                   87 |     0.59 |           0.68 |                   – |                4.8 / 37.0 |                                       16 |
| laguna-eddy           |                   70 |     0.56 |           0.59 |                0.40 |                0.7 / 64.8 |                                       12 |
| lombard-broderick     |                  176 |     0.71 |           0.79 |                0.60 |                0.2 / 94.7 |                                       46 |
| lombard-laguna        |                  156 |     0.81 |           0.84 |                0.74 |                9.3 / 55.4 |                                       85 |
| marina-broderick      |                   98 |     0.87 |           0.94 |                   – |              16.1 / 103.1 |                                       57 |
| marina-webster-beach  |                   40 |     0.97 |           1.00 |                0.95 |               18.6 / 32.5 |                                       35 |
| pierce-haight         |                   52 |     0.31 |           0.26 |                0.26 |                 0.0 / 9.3 |                                       12 |
| van-ness-sacramento   |                  195 |     0.71 |           0.79 |                0.67 |               17.9 / 98.6 |                                      100 |
| **Median**            |                      | **0.67** |       **0.74** | **0.65** (12 sites) |                           |                                          |

"–" is a site the near-edge field run refused. Vehicle-sized here is a size proxy, not a class;
the last column is how many of them end classed car, truck or bus.

| Arm            | Sites read | Median share | Pooled share | Encounters | Valid s | Accounted s | Suppressed by class s |
| -------------- | ---------: | -----------: | -----------: | ---------: | ------: | ----------: | --------------------: |
| box            |         23 |         0.67 |         0.66 |        852 |   212.3 |     1,592.5 |                 767.1 |
| box-body-older |         23 |         0.74 |         0.72 |        640 |   290.0 |     1,650.6 |                1016.3 |
| near-edge      |         12 |         0.65 |         0.63 |        208 |     1.1 |       417.9 |                 208.2 |

On kirk0 the field run fits paths for 14 of 18 vehicle-sized tracks under the box, 12 of 15 under
box-body-older and 16 of 24 under the near-edge arm, and finds no valid time under any: kirk0's
vehicles end classed `dynamic`.

## What it does not establish

- **Physical accuracy beyond kirk0's three vehicles.** The reference gaps are two trucks and a car
  on one tuning episode; nothing is held out. A box endpoint is always the box's held extent.
- **That the valid time is following.** The field run is provisional and reads tracks and boxes;
  its valid seconds are what the gate admits, not reviewed following.
- **A gap that needs an unseen end.** None of kirk0's reference gaps does; a truck whose unseen end
  bounds a gap would be long by its missing length.
- **Repeatability of 13 box-body-older cases**, which are one draw of a choice now fixed.

## Provenance

| Item        | Value                                                                                                                                                                                                                                                                        |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | reader and CLI `eb55988fa`–`d76ab354d`; physical scorer's box arm `52fcab2d7`; box-less estimates counted as gaps `2a34d9b48`; spec definitions `b8239ba36` and clause 1's comparator `ec766b355`, both before the scored replays                                            |
| Binaries    | harness `lidar-state-estimation-baseline` at `d76ab354d`; field runs with `velocity` at `d76ab354d`, the four cases with a box-less estimate and the 13 non-repeating box-body-older cases rescreened at `02a543bbd`; scorer at `96f0a0863`; all with the Makefile's LDFLAGS |
| Tuning      | `tuning.defaults.json` (sha256 `e6cf6c8a…`), `solid-body-a2.json` (`32f9fd29…`)                                                                                                                                                                                              |
| Captures    | kirk0, whole; first segment of 23 corpus sites (all but embarcadero-folsom), 70 s warm-up, 200 s                                                                                                                                                                             |
| References  | reviewed pack `ds_67842cb1b22b70e7`, physical revision 76, split `kirk0-tuning` of `kirk0-reference-tuning-r1.json` (split digest `sha256:74ddd659…`), episode `following-472-530`, gate 3 m                                                                                 |
| Raw outputs | evidence databases, field-run reports and screens under the run's scratch directory and the LiDAR volume (`velocity-campaign/exp-20261020/l8fol0003`); not in Git                                                                                                            |

## Recommendations

1. **Run the provisional field run with `--boxes`.** It scores every kirk0 reference gap it can
   reach within the bound, fits more paths than the solid body and reads every site. Keep it
   provisional and the endpoints temporally inferred until l8fol0005 scores a held-out window.
2. **Next, the class gate.** Half the accounted time on the corpus is suppressed for class; see
   class evidence (#804).
3. **Decide the observation table's key.** Keyed by track and acquisition time, a frame-seam
   cluster can replace the previous frame's row, in the live server as in evidence. Keying by
   frame, or by frame and acquisition time, is a migration and a live change, so it is a decision.
4. **Let the solid-body reader skip, not stop.** It refuses a whole case at one malformed solid
   body; counting such a body as a gap, as the box reader now does, would let the near-edge arm be
   compared on the whole corpus.
