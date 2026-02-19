# Ground Plane Extraction: Mathematical Tradeoffs

**Document Status:** Architecture Analysis  
**Target Component:** LIDAR Ground Plane Filter  
**Related:** [ground-plane-extraction.md](ground-plane-extraction.md), BackgroundGrid implementation  
**Author:** Ictinus (Product Architecture)  
**Date:** 2026

## Executive Summary

This document analyses the mathematical tradeoffs in ground plane estimation for the Hesai Pandar40P LIDAR sensor on Raspberry Pi 4. We compare algorithm complexity, storage requirements, grid density constraints, and computational budgets to recommend an optimal approach for real-time streaming point cloud processing.

**Key Findings:**
- Incremental covariance + PCA offers O(1) per-point complexity with ~1.1 MB storage for 100m × 100m coverage
- Point density limits confident plane fitting to ~35m range at 1m tile resolution (single revolution)
- Per-frame processing budget: ~8ms (14.4M FLOPs) for 720K points at 10 Hz rotation
- Settlement time: 0.1-0.5s for dense tiles, 1-3s for sparse (40-80m range)
- Float64 required for covariance accumulation; float32 sufficient for plane output

---

## 1. Problem Statement

### 1.1 Formal Definition

Given a stream of LiDAR point returns `P = {p₁, p₂, ..., pₙ}` in sensor coordinates `pᵢ = (xᵢ, yᵢ, zᵢ)`, estimate a piecewise-planar ground surface model `G` as a grid of tiles `T = {t₁, t₂, ..., tₘ}`, where each tile `tⱼ` is characterised by:

- **Plane equation**: `nⱼ · (p - cⱼ) = 0` where `nⱼ` is unit normal, `cⱼ` is tile centroid
- **Confidence score**: `σⱼ ∈ [0, 1]` derived from eigenvalue ratios (planarity measure)
- **Curvature classification**: `κⱼ ∈ {flat, gentle, moderate, steep}` based on adjacent tile normals
- **Point count**: `Nⱼ` number of points contributing to the tile's plane estimate

### 1.2 Constraints

**Hardware:**
- Raspberry Pi 4: ARM Cortex-A72 @ 1.8 GHz, 4 GB RAM, SD card storage
- Hesai Pandar40P: 40 channels, 10 Hz rotation, 0.3m-200m range, 0.2° azimuth resolution

**Performance:**
- Real-time processing: Must keep up with ~1800-2000 packets/sec (~720K points/revolution)
- Latency budget: < 100ms for ground classification (< 1 revolution delay)
- Memory budget: < 10 MB for ground plane representation (including overhead)

**Accuracy:**
- Plane fitting error: < 5cm RMS for flat surfaces (within sensor precision)
- Curvature detection: Resolve grade changes > 5° (typical urban road features)

---

## 2. Algorithm Options & Complexity Analysis

### 2.1 Incremental Covariance + PCA (Recommended)

**Approach:**  
Maintain a running covariance matrix for points in each tile. Fit plane via eigendecomposition of 3×3 covariance matrix. Ground normal is eigenvector with smallest eigenvalue.

**Update per point:**
```
Given point p = (x, y, z) in tile j:

1. Update sums (O(1)):
   S_x += x
   S_y += y  
   S_z += z
   S_xx += x²
   S_yy += y²
   S_zz += z²
   S_xy += xy
   S_xz += xz
   S_yz += yz
   N += 1

2. Compute centroid (O(1)):
   c = (S_x/N, S_y/N, S_z/N)

3. Build covariance matrix (O(1), deferred until tile "settles"):
   C = [ S_xx/N - c_x²    S_xy/N - c_x·c_y   S_xz/N - c_x·c_z ]
       [ S_xy/N - c_x·c_y  S_yy/N - c_y²      S_yz/N - c_y·c_z ]
       [ S_xz/N - c_x·c_z  S_yz/N - c_y·c_z   S_zz/N - c_z²    ]

4. Eigen-decomposition (O(1), ~100 FLOPs for 3×3):
   λ₁ ≥ λ₂ ≥ λ₃  (eigenvalues)
   v₁, v₂, v₃    (eigenvectors)
   
   Ground normal n = v₃ (smallest eigenvalue)
   Plane offset d = -n · c
   
5. Confidence score (O(1)):
   σ = (λ₂ - λ₃) / λ₁   [0 = linear/scattered, 1 = perfect plane]
```

**Storage per tile:**
- 9 doubles for upper triangle + diagonal of covariance sums: 72 bytes
- 3 doubles for centroid sums (S_x, S_y, S_z): 24 bytes
- 1 uint32 for point count: 4 bytes
- 3 floats for plane normal: 12 bytes
- 1 float for plane offset: 4 bytes
- 1 float for confidence score: 4 bytes
- 1 byte for curvature classification: 1 byte
- **Total: 121 bytes per tile** (with padding → 128 bytes aligned)

