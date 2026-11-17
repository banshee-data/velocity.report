# Route SLAM phase 1: a two-anchor bridge and the ego track from archive drives (v0.6.0)

Build the sensor platform's own track first: odometry over an archive drive, tied at each end to
the site's accepted static scan through the stationary interval with the tripod on the car roof,
with no prior factors along the route. Phase 1 uses only captures that
already exist and ends in one evaluation on hydra that decides whether phase 2 starts.

- **Status:** Proposed. The items and gates are the
  [reconciliation](../research/lidar-route-slam-oracle-comparison.md) of two independent oracle
  runs ([A](../research/lidar-route-slam-oracle-response-fable.md),
  [B](../research/lidar-route-slam-oracle-response-opus.md)). The owner pinned the gates and
  decisions at the reconciled defaults on 2026-11-17. Nothing here is built beyond the first-pass
  tools in #1056
- **Decisions:** D-36 (Gerardus owns registration; v0.6.0–v0.6.4 is the SLAM/mobile series), D-38
  (priors are measured spatial data)
- **Layers:** offline tooling over L1 decode and L2 rotations (`internal/lidar/motionexport`),
  offline platform kinematics (`internal/lidar/egotrack`); Gerardus odometry and pose graph
- **Target:** v0.6.0; phase 1 needs no new capture
- **Companion plans:** [lidar-route-slam-minimum-plan](lidar-route-slam-minimum-plan.md) (umbrella),
  [lidar-route-slam-phase2-priors-rate-plan](lidar-route-slam-phase2-priors-rate-plan.md),
  [lidar-route-capture-plan](lidar-route-capture-plan.md)
- **Canonical:** [motion-capture.md](../lidar/operations/motion-capture.md)

## Motivation

The ego track does not need priors registration, but priors registration needs everything the ego
track needs. Run A's derivation, which run B does not dispute: a position error that varies along the route as `e(ℓ)` changes
speed by `v × de/dℓ`. A bridge between two anchors without priors leaves about 2 m of error
mid-route, varying over about 1 km. That is a slope of `2e-3`, or 0.03 m/s at 15 m/s.
Acceleration, jerk and vibration do not see it at all. So the ego track can be trusted from a
bridge alone, and the drift model phase 1 measures is what phase 2's prior-factor weights and
alias guard depend on.

The same review found that today's checks are not independent of the trajectory they test. If
they stay that way, a wrong trajectory can pass every one of them. Phase 1 exists to make them
independent before any number is read as evidence.

## Current state

- **Built (#1056):** `lidar-motion-export` (LAS 1.4, azimuth clock, `run.json`, `frames.csv`,
  `-keep-every-rotation`) and `lidar-ego-track` (local cubic kinematics with σ, stops, vibration
  spectrum, end anchors). `lidar-ego-track` is tested on synthetic data only, since no
  trajectory exists yet. `lidar-motion-export` has now run on real drives (next bullet).
- **First real exports (hydra, owner, `7cfe581`):** three September drives, 28,050 rotations,
  exported at `-azimuth-stride 4`.
  - **Defect found:** the first-pass azimuth clock ordered a rotation's blocks by mean point time
    before unwrapping. Real returns drop channels, so that mean shuffled blocks within a packet.
    Where a packet straddled 0°/360°, some blocks unwrapped a whole turn off, and their points
    were timed one to three rotations early.
  - **Before the fix:**
    - 0.7% of points were mistimed, in 89% of rotations;
    - the median clock residual was 9–18 ms;
    - up to 23% of rotations were more than 2% off the median period.
  - **After the fix (unwrapping in firing order):**
    - the median clock residual is 176–188 µs;
    - every rotation's clock fit is accepted;
    - one period outlier remains, in the test drive.

  The residual is the scatter of the recorded packet times about each rotation's line, not a
  measured timing accuracy. It sits just above the roughly 160 µs floor both oracle runs
  derived from the missing per-block offsets, consistent with the clock carrying the timing.
  "Accepted" means the gates at `7cfe581`: span at least 300° and period 0.04–0.25 s. The
  owner's review then led to `866b9d4`, which changes them, so these figures need a re-run on the
  current head:
  - the robust pass repeats until its kept set settles;
  - a fit is refused above 10% rejected blocks or above 500 µs kept-block RMS;
  - residuals now cover kept blocks only.

  Neither oracle run caught the defect, though both read `clock.go`. The
  [first exports report](../lidar/operations/route-slam-first-exports-2026-11.md) names the
  drives and records the figures against the spec's criteria.

