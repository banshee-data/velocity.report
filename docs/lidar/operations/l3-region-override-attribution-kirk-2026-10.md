# L3 region overrides on kirk0 and kirk1: the volatile tercile, a road-height floor and the frontier, October 2026

The follow-up the [zero-code L3 arms report](l3-zero-code-arms-kirk-2026-10.md) asked for in its
Section 8: which of the B8 region overrides absorbs road users, whether a height band floor at road
height removes the road returns a narrowed L3 lets through, and whether switching the overrides
off moves kirk0's recall and false-foreground trade-off or only moves along it.

- **Status:** Measured, October 12, 2026. One machine, two captures, one site. Nothing shipped
  changes; no tuning default moves
- **Layers:** L3 background grid, L4 height band
- **Related:** [zero-code L3 arms](l3-zero-code-arms-kirk-2026-10.md),
  [beam-wise review](../../../data/maths/proposals/20261011-beamwise-background-subtraction-review.md),
  [annotation-scored tuning](annotation-scored-tuning.md)

## Summary

- **One window does both jobs.** Of the B8 region overrides, the ×8 window on the volatile
  tercile is what absorbs road users: kept alone it recovers 2.6 points of kirk0 recall over the
  shipped grid, while keeping only the stable and variable terciles' overrides is
  indistinguishable from switching all of them off (recall 0.930, 9.9 unlabelled clusters a
  frame). The earlier report attributed the near-field cost to the quiet terciles; it is the
  volatile one, which is also what suppressed the far-field flicker.
- **Switching the overrides off is a move along the curve, not a better curve.** Over 518 kirk0
  configs run in every mode, turning the overrides off never raised recall without raising the
  spurious clusters, and at every budget the best overrides-on config matches or beats the best
  overrides-off one. The September sweep's overrides-on frontier reaches 0.82 recall at under one
  spurious cluster a frame.
- **The shipped point is far inside that frontier.** With the overrides kept, `closeness_multiplier`
  2.0, `post_settle_update_fraction` 0.005 and `foreground_min_cluster_points` 8 (CM) take kirk0
  recall from 0.587 to 0.888 and pedestrians from 0.28 to 0.76 at the shipped arm's false
  positives (877 against 722), with HOTA 0.545 against 0.376 and IDF1 0.607 against 0.350; kirk1,
  which no sweep scored, has all 49 labelled road users covered against 38. On 17 corpus sites,
  label-free, it confirms 62 % more moving tracks with longer lives.
