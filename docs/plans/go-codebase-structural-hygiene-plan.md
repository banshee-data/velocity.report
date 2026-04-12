# Go Codebase Structural Hygiene Plan (v0.5.x)

- **Status:** Active — substantially complete on `main`; `dd/fix-more-of-it` carries the final JSON tag and error-drop fixes
- **Layers:** Cross-cutting (Go server, API, database, LiDAR pipeline)
- **Target:** v0.5.x — lock in the right structural defaults before v0.5.0 hardens them
- **Companion plans:**
  [go-structured-logging-plan.md](go-structured-logging-plan.md),
  [go-god-file-split-plan.md](go-god-file-split-plan.md),
  [data-database-alignment-plan.md](data-database-alignment-plan.md),
  [lidar-l8-analytics-l9-endpoints-l10-clients-plan.md](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md)

- **Canonical:** [go-package-structure.md](../platform/architecture/go-package-structure.md)

## Motivation

The original draft of this plan identified several structural risks. The large majority of
that work has now landed on `main` through PRs #406, #409, #412, #413, #416, #418, and
others. A small tail — JSON tag fixes, silent error-drop fixes, and documentation updates —
remains on the `dd/fix-more-of-it` branch.

This document now serves primarily as an evidence record: what was done, what remains, and
what is deferred to companion plans. Structural cleanup is cheapest before release
conventions become contracts; most of that window has now been used.

## Implementation Snapshot (2026-03-23)

| Area                                  | `main` | `dd/fix-more-of-it` | Notes                                                                                                                                                                                                                                                                                   |
|---------------------------------------|--------|---------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `database/sql` import boundary        | Done   | Done                | Non-test imports limited to `internal/db/` and `internal/lidar/storage/`; enforced by `scripts/check-db-sql-imports.sh`                                                                                                                                                                 |
| SQL type aliases / sentinel           | Done   | Done                | `sqlite.SQLDB`, `sqlite.SQLTx`, `sqlite.ErrNotFound` exported from `internal/lidar/storage/sqlite/dbconn.go`                                                                                                                                                                            |
| `cmd/tools/` boundary compliance      | Done   | Done                | `cmd/tools/backfill_ring_elevations/backfill.go` no longer imports `database/sql` directly                                                                                                                                                                                              |
| LiDAR server / endpoint package split | Done   | Done                | `server/`, `l8analytics/`, `l9endpoints/` all exist; `monitor/` and `visualiser/` removed; `server/` still large (53 files) — further decomposition via [go-god-file-split-plan](go-god-file-split-plan.md) and [L8/L9/L10 plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md)   |
| Single SQLite driver policy           | Done   | Done                | `mattn/go-sqlite3` removed from `go.mod`; `scripts/check-single-sqlite-driver.sh` wired into `make lint-go`                                                                                                                                                                             |
| Context propagation                   | Done   | Done                | Zero `_ = r` placeholders; 10 site/report DB methods accept `context.Context`; cancellation test present                                                                                                                                                                                |
| `serialmux.CurrentState` race         | Done   | Done                | `sync.RWMutex`-backed `currentState`; `CurrentStateSnapshot()` exported; test helper `resetCurrentState()`                                                                                                                                                                              |
| `EventAPI` JSON tags                  | **No** | Done                | PascalCase → lowercase `snake_case` (`"magnitude"`, `"uptime"`, `"speed"`); TypeScript `Event` interface updated                                                                                                                                                                        |
| `RadarObjectsRollupRow` JSON tags     | **No** | Done                | Bare struct → explicit `json:"snake_case"` tags; TypeScript `RawRadarStats` mapping updated                                                                                                                                                                                             |
| `export_bg_snapshot.go` error drops   | **No** | Done                | `_ = mgr.SetRingElevations(...)` → `if err :=` handling with `diagf` logging                                                                                                                                                                                                            |
| `datasource_handlers.go` error drops  | **No** | Done                | `_ = ws.startLiveListenerLocked()` and `_ = bgManager.SetParams(...)` → proper error handling with `opsf` logging                                                                                                                                                                       |

