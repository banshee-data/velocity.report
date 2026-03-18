# Go Codebase Structural Hygiene Plan

- **Status:** Draft
- **Layers:** Cross-cutting (Go server, API, database, LiDAR pipeline)
- **Target:** v0.5.0 onward — disrupt shaky conventions before they become foundations

## Motivation

The Go codebase is 61,000 lines of production code across 197 files. The LiDAR pipeline
architecture (L1–L6) is well-layered. Test coverage is strong (120,000 lines of test code,
216 test files, 1.93× test-to-production ratio). Dependency graph is acyclic. These are
genuine structural strengths.

But beneath that surface sit conventions that will compound if left to settle. God files
growing a few methods per milestone. HTTP handlers that never propagate context. A logging
layer that mixes three patterns. A global mutable map with no synchronisation. JSON tags
that contradict each other in the same package. These are not emergencies. They are the kind
of slow settling that turns a passable road into an expensive repair job two versions later.

This plan names the structural issues, groups them into four backlog items sized for
milestone scheduling, and orders them by the cost of deferral.

## Analysis Summary

| Category               | Finding                                                              | Severity |
| ---------------------- | -------------------------------------------------------------------- | -------- |
| Context propagation    | 30+ HTTP handlers ignore `r.Context()`                               | Critical |
| God files              | `db.go` (1,420 LOC), `api/server.go` (1,711), `webserver.go` (1,905) | High     |
| Race condition         | `serialmux.CurrentState` is a global map with no sync                | High     |
| DB abstraction leak    | 4 files import `database/sql` directly, bypassing `db` package       | Medium   |
| JSON tag inconsistency | `EventAPI` uses PascalCase; everything else uses snake_case          | Medium   |
| Silent error drops     | 5 production-code instances of `_ = expr` discarding errors          | Medium   |
| Logging inconsistency  | Mix of `log.Printf`, `fmt.Printf`, `monitoring.Logf`, emoji in logs  | Medium   |
| God functions          | `setupRoutes` (415 LOC), `buildCosineSpeedExpr` (318 LOC)            | Medium   |
| Test infrastructure    | `testutil.go` is 46 lines for 216 test files; DB setup inconsistent  | Low      |
| Flaky test risk        | `time.Sleep` in 9 test files                                         | Low      |

## Detailed Findings

### 1. Context Not Propagated (Critical)

Every HTTP handler in `internal/api/server.go` accepts `(w http.ResponseWriter, r
*http.Request)` but none of the 26 handler methods extract or forward `r.Context()`. The
same pattern holds across the LiDAR monitor handlers.

**Consequence:** No request timeout enforcement. No graceful shutdown propagation. No
cancellation of in-flight database queries when a client disconnects. On a Raspberry Pi
with a single SQLite writer, a hung query blocks the entire write path with no way to
interrupt it.

**Burden if deferred:** Every new handler written without context deepens the convention.
Retrofitting context after v0.5.0 means touching every handler and every DB method
signature — a change that grows linearly with feature count.

### 2. God Files (High)

Three files carry disproportionate weight:

| File                                  | Lines | Methods/Handlers    | Concerns mixed                                                           |
| ------------------------------------- | ----- | ------------------- | ------------------------------------------------------------------------ |
| `internal/db/db.go`                   | 1,420 | 20 receiver methods | Radar events, LiDAR backgrounds, regions, admin routes, stats            |
| `internal/api/server.go`              | 1,711 | 27 handler methods  | Sites, config, events, reports, serial commands, transit, DB stats       |
| `internal/lidar/monitor/webserver.go` | 1,905 | ~40 routes          | Status, snapshots, metrics, sweeps, grids, pcap, charts, debug, playback |

The `db` package already has good per-domain files (`site.go`, `transit_worker.go`,
`site_config_periods.go`). The god file is `db.go` itself, which mixes radar recording,
background snapshot CRUD, region snapshots, admin HTTP routes, and database statistics into
one 1,420-line file.

