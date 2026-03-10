# Platform: Typed UUID Prefixes

**Target:** 0.6.0
**Scope:** All UUID generation across the project

Every UUID in the system must carry a short type prefix so that any ID seen
in logs, databases, API responses, or debug output is immediately
identifiable by origin system. Tracks already use `trk_`; this plan extends
the convention to every remaining entity.

## Convention

- Format: `{prefix}_{uuidv4}` — e.g. `trak_550e8400-e29b-41d4-a716-446655440000`
- Prefixes: lowercase 4-letter abbreviation + underscore (`xxxx_`).
- The three run types each get a distinct prefix so you can tell at a glance
  whether a run ID came from an analysis run, a scene replay, or a
  reprocess operation.

## Existing ID Examples

Current production and test IDs observed in the codebase:

| Entity        | Example production ID                                  | Example test IDs                                     |
| ------------- | ------------------------------------------------------ | ---------------------------------------------------- |
| Track         | `trk_550e8400-e29b-41d4-a716-446655440000`             | `trk_0000abcd`, `trk_00001234`, `trk_0000test`       |
| Analysis Run  | `550e8400-e29b-41d4-a716-446655440000`                 | `run_123`, `test_run_001`, `run_1`, `run_2`          |
| Replay Run    | `replay-f47ac10b-58cc-4372-a567-0e02b2c3d479-3a7f1b2c` | (dynamically generated)                              |
| Reprocess Run | `reprocess-550e8400-a1b2c3d4`                          | (dynamically generated)                              |
| Scene         | `550e8400-e29b-41d4-a716-446655440000`                 | `scene-1`                                            |
| Evaluation    | `550e8400-e29b-41d4-a716-446655440000`                 | (auto-generated, checked non-empty)                  |
| Missed Region | `550e8400-e29b-41d4-a716-446655440000`                 | (auto-generated, checked non-empty)                  |
| Label         | `550e8400-e29b-41d4-a716-446655440000`                 | `label-001`, `label-002`, `label-003`                |
| Sweep         | `550e8400-e29b-41d4-a716-446655440000`                 | `sweep-001`, `sweep-002`, `sweep-valid`, `sweep-123` |

## Entity Inventory

| Entity        | Package / File                                      | Current format             | Proposed prefix | Example                                     |
| ------------- | --------------------------------------------------- | -------------------------- | --------------- | ------------------------------------------- |
| Track         | `l5tracks/tracking.go`                              | `trk_<uuid>` (done)        | `trak_`         | `trak_550e8400-e29b-41d4-a716-446655440000` |
| Analysis Run  | `storage/sqlite/analysis_run_manager.go`            | `<uuid>`                   | `runa_`         | `runa_550e8400-e29b-41d4-a716-446655440000` |
| Replay Run    | `monitor/scene_api.go`                              | `replay-{scene}-{uuid8}`   | `runy_`         | `runy_550e8400-e29b-41d4-a716-446655440000` |
| Reprocess Run | `monitor/run_track_api.go`                          | `reprocess-{run8}-{uuid8}` | `runs_`         | `runs_550e8400-e29b-41d4-a716-446655440000` |
| Scene         | `storage/sqlite/scene_store.go`                     | `<uuid>`                   | `scne_`         | `scne_550e8400-e29b-41d4-a716-446655440000` |
| Evaluation    | `storage/sqlite/evaluation_store.go`                | `<uuid>`                   | `eval_`         | `eval_550e8400-e29b-41d4-a716-446655440000` |
| Missed Region | `storage/sqlite/missed_region_store.go`             | `<uuid>`                   | `regn_`         | `regn_550e8400-e29b-41d4-a716-446655440000` |
| Label         | `api/lidar_labels.go`                               | `<uuid>`                   | `labl_`         | `labl_550e8400-e29b-41d4-a716-446655440000` |
| Sweep         | `sweep/runner.go`, `sweep/hint.go`, `sweep/auto.go` | `<uuid>`                   | `swep_`         | `swep_550e8400-e29b-41d4-a716-446655440000` |

### Replay / Reprocess migration rationale

The composite `replay-{sceneID}-{uuid8}` and `reprocess-{run8}-{uuid8}`
formats bake human-readable provenance into the primary key. Analysis shows
this is safe to replace with `runy_`/`runs_` + full UUID because:

- **No code parses the prefix for logic.** All lookups are exact string match
  (`WHERE run_id = ?`). No routing or dispatch inspects `replay-` vs
  `reprocess-` vs bare UUID.