## Updated Findings

| Category                   | Current state                                                                                                                                                                                               | Severity     | Release view                                                                                                                               |
|----------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------|--------------------------------------------------------------------------------------------------------------------------------------------|
| Context propagation        | Implemented: `internal/api/server.go` handlers now propagate `r.Context()` to DB calls; 10 site/report DB methods use `*Context` variants                                                                   | ~~Critical~~ | **Done**                                                                                                                                   |
| God files / package sprawl | `internal/lidar/monitor/` removed; `server/`, `l8analytics/`, `l9endpoints/` all exist; `internal/lidar/server/` remains large (53 files) and undivided — further decomposition is the live concern         | High         | Continue via [go-god-file-split-plan](go-god-file-split-plan.md) and [L8/L9/L10 plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md) |
| Global mutable state       | Implemented: `internal/serialmux/handlers.go` now uses `sync.RWMutex`-backed `currentState`; read via `CurrentStateSnapshot()`                                                                              | ~~High~~     | **Done**                                                                                                                                   |
| Query-boundary leak        | `internal/api/lidar_labels.go` still holds 10 raw SQL call sites; `internal/lidar/storage/sqlite/label_store.go` exists as a parallel implementation but the API layer has not been wired to delegate to it | Medium       | Outstanding — wire `LidarLabelAPI` to `LabelStore` (v0.5.x)                                                                                |
| JSON tag inconsistency     | `EventAPI` and `RadarObjectsRollupRow` now have explicit `snake_case` JSON tags; TypeScript interfaces updated to match                                                                                     | ~~Low~~      | **Done**                                                                                                                                   |
| Silent error drops         | Operational sites in `export_bg_snapshot.go` and `datasource_handlers.go` fixed; `echarts_handlers.go` `w.Write` and `db.Close()` remain (acceptable)                                                       | ~~Medium~~   | **Done** (operationally meaningful subset)                                                                                                 |
| Test infrastructure drift  | 40 internal test files still use `time.Sleep` (203 call sites); test DB setup is still inconsistent                                                                                                         | Low          | Worth reducing in early v0.5.x                                                                                                             |

## Analysis Notes

### 1. The `database/sql` import boundary is closed

`scripts/check-db-sql-imports.sh` fails CI if non-test code outside `internal/db/` and
`internal/lidar/storage/` imports `database/sql`. Completed baseline on `main`.

### 2. The remaining database issue is query placement, not import placement

`internal/api/lidar_labels.go` still executes raw SQL directly — 10 call sites via
`api.db.Query/QueryRow/Exec` (file is 716 lines; grep for `api.db.Query`).

`internal/lidar/storage/sqlite/label_store.go` exists with a `LabelStore` struct and its own
SQL for label CRUD, but `LidarLabelAPI` has not been wired to delegate to it. Both sides
independently execute SQL against the same tables. The HTTP layer still owns SQL strings.
This is the only material storage-hygiene gap remaining.

### 3. Package split is complete; `server/` decomposition continues via companions

`internal/lidar/monitor/` and `internal/lidar/visualiser/` are gone. `server/`,
`l8analytics/`, `l9endpoints/` all exist on `main`. `internal/lidar/server/` remains large
(53 files); further decomposition is tracked by
[lidar-l8-analytics-l9-endpoints-l10-clients-plan.md](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md).

### 4. Single SQLite driver policy is enforced

`github.com/mattn/go-sqlite3` is gone from `go.mod`. `scripts/check-single-sqlite-driver.sh`
is wired into `make lint-go`. 14 direct Go dependencies remain.

## Detailed Findings

### 1. Request context propagation — completed

