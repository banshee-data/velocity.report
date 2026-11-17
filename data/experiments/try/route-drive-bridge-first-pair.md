# Experiment: the first drive between two registered sites, exported and tracked

Export the car drive from lombard-laguna to columbus-north-point (2 September 2026). Check that
its point times are usable for de-skew, and once Gerardus returns a trajectory, measure the ego
track and the held-out far-end anchor.

- **Status:** Proposed
- **Layers:** L1 decode, L2 rotations (export only); Gerardus odometry and registration; offline ego kinematics
- **Related plan:** [lidar-route-slam-minimum-plan.md](../../../docs/plans/lidar-route-slam-minimum-plan.md)
- **Review brief:** [lidar-route-slam-oracle-brief.md](../../../docs/research/lidar-route-slam-oracle-brief.md)

## Goal

Decide whether a point-to-point drive bracketed by two registered static poses can be (a)
registered against the public priors along its route and (b) turned into a platform track whose
speed, acceleration, jerk and vibration carry stated, checked uncertainty, before any new capture
is made.

## Hypothesis

1. Export: every rotation's azimuth-clock fit succeeds. The period stays within 2% of its median
   while the car moves, and the residual of recorded packet times about the line is of the order
   measured on the perf capture (about 250 µs RMS). If so, the clock is carrying the timing,
   not the packets.
2. Registration: with anchor A only, the trajectory reaches B within the S1 drift budget (2% of
   distance). Submaps at intersections pass the static gates (ground p95 ≤ 0.20 m, facade
   p75 ≤ 0.20 m, facade range slope ≤ tan 0.5°), and mid-block submaps report along-street
   directions as weak rather than failing.
3. Ego track: during stops at lights, displacement stays below 0.3 m and speed below 0.3 m/s.
   Integrated distance agrees with the solved path length. Median speed σ is under 0.1 m/s with
   the default ±0.5 s fit.

## Inputs

| Item        | Value                                                                                                                                               |
| ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Start site  | lombard-laguna, `REGISTERED`, ICP, heading 265.7°, σ 0.002 m / 0.004°, record `gms:01M65JQPJ01KGD7PQ8P8FHDPNM`                                      |
| End site    | columbus-north-point, `REGISTERED`, ICP, heading 132.9°, σ 0.007 m / 0.007°, record `gms:01M678THXY2V5KK866E61J4353`                                |
| Site bounds | lombard-laguna ends 2026-09-02T11:37:17.82-07:00; columbus-north-point starts 11:53:34.55-07:00 (`tools/s2-archive/site-index.json`)                |
| Window      | 30 s each side of the gap, so both ends include static frames for the anchor check: `-from 2026-09-02T11:36:47-07:00 -to 2026-09-02T11:54:04-07:00` |
| Captures    | `s2_sf_2_20260902113332_00026.pcap` to `s2_sf_2_20260902115334_00030.pcap`, in order                                                                |
| Mount       | car, as recorded in the field notes; pass with `-platform` and `-mount-*` if known                                                                  |
| Build       | `make build-lidar-motion-export build-lidar-ego-track` at the PR head; the manifest records version and SHA                                         |

No tuning parameter is changed. The export runs no perception, so no experiment config file is
needed. The ego-track options are the defaults recorded in `summary.json`.

## Method

Paths: `$ARCHIVE` holds the captures, and `$LOCAL` is hydra's local evidence volume, outside
the checkout.

1. Build and check the stamp: `make build-lidar-motion-export build-lidar-ego-track`.
2. Export at full density, or with `-azimuth-stride 4` if disk is short (about 23 GB against
   6 GB):

   ```bash
   /tmp/lidar-motion-export \
     -pcap $ARCHIVE/s2_sf_2_20260902113332_00026.pcap -pcap $ARCHIVE/<00027> -pcap $ARCHIVE/<00028> \
     -pcap $ARCHIVE/<00029> -pcap $ARCHIVE/s2_sf_2_20260902115334_00030.pcap \
     -from 2026-09-02T11:36:47-07:00 -to 2026-09-02T11:54:04-07:00 \
     -start-site lombard-laguna -end-site columbus-north-point -platform car \
     -out-dir $LOCAL/route-slam/lombard-laguna--columbus-north-point/export
   ```

