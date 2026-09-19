# Backpack capture: quasi-static stabilisation and survey walks (v0.6.x)

- **Status:** Draft
- **Layers:** LiDAR pipeline (L2 Frames, L3 Grid, L5 Tracks), `pcapsplit`, capture index, platform hardware
- **Target:** v0.6.x; capture protocol and segment classification changes belong with the scene capture workflow
- **Companion plans:** [static-sensor-nudge-tolerance-plan](static-sensor-nudge-tolerance-plan.md) is the tripod case this generalises; [motion-static-parameter-tuning-plan](motion-static-parameter-tuning-plan.md) owns the motion classifier sweep; [spatial-priors-service-review](spatial-priors-service-review.md) owns walk reconstruction and rig hardware findings; [lidar-motion-capture-architecture-plan](lidar-motion-capture-architecture-plan.md) is the vehicle-mounted case this is not
- **Canonical:** [motion-capture.md](../lidar/operations/motion-capture.md)

## Motivation

The survey rig today is a Hesai Pandar40P on a tripod, sometimes on a camping chair, standing at
a junction for about twenty minutes. The next capture mode is a person wearing the sensor: walk
the junction once with the sensor tilted to map its geometry, then stand still for twenty minutes
to measure traffic. Nothing in the pipeline tolerates that. The background grid assumes each ring
and azimuth bin watches the same world ray for the whole capture, and the motion classifier reads
continuous small motion as a drive between junctions. The Van Ness episode in the nudge plan lost
a whole site to three bumps of a tripod; a wearer produces the same disturbance continuously.

The consequence of inaction is that every backpack capture is discarded as motion, and the survey
walk, which is the cheapest way to feed the spatial priors service with street-level geometry,
has no home in the workflow.

This is not the moving-sensor problem the motion capture architecture plan describes. A wearer
standing at a corner is a quasi-static sensor with a noisy pose. The tracked objects are still
cars on a ground plane in a world-fixed frame, so the 4-state tracker, the classifier, and the
speed maths survive untouched once frames are stabilised. Backpack capture needs ego-pose, not
3D object orientation, and that is the scope cut that keeps it small.

The workflow stays record-then-process throughout. The field rig gathers PCAPs during the walk
and the stand, and every stage in this plan runs afterwards on a workstation. Nothing here adds a
live requirement to the Raspberry Pi.

## Current state

- **Background model.** `BackgroundGrid` in [internal/lidar/l3grid/background.go](../../internal/lidar/l3grid/background.go) is a polar range image of 40 rings by 1800 azimuth bins with per-cell EMA, spread, freeze, and locked baseline. The [grid standards comparison](../lidar/architecture/lidar-background-grid-standards.md) chose it because it has no pose dependence, which is exactly the property a moving sensor breaks.
- **Motion classifier.** `EvaluateSensorMotion` in [internal/lidar/l3grid/background_drift.go](../../internal/lidar/l3grid/background_drift.go) reads motion from a foreground fraction of 0.20 or a background drift ratio of 0.35. A motion-to-static transition needs 60 s of stability and a site needs ten static minutes. See [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md).
- **Timing.** Every point carries an acquisition timestamp with firetime correction (`Point.Timestamp` in [internal/lidar/l2frames/types.go](../../internal/lidar/l2frames/types.go)), so intra-frame motion correction is possible without new hardware.
- **Pose.** The runtime has no pose representation. The `pose_id` columns in the [static pose alignment](../lidar/operations/static-pose-alignment.md) design are deferred, and the [reflective sign anchor proposal](../../data/maths/proposals/20260310-reflective-sign-pose-anchor-maths.md) defines a `FrameStabilitySignal` that nothing produces yet.
- **Navigation sensors.** No IMU or GNSS position ingest exists. [gps-ethernet-parsing.md](../lidar/architecture/gps-ethernet-parsing.md) proposes NMEA over UDP captured into the same PCAP, which is the pattern this plan reuses for the IMU.
- **Archive.** [tools/s2-archive/site-index.json](../../tools/s2-archive/site-index.json) lists 24 tripod sites of roughly twenty minutes each, recorded as five-minute PCAP files, with operator-marked positions and azimuth readings. This is the control corpus for every experiment below.
- **Sensor geometry.** The Pandar40P covers +15.2° to -24.6° vertically, with a dense band from +2.4° to -5.7° at about 0.34° ring spacing and gaps of 1° to 6° outside it, per the embedded angle correction table in `internal/lidar/l1packets/parse/sensor_configs/`.
- **Compute evidence.** The committed perf baselines under `internal/lidar/perf/baseline/` were captured on an M1; there is no Raspberry Pi baseline, and the per-frame budget is 98 ms.
- **Priors review.** [spatial-priors-service-review.md](spatial-priors-service-review.md) §8 and §9 already conclude that wobbling captures must be reconstructed as metric submaps outside the live pipeline, name KISS-ICP as the LiDAR-only odometry baseline, and rank a synchronised IMU as the highest-value hardware addition.

