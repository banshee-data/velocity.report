# Route SLAM phase 1: a two-anchor bridge and the ego track from archive drives (v0.6.0)

Build the sensor platform's own track first: odometry over an archive drive, tied to the accepted
static pose at each end, with no prior factors along the route. Phase 1 uses only captures that
already exist and ends in one evaluation on hydra that decides whether phase 2 starts.

- **Status:** Proposed. The items and gates are oracle run A's proposals
  ([response](../research/lidar-route-slam-oracle-response-fable.md)); they come from one oracle,
  with no cross-check run, and none is adopted until the owner pins it. Nothing here is built
  beyond the first-pass tools in #1056
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
track needs. Run A's derivation: a position error that varies along the route as `e(ℓ)` changes
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
  spectrum, end anchors). Both are tested on synthetic data only; neither has run on a
  full-rotation drive.
- **Gerardus:** robust point-to-plane ICP and the static gates (`src/register.rs`), a small pose
  graph (`src/graph.rs`). No odometry, de-skew, import or bridge solve.
- **Mount (owner, after run A):** for every ride between sites where the capture is continuous,
  the sensor stayed on its tripod, ratchet-strapped to the roof of a car. It was not carried.
  The tripod still moved from the ground to the roof and back at each site. So a site's
  accepted pose (tripod on the ground) is not the drive's first pose (tripod on the roof). The
  handover has to measure that step, not just check that the pose held. Oracle run B received
  this fact; run A did not.

## Findings

Run A's review of the first-pass tools. Each is a reason for an item below.

| Area               | Current state                                                                                        | Severity | Item |
| ------------------ | ---------------------------------------------------------------------------------------------------- | -------- | ---- |
| Stop check         | Stops come from the trajectory's own speed, so the check selects the intervals it then passes        | High     | 2, 8 |
| Stated σ           | Assumes independent pose noise and defaults to 0.02 m, which alone passes "median σ under 0.1 m/s"   | High     | 8    |
| Anchor handover    | An accepted pose describes the first 300 s of its static capture; nothing shows the mount held after | High     | 6    |
| Clock fallback     | A refused rotation is still exported with packet times, so one segment can mix two time conventions  | Medium   | 1    |
| Rotation timestamp | `frames.csv` stamps each rotation with its arrival-based start, which keeps the jitter               | Medium   | 1    |
| Frame growth       | With timeouts off, a capture that never completes a rotation grows one frame without limit           | Medium   | 1    |
| Gap rule           | Three median intervals: it halves in seconds at 20 Hz                                                | Low      | 8    |
| Path length        | Summing raw poses grows about 0.18 m/s at rest at 10 Hz                                              | Low      | 8    |
| Spectrum           | Always reports a peak and corrects down to a gain of 0.1                                             | Low      | 8    |
| Graph marginals    | `graph.rs` inverts a (6N−6)-square matrix per node and has no robust kernel                          | Medium   | 7    |
| ICP reuse          | `icp` is concrete over `Reference`/`Scan`; odometry needs a small trait                              | Low      | 4    |

## Design / approach

```text
archive pcaps -> lidar-motion-export     LAS, run.json, frames.csv with trust and stillness   (items 1, 2)
              -> Gerardus import          session + anchors from registration records          (item 3)
              -> odometry, two passes     per-rotation poses, quality, regime label            (items 4, 5)
              -> handover check           each end's margin scan against its accepted pose     (item 6)
              -> bridge solve             A-only, B-only, A+B; odometry and solved pose sets   (item 7)
              -> lidar-ego-track          kinematics from odometry poses, σ, stops, spectrum   (item 8)
              -> hydra evaluation 1       three drives, three rate arms, authored report       (item 9)
```

Three rules make the checks independent:

1. **Stillness is measured without a pose.** The exporter marks a rotation still when at least 0.8
   of its range cells (rings at or above the horizon, range over 5 m, 0.4° bins) are within
   0.03 m of the same cells 1.0 s earlier. Stop intervals come from that column, not from the
   trajectory.
