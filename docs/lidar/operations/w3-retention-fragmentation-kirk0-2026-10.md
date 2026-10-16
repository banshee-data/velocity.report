# W3 retention and fragmentation on kirk0, October 2026

The geometry convergence plan's W3 asks, for every reviewed object, how much of it reaches a
cluster and into how many clusters its returns fall. Until now nothing measured either: the
per-frame evaluator scores tracks, and the annotation-scored sweeps scored clusters with an L4 that
is not the product's. This report measures the shipped L4 on kirk0's reviewed pack with a new
`retention` subcommand of `lidar-ground-truth-eval`, and is the baseline the range-adaptive eps of
[l4clus0001](../../../data/experiments/l4clus/l4clus0001.md) is scored against.

- **Status:** Measured, October 13, 2026 (experiment
  [geom0003](../../../data/experiments/geom/geom0003.md)). Nothing shipped changes
- **Layers:** L3 foreground, L4 clustering
- **Related:** [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md)
  (W3), [L3 region override attribution](l3-region-override-attribution-kirk-2026-10.md),
  [experiment queue](../../../data/experiments/EXPERIMENTS.md)

## Summary

- **Half of the labelled returns reach a cluster.** Of 1.66 million reviewed-object returns over
  2,999 object-instants, 0.494 end up in an L4 cluster. The mean per instant is 0.214, because
  large near objects carry most returns: 1,595 instants (53%) reach no cluster at all.
- **Fragmentation is a truck problem and a middle-distance problem.** Of the instants that reach
  a cluster, 36% are split over two or more clusters: 59% of truck instants (truck 1 in 73% of
  its instants, up to four clusters), 17% of cars, no pedestrian. By range it peaks at 20 to 35 m
  (54% of instants, 1.75 clusters on average) and 35 to 50 m (46%, 1.55), against 30% inside 20 m
  and 3% beyond 50 m, where most objects are missed outright.
- **No merges.** No cluster held returns of two reviewed objects. In the trucks' episode truck 1
  recedes from 7 m to 37 m while truck 2 approaches from 39 m to 8 m, so they are never within the
  0.8 m neighbourhood of each other: the merging test l4clus0001 was to use them for starts from
  zero and can only get worse.
- **One parked car accounts for most near misses.** A car about 10 m from the sensor, labelled in
  all 706 measured frames, reaches no cluster in 550 of them: 550 of the 917 missed instants inside
  20 m. It is in the background, as a parked car should be for the tracker but not for the label.

## Method

- **Replay.** The shipped defaults (`config/tuning.defaults.json`, no replay options) through
  `lidar-state-estimation-baseline` on kirk0 (`-warmup 6 -duration 0 -sample-points 1024`), at
  commit 995036d64, as unit `replay/a0/kirk0` of geom0003.
- **Reference.** The reviewed pack ad8b9438 (dataset ds_95be6fb22498b44a), split `kirk0-tuning`
  of `kirk0-ad8b9438-tuning-r1.json`, in its archived version 1 form, since main did not yet read
  version 2 packs. Once #744 merged, main reads version 2 only: scored from the same evidence
  against the version 2 pack and its split `kirk0-reference-tuning-r1.json`, every table below is
  identical.
- **Matching.** The evidence keeps up to 1,024 sample points per cluster, and every one of them
  is bit-identical to a pack point of the same sample (459,935 of 460,065; the rest belong to no
  sample in the pack), so a cluster's points are matched to reviewed masks by exact float32
  identity, with no distance tolerance. A cluster larger than its sample (171 of 2,423) has its
  count per object expanded from the sample (Horvitz-Thompson); those 167 instants are marked
  approximate, and their fragment counts are lower bounds.
- **Not measured.** 279 instants before the warm-up ended, and 254 instants at 67 samples where
  the replay produced no evidence frame (706 frames against about 770 in the recording). The
  evidence cannot tell a missing frame from one with nothing clustered, so these are excluded
  rather than counted as misses.
- **Definitions.** Retention is clustered labelled returns over labelled returns (pooled), and
  per instant (mean). Main is the share in the cluster holding most of the object. An instant is
  fragmented when the object's returns are in two or more clusters, and merged when one of its
  clusters also holds another reviewed object's returns. Range bands are by the object's distance
  from the sensor.

## Findings

By class:

| Class      | Instants | Retention |  Mean |  Main | Missed | Fragmented | Mean clusters | Max |
| ---------- | -------: | --------: | ----: | ----: | -----: | ---------: | ------------: | --: |
| All        |    2,999 |     0.494 | 0.214 | 0.469 |  1,595 |  505 (36%) |          1.45 |   6 |
| Car        |    1,760 |     0.404 | 0.143 | 0.400 |  1,107 |  109 (17%) |          1.17 |   3 |
| Truck      |      923 |     0.560 | 0.366 | 0.520 |    272 |  386 (59%) |          1.78 |   6 |
| Pedestrian |      266 |     0.172 | 0.171 | 0.172 |    193 |     0 (0%) |          1.00 |   1 |
| Bus        |       50 |     0.275 | 0.173 | 0.269 |     23 |   10 (37%) |          1.41 |   3 |