- **Gerardus:** robust point-to-plane ICP and the static gates (`src/register.rs`), a small pose
  graph (`src/graph.rs`). No odometry, de-skew, import or bridge solve.
- **Mount (owner, after run A):** for every ride between sites where the capture is continuous,
  the sensor stayed on its tripod, ratchet-strapped to the roof of a car. It was not carried.
  The tripod still moved from the ground to the roof and back at each site. So a site's
  accepted pose (tripod on the ground) is not the drive's first pose (tripod on the roof). The
  handover has to measure that step, not just check that the pose held. Run B received this
  fact and run A did not; the comparison adopts run B's handover for that reason.
- **Continuity:** by file name, 14 of the archive's 21 same-day drives stay in one capture
  series; the other 7 cross a recorder restart. lombard-laguna also has a 55 s hole in its static
  parts just before the first drive.

## Findings

Both runs' review of the first-pass tools. Each is a reason for an item below.

| Area               | Current state                                                                                       | Severity | Item |
| ------------------ | --------------------------------------------------------------------------------------------------- | -------- | ---- |
| Stop check         | Stops come from the trajectory's own speed, so the check selects the intervals it then passes       | High     | 2, 8 |
| Stated σ           | Assumes independent pose noise and defaults to 0.02 m, which alone passes "median σ under 0.1 m/s"  | High     | 8    |
| Anchor handover    | The tripod moves from ground to roof at each site, so the accepted pose is not the drive's end pose | High     | 2, 6 |
| Self occlusion     | The roof, straps and tripod legs are in every rotation; ICP's 1 m minimum range does not mask them  | Medium   | 4    |
| Co-moving traffic  | A vehicle moving with the car looks static in the sensor frame; Huber never reaches zero weight     | Medium   | 4    |
| Pitch lever arm    | The sensor sits 2.5–3.5 m up, so body pitch adds about `h × dθ/dt` to its speed                     | Medium   | 8    |
| Clock fallback     | A refused rotation is still exported with packet times, so one segment can mix two time conventions | Medium   | 1    |
| Rotation timestamp | `frames.csv` stamps each rotation with its arrival-based start, which keeps the jitter              | Medium   | 1    |
| Frame growth       | With timeouts off, a capture that never completes a rotation grows one frame without limit          | Medium   | 1    |
| Gap rule           | Three median intervals: it halves in seconds at 20 Hz                                               | Low      | 8    |
| Path length        | Summing raw poses grows about 0.18 m/s at rest at 10 Hz                                             | Low      | 8    |
| Spectrum           | Always reports a peak and corrects down to a gain of 0.1                                            | Low      | 8    |
| Graph marginals    | `graph.rs` inverts a (6N−6)-square matrix per node and has no robust kernel                         | Medium   | 7    |
| ICP reuse          | `icp` is concrete over `Reference`/`Scan`; odometry needs a small trait                             | Low      | 4    |

## Design / approach

```text
archive pcaps -> lidar-motion-export     LAS, run.json, frames.csv with trust, stillness, lifts (items 1, 2)
              -> Gerardus import          session + site scans and accepted records            (item 3)
              -> odometry, two passes     per-rotation poses between the lifts, masks          (items 4, 5)
              -> handover                 roof-parked interval registered to the site's scan   (item 6)
              -> bridge solve             A-only, B-only, A+B; odometry and solved pose sets   (item 7)
              -> lidar-ego-track          kinematics from odometry poses, σ, stops, spectrum   (item 8)
              -> hydra evaluation 1       three drives, three rate arms, authored report       (item 9)
```

Three rules make the checks independent:

1. **Stillness is measured without a pose.** The exporter marks a rotation still when at least 0.8
   of its range cells (rings at or above the horizon, range over 5 m, 0.4° bins) are within
   0.03 m of the same cells 1.0 s earlier. Stop intervals come from that column, not from the
   trajectory.
2. **Each end is anchored through the roof, not the ground.** The chain breaks at both lifts. The
   stationary interval with the tripod on the roof is found from the rotations (a sensor-height
   step of at least 0.5 m from the ground interval). Its accumulated rotations are registered to
   the site's accepted scan with `register_against` (rank 6, separation ≥ 1.25, p75 ≤ 0.10 m),
   and checked by a triangle closure against a direct 3DEP registration of the same interval,
   within 0.2 m. A failed end removes that anchor and the drive's held-out claim.
3. **Derivatives come from odometry poses, and σ from a noise model that includes drift.**
   Linearly interpolated graph corrections would put a jerk step of about 0.4 m/s³ at every
   keyframe. σ is along-track and has three terms: white noise, a random walk per metre (`q_s`)
   and a scale bias (`ε_s`), all measured on the drive, not assumed.

Everything is fixed in seconds and metres, so one configuration serves both rotation rates. The
proposed values are:

- keyframes every 5 m, 10° or 2 s, and map insertion after 0.5 m or 2°;
- a local map radius of 100 m;
- speed and acceleration on a ±0.5 s window;
- jerk on its own window of ±0.65 s × `(σ_pos / 1 cm)^(2/11)`, reported only above 3σ.

**Drives.** All are continuous by file name:

- **develop on** marina-webster-beach → lombard-laguna and broadway-franklin → laguna-eddy, which
  have one registered end each;
- **test once, untuned,** on lombard-laguna → columbus-north-point, which has both ends registered;
- **second test:** the day-2 chain lombard-broderick → marina-broderick → marina-webster-beach →
  lombard-laguna → columbus-north-point, solved as one chain through two registered sites.

**Arms:** native rate, the 5 Hz export (`-keep-every-rotation 2`), and native poses decimated
without re-running SLAM. The stated σ is checked by odd/even rotation splits.

## Scope

Items 3–7 are Gerardus work; their titles are proposed entries for Gerardus's backlog.

### Item 1: export trust

**Summary:** Tighten the azimuth-clock gates and make every rotation's time convention explicit.

**Steps:**

1. Gate a rotation's clock on period deviation (≤ 0.5% of a running median of 5), boundary step
   (≤ 100 µs), kept-block residual (≤ 500 µs RMS), unwrapped span (≤ 370°) and rejected share
   (≤ 25% of blocks); never fall back silently to packet time.
2. Write the fit's intercept and slope, the time source used, and a fit-based pose reference
   time per rotation; record the tail motor speed and the calibration tables' hash in `run.json`.
3. Interpolate a rotation whose fit fails from its neighbours and label its time source; never
   export it with packet times unlabelled.
4. Cap a frame at 0.5 s of point time, and refuse a sector capture by name.
5. Write the time convention into the LAS files as a VLR, since a clear time-type bit reads as GPS
   week time.

**Milestone:** v0.6.0. Continue if at least 99.5% of rotations are trusted and the fitted rate
matches the tail motor speed.

**Evidence so far:**

- After `7cfe581`, all 28,050 rotations of three drives passed the clock fit, with a median
  residual of 176–188 µs (see Current state).
- `866b9d4` has since added two of this item's gates (the rejected-share cap at 10% and the
  kept-RMS limit at 500 µs) and recorded per-join grades in `run.json`. The drives need
  re-exporting under them.
- Re-exported at `-azimuth-stride 32` under the 10% cap, the three drives lost 118 of 28,050
  rotations to it. The robust pass rejects up to 18.6% of blocks to bunched arrival (99.9th
  percentile 12.8%), and the kept blocks still sit under 330 µs RMS at normal periods. With the cap
  at 25% and the span capped at 370°, only lombard-laguna frames 93 and 145 are refused, both by
  the kept residual (10 and 18 ms RMS). The frame builder keeps up to 1804 blocks (360.6°) at the
  seam, so the span cap sits above a full turn.
