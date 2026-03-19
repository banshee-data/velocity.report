# LiDAR Deterministic Run Config and Execution Metadata Plan

- **Status:** Draft
- **Layers:** Database, L8 Analytics, L9 Endpoints, L10 Clients, Recording/Replay
- **Precondition:** This plan assumes schema standardisation migrations `000030` and `000031` have already landed on `main`, including the post-31 table family names documented in commit `88ed856c5c0602af1f33d91542d3053d774a573a`.
- **Migration slot:** This work should be introduced as migration `000032`.
- **Related:** [LiDAR Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md), [Track Labelling & Auto-Aware Tuning](lidar-track-labeling-auto-aware-tuning-plan.md), `docs/plans/lidar-schema-standardisation-plan.md` on `main`, [VRLOG Wire Format Specification](../data/VRLOG_FORMAT.md), [VRLOG Analysis](../data/VRLOG_ANALYSIS.md)

## Goal

Replace the current scattered `params_json` model with a deterministic asset
model that cleanly separates reusable config from per-execution metadata and
answers:

1. What exact build version produced this run?
2. What exact effective runtime configuration did it use?
3. Can reusable requested params be shared across replay cases and future
   artifacts without being confused for executed configs?
4. Can the UI diff exact configs and group related runs deterministically?

The end state is:

- one canonical storage location for reusable parameter sets
- exact run-config rows deduplicated by deterministic content
- stable hashes with no timestamps, UUIDs, or other ephemeral fields in
  deterministic identity
- runs referencing the exact deterministic config that was actually executed
- replay cases and future recommendation artifacts referencing parameter sets by
  FK instead of copying JSON

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

### Three distinct layers

Separate three things that are currently collapsed into `params_json`:

1. deterministic parameter sets
2. exact deterministic run configs
3. non-deterministic execution metadata

This is the core design change for this plan:

- UUIDs, timestamps, and other per-execution facts stay on run/recording rows
- reusable parameter content is stored separately from exact executed configs
- exact reproducibility is represented by pairing a parameter set with embedded
  build identity on a deduplicated run-config row
- replay-case recommendations point to reusable parameter sets, not to
  build-specific executed configs

### Schema sketch

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
    build_name TEXT NOT NULL,
    build_version TEXT NOT NULL,
    build_git_sha TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (param_set_id, build_name, build_version, build_git_sha)
);

