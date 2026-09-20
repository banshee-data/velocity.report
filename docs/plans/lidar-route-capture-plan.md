# Route capture: cargo bike and backpack rigs, road-segment speeds (v0.6.x)

- **Status:** Draft
- **Layers:** LiDAR pipeline (L1 sidecars, L2 Frames, L3 Grid, L5 Tracks, L7 Scene, L8 Analytics), `pcapsplit`, capture index, report, platform hardware
- **Target:** v0.6.x; capture protocol, segment classification, and the road model belong with the scene capture workflow
- **Companion plans:** [static-sensor-nudge-tolerance-plan](static-sensor-nudge-tolerance-plan.md) is the tripod case the stop regime generalises; [motion-static-parameter-tuning-plan](motion-static-parameter-tuning-plan.md) owns the motion classifier sweep; [spatial-priors-service-review](spatial-priors-service-review.md) owns route reconstruction and rig hardware findings; [lidar-motion-capture-architecture-plan](lidar-motion-capture-architecture-plan.md) is the 7DOF design this borrows ego-motion compensation from and leaves 3D orientation to; [lidar-l7-scene-plan](lidar-l7-scene-plan.md) owns road polygons and the scene graph; [speed-percentile-aggregation-alignment-plan](speed-percentile-aggregation-alignment-plan.md) owns the aggregate rules the segment outputs follow
- **Canonical:** [motion-capture.md](../lidar/operations/motion-capture.md)

## Motivation

Two deployment rigs are the goal: a cargo bike that covers routes, and a backpack pedestrian who
covers routes too but can stop at a corner for as long as twenty minutes. Both should produce the
same output: vehicle speeds along road segments, with particular attention to what happens on
either side of an intersection. That output does not exist today. The system aggregates by site,
a fixed point where a tripod stood, and the drives between sites are cut off as motion and never
used.

Nothing in the pipeline tolerates a moving sensor. The background grid assumes each ring and
azimuth bin watches the same world ray for the whole capture, and the motion classifier reads any
sustained movement as a drive to be discarded. The Van Ness episode in the nudge plan lost a whole
site to three bumps of a tripod; a wearer produces the same disturbance continuously, and a bike
never stops producing it.

The consequence of inaction is that neither rig can be used, and the survey walk, which is the
cheapest way to feed the spatial priors service with street-level geometry, has no home in the
workflow.

This is smaller than the motion capture architecture plan. That plan bundles ego-motion with 3D
object orientation and a 13-state tracker. Route capture needs ego-pose only: the tracked objects
stay cars on a ground plane in a world-fixed frame, so the 4-state tracker, the classifier, and
the speed maths survive once every frame carries a pose. The ego-motion compensation section of
that plan is reused as written; the orientation sections are not needed.

The workflow stays record-then-process throughout. The rig gathers PCAPs during the ride, the
walk, and the stops, and every stage in this plan runs afterwards on a workstation. Nothing here
adds a live requirement to the Raspberry Pi.

## Current state

