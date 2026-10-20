# What the class gate holds back: following time a class change would release on the corpus

<!-- ignore-style-length -->

Under the published box the provisional following field run finds 212 s of valid following time
on the first segment of 23 corpus sites, and its class gate suppresses 767 s of the 1,593 s it
accounts ([following endpoints](following-box-endpoints-2026-10.md)). On kirk0 most reviewed
vehicles end classed `dynamic` because the car rule asks a large track to be fast or tall
(l8fol0002, on its own branch as `following-class-evidence-kirk-2026-10.md`). Before anyone
changes the classifier, this report measures how much following the class gate is holding back
and whether a rule that keeps the size test and drops the speed-or-height test would release it.
It answers l6cls0001 (the bookkeeping PR).

- **Status:** Measurement complete, 2026-10-20. One unattended run (exp-20261020); no default and
  no classifier changed
- **Layers:** L6 (classification), read where L8 following reads it
- **Related:** [following endpoints](following-box-endpoints-2026-10.md),
  following opportunity (#805),
  [headway report oracle](headway-report-oracle.md#provisional-field-run)

## Answer

**The class gate holds back two thirds of the following the field run could find.** Relabelling
every `dynamic` track as a car, an upper bound that admits non-vehicles by construction, raises
the corpus's valid following time from 212 s to 623 s, at 22 sites instead of 18, and leaves
130 s suppressed for class (the tracks the classifier called pedestrian, cyclist or bird).

**A size-and-motion car rule releases almost none of it.** Keeping the car rule's size test (mean
box length over 3.0 m or mean width over 1.5 m) and replacing its speed-or-height test with a peak
speed of 3 m/s raises valid time only to 243 s: it releases 43 of the 637 class-suppressed seconds
the upper bound releases (7 %), against the half the spec set before the reruns, and 157 of the
399 tracks it relabels (39 %) are not vehicle-sized, against the one in ten allowed. It fails both
conditions.

By the spec's reading, the rule's features are the problem, not its thresholds: the tracks that
hold the class-suppressed following do not pass the size test on their mean box. Of the 1,367 vehicle-sized tracks that end `dynamic`, 957 (70 %)
fail the size test on their mean box, while at least one cluster in ten of each is 3 m or longer;
speed alone stops only 168 (12 %). The
next lever is the measured extent and height of a track (the box's base on the ground surface,
[ground0004](../../../data/experiments/ground/ground0004.md), and one track per vehicle), not the
car rule's thresholds.

## Question

The spec's reading, written 2026-10-20 before the reruns: per arm, valid following seconds,
accounted seconds, seconds suppressed for class and encounters; for `size-moving` the tracks
relabelled and how many are not vehicle-sized. If `size-moving` releases at least half of the
class-suppressed seconds that `all-dynamic` releases, with no more than one in ten of its
relabelled tracks short of vehicle-sized, a classifier experiment on that rule is worth
specifying; if less, the features are the problem and the ground line comes first.

## Method

A counterfactual on the field run's own reading, with no replay. The box reader takes a track's
class from `lidar_tracks.object_class` for the whole passage, so rewriting that column on a copy
of each case's l8fol0003 box-arm evidence and running `velocity report headway --boxes` again
gives exactly what the field run would report had the classifier ended those tracks so. The
encounters and exposure windows the l8fol0003 field run stored were cleared on every copy, for
every arm, so each computed its own; the `as-run` arm reproduces l8fol0003's box figures exactly.

| Arm           | Relabelled as car                                                                                                                    |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `as-run`      | nothing                                                                                                                              |
| `size-moving` | a `dynamic` track with `bounding_box_length_avg` over 3.0 m or `bounding_box_width_avg` over 1.5 m, and `max_speed_mps` of 3 or more |
| `all-dynamic` | every `dynamic` track                                                                                                                |

Vehicle-sized is l8fol0001's definition: lasting 2 s or more, with a p90 cluster longer side of
3.0 m or more.

## Findings

### Pooled

| Arm         | Valid s | Accounted s | Suppressed for class s | Sites with valid time | Tracks relabelled | Of them not vehicle-sized |
| ----------- | ------: | ----------: | ---------------------: | --------------------: | ----------------: | ------------------------: |
| as-run      |   212.3 |     1,592.5 |                  767.1 |                    18 |                 – |                         – |
| size-moving |   242.8 |     1,592.5 |                  724.6 |                    19 |               399 |                157 (39 %) |
| all-dynamic |   622.5 |     1,592.5 |                  130.3 |                    22 |             5,993 |              4,626 (77 %) |

Accounted time and encounters (852) are the same under every arm: class decides suppression, not
which instants are accounted. Under `all-dynamic` the next reasons are an ambiguous leader
(247 s), no common path (237 s) and no observation (191 s), which no class change touches.

### By site

Valid following seconds, and seconds suppressed for class, per arm.

| Site                  | Valid as-run | size-moving | all-dynamic | Class-suppressed as-run / size-moving / all-dynamic | Relabelled size-moving (not vehicle-sized) |
| --------------------- | -----------: | ----------: | ----------: | --------------------------------------------------- | ------------------------------------------ |
| 1st-mission           |          0.3 |         0.4 |        15.4 | 44 / 44 / 9                                         | 12 (1)                                     |
| 3rd-folsom            |          1.1 |         1.1 |        16.3 | 30 / 30 / 11                                        | 9 (4)                                      |
| ashbury-downey        |         18.1 |        18.1 |        26.6 | 9 / 9 / 0                                           | 2 (0)                                      |
| broadway-gough        |          0.4 |         2.4 |        11.9 | 16 / 14 / 0                                         | 12 (5)                                     |
| bush-powell           |          0.8 |         0.8 |        25.3 | 66 / 66 / 35                                        | 14 (6)                                     |
| california-leon-baker |          9.3 |         9.3 |        17.4 | 10 / 10 / 0                                         | 3 (2)                                      |
| columbus-broadway     |          1.0 |         6.6 |        65.6 | 92 / 86 / 14                                        | 22 (13)                                    |
| columbus-north-point  |          0.0 |         0.0 |         1.1 | 3 / 2 / 0                                           | 8 (2)                                      |
| embarcadero-bay       |         24.9 |        24.9 |        51.2 | 89 / 89 / 26                                        | 6 (1)                                      |
| embarcadero-broadway  |         37.9 |        43.6 |        68.8 | 38 / 31 / 1                                         | 30 (6)                                     |
| embarcadero-bryant    |         47.3 |        48.8 |        67.4 | 45 / 43 / 7                                         | 77 (38)                                    |
| franklin-mcallister   |          3.6 |         4.5 |        34.7 | 39 / 37 / 1                                         | 41 (13)                                    |
| fulton-divisadero     |          0.0 |         0.0 |        11.5 | 15 / 15 / 1                                         | 7 (1)                                      |
| haight-divisadero     |          0.0 |         0.0 |         5.2 | 19 / 15 / 0                                         | 8 (1)                                      |
| howard-6th            |          0.0 |         0.3 |        16.4 | 38 / 38 / 0                                         | 9 (2)                                      |
| hyde-ofarrell         |          4.8 |         4.9 |        17.2 | 18 / 18 / 0                                         | 19 (8)                                     |
| laguna-eddy           |          0.7 |         1.4 |        21.0 | 36 / 35 / 1                                         | 18 (10)                                    |
| lombard-broderick     |          0.2 |         0.2 |        42.1 | 62 / 62 / 2                                         | 23 (12)                                    |
| lombard-laguna        |          9.3 |        10.1 |        28.0 | 27 / 26 / 2                                         | 26 (19)                                    |
| marina-broderick      |         16.1 |        16.4 |        22.0 | 16 / 15 / 0                                         | 15 (0)                                     |
| marina-webster-beach  |         18.6 |        18.6 |        19.0 | 0 / 0 / 0                                           | 1 (1)                                      |
| pierce-haight         |          0.0 |         0.0 |         0.0 | 8 / 7 / 7                                           | 9 (6)                                      |
| van-ness-sacramento   |         17.9 |        30.4 |        38.4 | 46 / 32 / 15                                        | 28 (6)                                     |

The upper bound releases most where traffic is dense and the classifier admits least: Columbus at
Broadway (1 s to 66 s), Lombard at Broderick (0.2 s to 42 s), Franklin, Bush. Where the classifier
already admits most vehicles (marina-webster-beach, ashbury-downey) there is little to release.

### Why the vehicle-sized tracks miss the rule

Of the corpus's vehicle-sized tracks that the classifier ended `dynamic`, under `size-moving`'s
two tests:

| Outcome                                     | Tracks | Share |
| ------------------------------------------- | -----: | ----: |
| pass both (relabelled)                      |    242 |  18 % |
| fail the size test only                     |    713 |  52 % |
| fail the speed test only (peak under 3 m/s) |    168 |  12 % |
| fail both                                   |    244 |  18 % |
| **vehicle-sized and `dynamic`**             |  1,367 |       |

The size test reads the track's mean box, and the mean is dragged down by the frames where only
part of the vehicle is seen: at Columbus at Broadway the vehicle-sized `dynamic` tracks' mean
length is 2.6 m at the median and 3.6 m at p90, mean width 1.1 and 1.5 m; at Lombard at
Broderick 2.4 and 3.2 m long, 1.0 and 1.4 m wide, at peak speeds of 8 to 11 m/s, so speed is not
what stops them. The vehicle-sized test reads the p90 of the clusters' longer side, which the same
tracks pass. A rule that judges size by the track's largest consistent extent rather than its
mean would see these as vehicles; the box model's held extents are such an extent.

## What it does not establish

- **That a relabelled track is a vehicle.** The corpus has no class labels; `all-dynamic` admits
  pedestrians' and cyclists' tracks that the classifier did not label as such, and its valid time
  is an upper bound on what a class change could release, not following that happened.
- **What the classifier would do online.** It decides on running features at each frame; the
  counterfactual applies a rule to the track's final means once.
- **Anything held out**, or on kirk0's reviewed objects, which the class measure (l8fol0002) reads.

## Provenance

| Item        | Value                                                                                                                                                          |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | spec at `e606e6750`, before any rerun; field runs with `velocity` at `02a543bbd` (the l8fol0003 branch, box-less estimates counted as gaps)                    |
| Evidence    | copies of l8fol0003's box-arm evidence databases (defaults `tuning.defaults.json`, harness `d76ab354d`), first segment of 23 corpus sites, 70 s warm-up, 200 s |
| Raw outputs | per-case summaries under the run's scratch directory (`results/l6cls0001`); the modified copies were discarded; not in Git                                     |

## Recommendations

1. **Do not tune the car rule's thresholds to release following.** The rule fails because the
   tracks holding the following do not look vehicle-sized on their mean box, not because a
   threshold is a little high.
2. **Measure extent and height where they are not lost.** The box's base on the ground surface
   (ground0004) and its length with one track per vehicle are the inputs a class rule needs; a
   classifier experiment should follow them, scored by the per-frame class measure on kirk0 with
   false applicability held at zero, and held out on label0001's window.
3. **Report the gate's share in the field run.** Two thirds of the following the field run could
   find is held back by class; the provisional report should say so beside its valid time.