**Consequence:** Merge conflicts. Cognitive load. Contributors cannot reason about radar
queries without scrolling past background snapshot code. The file will only grow as new
query methods are added.

**Burden if deferred:** Each milestone adds 2–3 methods to `db.go`. By v0.7.0 it will be
2,000+ lines and splitting it will require touching every import site.

### 3. Global Mutable State Without Synchronisation (High)

```go
// internal/serialmux/handlers.go:13
var CurrentState map[string]any
```

This is a package-level mutable map. It is written by `HandleConfigResponse` and read by
admin routes. There is no `sync.RWMutex`, no atomic access, no channel discipline. Under
concurrent access — which is the normal operating mode, since HTTP handlers and serial
readers run in separate goroutines — this is a data race.

A second instance exists in `monitoring/logger.go` where `Logf` is a mutable function
pointer, though this one is lower risk since it is typically set once at startup.

**Consequence:** Potential panic under race detector. Undefined behaviour in production.

**Burden if deferred:** The pattern may be copied to new shared state as features grow.

### 4. Database Abstraction Leaks (Medium)

Four files outside the `db` package import `database/sql` directly:

- `internal/api/lidar_labels.go`
- `internal/lidar/monitor/run_track_api.go`
- `internal/lidar/monitor/track_api.go`
- `internal/lidar/pipeline/tracking_pipeline.go`

These bypass the `db.DB` wrapper to use `sql.NullString`, `sql.NullFloat64`, or raw
transactions.

**Consequence:** The `db` package boundary leaks. Changes to the database layer (e.g.
connection pooling, query tracing, context propagation) must be applied in scattered
locations instead of one package.

### 5. JSON Tag Inconsistency (Medium)

The codebase convention is `snake_case` for JSON tags. 95%+ of structs follow it. But
`EventAPI` in `db.go` uses PascalCase:

```go
type EventAPI struct {
    Magnitude *float64 `json:"Magnitude,omitempty"`
    Uptime    *float64 `json:"Uptime,omitempty"`
    Speed     *float64 `json:"Speed,omitempty"`
}
```

**Consequence:** API clients must handle two naming conventions. This is an API contract
inconsistency that becomes a permanent obligation once external consumers exist.

**Burden if deferred:** Renaming JSON tags after v0.5.0 is a breaking API change. Doing it
now is a pre-release correction.

### 6. Silent Error Drops in Production Code (Medium)

Five production-code locations discard errors with `_ =`:

- `db.go:251` — `_ = db.Close()` in error path
- `deploy/sshconfig.go:44` — `homeDir, _ = os.UserHomeDir()`
- `lidar/l3grid/export_bg_snapshot.go:45,61` — `_ = mgr.SetRingElevations(elevs)`
- `lidar/monitor/datasource_handlers.go:122,134` — `_ = w.Write(...)`

Some of these are genuinely unrecoverable (HTTP response writes after headers sent). Others
mask real failures.

**Consequence:** Failures become invisible. Operators on a headless Pi cannot diagnose
problems they cannot see.

### 7. Logging Inconsistency (Medium)

The codebase uses three distinct logging mechanisms:

1. `log.Printf()` — standard library, no levels, no structure
2. `fmt.Printf()` — not logging at all, just prints to stdout
3. `monitoring.Logf` — package-level function pointer, replaceable but not structured

Emoji appears in log output (`⚠️ WARNING`). No log levels (DEBUG/INFO/WARN/ERROR). No
structured key-value pairs. No correlation between request and log line.

**Consequence:** On a Raspberry Pi running as a systemd service, operators use `journalctl`
to diagnose problems. Unstructured, unlevel, mixed-destination logs make diagnosis slower
than it needs to be.

### 8. Test Infrastructure Thinness (Low)

