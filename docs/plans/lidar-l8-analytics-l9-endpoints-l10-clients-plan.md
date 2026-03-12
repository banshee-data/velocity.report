# LiDAR L8 Analytics / L9 Endpoints / L10 Clients Refactor Plan

**Status:** Revised implementation plan - reviewed against repository state and backlog on 2026-03-12
**Layers:** L8 Analytics, L9 Endpoints, L10 Clients
**Related:** [L7 Scene plan](lidar-l7-scene-plan.md), [speed percentile aggregation alignment](speed-percentile-aggregation-alignment-plan.md), [schema simplification migration 030](schema-simplification-migration-030-plan.md), [tracks table consolidation](lidar-tracks-table-consolidation-plan.md)

## Executive Summary

velocity.report still documents and mostly implements a six-layer LiDAR stack ending at `L6 Objects`, but the code already contains partial `L8` analytics and partial `L9` endpoint shaping under the wrong owners. The earlier draft plan described the right destination but split the work into too many phases and pulled the `visualiser/` rename too early.

This revision collapses the work into three delivery phases with explicit subphases:

1. lock the ten-layer architecture contract and create the `l8analytics/` seed
2. migrate analytics out of `l6objects/`, `storage/sqlite`, and `monitor/` handlers
3. formalise `L9 Endpoints`, rename `visualiser/`, and replace `monitor/` with explicit `server/` and client-facing packages

The dependency order is deliberate: `L8` must exist before storage and handlers can delegate to it, and that migration must be largely complete before the `visualiser/` to `l9endpoints/` rename and `monitor/` package split.

## Review Conclusions

- The previous six-phase plan plus `Phase 4.5/5.5/6.5/7` tail was harder to schedule than to execute. The work naturally groups into three deliveries.
- The previous draft understated the `internal/lidar/visualiser/` rename blast radius. It affects `cmd/radar`, `cmd/tools/visualiser-server`, `cmd/tools/gen-vrlog`, `internal/lidar/analysis`, generated `pb` code, recorder imports, and multiple docs.
- `L9` already exists in practice. The goal is to formalise and rename it, not invent it.
- `monitor/` should not be decomposed before `L8` exists. Otherwise the same analytics logic is simply re-homed under a different package name.
- Adjacent backlog items around speed metrics, migration 030, and visualiser proto follow-through must be coordinated, but they are not reasons to keep this refactor fragmented.

## Backlog Alignment

This plan now directly absorbs the two backlog items currently attached to it:

| Backlog item                          | Release bucket | Covered by                       |
| ------------------------------------- | -------------- | -------------------------------- |
| `L8/L9/L10 layer refactor Phases 1-3` | `v0.5.1`       | Phase 1 and Phase 2 of this plan |
| `L8/L9/L10 layer refactor Phases 4-5` | `v0.6`         | Phase 3 of this plan             |

Adjacent backlog items that influence sequencing but remain separate deliverables:

| Item                                                                                                                    | Why it matters here                                                    | Rule for this plan                                                                                 |
| ----------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `Track speed metric redesign + aggregate-only percentiles`                                                              | defines canonical percentile semantics                                 | use its decisions when shaping `l8analytics/percentiles.go`; do not re-litigate metric naming here |
| `Schema simplification (migration 000030)`                                                                              | removes dead per-track percentile columns and renames `peak_speed_mps` | treat as follow-on storage cleanup once the `L8` API is stable                                     |
| `LiDAR tracks table consolidation`                                                                                      | depends on shared track analytics and storage helpers                  | sequence after Phase 2, not before                                                                 |
| `Visualiser track proto parity`, `debug overlay + cluster proto follow-through`, `performance and scene health metrics` | all depend on a stable `L9` contract and package path                  | rebase those implementations onto `l9endpoints/` during or after Phase 3                           |

## Repository Baseline

### Canonical LiDAR package tree today

Current layer packages under `internal/lidar/`:

- `l1packets/`
- `l2frames/`
- `l3grid/`
- `l4perception/`
- `l5tracks/`
- `l6objects/`

Cross-cutting packages already present:

- `pipeline/`
- `storage/sqlite/`
- `adapters/`
- `sweep/`
- `monitor/`
- `visualiser/`

