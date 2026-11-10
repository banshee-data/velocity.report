# Track-conditioned fragment joining on two placements drawn by lot

**Renumbered.** This experiment ran as l4clus0011 on 2026-11-10 and was renumbered l4clus0012
when main's own l4clus0011 (#1003, [six more placements with a label-free merge
reference](w3-join-membership-corpus-merge-reference-2026-11.md)) merged first. The two read the
same option on different placements and neither read the other's.

<!-- ignore-style-length -->

- **Status:** Measured on kirk0 and on two placements on 2026-11-10. `join-membership` passes M1
  and M2 and fails M3 and M4. Nothing is named for label0003's window, nothing is adopted and no
  default changes
- **Spec:** [l4clus0012](../../../data/experiments/l4clus/l4clus0012.md). Its rule (M1 to M4) was
  committed at `0a9316f`, before any replay
- **Tooling:** `scripts/lidar_merge_read.py`, the inferred-merge read, with fixture tests (commit
  `9ac2f49`, before any replay). No existing script changed
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`, and the [motion join set](../../../config/experiments/motion-fragment-join.json),
  fingerprint `719d291ed197`. `join-membership` differs from `join` only by the replay experiment
  `fragment_join_track_membership`
- **Related:** l5id0024's [lot report](likelihood-birth-gate-lot-2026-11.md), whose segments,
  peak test and join table this run reuses; l4clus0010's
  [truck membership report](w3-truck-membership-extent-kirk0-2026-11.md); l5id0019's
  [November benchmark](tracking-geometry-fixes-benchmark-2026-11.md)

## Summary

- **M1 holds on kirk0.** Against `a0` the paired bootstrap gives a HOTA 5th percentile of
  −0.0002 and an IDF1 5th percentile of +0.0004, with a switch change of 0 at the 95th
  percentile. The full comparison reads HOTA 0.4323, IDF1 0.4769, 90 switches and 42
  fragmentations, l5id0019's figures exactly, and `a0` reads the l5id README's anchor exactly.
- **M2 holds.** On l5id0019's R4 the candidate costs no noise at columbus-north-point's two
  segments or lombard-broderick's first. At lombard-broderick's second its duplicate pairs per
  hundred vehicle tracks rise from 11.4 to 11.6 (the same 19 pairs over 164 vehicle tracks
  against 167), which the clause counts as a cost. Three of four pass, and it loses no lasting
  moving track by footprint on any segment. Brief tracks move by +1, −1, 0 and −5, and
  spurious clusters a frame fall at every segment (−6 % to −13 %).
- **M3 fails at lombard-broderick's first segment.** Under the peak test, after the join credit,
  the moving count moves by +2.7 %, 0.0 %, **+6.6 %** and 0.0 %. The raw count there moves by
  −1.0 %; the credit adds back 15 of `a0`'s moving tracks the candidate carries in 14 joins (12
  of them concurrent, a duplicate in `a0` removed), while 15 of `a0`'s moving vehicles are carried
  in a second moving candidate track (10 short doubles, 4 sequential pieces). p85 moves by
  0.39 m/s at most, inside its bound.
- **M4 fails.** Inferred merges per segment against `a0`'s floor (17, 15, 18, 9) are 17, 18, 26 and
  12: over the floor plus one at three segments, and 14 in excess over the four where two are
  allowed. The motion join alone, the control, reads 17, 18, 19 and 10 (5 in excess). Most of the
  excess is the same `a0` pairs held by more candidate tracks; the pairs the floor does not have
  are 9 for the candidate (6 at lombard-broderick's first) and 2 for the control.
- **The merges the read finds are mostly not two vehicles abreast.** Every new pair at
  lombard-broderick's first segment is held for one frame by a car-sized footprint (1.6 to 1.8 m
  wide) as two of `a0`'s tracks pass within 2.3 to 3.2 m of each other. One footprint in both
  candidate arms, 3.6 m wide for four frames at lombard-broderick's second, holds a slow vehicle 6
  m from the sensor and a passing one: the case the spec names as its risk, read once.

## Question and rule

l4clus0010 found that track-conditioned membership took kirk0's fragmented truck instants from
198 to 145 of 936 with identity non-inferior, and l5id0019's screen gave it IDF1 0.4728 to 0.4769
with no noise cost on five quiet cases. kirk0 holds no two vehicles side by side, so the case every
report names as its risk, a footprint that joins two vehicles, was unread. This run reads the
candidate on l5id0024's two lot-drawn placements, the busiest segments read this month, with a
merge read written before the replays.

The rule, in short ([spec](../../../data/experiments/l4clus/l4clus0012.md)), read for
`join-membership`, with `join` reported as its control:

- **M1, the anchor.** On kirk0, against `a0`, the paired bootstrap's 5th percentiles of the HOTA
  and IDF1 deltas above −0.005 and the 95th percentile of the switch change +10 or less.
- **M2, noise.** On each segment under l5id0019's R4 (brief tracks rise by 3 at most, clutter by 5
  per hundred of `a0`'s moving tracks at most, duplicate pairs per hundred vehicle tracks do not
  rise), moving by l5id0024's peak test: pass on three of the four segments at least, and lose no
  lasting moving track by footprint on any.
- **M3, counts.** On each segment the moving count moves by 5 % at most after l5id0024's join
  credit, and p85 by 0.5 m/s at most.
- **M4, merges.** Inferred merges per segment no more than `a0`'s floor plus one, and no more than
  two over the four segments in excess of the floor.
- **Outcome.** A pass on M1 to M4 names `join-membership` for label0003's window, with label0001's
  masks on any adjacent pair as the merge reference that decides it. A fail names its clause.
  Nothing is adopted.

## Arms

| Arm               | Parameter set               | Replay experiments               | Parameter hash |
| ----------------- | --------------------------- | -------------------------------- | -------------- |
| `a0`              | `tuning.defaults.json`      | none                             | `ab0c4035780d` |
| `join`            | `motion-fragment-join.json` | none                             | `719d291ed197` |
| `join-membership` | `motion-fragment-join.json` | `fragment_join_track_membership` | `f17eae8f1ac6` |

The parameter hash folds the replay experiments into the tuning hash. Unlike l4clus0010, no arm
runs the solid body or its face state.

## Method

- **The merge read.** `lidar_merge_read.py` (commit `9ac2f49`, before any replay) reads two
  evidence databases or two rows CSVs. A track of `a0` is moving under l5id0024's peak test (2.0 s
  or more, maximum 1.0 s rolling median speed 1.5 m/s or more). A candidate row's footprint is its
  drawn box grown by 0.25 m on each side. Two of `a0`'s moving tracks are a duplicate pair when
  their drawn boxes, not grown, overlap by half the smaller's area or more as a mean over their
  shared frames (9 by 9 grid, as `lidar_flip_cost.py` estimates overlap); without boxes, when their
  centres lie within 2.0 m on half their shared frames. An inferred merge is a candidate track and
  an unordered pair of `a0`'s moving tracks, not a duplicate pair, such that one of the candidate
  track's footprints contains both centres at one frame. It is counted once per candidate track
  and pair. `a0` read against itself gives the floor. The fixture tests include a pass beside a
  parked car (not a merge: the parked car is not moving), a true two-abreast merge (one), a
  duplicate pair (none), neighbours in their own lanes (none) and the floor; one test records that
  a neighbour swallowed whole by a wide box reads as a duplicate.
- **The segments.** l5id0024's lot: the first two five-minute segments (0 to 300 s and 300 to
  600 s from the first packet) of columbus-north-point and lombard-broderick, from the Hugging
  Face dataset, one capture at a time, cut by the run's `pcapng_window.py`, each capture deleted
  once cut. All four segments' SHA-256 digests equal l5id0024's.
- **kirk0 replays.** `lidar-state-estimation-baseline` on kirk0 whole, `-warmup 6
-require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case, the committed
  cohort manifest, a copy of the run's scratch use ledger (migration 71) for each arm so that no
  two replays share one. Each arm's repeat pass reproduced its first pass's tracking baseline byte
  for byte.
