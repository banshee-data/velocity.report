# PCL ports, noise and splits: the November 2026 run

Ten hours of unattended experiments asked whether the PCL-derived candidates from the
[review](pcl-algorithm-review.md) reduce noise tracks, near-static tracks and splits on the refined
defaults of #901. None changes a default.

- **Status:** Measured, November 2, 2026. Replays of recorded captures, scored against kirk0's
  reviewed labels and, elsewhere, label-free counts
- **Specs:** [l3bg0027](../../../data/experiments/l3bg/l3bg0027.md),
  [l3bg0028](../../../data/experiments/l3bg/l3bg0028.md),
  [l4clus0005](../../../data/experiments/l4clus/l4clus0005.md),
  [l5id0006](../../../data/experiments/l5id/l5id0006.md),
  [l6cls0002](../../../data/experiments/l6cls/l6cls0002.md),
  [l6cls0003](../../../data/experiments/l6cls/l6cls0003.md)
- **Layers:** L3 grid, L4 clustering, L5 identity, track-level gating
- **Plan:** [PCL candidates](../../plans/lidar-pcl-algorithm-candidates-plan.md)

## Summary

- **No PCL-derived candidate passed its rule.** Border classes, radius-graph clustering and its
  height-conditioned form all fail.
- **The strongest split result is not PCL.** The existing overlap end with tentative priority
  (`body-older-tentative`) takes kirk0 identity switches from 87 to 5 and fragmentations from 47 to 35. Duplicates fall at all 23 first-segment sites, all 21 readable fourth-segment sites and all
  22 second-segment sites. It passes l5id0006's clause for that arm, held out, and stays off
  because the overlap end waits on label0001.
- **Its cost is brief tracks.** Each duplicate it ends can leave a short track behind, so brief
  tracks rise at all 23 sites, by 61 % at the median. Replay wall time rose about 40 %.
- **A track-level gate is not safe yet.** The gates remove much of the clutter at every site with
  no moved track touched, but on kirk0 they also remove stopped-car-sized tracks. The car-width
  form `g4`, paired with the overlap end, reached 26.5 % less clutter at the median site on unread
  segments against the 40 % its rule required.
- **One site would have misled in both directions.** Radius graph fails on kirk0 but improves
  spurious clusters at 20 of 21 corpus sites. `g4` on the overlap end ranges from −62 % to +46 %
  by site.

## Results against each rule

| Experiment | Arm                     | Verdict | Deciding figures                                                                   |
| ---------- | ----------------------- | ------- | ---------------------------------------------------------------------------------- |
| l3bg0028   | `veil`                  | Fails   | kirk0 retention at 50 m+ −1.3 points (limit 1); spurious clusters −0.8 % (need −5) |
| l3bg0028   | `coast`                 | Not run | Its footprint evidence needs a tracker hook that is not built                      |
| l4clus0005 | `radius-graph`          | Fails   | 20–35 m fragmented 6.5 → 4.5 %, 2.0 points of the 5 required                       |
| l4clus0005 | `conditional`           | Fails   | Fragmented rises to 17.8 %; ID switches 87 → 263                                   |
| l4clus0005 | `radius-graph-nocap`    | Fails   | 2.0 points; IDF1 −0.0268                                                           |
| l4clus0005 | `conditional-nocap`     | Fails   | ID switches 87 → 317; IDF1 −0.1110                                                 |
| l3bg0027   | `held`                  | Fails   | Vehicle tracks 36 → 138; HOTA 0.4293 → 0.2381; corpus not read                     |
| l5id0006   | `body-older-tentative`  | Passes  | Tuning clauses and held-out read below; no default change, by the rule             |
| l5id0006   | `body-older-inherit`    | Not run | `box_inherit` is not built; it concerns the box's axis, not splits                 |
| l6cls0002  | `g1`, `g2`, `g3`        | Fail    | kirk0 false negatives +11 to +13; clauses 1 and 2 pass                             |
| l6cls0002  | `g4`                    | Fails   | kirk0 false negatives +2; 47.5 % at the median site against 50 %                   |
| l6cls0003  | `g4` on the overlap end | Fails   | 26.5 % at the median site (need 40); worst leave-one-out 26.2 % (need 30)          |

