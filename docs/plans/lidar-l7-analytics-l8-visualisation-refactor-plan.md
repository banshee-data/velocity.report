# LiDAR L7 Analytics / L8 Visualisation Refactor Plan

**Status:** Proposed implementation plan
**Source:** Imported from the original planning document `plan-l7-l8.md`, then reviewed against the repository state on 2026-03-06.
**Scope:** LiDAR architecture docs, `internal/lidar`, `proto/velocity_visualiser/v1`, `web/`, and the macOS visualiser integration boundary.

## Executive Summary

velocity.report currently documents and implements a six-layer LiDAR model that ends at `L6 Objects`. The desired target is an eight-layer model that adds:

- `L7 Analytics` for canonical traffic, safety, and run-analysis logic
- `L8 Visualisation` for operator-facing rendering, payload shaping, dashboards, and visual review workflows

This is a breaking architectural change, not just a terminology update. The main goal is to stop overloading `L6 Objects`, `storage/sqlite`, and `monitor/` with responsibilities that belong to analytics and presentation.

The attached handover is directionally correct, but the repository review shows that the work needs to be more explicit about:

- the existing six-layer docs that must be updated
- analytics logic that already exists but is currently misplaced
- the fact that L8 already partially exists in `internal/lidar/visualiser/`, `proto/velocity_visualiser/v1/`, `web/`, and `tools/visualiser-macos/`
- the mixed nature of `internal/lidar/monitor/`, which currently contains infrastructure, analytics-backed APIs, and presentation shaping

## Repository Baseline Reviewed Against Current Code

### Canonical LiDAR package tree today

The current canonical LiDAR packages under `internal/lidar/` are:

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

There is no `internal/lidar/l7analytics/` package today, and there is no explicit `L8` package boundary on the Go side.

### Existing docs still describe a six-layer model

The six-layer model is still the canonical language in multiple places, including:

- `docs/lidar/architecture/lidar-data-layer-model.md`
- `docs/lidar/architecture/README.md`
- `docs/lidar/README.md`
- `docs/data/DATA_STRUCTURES.md`
- `docs/lidar/terminology.md`
- `internal/lidar/l1packets/doc.go`
- `internal/lidar/l2frames/doc.go`
- `internal/lidar/l3grid/doc.go`
- `internal/lidar/l4perception/doc.go`
- `internal/lidar/l5tracks/doc.go`
- `internal/lidar/l6objects/doc.go`

### Concrete ownership mismatches already in the repo

The repo already contains analytics and presentation logic that do not fit the current documented ownership:

| Current location | Current responsibility | Target ownership |
| --- | --- | --- |
| `internal/lidar/l6objects/comparison.go` | run comparison types and temporal IoU helpers | `L7 Analytics` |
| `internal/lidar/l6objects/quality.go` | mixed per-track quality helpers and run-level aggregate statistics | split between `L6 Objects` and `L7 Analytics` |
| `internal/lidar/storage/sqlite/track_store.go` | speed percentile calculation during persistence | `L7 Analytics` helper called by storage |
| `internal/lidar/storage/sqlite/analysis_run.go` | run comparison orchestration, percentiles, run-track summary logic | storage plus `L7 Analytics` service split |
| `internal/lidar/storage/sqlite/analysis_run_compare.go` | parameter diffing for run comparison | likely `L7 Analytics` |
| `internal/lidar/monitor/track_api.go` | track summary aggregation and response shaping | `L7 Analytics` plus `L8`/handler boundary |
| `internal/lidar/monitor/chart_data.go` | chart-specific view-model shaping | `L8 Visualisation` |
| `internal/lidar/monitor/chart_api.go` | presentation-facing chart APIs | `L8 Visualisation` |
| `internal/lidar/monitor/scene_api.go` | scene CRUD plus evaluation/replay orchestration | mixed infra plus `L7` application services |
| `internal/lidar/monitor/run_track_api.go` | run, labelling, evaluation, and comparison flows | mixed infra plus `L7` application services |

### L8 already exists in practice

The current repository already has clear L8 consumers and adapters:

- `internal/lidar/visualiser/` converts pipeline outputs into the canonical visualiser frame model and gRPC stream
- `proto/velocity_visualiser/v1/visualiser.proto` defines the visualiser payload contract
- `web/src/routes/lidar/*` implements browser-side LiDAR review and control workflows
- `tools/visualiser-macos/` is the native operator-facing visualiser

This means Phase 1 is not “invent L8 from nothing”. It is “formalize L8 ownership, move misplaced server-side presentation shaping into it, and document it consistently”.

## Target Eight-Layer Model