3. Check `rotation_hz` in `run.json`. It must be about 10 for this drive, and the site index
   records no rate. If it reads 20, the archive is 20 Hz and the rate arms below swap roles.
   Then read `frames.csv`: the share of rotations with `clock_ok`, the period over time (stops
   against motion), residual percentiles, packet gaps. Repeat with `-time-source packet_firetime`
   on a 60 s sub-window and compare the two time sources' frame-to-frame odometry residuals in
   Gerardus. That comparison is the evidence for the clock, not the clock's own residual.
4. Gerardus: import the LAS files and `run.json`, then run odometry, submaps and prior factors
   against the bundle for both sites' cells. Solve A-only and A+B. Write each trajectory in
   `velocity.report/ego-trajectory/v0`, frame EPSG:7131.
5. Write `anchors.json` with both sites' accepted poses in EPSG:7131: A `at: start`, B `at: end`,
   `held_out: true` for the A-only trajectory.
6. Ego track, for each solve:

   ```bash
   /tmp/lidar-ego-track -trajectory $LOCAL/.../traj-a-only.json -anchors $LOCAL/.../anchors.json \
     -out-dir $LOCAL/route-slam/lombard-laguna--columbus-north-point/ego-a-only
   ```

7. Rate arm (temporal ablation): repeat steps 2, 4 and 6 with `-keep-every-rotation 2` into a
   separate `export-5hz` directory, so the drive reads as 5 Hz at 10 Hz angular density. Hold the
   ego-track options fixed in seconds (the defaults) for both arms. Compare held-out B residuals,
   stop displacement, speed σ and odometry convergence failures. This measures sensitivity to
   inter-scan motion only; it is not a 5 Hz or 20 Hz sensor result.
8. Write the report under `docs/lidar/operations/`. Include summary tables only (timing,
   per-submap gates, anchor residuals, stops, kinematic maxima and σ, the jitter peak), with the
   manifest digests and build SHA as provenance. No trajectory, coordinates along the route or
   per-frame rows.

## Success criteria

- [ ] At least 99% of exported rotations have `clock_ok`, and no rotation in motion is a period outlier without a packet gap to explain it
- [ ] Held-out B residual (A-only solve) within 2% of the drive distance, and stated with the submap gate outcomes along the route
- [ ] Every stop at least 5 s long shows displacement under 0.3 m on the A+B solve
- [ ] Ego-track distance within 1% of the solved path length; median speed σ under 0.1 m/s
- [ ] The jitter spectrum is reported with its Nyquist limit and the fit's gain, and the report says what it cannot see
- [ ] The 5 Hz ablation is reported beside the native arm with the same windows, and the report states which of its differences a native 20 Hz capture would still have to confirm

## Risks and controls

| Risk                                                | Control                                                                                            |
| --------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| A file in the window does not join                  | The exporter refuses a broken join; report it and choose another pair                              |
| The drive includes parking manoeuvres at either end | The 30 s margins are static by the site bounds; anchor spread in `summary.json` flags a moving end |
| Traffic biases odometry                             | Report residuals at stops separately; flag submaps with large dynamic fractions                    |
| Overreading jerk                                    | Report jerk only with its σ and the fit's bandwidth; no jerk claim below 3σ                        |

## Outputs

- Local only: LAS segments, `run.json`, `frames.csv`, trajectories, `summary.json`, `kinematics.csv`.
- In Git: the authored report and its tables, linked from the plan and a devlog fragment.

## Result

Fill after running.

## Next action

If 1–3 pass, run the seven one-registered-end pairs qualitatively and move to the radar speed
comparison (route plan Item 11). If 2 fails on along-street drift, add `feature` anchors at
intersections before any capture day.