**Complexity:**
- Per-point update: O(1) – 9 multiplies + 9 adds
- Per-tile fitting: O(1) – ~100 FLOPs for 3×3 eigen (closed-form Jacobi)
- Per-frame: O(n) where n = points in frame

**Advantages:**
- True streaming algorithm – no buffering required
- Memory-efficient – only running statistics stored
- Numerical stability – covariance is symmetric positive semi-definite
- Incremental updates – supports long-term averages

**Disadvantages:**
- Outlier sensitivity – requires pre-filtering (height band, intensity)
- Equal weighting – doesn't account for varying point density with range
- Cold start – needs ~20-30 points for stable eigenvalues

---

### 2.2 RANSAC per Tile

**Approach:**  
Buffer points for each tile. Periodically run RANSAC to fit plane robustly against outliers.

**Algorithm:**
```
Given N points in tile j:

For k iterations (typically k = 100-1000):
  1. Sample 3 random points: p₁, p₂, p₃
  2. Compute plane: n = (p₂ - p₁) × (p₃ - p₁), normalise
  3. Count inliers: points within threshold τ (e.g., 10cm)
  4. If inlier_count > best_inlier_count:
       best_plane = (n, d)
       best_inlier_count = inlier_count

Refit plane using all inliers via least squares.
```

**Storage per tile:**
- Buffered points: N × 12 bytes (3 × float32 per point)
- For 1m × 1m tile at 30m range: ~4 points/revolution × 12 bytes = 48 bytes/revolution
- Accumulating 10 revolutions: ~480 bytes per tile
- Plus plane output: 20 bytes
- **Total: ~500 bytes per tile** (variable with point density)

**Complexity:**
- Per-iteration: O(N) for inlier counting
- Total: O(kN) where k = iterations, N = points in tile
- For N = 50, k = 100: ~5000 operations per tile
- For 10,000 tiles: 50M operations (not real-time on Pi 4)

**Advantages:**
- Robust to outliers (vehicles, vegetation)
- Well-established algorithm with tunable parameters

**Disadvantages:**
- Not streaming-friendly – requires buffering points
- High computational cost – quadratic scaling with point count
- Non-deterministic – results vary between runs
- Memory intensive – stores all points per tile

**Use case:** Offline PCAP analysis, post-processing of static captures.

---

### 2.3 Weighted Least Squares

**Approach:**  
Accumulate weighted normal equations, giving higher weight to nearby points (or higher-intensity returns). Solve periodically via Cholesky decomposition.

**Normal equations for plane fitting:**  
Plane: `z = ax + by + c`

```
Minimise: Σᵢ wᵢ(zᵢ - axᵢ - byᵢ - c)²

Normal equations (4×4 symmetric):
[ Σw·x²   Σw·xy   Σw·x   Σw   ] [ a ]   [ Σw·xz ]
[ Σw·xy   Σw·y²   Σw·y   Σw   ] [ b ] = [ Σw·yz ]
[ Σw·x    Σw·y    Σw     Σw   ] [ c ]   [ Σw·z  ]
[ Σw      Σw      Σw     Σw   ] [ d ]   [ Σw    ]

Solve via Cholesky: A = LLᵀ, then Ly = b, Lᵀx = y
```

**Storage per tile:**
- 10 doubles for symmetric 4×4 matrix: 80 bytes
- 4 doubles for right-hand side: 32 bytes
- 4 doubles for solution vector: 32 bytes
- **Total: 144 bytes per tile**

**Complexity:**
- Per-point update: O(1) – compute weight + 10 multiply-adds
- Per-tile solve: O(1) – Cholesky of 4×4 is ~20 FLOPs
- Weight computation: O(1) – e.g., `w = exp(-r²/σ²)` for distance weighting

**Advantages:**
- Handles varying point density naturally (via distance weighting)
- Streaming-compatible (accumulate sums incrementally)
- More robust than plain covariance (down-weights distant/weak returns)

**Disadvantages:**
- Assumes ground is locally flat (z = ax + by + c form breaks on steep slopes)
- Weight tuning required (σ parameter for distance weighting)
- Slightly more storage than covariance method

---

### 2.4 Voxel Grid + Height Map

**Approach:**  
Simplest representation – track min, max, mean (or median) Z-coordinate for each (x, y) grid cell. No plane fitting.

**Update per point:**
```
Given point p = (x, y, z):

cell = grid[(x, y)]
cell.z_min = min(cell.z_min, z)
cell.z_max = max(cell.z_max, z)
cell.z_sum += z
cell.count += 1
cell.z_mean = cell.z_sum / cell.count
```

