# Typed UUID prefixes

Active plan: [platform-typed-uuid-prefixes-plan.md](../../plans/platform-typed-uuid-prefixes-plan.md)

Every UUID in the system must carry a short type prefix so that any ID seen
in logs, databases, API responses, or debug output is immediately
identifiable by origin system.

## Convention

- Format: `{prefix}_{uuidv4}`; e.g. `trak_550e8400-e29b-41d4-a716-446655440000`
- Prefixes: lowercase 4-letter abbreviation, combined with the UUID as
  `{prefix}_{uuidv4}`.
- The three run types each get a distinct prefix so you can tell at a glance
  whether a run ID came from an analysis run, a scene replay, or a
  reprocess operation.

## Entity inventory

| Entity        | Package / File                                      | Current Format             | Proposed Prefix | Example                                     |
| ------------- | --------------------------------------------------- | -------------------------- | --------------- | ------------------------------------------- |
| Track         | `l5tracks/tracking.go`                              | `trk_<uuid>` (current)     | `trak_`         | `trak_550e8400-e29b-41d4-a716-446655440000` |
| Analysis Run  | `storage/sqlite/analysis_run_manager.go`            | `<uuid>`                   | `runa_`         | `runa_550e8400-e29b-41d4-a716-446655440000` |
| Replay Run    | `monitor/scene_api.go`                              | `replay-{scene}-{uuid8}`   | `runy_`         | `runy_550e8400-e29b-41d4-a716-446655440000` |
| Reprocess Run | `monitor/run_track_api.go`                          | `reprocess-{run8}-{uuid8}` | `runs_`         | `runs_550e8400-e29b-41d4-a716-446655440000` |
| Scene         | `storage/sqlite/scene_store.go`                     | `<uuid>`                   | `scne_`         | `scne_550e8400-e29b-41d4-a716-446655440000` |
| Evaluation    | `storage/sqlite/evaluation_store.go`                | `<uuid>`                   | `eval_`         | `eval_550e8400-e29b-41d4-a716-446655440000` |
| Missed Region | `storage/sqlite/missed_region_store.go`             | `<uuid>`                   | `regn_`         | `regn_550e8400-e29b-41d4-a716-446655440000` |
| Label         | `api/lidar_labels.go`                               | `<uuid>`                   | `labl_`         | `labl_550e8400-e29b-41d4-a716-446655440000` |
| Sweep         | `sweep/runner.go`, `sweep/hint.go`, `sweep/auto.go` | `<uuid>`                   | `swep_`         | `swep_550e8400-e29b-41d4-a716-446655440000` |

## Replay / reprocess migration rationale

The composite `replay-{sceneID}-{uuid8}` and `reprocess-{run8}-{uuid8}`
formats bake human-readable provenance into the primary key. Analysis shows
replacement with `runy_`/`runs_` + full UUID is safe because:

- **No code parses the prefix for logic.** All lookups are exact string match
  (`WHERE run_id = ?`). No routing or dispatch inspects `replay-` vs
  `reprocess-` vs bare UUID.
- **Provenance is stored in dedicated columns.** `parent_run_id` records
  reprocess lineage; `source_path` and `source_type` record the PCAP origin;
  `lidar_scenes.reference_run_id` FK links scenes to runs.
- **Uniqueness from the 8-char UUID suffix is fragile.** A full UUIDv4 is
  stronger (collision probability ~1 in 2^122 vs ~1 in 2^32).
- **Distinct prefixes preserve type context.** Unlike merging all runs under
  a single `run_` prefix, the `runa_`/`runy_`/`runs_` scheme keeps the run
  origin visible at a glance in logs and database rows.
- **Frontend impact is cosmetic only.** The web UI truncates `run_id` to 8
  chars for display; `runy_a1b` is equally readable as `replay-f`.

## Central helper design

The `id` package (in `internal/id/`) provides two functions:

- **`New(prefix string) string`** — returns a prefixed UUID v4 string in the form `{prefix}_{uuid}`, using `github.com/google/uuid`.
- **`Parse(s string) (prefix, uuid string, err error)`** — splits on the first `_` and validates the UUID portion. Optional validation helper.

## Call sites (11 total)

1. [internal/lidar/l5tracks/tracking.go](../../../internal/lidar/l5tracks/tracking.go): already `trk_`, switch to `id.New("trak")`
2. [internal/lidar/storage/sqlite/analysis_run_manager.go](../../../internal/lidar/storage/sqlite/analysis_run_manager.go): `id.New("runa")`
3. `internal/lidar/monitor/scene_api.go`: replace `fmt.Sprintf("replay-...")` with `id.New("runy")`
4. `internal/lidar/monitor/run_track_api.go`: replace `fmt.Sprintf("reprocess-...")` with `id.New("runs")`
5. [internal/lidar/storage/sqlite/scene_store.go](../../../internal/lidar/storage/sqlite/scene_store.go): `id.New("scne")`
6. [internal/lidar/storage/sqlite/evaluation_store.go](../../../internal/lidar/storage/sqlite/evaluation_store.go): `id.New("eval")`
7. [internal/lidar/storage/sqlite/missed_region_store.go](../../../internal/lidar/storage/sqlite/missed_region_store.go): `id.New("regn")`
8. [internal/api/lidar_labels.go](../../../internal/api/lidar_labels.go): `id.New("labl")`
9. [internal/lidar/sweep/runner.go](../../../internal/lidar/sweep/runner.go) (2 sites): `id.New("swep")`
10. [internal/lidar/sweep/hint.go](../../../internal/lidar/sweep/hint.go): `id.New("swep")`
11. [internal/lidar/sweep/auto.go](../../../internal/lidar/sweep/auto.go): `id.New("swep")`

## ID flow

```
id.New("xxxx")  →  SQLite (TEXT PK)  →  HTTP API (JSON + URL paths)  →  Clients (macOS / Web / CLI)
```

All downstream consumers treat IDs as opaque strings:

- **SQLite:** `TEXT PRIMARY KEY` / `TEXT NOT NULL` columns, exact-match `WHERE` clauses
- **API:** URL path segments (`/api/lidar/runs/{run_id}`) and JSON bodies
- **Web UI:** `run_id.substring(0, 8)` for display truncation
- **macOS visualiser:** `AppState.currentRunID`, `selectedTrackID` as opaque strings
- **Sweep system:** checkpoint/resume persistence by sweep_id exact match

## SQLite migration strategy

Accept mixed formats. All lookups treat IDs as opaque strings, so prefixed
and unprefixed IDs coexist without schema changes. Old rows keep their
existing format (bare UUIDs, `trk_*`, `replay-*`, `reprocess-*`); new rows
get the `xxxx_` prefix. No `UPDATE` migration needed: IDs are opaque primary
keys with no format constraint.

## Track prefix migration (`trk_` → `trak_`)

The existing `trk_` prefix is 3 characters. To align with the 4-character
convention, `l5tracks/tracking.go` updates from `trk_` to `trak_`. The
golden replay test assertion `track.TrackID[:4] == "trk_"` updates to check
for `trak_`. Mixed formats coexist in SQLite.

## Exit criteria

- Every `uuid.New()` / `uuid.NewString()` call site replaced with `id.New`.
- `replay-` and `reprocess-` composite formats replaced with `runy_` / `runs_`.
- `trk_` migrated to `trak_`.
- `internal/id` package exists with tests.
- No bare UUIDs generated anywhere in production code.
- Existing SQLite data continues to work (mixed format acceptance).
