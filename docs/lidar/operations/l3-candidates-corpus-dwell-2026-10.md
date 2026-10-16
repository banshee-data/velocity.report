# CMR and the dwell policy on the corpus, both segments, label-free, October 2026

The [region override attribution](l3-region-override-attribution-kirk-2026-10.md) screened the
overrides-on retune (CMR) against the shipped L3 on the first segment of 17 corpus sites: CMR
confirmed 62% more moving tracks and doubled the long stationary ones. The
[dwell policy](l3-dwell-policy-arrival-2026-10.md) was built to clear the stationary ones and was
measured on one 39-minute placement, where [its clock](l3-dwell-policy-clock-2026-10.md) did best
at 40 s. This run screens the shipped L3, CMR and CMR with the policy on the first segment of all
23 tuning and screen sites, and repeats the screen on the second segment of 22 of them, which no
first-segment screen read, adding the policy at 40 s and, at 40 s, with the support filter.

- **Status:** Measured, October 16, 2026 (experiments
  [l3bg0012](../../../data/experiments/l3bg/l3bg0012.md) and
  [l3bg0016](../../../data/experiments/l3bg/l3bg0016.md)). Nothing shipped changes; label-free
- **Layers:** L3 background, L5 tracks
- **Related:** [region override attribution](l3-region-override-attribution-kirk-2026-10.md),
  [dwell policy](l3-dwell-policy-arrival-2026-10.md),
  [the clock](l3-dwell-policy-clock-2026-10.md),
  [labelled cost](l3-dwell-policy-labelled-cost-2026-10.md),
  [levers](l3-region-levers-kirk-2026-10.md), [corpus pose sweep](l3-corpus-pose-sweep-2026-10.md)

## Summary

- **CMR's gain replicates on both segments.** Moving tracks rise from 1,331 to 2,081 over the
  first segments (up at 22 of 23 sites) and from 1,339 to 2,187 over the second (22 of 22), and
  road-surface clusters go to zero at every site. Without the policy CMR also raises stationary
  tracks over 60 s, from 5 to 19 and from 8 to 11.
