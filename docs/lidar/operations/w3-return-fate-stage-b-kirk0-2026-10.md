# Where the Stage B arms lose a reviewed return, on kirk0

<!-- ignore-style-length -->

The return-fate report found that under CMR the returns between a truck's fragments at 20 to
35 m were ones L3 still absorbs, and the input cap report asked whether the two Stage B options,
the dwell policy and the cell tercile, touch them. This report reads the fate of every reviewed
return under CMR and both options, with today's defaults beside them. It answers
[l3bg0019](../../../data/experiments/l3bg/l3bg0019.md), which `velocity lidar pcap-replay
--experiment` (#812) made runnable.

- **Status:** Measurement complete, 2026-10-21; no default changed and no Stage B arm changed
- **Layers:** L3 (background, the region overrides, the dwell policy), L4 (rejections), scored
  against reviewed returns
- **Related:** [W3 return fate](w3-return-fate-kirk0-2026-10.md),
  [DBSCAN input cap](l4-dbscan-input-cap-2026-10.md),
  [L3 candidates and the dwell policy](l3-candidates-corpus-dwell-2026-10.md),
  [replay L3 consolidation](replay-l3-consolidation-2026-10.md)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) and the
  [CMR set](../../../config/experiments/l3-retune-cm2-mp8-floor.json); the two options are replay
  experiments

## Answer

**Neither Stage B option touches the mid-range gaps, and under CMR those gaps are no longer
L3's.** Of the 1,358 reviewed returns in the gaps between fragments at 20 to 35 m under CMR, 13 %
are lost to L3 background and 72 % to DBSCAN noise. The dwell policy leaves them exactly as they
are. The cell tercile sends more of them to background (13 % to 22 %), not fewer. By the spec's
reading, an arm that moved a quarter of the background-lost gap returns into a cluster would be a
lead for the W3 fix; neither moves any, so the fix is a rule of its own, and at 20 to 35 m it is
an L4 rule: the minimum cluster size that turns those returns into noise, which
[l4clus0003](../../../data/experiments/l4clus/l4clus0003.md) asks about.

The premise came from the old L3. The earlier report's 20 to 35 m gaps were 96 % L3 background
under the L3 it called shipped; under CMR, and as much under today's consolidated defaults (46 %
background, 32 % noise), most of what remains is noise and shape rejections.

**What the options do instead.**

- **The dwell policy absorbs the reviewed parked car.** kirk0's object `obj_788622b5`, a car
  present at all 771 measured instants, loses its track after 20 s stationary: its clustered share
  falls from 0.853 to 0.748, a quarter of its returns go to background, and its missed instants
  go from 11 to 505, all of the arm's rise (480 to 970). That is the policy doing what it is for; on kirk0 the parked car is a
  reviewed road user, so the cost is counted here. Everything else is unchanged.
- **The cell tercile cuts fragmentation by losing returns, not by joining fragments.** Fragmented
  instants fall from 474 to 385 (within 20 m, 199 to 103), but cars keep 0.906 of their returns
  against 0.956, background doubles overall (0.018 to 0.039), and the bus's shape rejections rise
  from 0.128 to 0.287. Fewer fragments here means less of each vehicle, not whole vehicles.

## Question

The spec's reading, written 2026-10-18 before the replays: per arm, reviewed returns kept and lost
by stage, the share of fragmented instants' missing returns at each stage, pedestrians and trucks
apart, and the 20 to 35 m gap returns by stage. An arm that moves a quarter or more of the gap
returns from background into a cluster is a lead for the W3 fix; one that moves none says the fix
is a rule of its own, not a Stage B arm.

**Fixed before the recordings (2026-10-20).** A Stage B arm "moves" gap returns relative to `cmr`,
the set both options add to: the share of `cmr`'s 20 to 35 m gap returns lost to L3 background that
the arm puts into a cluster. `a0` is today's defaults, which since #787 carry the once-substituted
L3, so its row is reported for reference and not compared with the earlier report's.

## Method

Each arm was recorded with `velocity lidar pcap-replay --observations` at `main` (`8c94e23df`) with
the earlier fate report's settings: kirk0 whole, 6 s start and warm-up, settled before scoring,
UDP port 2369, every container verified foreground-complete (832 frames, no gaps). Each recording
names its experiment in its manifest and in the observation container's provenance.
`lidar-ground-truth-eval fate` read each on the reviewed pack's `kirk0-tuning` split (771 measured
samples; 289 warm-up instants not measured; 1,678,583 of 1,678,623 reviewed returns joined) with
range bands at 20, 35 and 50 m.

