# External-reference campaign: protocol and site feasibility

This protocol fixes, before any external pass is driven, what an RTK probe-vehicle campaign must
measure, how a pass qualifies as a reference, which passes may tune and which are held out, and
what result counts as feasible. It is the predeclaration that D-29's proposal needs before money
or field time is spent on it.

- **Status:** Proposed; predeclared November 5, 2026; nine of its eleven questions answered
  November 6, 2026 ([D-40](../DECISIONS.md#d-40--external-reference-campaign-answers)), the
  receiver (Q4) and the control-target survey (Q10) left open. No field work done: the
  fix-availability measurement and the site survey have not started, and no receiver has been
  bought or borrowed
- **Target:** v0.5.3 for the protocol and the feasibility measurement; the scripted passes and
  their scores are the v0.5.9 field-validation row ([backlog](../BACKLOG.md))
- **Layers:** L5 Tracks, L8 Analytics, annotation and offline evaluation, capture provenance
- **Canonical:** [point annotation tool](../lidar/operations/point-annotation-tool.md#scoring-against-physical-references) (the scorer this campaign reuses)
- **Related:** [physical-reference plan](lidar-physical-reference-review-plan.md#proposed-external-reference-rtk-probe-vehicle), [D-29](../DECISIONS.md#d-29--research-briefing-decisions-october-12-2026), [October 12 briefing, Q1](../lidar/operations/brief/research-briefing-2026-10-12.md#decisions), [cohort isolation](lidar-qualification-cohort-isolation-plan.md), [portable timing](../lidar/architecture/portable-capture-timing.md), [GPS ingest](../lidar/architecture/gps-ethernet-parsing.md), [geometry convergence levels](../lidar/operations/geometry-convergence-criteria.md), [kirk0 pilot](../lidar/operations/physical-reference-pilot-kirk0-2026-10.md), [instrumented-vehicle experiment](../research/2026-10-29-approach-stability-and-ttc.md#proposed-real-world-experiment-fixed-roadside-lidar-vs-instrumented-vehicle)

## 1. Purpose

Every physical reference the project holds today is a bounded fit to the sensor's own returns.
It can show a tracker drifting from its points, but not a bias the points and the tracker share.
A probe vehicle carrying an RTK GNSS receiver, driven past a sensor whose pose is surveyed, gives
a trajectory measured by something other than the LiDAR. The
[state estimation plan](lidar-state-estimation-plan.md) says plainly that there is no instrumented
probe vehicle, no survey and no external reference trajectory.

### What a completed campaign establishes

- How far L5's speed, body-centre path and following gap are from an external trajectory, for the
  probe vehicle, on the lanes, directions, speeds, gaps and ranges driven, at the surveyed site,
  within the reference envelope of section 6.
- Whether the operator-fitted references' bounds cover the probe's truth when an operator fits
  the probe vehicle blind: a check on the reference workflow itself, not only on the tracker.
- Whether a residential street can supply RTK-fixed passes at an affordable rate (section 10),
  which is a useful result even when the answer is no.

### What it does not establish

- Accuracy for any class the probe is not: trucks, vans, cyclists and pedestrians stay unscored.
  The physical-reference plan's rule holds: claim only the classes, views, ranges and support
  the references represent.
- Accuracy at another site. A development-cohort site cannot support a qualification claim
  (section 8); that needs passes at a qualification-cohort site, scored under isolated scoring.
- Naturalistic behaviour. Scripted passes are driven to a script; they measure the instrument,
  not how people drive.
- The sensor's absolute range accuracy. The sensor pose is fitted to control points seen in its
  own returns, so a range-scale error common to targets and vehicles is partly absorbed by the
  fit. Agreement is agreement in the frame the control points define.
- Body yaw beyond what the probe's course over ground supports, any behaviour threshold, and any
  L3 or L4 recall figure beyond the probe's own detection.
- Anything about the pipeline's code, the metric registry or its definitions, which D-29 leaves
  unchanged. Section 9 records the one place where scoring speed conflicts with that wording.

### The rules in five lines

1. No purchase and no scripted pass until the fix-availability measurement on the street says
   feasible (section 4.3) and the site survey closes its budget (section 3).
2. A pass is a reference only if the probe held an RTK fixed solution from window entry to exit
   and its computed envelope is inside the ceilings on every scored instant; the tracker's
   result never decides it.
3. Capture time is instrumented, never fitted: the LiDAR is GNSS-locked, the comparison instant
   is the native acquisition time of the probe's returns, and the clock is checked before and
   after every session.
4. Populations are assigned by lot, by session, before the first session, and the pilot site is a
   development-cohort site whose held-out passes support development claims only.
5. Scoring uses the existing physical scorer with the probe recorded as `import:rtk_probe_v1`,
   origin independent, never pooled with operator fits.

## 2. System boundary

```text
            PROBE VEHICLE (own car, consenting driver)          SITE (surveyed once per mount)
 +-----------------------------------------------+   +-------------------------------------------+
 | RTK rover + antenna over body centre          |   | Control points C1..Cn (RTK, averaged)     |
 |   fix type, h-accuracy, velocity, speed acc.  |   | Base / timing receiver on a control point |
 |   GNSS-time epochs                            |   |   PPS + GPRMC ------------+               |
 | Tape-measured lever arm and body dimensions   |   |                           v               |
 +----------------------+------------------------+   | Pandar40P (native time GNSS-locked)       |
                        |                            |   UDP 2368 + GPS status 10110 -> Pi PCAP  |
                        | raw log, trimmed to the    +---------------------+---------------------+
                        | site geofence, kept local                        | PCAP (kept local)
                        v                                                  v
 +----------------------------------------------------------------------------------------------+
 | OFFLINE WORKSTATION (nothing below runs on the Pi or in the pipeline)                         |
 |  survey fit: control points -> sensor pose + leave-one-out residual                           |
 |  replay (stamped build, config/experiments arm) -> VRLOG -> annotation pack (membership review)|
 |  probe importer: geodetic -> pack frame, evaluated at native acquisition instants,            |
 |     envelope per instant -> physical-reference import (import:rtk_probe_v1, independent)     |
 |  population manifest (assigned before collection) + cohort manifest digest                    |
 |  lidar-ground-truth-eval perframe -physical-reference  -> scores by population and stratum    |
 +----------------------------------------------------------------------------------------------+
```

The boundary that matters runs between reference and tracker. Nothing the tracker
produces flows into the survey fit, the clock chain, the envelope or the pass acceptance. The
probe's trajectory flows into exactly one place, the import, and from there only to the scorer.

## 3. Surveyed site

### 3.1 Site selection

A candidate street must satisfy all of these before the feasibility measurement:

| Requirement      | Test                                                                                                                |
| ---------------- | ------------------------------------------------------------------------------------------------------------------- |
| Legal loop       | A route that passes the sensor in both directions on public roads at legal speeds, without U-turns at the site      |
| Usual mount      | The sensor at its deployed height, about 2.3 m ([hardware](../../.github/knowledge/hardware.md#mounting))           |
| Control access   | Places for temporary control targets on public footway or with the frontager's permission, clear of the carriageway |
| Sky view         | Recorded as a sky-view photograph series at the antenna positions: the street's trees and buildings are the risk    |
| Cohort fit       | The site's S2 cell and cohort role are known (section 8); a site already in the development cohort is preferred     |
| Stationary marks | A safe place to park the probe on a surveyed mark at the start and end of each session                              |

**Decided (D-40):** the pilot is Kirkham at 9th (`kirkham-9th`), the kirk0 reference site,
assigned to the development cohort in the cohort manifest's first revision. If it comes out
infeasible, one more street is measured under the same rule before the campaign is dropped.

### 3.2 Sensor position and orientation

The sensor pose is estimated from control points, not assumed from the mount:

1. **Control points.** Place at least four temporary targets, each a flat retroreflective panel
   or a pole the returns resolve, spread in range and azimuth across the scoring window. Three
   non-collinear points determine a rigid transform; the fourth makes every fit leave one out.
   Each target's position is the mean of RTK fixed epochs over a dwell, with its spread recorded.
2. **Sensor position.** The ground mark below the sensor is surveyed the same way, and the
   vertical offset to the sensor-frame origin, as the Pandar40P manual defines it, is measured by
   tape. This is a check on the fit, not a substitute for it.
3. **Fit.** Locate each target's centre in static frames of the capture (settled background, not
   foreground, not the tracker), then fit the sensor-to-world transform to the surveyed
   positions. Record yaw, pitch, roll and position with their residuals.
4. **Uncertainty.** The stated sensor-pose uncertainty is the larger of the fit's residual and
   the worst leave-one-out prediction error at a held-out target. Its effect on a reference
   position at range R is the position term plus R times the orientation error; section 6
   evaluates it at the farthest scored range.
5. **Scoring window.** The scoring window lies within the range and azimuth span of the control
   points. The transform is not extrapolated beyond its support.

Re-survey whenever the mount is touched, the sensor is removed, or the start and end stationary
checks of a session disagree with the survey beyond their allocation. The survey record (target
positions, dwell spreads, fit, residuals, transform version) is the `transform_version` the import
states in its `PhysicalSource`.

## 4. RTK requirements

### 4.1 Receiver and corrections

The rover must report, per epoch: fix type, horizontal accuracy estimate, velocity and its speed
accuracy estimate, correction age, satellites used, and its GNSS time tag. A receiver that does not
expose these cannot be accepted, whatever its datasheet says. The
[spatial priors review](spatial-priors-service-review.md#concrete-product-families-to-evaluate)
names candidate families; none is chosen here.

Corrections must not send the probe's position off the site. A network-RTK virtual reference
station asks the rover to upload its position to a caster, which is vehicle data leaving the
device ([Tenet 1](../../TENETS.md#1-privacy-above-all)). In order of preference: a local base on a
surveyed control point, which also supplies the timing reference of section 5; post-processing
against a base log, which downloads and uploads nothing in the field; network RTK only from a
fixed mount point that needs no position upload, and only if the owner accepts it.

**Decided (D-40):** post-processing. The probe logs raw dual-frequency observations at 10 Hz or
faster, and is corrected afterwards against a public base station's logs at 1 s or finer from a
station within 10 km; nothing is uploaded, and network RTK is not used. A post-processed solution
gives no live fix status, so a pass's fix is known only after processing; the feasibility
measurement is post-processed the same way. Section 5's site receiver for the sensor's PPS and
GPRMC is still needed, since post-processing supplies no clock at the sensor.

### 4.2 Fix-type acceptance

| Solution                     | Typical horizontal accuracy in the repository's GPS design | Accepted                                                     |
| ---------------------------- | ---------------------------------------------------------- | ------------------------------------------------------------ |
| RTK fixed (GGA quality 4)    | 1 to 5 cm                                                  | Yes, if the epoch's own accuracy estimate is inside the term |
| RTK float (GGA quality 5)    | 10 to 50 cm                                                | No: above the 0.22 m centre ceiling on its own               |
| DGPS or standalone (2 and 1) | 0.5 to 10 m                                                | No                                                           |

The accuracy ranges are the [GPS ingest design's](../lidar/architecture/gps-ethernet-parsing.md#position-precision-requirements);
the epoch's own reported estimate, not the table, enters the envelope. A pass is accepted only if
the fixed solution held continuously from before the probe entered the scoring window until after
it left: a reacquisition inside the window can carry a wrong integer fix and excludes the pass.

### 4.3 Fix-availability measurement and its rule

Cellina et al. had both vehicles fixed at once for 2.3% of a motorway record
([briefing](../lidar/operations/brief/research-briefing-2026-10-12.md#decisions)). A static sensor
needs only the probe's fix, but trees and buildings can still cut it, so availability is measured
on the street, with the corrections mode the campaign will use, before anything is bought.

**Instrument.** A borrowed or rented RTK rover. If it is the model the campaign would buy, its
result stands. If it is a better surveying receiver, its result is an upper bound: a pass rules
nothing in until the purchased receiver repeats the measurement in its first session.

**Procedure.**

1. Mount the antenna on the probe's roof at the planned position, in the planned corrections mode.
2. Drive the loop at the middle scripted speed level, both directions, until at least 20
   drive-throughs per lane-direction have been recorded; the LiDAR need not run.
3. For each drive-through, mark whether it held RTK fixed continuously across the planned scoring
   window, judged from the receiver log alone.
4. Report the per-drive-through share with its Wilson 95% interval, per lane-direction, and the
   epoch-level fixed share inside the window beside it, which is the figure Cellina reports.

**Rule.** The site is feasible for a lane-direction when the interval's lower bound is at least
0.5, which at 20 drive-throughs needs 15 or more fixed. Half is the share at which reaching the
section 7 counts costs at most twice the attempts. The owner kept this rule (D-40); it may change
only before the measurement, never after it. Section 10 gives the outcome for each result.

| Fixed of 20 | Wilson 95% interval | Outcome                                        |
| ----------: | ------------------- | ---------------------------------------------- |
|    15 to 20 | lower bound ≥ 0.53  | Feasible                                       |
|     6 to 14 | straddles 0.5       | Insufficient: one extension to 40 (section 10) |
|      0 to 5 | upper bound ≤ 0.47  | Infeasible                                     |

## 5. Clock calibration

### 5.1 What must be aligned

The probe's epochs carry GNSS time. A pack sample carries the frame's timestamp, which in replay is
the PCAP time ([portable timing](../lidar/architecture/portable-capture-timing.md#what-the-repository-does-today):
the PCAP override takes precedence over every parser mode). The probe must be evaluated at the
instant the LiDAR actually acquired the probe's returns, and that instant is neither.

At 10 Hz (kirk0's pack is 832 samples over 83 s) one rotation is 100 ms, so a vehicle's returns can
be acquired up to 100 ms from its frame's timestamp: 1.1 m at 11.2 m/s (25 mph), five times the
centre ceiling. Frame-level time is therefore not acceptable, whatever the clock.

### 5.2 Method

**Chosen: GNSS-locked native LiDAR time.** A GNSS receiver at the site, the local base where there
is one, feeds PPS and GPRMC to the Pandar40P's GPS input, as the
[portable timing design](../lidar/architecture/portable-capture-timing.md#gnss-baseline-label-the-same-edge-at-both-sensors)
proposes. Native packet time is then UTC from the same GNSS time scale as the probe. The capture
records the sensor's GPS status packets on port 10110 beside the LiDAR packets.

For each sample, the importer takes the mean native acquisition time of the packets that carry the
probe's reviewed returns, decoded from the raw packet date and microsecond fields rather than
through the parser's `gps` mode, which the timing design shows mishandles them. It evaluates the
probe there and states the result at the sample's instant, as the reference record requires. The
spread of the probe's returns across the sweep is a property of the observation, not of the
reference: it is reported per pass for stratification, not added to the envelope.

**Rejected for scored passes:**

| Alternative                             | Why it is not accepted                                                                                                                                                                |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Host-arrival PCAP time, Pi clock on PPS | The constant part of acquisition-to-arrival latency is unidentifiable by minimum-delay methods (timing design); acceptable only with an independent end-to-end measurement            |
| Host-arrival PCAP time, Pi clock on NTP | The same, plus network clock error                                                                                                                                                    |
| Fitting the offset against the tracker  | It fits away the spatial error the campaign exists to measure ([instrumented-vehicle experiment](../research/2026-10-29-approach-stability-and-ttc.md#phase-a--low-cost-phone-pilot)) |

### 5.3 Residual bound

A residual time error δt moves the reference along the track by v δt. The timing design proposes a
5 cm timing-only geometry allowance
([motion gate](../lidar/architecture/portable-capture-timing.md#derive-the-timing-gate-from-motion)),
so the bound is |δt| ≤ 0.05 m / v_max: 4.5 ms at 11.2 m/s. Speed is barely affected: a constant
offset cancels on steady passes, and the matched-instant error is about |a| δt, under 0.005 m/s
at 1 m/s² of gentle acceleration.

| Contribution                                 | Expected size                                                 | Basis                                      |
| -------------------------------------------- | ------------------------------------------------------------- | ------------------------------------------ |
| LiDAR native time against the GNSS edge      | Tens of microseconds while locked                             | Timing design allocation, 15 µs            |
| Block and channel time not yet reconstructed | Up to about 0.5 ms                                            | Timing design: nine block separations      |
| Probe epoch time tag                         | Receiver measurement epoch; latency of output does not matter | Receiver log                               |
| Interpolation between probe epochs           | Position term, not time: at most a_max Δt² / 8                | Linear interpolation bound                 |
| Whole-second or leap-second error            | An invalid state, never a small residual                      | Timing design: loss, resets and provenance |

The contributions sum to under 1 ms against the 4.5 ms bound, if the lock holds. That margin is
why the method is instrumented rather than fitted.

### 5.4 Checks before and after each session

| Check                                         | Pass rule                                                                                                                           |
| --------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| LiDAR GPS lock, from the 10110 status packets | Locked at start, at end, and throughout every scored pass; a pass during holdover or unlock is excluded, with no holdover allowance |
| Native UTC second against the probe's UTC     | Equal at start and end; a whole-second disagreement fails the session                                                               |
| Native minus arrival time, 60 s still window  | Reported at start and end; a change between them describes the host clock and is a diagnostic, since arrival time is not used       |
| Stationary marks                              | The probe parked on a surveyed mark at start and end: its RTK position within the RTK term of the mark                              |
| Raw-return crossing (diagnostic only)         | The probe's predicted and observed raw-return crossing of a control line, reported; never used to adjust the clock                  |

A session that fails a pass rule contributes no scored pass. Its recordings are kept and the
reason recorded, as reference-side exclusions.

## 6. Reference envelope

### 6.1 Body reference point and lever arm

Mount the antenna over the body centre's plan position, on the roof centre line, so the horizontal
lever arm is near zero and heading error scarcely enters the centre. Measure the residual lever
arm from the antenna reference point to the body centre in body axes, x forward, y left, z up,
the record's [axis convention](../../internal/lidar/annotation/physical.go), by tape, twice, by
two people. The antenna's phase-centre offset comes from its datasheet and is vertical.

Heading, where a lever arm or the gap's follower axis needs it, is the probe's course over ground
on straight passes at 2 m/s or more, the speed floor the axis score already uses
([G1](../lidar/operations/geometry-convergence-criteria.md#criteria)). Below that floor the yaw is
unknown and the instants are not scored for yaw or gap.

### 6.2 Vehicle dimensions from an independent source

The body's length, width and height are measured on the vehicle by tape between its extreme
points, mirrors excluded and stated, and compared with the manufacturer's published dimensions
for the model and year. The tape is the value; the published figure is a check, and a
disagreement beyond the tape's stated bound stops the campaign until it is explained. In the
reference record each dimension is `inferred`, span `full`, with `Support.External` naming the
measurement record. Height then becomes scorable, which it was nowhere in the kirk0 pilot.

### 6.3 Uncertainty budget

Bounds combine by linear, worst-case addition, the rule the reference record already uses
([physical.go](../../internal/lidar/annotation/physical.go)), so no independence is assumed.

**Ceilings.** A pass instant is scored only where its envelope is inside these:

| Component | Ceiling   | Justification                                                                                                                                                               |
| --------- | --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Centre    | ≤ 0.22 m  | A quarter of the operator fits' smallest mean centre bound, 0.89 m on kirk0; and below car 8's 0.25 m median centre error, the best case the probe must resolve             |
| Gap       | ≤ 0.29 m  | A quarter of the operator fits' smallest mean gap bound, 1.16 m on kirk0                                                                                                    |
| Speed     | ≤ 0.2 m/s | The timing design's 0.2 m/s timing-only speed allowance; half of the 0.39 m/s (1.40 km/h) tracker speed precision Cellina reports, the only tracker-against-RTK figure held |

The kirk0 figures are the [pilot's component table](../lidar/operations/physical-reference-pilot-kirk0-2026-10.md#every-component).
"An error inside the bound is zero" ([W0](../lidar/operations/geometry-convergence-criteria.md#declaration)),
so a reference no tighter than the operator fits would add no resolution.

**Decided (D-40):** the quarter stands as the ceiling, and a tenth is the goal: each report also
states the share of scored instants whose envelope is inside 0.089 m for the centre and 0.116 m for
a gap. The tenth is not a ceiling, because the allocation below sums to 0.20 m for the centre and
0.24 m for a gap; reaching it would need about 2 cm RTK, a sensor yaw survey under 0.05° and timing
under 2 ms.

**Proposed allocation for the centre**, per instant:

| Term                    | Bounded by                                                        | Proposed allocation |
| ----------------------- | ----------------------------------------------------------------- | ------------------: |
| RTK position            | The epoch's reported horizontal accuracy, fixed solutions only    |            ≤ 0.05 m |
| Sensor pose             | Section 3.2's uncertainty, evaluated at the farthest scored range |            ≤ 0.05 m |
| Timing                  | v × δt, section 5.3                                               |            ≤ 0.05 m |
| Lever arm and heading   | Tape bound plus horizontal lever arm × heading bound              |            ≤ 0.03 m |
| Interpolation           | a_max Δt² / 8 between probe epochs                                |            ≤ 0.01 m |
| Body centre from bumper | Half the length bound, only if the measured reference is a bumper |            ≤ 0.01 m |
| **Sum**                 |                                                                   |        **≤ 0.20 m** |

The RTK allocation is the upper end of the fixed range in section 4.2; the timing allocation is the
timing design's. The sensor-pose allocation binds hardest: at 30 m, 0.05 m allows under 0.1° of
yaw error, so the survey decides the farthest range that can be scored.

**Path.** The centre error is reported along and across the probe's track. Timing moves the
reference along the track only, so on straight passes the cross-track envelope is the sum without
the timing term, about 0.14 m. That resolves the corpus's lateral p99 at sites above it (0.107 to
0.684 m by site, [G10](../lidar/operations/geometry-convergence-criteria.md#evidence-so-far)), not
the lowest.

**Gap**, two probes. Each bumper carries the RTK, lever-arm, interpolation and half-length
terms, about 0.10 m; the two bumpers 0.20 m. Sensor pose drops out of a distance between two
bodies. Projecting onto the follower's axis adds the lateral offset times the heading bound: about
0.03 m for kirk0's 1.6 m offsets at 1°. The sum, about 0.24 m, is inside the 0.29 m ceiling.

**Gap, one probe (D-40).** The campaign has one probe, so a following pass pairs it with an
operator-fitted vehicle, as follower or leader. Its envelope is the probe bumper's terms (about
0.10 m) plus the fitted vehicle's own gap-end bound, which on kirk0 is near 1.16 m, so a
half-external gap is not held to the 0.29 m ceiling: it is scored against its own envelope and
reported in its own table, never pooled with a two-probe gap. It tests the probe's end of the gap
against something external; it cannot test the fitted end.

**Speed.** The epoch's reported speed accuracy, plus angular rate times lever arm (near zero on
straight passes with the antenna over the centre), plus |a| δt.

### 6.4 When a pass is inside the envelope

A pass is a **clean pass** when all of the following hold, judged from the reference side alone,
before any tracker output for it is read:

1. Its session passed every section 5.4 check.
2. The probe held RTK fixed across the scoring window, section 4.2.
3. Every instant in the window has a computed envelope inside the three ceilings; an instant
   outside them is unscored for that component and counted, not dropped.
4. It was not aborted (section 7.4).

A pass the tracker misses, fragments or swaps stays in its population as a missed or wrong
prediction. Selecting passes by tracker outcome would make the campaign measure its own filter.

## 7. Scripted passes

### 7.1 Cells

| Factor           | Levels                                                     | Why                                                                                                                                                                       |
| ---------------- | ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Speed            | The posted limit, and 5 and 10 mph below it; never above   | Timing error grows with speed (section 5.3); legal speeds only                                                                                                            |
| Lane-direction   | Near lane and far lane, each in its legal direction        | Visible faces and range differ by lane ([instrumented-vehicle experiment](../research/2026-10-29-approach-stability-and-ttc.md#phase-a--low-cost-phone-pilot))            |
| Time gap (pairs) | 2 s and 3 s, at the middle speed, probe and fitted vehicle | Covers kirk0's following pairs, about 1.9 to 3.5 s at 6 m/s; 2 s is a safety floor for the drivers, not a threshold ([behaviour plan](lidar-behaviour-analytics-plan.md)) |

That gives six single-probe cells and four following cells, which with one probe are half
external (section 6.3).

### 7.2 Counts

At least five clean passes per cell per population, the starting count the instrumented-vehicle
experiment sets, adjusted only before collection. With two populations at the pilot site (section 8) that is 60 clean single passes and 40 clean following passes. At the feasibility rule's 0.5
floor it can take up to twice as many attempts. All cells are kept (D-40). Every attempted pass is logged with its outcome, so coverage and reference-quality
exclusions are published beside the scores.

### 7.3 Order

Within a session, the order of speed and gap levels is drawn by lot from a seed written into the
population manifest before the session; lane-direction follows the loop. The nominal level is a
stratum label: the probe's measured speed is the reference. Randomising stops a slow drift over a
session, in temperature, light or traffic, from lining up with a speed level.

### 7.4 Conduct, safety and abort

- Legal speeds and the 2 s floor only; no hard braking or lateral manoeuvres on a public street.
  Braking and turning belong to a closed course, as the instrumented-vehicle experiment says.
- A second person keeps the pass log and calls passes; the driver only drives.
- A pass is aborted, and logged as aborted, whenever a pedestrian, cyclist or child is near the
  carriageway, or the driver departs from the script for any reason. Aborted passes are not
  rescheduled into the same cell within the session.
- Other traffic is not removed or avoided: a pass with other vehicles is recorded with an
  interference flag and stays in its population.

## 8. Disjoint evaluation populations

### 8.1 Roles

| Population    | Where                              | Unit of assignment | Used for                                                                                                   |
| ------------- | ---------------------------------- | ------------------ | ---------------------------------------------------------------------------------------------------------- |
| Tuning        | Pilot site, development cohort     | Session            | Pinning the error limits, reference precision, coverage and missing-opportunity limits; scorer gate checks |
| Dev held-out  | Pilot site, development cohort     | Session            | A development held-out read, once the scorer's held-out path exists; never a qualification claim           |
| Qualification | A qualification-cohort site, later | Site               | The blind claim, under authorised isolated scoring only                                                    |

The [cohort plan](lidar-qualification-cohort-isolation-plan.md#purpose) makes site and capture
session the minimum exclusion units and groups every session from one site under one role. The
pilot site's passes will tune the scorer's limits, so the pilot site is a development-cohort site
by construction, and its held-out sessions are object-disjoint within that cohort, as
[`split.go`](../../internal/lidar/annotation/split.go) freezes them. A qualification claim needs a
second campaign, same protocol, at a site the cohort manifest has assigned to qualification.

### 8.2 Assignment before collection

Before the first session, write and digest a population manifest listing every planned session
with its population, drawn by lot, the order seeds of section 7.3, and this protocol's digest. The
cohort manifest's protocol-digest field carries the same digest. A session not in the manifest is
not scored; one added later goes to tuning. Assignments are never moved after a session is driven.

### 8.3 Keeping the held-out sessions held out

- Held-out probe logs are imported only when their scoring run is authorised, and are kept apart
  from the tuning material until then. **Decided (D-40):** they go to a read-restricted folder on
  the NAS when collected, their SHA-256 written to the custody record, and no agent or tuning run
  reads that folder until the held-out scoring run is authorised.
- The physical scorer refuses a held-out split today (`ErrPhysicalHeldOut`), because its limits
  must be pinned on tuning data first. The tuning population is what pins them; the held-out read
  follows the held-out path's delivery, not this campaign's schedule.
- Qualification sessions wait on the cohort plan's step 4, storage-level isolation, which is not
  built: until then they are refused, not isolated.
- Every replayed arm runs from a checked-in parameter set in
  [`config/experiments/`](../../config/experiments/README.md), with its path and fingerprint in the
  report, and from a `make`-stamped build.

## 9. Scoring

Score with the existing physical scorer, `lidar-ground-truth-eval perframe -physical-reference`,
against frozen splits whose episodes are the passes. It already matches by source, instant, frame
and object, accounts for every expected instant, and reports the reference's bound beside each error
([scoring](../lidar/operations/point-annotation-tool.md#scoring-against-physical-references)).

**The probe as a distinct reference source.** Each pass becomes a physical-reference import
(`velocity.report/physical-reference-import`, version 1) for the probe object:

| Field                   | Probe value                                                                                        |
| ----------------------- | -------------------------------------------------------------------------------------------------- |
| `review.method`         | `import:rtk_probe_v1`, beside the operator fits' `fit:mask_v1` and `manual_box`                    |
| `review.origin`         | `independent`                                                                                      |
| Component status        | `inferred`, never `observed`: observed means the sensor saw it in named frames                     |
| `support.external`      | The trimmed probe log's digest, the survey record and the vehicle measurement record               |
| Keyframes               | Every sample in the scoring window, anchor `body_centre`, position bound = that instant's envelope |
| Following               | Leader decision, `along_follower_axis` gaps, two-probe passes only                                 |
| Uncertainty assumptions | The envelope budget's version and this protocol's digest                                           |

The probe's membership masks are reviewed like any other object's. A person checks the import
against the receiver log before it is reviewed; nobody reviews it against the tracker.

**Results** are reported by population, by cell, by range band and by visible face, with the
probe rows and any operator-fitted rows in separate tables, never pooled. A single-probe pass
behind an operator-fitted leader gives a half-external gap; it is reported apart from two-probe
gaps or not at all.

**Operator blind fit.** For a subset of tuning passes, an operator fits the probe vehicle from its
returns without the probe trajectory in view. Scoring those fits against the probe tells whether
the operator bounds hold. If the reference store cannot hold a second source for one object at one
sample, the blind fit is scored at its own revision before the probe is imported.

**Speed is not yet scorable.** The reference record (schema version 1) has no velocity, and the
scorer's components are centre, yaw, length, width, height, bumpers, box and gap. Speed, the
campaign's first quantity, needs either a versioned velocity bound in the reference record with a
speed component in the evaluation tool, or a separate offline comparison over the scorer's matched
pairs. Either touches evaluation code, not the pipeline.

**Decided (D-40):** D-29's "nothing in code" means pipeline code. Speed is scored by an
evaluation-only velocity bound in a new version of the reference record and a speed component in
the offline evaluator; the pipeline, the registry and every definition stay unchanged. That is an
engineering prerequisite in section 15.

## 10. Feasibility outcome states

The feasibility decision covers both the fix-availability rule (section 4.3) and the survey
(section 3): the sensor-pose term must close at the farthest range the site needs scored.

| State            | Means                                                                                                                                                                                      | Triggers                                                                                                                                                                                                                       |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Feasible**     | Fix rule passes for each lane-direction to be scored, and the survey's sensor-pose term is within its allocation at the scoring window's far edge                                          | A purchase request goes to the owner with the measurement attached; the purchased receiver repeats section 4.3 in its first session; then the population manifest is frozen and scripted passes begin                          |
| **Infeasible**   | Fix upper bound below 0.5, or the survey cannot close its term at any useful range                                                                                                         | No purchase. A dated report records the measurement; one more candidate street may be tried if the owner names it (Q1); otherwise the RTK item is parked with this result as its reason                                        |
| **Insufficient** | The fix interval straddles 0.5, or the measurement was compromised: fewer than 20 valid drive-throughs, a corrections outage, a receiver not configured as declared, the survey incomplete | One extension to 40 drive-throughs at the same site, with the same rule; still straddling, the street is recorded insufficient and treated as not feasible for purchase. The threshold is never relaxed after the data is seen |

A lane-direction can be feasible while the other is not; the campaign then scores only the
feasible one and says so. Every outcome is reported in `docs/`, with raw logs kept local.

## 11. Privacy

- The probe is the project's own car, driven by a consenting operator. No other road user is
  instrumented, identified or followed; the capture records other traffic exactly as an ordinary
  capture does, and nothing in this protocol adds to it.
- No camera, including for marking passes: the pass log and the stationary marks do that work.
- The probe log is trimmed to a geofence around the site before it leaves the operator's custody,
  so the route to and from the site is never stored with the campaign.
- Corrections follow section 4.1: nothing uploads the probe's position by default.
- Raw logs, PCAPs and score outputs stay outside Git; reports carry digests and summaries
  ([repository instructions](../../AGENTS.md#keep-experiment-findings-in-git-and-raw-outputs-local)).

## 12. Failure registry

| Component               | How it fails                                            | Detected by                                      | Recovery                                                                       |
| ----------------------- | ------------------------------------------------------- | ------------------------------------------------ | ------------------------------------------------------------------------------ |
| RTK fix                 | Float, loss, or a wrong integer fix after reacquisition | Fix type and continuity across the window        | Pass excluded on the reference side; counted in coverage                       |
| Corrections             | Base outage or stale corrections                        | Correction age in the log                        | Pass excluded; a session-long outage makes the session insufficient            |
| LiDAR time lock         | Unlock, holdover, wrong second                          | 10110 status packets; UTC second check           | Affected passes excluded; whole-second error fails the session                 |
| Sensor pose             | Mount knocked, survey drift                             | Stationary marks; leave-one-out residual         | Re-survey; passes since the last good check excluded                           |
| Lever arm or dimensions | Mis-measured, antenna moved                             | Two-person measurement; manufacturer cross-check | Campaign stops until resolved; affected sessions re-imported                   |
| Importer                | Wrong frame, wrong instant, frame-time used             | Raw-return crossing diagnostic; import review    | Fix and re-import; references revise, scores rerun under a new revision        |
| Population leakage      | Held-out log read during tuning                         | Custodian record; cohort guard where it applies  | The leaked sessions move to tuning, recorded; a new held-out session is driven |
| Safety                  | Vulnerable road user near the carriageway               | Observer                                         | Abort, log, continue only when clear                                           |

## 13. Decision record

**Decided.** An RTK probe vehicle, GNSS-locked LiDAR time, and a control-point survey form the
external reference, scored by the existing physical scorer as a distinct source, with feasibility
measured on the street before anything is bought.

**Why.** It is the only option here that measures the trajectory with an instrument other than
the LiDAR at centimetre level, and the static sensor needs only the probe's fix.

**Alternatives considered:**

| Alternative             | Why not chosen                                                                                                                     |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Phone GNSS on the probe | Metre-level; the instrumented-vehicle experiment's Phase A calls it a coarse comparator. Useful as a dry run of the logistics only |
| Fixed radar             | Independent of the LiDAR, but speed-only with its own geometry error; corroboration, as the timing design says, not a reference    |
| CAN-bus speed           | Parked by D-28 ([backlog](../BACKLOG.md)); speed only, and a second vehicle interface                                              |
| Total station tracking  | Needs sight lines and a person per instrument; better reserved for checking control points if RTK cannot                           |
| Do nothing              | Keeps every physical gate a fit scored against fits, which G1's own note calls necessary, not sufficient                           |

## 14. Still to do in the field

None of this has been done.

- [x] Choose the candidate street and confirm its cohort role: Kirkham at 9th, development (D-40).
- [ ] Borrow or rent the feasibility receiver (Q4, open); corrections are post-processed (D-40).
- [ ] Permission for, and survey of, the temporary control targets (Q10, open).
- [ ] Sky-view photographs at the planned antenna positions.
- [ ] Fix-availability measurement: 20 drive-throughs per lane-direction, report and outcome.
- [ ] Place and survey the control points; survey the sensor mark.
- [ ] Fit the sensor pose and state its leave-one-out uncertainty and farthest scorable range.
- [ ] Confirm the Pandar40P accepts PPS and GPRMC from the site receiver and reports lock.
- [ ] Measure the probe's lever arm and dimensions, two people, against the published figures.
- [ ] Repeat the fix measurement with the purchased receiver, if the outcome is feasible.

## 15. Engineering prerequisites before the first scripted pass

- Confirm the pack sample timestamp's convention (frame start, end, or a reference azimuth) in
  `l2frames`, and how the importer finds the packets that carry an object's returns.
- The evaluation-only velocity bound in the reference record and the offline speed component
  (D-40).
- The report's goal line: the share of scored instants inside a tenth of the operator bounds.
- An offline decoder for native Hesai packet time, independent of the parser's `gps` mode.
- The probe importer: geodetic to pack frame through the survey transform, evaluation at native
  acquisition instants, per-instant envelope, import file output.
- The speed decision of section 9 and whatever it requires.
- Whether one pack can carry probe and operator references for the same object and sample.

## 16. Questions for the owner

Answered November 6, 2026 ([D-40](../DECISIONS.md#d-40--external-reference-campaign-answers)),
except Q4 and Q10.

| Id  | Question                                         | Answer                                                                                                        |
| --- | ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------- |
| Q1  | Pilot street and its cohort; a retry if it fails | Kirkham at 9th, development cohort; one more street is measured if it is infeasible                           |
| Q2  | One probe vehicle or two                         | One. Following passes pair it with an operator-fitted vehicle, scored and reported apart (section 6.3)        |
| Q3  | Corrections                                      | Post-processed against public base logs; no upload, no network RTK                                            |
| Q4  | Receiver for feasibility, and a purchase         | Open                                                                                                          |
| Q5  | The envelope ceilings                            | A quarter stands (centre 0.22 m, gap 0.29 m, speed 0.2 m/s); a tenth (0.089 m, 0.116 m) is reported as a goal |
| Q6  | The feasibility threshold                        | As proposed: Wilson 95 % lower bound at least 0.5 over 20 drive-throughs per lane-direction                   |
| Q7  | Speed scoring                                    | An evaluation-only velocity bound and offline speed component; pipeline unchanged                             |
| Q8  | The cells                                        | All kept: six single cells and four following cells, five clean passes each per population                    |
| Q9  | Correction age and output rate                   | Probe at 10 Hz or faster; base logs at 1 s or finer from a station within 10 km                               |
| Q10 | Control-target permission and survey             | Open                                                                                                          |
| Q11 | Custodian of the held-out logs                   | A read-restricted NAS folder, each log's SHA-256 in the custody record                                        |