CREATE TABLE lidar_run_records (
    run_id TEXT PRIMARY KEY,
    run_config_id TEXT NOT NULL REFERENCES lidar_run_configs(run_config_id),
    requested_param_set_id TEXT REFERENCES lidar_param_sets(param_set_id),
    sensor_id TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_path TEXT,
    replay_case_id TEXT REFERENCES lidar_replay_cases(replay_case_id),
    parent_run_id TEXT REFERENCES lidar_run_records(run_id),
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    status TEXT NOT NULL,
    error_message TEXT,
    frame_start_ns INTEGER,
    frame_end_ns INTEGER,
    duration_secs REAL,
    total_frames INTEGER,
    total_clusters INTEGER,
    total_tracks INTEGER,
    confirmed_tracks INTEGER,
    processing_time_ms INTEGER,
    statistics_json TEXT,
    vrlog_path TEXT,
    notes TEXT
);
```

`created_at` above is row-bookkeeping only. It is not part of any deterministic
identity or hash.

Deliberate simplification:

- do not introduce a standalone `lidar_builds` table
- keep build identity embedded in `lidar_run_configs`
- do not split `lidar_run_records` into extra source/stats/header tables
- this stays aligned with the simplified 030/031-style table families and avoids
  new table families that have no useful life outside exact config or execution
  identity

### Semantics

`lidar_param_sets` stores canonical deterministic parameter payloads only:

- no UUIDs
- no timestamps
- no wall-clock metadata
- no source paths
- no run status or duration fields
- no unordered map instability
- no `NaN`/`Inf`
- durations normalised to a single representation
- all omitted values must mean exactly one thing

`param_set_type` encodes the **shape contract** of the JSON payload. Each
type implies a distinct JSON structure:

- `requested` — sparse subset of tuning keys. Only the keys the caller
  explicitly set. The `params` object is partial and may omit entire layer
  blocks. No `build` block.
- `effective` — the complete runtime tuning surface as resolved by the
  engine. Every layer and every key is present. No `build` block.
- `legacy` — historical backfills where coverage is incomplete. Shape may
  be partial or non-standard. No `build` block.

The composed run config exported on read adds a `build` block to the
effective param set. That composed shape uses a separate schema version
(`run_config/v1`) and is never stored in `lidar_param_sets`. The `build`
block is the distinguishing structural difference: if it is present, the
object is a composed run config, not a standalone param set.

`lidar_run_configs` stores the exact deterministic executed config:

- one `param_set_id`
- one embedded build identity block:
  `build_name`, `build_version`, `build_git_sha`
- one exact `config_hash`

These column names align with existing codebase conventions: `build_version`
in VRLOG `header.json` / analysis reports, and `version.Version` /
`version.GitSHA` / `version.BuildTime` set via ldflags.

`lidar_run_records` remains the single mutable execution envelope:

- one `run_config_id` for the exact executed config
- optional `requested_param_set_id` for launch intent when a run was started
  from an explicit override or replay-case recommendation
- source and lineage fields:
  `sensor_id`, `source_type`, `source_path`, `replay_case_id`, `parent_run_id`
- lifecycle fields:
  `created_at`, `completed_at`, `status`, `error_message`
- hot summary fields used by run lists and detail headers:
  `frame_start_ns`, `frame_end_ns`, `duration_secs`, `total_frames`,
  `total_clusters`, `total_tracks`, `confirmed_tracks`,
  `processing_time_ms`, `vrlog_path`
- optional `statistics_json` for nested, cold, non-identity metrics only

### Run-record design opinion

`lidar_run_records` should stay as one row per execution and be "fat enough" to
answer the common run-list and run-detail queries without inventing more light
tables.

- do not create separate `lidar_run_stats`, `lidar_run_sources`, or execution
  header tables
- if a field is used for filtering, sorting, joins, or list rendering, keep it
  as a typed column on `lidar_run_records`
- use `statistics_json` only for sparse or nested detail that is not hot in SQL
- do not duplicate config identity, build identity, or top-level run counters
  inside `statistics_json`
- if a metric becomes query-critical, promote it to a typed column and stop
  mirroring it in JSON

### Hash rules

- `params_hash` is SHA-256 of the canonical param-set JSON, including
  `param_set_type` and `schema_version`
- `config_hash` is SHA-256 of the exact composed run config:
  `effective param set + build_name + build_version + build_git_sha`
- same effective params + same build identity => same `config_hash`
- same effective params + different build identity => different `config_hash`
- same requested params reused across runs/builds => same `params_hash`
- `params_hash` is not a cross-schema compatibility key; if the param schema
  changes, the hash must change too

### Canonical JSON shapes

Requested parameter set stored in `lidar_param_sets.params_json`:

```json
{
  "schema_version": "requested/v1",
  "param_set_type": "requested",
  "params": {
    "background": {
      "background_update_fraction": 0.02,
      "safety_margin_meters": 0.35
    },
    "clustering": {
      "eps": 0.7,
      "min_pts": 5
    },
    "tracking": {
      "max_tracks": 128,
      "max_misses": 4,
      "hits_to_confirm": 3
    }
  }
}
```

Effective parameter set stored in `lidar_param_sets.params_json`:

```json
{
  "schema_version": "effective/v1",
  "param_set_type": "effective",
  "params": {
    "background": {
      "background_update_fraction": 0.02,
      "closeness_sensitivity_multiplier": 1.25,
      "safety_margin_meters": 0.35,
      "neighbor_confirmation_count": 3,
      "noise_relative_fraction": 0.12
    },
    "clustering": {
      "eps": 0.7,
      "min_pts": 5,
      "cell_size": 0.7
    },
    "tracking": {
      "max_tracks": 128,
      "max_misses": 4,
      "hits_to_confirm": 3,
      "gating_distance_squared": 9.0
    },
    "classification": {
      "model_type": "rule_based"
    }
  }
}
```

Exact run config is composed on read/export from the effective parameter set
plus embedded build identity:

```json
{
  "schema_version": "run_config/v1",
  "param_set_type": "effective",
  "build": {
    "build_name": "velocity.report",
    "build_version": "0.5.0-pre6",
    "build_git_sha": "7b5242213"
  },
  "params": {
    "background": {
      "background_update_fraction": 0.02,
      "closeness_sensitivity_multiplier": 1.25,
      "safety_margin_meters": 0.35,
      "neighbor_confirmation_count": 3,
      "noise_relative_fraction": 0.12
    },
    "clustering": {
      "eps": 0.7,
      "min_pts": 5,
      "cell_size": 0.7
    },
    "tracking": {
      "max_tracks": 128,
      "max_misses": 4,
      "hits_to_confirm": 3,
      "gating_distance_squared": 9.0
    },
    "classification": {
      "model_type": "rule_based"
    }
  }
}
```

Run-config row stored in `lidar_run_configs`:

```json
{
  "run_config_id": "rc_01HV7M3W6R6M2J2A8M4T7B2E1C",
  "config_hash": "sha256:8b7442f7e3b1c1b4c2d7e4aa2a10d5db2ef7d34e89d9d1f6a84c6f4e21a8f95a",
  "param_set_id": "ps_01HV7M2W3JY8Q6R4B1D5N9T2K7",
  "build_name": "velocity.report",
  "build_version": "0.5.0-pre6",
  "build_git_sha": "7b5242213",
  "created_at": 1773952005123456789
}
```

Run-record row stored in `lidar_run_records`:

```json
{
  "run_id": "run_01HV7M4102M8QBX6NQ7P91QK0F",
  "run_config_id": "rc_01HV7M3W6R6M2J2A8M4T7B2E1C",
  "requested_param_set_id": "ps_01HV7M1M9H6R5C2X4D8T3N7P0A",
  "sensor_id": "kirkland_northbound",
  "source_type": "pcap_replay",
  "source_path": "/data/pcap/kirkland/kirk1.pcap",
  "replay_case_id": "case_kirk1_evening_rain",
  "parent_run_id": "run_01HV7JYME6J8QW8Q1K2Q2R3H5N",
  "created_at": 1773952006000000000,
  "completed_at": 1773952047032000000,
  "status": "completed",
  "error_message": null,
  "frame_start_ns": 1707436800000000000,
  "frame_end_ns": 1707436842034000000,
  "duration_secs": 42.034,
  "total_frames": 420,
  "total_clusters": 5821,
  "total_tracks": 118,
  "confirmed_tracks": 97,
  "processing_time_ms": 41032,
  "statistics_json": {
    "schema_version": "run_statistics/v1",
    "source_window": {
      "pcap_start_secs": 120.0,
      "pcap_duration_secs": 45.0
    },
    "label_rollup": {
      "classified": 63,
      "tagged_only": 14,
      "unlabelled": 41
    },
    "quality": {
      "split_candidate_count": 4,
      "merge_candidate_count": 2
    }
  },
  "vrlog_path": "/var/vrlog/run_01HV7M4102M8QBX6NQ7P91QK0F.vrlog",
  "notes": null
}
```

## Data Ownership After Migration

### Deterministic assets

- `lidar_param_sets`
- `lidar_run_configs`

### Execution records

- `lidar_run_records.run_config_id`
- `lidar_run_records.requested_param_set_id` when launch intent is known
- all per-run UUID, timestamp, source, parent, status, and duration fields
- hot summary counters and `statistics_json`

### Recommendation refs

- `lidar_replay_cases.recommended_param_set_id`

This points to reusable requested params, not to an executed config for any run.

### Derived only

- evaluation config data in `lidar_replay_evaluations` should always derive
  from `candidate_run_id -> run_config_id`
- run diff views should diff the composed exact config from
  `run_config_id -> param_set_id + embedded build identity`
- grouping in the UI should use `params_hash` from `effective` parameter sets
  only

### Future refs

Future entities must choose the correct level of reference:

- use `run_config_id` for exact executed provenance
- use `param_set_id` for reusable tuning intent or recommendations
- never copy canonical JSON blobs into consumer tables

## Phase Plan

## P0 / P1: Introduce and Adopt Deterministic Assets

### P0.1 Schema additions

Add migration `000032` on `main` that:

1. Creates `lidar_param_sets`
2. Creates `lidar_run_configs`
3. Adds nullable `run_config_id` to `lidar_run_records`
4. Adds nullable `requested_param_set_id` to `lidar_run_records`
5. Adds nullable `replay_case_id`, `completed_at`, `frame_start_ns`, and
   `frame_end_ns` to `lidar_run_records` if they do not already exist
6. Adds nullable `recommended_param_set_id` to `lidar_replay_cases`
7. Adds indexes:
   - `idx_lidar_param_sets_params_hash`
   - `idx_lidar_run_configs_config_hash`
   - `idx_lidar_run_records_run_config`
   - `idx_lidar_run_records_requested_param_set`
   - `idx_lidar_run_records_replay_case`
   - `idx_lidar_replay_cases_recommended_param_set`

Keep the legacy JSON columns during P0/P1:

- `lidar_run_records.params_json`
- `lidar_replay_cases.optimal_params_json`
- `lidar_replay_evaluations.params_json`
- `lidar_bg_snapshot.params_json`

### P0.2 Config asset package

Add one config-asset package, likely `internal/lidar/configasset/`, that:

- captures full effective runtime parameters
- captures reusable requested params
- captures current build identity
- builds canonical parameter-set JSON deterministically
- computes `params_hash` and `config_hash`
- validates the absence of forbidden fields
- inserts or reuses deduplicated parameter-set and run-config rows
- writes optional run-intent lineage (`requested_param_set_id`) without
  confusing it for exact executed provenance

Suggested API:

```go
type ParamSet struct {
    ParamSetID          string
    ParamsHash          string
    SchemaVersion       string
    ParamSetType        string
    ParamsJSON          []byte
}