## Findings

| Area                         | Current state                                                                                                 | Severity | Release view                       |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------- | -------- | ---------------------------------- |
| Background under rotation    | Any yaw beyond one bin or pitch beyond half a ring spacing invalidates cells; no correction path              | High     | v0.6.x                             |
| Motion classification        | Continuous small motion reads as a drive; a stabilised capture would read as static with no classifier change | High     | v0.6.x                             |
| Speed integrity during turns | A fast torso turn adds metres per second of apparent velocity to objects at range; nothing gates observations | High     | v0.6.x                             |
| Pose provenance              | No per-frame pose row, no capture mode, no pose source recorded                                               | Medium   | v0.6.x                             |
| Survey walk                  | No segment kind; a tilted walk is classified as motion and discarded                                          | Medium   | v0.6.x                             |
| IMU capture                  | No daemon, no packet format, no calibration path                                                              | Medium   | After the motion spectrum is known |
| Field compute                | No Pi baseline; live feedback claims are unsupported                                                          | Low      | v0.6.x                             |
| Sensor supply                | Pandar40P is a legacy model supplied used; alternatives meeting the 100 m rule were untracked                 | Low      | Continuous                         |

## Design / approach

### Two captures, two consumers

```text
site visit
|-- survey walk: sensor tilted about 45 degrees, 2 to 5 minutes around the junction
|     -> survey segment -> offline odometry (priors tooling) -> site submap -> spatial priors
|-- stand: sensor level, about 20 minutes at the chosen corner
      -> quasi-static segment -> stabiliser -> L3 to L6 -> traffic measurements
                                    -> reference pose registered to the submap
```

velocity.report owns the capture protocol, the PCAP bundle, segment classification, stabilisation
of the stand, provenance, and the export that hands the walk to the priors tooling. The spatial
priors project owns walk reconstruction, fusion with aerial and vector sources, and publication,
as its plan §4 already states. The live sensing binary never gains a SLAM dependency.

### Motion regimes

| Regime | Source                        | Expected size                          | Response                                 |
| ------ | ----------------------------- | -------------------------------------- | ---------------------------------------- |
| Micro  | breathing, postural sway      | sub-degree, centimetres, continuous    | Stabilise every frame                    |
| Meso   | weight shift, turning to look | 5° to 90°, step-like, every 10 to 60 s | Stabilise, gate the transition frames    |
| Macro  | walking to the next corner    | metres                                 | Existing motion segment; untouched       |
| Survey | deliberate tilted walk        | 45° tilt plus walking                  | New segment kind; handed to priors tools |

The sizes are expectations, not measurements. Item 2 measures them, and the IMU decision waits
on that measurement.

### Stabiliser

A stabilisation stage sits between frame assembly and the background grid, offline, in analysis
mode. It builds a keyframe map from the first settled seconds of the stand, then registers every
later frame to it with point-to-plane ICP on a downsampled set weighted towards ground, wall
planes, and high-intensity signs. This is the anchor ladder of the reflective sign proposal,
promoted from diagnostic to base case. The quasi-static prior keeps it cheap: the initial guess is
always the previous pose, and the overlap with the keyframe map is near total.

Per frame it outputs a rigid transform, a residual, and an inlier fraction. The transform corrects
the points; the residual and inlier fraction become the `FrameStabilitySignal`.

### The background grid under a corrected pose

| Geometry fact           | Value    | Consequence                                                             |
| ----------------------- | -------- | ----------------------------------------------------------------------- |
| Azimuth bin             | 0.2°     | Yaw correction is a circular shift of the azimuth index, exact to a bin |
| Dense-band ring spacing | 0.34°    | A 1° pitch change moves dense-band rays by three rings                  |
| Outer ring spacing      | 1° to 6° | Re-rendered points alias most in the sparse upper and lower rings       |

Two options, and the plan takes A first:

