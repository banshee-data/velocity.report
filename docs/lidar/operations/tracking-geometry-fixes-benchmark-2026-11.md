# The November tracking and geometry fixes, benchmarked on three captures: what they do and what comes next

<!-- ignore-style-length -->

Seven default-off options landed between November 6 and 8. Each was screened on its own question,
by its own author, with its own tools. This run reads them all with the same tools on the same
captures: identity on kirk0's reviewed labels, geometry on geom0015's 22 reviewed poses, and the
label-free noise, duplicate, count and cost measures on kirk0 and on the first two five-minute
segments of ashbury-downey and haight-divisadero. From those tables it ranks the next
background-noise (L3, L4) and tracking (L5) fixes. It answers
[l5id0019](../../../data/experiments/l5id/l5id0019.md).

- **Status:** Measured. kirk0 on 2026-11-08; the two placements on 2026-11-09, after the dataset's
  CDN was opened to the session. Nothing adopted, no default changed
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

- **The placements were read on November 9.** On November 8 the dataset's file CDN
  (`cas-server.xethub.hf.co`, `us.aws.cdn.hf.co`) refused this host with 403, and the first
  version of this report was kirk0's alone. Once the CDN was allowed, both placements were fetched
  one at a time and their first two five-minute segments read under the same rule, tools and
  builds. The label-free tables now cover five cases on three files.
- **The robust likelihood cost is the one large identity lever, and its costs hold off kirk0.** On
  kirk0 it takes identity switches from 90 to 6 and fragmentations from 46 to 30. All four paired
  intervals keep their sign: HOTA +0.0115 (+0.0023 to +0.0222), IDF1 +0.0166 (+0.0032 to
  +0.0338), switches −80 (−226 to −4), fragmentations −16 (−42 to −2). Off kirk0 its duplicate fall
  is larger: pooled over the five cases, duplicate pairs halve (26 to 13) and pair-frames fall 62 %
  (901 to 338). Its cost holds too: brief tracks +13 and clutter +12 pooled, clutter over R4's
  bound on four of five cases. No brief track is a re-birth on a live body in any case. The rule
  does not name it for a held-out read.
- **Two-point initialisation inflates the published speeds.** In the scored-window rows it raises
  p98 by 5 to 18 m/s at every case, on ashbury-downey's first segment from 11.2 to 28.9 m/s. Six
  tracks there carry 191 rows over 15 m/s, against none under `a0`, and 130 of those rows come
  after the track's first second. The evidence rows move p85 by +2.08 m/s on ashbury-downey's
  second segment. On kirk0 it was already not non-inferior on switches. **Moved: from a low-rank
  restatement to the first tracking fix, as a defect to keep off any default path.**
- **The near-edge tracked filter loses identity, and its duplicates rise everywhere.** On kirk0 HOTA
  −0.061 (−0.116 to −0.021) and IDF1 −0.106 (−0.164 to −0.013), with 186 more missed reference
  points. Duplicate pairs rise at all five cases (pooled 26 to 87) and brief tracks by 160. L3 and
  L4 give it the same clusters, so the loss is in association and lifecycle.
- **Track-conditioned joining's spurious-cluster drop holds.** It lowers spurious clusters a frame
  at all five cases: −6.9 % on kirk0, −4.9 and −1.5 % at ashbury-downey, −0.9 and −14.4 % at
  haight-divisadero; −6.2 % pooled. Duplicate pairs 26 to 22, and it costs noise on no case.
- **Box extents' width effect does not hold off kirk0.** On kirk0 the option widens moving vehicles'
  published boxes (median 1.91 to 2.02 m, rows over 2.2 m 36 to 45 %). On the four segments it
  moves the median by 0.02 m at most. The shared orientation, which the rule did not watch for
  width, widens boxes at every placement (pooled rows over 2.2 m 24 to 32 %) and raises duplicate
  pair-frames 36 %.
- **The background noise is not recurrent.** The D14 grid on the 120 s segments finds 0 to 2
  recurring cells per case, holding at most 15 % of the unowned clusters. Small artefacts are the
  largest clutter class at every case (54 of 93 clutter tracks pooled).
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
- **Two no-ops, at every case.** Covariance hygiene and the face-state shadow are byte-identical to
  `a0` on kirk0 and leave every label-free figure of the four segments unchanged. The shared
  orientation changes kirk0's identity by at most two switches and moves its box by 0.01 m, though
  it halves the shadow's side-face placement error. Off kirk0 it is not a no-op: duplicates rise
  at both haight-divisadero segments.
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
- **Box replays.** On kirk0,
  `lidar-box-eval -variants none -rows -require-settled=false -warmup 6 -duration 0`, the rest of
  the capture scored (771 frames). Each had an evidence database, so the solid body has its points.
  They ran after the timed replays, beside the scorers.
- **The placements.** `hf download` fetched each placement whole, one at a time:
  `20260901T1225_80858-0ac_ashbury-downey` (2,960,318,212 bytes) and
  `20260901T1259_80858-0ac_haight-divisadero` (2,785,665,108 bytes). Each was cut into its first
  two five-minute segments, 0 to 300 s and 300 to 600 s, and the placement deleted once the
  segments existed. Each segment ran every arm through `lidar-box-eval` with
  `-warmup 40 -duration 120`, `-variants none -rows -require-settled=false` and
  `-sample-points 1024`, with an evidence database, as l5id0012 ran segments. The four scripts read
  each arm's database and `a0`'s, which were deleted once their summaries were written. The
  segments were deleted once all arms had run.
