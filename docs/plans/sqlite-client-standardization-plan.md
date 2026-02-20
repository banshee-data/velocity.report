# SQLite Client Standardization Plan

## Status

Draft

## Goal

Use one canonical set of SQLite interfacing code for the shared schema in `internal/db/schema.sql`, and remove direct SQL access from `internal/api`.

## Scope

- In scope:
  - `internal/db`
  - `internal/api` (specifically DB access in API handlers)
  - `internal/lidar/storage/sqlite`
  - Test DB bootstrap/helpers that duplicate schema, PRAGMAs, or migration baselines
- Out of scope:
  - Changing API behavior/response formats
  - Reworking non-SQL business logic

## Current Duplication and Boundary Leaks

1. `internal/api` contains direct SQL repository logic:
   - `internal/api/lidar_labels.go:83`
   - `internal/api/lidar_labels.go:147`
   - `internal/api/lidar_labels.go:222`
   - `internal/api/lidar_labels.go:277`
   - `internal/api/lidar_labels.go:361`
   - `internal/api/lidar_labels.go:373`
   - `internal/api/lidar_labels.go:405`
2. SQLite bootstrap settings are duplicated and inconsistent:
   - Canonical settings in `internal/db/db.go:148` (`busy_timeout=30000`)
   - Separate test bootstrap in `internal/lidar/storage/sqlite/test_helpers.go:24` (`busy_timeout=5000`)
3. Migration/version bootstrapping is hardcoded in multiple places:
   - `internal/lidar/storage/sqlite/test_helpers.go:55`
   - `internal/lidar/storage/sqlite/track_store_test.go:64`
   - `internal/lidar/storage/sqlite/analysis_run_manager_test.go:58`
   - `internal/lidar/storage/sqlite/analysis_run_extended_test.go:56`
4. Test schemas are redefined instead of reusing the canonical schema:
   - `internal/api/lidar_labels_test.go:20`
   - `internal/lidar/storage/sqlite/scene_store_test.go:20`
   - `internal/lidar/storage/sqlite/sweep_store_test.go:21`
5. Mixed SQLite drivers in tests increase drift risk:
   - `modernc.org/sqlite` in `internal/lidar/storage/sqlite/track_store_test.go:10`
   - `github.com/mattn/go-sqlite3` in `internal/lidar/storage/sqlite/scene_store_test.go:9`, `internal/api/lidar_labels_test.go:11`
6. DB access is split across modules rather than a single SQL boundary:
   - `internal/db` methods (example: `internal/db/site.go:33`)
   - `internal/lidar/storage/sqlite` methods (example: `internal/lidar/storage/sqlite/analysis_run.go:320`)

## Target Architecture

One DB-facing SQLite layer under `internal/db` with:

- One connection/bootstrap path (open, PRAGMAs, migration handling)
- One set of repository interfaces and implementations for all schema-owned tables
- Zero raw SQL in `internal/api`
- `internal/lidar/storage` as domain/storage abstractions only, with SQLite implementation delegating to canonical `internal/db` repositories

## Proposed Work Plan

### Phase 0: Baseline and Safety Nets
Effort: 1 day

- [ ] Add a short architecture note in `internal/db` documenting "single SQL boundary" rules.
- [ ] Add CI guard checks:
  - [ ] fail if non-test files in `internal/api` import `database/sql`
  - [ ] fail if new hardcoded migration version values appear outside `internal/db`

### Phase 1: Define Canonical Repository Contracts in `internal/db`
Effort: 2-3 days

- [ ] Add repository interfaces for LiDAR tables currently served by `internal/lidar/storage/sqlite` and `internal/api/lidar_labels.go`.
- [ ] Add a repository container/facade on top of `*db.DB` (or equivalent) so callers do not access `*sql.DB` directly.
- [ ] Standardize "not found" semantics (single convention for all repos).

### Phase 2: Move Label SQL Out of `internal/api`
Effort: 2 days

- [ ] Create `internal/db` label repository methods for CRUD/list/export over `lidar_labels`.
- [ ] Refactor `internal/api/lidar_labels.go` to call repository methods only.
- [ ] Move label validation constants/helpers out of `internal/api` to a shared non-API package if needed by monitor handlers.
- [ ] Keep endpoint behavior unchanged with regression tests.

### Phase 3: Consolidate LiDAR SQLite Implementations
Effort: 4-6 days

- [ ] Migrate `internal/lidar/storage/sqlite` SQL implementations to canonical `internal/db` repositories.
- [ ] Keep compatibility wrappers temporarily in `internal/lidar/storage/sqlite` (thin adapters) to reduce blast radius.
- [ ] Remove direct `*sql.DB` plumbing from call sites where feasible (inject repo/facade instead).
- [ ] Preserve retry-on-busy behavior as shared helper in canonical DB layer.

### Phase 4: Unify Test DB Setup
Effort: 2-3 days

- [ ] Provide one shared test DB factory using canonical schema + migration version discovery.
- [ ] Replace inline `CREATE TABLE` test setups for labels/scenes/sweeps with shared setup where practical.
- [ ] Standardize on one SQLite driver for tests unless a specific test needs driver-specific behavior.
- [ ] Remove hardcoded `latestMigrationVersion := 15` patterns.

### Phase 5: Cleanup and Enforce
Effort: 1-2 days

- [ ] Remove deprecated wrappers after call sites are migrated.
- [ ] Add lint/guideline checks to prevent new SQL access in `internal/api`.
- [ ] Update contributor docs with the new DB access rules and examples.

## Estimated Total Effort

- Best case: 10 days
- Expected: 12-14 days
- Worst case (if hidden coupling/regressions emerge): 16 days

Assumption: one engineer, focused effort, with CI available.

## Risks

- Hidden coupling in LiDAR monitor handlers may surface during repository injection.
- Behavior drift from inconsistent historical error handling (`sql.ErrNoRows` vs wrapped messages vs `nil,nil`).
- Test failures from schema assumptions currently encoded in hand-written table definitions.

## Success Criteria

- [ ] No direct SQL queries in non-test `internal/api` code.
- [ ] One canonical DB setup path for PRAGMAs/migrations.
- [ ] One canonical repository set for schema-backed tables.
- [ ] Tests no longer rely on hardcoded migration version constants.
- [ ] Existing API behavior and LiDAR workflows pass regression tests.