| Arm         | Parameter set                                     | Replay experiment               |
| ----------- | ------------------------------------------------- | ------------------------------- |
| `a0`        | `config/tuning.defaults.json`                     | none                            |
| `cmr`       | `config/experiments/l3-retune-cm2-mp8-floor.json` | none                            |
| `cmr-dwell` | `config/experiments/l3-retune-cm2-mp8-floor.json` | `dwell_policy`                  |
| `cmr-cell`  | `config/experiments/l3-retune-cm2-mp8-floor.json` | `region_overrides_cell_tercile` |

## Findings

### Every reviewed return, by class

Share of each class's reviewed returns by fate; missed instants reached no cluster; fragmented
instants reached two or more.

| Class (returns)    | Arm       | Clustered | L3 background | Input cap | DBSCAN noise | Shape | Missed instants | Fragmented instants |
| ------------------ | --------- | --------: | ------------: | --------: | -----------: | ----: | --------------: | ------------------: |
| all (1,678,623)    | a0        |     0.870 |         0.020 |     0.106 |        0.002 | 0.002 |             326 |                 602 |
|                    | cmr       |     0.869 |         0.018 |     0.106 |        0.003 | 0.002 |             480 |                 474 |
|                    | cmr-dwell |     0.865 |         0.022 |     0.105 |        0.003 | 0.002 |             970 |                 473 |
|                    | cmr-cell  |     0.845 |         0.039 |     0.108 |        0.003 | 0.004 |             416 |                 385 |
| car (674,868)      | a0        |     0.955 |         0.033 |     0.010 |        0.001 | 0.001 |             126 |                 112 |
|                    | cmr       |     0.956 |         0.031 |     0.010 |        0.002 | 0.001 |             179 |                  82 |
|                    | cmr-dwell |     0.946 |         0.041 |     0.010 |        0.002 | 0.001 |             672 |                  81 |
|                    | cmr-cell  |     0.906 |         0.081 |     0.010 |        0.002 | 0.000 |             175 |                  37 |
| truck (981,611)    | a0        |     0.811 |         0.010 |     0.174 |        0.002 | 0.002 |             122 |                 481 |
|                    | cmr       |     0.810 |         0.009 |     0.174 |        0.004 | 0.001 |             192 |                 390 |
|                    | cmr-dwell |     0.811 |         0.009 |     0.173 |        0.004 | 0.001 |             189 |                 390 |
|                    | cmr-cell  |     0.806 |         0.010 |     0.177 |        0.004 | 0.001 |             163 |                 348 |
| pedestrian (5,142) | a0        |     0.849 |         0.119 |     0.000 |        0.031 | 0.001 |              75 |                   0 |
|                    | cmr       |     0.833 |         0.077 |     0.000 |        0.068 | 0.000 |             104 |                   0 |
|                    | cmr-dwell |     0.833 |         0.077 |     0.000 |        0.068 | 0.000 |             104 |                   0 |
|                    | cmr-cell  |     0.856 |         0.078 |     0.000 |        0.046 | 0.000 |              71 |                   0 |
| bus (17,002)       | a0        |     0.862 |         0.038 |     0.000 |        0.008 | 0.091 |               3 |                   9 |
|                    | cmr       |     0.837 |         0.011 |     0.000 |        0.013 | 0.128 |               5 |                   2 |
|                    | cmr-dwell |     0.837 |         0.011 |     0.000 |        0.013 | 0.128 |               5 |                   2 |
|                    | cmr-cell  |     0.675 |         0.015 |     0.000 |        0.012 | 0.287 |               7 |                   0 |

The trucks' largest loss is the DBSCAN input cap (17 % of their returns) under every arm, as the
input cap report found; neither option moves it.

### The gaps between fragments, by range

Reviewed returns that are in none of a fragmented instant's clusters, by fate.