- **How the segments were cut.** `pcap-split --start-seconds S --duration-seconds 300` analyses
  the window but writes the whole capture. Its writer routes every packet outside the analysed
  segments to the first or last segment (`internal/lidar/pcapsplit/writer.go`, by design for
  whole-capture splits), so both windows came out at 2.92 GB, 2,209,146 packets. Those files were
  deleted unread. The segments were cut instead by a scratch pcapng filter: header blocks
  copied, enhanced packets kept whose timestamp lies in the window from the first packet. Each
  cut's UDP 2369 packet count equals `pcap-split --dry-run`'s count for the same window: 540,018
  and 540,019 at ashbury-downey, 540,019 and 540,018 at haight-divisadero.
- **Two windows in the label-free read.** The rule names "each arm's evidence". The evidence
  database spans the replay after its first few seconds, about 154 s of a segment, warm-up
  included. The rows CSV spans only the scored 120 s. The tables read the evidence, as the rule
  says. `lidar_flip_cost.py` and `lidar_figure_sensitivity.py` were also run on the rows, as
  l5id0012 read segments, and reported where the two differ.
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

## Label-free: noise, duplicates and counts on three files

Five cases: kirk0 whole (6 s warm-up), and the first and second five-minute segments of
ashbury-downey and haight-divisadero (40 s warm-up, 120 s scored), written "ashbury 1", "ashbury 2",
"haight 1" and "haight 2". kirk0's arm-by-arm table comes first, then the four segments.

### kirk0

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
unowned clusters do not return to one place. The segments' grids are below. Under `near-edge`
the unowned count rises eightfold with the clusters a frame unchanged.
Ownership is a lasting track's row within 1 m of the cluster's centroid, and a body-centre track
sits metres from a truck's centroid, so most of that rise is the test, not new foreground.

**kirk0 against R4** (each arm less `a0`):

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

### The segments: `a0`

| Case      | Frames | Tracks | Brief | Near-static | Clutter (artefact / stationary / fragment / unclassed) | Spurious a frame | Unowned | Recurring cells (beside a path / background) | Unowned in recurring cells | Moving | p85, p98 (m/s) | Vehicle tracks | Dup pairs (per 100) | Pair-frames |
| --------- | -----: | -----: | ----: | ----------: | ------------------------------------------------------ | ---------------: | ------: | -------------------------------------------- | -------------------------: | -----: | -------------- | -------------: | ------------------- | ----------: |
| kirk0     |    773 |     79 |    12 |           3 | 15 (6 / 2 / 3 / 4)                                     |            0.695 |     537 | 0 (0 / 0)                                    |                        0 % |     38 | 7.06, 8.83     |             44 | 10 (22.7)           |         452 |
| ashbury 1 |  1,131 |     46 |     8 |          10 | 18 (12 / 1 / 1 / 4)                                    |            0.412 |     466 | 2 (0 / 2)                                    |                       15 % |     21 | 10.89, 11.58   |             24 | 1 (4.2)             |          86 |
| ashbury 2 |  1,343 |     39 |     7 |           6 | 13 (10 / 0 / 0 / 3)                                    |            0.383 |     514 | 1 (0 / 1)                                    |                        5 % |     18 | 10.84, 12.91   |             18 | 0 (0.0)             |           0 |
| haight 1  |  1,541 |     70 |    12 |           9 | 21 (12 / 3 / 3 / 3)                                    |            0.287 |     440 | 2 (2 / 0)                                    |                        6 % |     33 | 9.31, 10.38    |             29 | 4 (13.8)            |          52 |
| haight 2  |  1,538 |     79 |    18 |           8 | 26 (14 / 3 / 1 / 8)                                    |            0.393 |     605 | 0 (0 / 0)                                    |                        0 % |     28 | 9.65, 10.77    |             28 | 11 (39.3)           |         311 |

Frames are those with observations in the evidence, which spans the warm-up. The figures from the
evidence are tracks, brief, near-static, clutter, spurious, unowned, recurrence, moving and the
speeds. The duplicates come from `lidar-box-eval`'s scored window.

**The D14 grid on the segments.** Each segment's evidence spans about 154 s, long enough for a cell
to recur in 10 distinct seconds. Under `a0` 0 to 2 cells recur per case, three of five in the
background and two beside a path, and they hold 0 to 15 % of the case's unowned clusters. The
largest is ashbury-downey's first segment: two background cells holding 71 of its 466 unowned
clusters. Every arm but `near-edge` reads within two cells of `a0`'s grid; `near-edge` reads 2 to
10 cells, the ownership test again. So on these sites 85 % or more of the unowned clusters do not
return to one place: they are one-offs or fragments, not a surface the background failed to learn.

### R4 on every case

Each cell: brief tracks Δ, clutter Δ per hundred `a0` moving tracks, duplicate pairs `a0`→arm,
spurious clusters a frame Δ, then what R4 reads. "count" is the moving count over 5 %, "p85" a
p85 shift over 0.5 m/s.

