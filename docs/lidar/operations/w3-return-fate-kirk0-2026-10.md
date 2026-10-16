# Where kirk0's reviewed returns are lost, return by return, October 2026

[W3](w3-retention-fragmentation-kirk0-2026-10.md) found that half of kirk0's reviewed road-user
returns never reach a cluster and that trucks split at 20 to 50 m, and the
[range-adaptive eps](l4-range-adaptive-eps-2026-10.md) found the splitting is not beam spacing.
Neither could say where a return was lost, because the replay evidence keeps clusters, not the
fate of each return. This run joins the replay's foreground-complete observation log to the
reviewed masks, return by return, under the shipped defaults and the overrides-on retune (CMR).

- **Status:** Measured, October 16, 2026 (experiment
  [geom0004](../../../data/experiments/geom/geom0004.md)). Nothing shipped changes
- **Layers:** L3 foreground, L4 height band, input cap, DBSCAN and cluster filters
- **Related:** [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md),
  [range-adaptive eps](l4-range-adaptive-eps-2026-10.md),
  [region override attribution](l3-region-override-attribution-kirk-2026-10.md),
  [dwell policy on labelled road users](l3-dwell-policy-labelled-cost-2026-10.md)

## Summary

- **Under the shipped L3 the loss is the background model.** 45% of reviewed returns are called
  background by L3 (79% of pedestrians', 60% at 20 to 35 m); everything L4 sets aside is 5.8%, and
  almost all of that is the DBSCAN input cap.
- **The shipped grid's fragments are what absorption leaves.** Of the returns of a fragmented
  object that lie in none of its clusters, 97% were called background by L3 (truck 1 96%). The
  trucks split because parts of them never reach L4, not because DBSCAN cuts them.
- **CMR moves the loss into L4.** L3 now calls 1.8% of reviewed returns background, and the
  largest loss is the input cap: above 8,000 foreground points a frame DBSCAN sees a random
  subsample, and that drops 10.6% of all reviewed returns, 17% of the trucks' and 22% of truck 1's.
  Inside 20 m half the returns in the gaps between CMR's fragments are the cap's. At 20 to 35 m
  the gaps are DBSCAN noise (72%), and at 35 to 50 m the height band (44%) and noise (33%).
- **The reviewed pack was labelled on a wider foreground than the shipped grid now produces.**
  The a0 replay's foreground holds 54.4% of the pack's foreground returns; CMR's holds 97.7%. Of
  each replay's own foreground, 99.9% (a0) and 95.5% (CMR) is in the pack.
- **The join is exact.** Every reviewed return is matched by packet identity or proven absent
  except at the container's last frame, which has no successor to check (1,011 returns under the
  shipped grid and 40 under CMR, left unresolved, of 1,678,623); every match carries the pack's
  float32 coordinates, and on
  the instants W3 also scores the clustered counts agree with it exactly where clusters were not
  capped.

## Method

- **Replays.** `velocity lidar pcap-replay` on kirk0 from the arm's tuning file
  (`config/tuning.defaults.json`; [l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json)),
  `--start-seconds 6 --warmup-seconds 6 --duration-seconds 0 --require-settled`, with
  `--observations` writing the foreground-complete container: one record per frame holding every
  return L3 labelled foreground, before the height band, voxel step and input cap, with each
  cluster's members and each other return's rejection reason. `velocity lidar observations verify
--require foreground-complete` passes on both. The 832 container frames map one to one onto the
  pack's 832 samples; 61 are warm-up and 771 are measured, each within 10 ms of its sample.
