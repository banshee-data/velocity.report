# Go Codebase Structural Hygiene Plan (v0.5.x)

- **Status:** Draft
- **Layers:** Cross-cutting (Go server, API, database, LiDAR pipeline)
- **Target:** v0.5.x — disrupt shaky conventions before they become foundations
- **Companion plans:**
  [go-structured-logging-plan.md](go-structured-logging-plan.md) (v0.6+),
  [go-god-file-split-plan.md](go-god-file-split-plan.md) (god file splits)

## Motivation

The Go codebase is 61,000 lines of production code across 197 files. The LiDAR pipeline
architecture (L1–L6) is well-layered. Test coverage is strong (120,000 lines of test code,
216 test files, 1.93× test-to-production ratio). Dependency graph is acyclic. These are
genuine structural strengths.

But beneath that surface sit conventions that will compound if left to settle. God files
growing a few methods per milestone. HTTP handlers that never propagate context. A global
mutable map with no synchronisation. JSON tags that contradict each other in the same
package. Silent error drops that make failures invisible on a headless Pi. These are not
emergencies. They are the kind of slow settling that turns a passable road into an expensive
repair job two versions later.

This plan names the structural issues to address in v0.5.x and groups them into three
backlog items sized for milestone scheduling. Logging and observability are covered
separately in the [companion plan](go-structured-logging-plan.md).

## Analysis Summary

| Category            | Finding                                                                                                                     | Severity | Item |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------- | -------- | ---- |
| Context propagation | 30+ HTTP handlers ignore `r.Context()`                                                                                      | Critical | 1    |
| God files           | `db.go` (1,420 LOC), `api/server.go` (1,711), `webserver.go` (1,905) — see [god file split plan](go-god-file-split-plan.md) | High     | —    |
| Race condition      | `serialmux.CurrentState` is a global map with no sync                                                                       | High     | 2    |
| DB abstraction leak | 4 files import `database/sql` directly, bypassing `db` package                                                              | Medium   | 2    |
| JSON tag anomaly    | `EventAPI` uses PascalCase; everything else uses snake_case                                                                 | Medium   | 3    |
| Silent error drops  | Production `_ = expr` discarding errors (excluding `deploy/`)                                                               | Medium   | 2    |
| God functions       | `setupRoutes` (415 LOC), `buildCosineSpeedExpr` (318 LOC)                                                                   | Medium   | —    |
| Test infrastructure | `testutil.go` is 46 lines for 216 test files; DB setup inconsistent                                                         | Low      | 2    |
| Flaky test risk     | `time.Sleep` in 9 test files                                                                                                | Low      | 2    |

Logging inconsistency is tracked in the
[companion plan](go-structured-logging-plan.md).

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

Extracted to a dedicated plan:
[go-god-file-split-plan.md](go-god-file-split-plan.md).

That plan covers the three original god files (`db.go`, `api/server.go`, `webserver.go`),
two additional Tier 1 files discovered during the full codebase scan (`l5tracks/tracking.go`
at 1,676 LOC, `storage/sqlite/analysis_run.go` at 1,400 LOC), and phased splits for eleven
Tier 2 files above 700 LOC.

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

Production code discards errors with `_ =` in several locations. The `deploy/` package is
excluded from this plan — it carries separate operational constraints and will be addressed
as part of the deployment retirement workstream.

**Locations in scope (v0.5.x):**

- `internal/db/db.go:251` — `_ = db.Close()` in error path
- `internal/lidar/l3grid/export_bg_snapshot.go:45,61` —
  `_ = mgr.SetRingElevations(elevs)`
- `internal/lidar/monitor/datasource_handlers.go:122,134` —
  `_ = ws.startLiveListenerLocked()`
- `internal/lidar/monitor/datasource_handlers.go:535,536,570` —
  `_ = bgManager.SetParams(...)`
- `internal/lidar/monitor/echarts_handlers.go:116,135,152,197,313,406,476,562` —
  `_, _ = w.Write(...)` (HTTP response body writes)