**Storage per cell:**
- 1 float for z_min: 4 bytes
- 1 float for z_max: 4 bytes
- 1 float for z_mean: 4 bytes
- 1 float for z_sum (for incremental mean): 4 bytes
- 1 uint32 for count: 4 bytes
- **Total: 20 bytes per cell**

**Complexity:**
- Per-point: O(1) – 3 comparisons + 2 adds
- Per-frame: O(n)

**Advantages:**
- Minimal storage and computation
- Trivial to implement and debug
- Suitable for flat terrain

**Disadvantages:**
- Cannot represent tilted surfaces (misses road slopes)
- No plane normal information (required for ground classification)
- No curvature estimation
- Poor performance on hills/grades (z_mean is biased)

**Use case:** Initial prototyping, flat environments (parking lots, warehouses).

---

## 3. Storage Analysis

### 3.1 Comparison Table

Assuming **incremental covariance + PCA** (128 bytes/tile including alignment):

| Coverage Area | Tile Size | Tile Count | Storage (Raw) | Compressed (gzip) | Notes |
|---------------|-----------|------------|---------------|-------------------|-------|
| 50m × 50m     | 0.5m      | 10,000     | 1.28 MB       | 220 KB            | High resolution, dense urban |
| 50m × 50m     | 1.0m      | 2,500      | 320 KB        | 55 KB             | **Recommended for dense** |
| 100m × 100m   | 1.0m      | 10,000     | 1.28 MB       | 220 KB            | **Recommended for suburban** |
| 100m × 100m   | 2.0m      | 2,500      | 320 KB        | 55 KB             | Extended range, sparser |
| 200m × 200m   | 1.0m      | 40,000     | 5.12 MB       | 880 KB            | Full sensor range |
| 200m × 200m   | 2.0m      | 10,000     | 1.28 MB       | 220 KB            | Full range, coarse |

**Compression ratios:**  
Ground plane data is highly compressible:
- Covariance matrices: Spatial correlation → gzip achieves ~5-6× reduction
- Plane normals: Quantised to 16-bit integers → ~8× reduction
- Sparse grids: Many empty tiles → run-length encoding effective

**Memory overhead:**  
Beyond raw storage, additional allocations:
- Grid index structure: Hash map or 2D array (1-2 bytes per cell for occupancy)
- Neighbour adjacency list (for curvature): ~4 pointers × 8 bytes × occupied_tiles
- Tile update queue (during streaming): ~100-1000 tile IDs × 8 bytes

**Total in-memory budget:** ~2-3 MB for 100m × 100m at 1m resolution (comfortably within 10 MB limit).

---

### 3.2 Comparison with BackgroundGrid

Current `BackgroundGrid` implementation (polar coordinates):
- ~360 azimuth bins × 40 range rings = 14,400 cells
- 42 bytes per cell (smaller than ground plane tile, but includes different data)
- ~600 KB total (uncompressed)

**Ground plane vs BackgroundGrid:**
- Ground plane: Cartesian (x, y) grid, denser in close range, sparser at distance
- BackgroundGrid: Polar (θ, r) grid, uniform angular resolution, range-binned
- Ground plane tiles: Larger storage per tile (128 vs 42 bytes) but fewer tiles in practice (only occupied ground cells)
- BackgroundGrid: Every cell allocated (full 360° × 40 rings), but lighter per-cell

**Complementary roles:**
- BackgroundGrid: Static object detection (poles, walls, parked cars) in sensor frame
- Ground plane: Geometric ground model in world frame for elevation filtering

---

## 4. Grid Density vs PCAP Range Analysis

### 4.1 Point Density as Function of Range

**Pandar40P specifications:**
- 40 channels (vertical laser beams)
- 10 Hz rotation rate (36,000°/sec)
- Azimuth resolution: 0.2° (1800 azimuth steps per revolution)
- Points per revolution: 40 × 1800 = 72,000 points
- Corrected: Dual-return mode, 10 blocks/packet → ~720,000 points/revolution

**Spherical spreading law:**  
Point density (points per m²) at range `r` from sensor:

```
ρ(r) ≈ (N_channels × N_azimuth) / (4πr²)
```

For Pandar40P at 10 Hz (single revolution):
```
ρ(r) = 720,000 / (4π r²) ≈ 57,300 / r²  points/m²
```

**Specific ranges:**

| Range (m) | Azimuth Spacing (cm) | Elevation Spacing (cm) | Points/m² (per rev) | Points in 1m² tile |
|-----------|----------------------|------------------------|---------------------|--------------------|
| 5         | 1.7                  | ~30 (chan-dependent)   | 2,290               | 2,290              |
| 10        | 3.5                  | ~30                    | 573                 | 573                |
| 20        | 7.0                  | ~30                    | 143                 | 143                |
| 30        | 10.5                 | ~30                    | 64                  | 64                 |
| 50        | 17.4                 | ~30                    | 23                  | 23                 |
| 70        | 24.4                 | ~30                    | 12                  | 12                 |
| 100       | 34.9                 | ~30                    | 5.7                 | 6                  |
| 150       | 52.4                 | ~30                    | 2.5                 | 3                  |
| 200       | 69.8                 | ~30                    | 1.4                 | 1-2                |

