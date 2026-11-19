# Route SLAM phase 2: priors along the route and the rotation-rate question (v0.6.0)

Add prior factors from USGS 3DEP to the phase 1 bridge, so the drive is registered
along its whole route, and settle with a native 20 Hz capture whether any platform should run the
sensor at 20 Hz. Phase 2 starts only if phase 1's gate passes.

- **Status:** Proposed. The items and gates are the
  [reconciliation](../research/lidar-route-slam-oracle-comparison.md) of two independent oracle
  runs ([A](../research/lidar-route-slam-oracle-response-fable.md),
  [B](../research/lidar-route-slam-oracle-response-opus.md)). The owner pinned the gates and
  decisions at the reconciled defaults on 2026-11-17. Waits on gate 1 of
  [phase 1](lidar-route-slam-phase1-bridge-ego-plan.md). At the owner's request (2026-11-19),
  items 10a and 10–12 have a Gerardus first pass ahead of gate 1, in
  banshee-data/gerardus#90, with synthetic tests only. Item 10a's probe may run on hydra with
  evaluation 1. Item 10's factors are not read as evidence until gate 1 passes
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

| Area                    | Current state                                                                                       | Severity | Item   |
| ----------------------- | --------------------------------------------------------------------------------------------------- | -------- | ------ |
| Along-street aliases    | No lot-period check on a factor before it enters the graph                                          | High     | 11     |
| Gates at the wrong pose | Static gates are evaluated at the fit's own pose, not the solved one                                | Medium   | 12     |
| Silent facade skip      | Under 100 facade points the gate neither passes nor fails                                           | Medium   | 10, 12 |
| Vibration band          | Per-rotation poses see 0–5 Hz at 10 Hz; mount vibration extends higher                              | Medium   | 13     |
| Clock offset            | The azimuth clock is late by about 250 µs plus an unmeasured latency                                | Medium   | 14     |
| Rate evidence           | Only a temporal ablation exists; no native 20 Hz full-rotation capture                              | High     | 16     |
| Trajectory schema       | v0 carries one pose set and no noise model or regime                                                | Low      | 15     |
| Compute                 | A keyframe-rebuilt map may be too slow at scale; unmeasured until phase 1                           | Low      | 17     |
| Submap anchoring        | A submap from drifting odometry is stretched; a factor on its first keyframe carries half its drift | Medium   | 10     |
| Drift calibration       | Phase 1's submap innovations read about a third of synthetic drift of known size                    | Medium   | 12     |

## Design / approach

**Priors.** Submaps are cut every 50 m, or at 30° of turn, and are at most 150 m long. Each is
rebuilt from the export's raw points in the frame of its middle keyframe. A rigid fit centres
a drifting submap's stretch, so the factor lands on that keyframe. On a first keyframe it would
carry half the submap's drift: 0.23 m on a 1% over-read in Gerardus's test. Each submap is
registered against the bundle of the L16 cell under it with the existing gates. A factor keeps a direction only if its
scaled eigenvalue is at least `1e-2` of the largest, and still at least `1e-3` when recomputed
from points more than 2 m up. Before a factor enters the graph it passes an alias guard. The
along-street component is accepted directly only if the chain's predicted `σ_∥` is at most
0.6 m. Otherwise ICP is re-run from the chain pose shifted by ±3.7 m and ±7.6 m, and the factor
must win by a separation of at least 1.25. In both cases its innovation must be within
`max(3σ, 0.5 m)`, capped at 1.85 m. DataSF footprints are reported beside the 3DEP fits, not used
as factors, until their measured 1–1.4 m offsets from 3DEP are explained (run B). The gates are then re-read at the solved pose, with a submap
of under 100 facade points reported as ungated. Leave-one-out residuals, each factor predicted
by a graph solved without it, are the independent check.

**Rate.** One configuration, fixed in seconds and metres, runs at both rates. Every rotation is
registered on its own against the local map; pairs are not merged. Both oracle runs compute the
same turn residue: 0.225 m at 30 m at 10 Hz, and 0.056 m at 20 Hz. Run A expects the second,
centred de-skew pass to remove it at 10 Hz, so that a car gains little at 20 Hz. Run B does not,
and recommends 20 Hz for SLAM on every moving platform. Both expect a bike or a walker to gain
the fourfold cut in rotational residue (`α T² R`). Traffic measurement stays at 10 Hz until the
audit's qualification passes.