- **Identity.** `lidar-ground-truth-eval perframe` on the reconstructed pack, `kirk0-tuning`
  split with `-allow-tuning-split`, `obj_788622b5` ignored, online estimates as declared baselines,
  each candidate against `a0` with `-bootstrap 2000 -bootstrap-seed 20261108`.
- **kirk0 boxes.** `lidar-box-eval -variants none -rows -require-settled=false -warmup 6
-duration 0 -sample-points 1024` with an evidence database, for duplicate pairs and the merge
  read on kirk0.
- **The placements.** Each arm through `lidar-box-eval -warmup 40 -duration 120 -variants none
-rows -require-settled=false -sample-points 1024` with an evidence database. On each arm's
  database, as l5id0024 ran them: `lidar_noise_screen.py` and `lidar_clutter_origin.py` on their
  own; `lidar_flip_cost.py --match footprint`, `lidar_figure_sensitivity.py` (the median test
  plain and with `--joins`, and `--moving peak --joins`) and `lidar_merge_read.py` against `a0`'s;
  lost, shortened and added under the peak test from `lidar_flip_cost.py`'s coverage. Duplicates
  come from box-eval's summary.
- **Which window decides.** As in l5id0019 and l5id0024, the tables read the evidence, about
  154 s of each segment with the warm-up. The same scripts on the rows, which span only the scored
  120 s, are reported, not decided on. That applies to the merge read too.
- **The tracks behind the count.** l5id0024's scratch trace paired each moving candidate track that
  carries no `a0` moving track of its own with the `a0` track it covers most and that track's
  carrier, to say whether it is a piece or a double.
- **Order.** kirk0's three replays ran first, then box-eval on kirk0 and the twelve segment
  replays in two lanes, `GOMAXPROCS=2`, `nice -n 5`, on a host shared with another experiment.
  M1 was read before any segment's figures. The scripts ran as each segment pair finished; each
  evidence database was cut down to its track tables after its own scripts had run, to keep the
  scratch disk free.

**The anchor.** `a0` equals the l5id README's anchor: tracking baseline `79d9abff…`, HOTA 0.4284,
IDF1 0.4728, 90 switches, 46 fragmentations, 265 false negatives; bootstrap labelled-object HOTA
0.4798; box-eval 44 vehicle tracks, 10 duplicate pairs, 452 pair-frames. On the four segments
`a0` reproduces l5id0024's `a0` rows in every figure the two reports share.

## M1: identity on kirk0

Full-comparison figures, then the paired bootstrap (19 scored objects, 2,000 draws, seed
20261108).

