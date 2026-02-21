# `arena.go` Deprecation and Layered Type Layout Design (2026-02-17)

Status: Active

## Objective

Deprecate `internal/lidar/arena.go` and replace it with layer-aligned type files that match `docs/lidar/architecture/lidar-data-layer-model.md`.

This design keeps runtime behavior unchanged while making ownership and intent of shared models much clearer.

## Why this is needed

`arena.go` currently mixes:

- Live shared domain types used across runtime paths
- Legacy sidecar/container types not used by runtime
- Historical roadmap comments (including "Phase" markers) that no longer describe current code behavior

This reduces readability and makes model ownership ambiguous.

## Current Symbol Audit

Counts below are usages in `internal/lidar` excluding `arena.go` and arena-only tests.

| Symbol                        | Usage count | Action                                          | Target layer                      |
| ----------------------------- | ----------: | ----------------------------------------------- | --------------------------------- |
| `Point`                       |         156 | Keep                                            | L2 Frames                         |
| `FrameID`                     |         261 | Keep                                            | L2 Frames                         |
| `Pose`                        |           8 | Keep                                            | L2 Frames / L4 transform boundary |
| `BgSnapshot`                  |          35 | Keep                                            | L3 Grid                           |
| `RegionSnapshot`              |          34 | Keep                                            | L3 Grid                           |
| `RegionData`                  |           3 | Keep                                            | L3 Grid                           |
| `WorldCluster`                |         160 | Keep                                            | L4 Perception                     |
| `PoseCache`                   |           0 | Remove (or move only if future use is concrete) | N/A                               |
| `RingBuffer`                  |           0 | Remove                                          | N/A                               |
| `TrackState2D`                |           0 | Remove                                          | N/A                               |
| `Track` (arena variant)       |           0 | Remove                                          | N/A                               |
| `TrackObs`                    |           0 | Remove                                          | N/A                               |
| `TrackSummary`                |           0 | Remove                                          | N/A                               |
| `SidecarState`                |           0 | Remove                                          | N/A                               |
| `SystemEvent` (arena variant) |           0 | Remove                                          | N/A                               |
| `RetentionConfig`             |           0 | Remove                                          | N/A                               |
| `Event` + helper constructors |           0 | Remove                                          | N/A                               |

## Target File Layout (Same package, clear layer ownership)

Keep package name `lidar` for low-risk migration, but split model files by layer.

- `internal/lidar/l2_types_point.go`
- `internal/lidar/l2_types_pose.go`
- `internal/lidar/l3_types_background_snapshots.go`
- `internal/lidar/l4_types_world_cluster.go`

Notes:

- These files contain only active shared types.
- Removed legacy types are not relocated.
- No import-path churn is introduced in this step.

## Design Rules

1. One file should map to one layer concern whenever practical.
2. Keep type definitions byte-for-byte compatible for active persisted fields (JSON tags and field names).
3. Do not move tracking runtime state from `tracking.go` into shared model files.
4. No "Phase X" wording in runtime code comments; capability-based wording only.

## Migration Plan

1. Create the four layer-aligned type files and copy active type definitions unchanged.
2. Compile and run LiDAR package tests to confirm no behavior drift.
3. Delete `internal/lidar/arena.go`.
4. Delete arena-only tests tied to removed dead types (`arena_test.go`, `arena_extended_test.go`).
5. Add a guard check in CI/docs linting to prevent re-introducing `arena.go`-style mixed files.

## Compatibility and Risk

- **Runtime risk:** low, if active structs are copied exactly.
- **Persistence risk:** low, if DB-facing field names/types remain unchanged.
- **Testing risk:** low, but ensure removed tests only covered removed dead types.

Primary regression risk is accidental field drift while copying structs. This is mitigated by exact-copy migration plus compile/tests.

## Verification Checklist

- `go test ./internal/lidar/...`
- `rg -n "type (RingBuffer|SidecarState|TrackState2D|TrackObs|TrackSummary|RetentionConfig)" internal/lidar`
  - Expected: no runtime definitions unless intentionally reintroduced
- `rg -n "Phase [0-9]" internal/lidar`
  - Expected: no new phase-roadmap comments in runtime code

## Acceptance Criteria

1. `arena.go` no longer exists.
2. Active shared types are grouped by L2/L3/L4 aligned files.
3. Removed symbols have no runtime references.
4. LiDAR tests pass with unchanged behavior.
5. Documentation references to `arena.go` are updated to new file ownership.

## Completion Status (2026-02-17)

**All acceptance criteria met. This work is complete.**

### What was done:

1. **arena.go deleted** — all legacy types removed (RingBuffer, SidecarState, Track, TrackObs, TrackState2D, TrackSummary, RetentionConfig, SystemEvent, Event, PoseCache, FrameStats + constructors).
2. **arena_test.go and arena_extended_test.go deleted** — only tested removed legacy types.
3. **Active types migrated to layer packages** (not flat files as originally planned — went further):
   - `Point`, `PointPolar` → `l4perception/types.go`
   - `FrameID` → `l3grid/types.go` and `l4perception/types.go`
   - `Pose` → `l4perception/types.go`
   - `BgSnapshot`, `RegionSnapshot`, `RegionData` → `l3grid/types.go`
   - `WorldCluster`, `OrientedBoundingBox` → `l4perception/types.go`
4. **All callers updated** — sub-packages (l1packets, monitor, visualiser, sweep) and external callers (cmd/radar, internal/db) import from layer packages directly.
5. **CI guardrail active** — grep for "Phase [0-9]" in go-ci.yml lint job.
6. **Follow-on completed** — layer files already in subpackages (`l2frames`, `l3grid`, `l4perception`, `l5tracks`, `l6objects`).
