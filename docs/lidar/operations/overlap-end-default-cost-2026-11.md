# The overlap end as the default: what the flip would cost, across the corpus

<!-- ignore-style-length -->

l5id0006 found that the overlap end with tentative priority takes kirk0's identity switches from 87
to 5 and lowers duplicates at every site read, and left it off because an identity option was to
wait for label0001's labelled window. This report asks what making it the default would cost a
user before that window exists: real tracks lost or carried in pieces, spurious tracks, the
vehicle count, speeds, duplicates, and kirk's identity figures. It answers
[l5id0012](../../../data/experiments/l5id/l5id0012.md).

- **Status:** Measured, 2026-11-03. One unattended run on arrow-worker (exp-20261103, lane A); no
  default changed
- **Layers:** L5 (identity: the overlap end and tentative priority)
- **Related:** [PCL noise and splits](pcl-noise-and-splits-2026-11.md),
  [one vehicle, two tracks](l5-duplicate-tracks-2026-10.md),
  [l5id0006](../../../data/experiments/l5id/l5id0006.md),
  [label0001](../../../data/experiments/label/label0001.md)

## Summary

- **The flip fails its rule, on both sets.** Against the defaults, on the first segment of 23 sites
  and again on the third capture of 22 never read before, it passes counts, duplicates and kirk,
  and fails lost tracks, shortened tracks, spurious tracks and speeds. It stays off; nothing here
  recommends making it the default.
- **What it buys holds held out.** Duplicate pairs per hundred vehicle tracks fall at every site of
  both sets, pooled from 19.9 to 1.8 on the tuning set and from 25.4 to 3.1 held out. On kirk0
  identity switches fall from 90 to 5, HOTA rises 0.4283 to 0.4365 and IDF1 0.4706 to 0.4917; kirk1
  covers all 49 labelled road users in both arms.
- **What it costs is brief tracks.** Tracks under a second rise from 61 to 580 on the tuning set and
  from 90 to 816 held out. Clutter added comes to 21 per hundred real moving tracks (bound 5), 27
  held out.
- **Most "lost" tracks are duplicates ended, by these definitions' blind spot.** 10.0 % of the
  defaults' moving tracks are lost (bound 1 %), 10.2 % held out. But 212 of the 251 lost on the
  tuning set (224 of 267 held out) had another defaults track within 6 m on half their frames,
  and 218 (229) had an overlap-end track within 6 m: the survivor of a duplicate pair on a long
  vehicle, more than the scorer's 2 m away. This diagnostic was written after the result and does
  not change the verdict; it says the lost-track clause, as defined, mostly counts what the
  option is meant to remove.
- **Speeds move little in the middle and a lot at the tail.** The median absolute change in a
  matched track's p85 speed is 0.007 m/s (0.013 held out), but the 95th percentile is 1.97 m/s
  (2.30), against a bound of 1.0: where the surviving track is a different one, its speed differs.

## Method

**Arms.** `a0`, the defaults ([`tuning.defaults.json`](../../../config/tuning.defaults.json)), and
`overlap-end`, the defaults with the replay options `overlap_end_1`, `overlap_end_body`,
`overlap_end_older` and `tentative_priority`: l5id0006's `body-older-tentative`. No new code.

**Corpus and held-out replays.** `lidar-box-eval -variants none -rows`, 40 s warm-up and 120 s
scored, both arms of a site back to back: the first five-minute segment of 23 sites (the tuning
set) and the third of the 22 of them that have one (held out: read here for the first time for any
measure). No evidence database is written; the measures come from the rows, one per scored frame
of each confirmed track.

**Measures** ([`lidar_flip_cost.py`](../../../scripts/lidar_flip_cost.py), fixed with the rule
before any replay). A moving track lasts 2 s or more within the scored window, moves 5 m or more
first to last and has a median speed of 1 m/s or more. An `a0` moving track is lost when under half
its rows have an `overlap-end` row within 2 m at the same frame, and shortened when it is covered
but no single `overlap-end` track covers half of it; added tracks are the converse. Clutter is
brief (under 1 s) and near-static tracks together. Speeds are each matched pair's p85 row speed,
`overlap-end` less `a0`. Duplicates are `lidar-box-eval`'s pairs per hundred vehicle tracks.

**kirk.** `lidar-state-estimation-baseline`, kirk0 whole and kirk1's 270 s trim at 6 s warm-up with
an evidence database per case; kirk0 by `lidar-ground-truth-eval perframe` against the reviewed
pack ad8b9438 (version 2), split `kirk0-tuning`, ignoring the parked car `obj_788622b5`; kirk1 on
its 49 labelled road users, covered when any track overlaps 30 % of a reference's span.

