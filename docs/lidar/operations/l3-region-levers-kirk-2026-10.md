# The region map's two levers on kirk, October 2026

The [region map](l3-region-tercile-mechanism-2026-10.md) found that the volatile tercile's ×8
window holds back the flood of clusters the overrides-off arm produces, and also takes the road
users that pass in front of variable surfaces, which sit there for those surfaces and for region
merging. It named two levers: apply the overrides by each cell's own variance tercile rather than
its merged region's, and bound the volatile window in metres rather than in proportion to range.
This run builds both as replay options and scores them on the two labelled kirk captures, alone,
together and on top of the overrides-on retune (CMR).

- **Status:** Measured, October 16, 2026 (experiment
  [l3bg0010](../../../data/experiments/l3bg/l3bg0010.md)). Nothing shipped changes
- **Layers:** L3 background, region identification and overrides
- **Related:** [region map](l3-region-tercile-mechanism-2026-10.md),
  [region override attribution](l3-region-override-attribution-kirk-2026-10.md),
  [return fate](w3-return-fate-kirk0-2026-10.md)

## Summary

- **Bounding the volatile window lets the flood back.** A volatile cell's shipped window is 9.8 m
  deep at 20 m; capped at 1 m or 2 m it falls to the global window's depth beyond about 13 m and
  30 m. kirk0's recall rises from 0.587 to 0.924 (pedestrians 0.28 to 0.98), but unlabelled
  clusters rise from 0.31 a frame to 6.1, half the overrides-off flood (12.1). The two caps are
  the same arm in effect. The window's depth is what holds the flood back, so no depth separates
  it from the road users.
- **The cell's own tercile is a smaller, cleaner gain.** It changes the override of 39% of
  kirk0's seen cells, and takes recall to 0.705 (pedestrians 0.60, buses 0.84) and kirk1's
  coverage from 38 to 45 of 49, at 0.84 unlabelled clusters a frame. Alone it is behind CMR on
  every measure (CMR: 0.889 at 0.52).
- **On top of CMR it trades pedestrians for false positives.** CMR with the cell tercile reaches
  0.914 recall and 0.88 for pedestrians (CMR 0.76), but unlabelled clusters triple to 1.55 a
  frame, false positives on the reviewed pack go from 748 to 1,456, buses fall from 0.92 to 0.86,
  and HOTA is level (0.533 to 0.535).
- **Neither lever passes the experiment's criterion**, so neither goes to the corpus screen; CMR
  stays the L3 candidate.

## Method

- **The options.** `region_overrides_cell_tercile`: once regions are identified, each cell takes
  the override parameters identification assigns to its own tercile (the same set a region of that
  tercile copies), and a cell with no tercile runs at the global parameters.
  `volatile_window_cap_1m` and `_2m`: for a cell under a volatile override, the closeness window,
  the locked-baseline window and the neighbour-confirmation window computed from it are each capped
  at the larger of the global parameters' window and 1 m or 2 m; the freeze and stale-model
  thresholds keep the uncapped window. All are default-off replay options that do not move the
  config fingerprint; with none set, kirk0's tracking baselines are byte-identical to the commit
  before them. Combinations that would change nothing (a lever with the overrides off, both caps)
  are refused.