- **A height band floor at road height is free.** Moving `height_band_floor` from −2.8 to −2.0 m
  (0.3 m above the 2.3 m mount's road) removes every road-surface cluster in every regime, takes
  18 to 32 % of kirk0's unlabelled clusters with them, and costs at most 0.2 points of recall.
- **The price of finding more is stationary tracks.** Every arm that recovers road users makes
  more long stationary tracks: on the corpus, 39 to 67 over 20 s and 5 to 15 over 60 s. The dwell
  policy the beam-wise review asked for is still a prerequisite.
- **lidar-bench's L4 is bare DBSCAN**, without the product's height band or size filters, so the
  annotation-scored sweeps' spurious and fragmentation figures are for a different L4 than the
  one the harness and the server run. Mode-against-mode comparisons inside a sweep stand.

## 1. Arms

Each harness arm is a checked-in tuning config plus replay experiments, which are Go-level and do
not move the config fingerprint. Two experiments are new in this run:
`region_overrides_volatile_only` keeps the overrides of the volatile tercile only (noise ×8,
neighbour confirmation +2, update fraction ×0.5) and serves the globals to the stable and variable
terciles; `region_overrides_quiet_only` is its inverse, the ×4 and ×3 overrides of the stable and
variable terciles kept and the globals served to the volatile one.

| Arm | Tuning config                                                                | Experiments                                     | What it isolates                             |
| --- | ---------------------------------------------------------------------------- | ----------------------------------------------- | -------------------------------------------- |
| A0  | [tuning.defaults.json](../../../config/tuning.defaults.json)                 | none                                            | The shipped grid; the earlier report's A0    |
| F   | tuning.defaults.json                                                         | `no_region_overrides`, `support_filter`         | The earlier report's F: overrides off, E4 on |
| FR  | [height-floor-road.json](../../../config/experiments/height-floor-road.json) | `no_region_overrides`, `support_filter`         | F with the height band floor at road height  |
| R   | height-floor-road.json                                                       | none                                            | The floor alone on the shipped grid          |
| V   | tuning.defaults.json                                                         | `region_overrides_volatile_only`                | Only the volatile tercile's overrides        |
| QF  | tuning.defaults.json                                                         | `region_overrides_quiet_only`, `support_filter` | Only the quiet terciles' overrides, E4 on    |

`height-floor-road.json` changes one key, `l4.dbscan_xy_v1.height_band_floor`, from −2.8 to
−2.0 m. The Pandar40P sits about 2.3 m above the road at kirk0 (kirk0's settled background puts
road returns at −2.2 to −2.3 m within 15 m), so the shipped floor is 0.5 m below the road and
removes no road return, while −2.0 m removes returns within 0.3 m of it.

## 2. Method

**Harness arms.** As in the earlier report: `lidar-state-estimation-baseline` with `-warmup 6
-sample-points 1024 -campaign-metrics`, kirk0 whole (`-duration 0`) and kirk1's first 490,000 UDP
packets (272 s, re-trimmed with the same `tcpdump` command) with `-duration 172`. Each arm replays
twice and the harness verifies the repeat. Scoring is the earlier report's scripts unchanged: M1
covered labelled road users, object-frame recall and unlabelled clusters against the annotation
pack `8e422582` ground truth (kirk0), the lowest retained point inside each labelled footprint,
stationary tracks (M3), and `lidar-ground-truth-eval perframe` against the reviewed pack
`ad8b9438` with the tuning split `kirk0-ad8b9438-tuning-r1`. A0 and F reproduce the earlier
report's rows to the last digit on both captures (kirk0: recall 0.5871, 3.43 clusters a frame, 59
tracks; 0.930, 9.87 unlabelled, 204 tracks; kirk1: 38 and 49 of 49 covered), so the arms are on
the same footing as its A0 to H.

**Sweep.** The annotation-scored tuning harness (`lidar-bench -clusters-output`, scored per frame
against the same kirk0 ground truth by `score_run.py`, about 10 s a run) with a new flag,
`lidar-bench -experiment`, which applies the background experiments through the same function the
replay harness uses. Each sampled L3/L4 config runs in four modes back to back, so
modes are compared config by config: `on` (the shipped background model), `off`
(`no_region_overrides`), `off_sf` (`no_region_overrides`, `support_filter`) and `vol`
(`region_overrides_volatile_only`). The grid is the full factorial of `closeness_multiplier` 2.0,
3.0, 4.0; `noise_relative` 0.01, 0.02, 0.04, 0.08 (reaching above the September sweep's 0.02,
because with the overrides off the global window is the only window); `neighbour_confirmation_count`
2, 3, 5; `safety_margin_metres` 0.05, 0.15; `background_update_fraction` 0.005, 0.02;
`post_settle_update_fraction` 0, 0.005; `foreground_dbscan_eps` 0.4, 0.55, 0.8; and
`foreground_min_cluster_points` 3, 5, 8: 2,592 configs, run in a shuffled order with a fixed seed so
that a sweep cut by the clock is an unbiased sample. It ran 518 of them in every mode (2,083 runs,
no errors) before it was stopped to give the corpus lanes the machine. The shipped config and the
September sweep's best low-fragmentation config ran first in every mode.

**lidar-bench's L4 is not the production L4.** The benchmark runs bare DBSCAN on the foreground:
no height band, no foreground input cap, no voxel step and no cluster diameter or aspect filters.
The pipeline the harness and the server run applies all of them. The sweep's spurious-cluster and
fragmentation figures, and those of the September 11,861-config sweep, are therefore figures for
bare DBSCAN, and the height band floor cannot be swept there: run through the benchmark,
`height-floor-road.json` scores identically to the defaults. Comparisons between modes inside the
sweep are like for like; the floor is scored only by the harness arms.

