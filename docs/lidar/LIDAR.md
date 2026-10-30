# LiDAR

Documentation for the velocity.report LiDAR subsystem (Hesai Pandar40P). The
canonical ten-layer processing architecture, package map, implementation
status, and literature alignment all live in
[architecture/LIDAR_ARCHITECTURE.md](architecture/LIDAR_ARCHITECTURE.md). This
file is just the index — start there for any layer detail.

## Folder structure

| Folder             | Scope                                                                  |
| ------------------ | ---------------------------------------------------------------------- |
| `architecture/`    | System design, layer specifications, and the canonical ten-layer model |
| `operations/`      | Runtime operations: data source switching, auto-tuning, debugging      |
| `troubleshooting/` | Resolved investigation notes for reference                             |

## Quick links

The [SpatialRust segmentation review](operations/spatialrust-segmentation-review.md) compares
implemented upstream algorithms with our L3/L4 pipeline and proposes experiments, coding slices,
and adoption gates. The
[LiDAR-Processing-V2 review](operations/lidar-processing-v2-segmentation-review.md) reads a
classical range-image pipeline (RECM ground, JCP boundary vote, curved-voxel clustering, hull and
calipers outlines) against the same layers, and proposes one spike: a two-directional boundary
vote on the L3 grid (l3bg0026).
The [PCL review](operations/pcl-algorithm-review.md) reads the Point Cloud Library against the
owner's five questions (road and kerbs, descriptors, hulls, static reconstruction, occlusion) and
proposes four experiments, led by range-image border classes on the L3 grid
([plan](../plans/lidar-pcl-algorithm-candidates-plan.md)).
The [Astera handover](operations/pcl-astera-handover.md) records the completed thirteen-question
review, final branch checks, and the remaining implementation prerequisites.
The [one-frame compatibility check](operations/pcl-organised-grid-compatibility.md) rejects
direct native-grid use and specifies the required projection and normal-neighbourhood adaptation.
The [segmentation contract checks](operations/pcl-segmentation-contracts.md) reproduce unbounded
region growth, refined plane membership, and morphological opening on synthetic support.
The [oracle tooling runbook](operations/pcl-oracle-tooling.md) pins the installation and maps
stock CLI limitations to explicit campaign harness contracts.
The [joining-control design](operations/pcl-joining-cap-controls.md) separates cap losses from
connectivity with six matched arms and a frozen-input oracle.
The [planar registration disposition](operations/pcl-planar-registration-disposition.md) places
one comparator within facet arm B and corrects the earlier 2-D z-translation claim.
The [hypothesis-verification disposition](operations/pcl-hypothesis-verification-disposition.md)
drops a separate duplicate-track arm and records its visibility and temporal-identity limits.
The [Gerardus novelty audit](operations/pcl-gerardus-registration-gap-review.md) confirms its
existing observability diagnostics and narrows follow-up to initial proposals.
The [background-cleanup disposition](operations/pcl-background-cleanup-disposition.md) drops
blanket MLS/SOR export cleanup and explains why the evaluation points must stay fixed.
The [classification corpus requirements](operations/pcl-classification-corpus-requirements.md)
inventory reviewed labels and set independent-object floors for class, aspect, and range screens.
The [original-discussion reconciliation](operations/pcl-original-discussion-reconciliation.md)
maps the supplied conversation to existing owners and records the merged-detection partition question.
The [return-fate and border contract](operations/pcl-return-fate-border-contract.md) reuses frame
acquisition while separating diagnostic fates, border traits, and coasting evidence.
The [merged-detection disposition](operations/pcl-merged-detection-partition-disposition.md) drops
a standalone PCL splitting arm and records the evidence required by the existing revision owner.

