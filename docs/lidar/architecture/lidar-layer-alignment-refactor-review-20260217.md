# LiDAR Layer Alignment and Readability Review (2026-02-17)

## Goal

Make the codebase more logical and readable by aligning implementation with the six-layer model in `docs/lidar/architecture/lidar-data-layer-model.md`.

This review focuses on:

- Layer boundaries (L1-L6)
- Readability and ownership clarity
- Removing roadmap-phase comments from production code
- Simplifying HTTP route registration/dispatch (especially `mux.HandleFunc` usage)

## Baseline Evidence

### Layer model exists, but orchestration bypasses boundaries

- The model defines clean L1-L6 boundaries in `docs/lidar/architecture/lidar-data-layer-model.md:9`.
- The runtime callback currently crosses many layers in one function:
  - L3 foreground extraction in `internal/lidar/tracking_pipeline.go:156`
  - L4 transform/clustering in `internal/lidar/tracking_pipeline.go:246` and `internal/lidar/tracking_pipeline.go:274`
  - L5 track update in `internal/lidar/tracking_pipeline.go:311`
  - L6 classify + persistence + publish in `internal/lidar/tracking_pipeline.go:318` and `internal/lidar/tracking_pipeline.go:384`
- `TrackingPipelineConfig` currently pulls in DB, run manager, and visualiser concerns directly (`internal/lidar/tracking_pipeline.go:47`).

### Route registration and dispatch are hard to read and evolve

- `RegisterRoutes` contains 70+ explicit `mux.HandleFunc` calls (`internal/lidar/monitor/webserver.go:1190`).
- Run-track and scene APIs manually parse path strings and method-switch internally:
  - `internal/lidar/monitor/run_track_api.go:25`
  - `internal/lidar/monitor/scene_api.go:45`

### Hidden runtime dependencies via global registries

- `FrameBuilder` registry: `internal/lidar/frame_builder.go:10`
- `BackgroundManager` registry: `internal/lidar/background.go:1260`
- `AnalysisRunManager` registry: `internal/lidar/analysis_run_manager.go:28`

### Roadmap-phase comments have leaked into runtime code

- Pipeline: `internal/lidar/tracking_pipeline.go:156`
- Web server fields/routes: `internal/lidar/monitor/webserver.go:149` and `internal/lidar/monitor/webserver.go:1271`
- API dispatch/placeholder text: `internal/lidar/monitor/run_track_api.go:62` and `internal/lidar/monitor/run_track_api.go:494`
- Scene API placeholders: `internal/lidar/monitor/scene_api.go:16` and `internal/lidar/monitor/scene_api.go:364`
- Frontend UI logic: `web/src/routes/lidar/tracks/+page.svelte:55` and `web/src/lib/components/lidar/TrackList.svelte:21`

### Large files reduce local comprehensibility

- `internal/lidar/monitor/webserver.go` ~3909 lines
- `web/src/lib/components/lidar/TrackList.svelte` ~1013 lines
- `web/src/lib/components/lidar/MapPane.svelte` ~883 lines
- `web/src/routes/lidar/tracks/+page.svelte` ~786 lines

## Target Structure Aligned to L1-L6

Use layer-first package ownership inside `internal/lidar`:

| Layer         | Current anchors                                                | Proposed package ownership                                         |
| ------------- | -------------------------------------------------------------- | ------------------------------------------------------------------ |
| L1 Packets    | `internal/lidar/network/*`, `internal/lidar/parse/*`           | `internal/lidar/l1packets/{ingest,pcap,parse}`                     |
| L2 Frames     | `internal/lidar/frame_builder.go`, parts of `transform.go`     | `internal/lidar/l2frames/{framebuilder,geometry,export}`           |
| L3 Grid       | `internal/lidar/background.go`, `internal/lidar/foreground.go` | `internal/lidar/l3grid/{background,foreground,regions}`            |
| L4 Perception | `internal/lidar/clustering.go`, `ground.go`, `voxel.go`        | `internal/lidar/l4perception/{transform,cluster,ground,voxel,obs}` |
| L5 Tracks     | `internal/lidar/tracking.go`, `hungarian.go`                   | `internal/lidar/l5tracks/{tracker,association,lifecycle}`          |
| L6 Objects    | `internal/lidar/classification.go`, `quality.go`               | `internal/lidar/l6objects/{classification,quality,taxonomy}`       |

Cross-cutting packages:

- `internal/lidar/pipeline`: orchestration only (realtime + replay use cases)
- `internal/lidar/storage/sqlite`: DB repositories/adapters
- `internal/lidar/adapters/{http,grpc,udp}`: transport and IO boundaries

## Dependency Rules (to keep layers clean)

