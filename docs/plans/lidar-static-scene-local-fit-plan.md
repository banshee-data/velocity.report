# LiDAR static scene and local vector fit plan

- **Status:** Draft
- **Layers:** L7 Scene (offline fit, Go), annotation packs, macOS annotation tool
- **Target:** branch `dd/lidar/static-scene-fit`, stacked on the annotation tool (PR #579). The
  result is surfaced in the annotation flow, so it cannot start before that lands.
- **Companion plans:** [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md),
  [spatial-priors-service-plan.md](spatial-priors-service-plan.md),
  [static-sensor-nudge-tolerance-plan.md](static-sensor-nudge-tolerance-plan.md),
  [lidar-point-annotation-and-object-dataset-plan.md](lidar-point-annotation-and-object-dataset-plan.md),
  [lidar-occupancy-column-grid-plan.md](lidar-occupancy-column-grid-plan.md) <!-- link-ignore -->,
  [lidar-review-workflow-plan.md](lidar-review-workflow-plan.md) <!-- link-ignore -->
- **Canonical:** [vector-scene-map.md](../lidar/architecture/vector-scene-map.md)

## Motivation

An annotation pack is one batch of frames cut from one recording, and everything an operator
records against it is bound to that pack's digest. That is right for point masks: a point
index means nothing under different bytes. It is wrong for the street. The road, the
buildings and the signs are the same in every batch from a site, and today each batch meets
them as if for the first time: nothing identified in one pack is there when the next is
opened, and there is no way to see whether two batches agree about where the road is.

The consequence is already visible in the tool. The column brush takes its ground from the
fifth percentile of one sample's heights, which is a single level plane under a sensor assumed
to be level. On a street with a crown or a gradient, "0.5 m above the ground" means a
different slice of a car at each end of the view. And when a batch looks wrong, an operator
cannot tell a nudged sensor from a bad background settle from a genuinely different scene,
because there is nothing fixed to compare the batch against.

This plan adds that fixed thing: a small per-site model of the static scene made of simple
vector primitives, fitted to LiDAR points in the sensor's own local frame, carried from batch
to batch, and used to show each batch's agreement with it. The road comes first, then
buildings, then poles and signs.

## Current state

- Pack points are in the sensor frame. The exporter's `origin_note` says so: sensor origin as
  recorded, no site transform applied
  ([export.go](../../internal/lidar/annotation/export.go)).
- The sidecar is per pack and digest-bound
  ([sidecar.go](../../internal/lidar/annotation/sidecar.go)). Nothing in the annotation
  directory outlives a pack.
- The macOS column grid's ground is `ColumnGrid.estimateGroundZ`: the fifth percentile of one
  sample's z, one number, no slope
  ([BrushSelection.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/BrushSelection.swift)).
- The runtime's ground handling is `HeightBandFilter`, a fixed floor and ceiling in the sensor
  frame ([ground.go](../../internal/lidar/l4perception/ground.go)). It removes ground; it does
  not model it.
- The only persistent static model is the L3 background grid: a learned range per polar cell.
  It knows where the static scene is along each beam and nothing about what shape it is.
- The shapes are already designed and not built. [vector-scene-map.md](../lidar/architecture/vector-scene-map.md)
  defines ground, structure and volume features;
  [the ground-plane maths proposal](../../data/maths/proposals/20260221-ground-plane-vector-scene-maths.md)
  gives the plane estimators, including the offline robust refinement this plan uses; L7 is
  unimplemented ([lidar-l7-scene-plan.md](lidar-l7-scene-plan.md)).
- The priors plans cover getting from a rough fix to a registered capture. That is global
  registration, and it is out of scope here.

## Findings

| Area                | Current state                                          | Severity | Release view                 |
| ------------------- | ------------------------------------------------------ | -------- | ---------------------------- |
| Cross-batch memory  | None: every record is bound to one pack's digest       | High     | Blocks consistent review     |
| Ground model        | One level plane from one sample's fifth percentile     | High     | Wrong on any sloped street   |
| Batch agreement     | Not measurable: nothing fixed to measure against       | Medium   | Nudges and bad settles hide  |
| Static objects      | Not representable: only per-frame point masks exist    | Medium   | Re-identified in every batch |
| Vector scene design | Designed in the canonical doc, maths proposed, no code | Low      | This plan is its first slice |
| Global registration | Owned by the priors plans                              | n/a      | Out of scope                 |

## Design / approach

### The scene is a file beside the packs, in the sensor's local frame

A **scene** is a revisioned JSON document in the annotation directory, under
`scenes/<scene_id>/scene.json`, written with the same lock, temporary file and revision rules
as the sidecar store. It holds primitives and the batches that have been compared with it. It
is not bound to a pack digest, because it describes the street, not a set of bytes.

Its frame is the sensor frame of the batch it was first fitted from, the **reference batch**.
No latitude, no S2 cell, no site transform. Two batches from an unmoved sensor share that
frame exactly. Two batches either side of a nudge differ by a small rigid transform, and
finding that transform is Item 4, not an assumption.

A pack's sidecar gains two optional fields: the `scene_id` it was compared with, and the
`batch_alignment` found for it. Pack points are never transformed in place. They are
immutable, and an alignment is a recorded transform that display and residuals apply.

### Primitives, in the order they are built

All three are the canonical doc's feature classes at their simplest level of detail.

1. **Ground patch.** A bounded plane: unit normal, offset, and an outline polygon in the
   plane's own coordinates. Class `road` first, then `pavement`. One patch to begin with; a
   crowned or graded street becomes several patches that meet along a line, which is the
   "simplified mesh" and also where the kerbs are.
2. **Wall.** A vertical plane patch: a line segment in plan and a height range. Buildings,
   retaining walls, fences.
3. **Pole.** A vertical segment with a radius: sign posts, lamp posts, signal poles. A sign
   face is a small wall on a pole, and is left until poles work.

Every primitive carries the annotation model's `ReviewStatus` and `Provenance`, with
`Algorithm` and `AlgorithmVersion` when a proposer made it. That puts scene primitives in the
same propose-then-grade workflow as masks: code proposes a road plane, a person accepts,
adjusts or rejects it, and only a reviewed primitive is used to judge a batch.

### Fitting

The fit of record is Go, in a new `internal/lidar/l7scene` package, offline, deterministic
for a given pack and seed, and tested against synthetic planes with known noise.

- **Which points.** Static returns only. A pack's class byte already separates background and
  ground from foreground; foreground never enters a fit. Points are pooled over the batch's
  samples, so a car parked for the whole batch is the remaining risk (see Risks).
- **Road.** Candidate points are the pooled static returns in the lowest height band. A
  robust plane fit (RANSAC for the consensus set, then least squares on the inliers, as the
  maths proposal's offline refinement) gives the first patch. The outline is the concave hull
  of the inliers on the 0.5 m column lattice. Splitting into several patches is region growing
  over lattice tiles by normal angle and offset, and is a later step than one good plane.
- **Walls.** Static returns well above the road are projected to plan. Walls are the line
  segments that many of them fall on; each is extruded over the height range of its inliers.
- **Poles.** Static clusters that are narrow in plan and tall.
- **From a selection.** "Fit plane to selection" takes the points an operator has just
  brushed. It is the manual path for a wall the proposer missed, and it reuses the brushes.

### Residuals: what "aligned" means

For a batch and a reviewed primitive, the residual of a point is its signed distance to the
primitive, for points within the primitive's outline and within 0.5 m of it. Per batch, per
primitive, the scene records the count, median, MAD, 95th percentile of the absolute value,
the fraction within ±5 cm, and a histogram in 1 cm bins over ±0.5 m.

Those numbers are the product. A batch that agrees with the scene has a narrow histogram
centred on zero for every primitive. A sensor that dropped 3 cm moves the road's histogram
and leaves the walls'. A yaw nudge splits the walls' histograms by their distance from the
sensor and leaves the road's. A bad background settle widens everything.

### Local alignment, not registration

Given reviewed primitives, a batch's alignment is the small rigid transform that minimises
point-to-plane distance to them: the linearised point-to-plane least squares that ICP uses,
without the correspondence search, because the primitives say which plane a point belongs to.

What is observable depends on what has been fitted, and the tool must say so rather than
report a full transform it cannot know. A road alone fixes height, roll and pitch. Two walls
that are not parallel add x, y and yaw. With only a road, the report is "3 cm low, 0.4° nose
down" and nothing about yaw.

### In the annotation flow

- **Scene section in the pane.** Choose or create the scene for this pack; "Propose road"
  from this batch; the list of primitives with their status; accept, reject, delete.
- **Drawing.** Primitives are outlines in the top view and lines in the front and side
  views, drawn through the same `OrthoViewport` as the points, and wire patches in the 3D
  view through the renderer's existing line pipeline.
- **Colour mode.** A picker beside the class toggles: by class (today), by height above the
  road, by distance to the scene. Distance uses a diverging ramp clamped at ±0.2 m, so a
  batch that sits low is visibly one colour. Points near no primitive keep their class colour.
- **Histogram.** For the selected primitive, this batch's residual histogram over the
  reference batch's, with the numbers above beside it.
- **Ground from the plane.** `ColumnGrid` takes its ground height from the road patch at each
  column instead of one number, which is what makes voxel 1 mean the same slice of a car
  along a graded street.
- **Static objects persist.** A wall or pole reviewed in one pack is drawn when any pack that
  names the same scene is opened.

Signed distance to a plane is one dot product, so the Swift client computes display distances
itself rather than asking the server for seventy thousand numbers per sample. The statistics
of record stay in Go. A shared fixture pins the two to the same answers, as
`swift_fixture_test.go` already does for pack decoding.

### Research threads

Named so they can be pulled through the repository's citation tooling before any of them is
relied on. Three are already cited in [ground-plane-maths.md](../../data/maths/ground-plane-maths.md):
RANSAC (Fischler and Bolles, 1981), polar line-fit ground extraction (Zermas et al., 2017)
and Patchwork's concentric-zone ground model (Lim et al., 2021), the last being the most
directly relevant to a spinning sensor over a crowned road. To verify and add: MSAC and
MLESAC as better-scored consensus; region-growing plane segmentation of point clouds;
efficient RANSAC for multiple primitive shapes; the linearised point-to-plane ICP step;
Manhattan-world assumptions for walls; building reconstruction from footprints plus points;
and normal distributions transform cells as an alternative residual representation. None of
these is needed for Item 2.

## Scope

### Item 1: scene file and store

**Summary:** A revisioned scene document beside the packs, readable by both clients.

**Steps:**

1. Go types for scene, primitive (ground patch first) and batch comparison; JSON schema
   version 1; validation that rejects a non-unit normal or a degenerate outline.
2. Store with the sidecar store's lock, atomic write and revision rules; path validation
   through `ResolvePathWithinDirectory`.
3. Sidecar gains optional `scene_id` and `batch_alignment`; old sidecars still load.
4. Swift reader, with a Go-written fixture pinned in a test.

**Milestone:** first commit series on `dd/lidar/static-scene-fit`

### Item 2: road proposer and residuals

**Summary:** Fit one road plane from a pack's static returns and measure a batch against it.

**Steps:**

1. `l7scene` plane fit: seeded RANSAC plus inlier least squares; synthetic tests for a
   level, a tilted and a noisy plane, and for a plane with a parked-car-sized outlier block.
2. Outline from inliers on the 0.5 m lattice.
3. Residual statistics and histogram for a batch against a primitive.
4. CLI `velocity lidar scene-propose-road` and `velocity lidar scene-compare`, named as
   `annotation-export` is, and the matching API routes the macOS client calls.

**Milestone:** same branch, after Item 1

### Item 3: the scene in the annotation tool

**Summary:** Draw the scene, colour by distance, show the histogram, take ground from the plane.

**Steps:**

1. Scene section, drawing in all three views, accept and reject.
2. Colour mode picker and the distance ramp; Swift distance pinned to the Go fixture.
3. Histogram panel.
4. `ColumnGrid` ground from the road patch, with the single-number estimate as the fallback
   when a pack has no scene.

**Milestone:** same branch, after Item 2

### Item 4: batch-to-scene local alignment

**Summary:** Report the small rigid transform between a batch and the scene, and only the
degrees of freedom the fitted primitives can determine.

**Steps:**

1. Linearised point-to-plane least squares over reviewed primitives; observability from the
   rank of the normal equations, reported per axis.
2. Recorded in the sidecar, applied to display and residuals, never to pack bytes.
3. The report feeds the nudge-tolerance plan's thresholds.

**Milestone:** follows Item 3

### Item 5: walls

**Summary:** Propose vertical plane patches from static returns above the road, and fit one
from a brushed selection.

**Milestone:** follows Item 4; this is what makes x, y and yaw observable

### Item 6: poles and signs

**Summary:** Propose vertical segments from narrow, tall static clusters.

**Milestone:** follows Item 5

## Dependencies

- PR #579 (the annotation tool, brushes and navigable views) lands first.
- Packs with classification. A pack recorded without point classes cannot separate static
  returns, and the proposer refuses it rather than fitting through traffic.
- Full-scene packs. A foreground-only pack has no road in it.

## Risks

| Risk                                             | Likelihood | Impact | Mitigation                                                                               |
| ------------------------------------------------ | ---------- | ------ | ---------------------------------------------------------------------------------------- |
| Crown or gradient read as misalignment           | High       | Medium | Residual map shown before any alignment; split into patches before trusting one plane    |
| A vehicle parked all batch is fitted as static   | Medium     | Medium | Proposals start unreviewed; outlier-block test; operator rejects                         |
| Scene attached to the wrong site's pack          | Medium     | High   | Comparison refuses when the road inlier fraction is low, and says so                     |
| Go and Swift distance maths drift                | Low        | Medium | Shared fixture test in both suites                                                       |
| Reported alignment overstates what is known      | Medium     | High   | Observability from rank; unobservable axes are reported as unknown, never as zero        |
| Scope creep into registration and georeferencing | Medium     | Medium | Frame is the reference batch's sensor frame; anything global belongs to the priors plans |

## Checklist

### Complete

- [x] Annotation views that hold their framing, a 3D view, class toggles and frame sync:
      the surface this plan draws on (PR #579)

### Outstanding

- [ ] Item 1: scene file and store (`M`)
- [ ] Item 2: road proposer and residuals (`M`)
- [ ] Item 3: the scene in the annotation tool (`M`)
- [ ] Item 4: batch-to-scene local alignment (`M`)
- [ ] Item 5: walls (`L`)
- [ ] Item 6: poles and signs (`M`)
- [ ] Verify and add the research-thread citations (`S`)

### Deferred

- [ ] Priors as proposals: OSM road axes and building footprints placed in the local frame as
      unreviewed primitives, tracked by [spatial-priors-service-plan.md](spatial-priors-service-plan.md)
- [ ] Global registration and georeferencing, tracked by
      [spatial-priors-service-plan.md](spatial-priors-service-plan.md)
- [ ] Multi-patch meshes beyond planar patches, and volume features (vegetation), tracked by
      [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md)

### Accepted residuals (no action planned)

- The web client gets none of this. It keeps its existing track label CRUD unchanged.
- The runtime pipeline is untouched: no change to L3 settling or L4 ground removal.
