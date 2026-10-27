# Where a vehicle's lower body goes: the rings read by range on 23 corpus sites

<!-- ignore-style-length -->

The published track's base is its cluster's lowest return, and with range that return climbs up
the body. This report reads, ring by ring, what the returns that should have met a moving
vehicle's lower body were: background at the body (absorbed by L3), background short of it
(occluded), nothing (dropout) or foreground under L4's floor (clipped). The answer decides the
remedy, an L3 window or the box stood on the surface. It answers
[l3bg0024](../../../data/experiments/l3bg/l3bg0024.md).

- **Status:** Measured, 2026-10-27. One unattended run on arrow-worker (exp-20261026); no default
  changed
- **Layers:** L3 (the closeness window, the track guard), L4 (the floor), L5 (the published box)
- **Related:** [beam-wise review, the absorbed height](../../../data/maths/proposals/20261011-beamwise-background-subtraction-review.md#2-the-absorbed-height-derivation-section-5),
  [ground surface read](ground-surface-corpus-read-2026-10.md), [L3 track guard](l3-track-guard-kirk-2026-10.md),
  [ground0004](../../../data/experiments/ground/ground0004.md)

## Summary

- **L3 takes the lower body.** Beyond 20 m, two thirds of the ring-slots that should have met a
  moving vehicle below its lowest foreground return came back as background at the body's own
  range (66 % pooled under the L3 the spec was written against, `pre-consolidation`); 16 % were
  occluded by something nearer, 6 % dropped out and 0.1 % were clipped. Absorption is the largest
  share at 22 of 23 sites, so by the spec's reading the band is wider at range than the beam-wise
  derivation allows.
- **The track guard already sees it.** 58 % of those slots are returns L3 took as background that
  the track guard publishes as foreground inside the tracked box. The lower body is in the
  published frame and missing from the cluster: the guard's foreground never reaches L4.
- **A narrower window returns some of it, at a price (clause 1 not met).** `closeness-2` lowers
  the lowest return's p50 above the surface beyond 20 m by a median 0.41 m (15 of 23 sites by
  0.3 m or more), but raises spurious clusters a frame 1.44 × pooled, and by more than 25 % at 14
  sites, against l3bg0023's bounds of 10 % and 25 %. Absorption is still 60 % of its slots beyond
  20 m.
- **Occlusion and dropout are not the cause (clause 2 not met)** at any site but one (Laguna at
  Eddy, occluded), and **clipping is over a tenth only at Pierce at Haight**, whose surface departs
  furthest from one plane where its traffic goes (ground0001).
