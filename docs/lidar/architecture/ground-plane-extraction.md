# LiDAR Ground Plane Extraction — Architecture Specification

## 1. Overview & Motivation

### Purpose

This document specifies a **ground plane extraction subsystem** within L4 Perception that models the road surface as a piecewise-planar grid of tiles. The ground plane provides a stable geometric reference for measuring object heights above ground, enabling robust object classification and reducing false positives from ground clutter.

### Architecture Principles

**Sensor-iterative (LiDAR-only first):** All local PCAP observations are sensor-iterative. The ground plane subsystem **must function with the LiDAR sensor alone, with no GPS**. GPS is only additive — it enriches exports with geographic coordinates but is never required for core ground plane extraction or height-above-ground queries. Every algorithm described in this document operates in sensor-local coordinates by default.

**Two-tier ground model:** The system distinguishes between:

1. **Local scene ground** — per-observation-session ground tiles settled from live LiDAR returns. These are the working data for real-time perception.
2. **Global published ground** — a persistent, lat/long-aligned grid (0.001 millidegree tiles, approximately 111 m at the equator down to ~43.5 m at 67°N/S) that accumulates across observation sessions. Global tiles can be loaded at startup, diffed against the current local scene, and updated from settled local tiles. This global grid is a shared, publishable artefact.

### Motivation

The current L4 `HeightBandFilter` (`internal/lidar/l4perception/ground.go`) removes ground returns using fixed Z-band thresholds (floor at −2.8 m, ceiling at +1.5 m for a ~3 m sensor mount). This approach has limitations:

1. **No geometric surface model** — The filter discards ground points but doesn't model the surface itself. Height-above-ground measurements require a reference plane for accurate object classification (e.g., distinguishing pedestrians from vehicles).
2. **Fixed thresholds vulnerable to terrain variation** — San Francisco's hilly streets exhibit significant slope and curvature; a fixed floor height misclassifies points on steep grades.
3. **No confidence metric** — The current system cannot report "this area's ground is flat within ±5 cm" or "high curvature here—height measurements unreliable".
4. **No spatial awareness** — Height bands don't capture discontinuities like kerbs, driveways, speed humps, or transitions between road plates.

The ground plane subsystem addresses these needs by:

- **Extracting a geometric surface model** validated by point returns but defined as a mathematical surface (plane equations per tile)
- **Settling rapidly** to provide stable height references within seconds of observation
- **Handling discontinuous surfaces** — modelling flatness and curvature locally without requiring a global solver
- **Providing confidence metrics** — queryable per-tile planarity and coverage statistics
- **Optionally aligning with geographic coordinates** — lat/long-aligned Cartesian grid for integration with mapping tools and multi-device deployments (GPS additive, never required)

### Relationship to Existing Systems

The ground plane subsystem is part of **L4 Perception**, publishing a non-point-based interface (plane equations per tile) that is unioned into scene understanding alongside point-based cluster outputs:

```
L1 Packets → L2 Frames → L3 Background Grid → L4 Perception → L5 Tracks → L6 Objects
                                                  ├── Clustering (point-based)
                                                  ├── Ground Plane (surface-based)
                                                  └── Height-above-ground queries
```

- **L3 Background Grid** identifies static scene elements via EMA-updated per-cell range statistics and neighbour confirmation. It distinguishes background (stationary) from foreground (moving) but doesn't model surface geometry.
- **L4 Ground Plane** (within `internal/lidar/l4perception/`) consumes points classified as static ground (from L3 or raw frames) and fits local plane equations to build a geometric surface model. It publishes a `GroundSurface` interface — a non-point-based representation of the scene geometry.
- **L4 Clustering** uses the ground plane to compute height-above-ground for each cluster, improving object classification and reducing ground-clutter false positives.

This keeps all perception-level scene understanding within L4, maintaining the L3 grid's role as a fast foreground/background separator while adding geometric reasoning for height-based classification.

---

## 2. Ground Surface Model

### Piecewise-Planar Tile Representation

The ground surface is modelled as a **mosaic of locally-flat tiles** with curvature and discontinuities at tile boundaries. Each tile stores:

1. **Plane equation** — Unit normal vector **n** = (nx, ny, nz) and scalar offset **d** such that a point (x, y, z) lies on the plane if **n** · (x, y, z) = d. Equivalently: nx·x + ny·y + nz·z = d.
2. **Confidence metrics** — Point count, planarity score (ratio of eigenvalues from PCA), last-update timestamp.
3. **Settlement status** — Boolean indicating whether the tile has sufficient observations to be considered stable.
4. **Bounding box** — Tile centre and extents in the grid's coordinate frame.

### Local Plane Fitting — No Global Solver

Each tile fits its plane **independently** via incremental algorithms:

- **Incremental PCA** — Maintain per-tile running sum of points, sum of squared deviations, and 3×3 covariance matrix. Update eigenvalue decomposition periodically (e.g., every 10 observations or when settlement check is triggered).
- **Least-squares plane fit** — Alternatively, accumulate ΣX, ΣY, ΣZ, ΣXX, ΣXY, etc., and solve the normal equations for best-fit plane coefficients.

Both methods support **online updates** without storing all historical points, enabling efficient streaming operation on resource-constrained hardware (Raspberry Pi 4).

**Rationale for local fitting:**

- San Francisco's streets have **known high-curvature zones** (steep hills, curved intersections). A global plane solver would fail or require expensive iterative optimization.
- Local tiles can have different Z-heights and orientations (e.g., driveway ramps, kerbs, speed humps).
- Independent tile fitting is **parallelizable** and **incrementally updatable**, matching the streaming data model.

### Discontinuity Handling

Tile boundaries may exhibit:

- **Height discontinuities** (kerbs, driveways) — Adjacent tiles have different Z-offsets.
- **Slope transitions** (road curvature, hills) — Adjacent tiles have different normal vectors.
- **Coverage gaps** — Occlusion or limited sensor visibility leaves some tiles unobserved.

The system **does not enforce continuity constraints** between tiles. Each tile is an independent surface patch. Downstream consumers (L4 perception) can detect discontinuities by comparing adjacent tile normals and offsets.

---

## 3. Grid Structure & Alignment

### Two-Tier Grid Model

The ground plane operates at two distinct spatial scales:

#### Tier 1: Local Scene Grid (Sensor-Iterative)

The **local scene grid** is the primary working data, settled from live LiDAR returns during each observation session. It operates in sensor-local Cartesian coordinates and requires **no GPS**.

