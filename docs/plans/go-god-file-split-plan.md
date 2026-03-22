# Go God File Split Plan (v0.5.x)

- **Status:** Phase 1 Complete
- **Extracted from:**
  [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md) (Item 2,
  god files scope)
- **Target:** v0.5.1+

## Motivation

Three files carry disproportionate weight. Each mixes four or more concerns, making them
hard to navigate, prone to merge conflicts, and resistant to targeted review. Each milestone
adds 2–3 methods; by v0.7.0 these files will be 2,000+ lines apiece and splitting will
touch every import site.

Beyond the original three, a scan of the full Go codebase reveals thirteen further files
exceeding 700 LOC that would benefit from domain-driven splitting.

## Current State

### Tier 1 — God Files (>1,400 LOC, mixed concerns)

| File                                            | LOC       | Concerns mixed                                                                                                            |
| ----------------------------------------------- | --------- | ------------------------------------------------------------------------------------------------------------------------- |
| ~~`internal/lidar/monitor/webserver.go`~~       | ~~1,905~~ | **DONE** — split into `server/server.go` (426), `state.go` (174), `routes.go` (219), `tuning.go` (104), `status.go` (690) |
| `internal/api/server.go`                        | 1,711     | Struct, middleware, radar stats, sites, config, reports, timeline, transit                                                |
| `internal/l5tracks/tracking.go`                 | 1,676     | Tracker struct, Update(), association, splitting, merging, metrics, config                                                |
| `internal/db/db.go`                             | 1,420     | DB struct, migrations, radar CRUD, bg snapshots, region snapshots, admin API                                              |
| `internal/lidar/storage/sqlite/analysis_run.go` | 1,400     | Analysis run CRUD, completion, queries, filtering                                                                         |

### Tier 2 — Large Files (700–1,100 LOC, may benefit from splitting)

| File                                           | LOC   | Notes                                         |
| ---------------------------------------------- | ----- | --------------------------------------------- |
| `internal/lidar/server/track_api.go`           | 1,065 | Track query handlers + formatting             |
| `internal/lidar/l2frames/frame_builder.go`     | 1,021 | Frame detection, buffering, cleanup, registry |
| `internal/lidar/sweep/hint.go`                 | 1,002 | HINT algorithm + state machine                |
| `internal/lidar/sweep/auto.go`                 | 996   | Auto-tune algorithm + state machine           |
| `internal/lidar/l9endpoints/publisher.go`      | 884   | gRPC publishing + frame conversion            |
| `internal/lidar/l9endpoints/grpc_server.go`    | 868   | gRPC server handlers                          |
| `internal/lidar/server/run_track_api.go`       | 836   | Run-level track query handlers                |
| `internal/lidar/storage/sqlite/track_store.go` | 774   | Track persistence + queries                   |
| `internal/lidar/pipeline/tracking_pipeline.go` | 734   | Pipeline orchestration + state                |
| `internal/lidar/sweep/runner.go`               | 720   | Sweep orchestration + lifecycle               |
| `internal/lidar/server/datasource_handlers.go` | 701   | Data source switching + live listener         |

### Tier 3 — Approaching Threshold (500–700 LOC, monitor)

| File                                                | LOC | Notes                                                                 |
| --------------------------------------------------- | --- | --------------------------------------------------------------------- |
| `internal/lidar/server/status.go`                   | 690 | Grid, status, health, acceptance, background handlers (from 1C split) |
| `internal/lidar/analysis/report.go`                 | 646 | Analysis report generation                                            |
| `internal/lidar/l9endpoints/gridplotter.go`         | 632 | Grid visualisation                                                    |
| `internal/lidar/server/playback_handlers.go`        | 604 | PCAP playback handlers                                                |
| `internal/lidar/server/echarts_handlers.go`         | 580 | Chart rendering handlers                                              |
| `internal/lidar/server/client.go`                   | 558 | HTTP client for remote server                                         |
| `internal/db/migrate.go`                            | 546 | Migration engine                                                      |
| `internal/lidar/l6objects/classification.go`        | 539 | Object classification logic                                           |
| `internal/lidar/l3grid/foreground.go`               | 537 | Foreground extraction                                                 |
| `internal/lidar/l1packets/network/pcap_realtime.go` | 525 | PCAP real-time reader                                                 |

---

## Phase 1 — Tier 1 God Files

Target: v0.5.1. Mechanical file moves. No functional changes. Tests pass unchanged.

### 1A. Split `internal/db/db.go` (1,420 → ~200 + 4 domain files)

The `db` package already has good per-domain files (`site.go`, `transit_worker.go`,
`site_config_periods.go`, `site_report.go`). The problem is `db.go` itself, which mixes
radar recording, background snapshot CRUD, region snapshots, admin HTTP routes, and database
statistics.