| Arm                  | kirk0                                                       | ashbury 1                                                         | ashbury 2                                                   | haight 1                                                   | haight 2                                                          | Cases costing noise | Cases costing counts |
| -------------------- | ----------------------------------------------------------- | ----------------------------------------------------------------- | ----------------------------------------------------------- | ---------------------------------------------------------- | ----------------------------------------------------------------- | ------------------: | -------------------: |
| `two-point`          | +1 / +2.6 / 10→12 / −4 %; **dups, count**                   | −1 / −19.0 / 1→2 / −16 %; **dups**                                | 0 / −11.1 / 0→0 / −41 %; **p85**                            | +8 / +15.2 / 4→6 / +6 %; **brief, clutter, dups, count**   | 0 / −7.1 / 11→5 / −2 %; **count**                                 |              3 of 5 |               4 of 5 |
| `hygiene`            | 0 / 0 / 10→10 / 0 %; —                                      | 0 / 0 / 1→1 / 0 %; —                                              | 0 / 0 / 0→0 / 0 %; —                                        | 0 / 0 / 4→4 / 0 %; —                                       | 0 / 0 / 11→11 / 0 %; —                                            |              0 of 5 |               0 of 5 |
| `robust`             | +4 / +10.5 / 10→8 / −1 %; **brief, clutter, count**         | +3 / +14.3 / 1→0 / −11 %; **clutter**                             | 0 / 0 / 0→0 / −41 %; **count**                              | +3 / +6.1 / 4→3 / +2 %; **clutter**                        | +3 / +10.7 / 11→2 / +4 %; **clutter, count**                      |              4 of 5 |               3 of 5 |
| `face-state`         | 0 / 0 / 10→10 / 0 %; —                                      | 0 / 0 / 1→1 / 0 %; —                                              | 0 / 0 / 0→0 / 0 %; —                                        | 0 / 0 / 4→4 / 0 %; —                                       | 0 / 0 / 11→11 / 0 %; —                                            |              0 of 5 |               0 of 5 |
| `near-edge`          | +9 / +26.3 / 10→23 / +47 %; **brief, clutter, dups, count** | +59 / +257.1 / 1→8 / +138 %; **brief, clutter, dups, count, p85** | +39 / +211.1 / 0→3 / +47 %; **brief, clutter, dups, count** | +28 / +75.8 / 4→12 / +100 %; **brief, clutter, dups, p85** | +25 / +82.1 / 11→41 / +62 %; **brief, clutter, dups, count, p85** |              5 of 5 |               5 of 5 |
| `box-extent`         | 0 / 0 / 10→9 / +1 %; —                                      | 0 / 0 / 1→1 / 0 %; —                                              | 0 / 0 / 0→0 / 0 %; —                                        | 0 / 0 / 4→4 / 0 %; —                                       | +1 / +3.6 / 11→7 / −3 %; —                                        |              0 of 5 |               0 of 5 |
| `shared-orientation` | 0 / 0 / 10→9 / +2 %; —                                      | 0 / 0 / 1→1 / 0 %; —                                              | 0 / 0 / 0→0 / 0 %; —                                        | 0 / 0 / 4→5 / 0 %; **dups**                                | +1 / +3.6 / 11→14 / +3 %; **dups**                                |              2 of 5 |               0 of 5 |
| `join-membership`    | −1 / −2.6 / 10→9 / −7 %; **count**                          | 0 / 0 / 1→1 / −5 %; —                                             | 0 / 0 / 0→0 / −2 %; —                                       | 0 / 0 / 4→4 / −1 %; —                                      | 0 / 0 / 11→8 / −14 %; **count**                                   |              0 of 5 |               2 of 5 |
| `combined`           | +3 / +7.9 / 10→5 / −6 %; **clutter, count**                 | −1 / −19.0 / 1→0 / −22 %; **count, p85**                          | −1 / −16.7 / 0→0 / −45 %; **p85**                           | +11 / +27.3 / 4→4 / +7 %; **brief, clutter, count**        | +1 / −3.6 / 11→4 / −6 %; —                                        |              2 of 5 |               4 of 5 |

### Per site and pooled

Sums over each group's cases; spurious clusters a frame weighted by frames.

| Group             | Arm                  | Brief      | Clutter    | Artefacts | Moving (lost / short / added) | Dup pairs (per 100) | Pair-frames | Spurious a frame |
| ----------------- | -------------------- | ---------- | ---------- | --------: | ----------------------------- | ------------------- | ----------: | ---------------- |
| ashbury-downey    | `a0`                 | 15         | 31         |        22 | 39                            | 1 (2.4)             |          86 | 0.396            |
|                   | `two-point`          | 14 (−1)    | 25 (−6)    |        16 | 39 (0 / 0 / 2)                | 2 (4.5)             |          30 | 0.281 (−28.9 %)  |
|                   | `robust`             | 18 (+3)    | 34 (+3)    |        22 | 37 (0 / 0 / 1)                | 0 (0.0)             |           6 | 0.292 (−26.3 %)  |
|                   | `near-edge`          | 113 (+98)  | 123 (+92)  |        17 | 45 (0 / 11 / 0)               | 11 (7.1)            |         152 | 0.753 (+90.1 %)  |
|                   | `box-extent`         | 15 (0)     | 31 (0)     |        22 | 39 (0 / 0 / 0)                | 1 (2.4)             |          86 | 0.396 (0.0 %)    |
|                   | `shared-orientation` | 15 (0)     | 31 (0)     |        22 | 39 (0 / 0 / 0)                | 1 (2.4)             |          85 | 0.396 (0.0 %)    |
|                   | `join-membership`    | 15 (0)     | 31 (0)     |        22 | 39 (0 / 0 / 0)                | 1 (2.4)             |          85 | 0.384 (−3.2 %)   |
|                   | `combined`           | 13 (−2)    | 24 (−7)    |        16 | 37 (0 / 0 / 2)                | 0 (0.0)             |           0 | 0.263 (−33.7 %)  |
| haight-divisadero | `a0`                 | 30         | 47         |        26 | 61                            | 15 (26.3)           |         363 | 0.340            |
|                   | `two-point`          | 38 (+8)    | 50 (+3)    |        34 | 61 (1 / 0 / 0)                | 11 (20.0)           |         342 | 0.344 (+1.2 %)   |
|                   | `robust`             | 36 (+6)    | 52 (+5)    |        32 | 58 (0 / 0 / 0)                | 5 (10.4)            |          86 | 0.351 (+3.2 %)   |
|                   | `near-edge`          | 83 (+53)   | 95 (+48)   |        22 | 58 (1 / 23 / 1)               | 53 (42.4)           |         921 | 0.605 (+78.0 %)  |
|                   | `box-extent`         | 31 (+1)    | 48 (+1)    |        27 | 61 (0 / 0 / 0)                | 11 (20.8)           |         298 | 0.334 (−1.8 %)   |
|                   | `shared-orientation` | 31 (+1)    | 48 (+1)    |        26 | 60 (0 / 0 / 0)                | 19 (33.3)           |         610 | 0.347 (+2.0 %)   |
|                   | `join-membership`    | 30 (0)     | 47 (0)     |        26 | 59 (0 / 0 / 0)                | 12 (22.2)           |         287 | 0.311 (−8.7 %)   |
|                   | `combined`           | 42 (+12)   | 55 (+8)    |        35 | 57 (0 / 1 / 0)                | 8 (15.1)            |         185 | 0.338 (−0.5 %)   |
| pooled, 3 files   | `a0`                 | 57         | 93         |        54 | 138                           | 26 (18.2)           |         901 | 0.405            |
|                   | `two-point`          | 65 (+8)    | 91 (−2)    |        56 | 140 (1 / 0 / 2)               | 25 (17.4)           |         882 | 0.359 (−11.5 %)  |
|                   | `robust`             | 70 (+13)   | 105 (+12)  |        62 | 136 (0 / 0 / 1)               | 13 (9.8)            |         338 | 0.369 (−9.0 %)   |
|                   | `near-edge`          | 217 (+160) | 243 (+150) |        47 | 144 (1 / 37 / 1)              | 87 (25.1)           |       1,498 | 0.714 (+76.2 %)  |
|                   | `box-extent`         | 58 (+1)    | 94 (+1)    |        55 | 138 (0 / 0 / 0)               | 21 (15.0)           |         829 | 0.403 (−0.5 %)   |
|                   | `shared-orientation` | 58 (+1)    | 94 (+1)    |        54 | 137 (0 / 0 / 0)               | 29 (20.3)           |       1,222 | 0.411 (+1.3 %)   |
|                   | `join-membership`    | 56 (−1)    | 92 (−1)    |        55 | 134 (0 / 0 / 0)               | 22 (15.7)           |         788 | 0.380 (−6.2 %)   |
|                   | `combined`           | 70 (+13)   | 97 (+4)    |        61 | 134 (0 / 1 / 3)               | 13 (9.5)            |         410 | 0.347 (−14.4 %)  |