## The rule, applied

Pooled over a set's sites, `overlap-end` against `a0`. The worst leave-one-out line recomputes each
pooled figure leaving out each site in turn and gives the range.

| Clause              | Bound                                                                | Tuning set, 23 sites                                                                               | Held out, 22 sites                               | Verdict     |
| ------------------- | -------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- | ------------------------------------------------ | ----------- |
| 1. Lost real tracks | ≤ 1 % pooled; no site over 5 % (or 2)                                | 10.0 % (251 of 2,503; leave-one-out 9.2–10.3 %); 14 sites over                                     | 10.2 % (267 of 2,605; 9.2–11.0 %); 14 sites over | Fails       |
| 2. Shortened        | ≤ 2 %                                                                | 4.5 % (112; 3.8–4.7 %)                                                                             | 5.1 % (132; 4.8–5.2 %)                           | Fails       |
| 3. Spurious         | clutter added ≤ 5 per hundred; added moving ≤ 2 per hundred          | 21.2 and 2.1                                                                                       | 27.4 and 2.4                                     | Fails       |
| 4. Counts           | moving ≤ `a0` × 1.02 at all but three sites                          | over at 2 sites; pooled 0.891 of `a0`                                                              | over at none; 0.880                              | Passes      |
| 5. Speeds           | median \|Δp85\| ≤ 0.1, p95 ≤ 1.0, mean within ±0.2 m/s               | 0.007, 1.97, +0.115 (2,252 pairs)                                                                  | 0.013, 2.30, +0.139 (2,338 pairs)                | Fails (p95) |
| 6. Duplicates       | fall at all but three sites                                          | 23 of 23; 19.94 → 1.75 per hundred                                                                 | 22 of 22; 25.38 → 3.05                           | Passes      |
| 7. kirk (tuning)    | HOTA, IDF1 ≥ `a0` − 0.005; switches ≤ `a0`; kirk1 covered ≥ `a0` − 1 | HOTA 0.4283 → 0.4365, IDF1 0.4706 → 0.4917, switches 90 → 5, fragmentations 46 → 34; kirk1 49 → 49 | not read                                         | Passes      |

The verdict is the conjunction of both sets, so the flip is not recommended.

## What the lost tracks are

Written after the result, to read it, not to change it. For each lost `a0` moving track: whether
another `a0` track ran within 6 m on half its frames (a duplicate partner), and whether an
`overlap-end` track did.

| Set      | Lost | With an `a0` partner within 6 m | With an `overlap-end` track within 6 m |
| -------- | ---: | ------------------------------: | -------------------------------------: |
| Tuning   |  251 |                             212 |                                    218 |
| Held out |  267 |                             224 |                                    229 |

About 85 % of lost tracks are one of a pair the overlap end resolved, whose survivor runs more than
2 m away: on a bus or truck the two duplicates sit on different parts of the body. About 40 a set
have neither, and those are the candidates for real vehicles the flip would drop, about 1.6 % of
moving tracks, still over the 1 % bound. The count change agrees: moving tracks fall 11 to 12 %,
close to the duplicate pairs removed. Whether those 40 are real cannot be told without labels.

## Per site

Tuning set, the first five-minute segment:

| Site                  | a0 moving | overlap-end moving | lost | shortened | added | clutter a0 → overlap-end | brief a0 → overlap-end | dup/100 a0 → overlap-end | p85 speed diff, median abs |
| --------------------- | --------: | -----------------: | ---: | --------: | ----: | ------------------------ | ---------------------- | ------------------------ | -------------------------: |
| 1st-mission           |       227 |                220 |   16 |        26 |     4 | 57 → 127                 | 12 → 75                | 32.4 → 0.7               |                       0.04 |
| 3rd-folsom            |       159 |                144 |    9 |         8 |     1 | 26 → 47                  | 3 → 28                 | 23.9 → 7.3               |                       0.00 |
| ashbury-downey        |        25 |                 22 |    1 |         0 |     0 | 11 → 13                  | 1 → 3                  | 4.2 → 0.0                |                       0.00 |
| broadway-franklin     |        67 |                 63 |    0 |         0 |     0 | 5 → 8                    | 3 → 7                  | 7.2 → 0.0                |                       0.00 |
| bush-powell           |        78 |                 68 |    6 |         6 |     1 | 26 → 40                  | 0 → 15                 | 21.9 → 0.0               |                       0.01 |
| california-leon-baker |        33 |                 32 |    2 |         0 |     0 | 9 → 11                   | 2 → 4                  | 7.7 → 0.0                |                       0.00 |
| columbus-broadway     |       159 |                140 |   25 |         9 |     4 | 26 → 100                 | 4 → 73                 | 23.4 → 1.9               |                       0.03 |
| columbus-north-point  |        98 |                 70 |   18 |         5 |     0 | 19 → 78                  | 2 → 58                 | 42.4 → 4.3               |                       0.00 |
| embarcadero-bay       |        72 |                 78 |    5 |         1 |     2 | 20 → 32                  | 3 → 16                 | 6.2 → 0.0                |                       0.00 |
| embarcadero-broadway  |       162 |                148 |    9 |         9 |     1 | 12 → 49                  | 6 → 44                 | 20.3 → 1.0               |                       0.01 |
| embarcadero-bryant    |       236 |                189 |   37 |         8 |    12 | 42 → 84                  | 5 → 40                 | 18.6 → 0.0               |                       0.01 |
| franklin-mcallister   |       124 |                118 |   12 |         5 |     6 | 23 → 44                  | 2 → 22                 | 24.6 → 0.9               |                       0.03 |
| fulton-divisadero     |        83 |                 65 |    8 |         7 |     0 | 10 → 26                  | 2 → 16                 | 31.1 → 1.8               |                       0.01 |
| haight-divisadero     |        26 |                 24 |    2 |         0 |     1 | 5 → 10                   | 0 → 5                  | 13.0 → 0.0               |                       0.00 |
| howard-6th            |       116 |                115 |    5 |         3 |     3 | 5 → 19                   | 0 → 12                 | 13.1 → 3.9               |                       0.00 |
| hyde-ofarrell         |        79 |                 76 |    3 |         1 |     0 | 15 → 28                  | 3 → 18                 | 15.5 → 3.0               |                       0.00 |
| laguna-eddy           |        31 |                 24 |    1 |         0 |     1 | 17 → 27                  | 0 → 13                 | 14.7 → 0.0               |                       0.00 |
| lombard-broderick     |       254 |                208 |   44 |        13 |     3 | 11 → 49                  | 4 → 40                 | 18.3 → 1.4               |                       0.01 |
| lombard-laguna        |       157 |                138 |   22 |         4 |     9 | 9 → 28                   | 3 → 23                 | 17.6 → 2.9               |                       0.00 |
| marina-broderick      |        81 |                 64 |    7 |         3 |     1 | 3 → 17                   | 0 → 14                 | 21.0 → 2.1               |                       0.00 |
| marina-webster-beach  |        50 |                 47 |    2 |         1 |     0 | 14 → 25                  | 2 → 15                 | 10.5 → 2.6               |                       0.00 |
| pierce-haight         |        24 |                 28 |    2 |         1 |     1 | 29 → 31                  | 3 → 4                  | 5.6 → 0.0                |                       0.00 |
| van-ness-sacramento   |       162 |                148 |   15 |         2 |     3 | 52 → 84                  | 1 → 35                 | 13.3 → 0.9               |                       0.01 |

Held out, the third:

| Site                  | a0 moving | overlap-end moving | lost | shortened | added | clutter a0 → overlap-end | brief a0 → overlap-end | dup/100 a0 → overlap-end | p85 speed diff, median abs |
| --------------------- | --------: | -----------------: | ---: | --------: | ----: | ------------------------ | ---------------------- | ------------------------ | -------------------------: |
| 1st-mission           |       247 |                246 |    7 |        20 |     1 | 62 → 124                 | 6 → 77                 | 22.6 → 2.9               |                       0.02 |
| 3rd-folsom            |       185 |                149 |   21 |         9 |     2 | 33 → 103                 | 7 → 73                 | 25.3 → 5.0               |                       0.01 |
| broadway-franklin     |        93 |                 82 |    7 |         4 |     3 | 9 → 15                   | 3 → 10                 | 12.0 → 0.0               |                       0.01 |
| bush-powell           |        98 |                 95 |    4 |        10 |     1 | 26 → 49                  | 1 → 24                 | 16.2 → 0.0               |                       0.00 |
| california-leon-baker |        24 |                 24 |    1 |         0 |     0 | 9 → 14                   | 0 → 3                  | 4.5 → 0.0                |                       0.00 |
| columbus-broadway     |       166 |                162 |   21 |        15 |     4 | 35 → 119                 | 5 → 82                 | 18.1 → 0.0               |                       0.02 |
| columbus-north-point  |        73 |                 55 |    6 |         2 |     0 | 16 → 33                  | 1 → 19                 | 21.2 → 4.0               |                       0.01 |
| embarcadero-bay       |       156 |                151 |   11 |         8 |     2 | 38 → 144                 | 8 → 120                | 45.5 → 9.1               |                       0.01 |
| embarcadero-broadway  |       185 |                163 |   17 |         7 |     4 | 28 → 81                  | 6 → 57                 | 32.4 → 5.2               |                       0.01 |
| embarcadero-bryant    |       271 |                212 |   53 |        11 |    17 | 51 → 119                 | 16 → 88                | 33.8 → 2.2               |                       0.05 |
| franklin-mcallister   |       132 |                112 |   19 |        11 |     3 | 33 → 92                  | 5 → 67                 | 35.9 → 4.1               |                       0.06 |
| fulton-divisadero     |       101 |                 80 |    6 |         6 |     1 | 21 → 36                  | 2 → 19                 | 30.8 → 0.0               |                       0.00 |
| haight-divisadero     |        21 |                 19 |    0 |         0 |     0 | 3 → 5                    | 0 → 2                  | 12.5 → 0.0               |                       0.00 |
| howard-6th            |        71 |                 65 |    5 |         0 |     0 | 12 → 24                  | 2 → 12                 | 9.6 → 0.0                |                       0.00 |
| hyde-ofarrell         |        90 |                 82 |    4 |        11 |     0 | 10 → 28                  | 2 → 18                 | 32.8 → 1.9               |                       0.00 |
| laguna-eddy           |        36 |                 32 |    2 |         1 |     0 | 24 → 35                  | 4 → 17                 | 31.9 → 7.9               |                       0.00 |
| lombard-broderick     |       247 |                213 |   41 |         9 |     8 | 21 → 61                  | 6 → 47                 | 20.5 → 2.4               |                       0.03 |
| lombard-laguna        |       161 |                129 |   28 |         4 |    12 | 10 → 31                  | 4 → 25                 | 15.6 → 2.7               |                       0.00 |
| marina-broderick      |        47 |                 41 |    4 |         1 |     1 | 2 → 6                    | 2 → 6                  | 8.8 → 3.4                |                       0.00 |
| marina-webster-beach  |        37 |                 30 |    4 |         1 |     0 | 15 → 21                  | 1 → 8                  | 14.3 → 12.0              |                       0.00 |
| pierce-haight         |        44 |                 40 |    1 |         0 |     0 | 24 → 31                  | 3 → 12                 | 14.3 → 0.0               |                       0.01 |
| van-ness-sacramento   |       120 |                110 |    5 |         2 |     3 | 44 → 68                  | 6 → 30                 | 26.6 → 1.2               |                       0.02 |

## What it does not establish

- Whether any lost, added or merged track was right. Lost and added are measured against `a0`, not
  against labels; only kirk0 has reviewed per-frame labels, about 20 objects.
- Live timing. Replay wall times are not read: arrow-worker ran another lane and the Gerardus
  server alongside, and l5id0006 measured the overlap end at about 40 % more replay time on a Mac.
- label0001's labelled identity comparison, which this does not replace.
- Anything outside San Francisco or the 2.3 m mount.

## Provenance

| Item           | Value                                                                                                                                       |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Branch         | `exp/l5id-overlap-end-default`, from main `84492f9e`                                                                                        |
| Tools          | `lidar-box-eval`, `lidar-state-estimation-baseline`, `lidar-ground-truth-eval` at `934e0f493`, built with the Makefile's LDFLAGS and `pcap` |
| Parameters     | `config/tuning.defaults.json`, SHA-256 `9ad7377f…`, both arms                                                                               |
| Replay options | `overlap_end_1`, `overlap_end_body`, `overlap_end_older`, `tentative_priority` (`overlap-end` only)                                         |
| Corpus         | first and third five-minute captures of the sites in the spec, from the S2 archive index                                                    |
| kirk           | kirk0 whole; `kirk1-first270s.pcapng`; pack ad8b9438 version 2, split `kirk0-reference-tuning-r1`; kirk1 references `60a4774c`              |
| Host           | arrow-worker, 4 vCPU, 9 GB, shared with lane B and the Gerardus server                                                                      |
| Raw output     | `velocity-campaign/exp-20261103/laneA/l5id0012/` on the results share                                                                       |

## Recommendations

1. **Do not make the overlap end the default on this evidence.** It fails four clauses on both sets.
2. **Measure lost tracks against a match that knows the body.** A 2 m point match counts the
   surviving half of a duplicate on a long vehicle as a lost vehicle. A rerun scoring lost tracks
   by footprint overlap would separate the roughly 40 a set that may be real from the duplicates,
   with the same replays (the rows are archived).
3. **Find where the brief tracks come from before any gate.** Brief tracks rise about ninefold:
   each ended duplicate seems to leave a short-lived track, the cost the November run first saw.
   An option that suppresses the younger track's confirmation rather than ending it after the
   fact is the lever to try.
4. **Keep label0001 the deciding read** for identity, as l5id0004 set out.