## Method

**Host and captures.** hydra, an M1 Mac, with captures read from the USB LiDAR volume.

**kirk0.** `lidar-state-estimation-baseline` over the whole capture, 6 s warm-up, 1,024 sample
points; `lidar-ground-truth-eval retention` and `perframe` on `kirk0-tuning`, ignoring the parked
car `obj_788622b5`. Every replay repeated byte-equal.

**Corpus.** `lidar-box-eval -variants none` with an evidence database, 70 s warm-up and 200 s
scored, on the first segment of the 23 tuning and screen sites, the second capture of 22 sites and
the fourth capture of 22. embarcadero-folsom stayed unread. On the fourth capture
embarcadero-bay is too short, leaving 21 sites. `veil` and `radius-graph` have 21 first-segment
sites: marina-webster-beach and columbus-broadway failed on stale evidence from an interrupted
launch and were not rerun.

**Label-free measures** ([`lidar_noise_screen.py`](../../../scripts/lidar_noise_screen.py)). A
brief track lasts under 1 s. A near-static track has under 2 m net and p95 speed under 0.5 m/s.
Clutter is the two together. Spurious clusters a frame are observations no lasting track took,
counted without cluster identity, so they are an approximation. Gates
([`lidar_track_gate.py`](../../../scripts/lidar_track_gate.py)) remove tracks from a copy of the
evidence, so the real scorer reads them without a replay.

**The classifier is not PCL's.** OQ-02 rejected PCL's extractor on the native grid, so
`l3grid.BorderTraits` is a range-step rule in PCL's vocabulary. The PCL oracle comparisons in
l3bg0028 and l4clus0005, including l4clus0005's frozen-stream one-third gate, were not run. The
radius graph is covered by unit tests only, with no partition check against PCL.

## kirk0

All against `a0`, on the same reviewed window.

| Arm                    |   HOTA |   IDF1 | ID switches | Fragmentations | False negatives | False positives | Fragmented 20–35 m | 35–50 m |  50 m+ | Retention 50 m+ |
| ---------------------- | -----: | -----: | ----------: | -------------: | --------------: | --------------: | -----------------: | ------: | -----: | --------------: |
| `a0`                   | 0.4293 | 0.4767 |          87 |             47 |             502 |            4171 |              6.5 % |  32.2 % |  8.9 % |           0.779 |
| `veil`                 | 0.4299 | 0.4749 |          78 |             48 |             510 |            4077 |              6.7 % |  31.0 % |  4.7 % |           0.766 |
| `radius-graph`         | 0.4337 | 0.4713 |          90 |             30 |               — |            4178 |              4.5 % |  30.6 % | 17.5 % |           0.853 |
| `conditional`          | 0.3952 | 0.3974 |         263 |             45 |               — |            4163 |             17.8 % |  41.6 % | 28.6 % |           0.622 |
| `nocap`                | 0.4216 | 0.4608 |         148 |             47 |               — |            4238 |              6.5 % |  31.0 % |  8.9 % |           0.779 |
| `radius-graph-nocap`   | 0.4216 | 0.4499 |         247 |             30 |               — |            4242 |              4.5 % |  29.9 % | 17.5 % |           0.853 |
| `conditional-nocap`    | 0.3871 | 0.3657 |         317 |             41 |               — |            4197 |             17.8 % |  40.5 % | 28.8 % |           0.620 |
| `held`                 | 0.2381 | 0.1913 |         288 |            106 |               — |            6618 |             16.1 % |  26.7 % | 19.5 % |           0.565 |
| `body-older`           | 0.4371 | 0.4904 |           8 |             39 |               — |            4094 |              6.7 % |  32.4 % | 17.4 % |           0.828 |
| `body-older-tentative` | 0.4367 | 0.4918 |           5 |             35 |             488 |            4140 |              6.7 % |  33.3 % | 17.4 % |           0.828 |

No arm has a reviewed cross-object merge except `held`, with 8. A dash means the figure was not
carried into this table; it decides no verdict.

**Removing the input cap is not free.** `nocap` lifts retention from 0.851 to 0.993 and raises
identity switches from 87 to 148.

