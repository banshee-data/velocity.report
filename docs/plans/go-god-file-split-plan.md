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

Beyond the original three, a scan of the full Go codebase reveals further files exceeding
700 LOC that would benefit from domain-driven splitting.

## Current State

### Tier 1 — God Files (>1,000 LOC, mixed concerns)

| File                                                | Was       | Now | Status                                                                                                                             |
| --------------------------------------------------- | --------- | --- | ---------------------------------------------------------------------------------------------------------------------------------- |
| ~~`internal/lidar/monitor/webserver.go`~~           | ~~1,905~~ | —   | **DONE** — split into `server/server.go` (423), `state.go` (174), `routes.go` (220), `tuning.go` (122), `status.go` (690)          |
| ~~`internal/api/server.go`~~                        | ~~1,711~~ | 260 | **DONE** — split into 6 domain files (see 1B)                                                                                      |
| ~~`internal/lidar/l5tracks/tracking.go`~~           | ~~1,676~~ | 515 | **DONE** — split into 3 domain files (see 1D)                                                                                      |
| ~~`internal/db/db.go`~~                             | ~~1,420~~ | 337 | **DONE** — split into 4 domain files (see 1A)                                                                                      |
| ~~`internal/lidar/storage/sqlite/analysis_run.go`~~ | ~~1,400~~ | 391 | **DONE** — split into 4 domain files (see 1E)                                                                                      |
| ~~`internal/lidar/l3grid/background.go`~~           | ~~1,672~~ | 352 | **DONE** — split into `background.go` (352), `background_region.go` (474), `background_manager.go` (860) (see 1F)                  |
| ~~`internal/config/tuning.go`~~                     | ~~1,303~~ | 250 | **DONE** — split into `tuning.go` (250), `tuning_validate.go` (391), `tuning_codec.go` (280), `tuning_accessors.go` (361) (see 1G) |

### Tier 2 — Large Files (700–1,100 LOC, may benefit from splitting)

| File                                              | Was   | Now | Notes                                                                       |
| ------------------------------------------------- | ----- | --- | --------------------------------------------------------------------------- |
| `internal/lidar/l3grid/background_manager.go`     | —     | 861 | **NEW from 1F split** — exceeds 600 LOC target                              |
| `internal/lidar/sweep/hint.go`                    | 1,002 | 798 | HINT algorithm + state machine                                              |
| `internal/lidar/storage/sqlite/track_store.go`    | 774   | 774 | Track persistence + queries                                                 |
| `internal/lidar/server/track_api.go`              | 1,065 | 763 | Track query handlers + formatting (shrunk, not by split)                    |
| `internal/lidar/sweep/auto.go`                    | 996   | 762 | Auto-tune algorithm + state machine                                         |
| `internal/lidar/server/run_track_api.go`          | 836   | 752 | Run-level track query handlers                                              |
| `internal/lidar/pipeline/tracking_pipeline.go`    | 734   | 733 | Pipeline orchestration + state                                              |
| `internal/lidar/sweep/runner.go`                  | 720   | 720 | Sweep orchestration + lifecycle                                             |
| `internal/lidar/l9endpoints/recorder/recorder.go` | —     | 711 | **NEW** — not in original plan                                              |
| `internal/lidar/server/datasource_handlers.go`    | 701   | 702 | Data source switching + live listener                                       |
| `internal/lidar/server/client.go`                 | 558   | 647 | HTTP client (grew past 600 threshold)                                       |
| `internal/lidar/analysis/report.go`               | 646   | 646 | Analysis report generation                                                  |
| `internal/lidar/l9endpoints/gridplotter.go`       | 632   | 632 | Grid visualisation                                                          |
| `internal/lidar/l9endpoints/grpc_server.go`       | 868   | 619 | gRPC server handlers (shrunk)                                               |
| `internal/lidar/l2frames/frame_builder.go`        | 1,021 | 448 | **Partially split** — cleanup extracted to `frame_builder_cleanup.go` (602) |
| `internal/lidar/l9endpoints/publisher.go`         | 884   | 540 | gRPC publishing + frame conversion (shrunk)                                 |

