# Route SLAM phase 2: priors along the route and the rotation-rate question (v0.6.0)

Add prior factors from USGS 3DEP and DataSF to the phase 1 bridge, so the drive is registered
along its whole route, and settle with a native 20 Hz capture whether any platform should run the
sensor at 20 Hz. Phase 2 starts only if phase 1's gate passes.

- **Status:** Proposed. The items and gates are oracle run A's proposals
  ([response](../research/lidar-route-slam-oracle-response-fable.md)); they come from one oracle,
  with no cross-check run, and none is adopted until the owner pins it. Waits on gate 1 of
  [phase 1](lidar-route-slam-phase1-bridge-ego-plan.md)
- **Decisions:** D-36 (Gerardus owns registration; v0.6.0–v0.6.4 is the SLAM/mobile series), D-38
  (priors are measured spatial data)
- **Layers:** Gerardus submaps, prior factors and pose graph; offline tooling over L1 and L2
  (`internal/lidar/motionexport`), `internal/lidar/egotrack`, the trajectory contract
- **Target:** v0.6.0 for priors on the archive; the 20 Hz capture and its comparison follow the
  owner's capture decision
- **Companion plans:** [lidar-route-slam-minimum-plan](lidar-route-slam-minimum-plan.md) (umbrella),
  [lidar-route-slam-phase1-bridge-ego-plan](lidar-route-slam-phase1-bridge-ego-plan.md),
  [lidar-route-capture-plan](lidar-route-capture-plan.md)
- **Canonical:** [motion-capture.md](../lidar/operations/motion-capture.md)
- **Rate audit:** [20 Hz pipeline readiness](../research/lidar-20hz-readiness.md)

## Motivation

Phase 1 pins a drive only at its ends. Mid-route it can be metres out along the street, which is
harmless to the ego track's derivatives and useless for placing observations on named road
segments. Prior factors pin the trajectory to measured public geometry along the way. They are
weakest exactly where it matters: along a straight street, where every lot looks like the next.
Phase 1's measured drift model is what lets the solve tell a real along-street correction from an
alias one lot period away.

The rate question needs its own evidence. The archive can only drop rotations, which doubles
the motion between scans but keeps each sweep at 100 ms. Whether 50 ms sweeps help a bike or a
walker, and what a 20 Hz rotation does to registration noise, only a native capture can show.

## Current state

- **Gerardus:** `evaluate(refine)` runs the static gates against a site bundle. Three gaps:
  - Hypothesis separation is a generic runner-up cost ratio between coarse peaks at least 4 m or
    20° apart. No lot period exists in the code, so a 3.7 m alias can be merged before it is
    scored.
  - `evaluate(refine)` computes no separation.
  - The facade gates are skipped silently under 100 facade points.
- **Rate:** the Pandar40P runs at 10 or 20 Hz with the same points per second. The repository's
  only 20 Hz capture is a 60–290° sector capture that never completes a rotation. No real-data
  statement about the per-rotation clock at 20 Hz exists.
- **Trajectory contract:** the route plan's contract lists velocity; run A recommends removing it
  so only velocity.report differentiates.

## Findings

| Area                    | Current state                                                             | Severity | Item   |
| ----------------------- | ------------------------------------------------------------------------- | -------- | ------ |
| Along-street aliases    | No lot-period check on a factor before it enters the graph                | High     | 11     |
| Gates at the wrong pose | Static gates are evaluated at the fit's own pose, not the solved one      | Medium   | 12     |
| Silent facade skip      | Under 100 facade points the gate neither passes nor fails                 | Medium   | 10, 12 |
| Vibration band          | Per-rotation poses see 0–5 Hz at 10 Hz; mount vibration extends higher    | Medium   | 13     |
| Clock offset            | The azimuth clock is late by about 250 µs plus an unmeasured latency      | Medium   | 14     |
| Rate evidence           | Only a temporal ablation exists; no native 20 Hz full-rotation capture    | High     | 16     |
| Trajectory schema       | v0 carries one pose set and no noise model or regime                      | Low      | 15     |
| Compute                 | A keyframe-rebuilt map may be too slow at scale; unmeasured until phase 1 | Low      | 17     |

