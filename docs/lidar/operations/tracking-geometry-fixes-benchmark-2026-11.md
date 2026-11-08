# The November tracking and geometry fixes, benchmarked on kirk0: what they do and what comes next

<!-- ignore-style-length -->

Seven default-off options landed between November 6 and 8. Each was screened on its own question,
by its own author, with its own tools. This run reads them all with the same tools on the same
capture: identity on kirk0's reviewed labels, geometry on geom0015's 22 reviewed poses, and the
label-free noise, duplicate, count and cost measures. From those tables it ranks the next
background-noise (L3, L4) and tracking (L5) fixes. It answers
[l5id0019](../../../data/experiments/l5id/l5id0019.md).

- **Status:** Measured, 2026-11-08. One Claude Code cloud session; nothing adopted, no default
  changed
- **Spec:** [l5id0019](../../../data/experiments/l5id/l5id0019.md), rule committed at `2d79801a`
  before any replay
- **Layers:** L3 (through the track guard), L4 joining, L5 association, initialisation, solid body
  and published box; scored by the per-frame, physical and near-face evaluators and the label-free
  scripts
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`; the [join set](../../../config/experiments/motion-fragment-join.json),
  fingerprint `719d291ed197`. Every option is a replay experiment, not a tuning key
- **Related:** [kirk0's labels in a cloud run](cloud-run-kirk0-labelled-2026-11.md),
  [geom0015's bias baseline](d2-frozen-split-bias-baseline-2026-11.md),
  [truck membership (l4clus0010)](w3-truck-membership-extent-kirk0-2026-11.md),
  [experiment review, D13 to D15](experiment-review-2026-11-05.md#d-what-counts-as-noise),
  [face-state maths review](../../../data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md)

## Summary

- **The two Hugging Face placements were not read.** The dataset's file CDN
  (`cas-server.xethub.hf.co` and `us.aws.cdn.hf.co`) refused this host's CONNECT with 403 at
  22:03 to 22:04 UTC; `huggingface.co` itself answered. Every label-free figure below is kirk0's
  alone. The placements' segments, their recurrence grids and their per-site rows are not here.
- **The robust likelihood cost is the one large identity lever, and it costs brief tracks.** On
  kirk0 it takes identity switches from 90 to 6 and fragmentations from 46 to 30. All four paired
  intervals keep their sign: HOTA +0.0115 (+0.0023 to +0.0222), IDF1 +0.0166 (+0.0032 to
  +0.0338), switches −80 (−226 to −4), fragmentations −16 (−42 to −2). Duplicate pair-frames fall
  from 452 to 246. Brief tracks rise from 12 to 16 and clutter by 10.5 per hundred moving tracks,
  over R4's bounds of 3 and 5, so the rule does not name it for a held-out read. None of its brief
  tracks is a re-birth on a live body.
- **The near-edge tracked filter loses identity.** HOTA −0.061 (−0.116 to −0.021) and IDF1 −0.106
  (−0.164 to −0.013), with 186 more missed reference points, 30 more tracks and duplicate pairs up
  from 10 to 23. L3 and L4 give it the same clusters (9.16 a frame against 9.09), so the loss is in
  association and lifecycle.
- **The published box is where it was, and two options move it the wrong way.** `a0`'s box
  reproduces geom0015: 22 of 22 poses matched, median centre error 0.19 m (truck 1 0.19, car 8
  0.04, truck 2 1.49). `box_extent_solid_body` widens car 8's box from about 1.8 m to 2.3 to 2.6 m
  against a 1.73 m reference and moves its centre 0.29 m sideways. The overall median goes to
  0.28 m. No arm shortens truck 2's 3 m length shortfall by more than 0.1 m.
- **#989 shortened the solid body's truck 2 by 1.6 m.** The face-state solid body now reads truck
  2 at 4.60 m (reference 9.63 m) and its centre 2.66 m off, against 6.24 m and 0.81 m at `99a9627`.
  Car 8 improved from 0.89 to 0.36 m. A two-commit bisect puts the whole change on `fc9d8979`
  (#989). The shadow's widths run wide too: car 8 2.45 m against 1.73, truck 1 3.82 m against 2.91,
  and 6.20 m under the shared orientation.
- **Two no-ops and one near-no-op.** Covariance hygiene is byte-identical to `a0`. The face-state
  shadow leaves the tracks and the box byte-identical. The shared orientation changes identity by
  at most two switches and moves the box by 0.01 m, though it halves the shadow's side-face
  placement error.
- **Two-point initialisation does not help on kirk0.** Switches rise from 90 to 120, whose
  interval runs −28 to +116, outside the non-inferiority bound of +10. Fragmentations fall by 15,
  but that interval reaches zero. Duplicate pairs rise from 10 to 12.
- **The solid body quadruples Tracker.Update.** p50 rises from 0.58 to 2.1 to 2.7 ms and p99 from
  1.7 to 2.2 ms to 7.8 to 9.2 ms. Whole-frame p99 moves by 12 % at most.

## Question and rule

The question and the rule are in the [spec](../../../data/experiments/l5id/l5id0019.md), committed
at `2d79801a` before the first replay. In short:

- **R1, identity.** A gain or loss is claimed only where the paired 5th to 95th percentile interval
  lies wholly on one side of zero. Non-inferiority is l4clus0010's J1: the HOTA and IDF1 5th
  percentiles at −0.02 or above, the switch 95th at +10 or below.
- **R2, no-op.** The tracking baseline equals `a0`'s byte for byte, and for a geometry arm the box
  rows score as `a0`'s.
- **R3, geometry.** The box moves when the median centre error moves by 0.05 m overall, or by
  0.25 m for one vehicle, with no fewer matches. Truck length moves when the median absolute error
  moves by 0.5 m.
- **R4, noise.** An arm costs noise when brief tracks rise by more than 3, clutter by more than 5
  per hundred moving tracks, or duplicate pairs at all. It costs counts when the moving count moves
  by more than 5 % or p85 by more than 0.5 m/s.
- **R5, cost.** Tracker.Update or frame p99 more than 25 % over `a0`'s flags the arm for
  perf0001.
- **R6, what goes forward.** An arm is named for label0003's held-out window when it gains on R1
  or moves R3 toward the references, is R1 non-inferior, and costs no noise under R4.

## Method

- **Replays.** `lidar-state-estimation-baseline` on kirk0 whole as a one-case corpus:
  `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case, the
  committed cohort manifest, a scratch use ledger at migration 71. Each arm ran its first and repeat
  passes one arm at a time on an otherwise idle host, so the timings compare. Each took 70 to 76 s.
