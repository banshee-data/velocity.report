# Lane field and lane proposals from tracked traffic plan

How the Go server turns a deployment's tracked vehicles, cyclists and pedestrians into a top-down
lane field, proposes travel lanes, sidewalks and crossings from it with a confidence grade, and
keeps the lanes an operator approves in the macOS street painter as a snapshot that the speed
reports and the public survey pages read. The server never draws a lane on its own: every lane that
reaches Gerardus or a report was approved or drawn by an operator.

- **Status:** Proposed; waits on the accepted pose writeback (v0.5.5) and on the owner's decision on
  Gerardus ADR-0016 (proposed there). Not Active (D-32)
- **Decisions:** relies on D-18 (nearest-rank percentiles), D-32, D-33, D-36 (Gerardus owns the
  static street geometry; L8 alone owns aggregation and scoring; the L7 adapter composes the two),
  D-38 (geometry is measured spatial data) and Gerardus ADR-0006 and ADR-0011 (per-object tracks
  never leave the site; nothing published with fewer than five objects in a bin). Proposes one new
  register entry, written in the Design decisions section
- **Layers:** L8 analytics (the field and the proposals), L7 scene adapter (lane assignment), SQLite
  storage, HTTP API; consumed by the macOS visualiser and the web site viewer
- **Target:** unscheduled; proposed for the v0.7.1 slot (reference speeds), after the pose writeback
- **Companion plans:** [lane painter in the visualiser](lidar-visualiser-lane-painter-plan.md),
  [speed by direction and lane](platform-speed-by-direction-reporting-plan.md),
  [public survey direction histograms](public-survey-direction-histograms-plan.md),
  [posted speed limits](posted-speed-limits-plan.md),
  [lane-constrained fragment stitching](lidar-lane-stitching-plan.md) (its lane map, C2 and C3),
  [visualiser alignment client](lidar-visualiser-gerardus-alignment-plan.md) (the pose), and
  Gerardus [LANES-AND-LIMITS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANES-AND-LIMITS.md)
- **Canonical:** [3D vector scene map](../lidar/architecture/vector-scene-map.md)

The lane field is measured spatial data: where tracked objects were, counted, at the stated
acquisition window. A lane proposed from it is an inference the operator confirms or corrects, and
the painter records it as the operator's marking, not as a measurement. See
[Geometry priors: data purpose](../geometry-priors-data-purpose.md).

## Motivation

Speed reports today take one posted limit per report request, defaulting to 25 mph, and apply it to
every vehicle in the sensor's view. At an intersection where one street is signed 25 mph and the
cross street 30 mph, or where the two directions of one street carry different limits, every share
above the limit is wrong for at least one direction. The
[posted speed limits plan](posted-speed-limits-plan.md) settled the model (a limit belongs to a
stretch of road for one direction of travel) but is parked until road geometry exists in this
repository. That geometry now exists in Gerardus: the street painter marks travel lanes with a
direction of travel, and Gerardus stores them.

What is missing is the bridge from traffic to lanes and from lanes to reports. An operator painting
lanes by eye over a point cloud has no evidence of where traffic actually runs, and the reports have
no copy of the lanes to attribute a speed to. Without this plan every lane is a guess drawn once, and
no report can say which limit a speed was measured against.

## Current state

- **No lane concept in the Go code.** The word appears in comments and tuning keys only
  ([lane-stitching plan](lidar-lane-stitching-plan.md), Current state). There is no lane table, no
  lane identifier and no function from a position to a lane.
- **Tracks and their boxes are persisted.** Run tracks and their observations carry the published
  box per observation (migration 000062) and the class label (`car`, `truck`, `bus`, `pedestrian`,
  `cyclist`, `motorcyclist`, `bird`, `dynamic`, `internal/lidar/l6objects/classification.go`).
  Observation support (observed against coasted) is recorded per instant
  (`internal/lidar/l5tracks/continuity.go`).