## Design / approach

**Priors.** Submaps are cut every 50 m, or at 30° of turn. Each is registered against the bundle
with the existing gates. A factor keeps only the directions whose eigenvalue share is at least
`1e-2`, and along-street information is counted from points at least 2 m up. Before a factor
enters the graph it passes an alias guard: its innovation against the phase 1 prediction must be
within `max(3σ, 0.5 m)`, capped at 1.85 m, and its cost must beat the alternatives at ±3.7 m and
±7.6 m by a ratio of at least 1.25. The gates are then re-read at the solved pose, with a submap
of under 100 facade points reported as ungated. Leave-one-out residuals, each factor predicted
by a graph solved without it, are the independent check.

**Rate.** One configuration, fixed in seconds and metres, runs at both rates. Every rotation is
registered on its own against the local map; pairs are not merged. Run A's expectation, to be
tested and not assumed: a car gains little at 20 Hz once de-skew is centred, while a bike or a
walker gains a fourfold cut in rotational residue (`α T² R`). Its provisional recommendation is
car at 10 Hz, and bike and walking surveys at 20 Hz. Traffic measurement stays at 10 Hz until the
audit's qualification passes.

**The 20 Hz capture.** The first pair's route, driven three times on one day with one mount, at
600, 1,200 and 600 rpm:

- single return, full 360° field of view, confirmed from the tail motor speed and azimuth
  coverage, with sensor settings exported into the provenance;
- at least 60 s static at each registered end;
- three stops of 10 s, one firm brake and two ordinary turns per drive.

The two 10 Hz drives measure drive-to-drive spread, without which no rate difference can be read.
**Arms:** N20 (native), D10 (the 20 Hz capture keeping one rotation in two) and N10. N20 against
D10 isolates pose rate. D10 against N10 isolates sweep length and column density, read against
the N10 spread.

## Scope

Items 10–12 and 17 are Gerardus work; their titles are proposed entries for Gerardus's backlog.
Numbering continues from phase 1.

### Item 10: submaps and prior factors (Gerardus)

**Summary:** Per-submap fits, gates and factors with weak directions projected out.

**Milestone:** v0.6.0. Change course to feature anchors if there is under one full-rank factor per
500 m.

### Item 11: alias guard and robust re-solve (Gerardus)

**Summary:** Accept or drop each factor with a recorded reason, then re-solve with a robust kernel.

**Milestone:** v0.6.0. Stop if any accepted factor sits a lot period from its leave-one-out
prediction.

### Item 12: independent residuals (Gerardus)

**Summary:** Leave-one-out factor residuals, gates at the solved pose, and the covariance scale
compared with phase 1's.

**Milestone:** v0.6.0. Continue if the scale is within [0.5, 2] of phase 1's.

### Item 13: sub-rotation heave

**Summary:** Heave and tilt spectra to 40 Hz from per-sector ground fits within a rotation, using
the exported point times, so vibration above the per-rotation band is measured, not aliased.

**Milestone:** v0.6.1. Drop if road roughness swamps it at stops.

### Item 14: packet sidecar

**Summary:** A `packets.csv` per export with rotation index, UDP sequence, arrival time, first
block azimuth and the sensor's own timestamp, and the residual of native time against the azimuth
clock. This is evidence for the clock's real offset and lets a later run re-time points without
the captures.

**Milestone:** v0.6.1.

### Item 15: trajectory v1

**Summary:** A schema with two pose sets (odometry and solved), the noise model and a regime
label, read by a file-backed `PoseProvider` that refuses gaps. Velocity leaves the contract if the
owner agrees.

