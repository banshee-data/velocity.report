# The dwell policy on labelled road users, kirk0 and kirk1, October 2026

The [dwell policy](l3-dwell-policy-arrival-2026-10.md) removed every stationary track over 60 s
from the overrides-on retune (CMR) on the 39-minute Lombard–Laguna placement, but that capture has
no labels, and among its 86 absorptions were 16 tracks classed pedestrian. This run puts the
policy on the two labelled kirk captures, under the shipped L3 and CMR, to put a labelled number
on what it costs.

- **Status:** Measured, October 16, 2026 (experiment
  [l3bg0011](../../../data/experiments/l3bg/l3bg0011.md)). Nothing shipped changes
- **Layers:** L3 background, L5 tracks
- **Related:** [dwell policy and the arrival regime](l3-dwell-policy-arrival-2026-10.md),
  [region override attribution](l3-region-override-attribution-kirk-2026-10.md),
  [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md)

## Summary

- **The policy costs no moving road user on kirk.** It absorbs nothing under the shipped L3 on
  kirk0, one track under CMR, and one or two tracks classed bird or dynamic on kirk1. Object-frame
  recall against kirk0's sweep ground truth is unchanged under the shipped L3 and rises from 0.8885
  to 0.8901 under CMR, with every class equal or up; CMR keeps all 49 of kirk1's labelled road
  users.
- **The one absorption on kirk0 is the parked car, and the reviewed pack counts it.** Under CMR,
  obj_788622b5, the car parked about 10 m from the sensor, is recovered (missed in 11 of 773
  instants, against 550 under the shipped L3); the policy absorbs it after 20 s, and it is then
  missed in 505. That one object is the whole of the per-frame evaluator's change: false negatives
  rise from 928 to 1,419 and HOTA falls from 0.533 to 0.494. It is the policy doing what it is for,
  scored against a label that counts a parked car in every frame.
- **The two kirk0 references disagree about that object.** The sweep's ground truth labels the
  same spot noise (obj_3c67a8a9, reviewed), the reviewed pack a car.
- **CMR's W3, measured for the first time:** retention 0.869 against the shipped 0.494,
  fragmented instants 17% against 36% (35% against 54% at 20 to 35 m), pedestrians 0.83 against
  0.17. Two instants merge two reviewed objects, where the shipped L3 merges none.
- **The kirk windows cannot show the cost the Lombard–Laguna run raised.** Neither holds a road
  user who stands for 20 s other than the parked car; the pedestrian cost needs a longer labelled
  window, and is read label-free on the corpus in
  [l3bg0012](../../../data/experiments/l3bg/l3bg0012.md).

## Method

- **Arms.** The shipped defaults (`config/tuning.defaults.json`) and CMR
  ([l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json)),
  each with and without the replay option `dwell_policy` (20 s, 0.5 m/s, 1 m drift, the policy as
  built).
- **Replays.** `lidar-state-estimation-baseline -warmup 6 -sample-points 1024 -campaign-metrics`,
  kirk0 whole and kirk1's 272 s trim with `-duration 172`, as the attribution report's arms ran;
  the harness checks each replay against its repeat, including the policy's absorptions.
- **Scores.** The attribution report's `score_arm.py` unchanged: object-frame recall by class
  against kirk0's sweep ground truth (pack 8e422582), unlabelled clusters, stationary tracks (M3),
  and kirk1's covered road users (any track overlapping 30% of a labelled track's span). The
  reviewed-pack per-frame evaluator and W3 (`lidar-ground-truth-eval perframe` and `retention`)
  against pack ad8b9438 in its version 2 form, split `kirk0-tuning`. Each absorption is matched to
  the ground-truth frame nearest in time and the objects whose footprint, grown 0.5 m, contains it.
- **Reproduction.** The shipped arm reproduces the attribution report's row to the last digit on
  both captures (kirk0 recall 0.5871, 3.43 clusters a frame, 59 tracks; kirk1 38 of 49 covered),
  and CMR its per-frame row (HOTA 0.533, IDF1 0.593). Scoring the blended run's own CMR evidence
  against the version 1 and the version 2 reference gives identical figures.

## Findings

kirk0, against the sweep ground truth and the reviewed pack:

| Measure                              |            Shipped | Shipped + dwell |                CMR |        CMR + dwell |
| ------------------------------------ | -----------------: | --------------: | -----------------: | -----------------: |
| Tracks absorbed                      |                  — |               0 |                  — |                  1 |
| Object-frame recall (sweep GT)       |              0.587 |           0.587 |              0.889 |              0.890 |
| Recall: car / pedestrian / bus       | 0.63 / 0.28 / 0.53 |            same | 0.90 / 0.76 / 0.92 | 0.91 / 0.76 / 0.92 |
| Clusters a frame                     |               3.43 |            3.43 |               4.91 |               4.23 |
| Spurious clusters a frame (sweep GT) |               0.31 |            0.31 |               0.52 |               0.52 |
| Stationary over 20 s / over 60 s     |              0 / 0 |           0 / 0 |              2 / 1 |              1 / 0 |
| Per-frame MOTA (reviewed pack)       |              0.148 |           0.148 |              0.447 |              0.387 |
| Per-frame HOTA                       |              0.376 |           0.376 |              0.533 |              0.494 |
| Per-frame IDF1                       |              0.350 |           0.350 |              0.593 |              0.545 |
| False negatives / false positives    |        2,315 / 722 |            same |          928 / 748 |        1,419 / 719 |
| Identity switches                    |                124 |             124 |                375 |                135 |
| W3 retention                         |              0.494 |           0.494 |              0.869 |              0.865 |
| W3 missed instants                   |              1,595 |           1,595 |                483 |                973 |
| Parked car obj_788622b5 missed       |         550 of 706 |            same |          11 of 773 |         505 of 773 |

