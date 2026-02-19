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

11. **Routing enhancements**: ✅
    - Added Go 1.22+ HTTP method prefixes to 40+ route patterns (`"GET /path"`, `"POST /path"`)
    - Added `withDB` and `featureGate` middleware wrappers
    - Removed ~30 redundant method guard blocks from handlers

12. **Registry reduction**: ✅
    - Added `SensorRuntime` DI container in `pipeline/runtime.go`
    - Added `NewFrameBuilderDI`, `NewBackgroundManagerDI`, `NewAnalysisRunManagerDI` constructors
    - Global registries retained for backward compatibility; new code uses explicit wiring

### Future work

13. **Frontend decomposition**:
    - Extract `tracksStore`, `runsStore`, `missedRegionStore`
    - Keep components presentational

14. **Cross-layer placement fixes**: ✅
    - Extracted `ComputeTemporalIoU` and comparison types to `l6objects/comparison.go`; duplicated code in `adapters/ground_truth.go` removed
    - Split `l3grid/background.go` (2,610 → 1,628 lines) into `background_persistence.go` (450), `background_export.go` (350), `background_drift.go` (245)
    - Split `monitor/webserver.go` (4,067 → 2,749 lines) into `datasource_handlers.go` (682), `playback_handlers.go` (589)

## Layer Complexity Analysis (Post-Split)

### Size distribution (current)

| Package            | Source lines | Test lines | Largest file               | Notes                                                        |
| ------------------ | ------------ | ---------- | -------------------------- | ------------------------------------------------------------ |
| **l1packets**      | 3,510        | 5,039      | extract.go (621)           | Well-distributed across network/ and parse/ sub-packages     |
| **l2frames**       | 1,135        | 1,989      | frame_builder.go (973)     | Clean single-responsibility; frame assembly + geometry       |
| **l3grid**         | 3,929        | 5,646      | background.go (1,628)      | ✅ Split done — persistence, export, drift in separate files |
| **l4perception**   | 1,078        | 1,442      | cluster.go (469)           | Clean; DBSCAN, OBB, ground removal, voxel                    |
| **l5tracks**       | 1,738        | 1,849      | tracking.go (1,488)        | Cohesive; Kalman tracker, lifecycle, metrics                 |
| **l6objects**      | 1,141        | 1,014      | quality.go (388)           | Clean; classification, features, quality, comparison         |
| **pipeline**       | 608          | 35         | tracking_pipeline.go (541) | Thin orchestrator — expected to be small                     |
| **storage/sqlite** | 3,552        | 5,551      | analysis_run.go (1,325)    | ✅ Domain logic extracted to l6objects/comparison.go         |
| **adapters**       | 776          | 772        | ground_truth.go (380)      | Clean; export/training/ground-truth I/O                      |
| **sweep**          | 4,974        | 9,008      | hint.go (1,222)            | Well-decoupled; no layer imports, uses interfaces only       |
| **monitor**        | 10,040       | 23,646     | webserver.go (2,746)       | ✅ Split done — datasource and playback handlers extracted   |
| **visualiser**     | 3,286        | 7,319      | adapter.go (790)           | Clean; gRPC server, publisher, adapter                       |

**Total**: 35,767 source lines, 63,310 test lines across 12 packages (incl. visualiser).

### Balance assessment (post-split)

All three P0 outliers have been addressed:

1. **l3grid** — `background.go` reduced from 2,610 to 1,628 lines. Persistence, export, and drift detection in separate files. Package total stable at ~3,900 lines.

2. **storage/sqlite** — `CompareRuns`, `computeTemporalIoU`, and comparison types moved to `l6objects/comparison.go`. `analysis_run.go` reduced from 1,383 to 1,325 lines. `compareParams` remains (parameter diffing still coupled to store types).

3. **monitor** — `webserver.go` reduced from 4,067 to 2,746 lines. Datasource switching and playback controls extracted. Package total still ~10,000 lines — the largest in the stack.

### Completed cross-layer moves

#### ✅ Priority 1: Extract domain logic from storage

`ComputeTemporalIoU` and comparison types (`RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge`) moved to `l6objects/comparison.go`. Storage layer retains backward-compatible type aliases. Duplicate implementation in `adapters/ground_truth.go` replaced with thin wrapper.