| New file             | Functions to move                                                                                                                                                                                                       | Est. LOC |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `db_radar.go`        | `RecordRadarObject`, `RadarObjects`, `RadarObjectsRollupRow.String`, `buildCosineSpeedExpr`, `RadarObjectRollupRange`, `RecordRawData`, `Event.String`, `EventToAPI`, `Events`, `RadarDataRange`                        | ~500     |
| `db_bg_snapshots.go` | `ListRecentBgSnapshots`, `DeleteDuplicateBgSnapshots`, `InsertBgSnapshot`, `GetLatestBgSnapshot`, `GetBgSnapshotByID`, `scanBgSnapshot`, `CountUniqueBgSnapshotHashes`, `FindDuplicateBgSnapshots`, `DeleteBgSnapshots` | ~350     |
| `db_regions.go`      | `InsertRegionSnapshot`, `GetRegionSnapshotByGridHash`, `GetRegionSnapshotBySourcePath`, `GetLatestRegionSnapshot`, `scanRegionSnapshot`                                                                                 | ~80      |
| `db_admin.go`        | `GetDatabaseStats`, `AttachAdminRoutes` (+ inline handlers)                                                                                                                                                             | ~200     |
| `db.go` (remains)    | `DB` struct, `NewDB`, `NewDBWithMigrationCheck`, `OpenDB`, `applyPragmas`, type definitions, helpers                                                                                                                    | ~290     |

**Checklist:**

- [x] Create `db_radar.go` — move radar event types and query methods (562 LOC)
- [x] Create `db_bg_snapshots.go` — move background snapshot CRUD (278 LOC)
- [x] Create `db_regions.go` — move region snapshot CRUD (94 LOC)
- [x] Create `db_admin.go` — move admin routes and stats (183 LOC)
- [x] Verify `db.go` contains only struct, constructors, and shared helpers (337 LOC)
- [x] `make lint-go && make test-go` passes
- [ ] No file in `internal/db/` exceeds 550 LOC — `db_radar.go` at 562 (12 over; acceptable)

### 1B. Split `internal/api/server.go` (1,711 → ~200 + 6 domain files)

| New file               | Functions to move                                                                                                                                             | Est. LOC |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `server_middleware.go` | `loggingResponseWriter`, `statusCodeColor`, `LoggingMiddleware`                                                                                               | ~70      |
| `server_radar.go`      | `showRadarObjectStats`, `keysOfMap`, `convertEventAPISpeed`                                                                                                   | ~230     |
| `server_sites.go`      | `handleSites`, `listSites`, `getSite`, `createSite`, `updateSite`, `deleteSite`, `handleSiteConfigPeriods`, `listSiteConfigPeriods`, `upsertSiteConfigPeriod` | ~220     |
| `server_reports.go`    | `generateReport`, `handleReports`, `listAllReports`, `listSiteReports`, `getReport`, `downloadReport`, `getPDFGeneratorDir`, `deleteReport`                   | ~600     |
| `server_timeline.go`   | `handleTimeline`, `uniqueAnglesForRange`, `computeUnconfiguredGaps`                                                                                           | ~140     |
| `server_admin.go`      | `showConfig`, `listEvents`, `handleDatabaseStats`, `handleTransitWorker`                                                                                      | ~180     |
| `server.go` (remains)  | `Server` struct, `NewServer`, `SetTransitController`, `ServeMux`, `sendCommandHandler`, `writeJSONError`, `Start`                                             | ~170     |

**Checklist:**

- [x] Create `server_middleware.go` — move logging middleware (75 LOC)
- [x] Create `server_radar.go` — move radar stats handlers (269 LOC)
- [x] Create `server_sites.go` — move site CRUD handlers (231 LOC)
- [x] Create `server_reports.go` + `server_reports_generate.go` — move report generation and management (237 + 419 LOC)
- [x] Create `server_timeline.go` — move timeline and gap computation (123 LOC)
- [x] Create `server_admin.go` — move config, events, DB stats, transit (177 LOC)
- [x] Verify `server.go` contains only struct, constructors, and startup (260 LOC)
- [x] `make lint-go && make test-go` passes
- [x] No file in `internal/api/` exceeds 600 LOC

### 1C. ~~Split `internal/lidar/monitor/webserver.go`~~ — DONE

**Implemented on branch `dd/bringing-it-home`.** The `monitor/` package was renamed to
`server/` and `webserver.go` (1,573 LOC at time of split) was split into five files. The
`monitor/` directory was deleted (13 orphaned files, no remaining imports).