There is no `internal/lidar/l8analytics/` package today, and there is no explicit `internal/lidar/l9endpoints/` package.

### Current ownership mismatches

| Current location                                                                                                | Current responsibility                                              | Correct owner                           |
| --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | --------------------------------------- |
| `internal/lidar/l6objects/comparison.go`                                                                        | run comparison types and temporal IoU helpers                       | `L8 Analytics`                          |
| `internal/lidar/l6objects/quality.go`                                                                           | mixed per-object quality helpers and run-level aggregate statistics | split `L6` and `L8`                     |
| `internal/lidar/storage/sqlite/track_store.go`                                                                  | inline speed percentile calculation during persistence              | `L8 Analytics` helper called by storage |
| `internal/lidar/storage/sqlite/analysis_run.go`                                                                 | run comparison orchestration and run summary logic                  | storage plus `L8` split                 |
| `internal/lidar/storage/sqlite/analysis_run_compare.go`                                                         | parameter diffing and track matching logic                          | `L8 Analytics`                          |
| `internal/lidar/monitor/track_api.go`                                                                           | summary aggregation and response shaping in one file                | `L8` plus transport split               |
| `internal/lidar/monitor/run_track_api.go`                                                                       | labelling progress and evaluation aggregation inside handlers       | `L8` plus transport split               |
| `internal/lidar/monitor/scene_api.go`                                                                           | scene CRUD mixed with evaluation orchestration                      | infra plus `L8` split                   |
| `internal/lidar/monitor/chart_data.go`, `chart_api.go`, `echarts_handlers.go`, `templates.go`, `gridplotter.go` | chart, dashboard, and debug payload shaping                         | `L9 Endpoints`                          |

### L9 and L10 already exist in practice

`L9 Endpoints` already exists in code, just under inconsistent names:

- `internal/lidar/visualiser/` contains the gRPC stream adapter, frame codec, server-side visualiser model, replay and recorder code, and the canonical Go-side streaming surface.
- `proto/velocity_visualiser/v1/visualiser.proto` is already the formal wire contract seam.
- `internal/lidar/monitor/chart_api.go`, `chart_data.go`, `echarts_handlers.go`, `templates.go`, and `gridplotter.go` already behave like endpoint-layer code.

`L10 Clients` is documentation-only and already exists structurally:

- `web/src/routes/lidar/`
- `tools/visualiser-macos/`
- `tools/pdf-generator/`

### Existing docs still lag the code

The six-layer model is still described in multiple places, including:

- `docs/lidar/architecture/lidar-data-layer-model.md`
- `docs/lidar/architecture/README.md`
- `docs/lidar/README.md`
- `docs/data/DATA_STRUCTURES.md`
- `docs/lidar/terminology.md`
- `internal/lidar/l1packets/doc.go` through `internal/lidar/l6objects/doc.go`

## Target Ten-Layer Model

| Layer | Label      | Responsibility                                                                                                            |
| ----- | ---------- | ------------------------------------------------------------------------------------------------------------------------- |
| L1    | Packets    | wire transport, UDP capture, PCAP replay, packet parsing                                                                  |
| L2    | Frames     | frame assembly, timestamps, geometry conversion, exports                                                                  |
| L3    | Grid       | background model, foreground masking, persistence, drift, regions                                                         |
| L4    | Perception | per-frame scene interpretation, clustering, OBBs, ground removal                                                          |
| L5    | Tracks     | temporal association, identity, lifecycle, motion estimation                                                              |
| L6    | Objects    | semantic actor interpretation and per-object quality/classification                                                       |
| L7    | Scene      | persistent evidence-accumulated world model and multi-sensor fusion; see [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md) |
| L8    | Analytics  | canonical metrics, summaries, comparisons, scoring, evaluation logic                                                      |
| L9    | Endpoints  | server-side payload shaping, gRPC stream contract, dashboards, debug views, review payloads                               |
| L10   | Clients    | documentation label only; browser, native, and report-generation consumers of `L9` contracts                              |

## Design Rules