**When to capture at 20 Hz.** Capture only if the archive's priors pass (items 10–12) show the
failure is in turns, not along the street. Concretely: 10 Hz turn submaps fail the gates after the
second de-skew pass, while straight submaps pass. A higher rate cannot fix along-street
observability, because the prior gates do not depend on rate on straight submaps.

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

### Item 10a: facade probe (Gerardus)

**Summary:** `gerardus slam submaps`. It rebuilds the submaps from the raw points, prepares
the bundle of every cell they cross, and fits each against 3DEP from the bridge's pose. It
reports, per submap:

- the ground and facade fit against `Register`'s gates;
- the rank and weak directions;
- the correction against the bridge, along and across the street.

Nothing is fed back. This is the first registration of facades along a capture, and it tells
early whether straight streets constrain travel along them. It adds no gate.

**Milestone:** v0.6.0. Runs on hydra beside evaluation 1. First pass in
banshee-data/gerardus#90.

### Item 10: submaps and prior factors (Gerardus)

**Summary:** Per-submap fits, gates and factors with weak directions projected out. First
pass in banshee-data/gerardus#90 (`slam submaps --factors`). Each factor's covariance is
`10 ×` formal and at least 0.05 m per kept direction, a provisional choice that copies the
anchors' pinned floor. On the synthetic test street every factor keeps five directions, not
six: its ground swamps its cross-street walls under the 1e-2 share. Full-rank coverage may
read low on real streets for the same reason.

**Milestone:** v0.6.0. Change course to feature anchors if there is under one full-rank factor per
500 m.

### Item 11: alias guard and robust re-solve (Gerardus)

**Summary:** Accept or drop each factor with a recorded reason, then re-solve with a robust kernel.

**Milestone:** v0.6.0. Stop if any accepted factor sits a lot period from its leave-one-out
prediction.

### Item 12: independent residuals (Gerardus)

**Summary:** Leave-one-out factor residuals, gates at the solved pose, and the covariance scale
compared with phase 1's. This is also the independent check on phase 1's drift calibration.
On synthetic drift drawn at 1×, 4× and 16× the model, phase 1's submap innovations read about
0.37, 1.2 and 3.5, because each submap is fitted to the far end of the one before, which the
same odometry placed. Phase 1 therefore never applies a scale under 1, and this item's
leave-one-out scale is the number to believe.

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

**Milestone:** after gate 1 and the turn-submap test above, on the owner's decision. Void if the
tail motor speed or azimuth coverage is not as planned. The operator records the tripod's height
on the roof and the strap layout, and keeps a ground and a roof-parked interval at both ends.

### Item 17: voxel-hash map (Gerardus, conditional)

**Summary:** A map with running moments, built only if phase 1's odometry runs slower than ten
times real time.

**Milestone:** unscheduled.

### Item 18: hydra evaluation 2

**Summary:** Priors on every bridge drive, and the N20, D10 and N10 arms, with an authored report
of summary tables only.

**Milestone:** v0.6.0 for the archive part; the rate arms follow item 16.

### Item 19: field-of-view-relative rotation completion

**Summary:** Complete a rotation at `coverage ≥ FOV − 20°`, with the field of view taken from
configuration or learned from the first wraps. Finalise a frame as incomplete after twice the
tail-motor period. The builder already anticipates sector wraps (`azimuth_wrap_large_jump`) and
then gates them on 340°, which a sector cannot reach.

The runs differed on this. Run B called it a defect. Run A wanted it left alone because the
builder is shared with the live path. The reconciliation makes it a separate change outside the
spike, with recaptured performance baselines. The spike itself relies on the exporter's cap and
sector refusal from phase 1, item 1.

**Milestone:** unscheduled; P2.

## Gate 2

**Priors.** Continue the series if all of these hold:

- at least 80% of gated submaps pass at the solved pose;
- at least one accepted full-rank factor per 250 m over 80% of each route;
- pooled leave-one-out residuals have a median of at most 0.15 m and 90% within 0.30 m, from at
  least 20 samples;
- the held-out registered end is within 0.3 m and 0.3°, and its Mahalanobis² is in
  [0.115, 11.3];
- no accepted factor is a lot period off.

If along-street factors are too few, change course to feature anchors at intersections.

**Rate.** Adopt 20 Hz for a platform only if all of these hold:

- N20 beats both D10 and N10 on that platform's binding measure by more than the spread between the
  two 10 Hz drives;
- the two 10 Hz drives agree with each other better than either agrees with N20;
- the turn-submap pass rate at 20 Hz is at least the 10 Hz rate;
- per-metre drift is within 1.25 times the 10 Hz value;
- per-scan convergence is at least 99%;
- the speed σ ratio is within 0.8–1.25.

Otherwise keep 10 Hz and record what 20 Hz did show.

## Dependencies

- Gate 1 of phase 1, and its measured drift model.
- Site bundles for every cell a drive crosses, not only the end sites.
- The owner's decision on the 20 Hz capture, a vehicle and mount for one day, and the motor
  setting changed on the sensor between drives.
- The frame builder completing rotations on real 20 Hz data, which nothing has shown yet.

## Risks

| Risk                                                          | Likelihood | Impact | Mitigation                                                             |
| ------------------------------------------------------------- | ---------- | ------ | ---------------------------------------------------------------------- |
| Too few along-street factors on straight blocks               | High       | Medium | Expected; the gate counts them per 250 m and names the fallback        |
| A lot-period alias enters the graph                           | Medium     | High   | Item 11's guard and item 12's leave-one-out residuals                  |
| The two 10 Hz drives differ more than the rates do            | Medium     | Medium | That is a result: the gate then keeps 10 Hz                            |
| 1,200 rpm changes mount behaviour or power draw               | Low        | Medium | Measure both before a long session                                     |
| The 20 Hz capture is sector-limited or dual return by mistake | Low        | High   | Item 1's export guard refuses it; settings are recorded and checked    |
| The roof mount's strap or leg modes alias into the 10 Hz band | Medium     | Medium | Item 13's per-sector ground-tilt spectrum reaches about 20 Hz at 10 Hz |
| The reconciled gates hide a disagreement between the runs     | Low        | Medium | The comparison lists each run's value; the owner pins them             |

## Owner decisions

All taken by the owner on 2026-11-17, at the reconciled defaults:

- **20 Hz capture:** one 10–20–10 session on the registered pair's route, made after gate 1 and
  only if the turn-submap test calls for it.
- **Wider archive:** after gate 1, the 14 continuous drives beyond phase 1's set are used for
  odometry and submap gates. The columbus-north-point → embarcadero-bay → columbus-broadway chain
  is not used, because both legs cross a recorder restart.
- **Cross-check:** KISS-ICP's Python release runs on hydra only if gate 1's residual is poor.
- **Frame completion:** field-of-view-relative rotation completion (item 19) is a separate change,
  outside the spike.

## Checklist

### Outstanding

- [ ] Item 10a: Gerardus facade probe (`S`); first pass in banshee-data/gerardus#90, synthetic only
- [ ] Items 10–12: Gerardus prior factors, alias guard and independent residuals (`M`); first pass in
      banshee-data/gerardus#90, synthetic only
- [ ] Item 13: sub-rotation heave (`M`)
- [ ] Item 14: packet sidecar (`S`)
- [ ] Item 15: trajectory v1 and `PoseProvider` reader (`S`)
- [ ] Item 16: native 20 Hz capture (`M`)
- [ ] Item 18: hydra evaluation 2 and its report (`M`)
- [ ] Item 19: field-of-view-relative rotation completion, as a separate change (`S`)

### Deferred

- [ ] Item 17: voxel-hash map, only if phase 1's odometry is too slow
- [ ] A two-pose sweep model (CT-ICP style), only if 10 Hz with centred de-skew fails the facade
      gate in turns
