# 3D Vector Scene Map — Architecture Specification

Status: Proposed
Target Directory: docs/lidar/architecture/

**Status:** Proposed
**Layer:** L4 Perception (extends `GroundSurface` interface)
**Related:** [ground-plane-extraction.md](../../../lidar/architecture/ground-plane-extraction.md), [ground-plane-vector-scene-maths.md](../../../proposals/maths/ground-plane-vector-scene-maths.md), [lidar-data-layer-model.md](../../../lidar/architecture/lidar-data-layer-model.md)
**Date:** 2026-02

---

## 1. Overview & Motivation

### From Tiled Grid to Vector Polygons

The existing ground plane specification (`../../../lidar/architecture/ground-plane-extraction.md`) models the road surface as a **uniform tiled grid** — a mosaic of 1 m × 1 m tiles, each with an independent plane equation. This approach is efficient for flat-ish road surfaces but has inherent limitations when extended to describe the full observable scene:

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
│  │ (flat)    │ (5° slope)      │  with different normals  │
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

```go
// StructureFeature represents a vertical scene element (building, wall, fence).
type StructureFeature struct {
    // Footprint polygon (2D, viewed from above)
    FootprintVertices [][2]float64 // Ordered polygon vertices (X, Y)

    // Per-wall-segment data (one entry per footprint edge)
    WallSegments []WallSegment

    // Vertical extent
    ZMin float64 // Lowest observed return (metres, sensor frame)
    ZMax float64 // Highest observed return

    // Metadata
    PointCount uint32
    Confidence float32 // Aggregate planarity across wall segments
    LOD        uint8   // Level of detail
}

// WallSegment is a single planar face of a structure.
type WallSegment struct {
    // Wall-plane equation: n·p = d (normal is near-horizontal)
    Normal [3]float64
    Offset float64

    // Endpoints of this wall edge (footprint vertices i, i+1)
    StartIdx int
    EndIdx   int

    Planarity  float32
    PointCount uint32
}
```

**Why not store full 3D meshes?** We don't need photorealistic building models. A few wall planes with corner coordinates capture the coarse structure visible to LiDAR, sufficient for:

- **Occlusion reasoning** — "Is a potential track target behind a known building wall?"
- **Scene context** — "This cluster is adjacent to a known building facade" (not free-standing).
- **Map publishing** — Export building outlines as GeoJSON polygons for GIS integration.

### 2.3 Volume Features (Vegetation, Irregular Shapes)

Trees, hedges, and overhanging features don't conform to single planes. They produce diffuse, scattered returns. Model them as **approximate bounding volumes**:

```go
// VolumeFeature represents an irregular 3D scene element (tree, hedge, sign cluster).
type VolumeFeature struct {
    // Bounding representation (choose one)
    BoundingType BoundingKind   // OBB, ConvexHull, or Sphere
    Centre       [3]float64     // Approximate centroid
    Dimensions   [3]float64     // Half-extents for OBB; radius for sphere
    Orientation  [4]float64     // Quaternion rotation for OBB (identity for axis-aligned)
    HullVertices [][3]float64   // For ConvexHull type only

    // Density estimate
    PointCount     uint32
    PointDensity   float32 // Points per m³ (distinguishes solid vs sparse returns)
    ApproxVolume   float32 // Bounding volume in m³

    // Metadata
    Class      VolumeClass // tree, hedge, sign_cluster, awning, unknown
    LOD        uint8
    Confidence float32
}

// BoundingKind specifies the bounding representation for a VolumeFeature.
type BoundingKind uint8

const (
    BoundingBox    BoundingKind = iota // Oriented bounding box (OBB)
    BoundingHull                       // Convex hull (compact point set)
    BoundingSphere                     // Bounding sphere (simplest)
)
```

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

```go
// SceneFeature is the common envelope for all features in the vector scene map.
type SceneFeature struct {
    ID       FeatureID    // Globally unique within the map
    Class    FeatureClass // Ground, Structure, Volume
    LOD      uint8        // 0–3
    ParentID FeatureID    // ID of parent feature (0 = root / no parent)

    // Geometry (exactly one populated, determined by Class)
    Ground    *GroundFeature    // Non-nil for Class == Ground
    Structure *StructureFeature // Non-nil for Class == Structure
    Volume    *VolumeFeature    // Non-nil for Class == Volume

    // Common metadata
    PointCount       uint32
    Confidence       float32
    LastUpdatedNanos int64
    Settled          bool
}

// FeatureID uniquely identifies a feature within the scene map.
type FeatureID uint64

// FeatureClass distinguishes the three geometric classes.
type FeatureClass uint8

const (
    FeatureGround    FeatureClass = iota // Horizontal/sloped surface polygon
    FeatureStructure                     // Vertical structure polygon(s)
    FeatureVolume                        // 3D bounding volume
)
```

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

