# L7 scene and multi-sensor fusion plan

- **Status:** Parked (D-32, October 20, 2026): deferred, or planned for v1.0 and later; revisit when its v1.0 or v2.0 slot becomes next. Was: 📋 Planned; v1.0. Updated October 25, 2026: the static half of the scene (§1 items 1, 3 and 5) is being built outside this repository, in [Gerardus](https://github.com/banshee-data/gerardus), from settled backgrounds registered against USGS 3DEP; nothing of it is stored in velocity.report yet. See [§ 0](#0-where-implementation-stands-october-25-2026).
- **Layers:** L7 Scene
- **Canonical architecture:** [LIDAR_ARCHITECTURE.md](../lidar/architecture/LIDAR_ARCHITECTURE.md)
- **Maths index:** [data/maths/MATHS.md](../../data/maths/MATHS.md)
- **Canonical:** [vector-scene-map.md](../lidar/architecture/vector-scene-map.md)

Scene geometry in this plan is measured spatial data about observable physical conditions at the
time of observation, not a survey, plan or specification; see
[Geometry priors: data purpose](../geometry-priors-data-purpose.md).

This plan covers two related workstreams for the L7 Scene layer:

1. **L7 Scene layer**: design the canonical scene package
2. **Multi-sensor architecture**: how multiple sensors fuse at L7

---

## 0. Where implementation stands (October 25, 2026)

The persistent static scene is no longer only a plan. It is being built in Gerardus, the separate
map-priors repository velocity.report decided on ([Gerardus ADR-0006][gerardus-adr-0006]). Gerardus
reads nothing of velocity.report's: the visualiser uploads each settled background (`UploadScan`),
Gerardus registers it against USGS 3DEP aerial LiDAR, an operator accepts the pose in the
[alignment pane](../ui/visualiser/gerardus-alignment.md), and the accepted scans of a site form its
**site model**, a set of registered measurements. Every derived feature is computed from that model
and public data, and served back over `gerardus.align.v1alpha1` (gRPC) and read-only GeoJSON. Its
plans: [SITE-MODEL-SPIKE][gerardus-site-model-spike], [VECTOR-MAP][gerardus-vector-map] (W1–W9),
[BUILDINGS][gerardus-buildings] and [ROOFLINE-WALLS][gerardus-roofline-walls].

[gerardus-adr-0006]: https://github.com/banshee-data/gerardus/blob/main/docs/adr/0006-boundary-with-velocity-report.md
[gerardus-site-model-spike]: https://github.com/banshee-data/gerardus/blob/main/docs/plans/SITE-MODEL-SPIKE.md
[gerardus-vector-map]: https://github.com/banshee-data/gerardus/blob/main/docs/plans/VECTOR-MAP.md
[gerardus-buildings]: https://github.com/banshee-data/gerardus/blob/main/docs/plans/BUILDINGS.md
[gerardus-roofline-walls]: https://github.com/banshee-data/gerardus/blob/main/docs/plans/ROOFLINE-WALLS.md

| This plan's item                              | Where it stands                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | Where                                                                                                                                                                                                                                                                                           |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| §1.1 Static geometry                          | **Built as proposals**, from registered settled backgrounds and 3DEP rather than from accumulated L4 output: the ground surface (3DEP grid refined by the scans), facade traces, wall planes with popouts and recesses, kerbs and islands, LoD1 boxes and LoD2 building parts with roof planes and `roof:shape`, wall surfaces up to the roofline, lane markings, poles and signs, and trees (W1–W7). Kirkham–9th, for example: 646 building parts, 2,974 roof planes, 256 measured wall planes. Moving objects are kept out (W3: points inside track boxes, points seen on too few days, parked-car spaces)           | Gerardus `observed_features` rows ([ADR-0012](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0012-street-level-features-from-registered-scans.md), [ADR-0015](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0015-building-parts-and-roof-planes-as-derived-rows.md)) |
| §1.2 Dynamic canonical objects                | **Not built**, in either repository. L5 tracks and L6 labels stay per-run in velocity.report                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | —                                                                                                                                                                                                                                                                                               |
| §1.3 External priors                          | **Built, from different sources.** Public-domain first ([ADR-0008](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0008-public-domain-first-osm-for-vectors.md), [ADR-0009](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0009-city-streets-by-default.md)): USGS 3DEP, DataSF footprints, streets, intersections, kerbs and the Street Tree List, SFMTA's signs, meters and signals. OpenStreetMap is for display and comparison only, served to admins, and never shapes a derived feature; S3DB ingestion was not built. See the note under the [OSM priors service](#osm-priors-service) | Gerardus site bundles, one per S2 L16 cell                                                                                                                                                                                                                                                      |
| §1.4 Multi-sensor merged scene                | **Static half built:** several sensor positions at one site (Kirkham's three) share one site frame, register against 3DEP plus the scans already accepted, and are refined together by `RefineSite`, a small SE(3) pose graph. **Track fusion across sensors is not built** (§ 2 stands)                                                                                                                                                                                                                                                                                                                               | Gerardus `RegisterRequest.against_site_model`, `RefineSite`                                                                                                                                                                                                                                     |
| §1.5 Uncertainty and provenance               | **Built, in a different shape.** Registration records are append-only, with reviewer, gates and pose covariance; each derived row names the accepted records and `model_id` it came from, its parameters, the derivation version and server version, and its state (`proposed`), and keeps its id across re-derivation. Per-feature log-odds confidence and observation counts (§ 3.1) are not used                                                                                                                                                                                                                    | Gerardus registration log and `observed_features`                                                                                                                                                                                                                                               |
| § 3.4 Prior-to-observation alignment          | **Built differently:** point-to-plane ICP of each settled background against 3DEP, with ground, facade, rank and seed-agreement gates, instead of Procrustes over prior correspondences. DataSF outlines are moved onto 3DEP by a fitted footprint shift per assessor block, and served shifted. Tie-point placement (`SolveTiePoints`) is not built                                                                                                                                                                                                                                                                   | Gerardus `Register`, `boxes.rs`                                                                                                                                                                                                                                                                 |
| § 4 Scene-constrained prediction, scene graph | **Not built.** The geometry it would read (kerbs, ground surface, travel lanes marked in the street-use editor) now exists in Gerardus                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | —                                                                                                                                                                                                                                                                                               |

**What velocity.report has today:** the visualiser's alignment pane uploads backgrounds, registers and accepts poses, and draws every Gerardus layer over a capture, extruding nothing ([#863](https://github.com/banshee-data/velocity.report/pull/863)); the web site viewer at `/app/lidar/sites` reads the same layers from Gerardus's GeoJSON. Accepted poses do not yet reach velocity.report's own records (`PUT /api/lidar/sites/{l16}/pose`, [visualiser alignment plan](lidar-visualiser-gerardus-alignment-plan.md)), so `CoordinateFrameInfo` stays the identity, and no L7 feature is stored in this repository.

**Open before any of it is published:** Gerardus's launch gates (typed schemas, derivation
identities and compatibility fixtures before W4–W7 rows are persisted for release), the W9
evaluation against hand-checked samples, and ADR-0011's rule that nothing following a person over
time is published. Publication also needs the context in
[Geometry priors: data purpose](../geometry-priors-data-purpose.md#publication-checklist) stated on
every export (W8's 3D Tiles, CityJSON and FlatGeobuf): coordinate reference and units, acquisition
interval, evidence status (observed or inferred), uncertainty and coverage gaps, dataset version,
and source terms. The members and property allow-list are in the
[prior file format](../lidar/architecture/geometry-prior-service.md#property-allow-list); this plan
specifies none of the W8 formats, so how each carries them is not settled here.

**What is left, recalibrated October 26, 2026.** Gerardus's W1–W7 were sized at 14–21 weeks and built as proposals in three days, then took two days of correction on the roofs; a kind takes about a day to build and one to two to correct once measured, and the owner's checking does not compress ([VECTOR-MAP §4.0](https://github.com/banshee-data/gerardus/blob/main/docs/plans/VECTOR-MAP.md#40-where-it-stands-2026-10-26)).

- **Now:** finish Gerardus's buildings (W4) and trees (W7), about 1–1.5 weeks. Vector work then pauses for vehicle state estimation, filtering and tracking ([state-estimation plan](lidar-state-estimation-plan.md), the 05x tracking slots).
- **Later in Gerardus:** W8 exports (2–3 days, when a consumer needs them) and W9 evaluation (1–2 weeks, bound by hand labels).
- **In this repository:** the accepted pose written back (`S`, the backlog's Gerardus alignment client section); the v0.7.7 road layer for posted limits, re-scoped onto Gerardus's city streets, painted travel lanes and signs (`M`, was `L`); the v1.0 L7 row narrowed to canonical objects, cross-sensor track fusion and accumulated relations (`L`, was `XL`), which waits on the tracking work above and on a site with a second sensor.
- **Mobile mapping (SLAM)** is the [06x series](../BACKLOG.md#06x-roving-routes-) (v0.6.0–v0.6.4), after the 05x tracking work: the SLAM spike Gerardus runs, portable timing, then backpack and bike transfer and route speeds. The static deployment and reporting slots that were 06x are now 07x.

---

## 1. L7 scene: persistent canonical scene

L7 is the key architectural addition in the ten-layer model. It introduces a **persistent,
evidence-accumulated record of the observed scene** that transcends individual frames, tracks, and
sensors.

### What L7 scene contains

1. **Static geometry**: ground surface polygons, building footprints, walls, fences, vegetation volumes, kerbs. Derived from L4 perception outputs accumulated over many frames. Stored as vector features with hierarchical LOD (0–3). See [vector-scene-map.md](../lidar/architecture/vector-scene-map.md).

2. **Dynamic canonical objects**: long-lived vehicle/pedestrian geometry inferred from merged L5 tracks. A single car observed across 200 frames produces one canonical object with refined dimensions, not 200 per-frame clusters. See [vector-scene-map.md](../lidar/architecture/vector-scene-map.md).

3. **External priors**: geometry imported from OpenStreetMap (S3DB building outlines, road axes), community GeoJSON, or manual survey. Priors are treated as low-confidence initial features that observation evidence can validate, refine, or reject. Only geometry is taken from these sources (outlines, parts, heights, centrelines); use, material and other descriptive tags are not, and OSM-derived data keeps its ODbL attribution (see the [reference data plan](spatial-priors-reference-data-plan.md#3-free-or-readily-accessible-sources-worth-using)).

4. **Multi-sensor merged scene**: when multiple sensors observe the same area, L7 fuses their independent L1–L6 pipelines into a single coherent scene (see [§ 2. Multi-sensor architecture](#2-multi-sensor-architecture) below).

5. **Uncertainty and provenance**: every scene feature carries confidence bounds, observation count, source sensor IDs, first and last observed times, evidence status (observed, inferred, source data or prior only; [vector scene map §5.5](../lidar/architecture/vector-scene-map.md#55-provenance-evidence-status-and-uncertainty)), and edit history. User edits in VelocityVisualiser are tracked separately from automated refinement.

### L7 relationship to L4 perception

L4 produces **per-frame, single-sensor observations**:

- Ground tiles, cluster bounding boxes, height-band classifications

L7 accumulates these into **persistent, multi-frame geometry**:

- Vector polygons, canonical objects, refined dimensions

The separation ensures L4 remains a fast, stateless, per-frame operation while L7 handles the slower, stateful accumulation across time and sensors.

### L7 relationship to L5/L6

L5 tracks are **ephemeral identity**: they live for the duration of an object's visibility to one sensor. L6 objects are **per-track semantic labels**.

L7 canonical objects are **persistent identity**: a car that drives through the intersection, disappears for 30 seconds, then returns can be re-identified as the same canonical object. When multiple sensors are deployed (future), L7 is where tracks from different sensors merge into a single coherent object trajectory.

### OSM priors service

> **As built (October 25, 2026):** this service became Gerardus, with public-domain priors in place of OSM ([§ 0](#0-where-implementation-stands-october-25-2026)). Of the operations below, BOUNDS is the site bundle (an L16 cell and its neighbours); DIFF and SHIFT are the footprint offsets and the per-block footprint shift fitted on 3DEP; MERGE is not done (derived features stay proposals beside the public layers, never merged into them); EXPORT to an OSM changeset is not built, and exports are planned as 3D Tiles, CityJSON and FlatGeobuf ([VECTOR-MAP W8](https://github.com/banshee-data/gerardus/blob/main/docs/plans/VECTOR-MAP.md#4-work-in-order)). The box below is the original design.

The L7 Scene layer is designed to ingest external map priors as initial low-confidence features:

```
┌─────────────────────────────────────────────────────────────────┐
│  OSM Priors Service                                             │
│                                                                 │
│  1. Fetch S3DB building outlines for sensor coverage area       │
│  2. Convert to L7 SceneFeature (Structure, LOD 0–1)             │
│  3. Import road geometry as Ground features                     │
│  4. Load community GeoJSON for kerbs, crosswalks, vegetation    │
│                                                                 │
│  Priors arrive with source provenance (osm_way, timestamp)      │
│  Confidence: low (prior) → refined by L4 observations           │
│                                                                 │
│  Operations on priors:                                          │
│  • DIFF — compare prior geometry against observed scene         │
│  • SHIFT — apply local translation/rotation (XYZ offset)        │
│  • MERGE — adopt observed refinements into prior features       │
│  • EXPORT — emit refined geometry as OSM changeset (.osc)       │
│  • BOUNDS — clip/extend prior coverage to sensor FOV            │
│                                                                 │
│  Export workflow:                                               │
│  • Structure changes → proposal candidates (manual review)      │
│  • LOD 2–3 refinements → OSM proposal bundles                   │
│  • JOSM/iD review (manual upload only — no auto-write)          │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Multi-sensor architecture

### Current state: single sensor

Today, velocity.report runs one LiDAR sensor per deployment. Each sensor has its own L1–L6
pipeline instance. L7 scene accumulation and cross-sensor fusion remain planned. L8 analytics
and L9 endpoints already have code packages; L10 denotes the existing clients.

### Future state: multi-sensor scene fusion

Real-world traffic monitoring benefits from multiple sensors covering different angles of an intersection or corridor:

```
                    ┌─────────────────────┐
                    │   Intersection      │
                    │                     │
    Sensor A ◄──────┤  Coverage overlap   ├──────► Sensor B
    (LiDAR NW)      │  zone — track       │      (LiDAR SE)
                    │  handoff occurs     │
                    │  here               │
    Sensor C ◄──────┤                     ├──────► Sensor D
    (Radar W)       │                     │      (Radar E)
                    └─────────────────────┘
```

**Use cases for multi-sensor deployment:**

1. **Extended coverage**: two sensors on opposite sides of a bend cover the full trajectory of a turning vehicle, eliminating the blind zone.
2. **Multi-modality**: combine LiDAR (rich geometry, range accuracy) with radar (direct velocity measurement, weather resilience) for robust detection in all conditions.
3. **Occlusion resolution**: a pedestrian occluded from Sensor A may be visible to Sensor B; the merged scene shows the complete picture.
4. **Redundancy**: sensor failure or blockage (snow, spider web) is compensated by the remaining sensors.

### Layer impact of multi-sensor fusion

| Layer         | Single-sensor (today)               | Multi-sensor (future)                                                                               |
| ------------- | ----------------------------------- | --------------------------------------------------------------------------------------------------- |
| L1 Packets    | One UDP stream                      | Multiple streams, each sensor on its own port/interface                                             |
| L2 Frames     | One frame pipeline                  | Parallel frame pipelines, each in sensor-local coordinates                                          |
| L3 Grid       | One background grid                 | Per-sensor grids (polar coordinates are sensor-specific)                                            |
| L4 Perception | One set of clusters                 | Per-sensor clusters in sensor-local frames                                                          |
| L5 Tracks     | One `TrackSet`                      | Per-sensor `TrackSet` instances with sensor-local track IDs                                         |
| L6 Objects    | Per-sensor classification           | Per-sensor classification (unchanged)                                                               |
| **L7 Scene**  | **Single-sensor accumulated scene** | **Merged scene: cross-sensor track association, unified coordinate frame, fused canonical objects** |
| L8 Analytics  | Scene-contextualised metrics        | Multi-sensor coverage statistics, cross-sensor consistency metrics                                  |
| L9 Endpoints  | Single-sensor gRPC stream           | Merged multi-sensor stream, per-sensor debug views                                                  |
| L10 Clients   | Renders one pipeline                | Renders merged scene with per-sensor toggle overlays                                                |

**Key architectural principle:** L1–L6 remain per-sensor and sensor-local. Multi-sensor fusion happens exclusively at L7, where observations from all sensors merge into a single canonical scene. This keeps the real-time per-sensor pipeline simple and avoids premature coordinate transforms in the low-level layers.

### Radar-LiDAR fusion: open L5 vs L6 vs L7 decision (L4 ruled out)

The OPS243-A radar is a Doppler-only sensor. It outputs FFT-derived speed bins (speed + magnitude) with no spatial coordinates — no range, azimuth, or elevation. It does not produce a point cloud.

**L4 is ruled out.** Point-level fusion at L4 requires a shared geometric measurement space; OPS243-A provides no per-return geometry.

The active design decision is **L5 vs L6 vs L7**:

| Candidate                   | Why it might fit                                                                         | Tradeoffs                                                                                |
| --------------------------- | ---------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| L5 track-level association  | Uses existing Kalman state and measurement gating to map radar speed to LiDAR tracks     | Simpler and lower latency; may struggle when multiple objects occupy the radar beam cone |
| L6 object-level integration | Uses L6 feature aggregation and classification confidence to absorb radar speed evidence | Depends on still-evolving L6 contracts; may couple fusion placement to classifier design |
| L7 scene-level enrichment   | Uses persistent identity and broader scene context to disambiguate assignments           | More robust in ambiguous scenes; higher complexity and longer implementation path        |

L6 classification shape remains open, so this decision should stay open until L6 input contracts stabilise and we can measure ambiguity/error tradeoffs on real capture runs.

See [LIDAR_ARCHITECTURE.md § Multi-modality: radar + LiDAR](../lidar/architecture/LIDAR_ARCHITECTURE.md#multi-modality-radar--lidar) for the full rationale and measurement model. See [QUESTIONS.md Q10](../../data/QUESTIONS.md) for the evidence plan.

### Cross-sensor track handoff

When a vehicle moves from Sensor A's coverage into Sensor B's, the L7 scene must associate the departing L5 track from A with the arriving L5 track from B:

```
Time ─────────────────────────────────────────────►

Sensor A track:  ═══════════════╗
                                ║ departure zone
                                ║ (predicted trajectory)
                                ║
Sensor B track:                 ╠══════════════════
                                ║ arrival zone
                                ║ (new tentative track)

L7 canonical:    ═══════════════╬══════════════════
                 same object    ║ handoff point
                 ID throughout  ║ (spatial + temporal gating)
```

**Association strategies** (to be evaluated during implementation):

- **Spatial gating**: predicted position from A's Kalman state vs. B's new detection position; gate by Mahalanobis distance
- **Velocity consistency**: heading and speed must agree within tolerance
- **Temporal proximity**: departure time from A and arrival time at B must be consistent with expected transit time across the gap
- **Appearance consistency**: dimensions and classification from A must match B's observations

---

## 3. Mathematical foundations

L7 builds on the mathematical machinery already established in L3–L5 and extends it to the persistent-geometry and multi-sensor domains.

### 3.1 Scene accumulation: bayesian evidence grid

Static geometry accumulates via per-feature confidence updates. For each scene feature $f$ observed by frame evidence $z_t$:

$$P(f \mid z_{1:t}) \propto P(z_t \mid f) \cdot P(f \mid z_{1:t-1})$$

In practice, the log-odds formulation avoids numerical issues:

$$L(f \mid z_{1:t}) = L(f \mid z_{1:t-1}) + \log\frac{P(z_t \mid f)}{P(z_t \mid \neg f)}$$

This is the same occupancy-grid update used in OctoMap (Hornung et al., 2013) but applied to vector features rather than voxels. Each feature carries an observation count $N$ and accumulated log-odds confidence $L$.

**Related maths docs:** [background-grid-settling-maths.md](../../data/maths/background-grid-settling-maths.md) (L3 EMA settling), [ground-plane-maths.md](../../data/maths/ground-plane-maths.md) (L4 ground removal; production uses `HeightBandFilter`; tile-based surface estimation is planned, see [ground-plane-extraction.md](../lidar/architecture/ground-plane-extraction.md)).

### 3.2 Canonical object refinement: running sufficient statistics

When L5 tracks are promoted to L7 canonical objects, their geometry is refined using streaming Welford updates (same Welford algorithm proposed for the planned L4 ground-plane estimator):

$$\mu_{n+1} = \mu_n + \frac{\delta}{n+1}, \quad C_{n+1} = C_n + \delta \cdot (x_{n+1} - \mu_{n+1})^T$$

where $\delta = x_{n+1} - \mu_n$ and $C$ is the running scatter matrix. This produces refined dimensions (length, width, height) for canonical objects without buffering individual observations.

**Related maths doc:** [ground-plane-vector-scene-maths proposal](../../data/maths/proposals/20260221-ground-plane-vector-scene-maths.md) § 3.1.

### 3.3 Cross-sensor track association: gated nearest-neighbour

Track handoff from Sensor A to Sensor B uses Mahalanobis-distance gating:

$$d_M^2 = (\hat{x}_A - z_B)^T S^{-1} (\hat{x}_A - z_B)$$

where $\hat{x}_A$ is the predicted state from A's Kalman filter extrapolated to B's detection time, $z_B$ is B's new detection, and $S = H P_A H^T + R_B$ is the innovation covariance combining A's prediction uncertainty with B's measurement noise.

Association is accepted when $d_M^2 < \chi^2_{3,\alpha}$ (3-DOF gate for position). Velocity consistency adds a second gate on heading/speed residuals.

**Related maths docs:** [tracking-maths.md](../../data/maths/tracking-maths.md) (L5 Kalman filter), [clustering-maths.md](../../data/maths/clustering-maths.md) (DBSCAN spatial indexing).

### 3.4 Prior-to-observation alignment: rigid transform estimation

OSM priors are aligned to sensor-observed geometry via Procrustes analysis. Given $n$ corresponding point pairs $\{(p_i, q_i)\}$ between prior and observed features:

$$\min_{R, t} \sum_{i=1}^{n} \| R p_i + t - q_i \|^2$$

Solved in closed form via SVD of the cross-covariance matrix $H = \sum (p_i - \bar{p})(q_i - \bar{q})^T$. The rotation $R = V U^T$ (with sign correction) and translation $t = \bar{q} - R \bar{p}$.

This aligns OSM building outlines and road geometry to the sensor's local coordinate frame without requiring GPS-grade localisation.

As built in Gerardus, the alignment runs the other way: each sensor's settled background is registered by ICP against 3DEP, which is already surveyed, and the public outlines are shifted onto 3DEP; see [§ 0](#0-where-implementation-stands-october-25-2026).

---

## 4. Scene-constrained physics and geometric relationships

L7 is also the natural home for two concerns that cannot live in lower layers: **physics-constrained trajectory prediction** and **persistent geometric relationships** (the scene graph).

### 4.1 Physics motion model: layer placement

Single-object kinematics (richer Kalman state vectors) belong at L5: that is a per-track concern. But once prediction needs to account for scene geometry or multi-object interactions, L7 owns it because only L7 holds the accumulated road polygons, kerb boundaries, structure walls, and the set of all canonical objects.

| Scope                                                            | Layer        | Responsibility                                                                                    |
| ---------------------------------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------- |
| Single-object kinematics (CV, CA, CTRV, IMM)                     | L5 Tracks    | Per-track state estimator; extends the existing Kalman state vector                               |
| Scene-constrained prediction (road-following, kerb clipping)     | **L7 Scene** | Clips kinematic trajectory fans to physically plausible corridors defined by accumulated geometry |
| Multi-object interaction (following distance, gap acceptance)    | **L7 Scene** | Requires simultaneous visibility of all canonical objects plus road topology                      |
| Post-hoc kinematic analysis (braking events, stopping distances) | L8 Analytics | Derived measurements over historical L5/L7 state                                                  |

The prediction pipeline flows:

```
L5 kinematic prediction (unconstrained trajectory fan)
    ↓
L7 scene constraint (clip to road polygon, respect kerb boundaries)
    ↓
L7 interaction constraint (adjust for leading vehicle, gap acceptance)
    ↓
L7 constrained path probability distribution
```

**Design doc:** [lidar-bodies-in-motion-plan.md](lidar-bodies-in-motion-plan.md) expands this into a full implementation plan including sparse-cluster track linking and path prediction. **Maths proposal:** [bodies-in-motion-maths](../../data/maths/proposals/) (to be written).

### 4.2 Scene graph: geometric constraint relationships

"Cluster expected to touch ground plane" and "height above ground / base clamped" are instances of typed spatial relationships between features of different classes. The set of these relationships forms a scene graph.

**Per-frame queries (L4: stateless, planned):**

L4 would own the primitive geometric queries that run every frame. Currently, ground removal uses a simple `HeightBandFilter` (fixed Z-band gating). The queries below require the planned `GroundSurface` interface (see [ground-plane-extraction.md](../lidar/architecture/ground-plane-extraction.md)):

- `GroundSurface.QueryHeightAboveGround(x, y, z)`: point height relative to local ground
- Per-frame ground-contact check: "is this cluster's lowest point within 20 cm of the ground surface?"
- Base-Z clamping during cluster extraction: `cluster.BaseZ = max(clusterMinZ, groundZ)`

These would be stateless and would not require accumulated geometry or cross-frame state.

**Accumulated relationships (L7: stateful):**

The persistent relationship graph lives at L7 because it represents evidence accumulated over many frames:

```
Feature A          Relation              Feature B
─────────────────────────────────────────────────────
Cluster         → contacts_ground →     GroundPolygon
Cluster         → occluded_by    →     StructureFeature
Track           → follows_road   →     GroundPolygon
Track           → constrained_by →     GroundPolygon     (base Z clamped)
CanonicalObject → rests_on       →     GroundPolygon
CanonicalObject → bounded_by     →     StructureFeature  (cannot pass through)
VolumeFeature   → rooted_in      →     GroundPolygon     (tree base on ground)
```

Each relation carries accumulated confidence and relation-specific parameters:

- `contacts_ground`: expected base-Z offset from ground surface (typically ~0 for vehicles)
- `follows_road`: corridor width constraint
- `occluded_by`: occlusion angle range

**Key distinction:** L4 answers "what is the ground height here?" (stateless query). L7 answers "this object consistently contacts this ground polygon" (accumulated evidence) and uses that relationship to constrain predicted paths.

### 4.3 Ground-contact flow through the layers

```
L3 Background Grid
    ↓ settled polar cells with height statistics
L4 Perception
    ├── GroundSurface.QueryHeightAboveGround(x, y, z)     ← per-frame primitive
    ├── Cluster extraction: baseZ = max(clusterMinZ, groundZ)  ← per-frame clamp
    └── Per-frame observation: "cluster C contacts ground at (x,y)"
         ↓
L5 Tracks
    ├── Track state: z-component constrained by ground query
    └── Predicted base-Z uses ground surface as floor constraint
         ↓
L7 Scene
    ├── Accumulated relation: CanonicalObject → contacts_ground → GroundPolygon
    ├── Refined base offset (Welford mean of observed base-Z minus ground-Z)
    └── Physics constraint: predicted path base-Z follows ground polygon slope
```

---

## 5. References

### Scene and map construction

| Reference                                                                                                | Relevance                                                                                                                                   |
| -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Bao et al. (2022): HD-map generation survey (`Bao2022HDMap`); Li et al. (2022): HDMapNet (`Li2022HDMap`) | Survey and online map-learning framework, respectively; background for proposed L7 geometry, not validation of our stationary-sensor design |
| OpenStreetMap Simple 3D Buildings (S3DB), revision `3071724` (`OSMSimple3DBuildings`)                    | Proposed prior schema for building outlines, parts, and relative heights; ingestion remains planned                                         |
| Pannen et al. (2020): How to Keep HD Maps for Automated Driving Up To Date (ICRA 2020)                   | Map maintenance from live sensor data; directly relevant to our evidence-based refinement of priors                                         |
| CityJSON specification (v1.1)                                                                            | Interchange format for 3D city geometry; our L7 export target for building geometry                                                         |
| Hornung et al. (2013): **OctoMap** (doi:10.1007/s10514-012-9321-0)                                       | Probabilistic 3D occupancy mapping; log-odds update model used for scene feature confidence                                                 |
| Pomerleau et al. (2014): Long-term 3D map maintenance                                                    | Dynamic point removal from accumulated maps; related to our evidence-based scene refinement                                                 |
| [Radiance-field scene-model review](lidar-radiance-field-scene-model-review.md)                          | Decision note: classical L7 (this plan) supersedes radiance-field-style approaches for the persistent scene model                           |

### Multi-sensor tracking and fusion

| Reference                                                                                                          | Relevance                                                                                                                                                |
| ------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Bar-Shalom et al. (2011): Tracking and Data Fusion (Mathematics in Science and Engineering)                        | Canonical text on multi-sensor data fusion; covariance intersection, distributed Kalman filtering                                                        |
| Reid (1979): An algorithm for tracking multiple targets (IEEE TAC)                                                 | Multiple Hypothesis Tracking (MHT); the theoretical framework for multi-sensor association                                                               |
| Parker and Emmons (1997): Cooperative Multi-Robot Observation of Multiple Moving Targets (`Parker1997Cooperative`) | Mobile-robot placement and observation coverage; context for coverage planning, not a decentralised track-fusion method                                  |
| Dames (2017): Distributed Multi-Target Search and Tracking using the PHD Filter (`Dames2017`)                      | Distributed target-count/position estimation coupled to mobile-robot control; candidate estimator research for L7, requiring adaptation to fixed sensors |

The two mobile-robot papers do not establish our proposed canonical-object identity or
cross-sensor handoff scheme. The current L5 code uses a constant-velocity Kalman filter and
Hungarian assignment; neither a distributed PHD filter nor cooperative robot control is
implemented. The references use `Parker1997Cooperative` and `Bao2022HDMap` to match
the corrected authors and publication years.

### Physics-constrained prediction and scene graphs

| Reference                                                                                                | Relevance                                                                                              |
| -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Lefèvre et al. (2014): A survey on motion prediction and risk assessment for intelligent vehicles        | Taxonomy of physics-based, manoeuvre-based, and interaction-aware prediction; frames our L5→L7 split   |
| Schöller et al. (2020): What the Constant Velocity Model Can Teach Us About Pedestrian Motion Prediction | Surprisingly strong CV baseline; validates our L5 CV/CA starting point before adding scene constraints |
| Salzmann et al. (2020): Trajectron++: Dynamically-Feasible Trajectory Forecasting (ECCV 2020)            | Scene-conditioned trajectory prediction with dynamics integration; our L7 scene-constrained path model |
| Liang et al. (2020): Learning lane graph representations for motion forecasting (ECCV 2020)              | Lane-graph topology for trajectory prediction; relevant to our road-polygon corridor constraints       |

### Mathematical methods

| Reference                                                                      | Relevance                                                                                      |
| ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------- |
| Kalman (1960): A New Approach to Linear Filtering and Prediction Problems      | Foundation of the Kalman state estimator used in L5 tracking and L7 cross-sensor extrapolation |
| Welford (1962): Note on a method for calculating corrected sums                | Numerically stable online mean/variance; used for canonical object dimension refinement        |
| Schönemann (1966): A generalised solution of the orthogonal Procrustes problem | Closed-form rigid alignment via SVD; used for prior-to-observation registration                |
| Mahalanobis (1936): On the generalised distance in statistics                  | Statistical distance metric used for cross-sensor track association gating                     |
| Hornung et al. (2013): OctoMap                                                 | Log-odds occupancy update; adapted for vector-feature confidence accumulation                  |

## Vocabulary contract

The [published platform vocabulary](../platform/PLATFORM.md#platform-vocabulary) governs
this plan: capture is a static packet file; clip is an evaluation or annotation window;
run is a processing pass; labels are track verdicts; annotations are reviewed point masks.
Site means place, deployment means one sensor’s placement over a period, and survey means
one deployment’s measurement over a period. Scene is reserved for physical geometry.
Historical schema inventories describe their original state; implementation follows the
[D-31 cutovers](platform-vocabulary-and-data-model-plan.md#phase-1-under-d-31-three-renames)
and the [compatibility register](../platform/operations/compatibility-register.md).
New storage relationships remain Phase 2.