`internal/testutil/testutil.go` provides 3 assertion helpers and 2 HTTP helpers in 46 lines.
The 216 test files contain significant duplicated setup code. Database test setup follows at
least two patterns: the centralised `setupTestDB` helper, and 31 files that open raw
`sql.Open("sqlite", path)` connections directly.

Nine test files use `time.Sleep` for synchronisation, creating flaky test risk.

**Consequence:** New tests copy-paste setup from whichever file the author finds first,
deepening inconsistency. Flaky tests erode trust in CI.

---

## Backlog Items

### Item 1: Request Lifecycle — Context Propagation and Graceful Shutdown

**Summary:** Thread `context.Context` through the HTTP handler → database query path.
Enable request cancellation, timeout enforcement, and graceful shutdown.

**Scope:**

1. Extract `r.Context()` in every HTTP handler in `api/server.go` and
   `lidar/monitor/webserver.go`
2. Pass context to all `db.DB` methods that execute queries (add `ctx context.Context` as
   first parameter where missing)
3. Use `QueryRowContext` / `ExecContext` / `QueryContext` consistently in the `db` package
4. Add a `server.Shutdown(ctx)` path that propagates cancellation to in-flight requests
5. Add integration-level test verifying that a cancelled context aborts a database query

**Estimated effort:** 3–5 days. Mechanical but load-bearing. Must be done before new
handlers are written.

**Milestone:** v0.5.0 — this is a convention that must be established before it becomes too
expensive to retrofit.

**Dependencies:** None. Can proceed independently.

**Risk:** Method signature changes touch many call sites. Mitigated by the mechanical nature
of the change and strong existing test coverage.

---

### Item 2: Package Hygiene — God Files, Abstractions, Synchronisation

**Summary:** Split overloaded files into domain-scoped units. Fix abstraction leaks and
race conditions. Establish the convention that each file in a package owns one domain.

**Scope:**

1. **Split `internal/db/db.go`** into:
   - `db_radar.go` — `RecordRadarObject`, `RadarObjects`, `RadarObjectRollupRange`,
     `RecordRawData`, `Events`, `RadarDataRange`
   - `db_lidar.go` — all `BgSnapshot` and `RegionSnapshot` methods
   - `db_admin.go` — `GetDatabaseStats`, `AttachAdminRoutes`, TailSQL/backup handlers
   - `db.go` — `NewDB`, `OpenDB`, `Close`, struct definition, shared helpers only
2. **Split `internal/api/server.go`** into handler files by domain (sites, config, events,
   reports, serial, transit, admin)
3. **Protect `serialmux.CurrentState`** with `sync.RWMutex` and accessor functions; remove
   direct map access
4. **Remove direct `database/sql` imports** from the 4 leaking files; expose needed types
   through the `db` package boundary (type aliases or wrapper types for `sql.Null*`)
5. **Fix silent error drops** in the 5 production-code locations (log or return the error)

**Estimated effort:** 4–6 days. File splits are mechanical. The `CurrentState` fix is
small. The abstraction leak fix requires interface thought.

**Milestone:** v0.5.0 or v0.5.1 — before the split becomes more expensive.

**Dependencies:** Ideally after Item 1 (context propagation), since method signatures will
change. Can proceed in parallel if coordinated.

**Risk:** File moves change import paths. No functional changes; tests should pass
unchanged.

---

### Item 3: API Contract Consistency — JSON Tags and Naming Convention

**Summary:** Standardise JSON serialisation tags to `snake_case` across all API-facing
structs. Document the convention. Fix the `EventAPI` anomaly before v0.5.0 ships.

**Scope:**

1. Change `EventAPI` JSON tags from PascalCase to `snake_case` (`"Magnitude"` →
   `"magnitude"`, `"Uptime"` → `"uptime"`, `"Speed"` → `"speed"`)
2. Audit all exported structs with JSON tags for consistency (spot-check found no other
   violations, but a mechanical sweep confirms)