| Arm               |   HOTA |   IDF1 | Switches | Frag |  FN | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches  | Δ frag       | M1   |
| ----------------- | -----: | -----: | -------: | ---: | --: | --------------------------- | --------------------------- | ----------- | ------------ | ---- |
| `a0`              | 0.4284 | 0.4728 |       90 |   46 | 265 |                             |                             |             |              |      |
| `join`            | 0.4307 | 0.4804 |       89 |   42 | 262 | −0.0013 / +0.0018 / +0.0040 | +0.0004 / +0.0077 / +0.0181 | −3 / −1 / 0 | −12 / −4 / 0 |      |
| `join-membership` | 0.4323 | 0.4769 |       90 |   42 | 262 | −0.0002 / +0.0037 / +0.0081 | +0.0004 / +0.0032 / +0.0055 | 0 / 0 / 0   | −12 / −4 / 0 | Pass |

Bootstrap labelled-object HOTA: `a0` 0.4798, `join` 0.4819, `join-membership` 0.4835. Tracking
baselines: `a0` `79d9abff…`, `join` `9dc26717…` (l4clus0010's), `join-membership` `9a93b79b…`.
Joins on kirk0's 773 clustered frames: `join` 39 components in 39 frames, `join-membership` 79 in
78, as l4clus0010. Box-eval on kirk0: vehicle tracks, duplicate pairs and pair-frames 44, 10, 452
(`a0`), 43, 10, 431 (`join`) and 44, 9, 416 (`join-membership`). The merge read on kirk0's evidence
gives 4 in every arm, the floor included.

## M2 and M3: the four segments

Each segment's `a0` row, then the two candidates. Brief, near-static, clutter, spurious,
unowned, moving and speeds read the evidence; duplicates read box-eval's scored window. "Moving,
peak" gives the count under the peak test, then lost, shortened and added against `a0` by
footprint under the same test. Speeds are the peak test's p85 and p98.

| Case       | Arm               | Tracks | Brief | Near-static | Clutter (art / stat / frag / uncl) | Spurious a frame | Unowned | Moving, peak (lost / short / added) | p85, p98 (m/s) | Moving, median | Vehicle tracks | Dup pairs (per 100) | Pair-frames |
| ---------- | ----------------- | -----: | ----: | ----------: | ---------------------------------- | ---------------: | ------: | ----------------------------------- | -------------- | -------------: | -------------: | ------------------- | ----------: |
| columbus 1 | `a0`              |    191 |    32 |          11 | 43 (14 / 5 / 7 / 17)               |            0.886 |   1,366 | 73                                  | 8.75, 11.08    |             83 |             66 | 28 (42.4)           |         791 |
|            | `join`            |    180 |    33 |          10 | 43 (16 / 4 / 3 / 20)               |            0.836 |   1,289 | 65 (0 / 1 / 0)                      | 8.83, 11.08    |             76 |             63 | 26 (41.3)           |         941 |
|            | `join-membership` |    177 |    33 |          10 | 43 (15 / 4 / 3 / 21)               |            0.771 |   1,189 | 65 (0 / 1 / 0)                      | 8.83, 11.08    |             74 |             62 | 25 (40.3)           |         721 |
| columbus 2 | `a0`              |    179 |    36 |          10 | 46 (20 / 4 / 2 / 20)               |            0.927 |   1,425 | 78                                  | 8.61, 11.59    |             63 |             81 | 24 (29.6)           |         488 |
|            | `join`            |    176 |    35 |          10 | 45 (20 / 4 / 1 / 20)               |            0.903 |   1,388 | 77 (0 / 1 / 0)                      | 9.00, 11.59    |             62 |             81 | 21 (25.9)           |         369 |
|            | `join-membership` |    171 |    35 |          10 | 45 (20 / 4 / 2 / 19)               |            0.873 |   1,342 | 74 (0 / 1 / 0)                      | 9.00, 11.59    |             58 |             76 | 16 (21.1)           |         267 |
| lombard 1  | `a0`              |    356 |    76 |           5 | 81 (16 / 3 / 4 / 58)               |            1.866 |   2,880 | 196                                 | 11.52, 13.28   |            182 |            241 | 44 (18.3)           |       1,101 |
|            | `join`            |    356 |    79 |           5 | 84 (17 / 3 / 5 / 59)               |            1.777 |   2,742 | 197 (0 / 2 / 0)                     | 11.65, 13.28   |            182 |            239 | 38 (15.9)           |       1,140 |
|            | `join-membership` |    351 |    76 |           5 | 81 (18 / 3 / 4 / 56)               |            1.688 |   2,606 | 194 (0 / 3 / 0)                     | 11.67, 13.28   |            181 |            235 | 29 (12.3)           |         847 |
| lombard 2  | `a0`              |    317 |    77 |           2 | 79 (14 / 0 / 7 / 58)               |            1.750 |   2,693 | 173                                 | 12.53, 14.56   |            158 |            167 | 19 (11.4)           |         738 |
|            | `join`            |    307 |    72 |           2 | 74 (12 / 0 / 5 / 57)               |            1.646 |   2,536 | 172 (0 / 0 / 0)                     | 12.53, 14.65   |            158 |            165 | 18 (10.9)           |         696 |
|            | `join-membership` |    304 |    72 |           1 | 73 (11 / 0 / 4 / 58)               |            1.571 |   2,422 | 170 (0 / 0 / 0)                     | 12.53, 14.56   |            156 |            164 | 19 (11.6)           |         768 |

Under the old median test `join-membership` loses one moving track at columbus 1 and none
elsewhere (`join` none); both shorten 0 to 5.

**M2 on each segment** (candidate less `a0`): brief Δ, clutter Δ per hundred of `a0`'s moving
tracks (peak test), duplicate pairs per hundred vehicle tracks `a0` → candidate, lasting movers
lost. Bold marks what costs.

| Segment    | `join-membership`: brief Δ | Clutter Δ per 100 | Dup pairs per 100 | Movers lost | Costs noise | `join`: brief Δ, clutter Δ, dups per 100, lost | Costs noise |
| ---------- | -------------------------: | ----------------: | ----------------- | ----------: | ----------- | ---------------------------------------------- | ----------- |
| columbus 1 |                         +1 |              +0.0 | 42.4 → 40.3       |           0 | No          | +1, +0.0, 41.3, 0                              | No          |
| columbus 2 |                         −1 |              −1.3 | 29.6 → 21.1       |           0 | No          | −1, −1.3, 25.9, 0                              | No          |
| lombard 1  |                          0 |              +0.0 | 18.3 → 12.3       |           0 | No          | +3, +1.5, 15.9, 0                              | No          |
| lombard 2  |                         −5 |              −3.5 | 11.4 → **11.6**   |           0 | Yes         | −5, −2.9, 10.9, 0                              | No          |

`join-membership` passes on three of the four segments and loses no lasting mover, so **M2
holds**. Its one cost is the duplicate condition at lombard 2, where the pair count is the same
(19) and the vehicle tracks fall from 167 to 164. The control passes on all four.

### M3: the counts, peak test beside the old median test

Count changes against `a0`, raw and after the join credit. "Every member" is the credit without
l5id0024's exception for a member another moving candidate track also carries.

| Segment    | Arm               | Peak: `a0` → cand | Raw     | Joins (members) | Credit | Credited   | Every member | p85 Δ | Median: `a0` → cand | Raw    | Credit | Credited | p85 Δ |
| ---------- | ----------------- | ----------------- | ------- | --------------- | -----: | ---------- | ------------ | ----: | ------------------- | ------ | -----: | -------- | ----: |
| columbus 1 | `join-membership` | 73 → 65           | −11.0 % | 3 (13)          |     10 | +2.7 %     | +2.7 %       | +0.08 | 83 → 74             | −10.8% |      9 | +0.0 %   | +0.03 |
|            | `join`            | 73 → 65           | −11.0 % | 3 (12)          |      7 | −1.4 %     | +1.4 %       | +0.08 | 83 → 76             | −8.4%  |      6 | −1.2 %   | −0.00 |
| columbus 2 | `join-membership` | 78 → 74           | −5.1 %  | 4 (10)          |      4 | +0.0 %     | +2.6 %       | +0.39 | 63 → 58             | −7.9%  |      3 | −3.2 %   | +0.49 |
|            | `join`            | 78 → 77           | −1.3 %  | 4 (8)           |      2 | +1.3 %     | +3.8 %       | +0.39 | 63 → 62             | −1.6%  |      1 | +0.0 %   | +0.48 |
| lombard 1  | `join-membership` | 196 → 194         | −1.0 %  | 14 (29)         |     15 | **+6.6 %** | **+6.6 %**   | +0.14 | 182 → 181           | −0.5%  |     13 | +6.6 %   | +0.09 |
|            | `join`            | 196 → 197         | +0.5 %  | 5 (10)          |      5 | +3.1 %     | +3.1 %       | +0.13 | 182 → 182           | +0.0%  |      4 | +2.2 %   | +0.09 |
| lombard 2  | `join-membership` | 173 → 170         | −1.7 %  | 3 (6)           |      3 | +0.0 %     | +0.0 %       | +0.00 | 158 → 156           | −1.3%  |      3 | +0.6 %   | +0.06 |
|            | `join`            | 173 → 172         | −0.6 %  | 2 (4)           |      2 | +0.6 %     | +0.6 %       | +0.00 | 158 → 158           | +0.0%  |      2 | +1.3 %   | +0.00 |

Under the peak test with the credit, `join-membership`'s count moves by more than 5 % at lombard 1,
so **M3 fails**. It fails there under either credit and under the old median test too. p85 holds
everywhere (0.39 m/s at most). The control passes M3 on every segment. Read raw, without the
credit, the peak count moves by more than 5 % at both columbus segments for the candidate and at
columbus 1 for the control: a 78 s stop-and-go track there carries four of `a0`'s moving tracks in
both arms, as in l5id0024.

**Why the credited count rises at lombard 1.** The credit gives back the 15 moving tracks that the
candidate's 14 joins absorb; 12 of the joins are concurrent, a duplicate in `a0` removed. The
candidate also carries 15 of `a0`'s moving vehicles in a second moving track, so the raw count
barely falls (196 to 194) while the credit adds 15. These are the moving candidate tracks that are
not the carrier of any `a0` moving track:

| Segment    | Arm               | Moving candidate tracks with no `a0` track of their own | A piece (sequential) | A double (concurrent) | Over an `a0` track not moving |
| ---------- | ----------------- | ------------------------------------------------------: | -------------------: | --------------------: | ----------------------------: |
| columbus 1 | `join-membership` |                                                       3 |                    0 |                     2 |                             1 |
|            | `join`            |                                                       2 |                    0 |                     1 |                             1 |
| columbus 2 | `join-membership` |                                                       3 |                    1 |                     0 |                             1 |
|            | `join`            |                                                       4 |                    0 |                     3 |                             0 |
| lombard 1  | `join-membership` |                                                      17 |                    4 |                    10 |                             2 |
|            | `join`            |                                                       9 |                    3 |                     3 |                             2 |
| lombard 2  | `join-membership` |                                                       0 |                    0 |                     0 |                             0 |
|            | `join`            |                                                       1 |                    0 |                     1 |                             0 |

The kinds do not always sum to the first column: a track whose `a0` match has no carrier to compare
against is counted in the first column only (one at columbus 2 and lombard 1 for the candidate,
one at columbus 2 and lombard 1 for the control).

### The joins

Every join under the peak test for `join-membership`, from the evidence. Times are seconds from
the first evidence row of either arm. A starred member is also carried 0.5 or more by another
moving candidate track, so it earns no credit.

| Case       | Candidate track (s)       | Moving? | `a0` moving tracks carried (s, coverage)                                                                                                 | Kind       | Credit |
| ---------- | ------------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -----: |
| columbus 1 | `4f23cb65` 43.9 to 127.4  | yes     | 43.9 to 87.1 (0.97); 54.3 to 60.5 (1.00); 77.0 to 78.9 (0.80); 84.7 to 127.4 (0.98)                                                      | concurrent |      3 |
| columbus 1 | `e078c007` 109.0 to 135.4 | yes     | 108.9 to 135.4 (1.00); 109.6 to 116.0 (0.92); 114.3 to 116.0 (1.00)                                                                      | concurrent |      2 |
| columbus 1 | `701a48d2` 132.0 to 153.7 | yes     | 132.0 to 144.9 (1.00); 140.6 to 143.8 (1.00); 143.6 to 153.7 (1.00); 143.9 to 145.8 (1.00); 144.3 to 146.4 (0.94); 144.8 to 146.0 (1.00) | concurrent |      5 |
| columbus 2 | `23d7aa36` 26.5 to 45.7   | yes     | 26.5 to 41.7 (1.00); 41.8 to 45.7 (1.00)                                                                                                 | sequential |      1 |
| columbus 2 | `8bf7b68b` 52.1 to 92.2   | yes     | 52.1 to 79.1 (1.00); 77.6 to 92.2 (1.00)                                                                                                 | concurrent |      1 |
| columbus 2 | `1519021e` 88.6 to 115.0  | yes     | 88.6 to 110.8 (1.00); 113.1 to 115.0 (1.00)\*                                                                                            | sequential |      0 |
| columbus 2 | `3093bbf3` 106.5 to 150.9 | yes     | 106.5 to 149.6 (0.99); 114.9 to 118.5 (0.84); 146.6 to 151.8 (0.95)\*; 146.7 to 151.2 (0.94)                                             | concurrent |      2 |
| lombard 1  | `213a38d2` 0.0 to 47.0    | yes     | 0.0 to 41.8 (1.00); 41.9 to 47.0 (1.00)                                                                                                  | sequential |      1 |
| lombard 1  | `231080b9` 14.5 to 24.4   | yes     | 14.5 to 23.2 (0.95); 18.4 to 24.4 (0.91)                                                                                                 | concurrent |      1 |
| lombard 1  | `46a43312` 42.5 to 51.2   | yes     | 42.5 to 51.2 (1.00); 43.9 to 46.5 (0.57)                                                                                                 | concurrent |      1 |
| lombard 1  | `af601c58` 49.2 to 56.6   | yes     | 45.1 to 56.6 (0.77); 48.7 to 53.0 (0.58)                                                                                                 | concurrent |      1 |
| lombard 1  | `d06bac38` 52.1 to 62.7   | yes     | 51.5 to 62.6 (0.61); 52.1 to 62.7 (0.92)                                                                                                 | concurrent |      1 |
| lombard 1  | `c82036a3` 69.7 to 81.6   | yes     | 69.7 to 74.2 (0.92); 71.6 to 75.1 (0.50)                                                                                                 | concurrent |      1 |
| lombard 1  | `64870ec8` 74.9 to 89.8   | yes     | 74.9 to 89.0 (0.52); 76.7 to 89.8 (0.94)                                                                                                 | concurrent |      1 |
| lombard 1  | `495505cb` 76.0 to 88.3   | yes     | 76.0 to 88.3 (1.00); 79.8 to 86.6 (0.50)                                                                                                 | concurrent |      1 |
| lombard 1  | `4c152346` 78.1 to 91.2   | yes     | 78.1 to 84.3 (0.82); 83.6 to 91.2 (1.00)                                                                                                 | concurrent |      1 |
| lombard 1  | `379c32db` 81.2 to 91.9   | yes     | 79.5 to 88.2 (0.54); 89.0 to 91.9 (0.90)                                                                                                 | sequential |      1 |
| lombard 1  | `e3f2fb97` 81.6 to 89.5   | yes     | 80.9 to 89.5 (0.69); 81.6 to 83.3 (1.00); 83.4 to 87.7 (1.00)                                                                            | concurrent |      2 |
| lombard 1  | `2354b27c` 88.8 to 137.5  | yes     | 86.6 to 100.9 (0.72); 91.9 to 137.5 (0.93)                                                                                               | concurrent |      1 |
| lombard 1  | `98aa6b12` 138.1 to 153.2 | yes     | 138.1 to 153.0 (0.98); 149.8 to 152.9 (0.94)                                                                                             | concurrent |      1 |
| lombard 1  | `2ef09ae1` 144.0 to 153.1 | yes     | 139.2 to 153.1 (0.83); 143.9 to 147.2 (0.58)                                                                                             | concurrent |      1 |
| lombard 2  | `3d5fd58e` 5.0 to 18.6    | yes     | 5.0 to 18.6 (0.97); 8.6 to 11.3 (1.00)                                                                                                   | concurrent |      1 |
| lombard 2  | `3b537c11` 100.2 to 108.0 | yes     | 100.2 to 102.1 (0.75); 102.5 to 108.0 (1.00)                                                                                             | sequential |      1 |
| lombard 2  | `d17ee86d` 109.6 to 115.6 | yes     | 103.2 to 113.7 (0.60); 111.8 to 115.6 (1.00)                                                                                             | concurrent |      1 |

The control makes 14 joins: 3, 4, 5 and 2 a segment, 12 of them concurrent. Its columbus 1 and
columbus 2 joins carry the same `a0` tracks as the candidate's, with fewer members credited; at
lombard 1 it makes 5 where the candidate makes 14.

## M4: the merge read

Inferred merges per segment on the evidence, `a0` against itself as the floor, then each
candidate against `a0`. "Distinct pairs" counts the `a0` pairs however many candidate tracks hold
them; "new pairs" are those the floor does not hold.

| Segment    | Floor (`a0`): merges, distinct pairs | `join`: merges, distinct, new | `join` excess | `join-membership`: merges, distinct, new | `join-membership` excess | Floor + 1 holds? |
| ---------- | ------------------------------------ | ----------------------------- | ------------: | ---------------------------------------- | -----------------------: | ---------------- |
| columbus 1 | 17, 16                               | 17, 17, 1                     |             0 | 17, 17, 1                                |                        0 | Yes              |
| columbus 2 | 15, 12                               | 18, 12, 0                     |            +3 | 18, 12, 0                                |                   **+3** | **No**           |
| lombard 1  | 18, 17                               | 19, 16, 0                     |            +1 | 26, 22, 6                                |                   **+8** | **No**           |
| lombard 2  | 9, 8                                 | 10, 9, 1                      |            +1 | 12, 10, 2                                |                   **+3** | **No**           |
| Four       | 59                                   | 64                            |            +5 | 73                                       |                  **+14** |                  |

`join-membership` exceeds the floor plus one at three segments and by 14 over the four, against a
bound of two, so **M4 fails**. The control would fail it too (+3 at columbus 2, +5 over the four).
Read on the rows, which span only the scored 120 s (reported, not decided), the counts run the
other way: floor 89, 69, 255 and 127; `join` 73, 63, 237 and 114; `join-membership` 71, 56, 221 and 115. The rows read counts many more pairs because a rows track's footprint is drawn at every scored
frame, and fewer for the candidates because their duplicates in `a0`'s rows are gone; it was not
traced further.

**Where the excess comes from.** At columbus 2 the excess is the floor's own 12 pairs held by more
candidate tracks: a queue behind a long vehicle whose candidate tracks hand over to one another
counts the same pair once per track. At lombard 1 and lombard 2 the candidate holds pairs the floor
does not. Every one of `join-membership`'s nine new pairs, with the frames, the candidate box and
the two `a0` tracks:

| Case       | Candidate track | Time (s)       | Frames | Box width median / max (m) | Box length (m) | Box speed (m/s) | `a0` pair: max speeds (m/s) | Closest approach / median separation (m) | Arms              |
| ---------- | --------------- | -------------- | -----: | -------------------------- | -------------: | --------------: | --------------------------- | ---------------------------------------- | ----------------- |
| columbus 1 | `4f23cb65`      | 87.1           |      1 | 2.91 / 2.91                |          16.53 |            0.19 | 6.2, 8.0                    | 7.39 / 7.39 (1 shared frame)             | both              |
| lombard 1  | `46a43312`      | 43.9           |      1 | 1.84 / 1.84                |           4.40 |            7.96 | 11.1, 3.9                   | 2.52 / 11.86                             | `join-membership` |
| lombard 1  | `573ec999`      | 49.7           |      1 | 1.68 / 1.68                |           3.80 |            2.16 | 11.1, 9.1                   | 2.34 / 3.46                              | `join-membership` |
| lombard 1  | `8575e491`      | 69.0           |      1 | 1.71 / 1.71                |           4.32 |           10.81 | 11.1, 1.6                   | 3.23 / 24.81                             | `join-membership` |
| lombard 1  | `c82036a3`      | 71.4           |      1 | 1.65 / 1.65                |           3.80 |            4.02 | 10.2, 1.6                   | 2.51 / 2.51 (1 shared frame)             | `join-membership` |
| lombard 1  | `c82036a3`      | 71.5           |      1 | 1.65 / 1.65                |           3.80 |            4.18 | 10.2, 4.7                   | 2.49 / 11.26                             | `join-membership` |
| lombard 1  | `fff6455d`      | 76.7           |      1 | 1.64 / 1.64                |           4.55 |            9.18 | 6.6, 8.9                    | 2.26 / 24.17                             | `join-membership` |
| lombard 2  | `c44dd645`      | 25.1 to 25.5   |      4 | 3.58 / 3.67                |           5.53 |            2.13 | 7.2, 9.8                    | 2.34 / 4.52                              | both              |
| lombard 2  | `5cec029a`      | 131.4 to 131.7 |      3 | 1.43 / 1.47                |           2.93 |            8.61 | 8.7, 8.1                    | 1.67 / 2.05                              | `join-membership` |

The closest approach and median separation are the two `a0` tracks' centres over the frames they
share. In both candidates the same footprint holds the columbus 1 and the first lombard 2 pair,
under the control's own track identifiers.

- The six at lombard 1 are each one frame of a car-sized box (1.6 to 1.8 m wide, 3.8 to 4.6 m
  long) at the moment two of `a0`'s tracks pass closest, 2.3 to 3.2 m apart. In four of them one
  `a0` track is slow (a maximum of 1.6 to 4.7 m/s) and short (6.5 to 23.4 m first to last): three
  distinct tracks.
- At lombard 2, 25.1 to 25.5 s, both candidates draw a box 3.6 m wide for four frames over a slow
  track 6 m from the sensor (`a0` `2e471016`, 44 s, a maximum of 7.2 m/s) and a vehicle passing it at
  9.8 m/s, 2.3 m apart at closest. Of the nine, this is the one that reads as two vehicles side by
  side in one footprint.
- At lombard 2, 131.4 s, the two `a0` tracks run together for 23 s, 1.7 to 2.1 m apart, with only
  12 shared frames: inferred, a duplicate in `a0` that alternates between two tracks, which the
  overlap test misses because its boxes overlap little on the frames both are drawn.
- At columbus 1 a 16.5 m box (a bus or a vehicle with a long footprint, moving at 0.2 m/s at that
  frame) holds two `a0` tracks 7.4 m apart for one frame: a vehicle in a queue behind it.

The floor's own merges have the same shapes: at columbus 1, 9 of the floor's 17 are 17.5 to
18.6 m boxes over a queue at 143.6 to 146.0 s, 8 of them one track's.

### Pooled over the four segments

| Arm               | Brief | Clutter | Moving, peak | Moving, median | Dup pairs | Pair-frames | Inferred merges |
| ----------------- | ----: | ------: | -----------: | -------------: | --------: | ----------: | --------------: |
| `a0`              |   221 |     249 |          520 |            486 |       115 |       3,118 |      59 (floor) |
| `join`            |   219 |     246 |          511 |            478 |       103 |       3,146 |              64 |
| `join-membership` |   216 |     242 |          503 |            469 |        89 |       2,603 |              73 |

Spurious clusters a frame fall in both candidates at every segment, by 6 % to 13 % for
`join-membership` (0.886 to 0.771 at columbus 1, 1.866 to 1.688 at lombard 1). Joins per frame are
not read on the placements: box-eval does not write the join counter, which only the baseline tool
reports (kirk0 above).

## Against the rule

| Clause  | `join-membership`                                                                               | `join` (control, decides nothing)           |
| ------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------- |
| M1      | Pass: HOTA 5th −0.0002, IDF1 5th +0.0004, switch 95th 0                                         | Pass: −0.0013, +0.0004, 0                   |
| M2      | Pass: noise costs on 1 of 4 (duplicates per hundred 11.4 → 11.6 at lombard 2); no mover lost    | Pass: no cost on any segment; no mover lost |
| M3      | **Fail**: lombard 1's credited peak count +6.6 % (bound 5 %); p85 within 0.39 m/s               | Pass: +3.1 % at most; p85 within 0.39 m/s   |
| M4      | **Fail**: above the floor plus one at 3 of 4 (+3, +8, +3); 14 in excess over the four (bound 2) | Would fail: +3 at columbus 2; 5 in excess   |
| Outcome | Not named for label0003's window. Nothing adopted                                               |                                             |

## Interpretation

Each reading is marked measured or inferred.

- **Track-conditioned membership keeps its noise gain off kirk0.** Measured: on the busiest
  segments read this month it adds no brief tracks, takes duplicate pairs from 115 to 89 and pair
  frames from 3,118 to 2,603 pooled, and lowers spurious clusters a frame at every segment, more
  than the join alone does. The noise clause's one cost, at lombard 2, is three fewer vehicle
  tracks over the same 19 pairs.
- **On a busy arterial it doubles vehicles it was meant to join.** Measured: at lombard 1 it makes
  14 joins, 12 of them an `a0` duplicate removed, and carries 15 of `a0`'s moving vehicles in a
  second moving track, 10 of them short concurrent doubles. Inferred: a wider footprint absorbs a
  neighbour's cluster into one track on one frame and leaves its usual track starved, so a second
  track is born on the same vehicle a frame later. That is the cost l5id0024 found for the
  likelihood birth gate, in a smaller amount; the control, with the narrower footprint, makes 9
  such tracks there, 3 of them doubles.
- **The merge read finds contact more than merging.** Measured: of the candidate's nine new pairs,
  seven are held for one frame at the two `a0` tracks' closest approach, six by a footprint of
  ordinary size and one by a 16.5 m box over a queue. Inferred: these are footprints of one vehicle that touch a second at a crossing
  or in a lane change, not one footprint drawn around two. One new pair, held by both candidates,
  is a 3.6 m box over a slow vehicle and a passing one for four frames: a merge in the sense the
  spec means. The rest of the excess counts the floor's own queue pairs once per candidate track.
  The read as written counts all three alike, so M4's failure is a failure of the clause, not proof
  that the candidate merges vehicles.
- **The floor is high and shaped by long vehicles.** Measured: `a0` against itself gives 59 merges
  over the four segments, 9 of columbus 1's 17 from 18 m footprints over one queue. A bound of
  the floor plus one is narrow against a floor of that size, and the per-track count moves with
  any change in how a queue's tracks hand over.
- **The two clauses fail at the same place.** Measured: lombard 1 carries both M3's +6.6 % and
  M4's +8. Inferred: the doubles and the new contacts come from the same busy stretch (the new pairs lie at
  43.9 to 76.7 s, inside the 42 to 92 s where 10 of its 14 joins begin), where traffic runs closest.

