# Go Codebase Structural Hygiene Plan (v0.5.x)

- **Status:** Active — partially implemented on `main`, materially advanced on this branch
- **Layers:** Cross-cutting (Go server, API, database, LiDAR pipeline)
- **Target:** v0.5.x — lock in the right structural defaults before v0.5.0 hardens them
- **Companion plans:**
  [go-structured-logging-plan.md](go-structured-logging-plan.md),
  [go-god-file-split-plan.md](go-god-file-split-plan.md),
  [data-database-alignment-plan.md](data-database-alignment-plan.md),
  [lidar-l8-analytics-l9-endpoints-l10-clients-plan.md](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md)
- **Canonical:** [go-package-structure.md](../platform/architecture/go-package-structure.md)

## Motivation

The original draft of this plan correctly identified several structural risks, but it is no
longer an accurate snapshot of the codebase. Some of the proposed hygiene work has already
landed on `main`. This branch goes further again, especially in the LiDAR package split and
SQLite-driver cleanup.

That changes the job of this document. It should no longer read like a pure wishlist. It
should say, concretely:

1. what is already implemented on `main`
2. what this branch has implemented beyond `main`
3. what still needs to land before v0.5.0 or early v0.5.x

The main principle remains the same: structural cleanup is cheapest before release
conventions become contracts.

## Implementation Snapshot (2026-03-23)

| Area                                  | `main`          | This branch | Notes                                                                                                                                                                                                                                                                        |
| ------------------------------------- | --------------- | ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `database/sql` import boundary        | Implemented     | Implemented | Non-test imports are limited to `internal/db/` and `internal/lidar/storage/`; enforced by `scripts/check-db-sql-imports.sh`                                                                                                                                                  |
| SQL type aliases / sentinel           | Implemented     | Implemented | `internal/lidar/storage/sqlite/dbconn.go` exports `sqlite.SQLDB`, `sqlite.SQLTx`, and `sqlite.ErrNotFound`                                                                                                                                                                   |
| `cmd/tools/` boundary compliance      | Implemented     | Implemented | `cmd/tools/backfill_ring_elevations/backfill.go` no longer imports `database/sql` directly                                                                                                                                                                                   |
| LiDAR server / endpoint package split | Not implemented | Implemented | `internal/lidar/server/`, `internal/lidar/l8analytics/`, and `internal/lidar/l9endpoints/` all exist; `internal/lidar/monitor/` and `internal/lidar/visualiser/` removed; `internal/lidar/server/` still large (53 files) — further decomposition tracked by companion plans |
| Single SQLite driver policy           | Not implemented | Implemented | `main` still carries `github.com/mattn/go-sqlite3` in tests and `go.mod`; this branch standardises on `modernc.org/sqlite` and adds `scripts/check-single-sqlite-driver.sh`                                                                                                  |
| Context propagation                   | Not implemented | Implemented | 8 `_ = r` placeholders removed from `internal/api/server.go`; 10 DB methods in `site.go`/`site_report.go` now accept `context.Context` and use `*Context` SQL methods                                                                                                        |
| `serialmux.CurrentState` race         | Not implemented | Implemented | Replaced unsynchronised map with `sync.RWMutex`-backed private state; exported `CurrentStateSnapshot()`; unexported test helper `resetCurrentState`                                                                                                                          |

## Updated Findings

| Category                   | Current state                                                                                                                                                                                               | Severity     | Release view                                                |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------ | ----------------------------------------------------------- |
| Context propagation        | Implemented: `internal/api/server.go` handlers now propagate `r.Context()` to DB calls; 10 site/report DB methods use `*Context` variants                                                                   | ~~Critical~~ | **Done**                                                    |
| God files / package sprawl | `internal/lidar/monitor/` removed; `server/`, `l8analytics/`, `l9endpoints/` all exist; `internal/lidar/server/` remains large (53 files) and undivided — further decomposition is the live concern         | High         | Continue via companion plans                                |
| Global mutable state       | Implemented: `internal/serialmux/handlers.go` now uses `sync.RWMutex`-backed `currentState`; read via `CurrentStateSnapshot()`                                                                              | ~~High~~     | **Done**                                                    |
| Query-boundary leak        | `internal/api/lidar_labels.go` still holds 10 raw SQL call sites; `internal/lidar/storage/sqlite/label_store.go` exists as a parallel implementation but the API layer has not been wired to delegate to it | Medium       | Outstanding — wire `LidarLabelAPI` to `LabelStore` (v0.5.x) |
| JSON tag inconsistency     | `EventAPI` and `RadarObjectsRollupRow` now have explicit `snake_case` JSON tags; TypeScript interfaces updated to match                                                                                     | ~~Low~~      | **Done**                                                    |
| Silent error drops         | Operational sites in `export_bg_snapshot.go` and `datasource_handlers.go` fixed; `echarts_handlers.go` `w.Write` and `db.Close()` remain (acceptable)                                                       | ~~Medium~~   | **Done** (operationally meaningful subset)                  |
| Test infrastructure drift  | 40 internal test files still use `time.Sleep` (203 call sites); test DB setup is still inconsistent                                                                                                         | Low          | Worth reducing in early v0.5.x                              |

## What The Original Draft Got Right, And What Changed

### 1. The `database/sql` boundary problem is no longer open

The old draft flagged direct `database/sql` imports outside the database layer as a live
problem. That is now stale.

This is already implemented on both `main` and this branch:

