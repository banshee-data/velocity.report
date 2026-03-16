# LiDAR Immutable Run Config Asset Plan

- **Status:** Draft
- **Layers:** Database, L8 Analytics, L9 Endpoints, L10 Clients, Recording/Replay
- **Precondition:** This plan assumes schema standardisation migrations `000030` and `000031` have already landed on `main`, including the post-31 table family names documented in commit `88ed856c5c0602af1f33d91542d3053d774a573a`.
- **Migration slot:** This work should be introduced as migration `000032`.
- **Related:** [LiDAR Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md), [Track Labelling & Auto-Aware Tuning](lidar-track-labeling-auto-aware-tuning-plan.md), `docs/plans/lidar-schema-standardisation-plan.md` on `main`, [VRLOG Wire Format Specification](../data/VRLOG_FORMAT.md), [VRLOG Analysis](../data/VRLOG_ANALYSIS.md)

## Goal

Replace the current scattered `params_json` model with one immutable,
canonical, hashable run-config asset that answers:

1. What exact engine version produced this run?
2. What exact effective runtime configuration did it use?
3. Can this config be shared across runs, recordings, replay cases, and future track-group views?
4. Can the UI diff exact configs and group related runs deterministically?

The end state is:

- one canonical storage location for reproducible run configs
- immutable config rows, deduplicated by content hash
- stable hashes with no timestamps or ephemeral fields
- runs referencing the exact config that was actually executed
- replay cases and future grouped artifacts referencing configs by FK instead of copying JSON

## Motivation

The current schema spreads parameter JSON across multiple places:

- `lidar_run_records.params_json`
- `lidar_replay_cases.optimal_params_json`
- `lidar_replay_evaluations.params_json`
- `lidar_bg_snapshot.params_json`

That is not DRY, and more importantly it is not reliable as a reproducibility
model:

- replay/reprocess paths currently create ad hoc run rows before the actual
  analysis run starts, so the stored params can diverge from the executed run
- `RunParams` currently includes a timestamp, which breaks immutability and
  stable hashing
- run params are not a complete snapshot of the effective runtime tuning
  surface
- evaluations duplicate candidate-run params rather than referencing them
- background snapshot params are currently dead schema in practice
- replay cases store a mutable best-known params blob in a shape that is different
  from run params and is treated as a raw string by parts of the UI

This prevents deterministic grouping, exact diffing, and trustworthy replay
provenance.

## Non-goals

- Redesigning the full tuning API shape in the same change
- Solving every future artifact relationship up front with a generic
  polymorphic association table
- Reworking sweep result schemas beyond what is needed to attach canonical
  run configs
- Retrofitting perfect provenance onto historical rows that never captured
  enough data to reconstruct a full effective config

## Target Model

### Core concept

Introduce a single immutable asset table:

```sql
CREATE TABLE lidar_run_configs (
    run_config_id TEXT PRIMARY KEY,
    config_hash TEXT NOT NULL UNIQUE,
    params_hash TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    engine_name TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    git_sha TEXT NOT NULL,
    canonical_json TEXT NOT NULL,
    provenance_status TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
```

### Semantics

- `canonical_json` is the full effective run config actually applied to
  the engine, not the user request payload
- `config_hash` is SHA-256 of the canonical full asset, including engine
  identity and schema version
- `params_hash` is SHA-256 of the canonical params block only, excluding engine
  identity
- same params + same engine identity => same `config_hash`
- same params + different engine identity => different `config_hash`, same
  `params_hash`
- rows are immutable after insert

### Why two hashes

`config_hash` is for exact reproducibility:

- same engine
- same effective config
- exact replay identity

`params_hash` is for grouping and comparison:

- same effective params family across builds
- UI grouping and toggle logic
- regression views where engine version differs but parameter intent is the same

### Canonical JSON shape

The canonical asset should have two top-level blocks:

```json
{
  "schema_version": "v1",
  "engine": {
    "name": "velocity.report",
    "version": "0.5.0-preX",
    "git_sha": "abc123"
  },
  "params": {
    "...": "full resolved runtime config"
  }
}
```

Rules:

- no timestamps
- no wall-clock metadata
- no file paths
- no request-only partial fields
- no unordered map instability
- no `NaN`/`Inf`
- durations normalised to a single representation
- all omitted values must mean exactly one thing

## Data Ownership After Migration

### Authoritative

- `lidar_run_configs`
- `lidar_run_records.run_config_id`

### Mutable references

- `lidar_replay_cases.recommended_run_config_id`

This is not the executed config for any run. It is the replay case's current
recommended or best-known config reference.

### Derived only

- evaluation config data in `lidar_replay_evaluations` should always derive
  from `candidate_run_id -> run_config_id`
- run diff views should diff canonical config assets
- grouping in the UI should use `params_hash`

### Future attachment model

Future entities such as grouped track sets, comparison bundles, or recording
catalogues should reference `run_config_id` directly. Do not copy blobs
into those tables.

## Phase Plan

## P0 / P1: Introduce and Adopt the Immutable Config Asset