`hygiene` and `face-state` equal `a0` in every group and are left out. No brief track is a re-birth
on a live body in any arm at any case, `near-edge` included: the brief-origin split is 0 re-births,
0 to 2 splits and the rest "other" everywhere.

### Speeds: the evidence and the scored rows

p85 and p98 of moving tracks' maximum speeds (m/s; `a0`'s values, then each arm's shift). The
evidence rows carry the filter's speed at observed frames over the whole replay. The box-eval rows
carry every scored frame of every confirmed track, coasting included.

| Arm               | Read     | kirk0             | ashbury 1           | ashbury 2           | haight 1           | haight 2           |
| ----------------- | -------- | ----------------- | ------------------- | ------------------- | ------------------ | ------------------ |
| `a0`              | evidence | 7.06, 8.83 (n 38) | 10.89, 11.58 (n 21) | 10.84, 12.91 (n 18) | 9.31, 10.38 (n 33) | 9.65, 10.77 (n 28) |
| `a0`              | rows     | 6.98, 8.15 (n 51) | 10.72, 11.20 (n 24) | 10.68, 12.91 (n 21) | 9.60, 11.26 (n 36) | 9.66, 11.06 (n 34) |
| `two-point`       | evidence | −0.08, +0.20      | +0.45, +0.08        | +2.08, +2.15        | 0.00, 0.00         | +0.12, +1.13       |
| `two-point`       | rows     | +0.46, +7.00      | +5.78, +17.74       | +4.38, +14.58       | +0.47, +6.71       | +0.72, +5.12       |
| `robust`          | evidence | −0.07, 0.00       | +0.01, 0.00         | 0.00, 0.00          | 0.00, 0.00         | 0.00, 0.00         |
| `robust`          | rows     | 0.00, 0.00        | +0.17, −0.01        | 0.00, 0.00          | +0.43, −0.01       | +0.01, 0.00        |
| `near-edge`       | evidence | 0.00, −0.39       | −0.79, −0.48        | −0.28, −0.89        | −2.17, −1.56       | −1.67, −2.33       |
| `near-edge`       | rows     | −1.01, +0.57      | −2.90, −0.19        | −4.22, −1.62        | −3.36, −2.31       | −4.64, −2.26       |
| `join-membership` | evidence | 0.00, 0.00        | 0.00, +0.02         | 0.00, 0.00          | 0.00, 0.00         | +0.11, 0.00        |
| `join-membership` | rows     | +0.02, 0.00       | −0.06, 0.00         | 0.00, 0.00          | 0.00, −0.01        | +0.34, +0.90       |
| `combined`        | evidence | −0.08, +0.20      | +0.70, +0.08        | +2.08, +2.15        | 0.00, 0.00         | +0.12, +1.13       |
| `combined`        | rows     | +0.81, +7.36      | +5.78, +17.74       | +4.38, +14.58       | +0.47, +2.89       | +1.41, +5.12       |

Where the two-point speeds sit: on ashbury-downey's first segment, six two-point tracks carry 191
scored rows over 15 m/s, the fastest at 28.9 m/s, against no row over 11.8 m/s under `a0` or
`robust`. 130 of the 191 lie after the track's first second, so the wrong velocity outlasts
initialisation. At haight-divisadero's second segment it is one track and 33 rows, up to 16.3 m/s.
R4 reads the evidence. The rows show the effect is larger in what a scene export would publish,
since its p85 and p98 read each track's maximum instant speed.

### Box width off kirk0

