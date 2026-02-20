# Frontend Background Debug Surfaces Plan

**Status:** Planning-only (no implementation in this branch)
**Scope:** Swift visualiser debugging outputs for background settlement
**Related:** [`docs/lidar/visualiser/02-api-contracts.md`](../lidar/visualiser/02-api-contracts.md), [`docs/lidar/visualiser/03-architecture.md`](../lidar/visualiser/03-architecture.md), [`docs/lidar/visualiser/04-implementation-plan.md`](../lidar/visualiser/04-implementation-plan.md), [`config/README.maths.md`](../../config/README.maths.md)

## 1. Problem

Current visualiser debugging focuses on tracks/clusters. Background settling is
hard to diagnose because polar-cell state, Cartesian projection, and region
assignment are not exposed as first-class debug views.

## 2. Planned Outputs

1. **Polar background debug points**
   - ring, azimuth bin, representative range, spread, confidence, settle state
2. **Cartesian background debug points**
   - x/y/z, confidence, source cell `(ring, azimuth_bin)`, optional region ID
3. **Region map**
   - per-cell region assignment and per-region lifecycle/state

## 3. Why Three Views

1. Polar view validates L3 math directly (cell-level thresholds and confidence).
2. Cartesian view validates geometric transforms and renderer alignment.
3. Region map validates L3/L4 merge logic and boundary-aware assignment.

## 4. Data Contract Direction (Planned)

Use a debug bundle extension in gRPC stream messages with independent include
flags for each view. Keep these flags orthogonal to existing point/cluster/track
toggles.

## 5. UI Direction (Planned)

1. Add mode selector: `off | polar | cartesian | region-map`.
2. Add inspector panel for selected point/cell:
   - source cell, confidence, settle state, region ID.
3. Add region legend:
   - region ID, class, lifecycle phase.

## 6. Non-Goals

1. No algorithmic behavior changes to tracking/clustering.
2. No new tuning keys in this phase.
3. No persistence format changes required for MVP debug mode.

## 7. Validation Criteria

1. Debug mode off has no measurable behavior change in output pipeline.
2. Every rendered Cartesian debug point maps to a valid polar source cell.
3. Region-map transitions are auditable frame-to-frame.

## 8. Canonical Mapping Source

Config/math coupling for this plan is canonical in:

- [`config/README.maths.md`](../../config/README.maths.md)

Downstream mapping docs must sync from that file via:

- `make readme-maths-check`