2. **Each anchor is checked at the handover.** The static margin at each end is registered against
   the site's accepted pose (tolerance 0.10 m and 0.5°). A failed end removes that anchor and the
   drive's held-out claim.
3. **Derivatives come from odometry poses, and σ from a noise model that includes drift.**
   Linearly interpolated graph corrections would put a jerk step of about 0.4 m/s³ at every
   keyframe. σ uses white noise plus a random walk through the fit (initially σ_w = 0.01 m and
   q = 1e-3 m²/s, both to be measured at still intervals).

Everything is fixed in seconds and metres, so one configuration serves both rotation rates. The
keyframe spacing is 10 m or 10°, map insertion happens after 0.5 m or 2°, the local map radius is
100 m, and the speed and acceleration window is ±0.5 s with jerk on its own ±0.75 s window.

**Drives.** Develop on marina-webster-beach → lombard-laguna and columbus-broadway →
van-ness-sacramento (one registered end each). Run once, untuned, on lombard-laguna →
columbus-north-point (both ends registered). **Arms:** native rate, the 5 Hz export
(`-keep-every-rotation 2`), and native poses decimated without re-running SLAM.

## Scope

Items 3–7 are Gerardus work; their titles are proposed entries for Gerardus's backlog.

### Item 1: export trust

**Summary:** Tighten the azimuth-clock gates and make every rotation's time convention explicit.

**Steps:**

1. Gate a rotation's clock on period deviation (≤ 0.5% of a running median of 5), boundary step
   (≤ 100 µs), kept-block residual (≤ 500 µs RMS) and rejected share (≤ 10% of blocks); never fall
   back silently to packet time.
2. Write the fit's intercept and slope, the time source used, and a fit-based pose reference
   time per rotation; record the tail motor speed and the calibration tables' hash in `run.json`.
3. Cap a frame at 0.5 s of point time, and refuse a sector capture by name.

**Milestone:** v0.6.0. Continue if at least 99.5% of rotations are trusted and the fitted rate
matches the tail motor speed.

### Item 2: pose-free stillness

**Summary:** A `still` column per rotation and a list of still intervals, from range cells alone.

**Milestone:** v0.6.0. Continue if it marks both static margins and no rotation in obvious motion.

### Item 3: import and anchors (Gerardus)

**Summary:** Read LAS, `run.json` and `frames.csv`; refuse LAS without its manifest; write
`anchors.json` from the registration records, with registered ends floored at 0.05 m and 0.05°.

**Milestone:** v0.6.0. Stop if a record's frame cannot be related to EPSG:7131.

### Item 4: scan-to-map odometry (Gerardus)

**Summary:** Point-to-plane registration of each rotation against a keyframe-rebuilt local map,
with twist de-skew from point time, on ground-band points plus structure at least 2.5 m up.

**Milestone:** v0.6.0. Rethink if under 95% of drive-regime rotations register.

### Item 5: second pass and reverse run (Gerardus)

**Summary:** A second pass with centred de-skew, and a time-reversed run as a drift diagnostic.

**Milestone:** v0.6.0. Continue if pass 2 lowers residuals in turns.

### Item 6: handover check (Gerardus)

**Summary:** Register each end's static margin against the accepted pose; pass or fail per end.

**Milestone:** v0.6.0.

### Item 7: bridge solve (Gerardus)

**Summary:** A keyframe graph with per-metre odometry covariance, solved A-only, B-only and A+B,
with marginals from one inverse; write the odometry and solved pose sets in the v0 schema.

**Milestone:** v0.6.0. Kill if the open-loop residual exceeds 2% of the moving path in both
directions.

### Item 8: ego-track independence

**Summary:** Make `lidar-ego-track` honest about what it cannot see.

**Steps:**

1. σ from white noise plus a random walk through the fit; take stops from the exporter's still
   intervals; state the gap rule in seconds (0.35 s).
2. Give jerk its own window (±0.75 s) and report the under-read factor for onsets shorter than
   0.7 s; measure distance over moving samples only.
