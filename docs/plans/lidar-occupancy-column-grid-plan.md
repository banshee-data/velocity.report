# Occupancy column grid on S2 level 24

- **Status:** Draft
- **Layers:** L4 Perception, L7 Scene, L8 Analytics, L10 Clients (macOS visualiser), storage
- **Target:** v0.5.x for the grid definition and the selection column; occupancy production follows; physics is v2.0+
- **Companion plans:** [Review workflow](lidar-review-workflow-plan.md), [Background region overlays](lidar-background-region-overlay-plan.md), [S2 geographic indexing](s2-geographic-indexing-plan.md), [Static pose alignment](lidar-static-pose-alignment-plan.md), [L7 scene](lidar-l7-scene-plan.md)
- **Canonical:** [geographic-indexing.md](../lidar/architecture/geographic-indexing.md)

## Motivation

Three jobs want the same thing and none of them has it.

- **Selecting points.** The annotation window selects by lasso through a depth slab. A person
  separating a pedestrian from the parked car behind needs a unit smaller than "everything under
  this outline" and larger than one return.
- **Decimated occupancy.** A frame of returns is too much to keep and too sensor-specific to
  compare. What most questions need is coarser: was there something here, this tall, at this
  time.
- **Occupancy as a product.** Where road users dwell, where kerb space is taken and for how long,
  is the kind of answer the project exists to give, and it must be comparable between visits and
  between sensors.

The background model's grid cannot serve any of these. It is polar and centred on the sensor:
its cells are a few centimetres wide near the mast and metres wide at range, and they mean
nothing to a second sensor or a second visit. This plan defines one world-anchored grid, sized
to road users, that all three jobs share.

## Current state

Verified on 2026-09-20.

- **Geographic indexing has two fixed levels.** The
  [style guide](../lidar/architecture/geographic-indexing.md) uses S2 level 13 for database
  indexing and joins, and level 10 for filesystem layout. It states that the WGS84 fix remains
  the authoritative position, that a parent is obtained with `Parent(level)` and never by
  truncating text, and that a cell is not an identity for a site or a session. Its
  implementation plan targets v2.0.
- **The library is already a dependency.** `github.com/golang/geo` is in `go.mod`.
- **Sites are georeferenced by hand.** `tools/s2-archive/site-index.json` gives each site a
  latitude, a longitude, a `north_azimuth_deg` measured by an operator, and a
  `position_confidence`.
- **Packs are not georeferenced.** A pack's `CoordinateContract` describes a sensor-local frame
  in metres, with an origin note and no position on the Earth.
- **Selection is polygon and slab.** `PointSelectionEngine.candidates` evaluates a polygon in an
  orthographic view against an optional depth slab and returns canonical indices with a count of
  what the slab excluded. `apply` combines them with the membership under replace, add or
  subtract, and `MembershipHistory` holds the undo stack. Evaluation and application are separate
  so that the candidate count is shown before anything is committed.
- **Ground is a flat band unless the regional surface is on.** Height filtering uses absolute
  sensor-frame Z. `RegionalGroundSurface`, fitted in 10 m cells, exists on `dd/docs/state-est`
  and not yet on `main`. Marina at Webster measured a 4.35% grade.

## The level, measured

Cell size and the number of cells a footprint touches, computed with `golang/geo` at Columbus and
Broadway, 400 random positions and headings per shape, 5th to 95th percentile:

| Level  | Cell edge          | Pedestrian standing | Pedestrian mid-stride | Cyclist  | Car 4.5 x 1.8 m | Bus 12 x 2.55 m |
| ------ | ------------------ | ------------------- | --------------------- | -------- | --------------- | --------------- |
| 23     | 1.04 m             | 1 to 4              | 1 to 4                | 3 to 6   | 14 to 19        | 46 to 54        |
| **24** | **0.52 m, 1.7 ft** | **2 to 5**          | **4 to 7**            | 9 to 13  | **45 to 52**    | 152 to 167      |
| 25     | 0.26 m             | 6 to 9              | 11 to 13              | 25 to 31 | 153 to 163      | 542 to 568      |

