# LiDAR Foundations Fix-It

Owner: LiDAR pipeline maintainers
Status: In progress
Purpose/Summary: 20260221-lidar-foundations-fixit.

## Goal

Stabilize documentation and implementation boundaries so downstream work depends on explicit, accurate foundations.

## Completed in This Pass

1. Region-adaptive parameter parity fixed on the production path:
   - `internal/lidar/l3grid/foreground.go`
   - `ProcessFramePolarWithMask` now applies per-region noise/neighbor/alpha overrides.
2. Runtime tuning parity improvement:
   - `internal/lidar/monitor/webserver.go`
   - `/api/lidar/params` POST now supports `max_tracks`.
3. Tests added/updated:
   - `internal/lidar/l3grid/foreground_test.go`
   - `internal/lidar/monitor/webserver_test.go`
4. Validation:
   - `go test ./internal/lidar/...` passed.

## Remaining Gaps (Write-Up + Follow-Up)

### High

1. Velocity-coherent algorithm remains proposal-only; runtime selector is not active on `main`.
2. Dynamic algorithm selection docs contain branch-history details that can be misread as implemented behavior.

### Medium

1. `/api/lidar/params` still does not fully mirror canonical tuning schema for non-tracker runtime keys (frame/pipeline/ground/clustering shape controls).
2. Ground-plane maths includes planned model details while runtime ground removal remains height-band filtering.
3. Legacy link/path drift remains in some older LiDAR docs.

### Low

1. Several non-foundational placeholders and partial tooling paths are still present (export/visualiser ancillary flows).

## Foundation Policy (Effective Now)

1. Mark every algorithm doc section as one of: `Implemented`, `Planned`, `Deprecated`.
2. Keep vector-grid and velocity-coherent workstreams separated by a mask-level contract.
3. Only claim “implemented” when code path is active in `internal/lidar/pipeline/tracking_pipeline.go`.