- **L8 can fit a directed path from trajectories.** `internal/lidar/l8behaviour/localpath.go` fits a
  local following path with lateral grouping and refuses forks; `pathframe.go` gives arc length,
  signed lateral offset and tangent along a directed path. `internal/lidar/l8analytics/lateral_fit.go`
  measures lateral residuals against a straight fit.
- **The scene timeline already aggregates per track.** `internal/scene/timeline.go` groups classes
  into vehicle, person and cycle (`modeOf`) and builds one vehicle speed summary per scene with
  nearest-rank percentiles (D-18) and 5 mph buckets.
- **The painter exists and the web editor is gone.** The macOS street painter marks surfaces,
  travel lanes (a centreline in the direction of travel, tags `lane_type` and `width_m`), parked-car
  spaces and driveways over the placed Gerardus reference, and saves them with `EditStreetUse`
  ([hub doc](../ui/visualiser/gerardus-alignment.md#paint-mode-the-street-painter)). The web
  `/lidar/sites` page reads the same marks from Gerardus's GeoJSON and does not edit.
- **The accepted pose does not reach this repository yet.** `PUT /api/lidar/sites/{l16}/pose` is a
  v0.5.5 item; `CoordinateFrameInfo` is the identity. Tracks are in the sensor frame.
- **Published scenes carry Gerardus street geometry where a pose is accepted**, placed in the sensor
  frame by the inverse pose (`tools/s2-archive/scene-vectors.py`), but not the travel lanes.
- **Experiment lane0001 is proposed, not run:** a hand-drawn lane map for the kirk placement, with a
  pass rule on how much reviewed traffic it holds ([lane0001](../../data/experiments/lane/lane0001.md)).
- **The radar loses direction at the transit.** The transit worker reads `ABS(speed)`
  (`internal/db/transit_worker.go`), so a radar transit has no direction; the raw reading is signed
  and `radar_objects` carries `object_inbound` or `object_outbound`.

## Vocabulary

| Term          | Meaning here                                                                                                                                                                                                                                                            |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Lane field    | A top-down grid over the sensor's coverage, in the sensor frame, counting per cell how many distinct tracks of each class group passed in each heading sector, how many observations' boxes covered the cell, and the speeds seen. SPEC-V1's `LaneField`, computed here |
| Travel lane   | Gerardus's `TRAVEL_LANE` street-use mark: a centreline drawn in the direction of travel, with `lane_type` and `width_m`, and, after this work, a posted limit                                                                                                           |
| Direction     | The lanes of one street that carry traffic the same way: one approach at an intersection, one side of a two-way street. The unit a report groups speeds by. Named by street and compass bound, for example "Kirkham St eastbound"                                       |
| Limit segment | The posted-limits plan's unit: a stretch of one direction between two limit changes. Here a lane is split where its limit changes, so a lane is one limit segment                                                                                                       |
| Proposal      | A lane, sidewalk or crossing the server infers from the lane field, with a confidence grade, shown to the operator in the painter. Nothing is stored until the operator approves it                                                                                     |
| Lane snapshot | This repository's copy of the lanes accepted for one deployment, with the pose that places them, written by the visualiser after a successful save to Gerardus. What attribution and reports read                                                                       |

"Segment" on its own is avoided: in this repository it already names an annotation time window
(`lidar_segment_selections`), and in Gerardus a city street segment (a CNN). The lane-stitching
plan's "lane segment" is this plan's travel lane.

## Design

### Boundary: who computes what

| Where                        | Does                                                                                                                                                                                                         | Never does                                                                                      |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------- |
| Go server (this plan)        | Builds the lane field from persisted tracks; proposes lanes, sidewalks and crossings with a grade; stores the approved lane snapshot; attributes a position, heading and time to a lane, direction and limit | Draw a lane without an operator; send a track, a trajectory or a per-object sample off the host |
| macOS visualiser (companion) | Draws the field and the proposals in the plan view; the operator approves, edits, dismisses, enters limits and splits lanes at signs; saves to Gerardus; sends the snapshot here                             | Fit lanes itself from the live stream, except as a labelled fallback for a short capture        |
| Gerardus (its plan)          | Stores the approved lanes as authored street-use marks with their limit and provenance tags; serves city speed-limit records and sign codes to prefill limits                                                | See tracks; compute speed statistics                                                            |
| Web site viewer              | Reads the field as a raster and the snapshot's lanes, read-only                                                                                                                                              | Edit                                                                                            |

This keeps Gerardus ADR-0006 (velocity.report needs no network call to report: the snapshot is
local), ADR-0011 (only aggregates leave: the approved geometry and counts, never a track) and
D-36 (Gerardus owns the static geometry; L8 owns aggregation; the L7 adapter joins them).

### The lane field

**Frame and grid.** The sensor frame's horizontal plane, cells of 0.25 m, sparse (only occupied
cells are stored). A lane is about 3.3 m wide and a car 1.8 m, so a cell is fine enough to separate
lanes and coarse enough that one deployment's field is a few hundred thousand cells at most.
Extent follows the tracks: no fixed radius.

**Inputs.** Confirmed tracks of the selected runs, observed samples only (coasted and predicted
instants are dropped, as the stitching plan requires), with the observation's published box where
the box model converged and the cluster centroid otherwise, recorded per observation so a centroid
shift is not read as a lane change. Class groups follow `modeOf` in `internal/scene/timeline.go`:
vehicle (`car`, `truck`, `bus`), person (`pedestrian`), cycle (`cyclist`, `motorcyclist`); `bird`,
`dynamic` and unlabelled tracks are left out. Heading comes from the track's velocity when its
speed is at least 1 m/s and is otherwise unknown: a stopped vehicle counts towards extents, not
towards direction.

**What a cell holds**, per class group and per heading sector (16 sectors of 22.5°, plus one
"unknown heading" bucket):

| Field                   | Meaning                                                                                           |
| ----------------------- | ------------------------------------------------------------------------------------------------- |
| `tracks`                | Distinct tracks whose centre entered the cell: one per track, however many frames it stayed       |
| `observations`          | Observed samples whose centre fell in the cell                                                    |
| `box_cover`             | Observed samples whose box footprint covered the cell: the "box extents" the operator asks to see |
| `speed_sum`, `speed_sq` | Of the samples' speeds, for a mean and spread per cell                                            |

**Windows.** A field is built for one deployment and one time window: a capture, a day, or every
selected run. It also keeps two disjoint halves of the window (first and second half by time) so a
proposal can be tested for agreement between them (SPEC-V1 §6's acceptance: two disjoint windows
agreeing within 0.20 m). The field records the run ids it read, the tuning fingerprint of each, the
class grouping version and its own version, and is cached by that digest.

**Which runs.** Run records are immutable and a capture may have several (different tuning). The
default is the newest run of each selected capture under the default tuning; the operator may pick
others. The field never mixes runs of one capture.

**Where it is computed.** Offline, from the persisted tracks of a deployment, on request and
cached. It is not a live-pipeline stage and adds nothing to the Pi's frame time. The visualiser's
own accumulation from the stream (its `FrameAccumulation` pattern) is a fallback for a capture
being watched, labelled as such, and is the companion plan's.

**Suppression.** Inside the operator tools the field is shown whole, with cells below a chosen
track count faded by default (five, ADR-0011's floor). Anything that leaves the host (the snapshot's
provenance, a published raster) carries counts only where at least five distinct tracks contribute,
and never a track identifier.

### Proposals

Proposals are computed from the field, per class group, and graded. They are served to the
visualiser in the sensor frame; the visualiser places them in the site frame with the accepted pose.

**Travel lanes, from the vehicle field.**

1. **Directions.** A circular histogram of heading weighted by `tracks` gives the modes of travel.
   Modes are kept when separated by at least 60° and supported by at least five tracks. A two-way
   street gives two modes; a four-way intersection up to four, plus turning traffic that the next
   steps leave unclaimed.
2. **Axis and lateral profile.** For each mode, the cells of that mode's sectors are fitted with a
   dominant axis (weighted principal axis). Along the axis, in 10 m bins, the lateral density of
   `tracks` in 0.1 m steps is smoothed; its peaks, at least 2.2 m apart (the narrowest lane), are
   lane centres in that bin.
3. **Centrelines.** Peaks are linked bin to bin into polylines when the lateral change is under
   0.6 m per 10 m; a polyline shorter than 20 m is dropped. Where the street curves more than 15°
   over the coverage the straight axis fails, and the second version fits each lane's cluster with
   the L8 local-path method (`localpath.go`) instead; the first version reports curvature and
   abstains.
4. **Width.** Where `box_cover` across the lane falls to half its peak, clipped to 2.4–4.5 m,
   default 3.3 m when the extents are too thin to say.
5. **Grade.** Each lane carries its support (tracks through its corridor), the lateral spread of
   its peak, the heading spread, and whether the two half-windows each show the peak within
   0.20 m of the whole window's.

| Grade  | Initial rule, to be set by experiment lane0002                                                | Shown as                               |
| ------ | --------------------------------------------------------------------------------------------- | -------------------------------------- |
| High   | Support of at least 50 tracks, both halves agree within 0.20 m, lateral spread at most 0.35 m | Solid proposal                         |
| Medium | Support of at least 15 tracks, and either one half agrees within 0.20 m or both within 0.40 m | Dotted proposal                        |
| None   | Anything less                                                                                 | Not proposed; the field still shows it |

**Bike lanes, from the cycle field.** The same method on the cycle field. A cycle peak at least
1.0 m from every vehicle peak with support of at least 15 is proposed as a lane with
`lane_type=bike`; a cycle peak on a vehicle peak is folded into that lane's support as shared use.

**Sidewalks and crossings, from the person field.** Cells with at least five pedestrian tracks are
joined into connected ribbons. A ribbon that runs beside the carriageway (outside every vehicle
peak's corridor) is proposed as a `surface=sidewalk` polygon 1.5 m wide along its skeleton, snapped
to a kerb where Gerardus has one. A ribbon that crosses the vehicle field's band is proposed as a
crossing: where Gerardus's lane markings layer has a `crosswalk` within 3 m it is shown as
confirming that marking; otherwise it is proposed as a `surface=crossing` polygon, a surface value
the Gerardus plan asks for. People's positions are the most sensitive data the sensor holds, so the
person field is never exported and these proposals carry support counts only.

**Matching existing marks.** Before proposals are served, each is matched to the site's current
street-use marks (from the snapshot, or from Gerardus when the visualiser passes them): a lane whose
centreline lies within 0.5 m laterally of an existing lane, same direction, is reported as
"confirms" that mark with its support, or as "disagrees" with the offset, rather than as a new
proposal. The painter shows both states; the operator decides.

**Outputs.** A proposal set names its field digest, its method version and parameters, and lists
proposals with geometry (sensor frame), kind, suggested tags, grade, support, spreads, half-window
agreement and any matched mark. It is served, never stored: approving writes a street-use mark.

### The lane snapshot

The snapshot is this repository's copy of the lanes Gerardus holds for one deployment, as accepted
by the operator, with the pose that places them in the sensor frame. The visualiser writes it after
a successful `EditStreetUse`, so Gerardus is authoritative and the snapshot is a cache with
provenance. A report reads the snapshot in force at each measurement time; it never calls Gerardus.

| Record    | Fields                                                                                                                                                                                                                                                                                                               |
| --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Snapshot  | id; site (L16 token) and deployment (the config period today, `deployments` after vocabulary Item 14); the Gerardus pose record id and the pose itself; Gerardus server version and address; author; the feature rows' digest; created time; a `supersedes` link to the previous snapshot                            |
| Lane      | Gerardus feature id and row id; kind (`travel_lane`, `surface`); geometry in the site frame as Gerardus returned it, and in the sensor frame as derived at write time from the pose; tags as returned; derived bound (compass octant of travel), width, lane type, and the limit (value, unit, source, jurisdiction) |
| Direction | id; name; bearing; the lanes it groups; the limit when every lane in it agrees, else none and the lanes carry their own                                                                                                                                                                                              |

Snapshots are append-only; a new save makes a new snapshot that supersedes the old, so a report run
later against an earlier period still finds the lanes that were in force. The pose in the snapshot
must agree with the pose record the pose writeback stores for the same deployment, once that lands;
a disagreement refuses the snapshot with the two record ids named.

Directions are derived at write time: lanes whose bearings lie within 45° of each other and whose
`cnn` tags (the city street segment Gerardus snaps the lane to) name the same street form one
direction, named from the street's display name and the compass bound. The operator may rename a
direction in the painter; the name travels in the lane tags, so Gerardus holds it too.

The snapshot is also the lane map the stitching plan's C2 and C3 ask for: a directed centreline per
lane with a corridor half-width. The stitching plan's successors and transitions are not in it yet,
and that plan says where they would go.

### Attribution

One function, in a neutral package that imports no layer, from a position and heading in the
sensor frame and a capture time to the lane and direction containing it and the limit in force, or
to the reason there is none: no snapshot for that time, outside every lane's corridor, heading
opposed to every lane within reach, or a lane with no limit. The posted-limits plan's step 2 names
this function; the companion reporting plan says how LiDAR tracks and radar transits use it. It
lives here because it reads the snapshot.

Membership uses the lane's corridor (half its width plus 0.5 m) and a heading within 45° of the
lane's tangent, with hysteresis along a track so a vehicle straddling two lanes does not flicker.

### API

| Method and path                              | Purpose                                                                                                                                                                                  |
| -------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/lidar/sites/{l16}/lane-field`      | The field for a deployment and window (`from`, `to`, `runs`), as a sparse cell list with its digest and provenance; cached                                                               |
| `GET /api/charts/lane-field`                 | The same field as an SVG raster for the web, by class group, coloured by direction, beside `/api/charts/grid`                                                                            |
| `GET /api/lidar/sites/{l16}/lane-proposals`  | Proposals for a field (`field` digest), optionally matched against marks the client passes                                                                                               |
| `PUT /api/lidar/sites/{l16}/lanes`           | Writes a snapshot: the Gerardus street-use features as returned by `EditStreetUse`, the pose record and pose, author, request id (idempotent on the request id, as Gerardus's edits are) |
| `GET /api/lidar/sites/{l16}/lanes`           | The current snapshot, or the one in force at `at`                                                                                                                                        |
| `GET /api/lidar/sites/{l16}/lanes/attribute` | Attribution of one position, heading and time, for the painter's inspector and for tests                                                                                                 |

Requests and responses are JSON in the style of the existing LiDAR API; the field cell list may
also be served as a compact binary when the visualiser asks for it, as the scans endpoint does.

### Privacy

- Per-object tracks never leave the host (ADR-0011). The field, the proposals and the snapshot
  carry counts, geometry and spreads, never a track id or a trajectory.
- The person field is operator-only. No raster of it is served to the web or exported.
- Published rasters and snapshot provenance carry counts only where at least five distinct tracks
  contribute.
- The export privacy test (D-34) gains a case: no field or proposal file reaches a public scene.

## Scope

### Item 1: the lane field

**Summary:** Build and cache the field from persisted tracks for a deployment and window, with its
two halves, and serve it as JSON and as an SVG raster.

**Steps:**

1. A `lanefield` package under L8 analytics: inputs, grid, class grouping shared with the scene
   timeline, sectors, the two halves, digest and provenance.
2. The two endpoints, read-only, with the suppression floor as a parameter that defaults to five.
3. Unit fixtures: a synthetic two-way street with two lanes each way; a one-way street; a stopped
   vehicle counted in extents only; a coasted sample left out.
4. A measurement on kirk0 and columbus-broadway: cell counts, build time, and a rendering checked by
   eye against the point cloud.

**Milestone:** v0.7.1 `M`

### Item 2: lane, sidewalk and crossing proposals

**Summary:** Propose graded lanes, bike lanes, sidewalks and crossings from a field, matched against
existing marks, and serve them.

**Steps:**

1. Direction modes, axis, lateral profile, peak linking, width, and the grade rule as parameters.
2. The person-field ribbons and the crossing test against Gerardus's crosswalk markings.
3. Matching against marks the client passes.
4. Fixtures: two lanes each way recovered within 0.2 m; a bike lane beside them; a crossing where a
   pedestrian ribbon crosses; a curved street reported as curvature and abstained from.
5. The proposals endpoint.

**Milestone:** v0.7.1 `M`

### Item 3: the lane snapshot and attribution

**Summary:** Store approved lanes with their pose, derive directions, and attribute a position to a
lane, direction and limit.

**Steps:**

1. Migration: snapshot, lane and direction records, JSON-first with generated columns, append-only.
2. `PUT` and `GET` endpoints; the request-id rule; the pose agreement check once the pose record
   exists.
3. The neutral attribution package and its endpoint; membership with hysteresis.
4. Fixtures from the posted-limits plan: a lane split at a sign; opposite directions with different
   limits; a position outside every corridor; a heading opposed to the lane; a time before any
   snapshot.

**Milestone:** v0.7.1 `M`

### Item 4: experiment lane0002, proposals against drawn lanes

**Summary:** Score the proposals against lanes drawn by hand without seeing them, at kirk and
columbus-broadway, and set the grade thresholds from the result.

**Steps:**

1. Write the spec under `data/experiments/lane/` with its rule before the runs: lateral error per
   lane under 0.30 m against the drawn lane (SPEC-V1 §6's acceptance against an independent
   reference), correct lane count per direction, and the share of drawn lanes a high-grade proposal
   finds. lane0001's drawn map at kirk is one reference; the painter's own marks at
   columbus-broadway the other.
2. Run, report under `docs/lidar/operations/`, and adopt or revise the thresholds in the same change.

**Milestone:** v0.7.1 `S`

### Item 5: the field and the lanes in the web site viewer

**Summary:** Draw the field raster and the snapshot's lanes with their limits in the read-only site
viewer, so a browser can see what the painter saw.

**Steps:**

1. A "Traffic" layer on `/lidar/sites` reading `/api/charts/lane-field`, vehicle and cycle groups
   only.
2. Lanes from the snapshot drawn with their bound and limit, beside Gerardus's own street use.

**Milestone:** v0.7.9 `S`

## Dependencies

- The accepted pose writeback (v0.5.5) for the pose agreement check; the snapshot carries its own
  pose record so Item 3 does not block on it.
- Gerardus's lane limit tags and `crossing` surface value
  ([LANES-AND-LIMITS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANES-AND-LIMITS.md)),
  for what the snapshot stores; the snapshot stores whatever tags Gerardus returns, so the tag
  vocabulary can land in either order.
- The companion painter plan, which is the only writer of snapshots.
- Vocabulary Item 14 (`deployments`): until it lands the snapshot keys the deployment by site and
  config period.

## Risks

| Risk                                                                                                                                           | Likelihood | Impact | Mitigation                                                                                                                                                                            |
| ---------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Roadside LiDAR sees the sensor-facing side of vehicles, biasing centres towards the sensor by a class-dependent fraction of width (SPEC-V1 §6) | High       | Medium | Use the published box centre where the box model converged; report the bias in lane0002; a per-class correction is a follow-up                                                        |
| Lanes curve or merge within coverage and the straight axis fails                                                                               | Medium     | Medium | Report curvature and abstain in version one; the local-path fit in version two                                                                                                        |
| Too few tracks at quiet sites for a high grade                                                                                                 | Medium     | Low    | Medium grade and the field itself still guide the operator; windows can span days                                                                                                     |
| A proposal is approved without looking and a wrong lane enters Gerardus                                                                        | Medium     | Medium | Every approval is an authored mark with `gms:source=fitted` and its support; the matching step shows disagreement with existing marks                                                 |
| Pose error moves every lane sideways                                                                                                           | Medium     | High   | Proposals are in the sensor frame, so a pose error moves lanes relative to Gerardus's kerbs and shows in the painter; the snapshot refuses a pose that disagrees with the pose record |
| Person-field data leaks through a raster                                                                                                       | Low        | High   | The person field is never served to the web or exported; the export privacy test is extended                                                                                          |

## Design decisions

- **Proposals are computed on the sensor host, not in Gerardus.** SPEC-V1 §6 placed the fit in the
  Gerardus server from an uploaded `LaneField`. The tracks are here, the L8 path code is here, and
  ADR-0011 forbids tracks leaving. Gerardus receives approved lanes as authored marks with aggregate
  provenance. Proposed as Gerardus ADR-0016; this plan waits on the owner's acceptance.
- **Gerardus is authoritative for lanes; this repository keeps a snapshot.** ADR-0006: the Go
  server needs no network call to report. A snapshot is append-only and names the Gerardus rows it
  copied.
- **A lane is one limit segment.** Lanes are split where the limit changes, in the painter, so the
  posted-limits plan's arc-length limit changes are not stored separately.
- **Class grouping is the scene timeline's.** One grouping for the field, the timeline and the
  per-direction summaries, so the public pages and the operator tools count the same populations.
- **Proposed register entry:** "Lanes and limits: Gerardus holds the travel lanes and their posted
  limits as authored street-use marks; velocity.report proposes lanes from its own tracks on the
  sensor host, keeps an append-only snapshot of the accepted lanes per deployment, and attributes
  every reported speed to a lane and direction through it." Written into DECISIONS.md when the owner
  accepts ADR-0016 in Gerardus.

## Open questions

- Should the field default to the whole deployment or to the latest capture? The whole deployment
  gives support; the latest capture shows the street as it is now after a repaint.
- Is 0.25 m the right cell for the box-extents view, or should extents use 0.5 m to keep the
  raster small on the web?
- Does the near-side bias need correcting before lane0002, or is it measured there first?
- Which runs count for a published survey's directions: the survey's own export run only, as the
  scene timeline does today?

## Checklist

### Complete

- [x] Plan written; companion plans named; Gerardus plan and ADR-0016 proposed in the Gerardus repository

### Outstanding

- [ ] Item 1: lane field package and endpoints (`M`)
- [ ] Item 2: proposals and grades (`M`)
- [ ] Item 3: snapshot, attribution and migration (`M`)
- [ ] Item 4: experiment lane0002 spec, run and report (`S`)
- [ ] Item 5: web site viewer traffic layer (`S`)
- [ ] DECISIONS.md entry once ADR-0016 is accepted (`S`)

### Deferred

- [ ] Successors and merge transitions in the lane map: [lane-stitching plan](lidar-lane-stitching-plan.md)
- [ ] `LaneField` upload to Gerardus as evidence under k = 5: Gerardus M5
- [ ] Per-class near-side correction of track centres: after lane0002 measures it

### Accepted residuals (no action planned)

- [ ] The field is offline and cached; a live field on the Pi is not planned
- [ ] Turning traffic at an intersection is left unclaimed by lane proposals; the operator draws turn lanes
