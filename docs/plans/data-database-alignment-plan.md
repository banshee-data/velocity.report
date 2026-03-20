# Database Alignment Plan: Two-Package SQL Boundary

- **Status:** Proposed
- **Layers:** Cross-cutting (database infrastructure)
- **Supersedes:** Partially replaces
  [data-sqlite-client-standardisation-plan.md](data-sqlite-client-standardisation-plan.md)
  (original plan proposed collapsing everything into `internal/db`; this plan
  evaluates a two-package model instead)

## Context

A recent fix sealed the `database/sql` import boundary so that only two
package trees may import it:

1. **`internal/db/`** — radar/core domain: radar objects, events, sites,
   config periods, reports, transits, background snapshots, migrations,
   admin/debug routes.
2. **`internal/lidar/storage/sqlite/`** — LiDAR domain: tracks,
   observations, clusters, analysis runs, replay cases, evaluations,
   missed regions, sweeps.

Other packages use the type aliases (`sqlite.SQLDB`, `sqlite.SQLTx`) and
sentinel (`sqlite.ErrNotFound`) exported from the storage layer.
`scripts/check-db-sql-imports.sh` enforces this boundary in CI via
`make lint-go`.

The question now is whether to **settle on this two-package model** as the
long-term target, or push further toward a single package.

## Current State

### SQL operation count by package

| Package                                                 |  Files |  SQL ops | Domain                                                          |
| ------------------------------------------------------- | -----: | -------: | --------------------------------------------------------------- |
| `internal/db/db.go`                                     |      1 |      ~35 | Radar objects, events, BgSnapshot, RegionSnapshot, stats, admin |
| `internal/db/site.go`                                   |      1 |       ~5 | Site CRUD                                                       |
| `internal/db/site_config_periods.go`                    |      1 |       ~6 | Config periods                                                  |
| `internal/db/site_report.go`                            |      1 |       ~5 | Reports                                                         |
| `internal/db/transit_worker.go`                         |      1 |      ~14 | Transit analysis                                                |
| `internal/db/transit_gaps.go`                           |      1 |       ~1 | Transit gaps                                                    |
| `internal/db/migrate.go`                                |      1 |       ~5 | Migrations                                                      |
| `internal/db/migrate_cli.go`                            |      1 |       ~1 | Migration CLI                                                   |
| **`internal/db/` total**                                |  **8** |  **~72** | **Radar/core + infra**                                          |
| `internal/lidar/storage/sqlite/track_store.go`          |      1 |      ~14 | Tracks, clusters, observations                                  |
| `internal/lidar/storage/sqlite/analysis_run.go`         |      1 |      ~15 | Analysis runs                                                   |
| `internal/lidar/storage/sqlite/analysis_run_manager.go` |      1 |       ~3 | Run manager singleton                                           |
| `internal/lidar/storage/sqlite/sweep_store.go`          |      1 |       ~9 | Tuning sweeps                                                   |
| `internal/lidar/storage/sqlite/scene_store.go`          |      1 |       ~7 | Replay cases                                                    |
| `internal/lidar/storage/sqlite/evaluation_store.go`     |      1 |       ~4 | Evaluations                                                     |
| `internal/lidar/storage/sqlite/missed_region_store.go`  |      1 |       ~3 | Missed regions                                                  |
| **`internal/lidar/storage/sqlite/` total**              |  **7** |  **~55** | **LiDAR**                                                       |
| **Grand total**                                         | **15** | **~127** |                                                                 |

### Import boundary enforcement

- **Script:** `scripts/check-db-sql-imports.sh`
- **Wired into:** `make lint-go`
- **Exemptions:** `*_test.go`

### LiDAR BgSnapshot / RegionSnapshot — the crossover

Twelve methods on `db.DB` serve LiDAR background/region data. These exist
in `internal/db/` because `db.DB` implements the `l3grid.BgStore` interface
(compile-time assertion at `db.go:33`). This is the one area where the
boundary between "radar/core" and "lidar" is blurred.

### Label SQL in `internal/api/`

The earlier import-boundary fix removed the `database/sql` import from
`internal/api/lidar_labels.go`, but the file still contains raw SQL
queries executed via `sqlite.SQLDB` (the type alias). The import boundary
is honoured, but the _query_ boundary is not: SQL strings live in a
package whose job is HTTP routing, not storage.

## Options Evaluated

### Option A: Single package (`internal/db/` holds everything)

This is the approach in the original standardisation plan. All ~127 SQL
operations would live under `internal/db/`.

**Pros:**

- One place to look for any SQL query.
- One connection/bootstrap path, one set of PRAGMAs.
- Simplest mental model.

**Cons:**

- `internal/db/` is already 1,400+ lines in `db.go` alone and growing.
  Adding ~55 LiDAR operations would push it past 2,000 lines before tests.