## 3. Results: the harness arms

### 3.1 kirk0

| Arm | M1 covered | Tracks | Clusters/frame | FG points/frame | Object-frame recall |    Car / ped / bus | Unlabelled clusters/frame | Road-surface clusters/frame | Bottom in footprint p10 / median / p90 (m) | M3 stationary >20 s / >60 s |
| --- | ---------: | -----: | -------------: | --------------: | ------------------: | -----------------: | ------------------------: | --------------------------: | -----------------------------------------: | --------------------------: |
| A0  |   11 of 16 |     59 |           3.43 |           1,172 |               0.587 | 0.63 / 0.28 / 0.53 |                      0.31 |                        0.17 |                      −0.01 / +0.26 / +0.91 |                       0 / 0 |
| F   |   12 of 16 |    204 |          14.50 |           3,329 |               0.930 | 0.94 / 0.91 / 0.35 |                      9.87 |                        1.60 |                      −0.88 / −0.29 / +0.04 |                       4 / 2 |
| FR  |   12 of 16 |    166 |          12.63 |           3,262 |               0.928 | 0.94 / 0.91 / 0.41 |                      8.09 |                        0.00 |                      −0.83 / −0.25 / +0.06 |                       4 / 2 |
| R   |   11 of 16 |     51 |           3.23 |           1,169 |               0.587 | 0.63 / 0.28 / 0.53 |                      0.21 |                        0.00 |                      −0.01 / +0.26 / +0.91 |                       0 / 0 |
| V   |   12 of 16 |     67 |           3.71 |           1,277 |               0.613 | 0.65 / 0.30 / 0.53 |                      0.71 |                        0.25 |                      −0.41 / −0.00 / +0.81 |                       1 / 0 |
| QF  |   12 of 16 |    202 |          14.43 |           3,085 |               0.930 | 0.94 / 0.91 / 0.35 |                      9.88 |                        1.56 |                      −0.85 / −0.23 / +0.10 |                       3 / 2 |

Per-frame evaluator, reviewed pack, all episodes (3,709 reference points):

| Arm |   MOTA |  HOTA |  DetA |  AssA |  IDF1 |   IDR |   IDP |    FN |    FP | IDSW |  FM |
| --- | -----: | ----: | ----: | ----: | ----: | ----: | ----: | ----: | ----: | ---: | --: |
| A0  |  0.148 | 0.376 | 0.234 | 0.679 | 0.350 | 0.275 | 0.482 | 2,315 |   722 |  124 |  48 |
| F   | −1.229 | 0.336 | 0.166 | 0.759 | 0.376 | 0.710 | 0.256 |   675 | 7,253 |  338 |  45 |
| FR  | −0.911 | 0.341 | 0.184 | 0.708 | 0.396 | 0.683 | 0.279 |   689 | 6,069 |  331 |  47 |
| R   |  0.163 | 0.379 | 0.239 | 0.677 | 0.354 | 0.274 | 0.500 | 2,315 |   642 |  148 |  48 |
| V   |  0.153 | 0.368 | 0.238 | 0.657 | 0.386 | 0.322 | 0.484 | 2,157 |   914 |   69 |  47 |
| QF  | −1.260 | 0.312 | 0.166 | 0.662 | 0.323 | 0.610 | 0.220 |   665 | 7,250 |  467 |  41 |

Where the unlabelled clusters sit, per frame, by range (road-surface clusters in brackets):

