# The DBSCAN input cap under CMR, on kirk, October 2026

The [return fate](w3-return-fate-kirk0-2026-10.md) measure found that once the overrides-on retune
(CMR) keeps a near truck's returns, the largest remaining loss is L4's input cap: above 8,000
foreground points a frame DBSCAN clusters a uniform random subsample, which dropped 17% of the
trucks' reviewed returns and half of the returns in the gaps between CMR's fragments inside 20 m.
This run raises the cap under CMR to see whether those fragments join, and what it costs.

- **Status:** Measured, October 16, 2026 (experiment
  [l4clus0002](../../../data/experiments/l4clus/l4clus0002.md)). Nothing shipped changes
- **Layers:** L4 input cap, DBSCAN
- **Related:** [return fate](w3-return-fate-kirk0-2026-10.md),
  [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md),
  [region override attribution](l3-region-override-attribution-kirk-2026-10.md)

## Summary

- **Raising the cap recovers the returns and nothing else.** With the cap at 16,000 W3 retention
  under CMR rises from 0.869 to 0.972 and the trucks' from 0.810 to 0.979, but fragmented
  instants stay at 17% (trucks 49%, inside 20 m 11%), recall moves from 0.8885 to 0.8901 and HOTA
  from 0.533 to 0.534. Kirk1 never reaches the cap, and nothing there changes.
- **The cap thinned the trucks; it did not split them.** With every return in, the gaps inside 20
  m halve (15,586 returns to 7,927) and what is left is 93% L3 background: the 1.8% of reviewed
  returns CMR still absorbs sit between the fragments of a near truck. Truck 2's gaps are DBSCAN
  noise (48%) and the shape filters (22%), unchanged.
- **It costs the frame-time tail.** On kirk0 the 99th-percentile frame time goes from 48 ms to 129
  ms at 16,000 and 153 ms with no effective cap, the median unchanged at about 6 ms; on the Mac
  with other replays running, so read as direction, not size. That tail is what the cap exists
  to bound, and the Pi's budget is 100 ms a frame.
- **16,000 is effectively no cap on kirk0.** It leaves 0.3% of reviewed returns to the cap; the
  100,000 arm leaves none.

## Method

- **Arms.** CMR ([l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json))
  with `foreground_max_input_points` 8,000 (as shipped), 16,000
  ([cap16k](../../../config/experiments/l3-retune-cm2-mp8-floor-cap16k.json)) and 100,000
  ([cap100k](../../../config/experiments/l3-retune-cm2-mp8-floor-cap100k.json)), each CMR's file
  with that one key changed. The loader refuses a cap below 1, so 100,000, above any kirk frame's
  foreground, stands in for none.
- **Replays and scores.** As l3bg0011: `lidar-state-estimation-baseline -warmup 6 -sample-points
1024 -campaign-metrics`, kirk0 whole and kirk1's trim with `-duration 172`; `score_arm.py`, the
  reviewed-pack per-frame evaluator against the CMR arm, and W3. Frame times are the harness's
  `-campaign-metrics` percentiles.
- **Fate.** kirk0 through `velocity lidar pcap-replay --observations` and `lidar-ground-truth-eval
fate`, as geom0004 ran.

## Findings

kirk0:

| Measure                           |    CMR, 8,000 |   CMR, 16,000 |  CMR, 100,000 |
| --------------------------------- | ------------: | ------------: | ------------: |
| Object-frame recall (sweep GT)    |        0.8885 |        0.8901 |        0.8901 |
| Spurious clusters a frame         |          0.52 |          0.53 |          0.53 |
| Clusters a frame                  |          4.91 |          4.94 |          4.94 |
| Tracks                            |            68 |            69 |            69 |
| Per-frame HOTA / IDF1             | 0.533 / 0.593 | 0.534 / 0.594 | 0.535 / 0.594 |
| False negatives / false positives |     928 / 748 |     913 / 739 |     913 / 739 |
| Identity switches                 |           375 |           382 |           382 |
| W3 retention (all / trucks)       | 0.869 / 0.810 | 0.972 / 0.979 | 0.975 / 0.984 |
| W3 fragmented (all / trucks)      |     17% / 49% |     17% / 49% |     17% / 49% |
| Reviewed returns lost to the cap  |         0.106 |         0.003 |         0.000 |
| Frame time p50 / p95 / p99 (ms)   |   6 / 39 / 48 |  6 / 41 / 129 |  7 / 52 / 153 |