- **Background model.** `BackgroundGrid` in [internal/lidar/l3grid/background.go](../../internal/lidar/l3grid/background.go) is a polar range image of 40 rings by 1800 azimuth bins with per-cell EMA, spread, freeze, and locked baseline. The [grid standards comparison](../lidar/architecture/lidar-background-grid-standards.md) chose it because it has no pose dependence, which is exactly the property a moving sensor breaks.
- **Motion classifier.** `EvaluateSensorMotion` in [internal/lidar/l3grid/background_drift.go](../../internal/lidar/l3grid/background_drift.go) reads motion from a foreground fraction of 0.20 or a background drift ratio of 0.35. A motion-to-static transition needs 60 s of stability and a site needs ten static minutes. See [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md).
- **Timing.** Every point carries an acquisition timestamp with firetime correction (`Point.Timestamp` in [internal/lidar/l2frames/types.go](../../internal/lidar/l2frames/types.go)), so intra-frame motion correction is possible without new hardware.
- **Pose.** The runtime has no pose representation. The `pose_id` columns in the [static pose alignment](../lidar/operations/static-pose-alignment.md) design are deferred, and the [reflective sign anchor proposal](../../data/maths/proposals/20260310-reflective-sign-pose-anchor-maths.md) defines a `FrameStabilitySignal` that nothing produces yet.
- **Navigation sensors.** No IMU or GNSS position ingest exists. [gps-ethernet-parsing.md](../lidar/architecture/gps-ethernet-parsing.md) proposes NMEA over UDP captured into the same PCAP, which is the pattern this plan reuses for the IMU.
- **Aggregation.** Speeds aggregate by site over per-transit maximum speeds, with aggregate-only percentiles, per [percentile-aggregation-semantics.md](../radar/architecture/percentile-aggregation-semantics.md). There is no road segment, intersection, or distance-along-road dimension anywhere in the schema, the API, or the report.
- **Road geometry.** The [vector scene map](../lidar/architecture/vector-scene-map.md) defines ground classes including `road` and `crosswalk` and proposes an OSM anchor import for crosswalk boundaries and stop lines; none of it is implemented. The [traffic description language](../platform/architecture/traffic-description-language.md) already asks intersection questions it cannot answer.
- **Archive.** [tools/s2-archive/site-index.json](../../tools/s2-archive/site-index.json) lists 24 tripod sites of roughly twenty minutes each across three days. The drives between them were recorded by the same sensor and are cut off by `pcap-split` as motion segments. Nobody consumes them, and they are ready-made route-mode test data from a smoother, faster platform than a bike.
- **Fixed radar.** The OPS243 radar path is production-ready and measures per-transit speeds at a fixed site. It is the independent reference a moving rig can be validated against.
- **Sensor geometry.** The Pandar40P covers +15.2° to -24.6° vertically, with a dense band from +2.4° to -5.7° at about 0.34° ring spacing and gaps of 1° to 6° outside it, per the embedded angle correction table in `internal/lidar/l1packets/parse/sensor_configs/`.
- **Compute evidence.** The committed perf baselines under `internal/lidar/perf/baseline/` were captured on an M1; there is no Raspberry Pi baseline, and the per-frame budget is 98 ms.
- **Priors review.** [spatial-priors-service-review.md](spatial-priors-service-review.md) §8 and §9 conclude that moving captures must be reconstructed as metric submaps outside the live pipeline, name KISS-ICP as the LiDAR-only odometry baseline and LIO-SAM class tools once a synchronised IMU exists, and rank that IMU as the highest-value hardware addition.
- **Hardware supply.** The [LiDAR market watch](../platform/hardware/lidar-market-watch.md) holds the selection rule and a first snapshot. On vendor figures the only unit seen under the budget that also meets 100 m at 10% reflectivity is a used Pandar40P.

## Findings

| Area                      | Current state                                                                                                | Severity | Release view |
| ------------------------- | ------------------------------------------------------------------------------------------------------------ | -------- | ------------ |
| Route regime              | No ego-motion path: no trajectory input, no world-frame foreground, no deskew; moving captures are discarded | High     | v0.6.x       |
| Background under rotation | Any yaw beyond one bin or pitch beyond half a ring spacing invalidates cells; stops cannot be stabilised     | High     | v0.6.x       |
| Speed integrity           | Ego velocity error adds directly to object speed; nothing gates observations on pose quality                 | High     | v0.6.x       |
| Road model                | No segments, intersections, or crossings; no way to say where along a road a speed was measured              | High     | v0.6.x       |
| Segment aggregates        | Site is the only aggregation key; no distance-from-intersection bands, no presence minutes, no sample gating | High     | v0.6.x       |
| Motion classification     | Continuous small motion reads as a drive; the stop regime needs stabilised frames before the classifier runs | Medium   | v0.6.x       |
| Pose provenance           | No per-frame pose row, no capture mode, no pose source recorded                                              | Medium   | v0.6.x       |
| Survey walk               | No segment kind; a tilted walk is classified as motion and discarded                                         | Medium   | v0.6.x       |
| IMU and GNSS capture      | No daemon, no packet formats, no calibration path; both are needed for the bike                              | Medium   | v0.6.x       |
| Field compute             | No Pi baseline; live feedback claims are unsupported                                                         | Low      | v0.6.x       |
| Sensor supply             | One qualifying used unit under budget; alternatives were untracked until the market watch started            | Low      | Continuous   |

## Design / approach