- `L(n)` may depend only on `L(n-1)` and below.
- `L8 Analytics` may depend on `L1-L7`, but never on HTML, Svelte, Swift, chart libraries, or transport-layer response types.
- `L9 Endpoints` may depend on `L8` outputs and selected lower-layer artifacts needed for debug rendering, but it does not define canonical metrics or comparisons.
- `L10 Clients` render and interact; they do not recompute canonical analytics locally.
- `storage/sqlite` is persistence, not a permanent analytics owner.
- `monitor/` is transitional application code in the current tree, not a canonical domain layer.
- If a value is needed by web, Swift, or PDF consumers, the default answer is: compute it in `L8`, expose it in `L9`, render it in `L10`.

## Three-Phase Delivery Plan

### Phase 1: Architecture Contract and L8 Seed

Backlog coverage: first half of the `v0.5.1` backlog item.

#### 1A. Lock the docs and naming

- Update the canonical LiDAR docs from six layers to ten layers.
- Document `L9 Endpoints` as the explicit successor name for the current `visualiser/` package.
- Document `L10 Clients` as a label only; do not create a Go package for it.
- Update package docs under `internal/lidar/l1packets/` through `internal/lidar/l6objects/`.
- Add a short migration note explaining that the codebase will move from `visualiser/` plus `monitor/` into `l9endpoints/` plus `server/`.

Docs that must be updated in this subphase:

- `docs/lidar/architecture/lidar-data-layer-model.md`
- `docs/lidar/architecture/README.md`
- `docs/lidar/README.md`
- `docs/data/DATA_STRUCTURES.md`
- `docs/lidar/terminology.md`
- any repo-level architecture docs that still describe the six-layer model

#### 1B. Create `internal/lidar/l8analytics/`

Create the canonical analytics package with a small, stable initial split:

- `doc.go`
- `types.go`
- `comparison.go`
- `summary.go`
- `percentiles.go`
- `labels.go`

Initial moves into `L8`:

- `RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge`
- `ComputeTemporalIoU`
- `RunStatistics` and `ComputeRunStatistics`
- small shared result types used by summary endpoints and run comparison
- percentile helpers currently embedded in persistence or handler code

Current call sites that will need updating during or immediately after this move include:

- `internal/lidar/adapters/ground_truth.go`
- `internal/lidar/analysis/compare.go`
- `internal/lidar/storage/sqlite/analysis_run.go`
- `internal/lidar/storage/sqlite/analysis_run_compare.go`
- `internal/lidar/monitor/track_api.go`

#### 1C. Narrow `L6` back to object semantics

After Phase 1, `l6objects/` should own:

- per-object classification
- per-object quality predicates
- object-level attributes derived from tracked actors

After Phase 1, `l6objects/` should not own:

- run-level aggregates
- cross-run comparison orchestration
- transport-neutral percentile helpers

#### Phase 1 exit criteria

- ten-layer architecture docs are the documented source of truth
- `internal/lidar/l8analytics/` exists and is imported by real code
- `l6objects/` no longer owns run-level aggregate types
- no new analytics code is added to `storage/sqlite` or `monitor/` during the transition

### Phase 2: Analytics Migration and API Thinning

Backlog coverage: second half of the `v0.5.1` backlog item.

#### 2A. Remove analytics ownership from `storage/sqlite`

Move or extract the following into `l8analytics/`:

- percentile math from `internal/lidar/storage/sqlite/track_store.go`
- run summary helpers from `internal/lidar/storage/sqlite/analysis_run.go`
- parameter diffing from `internal/lidar/storage/sqlite/analysis_run_compare.go`
- temporal matching and Hungarian-assignment comparison logic from `internal/lidar/storage/sqlite/analysis_run_compare.go`
- run comparison orchestration from `internal/lidar/storage/sqlite/analysis_run.go`

Target end state for storage:

- storage loads and stores rows
- storage may call `l8analytics` helpers for derived values it must persist
- storage does not own comparison algorithms or canonical summary math

#### 2B. Thin `monitor/` handlers into transport shells

Move aggregate logic out of the handler files and into `l8analytics/` or small application services that wrap `l8analytics/`:

- `internal/lidar/monitor/track_api.go`
  - extract `TrackSummaryResponse` population logic and shared class and overall summaries
- `internal/lidar/monitor/run_track_api.go`
  - extract labelling-progress aggregation and run evaluation summaries