**Why `held` collapses.** Held returns quadruple L4's input, from a median of 3,053 points to
13,089, and 62 % of frames then exceed the cap against 8 %. The returns inside a tracked box are
mostly not lower body: this is the box paint l3bg0029 measured for edge follow. The spec's premise
that clients already receive these returns no longer holds, as `edge_follow_cluster` is off by
default.

**Quality gates on kirk0.**

| Gate | Tracks removed | ID switches | False negatives | False positives |   HOTA |   IDF1 |
| ---- | -------------: | ----------: | --------------: | --------------: | -----: | -----: |
| `g1` |             15 |          85 |             513 |            3904 | 0.4387 | 0.4890 |
| `g2` |             18 |          83 |             514 |            3898 | 0.4390 | 0.4894 |
| `g3` |             19 |          81 |             515 |            3892 | 0.4393 | 0.4898 |
| `g4` |             13 |          83 |             504 |            4082 | 0.4321 | 0.4814 |

`g1` removed tracks of 3.8 by 1.7 m for 39 s and 2.9 by 1.5 m. A quality gate must keep those, so
the false-negative clause did its job. `g4` was added after that result and is the only form that
adds no false negative on top of the overlap end (488 either way). `g2` on the overlap end shows
499 against `a0`'s 502, which looks harmless only because the overlap end's own gain hides it.

## Across sites

Sites are the unit. "Worst leave-one-out" recomputes the median leaving out each site in turn.

### First segment, against `a0`

| Measure                 | Arm                    | Better | Worse | Sites |  Median | Worst leave-one-out |
| ----------------------- | ---------------------- | -----: | ----: | ----: | ------: | ------------------: |
| Duplicates per hundred  | `body-older-tentative` |     23 |     0 |    23 |  −16.93 |              −15.05 |
| Brief tracks            | `body-older-tentative` |      0 |    23 |    23 | +61.1 % |             +64.6 % |
| Clutter                 | `body-older-tentative` |      0 |    23 |    23 | +32.1 % |             +33.7 % |
| Observed vehicle frames | `body-older-tentative` |     15 |     8 |    23 |  +0.8 % |              +0.6 % |
| Spurious clusters       | `radius-graph`         |     20 |     1 |    21 |  −5.0 % |              −4.6 % |
| Observed vehicle frames | `radius-graph`         |     19 |     2 |    21 |  +1.0 % |              +0.9 % |
| Duplicates per hundred  | `radius-graph`         |     14 |     4 |    21 |   −0.94 |               −0.63 |
| Spurious clusters       | `veil`                 |     14 |     7 |    21 |  −0.8 % |              −0.7 % |
| Clutter                 | `veil`                 |      7 |    11 |    21 |  +0.7 % |              +1.3 % |

### Gates on `a0`, clutter removed

| Gate | First segment, median | Worst leave-one-out | Least and most | Fourth capture, median |
| ---- | --------------------: | ------------------: | -------------: | ---------------------: |
| `g1` |                53.3 % |              49.0 % |     16 %, 76 % |                 46.6 % |
| `g2` |                73.9 % |              72.7 % |     59 %, 87 % |                 70.7 % |
| `g3` |                76.7 % |              76.5 % |     62 %, 89 % |                 74.3 % |
| `g4` |                47.5 % |              46.5 % |     14 %, 71 % |                 50.0 % |

No moved track was removed at any site, and clutter fell at every site. Reading the fourth capture
for these gates went beyond clause 4, which reads only a candidate that passed, and none did.

### Held-out read for `body-older-tentative` (l5id0006)

| Arm                    | Duplicates per hundred, pooled | Median by site | Observed vehicle frames |
| ---------------------- | -----------------------------: | -------------: | ----------------------: |
| `a0`                   |                          21.94 |          15.79 |                 271,866 |
| `body-older`           |                           2.28 |           1.10 |                 269,679 |
| `body-older-tentative` |                           1.90 |           1.16 |                 271,683 |

On 21 sites of the fourth capture. Duplicates are within 0.5 of `body-older`, and observed vehicle
frames are 1.007 of it, so the clause passes. One site is over 0.5 worse; the rule reads the
figure as a whole. The spec records this capture as read before for the box's labels, not for identity. The rule's two kirk captures were reduced to kirk0.