| Arm | Within 10 m |  10 to 20 m |  20 to 40 m | Beyond 40 m | Recurring at one 0.5 m position |
| --- | ----------: | ----------: | ----------: | ----------: | ------------------------------: |
| A0  | 0.01 (0.00) | 0.09 (0.01) | 0.18 (0.07) | 0.04 (0.01) |                            0.00 |
| F   | 0.09 (0.00) | 2.71 (0.02) | 4.34 (1.36) | 2.72 (0.14) |                            0.83 |
| FR  | 0.09 (0.00) | 2.63 (0.00) | 2.83 (0.00) | 2.54 (0.00) |                            0.83 |
| R   | 0.01 (0.00) | 0.08 (0.00) | 0.10 (0.00) | 0.02 (0.00) |                            0.00 |
| V   | 0.11 (0.00) | 0.32 (0.06) | 0.24 (0.10) | 0.04 (0.01) |                            0.00 |
| QF  | 0.12 (0.00) | 2.67 (0.02) | 4.31 (1.32) | 2.78 (0.14) |                            0.83 |

### 3.2 kirk1

| Arm | M1 covered | Candidate tracks | Clusters/frame | FG points/frame | Road-surface clusters/frame | M3 stationary >20 s / >60 s |
| --- | ---------: | ---------------: | -------------: | --------------: | --------------------------: | --------------------------: |
| A0  |   38 of 49 |               88 |           2.33 |             372 |                        0.31 |                       1 / 0 |
| F   |   49 of 49 |              206 |          11.25 |           1,351 |                        2.26 |                       7 / 2 |
| FR  |   49 of 49 |              169 |           8.97 |           1,153 |                        0.00 |                       5 / 2 |
| R   |   38 of 49 |               75 |           2.00 |             372 |                        0.00 |                       2 / 0 |
| V   |   43 of 49 |              105 |           2.85 |             398 |                        0.29 |                       1 / 0 |
| QF  |   49 of 49 |              208 |          11.09 |           1,208 |                        2.18 |                       8 / 2 |

### 3.3 Overrides on, the global window retuned

The sweep (Section 4) found the overrides-off curve no better than the overrides-on one, so four
overrides-on configs from the September sweep's frontier were run through the harness the same
way. They keep the region overrides and every other default and change two to four keys:

| Arm | Tuning config                                                                            | Keys changed from the defaults                                          |
| --- | ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| C   | [l3-retune-cm2.json](../../../config/experiments/l3-retune-cm2.json)                     | `closeness_multiplier` 3 → 2.0, `post_settle_update_fraction` 0 → 0.005 |
| CR  | [l3-retune-cm2-floor.json](../../../config/experiments/l3-retune-cm2-floor.json)         | C, and `height_band_floor` −2.8 → −2.0                                  |
| CM  | [l3-retune-cm2-mp8.json](../../../config/experiments/l3-retune-cm2-mp8.json)             | C, and `foreground_min_cluster_points` 5 → 8                            |
| CMR | [l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json) | CM, and `height_band_floor` −2.8 → −2.0                                 |

kirk0:

| Arm | M1 covered | Tracks | Clusters/frame | Object-frame recall |    Car / ped / bus | Unlabelled clusters/frame | Road-surface clusters/frame | Bottom in footprint p10 / median / p90 (m) | M3 >20 s / >60 s |
| --- | ---------: | -----: | -------------: | ------------------: | -----------------: | ------------------------: | --------------------------: | -----------------------------------------: | ---------------: |
| A0  |   11 of 16 |     59 |           3.43 |               0.587 | 0.63 / 0.28 / 0.53 |                      0.31 |                        0.17 |                      −0.01 / +0.26 / +0.91 |            0 / 0 |
| C   |   12 of 16 |    105 |           6.61 |               0.942 | 0.95 / 0.87 / 0.92 |                      1.74 |                        0.33 |                      −0.39 / −0.02 / +0.17 |            2 / 1 |
| CR  |   12 of 16 |     97 |           6.22 |               0.942 | 0.95 / 0.87 / 0.92 |                      1.43 |                        0.00 |                      −0.37 / −0.02 / +0.17 |            2 / 1 |
| CM  |   12 of 16 |     79 |           5.23 |               0.888 | 0.90 / 0.76 / 0.92 |                      0.76 |                        0.25 |                      −0.40 / −0.03 / +0.17 |            2 / 1 |
| CMR |   12 of 16 |     68 |           4.91 |               0.888 | 0.90 / 0.76 / 0.92 |                      0.52 |                        0.00 |                      −0.36 / −0.03 / +0.17 |            2 / 1 |
| FR  |   12 of 16 |    166 |          12.63 |               0.928 | 0.94 / 0.91 / 0.41 |                      8.09 |                        0.00 |                      −0.83 / −0.25 / +0.06 |            4 / 2 |