All 8 `_ = r` placeholders have been removed from `internal/api/server.go`. The 10 affected
DB methods (`CreateSite`, `GetSite`, `GetAllSites`, `UpdateSite`, `DeleteSite`,
`CreateSiteReport`, `GetSiteReport`, `GetRecentReportsForSite`, `GetRecentReportsAllSites`,
`DeleteSiteReport`) now accept `context.Context` as their first parameter and use
`ExecContext`, `QueryContext`, and `QueryRowContext` consistently.

A cancellation test (`TestListSites_ContextCancellation`) confirms that a cancelled request
context propagates correctly to the database layer.

The LiDAR server (`internal/lidar/server/`) already uses `r.Context()` in the relevant
handlers. No additional work needed there.

### 2. `serialmux.CurrentState` race — completed

`internal/serialmux/handlers.go` now uses:

```go
var (
    currentStateMu sync.RWMutex
    currentState   map[string]any
)
```

`HandleConfigResponse` holds the write lock while mutating the map. Callers read via
`CurrentStateSnapshot()` (returns a shallow copy under `RLock`). Tests reset state via
`resetCurrentState()`. Verified with `go test -race`.

### 3. ~~`EventAPI` JSON naming convention~~ — Done

`EventAPI` now uses `snake_case` JSON tags (`"magnitude"`, `"uptime"`, `"speed"`),
consistent with `RadarObjectsRollupRow` and the rest of the API surface.
TypeScript `Event` interface updated to match.

### 4. Silent error drops are still concentrated in a few important paths

Two silent error drop locations remain:

- `internal/db/db.go:181` — `_ = db.Close()` inside the `NewDBWithMigrationCheck`
  error-return path (line number shifted from the original 251)

- `internal/lidar/server/echarts_handlers.go` — 8 ignored `_, _ = w.Write(...)` results

The `export_bg_snapshot.go` (`SetRingElevations`) and `datasource_handlers.go` sites have
been fixed since this assessment was first written — all now use `if err :=` handling or
`opsf(...)` logging.

`w.Write` after headers are sent is a log-or-ignore case; `db.Close()` on the error-return
path is low-consequence but slightly untidy. Neither blocks v0.5.0.

### 5. Test infrastructure has drifted more than the original draft suggested

The original draft cited 9 `time.Sleep` test files. That is now stale.

Current scan:

- 40 internal test files use `time.Sleep`
- 203 `time.Sleep(...)` call sites exist under `internal/*_test.go`

That is now large enough to call a real test-infrastructure smell rather than a minor note.
It does not block v0.5.0 by itself, but it does justify a shared polling helper and a
phased reduction.

## Direct Go Dependency Review

This section reviews **direct `go.mod` dependencies for the Go module only**. Frontend
`package.json` trees are out of scope for this plan.

There are 14 direct Go dependencies. The duplicate SQLite driver (`mattn/go-sqlite3`) has
been removed from `main`.