Moving vehicle-sized rows of the scored window (published length 3 m or more, speed 1 m/s or
more): median published width, the share wider than 2.2 m, median length, and the median of the
width less the frame's observed span across the box.

| Case      | `a0`                      | `box-extent`              | `shared-orientation`      | `combined`                |
| --------- | ------------------------- | ------------------------- | ------------------------- | ------------------------- |
| kirk0     | 1.91 m, 36 %, 4.14; +0.08 | 2.02 m, 45 %, 4.12; +0.13 | 2.04 m, 44 %, 4.10; +0.14 | 2.05 m, 44 %, 4.15; +0.09 |
| ashbury 1 | 1.89 m, 12 %, 4.37; +0.15 | 1.89 m, 13 %, 4.30; +0.16 | 1.91 m, 20 %, 4.04; +0.22 | 1.91 m, 19 %, 4.04; +0.16 |
| ashbury 2 | 1.83 m, 11 %, 4.18; +0.08 | 1.83 m, 11 %, 4.17; +0.08 | 1.89 m, 26 %, 4.02; +0.16 | 1.85 m, 18 %, 4.00; +0.11 |
| haight 1  | 1.81 m, 19 %, 4.18; +0.16 | 1.80 m, 19 %, 4.17; +0.15 | 1.82 m, 24 %, 4.13; +0.28 | 1.86 m, 17 %, 4.12; +0.10 |
| haight 2  | 1.88 m, 25 %, 4.33; +0.08 | 1.90 m, 25 %, 4.33; +0.09 | 1.93 m, 31 %, 4.26; +0.10 | 1.90 m, 28 %, 4.23; +0.13 |
| pooled    | 1.87 m, 24 %, 4.21; +0.10 | 1.89 m, 27 %, 4.20; +0.13 | 1.95 m, 32 %, 4.07; +0.17 | 1.91 m, 29 %, 4.09; +0.12 |

There are no physical references off kirk0, so this is what the box drew, not how wide the
vehicles were.

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

R1 to R3 and R5 are kirk0's. R4 is read on each of the five cases: the column gives how many cost
noise and counts, and what.

| Arm                  | R1                                   | R2    | R3                    | R4, cases costing noise                            | R4, cases costing counts               | R5      | R6                  |
| -------------------- | ------------------------------------ | ----- | --------------------- | -------------------------------------------------- | -------------------------------------- | ------- | ------------------- |
| `two-point`          | No change; not non-inferior          |       | No move               | 3 of 5 (duplicates; brief and clutter at haight 1) | 4 of 5 (count; p85 +2.08 at ashbury 2) |         | Not named           |
| `hygiene`            | No change                            | No-op | No-op                 | 0 of 5                                             | 0 of 5                                 |         | Not named: a no-op  |
| `robust`             | Gains on all four; non-inferior      |       | No move               | 4 of 5 (clutter at four; brief at kirk0)           | 3 of 5 (count)                         |         | Not named: R4 noise |
| `face-state`         | No change                            | No-op | No-op (box)           | 0 of 5                                             | 0 of 5                                 | Flagged | Not named: a no-op  |
| `near-edge`          | Loses HOTA, IDF1                     |       | Truck 2 toward        | 5 of 5 (brief, clutter, duplicates)                | 5 of 5 (count, p85)                    | Flagged | Not named: a loss   |
| `box-extent`         | Gains HOTA, IDF1; non-inferior       |       | Away (overall, car 8) | 0 of 5                                             | 0 of 5                                 | Flagged | **Named**           |
| `shared-orientation` | No change                            |       | No move               | 2 of 5 (duplicates at both haight)                 | 0 of 5                                 | Flagged | Not named           |
| `join-membership`    | Gains IDF1; non-inferior             |       | No move               | 0 of 5                                             | 2 of 5 (count, −2 tracks)              |         | **Named**           |
| `combined`           | Gains HOTA, IDF1, frag; non-inferior |       | No move               | 2 of 5 (clutter; brief at haight 1)                | 4 of 5 (count, p85)                    | Flagged | Not named: R4 noise |

The five cases leave every R6 verdict as kirk0 alone gave it: `box-extent` and `join-membership`
cost noise on no case. R6 reads noise, not counts, so `join-membership`'s two fewer moving
tracks at two cases do not exclude it. R6 names `box-extent` on a gain of 0.0016 HOTA while R3
reads its box moving away from the references on kirk0. The rule did not foresee that, and the
verdict stands as written. The rule's count clause read on the evidence does not see the size of
two-point's speed defect, which the scored rows show (above). The recommendations below say what
to do with each.

## Does each kirk0 reading hold off kirk0?

