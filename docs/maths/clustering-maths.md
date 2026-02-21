# Clustering Maths

Status: Active
Purpose/Summary: clustering-maths.

**Status:** Implementation-aligned math note
**Layer:** L4 Perception (`internal/lidar/l4perception`)
**Related:** [Tracking Maths](tracking-maths.md), [Ground Plane Maths](ground-plane-maths.md)

## 1. Purpose

Clustering converts foreground points into object candidates with stable geometry for tracking.

Pipeline math components:

1. coordinate transform,
2. optional voxel downsampling,
3. DBSCAN neighborhood clustering,
4. cluster feature extraction (medoid + OBB/PCA).

## 2. Coordinate Transform

For polar point `(r, az, el)`:

- `x = r cos(el) sin(az)`
- `y = r cos(el) cos(az)`
- `z = r sin(el)`

World transform with row-major homogeneous matrix `T`:

- `wx = T00*x + T01*y + T02*z + T03`
- `wy = T10*x + T11*y + T12*z + T13`
- `wz = T20*x + T21*y + T22*z + T23`

## 3. Optional Voxel Downsampling

Voxel key for leaf size `l`:

- `ix = floor(x/l)`, `iy = floor(y/l)`, `iz = floor(z/l)`.

For each occupied voxel:

1. compute centroid of contained points,
2. retain original point closest to centroid.

This preserves observed geometry better than stride decimation.

## 4. DBSCAN Formulation

### 4.1 Neighborhood

Distance is 2D Euclidean in `(x,y)`:

`d2(i,j) = (x_i-x_j)^2 + (y_i-y_j)^2`

`j` is a neighbor if `d2(i,j) <= eps^2`.

### 4.2 Core rule

Point `i` is core if `|N_eps(i)| >= MinPts`.

Clusters are connected components grown from core points; noise points receive label `-1`.

### 4.3 Spatial index acceleration

A uniform grid with cell size near `eps` stores point indices.

Neighborhood query checks only 3x3 adjacent cells around the seed cell, giving practical near-linear behaviour in typical scenes.

## 5. Cluster-Level Geometry

## 5.1 Centroid as medoid

Implementation uses medoid-like centroid:

1. compute arithmetic mean `m`,
2. choose real point minimizing `||p_i - m||^2`.

This avoids non-physical centroids for non-convex point sets.

### 5.2 OBB via 2D PCA

For cluster points in XY:

1. compute covariance `C = [[c00, c01], [c01, c11]]`,
2. solve eigenpairs analytically,
3. principal eigenvector gives heading,
4. project points on principal/perpendicular axes to get `length/width`,
5. use Z-extents for height.

Heading smoothing for downstream tracking uses wrap-aware EMA (documented in tracking math).

### 5.3 Quality filters

Clusters are rejected if:

- longest OBB axis outside `[MinClusterDiameter, MaxClusterDiameter]`,
- aspect ratio exceeds `MaxClusterAspectRatio` (with thin-object guard when shortest axis is near noise floor).

## 6. Complexity

For `N` points:

- grid build: `O(N)`,
- DBSCAN expansion: near `O(N)` average with local neighborhoods; worst-case higher in dense degenerate scenes,
- cluster metrics: linear in points per cluster.

Memory is linear in `N`.

## 7. Assumptions and Limits

1. **2D density criterion (XY only)**
   - Works for road users; may merge vertically separated but planarly overlapping returns.
2. **Fixed `eps` and `MinPts` per run**
   - Scene/range-dependent optimal values vary.
3. **PCA OBB for shape**
   - Stable for elongated objects, ambiguous for near-square clusters.
4. **Foreground input quality dependency**
   - L3 misclassification directly affects cluster purity and fragmentation.
5. **Density-based model sensitivity**
   - Sparse long-range points and occlusion edges can be labeled noise.

## 8. Interface to Tracking

Tracking consumes:

- cluster medoid position (measurement),
- OBB dimensions/heading (shape/heading diagnostics),
- counts/intensity/height metrics.

Association quality is therefore jointly constrained by clustering noise and tracker gating math.