1. `L(n)` may depend on `L(n-1)` and below, but never upward.
2. SQL/database code is not allowed in L3-L6 domain packages.
3. HTTP/gRPC/UDP handlers do not parse business path state manually; they delegate to use-case services.
4. `pipeline` orchestrates layers and adapters, but does not own domain logic.

## Refactor Opportunities (Concrete)

### Task-specific follow-on design docs

- `docs/lidar/architecture/arena-go-deprecation-and-layered-type-layout-design-20260217.md`
  - Deprecates `internal/lidar/arena.go` and relocates active shared models by L2/L3/L4 ownership.
- `docs/lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md`
  - Splits LiDAR logging into `ops`/`debug`/`trace` streams and defines routing rubric.

### 1) Split `tracking_pipeline` into explicit stage interfaces

Problem:

- One callback owns extraction, transform, clustering, tracking, classification, persistence, and publishing (`internal/lidar/tracking_pipeline.go:109` onward).

Opportunity:

- Introduce stage interfaces:
  - `ForegroundStage` (L3)
  - `PerceptionStage` (L4)
  - `TrackingStage` (L5)
  - `ObjectStage` (L6)
  - `PersistenceSink` / `PublishSink` (adapters)

Outcome:

- Smaller units, clearer ownership, easier test isolation.

### 2) Move persistence behind repositories/adapters

Problem:

- Pipeline performs SQL writes directly (`internal/lidar/tracking_pipeline.go:345`).
- Track and scene stores are mixed into the same package as tracking math (`internal/lidar/track_store.go:1`, `internal/lidar/scene_store.go:1`, `internal/lidar/analysis_run.go:1`).

Opportunity:

- Keep domain structs in layer packages.
- Move DB operations to `internal/lidar/storage/sqlite`.

Outcome:

- Layered domain remains readable without SQL noise.

### 3) Replace large route blocks with declarative route tables

Problem:

- Route registration is long and duplicated (`internal/lidar/monitor/webserver.go:1190`).
- Method guards are repeated in handlers (`internal/lidar/monitor/run_track_api.go:488`).

Opportunity:

- Use grouped route tables with method+pattern (Go 1.22+ `ServeMux` patterns).
- Add wrappers: `withDB`, `method`, `featureGate`.

Example direction:

```go
type route struct {
    pattern string
    h       http.HandlerFunc
}

var playbackRoutes = []route{
    {"GET /api/lidar/playback/status", ws.handlePlaybackStatus},
    {"POST /api/lidar/playback/pause", ws.handlePlaybackPause},
    {"POST /api/lidar/playback/play", ws.handlePlaybackPlay},
}
```

Outcome:

- Faster scanning, fewer manual dispatch errors, easier endpoint diffs.

### 4) Remove hidden registries from runtime control flow

Problem:

- Global registries hide source-of-truth wiring and make behavior context-dependent (`internal/lidar/analysis_run_manager.go:28`).

Opportunity:

- Build a per-sensor runtime container in `cmd/radar` and pass explicit dependencies through constructors.

Outcome:

- Better determinism and easier integration testing.

### 5) Remove roadmap-phase comments from code and keep history in docs

Problem:

- Comments like "Phase X" drift over time and conflict with current behavior.
- Some comments advertise placeholders where behavior has already changed.

Opportunity:

- Replace with capability-oriented comments:
  - "Run replay endpoint"
  - "Ground truth evaluation endpoint"
  - "Missed region APIs"
- Keep timeline/progress in `docs/` only.

Guardrail:

- Add CI grep to flag new roadmap-phase comments in runtime code:
  - `rg -n "Phase [0-9]" internal web/src tools/visualiser-macos`

### 6) Break frontend LiDAR pages into stores + focused components

Problem:

- Large stateful Svelte files combine fetching, playback, labeling, and rendering (`web/src/routes/lidar/tracks/+page.svelte:1`, `web/src/lib/components/lidar/TrackList.svelte:1`).

Opportunity:

- Extract:
  - `tracksStore` (history, selected track, observations)
  - `runsStore` (scene/run loading + progress)
  - `missedRegionStore`
- Keep components presentational where possible.

Outcome:

- More legible logic boundaries and easier feature iteration.

## Current Implementation Progress

### Completed

1. **Layer contract pass** — package skeleton, interface contracts, dependency rules: ✅
   - Layer packages created: `l1packets/`, `l2frames/`, `l3grid/`, `l4perception/`, `l5tracks/`, `l6objects/`
   - Cross-cutting packages: `pipeline/`, `storage/sqlite/`, `adapters/`
   - Stage interfaces defined: `ForegroundStage`, `PerceptionStage`, `TrackingStage`, `ObjectStage`, `PersistenceSink`, `PublishSink`
   - CI guardrail for "Phase [0-9]" in runtime code

