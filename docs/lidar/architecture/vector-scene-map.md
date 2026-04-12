# 3D Vector Scene Map — Architecture Specification

- **Status:** Proposed
- **Layers:** L4 Perception (extends `GroundSurface` interface)
- **Related:** [ground-plane-extraction.md](./ground-plane-extraction.md), [ground-plane-vector-scene-maths.md](../../../data/maths/proposals/20260221-ground-plane-vector-scene-maths.md), [lidar-data-layer-model.md](./lidar-data-layer-model.md)

This specification extends the ground-plane tiled-grid model into a polygon-based vector scene map, enabling variable-resolution representation of ground surfaces, buildings, vegetation, and other static scene features.

---

## 1. Overview & Motivation

### From Tiled Grid to Vector Polygons

The existing ground plane specification (`ground-plane-extraction.md`) models the road surface as a **uniform tiled grid** — a mosaic of 1 m × 1 m tiles, each with an independent plane equation. This approach is efficient for flat-ish road surfaces but has inherent limitations when extended to describe the full observable scene:

1. **Tile uniformity wastes storage on homogeneous regions** — A straight, flat stretch of road produces hundreds of nearly-identical 1 m tiles where a single polygon would suffice.
2. **Buildings and walls are vertical surfaces** — Tiles are inherently horizontal (Z-up plane fits). Vertical structures require a fundamentally different representation: wall-plane parameters or bounding polygons with corner coordinates.
3. **Vegetation and irregular shapes don't conform to planes** — Trees, hedges, and overhanging foliage need approximate bounding volumes, not surface equations.
4. **No mechanism for variable detail** — A kerb edge and a featureless road centre receive identical 1 m resolution. Precision should follow measurement confidence and feature complexity.

The **vector scene map** extends the ground plane concept into a **polygon-based feature representation** where:

- **Ground surfaces** are described by polygons (not uniform tiles) with corner coordinates and per-polygon plane parameters. Large flat regions collapse into single polygons; fine detail emerges only at boundaries and discontinuities.
- **Buildings and walls** are described by vertical-plane polygons capturing coarse footprint and height without point explosion.
- **Vegetation and irregular objects** are described by approximate bounding volumes (convex hulls or oriented bounding boxes).
- **Detail varies by need** — A multi-resolution hierarchy stores coarse polygons globally and refined polygons only where measurement accuracy demands it.

### Architecture Principles

All principles from the ground plane specification carry forward:

- **Sensor-iterative (LiDAR-only first):** The vector scene map must function with LiDAR alone. GPS is additive — enables geographic alignment (Tier 2) but never required for local scene mapping.
- **Non-point-based output:** The scene map publishes polygons, plane equations, and bounding volumes — not raw point clouds. This matches the `GroundSurface` interface pattern: geometry derived from points but exposed as compact mathematical surfaces.
- **Incremental, streaming-compatible:** Features must be constructable incrementally from streaming LiDAR frames. Batch (PCAP) mode may use multi-pass refinement, but single-pass operation must be viable.
- **Privacy-first:** No camera data, no PII. Only geometric features (polygons, planes, volumes).

### Relationship to Existing Systems

The vector scene map is an **evolution** of the ground plane, not a replacement. It builds upon the same L4 Perception position:

```
L1 Packets → L2 Frames → L3 Background Grid → L4 Perception → L5 Tracks → L6 Objects
                                                  ├── Clustering (point-based)
                                                  ├── Ground Plane (surface tiles)
                                                  ├── Vector Scene Map (polygon features)
                                                  └── Height-above-ground queries
```

- The **ground plane** remains the core height reference for traffic monitoring.
- The **vector scene map** adds structural context: buildings define occlusion boundaries, vegetation defines clutter zones, refined ground polygons capture kerb geometry.
- The **clustering** system can query the scene map to distinguish "cluster near a known wall" from "cluster in open road".

---

## 2. Feature Taxonomy

### Three Feature Classes

Every observable scene element falls into one of three geometric classes:

| Class         | Geometric Model                                 | Typical Features                                        | Primary Attribute                                |
| ------------- | ----------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------ |
| **Ground**    | Horizontal/sloped polygon + plane equation      | Road surfaces, pavements, driveways, kerbs, crosswalks  | Plane normal, elevation, planarity               |
| **Structure** | Vertical polygon(s) + height extent             | Buildings, walls, fences, retaining walls, bridge piers | Wall-plane equation, height range, footprint     |
| **Volume**    | 3D bounding shape (OBB, convex hull, or sphere) | Trees, hedges, awnings, overhanging signs, light poles  | Approximate centre, bounding dimensions, density |

### 2.1 Ground Features

Ground features extend the existing `GroundTile` concept but replace the fixed grid with **variable-size polygons**:

```
┌──────────────────────────────────────────────────────────┐
│                                                          │
│  ┌─────────────────────────────┐                         │
│  │                             │  Large flat polygon     │
│  │    Road surface             │  (single plane, many m²)│
│  │    planarity ≥ 0.98         │                         │
│  │                             │                         │
│  └─────────┬───────────────────┘                         │
│            │ kerb edge (narrow polygon, 0.3m wide)       │
│  ┌─────────▼───────────────────┐                         │
│  │ Pavement  │ Driveway ramp   │  Two smaller polygons   │
│  │ (flat)    │ (5° slope)      │  with different normals │
│  └───────────┴─────────────────┘                         │
└──────────────────────────────────────────────────────────┘
```