| Dependency                             | Why it exists                                                                                                                 | Scope              | Keep?                                                             |
|----------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|--------------------|-------------------------------------------------------------------|
| `github.com/go-echarts/go-echarts/v2`  | HTML chart rendering in `internal/lidar/server/echarts_handlers.go`                                                           | Production         | Yes                                                               |
| `github.com/golang-migrate/migrate/v4` | Schema migration engine in `internal/db/migrate.go`                                                                           | Production         | Yes                                                               |
| `github.com/google/go-cmp`             | Test comparison helper in `cmd/radar/radar_test.go`                                                                           | Test-only          | Yes, low cost                                                     |
| `github.com/google/gopacket`           | PCAP parsing/counting in `internal/lidar/l1packets/network/*.go`                                                              | Production         | Yes                                                               |
| `github.com/google/uuid`               | Run, label, sweep, and track IDs across API and LiDAR storage packages                                                        | Production         | Yes                                                               |
| `github.com/stretchr/testify`          | Assertion helpers in LiDAR/server tests                                                                                       | Test-only          | Yes, unless the project decides to standardise on plain `testing` |
| `github.com/tailscale/tailsql`         | `/debug/tailsql/` admin SQL surface in `internal/db/db.go`                                                                    | Production/debug   | Yes while the route exists                                        |
| `go.bug.st/serial`                     | Physical serial-port access in `internal/serialmux/factory.go`                                                                | Production         | Yes                                                               |
| `gonum.org/v1/gonum`                   | Statistics helpers in `internal/db/db.go`                                                                                     | Production         | Yes                                                               |
| `gonum.org/v1/plot`                    | Plot generation in `internal/lidar/l9endpoints/gridplotter.go`                                                                | Production         | Yes                                                               |
| `google.golang.org/grpc`               | Visualiser streaming server/client surface in `internal/lidar/l9endpoints/*`                                                  | Production         | Yes                                                               |
| `google.golang.org/protobuf`           | Generated protobuf types and recorder codec support                                                                           | Production         | Yes                                                               |
| `modernc.org/sqlite`                   | Canonical SQLite driver for production and tests                                                                              | Production + tests | Yes                                                               |
| `tailscale.com`                        | `tsweb` debug/admin plumbing in `internal/db/db.go`, `internal/lidar/server/routes.go`, and `internal/serialmux/serialmux.go` | Production/debug   | Yes                                                               |

### Quick-win dependency removal — Done

`github.com/mattn/go-sqlite3` has been removed from `go.mod` on `main`. No further
dependency removals are available.

## v0.5.x Backlog Items

### Item 1: Request Lifecycle — make context the default path ✓ Complete

**Summary:** Finish the job of threading `context.Context` from HTTP entrypoints into
database and long-running operations.

**Scope:**

1. ~~Remove the 8 `_ = r` placeholders in `internal/api/server.go`~~ — Done
2. ~~Audit branch LiDAR server handlers (`internal/lidar/server/*`) and ensure request-scoped
   work uses `r.Context()`~~ — Already used `r.Context()` in 3 relevant call sites

3. ~~Add `context.Context` parameters to database methods that execute queries where still
   missing~~ — Done: 10 site/report methods updated

4. ~~Use `ExecContext`, `QueryContext`, and `QueryRowContext` consistently in database code~~ — Done
5. ~~Add at least one integration-level cancellation test~~ — Done: `TestListSites_ContextCancellation`

**Milestone:** v0.5.0

### Item 2: Package Hygiene — storage boundaries, shared state, and error visibility ✓ Substantially complete

**Summary:** Finish the storage/package cleanup that the import-boundary work started.

**Scope:**

1. ~~Protect `serialmux.CurrentState` with a mutex-backed accessor surface~~ — Done on `main`
2. Wire `LidarLabelAPI` to delegate to `LabelStore` — **outstanding** (v0.5.x, separate PR)
3. ~~Fix the remaining meaningful silent error drops~~ — Done (`dd/fix-more-of-it`;
   `export_bg_snapshot.go` and `datasource_handlers.go` fixed; `db.go:181` and echarts
   `w.Write` remain as low-consequence residuals)

4. ~~Keep the single-SQLite-driver invariant~~ — Done on `main`;
   `scripts/check-single-sqlite-driver.sh` wired into `make lint-go`

5. Expand shared test helpers and start replacing `time.Sleep` with polling/wait helpers —
   **deferred** (v0.5.x, separate effort)

6. Continue the package split tracked by
   [go-god-file-split-plan.md](go-god-file-split-plan.md) and
   [lidar-l8-analytics-l9-endpoints-l10-clients-plan.md](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md) —
   **deferred** to companion plans

**Milestone:** split across v0.5.0 and v0.5.1

### Item 3: API Contract Consistency — Done

**Summary:** Standardise JSON tag conventions for API-facing structs.