### P0.1 Schema additions

Add migration `000032` on `main` that:

1. Creates `lidar_run_configs`
2. Adds nullable `run_config_id` to `lidar_run_records`
3. Adds nullable `recommended_run_config_id` to `lidar_replay_cases`
4. Adds indexes:
   - `idx_lidar_run_configs_params_hash`
   - `idx_lidar_run_records_run_config`
   - `idx_lidar_replay_cases_recommended_run_config`

Keep the legacy JSON columns during P0/P1:

- `lidar_run_records.params_json`
- `lidar_replay_cases.optimal_params_json`
- `lidar_replay_evaluations.params_json`
- `lidar_bg_snapshot.params_json`

### P0.2 Central canonicalisation package

Add one canonical builder package, likely `internal/lidar/configasset/`, that:

- captures full effective runtime config
- builds canonical JSON deterministically
- computes `config_hash`
- computes `params_hash`
- validates the absence of forbidden fields
- inserts or reuses a deduplicated config row

Suggested API:

```go
type Asset struct {
    RunConfigID       string
    ConfigHash        string
    ParamsHash        string
    CanonicalJSON     []byte
    SchemaVersion     string
    EngineName        string
    EngineVersion     string
    GitSHA            string
}

func BuildEffectiveRunConfig(runtime Snapshot) (*Asset, error)
func UpsertRunConfig(db *sql.DB, asset *Asset) error
```

### P0.3 Define the effective runtime surface

The asset must cover the actual runtime tuning surface used by execution, not
just the old `RunParams` subset.

At minimum this includes:

- background parameters
- clustering parameters
- tracker parameters
- extended tracker parameters
- classification-related tunables that affect output
- any replay-mode parameters that materially change results

The implementation must not rely on partial request payloads as the source of
truth. It must resolve to the effective applied config.

### P0.4 Remove timestamps from the run-config model

The current `RunParams.Timestamp` field is incompatible with immutable,
hashable config assets and must not participate in the new canonical model.

Requirements:

- remove timestamps from the new canonical asset
- deprecate timestamp-bearing `RunParams` usage for persistence
- update tests that currently expect timestamp-bearing persisted config

### P0.5 Fix run ownership and eliminate duplicate run creation

This is mandatory before the new asset can be trusted.

Replay and reprocess currently insert run rows before execution, then the
analysis runtime creates another run internally. P0/P1 must make run creation
single-sourced:

- exactly one run row per execution
- exactly one `run_config_id` attached to that run
- the run row must be created only after the effective config asset has been
  built or resolved

Implementation guidance:

- make `AnalysisRunManager` or an equivalent single orchestration path the only
  run creator
- remove the pre-insert pattern from replay/reprocess endpoints
- allow replay/reprocess to pass a requested override, but the stored run must
  reference the resolved effective asset actually used

### P0.6 Requested config vs effective config

The system must distinguish between:

- requested config: user override, replay-case recommendation, or sweep-selected
  candidate values
- effective config: the full resolved runtime state that the engine actually
  used

Only the effective config goes into `lidar_run_configs`.

Requested configs may still exist transiently in:

- request bodies
- sweep requests
- replay-case recommendation workflows

But they must not be treated as reproducibility assets.

### P0.7 Backfill historical rows

Add a backfill step in the migration or a one-shot repair command that:

1. Reads existing `lidar_run_records.params_json`
2. Canonicalises whatever can be recovered into `lidar_run_configs`
3. Sets `lidar_run_records.run_config_id`
4. Reads existing `lidar_replay_cases.optimal_params_json`
5. Canonicalises into `lidar_run_configs`
6. Sets `lidar_replay_cases.recommended_run_config_id`

Historical configs that cannot be reconstructed fully should still be stored as
immutable assets, but marked clearly:

- `provenance_status = 'legacy_partial'`
- engine version may be `unknown`
- they must not pretend to be exact reproducible assets if they are not

### P0.8 API changes

Add config identity to the run and replay-case APIs.

Runs should expose:

- `run_config_id`
- `config_hash`
- `params_hash`
- `engine_version`
- optionally expanded canonical config

Replay cases should expose:

- `recommended_run_config_id`
- optionally expanded recommended config summary

During P0/P1, legacy fields may still be returned for compatibility, but new UI
work should read the config-reference model.

### P0.9 UI changes

Update the web UI to use references instead of raw JSON blobs.

Requirements:

- run detail pages diff canonical config assets
- run grouping and toggle logic use `params_hash`
- replay cases show recommended config summary via FK-backed config metadata
- remove reliance on stringly `optimal_params_json` parsing in new UI paths

### P0.10 Recording provenance

VRLOG provenance should be upgraded from the current tuning-hash approach to
the immutable run-config model.

P0/P1 requirements:

- write `config_hash` into recording metadata
- write `params_hash` into recording metadata
- emit a portable `execution_config.json` beside `header.json`

This makes a recording a self-contained reproducible artifact even when detached
from the database.

If backward compatibility is needed:

- keep `tuning_hash` during transition as deprecated metadata
- derive it from `params_hash` only if necessary for readers
- prefer `config_hash` / `params_hash` for all new tooling

### P0.11 Test plan

Add or update tests for:

- canonicalisation stability
- hash stability for equivalent configs
- hash divergence when engine version changes
- replay/reprocess creating exactly one run row
- run rows always having the correct `run_config_id`
- backfill correctness
- replay-case recommendation FK behaviour
- recording metadata including config identity

### P0/P1 acceptance criteria

- every new run record has a non-empty `run_config_id`
- the attached config is the exact effective config that was executed
- equivalent configs deduplicate to one asset row
- `params_hash` groups same-params runs across engine versions
- reprocess/replay no longer create duplicate run rows
- replay cases can reference a recommended config without storing free-form JSON as
  the long-term source of truth
- a recording can be inspected offline to recover the exact canonical run
  config

## P2: Remove Legacy Duplication and Enforce the New Model

P2 should only start after P0/P1 is fully landed, backfilled, and exercised in
real workflows.

### P2.1 Drop redundant JSON columns

Add a cleanup migration using the next available migration number that drops:

- `lidar_run_records.params_json`
- `lidar_replay_evaluations.params_json`

For `lidar_bg_snapshot.params_json`, choose one of two paths:

- drop it if it remains dead schema
- or replace it with a truly scoped background-only immutable config reference

Do not keep a misleading placeholder column.

For replay cases:

- remove `optimal_params_json`
- keep `recommended_run_config_id` as the durable model

### P2.2 Tighten constraints

After backfill and runtime adoption:

- make `lidar_run_records.run_config_id NOT NULL`
- add FK constraints where still nullable during P0/P1
- ensure new code paths cannot create config-less runs

### P2.3 Evaluation and diff cleanup

All evaluation and compare flows should derive config identity from run FKs.

Requirements:

- no persisted evaluation config snapshots in `lidar_replay_evaluations`
- no config diff path reading legacy run JSON blobs
- exact diff uses canonical config asset by `run_config_id`

### P2.4 UI simplification

After legacy column removal:

- remove legacy JSON rendering paths
- remove raw-string config editors tied directly to replay-case blobs
- use config summaries and explicit “clone/edit as new recommendation” flows
  instead of mutating shared blobs in place

### P2.5 Future artifact attachments

Extend the FK model to any new artifact introduced after P0/P1, for example:

- grouped track sets
- comparison bundles
- recording catalogue tables
- sweep recommendations promoted to reusable configs

Guideline:

- reference `run_config_id`
- never duplicate canonical config JSON into consumer tables

### P2.6 Test plan

Add cleanup-phase tests for:

- schema migration up/down correctness
- no remaining code paths reading legacy config columns
- exact diff and grouping continuing to work using only config references

### P2 acceptance criteria

- no duplicated persisted config JSON remains on runs or evaluations
- runs, replay cases, recordings, and future attached artifacts all use the same
  immutable config identity model
- the UI groups by `params_hash` and diffs by canonical config asset
- exact reproducibility is expressed by `config_hash`, not by ad hoc JSON blobs

## Implementation Notes for an Agent

### Recommended sequencing

1. Introduce schema and config-asset package
2. Fix single-source run creation
3. Capture full effective config at runtime
4. Attach config assets to new runs
5. Backfill existing runs and replay cases
6. Expose config metadata through API
7. Update UI and recording provenance
8. Remove legacy columns in P2

### Files likely touched in P0/P1

- `internal/db/migrations/`
- `internal/db/schema.sql`
- `internal/lidar/storage/sqlite/analysis_run.go`
- `internal/lidar/storage/sqlite/analysis_run_manager.go`
- `internal/lidar/storage/sqlite/scene_store.go`
- `internal/lidar/storage/sqlite/evaluation_store.go`
- `internal/lidar/monitor/scene_api.go`
- `internal/lidar/monitor/run_track_api.go`
- `internal/lidar/monitor/datasource_handlers.go`
- `internal/lidar/monitor/webserver.go`
- `internal/lidar/visualiser/recorder/recorder.go`
- `cmd/radar/radar.go`
- `web/src/lib/types/lidar.ts`
- `web/src/lib/api.ts`
- `web/src/routes/lidar/runs/+page.svelte`
- `web/src/routes/lidar/scenes/+page.svelte`

### Key guardrails

- no timestamps in canonical persisted config assets
- no mutable updates to existing config rows
- no generic "best effort" request snapshot treated as authoritative
- no duplicate run creation paths
- no grouping keyed off non-canonical JSON serialisation

## Summary

The core decision is simple:

- runs should point to one immutable canonical run-config asset
- replay cases should recommend configs by reference, not store mutable blobs
- recordings should embed the same config identity for portability
- exact reproducibility should be keyed by `config_hash`
- grouping and diff-association should be keyed by `params_hash`

P0/P1 establishes the model and migrates live code onto it.
P2 removes the old duplicated JSON storage once the new path is proven.
