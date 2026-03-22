# Go Package Structure

Import boundaries, file-size discipline, and structural hygiene for the Go codebase.

Active plans:
[go-codebase-structural-hygiene-plan.md](../../plans/go-codebase-structural-hygiene-plan.md),
[go-god-file-split-plan.md](../../plans/go-god-file-split-plan.md)

## Import Boundary: `database/sql`

Non-test imports of `database/sql` are restricted to `internal/db/` and
`internal/lidar/storage/`. Enforced by `scripts/check-db-sql-imports.sh` in CI.

`internal/lidar/storage/sqlite/dbconn.go` exports `sqlite.SQLDB`, `sqlite.SQLTx`, and
`sqlite.ErrNotFound` so callers do not need `database/sql` directly.

## Single SQLite Driver Policy

The project uses `modernc.org/sqlite` exclusively (v1.44.3, SQLite 3.51.2). The duplicate
`github.com/mattn/go-sqlite3` dependency has been removed.

Enforced by `scripts/check-single-sqlite-driver.sh` in `make lint-go`. The driver name is
`"sqlite"` everywhere.

## God File Discipline

### Methodology

1. Identify files exceeding 1,000 LOC that mix four or more concerns
2. Split mechanically by domain — no functional changes, tests pass unchanged
3. Target: no file exceeds 600 LOC after splitting (soft ceiling; 5–10% over is acceptable)
4. Monitor files approaching 700 LOC for future splitting

### Tier 1 — God Files (All Complete)

| Original file                                   | Was       | Split into                                                                                                                                                                       |
| ----------------------------------------------- | --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/db/db.go`                             | 1,420 LOC | `db.go` (337), `db_radar.go` (562), `db_bg_snapshots.go` (278), `db_regions.go` (94), `db_admin.go` (183)                                                                        |
| `internal/api/server.go`                        | 1,711 LOC | `server.go` (260), `server_middleware.go` (75), `server_radar.go` (269), `server_sites.go` (231), `server_reports.go` (644), `server_timeline.go` (123), `server_admin.go` (177) |
| `internal/lidar/monitor/webserver.go`           | 1,905 LOC | Renamed to `server/` package: `server.go` (426), `state.go` (174), `routes.go` (219), `tuning.go` (104), `status.go` (690)                                                       |
| `internal/lidar/l5tracks/tracking.go`           | 1,676 LOC | `tracking.go` (515), `tracking_association.go` (297), `tracking_update.go` (370), `tracking_metrics.go` (441), `tracking_config.go` (115)                                        |
| `internal/lidar/storage/sqlite/analysis_run.go` | 1,400 LOC | `analysis_run.go` (391), `analysis_run_queries.go` (599), `analysis_run_mutations.go` (246), `analysis_run_compare.go` (282), `analysis_run_manager.go` (260)                    |
| `internal/lidar/l3grid/background.go`           | 1,672 LOC | `background.go` (352), `background_region.go` (474), `background_manager.go` (860)                                                                                               |
| `internal/config/tuning.go`                     | 1,303 LOC | `tuning.go` (250), `tuning_validate.go` (391), `tuning_codec.go` (280), `tuning_accessors.go` (361)                                                                              |

### Tier 2 — Large Files (700–860 LOC, Watch List)

| File                                              | LOC | Notes                               |
| ------------------------------------------------- | --- | ----------------------------------- |
| `internal/lidar/l3grid/background_manager.go`     | 861 | From Tier 1 split, exceeds target   |
| `internal/lidar/sweep/hint.go`                    | 798 | HINT algorithm + state machine      |
| `internal/lidar/storage/sqlite/track_store.go`    | 774 | Track persistence + queries         |
| `internal/lidar/server/track_api.go`              | 763 | Track query handlers + formatting   |
| `internal/lidar/sweep/auto.go`                    | 762 | Auto-tune algorithm + state machine |
| `internal/lidar/server/run_track_api.go`          | 752 | Run-level track query handlers      |
| `internal/lidar/pipeline/tracking_pipeline.go`    | 733 | Pipeline orchestration + state      |
| `internal/lidar/sweep/runner.go`                  | 720 | Sweep orchestration + lifecycle     |
| `internal/lidar/l9endpoints/recorder/recorder.go` | 711 | Recorder                            |
| `internal/lidar/server/datasource_handlers.go`    | 702 | Data source switching               |

## Direct Dependency Inventory

14 direct Go dependencies after removing the duplicate SQLite driver:

| Dependency                             | Scope            | Purpose                     |
| -------------------------------------- | ---------------- | --------------------------- |
| `github.com/go-echarts/go-echarts/v2`  | Production       | HTML chart rendering        |
| `github.com/golang-migrate/migrate/v4` | Production       | Schema migrations           |
| `github.com/google/go-cmp`             | Test-only        | Test comparison helper      |
| `github.com/google/gopacket`           | Production       | PCAP parsing                |
| `github.com/google/uuid`               | Production       | Entity IDs                  |
| `github.com/stretchr/testify`          | Test-only        | Assertion helpers           |
| `github.com/tailscale/tailsql`         | Production/debug | Admin SQL surface           |
| `go.bug.st/serial`                     | Production       | Physical serial-port access |
| `gonum.org/v1/gonum`                   | Production       | Statistics helpers          |
| `gonum.org/v1/plot`                    | Production       | Plot generation             |
| `google.golang.org/grpc`               | Production       | Visualiser streaming        |
| `google.golang.org/protobuf`           | Production       | Generated protobuf types    |
| `modernc.org/sqlite`                   | Production+tests | Canonical SQLite driver     |
| `tailscale.com`                        | Production/debug | Debug/admin plumbing        |

## Completed Structural Fixes

- **Context propagation:** 8 `_ = r` placeholders removed; 10 DB methods accept
  `context.Context` and use `*Context` SQL methods
- **`serialmux.CurrentState` race:** replaced unsynchronised map with `sync.RWMutex`-backed
  private state; exported `CurrentStateSnapshot()`
- **Single SQLite driver:** `mattn/go-sqlite3` removed, all tests use `modernc.org/sqlite`

## Open Structural Items

- **Query boundary:** `internal/api/lidar_labels.go` still contains raw SQL (7 call sites)
  that should move to `internal/lidar/storage/sqlite/label_store.go`
- **`EventAPI` JSON tags:** `internal/db/db.go` still uses PascalCase JSON tags; should be
  `snake_case` before API freeze
- **Silent error drops:** concentrated in `internal/db/db.go`, `l3grid/export_bg_snapshot.go`,
  `server/datasource_handlers.go`, `server/echarts_handlers.go`
- **Test infrastructure:** 40 internal test files use `time.Sleep` (199 call sites); needs
  shared polling helper and phased reduction