**Note:** Elevation spacing depends on channel assignment. Pandar40P has non-uniform vertical distribution (-16° to +15°), so elevation density varies with tilt.

---

### 4.2 Minimum Points for Confident Plane Fit

**Eigenvalue stability:**  
For 3×3 covariance matrix, reliable eigendecomposition requires:
- Minimum 4 points (overdetermined: 3 DOF for plane, 1 constraint)
- Practical minimum: **20-30 points** for numerical stability

Reasoning:
- With N < 10 points: Eigenvalue λ₃ (ground normal) is noisy, sensitive to outliers
- With N = 20-30: λ₃ stabilises, confidence score σ = (λ₂ - λ₃)/λ₁ becomes meaningful
- With N > 50: Diminishing returns, plane estimate converges

**Range limits for confident fits:**

At **1m × 1m tile resolution**, single revolution (10 Hz):
- Confident to ~30m (64 points/tile)
- Marginal at 50m (23 points/tile)
- Sparse beyond 70m (12 points/tile, requires accumulation)

At **2m × 2m tile resolution**, single revolution:
- 4× more points per tile (2² scaling)
- Confident to ~60m (23 × 4 = 92 points/tile)
- Marginal at 100m (23 points/tile)

**Accumulation strategy:**  
Accumulate over K revolutions (K seconds at 10 Hz):
- Point count scales linearly: N_total = K × N_per_rev
- For K = 10 revolutions (1 second):
  - 1m tiles: Confident to ~50m (230 points)
  - 2m tiles: Confident to ~100m (230 points)

**Recommendation:**  
- Use 1m tiles for r < 40m (primary ground classification zone)
- Switch to 2m tiles for 40m < r < 80m (extended range)
- Accumulate 5-10 revolutions before declaring tile "settled"

---

### 4.3 Adaptive Tiling Strategies

**Option A: Fixed tile size, minimum range cutoff**  
- 1m × 1m tiles everywhere
- Discard tiles beyond 40m (insufficient points)
- Simplest implementation, ~1600 tiles for 40m radius

**Option B: Distance-based tile sizing**  
- r < 30m: 0.5m tiles (high resolution)
- 30m < r < 60m: 1.0m tiles (standard)
- 60m < r < 100m: 2.0m tiles (coarse)
- Complexity: 3 grid resolutions, coordinate mapping

**Option C: Hierarchical grid (quadtree)**  
- Start with coarse tiles (4m)
- Subdivide tiles with > 100 points into 4 children (2m each)
- Recursively subdivide to 0.5m minimum
- Adaptive resolution, but complex indexing

**Recommended: Option A** – Fixed 1m tiles, 50m cutoff, with 10-revolution accumulation for 30-50m range. Simplest to implement, predictable storage, sufficient for traffic monitoring (vehicles are within 30m).

---

## 5. Computational Complexity on Raspberry Pi 4

### 5.1 Hardware Specifications

**Raspberry Pi 4 Model B:**
- CPU: Broadcom BCM2711, Quad-core Cortex-A72 (ARMv8) @ 1.8 GHz
- L1 Cache: 48 KB I-cache + 32 KB D-cache per core
- L2 Cache: 1 MB shared
- RAM: 4 GB LPDDR4-3200
- Single-precision FLOPS: ~7.2 GFLOPS (theoretical, 1 FLOP/cycle × 4 cores × 1.8 GHz)
- Double-precision: ~3.6 GFLOPS (ARMv8 has native FP64 support)

**Thermal throttling:** Pi 4 throttles at ~80°C, reducing clock to 1.5 GHz. Critical for sustained workloads.

---

### 5.2 Per-Frame Processing Budget

**Frame definition:** 1 complete LiDAR revolution at 10 Hz = 0.1 second = 100ms.

**Point classification (tile assignment):**  
For each point `p = (x, y, z)`, compute tile index:
```
tile_x = floor(x / tile_size)
tile_y = floor(y / tile_size)
tile_index = (tile_x, tile_y)
```
- Operations: 2 divisions (or 2 multiplies if tile_size is power-of-2), 2 floor ops, 1 hash lookup
- Cost: ~5-10 cycles per point (with branch misprediction for new tiles)
- Total for 720K points: ~3.6-7.2M cycles = **2-4ms** at 1.8 GHz