```go
// VectorSceneMap holds the full multi-resolution feature set for a scene.
type VectorSceneMap struct {
    Features map[FeatureID]*SceneFeature
    mu       sync.RWMutex

    // Spatial index for fast 2D queries (ground + structure footprints)
    SpatialIndex *QuadTree // or R-tree; keyed by feature bounding box

    // LOD index for fast level-filtered queries
    LODIndex [4][]FeatureID // LODIndex[lod] = list of feature IDs at that LOD

    // Statistics
    GroundCount    uint32
    StructureCount uint32
    VolumeCount    uint32
    NextFeatureID  FeatureID
}
```

### 5.2 Storage Budget

**Comparison: tiled grid vs vector polygons for a 100 m × 100 m scene**

Compressed sizes assume gzip compression at ~4:1 ratio for tile grids (high redundancy in similar plane parameters) and ~3:1 for vector polygons (less redundancy due to variable geometry). These ratios are consistent with observed gzip performance on similar structured data (see [`ground-plane-maths.md`](../../../proposals/maths/ground-plane-vector-scene-maths.md) §10).

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

```sql
CREATE TABLE IF NOT EXISTS vector_scene_features (
    feature_id    INTEGER PRIMARY KEY,
    parent_id     INTEGER,               -- 0 for root features
    class         INTEGER NOT NULL,       -- 0=ground, 1=structure, 2=volume
    lod           INTEGER NOT NULL,       -- 0–3
    settled       INTEGER NOT NULL DEFAULT 0,
    confidence    REAL NOT NULL DEFAULT 0.0,
    point_count   INTEGER NOT NULL DEFAULT 0,
    last_updated  INTEGER NOT NULL,       -- timestamp nanos
    geometry_blob BLOB NOT NULL,          -- gzip-compressed feature geometry
    FOREIGN KEY (parent_id) REFERENCES vector_scene_features(feature_id)
);

CREATE INDEX IF NOT EXISTS idx_scene_feature_class ON vector_scene_features(class);
CREATE INDEX IF NOT EXISTS idx_scene_feature_lod ON vector_scene_features(lod);
CREATE INDEX IF NOT EXISTS idx_scene_feature_parent ON vector_scene_features(parent_id);

-- Snapshot table for versioned map states (analogous to ground_plane_snapshots)
CREATE TABLE IF NOT EXISTS vector_scene_snapshots (
    snapshot_id       INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp_nanos   INTEGER NOT NULL,
    sensor_id         TEXT,
    origin_lat        REAL,    -- NULL if no GPS
    origin_lon        REAL,
    feature_count     INTEGER,
    features_blob     BLOB,    -- gzip-compressed gob-encoded []SceneFeature
    features_hash     TEXT,    -- SHA256 deduplication
    lod_distribution  TEXT     -- JSON: {"lod0": 15, "lod1": 35, "lod2": 120, "lod3": 45}
);
```

### 5.4 Export Formats

| Format           | Use Case                     | Ground                                 | Structures                              | Volumes                               |
| ---------------- | ---------------------------- | -------------------------------------- | --------------------------------------- | ------------------------------------- |
| **GeoJSON**      | GIS tools (QGIS, Mapbox)     | Polygon features with plane properties | Polygon features with height properties | Point features with bounding metadata |
| **GeoPackage**   | Offline GIS with multi-layer | Separate layer per class               | Separate layer per class                | Separate layer per class              |
| **VTK PolyData** | ParaView / LidarView         | PolyData with cell normals             | PolyData with wall planes               | Glyph data (OBBs as boxes)            |
| **CityJSON**     | 3D city modelling            | Terrain surface                        | LOD 1–2 building shells                 | Vegetation objects                    |

GeoJSON remains the default export, consistent with the ground plane export specification. Structure: one `FeatureCollection` per class, filtered by LOD:

```json
{
  "type": "FeatureCollection",
  "metadata": {
    "scene_map_version": "1.0",
    "lod_filter": 1,
    "coordinate_system": "Sensor-XY",
    "feature_count": 42
  },
  "features": [
    {
      "type": "Feature",
      "geometry": { "type": "Polygon", "coordinates": [[[0.5, 2.1], [12.3, 2.0], ...]] },
      "properties": {
        "id": 1001,
        "class": "ground",
        "subclass": "road",
        "lod": 1,
        "plane_normal": [0.01, 0.005, 0.9999],
        "plane_offset": -2.85,
        "planarity": 0.98,
        "point_count": 2450,
        "area_m2": 145.2,
        "parent_id": 100
      }
    },
    {
      "type": "Feature",
      "geometry": { "type": "Polygon", "coordinates": [[[15.0, 3.0], [15.0, 12.5], ...]] },
      "properties": {
        "id": 2001,
        "class": "structure",
        "subclass": "building",
        "lod": 1,
        "z_min": -2.8,
        "z_max": 4.2,
        "wall_count": 3,
        "point_count": 890
      }
    }
  ]
}
```

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

```go
// SceneSurface extends GroundSurface with structural and volumetric queries.
type SceneSurface interface {
    GroundSurface // Embeds height-above-ground queries

    // FeaturesAt returns all features at (x, y) up to the specified LOD.
    // Returns features from coarsest to finest.
    FeaturesAt(x, y float64, maxLOD uint8) []SceneFeature

    // GroundPolygonAt returns the ground polygon containing (x, y) at the finest
    // available LOD up to maxLOD. Falls back to coarser LOD if finer unavailable.
    GroundPolygonAt(x, y float64, maxLOD uint8) (*GroundFeature, bool)

    // NearestStructure returns the closest structure feature to (x, y) within radius.
    NearestStructure(x, y, radius float64) (*StructureFeature, float64, bool)

    // VolumesInRadius returns all volume features within radius of (x, y).
    VolumesInRadius(x, y, radius float64) []VolumeFeature

    // FeaturesInBBox returns all features whose bounding box overlaps [xMin,yMin]–[xMax,yMax].
    FeaturesInBBox(xMin, yMin, xMax, yMax float64, maxLOD uint8) []SceneFeature
}
```

### 7.2 LOD Fallback Semantics

When querying at a specific LOD:

1. **Exact match** — If a feature exists at the requested LOD covering (x, y), return it.
2. **Finer available** — If finer LOD exists (child features), ignore them (user asked for coarser).
3. **Coarser fallback** — If no feature exists at the requested LOD, traverse up the parent chain until a covering feature is found.
4. **No coverage** — If no feature at any LOD covers (x, y), return `ok = false`.

This ensures that **every query returns something if any coverage exists**, regardless of which LOD the caller requests.

---

## 8. Seamless Navigation Between LOD Levels

### 8.1 The Global Map Problem

At the global level (Tier 2, lat/long-aligned), the map should be navigable as a coherent simplified representation:

- **Street-level polygons** (LOD 0–1) form a continuous surface with major boundaries visible: road edges, building outlines, vegetation zones.
- **Detail zones** (LOD 2–3) appear as nested refinements visible only when queried at higher LOD.

### 8.2 Transition Strategy

**Spatial containment invariant:** Every LOD N+1 feature is fully contained within its LOD N parent's boundary. This guarantees seamless transition:

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

1. Query global features within the sensor's coverage area.
2. Provide LOD 0–1 features as **priors** — the local scene map starts with known road outlines, building positions, and vegetation zones.
3. As the local Tier 1 ground tiles settle, refine or validate the prior polygons.
4. Create LOD 2–3 refinements where the local observations reveal detail not present in the global map.

When merging back (local → global):

1. LOD 0–1 polygons merge with weighted averaging (same as the ground tile merge algorithm).
2. LOD 2–3 polygons are added if they meet confidence thresholds.
3. LOD 2–3 polygons with stale timestamps are pruned (detail zones are assumed to change more frequently than coarse structure).

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

The `VectorSceneMap` can implement `GroundSurface` by delegating to the underlying `GroundPlaneGrid`:

```go
func (m *VectorSceneMap) QueryHeightAboveGround(x, y, z float64) (float64, float32, bool) {
    // Delegate to the tile-based ground plane for real-time queries
    return m.groundGrid.QueryHeightAboveGround(x, y, z)
}
```

For offline/export use cases, the vector polygons can directly answer height queries using their plane equations.

---

## 10. Construction Thresholds and Configuration

### Default Parameters

