# Vector-Grid vs Velocity Algorithm Workstreams

Status: Active separation plan (foundation guidance)

## Purpose

Separate foundational LiDAR work into two independent streams so the project can evolve motion algorithms without destabilizing static-scene reliability.

## Current Runtime Baseline (Implemented)

1. Foreground extraction uses L3 mask path:
   - `internal/lidar/l3grid/foreground.go` (`ProcessFramePolarWithMask`)
2. Perception and clustering:
   - `internal/lidar/l4perception/cluster.go`
3. Tracking and assignment:
   - `internal/lidar/l5tracks/tracking.go`

## Workstream A: Vector-Grid Foundations

Scope:

- L3 grid confidence, freeze/lock dynamics, region adaptation
- L4 ground/geometry confidence for static scene structure
- Future vector-scene priors and region-aware settlement

Code ownership:

- `internal/lidar/l3grid/*`
- `internal/lidar/l4perception/ground.go` (current height-band path)
- future ground/vector modules

Current status:

- Implemented core L3/L4/L5 pipeline is production active.
- Region-adaptive params now apply in the production mask path (`foreground.go`) as of 2026-02-21.

## Workstream B: Velocity-Coherent Algorithm

Scope:

- Point correspondence and velocity confidence
- Velocity-aware foreground extraction and sparse continuity
- Hybrid/side-by-side evaluation against baseline extractor

Code ownership:

- Future extractor modules (recommended under `internal/lidar/extractors/`)
- Optional pipeline selector/orchestration layer

Current status:

- Planning only. No velocity-coherent extractor is active in `main` runtime.
- Design references:
  - `docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md`
  - `docs/maths/proposals/20260220-velocity-coherent-foreground-extraction.md`

## Required Boundary Between A and B

Keep one narrow contract:

- Input: frame points + timestamp
- Output: foreground mask (`[]bool`) + extractor metrics

Rules:

1. Workstream A must not depend on velocity internals.
2. Workstream B must not reach into region-manager internals.
3. Pipeline stages after foreground mask (transform, clustering, tracking, storage, UI) remain unchanged.

## Dependency Policy

1. Treat vector-grid docs as foundation references.
2. Treat velocity docs as proposal/plan until production wiring and tests land.
3. Require side-by-side replay validation before any default mode change.