- The period-deviation and boundary-step gates are not built yet, so the 99.5% criterion is not
  yet tested.

### Item 2: pose-free stillness and lift labels

**Summary:** Per-rotation labels computed from the rotations alone, with no pose:

- **still**, when at least 0.8 of the range cells match 1.0 s earlier;
- the **sensor height** above the ground fit, and the share of near returns;
- **ground, lift, roof-parked or driving**, from those two.

Stop intervals and the handover intervals come from these labels, not from the trajectory.

**Milestone:** v0.6.0. Continue if it marks the ground and roof-parked intervals at both ends of
the test drive and no rotation in obvious motion.

### Item 3: import and anchors (Gerardus)

**Summary:** Read LAS, `run.json` and `frames.csv`; refuse LAS without its manifest; write
`anchors.json` from the registration records, with registered ends floored at 0.05 m and 0.05°.

**Milestone:** v0.6.0. Stop if a record's frame cannot be related to EPSG:7131.

### Item 4: scan-to-map odometry (Gerardus)

**Summary:** Point-to-plane registration of each rotation against a keyframe-rebuilt local map,
with twist de-skew from point time. It runs between the lifts only, on ground-band points plus
structure at least 2.5 m up, with three protections:

- a self mask for the roof, straps and tripod legs (2.5 m minimum range plus a learned mask);
- a co-moving mask: under 5 cm residual in the sensor frame but over 20 cm in the world frame;
- a redescending cut at 0.30 m.

A voxel-hash map is built only if this runs slower than ten times real time.

**Milestone:** v0.6.0. Rethink if under 95% of drive-regime rotations register.

### Item 5: second pass and reverse run (Gerardus)

**Summary:** A second pass with centred de-skew, and a time-reversed run as a drift diagnostic.

**Milestone:** v0.6.0. Continue if pass 2 lowers residuals in turns.

### Item 6: roof-to-site handover (Gerardus)

**Summary:** Register each end's roof-parked interval to the site's accepted scan, and close the
triangle against a direct 3DEP registration; pass or fail per end, with the anchor factor.

**Milestone:** v0.6.0.

### Item 7: bridge solve (Gerardus)

**Summary:** A keyframe graph solved A-only, B-only and A+B, with marginals from one inverse. Its
odometry covariance is per metre, per radian and per second, rotated into site axes, and
calibrated on submap-to-submap innovations. It writes the odometry and solved pose sets in the v0
schema, with the noise parameters. Anchors use `max(10 × formal, 0.05 m)` and 0.05° for registered
ends. Manually placed ends enter with the along-street direction projected out, and serve only as
alias detectors.

**Milestone:** v0.6.0. Kill if the open-loop residual exceeds 2% of the moving path in both
directions.

### Item 8: ego-track independence

**Summary:** Make `lidar-ego-track` honest about what it cannot see.

**Steps:**

1. Along-track σ from three terms: white noise, a random walk per metre and a scale bias. Take
   stops from the exporter's labels, and state the gap rule in seconds (0.35 s).
2. Give jerk its own window, ±0.65 s × `(σ_pos / 1 cm)^(2/11)`. Report it only above 3σ, with the
   under-read factor for short onsets.
3. Measure distance over moving samples only. Report kinematics at the sensor and at a
   road-reference point below it, which removes the pitch lever arm.
4. Report a spectrum peak only above 1.5 times the floor, and correct only where the fit's gain is
   at least 0.5 (1.3 Hz to Nyquist at ±0.5 s).

**Milestone:** v0.6.0. Continue if the odd/even empirical-to-stated σ ratio is in [0.75, 1.33].

### Item 9: hydra evaluation 1

**Summary:** Run the development drives, the test drive and the day-2 chain at the three arms, and
write the authored report with summary tables only. The first export of [route-drive-bridge-first-pair](../../data/experiments/try/route-drive-bridge-first-pair.md)
is its opening step.

**Milestone:** v0.6.0. Gate 1 below.

## Gate 1

Continue to phase 2 if, on the untuned test drive, all of these hold:

- both roof-to-site handovers pass, with triangle closures within 0.2 m;
- the held-out end, in both directions, has a Mahalanobis² in [0.115, 11.3] (both χ²(3) bounds),
  an along-street error of at most 1.85 m (half a lot period) and a yaw error of at most 1°;
- odometry drift over pose-free still intervals is at most 0.01 m/s median and 0.03 m/s worst,
  with no still interval missed;
- the odd/even empirical-to-stated σ ratio is in [0.75, 1.33];
- at least 99% of rotations pass the clock, and at least 98% of driving rotations register.

**Change course**, not kill, if stops show traffic bias (strengthen the co-moving mask first).
**Kill** the bridge approach if a handover fails at a registered site for a reason the labels
cannot explain, or if the held-out residual exceeds 2% of the path in both directions and an
outside KISS-ICP run agrees.

## Dependencies

- #1056 merged, and the archive captures on hydra's local volume.
- Gerardus's accepted registration records for the three drives' sites.
- The owner's decisions below, before the gates are read as criteria.

## Risks

| Risk                                                            | Likelihood | Impact | Mitigation                                                                                          |
| --------------------------------------------------------------- | ---------- | ------ | --------------------------------------------------------------------------------------------------- |
| The ground-to-roof step at a site is mis-registered             | Medium     | High   | Item 6 registers it at each end; a failed end loses only its anchor                                 |
| Strap or tripod-leg modes shake the sensor within a sweep       | Medium     | Medium | Phase 2's per-sector ground-tilt spectrum measures them; strap settling at first braking is flagged |
| The two oracles' gates differ in ways the reconciliation missed | Low        | Medium | The comparison lists each run's value beside the reconciled one                                     |
| Along-street motion is unobservable on some blocks              | Medium     | High   | Report the along-street eigenvalue share per rotation on drive one                                  |
| Traffic biases odometry at stops                                | Medium     | Medium | Compare still-interval drift with and without dense traffic                                         |
| The random-walk term `q` is unknown, so σ is wrong              | High       | Medium | Measure it from still intervals before reading σ                                                    |
| The archive is not 10 Hz throughout                             | Low        | Low    | Item 1 records the tail motor speed and fitted rate per drive                                       |
| Site bounds follow file rotation, not when the vehicle moved    | High       | Low    | The labels, not the site index, define the drive and its handovers                                  |
| A drive believed continuous crosses an unseen restart           | Low        | Medium | The exporter's join grading refuses it                                                              |

## Owner decisions

All taken by the owner on 2026-11-17, at the reconciled defaults from the
[comparison](../research/lidar-route-slam-oracle-comparison.md):

- How did the sensor ride between sites? **Answered by the owner:** on its tripod, ratchet-strapped
  to a car roof, wherever the capture is continuous.
- **Phase boundary:** the ego track is gated here, and priors are in phase 2.
- **Test drives:** lombard-laguna → columbus-north-point is the never-tuned test, and the day-2
  chain is the second test.
- **Manual ends:** manually placed ends serve only as alias detectors, never for held-out claims.
- **Gates:** the gates and defaults in this plan are pinned for evaluation 1.
- **Trajectory contract:** velocity leaves it, so only velocity.report differentiates. The
  [route plan's contract](lidar-route-capture-plan.md#trajectory-contract) is amended to match.
- **Reference points:** kinematics are reported at the sensor and at a road-reference point.
- **Field notes:** future drives record the tripod's height on the roof and the strap layout.

## Checklist

### Complete

- [x] First-pass export and ego-track tools (#1056)
- [x] Oracle runs A and B and their comparison
- [x] Gates and owner decisions pinned at the reconciled defaults (2026-11-17)

### Outstanding

- [ ] Item 1: export trust (`S`)
- [ ] Item 2: pose-free stillness (`M`)
- [ ] Items 3–7: Gerardus import, odometry, handover and bridge solve (`M`)
- [ ] Item 8: ego-track independence (`M`)
- [ ] Item 9: hydra evaluation 1 and its report (`S`)

### Deferred

- [ ] A third oracle run: not needed unless the owner disputes a reconciled gate