The absorption: a track of 49 returns at (−4.4, −9.2) m, absorbed 32.6 s into the capture after
20.1 s stationary. The per-frame evaluator's extra 491 false negatives against 494 newly missed
instants of the parked car say that nothing else moved. Identity switches fall from 375 to 135
as well; the evaluator does not attribute switches to objects, so how many were the car's is not
measured.

CMR's W3 by class and range (the shipped L3's, from the W3 report, in brackets):

| Group      | Retention     | Fragmented |
| ---------- | ------------- | ---------- |
| All        | 0.869 (0.494) | 17% (36%)  |
| Car        | 0.956 (0.404) | 5% (17%)   |
| Truck      | 0.810 (0.560) | 49% (59%)  |
| Pedestrian | 0.829 (0.172) | 0% (0%)    |
| Bus        | 0.837 (0.275) | 4% (37%)   |
| 0–20 m     | 0.870 (0.501) | 11% (30%)  |
| 20–35 m    | 0.913 (0.382) | 35% (54%)  |
| 35–50 m    | 0.725 (0.387) | 28% (46%)  |
| 50 m +     | 0.325 (0.307) | 0% (3%)    |

CMR's evidence holds all 773 frames where the shipped L3's holds 706 (a frame with nothing
clustered writes none), so it scores 3,253 instants against 2,999; 445 instants read a capped
cluster, against 167.

kirk1, label-free counts and covered road users:

| Measure                  | Shipped | Shipped + dwell |  CMR |       CMR + dwell |
| ------------------------ | ------: | --------------: | ---: | ----------------: |
| Tracks absorbed          |       — |        1 (bird) |    — | 2 (bird, dynamic) |
| Covered of 49 road users |      38 |              37 |   49 |                49 |
| Clusters a frame         |    2.33 |            2.25 | 3.09 |              2.88 |
| Tracks                   |      88 |              87 |  101 |               102 |
| Stationary over 20 s     |       1 |               0 |    1 |                 1 |

The covered count's fall from 38 to 37 is the metric, not a lost detection: the reference it
loses is a car labelled for 0.3 s that, in the shipped arm, only a 45 s stationary track overlaps
in time, and that track is the one the policy absorbs.

## Interpretation

On captures this short the policy's effect is one object, and that object is the case it was
built for. CMR recovers kirk0's parked car as foreground, as a narrower window should, and the
policy hands it to the background after 20 s, which is what a tracker should do with a parked
car. The reviewed pack labels the car in every frame, so a per-frame score charges the policy
for it; the sweep's ground truth labels the same returns noise and charges nothing. Which is right
depends on what the score is for: a count of road users moving through the scene wants the parked
car excluded, and the W3 report already recommended reporting parked objects apart.

The cost the Lombard–Laguna run raised, standing pedestrians absorbed and re-tracked, is not
visible here: no labelled pedestrian on either kirk window stands for 20 s.

## What this does not establish

- **The pedestrian cost.** Neither capture holds a person standing for 20 s; a labelled window
  that does (label0001 is the candidate) is needed.
- **Departures.** The parked car does not leave during kirk0, so the hole an absorbed object
  leaves behind is not measured.
- **Longer uptime.** The kirk windows are 83 s and 172 s; the arrival regime the policy answers
  grows over tens of minutes.
- **Held-out performance.** kirk0's pack split is a tuning split.

## Recommendations

- **Report parked and static labelled objects apart** in the per-frame evaluator and W3, or give
  the labels a parked status, before scoring any L3 change that absorbs stationary objects.
  Otherwise the dwell policy reads as a loss on every capture with a parked car.
- **Reconcile kirk0's two references** on obj_788622b5 (car in the reviewed pack) and
  obj_3c67a8a9 (noise in the sweep's ground truth).
- **Keep the policy on in the corpus screen** (l3bg0012), which measures the re-track rate after
  absorption across 23 sites, and in Stage B (l3bg0009); label0001's window should include
  standing pedestrians and a car that parks and leaves. Since measured
  ([the clock](l3-dwell-policy-clock-2026-10.md)): Stage B should run it at 40 s.

## Provenance

- **Code:** `dwell_policy` and the harness at 1f376de58 on `claude/experiment-followup` (the
  overnight branch merged with main), tools built stamped (0.5.1-pre45).
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/followup-20261015/l3bg0011/replay/{a0,a0-dwell,cmr,cmr-dwell}/{kirk0,kirk1}/`;
  scores and the absorption matches kept locally, outside the repository.
- **References:** kirk0 sweep ground truth (pack 8e422582, revision 204) and kirk1 reference
  labels as in the attribution report; reviewed pack `ds_67842cb1b22b70e7` (ad8b9438, version 2),
  split digest `sha256:96f71edbdac4318887a0c187b7e03ba77856964e7e3381d8775c44f039c0064f`.
- **Run:** eight replays of 24 to 50 s each (estimated 2 min) and eight scores of 4 to 44 s.