| File        | Contents                                                                                                                                                                                                                                                                                                     | LOC |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --- |
| `server.go` | `Server` struct, `Config`, `ParamDef`, `PlaybackStatusInfo`, `NewServer`, `Start`, `Close`, `writeJSONError`, `cloneTuningConfig`                                                                                                                                                                            | 426 |
| `state.go`  | `setBaseContext`, `baseContext`, `CurrentSource`, `CurrentPCAPFile`, `PCAPSpeedRatio`, `SetTracker`, `SetClassifier`, `Set*Runner`, `SetSweepStore`, `BenchmarkMode`, resetters                                                                                                                              | 174 |
| `routes.go` | `route` type, `withDB`, `featureGate`, `RegisterRoutes`, `setupRoutes`                                                                                                                                                                                                                                       | 219 |
| `tuning.go` | `handleTuningParams` (GET/POST for `/api/lidar/params`)                                                                                                                                                                                                                                                      | 104 |
| `status.go` | `handleGridStatus`, `handleSettlingEval`, `handleTrafficStats`, `handleGridReset`, `handleGridHeatmap`, `handleDataSource`, `handleHealth`, `handleLidarStatus`, `handleStatus`, `handleLidarPersist`, `handleAcceptanceMetrics`, `handleAcceptanceReset`, `handleBackgroundGrid`, `handleBackgroundRegions` | 690 |

**Checklist:**

- [x] `webserver.go` renamed to `server.go`; `WebServer` → `Server`, `WebServerConfig` → `Config`
- [x] `routes.go` created — route type, registration, middleware
- [x] `tuning.go` created — tuning params handler
- [x] `status.go` created — grid, status, health, acceptance, background handlers
- [x] `state.go` created — state accessors, setters, resetters
- [x] `server.go` contains only struct, config, lifecycle, and shared helpers
- [x] `monitor/` directory deleted (13 orphaned files, zero imports)
- [x] `go build ./...` passes; `go test ./internal/lidar/server/...` passes (64 s)
- [ ] `status.go` at 690 LOC exceeds the 600 LOC target — candidate for a follow-on split into `status_grid.go` + `status_acceptance.go`

### 1D. Split `internal/lidar/l5tracks/tracking.go` (1,676 → ~300 + 4 domain files)

| New file                  | Content to move                                           | Est. LOC |
| ------------------------- | --------------------------------------------------------- | -------- |
| `tracking_association.go` | Track-to-observation association, cost matrix, assignment | ~400     |
| `tracking_splitting.go`   | Track splitting and merging logic                         | ~300     |
| `tracking_metrics.go`     | Metrics collection, speed/distance stats, export          | ~300     |
| `tracking_config.go`      | `TrackerConfig`, `DefaultTrackerConfig`, validation       | ~200     |
| `tracking.go` (remains)   | `Tracker` struct, `NewTracker`, `Update()` orchestration  | ~300     |

**Checklist:**

- [x] Create `tracking_association.go` — move association logic (297 LOC)
- [x] Create `tracking_update.go` — move Kalman update method (370 LOC)
- [x] Create `tracking_metrics.go` — move metrics and stats (441 LOC)
- [x] Create `tracking_config.go` — move configuration (115 LOC)
- [x] Verify `tracking.go` contains only struct and orchestration (515 LOC)
- [x] `make lint-go && make test-go` passes
- [ ] No file in `internal/lidar/l5tracks/` exceeds 500 LOC — `tracking.go` at 515 (15 over; acceptable)

Note: The original plan did not include `tracking_splitting.go` — split/merge logic
is handled inline within `Update()` in `tracking.go`. A `tracking_update.go` file was
created instead to hold the Kalman `update` method extracted from
`tracking_association.go`.

### 1E. Split `internal/lidar/storage/sqlite/analysis_run.go` (1,400 → domain files)

| New file                    | Content to move                                | Est. LOC |
| --------------------------- | ---------------------------------------------- | -------- |
| `analysis_run_queries.go`   | Read queries: list, get, filter, search        | ~500     |
| `analysis_run_mutations.go` | Insert, update, complete, delete               | ~400     |
| `analysis_run.go` (remains) | Types, `AnalysisRunStore` struct, constructors | ~500     |

**Checklist:**

- [x] Create `analysis_run_queries.go` — move read operations (599 LOC)
- [x] Create `analysis_run_mutations.go` — move write operations (246 LOC)
- [x] Create `analysis_run_compare.go` — move CompareRuns and comparison helpers (292 LOC)
- [x] Verify `analysis_run.go` contains only types and constructors (391 LOC)
- [x] `make lint-go && make test-go` passes
- [x] No file in `internal/lidar/storage/sqlite/` exceeds 600 LOC (for Phase 1 files;
      `track_store.go` at 774 is a Tier 2 file tracked separately)

---

## Phase 2 — Tier 2 Large Files

Target: v0.5.2 or later. Lower priority; these files are large but generally single-concern.
Evaluate after Phase 1 completes — some may be fine as-is if their size reflects genuine
domain complexity rather than mixed concerns.