**Covariance update:**  
Per point: 9 multiplies + 9 adds for covariance sums, 3 adds for centroid.
```
S_xx += x * x;    // 1 multiply, 1 add
S_yy += y * y;
S_zz += z * z;
S_xy += x * y;
S_xz += x * z;
S_yz += y * z;
S_x += x;         // 1 add
S_y += y;
S_z += z;
```
- Cost: ~20 FLOPs per point (with pipelining, ~10-15 cycles on ARM)
- Total for 720K points: ~14.4M FLOPs = **7.2-10.8M cycles = 4-6ms** at 1.8 GHz

**Plane fitting (per settled tile):**  
3×3 eigendecomposition via Jacobi iteration or closed-form solution.
- Closed-form 3×3 eigen: ~100 FLOPs (compute cubic characteristic polynomial, solve analytically)
- Per tile: ~50 cycles (including confidence score computation)
- For 1000 active tiles per frame: 50K cycles = **0.03ms** (negligible)

**Total per-frame cost:**  
Point classification + covariance update: **6-10ms per revolution**

**Headroom:**  
Frame period = 100ms (at 10 Hz). Ground plane processing consumes ~6-10% of frame budget. Remaining 90ms available for:
- Packet reception and parsing (~10-20ms)
- Background grid updates (~5-10ms)
- Object detection (~20-30ms)
- Web API streaming (~10ms)

---

### 5.3 Memory Bandwidth Considerations

**Point data streaming:**  
720K points × 12 bytes (x, y, z as float32) = 8.64 MB per revolution.

At 10 Hz: **86.4 MB/sec point data throughput**.

**LPDDR4-3200 bandwidth:**  
Theoretical peak: 25.6 GB/sec (dual-channel, 64-bit width)  
Practical sustained: ~10-15 GB/sec (cache misses, contention)

Ground plane processing is **not memory-bound** – only 0.86% of available bandwidth.

**Cache-friendly access pattern:**  
Covariance updates touch same tile repeatedly (temporal locality). With 128 bytes per tile:
- L1 D-cache: 32 KB → can hold ~250 tiles actively updating
- L2 cache: 1 MB → can hold ~8000 tiles

For 1000 active tiles (typical per frame), **all tile data fits in L2 cache** → minimal DRAM access.

---

## 6. Settlement Time Analysis

### 6.1 Convergence Criteria

A tile is considered "settled" when:
1. **Point count threshold:** N ≥ N_min (typically 30-50 points)
2. **Eigenvalue stability:** λ₃ / λ₁ < 0.01 (indicates strong planarity)
3. **Temporal stability:** Plane normal hasn't changed by > 5° in last K revolutions

**Settlement latency** = time to accumulate N_min points at given range.

---

### 6.2 Settlement Times by Range

Using point density from Section 4.1, compute revolutions to reach N_min = 30 points:

| Range (m) | Points/Rev (1m tile) | Revs for N=30 | Time (sec) | Tile Size | Alt Points/Rev | Alt Revs |
|-----------|----------------------|---------------|------------|-----------|----------------|----------|
| 5         | 2,290                | 1             | 0.1        | 1m        | —              | —        |
| 10        | 573                  | 1             | 0.1        | 1m        | —              | —        |
| 20        | 143                  | 1             | 0.1        | 1m        | —              | —        |
| 30        | 64                   | 1             | 0.1        | 1m        | —              | —        |
| 40        | 36                   | 1             | 0.1        | 1m        | 144 (2m)       | 1        |
| 50        | 23                   | 2             | 0.2        | 1m        | 92 (2m)        | 1        |
| 60        | 16                   | 2             | 0.2        | 1m        | 64 (2m)        | 1        |
| 70        | 12                   | 3             | 0.3        | 1m        | 48 (2m)        | 1        |
| 80        | 9                    | 4             | 0.4        | 1m        | 36 (2m)        | 1        |
| 100       | 6                    | 5             | 0.5        | 1m        | 24 (2m)        | 2        |
| 150       | 3                    | 10            | 1.0        | 1m        | 12 (2m)        | 3        |

**Interpretation:**
- **Dense range (< 30m):** Single-revolution settlement – immediate ground classification
- **Medium range (30-60m):** 1-2 revolution delay – acceptable for moving vehicle (< 0.2s latency)
- **Sparse range (60-100m):** 3-5 revolutions – 0.3-0.5s latency (still real-time)
- **Very sparse (> 100m):** 10+ revolutions or switch to 2m tiles

---

### 6.3 Dynamic vs Static Capture Settlement

**Static PCAP capture:**  
Sensor stationary, accumulating points in same tiles over time.
- Settlement time: As above, but can accumulate indefinitely
- 1-second capture (10 revolutions): Confident tiles to 100m even at 1m resolution
- Ideal for offline map building

**Dynamic (vehicle-mounted sensor):**  
Sensor moving, tiles continuously enter/exit field of view.
- Settlement time: Limited by dwell time in tile's coverage
- At 30 km/h (~8.3 m/s), tile at 30m range is visible for ~3.6 seconds (36 revolutions)
- At 60 km/h (~16.7 m/s), tile at 30m range is visible for ~1.8 seconds (18 revolutions)
- Sufficient for settlement even at highway speeds