Both `RadarObjectsRollupRow` and `EventAPI` now carry explicit `snake_case` JSON tags.
TypeScript interfaces (`RawRadarStats` and `Event`) updated to match.

**Scope:**

1. ~~`EventAPI` tags — `snake_case` (`"magnitude"`, `"uptime"`, `"speed"`)~~ — Done
2. ~~Add `json` tags to `RadarObjectsRollupRow` (was relying on Go default PascalCase)~~ — Done
3. ~~Update TypeScript `RawRadarStats` type and mapping to match~~ — Done
4. ~~Update TypeScript `Event` interface to match~~ — Done
5. ~~Audit exported JSON-tagged structs for any other naming outliers~~ — Audited; no further issues found

**Milestone:** v0.5.0

**Why now:** this is cheap before release and annoying forever after.

## Release View

All four pre-v0.5.0 structural wins have landed on `main`:

1. ~~Single-SQLite-driver cleanup~~ — Done on `main`
2. ~~Request-context propagation~~ — Done on `main`
3. ~~`RadarObjectsRollupRow` and `EventAPI` JSON tags~~ — Done on `dd/fix-more-of-it`
4. ~~`serialmux.CurrentState` race fix~~ — Done on `main`

The `dd/fix-more-of-it` branch adds the JSON tag fixes (#3 above) plus silent error-drop
fixes in `export_bg_snapshot.go` and `datasource_handlers.go`. Once merged, all planned
pre-v0.5.0 work from this plan will be on `main`.

## Checklist

### Complete (on `main`)

- [x] `database/sql` import boundary enforced (`scripts/check-db-sql-imports.sh`)
- [x] SQL type aliases and `sqlite.ErrNotFound` sentinel exported
- [x] `cmd/tools/` boundary compliance
- [x] LiDAR package split: `server/`, `l8analytics/`, `l9endpoints/`; `monitor/` removed
- [x] Single SQLite driver: `mattn/go-sqlite3` removed, `check-single-sqlite-driver.sh` enforced
- [x] Context propagation: zero `_ = r` placeholders, 10 DB methods accept `context.Context`
- [x] `serialmux.CurrentState` race: `sync.RWMutex` + `CurrentStateSnapshot()`
- [x] Cancellation test: `TestListSites_ContextCancellation`
- [x] `rows.Err()` checks in `site_report.go` iteration functions

### Complete (on `dd/fix-more-of-it`, pending merge)

- [x] `EventAPI` JSON tags: PascalCase → lowercase `snake_case`
- [x] `RadarObjectsRollupRow` JSON tags: bare struct → explicit `snake_case`
- [x] TypeScript `Event` interface updated to match
- [x] TypeScript `RawRadarStats` mapping updated to match
- [x] `export_bg_snapshot.go`: `_ = mgr.SetRingElevations(...)` → `if err :=` handling
- [x] `datasource_handlers.go`: silent listener/params errors → `opsf(...)` logging
- [x] `version-bump.sh` script added

### Outstanding (v0.5.x, separate PRs)

- [ ] Wire `LidarLabelAPI` to delegate to `LabelStore` — 10 raw SQL call sites in
      `internal/api/lidar_labels.go`; `label_store.go` exists but is unused (`M` effort)

- [ ] `time.Sleep` test infrastructure reduction — 40 files, 203 call sites; shared
      polling/wait helper needed (`L` effort, phased)

- [ ] `internal/lidar/server/` further decomposition — 53 files; tracked by
      [lidar-l8-analytics-l9-endpoints-l10-clients-plan.md](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md)

### Accepted residuals (no action planned)

- [ ] `internal/db/db.go:181` — `_ = db.Close()` on error-return path in
      `NewDBWithMigrationCheck`; low consequence, function already returning an error

- [ ] `internal/lidar/server/echarts_handlers.go` — 8 × `_, _ = w.Write(...)` after
      headers sent; standard Go HTTP write-after-headers pattern, log-or-ignore