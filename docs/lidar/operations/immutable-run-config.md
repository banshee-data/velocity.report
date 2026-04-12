# Immutable run configuration

Deterministic asset model for LiDAR run configuration: separating reusable parameter sets from exact executed configs and per-execution metadata. Replaces the current scattered `params_json` model.

## Source

- Plan: `docs/plans/lidar-immutable-run-config-asset-plan.md`
- Status: Complete — all phases delivered (P0/P1 + P2)
- Migrations: 000035 (schema additions), 000036 (legacy column removal)

## Problem

The previous schema spread parameter JSON across four places (`lidar_run_records.params_json`, `lidar_replay_cases.optimal_params_json`, `lidar_replay_evaluations.params_json`, `lidar_bg_snapshot.params_json`). This was not DRY and not reliable as a reproducibility model:

- Replay/reprocess paths created ad hoc run rows before execution, so stored params could diverge from the executed run
- `RunParams` included a timestamp, breaking immutability and stable hashing
- Run params were not a complete snapshot of the effective runtime tuning surface
- Evaluations duplicated candidate-run params rather than referencing them

All of these issues are now resolved. Legacy `params_json` columns have been dropped from `lidar_run_records`, `lidar_replay_cases` (`optimal_params_json`), and `lidar_replay_evaluations` (migration 000036). `lidar_bg_snapshot.params_json` is retained for background algorithm reproducibility.

## Target model: three distinct layers

### 1. Deterministic parameter sets (`lidar_param_sets`)

Canonical deterministic parameter payloads only. No UUIDs, no timestamps, no wall-clock metadata, no source paths. Durations normalised. All omitted values must mean exactly one thing.

`param_set_type` encodes the shape contract:

- `requested` — sparse subset of tuning keys (partial, user-specified overrides)
- `effective` — complete runtime tuning surface as resolved by the engine (every key present)
- `legacy` — historical backfills where coverage is incomplete

### 2. Exact run configs (`lidar_run_configs`)

Pairs an effective parameter set with embedded build identity (`build_version`, `build_git_sha`). Deduplicated by `config_hash`.

Same effective params + same build = same `config_hash`.
Same effective params + different build = different `config_hash`.

### 3. Execution records (`lidar_run_records`)

Single mutable execution envelope: `run_config_id` for exact executed config, optional `requested_param_set_id` for launch intent, plus source/lineage fields, lifecycle fields, and hot summary counters.

**Design opinion:** Keep `lidar_run_records` as one fat row per execution. Do not create separate stats/sources/header tables. If a field is used for filtering, sorting, joins, or list rendering, keep it as a typed column.

## Schema

```sql
CREATE TABLE lidar_param_sets (
    param_set_id TEXT PRIMARY KEY,
    params_hash TEXT NOT NULL UNIQUE,
    schema_version TEXT NOT NULL,
    param_set_type TEXT NOT NULL,
    params_json TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE lidar_run_configs (
    run_config_id TEXT PRIMARY KEY,
    config_hash TEXT NOT NULL UNIQUE,
    param_set_id TEXT NOT NULL REFERENCES lidar_param_sets(param_set_id),
    build_version TEXT NOT NULL,
    build_git_sha TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (param_set_id, build_version, build_git_sha)
);
```

`lidar_run_records` gains `run_config_id` (NOT NULL after P2), `requested_param_set_id` (optional), and `replay_case_id`.

## Hash rules

- `params_hash` = SHA-256 of canonical param-set JSON including `param_set_type` and `schema_version`
- `config_hash` = SHA-256 of exact composed run config (effective param set + build identity)
- `created_at` is row-bookkeeping only — not part of any hash
- If the param schema changes, the hash must change too

## Canonical JSON shapes

**Requested parameter set** (stored in `lidar_param_sets.params_json`):

```json
{
  "schema_version": "requested/v1",
  "param_set_type": "requested",
  "params": {
    "background": { "background_update_fraction": 0.02 },
    "clustering": { "eps": 0.7, "min_pts": 5 },
    "tracking": { "max_tracks": 128 }
  }
}
```

**Effective parameter set** (complete — every layer, every key):

```json
{
  "schema_version": "effective/v1",
  "param_set_type": "effective",
  "params": {
    "background": { ... all keys ... },
    "clustering": { ... all keys ... },
    "tracking": { ... all keys ... },
    "classification": { ... all keys ... }
  }
}
```

**Composed run config** (composed on read/export — never stored in `lidar_param_sets`):

```json
{
  "schema_version": "run_config/v1",
  "param_set_type": "effective",
  "build": { "build_version": "0.5.0-pre6", "build_git_sha": "7b5242213" },
  "params": { ... }
}
```

The `build` block is the structural distinguisher: if present, it is a composed run config, not a standalone param set.

## Config asset package

`internal/lidar/storage/configasset/` — captures full effective runtime parameters, reusable requested params, current build identity. Builds canonical JSON deterministically, computes hashes, validates absence of forbidden fields, inserts or reuses deduplicated rows.

## Data ownership after migration

- **Deterministic assets:** `lidar_param_sets`, `lidar_run_configs`
- **Execution records:** `lidar_run_records` (run_config_id, requested_param_set_id, all per-run mutable fields)
- **Recommendation refs:** `lidar_replay_cases.recommended_param_set_id` → reusable requested params, not executed configs
- **Derived only:** evaluation config from `candidate_run_id → run_config_id`; diff views from `run_config_id → param_set_id + build identity`; grouping by `params_hash` from effective param sets

## Delivered phases

### P0/P1: introduce and adopt (migration 000035)

- Schema additions — `lidar_param_sets`, `lidar_run_configs`, nullable FKs on run_records and replay_cases
- Config asset package (`internal/lidar/storage/configasset/`)
- Effective runtime surface definition (background, clustering, tracker, classification tunables)
- Timestamp removal from deterministic config identity
- Single-source run creation (no duplicate run rows in replay/reprocess)
- Requested vs effective vs execution metadata separation
- Backfill tool (`cmd/tools/backfill_lidar_run_config`)
- API exposure of config identity on run and replay-case responses
- UI diff of exact composed configs, grouping by `params_hash`
- Recording provenance (`config_hash` and `params_hash` in VRLOG metadata)

### P2: remove legacy duplication (migration 000036)

- Dropped `params_json` from `lidar_run_records` and `lidar_replay_evaluations`
- Dropped `optimal_params_json` from `lidar_replay_cases`
- Evaluation and diff derive all config from run FKs
- UI uses only immutable config references (no legacy JSON rendering)

## Key guardrails

- No UUIDs or timestamps in parameter-set or run-config identity
- No mutable updates to existing deterministic asset rows
- No build-specific `run_config_id` used as a mutable recommendation pointer
- No duplicate run creation paths
- No grouping across schema changes by raw hash alone
- `statistics_json` must not duplicate exact config, build identity, or top-level typed counters
