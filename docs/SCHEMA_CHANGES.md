# Schema changes

- **Status:** Living log, started October 19, 2026. Nothing here is decided by being listed.
- **Scope:** Renames and schema changes that someone has proposed or decided but that have not
  landed. This covers database tables and columns, API routes, JSON and report fields, metric
  ids and aliases, proto fields, config keys, CLI flags and doc terms.
- **Related:** [Decisions register](DECISIONS.md), [backlog](BACKLOG.md),
  [platform vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md),
  [metrics registry](platform/architecture/metrics-registry.md),
  [schema review](../data/structures/SCHEMA-REVIEW.md)

Use this file to find every pending rename in one place, with the reasoning and the documents
that own it. It does not replace those documents. A rename is decided in
[DECISIONS.md](DECISIONS.md) or in the plan that owns it, and scheduled in
[BACKLOG.md](BACKLOG.md). When one lands, move its entry to [Done](#done) with the PR, or delete
it once its compatibility aliases are retired. Add an entry when a document proposes a rename,
records a misnomer it keeps, or leaves a compatibility alias behind.

Each entry gives what would change, the kind of name, its status, and where the reasoning lives.
Status is one of **considered** (raised, not proposed), **proposed** (a plan proposes it),
**decided** (a decision or plan commits to it), or **left over** (most of it landed; a remnant
remains).

## Candidates

### Headway names on a net gap

- **What:** the behaviour plan's feature names `distance_headway`, `time_headway`, `thw_min`,
  `thw_median`, `thw_below_{2.0,1.5,1.0}_seconds` and `thw_below_X_rate`. Also the registry
  aliases `time_headway`, `thw`, `distance_headway` of the `interaction.following_*` metric ids.
- **Kind:** metric aliases and doc terms. The canonical ids already say what they measure:
  `interaction.following_net_time_gap_s` and its `_min_s` and `_p50_s` forms.
- **Status:** considered. Owner, October 19, 2026: no rename yet; log it here for later.
- **Why:** in the literature, headway is a gross measure that includes a vehicle length (Evans
  1991, via [Michael et al. 2000](<https://doi.org/10.1016/S1369-8478(00)00015-2>); Jiao et al.
  2024; Chu et al. 2026). The plan computes a net gap and says so in prose. But a reader who sees
  "headway" will compare it with gross figures that run about L/v longer: 0.4 s at 25 mph, 1 s
  at 10 mph.
- **History:** the October 9 fixed-sensor briefing declined a rename to
  `clearance_time_gap_seconds` and kept the aliases "for migration only".
- **Where:**
  - [Behaviour plan Section 8.3](plans/lidar-behaviour-analytics-plan.md#83-following-behaviour)
  - [Metrics registry](platform/architecture/metrics-registry.md) (aliases column; "passage
    headway" reserved for a detector quantity)
  - `internal/lidar/l8behaviour/metrics.go` (metric ids and `Aliases`)
  - `internal/lidar/l8behaviour/surface.go` (`AuditSurfaceJSON` rejects aliases in published
    output)
  - [October 19 briefing, Q3](lidar/operations/brief/research-briefing-2026-10-19.md#open-questions)
  - [October 9 fixed-sensor briefing](lidar/operations/brief/fixed-sensor-research-briefing-2026-10-09.md)

### Following report and route names

- **What:** the report contract `headway_report_v2` and the route `/api/scenes/<id>/headway`
  take names carrying `following_net_time_gap_s`. The old names stay as aliases first and are
  retired later. The vocabulary plan also moves `/api/scenes[/{id}[/headway]]` to `/api/surveys`.
  Probably affected too, though the decision does not name them:
  - `/api/charts/histogram?kind=headway`
  - `velocity report headway`
  - `internal/report/headway`
  - `internal/api/server_scenes_headway.go` and `server_charts_headway.go`
- **Kind:** report contract and API routes.
- **Status:** decided (D-28, October 10, 2026), not done. The rename goes through vocabulary P2-F
  and the old names retire under P2-G. The October 9 briefing's "the surfaces keep their names"
  is superseded.
- **Where:** [DECISIONS.md D-28](DECISIONS.md),
  [vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md) (P2-F and the route ledger),
  [backlog](BACKLOG.md), [metrics registry](platform/architecture/metrics-registry.md).

### Platform vocabulary: tables, routes, proto, CLI and config

- **What:** the largest set of pending renames, listed in full in the
  [vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md)'s ledger. Examples:
  - **Tables:** `radar_data` → `radar_readings`, `radar_objects` → `radar_detections`,
    `radar_data_transits` → `radar_tracks`, `site` and `lidar_sites` → `sites`,
    `site_config_periods` → `deployments`, `lidar_scenes` → `surveys`, `lidar_replay_cases` →
    `lidar_clips`, `lidar_run_records` → `lidar_runs`, `lidar_replay_annotations` →
    `lidar_track_labels`. The capture tables become `capture_volumes`, `captures`,
    `capture_sequences`, `capture_periods` and `jobs`.
  - **Routes:** `/api/events` → `/api/radar/readings`, `/api/capabilities` → `/api/sensors`,
    `/api/lidar/scenes` → `/api/lidar/clips`.
  - **Proto:** `SourceMode` PCAP → CAPTURE (deprecated aliases until v0.7.5), `Track.heading`
    → `yaw`, `CoordinateFrameInfo` → `FrameInfo`, `LabelSet.session_id` → `clip_id`.
  - **CLI and environment:** `pcap-replay` → `replay`, `--lidar-pcap-dir` → `--capture-dir`,
    `LIDAR_EVIDENCE_DIR` → `LIDAR_VRLOG_DIR`.
  - **Config:** `l1.data_source` value `pcap` → `capture`, `config/profiles/` →
    `config/presets/`.
  - **Typed-ID prefix:** `scne_` → `clip_`.
- **Kind:** all of the above.
- **Status:** proposed; the plan "changes no code and no schema". Phases 1 and 2 have sub-plans
  and backlog items.
- **Why:** "scene" means five things, "capture" names a file, a session and a volume, and radar
  rows carry no sensor identity.
- **Decision record:** none for the programme. The vocabulary plan and the backlog say it will
  be "recorded as D-28", but D-28 is the October 2026 research-briefing decisions record.
  Capture identity alone is D-27. The vocabulary decision needs its own number.
- **Where:** [vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md) and its eleven
  `platform-vocabulary-phase{1,2}-*-plan.md` sub-plans in [plans/](plans),
  [backlog](BACKLOG.md), [D-27](DECISIONS.md).

### Compatibility aliases awaiting retirement

- **What:** remove the old routes, CLI spellings and flag names; the `pcap` wire value; the
  `transits` dataset value; the proto `session_id` field; the `pcap_file`, `session_id` and
  `source_period_id` columns; and `/api/lidar/pcap/files`.
- **Kind:** routes, CLI, proto field, database columns.
- **Status:** decided in the vocabulary plan (Item 18, P2-G), not done. It waits on every other
  vocabulary item.
- **Where:**
  - [vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md) (Item 18)
  - [retirement sub-plan](plans/platform-vocabulary-phase2-retirement-plan.md)
  - `internal/lidar/server/pipeline_state.go` (`DataSourceWire`, the legacy token)
  - `web/src/routes/lidar/replay-cases/+page.svelte` (the legacy `pcap_file` projection)

### Site identity, keys and timestamp units

- **What:**
  - `site_id` means two things: an integer pointing at `site(id)`, and text pointing at
    `lidar_sites`.
  - `radar_objects` has no primary key.
  - The estimate tables have no foreign keys.
  - Timestamp units are mixed; new columns should carry the unit in their name.
- **Kind:** database columns and keys.
- **Status:** proposed. The vocabulary plan's Items 14 and 17 settle two of these; the
  estimate-links and timestamp-units audits are open.
- **Where:** [schema review, wider watchlist](../data/structures/SCHEMA-REVIEW.md),
  [backlog](BACKLOG.md).

### Typed UUID prefixes

- **What:**
  - `trk_` → `trak_`.
  - Bare UUIDs get `runa_`, `eval_`, `regn_`, `labl_` and `swep_` prefixes.
  - `replay-{scene}-{uuid8}` → `runy_`, `reprocess-…` → `runs_`.
  - `scne_` → `clip_`, with the vocabulary plan.
- **Kind:** stored id format.
- **Status:** proposed, targeting 0.7.0; no decision record.
- **Where:** [typed UUID prefixes plan](plans/platform-typed-uuid-prefixes-plan.md),
  [typed UUID prefixes](platform/architecture/typed-uuid-prefixes.md).

### Tuning config schema v3

- **What:** `buffer_timeout` and `min_frame_points` move to `l2`; `flush_interval` and
  `background_flush` to `l3`; `min_observations_for_classification` to `l6`. L2 and L6 engine
  selectors are added, with migration from v2 through `config-migrate`.
- **Kind:** config keys.
- **Status:** proposed (Phase 2B), Phase 3 pending. Legacy dot-path keys are still accepted by
  `POST /api/lidar/params`.
- **Where:** [config restructure plan](plans/config-restructure-plan.md),
  [CONFIG.md](../config/CONFIG.md), `cmd/tools/config-migrate`, [backlog](BACKLOG.md).

### Metric alias audit in CI

- **What:** fail CI when a deprecated metric alias appears outside its migration window (a
  `metrics-lint` task). This is how the headway aliases above would eventually be enforced.
- **Kind:** metric ids (tooling).
- **Status:** proposed.
- **Where:** [metrics registry plan](plans/metrics-registry-and-observability-plan.md),
  [metrics registry](platform/architecture/metrics-registry.md), [backlog](BACKLOG.md).

### `peak_speed_mps` leftovers (D-19)

- **What:** the rename to `max_speed_mps` landed (migration 000030; proto, Go, TypeScript and
  Swift). Left over:
  - `peak_speed_mps` is reserved for a filtered metric that is not yet defined.
  - The metrics registry still uses it as its example alias.
  - The Swift visualiser keeps a compatibility shim for `PeakSpeedMps` JSON.
  - Phase 3 checkboxes in two plans are stale.
- **Kind:** metric name, JSON field, doc terms.
- **Status:** left over.
- **Where:** [D-19](DECISIONS.md),
  [speed percentile alignment plan](plans/speed-percentile-aggregation-alignment-plan.md),
  [unpopulated data structures plan](plans/unpopulated-data-structures-remediation-plan.md),
  [metrics registry](platform/architecture/metrics-registry.md).

### Smaller renames and kept misnomers

| What                                                                                                                                             | Kind              | Status                                                     | Where                                                                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------- | ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/debug/lidar/background/regions` → `/api/lidar/background/regions`                                                                              | Route             | Landed in #836 with no alias; moves to Done with its PR    | [web frontend consolidation plan](plans/web-frontend-consolidation-plan.md)                                                                             |
| Swift app calls the deprecated `/api/lidar/pcap/stop`; must move to `/api/lidar/replay/stop` before the alias goes                               | Route             | Proposed                                                   | Same plan; [web frontend consolidation](ui/web-frontend-consolidation.md)                                                                               |
| `AssociationCandidate.distance` holds a squared Mahalanobis distance                                                                             | Proto field       | Kept misnomer, not scheduled                               | `internal/lidar/l9endpoints/debug_proto.go`                                                                                                             |
| VRLOG recorder `tuning_hash`, "deprecated transition field"                                                                                      | JSON field        | No removal plan                                            | `internal/lidar/l9endpoints/recorder/recorder.go`                                                                                                       |
| Legacy JSON VRLOG frame encoding                                                                                                                 | File format       | Proposed retirement                                        | [backlog](BACKLOG.md)                                                                                                                                   |
| LiDAR label API keeps the legacy route and `label_id` naming                                                                                     | Route, JSON field | Covered by vocabulary Item 7                               | `internal/api/lidar_labels.go`                                                                                                                          |
| Web route `/lidar/scene-map` keeps its old name; `/lidar/replay-cases` serves Clips                                                              | Web route         | Covered by the vocabulary ledger                           | `web/src/routes/lidar/`                                                                                                                                 |
| `settling-eval --tuning`, refusing tombstone for `--config`                                                                                      | CLI flag          | v0.5.9                                                     | `internal/cmd/lidar/settling.go`, [PCAP analysis mode](lidar/operations/pcap-analysis-mode.md)                                                          |
| `velocity serve --lidar-listen` (the retired :8081 LiDAR listener), refusing tombstone for `--listen`                                            | CLI flag          | v0.5.9                                                     | `internal/cmd/server/radar.go`, [compatibility register](platform/operations/compatibility-register.md)                                                 |
| `velocity device upgrade --check`, kept for one release                                                                                          | CLI flag          | Removal due                                                | `internal/cmd/device/check.go`                                                                                                                          |
| Legacy v0.5.0-era release asset name variants                                                                                                    | File names        | TODO after v0.5.1                                          | `scripts/update-release-json.py`                                                                                                                        |
| Go compatibility type aliases to `l8analytics` and the layer packages                                                                            | Go types          | Left over; D-20 allows only necessary aliases after v0.5.0 | `internal/lidar/aliases.go`, [LiDAR architecture](lidar/architecture/LIDAR_ARCHITECTURE.md), [D-20](DECISIONS.md)                                       |
| Replay-case doc still lists "pending rename" of `scene_store.go`, `scene_api.go`, `/api/lidar/scenes` to replay-case names; target is now "clip" | Doc               | Stale doc; superseded by the vocabulary plan               | [replay case management](lidar/operations/clip-management-implementation.md), [terminology plan](plans/lidar-replay-case-terminology-alignment-plan.md) |
| `DisplayLabel`/`SelectableLabel` split, taxonomy API, public API field alignment                                                                 | Go/TS types, API  | Planned (Phases 3.5–6)                                     | [label vocabulary plan](plans/label-vocabulary-consolidation-plan.md)                                                                                   |
| Docs-site route `/tool/protractor/` → `/tools/protractor/` with an alias                                                                         | Docs route        | Planned (Milestone 2)                                      | [embedded offline docs site plan](plans/embedded-offline-docs-site.md)                                                                                  |
| `lidar-ml-classifier-training-plan.md` renamed to match its scope                                                                                | Plan file         | Deferred to keep the backlog link                          | [the plan](plans/lidar-ml-classifier-training-plan.md)                                                                                                  |

## Done

- D-31 clip cutover: migration 064 renames `lidar_replay_cases` → `lidar_clips`,
  `lidar_replay_case_files` → `lidar_clip_captures`, `lidar_replay_evaluations` →
  `lidar_clip_evaluations`, and `lidar_run_records` → `lidar_runs`; referring columns
  become `clip_id`. Capture associations replace the dropped clip `pcap_file` projection.
  The route is `/api/lidar/clips`, with no old API alias. The macOS identifier alias
  is recorded in the [register](platform/operations/compatibility-register.md).

Nothing has moved here yet.