kirk1: clusters a frame 3.09, 101 tracks and 49 of 49 covered in all three arms; p99 frame time
18, 24 and 17 ms.

The gaps between fragments (the returns of a fragmented instant in none of its clusters), CMR at
8,000 against 16,000:

| Group   | Gap returns    | L3 background | Input cap | DBSCAN noise  | Height band   | Shape filters |
| ------- | -------------- | ------------- | --------- | ------------- | ------------- | ------------- |
| 0–20 m  | 15,586 → 7,927 | 0.474 → 0.934 | 0.492 → — | 0.013 → 0.025 | 0.000 → 0.000 | 0.021 → 0.040 |
| 20–35 m | 1,358 → 1,358  | 0.127 → 0.127 | — → —     | 0.719 → 0.719 | 0.022 → 0.022 | 0.133 → 0.133 |
| 35–50 m | 1,537 → 1,302  | 0.088 → 0.107 | 0.141 → — | 0.333 → 0.376 | 0.438 → 0.517 | 0.000 → 0.000 |
| Truck 1 | 14,197 → 7,187 | 0.405 → 0.801 | 0.492 → — | 0.053 → 0.102 | 0.049 → 0.096 | 0.000 → 0.001 |
| Truck 2 | 1,723 → 1,703  | 0.285 → 0.290 | 0.014 → — | 0.478 → 0.484 | 0.000 → 0.000 | 0.223 → 0.225 |

Truck 1 is fragmented in 239 of its 542 instants in every arm.

## Interpretation

The cap's subsample is uniform, so a near truck that pushes a frame over 8,000 loses about a
fifth of its returns evenly. That is what the return fate counted, and raising the cap gives them
back. But the fragments do not join: a uniform thinning of a dense cloud evidently did not open
gaps wider than DBSCAN's 0.8 m neighbourhood, and the fragments were already separate, by returns
L3 calls background even under CMR. The fragmentation inside 20 m is therefore an L3 question, the same
residual absorption the dwell and region work is about, and the 20 to 35 m fragmentation is
DBSCAN's minimum cluster size and the shape filters, which the cap never touched.

So the cap is the right trade as shipped for detection and tracking: what it removes costs
nothing measurable in recall or identity on kirk0, and lifting it costs the frame-time tail. What
it does cost is the share of an object's returns that reaches its cluster, which matters to the
geometry work, whose boxes are fitted to cluster points.

## What this does not establish

- **The Pi's frame times.** The Mac's tails were measured with three other replay lanes running;
  only perf0001 on the target says whether 129 ms at the 99th percentile is real there, or worse.
- **Geometry.** Whether the recovered returns improve box extents or the containment measures is
  not scored here.
- **Other cap shapes.** A cap that keeps the nearest returns, or subsamples by region instead of
  uniformly, was not tried.
- **Other captures.** kirk0 is the one capture in which a frame passes the cap; kirk1 never does.

## Recommendations

- **Keep the cap at 8,000** for detection and tracking.
- **Score the geometry candidate with the cap lifted** if box extents on near trucks matter: the
  return fate shows the cap takes a fifth of a near truck's points.
- **Look at L3 for the near fragments under CMR**, not L4: 93% of the returns between them are
  ones it absorbs. A fate run of the dwell and lever arms on kirk0 would say whether either
  touches them.
- **Time any cap change on the Pi** (perf0001) before adopting it.

## Provenance

- **Code:** the harness at b14b10175 and `velocity` and the fate tool at ae7c391c0 on
  `claude/experiment-followup`, built stamped (0.5.1-pre45); the L3 and L4 code is the same in
  both.
- **Parameter sets:** `l3-retune-cm2-mp8-floor-cap16k.json` and `-cap100k.json`, committed with
  this report.
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/followup-20261015/l4clus0002/{replay,record}/`;
  scores and fate output kept locally, outside the repository.
- **Run:** six replays 11 min 8 s, six scores 12 min 11 s, three recordings 2 min 42 s and three
  fate scores 9 min 35 s, with other lanes sharing the machine and the USB volume.