### Rigs and regimes

| Rig                 | Platform motion             | Typical stop               | What it yields                                              |
| ------------------- | --------------------------- | -------------------------- | ----------------------------------------------------------- |
| Cargo bike          | 10 to 25 km/h along a route | Under a minute at lights   | Many segments with few samples each; vibration is the enemy |
| Backpack pedestrian | About 5 km/h along a route  | 5 to 20 minutes at corners | Fewer segments with many samples; sway is the enemy         |

Every capture from either rig decomposes into four regimes, detected from the data rather than
declared by the operator:

| Regime       | Detected by                                                           | Pipeline                                                             |
| ------------ | --------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Static       | Existing drift ratio and foreground thresholds                        | Existing polar grid; unchanged                                       |
| Quasi-static | Stabiliser residual low, ego speed near zero, continuous small motion | Stabiliser into the reference polar grid, then the existing L3 to L6 |
| Route        | Ego speed from the trajectory above walking-pace threshold            | Trajectory, deskew, world-anchored foreground, world-frame tracking  |
| Survey       | Ground-plane normal far from the sensor's spin axis                   | Handed to the priors tooling; no traffic measurement                 |

The split tool keeps `motion` and `static` and gains `quasi-static`, `route`, and `survey` as
sub-kinds, so the archive's existing motion segments become route segments without re-recording.

### Route regime pipeline

```text
PCAP bundle: LiDAR, IMU, GNSS streams on one interface
  -> L1 parses each stream; L2 frames keep per-point time
  -> offline odometry (priors tooling; LiDAR-only baseline, LiDAR-inertial when an IMU is present)
       produces one trajectory file per capture: pose, velocity, and quality per frame, plus deskew
  -> PoseProvider replays the trajectory into the sensing pipeline
  -> L3 world-anchored foreground: a short-horizon occupancy map in the world frame;
       points landing in cells the map has seen empty are foreground
  -> L4 clustering and L5 tracking in the world frame; measurement noise inflated by ego uncertainty
  -> L6 classes as today
  -> L7 projection onto the road model: segment, signed distance from the intersection boundary
  -> L8 segment aggregates and speed-by-distance profiles
```

Rules that hold throughout:

- **Ego velocity error is object speed error.** The trajectory carries a per-frame velocity
  uncertainty, and an observation whose ego uncertainty exceeds the gate never enters a speed
  aggregate. A tenet three rule, not a tuning choice.
- **Deskew is mandatory in the route regime.** A 100 ms sweep at 5 m/s moves half a metre, which
  smears every cluster in the frame. The odometry stage deskews with per-point timestamps, and an
  IMU makes the correction exact through bumps and quick yaw.
- **Odometry stays outside the sensing binary.** The priors review already chose that boundary:
  the workstation runs an established LiDAR or LiDAR-inertial odometry tool and the binary reads a
  trajectory file. It never gains a SLAM dependency.
- **The stop regime is the same machinery at zero velocity.** A quasi-static stop is a trajectory
  with a keyframe map and near-zero velocity, so the stabiliser described below is the degenerate
  case of the route pipeline rather than a separate system.
- **The world-anchored foreground engine is required, not deferred.** The earlier reading of this
  plan kept it as a fallback for pitch aliasing at stops. The route regime cannot run without it,
  and once it exists it also serves stops where re-rendering into the polar grid aliases badly.

### Trajectory contract

One trajectory file per capture, produced offline and consumed by the `PoseProvider` interface the
motion capture architecture plan already defines:

| Field                     | Meaning                                                            |
| ------------------------- | ------------------------------------------------------------------ |
| Frame index and timestamp | Matches the L2 frame it applies to                                 |
| Pose                      | Position and unit quaternion in the capture's local frame          |
| Velocity                  | Linear and angular, for de-biasing and for the ego speed threshold |
| Quality                   | Registration residual, inlier fraction, and a velocity uncertainty |
| Placement                 | Optional transform from the local frame to WGS84, with its source  |
| Provenance                | Odometry tool and version, parameters, IMU present or not          |

The local frame is the sensor's pose at the first frame. Placement comes from phone-grade GNSS
snapped to the road graph, refined by registration against the priors where they exist. Without
either, the trajectory is still valid for relative speeds; it just cannot be projected onto named
segments.

