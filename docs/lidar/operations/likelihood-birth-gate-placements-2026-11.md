# The likelihood cost with the fragment birth gate, off kirk0

<!-- ignore-style-length -->

- **Status:** Measured on kirk0 and on two placements on 2026-11-10. Every arm passes K1. No arm
  passes K1 to K3, so K4 names no arm and K5 is not read. Nothing is adopted and no default changes
- **Spec:** [l5id0023](../../../data/experiments/l5id/l5id0023.md). Its rule (K1 to K5) was
  committed at `ab19e51`, before any replay
- **Option:** `robust_birth_gate` now accepts `likelihood_cost` as its prerequisite as well as
  `robust_likelihood_cost` (commit `9f04ae0`). The gate itself, `TrackerConfig.FragmentBirthGate`,
  is unchanged, default-off and bit-identical when off
- **Parameter set:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`. Every arm differs only by replay experiments
- **Related:** l5id0021's [birth gate report](robust-birth-gate-kirk0-2026-11.md), l5id0019's
  [November benchmark](tracking-geometry-fixes-benchmark-2026-11.md), and the
  [November 10 review](experiment-review-2026-11-10.md#the-next-seven-days)

## Summary

- **K1 holds for all three candidates on kirk0.** `robust-birth` and `likelihood-birth` give the
  same paired bootstrap against `a0`: HOTA 5th percentile +0.0032, IDF1 +0.0055, switches 90 to 6. `robust-birth-tentative` holds by a thread: HOTA 5th percentile +0.0001, IDF1 +0.0006, 7
  switches. `robust-birth` reproduces l5id0021's figures exactly.
- **On the placements the gate does not hold clutter for the robust cost.** `robust-birth` costs
  noise on three of the four segments (clutter +14.3, +6.1 and +10.7 per hundred of `a0`'s moving
  tracks), so it fails K2. `likelihood-birth` costs noise on one segment, haight-divisadero's
  second (clutter +14.3). `robust-birth-tentative` costs noise on one segment, by one duplicate
  pair at haight-divisadero's first. Both pass K2. No arm loses a lasting moving track by
  footprint on the evidence.
- **K3 fails every arm.** Each arm moves the moving count by more than 5 % on at least one
  segment, and on each such segment at least one of the tracks behind the move is not a duplicate
  removed. `likelihood-birth` fails at haight-divisadero's second segment alone: 28 to 26 moving
  tracks (−7.1 %). Two of the four tracks that make the move are duplicates removed. One is a
  slow walker that `a0` carried in two tracks and the candidate carries in one, whose median
  speed of 0.52 m/s puts it under the moving threshold. One is a track the candidate adds,
  5.8 m first to last where `a0`'s twin is 4.8 m.
- **So K4 names no arm and K5 is not read.** K5 could not have been read in any case: label0003's
  spans do not exist yet.
- **The duplicates fall a long way.** Pooled over the four segments, duplicate pair-frames fall
  from `a0`'s 449 to 92 (`robust-birth`), 154 (`likelihood-birth`) and 151
  (`robust-birth-tentative`). Pairs fall from 16 to 5, 6 and 10.
- **What decided it is the count, and the count's definition.** The counts that move are made by
  the identity gain itself: a vehicle or walker carried in one track rather than two. A joined
  track's median speed or first-to-last distance can cross the moving test's threshold where its
  pieces did not. K3 reads that as a count cost.

## Question and rule

On kirk0 the likelihood cost takes identity switches from 90 to 6, and the fragment birth gate
holds its brief tracks and clutter within `a0`'s bounds (l5id0021, in sample). Plain
`likelihood_cost` scores there as the Student-t `robust_likelihood_cost` does. Three things were
unread: whether the gate holds noise on captures it was not shaped on; whether the plain
likelihood with the gate is the simpler candidate; and whether tentative priority settles the
contest the robust cost leaves.

The rule, in short ([spec](../../../data/experiments/l5id/l5id0023.md)):

- **K1, the anchor.** Each candidate's paired kirk0 bootstrap against `a0` keeps a 5th percentile
  of the HOTA and IDF1 deltas above 0. An arm that fails is dropped.
- **K2, noise on the placements.** On each segment, under l5id0019's R4: brief tracks rise by 3 at
  most, clutter by 5 per hundred of `a0`'s moving tracks at most, and duplicate pairs per hundred
  vehicle tracks do not rise. An arm passes when it costs no noise on at least three of the four
  segments and loses no lasting moving track by footprint on any.
- **K3, counts.** On each segment the moving count moves by 5 % at most and p85 by 0.5 m/s at
  most. A larger move is reported with the tracks that made it, and fails the arm unless every
  one is a duplicate removed.
- **K4, which arm.** Of the arms passing K1 to K3, the simplest is named (`likelihood-birth`, then
  `robust-birth`, then `robust-birth-tentative`), unless a later arm has 20 % or more fewer pooled
  duplicate pair-frames.
- **K5, held out, read once.** label0003's spans, for the named arm beside `a0`.

## Arms

| Arm                      | Replay experiments                                                  | Parameter hash |
| ------------------------ | ------------------------------------------------------------------- | -------------- |
| `a0`                     | none                                                                | `ab0c4035780d` |
| `robust-birth`           | `robust_likelihood_cost`, `robust_birth_gate`                       | `e9db12b214d1` |
| `likelihood-birth`       | `likelihood_cost`, `robust_birth_gate`                              | `63e42b981b90` |
| `robust-birth-tentative` | `robust_likelihood_cost`, `robust_birth_gate`, `tentative_priority` | `7f233d7a2bb9` |

Every arm runs from `config/tuning.defaults.json` (fingerprint `ab0c4035780d`). The parameter
hash folds the replay experiments into the tuning hash.

## Method

- **The prerequisite.** `trackerConfigFor` refused `robust_birth_gate` without
  `robust_likelihood_cost`. It now accepts either likelihood cost and still refuses the gate under
  `d²` alone. `TestRobustBirthGateNeedsALikelihoodCost` covers the refusal, the gate under each
  cost, and the gate staying off without its name. No tuning key changes, so the defaults'
  fingerprint does not move.
- **kirk0 replays.** `lidar-state-estimation-baseline` on kirk0 whole as a one-case corpus (the
  committed `kirk0-corpus.json` and `kirk0-index.json`), with `-warmup 6 -require-settled=false