**velocity.report use case:**  
Sensor is **stationary** (neighbourhood traffic monitoring). Settlement is not time-critical – can accumulate over minutes/hours for high-confidence ground model.

---

## 7. Curvature Estimation Mathematics

### 7.1 Finite Difference Curvature

Given a tile `T_i` with plane `(n_i, d_i)` and neighbouring tiles `T_j` (4-connected: north, south, east, west), compute curvature as:

**Angular curvature (plane normal difference):**
```
κ_ij = arccos(n_i · n_j)  [radians]
```

Where `n_i · n_j` is dot product of unit normals.

**Tile-to-tile height difference:**
```
Δh_ij = |d_i - d_j|  [metres]
```

**Gradient (slope) between tiles:**
```
grade_ij = Δh_ij / dist_ij × 100%
```
Where `dist_ij = tile_size` (for adjacent tiles).

---

### 7.2 Curvature Classification

Based on angular difference `κ` (in degrees):

| Class      | Angle Range | Grade Range | Examples                          |
|------------|-------------|-------------|-----------------------------------|
| Flat       | 0° - 1°     | 0% - 1.7%   | Parking lot, runway               |
| Gentle     | 1° - 5°     | 1.7% - 8.7% | Residential street, bike path     |
| Moderate   | 5° - 15°    | 8.7% - 27%  | SF hills (Filbert St), on-ramps   |
| Steep      | > 15°       | > 27%       | Lombard St (27%), Mt. Washington (35%) |

**San Francisco context:**  
- Typical SF hill: 10-15% grade = 6-9° angle
- Steepest SF streets: Filbert St (17.5%), 22nd St (16%) → 10° angle
- Lombard Street (famous switchbacks): 27% grade = 15° angle

**Detection threshold:**  
5° curvature change reliably distinguishes street features:
- Driveways (5-10° transition from flat to sloped)
- Speed humps (10-15° curvature over 1-2m)
- Curbs (sharp discontinuity, > 15° locally)

---

### 7.3 Curvature Computation Workflow

**Per tile:**
1. Compute plane `(n_i, d_i)` via PCA eigendecomposition
2. For each 4-connected neighbour tile `j`:
   - Compute `κ_ij = arccos(n_i · n_j)`
   - Store max curvature: `κ_i = max_j(κ_ij)`
3. Classify tile curvature: `class_i = classify(κ_i)`

**Storage:**  
1 byte per tile for curvature class (enum: 0=flat, 1=gentle, 2=moderate, 3=steep).

**Computational cost:**  
- Per tile: 4 dot products (3 FLOPs each) + 4 arccos (20 FLOPs each via Taylor series) = 92 FLOPs
- For 1000 tiles: 92K FLOPs = **0.05ms** at 1.8 GHz (negligible)

---

### 7.4 Numerical Stability for Arccos

**Problem:**  
`arccos(x)` is numerically unstable for `x ≈ 1` (nearly parallel normals).

**Solution:**  
Use identity: `arccos(x) = arctan(sqrt(1 - x²) / x)` for `x > 0.9`.

Alternatively, compute cross product magnitude for small angles:
```
For small θ: sin(θ) ≈ θ
n_i × n_j ≈ θ  (in radians, when n_i, n_j are unit vectors)
```

**Implementation:**
```go
func angleBetweenNormals(n1, n2 [3]float64) float64 {
    dot := n1[0]*n2[0] + n1[1]*n2[1] + n1[2]*n2[2]
    
    if dot > 0.9999 {  // < 0.8° angle, use cross product
        cross := [3]float64{
            n1[1]*n2[2] - n1[2]*n2[1],
            n1[2]*n2[0] - n1[0]*n2[2],
            n1[0]*n2[1] - n1[1]*n2[0],
        }
        crossMag := math.Sqrt(cross[0]*cross[0] + cross[1]*cross[1] + cross[2]*cross[2])
        return crossMag  // radians
    }
    
    return math.Acos(math.Max(-1.0, math.Min(1.0, dot)))  // clamp for safety
}
```

---

## 8. Numerical Stability Considerations

### 8.1 Catastrophic Cancellation in Covariance

**Problem:**  
Computing `S_xx/N - (S_x/N)²` suffers from catastrophic cancellation when points are far from origin.

Example: Points at `x ≈ 50m`, `S_xx ≈ 2500 × N`, `(S_x/N)² ≈ 2500`. Subtracting two large numbers with small difference loses precision.