| Range   | Arm       | Fragmented instants | Gap returns | L3 background | Height band | Input cap | DBSCAN noise | Shape |
| ------- | --------- | ------------------: | ----------: | ------------: | ----------: | --------: | -----------: | ----: |
| 0–20 m  | a0        |                 220 |      21,360 |         0.419 |       0.000 |     0.556 |        0.007 | 0.019 |
|         | cmr       |                 199 |      15,586 |         0.474 |       0.000 |     0.492 |        0.013 | 0.021 |
|         | cmr-dwell |                 198 |      19,532 |         0.379 |       0.000 |     0.594 |        0.010 | 0.016 |
|         | cmr-cell  |                 103 |      14,293 |         0.181 |       0.000 |     0.806 |        0.008 | 0.005 |
| 20–35 m | a0        |                 188 |       1,638 |         0.460 |       0.010 |     0.000 |        0.324 | 0.206 |
|         | cmr       |                 171 |       1,358 |         0.127 |       0.022 |     0.000 |        0.719 | 0.133 |
|         | cmr-dwell |                 171 |       1,358 |         0.127 |       0.022 |     0.000 |        0.719 | 0.133 |
|         | cmr-cell  |                 171 |       1,310 |         0.225 |       0.013 |     0.000 |        0.592 | 0.169 |
| 35–50 m | a0        |                 185 |       2,176 |         0.078 |       0.557 |     0.097 |        0.256 | 0.011 |
|         | cmr       |                 104 |       1,537 |         0.088 |       0.438 |     0.141 |        0.333 | 0.000 |
|         | cmr-dwell |                 104 |       1,549 |         0.088 |       0.434 |     0.142 |        0.336 | 0.000 |
|         | cmr-cell  |                 110 |       1,687 |         0.061 |       0.399 |     0.217 |        0.323 | 0.000 |

The reading's measure, relative to `cmr`'s 172 gap returns at 20 to 35 m that L3 absorbs: the
dwell policy puts none of them into a cluster (its 20 to 35 m row is identical), and the cell
tercile puts none either, while 123 more gap returns go to background under it than under `cmr`.
Within 20 m the gaps are the input cap's and L3's in roughly equal parts; beyond 35 m, the height
band's.

## What it does not establish

- **Anything beyond kirk0's tuning split.** One capture, read many times before; nothing here is
  held out, and no corpus has reviewed returns.
- **The options' value where they were designed to help.** The dwell policy was built to clear
  long stationary tracks on five-minute windows (the dwell reports); kirk0 is 83 s with one parked
  car, so this measures its cost on a reviewed stationary road user, not its benefit.
- **A fragmentation rate.** Fragmented instants are counted where reviewed returns reach two or
  more clusters; W3 (geom0003) is the fragmentation measure.

## Provenance

| Item        | Value                                                                                                                                                                                                                |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | `velocity` and `lidar-ground-truth-eval` at `main` `8c94e23df`, built with the Makefile's LDFLAGS; spec approved with its definitions at `5d8f3f29f`, before any recording                                           |
| Tuning      | `tuning.defaults.json` (sha256 `e6cf6c8a…`, params `sha256:15fe4df8…`); `l3-retune-cm2-mp8-floor.json` (`6bc94773…`): `cmr` params `sha256:713926e5…`, `cmr-dwell` `sha256:9a33403a…`, `cmr-cell` `sha256:d6a80b17…` |
| Capture     | kirk0 (`internal/lidar/perf/pcap/kirk0.pcapng`, 149,741 packets, `sha256:2864ebde…`)                                                                                                                                 |
| References  | reviewed pack `ds_67842cb1b22b70e7`, split `kirk0-tuning` of `kirk0-reference-tuning-r1.json` (two episodes), range bands 20, 35, 50 m                                                                               |
| Raw outputs | observation containers, VRLOGs and fate JSON under the run's scratch directory (`exp-20261020/stage/l3bg0019`, `results/l3bg0019`); not in Git                                                                       |

## Recommendations

1. **Take the mid-range fragments to L4.** At 20 to 35 m under CMR the gaps are DBSCAN noise;
   run [l4clus0003](../../../data/experiments/l4clus/l4clus0003.md) (CMR's minimum cluster size of 8
   against the default), on kirk0 only, since kirk1 has no reviewed pack to score retention on.
2. **Keep the cell tercile out of Stage B unless it buys something else.** On kirk0 it lowers
   fragmentation only by dropping returns, at a cost of 5 points of car returns and over twice the
   bus's shape rejections.
3. **Count the dwell policy's cost on reviewed parked vehicles** wherever Stage B scores it; on
   kirk0 the one parked car accounts for all of the rise in missed instants.