-sample-points 1024 -campaign-metrics`. Evidence per case, the committed cohort manifest, and a
  scratch use ledger at migration 71. Every arm's repeat pass reproduced its first pass's
  tracking baseline byte for byte.
- **kirk0 boxes.** `lidar-box-eval -variants none -rows -require-settled=false -warmup 6
-duration 0`, for duplicate pairs over 771 scored frames.
- **Identity.** `lidar-ground-truth-eval perframe` on the reconstructed pack, `kirk0-tuning`
  split with `-allow-tuning-split`, `obj_788622b5` ignored, online estimates as declared
  baselines. Each arm against `a0`, with `-bootstrap 2000 -bootstrap-seed 20261108`.
- **The placements.** The first two five-minute segments of ashbury-downey and
  haight-divisadero, cut as l5id0019 cut them, with equal hashes. Each arm ran through
  `lidar-box-eval -warmup 40 -duration 120 -variants none -rows -require-settled=false
-sample-points 1024` with an evidence database. On each arm's database:
  `lidar_noise_screen.py` and `lidar_clutter_origin.py` on their own, and
  `lidar_flip_cost.py --match footprint` and `lidar_figure_sensitivity.py` against `a0`'s.
  Duplicates come from `lidar-box-eval`'s summary.
- **Which window decides.** As in l5id0019, the tables read the evidence, which spans about 154 s
  of each segment, warm-up included. `lidar_flip_cost.py` and `lidar_figure_sensitivity.py` were
  also run on the rows, which span only the scored 120 s; that read is reported, not decided on.
- **The tracks behind a count move.** For each segment where a count moved by more than 5 %, the
  candidate was replayed again with its evidence kept. Its duplicates and figures equalled the
  first replay's. A scratch script built on `lidar_flip_cost.py`'s own loading and footprint
  coverage paired each of `a0`'s moving tracks with the candidate track that covers most of its
  rows. A track counts as a duplicate removed when another of `a0`'s tracks covers it on the same
  frames (`a0` carried the body twice) and the candidate carries the body once.
- **Order.** Every replay ran before K1 was read: two lanes, `GOMAXPROCS=2`, `nice -n 5`, on a
  host shared with two other experiments. No candidate's segment figures were read before every
  candidate had passed K1.

**The anchor.** `a0` equals the l5id README's anchor in every figure checked. Tracking baseline
`79d9abff…`, HOTA 0.4284, IDF1 0.4728, 90 switches, 46 fragmentations, 265 false negatives; 79
tracks, 12 brief, clutter 15, spurious 0.6947 a frame, 537 unowned; 44 vehicle tracks, 10
duplicate pairs, 452 pair-frames. On the segments, `a0` reproduces every figure in l5id0019's
segment table.

## K1: identity on kirk0

Full-comparison figures, then the paired bootstrap (19 scored objects, 2,000 draws, seed
20261108).

| Arm                      |   HOTA |   IDF1 | Switches | Frag |  FN | Tracks | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches      | Δ frag         | K1   |
| ------------------------ | -----: | -----: | -------: | ---: | --: | -----: | --------------------------- | --------------------------- | --------------- | -------------- | ---- |
| `a0`                     | 0.4284 | 0.4728 |       90 |   46 | 265 |     79 |                             |                             |                 |                |      |
| `robust-birth`           | 0.4370 | 0.4841 |        6 |   30 | 247 |     78 | +0.0032 / +0.0126 / +0.0228 | +0.0055 / +0.0183 / +0.0337 | −226 / −80 / −4 | −42 / −16 / −2 | Pass |
| `likelihood-birth`       | 0.4369 | 0.4839 |        6 |   30 | 247 |     78 | +0.0032 / +0.0126 / +0.0228 | +0.0055 / +0.0183 / +0.0337 | −226 / −80 / −4 | −42 / −16 / −2 | Pass |
| `robust-birth-tentative` | 0.4360 | 0.4942 |        7 |   31 | 248 |     69 | +0.0001 / +0.0103 / +0.0198 | +0.0006 / +0.0275 / +0.0634 | −224 / −80 / −2 | −41 / −15 / 0  | Pass |

The bootstrap's labelled-object HOTA is 0.4927 for `robust-birth` and `likelihood-birth`, 0.4905
for `robust-birth-tentative` and 0.4798 for `a0`. The two plain-gate arms give the same draws to
four decimals: on the labelled objects their associations agree. Tentative priority raises IDF1
most and HOTA least; its HOTA interval touches zero at the 5th percentile.

## kirk0, label-free (reported, not decided)

| Arm                      | Tracks | Brief | Near-static | Clutter (artefact / stationary / fragment / unclassed) | Spurious a frame | Unowned | Moving (lost / short / added) | p85, p98 (m/s) | Vehicle tracks | Dup pairs | Pair-frames | Births absorbed |
| ------------------------ | -----: | ----: | ----------: | ------------------------------------------------------ | ---------------: | ------: | ----------------------------- | -------------- | -------------: | --------: | ----------: | --------------: |
| `a0`                     |     79 |    12 |           3 | 15 (6 / 2 / 3 / 4)                                     |           0.6947 |     537 | 38                            | 7.06, 8.83     |             44 |        10 |         452 |               — |
| `robust-birth`           |     78 |    14 |           2 | 16 (7 / 1 / 2 / 6)                                     |           0.7206 |     556 | 41 (0 / 0 / 0)                | 7.01, 8.83     |             43 |         8 |         246 |              43 |
| `likelihood-birth`       |     78 |    13 |           2 | 15 (7 / 1 / 2 / 5)                                     |           0.7206 |     556 | 41 (0 / 0 / 0)                | 7.01, 8.83     |             43 |         8 |         246 |              43 |
| `robust-birth-tentative` |     69 |     8 |           2 | 10 (6 / 1 / 1 / 2)                                     |           0.6378 |     492 | 40 (0 / 0 / 0)                | 7.01, 8.83     |             38 |         7 |         241 |              43 |

Births absorbed is `fragment_births_absorbed` in the run's continuity, which counts frames, not
fragments. `lidar-box-eval` does not report it, so the segments have no such figure.

**Timing, reported only.** First-pass Tracker.Update p99: `a0` 17.1 ms, `robust-birth` 24.8 ms,
`likelihood-birth` 25.4 ms, `robust-birth-tentative` 23.5 ms. Frame p99: 544, 794, 930 and 689 ms.
Repeat passes moved the same figures by up to a factor of two (Tracker.Update p99 26.9 to
54.3 ms). The host ran six to ten other replay processes at the time, so these figures cannot
separate the arms. They are not a Pi cost.

## K2 and K3: the four segments

Each segment's `a0` row first, then the arms. Brief, near-static, clutter, spurious, unowned,
moving and speeds read the evidence; duplicates read `lidar-box-eval`'s scored window. "Moving"
gives the count, then lost, shortened and added against `a0` by footprint.

| Case      | Arm                      | Tracks | Brief | Near-static | Clutter (art / stat / frag / uncl) | Spurious a frame | Unowned | Moving (lost / short / added) | p85, p98 (m/s) | Vehicle tracks | Dup pairs (per 100) | Pair-frames |
| --------- | ------------------------ | -----: | ----: | ----------: | ---------------------------------- | ---------------: | ------: | ----------------------------- | -------------- | -------------: | ------------------- | ----------: |
| ashbury 1 | `a0`                     |     46 |     8 |          10 | 18 (12 / 1 / 1 / 4)                |            0.412 |     466 | 21                            | 10.89, 11.58   |             24 | 1 (4.2)             |          86 |
|           | `robust-birth`           |     48 |    11 |          10 | 21 (12 / 1 / 1 / 7)                |            0.369 |     417 | 20 (0 / 0 / 0)                | 10.90, 11.58   |             25 | 0 (0.0)             |           6 |
|           | `likelihood-birth`       |     46 |     9 |          10 | 19 (12 / 1 / 1 / 5)                |            0.366 |     414 | 20 (0 / 0 / 0)                | 10.90, 11.58   |             23 | 0 (0.0)             |           4 |
|           | `robust-birth-tentative` |     45 |     9 |           9 | 18 (11 / 1 / 1 / 5)                |            0.361 |     408 | 20 (0 / 0 / 0)                | 10.90, 11.58   |             23 | 0 (0.0)             |           2 |
| ashbury 2 | `a0`                     |     39 |     7 |           6 | 13 (10 / 0 / 0 / 3)                |            0.383 |     514 | 18                            | 10.84, 12.91   |             18 | 0 (0.0)             |           0 |
|           | `robust-birth`           |     37 |     7 |           6 | 13 (10 / 0 / 0 / 3)                |            0.227 |     305 | 17 (0 / 0 / 1)                | 10.84, 12.91   |             17 | 0 (0.0)             |           0 |
|           | `likelihood-birth`       |     38 |     7 |           6 | 13 (10 / 0 / 0 / 3)                |            0.228 |     306 | 18 (0 / 0 / 1)                | 10.84, 12.91   |             17 | 0 (0.0)             |           0 |
|           | `robust-birth-tentative` |     37 |     7 |           6 | 13 (10 / 0 / 0 / 3)                |            0.229 |     307 | 17 (0 / 0 / 1)                | 10.84, 12.91   |             17 | 0 (0.0)             |           0 |
| haight 1  | `a0`                     |     70 |    12 |           9 | 21 (12 / 3 / 3 / 3)                |            0.287 |     440 | 33                            | 9.31, 10.38    |             29 | 4 (13.8)            |          52 |
|           | `robust-birth`           |     73 |    15 |           8 | 23 (14 / 2 / 2 / 5)                |            0.311 |     476 | 32 (0 / 0 / 0)                | 9.31, 10.38    |             28 | 3 (10.7)            |          28 |
|           | `likelihood-birth`       |     71 |    14 |           8 | 22 (14 / 2 / 2 / 4)                |            0.308 |     472 | 32 (0 / 0 / 0)                | 9.31, 10.38    |             27 | 2 (7.4)             |          25 |
|           | `robust-birth-tentative` |     70 |    14 |           8 | 22 (15 / 2 / 2 / 3)                |            0.314 |     482 | 31 (0 / 0 / 0)                | 9.31, 10.38    |             27 | 4 (14.8)            |          28 |
| haight 2  | `a0`                     |     79 |    18 |           8 | 26 (14 / 3 / 1 / 8)                |            0.393 |     605 | 28                            | 9.65, 10.77    |             28 | 11 (39.3)           |         311 |
|           | `robust-birth`           |     76 |    21 |           8 | 29 (18 / 2 / 1 / 8)                |            0.408 |     628 | 25 (0 / 0 / 0)                | 9.65, 10.77    |             20 | 2 (10.0)            |          58 |
|           | `likelihood-birth`       |     78 |    21 |           9 | 30 (20 / 2 / 1 / 7)                |            0.390 |     601 | 26 (0 / 0 / 0)                | 9.65, 10.77    |             22 | 4 (18.2)            |         125 |
|           | `robust-birth-tentative` |     76 |    19 |           8 | 27 (18 / 2 / 0 / 7)                |            0.399 |     614 | 26 (0 / 0 / 0)                | 9.72, 10.77    |             23 | 6 (26.1)            |         121 |

**R4 on each segment** (each arm less `a0`): brief Δ, clutter Δ per hundred of `a0`'s moving
tracks, duplicate pairs per hundred vehicle tracks `a0` → arm, the moving count's change and p85's
change. Bold marks what costs.

| Arm                      | ashbury 1                                 | ashbury 2                       | haight 1                                      | haight 2                                        | Segments costing noise | K2   |
| ------------------------ | ----------------------------------------- | ------------------------------- | --------------------------------------------- | ----------------------------------------------- | ---------------------: | ---- |
| `robust-birth`           | +3 / **+14.3** / 4.2→0.0 / −4.8 % / +0.01 | 0 / 0 / 0→0 / **−5.6 %** / 0.00 | +3 / **+6.1** / 13.8→10.7 / −3.0 % / 0.00     | +3 / **+10.7** / 39.3→10.0 / **−10.7 %** / 0.00 |                 3 of 4 | Fail |
| `likelihood-birth`       | +1 / +4.8 / 4.2→0.0 / −4.8 % / +0.01      | 0 / 0 / 0→0 / 0.0 % / 0.00      | +2 / +3.0 / 13.8→7.4 / −3.0 % / 0.00          | +3 / **+14.3** / 39.3→18.2 / **−7.1 %** / 0.00  |                 1 of 4 | Pass |
| `robust-birth-tentative` | +1 / 0.0 / 4.2→0.0 / −4.8 % / +0.01       | 0 / 0 / 0→0 / **−5.6 %** / 0.00 | +2 / +3.0 / **13.8→14.8** / **−6.1 %** / 0.00 | +1 / +3.6 / 39.3→26.1 / **−7.1 %** / +0.07      |                 1 of 4 | Pass |

No arm loses or shortens a lasting moving track by footprint on any segment's evidence, so K2's
second condition holds for every arm. p85 moves by 0.07 m/s at most anywhere.

### The tracks behind each count move

Every move over 5 % is listed, with each of `a0`'s moving tracks that the candidate does not carry
as a moving track of its own, and each candidate moving track `a0` lacks. Times are from the
segment's first evidence row.

| Case      | Arm(s)                                                       | Count           | Track (`a0` unless marked)                                                                                                                                                                                        | Duplicate removed?                 |
| --------- | ------------------------------------------------------------ | --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------- |
| ashbury 2 | `robust-birth`, `robust-birth-tentative`                     | 18 → 17         | Two moving tracks, 111 to 137 s (26 m) and 134 to 146 s (36 m), barely overlapping in place (2 % or less of either's rows). The candidate carries one track, 111 to 146 s, 62 m                                   | No: a fragment joined              |
| haight 1  | `robust-birth-tentative`                                     | 33 → 31         | A 39 s track, 37 m, median 1.0 m/s. The candidate's track over it lives 18 to 58 s with median 0.8 m/s, so it is not moving                                                                                       | No: a threshold crossing           |
|           |                                                              |                 | Two tracks, 36 to 58 s and 50 to 81 s, 3 % or less overlapping in place. The candidate carries one track, 36 to 81 s, 42 m                                                                                        | No: a fragment joined              |
| haight 2  | `robust-birth`, `likelihood-birth`, `robust-birth-tentative` | 28 → 25, 26, 26 | A walker-paced track, 74 to 108 s, 34 m, median 1.02 m/s. `a0` had the same body near-static from 26 to 77 s in another track. The candidate carries both in one 82 s track, median 0.52 m/s, which is not moving | No: a threshold crossing on a join |
|           |                                                              |                 | Two moving tracks, 118 to 121 s, each covered on every row by the other `a0` track. The candidates carry one moving track there                                                                                   | Yes                                |
|           |                                                              |                 | A 2.6 s moving track at 135 s, covered on every row by another `a0` track. No candidate carries it as moving                                                                                                      | Yes                                |
|           | `likelihood-birth`, `robust-birth-tentative` (added)         |                 | A candidate track at 148 s, 5.8 to 6.1 m first to last, median 1.3 m/s. `a0`'s twin is 4.8 m first to last, under the 5 m threshold                                                                               | No: a threshold crossing           |

At haight 2, `likelihood-birth` and `robust-birth-tentative` net −2: two duplicates removed, one
track lost to the threshold and one added by it. `robust-birth` lacks the added track, so it nets
−3.

| Arm                      |             Segments over 5 % | Every track behind them a duplicate removed?                     | K3   |
| ------------------------ | ----------------------------: | ---------------------------------------------------------------- | ---- |
| `robust-birth`           |           ashbury 2, haight 2 | No                                                               | Fail |
| `likelihood-birth`       |                      haight 2 | No: one threshold crossing on a join, one added at the threshold | Fail |
| `robust-birth-tentative` | ashbury 2, haight 1, haight 2 | No                                                               | Fail |

### Pooled over the four segments

| Arm                      | Brief | Clutter | Moving | Dup pairs | Pair-frames |
| ------------------------ | ----: | ------: | -----: | --------: | ----------: |
| `a0`                     |    45 |      78 |    100 |        16 |         449 |
| `robust-birth`           |    54 |      86 |     94 |         5 |          92 |
| `likelihood-birth`       |    51 |      84 |     96 |         6 |         154 |
| `robust-birth-tentative` |    49 |      80 |     94 |        10 |         151 |

### The rows read (reported, not decided)

On the rows, which span only the scored 120 s, the moving count moves by −4.2 %, −4.8 %, −8.3 %
and −2.9 % under `likelihood-birth` (ashbury 1, ashbury 2, haight 1, haight 2). Under
`robust-birth` it moves +4.2 %, −4.8 %, −8.3 % and −8.8 %; under `robust-birth-tentative`,
−4.2 %, −4.8 %, −13.9 % and −2.9 %. The rows also count lasting moving tracks lost against `a0`:
three and two at the two haight segments under `robust-birth`, two at haight 1 under
`likelihood-birth`, and three at each haight segment under `robust-birth-tentative`. A row
track's duration is its span inside the window, so tracks cut by the window's edges move in and
out of "lasting". These rows-read losses were not traced track by track. On the rows read, every
arm's count also moves by more than 5 % at haight 1.

## Against the rule

| Clause | `robust-birth`                                        | `likelihood-birth`                            | `robust-birth-tentative`                                     |
| ------ | ----------------------------------------------------- | --------------------------------------------- | ------------------------------------------------------------ |
| K1     | Pass: HOTA 5th +0.0032, IDF1 5th +0.0055              | Pass: HOTA 5th +0.0032, IDF1 5th +0.0055      | Pass: HOTA 5th +0.0001, IDF1 5th +0.0006                     |
| K2     | **Fail**: clutter at ashbury 1, haight 1 and haight 2 | Pass: clutter at haight 2 only; no mover lost | Pass: one duplicate pair at haight 1 only; no mover lost     |
| K3     | **Fail**: ashbury 2 −5.6 %, haight 2 −10.7 %          | **Fail**: haight 2 −7.1 %, not all duplicates | **Fail**: ashbury 2 −5.6 %, haight 1 −6.1 %, haight 2 −7.1 % |
| K4     | Not eligible                                          | Not eligible                                  | Not eligible                                                 |
| K5     | Not read                                              | Not read                                      | Not read                                                     |

**No arm is named.** K5 is not read, for two reasons, either of which is enough: no arm passes K1
to K3, and label0003's frozen spans do not exist yet. The span reader the spec's implementation
step names was not built in this run for the second reason. By the spec, no default changes.

**Had K3 been read on net counts.** If the two threshold crossings at haight 2 were paired off,
`likelihood-birth`'s −2 there would be two duplicates removed, and it would pass K1 to K3. K4
would then name it: neither later arm passes K2 and K3 on that reading either (`robust-birth`
fails K2, and `robust-birth-tentative`'s haight 1 move is a threshold crossing and a fragment),
so the pair-frame exception does not arise. The rule as written reads the tracks, not the net, so this is a reading the rule does
not make. It is stated because it is the nearest the screen came to a name.

## Interpretation

Each reading is marked measured or inferred.

- **On identity, the plain likelihood is the robust cost on kirk0.** Measured: with the gate,
  `likelihood-birth` and `robust-birth` give the same bootstrap to four decimals, the same 6
  switches, the same 43 absorbed births and the same duplicates. `likelihood-birth` has one fewer
  brief track. The `ln det S` term does the work; the Student-t tail does not change the scored
  associations.
- **Off kirk0, the plain likelihood costs less noise than the robust cost.** Measured: with the
  gate, `likelihood-birth` raises clutter by at most 4.8 per hundred moving tracks on three
  segments, and by 14.3 at haight 2. `robust-birth` exceeds the bound at three segments. Pooled,
  brief tracks are 51 against 54 and clutter 84 against 86. Inferred: the Student-t cost's
  heavier tail lets more young tracks win the contests that confirm brief tracks. This is the
  same direction kirk0 showed (13 brief against 14).
- **The gate does not reach the placements' clutter.** Measured: haight 2's extra clutter under
  every candidate is artefacts (14 under `a0`, 18 to 20 under the candidates), not fragments of a
  mover (1 or 0). Inferred: the gate absorbs fragments inside a confirmed vehicle's box; these
  artefacts lie away from any mover, so the gate cannot touch them. They come from the likelihood
  cost itself, which changes who keeps which scrap.
- **Tentative priority settles duplicates on kirk0, not on the placements.** Measured: on kirk0
  it removes 9 tracks, 6 of them brief, and cuts clutter from 16 to 10. On the placements it adds
  a duplicate pair at haight 1 and keeps 6 pairs at haight 2, against `robust-birth`'s 2. Its
  pooled pair-frames (151) are higher than `robust-birth`'s (92).
- **The count moves the rule reads are made by the identity gain.** Measured: of the seven track
  changes behind the count moves, two are duplicates removed, two are fragments joined and three
  are a moving test's threshold crossed. Every candidate carries the haight 2 walker in one
  82 s track where `a0` used two. The joined track's median speed is 0.52 m/s, under the 1 m/s
  moving test. Inferred: a count built on a whole-life median speed penalises better continuity
  of a body that stops and starts. K3 was written to catch a candidate that loses road users; here
  it reads continuity as a loss. This is a fact about the count's definition, not about the cost.
- **The duplicates fall wherever there are any.** Measured: from 449 pooled pair-frames to 92 to
  154, and at haight 2 from 11 pairs to 2 to 6. This is the identity gain off kirk0, in the one
  measure here that reads it directly.

## Recommendations

1. **Name the threshold problem before the next count clause.** A count clause on these segments
   should read moving tracks through a test that does not change with the track's life, such as a
   peak rolling speed or a displacement rate. Or it should read each count move against the
   candidate's continuity, as the track table above does by hand. Until then, two joined
   stop-and-go bodies are enough to move a 5 % count bound on a 28-track segment.
2. **Prefer `likelihood-birth` to `robust-birth` as the identity candidate.** It is simpler, it
   matches robust's identity on kirk0, and it costs less noise on the placements. The robust form
   fails K2 here; the plain form does not. This is a ranking, not a K4 name.
3. **Drop tentative priority from this candidate.** It narrows K1 to +0.0001 and does not settle
   the placements' duplicates.
4. **Read the haight 2 artefacts.** The likelihood cost's clutter at haight 2 is artefacts away
   from movers. A tap like l5id0021's on that segment would say what they are born from, before
   another birth rule is written for them.
5. **K5 waits for label0003's spans and a named arm.** A future spec that names
   `likelihood-birth` under a revised count clause should be written before its replays, and
   should read the spans once.
6. **Keep everything default-off.** Nothing here is held out. kirk0 is in sample, and the four
   segments are a label-free screen whose held-out budget is spent (November 5 review, F21).

## What this does not establish

- **Physical truth on the placements.** The segments have no labels. Duplicates, brief tracks and
  counts are what the pipeline drew, not where road users were. That the haight 2 walker is one
  body is read from track footprints and times, not from truth.
- **That the walker is a pedestrian.** It moves at walker pace (median 1.02 m/s over its moving
  part) and stands for 51 s. Nothing here classifies it.
- **A held-out read.** K5 is not read, and the spans it needs do not exist.
- **A cost on a pedestrian partly hidden by a vehicle.** No capture read here has one.
- **Queues.** The segments are quiet residential corners with 18 to 33 moving tracks each, and no
  queues. One track moves a clause.
- **Pi cost.** The timings come from a shared cloud container under heavy load, and do not
  separate the arms.
- **The rows-read losses.** They were reported, not traced.

## Provenance

| Item          | Value                                                                                                                                                                                                                                                                                                                                                                                                         |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base          | `exp/l5id0023` from `ab19e51` (spec approved, rule committed before any replay)                                                                                                                                                                                                                                                                                                                               |
| Option        | `9f04ae0` (`9f04ae01e84f87596c0ef2f02975e0d204635190`): `robust_birth_gate` accepts `likelihood_cost`; `go test ./internal/lidar/replayeval ./internal/config` pass                                                                                                                                                                                                                                           |
| Tools         | Built at `9f04ae0` with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA` the full sha, `BuildTime`) and the `pcap` tag, against Ubuntu's libpcap 1.10.4. Every replay header and box-eval summary reads `build_version` 0.5.1-pre52 and `build_git_sha` `9f04ae01…`. `lidar-state-estimation-baseline` `sha256:3864bef0…`, `lidar-box-eval` `6a67497a…`, `lidar-ground-truth-eval` `6cf06318…` |
| Parameter set | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`                                                                                                                                                                                                                                                                                                                           |
| Arm hashes    | `a0` `ab0c4035780d`; `robust-birth` `e9db12b214d1`; `likelihood-birth` `63e42b981b90`; `robust-birth-tentative` `7f233d7a2bb9` (each folds its replay experiments into the tuning hash)                                                                                                                                                                                                                       |
| Baselines     | Tracking baseline digests, first and repeat equal: `a0` `79d9abff…`, `robust-birth` `dee8eb94…`, `likelihood-birth` `e6479076…`, `robust-birth-tentative` `d0cf47a3…`; source manifest `sha256:d7e33e0d…`                                                                                                                                                                                                     |
| Capture       | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`, equal to the repository's LFS pointer                                                                                                                                                                                                                                                                                       |
| Segments      | Cut as l5id0019 cut them, hashes equal to its: ashbury-downey 1 `971e093e…`, 2 `e953e352…`; haight-divisadero 1 `2afd5be4…`, 2 `5b4d6b8e…`                                                                                                                                                                                                                                                                    |
| Labels        | The pack RECONSTRUCTED as in l5id0019: label set `kirk0-reference` revision 2233, pack `ds_67842cb1b22b70e7` rebuilt by `lidar-pack-rebuild` (pack digest `sha256:f0ed3414…`, packets `3ffeba76…`, returns `f0f9ca27…`, points `7860a86c…`); split `kirk0-tuning` drafted with `-from-seconds 6`, manifest `7194bb61…`, reference digest `sha256:a784e69d…`; `obj_788622b5` ignored                           |
| Eligibility   | Committed cohort manifest; a scratch use ledger at migration 71; actor `appius-l5id0023`                                                                                                                                                                                                                                                                                                                      |
| Host          | Claude Code cloud container, 4 cores, 15 GB, shared with two other experiments; two lanes, `GOMAXPROCS=2`, `nice -n 5`                                                                                                                                                                                                                                                                                        |
| Raw output    | Scratch only. Evidence databases, rows and summaries were deleted once these tables were written                                                                                                                                                                                                                                                                                                              |
