# The dwell policy's clock: 20, 40 or 60 s over Lombard–Laguna, October 2026

The [dwell policy](l3-dwell-policy-arrival-2026-10.md) hands an object that has been stationary for
20 s to the L3 background. Over the 39-minute Lombard–Laguna placement under the overrides-on retune
(CMR) it removed every stationary track over 60 s, but it also absorbed 86 tracks, 16 of them
classed pedestrian, lost 42 of CMR's moving tracks, and an absorbed object came back as a new track
nearby more often than at the same spots without the policy. On the labelled kirk captures it cost
no moving road user ([labelled cost](l3-dwell-policy-labelled-cost-2026-10.md)), but no one there
stands for 20 s. This run tries the clock at 40 s and 60 s.

- **Status:** Measured, October 16, 2026 (experiment
  [l3bg0015](../../../data/experiments/l3bg/l3bg0015.md)). Nothing shipped changes; label-free
- **Layers:** L5 tracks, L3 background, the pipeline between them
- **Related:** [dwell policy](l3-dwell-policy-arrival-2026-10.md),
  [labelled cost](l3-dwell-policy-labelled-cost-2026-10.md),
  [pose step](l3-scene-change-pose-step-2026-10.md)

## Summary

- **At 40 s the policy keeps CMR's moving tracks and still clears the long stationary ones.**
  Moving tracks 1,044 (CMR 1,045; the 20 s clock 1,003), one stationary track over 60 s (CMR 14,
  20 s none), stationary track-seconds 1,719 (CMR 3,818, 20 s 625).
- **It absorbs a quarter as much, and nothing comes back.** 21 absorptions against 86, six of
  them classed pedestrian against 16; a new track starts within 2 m in the next 60 s after 6 of
  them, against 8 at the same points on CMR without the policy. At 20 s it was 41 against 28.
- **60 s gives back stationary tracks and saves almost no one.** Five stationary tracks over 60 s,
  12 absorptions of which five pedestrian-classed: a few people stand still for over a minute,
  and a longer clock does not spare them; only a rule by class would.
- **The drift rule caps the clock.** The longest stationary track is 76 s at both 40 and 60 s:
  a track whose estimated position wanders more than 1 m from where its stationary run began
  restarts the clock, so the tolerance, not only T, decides what is ever absorbed. On kirk0 the
  parked car's runs end at 34.7 and 35.9 s for that reason, so 40 s absorbs nothing there.

## Method

- **The option.** `dwell_policy_t40` and `dwell_policy_t60`, the same policy as `dwell_policy`
  (speed at most 0.5 m/s, within 1 m of where the stationary run began, a 0.2 m box margin) with
  the clock at 40 s and 60 s; default off, refused in pairs, fingerprint-neutral. With the 20 s
  option, kirk0's tracking baselines and dwell report under CMR are byte-identical to the build
  before the change.
- **Replays.** The whole placement (eight segments, `-warmup 70 -duration 0`, 32 retained points)
  under CMR ([l3-retune-cm2-mp8-floor.json](../../../config/experiments/l3-retune-cm2-mp8-floor.json))
  with each clock, as l3bg0008 replayed its arms. The shipped grid, CMR alone and CMR at 20 s are
  l3bg0008's replays; the code those arms run has not changed since.
- **Screen.** l3bg0008's: lifetimes from first to last estimate, stationary is over 20 s with a
  90th-percentile speed under 0.3 m/s, moving is at least 2 s at 2 m/s or more, in five-minute
  uptime bins. For each absorption, whether a track's first estimate falls within 2 m in the next
  60 s, and the same at the same points and times on CMR without the policy.
- **The sensor.** The placement holds a knock to the sensor at 32:50 of replay uptime, a second
  pose change at 34:51 and the pickup from 37:35 ([pose step](l3-scene-change-pose-step-2026-10.md)),
  so the 30–35 and 35–40 minute bins are not clean.

## Findings

| Measure                                     | Shipped        | CMR            | CMR, 20 s         | CMR, 40 s      | CMR, 60 s     |
| ------------------------------------------- | -------------- | -------------- | ----------------- | -------------- | ------------- |
| Moving tracks                               | 486            | 1,045          | 1,003             | 1,044          | 1,045         |
| Tracks                                      | 2,878          | 3,712          | 3,825             | 3,723          | 3,724         |
| Stationary over 20 s / 60 s / 300 s         | 43 / 10 / 0    | 44 / 14 / 2    | 24 / 0 / 0        | 48 / 1 / 0     | 45 / 5 / 0    |
| Stationary lifetime, median / p90 / max (s) | 34 / 111 / 293 | 39 / 252 / 654 | 22 / 43 / 48      | 38 / 42 / 76   | 38 / 61 / 76  |
| Stationary track-seconds                    | 2,500          | 3,818          | 625               | 1,719          | 1,908         |
| Tracks absorbed                             | —              | —              | 86                | 21             | 12            |
| … classed pedestrian / dynamic / bird / car | —              | —              | 16 / 48 / 12 / 10 | 6 / 13 / 2 / 0 | 5 / 6 / 1 / 0 |
| Re-tracked within 2 m in 60 s               | —              | —              | 41 of 86          | 6 of 21        | 2 of 12       |
| … at the same points without the policy     | —              | —              | 28 of 86          | 8 of 21        | 3 of 12       |

