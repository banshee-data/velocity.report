# LiDAR Background Grid — Standards Comparison

## Context

- Current implementation stores background as a **polar range image** (`BackgroundGrid` in `internal/lidar/background.go`): rings × azimuth bins with per-cell average range, spread, last-updated timestamp, freeze window, and acceptance counters. Snapshots persist to `lidar_bg_snapshot` with compressed `[]BackgroundCell` blobs (`internal/lidar/docs/schema.sql`).
- The grid is tuned for single-sensor, streaming foreground/background separation with EMA updates, neighbor confirmation, and runtime-adjustable thresholds.
- Question: should we align the background geometry with external standards (e.g., SLAM/LidarView ecosystems), and what are the tradeoffs?

## External Representations

| Standard | Geometry form | Notes | Pros | Cons |
| --- | --- | --- | --- | --- |
| ROS **OccupancyGrid** / **OctoMap** | 2D grid or octree occupancy | Canonical in robotics stacks; integrates with planners and Nav2 costmaps. | Broad ecosystem support; native to ROS; well-known semantics. | Binary/probabilistic occupancy only; loses per-cell range variance; memory grows with resolution; OctoMap slower for frequent per-frame updates. |
| **TSDF** / **VoxelGrid** (OpenVDB, Voxblox) | 3D signed-distance voxels | Used in dense SLAM; adaptive resolution via hashed or tree voxels. | Captures surfaces smoothly; good for multi-sensor fusion and map reuse; sparse storage with VDB. | Higher CPU/RAM; needs accurate poses; heavier than our per-frame subtraction path. |
| **VTK/ParaView** grids (LidarView) | `vtkImageData` / `vtkStructuredGrid` or `vtkPolyData` | LidarView/ParaView favor VTK XML (`.vti/.vts/.vtp`) for interchange and visualization. | Standardized I/O; strong tooling (ParaView, VTK.js); preserves attributes. | Format standardizes containers, not background semantics; still requires our own field schema. |
| **Point cloud files** (PCD, LAS, PLY) | Unstructured points | Supported by LidarView exports. | Ubiquitous viewers and libs; easy interchange. | No native notion of background grid; needs custom attributes to mark background cells. |
| Kitware **SLAM** outputs | Aggregated point clouds + pose graphs | Library is pose/SLAM-centric; mapping can be fed into occupancy/TSDF backends. | Compatible with common map backends; supports loop-closure maps. | Still requires a chosen map representation for background; not a drop-in replacement for our grid. |

## VTK export shape (LidarView/ParaView/CloudCompare)

- **vtkImageData (fast path):** Treat the polar grid as a 2D image (x = azimuth bin, y = ring, z = 1). Cell data arrays: `avg_range_m`, `range_spread_m`, `times_seen`, `frozen_until_ns`. Spacing: `(360/azBins deg, 1 ring, 1)`; origin at `(0, 0, 0)`. Suited for heatmaps and quick inspection.
- **vtkStructuredGrid (geometry-aware):** Convert each cell to 3D Cartesian using mean range + azimuth + optional ring elevation; attach the same cell-data arrays. Better for spatial overlays and merging with other clouds.
- **Export channels:** Extend `exportFrameToASC` to optionally emit VTK XML (`.vti`/`.vts`) alongside ASC so LidarView/ParaView/CloudCompare can ingest without changing runtime storage. Keep runtime grid unchanged.
- **Live tap (optional):** Serve a lightweight HTTP/WS stream of periodic VTK XML frames (or VTP polydata derived from foreground points) so external viewers can subscribe for live introspection. This is additive and avoids touching the hot path.

## Fit Analysis vs Current Grid

- **Latency & simplicity:** Our polar grid updates in-place per frame with O(rings × azBins) memory and no pose dependence. Occupancy/TSDF require pose fusion and 3D neighborhoods; costlier for Raspberry Pi and unnecessary for single-sensor foreground masking.
- **Semantics:** Background cells carry **mean + spread + recency + freeze**—richer than binary occupancy but specialized for background subtraction. Standard grids would discard spread/freeze unless extended with custom fields.
- **Interchange/Tooling:** VTK (`.vti/.vts`) or PCD/LAS exports would let LidarView/ParaView users inspect the grid without changing the internal model. OctoMap/TSDF would ease integration with robotics stacks but add dependencies and pose requirements.
- **Multi-sensor future:** If we add cross-sensor fusion or SLAM, TSDF/OctoMap provide a shared 3D world volume; our polar grid can still be the sensor-local layer feeding a fusion back-end.
- **Storage:** Current compressed `[]BackgroundCell` blob is compact and schema-aligned. VDB/OctoMap introduce larger persistence formats and new tooling.

## Recommendation

1. **Keep the polar background grid as the runtime model** for foreground masking—lowest latency and already tuned to our classifier.
2. **Add an export shim** that converts the grid to a standard container for tooling, without changing the core model:
   - For visualization: emit **VTK `vtkImageData` or `vtkStructuredGrid`** (cell fields: mean range, spread, times seen, frozen-until) and optionally **PCD** with per-cell attributes for LidarView/ParaView inspection.
   - For robotics/SLAM interop: optionally downsample to **2.5D occupancy grid** (occupied if cell deviates from background) for ROS users; treat as offline/analysis path so it doesn’t affect hot loop.
   - **Update `ExportBackgroundGridToASC` workflow** to optionally emit LidarView-friendly exports alongside ASC for debugging/introspection (no change to runtime grid storage).
3. **Defer full TSDF/OctoMap adoption** until we pursue multi-sensor fusion; revisit when pose graphs and world-frame transforms are available.

This keeps our background pipeline stable while enabling standards-based exports for external tools and future fusion work.