- **A. Re-render into the reference rays.** Project corrected points onto the nearest ring and
  azimuth of the reference pose. Every line of L3 stays as it is; only a projection step is added.
  The error is the elevation mismatch within a ring spacing, about 0.1 m at 20 m in the dense band,
  well inside the 0.5 m safety margin. Cells with no sample that frame simply do not update, which
  the grid already handles.
- **B. World-anchored grid engine.** A 2.5D grid anchored to the reference pose, selected like the
  L4 and L5 engines. Needed when pitch aliasing in A is shown to matter, or when a non-spinning
  sensor without rings enters the catalogue. The grid standards comparison rejected this for the
  fixed case precisely because it needs a pose; once a pose exists the objection lapses.

### Stability gate

- A frame whose residual or inlier fraction fails the gate updates nothing: no background
  learning, no track observation, and tracks coast.
- A speed observation is never emitted from an unstabilised frame. This is a tenet three rule,
  not a tuning choice.
- Gate thresholds are tuning parameters and are swept in the motion-static tuning plan's harness.

### Self-occlusion, deskew, and the visualiser

- The wearer occludes a fixed sector below the horizon. A static mask over those bins, captured
  once per rig, removes it before the grid sees it.
- Intra-frame deskew is deferred. A 100 ms sweep during a 30°/s turn smears 3°, but those frames
  are gated anyway. When it is needed, constant-velocity interpolation between frame poses uses
  the per-point timestamps that already exist; an IMU makes it exact.
- The FrameBundle gains a per-frame sensor pose so VelocityVisualiser can draw the sensor moving
  inside a fixed world. That view is the primary debugging tool for this work.

### Segment kinds and provenance

- The split tool keeps `motion` and `static`, evaluated on stabilised frames, and gains `survey`:
  a motion segment whose ground plane normal sits far from the sensor's spin axis. A 45° tilt is
  unmistakable from the ground fit, so the survey walk is detected without any operator marker.
- A capture carries `capture_mode` (`tripod` or `backpack`), `pose_source` (`none`, `lidar`, or
  `lidar+imu`), and the stabiliser's version and thresholds, in the segment metadata, the VRLOG
  header, and the run record, following the existing S2 conformance contract.
- Survey segments inherit the site's position tag, because they are part of the visit and stay
  within its cell. Walks between sites remain untagged motion, which also keeps the operator's
  route out of the archive.
- Every frame of a stabilised run has a pose row carrying its residual and inlier fraction,
  stored where the static pose alignment design put `pose_id`.

### Capture protocol

The protocol is what the operator does; the pipeline is built to that protocol rather than the
other way round.

1. Arrive, start capture, and stand still for ten seconds so the rig has a quiet start.
2. If an IMU is fitted, rotate the rig slowly through about 30° of yaw and back over ten seconds.
   This is the calibration wiggle the IMU time offset and mounting rotation are estimated from.
3. Tilt the sensor to the survey detent, about 45° from vertical, and walk the junction once:
   along each approach for a short distance, across each crossing, and back to the stand point.
   Two to five minutes, at walking pace, with no need to pause.
4. Return the sensor to the level detent, stand at the chosen corner, and stay for twenty minutes.
   Turning to look is fine; walking is not.
5. Stop capture, note the corner and the clock, and mark the position as the archive does today.

Why the tilt maps more: level, a side façade is only ever struck at the fixed ring elevations, and
walking does not change that. Tilted, the elevation at which a façade point is struck depends on
its bearing from the walker, so passing it sweeps the rings up and down it, and the forward rays
sweep the road surface at close range where the level sensor's dense band never reaches. The
angle is an experiment parameter; 45° is the starting value.

### The PCAP bundle

One capture is one set of PCAP files, as today, with the five-minute rotation kept. Extra sensors
join the same capture as UDP streams on the LiDAR interface:

| Stream | Packet source                                                                    | Port           | Parsed where                              |
| ------ | -------------------------------------------------------------------------------- | -------------- | ----------------------------------------- |
| LiDAR  | Pandar40P                                                                        | 2368           | L1, as today                              |
| IMU    | Daemon on the Pi reading the unit over I2C or serial, stamping with the Pi clock | one fixed port | L1 sidecar, like the proposed NMEA stream |
| GNSS   | Phone or USB receiver, NMEA over UDP                                             | 10110          | The GPS-over-Ethernet proposal            |