| kirk0 reading                                                       | Off kirk0 (the four segments)                                                                                                                            | Holds?                                                                       |
| ------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| `robust` raises brief tracks (+4) and clutter (+10.5 per hundred)   | Brief +3, 0, +3, +3; clutter per hundred +14.3, 0, +6.1, +10.7. Pooled over five cases brief +13, clutter +12, artefacts 54 to 62. No re-births anywhere | Yes: clutter over its bound at three of four; brief at the +3 bound at three |
| `robust` halves duplicate pair-frames                               | Pair-frames 86 to 6, 0 to 0, 52 to 28, 311 to 58; pooled 901 to 338 (−62 %), pairs 26 to 13                                                              | Yes, and larger                                                              |
| `join-membership` lowers spurious clusters (−6.9 %)                 | −4.9, −1.5, −0.9, −14.4 %; pooled −6.2 %; duplicates 26 to 22 pooled; no noise cost at any case                                                          | Yes, at every case                                                           |
| `near-edge` raises duplicates (10 to 23)                            | 1 to 8, 0 to 3, 4 to 12, 11 to 41; brief +59, +39, +28, +25; p85 −0.3 to −2.2 m/s                                                                        | Yes, everywhere                                                              |
| `box-extent` widens published vehicle boxes (car 8 to 2.3 to 2.6 m) | Median width moves by −0.01 to +0.02 m; rows over 2.2 m within 1 point; duplicates 4 to 4 and 11 to 7 at haight, none elsewhere                          | No: a kirk0 effect                                                           |
| `two-point` costs switches and duplicates                           | Duplicates 1 to 2, 0 to 0, 4 to 6, 11 to 5. Scored-row p98 +17.7, +14.6, +6.7, +5.1 m/s (kirk0 +7.0, which the first report did not read)                | Worse: a speed defect                                                        |
| `shared-orientation` is a near-no-op on what is published           | Duplicates 4 to 5 and 11 to 14 at haight, pair-frames 52 to 71 and 311 to 539; published width rows over 2.2 m 24 to 32 % pooled                         | No: costs duplicates                                                         |
| `hygiene` and the `face-state` shadow are no-ops                    | Every figure equal to `a0`'s at every segment                                                                                                            | Yes                                                                          |

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
unclassed (4 to 7). Off kirk0 the same: no brief track is a re-birth at any segment, and pooled
artefacts rise from 54 to 62. Inferred: clusters that a coasting track absorbed under `d²` are now
left unassigned, and some confirm as tracks of under a second. The birth gate on the live body
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

**Two-point initialisation gives tracks wrong speeds, and keeps them.** Measured on kirk0: 30 more
switches at the point estimate, an interval reaching +116, and two more duplicate pairs, with 15
fewer fragmentations. Measured off kirk0: scored-row p98 up 5 to 18 m/s at every case and up
7.0 m/s on kirk0. At ashbury-downey's first segment, six tracks run 191 rows over 15 m/s, up to
28.9 m/s on a street whose `a0` maximum is 11.8 m/s, and 130 of those rows come after the track's
first second. Inferred: the pair's velocity has a variance of `2R/Δt²`, about 10 m²/s² per axis.
When the first two clusters come from different parts of a body, or one is a neighbour's, the
track starts with a large, wrong velocity. The young track's gate then follows clusters that fit
that velocity, so the error is not corrected within a second. The maths review predicted a strict
improvement; on these captures the option is a defect in a published figure.

**Background noise is small, not recurrent, and mostly small artefacts.** Measured: kirk0 shows
9.09 clusters a frame, 0.69 of them spurious; the segments 0.29 to 0.41. The D14 grid on the
154 s segments finds 0 to 2 recurring cells per case, holding at most 15 % of the unowned
clusters, so the rest are one-offs or fragments. Small artefacts away from any mover are the
largest clutter class at every case: 54 of 93 pooled. Near-static tracks, lasting tracks on
something that does not move, are 36 pooled, of which 9 are stationary road users. A lasting
track owns its clusters, so these never reach the recurrence grid. Cluster counts move by at most
1 % on kirk0 across every arm, `near-edge` included, so the L3 track guard's feedback on what L4
sees is small. The one arm that lowers spurious clusters at every case without a noise cost is
the join, −6.2 % pooled. Robust and two-point lower them more where traffic is light (ashbury
−26 % and −29 %), but at a brief-track or speed cost.

## The next fixes, ranked

The ranking is by the size of the measured defect in what a user sees, then what the fix unblocks.
Each fix states its evidence, expected effect, what it builds on, and what would falsify it. The
November 9 read of the two placements changed this ranking, and each entry says whether it moved
and why.

### Tracking (L5)