Clusters a frame by five-minute uptime bin:

| Uptime (min) | Shipped |   CMR | CMR, 20 s | CMR, 40 s | CMR, 60 s |
| -----------: | ------: | ----: | --------: | --------: | --------: |
|          0–5 |    4.21 |  5.13 |      5.18 |      5.13 |      5.13 |
|         5–10 |    4.06 |  4.71 |      4.31 |      4.61 |      4.68 |
|        10–15 |    3.72 |  4.29 |      4.09 |      4.29 |      4.29 |
|        15–20 |    2.90 |  4.57 |      4.15 |      4.54 |      4.57 |
|        20–25 |    3.60 |  6.33 |      4.99 |      5.48 |      5.69 |
|        25–30 |    3.98 |  6.70 |      7.20 |      5.67 |      5.70 |
|        30–35 |    4.72 |  7.54 |      6.36 |      6.35 |      6.70 |
|        35–40 |    8.75 | 13.77 |      8.62 |      9.66 |     10.00 |

## Interpretation

The 20 s clock was short enough to absorb things that were only pausing: it took 86 tracks, its
re-track excess (41 against 28) says some of them moved on and were tracked again, and it confirmed
42 fewer moving tracks than CMR, probably for the same reason, though this screen cannot follow a
track through its absorption to show it. At 40 s
the pauses mostly end before the clock does: the absorptions fall to a quarter, the moving tracks
come back to CMR's count, and what is absorbed stays absorbed. What 40 s gives up is time: a
stationary object now holds a track for up to 40 s before it is handed over, so stationary
track-seconds are 1,719 rather than 625, still under half of CMR's and below the shipped grid's.

Beyond 40 s little changes, because the drift rule, not the clock, ends most stationary runs: an
estimated centre wanders as returns come and go (kirk0's parked car crosses 1 m after 34.7 s
without moving), and once it has moved 1 m from where the run began the clock restarts. That also bounds how long anything can
stay stationary and unabsorbed (76 s here). Five tracks classed pedestrian are absorbed at both 40
and 60 s: people who stood still for over a minute, which no clock short of minutes avoids.

The arrival regime's growth under CMR is held back at 40 s as well as at 20 s through the clean
part of the capture (5.5 to 5.7 clusters a frame at 20 to 30 minutes, against CMR's 6.3 to 6.7).

## What this does not establish

- **Correctness.** Lombard–Laguna has no labels: the counts say what the tracker did, not what
  was on the street, and a pedestrian-classed track may not be a person.
- **Other placements.** One capture; l3bg0012 and l3bg0016 screen the 20 s clock on the corpus's
  short segments, where neither longer clock can act much.
- **The drift tolerance.** It was fixed at 1 m; the run shows it matters, not what it should be.
- **Departures.** A car that parks for over 40 s and then leaves was not measured.

## Recommendations

- **Run Stage B (l3bg0009) with the clock at 40 s**, not 20 s: on this capture it keeps every
  moving track while clearing the long stationary ones.
- **Measure the drift tolerance next**: 1 m against a larger value, or drift measured from a
  running median of the position rather than the run's first estimate, on this placement and on
  kirk0's parked car.
- **Leave a class-aware rule** until labels show the pedestrians absorbed at 40 s are people.

## Provenance

- **Code:** `dwell_policy_t40` and `_t60` written by an agent as 4c35da80f on `agent/dwell-t` and
  cherry-picked as 214323824 on `claude/experiment-followup`; the harness built stamped there.
  The shipped, CMR and 20 s arms are l3bg0008's (harness at 995036d64 and eac41ad9e).
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/followup-20261015/l3bg0015/replay/{cmr-dwell-t40,cmr-dwell-t60}/lombard-laguna/`;
  l3bg0008's under `…/overnight-20261013/l3bg0008/replay/`; screens kept locally.
- **Run:** replays of 32 and 26 minutes and screens of 4 and 2 minutes, with other lanes reading
  the same volume.