### Tier 3 — Approaching Threshold (500–700 LOC, monitor)

| File                                                    | LOC | Notes                                                                 |
| ------------------------------------------------------- | --- | --------------------------------------------------------------------- |
| `internal/lidar/server/status.go`                       | 690 | Grid, status, health, acceptance, background handlers (from 1C split) |
| `internal/lidar/server/playback_handlers.go`            | 604 | PCAP playback handlers                                                |
| `internal/lidar/l2frames/frame_builder_cleanup.go`      | 602 | Cleanup timer, buffering, finalisation (from 2A split)                |
| `internal/lidar/l1packets/parse/extract.go`             | 601 | Packet parsing and extraction                                         |
| `internal/lidar/storage/sqlite/analysis_run_queries.go` | 599 | Read queries, list, get, filter, search (from 1E split; was 775)      |
| `internal/lidar/server/tuning_runtime.go`               | 582 | Runtime tuning param handlers                                         |
| `internal/lidar/server/echarts_handlers.go`             | 580 | Chart rendering handlers                                              |
| `internal/db/db_radar.go`                               | 562 | Radar event types and query methods (from 1A split)                   |
| `internal/db/migrate.go`                                | 546 | Migration engine                                                      |
| `internal/lidar/l3grid/foreground.go`                   | 542 | Foreground extraction                                                 |
| `internal/lidar/l9endpoints/publisher.go`               | 540 | gRPC publishing + frame conversion                                    |
| `internal/lidar/l6objects/classification.go`            | 539 | Object classification logic                                           |
| `internal/lidar/l4perception/cluster.go`                | 535 | Clustering logic                                                      |
| `internal/lidar/l1packets/network/pcap_realtime.go`     | 525 | PCAP real-time reader                                                 |
| `internal/lidar/l9endpoints/adapter.go`                 | 524 | Pipeline adapter                                                      |
| `internal/lidar/l5tracks/tracking.go`                   | 515 | Tracker struct and orchestration (from 1D split)                      |
| `internal/db/transit_worker.go`                         | 503 | Transit background worker                                             |

---

## Phase 1 — Tier 1 God Files

Target: v0.5.1. Mechanical file moves. No functional changes. Tests pass unchanged.

### 1A. ~~Split `internal/db/db.go`~~ — DONE

Split from 1,420 LOC into 4 domain files + core. All checklist items complete.

| File                 | Contents                                    | LOC |
| -------------------- | ------------------------------------------- | --- |
| `db.go`              | `DB` struct, constructors, pragmas, helpers | 337 |
| `db_radar.go`        | Radar event types and query methods         | 562 |
| `db_bg_snapshots.go` | Background snapshot CRUD                    | 278 |
| `db_regions.go`      | Region snapshot CRUD                        | 94  |
| `db_admin.go`        | Admin routes and database stats             | 183 |

**Checklist:**

- [x] Create `db_radar.go` — move radar event types and query methods (562 LOC)
- [x] Create `db_bg_snapshots.go` — move background snapshot CRUD (278 LOC)
- [x] Create `db_regions.go` — move region snapshot CRUD (94 LOC)
- [x] Create `db_admin.go` — move admin routes and stats (183 LOC)
- [x] Verify `db.go` contains only struct, constructors, and shared helpers (337 LOC)
- [x] `make lint-go && make test-go` passes
- [ ] No file in `internal/db/` exceeds 550 LOC — `db_radar.go` at 562 (12 over; acceptable)

### 1B. ~~Split `internal/api/server.go`~~ — DONE

Split from 1,711 LOC into 6 domain files + core. All checklist items complete.