### Stabiliser for stops

A stabilisation stage sits between frame assembly and the background grid, offline, in analysis
mode. It builds a keyframe map from the first settled seconds of the stop, then registers every
later frame to it with point-to-plane ICP on a downsampled set weighted towards ground, wall
planes, and high-intensity signs. This is the anchor ladder of the reflective sign proposal,
promoted from diagnostic to base case. The quasi-static prior keeps it cheap: the initial guess is
always the previous pose, and the overlap with the keyframe map is near total.

Per frame it outputs a rigid transform, a residual, and an inlier fraction. The transform corrects
the points; the residual and inlier fraction become the `FrameStabilitySignal`.

| Geometry fact           | Value    | Consequence                                                             |
| ----------------------- | -------- | ----------------------------------------------------------------------- |
| Azimuth bin             | 0.2°     | Yaw correction is a circular shift of the azimuth index, exact to a bin |
| Dense-band ring spacing | 0.34°    | A 1° pitch change moves dense-band rays by three rings                  |
| Outer ring spacing      | 1° to 6° | Re-rendered points alias most in the sparse upper and lower rings       |

Two ways to feed corrected points to the background model:

- **A. Re-render into the reference rays.** Project corrected points onto the nearest ring and
  azimuth of the reference pose. Every line of L3 stays as it is; only a projection step is added.
  The error is the elevation mismatch within a ring spacing, about 0.1 m at 20 m in the dense band,
  well inside the 0.5 m safety margin. Cells with no sample that frame simply do not update, which
  the grid already handles. This is the first thing to try for stops.
- **B. World-anchored grid engine.** The route regime's foreground engine, anchored to the stop's
  reference pose. Used at stops when A's aliasing is shown to matter, and always for a sensor
  without rings. The grid standards comparison rejected this for the fixed case precisely because
  it needs a pose; once a pose exists the objection lapses.

### Stability gate

- A frame whose residual, inlier fraction, or ego uncertainty fails the gate updates nothing: no
  background learning, no track observation, and tracks coast.
- A speed observation is never emitted from an ungated frame.
- Gate thresholds are tuning parameters and are swept in the motion-static tuning plan's harness.

### Road model

The simplest model that answers "what happens either side of the intersection" has three kinds of
feature and one rule about crosswalks:

| Feature      | Geometry                                                              | Rule                                                                                 |
| ------------ | --------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Intersection | Polygon bounded by its crosswalks, or stop lines where there are none | Crosswalks belong to the intersection, not to the road                               |
| Road segment | Directed centreline between two intersection boundaries, with a width | Carries both travel directions as a lane-side attribute                              |
| Crossing     | A crosswalk on a road with no intersection                            | A marker on the segment, not an intersection; it splits profiles but not the segment |

The seed is OpenStreetMap: highway ways for centrelines, junction nodes for intersections, and
crossing nodes and footways for crosswalks. Corrections are made by hand in GeoJSON, and the
spatial priors service publishes the corrected model per S2 cell so a rig with no network still
works from a local file, per tenet four. Segments carry stable identifiers independent of OSM
way ids, following the identifier rule in the priors plan.

Every track observation is projected onto the model as a segment or intersection identifier, a
signed distance along the centreline from the nearest intersection boundary, negative on approach
and positive on departure relative to the vehicle's direction, and a lateral offset. Crossings add
a second reference distance. The projection lives in L7, which already owns road polygons and the
scene graph.

### Segment aggregates

| Output                    | Definition                                                                                        |
| ------------------------- | ------------------------------------------------------------------------------------------------- |
| Per-vehicle band speed    | For each vehicle and distance band, its maximum and its typical observed speed inside the band    |
| Band aggregate            | Aggregate-only p50, p85, p98 over per-vehicle band maxima, with n and rig presence minutes        |
| Speed-by-distance profile | Band aggregates ordered from approach to departure, per segment and direction                     |
| Stop fraction             | Share of vehicles whose minimum speed in the 0 to 10 m approach band is below a stopped threshold |
| Segment summary           | The same aggregates over the whole segment, for the map view                                      |