- **Join.** The container records each return's packet sequence, block and channel; the version 2
  pack cites its returns by packet key, block and channel. The capture bridges the two: each
  packet's key is a digest of its payload and its sequence is the payload's last four bytes. A
  reviewed return absent from its frame's foreground, and from both neighbouring frames, is
  `l3_background`; one whose neighbour cannot be checked (a missing, empty or gapped neighbour, here
  only the container's last frame) is unresolved. Coordinates are not used to join; they are
  checked after.
- **Fates.** `clustered` (in an accepted cluster, so fragments can be counted), `l3_background`,
  `height_band`, `cluster_input_cap`, `cluster_noise` (DBSCAN noise, or a group below the minimum
  cluster points) and `cluster_shape` (a cluster the diameter or aspect filters rejected; the
  record does not say which). Voxel reduction is off in both arms.
- **Reference.** Pack ad8b9438 in its version 2 form (`ds_67842cb1b22b70e7`), split
  `kirk0-tuning`, road-user classes only. Range bands and objects as in W3.

## Findings

Fate of every reviewed return, the shipped L3 → CMR:

| Group      | Instants | Clustered     | L3 background | Input cap     | DBSCAN noise  | Shape filters | Height band   | Missed      | Fragmented |
| ---------- | -------: | ------------- | ------------- | ------------- | ------------- | ------------- | ------------- | ----------- | ---------- |
| All        |    3,243 | 0.489 → 0.869 | 0.453 → 0.018 | 0.054 → 0.106 | 0.002 → 0.003 | 0.002 → 0.002 | 0.000 → 0.001 | 1,846 → 480 | 36% → 17%  |
| Car        |    1,916 | 0.394 → 0.956 | 0.600 → 0.031 | 0.003 → 0.010 | 0.001 → 0.002 | 0.000 → 0.001 | 0.000 → 0.000 | 1,267 → 179 | 17% → 5%   |
| Truck      |      990 | 0.559 → 0.810 | 0.346 → 0.009 | 0.090 → 0.174 | 0.002 → 0.004 | 0.002 → 0.001 | 0.001 → 0.002 | 339 → 192   | 59% → 49%  |
| Pedestrian |      285 | 0.164 → 0.833 | 0.794 → 0.077 | 0.000 → 0.000 | 0.043 → 0.068 | 0.000 → 0.000 | 0.000 → 0.022 | 215 → 104   | 0% → 0%    |
| Bus        |       52 | 0.274 → 0.837 | 0.672 → 0.011 | 0.000 → 0.000 | 0.008 → 0.013 | 0.046 → 0.128 | 0.000 → 0.010 | 25 → 5      | 37% → 4%   |

By range from the sensor:

| Band    | Instants | Clustered     | L3 background | Input cap     | DBSCAN noise  | Height band   |
| ------- | -------: | ------------- | ------------- | ------------- | ------------- | ------------- |
| 0–20 m  |    1,886 | 0.496 → 0.870 | 0.445 → 0.018 | 0.057 → 0.112 | 0.001 → 0.000 | 0.000 → 0.000 |
| 20–35 m |      567 | 0.370 → 0.913 | 0.602 → 0.018 | 0.000 → 0.008 | 0.012 → 0.021 | 0.000 → 0.003 |
| 35–50 m |      442 | 0.375 → 0.725 | 0.500 → 0.033 | 0.001 → 0.030 | 0.063 → 0.110 | 0.035 → 0.078 |
| 50 m +  |      348 | 0.220 → 0.320 | 0.574 → 0.129 | 0.000 → 0.000 | 0.193 → 0.425 | 0.003 → 0.099 |

The objects W3 named:

| Object                   | Arm     | Clustered | L3 background | Input cap | Noise | Missed of instants |
| ------------------------ | ------- | --------: | ------------: | --------: | ----: | -----------------: |
| Truck 1, obj_35c661db    | shipped |     0.533 |         0.339 |     0.124 | 0.002 |         189 of 542 |
|                          | CMR     |     0.764 |         0.010 |     0.220 | 0.003 |         153 of 542 |
| Truck 2, obj_8e829a06    | shipped |     0.637 |         0.358 |     0.000 | 0.002 |         120 of 412 |
|                          | CMR     |     0.935 |         0.006 |     0.052 | 0.005 |          31 of 412 |
| Pedestrian, obj_a99f83f4 | shipped |     0.019 |         0.948 |     0.000 | 0.032 |         135 of 143 |
|                          | CMR     |     0.871 |         0.058 |     0.000 | 0.054 |          56 of 143 |
| Parked car, obj_788622b5 | shipped |     0.556 |         0.439 |     0.000 | 0.004 |         617 of 771 |
|                          | CMR     |     0.853 |         0.140 |     0.003 | 0.002 |          11 of 771 |

What the gaps between fragments are made of (the returns of a fragmented instant that are in none
of its clusters, by fate; not a spatial measure):

| Group   | Arm     | Gap returns | L3 background | Input cap | DBSCAN noise | Height band | Shape filters |
| ------- | ------- | ----------: | ------------: | --------: | -----------: | ----------: | ------------: |
| All     | shipped |     313,494 |         0.970 |     0.022 |        0.003 |       0.002 |         0.003 |
|         | CMR     |      18,481 |         0.417 |     0.427 |        0.091 |       0.038 |         0.027 |
| 0–20 m  | shipped |     291,354 |         0.974 |     0.023 |        0.000 |       0.000 |         0.002 |
|         | CMR     |      15,586 |         0.474 |     0.492 |        0.013 |       0.000 |         0.021 |
| 20–35 m | shipped |      19,015 |         0.957 |     0.000 |        0.030 |       0.001 |         0.013 |
|         | CMR     |       1,358 |         0.127 |     0.000 |        0.719 |       0.022 |         0.133 |
| 35–50 m | shipped |       3,115 |         0.684 |     0.000 |        0.123 |       0.185 |         0.008 |
|         | CMR     |       1,537 |         0.088 |     0.141 |        0.333 |       0.438 |         0.000 |

Agreement with W3: replaying the shipped arm through the W3 harness and scoring it with
`retention` reproduces W3's tables exactly; on the 2,989 instants both measure, the 2,822 with
uncapped clusters have equal clustered and fragment counts in every case, and on the 167 capped
instants retention's sample expansion (561,150.7 returns) brackets the exact count (561,177).
Fate measures 254 more instants, the frames where L4 accepted no cluster, which the evidence has
no row for, and 10 fewer, two samples just before the scoring window that it counts as warm-up.

## Interpretation

Two different pipelines sit behind kirk0's numbers. Under the shipped L3, retention and
fragmentation are one loss seen twice: the background model takes the returns, the object reaches
L4 in pieces, and nothing L4 could do would join pieces whose middle is absent. That is why the
range-adaptive eps moved nothing, and why the L3 retunes moved everything.

Once L3 keeps the returns, as CMR does, the losses that remain are L4's and are specific. Near
the sensor a truck alone can exceed the input cap, and the uniform subsample both drops returns
and thins the cloud the remaining fragments have to join across; [l4clus0002](../../../data/experiments/l4clus/l4clus0002.md)
raises the cap under CMR to test that. At 20 to 35 m the gaps become DBSCAN noise, where CMR's
minimum cluster size of 8 (the defaults' is 5) and the thinner returns of a receding truck meet. Beyond 35 m the height
band floor CMR moved to −2.0 m starts to cut the lower parts of objects (7.8% of returns at 35 to
50 m, 9.9% beyond), probably where the road falls away from the sensor's level, which this run
does not check. The bus loses 13% of its returns to the shape filters under CMR, its clusters rejected
whole as too large or too long.

The foreground agreement is a separate finding about the reference. The pack's foreground came
from its recording run (build 0.5.1-pre43, the running configuration hash 2319c8f8 that the server
reports for the shipped defaults); a replay of the defaults at this commit keeps 54% of it, and
almost nothing outside it. So kirk0's reviewed masks were drawn on a foreground the shipped
defaults no longer produce, and every recall figure against them is relative to that older
foreground, not to every return the object produced.

## What this does not establish

- **Every return the object produced.** The pack holds only what its recording run's L3 called
  foreground; a return that run called background is in no mask, and appears in neither arm's
  denominator. CMR's 4.5% of foreground outside the pack can be neither credited nor blamed.
- **Why the shipped grid keeps 54% of the pack's foreground.** The difference between pre43 and
  this commit, or between the server path the pack was recorded on and this replay, is not
  attributed.
- **Which shape filter.** The container records a rejected cluster, not whether the diameter or
  the aspect filter rejected it.
- **Spatial gaps.** The gap table counts a fragmented instant's unclustered returns by fate, not
  returns lying between the fragments in space.
- **Held-out performance.** kirk0's pack split is a tuning split; one capture of 83 s.

## Recommendations

- **Raise or restructure the input cap under CMR** and score it (l4clus0002, running): the cap is
  now the largest single loss, and the subsample is uniform, so it thins a near truck where a cap
  by priority or by region would not. Since measured ([input cap](l4-dbscan-input-cap-2026-10.md)):
  lifting it recovers the returns (retention 0.869 to 0.972) but joins no fragments, and costs the
  frame-time tail; the gaps it leaves are L3's.
- **Try the height band floor by range** rather than one height: at −2.0 m it removes road returns
  near the sensor, as intended, and the lower parts of objects beyond 35 m.
- **Read CMR's minimum cluster size of 8 against the 20 to 35 m gaps** before adopting it: it was
  chosen to curb spurious clusters, and it is where the mid-range fragments now break.
- **Attribute the 46% of the pack's foreground the shipped grid no longer keeps**: replay the
  defaults at pre43 against this commit on kirk0, and diff the region overrides first.
- **Use the fate measure for every L3 or L4 arm on kirk0**: it costs about 15 s of CPU an arm and
  says which stage moved.

## Provenance

- **Code:** `lidar-ground-truth-eval fate` and `parse.PacketSequence`, written by an agent as
  a1c46e6d1, 74e149cb7 and cb07d2c11 and cherry-picked as 1fdb207af to ae7c391c0 on
  `claude/experiment-followup`; `velocity` and the tool built stamped at ae7c391c0. A code review
  of the branch then found that a return near an uncheckable neighbour was counted proven absent;
  the fix (c6fdaf995) moves the last frame's absent returns to unresolved, and every figure here is
  from the rescore with it: of those quoted, only the shipped grid's car share (0.602 to 0.600) and
  the parked car's (0.440 to 0.439) moved.
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/followup-20261015/geom0004/replay/{a0,cmr}/`
  (`obs.vrlog`, the replay's VRLOG and logs); the fate JSON and Markdown kept locally. The agent's
  own runs at a1c46e6d1, which match these table for table, and its W3 cross-check are under
  `…/followup-20261015/agent-scratch/agent-fate/`.
- **Capture:** kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`,
  149,741 packets.
- **Reference:** pack `ds_67842cb1b22b70e7` (ad8b9438, version 2), split digest
  `sha256:96f71edbdac4318887a0c187b7e03ba77856964e7e3381d8775c44f039c0064f`.
- **Run:** replays of 33 s and 49 s, scores of 2 min 37 s and 1 min 47 s (the container read from
  the USB LiDAR volume).