| File                   | Contents                               | LOC |
| ---------------------- | -------------------------------------- | --- |
| `server.go`            | `Server` struct, constructors, startup | 260 |
| `server_middleware.go` | Logging middleware                     | 75  |
| `server_radar.go`      | Radar stats handlers                   | 269 |
| `server_sites.go`      | Site CRUD handlers                     | 231 |
| `server_reports.go`    | Report generation and management       | 644 |
| `server_timeline.go`   | Timeline and gap computation           | 123 |
| `server_admin.go`      | Config, events, DB stats, transit      | 177 |

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

### 1D. ~~Split `internal/lidar/l5tracks/tracking.go`~~ — DONE

Split from 1,676 LOC into 4 domain files + core. The planned `tracking_splitting.go` was
not created — split/merge logic remained in `tracking_association.go`.

| File                      | Contents                                         | LOC |
| ------------------------- | ------------------------------------------------ | --- |
| `tracking.go`             | `Tracker` struct, `NewTracker`, `Update()`       | 515 |
| `tracking_association.go` | Association, cost matrix, split/merge            | 297 |
| `tracking_update.go`      | Kalman update method                             | 370 |
| `tracking_metrics.go`     | Metrics collection, speed/distance stats, export | 441 |
| `tracking_config.go`      | `TrackerConfig`, `DefaultTrackerConfig`          | 115 |

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

### 1E. ~~Split `internal/lidar/storage/sqlite/analysis_run.go`~~ — DONE

Split from 1,400 LOC into 4 domain files + core. Two additional files (`_compare.go`,
`_manager.go`) were created beyond the original plan.

| File                        | Contents                                | LOC |
| --------------------------- | --------------------------------------- | --- |
| `analysis_run.go`           | Types, `AnalysisRunStore`, constructors | 391 |
| `analysis_run_queries.go`   | Read queries, list, get, filter, search | 599 |
| `analysis_run_mutations.go` | Insert, update, complete, delete        | 246 |
| `analysis_run_compare.go`   | Comparison queries                      | 282 |
| `analysis_run_manager.go`   | Manager orchestration                   | 260 |

**Checklist:**

- [x] Create `analysis_run_queries.go` — move read operations (599 LOC)
- [x] Create `analysis_run_mutations.go` — move write operations (246 LOC)
- [x] Create `analysis_run_compare.go` — move CompareRuns and comparison helpers (292 LOC)
- [x] Verify `analysis_run.go` contains only types and constructors (391 LOC)
- [x] `make lint-go && make test-go` passes
- [x] No file in `internal/lidar/storage/sqlite/` exceeds 600 LOC (for Phase 1 files;
      `track_store.go` at 774 is a Tier 2 file tracked separately)

### 1F. ~~Split `internal/lidar/l3grid/background.go` (1,672 LOC)~~ — DONE

Split from 1,672 LOC into 2 domain files + core. `background_manager.go` at 861 LOC
exceeds the original 600 LOC target — tracked in Tier 2 for a follow-on split.

| File                    | Contents                                                  | LOC |
| ----------------------- | --------------------------------------------------------- | --- |
| `background.go`         | `BackgroundParams`, `BackgroundCell`, `BackgroundGrid`    | 352 |
| `background_region.go`  | `RegionManager`, `Region`, `RegionParams`                 | 474 |
| `background_manager.go` | `BackgroundManager`, `AcceptanceMetrics`, global registry | 861 |

**Checklist:**

- [x] Create `background_region.go` — move RegionManager and region types (474 LOC)
- [x] Create `background_manager.go` — move BackgroundManager, registry, constructors (861 LOC)
- [x] Verify `background.go` contains only params, cell, and grid types (352 LOC)
- [x] `go build ./... && go test ./internal/lidar/l3grid/...` passes
- [ ] `background_manager.go` at 861 LOC exceeds 600 LOC target — candidate for follow-on split

### 1G. ~~Split `internal/config/tuning.go` (1,303 LOC)~~ — DONE