3. Report a spectrum peak only above 1.5 times the floor, and correct only where the fit's gain is
   at least 0.5 (1.3 Hz to Nyquist at ±0.5 s).

**Milestone:** v0.6.0. Continue if the empirical-to-stated σ ratio at stops is in [0.7, 1.4].

### Item 9: hydra evaluation 1

**Summary:** Run the three drives and three arms, and write the authored report with summary
tables only. The first export of [route-drive-bridge-first-pair](../../data/experiments/try/route-drive-bridge-first-pair.md)
is its opening step.

**Milestone:** v0.6.0. Gate 1 below.

## Gate 1

Continue to phase 2 if, on the untuned test drive, all of these hold:

- both handovers pass, or the failing end is explained and replaced;
- the open-loop residual at the held-out end is at most 0.5% of the moving path and 1°, in both
  directions, with the normalised innovation in [0.115, 11.3];
- odometry drift over pose-free still intervals is at most 0.01 m/s median and 0.03 m/s worst,
  with no still interval missed;
- the empirical-to-stated σ ratio is in [0.7, 1.4];
- at least 98% of drive-regime rotations register.

**Change course**, not kill, if stops show traffic bias (remove dynamic points first) or carried
stretches break the chain (restrict to driven stretches). **Kill** the bridge approach if the
residual exceeds 2% in both directions and an outside KISS-ICP run agrees.

## Dependencies

- #1056 merged, and the archive captures on hydra's local volume.
- Gerardus's accepted registration records for the three drives' sites.
- The owner's decisions below, before the gates are read as criteria.

## Risks

| Risk                                                         | Likelihood | Impact | Mitigation                                                           |
| ------------------------------------------------------------ | ---------- | ------ | -------------------------------------------------------------------- |
| The ground-to-roof step at a site is mis-registered          | Medium     | High   | Item 6 registers it at each end; a failed end loses only its anchor  |
| Gates were set by one oracle and never cross-checked         | High       | Medium | Owner pins them before evaluation 1; a second run can still be asked |
| Along-street motion is unobservable on some blocks           | Medium     | High   | Report the along-street eigenvalue share per rotation on drive one   |
| Traffic biases odometry at stops                             | Medium     | Medium | Compare still-interval drift with and without dense traffic          |
| The random-walk term `q` is unknown, so σ is wrong           | High       | Medium | Measure it from still intervals before reading σ                     |
| The archive is not 10 Hz throughout                          | Low        | Low    | Item 1 records the tail motor speed and fitted rate per drive        |
| Site bounds follow file rotation, not when the vehicle moved | High       | Low    | The stillness column, not the site index, defines the drive          |

## Owner decisions

Each with run A's default; none is taken yet.

- How did the sensor ride between sites? **Answered by the owner:** on its tripod, ratchet-strapped
  to a car roof, wherever the capture is continuous.
- Accept the phase boundary (ego track before priors)? Default: yes.
- Reserve lombard-laguna → columbus-north-point as a never-tuned test drive? Default: yes.
- Use manually placed ends for held-out claims? Default: no; use those drives to test the
  placements.
- Floor registered anchors at 0.05 m and 0.05° in the solve? Default: yes.
- Remove velocity from the trajectory contract, so only velocity.report differentiates?
  Default: yes.
- Report peak jerk with its under-read factor, or the acceleration step alone? Default: both.

## Checklist

### Complete

- [x] First-pass export and ego-track tools (#1056)
- [x] Oracle run A and its two-phase work order

### Outstanding

- [ ] Item 1: export trust (`S`)
- [ ] Item 2: pose-free stillness (`M`)
- [ ] Items 3–7: Gerardus import, odometry, handover and bridge solve (`M`)
- [ ] Item 8: ego-track independence (`M`)
- [ ] Item 9: hydra evaluation 1 and its report (`S`)

### Deferred

- [ ] A second oracle run and the question-by-question comparison: the owner chose a single run
