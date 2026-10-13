# A range-adaptive DBSCAN neighbourhood on kirk0, October 2026

[D-28](../../DECISIONS.md#d-28--research-briefing-decisions-october-2026) took one idea from the
[window research briefing](brief/research-briefing-2025-01-to-2026-10.md) as an experiment arm: a
clustering tolerance that grows with range, from Fontalvo et al. (Sensors 26(13):4068, 2026), to be
scored by W3 with kirk0's two trucks as the merging test. W3 now exists
([report](w3-retention-fragmentation-kirk0-2026-10.md)), and this is the arm.

- **Status:** Measured, October 13, 2026 (experiment
  [l4clus0001](../../../data/experiments/l4clus/l4clus0001.md)). Nothing shipped changes
- **Layers:** L4 clustering
- **Related:** [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md),
  [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md),
  [D-28](../../DECISIONS.md#d-28--research-briefing-decisions-october-2026)

## Summary

- **It barely moves W3.** At 20 to 35 m the share of fragmented object-instants stays at 54%
  (mean clusters 1.75 to 1.74); at 35 to 50 m it goes from 46% to 46% (91 to 93 of about 200
  instants, mean clusters 1.55 to 1.54). Retention rises by under one point in each band, and no
  cluster merges two reviewed objects.
- **The fragmentation at those ranges is not beam spacing.** L4 measures neighbourhoods in the
  horizontal plane, where the gap between adjacent returns at 20 to 50 m is 0.12 to 0.29 m, well
  inside the shipped 0.8 m. The paper's model widens the tolerance for the vertical gap between
  beams, which this DBSCAN never sees, so at these ranges it adds 2 to 12 cm to a neighbourhood
  that was not the constraint.
- **What changed is mostly recovery, not joining.** 21 of 2,999 instants changed cluster count,
  and every one that went from one cluster to two gained a five- or six-return cluster from points
  that were noise before. Beyond 50 m, the two fragmented instants are joined.
- **It costs search time.** In the agent's runs on an idle machine a replay with the option took
  31 s against 23 s without, the neighbour search visiting 25 index cells per point instead of 9.

## Method

- **The option.** `range_adaptive_eps`, a replay option, default off: a point at range r has
  eps(r)² = eps0² + λ·(Δθ·min(r, 200 m))², and two points are neighbours when their horizontal
  distance² is within the nearer point's eps², the paper's rule (Algorithm 1), which keeps the
  relation symmetric. λ = 1.9 (the paper's DBSCAN setting, inside its stable interval 1.7 to 2.4),
  Δθ = 0.337° (the Pandar40P's vertical spacing in its fine band, larger than the 0.2° azimuth step
  at 10 Hz), and the 200 m cap is the sensor's rated range. eps0, the shipped 0.8 m, takes the place
  of the paper's floor term, which was calibrated for a VLP-32C and would leave eps under 0.8 m to
  about 71 m. The resulting eps is 0.816 m at 20 m, 0.849 m at 35 m, 0.897 m at 50 m and 1.81 m at
  the cap. The spatial index keeps 0.8 m cells and widens the search ring by ring; tests check it
  against an exhaustive pair search on 6,000 points.
- **Off is unchanged.** With the option off the shipped arm's W3 tables at this commit are
  identical to geom0003's, measured at the commit before the option existed.
- **Replays.** kirk0, shipped defaults with and without the option, `-warmup 6 -duration 0
-sample-points 1024`, at commit 76b18bfd9, through the experiment queue (units
  `replay/a0/kirk0` and `replay/eps-range/kirk0`).
- **Score.** W3 (`lidar-ground-truth-eval retention`) against pack ad8b9438 version 1, split
  `kirk0-tuning`. kirk1 is not scored: it has no point-level masks, so the spec was narrowed to
  kirk0 before running.

## Findings

| Band or object       | Arm | Instants | Retention |  Main | Missed | Fragmented | Mean clusters | Merged |
| -------------------- | --- | -------: | --------: | ----: | -----: | ---------: | ------------: | -----: |
| 20–35 m              | a0  |      510 |     0.382 | 0.344 |    225 |  154 (54%) |          1.75 |      0 |
|                      | eps |      510 |     0.385 | 0.347 |    224 |  154 (54%) |          1.74 |      0 |
| 35–50 m              | a0  |      414 |     0.387 | 0.305 |    215 |   91 (46%) |          1.55 |      0 |
|                      | eps |      414 |     0.393 | 0.312 |    214 |   93 (46%) |          1.54 |      0 |
| 50 m and beyond      | a0  |      304 |     0.307 | 0.302 |    238 |     2 (3%) |          1.03 |      0 |
|                      | eps |      304 |     0.312 | 0.312 |    237 |     0 (0%) |          1.00 |      0 |
| All                  | a0  |    2,999 |     0.494 | 0.469 |  1,595 |  505 (36%) |          1.45 |      0 |
|                      | eps |    2,999 |     0.494 | 0.469 |  1,592 |  504 (36%) |          1.45 |      0 |
| Truck 1 obj_35c661db | a0  |      493 |     0.534 | 0.501 |    140 |  257 (73%) |          1.97 |      0 |
|                      | eps |      493 |     0.534 | 0.501 |    139 |  260 (73%) |          1.96 |      0 |
| Truck 2 obj_8e829a06 | a0  |      394 |     0.637 | 0.578 |    102 |  129 (44%) |          1.58 |      0 |
|                      | eps |      394 |     0.638 | 0.578 |    102 |  129 (44%) |          1.57 |      0 |

Confirmed tracks went from 48 to 49 and reacquisitions from 322 to 348. One bus instant at 31.7 m
lost its 51-return cluster under the option, probably to the size or aspect filter after the
cluster widened; that is not confirmed.

## Interpretation

The arm answers D-28's question in the negative for this sensor and this L4. Fragmentation at 20
to 50 m on kirk0 is real (half the instants), but its cause is not that neighbouring returns are
further apart than eps: in the horizontal plane they are not. The likelier cause, not measured
here, is returns missing between the fragments (a dropped band, an occluded panel, part of a body
absorbed by the background), which a neighbourhood a few centimetres wider cannot bridge. A
tolerance that did would be one across missing returns, the geometry plan's "no gap tolerance
across a dropped band", or an association that joins fragments, not a range law.

## What this does not establish

- **Other formulations.** The paper's floor term was replaced by the shipped eps; a larger λ or a
  three-dimensional distance would behave differently, and neither was tried.
- **Other captures and sensors.** One capture of 83 s on a tuning split. On a sensor with sparser
  azimuth sampling, or at 20 Hz (Δθ would be 0.4°), the gap the paper models is wider.
- **Cost on the Pi.** The 35% replay slow-down was on the Mac under load; the search is not pruned.

## Recommendations

- **Do not adopt `range_adaptive_eps`.** Keep it as an option for a sensor whose horizontal
  sampling gap approaches eps; record the answer to D-28's question here.
- **Look at the fragments themselves.** The W3 report's truck episodes, with the replay's
  per-return observation log, should show where the gaps are (missing returns or misplaced
  points), which decides between a gap-tolerant clustering and a fragment-joining association.
- **Leave the merging test at kirk0's two trucks**, which are never close enough to merge; a capture
  with vehicles side by side is needed to test merging at all.

## Provenance

- **Code:** `range_adaptive_eps` at 76b18bfd9 on `claude/experiment-overnight` (written by an
  agent as 26cce87a7 and cherry-picked); tools stamped at 76b18bfd9.
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/overnight-20261013/l4clus0001/replay/{a0,eps-range}/kirk0/`;
  the agent's own runs, which match these byte for byte in the W3 report JSON, under
  `…/overnight-20261013/agent-scratch/agent-eps/`; the full W3 JSON kept locally.
- **Run:** two replays of 28 s each and a score of 11 s (estimated 2 min each and 10 min).