Split from 1,303 LOC into 3 domain files + core. All files under 400 LOC. Tech debt
removal (#416) subsequently removed `L1Config` network fields (`UDPPort`, `UDPRcvBuf`,
`ForwardPort`, `ForegroundForwardPort`), reducing `tuning_validate.go` and
`tuning_accessors.go` by ~30 LOC total.

| File                  | Contents                                       | LOC |
| --------------------- | ---------------------------------------------- | --- |
| `tuning.go`           | Consts, type definitions, loaders              | 250 |
| `tuning_validate.go`  | All `Validate()` methods                       | 391 |
| `tuning_codec.go`     | `UnmarshalJSON`, codec helpers                 | 280 |
| `tuning_accessors.go` | `ActiveConfig`, `ActiveCommon`, `Get*` getters | 361 |

**Checklist:**

- [x] Create `tuning_validate.go` — move all Validate methods (391 LOC)
- [x] Create `tuning_accessors.go` — move ActiveConfig/ActiveCommon + getters (361 LOC)
- [x] Create `tuning_codec.go` — move JSON unmarshalling + codec helpers (280 LOC)
- [x] Verify `tuning.go` contains only types, consts, and loaders (250 LOC)
- [x] `go build ./... && go test ./internal/config/...` passes
- [x] No file in split exceeds 400 LOC

---

## Phase 2 — Tier 2 Large Files

Target: v0.5.2 or later. Lower priority; these files are large but generally single-concern.
Evaluate after Phase 1 completes — some may be fine as-is if their size reflects genuine
domain complexity rather than mixed concerns.

### 2A. ~~`internal/lidar/l2frames/frame_builder.go` (1,021 LOC)~~ — Partially Done

Cleanup logic extracted to `frame_builder_cleanup.go` (602 LOC). Core `frame_builder.go`
now 448 LOC. The cleanup file itself is at the threshold. Registry extraction did not
happen.

### 2B. `internal/lidar/sweep/hint.go` (798 LOC) and `auto.go` (762 LOC)

Both implement multi-round state machines. Splitting options:

- Extract state transitions into `hint_state.go` / `auto_state.go`
- Extract round evaluation into `hint_evaluation.go` / `auto_evaluation.go`
- Keep orchestration in `hint.go` / `auto.go`

### 2C. `internal/lidar/server/track_api.go` (763 LOC)

Split candidates:

- `track_api_queries.go` — query parameter parsing and DB queries
- `track_api_format.go` — response formatting and serialisation
- `track_api.go` — handler registration and dispatch

### 2D. `internal/lidar/server/run_track_api.go` (752 LOC)

Similar pattern to track_api.go — split by query vs format vs dispatch.

### 2E. `internal/lidar/l9endpoints/grpc_server.go` (619 LOC)

Split per-RPC handler implementations into separate files. `publisher.go` (540 LOC) now
below threshold.

### 2F. `internal/lidar/pipeline/tracking_pipeline.go` (733 LOC)

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
- `internal/lidar/server/playback_handlers.go` (604 LOC)
- `internal/lidar/l2frames/frame_builder_cleanup.go` (602 LOC) — from 2A split
- `internal/lidar/l1packets/parse/extract.go` (601 LOC)
- `internal/lidar/storage/sqlite/analysis_run_queries.go` (599 LOC) — from 1E split; was 775, now below Tier 2
- `internal/lidar/server/tuning_runtime.go` (582 LOC)
- `internal/lidar/server/echarts_handlers.go` (580 LOC)
- `internal/db/db_radar.go` (562 LOC) — from 1A split
- `internal/db/migrate.go` (546 LOC)
- `internal/lidar/l3grid/foreground.go` (542 LOC)
- `internal/lidar/l6objects/classification.go` (539 LOC)
- `internal/lidar/l1packets/network/pcap_realtime.go` (525 LOC)

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

Phase 1 target: no production file above 600 LOC in the seven split packages.
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