**Solution: Shift coordinates to tile centre**  
```
For tile centred at (c_x, c_y):
  x' = x - c_x
  y' = y - c_y
  z' = z - c_z  (where c_z = S_z / N initially)

Accumulate covariance in shifted coordinates:
  S_xx' = Σ(x')²
  S_xy' = Σ(x')(y')
  ...

Result: S_xx', S_yy', etc., are O(tile_size²) ≈ 1-4, not O(1000) → better conditioning.
```

**Implementation:**  
Compute initial centroid estimate from first K points (K = 10), then shift all subsequent points. Recompute centroid incrementally in shifted frame.

---

### 8.2 Welford's Algorithm for Running Variance

**Standard formula (two-pass):**
```
mean = Σx / N
variance = Σ(x - mean)² / N
```
Requires storing all points or two passes.

**Welford's incremental algorithm (one-pass):**
```
For each new point x:
  delta = x - mean
  mean += delta / N
  M2 += delta * (x - mean)
  
After N points:
  variance = M2 / N
```

**Adaptation for covariance matrix:**  
Extend Welford to 3D covariance:
```
For each new point p = (x, y, z):
  delta = p - centroid
  centroid += delta / N
  C += outer(delta, p - centroid)  // Rank-1 update
```

Where `outer(u, v) = u ⊗ v` is outer product (3×3 matrix).

**Storage:**  
Same as incremental covariance (9 doubles for C, 3 for centroid, 1 for N).

**Advantage:**  
Numerically stable for long accumulations (millions of points over hours).

---

### 8.3 Float32 vs Float64 Precision

**Float32 (single precision):**
- 23-bit mantissa → ~7 decimal digits precision
- Range: ±3.4 × 10³⁸
- Sufficient for: Coordinates (0.1mm resolution at 100m range), plane normals (0.001° angular resolution)

**Float64 (double precision):**
- 52-bit mantissa → ~15 decimal digits precision
- Range: ±1.7 × 10³⁰⁸
- Required for: Covariance sums (accumulating squares of large coordinates)

**Recommendation:**
- **Accumulation (covariance sums):** Float64 (avoid cumulative rounding errors over millions of points)
- **Plane output (normals, offsets):** Float32 (sufficient precision, saves 50% storage)
- **Point data (x, y, z):** Float32 (sensor precision is ~2cm at 100m, no need for float64)

**Conversion strategy:**  
```go
type TileAccumulator struct {
    S_xx, S_yy, S_zz float64  // Covariance sums (float64)
    S_xy, S_xz, S_yz float64
    S_x, S_y, S_z    float64
    N                uint32
}

type GroundPlaneTile struct {
    Normal   [3]float32  // Plane normal (float32)
    Offset   float32     // Plane offset d
    Centroid [3]float32  // Tile centre
    Confidence float32   // Planarity score
}
```

---

### 8.4 Eigenvalue Decomposition Conditioning

**Condition number of covariance matrix:**
```
cond(C) = λ_max / λ_min
```

For well-distributed points on a plane: `λ₁ ≈ λ₂ >> λ₃`, so `cond(C) ≈ λ₁ / λ₃ ≈ 10³-10⁶`.

**Ill-conditioned cases:**
1. **Collinear points** (line, not plane): `λ₂ ≈ λ₃ ≈ 0`, plane normal is undefined
2. **Single point repeated:** `λ₁ = λ₂ = λ₃ = 0`, covariance is singular
3. **Outlier dominance:** Single far outlier inflates λ₁, skewing plane fit

**Detection:**
```
if λ₃ / λ₁ > 0.1:  // Weak planarity
    confidence = 0.0
    reject tile
    
if N < N_min:  // Insufficient points
    reject tile
```

**Robust plane fitting:**  
For outlier rejection, use RANSAC in post-processing (offline PCAP analysis) or percentile filtering (discard points > 3σ from median).

---

## 9. Recommendations & Tradeoffs Summary

### 9.1 Algorithm Selection Matrix

| Use Case | Algorithm | Rationale |
|----------|-----------|-----------|
| **Real-time streaming (production)** | Incremental Covariance + PCA | O(1) per-point, minimal memory, numerically stable |
| **Offline PCAP analysis** | RANSAC per tile | Robust to outliers, high accuracy, not time-critical |
| **Resource-constrained** | Voxel Height Map | Minimal computation/storage, flat terrain only |
| **High-accuracy mapping** | Weighted Least Squares | Distance-weighted, handles density variation |

---

### 9.2 Configuration Recommendations

**For velocity.report (stationary traffic monitoring):**

| Parameter | Recommended Value | Rationale |
|-----------|-------------------|-----------|
| **Tile size** | 1.0m × 1.0m | Balances resolution vs point density to 40m range |
| **Coverage area** | 50m × 50m centred on sensor | Covers typical street width + sidewalks |
| **Minimum points** | 30 per tile | Ensures stable eigenvalues (3σ threshold) |
| **Settlement time** | 10 revolutions (1 sec) | Confident planes to 50m range |
| **Storage budget** | 320 KB raw, 55 KB compressed | 2,500 tiles for 50m × 50m at 1m resolution |
| **Precision** | Float64 accumulation, float32 output | Numerical stability with compact storage |
| **Curvature threshold** | 5° (8.7% grade) | Detects curbs, driveways, speed humps |
| **Update frequency** | Per revolution (10 Hz) | Continuous refinement, 0.1s latency |