Default bands measured from the crosswalk line: 0 to 10 m, 10 to 25 m, 25 to 50 m, and 50 to
100 m, on each side. A band with fewer vehicles than the report's minimum sample size shows its
count and no percentile. A moving rig samples a segment only while it is present, so presence
minutes are part of every aggregate and the report says so.

These follow the existing percentile rule: percentiles are aggregate-only, computed over a
population of per-vehicle maxima, and never rolled up from other percentiles.

### Self-occlusion, the visualiser, and provenance

- The wearer or the bike frame occludes a fixed sector. A static mask over those bins, captured
  once per rig, removes it before any foreground stage sees it.
- The FrameBundle gains a per-frame sensor pose so VelocityVisualiser can draw the sensor moving
  inside a fixed world. That view is the primary debugging tool for this work.
- A capture carries `capture_mode` (`tripod`, `backpack`, or `bike`), `pose_source` (`none`,
  `lidar`, or `lidar+imu`), the odometry provenance, and the stabiliser's version and thresholds,
  in the segment metadata, the VRLOG header, and the run record, following the existing S2
  conformance contract.
- Every frame of a processed run has a pose row carrying its quality fields, stored where the
  static pose alignment design put `pose_id`.

### Capture protocol

The protocol is what the operator does; the pipeline is built to that protocol rather than the
other way round.

Both rigs:

1. Start capture and hold still for ten seconds so the rig has a quiet start.
2. Rotate the rig slowly through about 30° of yaw and back over ten seconds. This is the wiggle
   the IMU time offset and mounting rotation are estimated from.
3. Cover the route. Stops need no announcement; the data detects them.
4. Stop capture, note the route and the clock, and mark positions as the archive does today.

Backpack additions:

- Stop at corners for five to twenty minutes when the junction matters. Turning to look is fine;
  walking is not.
- Optionally, before a long stop, tilt the sensor to the survey detent, about 45° from vertical,
  and walk the junction once, two to five minutes, then return it to level. Level, a side façade
  is only ever struck at the fixed ring elevations; tilted, the elevation at which a façade point
  is struck depends on its bearing, so passing it sweeps the rings up and down it, and the forward
  rays sweep the road surface at close range. The angle is an experiment parameter.

Bike additions:

- Ride at a steady pace and stop at lights as normal. A parked stop of ten minutes at a junction
  counts as a backpack-style stop and is processed the same way.
- The sensor and IMU share one rigid plate on a post above the cargo box; no soft mount between
  them, per the priors review.

### The PCAP bundle

One capture is one set of PCAP files, as today, with the five-minute rotation kept. Extra sensors
join the same capture as UDP streams on the LiDAR interface:

| Stream | Packet source                                                                    | Port           | Parsed where                              |
| ------ | -------------------------------------------------------------------------------- | -------------- | ----------------------------------------- |
| LiDAR  | Pandar40P                                                                        | 2368           | L1, as today                              |
| IMU    | Daemon on the Pi reading the unit over I2C or serial, stamping with the Pi clock | one fixed port | L1 sidecar, like the proposed NMEA stream |
| GNSS   | Phone or USB receiver, NMEA over UDP                                             | 10110          | The GPS-over-Ethernet proposal            |

The Pi clock stamps the PCAP arrival of LiDAR packets, the IMU samples, and the NMEA sentences, so
all three share one time base without PPS wiring. The sensor's internal per-point timestamps are
used for deskew within a frame. Millisecond jitter on the shared clock is a fraction of a bin at
the rotation rates either rig produces.

### Hardware

The sensor selection rule: at least 100 m range at 10% reflectivity, 360° horizontal coverage from
a multi-ring spinning unit, Ethernet UDP output, and a built-in IMU as a plus. The budget rule:
LiDAR plus IMU under US $500 in total, which means a used LiDAR at or under about US $450 and an
IMU under US $50. The Livox Mid-360 class is excluded by range. The
[LiDAR market watch](../platform/hardware/lidar-market-watch.md) tracks both rules weekly.