Each ground polygon stores:

| Field        | Type           | Description                                                |
| ------------ | -------------- | ---------------------------------------------------------- |
| `Boundary`   | `[][2]float64` | Ordered vertices defining polygon boundary (closed ring)   |
| `Normal`     | `[3]float64`   | Best-fit plane unit normal                                 |
| `Offset`     | `float64`      | Plane offset: **n** · **p** = d                            |
| `Planarity`  | `float32`      | Eigenvalue-ratio confidence [0, 1]                         |
| `PointCount` | `uint32`       | Source point observations                                  |
| `Class`      | `GroundClass`  | `road`, `pavement`, `kerb`, `ramp`, `crosswalk`, `unknown` |
| `LOD`        | `uint8`        | Level of detail (0 = coarsest, 3 = finest)                 |

**Key difference from tiles:** A single large polygon with 4–6 vertices replaces dozens of identical 1 m tiles for flat road segments. Corner positions are computed from the convex hull of contributing tile centres (or point clusters), giving **sub-tile positional accuracy** at polygon boundaries.

### 2.2 Structure Features (Buildings, Walls)

Structures are vertical surfaces observed as reflective returns from building facades, fences, and walls. They are modelled as:

- **Footprint polygon** — 2D outline (viewed from above) defining the structure's horizontal extent.
- **Vertical plane equation** — For each wall segment, a near-vertical plane fit: normal ≈ (nx, ny, 0) with nz ≈ 0.
- **Height range** — [Z_min, Z_max] observed extent above ground, capped by sensor visibility.

> **Source:** `StructureFeature` and `WallSegment` structs in `internal/lidar/` (when implemented). Fields: FootprintVertices, per-wall Normal/Offset/Planarity, ZMin/ZMax, PointCount, Confidence, LOD.

**Why not store full 3D meshes?** We don't need photorealistic building models. A few wall planes with corner coordinates capture the coarse structure visible to LiDAR, sufficient for:

- **Occlusion reasoning** — "Is a potential track target behind a known building wall?"
- **Scene context** — "This cluster is adjacent to a known building facade" (not free-standing).
- **Map publishing** — Export building outlines as GeoJSON polygons for GIS integration.

### 2.3 Volume Features (Vegetation, Irregular Shapes)

Trees, hedges, and overhanging features don't conform to single planes. They produce diffuse, scattered returns. Model them as **approximate bounding volumes**:

> **Source:** `VolumeFeature` and `BoundingKind` in `internal/lidar/` (when implemented). Fields: BoundingType (OBB/ConvexHull/Sphere), Centre, Dimensions, Orientation, HullVertices, PointCount, PointDensity, ApproxVolume, Class (tree/hedge/sign_cluster/awning/unknown), LOD, Confidence.

**Point density** is the distinguishing attribute: a tree canopy has high spatial extent but low density (many gaps between returns), while a solid pole has small extent but high density. This attribute helps L6 classification without storing raw point clouds.

---

## 3. Hierarchical Levels of Detail

### The Problem

A LiDAR sensor observing a street scene can produce:

- 50+ ground tiles that are essentially the same flat road surface
- 8–12 building wall segments along one block
- 3–5 tree canopies with hundreds of sparse returns each

Storing all of this at maximum resolution everywhere is wasteful. Conversely, a single coarse polygon for "the entire road" loses kerb edges and driveway dips.

### Multi-Resolution Hierarchy

The vector scene map uses **four levels of detail (LOD 0–3)**, inspired by mapping conventions (zoom levels) and point cloud LOD systems:

| LOD | Label       | Typical Scale | Ground Resolution                              | Structure Resolution                         | Volume Resolution                 | Use Case                                       |
| --- | ----------- | ------------- | ---------------------------------------------- | -------------------------------------------- | --------------------------------- | ---------------------------------------------- |
| 0   | **Block**   | > 50 m        | Single polygon per road segment                | Building footprint (rectangle)               | Vegetation zone (bounding sphere) | Global map overview, neighbourhood context     |
| 1   | **Street**  | 10–50 m       | Polygon per lane or surface type               | Simplified wall outlines (4–8 vertices)      | Tree canopy OBBs                  | Street-level navigation, occlusion reasoning   |
| 2   | **Feature** | 2–10 m        | Polygon per surface discontinuity (kerb, ramp) | Individual wall planes with accurate corners | Trimmed convex hulls              | Detailed mapping, GIS export, change detection |
| 3   | **Survey**  | < 2 m         | Sub-metre polygons at high-accuracy zones      | Wall planes with cm-level corner positions   | Dense bounding volumes            | Surveyed benchmarks, driveway dip profiles     |

### LOD Hierarchy as a Tree

Each feature exists at a **single primary LOD** but can be **refined** by splitting into child features at a finer level:

```
LOD 0: Road_Segment_Main_Street
  ├── LOD 1: Lane_Northbound (flat, planarity 0.99)
  │     ├── LOD 2: Kerb_East_Edge (linear polygon, 0.3m wide, +0.15m step)
  │     ├── LOD 2: Driveway_Dip_123 (sloped polygon, 5° grade)
  │     └── LOD 2: Crosswalk_Zone (flat, painted markings)
  └── LOD 1: Lane_Southbound (flat, planarity 0.98)
        └── LOD 2: Pothole_456 (depressed polygon, -0.08m)
```

**Properties of the hierarchy:**

- **LOD 0** always exists for every feature — it provides the coarsest representation.
- **LOD 1–3** exist **only where measurement accuracy demands refinement** or where an explicit survey has been performed.
- **Parent polygons completely contain their children** — the LOD 0 polygon's boundary encloses all LOD 1 polygons, which in turn enclose LOD 2, etc.
- **Querying at LOD N returns that level if available, otherwise falls back to the coarsest ancestor.**

### LOD Data Model

> **Source:** `SceneFeature`, `FeatureID`, and `FeatureClass` in `internal/lidar/` (when implemented). `SceneFeature` wraps ID, Class (Ground/Structure/Volume), LOD (0–3), ParentID, and exactly one of `*GroundFeature`, `*StructureFeature`, `*VolumeFeature`. Common metadata: PointCount, Confidence, LastUpdatedNanos, Settled.

---

## 4. Multi-Resolution Design: Coarse-to-Detail Transitions

### 4.1 Coarse Polygon Construction (LOD 0–1)

**Ground:** Starting from settled Tier 1 ground tiles (1 m grid), merge adjacent tiles with similar plane parameters into larger polygons.

**Merging criterion for adjacent ground tiles:**

```
merge if:
    angle(tile_a.Normal, tile_b.Normal) < merge_angle_threshold  (e.g., 2°)
    AND |tile_a.ZAt(shared_edge) - tile_b.ZAt(shared_edge)| < merge_z_threshold  (e.g., 3 cm)
```

**Algorithm (region growing):**

1. Seed from an unseen settled tile.
2. Flood-fill to neighbours meeting the merge criterion.
3. Compute the merged polygon boundary as the convex hull (or alpha-shape) of the region's tile centres.
4. Re-fit a single plane equation over all contributing tiles' statistics (weighted by point count).
5. Assign LOD 0 if region spans > 10 m; LOD 1 if 2–10 m.

This produces LOD 0–1 ground polygons: large flat regions collapse into single polygons with a handful of vertices. The merge thresholds guarantee that the single-polygon plane equation remains accurate within 3 cm.

**Structures:** Construct LOD 0 building footprints from L3 background grid cells classified as static vertical surfaces. Group contiguous vertical cells into rectangular outlines (minimum bounding rectangle). Refine wall segments at LOD 1 by fitting per-edge planes.

**Volumes:** Construct LOD 0 vegetation bounds from clusters of scattered returns that don't fit ground or structure models. Use bounding spheres for LOD 0; refine to OBBs or convex hulls at LOD 1.

### 4.2 Selective Refinement (LOD 2–3)

LOD 2–3 features exist **only where justified** by one of:

| Trigger                  | Description                                               | Example                                     |
| ------------------------ | --------------------------------------------------------- | ------------------------------------------- |
| **High curvature**       | Adjacent LOD 1 polygons have plane normal difference > 5° | Kerb edges, driveway ramps, speed humps     |
| **Height discontinuity** | Z-offset jump > 10 cm at polygon boundary                 | Kerb step-ups, loading dock edges           |
| **High point density**   | > 50 points/m² observed in a region                       | Near-field surfaces close to sensor         |
| **Explicit survey**      | User or automated survey marks a region for detail        | Driveway dip profile, intersection geometry |
| **OSM anchor import**    | Known geometric feature imported from OpenStreetMap       | Crosswalk boundaries, stop lines            |

**Refinement process:**

1. **Identify refinement zone** — Find boundaries within LOD 0–1 polygons where the merge criterion was marginal (near-threshold) or where curvature exceeds the threshold.
2. **Split parent polygon** — Subdivide the parent into two or more child polygons along the discontinuity boundary.
3. **Re-fit child planes** — Each child polygon gets its own plane equation fitted from its contributing tiles/points.
4. **Link hierarchy** — Set `ParentID` on children; parent retains its coarse representation for fast queries.

**Example — kerb refinement:**

```
LOD 1: Road_Segment (planarity 0.97, single flat polygon)
  ↓ curvature > 5° detected at eastern boundary
LOD 2: Road_Surface (planarity 0.99, flat, interior polygon)
LOD 2: Kerb_Strip (planarity 0.92, narrow polygon, +0.15m Z-step)
LOD 2: Pavement_Beyond (planarity 0.97, flat, +0.15m elevated)
```

The LOD 1 parent polygon is **preserved** — querying at LOD 1 still returns the single flat road polygon. Querying at LOD 2 returns the three sub-polygons.

### 4.3 Threshold-Based Pruning and Compression

**Pruning rules** prevent detail explosion:

| Rule                                 | Threshold                                  | Effect                                                 |
| ------------------------------------ | ------------------------------------------ | ------------------------------------------------------ |
| **Minimum polygon area**             | > 0.25 m² (LOD 2), > 0.1 m² (LOD 3)        | Prevents micro-polygons from noise                     |
| **Minimum vertex count**             | ≥ 3 (triangle minimum)                     | Degenerate geometries discarded                        |
| **Maximum vertex count per polygon** | ≤ 32 (LOD 0–1), ≤ 64 (LOD 2–3)             | Limits complex boundaries; simplify if exceeded        |
| **Planarity improvement threshold**  | Splitting must improve planarity by ≥ 0.05 | Don't split if the children aren't meaningfully better |
| **Confidence minimum**               | ≥ 0.70 for any exported polygon            | Low-confidence polygons stay at parent LOD             |
| **Stale timeout**                    | 120 s for LOD 2–3 features                 | Detailed features decay faster than coarse             |

**Vertex reduction (Douglas-Peucker):**

Polygon boundaries are simplified using the Douglas-Peucker algorithm with tolerance tuned per LOD:

| LOD | Douglas-Peucker Tolerance | Typical Vertices per Polygon |
| --- | ------------------------- | ---------------------------- |
| 0   | 2.0 m                     | 4–6                          |
| 1   | 0.5 m                     | 6–12                         |
| 2   | 0.1 m                     | 8–24                         |
| 3   | 0.02 m                    | 12–48                        |

This ensures coarse polygons are extremely compact (4–6 vertices for a road block) while survey-grade detail can express sub-decimetre corner accuracy.

**Compression (storage):**

- Polygons are stored as delta-encoded vertex lists (relative to polygon centroid) with quantisation per LOD.
- LOD 0–1: vertices quantised to 10 cm → 2 bytes per coordinate → 12 bytes/vertex.
- LOD 2–3: vertices quantised to 1 cm → 4 bytes per coordinate → 24 bytes/vertex.
- Plane parameters: float32 (normal) + float32 (offset) = 16 bytes per polygon regardless of LOD.

---

## 5. Storage Model

### 5.1 In-Memory Representation

> **Source:** `VectorSceneMap` struct in `internal/lidar/` (when implemented). Holds `map[FeatureID]*SceneFeature` with RWMutex, spatial index (QuadTree or R-tree), per-LOD feature ID lists, and per-class counters.

### 5.2 Storage Budget

**Comparison: tiled grid vs vector polygons for a 100 m × 100 m scene**

Compressed sizes assume gzip compression at ~4:1 ratio for tile grids (high redundancy in similar plane parameters) and ~3:1 for vector polygons (less redundancy due to variable geometry). These ratios are consistent with observed gzip performance on similar structured data (see [`ground-plane-maths.md`](../../../data/maths/proposals/20260221-ground-plane-vector-scene-maths.md) §10).

| Approach                    | Representation  | Element Count | Per-Element Size | Total Raw | Total Compressed (~3–4:1) |
| --------------------------- | --------------- | ------------- | ---------------- | --------- | ------------------------- |
| **Tiled grid (1 m)**        | GroundTile      | 10,000        | 128 bytes        | 1.28 MB   | ~320 KB (4:1)             |
| **Vector LOD 0**            | Ground polygons | ~15           | ~100 bytes       | ~1.5 KB   | ~0.5 KB (3:1)             |
| **Vector LOD 0–1**          | All polygons    | ~50           | ~120 bytes       | ~6 KB     | ~2 KB (3:1)               |
| **Vector LOD 0–2**          | All polygons    | ~200          | ~140 bytes       | ~28 KB    | ~9 KB (3:1)               |
| **Vector LOD 0–3 (survey)** | All polygons    | ~500          | ~180 bytes       | ~90 KB    | ~30 KB (3:1)              |

Even at maximum detail (LOD 3 everywhere), the vector representation is **14× more compact** than the uniform tiled grid. In practice, LOD 3 is applied selectively to < 5% of the scene area, yielding typical storage around **10–30 KB compressed** for a full intersection.

**Structure and volume features add modest overhead:**

| Feature Type               | Typical Count per Scene | Per-Feature Size | Total   |
| -------------------------- | ----------------------- | ---------------- | ------- |
| Structure (building, wall) | 5–15                    | ~200 bytes       | ~3 KB   |
| Volume (tree, hedge)       | 3–10                    | ~120 bytes       | ~1.2 KB |

**Total scene budget (100 m × 100 m):** ~35 KB compressed (ground + structures + vegetation at mixed LOD).

### 5.3 SQLite Persistence Schema

> **Source:** Schema in `internal/db/migrations/` (when implemented). Two tables: `vector_scene_features` (feature_id, parent_id, class, lod, settled, confidence, point_count, last_updated, geometry_blob) and `vector_scene_snapshots` (versioned map states with gzip-compressed gob-encoded features, SHA256 dedup, LOD distribution JSON). Indices on class, lod, and parent_id.

### 5.4 Export Formats

