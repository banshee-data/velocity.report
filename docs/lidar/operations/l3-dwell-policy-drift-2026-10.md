# The dwell policy's drift rule at 40 s, October 2026

The dwell policy's stationary clock restarts when a track's estimated position moves more than 1 m
from where its stationary run began. On kirk0 that rule keeps the policy at 40 s from ever
absorbing the parked car, whose estimate wanders 1 m in 34.7 s without the car moving
([the clock](l3-dwell-policy-clock-2026-10.md)). This run tries two other rules at 40 s over the
Lombard–Laguna placement: a 2 m tolerance, and the 1 m tolerance measured from the run's running
median position rather than its first estimate.

- **Status:** Measured, October 16, 2026 (experiment
  [l3bg0017](../../../data/experiments/l3bg/l3bg0017.md)). Nothing shipped changes; label-free
- **Layers:** L5 tracks (the stationary clock), L3 background
- **Related:** [the clock](l3-dwell-policy-clock-2026-10.md),
  [dwell policy](l3-dwell-policy-arrival-2026-10.md),
  [labelled cost](l3-dwell-policy-labelled-cost-2026-10.md)

## Summary

- **On Lombard–Laguna the drift rule hardly matters.** The running-median rule gives exactly the
  1 m rule's result (21 absorptions, the same 446 cells); 2 m adds one, a track classed dynamic.
  Moving tracks (1,044), stationary tracks over 60 s (one) and the re-track rate (7 of 22 against
  8 at the same points without the policy) are unchanged.
- **On kirk0 it decides one object.** Under either new rule the parked car's first stationary run
  is unbroken and it is absorbed at 40.1 s; under the 1 m rule from the first estimate it is never
  absorbed at 40 s.
- **The long stationary track the clock report pointed at is not a drift case.** The 76 s track
  that survives every clock moved 13.6 m, at up to 4.1 m/s, 38 to 40 s into its life; the speed
  rule restarts its clock, as it should.
- **Keep the 1 m first-estimate rule** for Stage B; whether to absorb a parked car at 40 s is a
  question for labels with parked cars in them, not for this placement.

## Method

- **The options.** `dwell_policy_t40_drift2m` (40 s, 2 m from the run's first estimate) and
  `dwell_policy_t40_median` (40 s, 1 m from the running median of the run's estimated positions
  over up to its last 400 observed frames, 40 s at 10 Hz), default off, refused in combination with
  any other dwell setting, fingerprint-neutral. The rule now lives on L5's tracker configuration;
  with no option set, kirk0's tracking baselines under the shipped defaults are byte-identical to
  the build before it, and under `dwell_policy` (20 s) the dwell report differs only by the added
  field naming the anchor. A 400-frame median trails a steady creep by half its run and so
  restarts after about 2 m of creep, as the 2 m rule does; a 200-frame window let a 3 m creep over
  50 s through, so it was not used.
- **Replays and screen.** As [l3bg0015](../../../data/experiments/l3bg/l3bg0015.md): the whole
  placement under CMR, 32 retained points, the stationary-lifetime screen and the re-track check
  against CMR without the policy.

## Findings

| Measure                                       | 40 s, 1 m from first | 40 s, 2 m from first | 40 s, 1 m from median |
| --------------------------------------------- | -------------------- | -------------------- | --------------------- |
| Moving tracks                                 | 1,044                | 1,044                | 1,044                 |
| Stationary over 20 s / 60 s                   | 48 / 1               | 48 / 1               | 48 / 1                |
| Stationary lifetime, median / p90 / max (s)   | 37.6 / 41.9 / 76.0   | 37.9 / 41.9 / 76.0   | 37.6 / 41.9 / 76.0    |
| Stationary track-seconds                      | 1,719                | 1,719                | 1,719                 |
| Tracks absorbed (pedestrian / dynamic / bird) | 21 (6 / 13 / 2)      | 22 (6 / 14 / 2)      | 21 (6 / 13 / 2)       |
| Cells absorbed                                | 446                  | 517                  | 446                   |
| Re-tracked within 2 m in 60 s / control       | 6 / 8                | 7 / 8                | 6 / 8                 |

kirk0 under CMR (the agent's check, one replay per rule): the 1 m first-estimate rule at 40 s
absorbs nothing; the 2 m and running-median rules each absorb the parked car (track 28) at 40.1 s,
12 returns at (−4.1, −9.3) m, the same frame under both.

## Interpretation

On a busy street most stationary tracks that reach 40 s are still enough that any of the three
rules lets the clock run, and the ones that do not are moving, which the speed rule catches before
the drift rule matters. The drift rule earns its place on large, partly seen objects whose
estimated centre slides as their visible returns change, which is what kirk0's parked car is. The
running median handles that slide without loosening the rule for a genuine creep, but on
Lombard–Laguna there was nothing for it to do.

## What this does not establish

- **Which parked-car behaviour is wanted.** Absorbing a parked car is the policy's purpose; the
  reviewed pack counts the car in every frame ([labelled cost](l3-dwell-policy-labelled-cost-2026-10.md)),
  so a labelled score would charge either rule for it.
- **Other placements.** One long capture and one short one.

## Recommendations

- **Keep the 1 m first-estimate rule** with the 40 s clock in Stage B.
- **Revisit the running median** if a labelled window with parked cars shows the policy failing to
  absorb them.

## Provenance

- **Code:** the two settings written by an agent as e8e2ea518 on `agent/dwell-drift` and
  cherry-picked as a0cd49a33 on `claude/experiment-followup`; the harness built stamped there.
  The 40 s, 1 m arm is l3bg0015's.
- **Evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/followup-20261015/l3bg0017/replay/{cmr-dwell-t40-drift2m,cmr-dwell-t40-median}/lombard-laguna/`;
  screens kept locally.
- **Run:** replays of 42 and 46 minutes and screens of 8 and 3 minutes, beside the corpus lanes.
