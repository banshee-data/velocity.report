# Lane snapshot, attribution and where street data lives plan

How velocity.report keeps its own append-only copy of the travel lanes an operator approved in
Gerardus, with each lane's posted limit and the date that limit took effect, attributes every
reported speed to the lane, direction and limit in force when it was measured, checks Gerardus's
lane proposals against lanes drawn without seeing them, and keeps every piece of street data out of
git: in Gerardus, in the device's database, or in the Hugging Face dataset.

- **Status:** Active in the v0.7.19 slot: Item 1 (the snapshot, `lanes pull`, `lanegeom` and
  attribution) is built; the pose agreement check waits on the accepted pose writeback (v0.5.5)
- **Decisions:** relies on D-18 (nearest-rank percentiles), D-32, D-33, D-36 (Gerardus owns the
  static street geometry; L8 alone owns aggregation and scoring), D-38 (geometry is measured spatial
  data), D-39 (lanes and limits, recorded with this plan) and Gerardus ADR-0006, ADR-0011 and
  ADR-0016 (accepted November 3, 2026: the unified design)
- **Layers:** SQLite storage (the snapshot), a neutral lane geometry package (`lanegeom`), HTTP API,
  the web site viewer, a render tool for the Hugging Face dataset
- **Target:** unscheduled; proposed as the v0.7.19 slot in
  [BACKLOG.md](../BACKLOG.md#v0719---lanes-from-tracks-and-directional-speeds-0719)
- **Companion plans:** [lanes from tracks](lidar-lane-proposals-from-tracks-plan.md) (the lane
  field and its upload), [lane painter](lidar-visualiser-lane-painter-plan.md) (review, limits and
  the snapshot's writer), [speed by direction and lane](platform-speed-by-direction-reporting-plan.md)
  (the reports that read the snapshot), [public survey direction histograms](public-survey-direction-histograms-plan.md),
  [posted speed limits](posted-speed-limits-plan.md), [lane-constrained fragment stitching](lidar-lane-stitching-plan.md)
  (its lane map), [visualiser alignment client](lidar-visualiser-gerardus-alignment-plan.md) (the
  pose); Gerardus [LANE-PROPOSALS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANE-PROPOSALS.md)
  (W10: field storage, proposals, review) and
  [LANES-AND-LIMITS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANES-AND-LIMITS.md)
  (W1c: the limit tags)
- **Canonical:** [3D vector scene map](../lidar/architecture/vector-scene-map.md)

The lanes copied here are the operator's reading of the street, approved from measured traffic or
drawn by hand, at the time each version was saved. They are not a survey. See
[Geometry priors: data purpose](../geometry-priors-data-purpose.md).

This plan was first written as "Lane field and lane proposals from tracked traffic", proposing lanes
on the sensor host. The owner chose the unified design on November 3, 2026 (Gerardus ADR-0016):
velocity.report computes the lane field and uploads it, Gerardus derives and grades the proposals,
and this repository keeps the snapshot. The field and the proposals sections are replaced by
pointers below; git history keeps the earlier text.

## Motivation

Speed reports take one posted limit per report request, defaulting to 25 mph, and apply it to every
vehicle in the sensor's view. At an intersection signed 25 mph one way and 30 mph the other, every
share above the limit is wrong for at least one direction. The
[posted speed limits plan](posted-speed-limits-plan.md) settled the model (a limit belongs to one
direction of one stretch of road, signed in a unit, in force over dates) and waited for road
geometry. That geometry is Gerardus's travel lanes, approved from traffic in the painter.

Reports cannot call Gerardus (ADR-0006), and a report run in March about September must use the
lanes and limits that stood in September. So velocity.report needs its own copy, with history, and
one function that says which lane, direction and limit governed a measurement. Without it no report
can say which limit a speed was measured against.

## Current state

- **No lane concept in the Go code.** The word appears in comments and tuning keys only
  ([lane-stitching plan](lidar-lane-stitching-plan.md), Current state). There is no lane table, no
  lane identifier and no function from a position to a lane.
- **Tracks and their boxes are persisted**, with the class label and per-instant observation
  support (`internal/lidar/l6objects/classification.go`, `internal/lidar/l5tracks/continuity.go`).
- **The painter exists and the web editor is gone.** The macOS street painter marks travel lanes
  (direction as vertex order, tags `lane_type` and `width_m`) and saves them with `EditStreetUse`
  ([hub doc](../ui/visualiser/gerardus-alignment.md#paint-mode-the-street-painter)). The web
  `/lidar/sites` page reads Gerardus's GeoJSON and does not edit.
- **The accepted pose does not reach this repository yet.** `PUT /api/lidar/sites/{l16}/pose` is a
  v0.5.5 item; `CoordinateFrameInfo` is the identity. Tracks are in the sensor frame.
- **Published scenes carry Gerardus street geometry where a pose is accepted**
  (`tools/s2-archive/scene-vectors.py`), but not the travel lanes.
- **Experiment lane0001 is proposed, not run:** a hand-drawn lane map for the kirk placement
  ([lane0001](../../data/experiments/lane/lane0001.md)).
- **Rendered plots of a site do not exist**, and the Hugging Face dataset reserves `derived/maps/`
  with a manifest `map_path` for them ([dataset card](../../tools/s2-archive/huggingface-dataset-README.md)).

## Vocabulary

| Term          | Meaning here                                                                                                                                                                                                           |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Lane field    | The [lanes from tracks plan](lidar-lane-proposals-from-tracks-plan.md)'s per-run raster of distinct tracks, box extents, eight heading bins and a speed sum per 0.25 m cell and class group, no cell under five tracks |
| Proposal      | A `travel_lane` row Gerardus derives from the summed lane fields, graded `high` or `medium`, with an id kept across re-derivation and an append-only review (Gerardus LANE-PROPOSALS §4–5)                             |
| Travel lane   | Gerardus's `TRAVEL_LANE` street-use mark, approved from a proposal or drawn by hand, with its limit tags (Gerardus LANES-AND-LIMITS §3.1)                                                                              |
| Direction     | The lanes of one street that carry traffic the same way, named by street and compass bound ("Kirkham St eastbound"); the unit a report groups speeds by                                                                |
| Limit segment | The posted-limits plan's unit. A lane is split where its limit changes along the street, so a lane is one limit segment; a limit that changes over time is a new version of the same lane                              |
| Lane snapshot | This repository's copy of the lanes Gerardus holds for one deployment, with the pose that places them, written after every save. What attribution and reports read                                                     |

"Segment" on its own is avoided: in this repository it names an annotation time window
(`lidar_segment_selections`), and in Gerardus a city street segment (a CNN).

## Design

### Boundary: who computes what

| Where                                                                         | Does                                                                                                                                                                     | Never does                                                 |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------- |
| Go server, L8 ([lanes from tracks](lidar-lane-proposals-from-tracks-plan.md)) | Builds the lane field per run; serves it; writes it for upload                                                                                                           | Send a track, a trajectory or a per-object sample anywhere |
| Go server (this plan)                                                         | Stores the lane snapshot with history; attributes a position, heading and time to a lane, direction and limit; renders site plots for the dataset                        | Propose or fit lanes; call Gerardus while reporting        |
| macOS visualiser ([lane painter](lidar-visualiser-lane-painter-plan.md))      | Draws the fields and proposals; the operator approves, edits, rejects, sets limits with dates and splits; uploads the field; saves to Gerardus; writes the snapshot here | Fit lanes                                                  |
| Gerardus (W10, W1c)                                                           | Stores lane fields; derives, grades and serves proposals; stores approved lanes and their limit tags; serves the City's speed-limit record                               | See tracks; compute speed statistics                       |
| Web site viewer                                                               | Reads Gerardus's summed field, proposals and lanes, and the snapshot's lanes, read-only                                                                                  | Edit                                                       |

This keeps ADR-0006 (velocity.report reports from a local copy), ADR-0011 (only the k-thresholded
field and the approved geometry leave the device) and D-36.

### The lane field and the proposals: elsewhere

- **The lane field** is specified once, in the [lanes from tracks plan §1](lidar-lane-proposals-from-tracks-plan.md#1-the-lane-field-l8),
  to Gerardus's contract (LANE-PROPOSALS §3). This plan's earlier per-deployment field with sixteen
  heading sectors is withdrawn, so there is one field and one wire format.
- **Proposals** are Gerardus's (LANE-PROPOSALS §4): directions along each city centreline, lateral
  profiles of track density, widths from box extents, edges moved onto traced paint, people's cells
  kept out of the carriageway, grades from tracks, spread, direction agreement and the two
  half-window fields. They keep ids across re-derivation and carry an append-only review. This
  plan's host-side proposals, and the dismissals kept in the app's preferences, are withdrawn.

### The lane snapshot

The snapshot is this repository's copy of the lanes Gerardus holds for one deployment, with the
pose that places them in the sensor frame. Gerardus is authoritative; the snapshot is a cache with
provenance, and a report reads it, never Gerardus.

**Two writers, one table.** The painter writes a snapshot after every successful `EditStreetUse`
([lane painter](lidar-visualiser-lane-painter-plan.md)); `velocity lidar lanes pull --site <l16>`
writes one from Gerardus's `street_use` GeoJSON in `frame=site`, for sites edited elsewhere and for
the archive captures replayed on arrow. Both store the features exactly as Gerardus returned them.

| Record    | Fields                                                                                                                                                                                                                                                                                                                                                     |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Snapshot  | id; site (L16 token) and deployment (the config period today, `deployments` after vocabulary Item 14); writer (`painter` or `pull`); the Gerardus pose record id and the pose itself; Gerardus server version and address; author; the feature rows' digest; `fetched_at`; a `supersedes` link to the previous snapshot                                    |
| Lane      | Gerardus feature id and row id; kind (`travel_lane`, `surface`); geometry in the site frame as returned, and in the sensor frame as derived at write time from the pose; tags as returned (`lane_type`, `width_m`, `street`, `cnn`, `bearing_deg`, `bound`, `direction_name`, `proposal`, `confidence`, the `speed_limit` family with its effective dates) |
| Direction | id; name; bearing; the lanes it groups; the limit when every lane in it agrees at a time, else none and the lanes carry their own                                                                                                                                                                                                                          |

Snapshots are append-only; a new save makes a new snapshot that supersedes the old. The pose in the
snapshot must agree with the pose record the pose writeback stores for the same deployment, once
that lands; a disagreement refuses the snapshot with the two record ids named.

**Directions** are derived at write time: lanes that share a `street` and whose `bearing_deg` lie
within 45° of their circular mean form one direction, named from the street and the compass bound
(Gerardus serves `street`, `cnn`, `bearing_deg` and `bound`; the operator may rename a direction in
the painter, and the name travels as `direction_name` on each lane). Before a deployment has lanes,
reports fall back to the two opposed modes of car headings ([speed by direction](platform-speed-by-direction-reporting-plan.md)).

The snapshot is also the lane map the stitching plan's C2 and C3 ask for: a directed centreline per
lane with a corridor half-width.

#### Limits that change between surveys

A snapshot is a full copy of the lanes current in Gerardus when it was written, so the union of a
deployment's snapshots holds every version of every lane this repository has been told about, each
with its Gerardus row id, its edit time and the date its limit took effect
(`speed_limit_effective_from`) and ended (`speed_limit_effective_to`, optional). The lane's feature
id is the same across versions when only its tags changed, which is what a limit change is.

For a measurement at time `t`, attribution reads the lane version whose effective date is the
latest not after `t`, across snapshots, with the latest edit time breaking a tie. A lane absent from
every snapshot written after some time is treated as ended at the first such snapshot's time unless
a version carries an explicit end. So:

- the City raising a limit between two surveys is one new version of the same lane with its date;
  both surveys attribute correctly and compare lane for lane;
- a change entered late re-attributes the earlier period on the next run, and a report pinned to
  the snapshot ids it read explains the difference;
- a report period spanning the date splits there ([speed by direction](platform-speed-by-direction-reporting-plan.md)).

The snapshot's `GET` answers `history=true` with each lane's versions and effective windows, which
is what the painter's inspector and the survey pages' "limit changed" line read.

### Attribution

`lanegeom`, a neutral package that imports no layer: directed lanes, projection of a point to
(lane, arc length, lateral offset, heading error), membership with hysteresis, and route distance,
shared with the lane-stitching plan's C2. On top of it, one function from a position and heading in
the sensor frame and a capture time to the lane and direction containing it and the limit in force,
or to the reason there is none: no snapshot for that time, outside every lane's corridor, heading
opposed to every lane within reach, or a lane with no limit. The posted-limits plan's step 2 names
this function; the reporting plan says how LiDAR tracks and radar transits use it.

Membership uses the lane's corridor (half its width plus 0.5 m) and a heading within 45° of the
lane's tangent, with hysteresis along a track so a vehicle straddling two lanes does not flicker.

### API

| Method and path                                 | Purpose                                                                                                                                                                                  |
| ----------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PUT /api/lidar/sites/{l16}/lanes`              | Writes a snapshot: the Gerardus street-use features as returned by `EditStreetUse`, the pose record and pose, author, request id (idempotent on the request id, as Gerardus's edits are) |
| `GET /api/lidar/sites/{l16}/lanes`              | The current snapshot, or the one in force at `at`                                                                                                                                        |
| `GET /api/lidar/sites/{l16}/lanes?history=true` | Every version of every lane the snapshots hold, with effective windows and Gerardus row ids                                                                                              |
| `GET /api/lidar/sites/{l16}/lanes/attribute`    | Attribution of one position, heading and time, for the painter's inspector and for tests                                                                                                 |

The lane field's own route, `GET /api/lidar/runs/{run_id}/lane-field`, is the lanes from tracks
plan's.

### What is committed, and what is published

Street data does not live in this repository (the owner, November 3, 2026; D-39): nothing drawn from
a street is committed to git except the survey assets already under `public_html/src/scenes/`,
which D-34 moves to the Hugging Face dataset. Everything else street-shaped belongs in Gerardus, in
the device's database, or in the dataset.

| Thing                                          | Lives in                                                                                     | Never in                       |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------- | ------------------------------ |
| A run's lane field                             | computed on request on the device; uploaded to Gerardus's cell database                      | git; any public export         |
| Proposals and their reviews                    | Gerardus's cell database                                                                     | git                            |
| Lanes, limits and their versions               | Gerardus (authoritative); the device's SQLite snapshots                                      | git; a committed lane map file |
| Experiment lane0002's references               | Gerardus marks drawn in the painter, read at run time; the report carries numbers only       | a committed lane file          |
| Rendered top-down plots of a site              | the Hugging Face dataset, `derived/maps/<capture stem>/`, named in the manifest's `map_path` | `docs/`, `public_html/`, PRs   |
| Per-direction summaries and lanes for a survey | the scene assets, with the survey, moving to the dataset under D-34                          | a second copy anywhere         |

The stitching plan's lane map "committed beside the placement configuration" is replaced by the
snapshot, and lane0001's drawing is done in the painter against Gerardus rather than as a file.

**The render tool.** `velocity lanes render` draws one PNG per site and window: the summed lane
field (vehicle and cycle groups only, people never), the snapshot's lanes with arrowheads, bounds
and limits, and the kerbs and centrelines from the scene's vectors file, north up, with a scale bar
and the field's provenance in the margin. `tools/s2-archive` publishes it with the scene into the
dataset and fills `map_path`. Documents, PRs and hub docs that want a picture of an intersection
link to the dataset URL. The tool runs from the Makefile like the other publishing steps and
refuses an output path inside the checkout.

### Privacy

- Per-object tracks never leave the device (ADR-0011). The field carries counts with no cell under
  five distinct tracks; the snapshot carries geometry, tags and aggregate support.
- The pedestrian field is drawn in the operator tools and reaches Gerardus as counts only; no
  raster of it is published or rendered into the dataset.
- The export privacy test (D-34) gains a case: no lane field or proposal file reaches a public scene.

## Scope

### Item 1: the lane snapshot and attribution

**Summary:** Store approved lanes with their pose and history, derive directions, and attribute a
position, heading and time to a lane, direction and limit.

**Steps:**

1. Migration: snapshot, lane and direction records, JSON-first with generated columns, append-only.
2. `PUT` and `GET` endpoints with `history`; the request-id rule; the pose agreement check once the
   pose record exists; `velocity lidar lanes pull`.
3. `lanegeom` and the attribution function, with its endpoint; membership with hysteresis.
4. Fixtures from the posted-limits plan: a lane split at a sign; opposite directions with different
   limits; a position outside every corridor; a heading opposed to the lane; a time before any
   snapshot; a limit raised between two snapshots, attributed by measurement time on both sides of
   its effective date; a late-entered change re-attributing an earlier time.

**Milestone:** v0.7.19 `M`

### Item 2: experiment lane0002, Gerardus's proposals against drawn lanes

**Summary:** Score Gerardus's `high` proposals against lanes drawn in the painter without the
proposals shown, at kirk and columbus-broadway, so its grade thresholds are set from evidence (the
real check of Gerardus W10e and the lanes from tracks plan's phase E, written as a spec).

**Steps:**

1. Write the spec under `data/experiments/lane/` with its rule before the runs: lateral error per
   lane under 0.30 m against the drawn lane (SPEC-V1 §6's acceptance against an independent
   reference), correct lane count per direction, and the share of drawn lanes a `high` proposal
   finds. The references are Gerardus marks drawn by hand with the proposals hidden.
2. Run, report under `docs/lidar/operations/` with numbers and provenance only, plots published to
   the dataset, and hand the thresholds to Gerardus's LANE-PROPOSALS §4.7 in the same change.

**Milestone:** v0.7.19 `S`

### Item 3: the field, proposals and lanes in the web site viewer

**Summary:** Draw Gerardus's summed field, its proposals by grade and the lanes with their limits in
the read-only site viewer, so a browser can see what the painter saw.

**Steps:**

1. A "Traffic" layer on `/lidar/sites` reading Gerardus's `lane-field` and `lane_proposals`
   endpoints, vehicle and cycle groups only.
2. Lanes from the snapshot drawn with their bound and limit, beside Gerardus's own street use.

**Milestone:** v0.7.19 `S`

### Item 4: rendered intersection plots in the dataset

**Summary:** `velocity lanes render` and its publication beside the scene assets, with `map_path`
filled, so a picture of a site's lanes, limits and traffic exists without a file in git.

**Steps:**

1. The renderer in Go over the field, the snapshot and the vectors file; a golden-image test on a
   synthetic site.
2. The publishing step in `tools/s2-archive`, the manifest field, and a refusal of any output path
   inside the checkout.

**Milestone:** v0.7.19 `S`

## Dependencies

- Gerardus W10c (approved lanes carry `proposal`, `street`, `bearing_deg`) and W1c step 1 (the
  `speed_limit` family with effective dates, `cnn`, `bound`): what the snapshot stores. It stores
  whatever tags Gerardus returns, so the order of landing does not matter.
- The lane painter, the snapshot's first writer; `lanes pull` is the second.
- The accepted pose writeback (v0.5.5) for the pose agreement check; the snapshot carries its own
  pose record so Item 1 does not block on it.
- Vocabulary Item 14 (`deployments`): until it lands the snapshot keys the deployment by site and
  config period.

## Risks

| Risk                                                                           | Likelihood | Impact | Mitigation                                                                                                                                          |
| ------------------------------------------------------------------------------ | ---------- | ------ | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Pose error moves every lane sideways relative to the tracks                    | Medium     | High   | The snapshot names its pose record and refuses one that disagrees with the pose writeback's; a refined pose means a new snapshot                    |
| A snapshot is not written after a save, and reports use stale lanes            | Medium     | Medium | The painter shows a standing notice until it is written; `lanes pull` catches up; every report names the snapshot ids it read                       |
| A late-entered limit change silently changes an old report's numbers on re-run | Medium     | Medium | Reports name their snapshot ids; the change is a new version with a date, so the difference is explainable, not silent                              |
| A proposal is approved without looking and a wrong lane enters the snapshot    | Medium     | Medium | Approved lanes carry `proposal` and `confidence`; lane0002 sets the grades; Gerardus's `lane_against_evidence` names a lane the traffic contradicts |
| Street data creeps into git through a plot, fixture or lane file               | Medium     | Medium | The render tool refuses paths in the checkout; fixtures are synthetic; review against D-39                                                          |

## Design decisions

- **Gerardus proposes; velocity.report reports.** The lane field is uploaded (an aggregate at k = 5,
  allowed by ADR-0011) and Gerardus derives the proposals where the paint, kerbs and intersections
  are (ADR-0016, accepted November 3, 2026). This repository keeps the snapshot.
- **Gerardus is authoritative for lanes; this repository keeps a snapshot.** ADR-0006: the Go
  server needs no network call to report. A snapshot is append-only and names the Gerardus rows it
  copied.
- **A lane is one limit segment at a time.** Lanes are split where the limit changes along the
  street; a limit that changes over time is a new version of the same lane with its effective date.
- **Limits are attributed by measurement time.** The union of append-only snapshots is the history;
  nothing is edited in place.
- **Street data is not committed** (D-39).

## Open questions

- Should the snapshot also keep Gerardus's proposals as they stood at each save, for lane0002's
  provenance, or is the proposal id on each approved lane enough?
- Which runs count for a published survey's directions: the survey's own export run only, as the
  scene timeline does today?
- Does `lanes pull` run on a schedule on a deployed device, or only by hand?

## Checklist

### Complete

- [x] Plan written; reconciled with the unified design (Gerardus ADR-0016) on November 3, 2026
- [x] Item 1: migration 68 (`lidar_lane_snapshots`, `lidar_lane_snapshot_lanes`, append-only by trigger, the limit's value, unit and effective dates as generated columns); `PUT`, `GET`, `?at=`, `?history=true` and `/lanes/attribute` on `/api/lidar/sites/{l16}/lanes`, idempotent on the request id; `velocity lidar lanes pull`; the neutral `internal/lidar/lanegeom` (projection, membership with hysteresis, route distance); `internal/lidar/lanes` with the version in force by measurement time, directions by street and bearing, `Attribute` and `AttributeTrack`; the posted-limits fixtures (`claude/lane-snapshot-attribution`)

### Outstanding

- [ ] Item 1 remainder: the pose agreement check against the pose writeback's record (v0.5.5); `lanes pull` reading the accepted record from Gerardus, which serves none over HTTP (it takes `--record-id` and `--pose` today)
- [ ] Item 2: lane0002's run and report, waiting on the owner's drawing and on lane fields uploaded to each site's Gerardus; the spec, `velocity lidar lanes score` and the report skeleton are written (`claude/lane0002-prep`)
- [ ] Item 3: web site viewer traffic layer (`S`)
- [ ] Item 4: render tool and dataset publication (`S`)

### Deferred

- [ ] Successors and merge transitions in the lane map: [lane-stitching plan](lidar-lane-stitching-plan.md)
- [ ] `LaneField` per road segment as an M5 observation: Gerardus M5

### Accepted residuals (no action planned)

- [ ] Turning traffic at an intersection has no lane; the operator draws turn lanes by hand