### `g4` on the overlap end, on the second capture (l6cls0003)

| Clause                     | Needed         | Result  | Verdict |
| -------------------------- | -------------- | ------- | ------- |
| Clutter, median site       | −40 %          | −26.5 % | Fails   |
| Sites where clutter fell   | 16 of 22       | 18      | Passes  |
| Worst leave-one-out        | −30 %          | −26.2 % | Fails   |
| Moved or car-sized removed | 0              | 0       | Passes  |
| Duplicates fall            | 20 of 22       | 22      | Passes  |
| Observed vehicle frames    | ≥ 0.99 of `a0` | 0.9925  | Passes  |

Site by site, clutter change runs from −62 % (bush-powell, laguna-eddy) to +46 %
(broadway-franklin, 13 to 19 tracks). lombard-broderick, embarcadero-bay and lombard-laguna also
rise, by 3 to 7 %.

## Frame cost

Timings were taken beside other replays on a machine also running the desktop, so they are rough.
On kirk0 the frame p99 was 186 ms for `a0`, 129 ms for `veil`, 279 ms for `radius-graph`, 195 ms for
`body-older`, 431 ms for `body-older-tentative` and 344 ms for `held`. Median corpus replay wall
time was 308 s for `a0` against 435 s and 434 s for the overlap end arms, under one load, so
the overlap end's cost is real but not yet measured cleanly.

## What this does not establish

- Physical accuracy. Replays measure what the pipeline drew, not where a vehicle was.
- That any removed or merged track at the 23 unlabelled sites was right. Accuracy rests on kirk0
  alone, with about 20 reviewed objects, skewed by two trucks.
- Live behaviour. Replays use real sensor returns but do not test live timing, dropped packets or
  Pi load.
- Anything outside San Francisco or the 2.3 m mount.
- That no PCL method can help. These are range-step and connectivity rules in PCL's vocabulary, not
  PCL's own code, and the oracle checks did not run.
- Independence of the corpus: its first segments have screened many earlier experiments.

## Recommendations

1. **Make the overlap end the next labelled decision.** Score `body-older-tentative` on label0001.
   It is the largest split gain measured here and repeats at every site read.
2. **Keep the rule's 5-point bar under review for the radius graph.** It was set when `a0` was near
   35 % at 20–35 m; `a0` is now 6.5 %, so the bar is close to unreachable. It stays failed, and
   relaxing it is the owner's decision. Its 50 m+ fragmentation doubled, which needs explaining
   first.
3. **Do not pursue `held_to_l4` or border-class withholding further.** Both are measured dead ends
   on these definitions.
4. **Find out what the overlap end's brief tracks are** before any further gate. They account for
   much of the shortfall in l6cls0003, and a gate tuned again on these segments would be
   overfitting. Any new gate needs unread segments: the third capture is the one left unread here.
5. **Stop reading the fourth capture for gates.** It was spent on this run.
6. **Score sites, not totals**, in future rules, with a leave-one-out line, as l6cls0002 does.

## Provenance

| Item                | Value                                                                                                                                                        |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Base                | main `ef7878250`                                                                                                                                             |
| Tools               | `lidar-box-eval` at `a2a0317d0`; others stamped `wip-*` builds of this branch's commits                                                                      |
| `a0` parameters     | `config/tuning.defaults.json`, SHA-256 `9ad7377fa5d356ba…`                                                                                                   |
| Uncapped parameters | `config/experiments/l4-cap100k.json`, SHA-256 `b787ef318ba88b79…`                                                                                            |
| Replay options      | `border_classes`, `radius_graph_xy`, `radius_graph_xy_overlap`, `held_to_l4`, `overlap_end_1`, `overlap_end_body`, `overlap_end_older`, `tentative_priority` |
| kirk0 pack          | `ad8b9438…-v2`, split `kirk0-reference-tuning-r1`, revision 2232                                                                                             |
| Evidence            | First and fourth captures at `/Volumes/lidar/evidence/pcl-20261102`; kirk0 and second captures in scratch                                                    |
| Scripts             | `~/vr-scratch/pcl-20261102`                                                                                                                                  |

Tools were built with `go build -tags=pcap` into scratch, not through `make`, so their version
stamp reads `dev`; the commit above identifies them.