- **The policy at 20 s clears them and keeps every moving track on five-minute segments.**
  Stationary tracks over 60 s fall to 1 and 0, those over 20 s to 44 and 47 (below the shipped
  grid's 56 and 62), and moving tracks are 2,084 and 2,212 against CMR's 2,081 and 2,187. The
  42 moving tracks it lost on the 39-minute placement do not appear here.
- **Absorbed objects come back more often than chance, at either clock.** At 20 s, 52% and 57%
  of the 154 and 152 absorptions see a new track start within 2 m in the next minute, against 33%
  and 42% at the same points on CMR; pedestrian-classed absorptions 41 and 34. At 40 s on the
  second segment there are four times fewer absorptions (37, ten pedestrian-classed) but the
  re-track rate is 59% against 38%; Lombard–Laguna's 40 s result, no excess, does not hold here.
- **At 40 s four long stationary tracks remain**, against none at 20 s and 11 for CMR alone; with
  the support filter added, one remains, but tracks lasting 2 s fall at 18 of 22 sites.
- **The knocks to the sensor do not move these conclusions.** Excluding the three cases per
  segment scored across one ([pose sweep](l3-corpus-pose-sweep-2026-10.md)) leaves every
  direction above unchanged.

## Method

- **Arms.** The shipped defaults; CMR
  ([l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json)); CMR
  with `dwell_policy` (20 s); on the second segment, CMR with `dwell_policy_t40` and CMR with
  `support_filter` and `dwell_policy_t40`.
- **Cases.** The first five-minute segment of the 23 tuning and screen sites, and the second
  segment of the 22 that have one (ashbury-downey has a single capture; embarcadero-folsom is held
  out), each replayed with `-warmup 70 -duration 200 -sample-points 256`, as the attribution
  report's corpus screen ran.
- **Screen.** Label-free counts with the zero-code-arms report's definitions (`score_free.py`):
  clusters and road-surface clusters a frame, tracks, tracks lasting 2 s, moving tracks (2 s at a
  90th-percentile speed of 2 m/s or more), stationary tracks (over 20 s at under 0.3 m/s). For each
  absorption, whether a track's first estimate lies within 2 m in the next 60 s, and the same at the
  same points and times on CMR without the policy; for the support-filter arm that control is CMR
  without the filter, since CMR with the filter alone was not run.
- **Code.** The first segment and the second segment's first three arms at 1f376de58, the 40 s
  arm at 214323824 and the support-filter arm at a0cd49a33; the code each arm runs is unchanged
  between them.

## Findings

Pooled over the sites, shipped → CMR → CMR with the policy at 20 s:

| Measure                              | First segment, 23 sites | Second segment, 22 sites |
| ------------------------------------ | ----------------------- | ------------------------ |
| Moving tracks                        | 1,331 → 2,081 → 2,084   | 1,339 → 2,187 → 2,212    |
| Tracks lasting 2 s or more           | 3,145 → 4,016 → 4,109   | 3,229 → 4,145 → 4,287    |
| Stationary over 20 s                 | 56 → 89 → 44            | 62 → 62 → 47             |
| Stationary over 60 s                 | 5 → 19 → 1              | 8 → 11 → 0               |
| Clusters a frame, site median        | 4.30 → 5.54 → 5.32      | 4.27 → 5.52 → 5.24       |
| Road-surface clusters a frame        | 0.22 → 0 → 0            | 0.22 → 0 → 0             |
| Sites where CMR's moving tracks rise | 22 of 23                | 22 of 22                 |

The policy's absorptions and what follows them:

| Arm and segment       | Absorbed | Pedestrian-classed | Re-tracked in 60 s | At the same points on CMR |
| --------------------- | -------: | -----------------: | -----------------: | ------------------------: |
| 20 s, first segment   |      154 |                 41 |           80 (52%) |                  51 (33%) |
| 20 s, second segment  |      152 |                 34 |           87 (57%) |                  64 (42%) |
| 40 s, second segment  |       37 |                 10 |           22 (59%) |                  14 (38%) |
| 40 s + support filter |       34 |                  7 |           19 (56%) |                  13 (38%) |

The second segment's dwell arms against CMR (pooled):

| Measure                    |   CMR |  20 s |  40 s | 40 s + support filter |
| -------------------------- | ----: | ----: | ----: | --------------------: |
| Moving tracks              | 2,187 | 2,212 | 2,189 |                 2,171 |
| Tracks lasting 2 s or more | 4,145 | 4,287 | 4,184 |                 3,993 |
| Stationary over 20 s       |    62 |    47 |    69 |                    60 |
| Stationary over 60 s       |    11 |     0 |     4 |                     1 |

Without the cases scored across a knock (first segment: marina-webster-beach, embarcadero-bryant,
van-ness-sacramento; second: haight-divisadero, marina-webster-beach, van-ness-sacramento): moving
tracks 1,077 → 1,740 → 1,743 and 1,957 (CMR) → 1,978 (20 s) → 1,960 (40 s); stationary over 60 s
3 → 14 → 0 and 7 → 0 → 3; re-tracked 54% against 29% (20 s, first) and 67% against 30% (40 s,
second, on 27 absorptions).

Sites that stand out: on the first segment marina-broderick is the one site where CMR confirms
fewer moving tracks than the shipped grid (107 to 80), as in the attribution report, and
lombard-broderick the one where the 20 s policy costs CMR moving tracks (184 to 165).

## Interpretation

CMR's recovery of road users is a corpus-wide effect, not a kirk one: it holds at every site but
one on two separate five-minute windows. Its cost in long stationary tracks is the dwell policy's
job, and on five-minute windows the 20 s clock does it completely without costing moving tracks;
the moving-track loss the 39-minute placement showed is a long-uptime effect that these windows
are too short to reach.

The re-track excess is the policy's real price and it is not specific to the clock: 15 to 21 more
absorbed objects in every hundred than at the same spots anyway are followed by a new track within
a minute, which is what absorbing something that then moves off looks like. At 40 s
the policy absorbs a quarter as much, so the price is paid less often, but each absorption is no
more likely to stick. Which clock is better turns on what the absorbed objects are, which only
labels can say.

## What this does not establish

- **Correctness.** No labels: moving tracks are not recall, and a pedestrian-classed absorption
  may not be a person.
- **Long uptime.** Five-minute windows; the arrival regime and the 20 s clock's moving-track loss
  appear over tens of minutes (the [clock](l3-dwell-policy-clock-2026-10.md) report's placement).
- **The support filter's own effect on the corpus.** CMR with the filter and no policy was not
  run, so its arm's re-track control is CMR without the filter.

## Recommendations

- **Take CMR with the dwell policy to Stage B (l3bg0009) at both clocks**, 20 s and 40 s, and let
  the held-out labels decide between fewer long stationary tracks and fewer absorptions.
- **Measure what the absorbed objects are** on that labelled window: the re-track excess is the
  question the labels must answer.
- **Read the support filter on its labelled kirk result** ([levers](l3-region-levers-kirk-2026-10.md)):
  on the corpus it trims short tracks at most sites and cannot be told apart from noise reduction
  without labels.

## Provenance

- **Code:** harness builds at 1f376de58, 214323824 and a0cd49a33 on `claude/experiment-followup`,
  stamped (0.5.1-pre45).
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/followup-20261015/{l3bg0012,l3bg0016}/replay/<arm>/<site>/`;
  screen JSON and tables kept locally.
- **Captures:** the D2 first-segment corpus and the second-segment corpus the held-out geometry
  replication built, under the LiDAR volume's `pcaps/s2`.
- **Run:** l3bg0012's 69 replays over 3 h 13 min and l3bg0016's 110 over about 4 h, sharing the
  machine and the volume with the other lanes; the 40 s arm's controls were filled after the fact
  where its screen ran before the site's CMR replay existed.
