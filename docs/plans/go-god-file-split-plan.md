# Go God File Split Plan (v0.5.x)

- **Status:** Draft
- **Extracted from:**
  [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md) (Item 2,
  god files scope)
- **Target:** v0.5.1+

## Motivation

Three files carry disproportionate weight. Each mixes four or more concerns, making them
hard to navigate, prone to merge conflicts, and resistant to targeted review. Each milestone
adds 2–3 methods; by v0.7.0 these files will be 2,000+ lines apiece and splitting will
touch every import site.

Beyond the original three, a scan of the full Go codebase reveals nine further files
exceeding 700 LOC that would benefit from domain-driven splitting.

## Current State

### Tier 1 — God Files (>1,400 LOC, mixed concerns)

| File                                            | LOC   | Concerns mixed                                                               |
| ----------------------------------------------- | ----- | ---------------------------------------------------------------------------- |
| `internal/lidar/monitor/webserver.go`           | 1,905 | Struct, lifecycle, routes, tuning params, grid, status, acceptance, persist  |
| `internal/api/server.go`                        | 1,711 | Struct, middleware, radar stats, sites, config, reports, timeline, transit   |
| `internal/l5tracks/tracking.go`                 | 1,676 | Tracker struct, Update(), association, splitting, merging, metrics, config   |
| `internal/db/db.go`                             | 1,420 | DB struct, migrations, radar CRUD, bg snapshots, region snapshots, admin API |
| `internal/lidar/storage/sqlite/analysis_run.go` | 1,400 | Analysis run CRUD, completion, queries, filtering                            |

### Tier 2 — Large Files (700–1,100 LOC, may benefit from splitting)

| File                                            | LOC   | Notes                                         |
| ----------------------------------------------- | ----- | --------------------------------------------- |
| `internal/lidar/monitor/track_api.go`           | 1,065 | Track query handlers + formatting             |
| `internal/lidar/l2frames/frame_builder.go`      | 1,021 | Frame detection, buffering, cleanup, registry |
| `internal/lidar/sweep/hint.go`                  | 1,002 | HINT algorithm + state machine                |
| `internal/lidar/sweep/auto.go`                  | 996   | Auto-tune algorithm + state machine           |
| `internal/lidar/visualiser/publisher.go`        | 884   | gRPC publishing + frame conversion            |
| `internal/lidar/visualiser/grpc_server.go`      | 868   | gRPC server handlers                          |
| `internal/lidar/monitor/run_track_api.go`       | 836   | Run-level track query handlers                |
| `internal/lidar/storage/sqlite/track_store.go`  | 774   | Track persistence + queries                   |
| `internal/lidar/pipeline/tracking_pipeline.go`  | 734   | Pipeline orchestration + state                |
| `internal/lidar/sweep/runner.go`                | 720   | Sweep orchestration + lifecycle               |
| `internal/lidar/monitor/datasource_handlers.go` | 701   | Data source switching + live listener         |

### Tier 3 — Approaching Threshold (500–700 LOC, monitor)

| File                                                | LOC | Notes                          |
| --------------------------------------------------- | --- | ------------------------------ |
| `internal/lidar/analysis/report.go`                 | 646 | Analysis report generation     |
| `internal/lidar/monitor/gridplotter.go`             | 632 | Grid visualisation             |
| `internal/lidar/monitor/playback_handlers.go`       | 604 | PCAP playback handlers         |
| `internal/lidar/monitor/echarts_handlers.go`        | 580 | Chart rendering handlers       |
| `internal/lidar/monitor/client.go`                  | 558 | HTTP client for remote monitor |
| `internal/db/migrate.go`                            | 546 | Migration engine               |
| `internal/lidar/l6objects/classification.go`        | 539 | Object classification logic    |
| `internal/lidar/l3grid/foreground.go`               | 537 | Foreground extraction          |
| `internal/lidar/l1packets/network/pcap_realtime.go` | 525 | PCAP real-time reader          |

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

- [ ] Create `db_radar.go` — move radar event types and query methods
- [ ] Create `db_bg_snapshots.go` — move background snapshot CRUD
- [ ] Create `db_regions.go` — move region snapshot CRUD
- [ ] Create `db_admin.go` — move admin routes and stats
- [ ] Verify `db.go` contains only struct, constructors, and shared helpers
- [ ] `make lint-go && make test-go` passes
- [ ] No file in `internal/db/` exceeds 550 LOC

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

- [ ] Create `server_middleware.go` — move logging middleware
- [ ] Create `server_radar.go` — move radar stats handlers
- [ ] Create `server_sites.go` — move site CRUD handlers
- [ ] Create `server_reports.go` — move report generation and management
- [ ] Create `server_timeline.go` — move timeline and gap computation
- [ ] Create `server_admin.go` — move config, events, DB stats, transit
- [ ] Verify `server.go` contains only struct, constructors, and startup
- [ ] `make lint-go && make test-go` passes
- [ ] No file in `internal/api/` exceeds 600 LOC

### 1C. Split `internal/lidar/monitor/webserver.go` (1,905 → ~300 + 5 domain files)

The monitor package already has good domain files (`sweep_handlers.go`, `chart_api.go`,
`datasource_handlers.go`, etc.). The problem is `webserver.go` still holds tuning params
(~400 LOC), grid/acceptance handlers, status endpoints, and route registration alongside
the core struct.