| Format           | Use Case                     | Ground                                 | Structures                              | Volumes                               |
| ---------------- | ---------------------------- | -------------------------------------- | --------------------------------------- | ------------------------------------- |
| **GeoJSON**      | GIS tools (QGIS, Mapbox)     | Polygon features with plane properties | Polygon features with height properties | Point features with bounding metadata |
| **GeoPackage**   | Offline GIS with multi-layer | Separate layer per class               | Separate layer per class                | Separate layer per class              |
| **VTK PolyData** | ParaView / LidarView         | PolyData with cell normals             | PolyData with wall planes               | Glyph data (OBBs as boxes)            |
| **CityJSON**     | 3D city modelling            | Terrain surface                        | LOD 1–2 building shells                 | Vegetation objects                    |

GeoJSON remains the default export, consistent with the ground plane export specification. Structure: one `FeatureCollection` per class, filtered by LOD. Each Feature carries a GeoJSON `Polygon` geometry plus class-specific properties: ground features include `plane_normal`, `plane_offset`, `planarity`, `area_m2`, and `parent_id`; structure features include `z_min`, `z_max`, and `wall_count`. All features carry `id`, `class`, `subclass`, `lod`, and `point_count`. A top-level `metadata` object records `scene_map_version`, `lod_filter`, `coordinate_system`, and `feature_count`.

---

## 6. Construction Pipeline

### 6.1 From Ground Tiles to Vector Polygons

The vector scene map is built **on top of** the settled Tier 1 ground plane grid. The pipeline:

```
L3 Background Grid (polar, foreground/background)
    ↓
L4 Ground Plane Grid (Tier 1: 1m tiles, sensor-local)
    ↓ (tiles settle, planes fitted)
Vector Scene Map Constructor
    ├── Ground polygon merging (region growing on settled tiles)
    ├── Structure extraction (vertical surface grouping from L3 static cells)
    ├── Volume extraction (scattered return clustering from L4 clusters)
    └── LOD assignment + pruning
```

**Timing:** The vector scene map is a **post-settlement** artefact. It is constructed after the ground plane grid reaches sufficient coverage (e.g., > 60% of near-field tiles settled). It can be recomputed periodically (every 30 s) or on-demand for export.

### 6.2 Ground Polygon Construction

**Step 1: Region Growing**

Starting from the set of settled ground tiles:

```
1. Mark all settled tiles as unvisited.
2. While unvisited tiles remain:
   a. Pick an unvisited tile as seed.
   b. Initialise region R = {seed}.
   c. For each tile T adjacent to R:
      - If T is settled, unvisited, and passes merge criterion:
        Add T to R, mark visited.
   d. Compute boundary polygon of R (alpha-shape or convex hull).
   e. Re-fit single plane equation over R's combined statistics.
   f. Emit as SceneFeature{Class: Ground, LOD: LOD_from_area(R)}.
```

**Step 2: Boundary Refinement**

For each pair of adjacent ground polygons where the plane parameters differ significantly (curvature > merge threshold):

```
1. Identify the shared boundary edge.
2. If curvature at boundary > refinement_threshold (5°) OR Z-step > 10 cm:
   a. Create LOD 2 child polygons along the boundary strip.
   b. Each child gets its own plane fit from the tiles in the boundary zone.
   c. Link children to parent.
```

**Step 3: Vertex Simplification**

Apply Douglas-Peucker simplification to each polygon boundary with LOD-appropriate tolerance.

### 6.3 Structure Extraction

**Input:** L3 background grid cells classified as static **and** with near-vertical surface orientation (normal Z-component < 0.3, indicating a wall-like surface).

**Algorithm:**

1. **Group vertical cells** — Cluster contiguous vertical L3 cells using connected-component labelling (4-connected in the polar grid, projected to Cartesian).
2. **Fit footprint** — Project cluster to XY plane, compute minimum bounding rectangle (or convex hull for complex shapes).
3. **Fit wall planes** — For each edge of the footprint polygon, collect points near that edge and fit a vertical plane (constrain normal to horizontal ± 15°).
4. **Estimate height** — Record Z_min and Z_max from contributing points.
5. **Emit** as `SceneFeature{Class: Structure}` at LOD determined by footprint area.

### 6.4 Volume Extraction

**Input:** L4 clusters that are:

- Not classified as ground (height above ground > 0.5 m)
- Not classified as structure (low planarity, scattered returns)
- Persistent across multiple frames (static background objects)

**Algorithm:**

1. **Identify persistent clusters** — From L5 tracking, find tracked objects with near-zero velocity over > 30 seconds.
2. **Compute bounding volume** — OBB from existing `ComputeOBB()` function in `l4perception/obb.go`.
3. **Estimate density** — `point_count / bounding_volume` — distinguishes solid objects (poles: high density) from diffuse (trees: low density).
4. **Emit** as `SceneFeature{Class: Volume}` at LOD determined by bounding volume dimensions.

---

## 7. Querying the Scene Map

### 7.1 SceneSurface Interface

The vector scene map publishes a query interface that extends `GroundSurface`:

> **Source:** `SceneSurface` interface in `internal/lidar/` (when implemented). Embeds `GroundSurface` and adds: `FeaturesAt(x, y, maxLOD)`, `GroundPolygonAt(x, y, maxLOD)`, `NearestStructure(x, y, radius)`, `VolumesInRadius(x, y, radius)`, `FeaturesInBBox(xMin, yMin, xMax, yMax, maxLOD)`.

### 7.2 LOD Fallback Semantics

When querying at a specific LOD:

1. **Exact match** — If a feature exists at the requested LOD covering (x, y), return it.
2. **Finer available** — If finer LOD exists (child features), ignore them (user asked for coarser).
3. **Coarser fallback** — If no feature exists at the requested LOD, traverse up the parent chain until a covering feature is found.
4. **No coverage** — If no feature at any LOD covers (x, y), return `ok = false`.

This ensures that **every query returns something if any coverage exists**, regardless of which LOD the caller requests.

---

## 8. Gap-Free Navigation Between LOD Levels

### 8.1 The Global Map Problem

At the global level (Tier 2, lat/long-aligned), the map should be navigable as a coherent simplified representation:

- **Street-level polygons** (LOD 0–1) form a continuous surface with major boundaries visible: road edges, building outlines, vegetation zones.
- **Detail zones** (LOD 2–3) appear as nested refinements visible only when queried at higher LOD.

### 8.2 Transition Strategy

**Spatial containment invariant:** Every LOD N+1 feature is fully contained within its LOD N parent's boundary. This guarantees gap-free transition:

```
Query at LOD 0: Returns ~15 large ground polygons + building footprints + vegetation spheres.
                 → Coherent block-level view with no gaps.

Query at LOD 1: Returns ~50 polygons (lanes, simplified walls, tree OBBs).
                 → Street-level view with lane separation visible.

Query at LOD 2: Returns ~200 polygons (kerb detail, individual wall planes, trimmed hulls).
                 → Feature-level view with kerb edges and discontinuities visible.

Query at LOD 3: Returns ~500 polygons (surveyed benchmarks, precise corners).
                 → Survey-grade detail where available; LOD 2 elsewhere.
```

**Gap-free guarantee:** Because parent polygons are preserved, querying at any LOD returns full scene coverage. Finer features subdivide their parents but do not leave gaps. A client can render LOD 0 as the base layer, then overlay LOD 1+ detail where available.

### 8.3 Global-to-Local Mapping

When loading a global (Tier 2) vector scene map at session startup:

1. If GPS is available and OSM priors are enabled, fetch/cache **OSM Simple 3D Buildings (S3DB)** data for the sensor coverage area (building outlines, `building:part=*`, optional `type=building` relations).
2. Convert S3DB objects into LOD 0–1 **structure priors** (footprints, height extents, optional roof metadata) with source provenance (`osm_way`, `osm_relation`, timestamp, tag confidence).
3. Load local or community GeoJSON priors for **non-OSM geometry** (ground polygons, kerbs, crosswalks, vegetation zones) and any site-specific overrides.
4. Seed the local scene map with these priors, then let settled Tier 1 ground tiles and observed wall planes validate/refine them.
5. Create LOD 2–3 refinements where local observations reveal detail not present in the priors.

When merging back (local → global):

1. Ground/volume LOD 0–1 polygons merge with weighted averaging (same as the ground tile merge algorithm).
2. Structure changes are first compared against the originating OSM S3DB prior and emitted as **proposal candidates** (manual review), not auto-written upstream.
3. LOD 2–3 local refinements are added to the local/global scene map if they meet confidence thresholds; OSM-compatible subsets may also be exported as proposal candidates.
4. LOD 2–3 polygons with stale timestamps are pruned (detail zones are assumed to change more frequently than coarse structure).

---

## 9. Integration with Ground Plane

### Backward Compatibility

The vector scene map does **not replace** the tiled ground plane grid. The construction hierarchy is:

```
Ground Plane Grid (Tier 1: 1m tiles)   ←  Primary working data for real-time perception
    ↓
Vector Scene Map (LOD 0–3 polygons)    ←  Derived artefact for mapping, export, context
```

**Real-time perception** continues to query the `GroundSurface` interface (tile-based), which is faster for per-point height queries. The vector scene map is for:

- **Export** — GeoJSON, CityJSON, GeoPackage output.
- **Scene context** — "What structures are near this cluster?"
- **Multi-session mapping** — Accumulate building footprints and vegetation zones across sessions.
- **LOD-filtered views** — Provide simplified or detailed scene descriptions on demand.

### GroundSurface Delegation

The `VectorSceneMap` implements `GroundSurface` by delegating height queries to the underlying `GroundPlaneGrid` for real-time use. For offline/export use cases, the vector polygons can directly answer height queries using their plane equations.

---

## 10. Construction Thresholds and Configuration

### Default Parameters

> **Source:** `VectorSceneParams` struct in `internal/lidar/` (when implemented). Key defaults: merge angle 2°, merge Z-offset 3 cm, LOD 0 min area 50 m², LOD 1 min area 5 m², refinement curvature 5°, refinement Z-step 10 cm, min confidence 0.70, Douglas-Peucker tolerances [2.0, 0.5, 0.1, 0.02] m per LOD, structure min height 1.0 m, volume min persistence 30 s.