**Level 24** is the level at which a pedestrian fills two to six cells and a car several dozen.
At level 23 a pedestrian can vanish into one cell and a car is a dozen and a half. At level 25 a
pedestrian is already nine.

At this latitude a level 24 cell is 0.517 to 0.521 m on a side and 0.251 m² in area: square to
within a percent. S2 cells vary in size across a cube face by a factor of about two over the
whole Earth, and negligibly across one city. The figures above are for San Francisco and are to
be re-measured, not assumed, for a deployment elsewhere.

## Findings

| Area                            | Current state                                    | Severity | Release view                                              |
| ------------------------------- | ------------------------------------------------ | -------- | --------------------------------------------------------- |
| A unit between return and lasso | None                                             | High     | Selection is slower and less repeatable than it should be |
| World-anchored occupancy        | None; the only grid is polar and sensor-bound    | High     | Nothing is comparable across visits or sensors            |
| Style guide levels              | Two, fixed; no occupancy level                   | Medium   | A third level is an amendment, made explicitly            |
| Pack georeference               | Absent                                           | Medium   | Columns cannot be keyed to S2 without it                  |
| Height above ground             | Sensor-frame Z unless the regional surface is on | Medium   | Three voxels of error at 40 m on a 4% grade               |
| Site fix accuracy               | Hand-measured position and north                 | Medium   | Bounds absolute, not relative, accuracy                   |

## Design / approach

### The column

A **column** is one S2 level 24 cell extended vertically. It is the unit of selection, of
decimated occupancy, and of occupancy production.

A column is divided into **voxels** 0.5 m tall, measured from the local ground. With a 0.52 m
cell that makes each voxel cubic to within a few percent, and inside the one to four foot range
wanted. A column holds eight voxels, from the ground to 4 m:

| Voxel  | Height above ground | Feet        | Typically holds                           |
| ------ | ------------------- | ----------- | ----------------------------------------- |
| 0      | 0 to 0.5 m          | 0 to 1.6    | Kerbs, road surface residue, wheels, feet |
| 1      | 0.5 to 1.0 m        | 1.6 to 3.3  | Car bodies, legs, children, dogs          |
| 2      | 1.0 to 1.5 m        | 3.3 to 4.9  | Car roofs and glass, torsos, cyclists     |
| 3      | 1.5 to 2.0 m        | 4.9 to 6.6  | Heads, SUV and van roofs                  |
| 4      | 2.0 to 2.5 m        | 6.6 to 8.2  | Vans, the lower body of buses and trucks  |
| 5 to 7 | 2.5 to 4.0 m        | 8.2 to 13.1 | Buses, trucks, signs, low branches        |

Eight voxels are eight bits. **A column's state in a frame is one byte**, bit `k` set when voxel
`k` holds at least a threshold number of returns. Returns above 4 m are counted and dropped from
the mask; they are overhead structure for every question asked here.

### The stack, and occupancy in two dimensions

The **stack** is the band of voxels in which road users are found: voxels 1 to 4, from 0.5 m to
2.5 m, which is 1.6 to 8.2 ft. A column is **occupied** when any bit of the stack is set:

```text
occupied = (mask & 0b00011110) != 0
```

Voxel 0 is kept and left out of the test. It is where ground-removal residue, kerbs and road
furniture live, and including it would mark the whole carriageway as occupied on a wet day. The
upper voxels are kept and left out for the opposite reason: a sign gantry is not a vehicle, while
a bus is still caught by the lower four.

This is the simplification wanted: once columns are built, every occupancy question is a test on
a byte per cell on a plane. Presence, dwell, flow across a line, a footprint's cell count and
shape, overlap between two tracks, all become integer and bit operations over a sparse map from
cell to byte, with no floating-point geometry in the inner loop. The height bits remain for the
questions that need them, such as telling a van from a car, without being paid for by the ones
that do not.

The stack's bounds are a parameter, recorded with the data. Voxels 1 to 4 are the proposal; the
band is to be checked against the labelled tracks before it is fixed.

### Planar, deliberately

