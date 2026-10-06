# Near-edge frame cost, October 2026

- **Status:** Complete. A2's Tracker.Update p99 is 9 to 21 times B0's, but A2 moves the whole frame's p99 by 2 % to 7 % and its p95 by 5 % to 10 %, at the October campaign's three timing sites. A2's update is 2 % to 6 % of its frame p99, and every frame p99 stays under 58 ms against the 98 ms budget.
- **Scope:** B0 and A2 on the span-selection build with frame timing, at 3rd-folsom, embarcadero-bryant and pierce-haight, 300 s scored, balanced order, four runs per arm and site, captures from the internal SSD, on the Mac.
- **Related:** [October near-edge campaign](near-edge-campaign-2026-10.md), [update cost profile](near-edge-update-cost-profile-2026-10.md), [near-edge tracked-state plan](../../plans/lidar-near-edge-tracked-state-plan.md).

The October campaign's cost screen compares Tracker.Update p99 alone: A2 breaches it when its update
p99 is over 1.5 times B0's. B0's update is about 0.5 % of a frame's CPU, so a ratio to it says little
about what a live sensor pays per frame. The screen's scope (the update, or the frame) is an open
decision. This record measures the frame.

## Method

| Item      | Value                                                                                                                                                                                                                                                |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Build     | The span-selection branch at `075fb20b7` (span and near-edge percentile selection) with the frame timing change cherry-picked (`c8da55a59`), built from a clean checkout with its git SHA stamped                                                    |
| Arms      | B0: no experiment. A2: `solid_body`, `solid_body_face_hysteresis`, `solid_body_course_faces`, `solid_body_full_members`, `near_edge_track`. The campaign's definitions                                                                               |
| Runs      | `lidar-state-estimation-baseline -case <site> -duration 300 -warmup 70 -campaign-metrics`, no evidence database. Each run replays twice; the repeat replay's timing is used                                                                          |
| Timing    | Frame: `frame_timing.json`, the wall time of each scored frame's pipeline callback (L3 background, L4 clustering, L5 tracking with its Tracker.Update, L6 classification and the replay's recording). Update: `tracker_timing.json`, as the campaign |
| Order     | Per site B0, A2, A2, B0, B0, A2, A2, B0, so a linear drift cancels: four runs per arm                                                                                                                                                                |
| Machine   | Apple M1 Pro, on AC power, under `caffeinate`. Each run waited for the one-minute load to stay under 3.5 for a minute; a run ending with the load at 8 or more would have been repeated, and none was                                                |
| Statistic | The median of each arm's four values per site; the p99 range is the four values' smallest and largest                                                                                                                                                |

## Findings

Milliseconds, medians of four. Every run timed 3,000 frames.

| Site               | Arm | Frame p50 | Frame p95 | Frame p99 | Frame p99 range | Update p50 | Update p99 |
| ------------------ | --- | --------: | --------: | --------: | --------------- | ---------: | ---------: |
| 3rd-folsom         | B0  |      4.96 |      9.69 |      49.5 | 48.3 to 49.9    |      0.033 |      0.102 |
| 3rd-folsom         | A2  |      5.11 |     10.51 |      50.3 | 48.9 to 51.2    |      0.167 |      1.184 |
| embarcadero-bryant | B0  |      4.68 |     11.91 |      51.5 | 50.6 to 52.7    |      0.032 |      0.118 |
| embarcadero-bryant | A2  |      4.77 |     13.08 |      55.0 | 54.8 to 57.1    |      0.144 |      2.424 |
| pierce-haight      | B0  |      4.88 |      6.72 |      13.7 | 12.8 to 14.8    |      0.029 |      0.101 |
| pierce-haight      | A2  |      4.89 |      7.08 |      14.5 | 13.8 to 15.0    |      0.047 |      0.882 |

A2 over B0:

| Site               | Update p99 | Frame p50 | Frame p95 | Frame p99 | A2 update p99 over A2 frame p99 |
| ------------------ | ---------: | --------: | --------: | --------: | ------------------------------: |
| 3rd-folsom         |       11.6 |      1.03 |      1.08 |      1.02 |                           2.4 % |
| embarcadero-bryant |       20.6 |      1.02 |      1.10 |      1.07 |                           4.4 % |
| pierce-haight      |        8.7 |      1.00 |      1.05 |      1.06 |                           6.1 % |

- **The frame barely moves.** A2 adds 0 % to 3 % at the frame p50, 5 % to 10 % at the p95 and 2 % to
  7 % at the p99: 0.8, 3.5 and 0.8 ms at the three sites' p99. Only at embarcadero-bryant do the
  four p99 values of the two arms not overlap.
- **The update ratio is large because B0's update is small.** A2's update p99 is 9 to 21 times B0's,
  as the campaign found, but that is 0.9 to 2.4 ms against a frame p99 of 15 to 55 ms.
- **The frame p99 is set elsewhere.** At 3rd-folsom and embarcadero-bryant B0's own frame p99 is
  about 50 ms, several hundred times its update p99, so the slowest frames are slow for reasons other than
  tracking. Every frame p99 is under 58 ms against the 98 ms budget.

## Interpretation

On a frame-level screen the same 1.5 ratio would pass A2 at all three sites, by a wide margin. On the
update-level screen it fails at all three. The two screens disagree because B0's update is about
0.5 % of a frame. This record does not choose between them; it supplies the frame-level evidence
the choice needs. If the screen moves to the frame, the update ratio remains worth reporting as a
regression signal for the tracker itself.

## Limitations

- One machine, three sites, 300 s windows, four runs per arm. Absolute milliseconds are Mac figures,
  not Pi ones; a Pi is slower, and the update's share of its frame may differ.
- The frame time includes the replay's recording, which a live sensor's VRLOG and gRPC publishing
  resemble but do not equal. Reading packets and assembling frames are outside it.
- A replay applies back-pressure rather than dropping frames, so a slow frame here is a delayed frame;
  on a live sensor a frame over budget would be lost instead.

## Provenance

| Item           | Value                                                                                                                              |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Build          | `c8da55a5982d161f6ec2327e8fcd285cf60a809f` (the span-selection branch at `075fb20b7` with `c8da55a59`), stamped                    |
| Source digests | 3rd-folsom `1d19feceed93…`, embarcadero-bryant `3c55a2292c59…`, pierce-haight `c9b3b0f8d588…`, in every run's manifests            |
| Raw outputs    | Run script, per-run logs and timing files, and `summary.json` on the LiDAR volume under `velocity-campaign/frame-timing-20261006/` |
