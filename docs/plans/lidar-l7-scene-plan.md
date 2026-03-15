# L7 Scene and Multi-Sensor Fusion Plan

**Status:** 📋 Planned — v1.0
**Created:** 2026-03-08
**Layers:** L7 Scene
**Canonical architecture:** [lidar-data-layer-model.md](../lidar/architecture/lidar-data-layer-model.md)
**Maths index:** [data/maths/README.md](../../data/maths/README.md)

This plan covers two related workstreams for the L7 Scene layer:

1. **L7 Scene layer** — design the canonical world model package
2. **Multi-sensor architecture** — how multiple sensors fuse at L7

---

## 1. L7 Scene — canonical world model

L7 is the key architectural addition in the ten-layer model. It introduces a **persistent, evidence-accumulated representation of the world** that transcends individual frames, tracks, and sensors.

### What L7 Scene contains

1. **Static geometry** — ground surface polygons, building footprints, walls, fences, vegetation volumes, kerbs. Derived from L4 perception outputs accumulated over many frames. Stored as vector features with hierarchical LOD (0–3). See [vector-scene-map.md](../lidar/architecture/vector-scene-map.md).

2. **Dynamic canonical objects** — long-lived vehicle/pedestrian geometry inferred from merged L5 tracks. A single car observed across 200 frames produces one canonical object with refined dimensions, not 200 per-frame clusters. See [vector-scene-map.md](../lidar/architecture/vector-scene-map.md).

3. **External priors** — geometry imported from OpenStreetMap (S3DB building outlines, road axes), community GeoJSON, or manual survey. Priors are treated as low-confidence initial features that observation evidence can validate, refine, or reject.