Road users are assumed to move on a locally planar surface. "Height" is height above the local
ground, not sensor-frame Z, so a column on a hill is still a column. That requires a ground
model: the regional ground surface where it exists, and a single fitted plane per site as the
fallback. Without one, a 4% grade puts a return at 40 m range 1.7 m out in height, which is three
voxels, and the stack test is meaningless. Sensor-frame Z is accepted only for sites measured as
flat, and the grid records which ground model it used.

Gravity, mass, suspension and any model of how a body must move are out of scope, and are a
v2.0+ concern. The grid is built so that they could use it, not so that they are needed.

### Keying, and what the style guide says

The grid adds a third fixed level to the geographic style guide, and the guide is amended to say
so rather than worked around:

- **Level 24** is the occupancy level. Level 13 remains the fine partition for indexing and
  joins, and level 10 the coarse one for the filesystem. A level 24 cell's level 13 and level 10
  ancestors are obtained with `Parent`, never computed separately and never by truncating text.
- **The WGS84 fix remains authoritative.** Columns are derived from sensor-local coordinates
  through the site's position and north azimuth. They are recomputable, and are recomputed when a
  fix is corrected.
- **A cell is not an identity.** Occupancy is keyed by cell and by the `source_id` and
  `calibration_id` of the evidence databases. The same cell under two calibrations is two
  records until a registration says they are the same place.
- **Dense data stores the 64-bit CellID.** The guide's rule to persist canonical tokens is for
  indexed records a person may read. A per-frame grid holds tens of thousands of cells, and
  stores integers, partitioned by the level 13 token the guide already defines. The amendment
  states this exception and its reason.

### Accuracy, relative and absolute

Within one calibration the grid is exactly repeatable: the same return always lands in the same
column. That is what selection, decimation and single-visit occupancy need.

Absolute placement is only as good as the site fix. An error of two metres in position moves
every column by four cells; an error of two degrees in north moves a return at 50 m by 1.7 m,
which is three cells. Both are tolerable within a visit and fatal to comparing two visits cell by
cell. Cross-visit comparison therefore waits on registration, in the
[static pose alignment](lidar-static-pose-alignment-plan.md) plan, and until then is done at a
coarser level or not at all. The grid records the `position_confidence` it was built with.

### Selection: a sphere and a column

Two tools join the lasso and the rectangle in the macOS annotation window. Both are new ways of
evaluating candidates; they feed the existing `apply`, the existing replace, add and subtract
modifiers, and the existing undo stack, and both show their candidate count before committing.

**Sphere.** Click a return to set the centre, snapped to the nearest return under the cursor
within the active slab, so the centre is a real point in three dimensions and not a guess at
depth. Drag or scroll to set the radius. Every return within that distance is a candidate, in
all three dimensions, whichever view the click was made in. Both orthographic views draw the
sphere's outline, so its reach in the axis the operator cannot see is visible. It is the tool
for a compact object in clutter: a pedestrian beside a wall, a head above a car roof.

**Column.** Click in the top view. Every return in the clicked column is a candidate, over the
voxels currently enabled: the stack by default, with each voxel toggled individually, so "this
cell, but not the ground" is one click. A brush radius takes in neighbouring columns; dragging
paints them. With the occupancy layer on, columns are drawn as the grid they are, and a
selection is a set of cells and voxels, which is repeatable in a way a freehand lasso is not. It
is the tool for anything standing on the ground, which is nearly everything.

Where a pack has no georeference, the column tool uses a local grid of the same pitch anchored
at the pack's origin. The selection behaves identically, and the pack records that its columns
are local and not S2 cells. Masks are still stored as canonical point indices, as they are
today: a column is how points are chosen, never how membership is recorded, so a mask does not
change meaning if the grid does.

Each tool also deselects. The modifier that subtracts with the lasso subtracts with the sphere
and the column.

### As a layer

The occupancy grid is a layer like the others in the
[review workflow](lidar-review-workflow-plan.md): toggled independently in the main visualiser
and the annotation window, off by default, remembered per user, with a legend. Drawn as cell
outlines on the ground, filled where occupied, with the voxel mask shown for the column under the
cursor. Pass 1 of the review workflow reads a track's footprint in cells, its cell count over
time and its height mask as evidence: two to six cells and a tall, narrow mask is a pedestrian
before any classifier is asked.