- **Grid axes** — X = right, Y = forward, Z = up (sensor-local frame, matching `SphericalToCartesian` output).
- **Origin** — Sensor position (0, 0, 0).
- **Tile indexing** — Integer indices (ix, iy) with tile centre at (ix · tileSize, iy · tileSize) relative to sensor origin.
- **Tile size** — Configurable, default 1.0 m × 1.0 m (see tile size table below).
- **Coverage** — Determined by sensor range and visibility; typically 50–100 m radius.
- **Lifecycle** — Created per observation session; settles within seconds; discarded when session ends (or promoted to global grid if GPS available).

This is the only tier required for core perception. The system **must** function at this tier with LiDAR data alone.

#### Tier 2: Global Published Grid (GPS-Enhanced, Optional)

When GPS coordinates are available, settled local tiles can be projected into a **global lat/long-aligned grid** that persists across observation sessions:

- **Grid axes** — X = East, Y = North (WGS84 local tangent plane convention).
- **Tile sizing** — 0.001 millidegree (≈0.001° × 0.001°). At the equator this is approximately 111 m × 111 m. At 67°N/S latitude this is approximately 43.5 m × 111 m (longitude shrinks by cos(latitude)). Each global tile spans multiple local scene tiles.
- **Tile indexing** — Integer millidegree indices: `ix = floor(longitude / 0.001)`, `iy = floor(latitude / 0.001)`.
- **Persistence** — Stored in SQLite and exportable as a shared artefact. Can be loaded at startup to seed local scene grids (providing prior ground estimates before LiDAR settling completes).
- **Diff/merge** — When a new observation session settles, its local tiles are diffed against the existing global grid. Consistent tiles strengthen confidence; divergent tiles trigger re-evaluation (construction, seasonal change, etc.).

Global tiles contain aggregate statistics from multiple observation sessions:

| Field             | Description                                         |
| ----------------- | --------------------------------------------------- |
| Mean plane normal | Weighted average of contributing local tile normals |
| Mean Z-offset     | Weighted average ground height                      |
| Session count     | Number of observation sessions contributing         |
| Last updated      | Timestamp of most recent contribution               |
| Confidence        | Combined planarity from all contributing sessions   |

### Lat/Long-Aligned Cartesian Grid (Tier 2)

When GPS is available, the local sensor-frame grid can be transformed to a geographic Cartesian grid:

**Benefits of Cartesian geographic alignment:**

1. **Multi-device fusion** — Multiple sensors at different locations can contribute to a shared ground plane map by mapping their observations into the same geographic grid.
2. **Export to GIS tools** — Tiles can be exported as GeoJSON polygons or ASC raster grids with lat/long coordinates for inspection in QGIS, Google Earth, etc.
3. **Terrain databases** — Future integration with external elevation models (e.g., USGS DEMs) for prior seeding or validation.

**Tradeoff:** Cartesian grids require a coordinate transform from sensor-local spherical (distance, azimuth, elevation) → sensor-local Cartesian (X=right, Y=forward, Z=up) → world-frame Cartesian (X=East, Y=North, Z=up). This adds computational cost but is necessary for geographic alignment.

### Tile Size Selection

Tile size trades off between:

- **Spatial resolution** — Smaller tiles capture fine-grained features (kerb edges, potholes) but require more storage and suffer from sparser observations per tile.
- **Robustness** — Larger tiles average out noise and converge faster but miss local discontinuities.
- **Computational cost** — Number of tiles grows as (coverage area / tile size²).

**Recommended tile sizes:**

| Tile size | Use case                                                                 | Typical tile count |
| --------- | ------------------------------------------------------------------------ | ------------------ |
| 0.5 m     | High-resolution mapping; detect kerbs and small features                 | ~10,000 for 50 m²  |
| 1.0 m     | **Default** — Balance between detail and performance for urban streets   | ~2,500 for 50 m²   |
| 2.0 m     | Coarse mapping; rapid convergence for low-point-density or sparse scenes | ~625 for 50 m²     |

For velocity.report's traffic monitoring use case, **1.0 m × 1.0 m tiles** are recommended: sufficient resolution to model road curvature and kerbs without excessive tile proliferation.

### Coordinate Transforms

**Sensor spherical → Sensor Cartesian:**