| Layer | Label | Responsibility |
| --- | --- | --- |
| L1 | Packets | wire transport, UDP capture, PCAP replay, packet parsing |
| L2 | Frames | frame assembly, timestamps, geometry conversion, exports |
| L3 | Grid | background model, foreground masking, persistence, drift, regions |
| L4 | Perception | per-frame scene interpretation, clustering, OBBs, ground removal |
| L5 | Tracks | temporal association, identity, lifecycle, motion estimation |
| L6 | Objects | semantic actor interpretation and object-level quality/classification |
| L7 | Analytics | canonical metrics, summaries, comparisons, scoring, evaluation logic |
| L8 | Visualisation | rendering, dashboards, review workflows, payload shaping, UI contracts |

## Design Rules

### Dependency rules

- `L(n)` may depend on `L(n-1)` and below, never upward.
- `L7 Analytics` may depend on `L1` through `L6`, but must not depend on UI, HTML, Svelte, SwiftUI, or chart-library code.
- `L8 Visualisation` may consume canonical `L7` outputs and selected raw `L3`/`L5`/`L6` artifacts for debug rendering.
- `L8` must not define canonical metrics, summaries, or comparisons.
- `storage/sqlite` is infrastructure and persistence, not the permanent home of analytics logic.
- `monitor/` is an application/integration boundary, not a canonical domain layer.

### Ownership rules

- `L6` owns semantic interpretation of individual tracked actors.
- `L7` owns aggregate meaning derived from tracks, runs, scenes, sweeps, and labels.
- `L8` owns human-facing rendering, interaction flows, review payloads, dashboards, debug views, and visualiser/web contracts.
- infrastructure owns transport, persistence, replay, runtime wiring, and process control.

### Anti-patterns to avoid

- canonical summary math embedded inside HTTP handlers
- canonical metrics computed inside Svelte, Swift, or chart-specific code
- SQL stores owning analytics algorithms
- `monitor/` becoming a permanent catch-all for every LiDAR concern
- moving files mechanically without clarifying ownership

## Corrections and Clarifications Relative to the Imported Handover

The imported draft should be preserved in spirit, but these repo-specific corrections must be reflected in the checked-in plan:

- `L8` is already partially implemented. The plan must formalize and clean up an existing boundary, not just “define” one.
- run comparison is already implemented, but it currently lives in `L6` and `storage/sqlite`; that is a concrete migration target for `L7`.
- `monitor/` is not just “transitional” in the abstract. It must be classified file-by-file into infra, `L7`-backed application services, and `L8` presentation code.
- the canonical layer doc path should stay stable if possible. Updating `docs/lidar/architecture/lidar-data-layer-model.md` in place is preferable to renaming it and creating widespread link churn.
- the first pass should focus on LiDAR. The radar-focused PDF generator and site-report flow may be referenced as broader presentation consumers, but they should not block this LiDAR refactor.

## Proposed Target Ownership Map

### L7 Analytics

Add a canonical package:

- `internal/lidar/l7analytics/`

Recommended initial file split:

- `doc.go`
- `types.go`
- `percentiles.go`
- `summary.go`
- `comparison.go`
- `labels.go`

Initial `L7` candidates to move or extract:

- `RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge`
- `ComputeTemporalIoU`
- speed percentile helpers now used by `track_store.go` and `analysis_run.go`
- run summary and track summary aggregation currently in `monitor/track_api.go`
- run-level statistics now mixed into `l6objects/quality.go`
- labelling progress and run-evaluation summary types where they represent aggregates, not raw storage rows
- parameter comparison helpers currently tied to run comparison

### L8 Visualisation

Keep the current L8 surface area explicit rather than forcing it all into one directory:

- `internal/lidar/visualiser/` remains the canonical Go-side visualiser adapter boundary
- `proto/velocity_visualiser/v1/` remains the canonical gRPC contract boundary
- `web/` remains the browser UI
- `tools/visualiser-macos/` remains the native visualiser client

For server-side presentation shaping, introduce one explicit home if the amount of Go-side L8 code continues to grow:

- preferred option: `internal/lidar/l8presentation/`

If that package is not created in the first implementation pass, the plan must still explicitly classify the following as L8-owned code:

- `internal/lidar/monitor/chart_data.go`
- chart-specific response structs in `internal/lidar/monitor/chart_api.go`
- ECharts/template assets and HTML dashboards in `internal/lidar/monitor/`

### monitor/ application split

The long-term goal is not to delete `monitor/` immediately. It is to make its mixed ownership obvious and bounded.

Provisional classification:

| monitor area | Likely target role |
| --- | --- |
| `stats.go`, `datasource*.go`, `playback_handlers.go`, `export_handlers.go`, route registration in `webserver.go` | infrastructure/application |
| `track_api.go`, `scene_api.go`, `run_track_api.go`, parts of `sweep_handlers.go` | thin HTTP layer over `L7` services |
| `chart_api.go`, `chart_data.go`, `echarts_handlers.go`, `templates.go`, `html/`, `assets/` | `L8 Visualisation` |

## Phased Implementation Plan

### Phase 1: Lock the Architecture Contract and Update Docs

### Goals

- make the eight-layer model the documented source of truth
- preserve stable documentation paths where possible
- record the breaking-change intent before code moves begin

### Work

- update `docs/lidar/architecture/lidar-data-layer-model.md` from six layers to eight layers rather than creating a second competing canonical doc
- update `docs/lidar/architecture/README.md` to describe `L1` through `L8`
- update `docs/lidar/README.md`, `docs/data/DATA_STRUCTURES.md`, and `docs/lidar/terminology.md`
- update package doc comments in `internal/lidar/l1packets/doc.go` through `internal/lidar/l6objects/doc.go`
- update any layer-language references in `ARCHITECTURE.md`, `README.md`, and `internal/lidar/aliases.go` if they describe the old model
- add a breaking-change note and a short migration note for future implementers

### Outputs

- a single canonical eight-layer architecture doc
- no repo docs still describing the LiDAR architecture as six layers unless explicitly marked historical
- a documented target interpretation of `monitor/` as transitional/application code

### Phase 2: Add the Canonical L7 Analytics Boundary

### Goals

- introduce a real `L7` home before moving logic around
- make it obvious which analytics concepts are canonical

### Work

- add `internal/lidar/l7analytics/` with package docs and tests
- move `RunComparison` types and `ComputeTemporalIoU` out of `l6objects/comparison.go`
- move run-level statistics that are not object semantics out of `l6objects/quality.go`
- add canonical helpers for speed percentile calculation and shared track/run aggregation
- define small, stable `L7` result types for summary endpoints and run comparison output

### Outputs

- `L7` exists as a real import path
- `L6` is narrowed back toward semantic actor interpretation
- moved code has direct tests in its new home

### Phase 3: Re-home Analytics Logic from Storage and Handlers

### Goals

- remove analytics computation from persistence code and route handlers
- make `storage/sqlite` persistence-only where practical

### Work

- update `internal/lidar/storage/sqlite/track_store.go` to call `L7` helpers for percentile math
- slim `internal/lidar/storage/sqlite/analysis_run.go` so it stores and loads data, but does not own comparison logic
- move `compareParams` and run comparison orchestration into `L7` if they remain canonical analytics behavior
- extract track summary aggregation from `internal/lidar/monitor/track_api.go` into `L7`
- extract run-labelling and run-evaluation aggregate logic from `internal/lidar/monitor/run_track_api.go` and `scene_api.go` into `L7`-backed services
- keep handler files responsible for request parsing, response codes, and transport concerns only

### Outputs

- storage code is simpler and easier to reason about
- handlers delegate to explicit use-case logic
- `L7` becomes the only canonical home of LiDAR run/summary/comparison analytics

### Phase 4: Formalize the L8 Visualisation Boundary

### Goals

- make presentation shaping explicit
- keep canonical metrics out of UI and chart code

### Work

- document `internal/lidar/visualiser/`, `proto/velocity_visualiser/v1/`, `web/`, and `tools/visualiser-macos/` as the primary L8 surfaces
- decide whether to create `internal/lidar/l8presentation/` for server-side payload shaping
- move chart-specific structs and transformation helpers from `internal/lidar/monitor/chart_data.go` and related code into the chosen L8 home
- review `internal/lidar/visualiser/adapter.go` and `grpc_server.go` to confirm they only adapt and format existing domain outputs
- review `web/src/lib/api.ts`, `web/src/lib/types/lidar.ts`, and LiDAR route code to ensure canonical analytics are supplied by the server, not recomputed in the client
- keep debug overlays and view-model shaping in L8 even when they directly consume `L3`, `L5`, or `L6` artifacts

### Outputs

- the Go-side visualisation boundary is documented and intentional
- server-side chart and dashboard shaping is no longer mixed with analytics logic
- client applications consume canonical analytics rather than recreating them

### Phase 5: Decompose monitor/ into Explicit Roles

### Goals

- reduce `monitor/` ambiguity without forcing an unnecessary full rewrite
- keep route registration and runtime wiring stable while ownership improves underneath