| Topic                    | Document                                                                                           |
| ------------------------ | -------------------------------------------------------------------------------------------------- |
| Ten-layer architecture   | [architecture/LIDAR_ARCHITECTURE.md](architecture/LIDAR_ARCHITECTURE.md)                           |
| Pipeline component map   | [architecture/lidar-pipeline-reference.md](architecture/lidar-pipeline-reference.md)               |
| Tracking implementation  | [architecture/foreground-tracking.md](architecture/foreground-tracking.md)                         |
| Packet format            | [../../data/structures/HESAI_PACKET_FORMAT.md](../../data/structures/HESAI_PACKET_FORMAT.md)       |
| Auto-tuning              | [operations/auto-tuning.md](operations/auto-tuning.md)                                             |
| Refinement criteria      | [operations/retrospective-refinement-criteria.md](operations/retrospective-refinement-criteria.md) |
| Uncertainty criteria     | [operations/adaptive-uncertainty-criteria.md](operations/adaptive-uncertainty-criteria.md)         |
| Analysis worker          | [operations/analysis-worker.md](operations/analysis-worker.md)                                     |
| Track labelling          | [operations/track-labelling-ui-implementation.md](operations/track-labelling-ui-implementation.md) |
| Point annotation         | [operations/point-annotation-tool.md](operations/point-annotation-tool.md)                         |
| Label sets               | [operations/label-sets.md](operations/label-sets.md)                                               |
| Site location correction | [Broadway at Franklin](operations/broadway-franklin-location-correction.md)                        |
| Label set migration      | [operations/annotation-label-set-migration.md](operations/annotation-label-set-migration.md)       |
| Per-frame evaluation     | [operations/per-frame-evaluation.md](operations/per-frame-evaluation.md)                           |
| Recording-use guard plan | [../plans/lidar-recording-use-guard-plan.md](../plans/lidar-recording-use-guard-plan.md)           |
| Headway report oracle    | [operations/headway-report-oracle.md](operations/headway-report-oracle.md)                         |
| SUMO counting lines      | [SUMO headway comparison](operations/sumo-columbus-broadway-headways-2026-10.md)                   |
| Near-edge frame cost     | [operations/near-edge-frame-cost-2026-10.md](operations/near-edge-frame-cost-2026-10.md)           |
| macOS visualiser         | [../ui/visualiser/architecture.md](../ui/visualiser/architecture.md)                               |
| Geometry data purpose    | [../geometry-priors-data-purpose.md](../geometry-priors-data-purpose.md)                           |
| Backlog                  | [../BACKLOG.md](../BACKLOG.md)                                                                     |

