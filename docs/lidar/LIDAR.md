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
| Label set migration      | [operations/annotation-label-set-migration.md](operations/annotation-label-set-migration.md)       |
| Per-frame evaluation     | [operations/per-frame-evaluation.md](operations/per-frame-evaluation.md)                           |
| Recording-use guard plan | [../plans/lidar-recording-use-guard-plan.md](../plans/lidar-recording-use-guard-plan.md)           |
| Headway report oracle    | [operations/headway-report-oracle.md](operations/headway-report-oracle.md)                         |
| SUMO counting lines      | [SUMO headway comparison](operations/sumo-columbus-broadway-headways-2026-10.md)                   |
| Near-edge frame cost     | [operations/near-edge-frame-cost-2026-10.md](operations/near-edge-frame-cost-2026-10.md)           |
| macOS visualiser         | [../ui/visualiser/architecture.md](../ui/visualiser/architecture.md)                               |
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
an annotation is a reviewed point mask. Scene denotes physical geometry.
The definitions do not add storage relationships: unified sites, deployment rows, surveys
and label consolidation remain Phase 2.

## Sites, deployments and surveys

A site is a physical place; its S2 site cell is a geographic index. A deployment
is one sensor's placement and settings over a period. A survey is that deployment's
measurement over a period. Current screens use these nouns where their meaning
matches the existing records; unified site, deployment and survey relationships
remain Phase 2. The location browser is [Clip locations](../../web/src/routes/lidar/clip-locations/+page.svelte),
served by `/api/lidar/clip-locations` and grouped by L10 district.

## Street use editor

Street use is now painted in the macOS VelocityVisualiser: once the alignment pane has placed the
sensor, **Paint streets** marks surfaces, travel lanes, parked-car spaces and driveways from a
top-down view over the reference, kerbs, City streets, boxes, ground mesh and registered scans, and
saves them to Gerardus with `EditStreetUse`. See
[Paint mode](../ui/visualiser/gerardus-alignment.md#paint-mode-the-street-painter). The web page
below is the earlier editor; it does not draw surfaces.

No public layer says which stretch of kerb is a driveway, where cars park, or where lanes run.
The Streets page (`/lidar/streets`, in the LiDAR navigation) lets an operator mark them once per
site, over geometry that is already public or measured. It is the client half of W1 in Gerardus's
vector map plan (`docs/plans/VECTOR-MAP.md` §4.1 in the Gerardus repository); Gerardus stores the
marks as authored features and serves them to the alignment pane and to later work (kerb cuts,
transient points in parked-car spaces, lanes per approach).

**Choosing a site.** The picker lists the located sites from `GET /api/lidar/clip-locations`, by
label and L16 token; any L16 token can also be typed, and `?site=<token>` opens one directly. The
page then reads `GET /api/v1/sites/{l16}/geojson?layers=city_streets,city_intersections,kerbs,boxes,footprints,street_use`
from Gerardus: GeoJSON in WGS 84 with height as a third coordinate, each feature carrying its
`layer`, `id`, `kind`, tags and, for street use, `author` and `edited_at`. A layer the bundle lacks
is absent, and the page says so; until kerbs exist (W2), driveways and rows snap to the box walls'
feet.

**What is drawn.** City street centrelines thin and grey, intersections as rings, kerbs and islands
strong and white, boxes and footprints as outlines, and the street use by kind: driveways pink;
spaces filled by `space_type` (metered, permit, loading, disabled, free), with a divider per space
for a row; lanes by `lane_type` (general, bus, bike, turn) with arrows in the direction of travel.
The ground is plain and dark. OpenStreetMap tiles are an optional backdrop: none is requested until
the operator allows it, once per visit, as in the site map editor, and nothing is snapped to or
taken from them.

**Tools.**

| Tool          | How                                                                                                                                              | Tags                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------- |
| Driveway      | Click the two ends of the kerb cut                                                                                                               | none                                |
| Space         | Click the corners; Enter or double-click finishes                                                                                                | `space_type`, `count`, `time_limit` |
| Row of spaces | Click two points along the kerb; the row runs between them, `count` spaces deep on the street side (towards the nearest centreline), with a flip | `space_type`, `count`, `time_limit` |
| Travel lane   | Click along the lane in its direction of travel; Enter or double-click finishes; Reverse turns it                                                | `lane_type`, `width_m`              |
| Select        | Click a mark to edit its tags; drag a vertex to move it, right-click one to remove it; Delete removes the mark                                   | as above                            |

Every vertex snaps, as a preview, to a vertex within 0.5 m of a kerb, centreline or other mark, or
else to the nearest point along one; driveways and rows also snap to box walls. Gerardus snaps and
drapes every vertex again in the site frame when the batch is saved. Undo (Ctrl or ⌘ Z) steps back
through unsaved changes; Escape cancels a drawing and Backspace drops its last vertex.

**Saving.** Save sends one batch to `POST /api/v1/sites/{l16}/street-use`: the author (required,
remembered in this browser), a note, and an upsert for each new or changed mark (`properties.id`
empty for a new one) and a remove for each deleted one. A mark moved and moved back sends nothing.
Gerardus answers with every current mark and a list of problems, each `"<id>: …"`; the page shows
each beside the mark it names, with a red halo on the map, and any it cannot place under Save. A
400 shows Gerardus's message and keeps the unsaved marks.

**Gerardus's address and failures.** The browser calls Gerardus directly; its address is a setting
on the Settings page (and on the Streets page), kept in this browser, default
`http://127.0.0.1:50061`. Gerardus must allow the page's origin (it answers loopback and tailnet
origins). The page says which failure it met: nothing answering (or an origin Gerardus refused,
which a browser cannot tell apart), a site with no prepared bundle (404: run `gerardus
prepare-site`), or a Gerardus without this API.

What leaves the machine: the site token and the marks go to Gerardus at the address set; map tiles
go to OpenStreetMap only after consent. No sensor data is sent. The client is
`web/src/lib/streets/` (the typed API client, the geometry and the edit state, with tests).