2. **Phase-comment cleanup** — all roadmap-phase comments removed: ✅
   - Go runtime code (18 files), Svelte/TypeScript, Swift files
   - Replaced with capability-oriented descriptions

3. **Route table conversion** — grouped `[]route` slices: ✅
   - `RegisterRoutes` refactored into `coreRoutes`, `snapshotRoutes`, `metricsRoutes`, `sweepRoutes`, `gridRoutes`, `pcapRoutes`, `chartRoutes`, `debugRoutes`, `playbackRoutes`, `trackRoutes`

4. **501 stub replacement** — evaluation and reprocess endpoints: ✅
   - `lidar_evaluations` table (migration 000028) with `EvaluationStore`
   - `handleCreateSceneEvaluation`, `handleListSceneEvaluations`, `handleReprocessRun` implemented

5. **Implementation migration to layer packages**: ✅
   - **L2 Frames** → `l2frames/`: `frame_builder.go` (914 lines), `export.go`, `geometry.go`, `debug.go`
   - **L3 Grid** → `l3grid/`: `background.go` (2608 lines), `foreground.go`, `config.go`, `background_flusher.go`, `foreground_snapshot.go`, `types.go` (BgSnapshot, RegionSnapshot, RegionData)
   - **L4 Perception** → `l4perception/`: `clustering.go`, `dbscan_clusterer.go`, `obb.go`, `ground.go`, `voxel.go`, `types.go` (WorldPoint, PointPolar)
   - **L5 Tracks** → `l5tracks/`: `tracking.go` (1487 lines), `tracker_interface.go`, `hungarian.go`, `types.go` (TrackedObject, TrackState)
   - **L6 Objects** → `l6objects/`: `classification.go`, `features.go`, `quality.go`, `types.go`
   - **Storage** → `storage/sqlite/`: `scene_store.go`, `track_store.go`, `evaluation_store.go`, `sweep_store.go`, `missed_region_store.go`, `analysis_run.go` (1342 lines), `analysis_run_manager.go`
   - Parent files replaced with backward-compatible type aliases

### Remaining

6. **L1 Packets migration** — move `network/` and `parse/` into `l1packets/`: ✅
   - Moved `internal/lidar/network/` → `internal/lidar/l1packets/network/`
   - Moved `internal/lidar/parse/` → `internal/lidar/l1packets/parse/`
   - Updated all callers (cmd/radar, cmd/tools, monitor)

7. **Pipeline migration** — move `tracking_pipeline.go` → `pipeline/`: ✅
   - Moved orchestration logic to `pipeline/tracking_pipeline.go` (canonical)
   - Pipeline imports directly from l2frames, l3grid, l4perception, l5tracks, l6objects, storage/sqlite
   - Parent replaced with backward-compatible type aliases

8. **Adapters migration** — move export/training/ground-truth to `adapters/`: ✅
   - `track_export.go` → `adapters/track_export.go` (canonical)
   - `training_data.go` → `adapters/training_data.go` (canonical)
   - `ground_truth.go` → `adapters/ground_truth.go` (canonical)
   - Parent files replaced with backward-compatible type aliases

9. **Shim removal and caller update** — remove all backward-compat alias files: ✅
   - Removed 27 individual shim files from `internal/lidar/`
   - Updated all sub-package callers (l1packets, monitor, visualiser) to use layer imports
   - Updated all external callers (cmd/radar, internal/db) to use layer imports
   - Remaining `lidar.` imports are only for `Debugf`/`SetDebugLogger` (debug.go stays)
   - `aliases.go` retained only for parent package's own integration tests

10. **Arena.go deprecation** — remove legacy types: ✅
    - Removed `arena.go`, `arena_test.go`, `arena_extended_test.go`
    - All legacy types deleted (RingBuffer, SidecarState, Track, TrackObs, etc.)
    - Active types (Pose, Point, PointPolar, etc.) already migrated to layer packages
    - See `arena-go-deprecation-and-layered-type-layout-design-20260217.md` for details

### Future work

11. **Routing enhancements**:
    - Add HTTP method prefixes to route patterns (`"GET /path"`)
    - Add `withDB`/`method`/`featureGate` middleware wrappers
    - Inline run/scene path dispatch into route tables

12. **Registry reduction**:
    - Move to explicit runtime wiring via dependency injection

13. **Frontend decomposition**:
    - Extract `tracksStore`, `runsStore`, `missedRegionStore`
    - Keep components presentational

## Quick Wins (Low Risk, High Readability)

- ~~Replace phase-labeled placeholder response text~~ ✅ Done
- ~~Convert `RegisterRoutes` into grouped slices~~ ✅ Done
- ~~Add a doc-only "layer ownership matrix" to package headers~~ ✅ Done (doc.go files)