The Pi clock stamps both the PCAP arrival of LiDAR packets and the IMU samples, so both streams
share one time base without PPS wiring. The sensor's internal per-point timestamps are used only
for ordering within a frame. Millisecond jitter on that shared clock is a fraction of a bin at
the rotation rates a wearer produces.

### Hardware ladder

The sensor selection rule is fixed: at least 100 m range at 10% reflectivity, 360° horizontal
coverage from a multi-ring spinning unit, and Ethernet UDP output. A built-in IMU is a plus. The
Livox Mid-360 class is excluded by range. The working configuration is the Pandar40P plus an
external six-axis IMU, and the [LiDAR market watch](../platform/hardware/lidar-market-watch.md)
tracks alternatives weekly.

| Tier | Kit                                                   | Adds                                                                                                          | Who can run it                      |
| ---- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| 0    | Existing rig, backpack mast with two detents, battery | LiDAR-only stabilisation; works wherever there are walls and signs                                            | Anyone with the current kit         |
| 1    | + six-axis IMU on a rigid bracket                     | Rotation rate for deskew and a registration prior; tilt from gravity; a scene-independent gate for open roads | Makers, if calibration is automatic |
| 2    | + phone-grade GNSS                                    | S2 cell, map pin, seed for registration against the priors                                                    | Anyone                              |
| 2b   | + RTK or PPK GNSS                                     | Antenna position in open sky; poor in urban canyons; no heading                                               | Engineers                           |
| 3    | + Coral or GPU                                        | Nothing for registration, DBSCAN, or Kalman; only a learned detector would use it                             | Engineers                           |

Field compute stays a recorder. A Raspberry Pi 4 records the bundle comfortably. A live stability
meter, built from L3-only plus the registration residual, is the only reason to want more field
compute, and that claim needs the Pi baseline in Item 7 before it is made. Desk processing runs on
any laptop; a Mac mini is a desk option, not a field one, and no accelerator has a role.

## Scope

### Item 1: capture protocol and bundle conventions

**Summary:** Publish the field protocol, the mast detents, the file and metadata conventions, and
the archive index fields, so captures made before the software exists are usable by it.

**Steps:**

1. Add the protocol above to the getting-started guide and a one-page field checklist.
2. Extend the archive's map marks and site index with `capture_mode`, `survey_files`, and `tilt_deg`.
3. Reserve the IMU UDP port and document the tcpdump filter that captures all streams.
4. Record which mast and detent geometry the rig uses, with measured reference marks.

**Milestone:** v0.6.x

### Item 2: motion spectrum measurement

**Summary:** Measure, per frame, the rigid transform between a capture and its first settled
frames, for the Van Ness tripod episode and for the first backpack captures.

**Steps:**

1. Implement the per-frame rigid transform estimator as an offline `pcap-split` report, which is
   test A of the nudge tolerance plan.
2. Report amplitude and frequency of yaw, pitch, roll, and translation, and the settle time after
   each step.
3. Run part one of [backpack-motion-spectrum-and-stabiliser](../../data/experiments/try/backpack-motion-spectrum-and-stabiliser.md).

**Milestone:** v0.6.x

### Item 3: offline stabiliser and stability gate

**Summary:** Keyframe registration, re-rendering into reference rays, the gate, the self-occlusion
mask, and pose provenance, all in analysis mode.

**Steps:**

1. Keyframe map from the first settled seconds; point-to-plane registration with the anchor ladder weighting.
2. Re-render corrected points into the reference polar grid (option A).
3. Gate background updates and track observations on residual and inlier fraction.
4. Per-frame pose rows, `capture_mode` and `pose_source` in provenance, and the sensor pose in the FrameBundle.
5. Run part two of the experiment: A/B against the tripod archive at the same site.

**Milestone:** v0.6.x

### Item 4: survey segment kind and priors hand-off

**Summary:** Detect the tilted walk, tag it, and export it in a form the priors tooling reads.

**Steps:**

1. Ground-normal tilt detection in the split classifier; `survey` segment type.
2. Frame export with per-point timestamps in LAS 1.4 or per-frame PCD, whichever the priors
   experiment settles on.
3. Register the stand's reference pose to the walk submap and record the transform.
4. Run [survey-walk-geometry-gain](../../data/experiments/try/survey-walk-geometry-gain.md).

**Milestone:** v0.6.x

### Item 5: IMU capture and automatic calibration