- Creates an import cycle risk: `internal/db/` already depends on
  `internal/lidar/l3grid` (for `BgStore`). Moving track/run stores into
  `internal/db/` would require importing LiDAR domain types, deepening the
  coupling.
- The LiDAR subsystem is optional (feature-gated). Keeping its SQL in the
  core package means the core carries lidar domain knowledge even when
  lidar is disabled.
- Contradicts the existing `doc.go` design rationale that placed LiDAR
  storage in its own package to keep domain logic free of SQL noise.

**Verdict:** Rejected — cost exceeds benefit. The import cycle pressure
alone makes this impractical without significant restructuring.

### Option B: Two packages (current model, tightened)

Keep `internal/db/` for radar/core and `internal/lidar/storage/sqlite/`
for LiDAR. Tighten the boundary where it is currently blurred.

**Pros:**

- Already in place — minimal migration work.
- Natural domain split: radar/core tables vs LiDAR tables.
- LiDAR package can be excluded from builds where lidar is disabled.
- Keeps the import graph shallow: domain types flow downward, storage
  implementations do not leak upward.
- Enforced today by CI lint script.

**Cons:**

- Two connection bootstrap paths (one in `db.NewDB`, one in test helpers
  for the sqlite package). Need to unify PRAGMAs and busy_timeout.
- BgSnapshot/RegionSnapshot methods on `db.DB` serve LiDAR domain but
  live in `internal/db/`. This is a known crossover.
- Label SQL currently lives in `internal/api/`, outside both packages.

**Verdict: Recommended.** The two-package model matches the domain split,
is already enforced, and avoids the import cycle that Option A creates.

### Option C: Three packages (split BgSnapshot out)

Move BgSnapshot/RegionSnapshot methods into a third package to eliminate
the crossover.

**Verdict:** Not worth the effort. The `l3grid.BgStore` interface is
small (2 methods), the coupling is explicit (compile-time assertion), and
a third package adds a boundary without adding clarity. Note as future
work only if `internal/db/` grows further.

## Recommended Target: Option B (Two Packages, Tightened)

### Remaining work

The import boundary is already enforced. The remaining gaps are:

#### Gap 1: Label SQL in `internal/api/lidar_labels.go`

`lidar_labels.go` contains 8 raw SQL queries (`Query`, `QueryRow`,
`Exec`) inside HTTP handlers. These should move to a store in
`internal/lidar/storage/sqlite/`.

**Work:**

- Create `internal/lidar/storage/sqlite/label_store.go` with a
  `LabelStore` struct and CRUD/list/export methods.
- Refactor `internal/api/lidar_labels.go` to accept a `LabelStore`
  (interface or concrete) and delegate all SQL.
- Update `internal/lidar/monitor/webserver.go` to inject the store.
- Move label test DB setup to use the shared test helper.

**Effort:** S — 1–2 days. Mechanical move, no schema changes.

#### Gap 2: Unified PRAGMA / bootstrap path

Test helpers in `internal/lidar/storage/sqlite/test_helpers.go` apply
`busy_timeout=5000` while production uses `busy_timeout=30000` via
`db.applyPragmas`. Both packages should share one canonical PRAGMA set.

**Work:**

- Extract a shared `ApplyPragmas(*sql.DB) error` function (or move the
  existing one to a location both packages can import).
- Update test helpers to use the shared function.

**Effort:** XS — half a day.

#### Gap 3: BgSnapshot crossover documentation

The 12 BgSnapshot/RegionSnapshot methods on `db.DB` serve LiDAR but live
in `internal/db/` because `db.DB` implements `l3grid.BgStore`. This is
acceptable for now — the interface is small and the coupling is explicit.
Document it and monitor growth.

**Work:**

- Add a short comment block in `internal/db/db.go` documenting which
  methods serve the LiDAR subsystem and why they live here.

**Effort:** XS — trivial.

#### Gap 4: Mixed SQLite drivers in tests

Some test files use `github.com/mattn/go-sqlite3`, others use
`modernc.org/sqlite`. Production uses `modernc.org/sqlite`. Standardise
on one driver for tests to prevent behavioural drift.

**Work:**

- Replace `mattn/go-sqlite3` imports in test files with
  `modernc.org/sqlite`.
- Verify all tests still pass (driver-level behaviour differences are
  rare but possible).

**Effort:** XS — half a day.

#### Gap 5: Hardcoded migration versions in tests

Several test files hardcode `latestMigrationVersion := N`. These break
silently when new migrations are added. Replace with dynamic version
discovery.

**Work:**

- Add a `LatestMigrationVersion() uint` function in `internal/db/`.
- Replace hardcoded constants in test files.

**Effort:** XS — half a day.

#### Gap 6: `cmd/tools/` direct database access

Standalone CLI tools under `cmd/tools/` (e.g. `backfill_ring_elevations`)
previously bypassed the import boundary entirely, opening SQLite via
`sql.Open` and applying ad-hoc PRAGMAs. This undermines the two-package
model: connection bootstrap, PRAGMA configuration, and raw SQL leak into
a third location.