| Part          | Choice                                                                                       | Why                                                                                                 |
| ------------- | -------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| LiDAR         | Used Hesai Pandar40P                                                                         | The only unit in the first snapshot meeting both rules; it is also what the pipeline already parses |
| IMU           | Six-axis MEMS breakout, chosen by noise density and bias stability, logged at 200 Hz or more | The priors review's guidance; magnetometers are useless next to the sensor and passing cars         |
| GNSS          | The operator's phone, or a USB receiver, as NMEA over UDP                                    | Needed to place a route trajectory on the road graph; phone grade is enough                         |
| Power         | 12 V pack sized for the sensor's roughly 18 W plus the Pi                                    | The sensor dominates; the computer choice barely moves the budget                                   |
| Field compute | Raspberry Pi 4 as a recorder                                                                 | Record-then-process; a live stability meter needs the Pi baseline first                             |
| Desk compute  | Any laptop; a Mac mini is a desk option                                                      | Odometry and analysis run here; no accelerator has a role                                           |

Units that fail the range rule on the vendors' own figures, so nobody re-checks them: Hesai XT32
and XT16, the standard Ouster OS1, RoboSense Airy, and the Velodyne VLP-16, which states no 10%
figure at all. Units that meet the range rule but not the budget, watched for price drops: Hesai
OT128 and Pandar128E3X, Ouster OS2 and OS1 Max, RoboSense Helios-32 and Ruby Plus, Velodyne
VLP-32C. Older units not yet seen in a snapshot and added to the watch: Velodyne HDL-32E,
RoboSense RS-LiDAR-16 and RS-LiDAR-32, Hesai Pandar64 and Pandar20.

### Privacy

The trajectory is the operator's location history. It is used locally to place observations on
segments and is never part of a priors upload or a published catalogue. What leaves the machine is
segment geometry and segment aggregates. The existing rule that motion segments carry no S2 tag
stays for anything exported; internally, route segments are placed so their observations can be
aggregated, and the raw trajectory can be deleted once the aggregates are written.

## Scope

### Item 1: capture protocol and bundle conventions

**Summary:** Publish the field protocol for both rigs, the mount and detent geometry, the file and
metadata conventions, and the archive index fields, so captures made before the software exists
are usable by it.

**Steps:**

1. Add the protocol above to the getting-started guide and a one-page field checklist per rig.
2. Extend the archive's map marks and site index with `capture_mode`, route files, and `tilt_deg`.
3. Reserve the IMU UDP port and document the tcpdump filter that captures all three streams.
4. Record the mount, plate, and detent geometry for each rig, with measured reference marks.

**Milestone:** v0.6.x

### Item 2: motion spectrum and the car-drive proxy

**Summary:** Measure, per frame, the rigid transform between a capture and its reference, for the
Van Ness tripod episode, the first backpack stops, and the archive's car drives as the bike proxy.

**Steps:**

1. Implement the per-frame rigid transform estimator as an offline `pcap-split` report, which is
   test A of the nudge tolerance plan.
2. Report amplitude and frequency of yaw, pitch, roll, and translation at stops, and settle time
   after each step.
3. Run the archive's motion segments through the odometry baseline and report ego speed, residual,
   and where registration fails, as the first route-regime evidence.
4. Run part one of [backpack-motion-spectrum-and-stabiliser](../../data/experiments/try/backpack-motion-spectrum-and-stabiliser.md).

**Milestone:** v0.6.x

### Item 3: stop stabiliser and stability gate

**Summary:** Keyframe registration, re-rendering into reference rays, the gate, the self-occlusion
mask, and pose provenance, all in analysis mode.

**Steps:**

1. Keyframe map from the first settled seconds; point-to-plane registration with the anchor ladder weighting.
2. Re-render corrected points into the reference polar grid (option A).
3. Gate background updates and track observations on residual and inlier fraction.
4. Per-frame pose rows, `capture_mode` and `pose_source` in provenance, and the sensor pose in the FrameBundle.
5. Run part two of the experiment: A/B against the tripod archive at the same site.

**Milestone:** v0.6.x

### Item 4: trajectory contract and PoseProvider

**Summary:** The trajectory file format, the offline odometry recipe in the priors tooling, deskew,
and the `PoseProvider` that replays a trajectory into the pipeline.

**Steps:**

1. Fix the trajectory schema above and its provenance fields.
2. Script the odometry recipe: LiDAR-only baseline for existing captures, LiDAR-inertial once IMU
   streams exist; deskew with per-point timestamps.