#### ✅ Priority 2: Extract persistence and export from l3grid

Split `background.go` into:

- `background.go` — core grid processing, EMA updates, region management (1,628 lines)
- `background_persistence.go` — snapshot serialisation, database restore/persist (450 lines)
- `background_export.go` — heatmaps, ASC export, region debug info (350 lines)
- `background_drift.go` — M3.5 sensor movement and drift detection (245 lines)

#### ✅ Priority 3: Split monitor/webserver.go

Split `webserver.go` into:

- `webserver.go` — server init, route registration, remaining handlers (2,746 lines)
- `datasource_handlers.go` — UDP/PCAP data source management (682 lines)
- `playback_handlers.go` — PCAP/VRLOG playback controls (589 lines)

#### Not recommended to move

- **l5tracks/tracking.go** (1,488 lines) — cohesive Kalman tracker; all methods serve tracking lifecycle. Well-bounded.
- **sweep/** (4,974 lines) — fully decoupled, uses interfaces only, no layer imports. Clean design.
- **l1packets/** (3,510 lines) — well-distributed across network/ and parse/ sub-packages. No cross-layer concerns.
- **l2frames/frame_builder.go** (973 lines) — single-responsibility frame assembly. Clean.
- **l4perception/** (1,078 lines) — small, focused clustering/segmentation. Clean.

## Further Opportunities to Reduce Size and Complexity

These are lower-priority improvements that would further improve readability and maintainability but are not blocking current development.

### Opportunity 1: Extract ECharts handlers from monitor/webserver.go ✅

**Completed**: Extracted 9 chart/dashboard handlers into `echarts_handlers.go` (580 lines). `webserver.go` reduced from 2,746 to 1,775 lines.

### Opportunity 2: Extract export handlers from monitor/webserver.go ✅

**Completed**: Extracted 8 export/snapshot handlers into `export_handlers.go` (391 lines). `webserver.go` further reduced to 1,775 lines.

### Opportunity 3: Split sweep/hint.go (1,222 lines) ✅

**Completed**: Extracted progress tracking into `hint_progress.go` (153 lines) and notification/utility functions into `hint_notifications.go` (84 lines). `hint.go` reduced to 998 lines.

### Opportunity 4: Split sweep/auto.go (1,214 lines) ✅

**Completed**: Extracted grid narrowing, bounds computation, and utility functions into `auto_narrowing.go` (227 lines). `auto.go` reduced to 993 lines.

### Opportunity 5: Split sweep/runner.go (1,195 lines) ✅

**Completed**: Extracted parameter generation and combination logic into `sweep_params.go` (242 lines). `runner.go` reduced to 953 lines.

### Opportunity 6: Reduce storage/sqlite/analysis_run.go (1,325 lines) ✅

**Completed**: Extracted `compareParams` and `computeTemporalIoU` into `analysis_run_compare.go` (112 lines). `analysis_run.go` reduced to 1,216 lines. RunParams types remain in the storage package to avoid circular imports; full domain extraction deferred to a future PR.

### Opportunity 7: Retire Go-embedded HTML dashboards

**Status**: Deferred — requires corresponding Svelte dashboard implementation first (frontend consolidation Phases 1–5).

**Current state**: monitor package contains 5 `go:embed` directives and ~600 lines of Go HTML templates plus 12+ ECharts JavaScript chart handlers. These are the legacy debug dashboards scheduled for replacement.

**Impact**: ~2,000 lines removed from monitor. Eliminates Go template injection surface.

**Risk**: Medium — requires corresponding Svelte dashboard implementation first.

### Opportunity 8: Consolidate visualiser adapter/publisher (790+740 lines) ✅

**Completed**: Extracted point cloud memory pool (sync.Pool) and decimation codec (Release, ApplyDecimation, uniform/foreground/voxel decimation) into `frame_codec.go` (280 lines). `adapter.go` reduced from 790 to 519 lines.

## Quick Wins (Low Risk, High Readability)

- ~~Replace phase-labeled placeholder response text~~ ✅ Done
- ~~Convert `RegisterRoutes` into grouped slices~~ ✅ Done
- ~~Add a doc-only "layer ownership matrix" to package headers~~ ✅ Done (doc.go files)
