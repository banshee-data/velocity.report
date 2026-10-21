# CMR's minimum cluster size against the mid-range fragments, on kirk0

<!-- ignore-style-length -->

The Stage B fate report found that under CMR the reviewed returns in the gaps between a
vehicle's fragments at 20 to 35 m are mostly DBSCAN noise, and the return-fate report had named
CMR's minimum cluster size of 8 as where those fragments break. This report replays kirk0 with
the minimum at 8 (`cmr`) and at the default 5 (`cr`), the two parameter sets differing in that key
alone, and applies the rule written before the replays. It answers
[l4clus0003](../../../data/experiments/l4clus/l4clus0003.md).

- **Status:** Rule failed, 2026-10-21; the minimum of 8 stays in the Stage B arms, and l3bg0009
  gains no `cr` arm
- **Layers:** L4 (DBSCAN's minimum cluster size), scored against kirk0's reviewed returns and
  objects
- **Related:** [Stage B return fate](w3-return-fate-stage-b-kirk0-2026-10.md),
  [W3 return fate](w3-return-fate-kirk0-2026-10.md),
  [DBSCAN input cap](l4-dbscan-input-cap-2026-10.md),
  [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md)
- **Parameter sets:** [l3-retune-cm2-floor](../../../config/experiments/l3-retune-cm2-floor.json)
  (`cr`, minimum 5) and [l3-retune-cm2-mp8-floor](../../../config/experiments/l3-retune-cm2-mp8-floor.json)
  (`cmr`, minimum 8)

## Answer

**The default minimum does not join the mid-range fragments, and it costs what the rule
forbids.** With the minimum at 5, fragmented instants at 20 to 35 m are 36 % against `cmr`'s 35 %
(186 against 171 instants), where the rule needed five points fewer. Unlabelled clusters a frame
are 1.43 against 0.52, 2.75 times `cmr`'s against the 1.5 allowed, and per-frame HOTA is 0.515
against 0.533, 0.019 lower against the 0.01 allowed. Every clause fails, so by the rule the minimum
of 8 stays in the Stage B arms and l3bg0009 gains no `cr` arm.

The smaller minimum finds more of what is there and splits it as often. Object-frame recall rises
from 0.889 to 0.942 and the reviewed instants that reach no cluster fall from 483 to 315, but the
returns those extra clusters hold arrive as more pieces, not as joined vehicles: fragmented
instants rise everywhere (all 474 to 584, trucks 390 to 463), tracks from 68 to 97, identity
switches from 375 to 397 and track fragmentations from 59 to 98. The 20 to 35 m gaps that turn into
DBSCAN noise under CMR do not become one cluster at a smaller minimum; they become small clusters
of their own.

## Question

The spec's rule, written 2026-10-18 before the replays: the minimum of 8 stays in the Stage B arms
unless, on the kirk captures, fragmented instants at 20 to 35 m fall by five points or more under
`cr`, with the unlabelled clusters a frame no more than 1.5 × `cmr`'s and HOTA no lower than
`cmr`'s less 0.01. If it passes, l3bg0009 gains `cr` as an arm; it changes no default itself.

**Amended before the replays (2026-10-21).** kirk0 only: kirk1 has no reviewed pack or split, so
neither measure the rule reads can be scored on it; the thresholds are unchanged. Fragmented
instants at 20 to 35 m are the W3 retention measure's fragmented share in that band, five points
being percentage points; unlabelled clusters a frame are `score_arm.py`'s spurious clusters a frame
against kirk0's sweep ground truth; HOTA is the per-frame evaluator's, `cr` against `cmr`.

## Method

Each arm replayed the whole of kirk0 with `lidar-state-estimation-baseline` at `95961cfd0` (main
`06094759d` plus the amended spec), 6 s warm-up and 1,024 sample points, as the earlier kirk lanes
did. Each evidence database was scored with `lidar-ground-truth-eval retention` (W3, `kirk0-tuning`,
range bands 20, 35, 50 m), `score_arm.py` (object-frame recall and spurious clusters against the
sweep ground truth), and `lidar-ground-truth-eval perframe` with `cmr` as arm A and `cr` as arm B.

## Findings

### The rule

| Clause                         | `cr` (minimum 5) | `cmr` (minimum 8) | Needed                    | Result |
| ------------------------------ | ---------------: | ----------------: | ------------------------- | ------ |
| Fragmented instants at 20–35 m |       36 % (186) |        35 % (171) | `cr` five points lower    | fail   |
| Unlabelled clusters a frame    |             1.43 |              0.52 | at most 0.78 (1.5 × 0.52) | fail   |
| Per-frame HOTA                 |            0.515 |             0.533 | at least 0.523            | fail   |

### Beside it

| Measure                                              |                   `cr` |                 `cmr` |
| ---------------------------------------------------- | ---------------------: | --------------------: |
| Object-frame recall (sweep ground truth)             |                  0.942 |                 0.889 |
| Clusters a frame                                     |                   6.22 |                  4.91 |
| Tracks                                               |                     97 |                    68 |
| W3 retention (all / cars / trucks)                   |  0.871 / 0.957 / 0.812 | 0.869 / 0.956 / 0.810 |
| Missed instants (all)                                |                    315 |                   483 |
| Fragmented instants (all / cars / trucks)            | 584 (20 %) / 112 / 463 | 474 (17 %) / 82 / 390 |
| Fragmented instants by range: 0–20 / 20–35 / 35–50 m |        222 / 186 / 155 |       199 / 171 / 104 |
| Per-frame DetA / AssA                                |          0.443 / 0.659 |         0.485 / 0.645 |
| IDF1                                                 |                  0.568 |                 0.593 |
| MOTA                                                 |                  0.295 |                 0.447 |
| Identity switches / track fragmentations             |               397 / 98 |              375 / 59 |

`cmr`'s per-frame HOTA (0.533) and W3 figures match l4clus0002's CMR arm exactly, a check that the
replay and scoring reproduce. Retention, the share of reviewed returns that reach any cluster,
barely moves (0.869 to 0.871): the minimum decides whether a small group of returns counts as a
cluster, and most reviewed returns are in groups far larger than either minimum.

## What it does not establish

- **kirk1, or anything held out.** One capture's tuning split, read many times before.
- **The corpus's spurious-cluster cost**, which the L3 candidates report measured label-free; the
  1.43 here is against kirk0's sweep ground truth, where unlabelled includes parked and static
  objects nobody masked.
- **Any other L4 rule for the mid-range gaps.** It rules out the minimum cluster size as the lever;
  it says nothing about DBSCAN's radius, a
  [range-adaptive radius](l4-range-adaptive-eps-2026-10.md), or merging fragments after clustering.

## Provenance

| Item        | Value                                                                                                                                                                  |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | harness and evaluator at `95961cfd0`, built with the Makefile's LDFLAGS; the spec's amendment committed at `95961cfd0` before any replay                               |
| Tuning      | `l3-retune-cm2-floor.json` (sha256 `a36badfc…`, params `sha256:d05cd6b6…`); `l3-retune-cm2-mp8-floor.json` (`6bc94773…`, params `sha256:713926e5…`)                    |
| Capture     | kirk0 (`internal/lidar/perf/pcap/kirk0.pcapng`), whole, 6 s warm-up                                                                                                    |
| References  | reviewed pack `ds_67842cb1b22b70e7`, split `kirk0-tuning` of `kirk0-reference-tuning-r1.json`; sweep ground truth and reference tracks of the blended run (`dd98c68e`) |
| Raw outputs | evidence databases and scores under the run's scratch directory (`exp-20261020/kirk/l4clus0003`, `results/l4clus0003`); not in Git                                     |

## Recommendations

1. **Keep the minimum of 8 in Stage B.** Lowering it buys object-frame recall at a cost the rule was
   written to refuse, and does not touch the fragments it was meant to.
2. **Look at joining fragments, not at admitting smaller ones.** At 20 to 35 m the returns between
   fragments do not form a cluster of their own at either minimum; a rule that links fragments with
   a shared motion (the merge guard's spans, or one track per vehicle on the body) is where the W3
   fix goes next.