The [October near-edge campaign report](operations/near-edge-campaign-2026-10.md) preserves the
completed experiment's per-case findings, methods, uncertainty limits, and follow-up decisions.
Raw replay outputs remain local. The [update cost profile](operations/near-edge-update-cost-profile-2026-10.md) answers
its first follow-up: the cost breach is the solid body's extent admission, which the shadow shares. The [D2 re-run](operations/d2-medoid-obb-current-solver-2026-10.md)
refreshes the medoid against OBB-centre comparison on the current solver. The
[span selection timing](operations/near-edge-span-selection-timing-2026-10.md) measures the
solid body's update cost after its percentiles are selected rather than sorted. The
[physical-reference pilot](operations/physical-reference-pilot-kirk0-2026-10.md) is the first
frozen score against reviewed body-centre, size and following-gap references, on kirk0. The
[solid-body alignment](operations/solid-body-physical-alignment-kirk0-2026-10.md) follows it:
why the solid body misses both trucks, and opt-in experiments that correct their heading and
length, guarded on the 23-site corpus. The
[zero-code L3 arms report](operations/l3-zero-code-arms-kirk-2026-10.md) measures what
holding the region overrides off, and narrower base windows, do to labelled recall, false
foreground and stationary tracks on kirk0 and kirk1. The
[geometry convergence plan](../plans/lidar-tracker-geometry-convergence-plan.md) sets out what
replaces those patches: a heading the tracker observes every frame, and a box that holds its
points. Its [end-on truck window spike](operations/end-on-truck-window-spike-2026-10.md)
counts the second capture that plan needs: 33 clean windows in the corpus, an archive 6.7
times its size, and a selection rule in place of a search. The
[containment corpus screen](operations/solid-body-containment-corpus-2026-10.md) scores the
plan's first observation model, the box held to its points, on kirk0 and the 23 sites. The
[rectangle heading screen](operations/solid-body-rectangle-heading-2026-10.md) scores the
second, the heading observed from each frame's rectangle fit. The
[held-out replication](operations/geometry-convergence-heldout-2026-10.md) replays the
candidate on each site's second segment: the heading holds after four seconds, and the lateral
margin holds on matched windows but not per site. The
[published box report](operations/published-box-rotation-2026-10.md) puts the same two
observations into the box the visualiser and the scene assets draw, which held its returns on
only 3 % of vehicle frames: a box model that holds them all and turns no faster than a vehicle can, with the tracks
unchanged. The [refinement report](operations/published-box-refinement-2026-10.md) makes that box
model the default and finds that what remains of its errors is upstream: the course's lag is the
Kalman velocity's, and a truck drawn short is a truck held by two tracks. The [duplicate tracks report](operations/l5-duplicate-tracks-2026-10.md) counts those: a
quarter of vehicle tracks share their vehicle with another, and none of the identity options
tried holds one track per vehicle on held-out data without losing a car. The
[region override attribution](operations/l3-region-override-attribution-kirk-2026-10.md)
follows the zero-code L3 arms: the volatile tercile's window is the one that absorbs road users,
switching the overrides off moves along the recall trade-off rather than across it, and an
overrides-on retune with the height band floor at road height reaches the recall at the shipped
false positives. The [region map](operations/l3-region-tercile-mechanism-2026-10.md) checks why:
the volatile tercile holds the overrides-off flood, but lanes are there for the surfaces behind
them, not traffic during settling; neither of the two [levers](operations/l3-region-levers-kirk-2026-10.md)
it suggests beats the retune. The [dwell policy](operations/l3-dwell-policy-arrival-2026-10.md)
hands an object stationary for 20 s to the background, which removes the retune's long stationary
tracks over a 39-minute capture, and with its [clock at 40 s](operations/l3-dwell-policy-clock-2026-10.md)
keeps every moving track (its [drift rule](operations/l3-dwell-policy-drift-2026-10.md) barely matters
there; on the [corpus's two segments](operations/l3-candidates-corpus-dwell-2026-10.md) it
keeps CMR's moving tracks at either clock); on the labelled kirk captures it
[costs no moving road user](operations/l3-dwell-policy-labelled-cost-2026-10.md) and absorbs the
parked car the reviewed pack counts. The [pose step](operations/l3-scene-change-pose-step-2026-10.md)
shows the capture's late scene change is a knock to the sensor, then a second change and the
pickup, and a [corpus sweep](operations/l3-corpus-pose-sweep-2026-10.md) finds unrecorded knocks at
six other placements, four of them with one inside a scored window. The [W3 report](operations/w3-retention-fragmentation-kirk0-2026-10.md)
measures how much of each reviewed object reaches a cluster and into how many it splits, and the
[range-adaptive eps](operations/l4-range-adaptive-eps-2026-10.md) does not change it. The
[return fate](operations/w3-return-fate-kirk0-2026-10.md) says which stage loses each reviewed
return: L3 under the shipped grid, the input cap, DBSCAN noise and the height band under CMR.
Lifting the [input cap](operations/l4-dbscan-input-cap-2026-10.md) recovers the returns but joins
no fragments. The [October 18 experiment review](operations/experiment-review-2026-10-18.md) reads all of
this for the following-distance metric: what each line established, the next week's experiments
and labelling, and the one labelled window three adoptions wait on.

## Terminology

Core domain terms for the LiDAR tracking system. Tuning-pipeline terms (sweep,
combo, round, auto-tune, HINT) live in
[operations/tuning-guide.md §Glossary](operations/tuning-guide.md#glossary).

| Term            | Definition                                                                                                                    |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| **Point**       | A single 3D measurement from the LiDAR sensor (x, y, z, intensity, timestamp).                                                |
| **Cluster**     | A group of spatially-proximate foreground points identified by DBSCAN, representing a potential object.                       |
| **Track**       | A temporally-linked sequence of clusters representing a moving object across frames, maintained by the Kalman-filter tracker. |
| **Observation** | A single cluster-to-track association at one point in time (one frame's measurement of a track).                              |
| **Clip**        | A window chosen for evaluation or annotation, with ordered captures, a reference run and tuning provenance.                   |
| **Run**         | One processing pass over a source with fixed parameters, producing tracks for comparison with the clip’s reference run.       |

## Label taxonomy

Labels are applied by human reviewers to tracks within a run. The same taxonomy is used across all platforms (Go backend, Svelte web frontend, macOS app).

### Object classes (`user_label`)

Classify what object the track represents. Ships 7 active labels.

<!-- Canonical source: internal/api/lidar_labels.go → AllObjectClasses -->

| Label        | Description                                              |
| ------------ | -------------------------------------------------------- |
| `car`        | Passenger car, SUV, van, or truck                        |
| `bus`        | Bus, coach, or large passenger vehicle (length > 7 m)    |
| `pedestrian` | Person walking, running, or using a mobility aid         |
| `cyclist`    | Person on a bicycle, e-scooter, or motorcycle            |
| `bird`       | Bird or other airborne fauna                             |
| `noise`      | Spurious track (sensor noise, rain, dust, or vegetation) |
| `dynamic`    | Ambiguous dynamic object or insufficient observations    |

### Quality flags (`quality_label`)

Rate the measurement quality of a track. Multi-select (comma-separated).

<!-- Canonical source: internal/api/lidar_labels.go → AllQualityFlags -->

| Flag              | Description                                                              |
| ----------------- | ------------------------------------------------------------------------ |
| `good`            | Clean, accurate track with correct speed and trajectory                  |
| `noisy`           | Track has noisy position or speed estimates                              |
| `jitter_velocity` | Speed estimates jitter significantly                                     |
| `jitter_heading`  | Heading estimates jitter significantly                                   |
| `merge`           | Two or more distinct objects incorrectly merged into one track           |
| `split`           | Single object incorrectly split into multiple tracks                     |
| `truncated`       | Track starts late or ends early relative to the object's true trajectory |
| `disconnected`    | Track was lost and recovered: identity may have changed                  |

### Canonical sources

- **Go backend:** [internal/api/lidar_labels.go](../../internal/api/lidar_labels.go); `validObjectClasses`, `validQualityFlags`
- **Svelte frontend:** [web/src/lib/types/lidar.ts](../../web/src/lib/types/lidar.ts); `ObjectClass`, `QualityLabel`
- **macOS app:** [tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift](../../tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift); `LabelPanelView`

> **Note:** `object_class` (e.g. `pedestrian`, `car`, `bird`, `dynamic`) is a _sensor-assigned_ classification from the tracker, not a human label. It is distinct from the `user_label` taxonomy above. Ships 7 classes: car, bus, pedestrian, cyclist, bird, dynamic, noise. Truck and motorcyclist are reserved for future use (proto enum values allocated but not user-assignable).

## Shared vocabulary

The [platform vocabulary](../platform/PLATFORM.md#platform-vocabulary) defines capture, evidence, estimate, track, clip,
label, annotation, site, deployment and survey. A clip is an evaluation or annotation window;
a run is one processing pass. A label is a person’s class and quality verdict on a track;
an annotation is a reviewed point mask. Scene denotes measured spatial data about the physical
surroundings (see
[Geometry priors: data purpose and interpretation](../geometry-priors-data-purpose.md)).
The definitions do not add storage relationships: unified sites, deployment rows, surveys
and label consolidation remain Phase 2.

## Sites, deployments and surveys

A site is a physical place; its S2 site cell is a geographic index. A deployment
is one sensor's placement and settings over a period. A survey is that deployment's
measurement over a period. Current screens use these nouns where their meaning
matches the existing records; unified site, deployment and survey relationships
remain Phase 2. The location browser is [Clip locations](../../web/src/routes/lidar/clip-locations/+page.svelte),
served by `/api/lidar/clip-locations` and grouped by L10 district.

## Site viewer

The Site viewer (`/lidar/sites`, in the LiDAR navigation) shows what Gerardus serves for a
site: its vector map on a 2D map, and the same layers with the ground and the scans in 3D. It
only reads. Street use (driveways, parked-car spaces, travel lanes and surfaces) is marked in the
macOS app's [Paint mode](../ui/visualiser/gerardus-alignment.md#paint-mode-the-street-painter), which saves it to Gerardus with `EditStreetUse`; the web street editor that came before this page is gone,
and `/lidar/streets` redirects here, keeping `?site=`. It is the browser's view of W1 in
Gerardus's vector map plan (`docs/plans/VECTOR-MAP.md` §4.1 in the Gerardus repository).

**What it shows.** Measured or derived geometry at each source's acquisition time: public
footprints, aerial points and street data, and what Gerardus's accepted scans observed. Observed
features and wall planes come from the scans; footprints, boxes, box walls, building parts, roof
planes and wall surfaces are derived from public data; street use is the operator's marking, not a measurement. It is not a plan, a land or
boundary survey, a structural assessment or a specification, and a feature that is not drawn is not
asserted absent. See
[Geometry priors: data purpose and interpretation](../geometry-priors-data-purpose.md).

**Choosing a site.** The picker lists Gerardus's prepared sites from `GET /api/v1/sites`, each with
its label, its site-frame origin (WGS 84 longitude and latitude at the list's `epoch`, and
height) and the layers it can serve. A Gerardus older than that list (one that answers 404 or as gRPC) gets the located sites
from `GET /api/lidar/clip-locations` instead, and the page says so. Any L16 token can be typed,
`?site=<token>` opens one directly, and `?view=3d` opens the 3D view.

**The 2D map.** The page reads `GET /api/v1/sites/{l16}/geojson?layers=…` for every layer the site
serves: GeoJSON in WGS 84 with height as a third coordinate, each feature carrying its `layer`,
`id`, `kind`, tags, `author` and `edited_at` (observed features and wall planes also `state` and
`model_id`). Gerardus refuses a layer name it does not know with a 400, so a Gerardus that did not
list its layers is asked again without `observed` and `wall_planes`. Each layer has a checkbox with its feature count.
Street use is drawn by kind (driveways pink; spaces filled by `space_type`; lanes by `lane_type`
with arrows in the direction of travel) and surfaces filled by `surface`: carriageway, sidewalk,
verge, apron or median. Kerbs and islands are strong and white, city centrelines thin and grey,
boxes and footprints outlined, wall planes drawn along their foot by `role` (wall, popout, recess),
and what the scans observed by kind (facade, kerb, sensor, correction, pole). A click shows a
feature's properties. The ground is plain and dark.

**The 3D view** (three.js) asks for the same layers with `&frame=site`, in site-frame metres
(`[x east, y north, z up]`; the collection's `frame` member says `"site"`), and for `GET /api/v1/sites/{l16}/ground`, the ground grid, which it
draws as a mesh through the centres of the cells that have a height (`x0`, `y0` is the south-west
cell's centre). Where the site lists `box_walls`, the 3D view asks for it instead of `boxes`: each
box wall a vertical polygon with its z, its top on the building's roofline where Gerardus has roof
planes for it (`top=roofline`) and at the box's height elsewhere (`top=box`), filled translucent as it
comes and never raised by its `height_m`; walls on edges another footprint shares are fainter. From a Gerardus that lists no
box walls, boxes are raised by `height_m` from their base, an inferred extent. Wall planes are
translucent and coloured by role, surfaces are
translucent fills draped on their own heights, kerbs, centrelines and observed lines sit at their
z, lanes carry arrows, and a point with a `height_m` (a pole) stands as an upright. Scan points
come from `GET /api/v1/sites/{l16}/scans?voxel_m=…` (little-endian float32 x, y, z triplets,
coloured by height) at the voxel chosen, 0.1 to 1 m; Gerardus clamps it to 0.05–2 m and says the one
it used in `X-Gerardus-Voxel-M`, which the page shows; Gerardus answers 403 unless it serves scans
to this browser's peer, and the page says so. The camera orbits about the site (drag; right-drag
or Shift-drag pans; scroll zooms), Reset view returns to the view from the south, a north arrow
turns with the camera, and each layer, the ground and the scans can be hidden. Loading another
site disposes the old site's geometry. A collection in longitude and latitude (`"frame": "wgs84"`,
or, from a Gerardus too old to say, positions by the origin) is reported, not drawn.

**Buildings, markings, furniture and trees.** Gerardus's W4–W7 layers, `building_parts`,
`roof_planes`, `lane_markings`, `street_furniture` and `trees`, are not among its default layers,
and it refuses a layer name it does not know, so the page asks for each only when
`GET /api/v1/sites` lists it for the site. Their features carry the tags plus `layer`, `id`, `kind`,
`model_id` and `state`; kinds are `building_part`, `roof_plane`, `lane_marking`, `pole`, `sign` and
`tree`. Beside them, `wall_surfaces` (kind `wall_surface`, Gerardus `docs/plans/ROOFLINE-WALLS.md`)
holds vertical faces derived along each building part's outline up to its roofline (derived from
public data, not observed walls), each a vertical polygon with its z, tagged
`part`, `edge`, `role` (`wall`, `gable`, `step` or `party`) and `top`; like `box_walls` it is asked
for only when listed, and only for the 3D view, since seen from above a wall is a line along its
foot. On the 2D map, building parts are outlined and faintly filled by `roof:shape` (each Simple
3D Buildings shape its own colour, grey for none or `unknown`), roof planes filled by `slope_deg`
(blue flat, yellow at 22.5°, red at 45° and steeper), lane markings white or yellow by `colour`,
dashed when `pattern` is `dashed` and heavier when it is `double`, poles and signs as dots (a
vertical segment seen from above), and trees as their crown, a circle of `crown_radius_m` (2 m
when absent). A click shows the feature's properties, `roof:shape`, `model_id` and `state`
among them. In 3D, a building part's outline lies at its base (the ground plus `min_height`).
When the site serves `wall_surfaces`, the part is that outline alone and its wall surfaces are
filled translucent in their own planes, so no wall stands above the eave: a part's `height` is the
top of its roof. From a Gerardus without wall surfaces a part is drawn as a prism raised by
`height − min_height`, an extent inferred from the part's height tags rather than its roof
planes, as before. Roof planes are
triangulated at their z;
markings lie on the road at their own z, area markings filled; poles are cylinders of `diameter_m`
(0.2 m when absent); signs are small boxes, 0.6 m by 5 cm, over the face; and trees are a trunk
cylinder of `dbh_m` (0.3 m when absent) up to `crown_base_m` with a translucent crown ellipsoid of
radius `crown_radius_m` from `crown_base_m` to `height_m`. Each layer has its own checkbox.

**Browser maps.** OpenStreetMap tiles behind the 2D map remain optional and require the existing
per-visit consent. OSM vector layers (`osm`, `intersections`) are retired: they are not requested
from Gerardus or drawn as LiDAR references. City streets, kerbs, markings and measured geometry
remain independent of the browser map.

**Gerardus's address and failures.** The browser calls Gerardus directly; its address is a setting
on the Settings page (and on this page), kept in this browser, default `http://127.0.0.1:50061`.
Gerardus must allow the page's origin (it answers loopback and tailnet origins). The page says
which failure it met: nothing answering (or an origin Gerardus refused, which a browser cannot tell
apart), a site with no prepared bundle (404: run `gerardus prepare-site`), a Gerardus without this
API, a server error, or an answer off the contract.

What leaves the machine: the site token goes to Gerardus at the address set; map tiles go to
OpenStreetMap only after consent. Nothing is sent to Gerardus but requests to read. The client is
`web/src/lib/gerardus/` (the typed API client, the map styles, and the 3D builders as plain arrays,
with tests); the 3D view is `web/src/lib/components/lidar/SiteScene3D.svelte`.