- **Box replays.** `lidar-box-eval -variants none -rows -require-settled=false -warmup 6
-duration 0`, the rest of the capture scored (771 frames), with an evidence database so the solid
  body has its points. They ran after the timed replays, beside the scorers.
- **Labels.** The reviewed pack `ds_67842cb1b22b70e7`, reconstructed by `lidar-pack-rebuild` as
  in the [cloud labelled run](cloud-run-kirk0-labelled-2026-11.md#the-reconstructed-pack). The
  packets and returns match the published digests. The points are decoded by the current parser,
  and their digest differs. The split was drafted with `-from-seconds 6 -split kirk0-tuning`;
  `obj_788622b5`, the parked car, is ignored by every score.
- **Identity.** `perframe` on the online-stage estimates as declared baselines, each arm against
  `a0`, with `-bootstrap 2000 -bootstrap-seed 20261108`.
- **Geometry.** `perframe -physical-reference -a-boxes -b-boxes`, gate 3 m, for the published
  boxes. For the solid-body arms, also `-b-solid-body` against `a0`'s box. The physical references
  (revision 77) pin the published pack's digest, so a scratch copy re-pinned to the reconstructed
  digest was scored, as l4clus0010 did. The only change is that field. The 22 reviewed poses
  scored are geom0015's: truck 1 nine, car 8 seven, truck 2 six, all within samples 472 to 530.
  `lidar-near-face-eval` read the solid bodies' faces and spans against the labelled masks.
- **Label-free.** On each arm's evidence: `lidar_flip_cost.py --match footprint` and
  `lidar_figure_sensitivity.py` against `a0`'s evidence, and `lidar_noise_screen.py` and
  `lidar_clutter_origin.py` on their own. Duplicates come from `lidar-box-eval`'s summary.
- **Bisect.** The face-state arm replayed again at `50c7790b` (before #989) and at `fc9d8979`
  (#989), each built from `git archive` with the Makefile's LDFLAGS and scored with this run's
  evaluator. The l4clus0010 session's face-state evidence at `99a9627` was rescored the same way.

**The anchor.** `a0` reproduces every published kirk0 figure it was checked against. Full
comparison: HOTA 0.4284, IDF1 0.4728, 90 switches, 46 fragmentations. Box-eval: 44 vehicle tracks,
10 duplicate pairs, 4,523 observed vehicle frames. Noise screen: 12 brief tracks, 0.6947 spurious
clusters a frame. Its tracking baseline digest is `79d9abff…`, as l4clus0010 recorded. The box rows
match geom0015's. `join-membership` reproduces l4clus0010's `fix` arm to the fourth decimal.

## kirk0, labelled: identity

Full comparison (all hypotheses), then the paired bootstrap against `a0` (19 scored objects, 2,000
draws). The bootstrap's labelled-object point figures for `a0` are HOTA 0.4798 (0.4111 to 0.5415),
IDF1 0.5582 (0.5107 to 0.6232) and 90 switches (4 to 242).

| Arm                  |   HOTA |   IDF1 | Switches | Frag |  FN | Tracks | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches       | Δ frag         | R1                               |
| -------------------- | -----: | -----: | -------: | ---: | --: | -----: | --------------------------- | --------------------------- | ---------------- | -------------- | -------------------------------- |
| `a0`                 | 0.4284 | 0.4728 |       90 |   46 | 265 |     79 |                             |                             |                  |                |                                  |
| `two-point`          | 0.4238 | 0.4689 |      120 |   31 | 248 |     82 | −0.0123 / −0.0043 / +0.0047 | −0.0182 / −0.0019 / +0.0144 | −28 / +28 / +116 | −41 / −15 / 0  | No change; not non-inferior      |
| `hygiene`            | 0.4284 | 0.4728 |       90 |   46 | 265 |     79 | 0 / 0 / 0                   | 0 / 0 / 0                   | 0 / 0 / 0        | 0 / 0 / 0      | No-op (R2)                       |
| `robust`             | 0.4362 | 0.4827 |        6 |   30 | 247 |     81 | +0.0023 / +0.0115 / +0.0222 | +0.0032 / +0.0166 / +0.0338 | −226 / −80 / −4  | −42 / −16 / −2 | Gains on all four                |
| `face-state`         | 0.4284 | 0.4728 |       90 |   46 | 265 |     79 | 0 / 0 / 0                   | 0 / 0 / 0                   | 0 / 0 / 0        | 0 / 0 / 0      | No-op (R2): a shadow             |
| `near-edge`          | 0.3539 | 0.3514 |       33 |   51 | 451 |    109 | −0.1157 / −0.0612 / −0.0210 | −0.1639 / −0.1059 / −0.0125 | −201 / −55 / +24 | −23 / +5 / +29 | Loses HOTA and IDF1              |
| `box-extent`         | 0.4297 | 0.4740 |       88 |   46 | 265 |     78 | +0.0001 / +0.0016 / +0.0032 | +0.0002 / +0.0015 / +0.0029 | −6 / −2 / 0      | 0 / 0 / 0      | Gains HOTA and IDF1 (small)      |
| `shared-orientation` | 0.4285 | 0.4731 |       88 |   46 | 265 |     78 | −0.0001 / +0.0002 / +0.0006 | −0.0007 / +0.0004 / +0.0016 | −6 / −2 / 0      | 0 / 0 / 0      | No change                        |
| `join-membership`    | 0.4323 | 0.4769 |       90 |   42 | 262 |     76 | −0.0002 / +0.0037 / +0.0081 | +0.0004 / +0.0032 / +0.0055 | 0 / 0 / 0        | −12 / −4 / 0   | Gains IDF1                       |
| `combined`           | 0.4377 | 0.4833 |        8 |   27 | 247 |     81 | +0.0015 / +0.0130 / +0.0232 | +0.0036 / +0.0164 / +0.0317 | −224 / −78 / 0   | −44 / −19 / −3 | Gains HOTA, IDF1, fragmentations |

Every arm but `two-point` and `near-edge` is R1 non-inferior. The bootstrap's labelled-object HOTA
is 0.4917 for `robust` and 0.4931 for `combined`, against `a0`'s 0.4798. `combined` carries
two-point initialisation, which adds two switches to robust's six.

## kirk0, labelled: geometry

**The published box on geom0015's 22 reviewed poses** (gate 3 m; median centre error over matched
poses, then by vehicle as matched: median error / median offset toward the sensor, m; published
length, median over the vehicle's poses, against the reference):

| Arm                  | Matched | Median (m) | Truck 1 (L 10.63)        | Car 8 (L 4.26)          | Truck 2 (L 9.63)        | Box IoU | Yaw  | R3                       |
| -------------------- | ------: | ---------: | ------------------------ | ----------------------- | ----------------------- | ------: | ---- | ------------------------ |
| `a0`                 | 22 / 22 |      0.190 | 9: 0.19 / +0.15; L 10.60 | 7: 0.04 / −0.03; L 4.40 | 6: 1.49 / +0.25; L 6.59 |   0.887 | 1.2° |                          |
| `two-point`          | 22 / 22 |      0.202 | 9: 0.19 / +0.16; L 10.63 | 7: 0.01 / +0.01; L 4.40 | 6: 1.48 / +0.27; L 6.62 |   0.878 | 1.3° | No move                  |
| `hygiene`            | 22 / 22 |      0.190 | as `a0`                  | as `a0`                 | as `a0`                 |   0.887 | 1.2° | No-op                    |
| `robust`             | 22 / 22 |      0.186 | 9: 0.19 / +0.15; L 10.61 | 7: 0.08 / −0.07; L 4.40 | 6: 1.47 / +0.27; L 6.61 |   0.859 | 1.3° | No move                  |
| `face-state`         | 22 / 22 |      0.190 | as `a0`                  | as `a0`                 | as `a0`                 |   0.887 | 1.2° | No-op                    |
| `near-edge`          | 22 / 22 |      0.193 | 9: 0.19 / +0.14; L 10.64 | 7: 0.03 / −0.02; L 4.40 | 6: 1.04 / −0.49; L 6.69 |   0.893 | 1.2° | Truck 2 toward (−0.45 m) |
| `box-extent`         | 22 / 22 |      0.281 | 9: 0.19 / +0.13; L 10.62 | 7: 0.29 / −0.29; L 4.40 | 6: 1.49 / +0.25; L 6.59 |   0.802 | 0.8° | Away: overall, car 8     |
| `shared-orientation` | 22 / 22 |      0.178 | 9: 0.19 / +0.13; L 10.62 | 7: 0.09 / −0.09; L 4.40 | 6: 1.49 / +0.25; L 6.59 |   0.845 | 0.8° | No move                  |
| `join-membership`    | 22 / 22 |      0.190 | 9: 0.19 / +0.15; L 10.60 | 7: 0.03 / −0.02; L 4.40 | 6: 1.49 / +0.25; L 6.59 |   0.888 | 1.2° | No move                  |
| `combined`           | 22 / 22 |      0.194 | 9: 0.19 / +0.14; L 10.63 | 7: 0.01 / −0.00; L 4.40 | 6: 1.40 / +0.27; L 6.68 |   0.888 | 1.2° | No move                  |

No arm moves the published truck lengths by 0.5 m. The box already reads truck 1 within 0.03 m on
these poses; truck 2 stays about 3 m short in every arm. Under `box-extent`, car 8's published
width is 2.32 to 2.56 m on its seven poses, against 1.78 to 1.90 m under `a0` and a 1.73 m
reference. Its centre moves 0.19 to 0.41 m across the body, half the excess width.

**The solid body's own rows** (`-b-solid-body`, the same 22 poses):

| Arm and build                     | Median (m) | Truck 1: error / toward; L, W | Car 8: error / toward; L, W | Truck 2: error / toward; L, W        | Truck \|L\| error, median of 15 |
| --------------------------------- | ---------: | ----------------------------- | --------------------------- | ------------------------------------ | ------------------------------: |
| `face-state` at `99a9627`         |      1.195 | 1.29 / +0.16; 9.85, —         | 0.89 / −0.78; 4.95, —       | 0.81 / +0.54; 6.24, —                |                          0.82 m |
| `face-state` at `50c7790b` (#984) |      1.195 | the same as at `99a9627`      | the same                    | the same                             |                          0.82 m |
| `face-state` at `fc9d8979` (#989) |      1.028 | 1.03 / +0.35; 9.73, 3.82      | 0.36 / −0.32; 3.90, 2.45    | 2.66 / +2.64; 4.60, 1.90             |                          0.98 m |
| `face-state` (this build)         |      1.028 | the same as at `fc9d8979`     | the same                    | the same                             |                          0.98 m |
| `near-edge`                       |      0.552 | 0.55 / +0.37; 9.61            | 0.31 / −0.27; 3.90          | 2.68 / +2.66; 4.53                   |                          1.03 m |
| `box-extent`                      |      1.028 | as `face-state`               | as `face-state`             | as `face-state`                      |                          0.98 m |
| `shared-orientation`              |      1.986 | 2.00 / −0.10; 9.54, 6.20      | 0.21 / −0.19; 3.95, 1.99    | 2.98 / +2.84; 3.71, 3.05 (5 matched) |                          1.10 m |
| `combined`                        |      2.021 | 2.03 / −0.12; 9.54            | 0.13 / −0.09; 3.95          | 2.97 / +2.82; 3.76                   |                          1.09 m |

References: truck 1 10.63 × 2.91 m, car 8 4.26 × 1.73 m, truck 2 9.63 × 2.84 m.

**Against the labelled spans** (`lidar-near-face-eval`, all scored instants). The share of
instants whose believed length falls short of the labelled span by more than 0.2 m is 6 % before
#989 and 9 % at this build. F0, on an older build, found the belief short of the span on 83 % of
car and 97 % of truck instants. The span shortfall F0 found is gone. What the references still show is length the frame's span cannot see. Side-face
placement, mean absolute: 0.42 m under `face-state`, 0.15 m under `shared-orientation`, 0.14 m
under `combined`. The share of instants whose width is short of the span by more than 0.2 m:
26 %, 7 % and 6 %.

## kirk0, label-free: noise, duplicates and counts

kirk0 is the only case read. For per-site and pooled read the same: one site, one row.

| Arm                  | Tracks | Brief | Near-static | Clutter (artefact / stationary / fragment / unclassed) | Clusters a frame | Spurious a frame | Unowned clusters | Moving (lost / short / added) | p85, p98 (m/s) | Brief origin (re-birth / split / other) | Vehicle tracks | Dup pairs (per 100) | Pair-frames |
| -------------------- | -----: | ----: | ----------: | ------------------------------------------------------ | ---------------: | ---------------: | ---------------: | ----------------------------- | -------------- | --------------------------------------- | -------------: | ------------------- | ----------: |
| `a0`                 |     79 |    12 |           3 | 15 (6 / 2 / 3 / 4)                                     |             9.09 |           0.6947 |              537 | 38                            | 7.06, 8.83     | —                                       |             44 | 10 (22.7)           |         452 |
| `two-point`          |     82 |    13 |           3 | 16 (6 / 2 / 3 / 5)                                     |                — |           0.6675 |              517 | 40 (0 / 0 / 0)                | 6.98, 9.03     | 0 / 1 / 12                              |             45 | 12 (26.7)           |         510 |
| `hygiene`            |     79 |    12 |           3 | 15 (6 / 2 / 3 / 4)                                     |                — |           0.6947 |              537 | 38 (0 / 0 / 0)                | 7.06, 8.83     | 0 / 1 / 11                              |             44 | 10 (22.7)           |         452 |
| `robust`             |     81 |    16 |           3 | 19 (8 / 1 / 3 / 7)                                     |             9.09 |           0.6856 |              529 | 41 (0 / 0 / 0)                | 6.99, 8.83     | 0 / 1 / 15                              |             43 | 8 (18.6)            |         246 |
| `face-state`         |     79 |    12 |           3 | 15 (6 / 2 / 3 / 4)                                     |                — |           0.6947 |              537 | 38 (0 / 0 / 0)                | 7.06, 8.83     | 0 / 1 / 11                              |             44 | 10 (22.7)           |         452 |
| `near-edge`          |    109 |    21 |           4 | 25 (8 / 2 / 3 / 12)                                    |             9.16 |           1.0220 |            4,408 | 41 (0 / 3 / 0)                | 7.05, 8.44     | 0 / 1 / 20                              |             66 | 23 (34.8)           |         425 |
| `box-extent`         |     78 |    12 |           3 | 15 (6 / 2 / 2 / 5)                                     |             9.07 |           0.7025 |              543 | 38 (0 / 0 / 0)                | 7.02, 8.83     | 0 / 1 / 11                              |             45 | 9 (20.0)            |         445 |
| `shared-orientation` |     78 |    12 |           3 | 15 (6 / 2 / 3 / 4)                                     |                — |           0.7115 |              549 | 38 (0 / 0 / 0)                | 7.06, 8.83     | 0 / 1 / 11                              |             44 | 9 (20.5)            |         527 |
| `join-membership`    |     76 |    11 |           3 | 14 (7 / 2 / 1 / 4)                                     |             8.97 |           0.6468 |              500 | 36 (0 / 0 / 0)                | 7.06, 8.83     | 0 / 0 / 11                              |             44 | 9 (20.5)            |         416 |
| `combined`           |     81 |    15 |           3 | 18 (10 / 1 / 1 / 6)                                    |             9.02 |           0.6533 |              504 | 40 (0 / 0 / 1)                | 6.98, 9.03     | 0 / 0 / 15                              |             43 | 5 (11.6)            |         225 |

Clusters a frame are the evidence's observation records over its 773 frames, read for the arms
where the question arose. **The D14 recurrence grid is empty on kirk0**: no 0.5 m cell holds
unowned clusters in 10 or more distinct seconds, in any arm. kirk0 runs 77 s, and its 500-odd
unowned clusters do not return to one place. The grid needs the placements' segments, which were
not read. Under `near-edge` the unowned count rises eightfold with the clusters a frame unchanged.
Ownership is a lasting track's row within 1 m of the cluster's centroid, and a body-centre track
sits metres from a truck's centroid, so most of that rise is the test, not new foreground.

**Against R4** (each arm less `a0`):

| Arm                  | Brief | Clutter per 100 moving | Duplicate pairs | Moving count | p85 (m/s) | Noise | Counts |
| -------------------- | ----: | ---------------------: | --------------- | -----------: | --------: | ----- | ------ |
| `two-point`          |    +1 |                   +2.6 | 10 → 12         |   +2 (5.3 %) |     −0.08 | Costs | Costs  |
| `hygiene`            |     0 |                      0 | 10 → 10         |            0 |      0.00 | —     | —      |
| `robust`             |    +4 |                  +10.5 | 10 → 8          |   +3 (7.9 %) |     −0.07 | Costs | Costs  |
| `face-state`         |     0 |                      0 | 10 → 10         |            0 |      0.00 | —     | —      |
| `near-edge`          |    +9 |                  +26.3 | 10 → 23         |   +3 (7.9 %) |     −0.00 | Costs | Costs  |
| `box-extent`         |     0 |                      0 | 10 → 9          |            0 |     −0.04 | —     | —      |
| `shared-orientation` |     0 |                      0 | 10 → 9          |            0 |     −0.00 | —     | —      |
| `join-membership`    |    −1 |                   −2.6 | 10 → 9          |   −2 (5.3 %) |      0.00 | —     | Costs  |
| `combined`           |    +3 |                   +7.9 | 10 → 5          |   +2 (5.3 %) |     −0.08 | Costs | Costs  |

On kirk0, 5 % of the moving count is two tracks. No arm lost, shortened or added a moving track
against `a0` except `near-edge` (3 shortened) and `combined` (1 added). So the count changes are tracks
that overlap `a0`'s: carried in more or fewer pieces, not new vehicles.

## Cost

Replay wall time on the cloud container, each arm alone. Two runs per arm: the first writes the
evidence database, whose writes fall inside the frame callback; the repeat does not.

| Arm                  | Tracker.Update p50, first / repeat (ms) | Tracker.Update p99 (ms) | Frame p50 (ms) | Frame p99 (ms) | R5               |
| -------------------- | --------------------------------------- | ----------------------- | -------------- | -------------- | ---------------- |
| `a0`                 | 0.58 / 0.57                             | 1.74 / 2.21             | 56.4 / 20.8    | 112.3 / 80.4   |                  |
| `two-point`          | 0.55 / 0.56                             | 1.74 / 2.42             | 54.5 / 20.3    | 114.5 / 79.8   | —                |
| `hygiene`            | 0.55 / 0.54                             | 1.72 / 2.05             | 55.0 / 19.5    | 111.8 / 74.6   | —                |
| `robust`             | 0.55 / 0.53                             | 1.69 / 1.91             | 55.1 / 19.9    | 121.0 / 81.9   | —                |
| `face-state`         | 2.14 / 2.23                             | 7.81 / 8.02             | 57.8 / 21.7    | 121.0 / 78.3   | Flagged (Update) |
| `near-edge`          | 2.29 / 2.48                             | 7.79 / 9.17             | 57.3 / 21.6    | 116.2 / 79.0   | Flagged (Update) |
| `box-extent`         | 2.11 / 2.18                             | 7.81 / 8.26             | 56.6 / 21.4    | 118.7 / 82.7   | Flagged (Update) |
| `shared-orientation` | 2.53 / 2.60                             | 8.08 / 8.45             | 58.5 / 23.2    | 125.3 / 83.2   | Flagged (Update) |
| `join-membership`    | 0.53 / 0.57                             | 1.58 / 2.13             | 54.3 / 21.3    | 109.8 / 73.6   | —                |
| `combined`           | 2.59 / 2.67                             | 7.95 / 8.22             | 58.8 / 22.6    | 122.8 / 79.5   | Flagged (Update) |

`a0`'s two runs differ by 27 % at the Tracker.Update p99 on identical work, so R5 is read only
where both runs clear the bound. Every arm with the solid body does, by 260 to 365 %. No arm's
frame p99 does.

## Against the rule

| Arm                  | R1                                   | R2    | R3                    | R4                             | R5      | R6                  |
| -------------------- | ------------------------------------ | ----- | --------------------- | ------------------------------ | ------- | ------------------- |
| `two-point`          | No change; not non-inferior          |       | No move               | Noise (duplicates), counts     |         | Not named           |
| `hygiene`            | No change                            | No-op | No-op                 | —                              |         | Not named: a no-op  |
| `robust`             | Gains on all four; non-inferior      |       | No move               | Noise (brief, clutter), counts |         | Not named: R4 noise |
| `face-state`         | No change                            | No-op | No-op (box)           | —                              | Flagged | Not named: a no-op  |
| `near-edge`          | Loses HOTA, IDF1                     |       | Truck 2 toward        | Noise, counts                  | Flagged | Not named: a loss   |
| `box-extent`         | Gains HOTA, IDF1; non-inferior       |       | Away (overall, car 8) | —                              | Flagged | **Named**           |
| `shared-orientation` | No change                            |       | No move               | —                              | Flagged | Not named           |
| `join-membership`    | Gains IDF1; non-inferior             |       | No move               | Counts (−2 tracks)             |         | **Named**           |
| `combined`           | Gains HOTA, IDF1, frag; non-inferior |       | No move               | Noise (clutter), counts        | Flagged | Not named: R4 noise |

R6 reads noise, not counts, so `join-membership`'s two fewer moving tracks do not exclude it. R6
names `box-extent` on a gain of 0.0016 HOTA while R3 reads its box moving away from the
references. The rule did not foresee that, and the verdict stands as written. The recommendation
below says what to do before spending the held-out window on it.

## Interpretation

Each reading below is marked measured or inferred.

**The association cost, not the geometry, is where kirk0's identity errors are.** Measured: the
robust cost alone does what the overlap end with tentative priority did (5 switches, HOTA 0.4374
in the [cloud run](cloud-run-kirk0-labelled-2026-11.md)). It does so without ending tracks and
without the overlap end's 18 brief tracks; it leaves 16. It halves duplicate pair-frames. Inferred:
a coasting track's predicted covariance grows, so its bare `d²` cost falls and it outbids a live
track for a cluster. That is S3's finding. The likelihood's `ln det S` charges the coasting track
for its uncertainty, and the Student-t tail stops one large innovation from dominating. This run
does not separate the two: plain `likelihood_cost` was not an arm.

**Robust's brief tracks are births away from bodies, not re-births.** Measured: 15 of its 16 brief
tracks are "other" (none a re-birth, one a split), and its clutter gains are artefacts (6 to 8) and
unclassed (4 to 7). Inferred: clusters that a coasting track absorbed under `d²` are now left
unassigned, and some confirm as tracks of under a second. The birth gate on the live body
(`birth_gate_body`) cannot remove them. They are not on a body.

**Putting the solid body on the tracked path costs identity before it can help geometry.**
Measured: `near-edge` loses 0.06 HOTA and 0.11 IDF1, misses 186 more reference points and adds 30
tracks, with L3 and L4 unchanged. Its published box moves truck 2 toward the reference, from 1.49
to 1.04 m. Inferred: the tracked body's predicted faces come from the extent state, which reads
truck 2 at 4.5 m, so the truck's cluster falls outside the face-residual gate and seeds new tracks.

**The extent state is now honest about the span and wrong about the body.** Measured: since #989
the believed length almost never falls short of the labelled span (9 % of instants), but truck 2
reads 4.60 m against 9.63 m, where the box model's own belief reads 6.59 m and #989's parent read
6.24 m. The widths are wide on every vehicle: car 8 2.45 m against 1.73 m. Inferred: two faults.
First, the projection holds the length to what the current frame shows, so length seen at
different times (the rear on approach, the front on departure) no longer accumulates, which is the
observability case of the maths review's §3.1. Second, spans measured across an axis whose
heading is not yet converged are admitted as width bounds and never released. A lower bound only
ever raises the belief. The shared orientation, which takes the box filter's heading from a
track's first frames, lifts truck 1's width to 6.20 m and is consistent with that.

**`box_extent_solid_body` imports the extent state's faults into the published box.** Measured:
truck lengths do not move, because the frame's span floor exceeds the extent state's length;
car 8's width does, to 2.3 to 2.6 m. So as built the option carries only the wrong half of the
extent state to the product. Its 0.0016 HOTA gain is the L3 track guard's feedback on one or two
tracks, not a better box.

**Two-point initialisation misfires on kirk0.** Measured: 30 more switches at the point estimate,
an interval reaching +116, and two more duplicate pairs, with 15 fewer fragmentations. Inferred:
the pair's velocity has a variance of `2R/Δt²`, about 10 m²/s² per axis. On a long body whose
first two clusters come from different parts, it starts the track with a large, wrong velocity at
high confidence, and the young track takes a neighbour's cluster. The maths review predicted a
strict improvement; on this capture it is not one.

**Background noise on kirk0 is small and not recurrent.** Measured: 9.09 clusters a frame, 0.69 of
them spurious, 537 unowned over 773 frames, none recurring over 10 s, and six of fifteen clutter
tracks small artefacts away from any mover. Cluster counts move by at most 1 % across every arm,
`near-edge` included, so the L3 track guard's feedback on what L4 sees is small here. The one
arm that lowers spurious clusters is the join: 0.695 to 0.647, with fragments of a mover 3 to 1.

## The next fixes, ranked

The ranking is by the size of the measured defect in what a user sees, then what the fix unblocks.
Each fix states its evidence row, expected effect, what it builds on, and what would falsify it.

### Tracking (L5)

1. **A birth rule for robust's released clusters, then robust on the corpus.**
   - Evidence: the `robust` rows. Switches 90 to 6, all four intervals keep their sign, but brief
     tracks rise +4 and clutter +10.5 per hundred, all "other" births.
   - Expected effect (inferred): a tentative track confirms only when its clusters' size and
     displacement over the tentative frames are a road user's. That should return brief tracks to
     within `a0`'s plus 3 and keep switches under 10.
   - Builds on: `robust_likelihood_cost` (#983). It would replace the overlap end as the duplicate
     lever; that one failed its corpus rule on lost real tracks (10 %).
   - Zero-code controls to run first: `robust` with `likelihood_cost`'s `ln det S` alone (refused
     together, so as a separate arm), and `robust` with the L3 `support_filter`.
   - Falsified if: on the placements' segments the duplicate fall does not hold, or the brief
     tracks prove to be re-births (then `birth_gate_body` is the tool).
2. **Restore length accumulation in the extent state and gate width bounds on the heading.**
   - Evidence: the solid-body table and the bisect. #989 took truck 2 from 6.24 to 4.60 m (centre
     0.81 to 2.66 m); widths read 2.45, 3.82 and, under the shared orientation, 6.20 m.
   - Expected effect (inferred): keep #989's span projection as a lower bound per frame. Take the
     censored posterior over the largest consistent span across frames along a converged axis
     (maths review §3.2), and refuse or widen a span bound whose axis variance exceeds a set
     bound. Truck 2 should then read at least the box's 6.59 m and car widths under 2.0 m.
   - Builds on: #970, #982 and #989.
   - Falsified if: truck 2 stays under 6.24 m, or car 8's solid-body centre error rises back over
     0.5 m.
3. **Take only evidenced length from the extent state into the box.**
   - Evidence: the `box-extent` rows. Car 8's box width goes from 1.8 to 2.3 to 2.6 m and its
     centre from 0.04 to 0.29 m off, while no truck length moves.
   - Expected effect (inferred): use `2hL` only where it exceeds the box's own belief with
     two-sided evidence, and never `2hW` while the far side is unseen. Car 8 should return to
     `a0`'s rows, and with fix 2 truck 2's box should gain length.
   - Builds on: `box_extent_solid_body` (#984).
   - Falsified if: with fix 2 in, truck 2's box length still does not move past 6.59 m.
4. **Fix the near-edge tracked gate before more geometry rides the tracked path.**
   - Evidence: the `near-edge` row. HOTA −0.06, IDF1 −0.11, 186 more misses, 30 more tracks, with
     L3 and L4 unchanged.
   - Expected effect (inferred): with fix 2's extents the face residual should gate truck clusters
     again. Run `near_edge_track_a1` (medoid gate) and `near-edge` with `robust_likelihood_cost`
     as zero-code arms first, to locate the loss.
   - Builds on: `near_edge_track` and its A1 ablation.
   - Falsified if: A1 shows the same loss; the gate is then not the cause.
5. **Restate two-point initialisation.**
   - Evidence: the `two-point` row. Switches +30 (interval to +116), duplicates +2.
   - Expected effect (inferred): initialise from the class's speed range (the review's
     `(v_max/2)²` variance) or a least-squares fit over the tentative frames rather than one pair.
     Fragmentations should keep their −15 while switches stay within non-inferiority.
   - Builds on: `two_point_velocity_init` (#969).
   - Falsified if: the variant also raises switches. Then the cost is in young-track gating, not
     the initial velocity.
6. **Keep the shared orientation for heading; it waits on fix 2.**
   - Evidence: box yaw median 1.2° to 0.8°, side placement 0.42 to 0.15 m, against truck 1's
     solid-body width 6.20 m.
   - Expected effect (inferred): with width bounds gated on the heading, its side-face gain should
     hold without the width blow-up.
   - Falsified if: with fix 2 in, the width still grows past 3.5 m on truck 1.

### Background noise (L3, L4)

1. **Take track-conditioned joining to the corpus, with a merge reference.**
   - Evidence: the `join-membership` row. Spurious clusters 0.695 to 0.647 a frame, clusters 9.09
     to 8.97, fragments of a mover 3 to 1, duplicates 10 to 9, an IDF1 gain that keeps its sign,
     and two fewer moving tracks with none lost.
   - Expected effect (measured on kirk0): about 7 % fewer spurious clusters.
   - Builds on: #986. l4clus0010's fix (b), the solid body's extent in the footprint, should wait
     for tracking fix 2: today it would read truck 2 at 4.6 m.
   - Falsified if: the placements' segments show no fall in spurious clusters, or any merge of two
     vehicles side by side.
2. **Find out what the artefact births are before building a filter.**
   - Evidence: artefacts are the largest clutter class in every arm (6 of 15 under `a0`, 10 of 18
     under `combined`), no brief track is a re-birth in any arm, and the recurrence grid is empty
     on a 77 s capture.
   - Expected effect (inferred): if the D14 grid on the placements' five-minute segments shows the
     artefacts recurring in background cells, the existing default-off L3 `support_filter` is the
     first lever. If they do not recur, the lever is L5's birth rule (tracking fix 1).
   - Falsified if: labels show the artefacts are small road users. Neither filter should then
     ship.
3. **No L3 settling or region fix is ranked by this run.**
   - Evidence: L4's input moved by at most 1 % in every arm, and no region or settling option was
     an arm.
   - Next step: the placements' recurrence grids and the l3bg0030 corpus read are what would rank
     them. Both need captures this host could not fetch.

### Held-out reads on label0003's window

label0003's window is embarcadero-folsom 575 to 755 s.

- **Worth a held-out read:** `join-membership` now, with a merge reference (named by R6). `robust`
  once fix 1's birth rule holds its brief tracks: by the rule it is not named today, and it is the
  strongest identity candidate this run measured.
- **Named by the rule, but defer:** `box-extent`. Spend the window on it after fix 3, since today
  it moves car 8's box 0.25 m away.
- **No-ops on kirk0:** `hygiene` (byte-identical; the cap is never reached), the `face-state`
  shadow (byte-identical tracks and box; it costs Tracker.Update a factor of four), and
  `shared-orientation` for identity.
- **Not before rework:** `near-edge` (a loss) and `two-point` (a regression on switches).

## What this does not establish

- **Anything off kirk0.** The placements were refused by the network policy. Every label-free
  figure is one 77 s capture with 38 moving tracks, so the 5 % count bound is two tracks and the
  brief-track bound is a handful.
- **Anything held out.** kirk0 is a tuning capture. Its two trucks carry most of the switches and
  all of the truck instants.
- **That the reconstructed pack is the published one.** Its returns are and its points are not.
  The physical references were re-pinned in scratch, changing only the pack digest.
- **Which half of the robust cost acts.** Plain `likelihood_cost` was not an arm.
- **The mechanisms marked inferred.** That two-point's misfire is a wrong velocity, that the width
  ratchet is heading-driven and that robust's brief tracks are released clusters each come from
  reading the rows and the code, not from a test that isolates them.
- **Pi cost.** The timings are a cloud container's.

## Provenance

| Item                 | Value                                                                                                                                                                                                                                                                                                                                                                           |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base                 | main `fe6549b3` (`fe6549b39ef487d20693b17b2e48e5d1743878c0`); spec committed at `2d79801a`, which changes no tool source                                                                                                                                                                                                                                                        |
| Tools                | Built at `fe6549b3` with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA` the full sha) and the `pcap` tag; every replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `fe6549b3…`. `lidar-state-estimation-baseline` `sha256:8f73e500…`, `lidar-box-eval` `9f2b9445…`, `lidar-ground-truth-eval` `ee1fc25e…`, `lidar-near-face-eval` `befbf327…` |
| Bisect builds        | `50c7790b` (#984, before #989) and `fc9d8979` (#989), from `git archive`, the same LDFLAGS; the l4clus0010 session's face-state evidence at `99a9627` rescored                                                                                                                                                                                                                  |
| Parameter sets       | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`; `config/experiments/motion-fragment-join.json`, fingerprint `719d291ed197`, file SHA-256 `2e604040…`                                                                                                                                                                                       |
| Arm parameter hashes | `a0` `ab0c4035780d`; `two-point` `3184a2e6c676`; `hygiene` `eed1fb98567a`; `robust` `0fc4dbd7c131`; `face-state` `80e04c6ea08e`; `near-edge` `fa4f7b0d0bea`; `box-extent` `7c8a88e85dc1`; `shared-orientation` `67c2f2ae8ed2`; `join-membership` `f17eae8f1ac6`; `combined` `da955b128e3f` (each folds the replay experiments into the tuning hash)                             |
| Replay               | `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case, every arm repeat-verified; `lidar-box-eval -variants none -rows -warmup 6 -duration 0 -require-settled=false` with an evidence database                                                                                            |
| Capture              | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`, equal to the repository's LFS pointer                                                                                                                                                                                                                                                         |
| Labels               | label set `kirk0-reference` revision 2233; pack `ds_67842cb1b22b70e7` reconstructed (pack digest `sha256:f0ed3414…`, packets `3ffeba76…`, returns `f0f9ca27…`, points `7860a86c…`); physical references revision 77, content `sha256:189ecf65…`, re-pinned copy `fef9f97f…`; dataset `banshee-data/sf-street-speeds`                                                            |
| Split                | drafted, `kirk0-tuning`, episode `kirk0-tuning-from-61` (samples 61 to 831), `obj_788622b5` ignored, manifest `7194bb61…`, reference digest `sha256:a784e69d…`                                                                                                                                                                                                                  |
| Placements           | `20260901T1225_80858-0ac_ashbury-downey` and `20260901T1259_80858-0ac_haight-divisadero`: not fetched; `hf download` stalled on `cas-server.xethub.hf.co` (403) and the resolve URL's redirect to `us.aws.cdn.hf.co` was refused (403)                                                                                                                                          |
| Eligibility          | committed cohort manifest; a scratch use ledger at migration 71; actor `appius-l5id0019`                                                                                                                                                                                                                                                                                        |
| Host                 | Claude Code cloud container, 4 cores, 15 GB; raw output in the session scratch, not kept                                                                                                                                                                                                                                                                                        |