- **The shipped L3 is `closeness-2`'s.** `defaults`, the L3 since l3bg0025 (closeness 2 with a
  0.3 m margin and neighbour confirmation 5), lowers the lowest return beyond 20 m by a median
  0.42 m against `pre-consolidation`, at 1.45 × its spurious clusters (1.01 × `closeness-2`'s), and
  still loses 61 % of those slots to absorption. The lower body the window returns, the defaults
  already have; the rest is not the window's to return.

## Method

**Replays.** `lidar-box-eval -lower-body -variants none` on the first five-minute segment of the 23
tuning and screen sites (embarcadero-folsom held out and unread), 200 s scored after a 70 s
warm-up, with `box_ground` so the blended surface is fitted (it moves no return and no cluster),
and an evidence database for the screen; then kirk0 whole after a 20 s warm-up. The two arms the
reading decides on ran first, a site's pair back to back; `defaults` after them.

**The block** (replayeval's `FrameTap`, this PR). For each scored frame of a confirmed track
observed in it, moving at 2 m/s or more, whose published box is 3 m long or more:

- the lowest foreground return's height above the blended surface under the box's centre;
- each azimuth slot of the L3 grid (0.2°) whose ray enters the box's footprint, on each ring
  whose beam meets the footprint's near face above the blended surface and below that lowest
  return: the height at which the body should have returned something;
- the slot's nearest return on that ring, classed from **L3's own mask** (before the track guard
  gives held returns the foreground class for clients): **absorbed**, background between the near
  face less the L3 closeness window and the far side plus it; **occluded**, background nearer than
  the near face less the window; **dropout**, no return; **clipped**, foreground below L4's floor.
  Two more are counted apart: **behind**, background past the far side plus the window (the beam
  passed under or beside the body), and **foreground**, foreground above the floor. The window is
  `ClosenessThresholdMetres` with the arm's L3 parameters and a spread of 0, the settled cell's.
  **Guard-held** counts the slots whose return L3 took as background and the guard publishes as
  foreground.

The definitions, and that the reading compares the spec's four classes only, were committed before
any replay. The kirk0 check run that preceded them read the published mask, which showed two thirds
of the slots as foreground L4 never received; the block was changed to L3's mask then.

**The screen.** l3bg0023's on each evidence database, with l4clus0004's label-free readings:
vehicle tracks lasting 2 s or more with a p90 cluster side of 3.0 m or more, and spurious clusters
a frame that no lasting track ever takes as its measurement. l3bg0023 states no spurious bound, so
its corpus tolerances apply: within 10 % pooled and 25 % per site.

## Results

Per site, beyond 20 m: the classes and the guard-held share under `pre-consolidation`, and each
arm's p50 of the lowest return above the surface.

| Site                  | Track frames | Absorbed | Occluded | Dropout | Clipped | Behind | Foreground | Guard-held | p50 `pre-consolidation` | `closeness-2` | `defaults` |
| --------------------- | -----------: | -------: | -------: | ------: | ------: | -----: | ---------: | ---------: | ----------------------: | ------------: | ---------: |
| 1st-mission           |         1026 |     53 % |     37 % |     4 % |     0 % |    5 % |        1 % |       50 % |                  1.72 m |        1.22 m |     1.24 m |
| 3rd-folsom            |          380 |     68 % |     14 % |     8 % |     0 % |    7 % |        2 % |       60 % |                  1.67 m |        1.18 m |     1.20 m |
| ashbury-downey        |          988 |     54 % |     11 % |     7 % |     0 % |   27 % |        1 % |       39 % |                  1.18 m |        0.71 m |     0.70 m |
| broadway-gough        |          311 |     61 % |     10 % |     6 % |     8 % |   13 % |        3 % |       53 % |                  0.62 m |        0.62 m |     0.65 m |
| bush-powell           |          389 |     77 % |      7 % |     4 % |     0 % |   10 % |        2 % |       63 % |                  1.35 m |        0.56 m |     0.59 m |
| california-leon-baker |          151 |     62 % |      4 % |     5 % |     5 % |   22 % |        2 % |       43 % |                  1.40 m |        1.04 m |     1.04 m |
| columbus-broadway     |          608 |     73 % |     14 % |     5 % |     0 % |    6 % |        2 % |       66 % |                  1.26 m |        0.74 m |     0.73 m |
| columbus-north-point  |          314 |     65 % |     23 % |     3 % |     0 % |    6 % |        2 % |       59 % |                  1.06 m |        0.75 m |     0.75 m |
| embarcadero-bay       |          938 |     71 % |     13 % |     7 % |     0 % |    7 % |        3 % |       61 % |                  1.05 m |        0.65 m |     0.68 m |
| embarcadero-broadway  |         1911 |     67 % |     11 % |     6 % |     0 % |    7 % |        9 % |       62 % |                  0.91 m |        0.68 m |     0.70 m |
| embarcadero-bryant    |          731 |     71 % |     11 % |     6 % |     0 % |    5 % |        7 % |       66 % |                  1.08 m |        0.82 m |     0.84 m |
| franklin-mcallister   |          450 |     74 % |      8 % |     3 % |     0 % |    8 % |        8 % |       58 % |                  0.43 m |        0.63 m |     0.60 m |
| fulton-divisadero     |          116 |     63 % |     16 % |     3 % |     0 % |   12 % |        6 % |       42 % |                  0.79 m |        0.79 m |     0.77 m |
| haight-divisadero     |           36 |     73 % |      4 % |     3 % |     3 % |   17 % |        0 % |       59 % |                  1.79 m |        1.38 m |     1.38 m |
| howard-6th            |           97 |     48 % |     38 % |     6 % |     0 % |    6 % |        1 % |       45 % |                  1.80 m |        1.02 m |     1.03 m |
| hyde-ofarrell         |          151 |     74 % |      6 % |     3 % |     0 % |    9 % |        9 % |       66 % |                  0.63 m |        0.86 m |     0.80 m |
| laguna-eddy           |           28 |     37 % |     54 % |     4 % |     0 % |    4 % |        1 % |       32 % |                  1.25 m |        0.75 m |     0.75 m |
| lombard-broderick     |          535 |     66 % |     20 % |     7 % |     0 % |    7 % |        1 % |       50 % |                  1.77 m |        0.70 m |     0.73 m |
| lombard-laguna        |         1063 |     61 % |     19 % |     4 % |     0 % |    7 % |       10 % |       57 % |                  0.93 m |        0.76 m |     0.76 m |
| marina-broderick      |          575 |     79 % |      3 % |     6 % |     0 % |   10 % |        2 % |       70 % |                  1.30 m |        0.90 m |     0.89 m |
| marina-webster-beach  |          783 |     77 % |      3 % |     9 % |     0 % |    8 % |        2 % |       60 % |                  0.78 m |        0.69 m |     0.76 m |
| pierce-haight         |           34 |     83 % |      0 % |     1 % |    11 % |    4 % |        0 % |       76 % |                  1.73 m |        1.16 m |     1.10 m |
| van-ness-sacramento   |          566 |     74 % |      5 % |     7 % |     0 % |   11 % |        3 % |       65 % |                  1.32 m |        0.86 m |     0.82 m |

Pooled over the 23 sites by range bin, ring-slots summed; the p50 is the median of the sites'.

**pre-consolidation**, pooled over 23 sites

| Range   | Track frames | Lowest return above surface, p50 (median of sites) | Ring-slots | Absorbed | Occluded | Dropout | Clipped | Behind | Foreground | Guard-held |
| ------- | -----------: | -------------------------------------------------: | ---------: | -------: | -------: | ------: | ------: | -----: | ---------: | ---------: |
| 0–10 m  |         8345 |                                             0.51 m |  2,309,485 |     82 % |      7 % |     2 % |     0 % |    6 % |        4 % |       74 % |
| 10–20 m |        16088 |                                             0.64 m |  5,104,489 |     76 % |     10 % |     2 % |     0 % |    7 % |        5 % |       67 % |
| 20–30 m |         6723 |                                             1.10 m |  1,825,828 |     68 % |     16 % |     4 % |     0 % |    8 % |        4 % |       60 % |
| 30–40 m |         2965 |                                             1.52 m |    528,888 |     63 % |     16 % |     8 % |     0 % |   10 % |        3 % |       55 % |
| 40–50 m |         1208 |                                             1.84 m |    122,224 |     54 % |     19 % |    11 % |     0 % |   12 % |        3 % |       44 % |
| 50–60 m |          610 |                                             1.85 m |     32,906 |     52 % |     25 % |    16 % |     0 % |    4 % |        4 % |       45 % |
| 60–70 m |          382 |                                             1.70 m |     12,020 |     42 % |     24 % |    27 % |     0 % |    2 % |        4 % |       38 % |
| 70 m on |          293 |                                             1.02 m |      4,972 |     35 % |     16 % |    40 % |     0 % |    3 % |        5 % |       36 % |

**closeness-2**, pooled over 23 sites

| Range   | Track frames | Lowest return above surface, p50 (median of sites) | Ring-slots | Absorbed | Occluded | Dropout | Clipped | Behind | Foreground | Guard-held |
| ------- | -----------: | -------------------------------------------------: | ---------: | -------: | -------: | ------: | ------: | -----: | ---------: | ---------: |
| 0–10 m  |        12231 |                                             0.43 m |  2,141,752 |     74 % |      8 % |     2 % |     0 % |   12 % |        5 % |       65 % |
| 10–20 m |        28393 |                                             0.46 m |  6,039,091 |     70 % |     12 % |     1 % |     1 % |    7 % |        8 % |       61 % |
| 20–30 m |        19216 |                                             0.65 m |  3,303,990 |     62 % |     16 % |     4 % |     1 % |    9 % |        9 % |       56 % |
| 30–40 m |         9676 |                                             0.88 m |  1,254,556 |     59 % |     17 % |     8 % |     0 % |    8 % |        8 % |       55 % |
| 40–50 m |         4968 |                                             1.18 m |    395,964 |     54 % |     21 % |    12 % |     0 % |    6 % |        7 % |       50 % |
| 50–60 m |         2729 |                                             1.39 m |    131,845 |     51 % |     21 % |    16 % |     0 % |    4 % |        7 % |       43 % |
| 60–70 m |         1550 |                                             1.42 m |     50,551 |     40 % |     24 % |    24 % |     0 % |    4 % |        9 % |       37 % |
| 70 m on |         1401 |                                             1.54 m |     26,190 |     35 % |     24 % |    27 % |     1 % |    4 % |        9 % |       34 % |

**defaults**, pooled over 23 sites

| Range   | Track frames | Lowest return above surface, p50 (median of sites) | Ring-slots | Absorbed | Occluded | Dropout | Clipped | Behind | Foreground | Guard-held |
| ------- | -----------: | -------------------------------------------------: | ---------: | -------: | -------: | ------: | ------: | -----: | ---------: | ---------: |
| 0–10 m  |        12096 |                                             0.45 m |  2,054,989 |     82 % |      6 % |     1 % |     0 % |    6 % |        5 % |       70 % |
| 10–20 m |        28351 |                                             0.48 m |  6,130,678 |     72 % |     11 % |     1 % |     1 % |    7 % |        8 % |       61 % |
| 20–30 m |        19413 |                                             0.66 m |  3,326,930 |     64 % |     15 % |     4 % |     1 % |    8 % |        9 % |       57 % |
| 30–40 m |         9767 |                                             0.88 m |  1,262,974 |     60 % |     17 % |     8 % |     0 % |    8 % |        7 % |       55 % |
| 40–50 m |         5060 |                                             1.17 m |    412,447 |     54 % |     21 % |    12 % |     0 % |    6 % |        7 % |       48 % |
| 50–60 m |         2773 |                                             1.37 m |    137,035 |     48 % |     23 % |    17 % |     0 % |    4 % |        7 % |       40 % |
| 60–70 m |         1457 |                                             1.49 m |     51,738 |     38 % |     25 % |    24 % |     0 % |    3 % |        9 % |       34 % |
| 70 m on |         1387 |                                             1.53 m |     27,704 |     37 % |     22 % |    28 % |     0 % |    4 % |        9 % |       35 % |

The screen, per site:

| Site                  | Spurious a frame `pre-consolidation` | `closeness-2` | `defaults` | Vehicle tracks `pre-consolidation` | `closeness-2` | `defaults` |
| --------------------- | -----------------------------------: | ------------: | ---------: | ---------------------------------: | ------------: | ---------: |
| 1st-mission           |                                 3.25 |          5.71 |       5.33 |                                 90 |           207 |        206 |
| 3rd-folsom            |                                 1.58 |          2.54 |       2.55 |                                 74 |           102 |        110 |
| ashbury-downey        |                                 0.58 |          0.70 |       0.61 |                                 37 |            32 |         34 |
| broadway-gough        |                                 1.50 |          1.46 |       1.52 |                                 56 |           130 |        123 |
| bush-powell           |                                 1.57 |          1.61 |       1.79 |                                 82 |           113 |        121 |
| california-leon-baker |                                 0.65 |          0.57 |       0.56 |                                 29 |            40 |         38 |
| columbus-broadway     |                                 2.90 |          4.77 |       4.61 |                                144 |           262 |        248 |
| columbus-north-point  |                                 1.18 |          1.59 |       1.65 |                                 43 |            76 |         73 |
| embarcadero-bay       |                                 0.81 |          1.25 |       1.33 |                                 62 |            90 |         88 |
| embarcadero-broadway  |                                 1.98 |          3.36 |       3.40 |                                100 |           192 |        185 |
| embarcadero-bryant    |                                 1.45 |          3.00 |       3.16 |                                 76 |           129 |        123 |
| franklin-mcallister   |                                 1.72 |          2.58 |       2.51 |                                114 |           213 |        208 |
| fulton-divisadero     |                                 1.14 |          1.52 |       1.54 |                                 26 |            85 |         90 |
| haight-divisadero     |                                 0.81 |          0.85 |       0.81 |                                 20 |            50 |         53 |
| howard-6th            |                                 0.75 |          1.68 |       1.69 |                                 40 |            92 |         83 |
| hyde-ofarrell         |                                 1.20 |          1.70 |       1.79 |                                 57 |           102 |        109 |
| laguna-eddy           |                                 0.91 |          1.11 |       1.15 |                                 39 |            69 |         73 |
| lombard-broderick     |                                 3.27 |          3.94 |       4.17 |                                 81 |           300 |        292 |
| lombard-laguna        |                                 2.65 |          2.75 |       2.78 |                                118 |           184 |        185 |
| marina-broderick      |                                 0.94 |          1.15 |       1.13 |                                100 |            97 |         96 |
| marina-webster-beach  |                                 0.63 |          0.85 |       0.87 |                                 41 |            38 |         38 |
| pierce-haight         |                                 1.30 |          2.02 |       2.19 |                                 43 |            65 |         57 |
| van-ness-sacramento   |                                 2.21 |          3.51 |       3.57 |                                169 |           241 |        242 |

**kirk0.** The same shape. Under `pre-consolidation` the lowest return sits 0.43, 1.06, 1.56,
1.89 and 2.15 m above the surface in the 0–10 to 40–50 m bins (the spec's development figures,
0.56 to 2.09 m, again), with 59 % of the slots beyond 20 m absorbed and 29 % occluded by the parked
vehicles and trucks; `closeness-2` brings the p50 beyond 20 m from 1.78 to 0.77 m and `defaults`
to 0.82 m, with absorption still 59 and 60 % there.

## Reading, against the spec's

- **Absorption is the largest share beyond 20 m on most sites** (22 of 23), so the band is wider at
  range than the derivation allows. **`closeness-2` must then lower the lowest return's p50 above
  the surface beyond 20 m by 0.3 m or more without raising spurious clusters beyond l3bg0023's
  bound.** It lowers it by 0.41 m (median) and raises spurious clusters 1.44 × pooled and over
  1.25 × at 14 sites, so it does not pass, and the window's shape (the review's claim C1) is not
  yet the next spec.
- **Occlusion or dropout the largest share:** not met. It is at one site, Laguna at Eddy.
- **Clipping over a tenth of the share:** Pierce at Haight (11 %). ground0001 measured its
  cell planes departing from one plane by 1.28 m at p90 where its traffic goes, with the largest
  signed median of any site (+0.55 m): the floor sits above its road there, and it is ground0002's.

## What could mislead

- **The arms' tracks are not the same tracks.** `closeness-2` has 1.8 × `pre-consolidation`'s
  vehicle tracks (2,909 against 1,641), so its slots and p50s are over a different population; a
  track it gains may be a far one whose lowest return sits high.
- **A beam under the sill.** A beam that passes just under the body and meets the road within the
  footprint plus the window counts as absorbed. It must be within about 0.2 m of the road at the
  near face to do so, against site medians of the lowest return of 0.43 to 1.80 m beyond 20 m
  (1.25 m at the median site), so it can account for at most a sixth of the band at the median
  site, though half of it at Franklin at McAllister; `behind` (8 %) counts the beams that pass
  further.
- **The surface is the sensor's own** (ground0001): where the fit sits below the road, every
  height reads high.
- **The window is the settled cell's.** Region overrides and a cell's own spread widen it in
  places; a return they make background is still L3's, and classed by where it lies, not by why.

## What it does not establish

- **That returning the guard's foreground to L4 is safe.** It is the obvious next lever and is not
  tested here.
- **Recall.** This is geometry, not detection.
- **Pedestrians**, whose lower legs the review puts in the band at every range, and **anything
  held out**.

## Provenance

| Item        | Value                                                                                                                                                                                                                            |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | tools at `1a3083007` on `claude/l3bg0024-lower-body` (0.5.1-pre47), built with the Makefile's LDFLAGS; the block and its definitions at `789dcd074` and `1a3083007`, the reading's classes at `53b4d4352`, all before any replay |
| Tuning      | `config/experiments/l3-pre-consolidation.json`, `config/experiments/replay-l3-closeness2.json`, `config/tuning.defaults.json` (SHA-256 `aefcc10c…`), each with the replay experiment `box_ground`                                |
| Corpus      | the first-segment corpus and index of the October campaigns (`32e33814…`, `ccf180d3…`), 23 sites, 70 s warm-up, 200 s scored, captures over NFS through a resolved root; kirk0 `sha256:2864ebde…` from the captures share        |
| Screen      | `screen_l3bg0024.py` over `screen_consol_arrow.py` `1758f7a7…` and `score_free.py` `706458a0…`                                                                                                                                   |
| Host        | arrow-worker (2 vCPU) beside its other services; 69 corpus units in 4 h 50 min (11:07 to 15:57Z), kirk0's three in 2 min                                                                                                         |
| Raw outputs | `box.json` and screens per unit under the run's scratch directory, archived to `velocity-campaign/exp-20261026/l3bg0024` on the results share; evidence databases deleted once screened; not in Git                              |

## Recommendations

1. **Give the guard's foreground to L4, as its own spec** (proposed as
   [l3bg0027](../../../data/experiments/l3bg/l3bg0027.md)). Most of the absorbed lower body is
   already published as foreground inside the tracked box; clustering it with the track's points,
   or standing the box's base on it, would return the lower body without moving L3's window or its
   spurious clusters. The spec should read the box's base and height against kirk0's physical
   references and the corpus's spurious clusters, as this one did.
2. **Do not look to the window for the rest.** The shipped L3 already has `closeness-2`'s window,
   the 0.42 m it returns and the spurious clusters it costs, and still loses 61 % of the slots
   beyond 20 m to absorption; the window's shape (C1) is not worth a spec until a narrower window
   can be shown not to cost more.
3. **Read Pierce at Haight in ground0002 first.** It is the one site whose floor clips the lower
   body, and the one whose surface departs most from a plane where the traffic goes.
4. **ground0004's held base stays the fallback** for what neither L3 nor the guard returns: the
   occluded 16 % and the dropout.
