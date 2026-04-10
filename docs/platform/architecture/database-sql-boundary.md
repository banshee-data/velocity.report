# Database SQL Boundary: Two-Package Model

- **Status:** Implemented (March 2026)
- **Layers:** Cross-cutting (database infrastructure)

Sealed import boundary for `database/sql` usage in the codebase, restricting direct SQL access to two packages split by domain.

## Summary

The SQLite database access boundary follows a two-package model:

1. **`internal/db/`** — radar/core domain: radar objects, events, sites,
   config periods, reports, transits, background snapshots, migrations,
   admin/debug routes.
2. **`internal/lidar/storage/sqlite/`** — LiDAR domain: tracks,
   observations, clusters, analysis runs, replay cases, evaluations,
   missed regions, sweeps.

Other packages use type aliases (`sqlite.SQLDB`, `sqlite.SQLTx`) and
sentinel values (`sqlite.ErrNotFound`) from the storage layer.
`scripts/check-db-sql-imports.sh` enforces this boundary in CI via
`make lint-go`.

## SQL Operation Count

| Package                                                 | Files | SQL ops | Domain             |
| ------------------------------------------------------- | ----: | ------: | ------------------ |
| `internal/db/` (db.go, site, config, report, transit…)  |     8 |     ~72 | Radar/core + infra |
| `internal/lidar/storage/sqlite/` (tracks, runs, sweep…) |     7 |     ~55 | LiDAR              |
| **Grand total**                                         |    15 |    ~127 |                    |

## Options Evaluated

### Option A: Single package (`internal/db/`)

All ~127 SQL operations in one package. **Rejected** — `internal/db/` is
already 1,400+ lines; adding LiDAR operations creates import cycle risk
(`db` → `l3grid` → `db`) and means the core carries LiDAR domain
knowledge even when LiDAR is disabled.

### Option B: Two packages (current model) ✅

Keep the domain split. Tighten boundary where blurred. Minimal migration
work, natural domain ownership, LiDAR package excludable from non-LiDAR
builds. Already enforced by CI lint script.

### Option C: Three packages (split BgSnapshot)

Move BgSnapshot/RegionSnapshot to a third package. **Not worth the
effort** — the `l3grid.BgStore` interface is small (2 methods) and the
coupling is explicit (compile-time assertion).

## BgSnapshot Crossover

Twelve methods on `db.DB` serve LiDAR background/region data because
`db.DB` implements the `l3grid.BgStore` interface (compile-time assertion
at `db.go:33`). This is accepted as a stable seam: the interface is small,
the coupling is explicit, and a third package would add a boundary without
adding clarity.

## Remaining Gaps

| Gap                          | Size | Priority | Notes                                               |
| ---------------------------- | ---- | -------- | --------------------------------------------------- |
| Label SQL in `internal/api/` | S    | High     | Last query-boundary violation; 8 queries to move    |
| Unified PRAGMA bootstrap     | XS   | Medium   | Test helpers vs production differ on busy_timeout   |
| BgSnapshot crossover docs    | XS   | Low      | Comment block in `db.go`                            |
| Migration version discovery  | XS   | Low      | Replace hardcoded `latestMigrationVersion` in tests |
| `cmd/tools/` DB access       | XS   | High     | `backfill_ring_elevations` bypasses boundary        |

## Enforcement

- **Import boundary script:** `scripts/check-db-sql-imports.sh`
- **CI integration:** `make lint-go`
- **Exemptions:** `*_test.go` files

## Relationship to Other Plans

- **sqlite-client-standardisation** — superseded target architecture (two
  packages instead of one). Phases 0, 2, 4, 5 still relevant; phases 1, 3
  replaced.
- **deploy-distribution-packaging** — single-binary plan eliminates
  `cmd/tools/` exemption by construction.
- **lidar-tracks-table-consolidation** — orthogonal (Go-level struct
  duplication, not SQL package boundary).

## History

- Original proposal: collapse all SQL into `internal/db/`
  ([sqlite-client-standardisation plan](../../plans/data-sqlite-client-standardisation-plan.md))
- Replaced by two-package model
  ([database-alignment plan](../../plans/data-database-alignment-plan.md))