3. Implement the file-backed `PoseProvider` with interpolation at frame time.
4. Ego speed threshold and regime labelling in `pcap-split` from the trajectory.

**Milestone:** v0.6.x

### Item 5: world-anchored foreground and world-frame tracking

**Summary:** The route regime's L3 engine and the tracker changes that let L4 to L6 run in a
world-fixed frame with a moving sensor.

**Steps:**

1. Short-horizon occupancy map in the world frame, selected like the L4 and L5 engines.
2. Foreground by map disagreement; free-space carving along rays so parked cars the map has
   already seen are background.
3. Tracker measurement noise inflated by ego uncertainty; ego-velocity de-biasing per the motion
   capture architecture plan's update step.
4. Ego-uncertainty gate on speed observations.

**Milestone:** v0.6.x

### Item 6: road model, projection, and segment aggregates

**Summary:** Intersections, segments, and crossings from an OSM seed with GeoJSON corrections;
projection of observations; band aggregates; the speed-by-distance report chart.

**Steps:**

1. GeoJSON schema for intersections, segments, and crossings; S2 L13 indexing; stable identifiers.
2. OSM seed import through the vector scene map's anchor import, with hand corrections.
3. Projection in L7: segment, signed distance, lateral offset, direction.
4. Band aggregates with n and presence minutes; minimum sample rule; stop fraction.
5. Report chart and API for the speed-by-distance profile and the segment map.

**Milestone:** v0.6.x

### Item 7: survey segment kind and priors hand-off

**Summary:** Detect the tilted walk, tag it, and export it in a form the priors tooling reads.

**Steps:**

1. Ground-normal tilt detection in the split classifier; `survey` segment kind.
2. Frame export with per-point timestamps in LAS 1.4 or per-frame PCD, whichever the priors
   experiment settles on.
3. Register a stop's reference pose to the walk submap and record the transform.
4. Run [survey-walk-geometry-gain](../../data/experiments/try/survey-walk-geometry-gain.md).

**Milestone:** v0.6.x

### Item 8: IMU and GNSS capture with automatic calibration

**Summary:** A Pi daemon that broadcasts IMU samples as UDP, NMEA relay from a phone or receiver,
L1 sidecar parsers, and calibration from the start wiggle. Required for the bike; for backpack
stops, gated on Item 2 showing that LiDAR-only stabilisation loses frames it should keep.

**Steps:**

1. Choose a six-axis unit by noise and bias figures, per the priors review, not by rate alone.
2. Daemon, packet formats, and replay parsers; the PCAP remains the single artefact.
3. Mounting rotation from gravity against the LiDAR ground normal; time offset from yaw-rate
   cross-correlation over the wiggle.
4. Use the IMU in odometry and deskew; measure the gain against the LiDAR-only baseline.

**Milestone:** v0.6.x

### Item 9: LiDAR market watch

**Summary:** A weekly scheduled check of vendor announcements and used listings against the range
rule and the budget rule, appended to the market watch page.

**Steps:**

1. Keep the page's rules current; the first snapshot is in.
2. Run the weekly routine, which pushes each entry to the `claude/lidar-market-watch` branch.
3. Review monthly; a unit that meets both rules becomes a purchase decision.

**Milestone:** Continuous

### Item 10: Raspberry Pi performance baseline

**Summary:** Capture `lidar-bench` baselines on the Pi 4 so every field-compute claim in this plan
rests on a measurement.

**Steps:**

1. Run the gated profiles on the Pi and commit the baselines with a platform suffix.
2. Record L3-only headroom, which bounds what a live stability meter may cost.

**Milestone:** v0.6.x

### Item 11: validation against the fixed radar

**Summary:** Prove that route-regime speeds agree with an independent fixed sensor before any
segment aggregate is published.

**Steps:**

1. Run [route-speed-accuracy-vs-fixed-radar](../../data/experiments/try/route-speed-accuracy-vs-fixed-radar.md)
   with the car drives first, then the bike.
2. Set the ego-uncertainty gate and the minimum sample rule from its results.

**Milestone:** v0.6.x

## Dependencies

- PR #569's capture index and multi-file continuous classification, which the segment and
  provenance changes extend.
