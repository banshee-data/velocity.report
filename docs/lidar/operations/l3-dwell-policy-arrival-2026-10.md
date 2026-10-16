# A dwell policy, and the arrival regime over a 39-minute capture, October 2026

Every L3 retune that recovers road users also leaves more objects foreground for longer: on the
corpus the overrides-on retune (CMR) took tracks stationary for over 20 s from 56 to 89 (the
[attribution report](l3-region-override-attribution-kirk-2026-10.md)). The
[beam-wise review](../../../data/maths/proposals/20261011-beamwise-background-subtraction-review.md)
§5.1 predicts why, and that it gets worse with uptime: whether an arriving object stays
foreground depends on how old the background is when it arrives. This run replays the whole
Lombard–Laguna placement, 39 minutes in eight segments, under the shipped L3, CMR, and CMR with a
new dwell policy that hands an object stationary for 20 s to the background.

- **Status:** Measured, October 13, 2026 (experiment
  [l3bg0008](../../../data/experiments/l3bg/l3bg0008.md)). Nothing shipped changes; label-free
- **Layers:** L3 background, L5 tracks, the pipeline between them
- **Related:** [region override attribution](l3-region-override-attribution-kirk-2026-10.md),
  [region map](l3-region-tercile-mechanism-2026-10.md),
  [scene-change recovery proposal](../../../data/experiments/try/lombard-laguna-scene-change-recovery.md)

## Summary

- **The dwell policy removes the long stationary tracks and keeps the recovered road users.**
  Under CMR with the policy no track stays stationary for more than 60 s (CMR 14, shipped 10), the
  longest stationary track lasts 48 s (CMR 654 s), and stationary track-seconds fall from 3,818 to
  625, a quarter of the shipped L3's 2,500. Moving tracks fall only from 1,045 to 1,003, against the
  shipped 486.
- **The arrival regime grows with uptime under CMR, not under the shipped L3.** Clusters per frame
  under CMR rise from about 4.7 in the first 15 minutes to 6.7 at 25 to 30 minutes; the shipped L3
  stays near 4. The policy's absorptions follow it: 26 in the first 25 minutes, 60 in the last 15.
- **It absorbs people too.** Of 86 absorbed tracks, 16 were classed pedestrian: people standing
  for more than 20 s. One spot was absorbed six times, so an absorption does not always stick.
- **The last five minutes are the scene change.** All three arms jump to 8.6 to 13.8 clusters per
  frame after the event the scene-change proposal documents (45 tracks in 0.2 s at 33:20.6); that
  is a separate problem, and the policy cuts CMR's last-bin count from 13.8 to 8.6 without solving it.
  Since measured ([pose step](l3-scene-change-pose-step-2026-10.md)): the event is a knock to the
  sensor at 32:50 of this replay's uptime (34:00.1 into the placement; 33:20.6 was the web export's
  own offset), a second pose change follows at 34:51, and from 37:35 the sensor is being picked up,
  so the last bin is not one event's tail.

## Method

- **The policy.** `dwell_policy`, a replay option, default off. L5 keeps a stationary clock per
  track, counting observed frames while its speed stays at or under 0.5 m/s (the threshold L5
  already uses) and it stays within 1 m of where the run began; coasted frames neither advance nor
  reset it. When a confirmed track observed this frame has been stationary for 20 s (the review's
  M3 threshold), the pipeline asks L3 to absorb the frame's foreground returns inside the track's
  box (grown 0.2 m): each cell's background range is set to the returns' mean, its spread kept or
  widened, its freeze lifted and a locked baseline moved to the object. The track is then ended
  with a new expiry reason, `absorbed`, and never re-enters association. Off, kirk0's replay output
  is byte-identical to the commit before it; the harness checks the policy's report is identical
  on its repeat replay.
- **Arms.** a0, the shipped defaults; CMR,
  [l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json);
  CMR + dwell, the same file with `dwell_policy`. Each over the whole placement
  (`-warmup 70 -duration 0`), 32 retained points per observation so the evidence fits the internal
  disk; the screen reads counts and tracks, not shapes.
- **Screen.** Label-free, with the zero-code-arms report's definitions: a track's lifetime is its
  first to last estimate; stationary is a lifetime over 20 s with p90 speed under 0.3 m/s; moving
  is a lifetime of at least 2 s with p90 speed of at least 2 m/s. Uptime is counted from the first
  scored frame, about 70 s into the placement, in five-minute bins.

## Findings

The whole placement:

| Measure                                     |             a0 |            CMR |       CMR + dwell |
| ------------------------------------------- | -------------: | -------------: | ----------------: |
| Moving tracks                               |            486 |          1,045 |             1,003 |
| Confirmed tracks                            |          2,719 |          3,608 |             3,720 |
| Stationary over 20 s                        |             43 |             44 |                24 |
| Stationary over 60 s                        |             10 |             14 |                 0 |
| Stationary over 300 s                       |              0 |              2 |                 0 |
| Stationary lifetime, median / p90 / max (s) | 34 / 111 / 293 | 39 / 252 / 654 |      22 / 43 / 48 |
| Stationary track-seconds                    |          2,500 |          3,818 |               625 |
| Reacquisitions                              |         11,541 |         13,948 |            13,211 |
| Observed track-instants                     |         94,047 |        147,614 |           121,128 |
| Tracks absorbed                             |              — |              — | 86 (16,029 cells) |