By range from the sensor:

| Band    | Instants |   Returns | Retention | Missed | Fragmented | Mean clusters | Max |
| ------- | -------: | --------: | --------: | -----: | ---------: | ------------: | --: |
| 0–20 m  |    1,771 | 1,569,317 |     0.501 |    917 |  258 (30%) |          1.36 |   4 |
| 20–35 m |      510 |    76,166 |     0.382 |    225 |  154 (54%) |          1.75 |   6 |
| 35–50 m |      414 |    16,756 |     0.387 |    215 |   91 (46%) |          1.55 |   4 |
| 50 m +  |      304 |     2,212 |     0.307 |    238 |     2 (3%) |          1.03 |   2 |

Objects that shape the totals:

| Object            | Class      | Instants | Retention | Missed | Fragmented | Max clusters |
| ----------------- | ---------- | -------: | --------: | -----: | ---------: | -----------: |
| obj_35c661db (T1) | truck      |      493 |     0.534 |    140 |  257 (73%) |            4 |
| obj_8e829a06 (T2) | truck      |      394 |     0.637 |    102 |  129 (44%) |            6 |
| obj_788622b5      | car        |      706 |     0.596 |    550 |    14 (9%) |            2 |
| obj_a99f83f4      | pedestrian |      120 |     0.020 |    112 |     0 (0%) |            1 |
| obj_4116ba22      | car        |       25 |     0.000 |     25 |     0 (0%) |            0 |

- **Truck 1** is split in 109 of its 133 instants inside 20 m and 58 of its 67 at 20 to 35 m:
  the geometry plan's "truck 1's cab was two separate clusters", now counted.
- **Truck 2**, approaching from 36 m to 15 m, keeps only 0.18 to 0.34 of its returns in one
  cluster: the plan's "truck 2's cab never reached the tracker" is a retention loss, not a
  fragmentation one.
- **Pedestrians are never split but mostly lost**: 193 of 266 instants reach no cluster, and one
  pedestrian (obj_a99f83f4) keeps 2% of its returns. The minimum cluster size and the region
  overrides are the candidates; this evidence cannot say which.

## Interpretation

Fragmentation in the middle distances is what a range-adaptive neighbourhood is meant to fix, and
it is large enough to measure a change in: half the instants at 20 to 50 m are split. Retention is
the larger loss, though: inside 20 m half the returns of labelled objects never reach a cluster,
and the parked car shows that some of that is the background doing its job on an object the label
counts. A clustering change cannot recover returns that L3 or the size filters have already
dropped, so l4clus0001 should be read on fragmentation and merges, and retention read for harm.

## What this does not establish

- **Held-out performance.** `kirk0-tuning` is a tuning split; these numbers describe the capture
  the defaults were tuned near and must not be quoted as held out.
- **Why a return was lost.** The evidence records clusters, not the fate of each return: L3
  background, the height band, DBSCAN noise and the size filters all look the same here. The
  replay's foreground-complete observation log (`velocity lidar replay -observations`) records the
  cause per return and is the next measurement.
- **Retention of every return the object produced.** The pack is foreground-only: retention is
  against what the recording run's L3 called foreground.
- **Near-merge distances.** The plan's D3 wants the distance between different objects' clusters;
  only shared clusters are counted here.
- **Other captures.** One capture, 83 s, one direction of truck traffic.

## Recommendations

- **Score l4clus0001 on fragmentation at 20 to 50 m and on merges**, against this baseline (54%
  and 46%; no merges), with retention read only for harm.
- **Attribute lost returns** with the replay's observation log before tuning against retention:
  the pedestrian losses and truck 2's cab are the cases to explain.
- **Exclude parked objects from retention** or report them apart: obj_788622b5 alone is 60% of
  the near misses and is the background working as designed.

## Provenance

- **Tool:** `lidar-ground-truth-eval retention`, commit 74e135f57 on `claude/experiment-overnight`
  (built stamped, 0.5.1-pre45); replay harness stamped at 995036d64.
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/overnight-20261013/geom0003/replay/a0/kirk0/`
  (`ev/observations.db`, source manifest, run log); the full report JSON kept locally, outside the
  repository.
- **Reference:** pack digest `sha256:95be6fb22498b44a…`, split digest
  `sha256:65c2dc41e73d8a03cc28ef96e2582e94158f0ed7d84308414de8050d7930b3d3` (version 1, as run);
  version 2 pack `ds_67842cb1b22b70e7`, split digest
  `sha256:96f71edbdac4318887a0c187b7e03ba77856964e7e3381d8775c44f039c0064f`, rescored at the
  merge with main.
- **Run:** replay 1 min 25 s and score 4 min 3 s on the M1 Pro (estimated 1 min and 5 min); the
  measure itself was built by an agent in 15 minutes against an 8-hour estimate for a person.
