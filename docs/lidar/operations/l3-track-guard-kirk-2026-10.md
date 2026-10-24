# L3 track guard: keeping tracked vehicles out of the background

Replays of the kirk captures keep vehicles in the background. The full frames a site model
accumulates carry vehicle returns classed as background, and the settled background Gerardus
registers keeps vehicle-shaped residue. This report measures the leak on kirk0, finds where it
comes from in L3, and adds a guard that uses what L5 knows and L3 cannot: where the tracked
objects are.

- **Status:** Complete, 2026-10-23; measured again 2026-10-24 with the box placed on its base (see
  [The box's height](#the-boxs-height)). kirk0 only; no held-out capture yet
- **Layers:** L3 (background grid), the pipeline between L5 and L3, the published mask (full frames, streamed foreground)
- **Related:** [backlog item](../../BACKLOG.md) (v0.5.13, background/foreground separation leaks moving objects), [region override attribution](l3-region-override-attribution-kirk-2026-10.md), Gerardus `docs/plans/SITE-MODEL-SPIKE.md` §11
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) (the guard on);
  [track-guard-off.json](../../../config/experiments/track-guard-off.json) (arm `off`, L3 as it ran
  before); [track-guard-hold-only.json](../../../config/experiments/track-guard-hold-only.json)
  (arm `hold`, no edge follow)

## Summary

- **The leak, guard off.** Over kirk0's 832 frames, 420,459 of the published background returns
  lie inside the boxes of observed confirmed tracks (4,339 box-frames): 20 % of all returns inside
  those boxes. 270,037 of them are more than 0.3 m above the box's base, so they are the vehicle,
  not the road under it. Cars moving at 3 to 8 m/s had 40 to 80 % of their in-box returns
  published as background.
- **Where it comes from.** A return inside its cell's acceptance window is background, and the
  windows on the road are metres wide: of the in-box background returns 56 % were inside the
  cell's EMA window, 34 % inside its locked-baseline window and 10 % passed by neighbour
  confirmation. Of the window-accepted ones, 99 % were in cells running a region override
  (noise × 3 to × 8), 68 % had windows of 4 m or more, and 76 % were more than 0.3 m in front of
  the background the cell had learned. Each accepted return then updates its cell, at the 5×
  re-acquisition rate after recent foreground, so the traffic pulls the cell's range towards the
  vehicles and widens its spread, which widens the window. Freeze protects; the deadlock breaker
  (59 returns) and seeding (14) hardly count.
- **The settled background.** 1,767 settled cells lie inside a moving track's box (1 m/s or more)
  and above its bottom 0.3 m. Comparing each with what it held at the last checkpoint before the
  first such box covered it: 491 held the same range before the vehicle came (static structure
  the box overlaps), 131 were not settled yet, and **1,134 learned a surface at least 0.3 m nearer
  than before: the vehicle.** The raw count overstates the leak; this split is the measure.
- **The guard** (`l3.ema_baseline_v1.track_guard`, on in the defaults): the boxes of confirmed
  tracks moving at 1 m/s or more are held for the frames after each tracker update, so their
  returns change nothing in their cells, for a second after as well; and the published mask
  carries every background return inside a tracked object's box as foreground.
- **Result.** In the published frames, **0** background returns inside any observed confirmed
  track's box. In the settled background, **71** cells learned nearer under a moving box, against
  1,134: 94 % fewer. The hold alone (arm `hold`) gives the same background; without the edge
  follow its frames still publish 12 % of in-box returns as background, since the hold changes
  what the model learns, not what it classes.
- **Tracks.** L4's input moves with the hold (the model stops learning vehicles), so the tracks
  change. On kirk0: 114 tracks confirmed against 105; the association rate at 5 m/s or more falls
  from 0.71 to 0.66 and the lateral RMS at 2 m/s or more rises from 0.23 m to 0.28 m, while the
  longitudinal RMS and mean NIS at 5 m/s or more improve (0.52 m to 0.43 m, 2.19 to 1.75).
  Without labels this cannot say whether the longer tracks are the same vehicles followed further
  or more fragments joined; it needs a scored run before the guard is trusted for tracking, not
  only for the background.

## Method

One replay of kirk0 (`/Volumes/lidar/lidar/pcaps/kirk/kirk0.pcapng`, 832 frames, port 2369) per
arm, with `velocity lidar pcap-replay --background-leak`, which writes `background_leak.json`
(`replayeval.BackgroundLeakReport`). The measure reads what a site model takes from a replay:

- **Frames.** Each published frame's mask (0 background, 1 foreground), counted inside and outside
  the published box of every confirmed track observed in that frame. Coasting tracks are left out,
  since their box is a prediction. A return under a box's base is outside it.
- **Settled background.** The cells `GenerateBackgroundSnapshot` sends at the end, counted inside
  every box the replay drew and inside moving tracks' boxes. Every 50 frames the replay keeps the
  settled cells' ranges (`BackgroundManager.SnapshotRanges`); each settled cell under a moving box
  at the end is compared with its range at the last checkpoint before the first box that
  covered it.

The causes were read from per-return instrumentation in `ProcessFramePolarWithMaskAt`, run once
on the `off` arm and removed.

## Results

| kirk0, 832 frames                                   |      `off` |     `hold` |   defaults |
| --------------------------------------------------- | ---------: | ---------: | ---------: |
| Box-frames (observed confirmed tracks)              |      4,339 |      4,909 |      4,909 |
| Published background returns                        | 55,598,651 | 55,177,192 | 54,745,758 |
| Of those inside a box                               |    420,459 |    295,787 |          0 |
| Share of in-box returns published as background     |       20 % |       12 % |        0 % |
| Settled cells                                       |     70,004 |     69,893 |     69,893 |
| Inside a moving track's box, above its bottom 0.3 m |      1,767 |        632 |        632 |
| Learned ≥ 0.3 m nearer after the box came           |  **1,134** |     **71** |     **71** |
| Same range before the box came                      |        491 |        495 |        495 |
| Not settled before the box came                     |        131 |         53 |         53 |

| Tracks, kirk0                               |              `off` |             defaults |
| ------------------------------------------- | -----------------: | -------------------: |
| Confirmed                                   |                105 |                  114 |
| Association rate ≥ 2 m/s (matched / missed) | 0.61 (1,028 / 662) | 0.58 (1,531 / 1,108) |
| Association rate ≥ 5 m/s                    |   0.71 (400 / 164) |     0.66 (466 / 240) |
| Lateral RMS ≥ 2 m/s                         |             0.23 m |               0.28 m |
| Longitudinal RMS ≥ 5 m/s                    |             0.52 m |               0.43 m |
| Mean NIS ≥ 5 m/s                            |               2.19 |                 1.75 |

Born tracks, support instants, reacquisitions and time in view were not measured again after the
box fix; with the box half its height too low, the defaults gave 116 confirmed, 882
reacquisitions against 447, and 609 s of moving tracks in view against 454 s.

`hold` and the defaults run identical tracks: the edge follow writes only the published mask.

**The speed floor.** A first version held every confirmed track. Still tracks' time in view rose
from 225 s to 351 s, three of them over 70 s at under 0.15 m/s: a static object tracked as one
kept itself foreground by its own hold, since its cells never learned it. Holding only tracks at
1 m/s or more (`hold_min_speed_mps`) puts still tracks back at 216 s; the settled background
learns 498 cells under moving boxes instead of 223 (both measured with the box too low, before
the fix below). A stopped vehicle is absorbed as before.

**Other settings tried once** (no change adopted, measured with the box too low): a cool-down of
0 s leaves 655 learned cells; a hold margin of 0 m leaves 725.

**What is left.** The 71 cells are learned where no hold applied: before a track is confirmed
(four hits), while it moves under 1 m/s, or beyond the box's margin.

## The box's height

The first measurement (2026-10-23) placed every box half its height too low.
`l5tracks.PublishedBox` took `LatestZ` as the box's vertical centre, but `LatestZ` is the cluster
OBB's `CenterZ`, which is the cluster's lowest return: the box's base, where viewers draw it and
where the visualiser's transient boxes put it. Unless the box model fitted a height (the ground
anchor, off by default), the guard's held boxes and the leak measure's boxes reached from half a
vehicle below the road to half its height above it, and the edge-follow boxes taken from cluster
OBBs did the same. Gerardus's validation on Kirkham found it (Gerardus
`docs/plans/VECTOR-MAP.md` §4.3, "Validated on Kirkham"). The fix puts the centre at
`LatestZ + OBBHeight / 2`, and the cluster boxes' at `CenterZ + Height / 2`.