3. Update any frontend or Python code that consumes `EventAPI` fields
4. Add a linter rule or test that enforces `snake_case` JSON tags on exported structs
   (optional, but prevents recurrence)
5. Document the convention: "All JSON API fields use `snake_case`" in the coding standards

**Estimated effort:** 1–2 days. Small scope but high value — this is a breaking change that
is free to make before v0.5.0 and expensive after.

**Milestone:** v0.5.0 — must land before the release locks the API contract.

**Dependencies:** None. Fully independent.

**Risk:** Minimal. The `EventAPI` struct is consumed by the web frontend and PDF generator.
Both must be updated in the same change.

---

### Item 4: Observability Foundation — Structured Logging and Test Infrastructure

**Summary:** Replace the mixed logging patterns with Go's `log/slog` (available since Go
1.21; the project uses Go 1.25). Expand test utilities to reduce duplication and eliminate
`time.Sleep` synchronisation.

**Scope:**

1. **Introduce `log/slog`** as the standard logging interface:
   - Replace `log.Printf` / `fmt.Printf` calls with `slog.Info`, `slog.Warn`, `slog.Error`
   - Replace `monitoring.Logf` with an `slog.Handler`-backed logger
   - Add structured key-value context to log calls (request path, duration, error)
   - Remove emoji from log messages
   - Configure JSON output for systemd/journalctl consumption
2. **Expand `internal/testutil/`**:
   - Add `SetupTestDB(t) *db.DB` and `CleanupTestDB(t, *db.DB)` as canonical helpers
   - Add `WaitFor(t, condition func() bool, timeout)` to replace `time.Sleep`
   - Migrate the 9 `time.Sleep` test files to use polling helpers
3. **Standardise database test setup** — deprecate raw `sql.Open` patterns in favour of the
   canonical helper

**Estimated effort:** 5–8 days. The `slog` migration is mechanical but touches many files.
Test infrastructure improvements can be done incrementally.

**Milestone:** v0.5.1 or v0.6.0 — important but less urgent than Items 1–3. The logging
convention should be established early; the test infrastructure can follow.

**Dependencies:** Item 2 (package splits) should land first so the logging changes apply to
the final file layout.

**Risk:** `slog` migration touches many files. Functional behaviour is unchanged. Tests
validate that logging calls do not alter control flow.

---

## Scheduling Recommendation

| Milestone | Items                                | Rationale                                                                           |
| --------- | ------------------------------------ | ----------------------------------------------------------------------------------- |
| v0.5.0    | Item 1 (context), Item 3 (JSON tags) | Convention-setting. Must land before the release locks the contract.                |
| v0.5.1    | Item 2 (package hygiene)             | Structural. Reduces cost of all future changes. Can land immediately after release. |
| v0.6.0    | Item 4 (observability + test infra)  | Operational. Important but not blocking. Benefits from stable file layout.          |

Items 1 and 3 are independent and can proceed in parallel. Item 2 benefits from Item 1
landing first (fewer conflicts when signatures change). Item 4 benefits from Item 2 landing
first (fewer file moves during logging migration).

## What This Plan Does Not Cover

- **Schema migrations** (000030, 000031) — covered by the existing schema simplification
  plan
- **Backward compatibility shim removal** — already complete per the v0.5.0 shim plan
- **LiDAR pipeline architecture** — the L1–L6 layering is sound; this plan addresses
  infrastructure around it, not within it
- **Frontend or Python code** — except where JSON tag changes require coordinated updates
- **Performance** — no bottleneck evidence warrants structural change; measure first
- **New features** — this plan reduces maintenance burden; it adds no capabilities

## Verification

Each backlog item should be verified by:

1. `make lint-go && make test-go` — no regressions
2. `go vet ./...` with race detector — no new warnings
3. Manual inspection that god files are below 500 lines after splitting
4. Spot-check that `r.Context()` flows through to database calls
5. JSON tag audit confirming 100% `snake_case` on exported API structs
