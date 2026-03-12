# Layer Dependency Hygiene: Move Sensor-Frame Types to L2

- **Status:** ✅ Implemented
- **Created:** 2026-03-08
- **Layers:** L1 Packets, L2 Frames, L3 Grid, L4 Perception
- **Canonical architecture:** [lidar-data-layer-model.md](../lidar/architecture/lidar-data-layer-model.md)

The layer model declares strict forward-only dependencies (L1→L2→L3→…, never upward). An audit of real imports reveals several violations where lower layers import types from L4.

## Known violations

| Violation                                          | Files affected                                                                 | Import path                                                            | Severity                                       |
| -------------------------------------------------- | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------- | ---------------------------------------------- |
| **L1→L4** for `PointPolar`                         | `l1packets/network/listener.go`, `foreground_forwarder.go`, `parse/extract.go` | Parser and FrameBuilder interfaces reference `l4perception.PointPolar` | Critical — L1 depends on L4, skipping 3 layers |
| **L1→L3** for `BackgroundManager`                  | `l1packets/network/pcap_realtime.go`, stub files                               | PCAP replay callback signatures reference `l3grid.BackgroundManager`   | Critical — L1 depends on L3, skipping 2 layers |
| **L2→L4** for `PointPolar`, `Point`                | `l2frames/geometry.go`                                                         | Type aliases + function re-exports from `l4perception`                 | High — L2 depends on L4                        |
| **L3→L4** for `PointPolar`, `SphericalToCartesian` | `l3grid/types.go`, `background_export.go`, `background_drift.go`               | Type alias and function calls into `l4perception`                      | High — L3 depends on L4                        |

## Root cause

Four items currently defined in `l4perception/types.go` are sensor-frame primitives that belong at L2:

| Item                     | Current home | Natural home  | Rationale                                                                                                                                                                       |
| ------------------------ | ------------ | ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PointPolar` struct      | L4           | **L2 Frames** | Polar-coordinate return from a single laser — the unit of frame assembly. Fields are all sensor-frame: `Channel`, `Azimuth`, `Elevation`, `Distance`, `BlockID`, `UDPSequence`. |
| `Point` struct           | L4           | **L2 Frames** | Sensor-frame Cartesian point — output of L2's spherical→Cartesian transform.                                                                                                    |
| `SphericalToCartesian()` | L4           | **L2 Frames** | Pure geometry: `(distance, azimuth, elevation) → (x, y, z)`. Already re-exported by L2.                                                                                         |
| `ApplyPose()`            | L4           | **L2 Frames** | Pure maths: 4×4 homogeneous transform. Coordinate geometry is L2's job.                                                                                                         |

These are all pure data types or stateless functions with zero imports from L3+.

## What stays in L4

| Item                  | Why it stays                                                                                       |
| --------------------- | -------------------------------------------------------------------------------------------------- |
| `WorldPoint`          | World-frame coordinates with `SensorID` and `time.Time` — site-level abstraction, not sensor-frame |
| `WorldCluster`        | Detection output: centroid, dimensions, OBB — core perception output                               |
| `OrientedBoundingBox` | PCA-fitted 7-DOF box — perception-level geometry                                                   |
| `TrackObservation`    | Bridges L4→L5 — requires cluster context                                                           |

## Migration plan

1. Move `PointPolar`, `Point`, `SphericalToCartesian`, `ApplyPose` definitions from `l4perception/types.go` to `l2frames/types.go`
2. Add backward-compatible type aliases in L4: `type PointPolar = l2frames.PointPolar` (keeps existing L4 callers compiling)
3. Update L3 alias source: `l3grid/types.go` → import from `l2frames` (already legal)
4. Update L1 imports: `l1packets/network/` → import from `l2frames` (makes L1→L2 the only layer dependency)
5. Update parent `aliases.go` to reference `l2frames`
6. Update pipeline, adapters, monitor, visualiser imports

## After migration

Dependency graph becomes:

```
L1 → L2  (Parser returns l2frames.PointPolar — "I produce frame-level data from raw bytes")
L2       owns PointPolar, Point, SphericalToCartesian, ApplyPose
L3 → L2  (type alias, function calls — already permitted)
L4 → L2  (imports sensor-frame types it transforms into world-frame — permitted)
```

**Note on L1→L3 callback:** `l1packets/network/pcap_realtime.go` references `l3grid.BackgroundManager` via PCAP replay callbacks. This is a separate violation (L1→L3 skip) that requires interface extraction or callback inversion — tracked separately from the type-ownership migration above.

## Estimated scope

~15 production files and ~6 test files affected across `l1packets/`, `l2frames/`, `l3grid/`, `l4perception/`, `pipeline/`, `adapters/`, `monitor/`, and `visualiser/`.