- `scripts/check-db-sql-imports.sh` fails CI if non-test code outside `internal/db/` and
  `internal/lidar/storage/` imports `database/sql`
- `internal/lidar/storage/sqlite/dbconn.go` exports `sqlite.SQLDB`, `sqlite.SQLTx`, and
  `sqlite.ErrNotFound` so callers do not need `database/sql`
- `internal/lidar/pipeline/tracking_pipeline.go` already consumes `*sqlite.SQLDB` rather
  than importing `database/sql` directly

That means the old "DB abstraction leak" item should be treated as **completed baseline**,
not future work.

### 2. The remaining database issue is now query placement, not import placement

`internal/api/lidar_labels.go` still executes raw SQL directly — 10 call sites via `api.db.Query/QueryRow/Exec`. The line numbers from the original draft have shifted (the file is now 716 lines), so exact references are omitted here; the pattern is visible in any grep for `api.db.Query`.

`internal/lidar/storage/sqlite/label_store.go` now exists and defines a `LabelStore` struct with a `NewLabelStore(db DBClient)` constructor, its own SQL for label CRUD, and a parallel `LidarLabel` model with proper snake_case JSON tags. The two implementations are in sync at the schema level but the API layer (`LidarLabelAPI`) has not been wired to delegate to `LabelStore`. Both sides independently execute SQL against the same tables.

The import boundary is respected, but the package boundary is not. The HTTP layer still owns SQL strings, and `label_store.go` is unused dead weight until the wiring happens. Migration is the real storage-hygiene gap.

### 3. This branch is already executing part of the package-split plan

Compared with `refs/heads/main`, this branch has already moved substantial LiDAR code:

- `internal/lidar/monitor/*` to `internal/lidar/server/*`
- `internal/lidar/visualiser/*` to `internal/lidar/l9endpoints/*`
- new `internal/lidar/l8analytics/*`
- new `internal/lidar/l9endpoints/l10clients/*` for client assets/templates

So the package-split work is no longer theoretical on this branch. The immediate hygiene
question is no longer "should we split?" but "what is the merge and follow-through plan, and
which remaining cross-package leaks should be cleaned before v0.5.0?"

### 4. The duplicate SQLite adapter was a real quick win, and it should not return

Before this branch change:

- `main` had `github.com/mattn/go-sqlite3` in `go.mod`
- `main` still used blank `mattn` imports and `sql.Open("sqlite3", ":memory:")` in test
  files under `internal/api/`, `internal/lidar/monitor/`, and
  `internal/lidar/storage/sqlite/`
- production already used `modernc.org/sqlite`

That was exactly the kind of pre-0.5.0 duplicate dependency worth removing. This branch now:

- removes `github.com/mattn/go-sqlite3` from `go.mod`
- converts test code to `modernc.org/sqlite`
- standardises the driver name on `"sqlite"`
- adds `scripts/check-single-sqlite-driver.sh`
- wires that check into `make lint-go`

This should be treated as a release-hardening cleanup, not optional polish.

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

On this branch there are 14 direct Go dependencies after removing the duplicate SQLite
driver. `main` still has 15 until this cleanup is merged.

| Dependency                             | Why it exists                                                                                                                 | Scope              | Keep?                                                             |
| -------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ------------------ | ----------------------------------------------------------------- |
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

### Quick-win dependency removal

`github.com/mattn/go-sqlite3` was the only clear removal candidate:

- it duplicated `modernc.org/sqlite`
- it was only used in tests
- it reintroduced a second SQLite behaviour surface
- it pulled CGO baggage into a codebase that had already standardised production on pure-Go
  SQLite

That quick win is now done on this branch and should be merged before v0.5.0.

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

### Item 2: Package Hygiene — storage boundaries, shared state, and error visibility

**Summary:** Finish the storage/package cleanup that the import-boundary work started.

**Scope:**

1. ~~Protect `serialmux.CurrentState` with a mutex-backed accessor surface~~ — Done
2. Wire `LidarLabelAPI` to delegate to `LabelStore` — `label_store.go` exists as a
   parallel implementation but `lidar_labels.go` still owns 10 raw SQL call sites;
   API layer migration is outstanding
3. ~~Fix the remaining meaningful silent error drops~~ — Done (operational subset;
   export_bg_snapshot.go and datasource_handlers.go fixed; db.go:181 and echarts
   w.Write remain as low-consequence residuals)
4. Keep the single-SQLite-driver invariant:
   - merge this branch's `modernc`-only cleanup
   - keep `scripts/check-single-sqlite-driver.sh` in `make lint-go`
5. Expand shared test helpers and start replacing `time.Sleep` with polling/wait helpers
6. Continue the package split tracked by
   [go-god-file-split-plan.md](go-god-file-split-plan.md) and
   [lidar-l8-analytics-l9-endpoints-l10-clients-plan.md](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md)

**Milestone:** split across v0.5.0 and v0.5.1

**Why it matters:** the import boundary is already fixed; this item keeps the internals from
sliding back into mixed responsibilities.

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

Before v0.5.0, the most valuable structural wins are now:

1. ~~merge the single-SQLite-driver cleanup from this branch~~ — Done
2. ~~finish request-context propagation~~ — Done
3. ~~fix `RadarObjectsRollupRow` and `EventAPI` JSON tags~~ — Done
4. ~~remove the `serialmux.CurrentState` race~~ — Done

Everything else is still worth doing, but those four items most directly reduce the chance
that v0.5.0 bakes in avoidable technical debt.
