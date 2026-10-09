# Box containment on kirk0 and the corpus: the W2 screen

<!-- ignore-style-length -->

The [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md)'s
second observation model, `solid_body_containment` (W2), floors the reported box at the
frame's observed spans and holds the body's position to the interval its points allow. This
report scores it alone and with the alignment candidate, on the frozen kirk0 split and on the
23-site label-free corpus, against the levels the
[criteria record](geometry-convergence-criteria.md) predeclared.

- **Status:** Complete, 2026-10-09. Corpus screen and kirk0 timing read; the cost breach found and fixed on the same branch, within G12 on A2 and 15 % over on the shadow
- **Layers:** L5 solid body
- **Related:** [convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md#42-a-containment-constraint-the-box-holds-the-points) (W2), [criteria record](geometry-convergence-criteria.md), [alignment report](solid-body-physical-alignment-kirk0-2026-10.md), [kirk0 pilot](physical-reference-pilot-kirk0-2026-10.md)

## Summary

- **Lapses halve everywhere.** Alone on A2, the solid body lapses to the medoid 113 fewer times
  per site at the median, better at all 23 sites; with the alignment candidate, 101 fewer, all 23.
  The rows kept as body centre instead are the mechanism behind every other figure here.
- **The lateral tail over every scored window falls.** The five-point lateral p99 over all
  windows, body centre and medoid alike, a population the change leaves the same size, falls
  0.070 m at the corpus median alone (19 sites better, 2 worse) and 0.074 m with the candidate
  (16 better, 5 worse); p95 falls 0.03 and 0.02 m; the share of tracks with an excursion falls
  at 19 and 15 sites.
- **The body-centre p99, the predeclared G10 figure, rises at most sites**, by 0.007 m alone
  (13 worse, 8 better) and 0.017 m with the candidate (17 worse, 4 better). On the windows both
  arms score it is unchanged: pooled over six sites, 2,976 matched windows go from 0.205 to
  0.212 m at p99, with a median paired difference of zero. The rise is the windows the hold adds,
  frames that used to lapse, whose p99 is 0.43 m on their own; they replace medoid rows that
  were worse still, which is why the all-window tail falls.
- **G5 and G6 are not met by construction**, as the criteria record assumed. The held share
  (body-centre rows holding 98 % of their points) is 0.60 alone and 0.66 with the candidate at
  the corpus median, against a level of 0.95; the reported length is short of the frame's span
  in 2.5 % and 1.8 % of rows, against 0. The floor is the window-minimum span and the spans are
  trimmed, so a frame whose cluster grows past the window minimum is not held.
- **On kirk0 nothing physical moves except the extents and gaps.** Car 8's length goes from
  3.88 to 4.05 m of 4.26 (all 7 poses within 15 %, from 5), truck 2's reported length from 0.63
  to 1.96 m of 9.63, truck 1's from 5.88 to 6.38 m of 10.63; the mean box IoU from 0.41 to 0.46;
  car 8's gap behind truck 1 from 0.27 to 0.14 m mean error. Centre, yaw and gaps within bound
  are unchanged. Identity: 93 switches against 100 alone, 118 with the candidate, both inside
  the predeclared band of 19.
- **The cost breaches G12.** In balanced order on kirk0, `Tracker.Update` p99 goes from 4.28 to
  7.30 ms on A2 (+71 %) and from 3.06 to 6.18 ms on the shadow (+102 %); the median triples
  (0.137 to 0.416 ms, 0.169 to 0.481 ms); a frame's p99 rises 7 to 8 %. The level is 10 % at
  p99. A CPU profile of the replay puts the whole of the increase in the floor's span search
  (`trimmedSpan` through `NthFloat64`, 0.13 to 0.35 s of the replay's CPU; the tracker's share
  of the replay 1.1 to 2.1 %): 21 angles times two order statistics per axis, 42 selections
  over the cluster's full membership every frame. The fix, below, searches the angle on a
  256-point subset and reads the span on every point at the angle found: A2's p99 is then
  4.27 to 4.15 ms (−3 %), the shadow's 3.07 to 3.53 ms (+15 %), and both medians stay about
  2.5 times the arm without containment; kirk0's geometry under the fix is identical.

## Question

Does holding the box to its points lower the lateral tail and the lapse count without raising
either anywhere, and do the per-row guards show the constraint running as designed?

## Method

- **Arms.** `a2`: `solid_body`, `solid_body_face_hysteresis`, `solid_body_course_faces`,
  `solid_body_full_members`, `near_edge_track`. `a2-cont`: the same with
  `solid_body_containment`. `a2-allo`: `a2` with the alignment candidate's four experiments
  (`solid_body_course_heading`, `solid_body_extent_growth`,
  `solid_body_end_face_centring_open_prior`, `solid_body_vehicle_extent_floor`).
  `a2-allo-cont`: `a2-allo` with containment. On kirk0 also the shadow with and without it.
- **What containment does.** The reported length and width are never below the frame's
  window-minimum spans (the smallest 1 %-trimmed extent over axes within 10° of the believed
  one, which understates along a wrong axis and cannot overstate); the extent beliefs never read
  the floor. On the body axes a fix left open, the position's marginal is truncated to the
  interval [hi − h, lo + h] with h the belief's upper tail, and the truncated moments are applied
  through the state's regression on that direction. A body-centre claim no longer lapses while
  the cluster has enough points to hold it. A merge candidate gets neither. Every row records the
  floor it took, the shift, its containment share and its observed spans.
- **kirk0.** Frozen split `kirk0-ad8b9438-tuning-r1`, episode `following-472-530` for physical
  scoring, `whole-pack` for identity, as the pilot and the alignment report scored them.
- **Corpus.** The D2 first-segment corpus, 23 tuning and screen sites (`embarcadero-folsom`
  held out), 200 s scored after a 70 s warm-up, pass 4 of the alignment run's harness: the two
  containment arms replayed at a91db3862, compared with the earlier passes' `a2` and `a2-allo`
  at 17f2719e9 and be79fbd53, whose A2 control replay of kirk0 is identical across those builds.
- **Two lateral populations.** The predeclared G10 figure is the five-point lateral p99 over
  windows of five consecutive body-centre rows. Containment moves rows from medoid to body
  centre, so that population grows at 21 to 23 sites (+47 and +55 windows at the median) and is
  not the same windows before and after. The p99 over every scored window (body centre and
  medoid) keeps its size within a handful of windows (+1 and +8 at the median) and is read
  beside it. The matched-window check reads the two arms' evidence databases, builds the same
  windows, pairs them by centre frame time and position within 1.5 m, and reads the paired
  windows, the windows only the containment arm has, and the windows it lost, apart.

## Findings on kirk0

A2, alone and with the candidate; the shadow beside them. Rows are the 1,918 to 1,951
solid-body rows of the kirk0 replay.

| Measure                                  | A2                  | A2 + containment   | Candidate          | Candidate + containment | Shadow             | Shadow + containment |
| ---------------------------------------- | ------------------- | ------------------ | ------------------ | ----------------------- | ------------------ | -------------------- |
| Lapses                                   | 49                  | 26                 | 62                 | 27                      | 55                 | 27                   |
| Medoid rows                              | 996                 | 810                | 991                | 831                     | 937                | 709                  |
| Held share, body centre (rows)           | 0.18 (928)          | 0.41 (1,108)       | 0.29 (940)         | 0.40 (1,103)            | 0.20 (1,014)       | 0.34 (1,242)         |
| Rows floored / shifted                   | 0 / 0               | 919 / 416          | 0 / 0              | 773 / 332               | 0 / 0              | 919 / 434            |
| Median shift                             |                     | 0.024 m            |                    | 0.008 m                 |                    | 0.023 m              |
| Length short of the span (share of rows) | 0.45                | 0.036              | 0.24               | 0.037                   | 0.39               | 0.037                |
| Width short                              | 0.20                | 0.068              | 0.21               | 0.048                   | 0.19               | 0.036                |
| Lateral p99, body-centre windows (n)     | 0.291 (220)         | 0.156 (228)        | 0.152 (174)        | 0.156 (176)             | 0.532 (214)        | 0.175 (234)          |
| Excursion share, body-centre windows     | 0.20                | 0                  | 0                  | 0                       | 0.29               | 0                    |
| Lateral p99, all windows (n)             | 0.324 (283)         | 0.254 (270)        | 0.616 (258)        | 0.623 (233)             | 0.311 (296)        | 0.311 (296)          |
| ID switches / HOTA / IDF1                | 100 / 0.310 / 0.285 | 93 / 0.306 / 0.275 | 89 / 0.310 / 0.297 | 118 / 0.310 / 0.272     | 96 / 0.356 / 0.330 | 96 / 0.353 / 0.330   |

Physical, medians over scored poses: truck 1's centre 2.19 m in both A2 arms and 1.00 to 0.91 m
in the candidate arms; car 8's 0.25 and 0.23 m; truck 2's 2.00 and 1.93 m. Yaw unchanged in
every pair. Lengths: car 8 3.88 to 4.05 m (A2) and 3.88 to 3.91 m (candidate) of 4.26; truck 1
5.88 to 6.38 m and 8.63 to 8.88 m of 10.63; truck 2 0.63 to 1.96 m and 0.63 to 0.75 m of 9.63.
Poses within the centre bound: truck 1 2 of 8, car 8 3 of 5, truck 2 0 of 6 on A2 with or
without containment; 7 of 8 or 9, 4 of 6, 0 of 6 on the candidate. Gaps within bound 3 of 7 in
all four A2 arms; mean gap error 1.15 to 0.91 m (A2) and 1.60 to 1.56 m (candidate). The
shadow's all-window lateral is identical with and without containment because its point
estimates are the tracked filter's, which the shadow does not touch.

## Findings on the corpus

Median per-site change, with the count of sites worse / better, over 23 sites.

| Measure                       | Containment on A2 | Containment on the candidate |
| ----------------------------- | ----------------- | ---------------------------- |
| Lapses                        | −113 (0 / 23)     | −101 (0 / 23)                |
| Medoid share of rows          | −0.035 (0 / 23)   | −0.033 (1 / 22)              |
| Body-centre windows           | +47 (0 / 23)      | +55 (2 / 21)                 |
| Body-centre lateral p95       | +0.011 m (15 / 8) | +0.009 m (17 / 5)            |
| Body-centre lateral p99 (G10) | +0.007 m (13 / 8) | +0.017 m (17 / 4)            |
| Body-centre excursion share   | 0 (8 / 8)         | 0 (7 / 5)                    |
| Steady-run lateral p99        | +0.022 m (14 / 7) | +0.034 m (16 / 4)            |
| All windows                   | +1 (11 / 12)      | +8 (7 / 15)                  |
| All-window lateral p95        | −0.032 m (1 / 21) | −0.023 m (4 / 18)            |
| All-window lateral p99        | −0.070 m (2 / 19) | −0.074 m (5 / 16)            |
| All-window maximum            | −0.17 m (7 / 14)  | −0.08 m (5 / 14)             |
| All-window excursion share    | −0.036 (2 / 19)   | −0.020 (3 / 15)              |
| Near-edge fix share           | −0.003 (12 / 11)  | −0.003 (15 / 8)              |
| Clusters unassigned, share    | +0.0014 (14 / 8)  | +0.0014 (17 / 6)             |
| Tracks; median lifetime       | 0; 0              | 0; 0                         |

The guards under containment, corpus median (range over sites): held share on body-centre rows
0.595 (0.45 to 0.80) alone and 0.663 (0.50 to 0.81) with the candidate; over all rows 0.69 and
0.70; rows floored 0.34 and 0.32; rows shifted 0.12 and 0.13 with a median shift of 0.014 and
0.009 m; length short of the frame's span 0.025 (0.008 to 0.058) and 0.018 (0.002 to 0.053);
width short 0.058 (0.015 to 0.156) and 0.040 (0.016 to 0.101).

**The matched windows.** At the six sites where the body-centre p99 rose most on A2
(lombard-broderick, hyde-ofarrell, embarcadero-bay, bush-powell, 1st-mission, lombard-laguna),
2,976 windows pair across the arms: p95 0.071 to 0.076 m, p99 0.205 to 0.212 m, median paired
difference 0.000 m. The 687 windows only the containment arm scores have p95 0.221 m and p99
0.428 m; the 663 windows it no longer scores had p99 0.263 m. On the candidate at four sites
(lombard-broderick, hyde-ofarrell, california-leon-baker, franklin-mcallister), 1,606 matched
windows go 0.177 to 0.176 m at p99 and the 302 added windows have p99 0.459 m. At
hyde-ofarrell the row mix shows what was added: body-centre rows with a fix 4,522 to 4,689,
body-centre rows kept without one 135 to 434, medoid rows 5,409 to 4,912, lapses 138 to 44.
Pooled over the 23 sites' 212,000 rows on A2: body-centre rows with a fix 43.1 to 42.5 %, kept
without one 2.5 to 6.5 %, medoid 54.4 to 51.0 %, lapses 4,707 to 2,168; under containment
32.9 % of rows took a floor and 12.3 % a shift. The matched-window read over all 23 sites
failed on a disk I/O error on the LiDAR volume while the timing ran; the six-site and
four-site figures above are what was read.

**Merge-heavy sites**, the screen the plan named for containment pulling toward merged
clusters: at lombard-laguna the all-window p99 falls 0.758 to 0.531 m alone and 0.740 to 0.633 m
with the candidate; at 1st-mission 0.517 to 0.322 m alone, but 0.366 to 0.398 m with the
candidate, with the maximum rising from 0.98 to 1.64 m. That one site-arm is the only place the
all-window tail got worse by more than 0.03 m.

**Largest single moves** in the body-centre p99: ashbury-downey 0.684 to 0.182 m and
columbus-north-point 0.263 to 0.169 m alone; lombard-broderick 0.215 to 0.279 m alone and
0.186 to 0.314 m with the candidate, hyde-ofarrell 0.106 to 0.263 m and california-leon-baker
0.106 to 0.213 m with the candidate, each of which the matched windows show to be the added
windows, not the shared ones.

## Against the predeclared levels

| Id  | Level                                  | Alone on A2 (kirk0; corpus median)                                                                                    | With the candidate                  | Reading                                                                   |
| --- | -------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ----------------------------------- | ------------------------------------------------------------------------- |
| G5  | Held share ≥ 0.95                      | 0.41; 0.60                                                                                                            | 0.40; 0.66                          | Not met; not met by construction as the record assumed                    |
| G6  | Reported box short in 0 rows           | Length 0.036, width 0.068; 0.025, 0.058                                                                               | 0.037, 0.048; 0.018, 0.040          | Not met; the test reads the frame's span, the floor is the window minimum |
| G10 | Candidate p99 not above the baseline's | kirk0 0.291 to 0.156 m; corpus +0.007 m (13 / 8)                                                                      | 0.152 to 0.156 m; +0.017 m (17 / 4) | Not met as written; met on the same windows and on all windows            |
| G11 | Within 19 switches of 100              | 93                                                                                                                    | 118                                 | Met, both                                                                 |
| G12 | `Tracker.Update` p99 within 10 %       | 4.28 to 7.30 ms (+71 %) as screened; 4.27 to 4.15 ms (−3 %) with the floor's search capped; shadow +102 %, then +15 % | Not timed                           | Met on A2 after the fix; not on the shadow; the Pi unmeasured             |

G7 (centre), G8 (length) and G9 (gaps) are not W2's to move and did not: the within-bound counts
are the same with and without containment in every pair.

## The cost, and its fix

The floor's window-minimum span is the smallest trimmed extent over 21 angles within 10° of
the believed axis, and a trimmed extent is two order statistics, so the floor was 42
selections over the cluster per axis pair per frame. Under `solid_body_full_members` the cluster
is its whole membership, a thousand returns and more on a near body, and the profile above puts
the whole of the increase there. The fix (`floorPoints`, `floorSpan`): the angle is searched on
a 256-point subset drawn at random with the count as the seed, so a frame's floor is
reproducible and no ring or azimuth order can leave a face out, and the span is then read on
every point at the angle found. The span at any angle in the window is at least the window's
minimum, so the floor is never below the whole cloud's window minimum, and it overstates it by
at most the span's change over the search's one-degree step. The containment share, the
observed spans and the truncation still read every return.

`Tracker.Update` on kirk0, balanced order, median of eight runs each:

| Arm                  | p99 as screened | p99 with the fix | p50 as screened | p50 with the fix | Frame p99 with the fix |
| -------------------- | --------------- | ---------------- | --------------- | ---------------- | ---------------------- |
| Shadow               | 3.06 ms         | 3.07 ms          | 0.169 ms        | 0.173 ms         | 54.2 ms                |
| Shadow + containment | 6.18 (+102 %)   | 3.53 (+15 %)     | 0.481           | 0.435            | 54.6                   |
| A2                   | 4.28            | 4.27             | 0.137           | 0.135            | 54.3                   |
| A2 + containment     | 7.30 (+71 %)    | 4.15 (−3 %)      | 0.416           | 0.336            | 55.2                   |

A2 is inside G12 at p99 and the shadow is 15 % over; the medians remain about 2.5 times the
arm without containment, which is the subset's 42 selections, the two full selections, the
truncation's extremes and the hold. kirk0 replayed under the fix (`c5-a2-cont-cap`) is the
screened arm to the row: 1,918 rows, 848 fixes, 26 lapses, 810 medoid rows, the same 228
body-centre windows at p99 0.156 m and the same 270 windows at 0.254 m; the only differences
are 924 floored rows against 919 and a held share of 0.407 against 0.405, from the few frames
whose floor now sits a return's spacing higher. The next lever, if the shadow must meet the
level, is a coarse-to-fine angle search in the floor's path alone (five angles then four), which
is not done here because the faces share the span search and a change to it would move A2's
control.

## Interpretation

- Holding the claim instead of lapsing is the whole effect. The hold keeps frames with too few
  face points as body-centre rows at the predicted-and-truncated position, which is a worse
  position than a fixed frame's (p99 0.43 m against 0.21 m on the same sites) and a far better
  one than the medoid the row used to carry. Every "worse" body-centre figure is this population
  change, and every all-window figure is its net effect.
- The constraint does not fight the filter: on windows both arms score the paired difference is
  zero at the median and the tails are within 0.01 m, which is what bounding the truncation by
  the state's own σ was meant to give.
- The held share is far from 0.95 because the floor is deliberately a lower bound. The
  window-minimum span understates a growing or turning cluster, and the trimmed extremes leave the
  outermost 1 % of points outside by design; at hyde-ofarrell the rows that took a floor hold
  0.88 to 0.93 of their points on average, the rows that did not 0.986. Which of the three
  (window minimum, trim, position held on one axis only) accounts for most of the gap is not
  attributed here; the per-row record distinguishes floored rows, so W4 can read it.
- With the candidate, the switches move to 118, inside the band but at its edge, and
  1st-mission's maximum grows. Containment on top of growth admission reaches merged clusters the
  merge test admitted; the plan's W4 reconciliation is where that belongs.

## What this cannot establish

- A rate: three vehicles on one tuning capture, and a corpus with no references.
- Whether the added windows' 0.43 m tail is the hold's position or the frames themselves; the
  medoid rows they replaced cannot be scored on the same windows because they were not body
  centre.
- The cost on a Pi, and with the candidate on the Mac; the median's 2.5 times is a Mac figure
  for an update that is 1 to 2 % of a replay's CPU, and the Pi's share is unknown.
- Anything after P11 or W3, which move G-GEO-1's inputs.

## Recommendations

- Record two amendments in the criteria record rather than silently re-reading the levels: G10's
  operational test gains the all-window p99 and the matched-window comparison as its
  population-stable forms; G5 and G6 state the floor's definition and set the level on the
  floored row's share, not 0.95, or change the floor. Neither is adopted here; both are the
  user's call.
- Score W2 with W1b next, as the plan sequences it, and read 1st-mission under the candidate
  when W4 reconciles containment with growth admission.
- Add the hold to the row's fallback vocabulary so the held rows can be stratified without
  joining two evidence databases.

## Provenance

- **Code.** Branch `claude/geometry-convergence`; the containment arms at a91db3862 (`W2 containment: kirk0 result in the plan, backlog and devlog`), whose `baseline` binary is SHA-256 `dd99794d…0adc14`; `a2` and `a2-allo` from the alignment run's passes 1 and 3. kirk0 arms `c2-a2`, `c4-a2-cont`, `c2-a2-allo`, `c4-a2-allo-cont`, `c4-sh`, `c4-sh-cont` in the scratch ledger.
- **Corpus pass 4.** 2026-10-09 17:37 to 18:39 UTC, 46 replays, all sites `ok`; evidence under `work/velocity-campaign/physical-align-corpus-20261009/<site>/{a2-cont,a2-allo-cont}` on the LiDAR volume, scripts `corpus/summary2.py`, `corpus/paired.py` and `corpus/holdrows.py` under `~/vr-scratch/physical-align`.
- **References.** Pack `ad8b9438-c1d4-40f0-bd0a-f1852c984569`, split `kirk0-ad8b9438-tuning-r1`.
- **Timing.** `timing3.sh`: shadow, shadow with containment, A2, A2 with containment, forward then reverse, twice, each run waiting for a one-minute load under 3.5; medians of eight `tracker_timing.json` per arm, nothing else running. As screened at a91db3862 under `timing-a91db3862`; with the fix, built from this branch's working tree before its commit and stamped `w2cap-wip` (SHA-256 in `bin/sha256.txt`), under `timing-w2cap`; the profiles under `prof-a91db3862`. The capped arm `c5-a2-cont-cap` and its score against `c2-a2` are in the same scratch tree.
