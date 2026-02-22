# Ground Plane Maths

**Status:** Implementation-aligned math note
**Layer:** L4 Perception
**Related:** [Background Grid Settling Maths](background-grid-settling-maths.md), [Clustering Maths](clustering-maths.md), [`docs/lidar/architecture/ground-plane-extraction.md`](../lidar/architecture/ground-plane-extraction.md), [Ground Plane and Vector-Scene Proposal Maths](proposals/20260221-ground-plane-vector-scene-maths.md)

## Scope

This document covers the **implemented** runtime ground-removal math used by the current pipeline.

Current runtime path:

- `internal/lidar/pipeline/tracking_pipeline.go`
- `internal/lidar/l4perception/ground.go`

## Implemented Model

The production pipeline uses a vertical height-band filter in world/sensor frame coordinates:

- keep point `p=(x,y,z)` iff `z_floor <= z <= z_ceiling`
- reject points below floor (ground-plane returns and low artifacts)
- reject points above ceiling (overhead returns)

Default behavior is provided by `DefaultHeightBandFilter()`; overrides are wired through pipeline config (`HeightBandFloor`, `HeightBandCeiling`, `RemoveGround`).

## Runtime Characteristics

1. O(N) per frame with constant-time point checks.
2. Deterministic and low-latency.
3. No plane-fit state, no temporal estimator, no tile settlement.

## Limits (Known)

1. Height-band filtering is not slope-aware.
2. It does not model local plane orientation or curvature.
3. It cannot express region priors (lanes, kerbs, structures).

## Proposal Boundary

Advanced tile-plane/vector-scene maths (region selection, confidence settlement, curvature and global-surface priors) is separated into proposal material:

- [proposals/20260221-ground-plane-vector-scene-maths.md](proposals/20260221-ground-plane-vector-scene-maths.md)

That proposal is not active in current runtime.