Per-frame evaluator, reviewed pack, all episodes:

| Arm |  MOTA |  HOTA |  DetA |  AssA |  IDF1 |   IDR |   IDP |    FN |    FP | IDSW |  FM |
| --- | ----: | ----: | ----: | ----: | ----: | ----: | ----: | ----: | ----: | ---: | --: |
| A0  | 0.148 | 0.376 | 0.234 | 0.679 | 0.350 | 0.275 | 0.482 | 2,315 |   722 |  124 |  48 |
| C   | 0.242 | 0.502 | 0.431 | 0.645 | 0.536 | 0.592 | 0.490 |   802 | 1,573 |  435 |  95 |
| CR  | 0.295 | 0.515 | 0.443 | 0.659 | 0.568 | 0.614 | 0.529 |   807 | 1,409 |  397 |  98 |
| CM  | 0.415 | 0.545 | 0.472 | 0.692 | 0.607 | 0.604 | 0.611 |   921 |   877 |  370 |  61 |
| CMR | 0.447 | 0.533 | 0.485 | 0.645 | 0.593 | 0.579 | 0.608 |   928 |   748 |  375 |  59 |

kirk1, which the September sweep did not score:

| Arm | M1 covered | Candidate tracks | Clusters/frame | FG points/frame | Road-surface clusters/frame | M3 >20 s / >60 s |
| --- | ---------: | ---------------: | -------------: | --------------: | --------------------------: | ---------------: |
| A0  |   38 of 49 |               88 |           2.33 |             372 |                        0.31 |            1 / 0 |
| C   |   49 of 49 |              166 |           4.45 |             653 |                        0.56 |            2 / 0 |
| CR  |   49 of 49 |              131 |           3.82 |             625 |                        0.00 |            1 / 0 |
| CM  |   49 of 49 |              125 |           3.60 |             654 |                        0.42 |            2 / 0 |
| CMR |   49 of 49 |              101 |           3.09 |             624 |                        0.00 |            1 / 0 |

## 4. Results: the frontier sweep

The shipped config in each mode reproduces the September sweep's row (recall 0.560, 0.74 spurious
clusters a frame) and the harness's ordering:

| Mode   | Recall | Car / ped / bus    | Fragmentation | Spurious/frame | Clusters/frame |
| ------ | -----: | ------------------ | ------------: | -------------: | -------------: |
| on     |  0.560 | 0.62 / 0.21 / 0.53 |          1.46 |           0.74 |           3.44 |
| vol    |  0.580 | 0.64 / 0.22 / 0.53 |          1.44 |           1.24 |           4.09 |
| off    |  0.864 | 0.91 / 0.65 / 0.35 |          1.26 |          12.94 |          17.29 |
| off_sf |  0.854 | 0.90 / 0.65 / 0.33 |          1.26 |          10.25 |          14.58 |

The best recall reachable under a budget of spurious clusters a frame, fragmentation at most 1.6
(recall / pedestrian recall), over the 518 configs in each mode, beside the September sweep's
11,861 overrides-on configs:

| Spurious/frame | on           | vol          | off          | off_sf       | September, on |
| -------------: | ------------ | ------------ | ------------ | ------------ | ------------- |
|            ≤ 1 | 0.764 / 0.54 | 0.540 / 0.14 | 0.727 / 0.25 | 0.726 / 0.25 | 0.817 / 0.47  |
|            ≤ 2 | 0.836 / 0.62 | 0.828 / 0.53 | 0.813 / 0.48 | 0.808 / 0.47 | 0.862 / 0.55  |
|            ≤ 3 | 0.880 / 0.60 | 0.878 / 0.61 | 0.842 / 0.58 | 0.844 / 0.52 | 0.872 / 0.62  |
|            ≤ 5 | 0.889 / 0.62 | 0.888 / 0.62 | 0.857 / 0.62 | 0.858 / 0.58 | 0.900 / 0.70  |
|            ≤ 8 | 0.896 / 0.69 | 0.894 / 0.64 | 0.884 / 0.65 | 0.880 / 0.66 | 0.901 / 0.70  |
|           ≤ 12 | 0.903 / 0.71 | 0.896 / 0.66 | 0.895 / 0.70 | 0.884 / 0.69 | 0.906 / 0.71  |