## Recommendations

1. **Do not carry `join-membership` to label0003's window.** It fails M3 and M4 as written. The
   noise gain is real, but on a busy arterial it doubles vehicles, and the merge read cannot clear
   it without a reference.
2. **Read the merge reference before another merge clause.** label0001's masks on an adjacent
   pair would say which of the nine new pairs are two vehicles in one cluster. The lombard 2 box
   at 25.1 to 25.5 s is the case to look at first, since both join arms draw it.
3. **Count merges by pair, not by candidate track, and require more than a frame.** A clause that
   counts distinct `a0` pairs held for two or more frames by a footprint wider than a lane's share
   would separate the one merge-like case from contacts and queue hand-overs. That is a new rule
   for a new spec, written before its read, not a re-reading of this one.
4. **Charge the doubles in the count clause.** As l5id0024 recommended, a credit for joins is honest
   only beside a charge for the second tracks a candidate adds; at lombard 1 they balance (15 and
   15), which the credit alone reads as a 6.6 % rise.
5. **Look at the doubles at lombard 1 before tuning the footprint.** A tap on when the second track
   is born (the frame after a widened footprint absorbs a neighbour's cluster, or not) would say
   whether the membership width or the join gap makes them.
6. **Keep everything default-off.** kirk0 is in sample, and the two placements have now been read
   by two candidates.

## What this does not establish

- **Physical truth on the placements.** The segments have no labels. Merges, joins, doubles and
  counts are what the pipeline drew, not where road users were. That a footprint holds two vehicles
  is read from centres and boxes, which is why the floor is read the same way.
- **That any inferred merge is a merge.** The read infers; no merge was confirmed against masks.
- **That the doubles are vehicles.** They are moving candidate tracks over `a0` moving tracks.
- **A held-out read.** label0003's window was not read.
- **A reading of other sites.** Two placements drawn by lot from 21 are a thin screen; the busy one
  decided.
- **The cause of the doubles.** It is inferred from their timing, not traced in the tracker.
- **Pi cost.** No timing was read; the host was shared with another experiment.
- **The rows read.** Its merge and count figures were reported, not traced.

## Provenance

| Item           | Value                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base           | `exp/l4clus0012` from `0a9316f` (spec approved, rule committed before any replay)                                                                                                                                                                                                                                                                                                                                                                                                            |
| Tooling        | `9ac2f49` (`lidar_merge_read.py` and its tests, committed before any replay); `python3 -m unittest test_lidar_merge_read` passes (10 tests); `lidar_flip_cost.py` and `lidar_figure_sensitivity.py` unchanged, their tests pass                                                                                                                                                                                                                                                              |
| Tools          | Built at `0a9316f` (`0a9316f993b2bb5e8261ea54e190c7234732bffe`) by the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA`, `BuildTime`) with the `pcap` tag, against the run's static libpcap (submodule `a999701d`). Every replay header and box-eval summary reads `build_version` 0.5.1-pre52 and `build_git_sha` `0a9316f9…`. `lidar-state-estimation-baseline` `sha256:bbb34db1f57b6fba…`, `lidar-box-eval` `ea984cb1e9c61313…`, `lidar-ground-truth-eval` `3c574b7b074c3211…` |
| Parameter sets | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`; `config/experiments/motion-fragment-join.json`, fingerprint `719d291ed197`, file SHA-256 `2e604040…`                                                                                                                                                                                                                                                                                                    |
| Arm hashes     | `a0` `ab0c4035780d`; `join` `719d291ed197`; `join-membership` `f17eae8f1ac6`                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Baselines      | Tracking baseline digests, first and repeat equal: `a0` `79d9abff…`, `join` `9dc26717…`, `join-membership` `9a93b79b…`                                                                                                                                                                                                                                                                                                                                                                       |
| Capture        | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`                                                                                                                                                                                                                                                                                                                                                                                                             |
| Placements     | Hugging Face `banshee-data/sf-street-speeds`: columbus-north-point `20260902T1153_80858-0e4`, `sha256:f6f4a4993a2a83a1…8389`; lombard-broderick `20260902T0938_80858-0d4`, `sha256:1eb08d9e6ced4e20…4643`; l5id0024's lot                                                                                                                                                                                                                                                                    |
| Segments       | Cut by `pcapng_window.py` at 0 and 300 s for 300 s, each equal to l5id0024's: columbus 1 `ceeb92abc1c45467…e1e0`, 2 `7dc184577c8b2867…7b98`; lombard 1 `a38d974e77fc69a7…77aa`, 2 `667e235853dfa873…33c3`                                                                                                                                                                                                                                                                                    |
| Labels         | The pack RECONSTRUCTED as in l5id0019 and l5id0024: label set `kirk0-reference` revision 2233, pack `ds_67842cb1b22b70e7` rebuilt by `lidar-pack-rebuild` (pack digest `sha256:f0ed3414…`); split `kirk0-tuning` manifest `7194bb61…`, reference digest `sha256:a784e69d…`; `obj_788622b5` ignored                                                                                                                                                                                           |
| Eligibility    | Committed cohort manifest; a copy of the run's scratch use ledger (migration 71) per arm; actor `appius-l4clus0012`                                                                                                                                                                                                                                                                                                                                                                          |
| Host           | Claude Code cloud container, 4 cores, shared with another experiment; two lanes, `GOMAXPROCS=2`, `nice -n 5`; kirk0 replays 1.7 to 2.0 min each with repeat, segments 2.0 to 4.6 min each                                                                                                                                                                                                                                                                                                    |
| Raw output     | Scratch only. Captures were deleted once cut and replayed; evidence databases, rows and summaries were deleted once these tables were written                                                                                                                                                                                                                                                                                                                                                |