4. **Multi-sensor merged scene** — when multiple sensors observe the same area, L7 fuses their independent L1–L6 pipelines into a single coherent world model (see [§ 2. Multi-sensor architecture](#2-multi-sensor-architecture) below).

5. **Uncertainty and provenance** — every scene feature carries confidence bounds, observation count, source sensor IDs, and edit history. User edits in VelocityVisualiser are tracked separately from automated refinement.

### L7 relationship to L4 Perception

L4 produces **per-frame, single-sensor observations**:

- Ground tiles, cluster bounding boxes, height-band classifications

L7 accumulates these into **persistent, multi-frame geometry**:

- Vector polygons, canonical objects, refined dimensions

The separation ensures L4 remains a fast, stateless, per-frame operation while L7 handles the slower, stateful accumulation across time and sensors.

### L7 relationship to L5/L6

L5 tracks are **ephemeral identity** — they live for the duration of an object's visibility to one sensor. L6 objects are **per-track semantic labels**.

L7 canonical objects are **persistent identity** — a car that drives through the intersection, disappears for 30 seconds, then returns can be re-identified as the same canonical object. When multiple sensors are deployed (future), L7 is where tracks from different sensors merge into a single coherent object trajectory.

### OSM priors service

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

Today, velocity.report runs one LiDAR sensor per deployment. Each sensor has its own L1–L6 pipeline instance. L7–L10 do not yet exist as code packages.

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

1. **Extended coverage** — two sensors on opposite sides of a bend cover the full trajectory of a turning vehicle, eliminating the blind zone.
2. **Multi-modality** — combine LiDAR (rich geometry, range accuracy) with radar (direct velocity measurement, weather resilience) for robust detection in all conditions.
3. **Occlusion resolution** — a pedestrian occluded from Sensor A may be visible to Sensor B; the merged scene shows the complete picture.
4. **Redundancy** — sensor failure or blockage (snow, spider web) is compensated by the remaining sensors.

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

- **Spatial gating** — predicted position from A's Kalman state vs. B's new detection position; gate by Mahalanobis distance
- **Velocity consistency** — heading and speed must agree within tolerance
- **Temporal proximity** — departure time from A and arrival time at B must be consistent with expected transit time across the gap
- **Appearance consistency** — dimensions and classification from A must match B's observations

---

## 3. Mathematical foundations

L7 builds on the mathematical machinery already established in L3–L5 and extends it to the persistent-geometry and multi-sensor domains.

### 3.1 Scene accumulation — Bayesian evidence grid

Static geometry accumulates via per-feature confidence updates. For each scene feature $f$ observed by frame evidence $z_t$:

$$P(f \mid z_{1:t}) \propto P(z_t \mid f) \cdot P(f \mid z_{1:t-1})$$

In practice, the log-odds formulation avoids numerical issues:

$$L(f \mid z_{1:t}) = L(f \mid z_{1:t-1}) + \log\frac{P(z_t \mid f)}{P(z_t \mid \neg f)}$$

This is the same occupancy-grid update used in OctoMap (Hornung et al., 2013) but applied to vector features rather than voxels. Each feature carries an observation count $N$ and accumulated log-odds confidence $L$.

**Related maths docs:** [background-grid-settling-maths.md](../../data/maths/background-grid-settling-maths.md) (L3 EMA settling), [ground-plane-maths.md](../../data/maths/ground-plane-maths.md) (L4 surface estimation).

### 3.2 Canonical object refinement — running sufficient statistics

When L5 tracks are promoted to L7 canonical objects, their geometry is refined using streaming Welford updates (identical to the L4 ground-plane estimator):

$$\mu_{n+1} = \mu_n + \frac{\delta}{n+1}, \quad C_{n+1} = C_n + \delta \cdot (x_{n+1} - \mu_{n+1})^T$$

where $\delta = x_{n+1} - \mu_n$ and $C$ is the running scatter matrix. This produces refined dimensions (length, width, height) for canonical objects without buffering individual observations.

**Related maths doc:** [ground-plane-vector-scene-maths proposal](../../data/maths/proposals/20260221-ground-plane-vector-scene-maths.md) § 3.1.

### 3.3 Cross-sensor track association — gated nearest-neighbour

Track handoff from Sensor A to Sensor B uses Mahalanobis-distance gating:

$$d_M^2 = (\hat{x}_A - z_B)^T S^{-1} (\hat{x}_A - z_B)$$

where $\hat{x}_A$ is the predicted state from A's Kalman filter extrapolated to B's detection time, $z_B$ is B's new detection, and $S = H P_A H^T + R_B$ is the innovation covariance combining A's prediction uncertainty with B's measurement noise.

Association is accepted when $d_M^2 < \chi^2_{3,\alpha}$ (3-DOF gate for position). Velocity consistency adds a second gate on heading/speed residuals.

**Related maths docs:** [tracking-maths.md](../../data/maths/tracking-maths.md) (L5 Kalman filter), [clustering-maths.md](../../data/maths/clustering-maths.md) (DBSCAN spatial indexing).

### 3.4 Prior-to-observation alignment — rigid transform estimation

OSM priors are aligned to sensor-observed geometry via Procrustes analysis. Given $n$ corresponding point pairs $\{(p_i, q_i)\}$ between prior and observed features:

$$\min_{R, t} \sum_{i=1}^{n} \| R p_i + t - q_i \|^2$$

Solved in closed form via SVD of the cross-covariance matrix $H = \sum (p_i - \bar{p})(q_i - \bar{q})^T$. The rotation $R = V U^T$ (with sign correction) and translation $t = \bar{q} - R \bar{p}$.

This aligns OSM building outlines and road geometry to the sensor's local coordinate frame without requiring GPS-grade localisation.

---

## 4. Scene-constrained physics and geometric relationships

L7 is also the natural home for two concerns that cannot live in lower layers: **physics-constrained trajectory prediction** and **persistent geometric relationships** (the scene graph).

### 4.1 Physics motion model — layer placement

Single-object kinematics (richer Kalman state vectors) belong at L5 — that is a per-track concern. But once prediction needs to account for scene geometry or multi-object interactions, L7 owns it because only L7 holds the accumulated road polygons, kerb boundaries, structure walls, and the set of all canonical objects.

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

### 4.2 Scene graph — geometric constraint relationships

"Cluster expected to touch ground plane" and "height above ground / base clamped" are instances of typed spatial relationships between features of different classes. The set of these relationships forms a scene graph.

**Per-frame queries (L4 — stateless):**

L4 owns the primitive geometric queries that run every frame:

- `GroundSurface.QueryHeightAboveGround(x, y, z)` — point height relative to local ground
- Per-frame ground-contact check: "is this cluster's lowest point within 20 cm of the ground surface?"
- Base-Z clamping during cluster extraction: `cluster.BaseZ = max(clusterMinZ, groundZ)`

These are stateless and do not require accumulated geometry or cross-frame state.

**Accumulated relationships (L7 — stateful):**

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

- `contacts_ground` — expected base-Z offset from ground surface (typically ~0 for vehicles)
- `follows_road` — corridor width constraint
- `occluded_by` — occlusion angle range

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

| Reference                                                                               | Relevance                                                                                                                |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| HD Map construction surveys (Liu et al., 2020; Li et al., 2022)                         | Canonical approach to persistent vector map construction from LiDAR; our L7 follows this paradigm at neighbourhood scale |
| OpenStreetMap Simple 3D Buildings (S3DB) specification                                  | Prior source for building outlines; our priors service ingests S3DB tags                                                 |
| Pannen et al. (2020) — How to Keep HD Maps for Automated Driving Up To Date (ICRA 2020) | Map maintenance from live sensor data; directly relevant to our evidence-based refinement of priors                      |
| CityJSON specification (v1.1)                                                           | 3D city model interchange format; our L7 export target for building geometry                                             |
| Hornung et al. (2013) — **OctoMap** (doi:10.1007/s10514-012-9321-0)                     | Probabilistic 3D occupancy mapping; log-odds update model used for scene feature confidence                              |
| Pomerleau et al. (2014) — Long-term 3D map maintenance                                  | Dynamic point removal from accumulated maps; related to our evidence-based scene refinement                              |

### Multi-sensor tracking and fusion

| Reference                                                                                    | Relevance                                                                                         |
| -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Bar-Shalom et al. (2011) — Tracking and Data Fusion (Mathematics in Science and Engineering) | Canonical text on multi-sensor data fusion; covariance intersection, distributed Kalman filtering |
| Reid (1979) — An algorithm for tracking multiple targets (IEEE TAC)                          | Multiple Hypothesis Tracking (MHT); the theoretical framework for multi-sensor association        |
| Kim & Liu (2017) — Cooperative multi-robot observation of targets                            | Decentralised track fusion across sensor nodes; relevant to our distributed edge architecture     |
| Dames & Kumar (2017) — Detecting, localising, and tracking an unknown number of targets      | Multi-sensor PHD filter; advanced alternative to our proposed gating-based approach               |

### Physics-constrained prediction and scene graphs

| Reference                                                                                                 | Relevance                                                                                              |
| --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Lefèvre et al. (2014) — A survey on motion prediction and risk assessment for intelligent vehicles        | Taxonomy of physics-based, manoeuvre-based, and interaction-aware prediction; frames our L5→L7 split   |
| Schöller et al. (2020) — What the Constant Velocity Model Can Teach Us About Pedestrian Motion Prediction | Surprisingly strong CV baseline; validates our L5 CV/CA starting point before adding scene constraints |
| Salzmann et al. (2020) — Trajectron++: Dynamically-Feasible Trajectory Forecasting (ECCV 2020)            | Scene-conditioned trajectory prediction with dynamics integration; our L7 scene-constrained path model |
| Liang et al. (2020) — Learning lane graph representations for motion forecasting (ECCV 2020)              | Lane-graph topology for trajectory prediction; relevant to our road-polygon corridor constraints       |

### Mathematical methods

| Reference                                                                       | Relevance                                                                                      |
| ------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Kalman (1960) — A New Approach to Linear Filtering and Prediction Problems      | Foundation of the Kalman state estimator used in L5 tracking and L7 cross-sensor extrapolation |
| Welford (1962) — Note on a method for calculating corrected sums                | Numerically stable online mean/variance; used for canonical object dimension refinement        |
| Schönemann (1966) — A generalised solution of the orthogonal Procrustes problem | Closed-form rigid alignment via SVD; used for prior-to-observation registration                |
| Mahalanobis (1936) — On the generalised distance in statistics                  | Statistical distance metric used for cross-sensor track association gating                     |
| Hornung et al. (2013) — OctoMap                                                 | Log-odds occupancy update; adapted for vector-feature confidence accumulation                  |