| New file                  | Functions to move                                                                                                                   | Est. LOC |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `webserver_routes.go`     | `RegisterRoutes`, `setupRoutes`, `withDB`, `featureGate`                                                                            | ~200     |
| `webserver_tuning.go`     | `handleTuningParams`                                                                                                                | ~400     |
| `webserver_grid.go`       | `handleGridStatus`, `handleGridReset`, `handleGridHeatmap`, `handleBackgroundGrid`, `handleBackgroundRegions`, `handleSettlingEval` | ~350     |
| `webserver_status.go`     | `handleHealth`, `handleLidarStatus`, `handleStatus`, `handleDataSource`, `handleLidarPersist`                                       | ~300     |
| `webserver_acceptance.go` | `handleAcceptanceMetrics`, `handleAcceptanceReset`, `handleTrafficStats`                                                            | ~200     |
| `webserver.go` (remains)  | `WebServer` struct, `NewWebServer`, `Start`, `Close`, setters, accessors, reset helpers                                             | ~300     |

**Checklist:**

- [ ] Create `webserver_routes.go` — move route registration
- [ ] Create `webserver_tuning.go` — move tuning params handler
- [ ] Create `webserver_grid.go` — move grid and background handlers
- [ ] Create `webserver_status.go` — move status and health handlers
- [ ] Create `webserver_acceptance.go` — move acceptance and traffic handlers
- [ ] Verify `webserver.go` contains only struct, lifecycle, and helpers
- [ ] `make lint-go && make test-go` passes
- [ ] No file in `internal/lidar/monitor/` exceeds 700 LOC

### 1D. Split `internal/lidar/l5tracks/tracking.go` (1,676 → ~300 + 4 domain files)

| New file                  | Content to move                                           | Est. LOC |
| ------------------------- | --------------------------------------------------------- | -------- |
| `tracking_association.go` | Track-to-observation association, cost matrix, assignment | ~400     |
| `tracking_splitting.go`   | Track splitting and merging logic                         | ~300     |
| `tracking_metrics.go`     | Metrics collection, speed/distance stats, export          | ~300     |
| `tracking_config.go`      | `TrackerConfig`, `DefaultTrackerConfig`, validation       | ~200     |
| `tracking.go` (remains)   | `Tracker` struct, `NewTracker`, `Update()` orchestration  | ~300     |

**Checklist:**

- [ ] Create `tracking_association.go` — move association logic
- [ ] Create `tracking_splitting.go` — move split/merge logic
- [ ] Create `tracking_metrics.go` — move metrics and stats
- [ ] Create `tracking_config.go` — move configuration
- [ ] Verify `tracking.go` contains only struct and orchestration
- [ ] `make lint-go && make test-go` passes
- [ ] No file in `internal/lidar/l5tracks/` exceeds 500 LOC

### 1E. Split `internal/lidar/storage/sqlite/analysis_run.go` (1,400 → domain files)

| New file                    | Content to move                                | Est. LOC |
| --------------------------- | ---------------------------------------------- | -------- |
| `analysis_run_queries.go`   | Read queries: list, get, filter, search        | ~500     |
| `analysis_run_mutations.go` | Insert, update, complete, delete               | ~400     |
| `analysis_run.go` (remains) | Types, `AnalysisRunStore` struct, constructors | ~500     |

**Checklist:**

- [ ] Create `analysis_run_queries.go` — move read operations
- [ ] Create `analysis_run_mutations.go` — move write operations
- [ ] Verify `analysis_run.go` contains only types and constructors
- [ ] `make lint-go && make test-go` passes
- [ ] No file in `internal/lidar/storage/sqlite/` exceeds 600 LOC

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

### 2C. `internal/lidar/monitor/track_api.go` (1,065 LOC)

Split candidates:

- `track_api_queries.go` — query parameter parsing and DB queries
- `track_api_format.go` — response formatting and serialisation
- `track_api.go` — handler registration and dispatch

### 2D. `internal/lidar/monitor/run_track_api.go` (836 LOC)

Similar pattern to track_api.go — split by query vs format vs dispatch.

### 2E. `internal/lidar/visualiser/publisher.go` (884 LOC) and `grpc_server.go` (868 LOC)

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

- `internal/lidar/analysis/report.go` (646 LOC)
- `internal/lidar/monitor/gridplotter.go` (632 LOC)
- `internal/lidar/monitor/playback_handlers.go` (604 LOC)
- `internal/lidar/monitor/echarts_handlers.go` (580 LOC)
- `internal/lidar/monitor/client.go` (558 LOC)
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

- **Context propagation** — covered in
  [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md) (Item 1)
- **Race conditions** (`serialmux.CurrentState`) — remains in the hygiene plan (Item 2)
- **DB abstraction leaks** — remains in the hygiene plan (Item 2)
- **Silent error drops** — remains in the hygiene plan (Item 2)
- **JSON tag consistency** — covered in the hygiene plan (Item 3)
- **Structured logging** — covered in
  [go-structured-logging-plan.md](go-structured-logging-plan.md)
- **Functional refactoring** — this plan moves code between files; it does not change
  behaviour, signatures, or abstractions