- **Arms.** The shipped defaults (a0); the overrides off (`no_region_overrides` alone, without the
  attribution report's support filter); each lever; the cell tercile with the 1 m cap; and, added
  after those six were scored, CMR ([l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json))
  with and without the cell tercile.
- **Replays and scores.** As l3bg0011: kirk0 whole and kirk1's trim (`-duration 172`), `-warmup 6
-sample-points 1024 -campaign-metrics`; the attribution report's `score_arm.py` against kirk0's
  sweep ground truth and kirk1's reference labels; the reviewed-pack per-frame evaluator against
  a0; W3.

Window depth of a cell at typical spread (each tercile's median on kirk0) under the shipped
defaults, metres:

| Cell                 | 10 m | 20 m | 35 m | 50 m |
| -------------------- | ---: | ---: | ---: | ---: |
| Global, no override  |  0.8 |  1.4 |  2.3 |  3.2 |
| Stable, ×4           |  2.6 |  5.0 |  8.6 | 12.2 |
| Variable, ×3         |  2.0 |  3.8 |  6.5 |  9.2 |
| Volatile, ×8         |  5.0 |  9.8 | 17.0 | 24.2 |
| Volatile, capped 1 m |  1.0 |  1.4 |  2.3 |  3.2 |
| Volatile, capped 2 m |  2.0 |  2.0 |  2.3 |  3.2 |

## Findings

kirk0, against the sweep ground truth (recall, unlabelled clusters) and the reviewed pack
(per-frame, W3):

| Arm                | Recall | Car / ped / bus    | Clusters a frame | Unlabelled a frame | Tracks | HOTA  | IDF1  | FP / FN       | W3 retention, fragmented |
| ------------------ | -----: | ------------------ | ---------------: | -----------------: | -----: | ----- | ----- | ------------- | ------------------------ |
| Shipped            |  0.587 | 0.63 / 0.28 / 0.53 |             3.43 |               0.31 |     59 | 0.376 | 0.350 | 722 / 2,315   | 0.494, 36%               |
| Overrides off      |  0.938 | 0.95 / 0.91 / 0.37 |             16.7 |              12.05 |    224 | 0.309 | 0.317 | 8,609 / 690   | refused                  |
| Cell tercile       |  0.705 | 0.72 / 0.60 / 0.84 |             4.20 |               0.84 |     74 | 0.397 | 0.381 | 1,035 / 1,992 | 0.520, 28%               |
| Cap 1 m            |  0.924 | 0.93 / 0.98 / 0.37 |             9.95 |               6.15 |    157 | 0.418 | 0.417 | 4,367 / 1,256 | 0.813, 12%               |
| Cap 2 m            |  0.925 | 0.93 / 0.98 / 0.37 |             9.93 |               6.12 |    153 | 0.419 | 0.419 | 4,350 / 1,237 | 0.811, 12%               |
| Cell tercile + 1 m |  0.931 | 0.93 / 0.98 / 0.69 |             9.32 |               5.51 |    146 | 0.438 | 0.439 | 3,749 / 1,203 | 0.821, 11%               |
| CMR                |  0.889 | 0.90 / 0.76 / 0.92 |             4.91 |               0.52 |     68 | 0.533 | 0.593 | 748 / 928     | 0.869, 17%               |
| CMR + cell tercile |  0.914 | 0.92 / 0.88 / 0.86 |             5.98 |               1.55 |     85 | 0.535 | 0.585 | 1,456 / 821   | 0.845, 14%               |

W3 refuses the overrides-off arm: only 42% of its retained cluster points are points of the
reviewed pack, below the measure's 50% floor, because most of its clusters are returns no reviewed
object holds.

kirk1, label-free counts and covered road users:

| Arm                | Covered of 49 | Clusters a frame | Tracks | Stationary over 20 s |
| ------------------ | ------------: | ---------------: | -----: | -------------------: |
| Shipped            |            38 |             2.33 |     88 |                    1 |
| Overrides off      |            49 |            12.76 |    227 |                   10 |
| Cell tercile       |            45 |             3.14 |    122 |                    1 |
| Cap 1 m            |            49 |             8.57 |    207 |                    3 |
| Cap 2 m            |            49 |             8.35 |    204 |                    4 |
| Cell tercile + 1 m |            49 |             8.17 |    214 |                    5 |
| CMR                |            49 |             3.09 |    101 |                    1 |
| CMR + cell tercile |            49 |             3.88 |    106 |                    0 |

Under the cell tercile 27,616 of kirk0's 69,950 seen cells change override: among them 3,983
cells stable by their own spread that sat in volatile regions, and 2,191 volatile cells that sat
in stable regions. The region map's "about 5,000 cells" was the net movement into volatile
regions, not the gross.

## Interpretation

The metre cap answers the region map's question about depth directly. At 20 m the volatile window
is about 10 m deep, which looked like the obvious excess; cut to 1 or 2 m, most of what it held
back comes through: half the overrides-off flood on kirk0, about 60% of it on kirk1 by clusters a
frame. The flood is surface returns that vary by metres, not centimetres, so a window shallow
enough to pass a road user standing in front of a hedge passes the hedge too. Depth is not the
lever; something that tells a moving object from a flickering surface is, which is what CMR's
narrower global window, minimum cluster size and road-height floor do between them, and what the
dwell policy and the support filter address from other sides.

The cell tercile does what the region map predicted on its own terms: lane cells that are stable
by their own spread stop inheriting a volatile window from the region they were merged into, and
pedestrians and buses come back. But it also gives volatile cells inside stable regions the
volatile window they had been denied, and on top of CMR that second effect outweighs the first in
false positives. A per-cell override is a real improvement in the map's fidelity, not a free
recall gain.

The criterion as the spec wrote it asked for unlabelled clusters near "the shipped 3.4"; 3.4 is
the shipped grid's clusters a frame, and its unlabelled clusters are 0.31. Read either way, only
the cell tercile alone stays near the shipped figure, and its recall is well short of CMR's at a
higher false-positive count.

## What this does not establish

- **Other cap forms.** A cap relative to the cell's own settled spread, or one by range band, was
  not tried; the result says depth alone does not separate, not that no cap could.
- **The cell tercile with a filter.** CMR plus the cell tercile plus the support filter or a larger
  minimum cluster size might keep the pedestrians without the false positives; not run.
- **Held-out performance.** kirk0's pack split is a tuning split; two short captures.

## Recommendations

- **Do not pursue the metre cap.** It is the overrides switched off at range.
- **Keep the cell tercile as an option, not a candidate**: try it with CMR and the support filter
  if pedestrian recall becomes the bottleneck after Stage B.
- **Leave CMR as the L3 candidate** for Stage B (l3bg0009), with the dwell policy, as planned; the
  corpus screen (l3bg0012) runs without a lever arm.
- **Read region-merging effects gross, not net**, in any later region work: the net 5,000 cells
  hid 27,616 changed overrides.

## Provenance

- **Code:** the three options written by an agent as f38860249 on `agent/l3-levers` (from
  1f376de58) and cherry-picked as b14b10175 on `claude/experiment-followup`; the harness built
  stamped at b14b10175. With no option set, kirk0's tracking baselines and replay summary are
  byte-identical to 1f376de58's (checked independently by the agent and here).
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/followup-20261015/l3bg0010/replay/{a0,off,cell,cap1,cap2,cell-cap1,cmr,cmr-cell}/{kirk0,kirk1}/`;
  scores kept locally.
- **References:** as in l3bg0011: kirk0 sweep ground truth (pack 8e422582, revision 204), kirk1
  reference labels, reviewed pack `ds_67842cb1b22b70e7` split digest
  `sha256:96f71edbdac4318887a0c187b7e03ba77856964e7e3381d8775c44f039c0064f`.
- **Run:** sixteen replays and sixteen scores over about 50 minutes with other lanes running; one
  CMR kirk1 replay ran twice, its first done mark refused while a commit hook had stashed the
  spec's new arms.