Using `SphericalToCartesian` from `internal/lidar/l4perception/cluster.go` (note: current implementation has X=right, Y=forward, Z=up, not the comment's stated X=forward, Y=right):

```go
x, y, z := SphericalToCartesian(distance, azimuthDeg, elevationDeg)
// Returns: x = distance * cos(elev) * sin(az)
//          y = distance * cos(elev) * cos(az)
//          z = distance * sin(elev)
// Coordinate frame: X=right, Y=forward, Z=up (sensor-local)
```

**Sensor Cartesian → World Cartesian:**

Apply sensor pose transform (rotation + translation) using `ApplyPose`:

```go
wx, wy, wz := ApplyPose(x, y, z, sensorPose)
// sensorPose is a 4×4 homogeneous transform matrix (row-major [16]float64)
// Output: (wx, wy, wz) in world frame (X=East, Y=North, Z=up relative to origin)
```

**World Cartesian → Tile index:**

```go
ix := int(math.Floor(wx / tileSize))
iy := int(math.Floor(wy / tileSize))
```

The ground plane subsystem **can optionally** integrate with GPS/PTP parsing (see `internal/lidar/l1packets/parse/extract.go`) to obtain sensor position and heading for the pose transform. Without GPS, only Tier 1 local scene tiles are available (which is sufficient for all core perception tasks).

### Relationship to L3 Polar Background Grid

The L3 `BackgroundGrid` and L4 ground plane serve complementary roles:

| Aspect               | L3 Background Grid (polar)                       | L4 Ground Plane (Cartesian)                             |
| -------------------- | ------------------------------------------------ | ------------------------------------------------------- |
| **Geometry**         | Rings × azimuth bins                             | Cartesian tiles (sensor-local or lat/long aligned)      |
| **Purpose**          | Foreground/background separation                 | Surface modelling for height-above-ground               |
| **Representation**   | Per-cell range statistics (mean, spread, freeze) | Per-tile plane equation (normal, offset)                |
| **Update rate**      | Per-frame EMA updates                            | Incremental PCA/least-squares                           |
| **Coordinate frame** | Sensor-centric polar                             | Sensor-local Cartesian (Tier 1) or world-frame (Tier 2) |
| **Export format**    | VTK ImageData, ASC (debugging)                   | GeoJSON, ASC raster, VTK StructuredGrid                 |
| **GPS required**     | No                                               | No (Tier 1); Yes (Tier 2 global grid)                   |

The ground plane can consume **ground-classified points** from the L3 background grid (cells marked as static and within ground Z-band) or operate independently on raw L2 frame points filtered by elevation. Initial implementation should support both modes for flexibility.

---

## 4. Settlement & Stabilisation

### Fast Convergence via Incremental Algorithms

Each tile must converge rapidly to a stable plane estimate. Target: **settled within 5–10 seconds** of first observation (approximately 50–100 LiDAR frames at 10 Hz).

**Incremental covariance accumulation:**

For each new point (x, y, z) added to tile (ix, iy):

1. Update running statistics:

   ```
   n += 1
   sum_x += x;  sum_y += y;  sum_z += z
   sum_xx += x*x;  sum_yy += y*y;  sum_zz += z*z
   sum_xy += x*y;  sum_xz += x*z;  sum_yz += y*z
   ```

2. When `n ≥ minPointsForPlane` (e.g., 10 points), compute mean and covariance matrix:

   ```
   mean = (sum_x/n, sum_y/n, sum_z/n)
   cov[i,j] = (sum_ij / n) - (sum_i/n)*(sum_j/n)
   ```

3. Perform eigenvalue decomposition of covariance matrix. Smallest eigenvector is plane normal **n**; offset **d** = **n** · mean.

**Efficiency:** No need to re-fit from scratch; eigenvalue decomposition is ~O(1) for 3×3 matrices (closed-form solution via cubic equation).

### Minimum Point Threshold

A tile is marked **settled** when:

1. `pointCount ≥ minPointsForSettlement` (recommended: 20 points)
2. `planarityScore ≥ minPlanarityThreshold` (e.g., 0.95 — see Confidence section)
3. `timeObserved ≥ minSettlementDuration` (e.g., 5 seconds)

Tiles that don't meet these criteria are marked **unsettled** and excluded from height-above-ground queries.

### Freeze/Lock Mechanism

Once a tile is **settled**, it enters a **locked baseline** state analogous to the L3 background grid's freeze mechanism:

- **New observations within tolerance** (±2 cm from plane) increment a confirmation counter and update plane parameters with a low alpha (e.g., 0.01).
- **Observations outside tolerance** (potential ground change or outlier) decrement confirmation counter. If counter falls below threshold, tile reverts to **unsettled** and re-accumulates.
- **Freeze window** — After detecting a potential outlier, the tile freezes updates for a short duration (e.g., 3 seconds) to avoid thrashing from transient occlusions (vehicle shadows, debris).

### Warm-up vs Steady-State Behaviour

**Warm-up phase (first 10 seconds):**

- Tiles accumulate points aggressively with high update rate.
- Settlement checks run every 1 second.
- No height-above-ground queries permitted until sufficient tiles are settled.

**Steady-state phase (after warm-up):**

- Settled tiles update slowly (alpha = 0.01) for stability.
- Unsettled tiles continue accumulating at higher rate (alpha = 0.1).
- Settlement checks run every 10 seconds to reduce CPU load.

**Per-tile state machine:**

```
EMPTY → ACCUMULATING → SETTLED → LOCKED_BASELINE
         ↑                ↓
         └─── REACQUIRE ←─┘
            (outlier detected)
```

### Settlement Propagation

Tiles settle **independently**; there is no spatial coupling during convergence. However, a tile's **confidence score** can inform its neighbours:

- If a tile has high planarity and many observations, adjacent tiles can seed their plane normals from the settled neighbour (orientation hint) to accelerate convergence.
- This is an **optional optimization** and not required for correctness.

---

## 5. Confidence & Curvature Classification

### Per-Tile Confidence Metric

Each tile computes a **planarity score** from the eigenvalues of its covariance matrix:

Let λ₁ ≥ λ₂ ≥ λ₃ be the eigenvalues (sorted descending). For a perfect plane, λ₃ = 0 (no variance in the normal direction).

**Planarity score:**

```
planarity = 1 - (λ₃ / λ₂)
```

- **planarity ≈ 1.0** — Tile is highly planar (points lie on a tight plane)
- **planarity < 0.9** — Tile has significant scatter; potential curved surface or mixed ground/object returns
- **planarity < 0.5** — Tile is non-planar; unreliable for height measurements

**Classification thresholds:**

| Planarity | Classification       | Use for height queries? |
| --------- | -------------------- | ----------------------- |
| ≥ 0.95    | High-confidence flat | Yes                     |
| 0.85–0.95 | Moderate-confidence  | Yes, with caution       |
| 0.70–0.85 | Low-confidence       | No (warn user)          |
| < 0.70    | Non-planar/invalid   | No (exclude from map)   |

### Flatness vs Curvature Classification

**Flatness** — Measure of local planarity within a single tile (via planarity score).

**Curvature** — Measure of plane orientation change across adjacent tiles:

```
curvature = arccos(n₁ · n₂)
```

where n₁ and n₂ are normal vectors of adjacent tiles.

- **Low curvature** (<5°) — Smooth, continuous surface (typical for straight road segments)
- **Medium curvature** (5–15°) — Gentle slope change (curved road, ramp)
- **High curvature** (>15°) — Sharp transition (kerb, speed hump, hill crest)

**Known high-curvature zones (San Francisco):**

- Steep hills (e.g., Filbert St, 31.5% grade → ~17° slope)
- Curved intersections (e.g., Lombard St switchbacks)
- Freeway on/off ramps

The system can pre-load a **curvature mask** from prior mapping data (if available) or learn curvature zones dynamically from observed plane normals.

### Confidence Propagation to Neighbours

A tile's confidence can inform adjacent tiles:

1. **Spatial smoothness prior** — If all 8 neighbours are high-confidence flat, a tile's planarity threshold can be relaxed (e.g., 0.90 instead of 0.95) since it's in a confirmed flat region.
2. **Boundary discontinuity detection** — If a tile's plane differs significantly from neighbours (large curvature or Z-offset), mark the boundary as a **discontinuity edge** (potential kerb or ramp).

This propagation is **optional** and can be implemented as a post-processing step after initial tile settlement.

### User-Queryable Confidence

The system must support queries like:

```go
// Is the region around (lat, lon) flat within ±5 cm?
func (g *GroundPlaneGrid) QueryFlatness(lat, lon float64, radiusMeters, toleranceCm float64) (bool, float64) {
    tiles := g.GetTilesInRadius(lat, lon, radiusMeters)
    avgPlanarity := 0.0
    for _, tile := range tiles {
        if !tile.Settled {
            return false, 0.0 // Insufficient data
        }
        avgPlanarity += tile.Planarity
        // Check Z-variance across tiles
        if tile.MaxZDeviation() > toleranceCm {
            return false, avgPlanarity / float64(len(tiles))
        }
    }
    avgPlanarity /= float64(len(tiles))
    return avgPlanarity >= 0.95, avgPlanarity
}
```

This enables downstream consumers to assess height measurement reliability and flag uncertain regions.

---

## 6. Pixel Validation

### Ground Plane Validated by Point Returns

Each tile's plane equation is **derived from and validated by the raw LiDAR point returns** that fall within its spatial bounds. The plane is not imposed externally; it is the **least-squares or PCA fit to observed data**.

**Validation process:**

1. **Point assignment** — Each incoming point (x, y, z) is assigned to tile (ix, iy) based on its world-frame X and Y coordinates.
2. **Inlier filtering** — Compute point's distance to current plane:
   ```
   distance = |n·(x,y,z) - d|
   ```
   If distance > outlierThreshold (e.g., 10 cm), mark as outlier and exclude from fit.
3. **Incremental fit update** — Inlier points update the tile's running covariance statistics and re-trigger plane fitting.

### Outlier Rejection

Two outlier rejection strategies:

**1. RANSAC-like iterative fitting** (offline/batch mode, e.g., PCAP replay):

- Randomly sample 3 points, fit plane, count inliers within ε.
- Repeat N times, keep plane with most inliers.
- Refine plane with all inliers.

**2. Median-based filtering** (online/streaming mode):

- Maintain a circular buffer of recent point-to-plane distances (per tile).
- Compute median distance. Reject points with distance > 3 × median.
- This is computationally lighter than RANSAC and suitable for real-time operation.

**Recommendation:** Use median-based filtering for streaming; RANSAC for PCAP post-processing.

### Minimum Coverage Requirements

Not all tiles have equal sensor visibility:

- **Near-field tiles** (within 10 m) receive many points per frame due to dense azimuth/elevation sampling.
- **Far-field tiles** (>30 m) receive sparse points; may take minutes to accumulate sufficient observations.
- **Occluded tiles** (behind buildings, vehicles) may never be observed.

The system must track **coverage density** per tile:

```go
type GroundTile struct {
    // ...
    PointDensity float32 // points per m² per second
    LastSeenTimestamp int64
}
```

Tiles with `PointDensity < minDensityThreshold` (e.g., 1 point/m²/s) are marked **low-coverage** and excluded from confident height queries.

### Re-validation Cadence

**Streaming mode:**

- Tiles re-validate incrementally with every frame (add new points, update covariance).
- Outlier checks run per-frame for new points.

**PCAP replay mode:**

- Multi-pass: first pass accumulates all points per tile, second pass fits planes, third pass refines with outlier rejection.
- Faster convergence than streaming (can use global statistics).

**Confidence decay:**

If a tile hasn't received new observations for `staleTimeout` (e.g., 60 seconds), its confidence score decays:

```
confidence *= exp(-deltaTime / decayTimeConstant)
```

This prevents stale tiles from being trusted indefinitely in dynamic environments (parked cars move, construction zones change).

---

## 7. Integration with Existing Pipeline

### Layer Position: L4 Perception (Ground Surface Interface)

The ground plane is part of **L4 Perception**, publishing a **non-point-based interface** (`GroundSurface`) that is unioned with point-based clustering for scene understanding:

```
L2 Frames (SphericalToCartesian)
    ↓
L3 Background Grid (identifies static points)
    ↓
L4 Perception
    ├── Ground Plane Extractor → GroundSurface interface (plane equations per tile)
    ├── Clustering (DBSCAN → WorldCluster)
    └── Height-above-ground queries (using GroundSurface)
         ↓
L5 Tracking
```

The `GroundSurface` interface is deliberately **non-point-based**: it exposes plane equations, height queries, and confidence metrics rather than point clouds. This allows L4 clustering to query height-above-ground without coupling to the internal tile representation.

### Two Operating Modes

**Mode 1: Post-L3 (leverages background grid)**

- L3 background grid classifies points as background/foreground.
- Ground plane extractor consumes **static background points within ground Z-band** (e.g., −3.0 m < Z < −2.5 m).
- Advantage: Pre-filtered points reduce noise; faster convergence.
- Disadvantage: Depends on L3 settlement (background grid must stabilise first).

**Mode 2: Direct from L2 (independent)**

- Ground plane extractor receives all L2 frame points, applies its own ground filter (simple Z-band threshold: −3.0 m < Z < −2.0 m).
- Fits planes independently of L3 background grid.
- Advantage: No dependency on L3 settlement; can operate in parallel.
- Disadvantage: More noisy points (vehicles, pedestrians) require robust outlier rejection.

**Recommendation:** Support both modes via configuration flag. Default to Mode 1 (post-L3) for production; Mode 2 for offline PCAP analysis where L3 grid may not be available.

### Replacing HeightBandFilter

The current `HeightBandFilter` (`internal/lidar/l4perception/ground.go`) uses fixed Z thresholds:

```go
type HeightBandFilter struct {
    FloorHeightM   float64 // e.g., -2.8 m
    CeilingHeightM float64 // e.g., +1.5 m
}
```

**Migration path:**

1. **Phase 1 (co-existence)** — Ground plane extractor runs in parallel with HeightBandFilter. Both produce height-filtered points; compare outputs for validation.
2. **Phase 2 (hybrid)** — HeightBandFilter uses ground plane's Z-offset per tile as a **dynamic floor threshold**:
   ```go
   tile := groundPlane.GetTileAtPoint(x, y)
   dynamicFloor := tile.ZOffset(x, y) // ground plane's Z at (x,y)
   if z < dynamicFloor || z > dynamicFloor + maxHeight {
       // Discard point
   }
   ```
3. **Phase 3 (replacement)** — HeightBandFilter replaced by `GroundPlaneFilter` that queries the ground plane grid directly.

**Backward compatibility:** HeightBandFilter remains available as a fallback if ground plane is unsettled or unavailable.

### Data Flow Diagram

```
┌─────────────────┐
│ L1: UDP Packets │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ L2: Frame Build │ (SphericalToCartesian → sensor-local Cartesian)
└────────┬────────┘
         │
         ▼
┌──────────────────┐
│ L3: Background   │
│     Grid         │
│ (foreground sep) │
└────────┬─────────┘
         │ Ground-classified static points
         ▼
┌───────────────────────────────────────────────────────────┐
│ L4: Perception                                            │
│                                                           │
│  ┌─────────────────────┐    ┌──────────────────────────┐  │
│  │ Ground Plane        │    │ Clustering (DBSCAN)      │  │
│  │ Extractor           │    │                          │  │
│  │ (tile plane fitting)│───▶│ height-above-ground      │  │
│  │                     │    │ queries via GroundSurface │  │
│  └─────────┬───────────┘    └──────────────────────────┘  │
│            │                                              │
│  ┌─────────▼───────────┐   (optional, GPS additive)      │
│  │ Global Grid         │                                  │
│  │ diff/merge (Tier 2) │                                  │
│  └─────────────────────┘                                  │
└───────────────────────────────────────────────────────────┘
         │
         ▼
┌───────────────────────┐
│ L5: Tracking          │
└───────────┬───────────┘
            │
            ▼
┌───────────────────────┐
│ L6: Object classes    │
└───────────────────────┘
```

### Export Formats

The ground plane grid must support export for visualization and analysis:

**1. ASC Raster Grid** (existing format used by background grid):

```
ncols 100
nrows 100
xllcorner 0.0
yllcorner 0.0
cellsize 1.0
NODATA_value -9999
<grid of Z-offsets, row-major>
```

**2. VTK StructuredGrid** (for ParaView/LidarView):

```xml
<VTKFile type="StructuredGrid">
  <StructuredGrid WholeExtent="0 99 0 99 0 0">
    <Piece Extent="0 99 0 99 0 0">
      <Points>
        <DataArray Name="Points" NumberOfComponents="3" format="ascii">
          <!-- X Y Z for each grid vertex -->
        </DataArray>
      </Points>
      <CellData>
        <DataArray Name="PlaneNormalX" ... />
        <DataArray Name="PlaneNormalZ" ... />
        <DataArray Name="Planarity" ... />
        <DataArray Name="PointCount" ... />
      </CellData>
    </Piece>
  </StructuredGrid>
</VTKFile>
```

**3. GeoJSON** (for GIS tools):

```json
{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "geometry": {
        "type": "Polygon",
        "coordinates": [[lon1, lat1], [lon2, lat2], ...] // tile corners
      },
      "properties": {
        "z_offset": -2.85,
        "plane_normal": [0.01, 0.02, 0.999],
        "planarity": 0.98,
        "point_count": 145,
        "settled": true
      }
    },
    ...
  ]
}
```

**4. PCD (Point Cloud Data)** — Export tile centres with attributes:

```
# .PCD v0.7
FIELDS x y z normal_x normal_y normal_z planarity point_count
SIZE 4 4 4 4 4 4 4 4
TYPE F F F F F F F I
...
```

These exports integrate with the existing `exportFrameToASC` workflow and LidarView standards (see `docs/lidar/architecture/lidar-background-grid-standards.md`).

---

## 8. Data Structures (Go)

### Core Types

```go
package l4perception // ground plane lives within L4 Perception

import (
    "sync"
    "time"
)

// GroundTile represents a single tile in the ground plane grid.
// Each tile models the ground surface as a plane equation: n·(x,y,z) = d.
type GroundTile struct {
    // Spatial bounds (world frame, metres)
    CentreX float64
    CentreY float64
    TileSize float64 // e.g., 1.0 m

    // Plane equation: n·(x,y,z) = d where n is unit normal
    NormalX float64 // Normal vector X component
    NormalY float64 // Normal vector Y component
    NormalZ float64 // Normal vector Z component (typically ~1.0 for near-horizontal ground)
    Offset  float64 // Scalar d in plane equation

    // Running statistics for incremental plane fitting
    PointCount   uint32
    SumX, SumY, SumZ float64
    SumXX, SumYY, SumZZ float64
    SumXY, SumXZ, SumYZ float64

    // Confidence and settlement
    Planarity        float32 // Range [0,1]; 1.0 = perfect plane
    Settled          bool    // true if meets minPoints + minPlanarity + minDuration
    LastUpdateNanos  int64   // Timestamp of last observation
    ConfirmationCount uint32 // Increments when new points match plane; decrements on outliers

    // Freeze mechanism (similar to BackgroundCell)
    FrozenUntilNanos int64 // Don't update until this time (outlier protection)
    LockedBaseline   bool  // true if tile has high confidence and resists change

    // Coverage metrics
    PointDensity float32 // Points per m² per second (recent average)
}

// GroundPlaneGrid manages the Cartesian tile grid (Tier 1: local scene).
// Operates in sensor-local coordinates. No GPS required.
type GroundPlaneGrid struct {
    // Configuration
    TileSize float64 // Metres per tile (e.g., 1.0)
    MinPointsForSettlement int // e.g., 20
    MinPlanarityThreshold float32 // e.g., 0.95
    MinSettlementDurationNanos int64 // e.g., 5e9 (5 seconds)
    OutlierThresholdMeters float32 // e.g., 0.10 m

    // Tiles stored in a hash map (sparse grid)
    Tiles map[TileIndex]*GroundTile
    mu    sync.RWMutex // Protects Tiles map

    // Optional: spatial index for fast radius queries
    // (e.g., R-tree or quad-tree; defer to future optimization)

    // Statistics
    TotalPoints   uint64
    SettledTiles  uint32
    UnsettledTiles uint32
}

// TileIndex is a 2D grid coordinate (ix, iy).
type TileIndex struct {
    IX int
    IY int
}

// GroundPlaneParams mirrors BackgroundParams pattern for configuration.
type GroundPlaneParams struct {
    TileSize float64 // Metres per tile
    MinPointsForSettlement int
    MinPlanarityThreshold float32
    MinSettlementDurationNanos int64
    OutlierThresholdMeters float32
    StaleTimeoutNanos int64 // Tiles not seen for this duration decay confidence
    UpdateAlphaUnsettled float32 // e.g., 0.10 (fast convergence)
    UpdateAlphaSettled float32 // e.g., 0.01 (stability after settlement)
    EnableOutlierRejection bool // Use median-based outlier filter
}

// GroundSurface is the non-point-based interface published by the ground plane
// extractor to the rest of L4 Perception. It provides height-above-ground queries
// without exposing the internal tile representation.
type GroundSurface interface {
    // QueryHeightAboveGround returns the height of a point above the ground plane.
    // Returns (height, confidence, ok). ok=false if no settled tile at (x, y).
    QueryHeightAboveGround(x, y, z float64) (height float64, confidence float32, ok bool)

    // IsSettled reports whether the ground plane has sufficient coverage to be trusted.
    IsSettled() bool

    // TileAt returns the ground plane parameters at (x, y).
    // Returns (normal, offset, confidence, ok).
    TileAt(x, y float64) (normal [3]float64, offset float64, confidence float32, ok bool)
}

// GlobalGroundGrid is the Tier 2 persistent grid aligned to lat/long millidegree tiles.
// GPS required for population. Loaded at startup to seed local scene grids.
type GlobalGroundGrid struct {
    // Grid resolution: system constant at 0.001 degrees (~111 m equator, ~43.5 m at 67° latitude).
    // Fixed to enable cross-device interoperability — all velocity.report instances share the same grid.
    ResolutionDeg float64 // Always 0.001

    // Tiles indexed by millidegree coordinates
    Tiles map[GlobalTileIndex]*GlobalGroundTile
    mu    sync.RWMutex

    // Persistence
    LastFlushNanos int64
}

// GlobalTileIndex identifies a tile in the global lat/long grid.
type GlobalTileIndex struct {
    LatMillideg int // floor(latitude / 0.001)
    LonMillideg int // floor(longitude / 0.001)
}

// GlobalGroundTile aggregates ground plane statistics across observation sessions.
type GlobalGroundTile struct {
    MeanNormal     [3]float64 // Weighted average plane normal
    MeanZOffset    float64    // Weighted average ground height
    SessionCount   uint32     // Number of contributing sessions
    LastUpdatedNanos int64    // Most recent contribution
    Confidence     float32   // Combined planarity from all sessions
    TotalPoints    uint64    // Sum of points across all sessions
}
```

### Key Methods

```go
// AddPoint updates the tile containing world-frame point (x, y, z).
// Returns the tile index and whether the point was accepted (not an outlier).
func (g *GroundPlaneGrid) AddPoint(x, y, z float64, timestamp int64) (TileIndex, bool) {
    idx := g.WorldToTileIndex(x, y)
    tile := g.GetOrCreateTile(idx)

    // Outlier check (if tile is settled)
    if tile.Settled {
        dist := tile.DistanceToPlane(x, y, z)
        if dist > g.OutlierThresholdMeters {
            // Potential outlier; freeze and decrement confirmation
            tile.ConfirmationCount--
            if tile.ConfirmationCount < g.MinConfirmationThreshold {
                tile.Settled = false // Revert to unsettled
            }
            tile.FrozenUntilNanos = timestamp + 3e9 // 3-second freeze
            return idx, false
        }
    }

    // Update running statistics
    tile.PointCount++
    tile.SumX += x
    tile.SumY += y
    tile.SumZ += z
    tile.SumXX += x * x
    tile.SumYY += y * y
    tile.SumZZ += z * z
    tile.SumXY += x * y
    tile.SumXZ += x * z
    tile.SumYZ += y * z
    tile.LastUpdateNanos = timestamp

    // Check settlement criteria
    if !tile.Settled && tile.PointCount >= g.MinPointsForSettlement {
        g.FitPlane(tile)
        if tile.Planarity >= g.MinPlanarityThreshold &&
           (timestamp - tile.FirstObservationNanos) >= g.MinSettlementDurationNanos {
            tile.Settled = true
            tile.LockedBaseline = false // Not yet locked; needs more confirmation
            g.SettledTiles++
        }
    } else if tile.Settled {
        // Incremental update with low alpha
        g.UpdatePlane(tile, x, y, z, g.UpdateAlphaSettled)
        tile.ConfirmationCount++
        if tile.ConfirmationCount > g.LockedBaselineThreshold {
            tile.LockedBaseline = true
        }
    }

    g.TotalPoints++
    return idx, true
}

// FitPlane performs eigenvalue decomposition to extract plane normal and offset.
func (g *GroundPlaneGrid) FitPlane(tile *GroundTile) {
    // Compute mean
    n := float64(tile.PointCount)
    meanX := tile.SumX / n
    meanY := tile.SumY / n
    meanZ := tile.SumZ / n

    // Compute covariance matrix (3×3 symmetric)
    cxx := tile.SumXX/n - meanX*meanX
    cyy := tile.SumYY/n - meanY*meanY
    czz := tile.SumZZ/n - meanZ*meanZ
    cxy := tile.SumXY/n - meanX*meanY
    cxz := tile.SumXZ/n - meanX*meanZ
    cyz := tile.SumYZ/n - meanY*meanZ

    // Eigenvalue decomposition (use library or closed-form solution)
    // Smallest eigenvector → plane normal
    // Eigenvalues λ1 ≥ λ2 ≥ λ3
    // Planarity = 1 - (λ3 / λ2)
    normal, eigenvalues := eigenDecomp3x3(cxx, cyy, czz, cxy, cxz, cyz)
    tile.NormalX = normal[0]
    tile.NormalY = normal[1]
    tile.NormalZ = normal[2]
    tile.Offset = normal[0]*meanX + normal[1]*meanY + normal[2]*meanZ

    // Compute planarity score
    if eigenvalues[1] > 1e-6 { // Avoid division by zero
        tile.Planarity = float32(1.0 - eigenvalues[2]/eigenvalues[1])
    } else {
        tile.Planarity = 0.0 // Degenerate case
    }
}

// DistanceToPlane computes signed distance from point (x,y,z) to tile's plane.
func (tile *GroundTile) DistanceToPlane(x, y, z float64) float64 {
    return math.Abs(tile.NormalX*x + tile.NormalY*y + tile.NormalZ*z - tile.Offset)
}

// QueryHeightAboveGround returns the height of point (x, y, z) above the ground plane.
// Returns (height, confidence, ok). ok=false if no settled tile at (x, y).
func (g *GroundPlaneGrid) QueryHeightAboveGround(x, y, z float64) (float64, float32, bool) {
    idx := g.WorldToTileIndex(x, y)
    g.mu.RLock()
    tile, exists := g.Tiles[idx]
    g.mu.RUnlock()

    if !exists || !tile.Settled {
        return 0.0, 0.0, false
    }

    // Signed distance to plane (positive if above ground)
    height := tile.NormalX*x + tile.NormalY*y + tile.NormalZ*z - tile.Offset
    return height, tile.Planarity, true
}

// WorldToTileIndex converts world-frame (x, y) to tile index (ix, iy).
func (g *GroundPlaneGrid) WorldToTileIndex(x, y float64) TileIndex {
    return TileIndex{
        IX: int(math.Floor(x / g.TileSize)),
        IY: int(math.Floor(y / g.TileSize)),
    }
}

// GetOrCreateTile retrieves or allocates a tile at index idx.
func (g *GroundPlaneGrid) GetOrCreateTile(idx TileIndex) *GroundTile {
    g.mu.Lock()
    defer g.mu.Unlock()

    if tile, exists := g.Tiles[idx]; exists {
        return tile
    }

    // Create new tile
    tile := &GroundTile{
        CentreX:  float64(idx.IX)*g.TileSize + g.TileSize/2.0,
        CentreY:  float64(idx.IY)*g.TileSize + g.TileSize/2.0,
        TileSize: g.TileSize,
        NormalZ:  1.0, // Default to horizontal plane
        Planarity: 0.0,
        Settled:  false,
    }
    g.Tiles[idx] = tile
    g.UnsettledTiles++
    return tile
}
```

### Storage Schema

Extend `internal/lidar/storage/sqlite/schema.sql` with a ground plane table:

```sql
CREATE TABLE IF NOT EXISTS ground_plane_snapshots (
    snapshot_id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp_nanos INTEGER NOT NULL,
    sensor_id TEXT,
    origin_lat REAL,
    origin_lon REAL,
    tile_size_meters REAL,
    tiles_blob BLOB, -- gzip-compressed gob-encoded []GroundTile
    tiles_hash TEXT, -- SHA256 for deduplication
    settled_tile_count INTEGER,
    total_point_count INTEGER,
    params_json TEXT -- JSON serialisation of GroundPlaneParams
);

CREATE INDEX IF NOT EXISTS idx_ground_plane_timestamp ON ground_plane_snapshots(timestamp_nanos);
```

**Serialisation format:**

- Use `gob` encoding (Go standard) for `[]GroundTile` slice.
- Compress with `gzip` to reduce BLOB size (similar to `BackgroundCell` snapshots).
- Compute SHA256 hash for deduplication (avoid storing identical snapshots).

---

## 9. Open Questions

### Sensor Mount Calibration

**Problem:** The sensor's tilt angle (pitch, roll) relative to gravity affects the ground plane orientation in sensor frame. A sensor mounted with 2° forward tilt will observe the ground plane at a different angle than a perfectly level sensor.

**Options:**

1. **Manual calibration** — User provides pitch/roll/yaw offsets during sensor setup. Apply correction transform before ground plane fitting.
2. **Automatic calibration** — Assume ground is locally horizontal (Z-normal ≈ [0, 0, 1]) and compute sensor tilt from observed ground plane normal. Requires settled tiles first.
3. **IMU integration** — Use IMU (if available) to measure sensor orientation relative to gravity. Apply IMU-derived transform to correct LiDAR points.

**Recommendation:** Start with manual calibration; add automatic calibration as post-processing step for PCAP analysis. IMU integration is future work (requires hardware support).

### Multi-Pass PCAP vs Single-Pass Streaming

**PCAP replay mode:**

- Multiple passes enable global statistics (e.g., compute median Z-offset across all observations before fitting planes).
- Can use RANSAC with larger sample sizes for better outlier rejection.
- Slower but more accurate.

**Streaming mode:**

- Single-pass only; must make decisions with limited context.
- Relies on incremental algorithms and median-based outlier filtering.
- Faster but less robust to transient occlusions.

**Recommendation:** Support both modes. PCAP analysis tool (`cmd/tools/pcap-analyse/main.go`) should use multi-pass for maximum accuracy; live streaming uses single-pass incremental algorithms.

### GPS Geo-Referencing Integration (Additive Only)

**Principle:** GPS is strictly additive. The ground plane **must** function without GPS. GPS enables Tier 2 global grid population and geographic exports but is never required for core perception.

**Current state:** GPS parsing exists (`internal/lidar/l1packets/parse/extract.go`) but integration with LiDAR pipeline is incomplete.

**When GPS is available:**

1. **Sensor position** (lat, lon, altitude) to define Tier 2 grid origin.
2. **Sensor heading** (compass bearing) to transform sensor-local X/Y → world-frame East/North.
3. **Timestamp synchronisation** — GPS time must align with LiDAR frame timestamps (PTP or GPS-disciplined system clock).

**When GPS is unavailable (primary operating mode):**

- Tier 1 local scene grid operates in sensor-local coordinates.
- Height-above-ground queries, clustering, and all L4 perception functions work normally.
- No geographic exports (GeoJSON tile corners require GPS coordinates).
- No Tier 2 global grid population (requires geographic positioning).

**Recommendation:** GPS integration is a separate enhancement phase. Core ground plane implementation is LiDAR-only.

### Integration with External Elevation Models

**Future enhancement:** Seed ground plane tiles from prior terrain data (e.g., USGS DEMs, OpenStreetMap elevation layers).

**Benefits:**

- Faster convergence (start with prior estimate).
- Coverage for occluded areas (sensor never observes but terrain model provides estimate).

**Challenges:**

- External DEMs may be outdated (construction, road changes).
- Resolution mismatch (DEM at 10 m resolution vs ground plane at 1 m).
- Licensing and data availability.

**Recommendation:** Defer to future work. Initial implementation uses LiDAR observations only.

### Ground Plane Maintenance in Dynamic Environments

**Problem:** Parked cars, construction equipment, temporary barriers can occlude ground points. How to maintain ground plane when parts of the scene change?

**Options:**

1. **Confidence decay** — Tiles not observed for >60 seconds lose confidence; excluded from queries.
2. **Temporal filtering** — Maintain history of plane parameters per tile; detect sudden changes (construction, kerb modification) and flag for manual review.
3. **Multi-device cross-validation** — If multiple sensors observe the same area, compare their ground plane estimates to detect sensor-local occlusions.

**Recommendation:** Implement confidence decay (Option 1) for initial version. Temporal filtering (Option 2) is future enhancement for long-running deployments.

### OpenStreetMap Integration (Future V2)

**Vision:** Import OSM polylines (kerbs, crosswalks, signs, road edges) as real-world geometric anchors for ground plane validation and refinement.

**Import workflow (v1 — read-only):**

1. Query OSM Overpass API for road geometry within the global grid's bounding box.
2. Parse polylines (ways) for kerb lines, crosswalks, stop lines, sign positions.
3. Project OSM features onto the ground plane grid as **anchor constraints** — known height discontinuities (kerbs: +0.15 m), known flat regions (crosswalks), known positions (signs).
4. Use anchors to validate and refine ground plane tile boundaries.

**Update workflow (v2 — write-back, requires OSM API key):**

1. Compare settled ground plane against existing OSM data.
2. Identify discrepancies: kerb positions shifted, crosswalk faded/repainted, new road features.
3. Propose edits to OSM as changesets with more accurate positions derived from LiDAR measurements.
4. Requires user authentication via OSM API key and manual review before submission.

**Privacy note:** OSM write-back shares geometric features (kerb positions, road edges) — never vehicle data or PII. This is consistent with privacy-first design as it enriches the public map, not a private database.

**Deferred to:** Future work (v2). Core ground plane and Tier 2 global grid must be stable first.

---

## References

### Related Documents

- **LiDAR Data Layer Model** — `docs/lidar/architecture/lidar-data-layer-model.md` (six-layer model, L1–L6 definitions)
- **Background Grid Standards** — `docs/lidar/architecture/lidar-background-grid-standards.md` (VTK/PCD export standards, ROS interop)
- **L3 Background Grid** — `internal/lidar/l3grid/background.go` (EMA updates, freeze mechanism, settlement detection)
- **L4 HeightBandFilter** — `internal/lidar/l4perception/ground.go` (current Z-band filtering, replacement target)
- **PCAP Analysis Tool** — `cmd/tools/pcap-analyse/main.go` (multi-pass pipeline, export formats)

### External Standards

- **VTK File Formats** — [VTK XML formats](https://vtk.org/wp-content/uploads/2015/04/file-formats.pdf) (`.vti`, `.vts`, `.vtp`)
- **GeoJSON Specification** — [RFC 7946](https://tools.ietf.org/html/rfc7946) (geographic feature collections)
- **PCD Format** — [Point Cloud Data](https://pointclouds.org/documentation/tutorials/pcd_file_format.html) (PCL standard)
- **WGS84 / EPSG:4326** — [World Geodetic System 1984](https://epsg.io/4326) (lat/long coordinate reference system)

---

## Implementation Roadmap

### Phase 1: Core Extraction (MVP)

- [ ] Implement `GroundTile` and `GroundPlaneGrid` structs
- [ ] Incremental covariance accumulation and plane fitting (PCA-based)
- [ ] Tile settlement detection (point count + planarity + duration)
- [ ] Basic outlier rejection (median-based filter)
- [ ] Integration with L2 frames (Mode 2: direct from frames)
- [ ] ASC raster grid export
- [ ] Unit tests for plane fitting accuracy

### Phase 2: Confidence & Stability

- [ ] Planarity score computation (eigenvalue ratio)
- [ ] Freeze/lock mechanism (similar to BackgroundCell)
- [ ] Confidence decay for stale tiles
- [ ] Neighbour-based curvature detection
- [ ] VTK StructuredGrid export (ParaView/LidarView)
- [ ] Integration tests with PCAP replay

### Phase 3: Pipeline Integration

- [ ] Mode 1: Post-L3 integration (consume ground-classified points from background grid)
- [ ] Replace HeightBandFilter with GroundPlaneFilter
- [ ] L4 perception: query `HeightAboveGround` per cluster
- [ ] GeoJSON export (requires GPS geo-referencing)
- [ ] Multi-pass PCAP analysis (global statistics, RANSAC)

### Phase 4: Production Hardening

- [ ] Storage: `ground_plane_snapshots` table and periodic flushing
- [ ] Performance profiling (target: <5 ms per frame on Raspberry Pi 4)
- [ ] Spatial index (R-tree or quad-tree) for fast radius queries
- [ ] Sensor mount calibration (manual pitch/roll/yaw correction)
- [ ] Documentation: user guide, calibration procedure, troubleshooting

### Phase 5: Future Enhancements

- [ ] Automatic sensor tilt calibration
- [ ] Multi-device ground plane fusion
- [ ] External DEM integration (USGS, OSM elevation)
- [ ] Temporal change detection (construction zone monitoring)
- [ ] IMU integration for dynamic tilt correction
- [ ] Tier 2 global grid: diff/merge across observation sessions
- [ ] OSM polyline import for anchor constraints (kerbs, crosswalks, signs)
- [ ] OSM write-back workflow (v2, requires API key)
- [ ] **Vector scene map** — Extend tile-based ground plane into polygon-based multi-feature representation with buildings, vegetation, and hierarchical LOD (see `docs/lidar/architecture/vector-scene-map.md`)

---

## Conclusion

The ground plane extraction subsystem provides a geometric foundation for height-based object classification in velocity.report's LiDAR pipeline. By modelling the road surface as a mosaic of locally-flat tiles, it enables:

- **Accurate height-above-ground measurements** for pedestrian/vehicle/cyclist classification
- **Adaptive ground filtering** that handles San Francisco's hilly terrain and kerb discontinuities
- **Confidence-aware queries** for assessing measurement reliability
- **Optional geographic alignment** for multi-device fusion and GIS integration (GPS additive)

The piecewise-planar tile approach balances **spatial resolution** (1 m tiles capture local features), **computational efficiency** (incremental PCA, O(1) per-tile updates), and **robustness** (outlier rejection, confidence decay). Settlement within 5–10 seconds ensures rapid deployment while maintaining stability via locked baseline mechanisms.

The ground plane lives within **L4 Perception**, publishing a `GroundSurface` interface — a non-point-based representation of the scene geometry that is unioned with point-based clustering for scene understanding. This preserves the existing L3 background grid's role for foreground/background separation while adding geometric surface reasoning within perception.

The **two-tier model** (local scene tiles + global published grid) separates concerns: Tier 1 operates with LiDAR alone (sensor-iterative, no GPS dependency), while Tier 2 enriches the system when GPS is available, enabling cross-session accumulation and geographic exports. The system always functions with the LiDAR-only sensor; GPS is only additive.

Open questions around sensor calibration, GPS geo-referencing, and multi-device fusion are deferred to future work, allowing an incremental implementation path that delivers value early (accurate height measurements) while preserving extensibility for advanced features (geographic alignment, OSM integration, external terrain databases).