```go
// VectorSceneParams configures the scene map construction.
type VectorSceneParams struct {
    // Ground polygon merging
    MergeAngleThresholdDeg float64 // Max normal angle difference for merging (default: 2.0°)
    MergeZThresholdM       float64 // Max Z-offset difference at shared edge (default: 0.03 m)

    // LOD assignment
    LOD0MinAreaM2 float64 // Polygons > this area are LOD 0 (default: 50.0 m²)
    LOD1MinAreaM2 float64 // Polygons > this area are LOD 1 (default: 5.0 m²)
    LOD2MinAreaM2 float64 // Polygons > this area are LOD 2 (default: 0.25 m²)

    // Refinement triggers
    RefinementCurvatureDeg   float64 // Curvature threshold to trigger LOD 2+ (default: 5.0°)
    RefinementZStepM         float64 // Z-step threshold to trigger LOD 2+ (default: 0.10 m)
    RefinementDensityPtsPerM2 float64 // Point density for LOD 3 eligibility (default: 50.0)

    // Pruning (per-LOD minimum polygon area; see §4.3 pruning table)
    MinPolygonAreaLOD2M2   float64 // Discard LOD 2 polygons smaller than this (default: 0.25 m²)
    MinPolygonAreaLOD3M2   float64 // Discard LOD 3 polygons smaller than this (default: 0.10 m²)
    MaxVerticesPerPolygon  int     // Simplify if exceeded (default: 32)
    PlanarityImprovementMin float64 // Min planarity gain from split (default: 0.05)
    MinConfidence          float32 // Exclude features below this (default: 0.70)
    StaleTimeoutLOD01Nanos int64   // Stale timeout for LOD 0–1 (default: 300e9 = 5 min)
    StaleTimeoutLOD23Nanos int64   // Stale timeout for LOD 2–3 (default: 120e9 = 2 min)

    // Vertex simplification (Douglas-Peucker tolerance per LOD)
    SimplifyToleranceLOD [4]float64 // {2.0, 0.5, 0.1, 0.02} metres

    // Structure extraction
    StructureMinHeight float64    // Minimum vertical extent for structures (default: 1.0 m)
    StructureMaxNormalZ float64   // Max Z-component of normal for "vertical" (default: 0.3)

    // Volume extraction
    VolumeMinPersistenceSec float64 // Min seconds a cluster must persist (default: 30.0)
    VolumeMinPointCount     int     // Min points for volume feature (default: 50)
}
```

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

The V2 OSM write-back workflow from `../../../lidar/architecture/ground-plane-extraction.md` applies directly: export vector scene polygons as proposed OSM changesets.

### Multi-Device Fusion

Multiple sensors observing the same area can each produce a local vector scene map. Merging follows the same weighted-average approach as ground tile merging, operating on the polygon feature level. Building footprints from different viewpoints can be merged to refine corner positions.

### Mobile Deployment

A vehicle-mounted sensor produces a stream of local scene maps along its route. The global map accumulates structure and ground features into a corridor-style vector map. LOD 0–1 provides the route-level overview; LOD 2–3 captures surveyed detail at specific intersections.

---

## References

### Internal Documents

- **Ground Plane Extraction** — `docs/lidar/architecture/../../../lidar/architecture/ground-plane-extraction.md` (tile-based ground model, Tier 1/2 design)
- **Ground Plane Proposal Maths** — `docs/proposals/maths/ground-plane-vector-scene-maths.md` (algorithm trade-offs)
- **LiDAR Layer Model** — `docs/lidar/architecture/../../../lidar/architecture/lidar-data-layer-model.md` (L1–L6 layer definitions)
- **Background Grid Standards** — `docs/lidar/architecture/lidar-background-grid-standards.md` (VTK/PCD export)
- **PCAP Export Tool** — `docs/plans/pcap-ground-plane-export-tool.md` (CLI flags, export formats)

### External Standards

- **GeoJSON** — [RFC 7946](https://tools.ietf.org/html/rfc7946) (geographic feature collections)
- **CityJSON** — [CityJSON 1.1](https://www.cityjson.org/specs/1.1.3/) (3D city model standard)
- **GeoPackage** — [OGC GeoPackage](https://www.geopackage.org/) (SQLite-based geodata container)
- **Douglas-Peucker** — Douglas & Peucker (1973), "Algorithms for the reduction of the number of points required to represent a digitised line"
- **Alpha Shapes** — Edelsbrunner et al. (1983), "On the Shape of a Set of Points in the Plane"