### 2A. `internal/lidar/l2frames/frame_builder.go` (1,021 LOC)

Split candidates:

- `frame_builder_cleanup.go` — cleanup timer, frame buffering, finalisation
- `frame_builder_registry.go` — global registry (`RegisterFrameBuilder`,
  `UnregisterFrameBuilder`, `GetFrameBuilder`)
- `frame_builder.go` — core detection logic, `AddPacket`, azimuth tracking

### 2B. `internal/lidar/sweep/hint.go` (1,002 LOC) and `auto.go` (996 LOC)

Both implement multi-round state machines. Splitting options:

- Extract state transitions into `hint_state.go` / `auto_state.go`
- Extract round evaluation into `hint_evaluation.go` / `auto_evaluation.go`
- Keep orchestration in `hint.go` / `auto.go`

### 2C. `internal/lidar/server/track_api.go` (1,065 LOC)

Split candidates:

- `track_api_queries.go` — query parameter parsing and DB queries
- `track_api_format.go` — response formatting and serialisation
- `track_api.go` — handler registration and dispatch

### 2D. `internal/lidar/server/run_track_api.go` (836 LOC)

Similar pattern to track_api.go — split by query vs format vs dispatch.

### 2E. `internal/lidar/l9endpoints/publisher.go` (884 LOC) and `grpc_server.go` (868 LOC)

- `publisher.go` — split frame conversion from publishing logic
- `grpc_server.go` — split per-RPC handler implementations into separate files

### 2F. `internal/lidar/pipeline/tracking_pipeline.go` (734 LOC)

Split candidates:

- `tracking_pipeline_setup.go` — initialisation and wiring
- `tracking_pipeline.go` — runtime orchestration

### 2G. `internal/lidar/sweep/runner.go` (720 LOC)

Split candidates:

- `runner_lifecycle.go` — start, stop, suspend, resume
- `runner.go` — core round execution

---

## Phase 3 — Tier 3 Files and Ongoing Hygiene

Target: v0.6.0+. These files are approaching the threshold. Address opportunistically when
making functional changes in the same file, rather than as standalone refactors.

**Guideline:** Any file that exceeds 600 LOC during a feature change should be split as
part of that change.

### Candidates

- `internal/lidar/server/status.go` (690 LOC) — candidate from 1C split; grid + acceptance could separate
- `internal/lidar/analysis/report.go` (646 LOC)
- `internal/lidar/l9endpoints/gridplotter.go` (632 LOC)
- `internal/lidar/server/playback_handlers.go` (604 LOC)
- `internal/lidar/server/echarts_handlers.go` (580 LOC)
- `internal/lidar/server/client.go` (558 LOC)
- `internal/db/migrate.go` (546 LOC)
- `internal/lidar/l6objects/classification.go` (539 LOC)
- `internal/lidar/l3grid/foreground.go` (537 LOC)

---

## Execution Rules

1. **One file split per commit.** Each commit moves functions from one source file into one
   or more new files. No functional changes, no renames, no interface changes in the same
   commit.
2. **Tests must pass unchanged.** File splits within a Go package do not change import
   paths. Existing tests are the verification gate.
3. **No new abstractions.** This plan moves code, not refactors it. Introducing interfaces,
   generic helpers, or new packages is out of scope.
4. **Target: no production file above 600 LOC.** Phase 1 enforces stricter limits per
   package (see checklists). Phase 2 and 3 use 600 LOC as the general threshold.
5. **Race detector clean.** Run `go test -race ./...` after each phase.

## Verification

After each phase:

```bash
make lint-go && make test-go
go test -race ./...
find internal/ -name '*.go' ! -name '*_test.go' -exec wc -l {} + | sort -rn | head -20
```

Phase 1 target: no production file above 600 LOC in the five split packages.
Phase 2 target: no production file above 700 LOC across the codebase.
Phase 3 target: no production file above 600 LOC across the codebase.

## What This Plan Does Not Cover

- **Context propagation** — ~~covered in
  [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md) (Item 1)~~ **Complete.** All 8 `_ = r` placeholders removed from `internal/api/server.go`; 10 DB methods accept `context.Context`.
- **Race conditions** (`serialmux.CurrentState`) — ~~remains in the hygiene plan (Item 2)~~ **Complete.** Replaced with `sync.RWMutex`-backed private state; `CurrentStateSnapshot()` and `ResetCurrentState()` exported.
- **DB abstraction leaks** — remains in the hygiene plan (Item 2)
- **Silent error drops** — remains in the hygiene plan (Item 2)
- **JSON tag consistency** — covered in the hygiene plan (Item 3)
- **Structured logging** — covered in
  [go-structured-logging-plan.md](go-structured-logging-plan.md)
- **Functional refactoring** — this plan moves code between files; it does not change
  behaviour, signatures, or abstractions