---

## 11. Implementation Roadmap

### Phase 1: Ground Polygon Merging

- [ ] Implement region-growing merge over settled ground tiles
- [ ] Alpha-shape or convex-hull boundary computation
- [ ] Re-fit merged plane equations (weighted by tile point counts)
- [ ] Douglas-Peucker vertex simplification
- [ ] LOD 0–1 assignment based on polygon area
- [ ] GeoJSON export of ground polygons
- [ ] Unit tests: merge criterion, boundary computation, planarity preservation

### Phase 2: Structure and Volume Extraction

- [ ] Vertical surface grouping from L3 background cells
- [ ] Footprint polygon construction (minimum bounding rectangle)
- [ ] Per-wall plane fitting with horizontal-normal constraint
- [ ] Volume extraction from persistent L5 tracked objects
- [ ] OBB and bounding sphere computation
- [ ] LOD assignment for structures and volumes
- [ ] GeoJSON export of all three feature classes

### Phase 3: Hierarchical LOD and Refinement

- [ ] Parent–child feature linkage (`ParentID`)
- [ ] Selective refinement at curvature/Z-step boundaries
- [ ] LOD 2–3 polygon creation from boundary zones
- [ ] LOD fallback query semantics
- [ ] Threshold-based pruning (area, vertex count, planarity improvement)
- [ ] `SceneSurface` interface implementation
- [ ] Integration tests with PCAP replay

### Phase 4: Multi-Session Mapping

- [ ] SQLite persistence (`vector_scene_features`, `vector_scene_snapshots`)
- [ ] Global-to-local prior loading at session startup
- [ ] Local-to-global merge with weighted averaging
- [ ] OSM S3DB structure-prior provider (Overpass/PBF import + local cache)
- [ ] S3DB → `StructureFeature` translation (outlines, `building:part=*`, height/roof tags, provenance)
- [ ] Scan-vs-S3DB delta detection (footprint alignment, height, missing parts)
- [ ] Export human-review OSM proposal bundles (`.osc`/`.osm`, GeoJSON diff, QA report)
- [ ] JOSM/iD review workflow docs (manual upload only; no automatic OSM writes)
- [ ] Stale feature pruning for LOD 2–3
- [ ] CityJSON export (3D building shells)
- [ ] Performance profiling on Raspberry Pi 4

---

## 12. Relationship to Future Work

### OSM Integration

The vector scene map provides natural anchor points for OpenStreetMap features:

- **Road polygons** (LOD 0–1) correspond to OSM `highway` ways.
- **Building footprints** (LOD 1) correspond to OSM `building` outlines.
- **Kerb polygons** (LOD 2) correspond to OSM `barrier=kerb` ways.
- **Crosswalk polygons** (LOD 2) correspond to OSM `highway=crossing` nodes/ways.

#### OSM Simple 3D Buildings as the Preferred Structure Prior

For **structure features** (buildings, building parts, walls), the preferred
Tier 2 prior source should be **OpenStreetMap Simple 3D Buildings (S3DB)**
rather than a velocity.report-specific GeoJSON corpus.

Rationale:

- OSM already provides a global, community-maintained building base map.
- S3DB adds the 3D attributes we need for coarse structure priors (`height`,
  `min_height`, `building:levels`, `roof:*`).
- Using OSM as the default structure prior reduces duplicate datasets and makes
  our refinement work easier to contribute back upstream.
- The supplemental GeoJSON prior service remains useful for geometry that is
  poorly represented in OSM today (ground surfaces, kerbs, vegetation zones,
  local survey refinements).

**S3DB import mapping (OSM → vector scene map):**

| OSM construct / tags                                  | Vector scene usage                                       | Notes                                           |
| ----------------------------------------------------- | -------------------------------------------------------- | ----------------------------------------------- |
| `building=*` outline (way or multipolygon)            | LOD 0 structure footprint prior                          | Stores overall footprint + metadata             |
| `building:part=*` polygons                            | LOD 1 structure sub-features / wall segments             | Preferred for per-part heights                  |
| `type=building` relation (`outline` + `part` members) | Explicit grouping of outline and parts                   | Use when present; infer containment otherwise   |
| `height=*`, `min_height=*`                            | `ZMax`, `ZMin` priors (after local frame transform)      | Highest-confidence height tags                  |
| `building:levels=*`, `building:min_level=*`           | Height fallback priors with lower confidence             | Convert with configured level-height heuristics |
| `roof:shape`, `roof:height`, `roof:levels`, `roof:*`  | Optional roof prior metadata / roof uncertainty envelope | Used to avoid overfitting roof returns as walls |

**Height precedence for priors (high → low confidence):**

1. Explicit metric tags (`height=*`, `min_height=*`, `roof:height=*`)
2. Mixed metric + levels (e.g. `height=*` plus `building:levels=*`)
3. Levels-only estimates (`building:levels=*`, `roof:levels=*`) using configured defaults
4. Footprint-only prior (geometry only, no reliable height)

Implementation notes:

- GPS is still optional. If unavailable, the system runs LiDAR-only with no OSM fetch.
- When GPS is available, convert OSM WGS84 geometry into the sensor-local frame
  (ENU / local tangent plane) before prior scoring and wall fitting.
- Treat OSM priors as **soft constraints** (`w_prior`) and allow local LiDAR to
  override them when observations disagree consistently.

#### Scan-Derived OSM Update Proposal Tooling (Contribute Back Upstream)

The V2 OSM write-back workflow from `ground-plane-extraction.md`
should be extended for S3DB-aware building updates. The key design requirement
is **human-reviewed proposals by default** (not autonomous uploads).

**Proposed tooling pipeline:**

1. `vr-map prior import-osm-s3db`
   Fetch/parse OSM S3DB data for an area (Overpass API or local `.pbf`
   extract). Cache raw OSM objects and translated structure priors with version
   metadata.
2. `vr-map structure reconcile`
   Compare observed LiDAR-derived `StructureFeature`s against imported S3DB
   priors. Produce candidate deltas with confidence scores and evidence metrics.
3. `vr-map structure export-osm-proposals`
   Emit a review bundle containing:
   `*.osc` (`osmChange`) patch or JOSM-loadable `.osm` draft objects,
   GeoJSON diff overlay (before/after footprints, heights, parts), and a QA
   report (Markdown/JSON) with residual error stats and conflict flags.
4. Manual review in JOSM/iD (recommended path)
   Validate geometry, tags, and local knowledge, then upload under a human
   mapper account with appropriate changeset comments.

**Candidate update types (S3DB-aware):**

- Add missing `height=*` or `min_height=*` where LiDAR confidence is high.
- Improve existing heights where residual error is systematic and above threshold.
- Propose `building:part=*` splits when a single outline clearly contains
  multiple height regimes or roof forms.
- Add or refine `roof:shape` / `roof:height` only when confidence is
  sufficient; otherwise emit a "needs survey/manual review" hint instead of a
  tag edit.
- Propose footprint vertex adjustments for persistent facade alignment errors.

**Safety and community constraints:**

- No automatic OSM uploads from the perception pipeline.
- Every proposed object change must be reviewable individually.
- Proposal export should preserve OSM IDs/version numbers and detect edit conflicts.
- Bulk or repeated scripted edits must follow OSM community guidance for
  automated/mechanical edits before execution.

### Supplemental Geometry-Prior Service

Community-maintained static GeoJSON priors (ground, kerbs, vegetation) not well represented in OSM. Local-first with optional static fetch from a public CDN; 0.01° grid-based folder structure; immutable contributor files with optional GPG signatures and CI-maintained trust manifest. Full architecture, file format specification, trust model, and hosting options: **[geometry-prior-service.md](./geometry-prior-service.md)**.

### Multi-Device Fusion

Multiple sensors observing the same area can each produce a local vector scene map. Merging follows weighted polygon averaging. Building footprints from different viewpoints refine corner positions.

---

## Open Questions

No open questions for the vector scene map core. Design choices (alpha-shape vs convex-hull boundary, QuadTree vs R-tree spatial index) are documented in §4 and §10. Open questions for the geometry-prior service are in [geometry-prior-service.md](./geometry-prior-service.md).

---

## References

### Internal Documents

- **Ground Plane Extraction** — [ground-plane-extraction.md](./ground-plane-extraction.md) (tile-based ground model, Tier 1/2 design)
- **Ground Plane Proposal Maths** — [ground-plane-vector-scene-maths.md](../../../data/maths/proposals/20260221-ground-plane-vector-scene-maths.md) (algorithm trade-offs)
- **LiDAR Layer Model** — [lidar-data-layer-model.md](./lidar-data-layer-model.md) (L1–L6 layer definitions)
- **Background Grid Standards** — [lidar-background-grid-standards.md](./lidar-background-grid-standards.md) (VTK/PCD export)
- **PCAP Export Tool** — [pcap-ground-plane-export-tool-plan.md](../../plans/pcap-ground-plane-export-tool-plan.md) (CLI flags, export formats)

### External Standards

- **OpenStreetMap Simple 3D Buildings** — [OSM Wiki: Simple 3D Buildings](https://wiki.openstreetmap.org/wiki/Simple_3D_Buildings) (building outlines, parts, and roof tagging model)
- **OpenStreetMap Automated Edits Guidance** — [OSM Wiki: Automated Edits code of conduct](https://wiki.openstreetmap.org/wiki/Automated_Edits_code_of_conduct) (community process for scripted/mechanical edits)
- **GeoJSON** — [RFC 7946](https://tools.ietf.org/html/rfc7946) (geographic feature collections)
- **CityJSON** — [CityJSON 1.1](https://www.cityjson.org/specs/1.1.3/) (3D city model standard)
- **GeoPackage** — [OGC GeoPackage](https://www.geopackage.org/) (SQLite-based geodata container)
- **Douglas-Peucker** — Douglas & Peucker (1973), "Algorithms for the reduction of the number of points required to represent a digitised line"
- **Alpha Shapes** — Edelsbrunner et al. (1983), "On the Shape of a Set of Points in the Plane"
