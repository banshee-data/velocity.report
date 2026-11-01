# The mean of a cluster's returns as its position, November 2026

The [foreground refinement report](l3-foreground-refinement-2026-10.md#what-remains) left medoid
jitter mostly as composition: a medoid is one return, so it steps between returns as a vehicle's
visible surface changes. This run asks whether the mean of the cluster's returns holds identity as
well and moves less.

- **Status:** Measured, November 1, 2026 (experiment
  [l4clus0006](../../../data/experiments/l4clus/l4clus0006.md)). The target passes; one non-inferiority
  clause fails by 1.6 points, so the rule's pass is not met. Nothing moves in the defaults, and the
  arm stays its own parameter set,
  [foreground-refine-mean.json](../../../config/experiments/foreground-refine-mean.json)
- **Layers:** L4 cluster position, L5 measurement
- **Related:** [D2 medoid against OBB centre](d2-medoid-obb-current-solver-2026-10.md)

## Summary

- **On four held-out captures, against foreground refinement alone:** position jitter −36 %, p90
  −18 %, filtered track position jitter −28 %, speed jitter −33 %, all four captures lower.
- **The cost is small:** moving-track time −3 %, stationary tracks +7 %, median life −4 %. Broken links
  rise 11.6 % as a mean of capture ratios, over the rule's 10 %. In counts that is 67 to 69, and the
  mean is carried by kirk1 going from 10 to 16.
- **Identity improves on kirk0's reviewed objects:** ID switches 117 to 87, identity recall 0.697 to
  0.780, false negatives 536 to 502.
- **It works without refinement too.** On the defaults the mean alone takes kirk0's ID switches from
  440 to 174 and HOTA from 0.454 to 0.494, at no measured cost in false positives or false negatives,
  and development-set position jitter down 24 %. The medoid's stated reason, that the mean of a
  non-convex cluster can fall off the cloud, did not show up as a cost on any capture here.

## Method

The option is the optional block `l4.dbscan_xy_v1.cluster_position`, `centre` `medoid` or `mean`,
absent from the defaults. Z stays the medoid's. The mean is already computed to find the medoid, so
it adds no work.

Offline on three development captures, the same tracks were measured with each candidate centre
computed beside the medoid, without reaching the tracker:

| Capture        | Medoid | OBB centre | Coordinate median | Trimmed mean |
| -------------- | -----: | ---------: | ----------------: | -----------: |
| lombard-laguna |  0.259 |      0.258 |             0.196 |        0.182 |
| howard-6th     |  0.186 |      0.185 |             0.171 |        0.140 |
| 3rd-folsom     |  0.179 |      0.165 |             0.124 |        0.103 |

Through the tracker on the seven development captures, as ratios to the medoid, the trimmed mean and
plain mean are level on jitter (0.62, 0.63) and p90 (0.84, 0.84), and on kirk0 the mean is ahead on ID
switches (95, 87) and identity recall (0.772, 0.780). The plain mean needs no sort, so it is the arm.

Measures are those of [l3bg0029](l3-foreground-refinement-2026-10.md#measuring-without-labels), read on
the position each arm reports, so a lower figure means the tracker is given a steadier number. Track
state jitter is the same statistic on the filter's output, and speed jitter the median frame-to-frame
change in filtered speed.

## Results

The rule was committed before the held-out read: the first 200 s of pierce-haight,
columbus-north-point and hyde-ofarrell, and kirk1's first 178 s, the first 30 s not scored. None was
read for l3bg0029. Refine → refine with the mean:

| Capture              | Jitter m      | Jitter p90 m  | State jitter m | Speed jitter m/s | Moving track s | Stationary | Broken links |
| -------------------- | ------------- | ------------- | -------------- | ---------------- | -------------- | ---------- | ------------ |
| pierce-haight        | 0.160 → 0.118 | 0.305 → 0.290 | 0.083 → 0.066  | 0.284 → 0.177    | 550 → 567      | 28 → 27    | 12 → 13      |
| columbus-north-point | 0.153 → 0.110 | 0.333 → 0.302 | 0.075 → 0.057  | 0.233 → 0.168    | 1484 → 1381    | 57 → 62    | 24 → 21      |
| hyde-ofarrell        | 0.164 → 0.098 | 0.322 → 0.209 | 0.083 → 0.048  | 0.244 → 0.142    | 1794 → 1732    | 48 → 49    | 21 → 19      |
| kirk1                | 0.167 → 0.087 | 0.358 → 0.282 | 0.071 → 0.054  | 0.297 → 0.228    | 1036 → 992     | 20 → 24    | 10 → 16      |

| Clause                       | Bound         | Result              | Verdict  |
| ---------------------------- | ------------- | ------------------- | -------- |
| Median position jitter       | −20 %         | −36 %, four of four | Pass     |
| p90 position jitter          | no higher     | −18 %               | Pass     |
| Moving-track seconds         | at most −5 %  | −3 %                | Pass     |
| Stationary tracks            | at most +10 % | +7 %                | Pass     |
| Broken links                 | at most +10 % | +11.6 %             | **Fail** |
| Track state and speed jitter | no higher     | −28 %, −33 %        | Pass     |
| kirk0 ID switches            | no higher     | 117 to 87           | Pass     |
| kirk0 identity recall        | no lower      | 0.697 to 0.780      | Pass     |
| kirk0 false negatives        | at most +5 %  | 536 to 502          | Pass     |

Against the defaults on the development set, refinement and the mean together bring position jitter
from 0.264 to 0.137 m, p90 from 0.543 to 0.340 m, filtered position jitter from 0.166 to 0.079 m and
speed jitter from 0.551 to 0.252 m/s, as means over the seven captures.

## What this does not establish

- Jitter is a statistic of the reported position, not an error against a surveyed vehicle: a steadier
  position could be steadily wrong. The mean of a vehicle's visible returns is biased towards the
  sensor-facing side, as the medoid is. Physical accuracy needs the physical references.
- MOTP on kirk0 improves (1.159 to 1.094 m), which is one capture.
- Non-convex clusters were not studied one by one. The evidence is that no aggregate suffered.

## Recommendations

1. **Decide the broken-links clause.** Read by the rule the arm does not pass, and the rule stands. In
   counts the change is two links in 67, so a re-read with more capture would settle it cheaply.
2. **Try the mean on the defaults under its own rule.** It is a one-key change with the largest gain
   per line in this work, and it does not depend on refinement.

## Provenance

- **Code:** branch `claude/vrlog-foreground-extraction-f1a9a3`, commit `ff06c1a05`.
- **Parameter sets:** [foreground-refine.json](../../../config/experiments/foreground-refine.json),
  [foreground-refine-mean.json](../../../config/experiments/foreground-refine-mean.json).
- **Reference:** as [l3bg0029](l3-foreground-refinement-2026-10.md#provenance).
- **Raw output:** `~/vr-scratch/fg-20261031/`, untracked.
