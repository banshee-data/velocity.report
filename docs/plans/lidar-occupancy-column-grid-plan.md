# Occupancy column grid: a 0.5 m lattice anchored by S2

- **Status:** Draft
- **Layers:** L4 Perception, L7 Scene, L8 Analytics, L10 Clients (macOS visualiser), storage
- **Target:** v0.5.x. The brushes and the local grid are delivered on the annotation branch (PR #579), where the rest of the annotation tool is; occupancy production follows; physics is v2.0+
- **Companion plans:** [Review workflow](lidar-review-workflow-plan.md), [Background region overlays](lidar-background-region-overlay-plan.md), [S2 geographic indexing](s2-geographic-indexing-plan.md), [Spatial priors service](spatial-priors-service-plan.md), [Reference data](spatial-priors-reference-data-plan.md), [L7 scene](lidar-l7-scene-plan.md)
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
nothing to a second sensor or a second visit. This plan defines one grid, sized to road users,
that all three jobs share.

It has to work before we know where we are. At scan time there is no registration to the real
world: a rough position, good to about a city block, and nothing better until an offline
alignment has been run. So the grid is local first. An S2 cell says roughly where it is, a fixed
spacing says how it is laid out, and the two are meshed with reality later, without anything
already selected or counted changing its meaning.

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
- **A capture is not registered when it is made.** The rough position available at scan time is a
  latitude and longitude to three decimal places. Registration to reality comes from offline
  alignment against priors, in the [spatial priors service](spatial-priors-service-plan.md) plan,
  whose rule is that a source with unresolved georeferencing "may remain a local experiment
  asset; its guessed position must not create verified public coverage".
- **Dense S2 storage is already discouraged.** That plan uses "local metric tiles" for dense
  processing, not S2 areas, and notes that a signed 64-bit column cannot hold every S2 ID. San
  Francisco's IDs have the top bit set.
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

## The spacing, measured

Two things were measured with `golang/geo` at Columbus and Broadway: what an S2 cell of the right
size looks like there, and how many cells a road user touches on a grid of that size. Footprint
counts are from 400 random positions and headings per shape, 5th to 95th percentile.

**S2 cells of the right size are not square.** A level 24 cell there has edges of 0.517 m and
0.521 m, which looks square, and an interior corner of 111.3 degrees, which is not. One family of
edges runs exactly north to south, along meridians; the other bears 68.7 degrees. The cells are
parallelograms. A square lattice laid over them drifts by 75 cells in 100 m, so the two can
never be made to coincide, at any offset or rotation.

**A square lattice of the same size does the job better.**

| Grid                | Pedestrian standing | Pedestrian mid-stride | Cyclist  | Car 4.5 x 1.8 m | Bus 12 x 2.55 m |
| ------------------- | ------------------- | --------------------- | -------- | --------------- | --------------- |
| S2 level 24, skewed | 2 to 5              | 4 to 7                | 9 to 13  | 45 to 52        | 152 to 167      |
| Square, 0.4 m       | 3 to 6              | 5 to 8                | 12 to 16 | 65 to 73        | 226 to 239      |
| **Square, 0.5 m**   | **2 to 4**          | **4 to 6**            | 8 to 13  | **44 to 51**    | 151 to 162      |
| Square, 0.6 m       | 2 to 4              | 3 to 6                | 7 to 10  | 32 to 38        | 109 to 119      |

At **0.5 m** a pedestrian fills two to six cells and a car about four dozen, which is the target.
It matches level 24 in area (0.250 m² against 0.251 m²), so nothing about the scale changes, and
with 0.5 m voxels the boxes are exactly cubic, which skewed cells could not be.

**What a rough fix supports.** A thousandth of a degree there is 111 m north to south and 88 m
east to west, so a fix rounded to three places is wrong by up to 71 m. Against S2 levels:

| Level  | Cell edge | True position in the same cell as the rounded fix | In that cell or its 8 neighbours |
| ------ | --------- | ------------------------------------------------- | -------------------------------- |
| 13     | 1,059 m   | 96.0%                                             | 100%                             |
| 14     | 530 m     | 91.0%                                             | 100%                             |
| 15     | 265 m     | 82.4%                                             | 100%                             |
| **16** | **132 m** | **63.5%**                                         | **100%**                         |
| 17     | 66 m      | 36.5%                                             | 100%                             |
| 18     | 33 m      | 10.6%                                             | 78.4%                            |

**Level 16** is the rough anchor: its cell is the size of the fix's own uncertainty, about one
intersection and its approaches, and the true position is always in it or a neighbour. No level
makes the rounded fix's own cell certain, not even level 13, so a rough anchor always means
"this cell and the eight around it".

These figures are for San Francisco. Cell size and skew vary over the Earth, and are re-measured
for a deployment elsewhere, not assumed.

## Findings

| Area                            | Current state                                     | Severity | Release view                                              |
| ------------------------------- | ------------------------------------------------- | -------- | --------------------------------------------------------- |
| A unit between return and lasso | None                                              | High     | Selection is slower and less repeatable than it should be |
| World-anchored occupancy        | None; the only grid is polar and sensor-bound     | High     | Nothing is comparable across visits or sensors            |
| Registration at scan time       | None; a fix good to about 71 m                    | High     | The grid must work before the capture is placed           |
| S2 cells as columns             | Parallelograms with a 111 degree corner here      | High     | Use S2 to anchor, a square lattice to subdivide           |
| Style guide levels              | Two, fixed; no rough anchor level                 | Medium   | Level 16 is an amendment, made explicitly                 |
| Height above ground             | Sensor-frame Z unless the regional surface is on  | Medium   | Three voxels of error at 40 m on a 4% grade               |
| Rough north                     | Operator-measured where present, absent otherwise | Medium   | Provisional lattice orientation is arbitrary; harmless    |

## Design / approach

### The column

A **column** is one cell of a square lattice with a 0.5 m pitch, extended vertically. It is the
unit of selection, of decimated occupancy, and of occupancy production.

A column is divided into **voxels** 0.5 m tall, measured from the local ground. Each is a 0.5 m
cube, 1.64 ft on a side, inside the one to four foot range wanted. A column holds eight voxels,
from the ground to 4 m:

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

### Two frames: provisional, then registered

A grid is a lattice in a frame, and a capture has two frames in its life.

**Provisional, from the moment of the scan.** The frame is the sensor's own: metres, origin at
the sensor, axes as the sensor has them, or turned to the operator's rough north where one was
measured. The lattice is laid in that frame at a 0.5 m pitch, with a lattice point at the
origin. The grid records its **rough anchor**: the S2 level 16 cell of the three-decimal fix.
That is enough to say which intersection this probably is, to file the capture, and to fetch
the right priors. It is not enough to say which cell on the Earth a column is, and nothing
pretends otherwise. A provisional grid is fully usable for selection, for decimated occupancy,
and for every within-capture question, because within one frame it is exactly repeatable: the
same return always lands in the same column.

**Registered, after offline alignment.** Registration, described in the
[spatial priors service](spatial-priors-service-plan.md) plan, produces a rigid transform from
the sensor's frame to an east, north, up frame at a declared geodetic origin, with its
uncertainty and the priors it was fitted to. The registered lattice is laid in that frame, still
at 0.5 m, with its lattice points at whole multiples of the pitch from the origin of the priors
tile. Every capture registered to the same tile therefore shares one lattice, and their columns
are the same columns.

**Meshing the two is recomputation, never relabelling.** A provisional column and a registered
column differ by a translation of up to tens of metres and a rotation of whatever the rough
north was wrong by, so no provisional column maps onto one registered column. Columns and
occupancy are derived data, and are rebuilt from retained returns and observations under the
registered transform. Where only the provisional bytes were kept, they are resampled, and the
result is marked as resampled. What never needs rebuilding is an annotation: a mask is a list of
point indices, and means the same thing in either frame.

Columns are keyed by a grid identity and two small signed integers, `(grid, i, j)`. The grid
identity names the frame, its status (`provisional`, `registered`, `manually_placed`), the pitch,
and the evidence databases' `source_id` and `calibration_id`. Provisional and registered data
are never mixed in one comparison, and cross-visit comparison is allowed only between grids
registered to the same priors tile.

### What this asks of the geographic style guide

Less than the first draft of this plan did. Columns are not S2 cells, so no occupancy level is
added. One thing is:

- **Level 16 is the rough anchor level**, for a capture that has a rough fix and no
  registration. It is recorded with its status, and always read as "this cell or one of its
  eight neighbours". Its level 13 and level 10 ancestors come from `Parent`, as the guide
  requires, never from truncating text.
- **The WGS84 fix remains authoritative**, and the rough fix is recorded as the rough fix it is:
  three decimal places, with its error bound, never padded with zeros to look like a survey.
- **A cell is not an identity.** A rough anchor says where to look, not which site this is.

Registered grids need nothing new: they are indexed at level 13 like everything else, from the
registered origin.

### Accuracy: what each stage can promise

| Stage       | Placement on the Earth                        | Good for                                                         |
| ----------- | --------------------------------------------- | ---------------------------------------------------------------- |
| Provisional | Within about 71 m, heading unknown or rough   | Selection, decimation, all within-capture occupancy              |
| Registered  | As good as the alignment and the priors allow | Cross-visit and cross-sensor comparison, on the same priors tile |

Comparing two visits column by column needs them to agree to within half a column: 0.25 m in
position, and about 0.3 degrees in heading for returns at 50 m. Whether alignment to public
LiDAR reaches that is not yet known, and is one of the things the registration experiment
measures. Two captures registered to the same priors release share that release's errors, so
they may agree with each other better than either agrees with the Earth; that is a hypothesis to
test and not a result.

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

The column tool needs no registration. It works on the provisional lattice from the first
moment a pack exists, which is when annotation actually happens. Masks are still stored as
canonical point indices, as they are today: a column is how points are chosen, never how
membership is recorded, so a mask means the same thing after the capture is registered as it did
before.

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

**Summary:** A 0.5 m square lattice, eight 0.5 m voxels, the stack, and a level 16 rough anchor.

**Steps:**

1. Amend the geographic style guide: level 16 as the rough anchor level, its status, its
   "cell or eight neighbours" reading, and how a rough fix is recorded.
2. A small Go package: point to column and voxel, given a frame and a ground model; column mask
   from a set of returns; grid identity with its status.
3. Commit the measurements above as tests, so the spacing's fitness and the anchor's containment
   are checked and not remembered.
4. Re-measure the stack bounds against the labelled tracks before fixing them.

**Milestone:** v0.5.x

### Item 2: sphere and column selection in the macOS tool

**Status:** Delivered on the annotation branch in `305bc1bd3`, with the local lattice, per-voxel
toggles, a planar ground estimated when a pack opens, and the bracket keys for brush size. One
departure from the design above: a brush adds by default and subtracts with option, where the
lasso still replaces. A dab that erased the dab before it is not painting.

**Summary:** Two candidate evaluators beside the polygon, sharing apply, modifiers, undo and the count.

**Steps:**

1. Sphere: centre snapped to a return, radius by drag or scroll, outline in both views.
2. Column: click and paint in the top view, per-voxel toggles, brush radius.
3. Both work on the provisional lattice, so neither waits on registration.
4. Deselection through the existing subtract modifier, and both tools in the undo stack.
5. Tests beside the existing selection tests: a sphere selects by true distance, a column ignores
   disabled voxels, and neither changes how a mask is stored.

**Milestone:** v0.5.x

### Item 3: frame and ground in the review pack

**Summary:** A pack carries its grid identity, rough anchor, frame status and ground model, as context.

**Steps:**

1. Context entry outside the point digest, so adding it, or registering the capture later,
   invalidates no annotation.
2. Ground model reference: regional surface, single plane, or sensor-frame Z, stated.
3. The occupancy layer in both macOS views, with the frame's status shown beside it.

**Milestone:** v0.5.x

### Item 4: decimated occupancy

**Summary:** A byte per occupied column per frame, as a sparse map, recorded alongside a run.

**Steps:**

1. Builder from foreground returns, off the frame path or within its budget.
2. Storage keyed by grid identity and `(i, j)`; the capture indexed at level 13 as usual.
3. Determinism: two replays, identical bytes.
4. Size measured on columbus-broadway before any retention decision.

**Milestone:** v0.5.x, after items 1 and 3

### Item 5: meshing with reality

**Summary:** Rebuild a capture's columns and occupancy on the registered lattice once it has been aligned.

**Steps:**

1. Consume a registration record from the priors service plan; lay the registered lattice on the
   priors tile's origin.
2. Rebuild from retained returns and observations; resample, and say so, where only bytes remain.
3. A test that annotations are byte-identical before and after.

**Milestone:** After the registration experiment reports; not scheduled here.

### Item 6: occupancy as a product

**Summary:** Dwell, presence and flow from the two-dimensional test, per site, across visits.

**Milestone:** After item 5; not scheduled here.

## Dependencies

- **The regional ground surface** (PR #559) for any site that is not flat.
- **The review workflow's context layers**, for the pack's grid identity and the occupancy layer.
- **Registration from priors**, in the [spatial priors service](spatial-priors-service-plan.md)
  plan, before any cross-visit comparison. Nothing else here waits on it.

## Risks

- **A provisional grid mistaken for a placed one.** It looks the same on screen. The frame's
  status is part of the grid's identity, shown wherever the grid is, and provisional and
  registered data are never combined.
- **Registration that never reaches half a column.** Then cross-visit comparison is done on
  coarser blocks of columns, and the plan says so rather than comparing cells that do not
  correspond.
- **The stack fitted to San Francisco.** Voxels 1 to 4 suit cars and adults on city streets. A
  site with trucks, or one where children matter most, may want other bounds, which is why they
  are a recorded parameter and not a constant.
- **Columns hiding what a lasso would catch.** A pedestrian leaning on a car shares a column with
  it. The column tool is quick, not exact; the sphere and the lasso remain, and membership is
  still stored point by point.
- **A second spatial index to keep in step.** The polar background grid and this one answer
  different questions and stay separate. Nothing here replaces the background model.

## Checklist

### Complete

- [x] Sphere selection, centred on a snapped return, drawn in both views
- [x] Column selection with per-voxel toggles, paint, and gap-free fast strokes
- [x] Local 0.5 m lattice in the annotation window, with ground and voxel bands in the side views
- [x] Bracket keys for brush size

### Outstanding

- [ ] Style guide amendment: level 16 rough anchor (`S`)
- [ ] Grid package, grid identity, and the spacing and containment tests (`M`)
- [ ] Stack bounds checked against labelled tracks (`S`)
- [ ] Pack frame, rough anchor and ground model context (`M`)
- [ ] Occupancy layer in both macOS views, showing frame status (`M`)
- [ ] Decimated occupancy builder, storage and size measurement (`L`)

### Deferred

- [ ] Occupancy products: dwell, presence, flow
- [ ] Meshing with reality, and cross-visit comparison, after the registration experiment
- [ ] Physics: gravity, mass and motion constraints, v2.0+

### Accepted residuals (no action planned)

- [ ] S2 cell size and skew are measured for San Francisco and re-measured per deployment
- [ ] A provisional grid is never promoted in place; registration builds a new one
- [ ] Returns above 4 m are counted and not masked