The HTTP response-write drops (`w.Write` after headers sent) are genuinely unrecoverable —
the correct fix is to log the failure rather than return it. The `SetRingElevations` and
`startLiveListenerLocked` drops mask real failures that operators cannot diagnose on a
headless Pi.

**Consequence:** Failures become invisible. Operators cannot diagnose problems they cannot
see.

### 7. Test Infrastructure Thinness (Low)

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

### Item 2: Package Hygiene — Abstractions, Error Visibility, Test Infrastructure

**Summary:** Fix abstraction leaks, race conditions, and silent error drops. Establish the
convention that each file in a package owns one domain and every error is either returned or
logged. God file splits are tracked separately in
[go-god-file-split-plan.md](go-god-file-split-plan.md).

**Scope:**

1. **Split god files** — see [go-god-file-split-plan.md](go-god-file-split-plan.md)
   for per-file checklists and phasing
2. **Protect `serialmux.CurrentState`** with `sync.RWMutex` and accessor functions; remove
   direct map access
3. **Remove direct `database/sql` imports** from the 4 leaking files; expose needed types
   through the `db` package boundary (type aliases or wrapper types for `sql.Null*`)
4. **Fix silent error drops** (excluding `deploy/`):
   - `db.go:251` — log the `Close()` error
   - `l3grid/export_bg_snapshot.go:45,61` — return or log `SetRingElevations` error
   - `monitor/datasource_handlers.go:122,134` — log `startLiveListenerLocked` error
   - `monitor/datasource_handlers.go:535,536,570` — log `SetParams` error
   - `monitor/echarts_handlers.go` (8 sites) — log `w.Write` errors at debug level
5. **Expand `internal/testutil/`**:
   - Add `SetupTestDB(t) *db.DB` and `CleanupTestDB(t, *db.DB)` as canonical helpers
   - Add `WaitFor(t, condition func() bool, timeout)` to replace `time.Sleep`
   - Migrate the 9 `time.Sleep` test files to use polling helpers
   - Standardise database test setup — deprecate raw `sql.Open` patterns

**Estimated effort:** 5–7 days. File splits are mechanical. The `CurrentState` fix is
small. The abstraction leak fix requires interface thought. Error-drop fixes are
straightforward.

**Milestone:** v0.5.1 — before the split becomes more expensive.

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

## Scheduling Recommendation

| Milestone | Items                                | Rationale                                                                  |
| --------- | ------------------------------------ | -------------------------------------------------------------------------- |
| v0.5.0    | Item 1 (context), Item 3 (JSON tags) | Convention-setting. Must land before the release locks the contract.       |
| v0.5.1    | Item 2 (package hygiene + errors)    | Structural. Reduces cost of all future changes. Includes error visibility. |

Items 1 and 3 are independent and can proceed in parallel. Item 2 benefits from Item 1
landing first (fewer conflicts when signatures change).

## What This Plan Does Not Cover

- **Structured logging** — covered in
  [go-structured-logging-plan.md](go-structured-logging-plan.md) (v0.6+)
- **Schema migrations** (000030, 000031) — covered by the existing schema simplification
  plan
- **Backward compatibility shim removal** — already complete per the v0.5.0 shim plan
- **God file splits** — extracted to
  [go-god-file-split-plan.md](go-god-file-split-plan.md)
- **LiDAR pipeline architecture** — the L1–L6 layering is sound; this plan addresses
  infrastructure around it, not within it
- **Frontend or Python code** — except where JSON tag changes require coordinated updates
- **`deploy/` silent error drops** — deferred to the deployment retirement workstream
- **Performance** — no bottleneck evidence warrants structural change; measure first
- **New features** — this plan reduces maintenance burden; it adds no capabilities

## Verification

Each backlog item should be verified by:

1. `make lint-go && make test-go` — no regressions
2. `go vet ./... && go test -race ./...` — no new vet warnings; race detector clean
3. God file LOC targets met — see [go-god-file-split-plan.md](go-god-file-split-plan.md)
4. Spot-check that `r.Context()` flows through to database calls
5. JSON tag audit confirming 100% `snake_case` on exported API structs
6. Grep for `_ =` in production code (excluding `deploy/`, `*.pb.go`, and `*_test.go`)
   returns zero results