Paired against `on` for the same config (518 configs):

| Mode   | Recall change, median (IQR) | Pedestrian change, median | Spurious/frame change, median (IQR) | Configs where recall rises and spurious falls |
| ------ | --------------------------- | ------------------------: | ----------------------------------- | --------------------------------------------: |
| vol    | +0.046 (+0.001 to +0.130)   |                    +0.015 | +0.78 (+0.46 to +1.31)              |                                      0 of 518 |
| off    | +0.469 (+0.060 to +0.670)   |                    +0.487 | +10.04 (+6.89 to +16.67)            |                                      0 of 518 |
| off_sf | +0.459 (+0.052 to +0.654)   |                    +0.455 | +8.17 (+5.72 to +11.51)             |                                      0 of 518 |

With the overrides off, the configs on the frontier at two and five spurious clusters a frame use
`noise_relative` 0.08, four times the shipped value: a uniformly wide window standing in for the
×8 one, and still short of `on` at the same budget. With the overrides on, the frontier configs
keep `noise_relative` 0.02 and narrow `closeness_multiplier` to 2.0 to 3.0 with
`foreground_min_cluster_points` 5 to 8; the September sweep's frontier, finer and locally searched,
says the same, and Section 3.3 runs four of its points through the harness.

## 5. Results: the corpus, label-free

CMR against the shipped config on the first segment of 17 of the 23 corpus sites (the run was
stopped when the machine ran short of disk), 200 s scored after a 70 s warm-up, both arms from
their parameter sets, counted with the definitions of Section 2. CR ran on the first 6 sites
before it was dropped to give CMR the time. None of these sites has labels, so the counts say how
much the output moved, not whether it is right.

| Measure (per site)               | Shipped, median | CMR, median | Median change | Sites up / down | Pooled over 17 sites |
| -------------------------------- | --------------: | ----------: | ------------: | --------------- | -------------------- |
| Clusters a frame                 |            3.91 |        4.98 |         +0.99 | 16 / 1          |                      |
| Road-surface clusters a frame    |           0.183 |           0 |        −0.183 | 0 / 17          |                      |
| Tracks                           |             225 |         235 |           +20 | 11 / 5          | 4,858 → 5,174        |
| Tracks lasting 2 s or more       |             123 |         131 |           +35 | 15 / 1          | 2,095 → 2,771        |
| Moving tracks (2 s, p90 ≥ 2 m/s) |              52 |          80 |           +21 | 16 / 1          | 949 → 1,533          |
| Median track lifetime (s)        |             1.6 |         2.3 |          +0.8 | 16 / 1          |                      |
| Share of tracks under 1 s        |           0.376 |       0.314 |        −0.083 | 2 / 15          |                      |
| Stationary tracks over 20 s (M3) |               1 |           2 |            +1 | 12 / 1          | 39 → 67              |
| Stationary tracks over 60 s (M3) |               0 |           0 |             0 | 6 / 0           | 5 → 15               |

The one site where moving tracks fall, marina-broderick (107 to 80), is also the one where the
shipped config already confirms the most. CR on its 6 sites goes further in the same direction:
clusters a frame double (median 3.0 to 6.0), moving tracks rise 237 to 404 pooled, and stationary
tracks over 20 s rise 18 to 54.

## 6. Interpretation