By uptime (clusters per frame; stationary tracks alive at the bin's end; the policy's absorptions):

| Uptime (min) | a0 clusters | CMR clusters | CMR + dwell clusters | a0 alive | CMR alive | Dwell alive | Absorbed |
| -----------: | ----------: | -----------: | -------------------: | -------: | --------: | ----------: | -------: |
|          0–5 |        4.21 |         5.14 |                 5.18 |        1 |         0 |           0 |        4 |
|         5–10 |        4.06 |         4.71 |                 4.31 |        0 |         0 |           0 |        6 |
|        10–15 |        3.72 |         4.29 |                 4.09 |        1 |         0 |           0 |        4 |
|        15–20 |        2.90 |         4.57 |                 4.15 |        0 |         1 |           0 |        4 |
|        20–25 |        3.60 |         6.33 |                 4.99 |        2 |         1 |           1 |        8 |
|        25–30 |        3.98 |         6.70 |                 7.20 |        1 |         2 |           0 |       26 |
|        30–35 |        4.72 |         7.54 |                 6.36 |        4 |         8 |           0 |       16 |
|        35–40 |        8.75 |        13.77 |                 8.62 |        0 |         0 |           0 |       18 |

The 30–35 minute bin holds the scene change at 33:20.6 placement time, and the last bin is its
tail. (Since measured: the knock is at 32:50 of uptime and a second pose change at 34:51, both in
the 30–35 bin; the last bin includes about 77 s of the sensor being carried, from 37:35, past the
site's indexed end. See the [pose step](l3-scene-change-pose-step-2026-10.md).)

The absorbed tracks sat at a median 20 m from the sensor (10th to 90th percentile 5 to 49 m); they
were classed dynamic (48), pedestrian (16), bird (12) and car (10), with a median of 23 returns and
one of 5,209. Six absorptions fell within 2 m of one spot (−6, −18).

## Interpretation

The shipped L3 holds its foreground count roughly level for half an hour; CMR does not. CMR's
narrower window (`closeness_multiplier` 2.0 against 3) keeps more returns foreground, and the
count grows with uptime, as the review's §5.1 predicts for objects that arrive after the
background has aged. The dwell policy is the counterpart the attribution report said any narrowed
L3 needs: it lets an arrived object become background on a clock of its own, rather than the
background's. On this capture it does that without giving back the road users CMR recovers.

Its costs are visible in the counts. A pedestrian standing for 20 s is absorbed and re-tracked as
a new object when they move, and an object absorbed in one frame can return when its departure
leaves a ghost or its cells' new background does not hold, as the six absorptions at one spot
suggest. Observed track-instants fall by 18%, from 147,614 to 121,128.

## What this does not establish

- **Correctness.** Lombard–Laguna has no labels; moving tracks are not recall, and the screen
  counts what the tracker produced, not what was on the street.
- **The departures.** An absorbed object that leaves makes a hole in the background; the agent
  that built the policy measured a 56-frame recovery for a flanked car on a synthetic ring, and this
  run did not measure departures at all.
- **Other values of T.** 20 s is fixed; a pedestrian-aware T, or a longer one, was not tried.
  Since measured ([the clock](l3-dwell-policy-clock-2026-10.md)): at 40 s the policy keeps CMR's
  moving tracks, absorbs a quarter as many tracks with no re-track excess, and still leaves only
  one stationary track over 60 s.
- **One placement, one commit pair.** a0 and CMR were replayed at 995036d64 and CMR + dwell at
  eac41ad9e; the commits between them add the region map to the evidence and the option itself,
  neither of which changes a replay with the option off.

## Recommendations

- **Run Stage B (l3bg0009) with the dwell policy on**, once its labelled held-out window
  (label0001) and the Pi timing (perf0001) exist. It is the counterpart any overrides-on retune
  needs before it can ship.
- **Measure the cost before shipping it**: the pedestrians it absorbs and the spot it absorbs
  repeatedly, on a capture with labels, and departures on one with a car that parks and leaves.
  Since measured on kirk0 and kirk1 ([labelled cost](l3-dwell-policy-labelled-cost-2026-10.md)):
  no moving road user is absorbed, and the one absorption is a parked car the reviewed pack labels
  in every frame; neither capture holds a person standing for 20 s, so the pedestrian cost is still
  open.
- **Treat the scene change separately**, as the existing proposal does: the policy is not a
  recovery mechanism.

## Provenance

- **Code:** `dwell_policy` at eac41ad9e on `claude/experiment-overnight` (written by an agent as
  562dcb769 and cherry-picked); a0 and CMR replays from the harness stamped at 995036d64.
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/overnight-20261013/l3bg0008/replay/{a0,cmr,cmr-dwell}/lombard-laguna/`;
  the discarded first a0 attempt at 256 retained points under `…/l3bg0008/discarded/`; screen JSON
  kept locally.
- **Run:** replays of 16 min 18 s, 23 min 59 s and 19 min 55 s (estimated 15 min each) reading the
  captures from the USB LiDAR volume; screens of 7 min 39 s, 13 min 24 s and 1 min 20 s. The first
  a0 attempt was stopped after 9 min 31 s: at 256 retained points its evidence would have filled
  the internal disk.