## Scope

### Item 1: the grid, defined and measured

**Summary:** Level 24 columns, eight 0.5 m voxels, the stack, and the style guide amended to match.

**Steps:**

1. Amend the geographic style guide: the third level, its purpose, its parents, and the integer
   storage exception.
2. A small Go package: sensor-local point to cell and voxel, given a site fix and a ground model;
   column mask from a set of returns.
3. Commit the measurement above as a test, so the level's fitness is checked and not remembered.
4. Re-measure the stack bounds against the labelled tracks before fixing them.

**Milestone:** v0.5.x

### Item 2: sphere and column selection in the macOS tool

**Summary:** Two candidate evaluators beside the polygon, sharing apply, modifiers, undo and the count.

**Steps:**

1. Sphere: centre snapped to a return, radius by drag or scroll, outline in both views.
2. Column: click and paint in the top view, per-voxel toggles, brush radius.
3. Local-grid fallback for packs with no georeference, recorded in the pack.
4. Deselection through the existing subtract modifier, and both tools in the undo stack.
5. Tests beside the existing selection tests: a sphere selects by true distance, a column ignores
   disabled voxels, and neither changes how a mask is stored.

**Milestone:** v0.5.x

### Item 3: georeference and ground in the review pack

**Summary:** A pack carries the site fix, north, confidence and ground model it was cut with, as context.

**Steps:**

1. Context entry outside the point digest, so adding it invalidates no annotation.
2. Ground model reference: regional surface, single plane, or sensor-frame Z, stated.
3. The occupancy layer in both macOS views.

**Milestone:** v0.5.x

### Item 4: decimated occupancy

**Summary:** A byte per occupied column per frame, as a sparse map, recorded alongside a run.

**Steps:**

1. Builder from foreground returns, off the frame path or within its budget.
2. Storage keyed by source, calibration and cell, partitioned by the level 13 token.
3. Determinism: two replays, identical bytes.
4. Size measured on columbus-broadway before any retention decision.

**Milestone:** v0.5.x, after items 1 and 3

### Item 5: occupancy as a product

**Summary:** Dwell, presence and flow from the two-dimensional test, per site.

**Milestone:** After item 4 and after cross-visit registration; not scheduled here.

## Dependencies

- **The regional ground surface** (PR #559) for any site that is not flat.
- **The review workflow's context layers**, for the pack's georeference and the occupancy layer.
- **Site fixes** in the archive index, with their stated confidence.
- **Static pose alignment**, before any cross-visit comparison.

## Risks

- **A grid that looks more exact than it is.** Cell IDs are precise to the bit and the site fix
  is not. The confidence is recorded and shown, and cross-visit use is gated on registration.
- **The stack fitted to San Francisco.** Voxels 1 to 4 suit cars and adults on city streets. A
  site with trucks, or one where children matter most, may want other bounds, which is why they
  are a recorded parameter and not a constant.
- **Columns hiding what a lasso would catch.** A pedestrian leaning on a car shares a column with
  it. The column tool is quick, not exact; the sphere and the lasso remain, and membership is
  still stored point by point.
- **A second spatial index to keep in step.** The polar background grid and this one answer
  different questions and stay separate. Nothing here replaces the background model.

## Checklist

### Outstanding

- [ ] Style guide amendment: level 24, parents, integer storage (`S`)
- [ ] Grid package and the level-fitness test (`M`)
- [ ] Stack bounds checked against labelled tracks (`S`)
- [ ] Sphere selection (`M`)
- [ ] Column selection with per-voxel toggles and paint (`M`)
- [ ] Pack georeference and ground model context (`M`)
- [ ] Occupancy layer in both macOS views (`M`)
- [ ] Decimated occupancy builder, storage and size measurement (`L`)

### Deferred

- [ ] Occupancy products: dwell, presence, flow
- [ ] Cross-visit comparison, after registration
- [ ] Physics: gravity, mass and motion constraints, v2.0+

### Accepted residuals (no action planned)

- [ ] Cell sizes are measured for San Francisco and re-measured per deployment
- [ ] Returns above 4 m are counted and not masked