**Summary:** A Pi daemon that broadcasts IMU samples as UDP, an L1 sidecar parser, and calibration
from the start wiggle. Gated on Item 2 showing that LiDAR-only stabilisation loses frames it
should keep.

**Steps:**

1. Choose a six-axis unit by noise and bias figures, per the priors review, not by rate alone.
2. Daemon, packet format, and replay parser; the PCAP remains the single artefact.
3. Mounting rotation from gravity against the LiDAR ground normal; time offset from yaw-rate
   cross-correlation over the wiggle.
4. Use the IMU as the registration prior and for deskew; measure the gain against Item 3.

**Milestone:** v0.6.x, after Item 2

### Item 6: LiDAR market watch

**Summary:** A weekly scheduled check of vendor announcements and used listings against the
sensor selection rule, appended to the market watch page.

**Steps:**

1. Publish the page with the rule, the entry format, and the first snapshot.
2. Run the weekly routine, which pushes each entry to the `claude/lidar-market-watch` branch.
3. Review monthly; promote any unit that meets the rule and a price ceiling into a purchase decision.

**Milestone:** Continuous

### Item 7: Raspberry Pi performance baseline

**Summary:** Capture `lidar-bench` baselines on the Pi 4 so every field-compute claim in this plan
rests on a measurement.

**Steps:**

1. Run the gated profiles on the Pi and commit the baselines with a platform suffix.
2. Record L3-only headroom, which bounds what a live stability meter may cost.

**Milestone:** v0.6.x

## Dependencies

- PR #569's capture index and multi-file continuous classification, which the segment and
  provenance changes extend.
- The nudge tolerance plan's test A is Item 2's first deliverable; the two plans share it.
- The spatial priors experiment in the priors review §8 consumes Item 4's export and decides the
  walk reconstruction tool.
- The sign anchor proposal supplies the anchor ladder and the stability signal contract.
- A backpack mast with two detents and a battery supply for the sensor, which are field kit
  rather than software.

## Risks

| Risk                                                                         | Likelihood | Impact | Mitigation                                                                             |
| ---------------------------------------------------------------------------- | ---------- | ------ | -------------------------------------------------------------------------------------- |
| Sway pitch exceeds what re-rendering tolerates and option A aliases badly    | Medium     | Medium | Item 2 measures it first; option B is the fallback                                     |
| Too few planar anchors at some corners, so LiDAR-only registration drifts    | Medium     | Medium | Gate and report; Item 5's IMU prior; treat as a site-suitability finding               |
| The wearer occludes the corner that matters                                  | Medium     | Low    | Mast above head height; the mask reports its sector so the operator can turn           |
| Backpack and tripod speed distributions differ for reasons other than motion | Medium     | High   | Same site, same hour, adjacent windows; report both with sample sizes before any claim |
| Survey walk output never reaches a priors product                            | Medium     | Medium | Item 4 exports a format the priors experiment already plans to ingest                  |
| IMU bought before the spectrum is measured                                   | Low        | Low    | Item 5 is gated on Item 2 by this plan                                                 |
| Market watch drifts into unverified prices                                   | Medium     | Low    | Entries carry URLs and dates; "not stated" is a valid value                            |

## Checklist

### Complete

- [x] Ideation and scope cut recorded in this plan.

### Outstanding

- [ ] Item 1: capture protocol, detents, bundle conventions, archive fields (`S`)
- [ ] Item 2: per-frame rigid transform report and motion spectrum experiment (`M`)
- [ ] Item 3: offline stabiliser, gate, mask, provenance, FrameBundle pose (`L`)
- [ ] Item 4: survey segment detection, export, submap registration (`M`)
- [ ] Item 5: IMU daemon, sidecar parser, automatic calibration (`M`)
- [ ] Item 6: market watch page and weekly routine (`S`)
- [ ] Item 7: Raspberry Pi perf baseline (`S`)

### Deferred

- [ ] Intra-frame deskew: gated frames make it unnecessary for the stand; revisit with Item 5.
- [ ] World-anchored grid engine (option B): only if option A aliasing is shown to matter or a ringless sensor is adopted.
- [ ] 3D object orientation and the 13-state tracker: not needed for quasi-static capture, tracked by [lidar-motion-capture-architecture-plan](lidar-motion-capture-architecture-plan.md).

### Accepted residuals (no action planned)

- [ ] Live stabilised tracking in the field: the workflow is record-then-process by design.
- [ ] RTK positioning: absolute placement comes from registration against the priors, seeded by phone-grade GNSS.