- `internal/lidar/monitor/scene_api.go`
  - split scene CRUD from scene evaluation orchestration
- keep `sweep_handlers.go`, `export_handlers.go`, and `pcap_files_api.go` transport-focused unless they reveal hidden analytics ownership during the extraction

Target end state for handlers:

- request parsing
- auth and validation
- response serialization
- delegation to `l8analytics/` or thin application services

They must not own:

- percentile calculations
- canonical summary statistics
- comparison logic
- run-evaluation aggregation

#### 2C. Coordinate dependent backlog work without absorbing it

Phase 2 must respect these rules:

- adopt the canonical aggregate speed metric decisions from the speed-metric redesign work
- do not block this phase on migration `000030`; it is cleanup, not a prerequisite for extracting ownership
- do not block this phase on tracks-table consolidation; that work benefits from a stable `L8` API
- keep HTTP and gRPC response shapes compatible where practical so the later `L9` work is not forced into simultaneous contract churn

#### Phase 2 exit criteria

- `L8` is the only canonical home for LiDAR run, summary, percentile, and comparison analytics
- `storage/sqlite` is persistence-first and no longer owns matching algorithms
- `monitor/` handlers are thin transport shells over extracted services
- the backlog item currently named `L8/L9/L10 layer refactor Phases 1-3` is complete in substance

### Phase 3: L9 Endpoints Formalisation and `monitor/` Replacement

Backlog coverage: the `v0.6` backlog item currently named `L8/L9/L10 layer refactor Phases 4-5`.

#### 3A. Rename `visualiser/` to `l9endpoints/`

Rename:

- `internal/lidar/visualiser/` to `internal/lidar/l9endpoints/`

Update all code, generated bindings, and docs that reference the old path. The current blast radius includes at least:

- `cmd/radar/radar.go`
- `cmd/tools/visualiser-server/main.go`
- `cmd/tools/gen-vrlog/main.go`
- `internal/lidar/analysis/report.go`
- `internal/lidar/analysis/compare.go`
- `internal/lidar/analysis/compat_test.go`
- `internal/lidar/analysis/report_test.go`
- `internal/lidar/analysis/compare_test.go`
- `internal/lidar/visualiser/recorder/*`
- generated `pb` imports and `proto/velocity_visualiser/v1/visualiser.proto` `go_package`
- docs that currently point to `internal/lidar/visualiser/*`

Rename rules:

- do the rename as one coherent step; do not leave mixed `visualiser` and `l9endpoints` imports in the tree
- keep the proto package name stable unless there is a separate versioning decision
- treat generated bindings as part of the rename, not a later cleanup pass

#### 3B. Move chart, dashboard, and debug payload shaping into `l9endpoints/`

Move the endpoint-layer code out of `monitor/`:

- `chart_api.go`
- `chart_data.go`
- `echarts_handlers.go`
- `templates.go`
- `gridplotter.go`
- `html/`
- `assets/`

Recommended shape:

- `l9endpoints/` defines the small interfaces it needs from the server layer
- `server/` satisfies those interfaces during route registration
- `l9endpoints/` owns chart and view-model structs, coordinate transforms, debug payload formatting, HTML templates, and embedded assets
- canonical metrics continue to come from `l8analytics/`

#### 3C. Replace `monitor/` with explicit packages

Split the remaining mixed package into named roles:

| Target package                                      | Takes ownership of                                                                                                                                                                                                                      |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/lidar/server/`                            | `webserver.go`, `datasource.go`, `datasource_handlers.go`, `playback_handlers.go`, `stats.go`, `mock_background.go`, `track_api.go`, `run_track_api.go`, `scene_api.go`, `sweep_handlers.go`, `export_handlers.go`, `pcap_files_api.go` |
| `internal/lidar/l9endpoints/`                       | existing streaming and visualiser code plus chart, dashboard, and debug endpoint code moved in 3B                                                                                                                                       |
| `internal/lidar/client/` or `internal/lidar/sweep/` | `client.go` and `direct_backend.go`, depending on which interface boundary produces the cleaner dependency graph                                                                                                                        |

Specific rules for this subphase:

- `server/` may import `l8analytics/` and `l9endpoints/`
- `l9endpoints/` must not import `server/`
- if a compatibility shim is needed for one release window, keep it tiny and documented; do not let `monitor/` keep growing during the transition
- delete `monitor/` once imports are gone and tests have moved

#### 3D. Hardening, migration support, and follow-through

Complete the structural refactor with the artifacts that keep it maintainable:

- migration note listing package moves and expected caller updates
- generated architecture graph and regeneration script
- tests moved alongside extracted `L8`, `L9`, and `server/` code
- docs updated to reference `l9endpoints/` and `server/`
- adjacent visualiser backlog work rebased onto the new `L9` package path

#### Phase 3 exit criteria

- `internal/lidar/l9endpoints/` is the canonical Go home for `L9 Endpoints`
- `monitor/` no longer contains chart, view-model, or analytics logic
- the remaining HTTP application layer has an explicit home under `server/`
- `L10` clients consume stable `L9` contracts rather than hidden `monitor/` internals
- the backlog item currently named `L8/L9/L10 layer refactor Phases 4-5` is complete in substance

## Target Package End State

```text
internal/lidar/
├── l1packets/
├── l2frames/
├── l3grid/
├── l4perception/
├── l5tracks/
├── l6objects/
├── l8analytics/              # canonical analytics
├── l9endpoints/              # streaming, dashboards, chart/debug payload shaping
├── server/                   # HTTP application server, route registration, transport
├── sweep/ or client/         # sweep-facing backend/client adapter
├── pipeline/
├── storage/sqlite/
├── adapters/
└── ...
```

`L10 Clients` remains a documentation label spanning `web/`, `tools/visualiser-macos/`, and `tools/pdf-generator/`. No Go `l10clients/` package should be created.

## Risks and Guardrails

- The `visualiser/` rename is broader than it first looks. Treat generated code, recorder imports, and docs as first-class rename targets.
- Do not collapse storage refactors and the package-rename work into one PR unless tests are already strong enough to localise failures.
- `l6objects/quality.go` is mixed ownership today. Keep per-object predicates in `L6`; move aggregate run metrics to `L8`.
- Do not let `server/` become a new catch-all. It is transport and application wiring, not a new domain layer.
- Preserve response compatibility where practical during Phase 2 so Phase 3 can focus on package ownership rather than avoidable contract churn.
- If temporary type aliases are used during migration, treat them as short-lived compatibility scaffolding with an explicit removal step.

## Non-Goals

- full LiDAR subsystem rewrite
- broad mechanical renaming with no ownership improvement
- redesign of the web app or macOS visualiser unrelated to the layer split
- making the radar PDF generator a blocker for the LiDAR refactor

## Checklist

### Phase 1

- [ ] ten-layer LiDAR architecture docs updated
- [ ] `L9 Endpoints` and `L10 Clients` naming documented
- [ ] package doc comments under `internal/lidar/l1packets/` through `internal/lidar/l6objects/` updated
- [ ] `internal/lidar/l8analytics/` created
- [ ] comparison types and temporal IoU moved out of `l6objects/`
- [ ] run-level statistics moved out of `l6objects/quality.go`

### Phase 2

- [ ] percentile helpers extracted from storage-owned code
- [ ] run comparison orchestration moved out of `storage/sqlite`
- [ ] `analysis_run_compare.go` no longer owns canonical matching algorithms
- [ ] track summary logic delegates to `l8analytics/`
- [ ] labelling progress and evaluation aggregation delegates to `l8analytics/`
- [ ] handler files are transport-only in responsibility
- [ ] direct unit tests exist for moved `L8` logic

### Phase 3

- [ ] `internal/lidar/visualiser/` renamed to `internal/lidar/l9endpoints/`
- [ ] proto `go_package`, generated bindings, and imports updated coherently
- [ ] chart, dashboard, and debug code moved out of `monitor/` and into `l9endpoints/`
- [ ] `server/` package created and populated
- [ ] `client.go` and `direct_backend.go` moved behind an explicit package boundary
- [ ] no production analytics logic remains in `monitor/`
- [ ] migration note and generated architecture artifacts added or refreshed
- [ ] docs and tests updated to the final package layout