**Work:**

- Refactor each `cmd/tools/` binary to use `db.OpenDB()` (or an
  appropriate store method) instead of `sql.Open` + manual PRAGMAs.
- Replace `*sql.DB` parameters with the `sqlite.SQLDB` type alias so the
  import boundary is honoured in non-test code.
- Remove the `cmd/tools/` exemption from `scripts/check-db-sql-imports.sh`.

**Relationship to single-binary plan:** The
[deploy-distribution-packaging-plan.md](deploy-distribution-packaging-plan.md)
moves these standalone tools into subcommands of a single
`velocity-report` binary (e.g. `velocity-report backfill-rings`). Once
that migration is complete, all database access originates from one
binary whose `main` constructs the `db.DB` — eliminating the need for
any `cmd/tools/` exemption. This gap is the first step on that path.

**Effort:** XS — half a day. Mechanical refactor; `backfill_ring_elevations`
is the only current violator.

### Effort summary

| Gap                            | Size |    Days | Priority                                       |
| ------------------------------ | ---- | ------: | ---------------------------------------------- |
| 1. Label SQL move              | S    |     1–2 | High — last remaining query-boundary violation |
| 2. Unified PRAGMAs             | XS   |     0.5 | Medium — reduces drift risk                    |
| 3. BgSnapshot docs             | XS   |     0.1 | Low — documentation only                       |
| 4. Driver unification          | XS   |     0.5 | Medium — prevents behavioural drift            |
| 5. Migration version discovery | XS   |     0.5 | Low — convenience                              |
| 6. cmd/tools/ DB access        | XS   |     0.5 | High — boundary exemption removal              |
| **Total**                      |      | **3–5** |                                                |

Compare with the original standardisation plan's 10–16 day estimate:
the two-package model requires roughly **one-quarter** of the effort
because it preserves the existing split rather than fighting the import
graph.

## Is This Cleanup Warranted?

### Arguments for

1. **Query boundary = change boundary.** When connection pooling, query
   tracing, or context propagation changes are needed, they apply in two
   known locations instead of scattered across `internal/api/` or future
   callers.
2. **The LiDAR subsystem is optional.** Keeping its SQL in a separate
   package means `go build` without the lidar flag compiles a smaller,
   simpler binary.
3. **The lint guard already exists.** The enforcement mechanism is in
   place. The remaining gaps (label SQL, PRAGMAs, drivers) are small.
   Finishing the job now prevents the boundary from eroding.
4. **New contributors.** "SQL lives in two packages" is easier to explain
   than "SQL lives in two packages, except for labels which are in the
   API layer, and some tests use a different SQLite driver."

### Arguments against

1. **The system works today.** The label SQL in `internal/api/` is not
   causing bugs. Moving it is hygiene, not a fix.
2. **Opportunity cost.** 3–4 days spent on plumbing is 3–4 days not
   spent on features, analysis accuracy, or the visualiser.
3. **Risk of churn.** Moving code always risks introducing regressions,
   even with tests.

### Verdict

**Warranted, at moderate priority.** The import boundary is already
enforced. The remaining gaps are small, low-risk, and mechanical. Do
them in the next maintenance window — not urgently, but before the next
round of feature work adds more callers that depend on the current
(slightly leaky) arrangement.

Gap 1 (label SQL) should be done first: it is the last remaining
query-boundary violation and the most likely to attract imitators if
left in place. Gaps 2–5 can be batched into a single follow-up PR.

## Relationship to Existing Plans

- **data-sqlite-client-standardisation-plan.md** — This plan supersedes
  the target architecture (two packages instead of one). Phases 0, 2, 4,
  and 5 of the original plan are still relevant; phases 1 and 3 (move
  LiDAR SQL into `internal/db/`) are replaced by the two-package model.
- **deploy-distribution-packaging-plan.md** — The single-binary plan
  moves standalone `cmd/tools/` binaries into subcommands of
  `velocity-report`. Once complete, all database access originates from
  one binary — reinforcing the two-package boundary by construction.
  Gap 6 is the preparatory step: each tool uses `db.OpenDB()` now so
  the eventual move is a rename, not a refactor.
- **lidar-tracks-table-consolidation-plan.md** — Orthogonal. That plan
  addresses Go-level struct duplication between live and analysis tracks,
  not the SQL package boundary.
- **schema-simplification-migration-030-plan.md** — Complete. Migrations
  030–031 are landed. No interaction.

## Future Work (Not In Scope)

- Evaluate whether BgSnapshot methods should move to a small `internal/db/lidar`
  sub-package if `internal/db/db.go` grows past ~2,000 lines.
- Evaluate repository interface abstraction (interfaces over concrete
  stores) if a second storage backend becomes likely. Not warranted
  today — SQLite is the only target.
