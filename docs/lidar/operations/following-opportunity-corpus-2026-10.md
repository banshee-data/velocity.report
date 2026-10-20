# Both parties observed (B1): following opportunity on the corpus

<!-- ignore-style-length -->

The behaviour plan's B1 asks how often both parties of an interaction are observed at a real
roadside site, and calls it the question nothing else in the plan matters as much as. It had
never been measured. This report counts, label-free, the vehicle following the tracker sees on
the first five-minute segment of 23 corpus sites, under the defaults and under one track per
vehicle, and ranks the sites. It answers [l8fol0001](../../../data/experiments/l8fol/l8fol0001.md).

- **Status:** Measurement complete, 2026-10-20. One unattended run (exp-20261020); no default
  changed and no rule passes or fails
- **Layers:** L5 (tracks), read by the L8 segment finders
- **Related:** [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md#13-open-research-questions),
  [0.5.2 review R5](0.5.2-sprint-review.md#r5-encounter-totals-are-not-unique-follower-opportunity),
  [experiment review, October 18](experiment-review-2026-10-18.md),
  [duplicate tracks](l5-duplicate-tracks-2026-10.md),
  [replay determinism](replay-determinism-2026-10.md)
- **Parameter set:** the [defaults](../../../config/tuning.defaults.json) for both arms; one track
  per vehicle is the replay experiments `overlap_end_1`, `overlap_end_body`, `overlap_end_older`

## Answer

**Both parties are observed at almost every site, often enough to measure.** Over 100 minutes of
tracker output (23 sites, about 4.4 minutes of tracks each), the standard `following`
selector counts 2,111 pair-seconds of following under one track per vehicle: about 21 for every
minute of capture, 8.3 per vehicle-track-minute. Counting only pairs in which both tracks are
vehicle-sized, 1,249 pair-seconds, 4.9 per vehicle-track-minute, and a median site of 4.4. One
site of 23 (pierce-haight) has none; three more have under 10 vehicle pair-seconds.

**Most pairs are brief.** A track pair's following lasts a median 0.5 s (5 frames), and only 666
of 2,264 pairs (29 %) last a second or more; the median closest gap is 18 m, with 140 pairs
closer than 5 m. A pair is a pair of tracks, so a vehicle split between fragments makes several
short pairs out of one following episode; the counts are an upper bound on pairs and say little
about episode length.

**Duplicates hardly touch the standard counts but halve the near pairs and cut leader changes by
two fifths.** One track per vehicle changes pooled pair-seconds by under 1 % and pairs by 8 %,
because the selector already ignores gaps under 3 m, where a duplicate sits. Read with the minimum
gap at 0 m, pairs closer than 1 m fall from 172 to 93 and pairs closer than 3 m from 487 to 335;
leader changes fall from 18.1 to 11.0 per minute. A duplicate shows up as a leader that flickers
between two tracks of one vehicle, not as a follower a metre behind.

**Where to spend reference hours.** By vehicle pair-seconds the three Embarcadero sites lead
(Broadway, Bryant, Bay), then Lombard at Laguna and at Broderick, and Van Ness at Sacramento. The
site that led on all pairs, marina-webster-beach, leads only because of a 20-second column of five
non-vehicle tracks on the waterfront path, probably cyclists or runners; by vehicle pairs it is
fifteenth. The lot-drawn held-out site, embarcadero-folsom, was not read (see below).

## Question

The spec's reading, written 2026-10-18 before the replays, with its definitions fixed
2026-10-20 before the replays:

1. pair-seconds per vehicle-track-minute under each arm, and the share `body-older` removes;
2. the closest gap per pair under `body-older`, with pairs under 1 m listed apart;
3. followers and leader changes per minute;
4. the ranking of sites by pair-seconds under `body-older`.

A measurement: no default changes and no rule passes or fails.

## Method

**Replays.** Each of the 23 sites' first segment was replayed for 200 s after a 70 s warm-up with
`lidar-state-estimation-baseline` (256 sample points), once under the defaults (`a0`) and once
under l5id0004's one track per vehicle (`body-older`), each into its own evidence database.

**The screen.** On each evidence database, `velocity lidar segments` with the standard
`following`, `leader_changes` and `exposure` selectors (every 10 s window, summed per case), then
`following` again with the minimum gap at 0 m for item 2. A Python replica of the finder gives the
closest gap and frame count per (follower, leader) pair; it reproduces the CLI's pair-frame total
exactly at every case. A vehicle-track-minute (VTM) is a minute of life of a vehicle-sized track:
one lasting 2 s or more whose associated clusters' longer box side has a p90 of 3.0 m or more
(l3bg0023's screen). A vehicle pair is one whose follower and leader are both vehicle-sized.

**The selector.** Followers and leaders moving at 3 m/s or more, headings within 20°, the leader
3 to 40 m ahead along the follower's heading and within 2.5 m across it. It reads track centres
and does not look at class, so a pair of cyclists counts as following.

**Not read.** embarcadero-folsom, the 24th corpus site, is the lot for label0001's held-out
window and no screen has read it; it stays unread here, so the spec's "lot-drawn held-out site,
read last and reported apart" is not reported. The second segments stay held out for this
measure.

**Ten cases did not repeat under `body-older`.** The harness replays each case twice and refused
10 of the 23 `body-older` cases because the repeat differed: the overlap end chose its survivor in
Go map order. That is fixed on its own branch
([determinism report](replay-determinism-2026-10.md#a-fourth-mechanism-the-overlap-ends-order-october-20)).
Those ten (1st-mission, columbus-broadway, columbus-north-point, embarcadero-broadway,
embarcadero-bryant, franklin-mcallister, fulton-divisadero, hyde-ofarrell, laguna-eddy,
lombard-broderick) are screened from the first run's evidence, which the harness writes before it
repeats; each is one draw of the survivor choice. Their `a0` cases repeated.

**Replicated after the fix.** The whole `body-older` arm was replayed again with the fix
(`d6b0cf2c6`, on `claude/overlap-end-determinism`): every one of the 23 cases passed the repeat
check, 18 sites screen identically, and the pooled figures move by under 0.3 % (pair-seconds 2,111
to 2,110, pairs 2,264 to 2,266, leader changes 1,104 to 1,102, pairs closer than 1 m 93 to 92,
vehicle-track-minutes 254.4 to 254.8). The tables below are the first runs'; the survivor draw
does not change any reading.

## Findings

### Pooled

| Measure                               | a0            | body-older    |
| ------------------------------------- | ------------- | ------------- |
| Minutes read                          | 100.3         | 100.3         |
| Vehicle-track-minutes                 | 312.1         | 254.4         |
| Pair-seconds (standard)               | 2,128         | 2,111         |
| Pair-seconds per VTM                  | 6.82          | 8.30          |
| Vehicle pair-seconds                  | 1,267         | 1,249         |
| Vehicle pair-seconds per VTM          | 4.06          | 4.91          |
| Pairs (both vehicle-sized)            | 2,471 (1,218) | 2,264 (1,039) |
| Followers per minute                  | 14.2          | 13.7          |
| Leader changes per minute             | 18.1          | 11.0          |
| Pairs closer than 1 m (minimum gap 0) | 172           | 93            |
| Pairs closer than 3 m (minimum gap 0) | 487           | 335           |

One track per vehicle cuts vehicle-track-minutes by a fifth (a duplicate's minutes are counted
twice under `a0`), so the per-VTM rates rise under it while the absolute counts barely move. Per
site, the share of pair-seconds it removes has a median of 0.5 % added, from 13 % added to 10 %
removed (p10 to p90): no site's standard count depends on duplicates.

### The closest gap

Per pair under `body-older` (standard selector, so 3 m and over): p10 6.7 m, median 18.4 m, p90
33.5 m; for vehicle pairs, median 19.4 m.

| Closest gap | 3–5 m | 5–10 m | 10–20 m | 20–30 m | 30–40 m |
| ----------- | ----: | -----: | ------: | ------: | ------: |
| a0 pairs    |   182 |    304 |     904 |     664 |     417 |
| body-older  |   140 |    274 |     848 |     624 |     378 |

Under `body-older`, 14 vehicle pairs remain closer than 1 m at 10 sites (1 to 25 frames each, most
under 6): two tracks on one vehicle that the overlap end did not end, or two vehicles side by side
within the selector's 2.5 m lateral band. The labels say which; until then they are split vehicles,
not followers, as the spec required.

### By site

Ranked by vehicle pair-seconds under `body-older`. "All pairs" is the standard selector's count,
which includes non-vehicle tracks.

| Rank | Site                  |  VTM | All pair-s | Vehicle pair-s | Vehicle pair-s per VTM | Vehicle pairs | Followers / min | Leader changes / min |
| ---: | --------------------- | ---: | ---------: | -------------: | ---------------------: | ------------: | --------------: | -------------------: |
|    1 | embarcadero-broadway  | 17.2 |        199 |            162 |                   9.42 |           122 |            25.0 |                 22.5 |
|    2 | embarcadero-bryant    | 17.3 |        189 |            131 |                   7.55 |           100 |            20.9 |                 20.7 |
|    3 | embarcadero-bay       | 13.8 |        167 |            130 |                   9.39 |            62 |            11.6 |                 13.2 |
|    4 | lombard-laguna        | 11.9 |        137 |             94 |                   7.92 |            93 |            31.2 |                 16.8 |
|    5 | lombard-broderick     | 17.7 |        169 |             92 |                   5.20 |           116 |            37.1 |                 38.9 |
|    6 | van-ness-sacramento   | 16.0 |        113 |             87 |                   5.42 |            92 |            23.9 |                 12.5 |
|    7 | franklin-mcallister   | 19.2 |        107 |             76 |                   3.97 |            77 |            20.0 |                 20.0 |
|    8 | columbus-broadway     | 17.2 |        149 |             75 |                   4.37 |            45 |            21.8 |                 14.3 |
|    9 | bush-powell           | 12.9 |         89 |             58 |                   4.47 |            48 |            13.6 |                  8.9 |
|   10 | marina-broderick      | 13.0 |         65 |             56 |                   4.28 |            33 |             8.8 |                  3.0 |
|   11 | 3rd-folsom            |  9.6 |         75 |             50 |                   5.17 |            48 |            13.9 |                 10.9 |
|   12 | 1st-mission           | 15.5 |        102 |             46 |                   2.95 |            33 |            16.4 |                 16.4 |
|   13 | broadway-gough        |  7.4 |         73 |             42 |                   5.59 |            39 |            17.5 |                 20.5 |
|   14 | hyde-ofarrell         |  8.5 |         64 |             40 |                   4.70 |            37 |            11.1 |                  7.1 |
|   15 | marina-webster-beach  |  5.0 |        252 |             24 |                   4.90 |            10 |             6.3 |                 13.5 |
|   16 | howard-6th            |  6.1 |         59 |             21 |                   3.50 |            19 |             8.4 |                  7.5 |
|   17 | laguna-eddy           | 11.2 |         18 |             16 |                   1.44 |            12 |             3.4 |                  0.0 |
|   18 | ashbury-downey        |  4.0 |         18 |             16 |                   4.03 |            10 |             3.2 |                  0.0 |
|   19 | fulton-divisadero     |  6.6 |         33 |             15 |                   2.21 |            18 |             9.3 |                  3.2 |
|   20 | california-leon-baker |  3.9 |         15 |             12 |                   3.15 |            12 |             4.3 |                  0.2 |
|   21 | haight-divisadero     |  6.5 |          8 |              4 |                   0.67 |             7 |             3.0 |                  0.7 |
|   22 | columbus-north-point  |  5.4 |          8 |              3 |                   0.51 |             6 |             3.6 |                  1.8 |
|   23 | pierce-haight         |  8.5 |          1 |              0 |                   0.00 |             0 |             0.2 |                  0.0 |

Leader changes per minute are high wherever followers are many (Lombard at Broderick, 38.9):
the finder counts a change each time a follower's nearest leader track changes, so fragments and
lane changes both add to it. Per minute of capture, the ranking is about traffic, not tracker
quality: the slow residential sites (Haight, Pierce, Ashbury) have little following to see.

## What it does not establish

- **A rate of real following.** The finder reads tracks: fragmentation adds pairs and shortens
  them, occlusion hides them, and class is not read. The counts are an upper bound on pairs; the
  labelled window (label0002) is where pairs and episodes meet.
- **That the gate would admit these pairs.** The following gate requires both parties classed as
  rigid vehicles, which on kirk0 holds for under a third of reviewed vehicles
  ([class evidence](following-class-evidence-kirk-2026-10.md)); vehicle-sized is a size proxy, not
  a class.
- **Anything about embarcadero-folsom** or the second segments, which were not read.
- **Repeatability of ten `body-older` cases** in the tables, which are one draw of a choice now
  fixed; the repeat-verified replication changes the pooled figures by under 0.3 %.

## Provenance

| Item        | Value                                                                                                                                                    |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | tools at `fc96bca7d` (main), built with the Makefile's LDFLAGS; spec definitions committed at `0f0daff7d` and approval at `bae86a7b2`, before any replay |
| Tuning      | `config/tuning.defaults.json` for both arms; `body-older` adds the replay experiments `overlap_end_1`, `overlap_end_body`, `overlap_end_older`           |
| Selectors   | the built-in standard selectors: `following` `sha256:17227f6f…`, `leader_changes` `sha256:298fc7bf…`, `exposure` `sha256:60567467…`                      |
| Captures    | first five-minute segment of 23 corpus sites (every site but embarcadero-folsom) from the LiDAR volume's capture root, 70 s warm-up, 200 s replayed      |
| Raw outputs | evidence databases and per-case screens under the run's scratch directory and the LiDAR volume (`velocity-campaign/exp-20261020/l8fol0001`); not in Git  |

## Recommendations

1. **Spend following references at the Embarcadero sites first.** They hold the most vehicle
   following per minute and per vehicle-track-minute, and embarcadero-folsom, label0001's lot, is
   on the same corridor, so the held-out window is likely to hold pairs without anyone having read
   it. Lombard and Van Ness are next.
2. **Count following by episode, not by track pair.** Half the pairs last under half a second. An
   R5 accounting by track pair would count one following episode several times; the encounter
   method's episodes, or a pair keyed on bodies once one track per vehicle ships, are the
   denominator.
3. **Read leader changes as an identity measure.** One track per vehicle cuts them by two fifths
   without changing following; a leader-change rate per follower-minute is a cheap label-free
   check on duplicates for any identity arm.
4. **Quote the per-site `body-older` figures with the replication's caveat:** after the
   determinism fix they move by at most a few tenths of a pair-second at five sites.