1. **Keep two-point initialisation off, and restate it before any further read.** **Moved: up from
   fifth.** The placements show it inflating published speeds, which the kirk0 read did not see.
   - Evidence: the speed table. Scored-row p98 up 5 to 18 m/s at every case (kirk0 +7.0). At
     ashbury-downey's first segment, 191 rows over 15 m/s on six tracks, up to 28.9 m/s, where
     `a0` has none over 11.8 m/s; 130 of them after the first second. On kirk0, switches +30
     (interval to +116) and duplicates +2.
   - Expected effect (inferred): initialise from the class's speed range (the review's
     `(v_max/2)²` variance) or a least-squares fit over the tentative frames, and refuse a pair
     whose implied speed is outside the class's range. The scored rows' p98 should then stay within
     0.5 m/s of `a0`'s while fragmentations keep kirk0's −15.
   - Builds on: `two_point_velocity_init` (#969). It is default-off and on no path; the defect is
     in what any adoption would publish.
   - Falsified if: the variant still writes rows over 15 m/s where `a0` writes none. The cause
     would then be the young track's gating, not the initial velocity.
2. **A birth rule for robust's released clusters, then robust on the corpus.** **Moved: down one,
   to make room for the speed defect; the evidence for it is stronger.**
   - Evidence: kirk0's identity rows (switches 90 to 6, all four intervals keep their sign) and the
     five cases. Duplicate pairs halve pooled (26 to 13) and pair-frames fall 62 %. Brief tracks
     rise +13 pooled, clutter goes over its bound at four of five cases, and pooled artefacts rise
     from 54 to 62. All births are "other", never re-births.
   - Expected effect (inferred): a tentative track confirms only when its clusters' size and
     displacement over the tentative frames are a road user's. That should hold brief tracks
     within `a0`'s plus 3 at each case and keep kirk0's switches under 10.
   - Builds on: `robust_likelihood_cost` (#983). It would replace the overlap end as the duplicate
     lever; that one failed its corpus rule on lost real tracks (10 %).
   - Zero-code controls to run first: `robust` against plain `likelihood_cost` (refused together,
     so as a separate arm), and `robust` with the L3 `support_filter`.
   - Falsified if: the birth rule removes the brief tracks but brings the duplicates back.
3. **Restore length accumulation in the extent state and gate width bounds on the heading.**
   **Unchanged** (kirk0 physical evidence; the placements have no references).
   - Evidence: the solid-body table and the bisect. #989 took truck 2 from 6.24 to 4.60 m (centre
     0.81 to 2.66 m); widths read 2.45, 3.82 and, under the shared orientation, 6.20 m.
   - Expected effect (inferred): keep #989's span projection as a lower bound per frame. Take the
     censored posterior over the largest consistent span across frames along a converged axis
     (maths review §3.2), and refuse or widen a span bound whose axis variance exceeds a set
     bound. Truck 2 should then read at least the box's 6.59 m and car widths under 2.0 m.
   - Builds on: #970, #982 and #989.
   - Falsified if: truck 2 stays under 6.24 m, or car 8's solid-body centre error rises back over
     0.5 m.
4. **Fix the near-edge tracked gate before more geometry rides the tracked path.** **Unchanged;
   strengthened.** Its costs hold at every case.
   - Evidence: kirk0's HOTA −0.06 and IDF1 −0.11. At the five cases, duplicates rise everywhere
     (pooled 26 to 87), brief tracks +160 and p85 down by up to 2.2 m/s.
   - Expected effect (inferred): with fix 3's extents the face residual should gate truck clusters
     again. Run `near_edge_track_a1` (medoid gate) and `near-edge` with `robust_likelihood_cost`
     as zero-code arms first, to locate the loss.
   - Builds on: `near_edge_track` and its A1 ablation.
   - Falsified if: A1 shows the same loss; the gate is then not the cause.
5. **Take only evidenced length from the extent state into the box.** **Moved: down from third.**
   The width effect it would remove is a kirk0 effect; off kirk0 the option barely changes the box.
   - Evidence: on kirk0, car 8's box width goes from 1.8 to 2.3 to 2.6 m and its centre from 0.04
     to 0.29 m off. On the four segments the median published width moves by 0.02 m at most. No
     truck length moves anywhere it can be checked.
   - Expected effect (inferred): use `2hL` only where it exceeds the box's own belief with
     two-sided evidence, and never `2hW` while the far side is unseen. Its value then depends on
     fix 3: without a longer extent state there is nothing for it to carry.
   - Builds on: `box_extent_solid_body` (#984).
   - Falsified if: with fix 3 in, truck 2's box length still does not move past 6.59 m.
6. **Explain the shared orientation's duplicates before keeping it for heading.** **Changed: kirk0
   read it as a near-no-op; the placements give it a cost.**
   - Evidence: on kirk0, box yaw median 1.2° to 0.8° and side placement 0.42 to 0.15 m, against
     truck 1's solid-body width of 6.20 m. Off kirk0, duplicates rise at both haight-divisadero
     segments (4 to 5, 11 to 14; pair-frames 52 to 71, 311 to 539), and published width rows over
     2.2 m rise from 24 to 32 % pooled.
   - Expected effect (inferred): the shared heading changes the published box. A box that is
     wider, or turned, overlaps a neighbour's more, and that reads as a duplicate. Gating width
     bounds on the heading (fix 3) should remove both.
   - Falsified if: with fix 3 in, duplicates still rise at haight-divisadero.

### Background noise (L3, L4)

1. **Take track-conditioned joining to the corpus, with a merge reference.** **Unchanged; now held
   at every case.**
   - Evidence: spurious clusters a frame −6.9 % on kirk0, −4.9 and −1.5 % at ashbury-downey, −0.9
     and −14.4 % at haight-divisadero; −6.2 % pooled. Duplicate pairs 26 to 22 pooled, no noise
     cost at any case, an IDF1 gain on kirk0 that keeps its sign. It loses two moving tracks at two
     cases, with none lost by the footprint match.
   - Expected effect (measured on these cases): about 6 % fewer spurious clusters.
   - Builds on: #986. l4clus0010's fix (b), the solid body's extent in the footprint, should wait
     for tracking fix 3: today it would read truck 2 at 4.6 m.
   - Falsified if: the corpus shows any merge of two vehicles side by side.
2. **Remove small artefact tracks at birth or absorb them, not with a background filter.**
   **Changed: the D14 grid answered the open question.**
   - Evidence: the recurrence grid on the segments holds 0 to 15 % of unowned clusters, so most are
     one-offs, not a background surface. Small artefacts are 54 of 93 clutter tracks pooled.
     Near-static tracks are 36, of which 9 are stationary road users; being lasting tracks, they own
     their clusters and never reach the grid.
   - Expected effect (inferred): the brief artefacts are tracking fix 2's birth rule, which should
     cut them at every case. The near-static small ones are a dwell-type absorption, which exists
     default-off as `dwell_policy`, restricted to small boxes. The L3 `support_filter` can remove
     at most the recurring 15 %, so it ranks below both.
   - Falsified if: labels show the small near-static tracks are pedestrians or other road users,
     or the dwell policy removes a stationary road user.
3. **No L3 settling or region fix is ranked, and on these sites the room for one is small.**
   **Changed: from unread to bounded.**
   - Evidence: L4's input moved by at most 1 % in every arm on kirk0. The recurring background
     share is at most 15 % on the segments, and no region or settling option was an arm.
   - Next step: the l3bg0030 corpus read and the D14 grid on busier sites, where a learned surface
     that moves would show as recurring background cells.

### Held-out reads on label0003's window

label0003's window is embarcadero-folsom 575 to 755 s.

- **Worth a held-out read:** `join-membership`, with a merge reference (named by R6; its
  spurious-cluster drop holds at every case). `robust` once fix 2's birth rule holds its brief
  tracks: by the rule it is not named, and it is the strongest identity candidate this run
  measured.
- **Named by the rule, but not worth the window yet:** `box-extent`. Off kirk0 it barely changes
  the box, and on kirk0 it moves car 8's box 0.25 m away. Read it after fixes 3 and 5.
- **No-ops at every case:** `hygiene` (byte-identical; the cap is never reached) and the
  `face-state` shadow (byte-identical tracks and box; it costs Tracker.Update a factor of four).
- **Not before rework:** `two-point` (a speed defect), `near-edge` (a loss), `shared-orientation`
  (a duplicate cost off kirk0) and `combined`, which carries two-point's speeds.

## What this does not establish

- **Anything beyond three captures.** The label-free read is five cases on two placements and
  kirk0, 18 to 38 moving tracks each, so the 5 % count bound is one or two tracks per case and a
  p85 of 18 tracks is the 16th value. Both placements are quiet residential corners, and neither
  is a busy arterial.
- **Labelled truth off kirk0.** The placements carry no labels, so their lost, added and duplicate
  figures are against `a0`, not against what was on the road. Their box widths are what the box
  drew.
- **Read twice on the segments.** The rule's evidence read includes each segment's warm-up; the
  scored-window rows, which include coasting frames, were read as a sensitivity for the moving
  count and the speeds. Pooled moving counts agree within 2.4 % for six arms. They differ for
  three: `two-point` +12.7 % on the rows against +1.4 %, `combined` +9.0 % against −2.9 %, and
  `near-edge` +116 % against +4.3 %. Two-point's speed defect is far larger on the rows. Those
  three are the arms the ranking already puts behind rework.
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

| Item                 | Value                                                                                                                                                                                                                                                                                                                                                                                                        |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Base                 | main `fe6549b3` (`fe6549b39ef487d20693b17b2e48e5d1743878c0`); spec committed at `2d79801a`, which changes no tool source                                                                                                                                                                                                                                                                                     |
| Tools                | Built at `fe6549b3` with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA` the full sha) and the `pcap` tag; every replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `fe6549b3…`. `lidar-state-estimation-baseline` `sha256:8f73e500…`, `lidar-box-eval` `9f2b9445…`, `lidar-ground-truth-eval` `ee1fc25e…`, `lidar-near-face-eval` `befbf327…`                              |
| Bisect builds        | `50c7790b` (#984, before #989) and `fc9d8979` (#989), from `git archive`, the same LDFLAGS; the l4clus0010 session's face-state evidence at `99a9627` rescored                                                                                                                                                                                                                                               |
| Parameter sets       | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`; `config/experiments/motion-fragment-join.json`, fingerprint `719d291ed197`, file SHA-256 `2e604040…`                                                                                                                                                                                                                    |
| Arm parameter hashes | `a0` `ab0c4035780d`; `two-point` `3184a2e6c676`; `hygiene` `eed1fb98567a`; `robust` `0fc4dbd7c131`; `face-state` `80e04c6ea08e`; `near-edge` `fa4f7b0d0bea`; `box-extent` `7c8a88e85dc1`; `shared-orientation` `67c2f2ae8ed2`; `join-membership` `f17eae8f1ac6`; `combined` `da955b128e3f` (each folds the replay experiments into the tuning hash)                                                          |
| Replay               | `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case, every arm repeat-verified; `lidar-box-eval -variants none -rows -warmup 6 -duration 0 -require-settled=false` with an evidence database                                                                                                                         |
| Capture              | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`, equal to the repository's LFS pointer                                                                                                                                                                                                                                                                                      |
| Labels               | label set `kirk0-reference` revision 2233; pack `ds_67842cb1b22b70e7` reconstructed (pack digest `sha256:f0ed3414…`, packets `3ffeba76…`, returns `f0f9ca27…`, points `7860a86c…`); physical references revision 77, content `sha256:189ecf65…`, re-pinned copy `fef9f97f…`; dataset `banshee-data/sf-street-speeds`                                                                                         |
| Split                | drafted, `kirk0-tuning`, episode `kirk0-tuning-from-61` (samples 61 to 831), `obj_788622b5` ignored, manifest `7194bb61…`, reference digest `sha256:a784e69d…`                                                                                                                                                                                                                                               |
| Placements           | Refused on 2026-11-08 (403 from `cas-server.xethub.hf.co` and `us.aws.cdn.hf.co`); fetched on 2026-11-09 with `hf download`, one at a time. `20260901T1225_80858-0ac_ashbury-downey` `sha256:0f6811cd96c8a3ae…` (2,960,318,212 bytes, equal to the dataset's Xet hash); `20260901T1259_80858-0ac_haight-divisadero` `sha256:da4cb4a34ec4828a…` (2,785,665,108 bytes). Each deleted once its segments existed |
| Segments             | 0 to 300 s and 300 to 600 s of each placement, cut by a scratch pcapng window filter, with UDP 2369 counts equal to `pcap-split --dry-run`'s for the same window. ashbury-downey 1 `971e093e…` (540,018 packets), 2 `e953e352…` (540,019); haight-divisadero 1 `2afd5be4…` (540,019), 2 `5b4d6b8e…` (540,018). Each deleted once its arms' summaries were written                                            |
| Segment replays      | `lidar-box-eval` at `fe6549b3` (`9f2b9445…`), `-warmup 40 -duration 120 -variants none -rows -require-settled=false -sample-points 1024` with an evidence database, every arm, three lanes; the four scripts at `fe6549b3` on each arm's database against `a0`'s, and `lidar_flip_cost.py` and `lidar_figure_sensitivity.py` again on the rows                                                               |
| Eligibility          | committed cohort manifest; a scratch use ledger at migration 71; actor `appius-l5id0019`                                                                                                                                                                                                                                                                                                                     |
| Host                 | Claude Code cloud container, 4 cores, 15 GB; raw output in the session scratch, not kept                                                                                                                                                                                                                                                                                                                     |
