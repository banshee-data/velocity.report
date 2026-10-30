# Unrecorded knocks to the sensor across the corpus placements, October 2026

The [pose step](l3-scene-change-pose-step-2026-10.md) found that Lombard–Laguna's scene change was
a knock to the sensor, followed by a second change and the pickup, none of them recorded. Every L3
experiment on the corpus replays these placements, and a knock inside a scored window reads as the
background model's failure. This run sweeps all 23 tuning and screen placements for the same
thing.

- **Status:** Measured, October 16, 2026 (experiment
  [l3bg0014](../../../data/experiments/l3bg/l3bg0014.md)). Nothing shipped changes; label-free
- **Layers:** L1 packets and L2 geometry (raw returns, before any background model)
- **Related:** [pose step](l3-scene-change-pose-step-2026-10.md),
  [region override attribution](l3-region-override-attribution-kirk-2026-10.md) (the corpus
  screen), [held-out geometry replication](geometry-convergence-heldout-2026-10.md)

## Summary

- **Seven of the 23 placements hold at least one persistent pose change**, from 0.03° to 0.27°:
  ashbury-downey (three), broadway-franklin, embarcadero-bryant (two), haight-divisadero,
  lombard-laguna (two, then the pickup), marina-webster-beach (two) and van-ness-sacramento (the
  sensor knocked and adjusted repeatedly for twelve minutes). None appears in any log.
- **Six corpus cases are scored across one.** In the first segment, the one the L3 corpus screens
  (l3bg0006, l3bg0012) read: marina-webster-beach at 175 s, embarcadero-bryant at 205 s and
  van-ness-sacramento at about 150 s, all inside the 70 to 270 s scored window. In the second
  segment, the one the held-out geometry replication and l3bg0016 read: haight-divisadero at 152 s
  and marina-webster-beach at 145 s, and van-ness-sacramento, whose capture opens with the pose
  snapping back and then wobbles.
- **The site index's static parts catch the large changes and miss the small ones.** Its
  classifier ended a part at Lombard–Laguna's knock and at van-ness-sacramento's, but not at
  marina-webster-beach's 0.11° or embarcadero-bryant's 0.13°, which sit inside static parts. A
  replay reads every packet between a site's bounds either way.
- **Three placements creep without a step**, up to 0.088° over twenty minutes
  (california-leon-baker), 0.079° (fulton-divisadero) and 0.057° (pierce-haight), against
  0.008° over 30 minutes at Lombard–Laguna before its knock: a mount settling, not a knock.

## Method

- **The tool.** `lidar-pose-step`, as in the pose step report: the median range of each static
  cell (ring by 0.2° bin) over a 10 s window, and a robust six-degree point-to-plane fit between two
  windows' static backgrounds, with abstention on one-plane or one-sector support. Nothing runs the
  pipeline.
- **Sweep within parts.** In each static part of the site index, a reference window 5 to 15 s in,
  compared with a 10 s window every 30 s to the part's end: 848 windows over the 23 placements, none
  abstaining. A step is a change of more than 0.03° or 0.3 cm between consecutive windows (about
  three times the largest quiet pair at Lombard–Laguna, 0.009° and 0.035 cm); it is persistent when
  the median of the next three windows differs from the median of the previous three by as much,
  and transient otherwise.
- **Boundary pass.** The index ends a static part where its classifier saw motion, so a knock it
  caught sits between parts, where the sweep does not look. Each part's window 21 to 31 s before
  its end is compared with the next part's first window and with its window 60 s in; 53
  boundaries. A first version, with the before-window ending 1 s before the part's end, missed
  Lombard–Laguna's knock: the index ends that part 7 s after it, so the window straddled the step.
- **Positive control.** Lombard–Laguna's two changes are found again, the first at the boundary
  (0.274°) and the second within a part (0.127°).
- **Placing a change.** Its time is placed in its capture from the captures' first and last packet
  times, and against the first- and second-segment corpora's captures, scored from 70 to 270 s.

## Findings

Persistent pose changes (yaw and tilt changes in degrees, translation in centimetres):