type BuildIdentity struct {
    BuildName    string
    BuildVersion string
    BuildGitSHA  string
}

type RunConfig struct {
    RunConfigID   string
    ConfigHash    string
    ParamSetID    string
    BuildName     string
    BuildVersion  string
    BuildGitSHA   string
}

func MakeEffectiveParamSet(runtime Snapshot) (*ParamSet, error)
func MakeRequestedParamSet(request Snapshot) (*ParamSet, error)
func ReadBuildIdentity(info BuildInfo) (BuildIdentity, error)
func EnsureRunConfig(db *sql.DB, paramSet *ParamSet, build BuildIdentity) (*RunConfig, error)
```

### P0.3 Define the effective runtime surface

`effective` parameter sets must cover the actual runtime tuning surface used by
execution, not just the old `RunParams` subset.

At minimum this includes:

- background parameters
- clustering parameters
- tracker parameters
- extended tracker parameters
- classification-related tunables that affect output
- any replay-mode parameters that materially change results

The implementation must not rely on partial request payloads as the source of
truth. It must resolve to the effective applied parameter set.

### P0.4 Remove timestamps from deterministic config identity

The current `RunParams.Timestamp` field is incompatible with deterministic,
hashable assets and must not participate in parameter-set or run-config
identity.

Requirements:

- remove timestamps from canonical parameter sets
- keep timestamps and UUIDs only on execution rows / recording headers
- deprecate timestamp-bearing `RunParams` usage for persistence
- update tests that currently expect timestamp-bearing persisted config

### P0.5 Fix run ownership and eliminate duplicate run creation

This is mandatory before the new asset model can be trusted.

Replay and reprocess currently insert run rows before execution, then the
analysis runtime creates another run internally. P0/P1 must make run creation
single-sourced:

- exactly one run row per execution
- exactly one `run_config_id` attached to that run
- the run row must be created only after the effective param set and current
  build identity have been resolved into a run config
- if explicit launch intent exists, the row may also carry
  `requested_param_set_id`, but exact reproducibility still hangs off
  `run_config_id`

Implementation guidance:

- make `AnalysisRunManager` or an equivalent orchestration path the only run
  creator
- remove the pre-insert pattern from replay/reprocess endpoints
- preserve endpoint-owned execution metadata such as `parent_run_id`,
  source/replay window, and scene linkage in that orchestration path
- return the final `run_id` from that orchestration path so endpoint API
  behaviour stays stable

### P0.6 Requested params vs effective params vs execution metadata

The system must distinguish between:

- requested params: user override, replay-case recommendation, or
  sweep-selected candidate values
- effective params: the full resolved runtime state that the engine actually
  used
- build identity: the binary/code identity that executed the run
- execution metadata: UUIDs, timestamps, source paths, replay windows,
  statuses, and derived run totals

Storage rules:

- requested params may be stored in `lidar_param_sets` as
  `param_set_type = 'requested'`
- effective params must be stored in `lidar_param_sets` as
  `param_set_type = 'effective'`
- launch intent may be attached to `lidar_run_records.requested_param_set_id`
  when known
- exact executed configs live in `lidar_run_configs` with embedded build
  identity
- execution metadata must not be hashed into `params_hash` or `config_hash`
- `statistics_json` must not duplicate exact config, build identity, or the
  top-level typed counters already present on `lidar_run_records`

### P0.7 Backfill historical rows

Add a backfill step in the migration or a one-shot repair command that:

1. Reads existing `lidar_run_records.params_json`
2. Canonicalises recoverable content into `lidar_param_sets`
3. Marks those rows as `effective` where exact resolution is possible
4. Marks them as `legacy` where coverage is incomplete
5. Resolves `lidar_run_configs` with whatever build identity is known
6. Uses explicit `unknown` build values where historical build identity cannot
   be recovered
7. Sets `lidar_run_records.run_config_id`
8. Leaves `requested_param_set_id` NULL unless original launch intent is known
   with confidence
9. Reads existing `lidar_replay_cases.optimal_params_json`
10. Canonicalises it into `lidar_param_sets` as `requested`
11. Sets `lidar_replay_cases.recommended_param_set_id`

Important:

- do not backfill replay-case recommendation blobs into `lidar_run_configs`
- do not pretend `legacy` rows are exact reproducibility assets

### P0.8 API changes

Add deterministic config identity to the run and replay-case APIs.

Runs should expose:

- `run_config_id`
- `requested_param_set_id`
- `param_set_id`
- `build_name`
- `build_version`
- `build_git_sha`
- `config_hash`
- `params_hash`
- `schema_version`
- `param_set_type`
- `replay_case_id`
- `completed_at`
- `frame_start_ns`
- `frame_end_ns`
- `statistics_json`
- optionally expanded exact config composed from param set + build identity

Replay cases should expose:

- `recommended_param_set_id`
- optionally expanded requested params summary

During P0/P1, legacy fields may still be returned for compatibility, but new UI
work should read the reference model.

### P0.9 UI changes

Update the web UI to use references instead of raw JSON blobs.

Requirements:

- run detail pages diff exact composed configs
- run grouping and toggle logic use `params_hash` from effective param sets
- replay cases show requested params summaries, not build-specific run-config
  IDs
- replay cases use explicit "clone/edit as new recommendation" and "resolve
  against current build" flows
- remove reliance on stringly `optimal_params_json` parsing in new UI paths

### P0.10 Recording provenance

VRLOG provenance should be upgraded from the current tuning-hash approach to the
deterministic asset model.

P0/P1 requirements:

- write `config_hash` into recording metadata
- write `params_hash` into recording metadata
- emit a portable `execution_config.json` beside `header.json`
- keep non-deterministic recording facts in `header.json` rather than in the
  deterministic config export

This makes a recording a self-contained artifact with a clean split between:

- deterministic exact config
- non-deterministic recording/session metadata

If backward compatibility is needed:

- keep `tuning_hash` during transition as deprecated metadata
- derive it from `params_hash` only if necessary for readers
- prefer `config_hash` / `params_hash` for all new tooling

### P0.11 Test plan

Add or update tests for:

- canonicalisation stability for parameter sets
- hash stability for equivalent param sets and run configs
- `config_hash` divergence when build identity changes
- timestamps/UUIDs never changing `params_hash` or `config_hash`
- replay/reprocess creating exactly one run row
- run rows always having the correct `run_config_id`
- explicit requested launch intent preserved on `requested_param_set_id` when
  applicable
- backfill correctness for `effective`, `requested`, and `legacy`
- replay-case `recommended_param_set_id` behaviour
- recording metadata preserving the deterministic / non-deterministic split

### P0/P1 acceptance criteria

- every new run record has a non-empty `run_config_id`
- the attached run config is the exact deterministic pairing that was executed
- `requested_param_set_id` is optional lineage only and never replaces
  `run_config_id`
- equivalent parameter sets deduplicate independently and equivalent exact run
  configs deduplicate to one row
- `params_hash` groups only exact-equal param sets within the same
  schema/version/type
- reprocess/replay no longer create duplicate run rows
- replay cases reference reusable `requested` parameter sets rather than
  build-specific executed configs
- a recording can be inspected offline to recover both the exact deterministic
  config and the per-recording execution metadata

## P2: Remove Legacy Duplication and Enforce the New Model

P2 should only start after P0/P1 is fully landed, backfilled, and exercised in
real workflows.

### P2.1 Drop redundant JSON columns

Add a cleanup migration using the next available migration number that drops:

- `lidar_run_records.params_json`
- `lidar_replay_evaluations.params_json`

For `lidar_bg_snapshot.params_json`, choose one of two paths:

- drop it if it remains dead schema
- or replace it with a truly scoped background-only parameter-set reference

Do not keep a misleading placeholder column.

For replay cases:

- remove `optimal_params_json`
- keep `recommended_param_set_id` as the durable model

### P2.2 Tighten constraints

After backfill and runtime adoption:

- make `lidar_run_records.run_config_id NOT NULL`
- add FK constraints for `run_config_id` and `recommended_param_set_id`
- ensure new code paths cannot create config-less runs

### P2.3 Evaluation and diff cleanup

All evaluation and compare flows should derive config identity from run FKs.

Requirements:

- no persisted evaluation config snapshots in `lidar_replay_evaluations`
- no config diff path reading legacy run JSON blobs
- exact diff uses `run_config_id -> param_set_id + embedded build identity`

### P2.4 UI simplification

After legacy column removal:

- remove legacy JSON rendering paths
- remove raw-string config editors tied directly to replay-case blobs
- use parameter-set summaries and explicit clone/edit flows instead of mutating
  shared blobs in place
- keep build-identity resolution explicit when turning a recommendation into a
  run

### P2.5 Future artifact attachments

Extend the reference model to any new artifact introduced after P0/P1, for
example:

- grouped track sets
- comparison bundles
- recording catalogue tables
- sweep recommendations promoted to reusable parameter sets

Guideline:

- reference `run_config_id` for exact executed provenance
- reference `param_set_id` for reusable tuning intent
- never duplicate canonical JSON into consumer tables

### P2.6 Test plan

Add cleanup-phase tests for:

- schema migration up/down correctness
- no remaining code paths reading legacy config columns
- exact diff and grouping continuing to work using only deterministic references

### P2 acceptance criteria

- no duplicated persisted config JSON remains on runs or evaluations
- deterministic assets are separated cleanly from execution metadata
- the UI groups by exact parameter-set identity and diffs by exact run config
- exact reproducibility is expressed by `config_hash`, not by ad hoc JSON blobs

## Implementation Notes for an Agent

### Recommended sequencing

1. Introduce `param_set` and `run_config` schema plus the config-asset package
2. Fix single-source run creation with an explicit execution-orchestration path
3. Capture full effective parameter sets at runtime
4. Capture current build identity and resolve exact run configs
5. Attach run configs to new runs
6. Backfill existing runs and replay-case requested params
7. Expose deterministic config metadata through API
8. Update UI and recording provenance around the deterministic /
   non-deterministic split
9. Remove legacy columns in P2

### Files likely touched in P0/P1

- `internal/db/migrations/`
- `internal/db/schema.sql`
- `internal/lidar/configasset/`
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

- no UUIDs or timestamps in parameter-set or run-config identity
- no mutable updates to existing deterministic asset rows
- no build-specific `run_config_id` used as a mutable recommendation pointer
- no generic "best effort" request snapshot treated as authoritative effective
  params
- no duplicate run creation paths
- no grouping across schema changes by raw hash alone

## Summary

The core decision is simple:

- reusable parameter intent should live in `lidar_param_sets`
- exact reproducibility should live in `lidar_run_configs` with embedded build
  identity
- run rows and recording headers should keep UUIDs, timestamps, and other
  execution metadata
- replay cases should recommend parameter sets, not build-specific run configs
- exact reproducibility should be keyed by `config_hash`
- grouping should be keyed by exact parameter-set identity via `params_hash`

P0/P1 establishes the model and migrates live code onto it.
P2 removes the old duplicated JSON storage once the new path is proven.