1. **The volatile tercile's ×8 window is what absorbs road users, not the quiet terciles'.**
   Keeping only the volatile overrides (V) recovers 2.6 points of kirk0 recall and five kirk1 road
   users; keeping only the quiet ones (QF) is the overrides-off arm F to the third decimal on
   recall, unlabelled clusters and road surface. The earlier report's reading, that the ×3 and ×4
   windows on the quiet terciles absorbed the lower part of near-field road users while the ×8 one
   suppressed far-field flicker, has the attribution backwards: one window does both. The likely
   mechanism is that the tercile is drawn from each cell's spread during settling, and a cell on a
   lane through which traffic passed while the grid settled has a wide spread, so the lanes land
   in the volatile tercile with the vegetation. This run did not record the region map, so the
   mechanism is inferred, not measured.
2. **Switching the overrides off moves along the trade-off, never across it.** In none of 518
   configs does it raise recall without raising the spurious clusters, and at every budget the
   best overrides-on config matches or beats the best overrides-off one; the overrides-off configs
   that reach a low budget do it by widening the global window fourfold, rebuilding the ×8 window
   everywhere. E4 removes about two spurious clusters a frame from that flood for about one point
   of recall, which is a step along the same curve. The flood F showed is the price of a window
   too narrow for the scene's moving surfaces, not of the overrides' existence.
3. **The shipped operating point is far inside the overrides-on frontier.** Narrowing the global
   multiplier from 3 to 2 with the overrides kept (C) takes kirk0 recall from 0.587 to 0.942 and
   pedestrians from 0.28 to 0.87, at 1.7 spurious clusters a frame against F's 9.9; with
   `foreground_min_cluster_points` at 8 (CM, CMR) the false positives return to the shipped
   arm's (877 and 748 against 722) at 0.888 recall, and the two hold the best per-frame scores of
   any arm measured on these captures (CMR's MOTA 0.447, CM's HOTA 0.545 and IDF1 0.607, against
   the shipped 0.148, 0.376 and 0.350). kirk1, which no sweep scored, agrees: each of the four
   covers all 49 labelled road users, with 101 to 166 tracks against F's 206.
4. **The road-height floor is free.** It removes every road-surface cluster in every regime it
   was tried in (F, the shipped grid, C and CM), takes 18 to 32 % of kirk0's unlabelled clusters with
   them, and costs at most 0.2 points of recall; with the overrides off it also gives the bus back
   six points. It is a one-key change with no code. The road-attached returns inside the labelled
   footprints (the bottom-in-footprint medians below zero) are not road-surface clusters and the
   floor does not remove them: they are road returns DBSCAN attaches to the object within 0.8 m,
   above the floor's 0.3 m.
5. **Identity switches rise with recall.** Every arm that finds more of the road users switches
   identity more often: 124 for the shipped arm, 370 to 435 for C to CMR, 331 to 467 for the
   overrides-off arms. IDF1 rises regardless, because the shipped arm's few switches come from
   tracking few objects. The quiet-only arm's 467 against F's 338, at identical recall and
   unlabelled clusters, says the quiet terciles' widening costs association when the volatile one
   is off; it is not a direction anyone proposed.
6. **The benchmark's L4 is not the product's.** lidar-bench clusters bare foreground, so every
   spurious and fragmentation figure from the September sweep and this one counts clusters the
   product's height band and size filters would partly remove. Mode against mode inside the sweep
   is sound; a sweep figure against a harness figure is not.

## 7. What this does not establish

- One site. kirk0 and kirk1 are the same street on the same day, and the sweep and the September
  frontier were both scored on kirk0, so C to CMR were chosen on the capture that scores them;
  kirk1's coverage is the only out-of-sample label check, and it counts tracks overlapping a
  first-sighting span, not detections. The per-frame scores use a tuning split.
- The corpus screen is label-free: more clusters and tracks are not more road users, and more
  stationary tracks may be parked cars or flicker.
- The arrival regime: a narrowed window makes a car that parks after settling stay foreground
  longer, and every arm that finds more road users shows more stationary tracks. An 83 s and a
  172 s capture cannot measure the long-uptime behaviour the beam-wise review's Section 5.1
  predicts, so the dwell policy remains a prerequisite for shipping any narrowed L3.