| Placement            | Time into placement | Size                          | Where                                   |
| -------------------- | ------------------: | ----------------------------- | --------------------------------------- |
| ashbury-downey       |               655 s | yaw +0.03, tilt +0.09, 0.4 cm | after the first segment's scored window |
|                      |               805 s | yaw +0.15, tilt +0.06, 0.4 cm | after it                                |
|                      |             1,135 s | yaw −0.12, 0.1 cm             | after it                                |
| broadway-franklin    |               955 s | yaw −0.03, 0.4 cm             | a later capture                         |
| embarcadero-bryant   |               205 s | yaw +0.13, tilt +0.09, 0.4 cm | **first segment, scored (205 s)**       |
|                      |    709 to 952 s gap | yaw −0.10, 0.1 cm             | between parts, a later capture          |
| haight-divisadero    |               415 s | yaw −0.06, 0.8 cm             | **second segment, scored (152 s)**      |
| lombard-laguna       |             2,040 s | yaw +0.26, tilt 0.09, 0.7 cm  | between parts (the knock)               |
|                      |             2,157 s | yaw +0.07, tilt −0.09, 0.4 cm | a later capture                         |
| marina-webster-beach |               175 s | yaw +0.11, tilt +0.07, 0.7 cm | **first segment, scored (175 s)**       |
|                      |               446 s | yaw +0.06, tilt +0.07, 0.3 cm | **second segment, scored (145 s)**      |
| van-ness-sacramento  |              ~150 s | yaw −0.19, tilt +0.08         | **first segment, scored**; see below    |

van-ness-sacramento's index has a static part from 0 to 154.5 s, then none until 901 s. Against a
reference window 10 to 20 s in, the static background fits yaw −0.19° from about 150 s, returns to
within 0.04° at about 300 s (the start of the second-segment capture), wobbles by up to 0.035°
through it, steps to −0.14° at about 630 s and drifts to −0.21° by 820 s; every window keeps about
50,000 static cells, so the sensor was knocked and adjusted in place, not carried.

Slow drift with no step (largest rotation against the part's reference): california-leon-baker
0.088°, fulton-divisadero 0.079°, pierce-haight 0.057°; every other placement without a step 0.03°
or less.

## Interpretation

A knock of a tenth of a degree moves a static surface only 3.5 cm across the beam at 20 m and 9 cm
at 50 m, well inside the closeness window for a surface seen face-on. But where a beam meets a depth
edge or a surface at grazing incidence, that shift becomes a range change of metres: at
Lombard–Laguna the median static cell changed by 2 cm while 24% changed by more than 10 cm. Those
cells read as foreground until the grid relearns them, which takes minutes. That is what
Lombard–Laguna's 0.26° did in the replays, and the six cases above carry the same thing at smaller
scale inside windows that are scored as if the scene were still.

The corpus screens compare arms on the same captures, so a knock moves every arm's counts at once
and mostly cancels in a paired comparison; what it does not cancel is an arm that relearns faster
or slower than another, which is exactly what L3 retunes differ in. The six cases should be read
apart, not dropped silently.

## What this does not establish

- **Changes under 0.03°.** The threshold is three times the quiet pairs' largest; smaller knocks
  are not ruled out.
- **The effect on any arm's counts.** This finds the changes; it does not measure what they did to
  each experiment's results.
- **embarcadero-folsom**, the held-out site, was not swept.

## Recommendations

- **Report the six cases apart** in every corpus screen and in the held-out geometry replication:
  first segment marina-webster-beach, embarcadero-bryant and van-ness-sacramento; second segment
  haight-divisadero, marina-webster-beach and van-ness-sacramento.
- **Run the pose sweep when a capture enters the corpus**, and record the changes in the site
  index beside its static parts, so the next corpus or held-out window avoids them or marks them.
  The sweep takes about a second a window.
- **Sweep embarcadero-folsom** before its held-out window (label0001) is labelled, without scoring
  anything on it.

## Provenance

- **Code:** `lidar-pose-step` at fed1ef684 on `claude/experiment-followup`, built stamped
  (0.5.1-pre45); the pair lists from `knock_sweep.py` and the analysis from `knock_analyse.py`,
  scratch scripts kept outside the repository.
- **Captures:** the 23 placements of `tools/s2-archive/site-index.json` other than
  embarcadero-folsom, under the LiDAR volume's `pcaps/s2`; segment captures from the D2
  first-segment corpus and the second-segment corpus the held-out replication built.
- **Outputs:** per-placement fit JSON and Markdown, the boundary passes and van-ness-sacramento's
  window series, kept locally.
- **Run:** the sweep 40 minutes, the two boundary passes 9 and 10 minutes, with other lanes reading
  the same volume.