---

### 9.3 Performance Targets

**Raspberry Pi 4 benchmarks:**

| Stage | Target Time | Expected Load | Notes |
|-------|-------------|---------------|-------|
| Point classification | 2-4 ms | 2-4% per frame | Tile index lookup, hash table access |
| Covariance update | 4-6 ms | 4-6% per frame | 9 FLOPs per point, cache-friendly |
| Plane fitting | < 0.1 ms | < 0.1% per frame | Only on settled tiles (~100/frame) |
| Curvature estimation | < 0.1 ms | < 0.1% per frame | 4 neighbours × dot product |
| **Total** | **< 10 ms** | **< 10% per frame** | 90ms headroom for other processing |

---

### 9.4 Tradeoff Summary Table

| Dimension | Coarse (2m tiles) | **Recommended (1m tiles)** | Fine (0.5m tiles) |
|-----------|-------------------|----------------------------|-------------------|
| **Storage** | 280 KB (100m × 100m) | 1.1 MB (100m × 100m) | 4.5 MB (100m × 100m) |
| **Confident range** | 60m (single rev) | 35m (single rev) | 20m (single rev) |
| **Settlement time** | 0.1s (< 40m) | 0.1-0.5s (< 40m) | 0.3-1.0s (< 30m) |
| **Spatial resolution** | Misses narrow features | Detects curbs, driveways | High detail, sparse data |
| **Computation** | 6-8 ms/frame | 8-10 ms/frame | 12-15 ms/frame |
| **Use case** | Extended range (rural) | **Urban streets** | Parking lots, warehouses |

---

### 9.5 Future Enhancements

**Phase 1 (MVP):**  
- Incremental covariance + PCA with 1m tiles, 50m × 50m coverage
- Float64 accumulation, float32 output
- Simple height-band pre-filter (z ∈ [-2.8m, +1.5m])
- Settlement: 10 revolutions minimum

**Phase 2 (Robustness):**  
- Outlier rejection: Discard points > 3σ from tile median Z
- Adaptive settlement: Variable revolution count based on point density
- Curvature-based classification: Flat/gentle/moderate/steep

**Phase 3 (Advanced):**  
- RANSAC refinement for high-curvature tiles (offline post-processing)
- Temporal stability tracking: Flag tiles with changing ground (construction, snow)
- Multi-scale tiles: 1m for < 40m, 2m for 40-80m range

**Phase 4 (Mapping):**  
- Global ground map accumulation over hours/days
- Localisation: Match current ground plane to historical map
- Change detection: Identify new obstacles or road surface degradation

---

## References

### Codebase
- `internal/lidar/background/grid.go` – BackgroundGrid implementation (polar coordinates)
- `internal/lidar/background/cell.go` – BackgroundCell structure (42 bytes)
- `internal/lidar/processor/height_band.go` – Existing HeightBandFilter (simple Z-band)
- `docs/lidar/architecture/ground-plane-extraction.md` – Ground plane specification
- `cmd/replay-server/` – PCAP replay infrastructure (80K packets, 28.7M points)

### Literature
- Rusu et al., "Fast Point Feature Histograms (FPFH) for 3D Registration" (2009) – PCA-based surface normal estimation
- Zermas et al., "Fast Segmentation of 3D Point Clouds for Ground Vehicles" (2017) – Real-time ground plane extraction
- Moosmann et al., "Velodyne SLAM" (2011) – Incremental ground surface mapping for autonomous vehicles
- Hesai Pandar40P Datasheet – Sensor specifications (40 channels, 10 Hz, 0.2° azimuth resolution)

### Standards
- IEEE 1873-2015 – Robot Map Data Representation (grid maps, elevation maps)
- ISO 8855:2011 – Road vehicles (coordinate systems, grade definitions)

---

**Document Maintenance:**  
This document should be updated when:
- Benchmark results from Raspberry Pi 4 deviate significantly from estimates
- PCAP analysis reveals different point density patterns
- Ground plane implementation reveals numerical stability issues not covered here
- Performance requirements change (e.g., higher rotation rate, extended range)

**Next Steps:**  
1. Implement incremental covariance accumulator (Go)
2. Benchmark covariance update loop on Pi 4 (validate 4-6ms estimate)
3. Test eigendecomposition library (gonum/mat) for 3×3 matrices (validate ~100 FLOPs)
4. Profile PCAP replay with ground plane extraction (validate 10ms total budget)
5. Document results in companion implementation guide
