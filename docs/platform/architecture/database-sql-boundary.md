# Database SQL Boundary: Two-Package Model

- **Status:** Implemented (March 2026)
- **Layers:** Cross-cutting (database infrastructure)

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

## Design Rationale

A single-package model (`internal/db/` owns everything) was evaluated and
rejected. The two-package split reflects genuine domain ownership: radar
schema and LiDAR schema have different evolution rates, different
migration cadences, and different test fixtures.

The crossover area — twelve `db.DB` methods serving LiDAR
background/region data via the `l3grid.BgStore` interface — is accepted
as a stable seam rather than a defect.

## Enforcement

- **Import boundary script:** `scripts/check-db-sql-imports.sh`
- **CI integration:** `make lint-go`
- **Exemptions:** `*_test.go` files

## History

- Original proposal: collapse all SQL into `internal/db/`
  ([sqlite-client-standardisation plan](../../docs/plans/data-sqlite-client-standardisation-plan.md))
- Replaced by two-package model
  ([database-alignment plan](../../docs/plans/data-database-alignment-plan.md))