**Milestone:** v0.6.0. Hold until Gerardus's proto is drafted.

### Item 16: native 20 Hz capture

**Summary:** The 10–20–10 session described above, as three bundles with settings in provenance.
The sensor's power draw and the mount's response at 1,200 rpm are measured before a long session.

**Milestone:** after gate 1, on the owner's decision. Void if the tail motor speed or azimuth
coverage is not as planned.

### Item 17: voxel-hash map (Gerardus, conditional)

**Summary:** A map with running moments, built only if phase 1's odometry runs slower than ten
times real time.

**Milestone:** unscheduled.

### Item 18: hydra evaluation 2

**Summary:** Priors on every bridge drive, and the N20, D10 and N10 arms, with an authored report
of summary tables only.

**Milestone:** v0.6.0 for the archive part; the rate arms follow item 16.

## Gate 2

**Priors.** Continue the series if all of these hold:

- at least 80% of gated submaps pass at the solved pose;
- at least one accepted full-rank factor per 250 m over 80% of each route;
- pooled leave-one-out residuals have a median of at most 0.15 m and 90% within 0.30 m, from at
  least 20 samples;
- the held-out registered end is within 0.3 m and 0.3°;
- no accepted factor is a lot period off.

If along-street factors are too few, change course to feature anchors at intersections.

**Rate.** Adopt 20 Hz for a platform only if N20 beats both D10 and N10 on that platform's
binding measure by more than the spread between the two 10 Hz drives. Otherwise keep 10 Hz and
record what 20 Hz did show.

## Dependencies

- Gate 1 of phase 1, and its measured drift model.
- Site bundles for every cell a drive crosses, not only the end sites.
- The owner's decision on the 20 Hz capture, a vehicle and mount for one day, and the motor
  setting changed on the sensor between drives.
- The frame builder completing rotations on real 20 Hz data, which nothing has shown yet.

## Risks

| Risk                                                          | Likelihood | Impact | Mitigation                                                          |
| ------------------------------------------------------------- | ---------- | ------ | ------------------------------------------------------------------- |
| Too few along-street factors on straight blocks               | High       | Medium | Expected; the gate counts them per 250 m and names the fallback     |
| A lot-period alias enters the graph                           | Medium     | High   | Item 11's guard and item 12's leave-one-out residuals               |
| The two 10 Hz drives differ more than the rates do            | Medium     | Medium | That is a result: the gate then keeps 10 Hz                         |
| 1,200 rpm changes mount behaviour or power draw               | Low        | Medium | Measure both before a long session                                  |
| The 20 Hz capture is sector-limited or dual return by mistake | Low        | High   | Item 1's export guard refuses it; settings are recorded and checked |
| Gates were set by one oracle and never cross-checked          | High       | Medium | Owner pins them before evaluation 2                                 |

## Owner decisions

- Capture a native 20 Hz session, and when? Run A's default: one 10–20–10 session on the
  registered pair's route, after gate 1 passes.
- Admit the columbus-north-point → embarcadero-bay → columbus-broadway chain (two registered ends
  with a static site between) as a second test? Default: yes.
- Run KISS-ICP's Python release on hydra as an outside cross-check? Default: only if gate 1's
  residual is poor.

## Checklist

### Outstanding

- [ ] Items 10–12: Gerardus prior factors, alias guard and independent residuals (`M`)
- [ ] Item 13: sub-rotation heave (`M`)
- [ ] Item 14: packet sidecar (`S`)
- [ ] Item 15: trajectory v1 and `PoseProvider` reader (`S`)
- [ ] Item 16: native 20 Hz capture (`M`)
- [ ] Item 18: hydra evaluation 2 and its report (`M`)

### Deferred

- [ ] Item 17: voxel-hash map, only if phase 1's odometry is too slow
- [ ] A two-pose sweep model (CT-ICP style), only if 10 Hz with centred de-skew fails the facade
      gate in turns