- The volatile-tercile mechanism (lanes classified volatile by traffic during settling) is
  inferred from the attribution, not observed: the harness does not persist the region map.
- No Pi timing. Frame timings on the Mac ran under heavy contention and are not reported.
- A tuning change moves the config fingerprint; none of these sets is a proposal to change the
  defaults without the perf baselines recaptured and a held-out score.

## 8. Recommendations

1. **Do not pursue the overrides-off line further.** Its recall is reachable with the overrides
   on, at a fraction of the false foreground. Retire F, FR and the E4-with-overrides-off arms as
   candidates; keep `support_filter` and the scope experiments as diagnostics.
2. **Take the overrides-on retune to a decision**, CM or CMR, through the beam-wise review's Stage
   B: a held-out score (a full-scene kirk0 label or the W7 capture), the dwell policy for the
   stationary tracks, a Pi timing, and the perf baselines recaptured for the new fingerprint. It
   is a tuning change of two to four keys, no code.
3. **Move the height band floor to road height** with the same change or before it: one key, no
   code, no recall cost on either capture. Its value should come from the mount height per site;
   −2.0 m is right for the 2.3 m mounts recorded so far.
4. **Make the region tercile measure the background, not the traffic.** If the overrides stay, the
   settling spread that sorts cells into terciles should exclude foreground-classified returns
   (or use a robust spread, the review's E5), so that a lane is not classed with the vegetation
   because cars passed during settling. Record the region map in the harness evidence first, so
   the mechanism inferred here can be checked.
5. **Give lidar-bench the product's L4**, or name the difference in its output: a sweep scored on
   bare DBSCAN overstates the spurious clusters of every config by the ones the height band and
   size filters would remove, and cannot tune those keys at all.

## 9. Provenance

- Code: replay experiments `region_overrides_volatile_only` and `region_overrides_quiet_only`
  (`l3grid.BackgroundParams.RegionOverrideScope`) and `lidar-bench -experiment` at `c40603ddb` and
  `27543717a`, with tests. Every replay and sweep run used tools built at `27543717a` with the
  Makefile's version stamp (`0.5.1-pre45`, git sha set) and `-tags=pcap`.
- Parameter sets and fingerprints: `tuning.defaults.json` `fd35b0b28fc1`,
  `height-floor-road.json` `e0fa04785dda`, `l3-retune-cm2.json` `a9f7ca98f44c`,
  `l3-retune-cm2-floor.json` `06c413eb43ef`, `l3-retune-cm2-mp8.json` `1ed895f795a8` and
  `l3-retune-cm2-mp8-floor.json` `20a30e4765fc`, all in
  [config/experiments/](../../../config/experiments/README.md).
- Captures: kirk0 `internal/lidar/perf/pcap/kirk0.pcapng`; kirk1 the first 490,000 UDP packets of
  `pcaps/kirk/kirk1.pcapng` (272.2 s, SHA-256 `296e6264…`); corpus first segments from the D2
  first-segment corpus on the LiDAR volume.
- Labels as in the earlier report: runs `dd98c68e` and `60a4774c`, pack `8e422582` ground truth
  (revision 204), reviewed pack `ad8b9438` with split `kirk0-ad8b9438-tuning-r1`
  (`sha256:b58ee590…`).
- Harness flags: kirk `-warmup 6 -sample-points 1024 -campaign-metrics`, `-duration 0` (kirk0)
  and `172` (kirk1); corpus `-duration 200 -warmup 70 -sample-points 256`. Every replay repeat
  verified.
- Sweep: 2,083 runs (518 configs × 4 modes plus the two references), seed 20261011, scored by the
  September harness's `score_run.py` against `kirk0-tuning/ground-truth.json`; dumps deleted after
  scoring. It ran at five parallel jobs, then two, and was stopped; its rows are complete.
- Evidence databases for the harness arms and the corpus screen are under
  `work/velocity-campaign/blend-20261011/` on the LiDAR volume; scripts and score rows are in the
  run's scratch directory outside the repository.