With the box where it belongs, the measure counts less of the road under a box (420,459 in-box
background returns with the guard off, not 534,882) and the guard keeps more of the vehicle out
of the background: 71 cells learned nearer under moving boxes, not 498.

| kirk0, settled cells learned ≥ 0.3 m nearer under moving boxes | `off` | defaults |
| -------------------------------------------------------------- | ----: | -------: |
| Box half its height too low (2026-10-23)                       | 2,414 |      498 |
| Box on its base (2026-10-24)                                   | 1,134 |       71 |

## Replay determinism

Runs of one arm agree on every count but `background_in_track_boxes_above_base`, which varied by
up to 14 (432,201 to 432,215, with the box too low) between runs of `off`. The replay itself is not bit-identical run to
run on this capture; the figures above are one run each.

## Provenance

- Base commit `a0a404212`; the code is this change's. The 2026-10-24 figures are from `1b6496563`
  with the box fix. Binaries built in the worktree with
  `go build -tags pcap` (unstamped: the VRLOGs record `dev`/`unknown`).
- `params_sha256`: `off` `657e94fae107…`, `hold` `c3b54e260671…`, defaults `e44f577dc1df…`.
- The replay outputs were not kept; `background_leak.json` per arm is reproduced by the command
  above.
- Covered by `TestBackgroundLeakOnKirk0` (pcap tag), which replays 30 s of kirk0 under the
  defaults and the `off` arm and requires no in-box background under the defaults.