### Work

- classify every `internal/lidar/monitor/*.go` file as infra/application, `L7`-backed API, or `L8` presentation
- move only clearly owned code in this pass
- leave infrastructure-oriented files in `monitor/`
- route mixed handlers through extracted services instead of embedding business logic
- mark deferred moves with explicit destination comments in docs, not vague future intent

### Outputs

- a documented file-by-file ownership map for `monitor/`
- fewer mixed-responsibility files
- no new logic added to `monitor/` without an explicit ownership reason

### Phase 6: Generated Architecture Artifacts, Tests, and Migration Support

### Goals

- make the architecture visible and reproducible
- keep the breaking change implementable without tribal knowledge

### Work

- add a generated DOT graph for `L1` through `L8`
- generate and check in the corresponding SVG
- add a small reproducible script for regeneration
- add tests for moved analytics helpers and for handlers that now depend on extracted services
- add a migration note listing package moves, deferred moves, and any compatibility shims
- add a lightweight guardrail so generated architecture artifacts are not silently stale

### Suggested artifact paths

- `docs/lidar/architecture/generated/lidar-layer-model.dot`
- `docs/lidar/architecture/generated/lidar-layer-model.svg`
- `scripts/generate-lidar-layer-graph.sh`

## Recommended Sequencing

1. update the canonical docs to the eight-layer model
2. create `internal/lidar/l7analytics/`
3. move obvious `L7` code out of `l6objects`, `storage/sqlite`, and handler files
4. formalize the L8 boundary and re-home server-side presentation shaping
5. classify and trim `monitor/`
6. add graph generation, migration notes, and verification

## Risks and Guardrails

### Main risks

- broad rename churn with little ownership value
- moving storage and handler code at the same time without enough tests
- blurring `L6` versus `L7` around “quality” and training-curation helpers
- breaking web or visualiser clients by changing response shapes and contracts too aggressively

### Guardrails

- preserve stable doc paths when possible
- prefer extracting logic first, then relocating files if the ownership benefit is clear
- keep response contracts compatible where possible during the transition
- do not let `storage/sqlite` or `monitor/` become the fallback home for new analytics
- document deferred moves explicitly instead of leaving hidden architectural debt

## Non-Goals

- full rewrite of the LiDAR subsystem
- full removal of `monitor/` in one pass
- broad mechanical renaming with no ownership improvement
- major redesign of the web app or macOS visualiser unrelated to the layer split
- refactoring the radar-focused PDF generator as a prerequisite for the LiDAR layer split

## Complete Checklist

### Docs and architecture

- [ ] `docs/lidar/architecture/lidar-data-layer-model.md` updated to the eight-layer model
- [ ] `docs/lidar/architecture/README.md` updated to describe `L1` through `L8`
- [ ] `docs/lidar/README.md`, `docs/data/DATA_STRUCTURES.md`, and `docs/lidar/terminology.md` updated
- [ ] relevant package doc comments under `internal/lidar/` updated
- [ ] breaking-change rationale documented
- [ ] migration note documented

### L7 analytics boundary

- [ ] `internal/lidar/l7analytics/` exists with package docs
- [ ] `RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge`, and temporal IoU logic moved out of `L6`
- [ ] speed percentile helpers no longer live only in storage code
- [ ] run-level summary/statistics logic split out of `l6objects/quality.go`
- [ ] handler summary logic delegates to `L7`
- [ ] comparison logic delegates to `L7`
- [ ] new or moved `L7` code has direct unit tests

### L8 visualisation boundary

- [ ] canonical `L8` surfaces documented for Go, proto, web, and macOS
- [ ] server-side chart/view-model shaping has an explicit `L8` home
- [ ] `chart_data.go`-style presentation helpers are no longer treated as analytics code
- [ ] clients do not compute canonical summary metrics locally
- [ ] debug and dashboard payload shaping is explicitly classified as `L8`

### monitor/ decomposition

- [ ] each `monitor/` file is classified as infra/application, `L7`-backed API, or `L8` presentation
- [ ] infrastructure-oriented files remain in `monitor/`
- [ ] mixed handlers call extracted services instead of embedding analytics math
- [ ] deferred moves are documented with explicit destinations
- [ ] no new upward dependency violations are introduced

### Generated artifacts and verification

- [ ] DOT graph added
- [ ] SVG generated and checked in
- [ ] graph generation is reproducible via script
- [ ] tests updated for moved analytics and changed handlers
- [ ] verification or CI guardrail exists for generated artifacts
- [ ] final checked-in plan is sufficient to drive follow-on implementation PRs