- The nudge tolerance plan's test A is Item 2's first deliverable; the two plans share it.
- The spatial priors experiment in the priors review §8 supplies the odometry recipe and consumes
  Item 7's export.
- The sign anchor proposal supplies the anchor ladder and the stability signal contract.
- The vector scene map's OSM anchor import is Item 6's seed; if it has not landed, Item 6 imports
  the seed directly and hands the loader back later.
- A fixed radar deployment on a segment the rigs can pass, for Item 11.
- A bike mount with a rigid plate, a backpack mast with two detents, and a battery supply for the
  sensor, which are field kit rather than software.

## Risks

| Risk                                                                      | Likelihood | Impact | Mitigation                                                                                     |
| ------------------------------------------------------------------------- | ---------- | ------ | ---------------------------------------------------------------------------------------------- |
| Ego velocity bias from odometry shifts every speed on a segment           | Medium     | High   | Item 11 measures it against the radar before anything is published; gate on ego uncertainty    |
| Bike vibration breaks registration between frames                         | Medium     | High   | IMU required for the bike; rigid plate; the car drives show the ceiling before the bike exists |
| Map matching picks the wrong segment at complex junctions                 | Medium     | Medium | Presence minutes and lateral offset flag ambiguous placements; hand corrections in GeoJSON     |
| OSM lacks crosswalks where the rule needs them                            | High       | Medium | Stop-line fallback; corrections published through the priors service                           |
| Few vehicles per segment from a moving rig                                | High       | Medium | Minimum sample rule; counts always shown; backpack stops where the junction matters            |
| Sway pitch exceeds what re-rendering tolerates and option A aliases badly | Medium     | Medium | Item 2 measures it first; option B exists anyway for the route regime                          |
| Too few planar anchors at some corners, so LiDAR-only registration drifts | Medium     | Medium | Gate and report; IMU prior; treat as a site-suitability finding                                |
| The wearer or frame occludes the approach that matters                    | Medium     | Low    | Mast above head height; the mask reports its sector so the operator can turn                   |
| Used Pandar40P supply dries up or prices rise past the budget             | Medium     | Medium | Market watch tracks the older 100 m units; the parser is pluggable per the sensor catalog plan |
| Survey walk output never reaches a priors product                         | Medium     | Medium | Item 7 exports a format the priors experiment already plans to ingest                          |
| Market watch drifts into unverified prices                                | Medium     | Low    | Entries carry URLs and dates; "not stated" is a valid value                                    |

## Checklist

### Complete

- [x] Ideation and scope cut recorded in this plan.
- [x] Market watch page published with the first snapshot and the weekly routine running.

### Outstanding

- [ ] Item 1: capture protocol for both rigs, mounts, bundle conventions, archive fields (`S`)
- [ ] Item 2: per-frame rigid transform report, motion spectrum, car-drive proxy (`M`)
- [ ] Item 3: stop stabiliser, gate, mask, provenance, FrameBundle pose (`L`)
- [ ] Item 4: trajectory contract, odometry recipe, PoseProvider, regime labelling (`M`)
- [ ] Item 5: world-anchored foreground engine and world-frame tracking (`L`)
- [ ] Item 6: road model, projection, band aggregates, report chart (`L`)
- [ ] Item 7: survey segment detection, export, submap registration (`M`)
- [ ] Item 8: IMU and GNSS daemons, sidecar parsers, automatic calibration (`M`)
- [ ] Item 9: market watch upkeep (`S`)
- [ ] Item 10: Raspberry Pi perf baseline (`S`)
- [ ] Item 11: validation against the fixed radar (`M`)

### Deferred

- [ ] 3D object orientation and the 13-state tracker: not needed for route capture, tracked by [lidar-motion-capture-architecture-plan](lidar-motion-capture-architecture-plan.md).
- [ ] Lane-level placement: lateral offset is recorded, but lanes are not modelled until a segment has enough samples to justify them.
- [ ] Live stabilised tracking in the field: the workflow is record-then-process by design.

### Accepted residuals (no action planned)

- [ ] RTK positioning: placement comes from phone-grade GNSS snapped to the road graph and refined against the priors.
- [ ] Odometry inside the sensing binary: the trajectory file is the boundary, per the priors plan.