- **Provenance is stored in dedicated columns.** `parent_run_id` records
  reprocess lineage; `source_path` and `source_type` record the PCAP origin;
  the `lidar_scenes.reference_run_id` FK links scenes to runs.
- **Uniqueness from the 8-char UUID suffix is fragile.** A full UUIDv4 is
  stronger (collision probability ~1 in 2^122 vs ~1 in 2^32).
- **Distinct prefixes preserve type context.** Unlike merging all runs under
  a single `run_` prefix, the `runa_`/`runy_`/`runs_` scheme keeps the run
  origin visible at a glance in logs and database rows.
- **Frontend impact is cosmetic only.** The web UI truncates `run_id` to 8
  chars for display; `runy_a1b` is equally readable as `replay-f`.

## Implementation

### Phase 1: Central helper

Create `internal/id/id.go`:

```go
package id

import "github.com/google/uuid"

// New returns a prefixed UUID v4 string: "{prefix}_{uuid}".
func New(prefix string) string {
    return prefix + "_" + uuid.NewString()
}
```

### Phase 2: Migrate generation sites

Replace each `uuid.New().String()` / `uuid.NewString()` call with
`id.New("xxxx")` using the prefix from the table above.

Call sites (11 total):

1. `internal/lidar/l5tracks/tracking.go` — already `trk_`, switch to `id.New("trak")`
2. `internal/lidar/storage/sqlite/analysis_run_manager.go` — `id.New("runa")`
3. `internal/lidar/monitor/scene_api.go` — replace `fmt.Sprintf("replay-...")` with `id.New("runy")`
4. `internal/lidar/monitor/run_track_api.go` — replace `fmt.Sprintf("reprocess-...")` with `id.New("runs")`
5. `internal/lidar/storage/sqlite/scene_store.go` — `id.New("scne")`
6. `internal/lidar/storage/sqlite/evaluation_store.go` — `id.New("eval")`
7. `internal/lidar/storage/sqlite/missed_region_store.go` — `id.New("regn")`
8. `internal/api/lidar_labels.go` — `id.New("labl")`
9. `internal/lidar/sweep/runner.go` (2 sites) — `id.New("swep")`
10. `internal/lidar/sweep/hint.go` — `id.New("swep")`
11. `internal/lidar/sweep/auto.go` — `id.New("swep")`

### Phase 3: Track prefix migration (`trk_` → `trak_`)

The existing `trk_` prefix is 3 characters. To align with the 4-character
convention, update `l5tracks/tracking.go` from `trk_` to `trak_`. The
golden replay test validates `track.TrackID[:4] == "trk_"` — update this
assertion to check for `trak_` (5 chars including underscore).

Accept mixed formats: existing `trk_`-prefixed IDs in SQLite coexist with
new `trak_`-prefixed IDs.

### Phase 4: SQLite migration

Accept mixed formats. All lookups treat IDs as opaque strings, so prefixed
and unprefixed IDs coexist without schema changes. Old rows keep their
existing format (bare UUIDs, `trk_*`, `replay-*`, `reprocess-*`); new rows
get the `xxxx_` prefix.

No `UPDATE` migration needed — IDs are opaque primary keys with no format
constraint.

### Phase 5: Validation (optional)

Add an `id.Parse(s string) (prefix, uuid string, err error)` that splits on
the first `_` and validates the UUID portion. Useful for API input validation
and debugging, but not required for correctness since IDs are opaque keys.

## ID Flow

```
id.New("xxxx")  →  SQLite (TEXT PK)  →  HTTP API (JSON + URL paths)  →  Clients (macOS / Web / CLI)
```

All downstream consumers treat IDs as opaque strings:

- **SQLite:** `TEXT PRIMARY KEY` / `TEXT NOT NULL` columns, exact-match `WHERE` clauses
- **API:** URL path segments (`/api/lidar/runs/{run_id}`) and JSON bodies
- **Web UI:** `run_id.substring(0, 8)` for display truncation
- **macOS visualiser:** `AppState.currentRunID`, `selectedTrackID` as opaque strings
- **Sweep system:** checkpoint/resume persistence by sweep_id exact match

## Exit Criteria

- Every `uuid.New()` / `uuid.NewString()` call site replaced with `id.New`.
- `replay-` and `reprocess-` composite formats replaced with `runy_` / `runs_`.
- `trk_` migrated to `trak_`.
- `internal/id` package exists with tests.
- No bare UUIDs generated anywhere in production code.
- Existing SQLite data continues to work (mixed format acceptance).
