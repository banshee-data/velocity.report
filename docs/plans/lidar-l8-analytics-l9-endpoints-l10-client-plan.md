# LiDAR L8 Analytics / L9 Endpoints / L10 Client Tier Refactor Plan

**Status:** Proposed implementation plan — **layer numbers updated 2026-03-08**
**Layers:** L8 Analytics, L9 Endpoints, L10 Client
**Source:** Imported from the original planning document `plan-l7-l8.md`, then reviewed against the repository state on 2026-03-06. Updated 2026-03-07 to adopt a nine-layer model. **Updated 2026-03-08:** Renumbered to ten-layer model — L7 Scene inserted, previous L7→L8, L8→L9, L9→L10. All body references updated. File renamed from `lidar-l7-analytics-l8-presentation-l9-client-plan.md`. **Updated 2026-07-13:** L9 renamed from Presentation to Endpoints; file renamed from `lidar-l8-analytics-l9-presentation-l10-client-plan.md`.
**Scope:** LiDAR architecture docs, `internal/lidar`, `proto/velocity_visualiser/v1`, `web/`, and the macOS visualiser integration boundary.

## Executive Summary

velocity.report currently documents and implements a six-layer LiDAR model that ends at `L6 Objects`. The desired target is a ten-layer model. L7 Scene (persistent world model and multi-sensor fusion) is covered in a [separate plan](lidar-l7-scene-plan.md). This plan adds:

- `L8 Analytics` for canonical traffic, safety, and run-analysis logic
- `L9 Endpoints` for server-side operator-facing payload shaping, the gRPC stream contract, dashboards, debug views, and visual review workflows — canonical Go home is `internal/lidar/l9endpoints/` (renamed from `internal/lidar/visualiser/`)
- `L10 Client Tier` (documentation label only, no Go package) for downstream rendering consumers: browser (Svelte), native app (Swift/VeloVis), and report generation (Python PDF generator)

This is a breaking architectural change, not just a terminology update. The main goal is to stop overloading `L6 Objects`, `storage/sqlite`, and `monitor/` with responsibilities that belong to analytics and endpoint.

The original plan was directionally correct, but the repository review shows that the work needs to be more explicit about:

- the existing six-layer docs that must be updated
- analytics logic that already exists but is currently misplaced
- the fact that L9 already partially exists in `internal/lidar/visualiser/`, `proto/velocity_visualiser/v1/`, `web/`, and `tools/visualiser-macos/`
- the mixed nature of `internal/lidar/monitor/`, which currently contains infrastructure, analytics-backed APIs, and endpoint shaping
- the L10 client tier, which is already enforced structurally by language and directory boundaries but must be named and documented so the dependency chain is explicit

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

There is no `internal/lidar/l8analytics/` package today, and there is no explicit `L9` package boundary on the Go side.

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

The repo already contains analytics and endpoint logic that do not fit the current documented ownership:

| Current location (under `internal/lidar/`) | Current responsibility                                             | Target ownership                              |
| ------------------------------------------ | ------------------------------------------------------------------ | --------------------------------------------- |
| `l6objects/comparison.go`                  | run comparison types and temporal IoU helpers                      | `L8 Analytics`                                |
| `l6objects/quality.go`                     | mixed per-track quality helpers and run-level aggregate statistics | split between `L6 Objects` and `L8 Analytics` |
| `storage/sqlite/track_store.go`            | speed percentile calculation during persistence                    | `L8 Analytics` helper called by storage       |
| `storage/sqlite/analysis_run.go`           | run comparison orchestration, percentiles, run-track summary logic | storage plus `L8 Analytics` service split     |
| `storage/sqlite/analysis_run_compare.go`   | parameter diffing for run comparison                               | likely `L8 Analytics`                         |
| `monitor/track_api.go`                     | track summary aggregation and response shaping                     | `L8 Analytics` plus `L9`/handler boundary     |
| `monitor/chart_data.go`                    | chart-specific view-model shaping                                  | `L9 Endpoints`                                |
| `monitor/chart_api.go`                     | endpoint-facing chart APIs                                         | `L9 Endpoints`                                |
| `monitor/scene_api.go`                     | scene CRUD plus evaluation/replay orchestration                    | mixed infra plus `L8` application services    |
| `monitor/run_track_api.go`                 | run, labelling, evaluation, and comparison flows                   | mixed infra plus `L8` application services    |

### L9 and L10 already exist in practice

The current repository already has clear L9 and L10 surfaces:

**Go-side L9 (to be renamed `internal/lidar/l9endpoints/`):**

- `internal/lidar/visualiser/` currently contains the gRPC stream adapter, proto frame encoding, the canonical server-side visualiser model, and playback/replay adapters — this is the core of L9 Endpoints
- `proto/velocity_visualiser/v1/visualiser.proto` defines the canonical gRPC contract boundary between Go L9 and L10 consumers
- `internal/lidar/monitor/chart_data.go`, `chart_api.go`, `echarts_handlers.go`, `templates.go` — server-side chart and dashboard shaping that logically belongs to L9

**L10 Client Tier (documentation-only label, no Go package):**

- `web/src/routes/lidar/*` — browser-side LiDAR review and control workflows (Svelte)
- `tools/visualiser-macos/` — native operator-facing visualiser (Swift/Metal)
- `tools/pdf-generator/` — report generation that consumes REST API outputs (Python)

This means Phase 1 is not "invent L9 from nothing". It is "rename and formalise `internal/lidar/visualiser/` as `internal/lidar/l9endpoints/`, absorb the misplaced chart/dashboard shaping, document the L10 boundary, and apply consistent naming across L1–L9".

## Target Ten-Layer Model

| Layer | Label       | Responsibility                                                                                                                                                                                  |
| ----- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| L1    | Packets     | wire transport, UDP capture, PCAP replay, packet parsing                                                                                                                                        |
| L2    | Frames      | frame assembly, timestamps, geometry conversion, exports                                                                                                                                        |
| L3    | Grid        | background model, foreground masking, persistence, drift, regions                                                                                                                               |
| L4    | Perception  | per-frame scene interpretation, clustering, OBBs, ground removal                                                                                                                                |
| L5    | Tracks      | temporal association, identity, lifecycle, motion estimation                                                                                                                                    |
| L6    | Objects     | semantic actor interpretation and object-level quality/classification                                                                                                                           |
| L7    | Scene       | persistent evidence-accumulated world model; static geometry, canonical objects, external priors, multi-sensor fusion. See [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md)                     |
| L8    | Analytics   | canonical metrics, summaries, comparisons, scoring, evaluation logic                                                                                                                            |
| L9    | Endpoints   | server-side payload shaping, gRPC stream contract, dashboards, debug views, review payloads — `internal/lidar/l9endpoints/`                                                                     |
| L10   | Client Tier | **documentation label only — no Go package.** Rendering consumers: browser (Svelte), native (Swift/VeloVis), report gen (Python). Depend on L9 contracts; must not recompute canonical metrics. |

## Design Rules

### Dependency rules

- `L(n)` may depend on `L(n-1)` and below, never upward.
- `L8 Analytics` may depend on `L1` through `L6`, but must not depend on UI, HTML, Svelte, SwiftUI, or chart-library code.
- `L9 Endpoints` may consume canonical `L8` outputs and selected raw `L3`/`L5`/`L6` artifacts for debug rendering. It must not define canonical metrics, summaries, or comparisons.
- `L10 Client Tier` consumes the contracts published by `L9` (proto, JSON APIs). L10 code must not compute canonical analytics — if a metric is needed, the server must provide it via `L8`-backed `L9` endpoints.
- `storage/sqlite` is infrastructure and persistence, not the permanent home of analytics logic.
- `monitor/` is an application/integration boundary, not a canonical domain layer.

### Ownership rules

- `L6` owns semantic interpretation of individual tracked actors.
- `L8` owns aggregate meaning derived from tracks, runs, scenes, sweeps, and labels.
- `L9` owns server-side rendering payloads, the gRPC stream contract, chart/view-model shaping, HTTP endpoint structs, and debug overlay payloads. Canonical Go home: `internal/lidar/l9endpoints/`.
- `L10` (documentation only) owns client rendering, interaction UX, and local display logic. Receives canonical data from `L9`; does not own analytics.
- Infrastructure owns transport, persistence, replay, runtime wiring, and process control.

### Anti-patterns to avoid

- canonical summary math embedded inside HTTP handlers
- canonical metrics computed inside Svelte, Swift, or chart-specific code (L10 computing what L8 should own)
- SQL stores owning analytics algorithms
- `monitor/` becoming a permanent catch-all for every LiDAR concern
- moving files mechanically without clarifying ownership
- `internal/lidar/visualiser/` being referenced by its old name after the rename to `l9endpoints/`

## Corrections and Clarifications Relative to the Imported Handover

The imported draft should be preserved in spirit, but these repo-specific corrections must be reflected in the checked-in plan:

- `L9` is already partially implemented. The plan must formalise and clean up an existing boundary, not just "define" one.
- The canonical Go package for `L9` is `internal/lidar/l9endpoints/`, renamed from `internal/lidar/visualiser/`. All import paths referencing the old name must be updated.
- run comparison is already implemented, but it currently lives in `L6` and `storage/sqlite`; that is a concrete migration target for `L8`.
- `monitor/` is not just "transitional" in the abstract. It must be classified file-by-file into infra, `L8`-backed application services, and `L9` endpoint code.
- the canonical layer doc path should stay stable if possible. Updating `docs/lidar/architecture/lidar-data-layer-model.md` in place is preferable to renaming it and creating widespread link churn.
- the first pass should focus on LiDAR. The radar-focused PDF generator and site-report flow are L10 consumers and may be referenced as such, but they should not block this LiDAR refactor.
- `L10` is documentation-only. No Go package is created. The client-tier boundary is already enforced structurally by language (JS/Swift/Python vs Go) and by `proto/velocity_visualiser/v1/` acting as the formal wire contract seam.

## Proposed Target Ownership Map

### L8 Analytics

Add a canonical package:

- `internal/lidar/l8analytics/`

Recommended initial file split:

- `doc.go`
- `types.go`
- `percentiles.go`
- `summary.go`
- `comparison.go`
- `labels.go`

Initial `L8` candidates to move or extract:

- `RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge`
- `ComputeTemporalIoU`
- speed percentile helpers now used by `track_store.go` and `analysis_run.go`
- run summary and track summary aggregation currently in `monitor/track_api.go`
- run-level statistics now mixed into `l6objects/quality.go`
- labelling progress and run-evaluation summary types where they represent aggregates, not raw storage rows
- parameter comparison helpers currently tied to run comparison

### L9 Endpoints

Rename `internal/lidar/visualiser/` to `internal/lidar/l9endpoints/`. This is a first-class Go package consistent with the `l1packets/` through `l8analytics/` naming scheme.

**What moves in from monitor/ during Phase 4:**

- `internal/lidar/monitor/chart_data.go` — chart view-model shaping
- `internal/lidar/monitor/chart_api.go` — endpoint-facing chart response structs
- `internal/lidar/monitor/echarts_handlers.go` — ECharts-specific formatting
- `internal/lidar/monitor/templates.go` — HTML template rendering and assets

**What stays in `internal/lidar/l9endpoints/` from the current `visualiser/` package:**

- `adapter.go` — converts pipeline domain objects into the proto frame model
- `frame_codec.go` — proto encoding
- `grpc_server.go` — gRPC streaming server
- `model.go` — canonical server-side visualiser data model (Track, FrameBundle, etc.)
- `publisher.go`, `recorder/`, `replay.go` — stream publishing and replay infrastructure
- `config.go` — L9 endpoints configuration
- `lidarview_adapter.go` — LidarView adapter
- `pb/` — compiled proto bindings (stays co-located)

**What `internal/lidar/l9endpoints/` must not contain:**

- canonical analytics, summary calculations, or comparison logic (those belong in `L8`)
- infrastructure wiring, route registration, or HTTP multiplexing (those remain in `monitor/`)

**Proto contract:**

`proto/velocity_visualiser/v1/visualiser.proto` remains the formal contract seam between the Go `L9` package and L10 consumers. Changes to the proto are breaking changes for all L10 clients.

### L10 Client Tier (documentation label only)

`L10` has no Go package. It is a documentation label for the rendering consumers that sit downstream of the `L9` contract boundary.

| L10 surface               | Language          | Consumes                                               |
| ------------------------- | ----------------- | ------------------------------------------------------ |
| `web/src/routes/lidar/`   | TypeScript/Svelte | HTTP JSON APIs and gRPC-Web stream from `L9`           |
| `tools/visualiser-macos/` | Swift/Metal       | gRPC stream defined by `proto/velocity_visualiser/v1/` |
| `tools/pdf-generator/`    | Python            | REST API endpoints backed by `L8` analytics            |

**Rules for L10 surfaces:**

- may call `L9` endpoints (REST, gRPC-Web) and render the results
- must not recompute canonical metrics locally — request them from `L8`-backed `L9` endpoints instead
- when a new summary field is needed, the correct fix is adding an `L8` helper and exposing it through an `L9` endpoint, not computing it in Svelte, Swift, or Python
- the proto contract (`proto/velocity_visualiser/v1/`) is an explicit versioned seam; L10 clients must track breaking changes declared there

### monitor/ application split

The long-term goal is not to delete `monitor/` immediately. It is to make its mixed ownership obvious and bounded, and to enforce the downward dependency chain by having handler code call into explicitly-layered packages rather than embedding logic directly.

Provisional classification:

| monitor area                                                                                                     | Likely target role                      | Dependency rule                                                                               |
| ---------------------------------------------------------------------------------------------------------------- | --------------------------------------- | --------------------------------------------------------------------------------------------- |
| `stats.go`, `datasource*.go`, `playback_handlers.go`, `export_handlers.go`, route registration in `webserver.go` | infrastructure/application              | may import `L1`–`L9` as needed                                                                |
| `track_api.go`, `scene_api.go`, `run_track_api.go`, parts of `sweep_handlers.go`                                 | thin HTTP layer over `L8` services      | must call `l8analytics/` for any summary or comparison math; no analytics embedded in handler |
| `chart_api.go`, `chart_data.go`, `echarts_handlers.go`, `templates.go`, `html/`, `assets/`                       | `L9 Endpoints` — move to `l9endpoints/` | must call `l8analytics/` for metric values; may shape output for L10 consumers                |

**Enforcement expectation:** after Phase 4, no file in `monitor/` may contain summary statistics, comparison logic, or percentile calculations. Any handler that currently computes these must be refactored to call an `L8` function. This makes the API layer a thin translation boundary — request parsing, authorisation checks, and response serialisation only.

## Phased Implementation Plan

### Phase 1: Lock the Architecture Contract and Update Docs

### Goals

- make the ten-layer model the documented source of truth
- preserve stable documentation paths where possible
- record the L9 rename, L10 client-tier label, and breaking-change intent before code moves begin

### Work

- update `docs/lidar/architecture/lidar-data-layer-model.md` from six layers to ten layers: add L7 (Scene), L8 (Analytics), L9 (renamed to Endpoints), and L10 (Client Tier, documentation-only)
- update `docs/lidar/architecture/README.md` to describe `L1` through `L10`
- update `docs/lidar/README.md`, `docs/data/DATA_STRUCTURES.md`, and `docs/lidar/terminology.md`
- update package doc comments in `internal/lidar/l1packets/doc.go` through `internal/lidar/l6objects/doc.go`
- update any layer-language references in `ARCHITECTURE.md`, `README.md`, and `internal/lidar/aliases.go` if they describe the old model
- document `internal/lidar/visualiser/` in the architecture docs as "will be renamed to `internal/lidar/l9endpoints/` in Phase 4"
- document the L10 client-tier surfaces in the architecture: `web/`, `tools/visualiser-macos/`, `tools/pdf-generator/`
- add a breaking-change note and a short migration note for future implementers

### Outputs

- a single canonical ten-layer architecture doc
- no repo docs still describing the LiDAR architecture as six layers unless explicitly marked historical
- `L9 Endpoints` and `L10 Client Tier` documented with ownership rules and the planned rename noted
- a documented target interpretation of `monitor/` as transitional/application code

### Phase 2: Add the Canonical L8 Analytics Boundary

### Goals

- introduce a real `L8` home before moving logic around
- make it obvious which analytics concepts are canonical

### Work

- add `internal/lidar/l8analytics/` with package docs and tests
- move `RunComparison` types and `ComputeTemporalIoU` out of `l6objects/comparison.go`
- move run-level statistics that are not object semantics out of `l6objects/quality.go`
- add canonical helpers for speed percentile calculation and shared track/run aggregation
- define small, stable `L8` result types for summary endpoints and run comparison output

### Outputs

- `L8` exists as a real import path
- `L6` is narrowed back toward semantic actor interpretation
- moved code has direct tests in its new home

### Phase 3: Re-home Analytics Logic from Storage and Handlers

### Goals

- remove analytics computation from persistence code and route handlers
- make `storage/sqlite` persistence-only where practical

### Work

- update `internal/lidar/storage/sqlite/track_store.go` to call `L8` helpers for percentile math
- slim `internal/lidar/storage/sqlite/analysis_run.go` so it stores and loads data, but does not own comparison logic
- move `compareParams` and run comparison orchestration into `L8` if they remain canonical analytics behavior
- extract track summary aggregation from `internal/lidar/monitor/track_api.go` into `L8`
- extract run-labelling and run-evaluation aggregate logic from `internal/lidar/monitor/run_track_api.go` and `scene_api.go` into `L8`-backed services
- keep handler files responsible for request parsing, response codes, and transport concerns only

### Outputs

- storage code is simpler and easier to reason about
- handlers delegate to explicit use-case logic
- `L8` becomes the only canonical home of LiDAR run/summary/comparison analytics

### Phase 4: Formalise the L9 Endpoints Boundary

### Goals

- rename `internal/lidar/visualiser/` to `internal/lidar/l9endpoints/`
- absorb the misplaced endpoint code from `monitor/` into the explicit L9 home
- keep canonical metrics out of UI and chart code

### Work

- rename `internal/lidar/visualiser/` to `internal/lidar/l9endpoints/` and update all import paths
  - external callers requiring import-path updates include: `cmd/radar/radar.go`, `cmd/tools/visualiser-server/main.go`, and `cmd/tools/gen-vrlog/main.go`
- move chart and dashboard endpoint code from `monitor/` into `internal/lidar/l9endpoints/`:
  - `monitor/chart_data.go` — coordinate transforms, polar/cartesian downsampling, chart view-model structs
  - `monitor/chart_api.go` — endpoint-facing chart response structs
  - `monitor/echarts_handlers.go` — ECharts-specific formatting and series helpers
  - `monitor/templates.go` — HTML template rendering and embedded assets
- update `monitor/` callers of moved chart code to import from `l9endpoints/`
- review `internal/lidar/l9endpoints/adapter.go` and `grpc_server.go` to confirm they only adapt and format existing domain outputs; note `grpc_server.go` imports `l6objects` for classification during VRLOG replay — this is acceptable L6→L9 usage
- review `web/src/lib/api.ts`, `web/src/lib/types/lidar.ts`, and LiDAR route code to confirm canonical analytics are supplied by the server, not recomputed in the client
- keep debug overlays and view-model shaping in `l9endpoints/` even when they directly consume `L3`, `L5`, or `L6` artifacts

### Outputs

- `internal/lidar/visualiser/` no longer exists; all callers use `internal/lidar/l9endpoints/`
- server-side chart and dashboard shaping has a single, named home
- `monitor/` no longer contains endpoint-layer types or coordinate transforms
- client applications consume canonical analytics from `L8`-backed `L9` endpoints

### Phase 5: Decompose monitor/ into Explicit Roles

### Goals

- reduce `monitor/` ambiguity without forcing an unnecessary full rewrite
- keep route registration and runtime wiring stable while ownership improves underneath

### Work

- classify every `internal/lidar/monitor/*.go` file as infra/application, `L8`-backed API, or `L9` endpoint
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

- add a generated DOT graph for `L1` through `L9`
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

1. update the canonical docs to the ten-layer model
2. create `internal/lidar/l8analytics/`
3. move obvious `L8` code out of `l6objects`, `storage/sqlite`, and handler files
4. formalize the L9 boundary and re-home server-side endpoint shaping
5. classify and trim `monitor/`
6. add graph generation, migration notes, and verification

## Risks and Guardrails

### Main risks

- broad rename churn with little ownership value
- moving storage and handler code at the same time without enough tests
- blurring `L6` versus `L8` around "quality" and training-curation helpers: `l6objects/quality.go` mixes per-track quality predicates (clearly L6) with `RunStatistics` / `ComputeRunStatistics` (15+ run-level aggregate fields — these are L8). The split must be deliberate: object-level predicates stay in L6; aggregate run metrics move to `l8analytics/summary.go`
- breaking web or visualiser clients by changing response shapes and contracts too aggressively

### Guardrails

- preserve stable doc paths when possible
- prefer extracting logic first, then relocating files if the ownership benefit is clear
- keep response contracts compatible where possible during the transition
- do not let `storage/sqlite` or `monitor/` become the fallback home for new analytics
- document deferred moves explicitly instead of leaving hidden architectural debt — the inventory below is the concrete record of what remains and where it belongs

## Concrete Tech Debt Inventory

This table records every known ownership mismatch in the current codebase with the exact file and the concrete function or logic that needs to move. Future work must address all of these to complete the structural refactor.

### Misplaced L8 logic in L6

| File                                         | Code that needs to move                                                                     | Target in `l8analytics/`                                 | Callers to update                                                        |
| -------------------------------------------- | ------------------------------------------------------------------------------------------- | -------------------------------------------------------- | ------------------------------------------------------------------------ |
| `internal/lidar/l6objects/comparison.go`     | `RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge` types                             | `types.go` or `comparison.go`                            | `storage/sqlite/analysis_run_compare.go`, `monitor/run_track_api.go`     |
| `internal/lidar/l6objects/comparison.go`     | `ComputeTemporalIoU` function                                                               | `comparison.go`                                          | same as above                                                            |
| `internal/lidar/l6objects/quality.go`        | `RunStatistics` struct and `ComputeRunStatistics` function (15+ aggregate run-level fields) | `summary.go`                                             | `storage/sqlite/analysis_run.go`, `monitor/track_api.go`                 |
| `internal/lidar/l6objects/classification.go` | legacy speed-summary helper at callsite near line 508                                       | caller delegates to `l8analytics/percentiles.go` instead | any code calling the classification helper for legacy speed-summary math |

**L6 after this move:** retains per-object quality predicates, per-object classification, and per-track attributes. Does not own run-level aggregates or comparison orchestration.

### Misplaced L8 logic in storage

| File                                                    | Code that needs to move                                                                                 | Target                                                              | Notes                                                                                                          |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `internal/lidar/storage/sqlite/track_store.go`          | legacy single-track speed-summary math called inline during `InsertTrack()`                             | caller passes pre-computed values from `l8analytics/percentiles.go` | percentile math should not live in the persistence path                                                        |
| `internal/lidar/storage/sqlite/analysis_run.go`         | legacy speed-summary helper call inline at line ~226 during run persistence                             | same as above                                                       | same problem as `track_store.go`                                                                               |
| `internal/lidar/storage/sqlite/analysis_run.go`         | run comparison orchestration logic currently mixed with storage                                         | `l8analytics/comparison.go` service function                        | storage should store and load, not orchestrate comparison                                                      |
| `internal/lidar/storage/sqlite/analysis_run_compare.go` | Hungarian algorithm for track matching, temporal IoU calculation at persistence layer (lines 1042–1216) | `l8analytics/comparison.go`                                         | this is the most substantial misplacement: a full combinatorial matching algorithm lives in the SQLite package |

**storage/sqlite after this move:** stores and retrieves data; calls `l8analytics` helpers for any derived values it must persist; does not own analytics algorithms.

### Misplaced L9 endpoints logic in monitor/

| File                                         | Code that needs to move                                                                                                                  | Target in `l9endpoints/`    |
| -------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | --------------------------- |
| `internal/lidar/monitor/chart_data.go`       | all chart view-model structs, polar/cartesian coordinate transforms, downsampling helpers, `PreparePolarChartData` and related functions | `l9endpoints/chart_data.go` |
| `internal/lidar/monitor/chart_api.go`        | endpoint-facing chart response structs                                                                                                   | `l9endpoints/chart_api.go`  |
| `internal/lidar/monitor/echarts_handlers.go` | ECharts-specific series formatting and response helpers                                                                                  | `l9endpoints/echarts.go`    |
| `internal/lidar/monitor/templates.go`        | HTML template rendering, embedded template assets                                                                                        | `l9endpoints/templates.go`  |

**monitor/ after this move:** retains route registration, request parsing, authorisation checks, response serialisation, and infrastructure wiring. Does not contain chart structs, coordinate math, or ECharts helpers.

### visualiser/ → l9endpoints/ rename: affected files

The directory `internal/lidar/visualiser/` becomes `internal/lidar/l9endpoints/`. Files that stay (all kept, just under the new path):

| File                   | Role                                                                                               |
| ---------------------- | -------------------------------------------------------------------------------------------------- |
| `adapter.go`           | converts pipeline domain objects into the proto frame model                                        |
| `frame_codec.go`       | proto frame encoding                                                                               |
| `grpc_server.go`       | gRPC streaming server (imports `l6objects` for VRLOG classification — acceptable L6→L9 dependency) |
| `model.go`             | canonical server-side data model (Track, FrameBundle, etc.)                                        |
| `publisher.go`         | stream publishing                                                                                  |
| `recorder/`            | stream recording infrastructure                                                                    |
| `replay.go`            | PCAP and VRLOG replay adapters                                                                     |
| `config.go`            | L9 endpoints configuration                                                                         |
| `lidarview_adapter.go` | LidarView export adapter                                                                           |
| `pb/`                  | compiled proto bindings (co-located, stays)                                                        |

**External callers that require import-path updates (exhaustive):**

| Caller                                | Import to change                                           |
| ------------------------------------- | ---------------------------------------------------------- |
| `cmd/radar/radar.go`                  | `internal/lidar/visualiser` → `internal/lidar/l9endpoints` |
| `cmd/tools/visualiser-server/main.go` | `internal/lidar/visualiser` → `internal/lidar/l9endpoints` |

No other files outside `internal/lidar/` import `internal/lidar/visualiser/` directly. The rename is low-risk.

### Deferred moves (explicitly out of scope for the initial phases)

These ownership issues are noted here to avoid hidden architectural debt, but are not blocking for the initial phases:

| Item                                                                                    | Current location | Correct owner           | Why deferred                                                                |
| --------------------------------------------------------------------------------------- | ---------------- | ----------------------- | --------------------------------------------------------------------------- |
| `monitor/gridplotter.go` — grid visualisation and colourisation                         | `monitor/`       | `l9endpoints/`          | requires understanding grid overlay contracts; defer to Phase 4 follow-up   |
| Labelling-progress and evaluation-summary aggregate types in `monitor/run_track_api.go` | `monitor/`       | `l8analytics/labels.go` | extraction requires splitting aggregation from transport; Phase 3 follow-up |
| Scene CRUD vs. evaluation orchestration in `monitor/scene_api.go`                       | `monitor/`       | `l8analytics/labels.go` | extraction requires splitting aggregation from transport; Phase 3 follow-up |

## Full monitor/ deprecation analysis

### Package Census

`internal/lidar/monitor/` currently contains **10,154 lines** of production Go code across 19 files, **24,682 lines** of tests across 22 test files, and **6,757 lines** of embedded web assets (HTML templates, CSS, JavaScript including `echarts.min.js`). It is the single largest package in the LiDAR subsystem.

Only two external callers import the package:

| Caller               | What it uses                                                                         |
| -------------------- | ------------------------------------------------------------------------------------ |
| `cmd/radar/radar.go` | `WebServer`, `WebServerConfig`, `NewWebServer`, `NewPacketStats`, `NewDirectBackend` |
| `cmd/sweep/main.go`  | `Client`, `NewClient`, `NewClientBackend`, `BackgroundParams`, `TrackingParams`      |

This means the blast radius of any refactor is narrow at the import boundary — there are only two consumers to update — but wide internally because `WebServer` is a 1,854-line god struct that directly wires together nearly every LiDAR concern.

### Is Full Deprecation Possible?

**Yes, but it requires replacing `monitor/` with three to four focused packages, not deleting it outright.** The package currently conflates four distinct roles:

1. **HTTP server infrastructure** — route registration, middleware, request/response helpers, data source lifecycle
2. **L8-backed application handlers** — REST APIs that should delegate to `l8analytics/` for business logic
3. **L9 endpoints** — chart shaping, ECharts rendering, HTML dashboards, debug overlays, grid plotting
4. **Client SDK** — an HTTP client and in-process backend for sweep tooling

Each role has a natural home. No code needs to be discarded — it all needs to be re-homed.

### File-by-File Ownership Map

#### Role 1 — Infrastructure / Application Wiring

These files own HTTP lifecycle, route registration, data source management, and process control. They form the residual "server" package after extraction.

| File                     | Lines | Responsibility                                                                                                  | Target package                          |
| ------------------------ | ----- | --------------------------------------------------------------------------------------------------------------- | --------------------------------------- |
| `webserver.go`           | 1,854 | god struct: route table, config, state management, status handlers, tuning param GET/POST, health, JSON helpers | split — see breakdown below             |
| `datasource.go`          | 395   | `DataSource` enum, `ReplayConfig`, `DataSourceManager` interface, `RealDataSourceManager`, mock                 | `internal/lidar/server/`                |
| `datasource_handlers.go` | 687   | live UDP start/stop, PCAP replay goroutine management, state reset, `StartPCAPForSweep`                         | `internal/lidar/server/`                |
| `playback_handlers.go`   | 586   | VRLOG/PCAP playback control: pause, play, seek, rate, load/stop                                                 | `internal/lidar/server/`                |
| `mock_background.go`     | 163   | `BackgroundManagerProvider` interface abstraction, mock for testing                                             | `internal/lidar/server/`                |
| `stats.go`               | 155   | `PacketStats`, `StatsSnapshot`, thread-safe counters, `FormatWithCommas`                                        | `internal/lidar/server/` or `pipeline/` |

**webserver.go decomposition** — this is the hardest file. It must be split into:

- **Route table and server lifecycle** (~400 lines) → `internal/lidar/server/server.go`
- **Status/health handlers** (~150 lines) → `internal/lidar/server/status.go`
- **Tuning parameter handlers** (~300 lines) → `internal/lidar/server/tuning.go`
- **State management and reset** (~200 lines) → `internal/lidar/server/state.go`
- **JSON response helpers** (`writeJSON`, `writeJSONError`) (~50 lines) → shared `httputil/` or `internal/lidar/server/`
- **Configuration struct** (`WebServerConfig`, `ParamDef`, `PlaybackStatusInfo`) → `internal/lidar/server/config.go`
- **Remaining handler-glue** (~750 lines of `setupRoutes`, CORS, middleware, form parsing) → `internal/lidar/server/routes.go`

#### Role 2 — L8-Backed Application Handlers (REST API Layer)

These files expose REST endpoints whose business logic should delegate to `l8analytics/`. After Phase 3, they contain only request parsing, authorisation, response serialisation, and `l8analytics` calls.

| File                 | Lines | Responsibility                                                                                                | Target package                                                                           |
| -------------------- | ----- | ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `track_api.go`       | 1,071 | track listing, single-track detail, observation overlays, cluster listing, **summary statistics computation** | `internal/lidar/server/` (handlers) + `l8analytics/` (summary logic)                     |
| `run_track_api.go`   | 793   | analysis run CRUD, track labelling, evaluation orchestration, missed region reporting, **labelling progress** | `internal/lidar/server/` (handlers) + `l8analytics/` (evaluation & labelling aggregates) |
| `scene_api.go`       | 460   | scene CRUD, scene replay, scene evaluation creation, **evaluation orchestration**                             | `internal/lidar/server/` (CRUD handlers) + `l8analytics/` (evaluation service)           |
| `sweep_handlers.go`  | 497   | sweep/auto-tune/HINT lifecycle handlers                                                                       | `internal/lidar/server/` (thin transport only)                                           |
| `export_handlers.go` | 391   | snapshot/frame/foreground ASCII export, snapshot listing/cleanup                                              | `internal/lidar/server/` (transport only)                                                |
| `pcap_files_api.go`  | 109   | list available PCAP files with scene-usage flags                                                              | `internal/lidar/server/` (transport only)                                                |

**Key extraction work in these files:**

- `track_api.go` embeds `TrackSummaryResponse`, `ClassSummary`, `OverallSummary` computation inline. The aggregate statistics (speed percentiles, class-level counts, overall summary) are L8 analytics. The response types and JSON serialisation are transport.
- `run_track_api.go` computes `handleLabellingProgress` and `handleEvaluateRun` inline. Labelling-progress aggregation and evaluation orchestration are L8. The HTTP handler shell remains.
- `scene_api.go` mixes scene CRUD (infrastructure — stays) with `handleCreateSceneEvaluation` (L8 orchestration — extract).

#### Role 3 — L9 Endpoints

These files produce visualisation payloads, render HTML dashboards, generate chart data, and drive debug overlays. They belong in `internal/lidar/l9endpoints/`.

| File                  | Lines  | Responsibility                                                                                         | Target                       |
| --------------------- | ------ | ------------------------------------------------------------------------------------------------------ | ---------------------------- |
| `chart_api.go`        | 375    | HTTP handlers returning JSON for polar, heatmap, foreground, cluster, traffic charts; `Prepare*` funcs | `l9endpoints/chart_api.go`   |
| `chart_data.go`       | 231    | `PolarChartData`, `HeatmapChartData`, `ClustersChartData`, `TrafficMetrics`; coordinate transforms     | `l9endpoints/chart_data.go`  |
| `echarts_handlers.go` | 580    | go-echarts rendering: polar, heatmap, foreground, cluster, track, region charts; debug dashboard HTML  | `l9endpoints/echarts.go`     |
| `gridplotter.go`      | 632    | `GridPlotter`: time-series grid cell sampling during PCAP replay, PNG plot generation                  | `l9endpoints/gridplotter.go` |
| `templates.go`        | 195    | `TemplateProvider`, `AssetProvider` interfaces; `EmbeddedTemplateProvider`, mock implementations       | `l9endpoints/templates.go`   |
| `html/` directory     | ~400   | dashboard.html, regions_dashboard.html, status.html, sweep_dashboard.html                              | `l9endpoints/html/`          |
| `assets/` directory   | ~6,350 | CSS, JS, echarts.min.js                                                                                | `l9endpoints/assets/`        |

#### Role 4 — Client SDK

These files provide an HTTP client and in-process adapter for the sweep/auto-tune tooling. They are consumed by `cmd/sweep/main.go` and `cmd/radar/radar.go`.

| File                | Lines | Responsibility                                                                             | Target package                                                   |
| ------------------- | ----- | ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------- |
| `client.go`         | 554   | `Client`: HTTP client wrapping all monitor endpoints; `BackgroundParams`, `TrackingParams` | `internal/lidar/client/` or `internal/lidar/sweep/client.go`     |
| `direct_backend.go` | 426   | `DirectBackend`: in-process `sweep.SweepBackend` implementation avoiding HTTP round-trips  | `internal/lidar/sweep/direct.go` or stays co-located with server |

**Decision point:** `Client` and `DirectBackend` both implement `sweep.SweepBackend`. They could live in `internal/lidar/sweep/` alongside the existing sweep package, or in a new `internal/lidar/client/` package. The key constraint is that `DirectBackend` holds a pointer to `WebServer`, so it must be in the same package as the server or accept an interface.

### Proposed Target Package Structure

After full deprecation of `monitor/`, the code redistributes into:

```
internal/lidar/
├── server/                    # NEW — HTTP application server (replaces monitor/)
│   ├── server.go              # Server struct, lifecycle, route table
│   ├── config.go              # WebServerConfig, ParamDef, PlaybackStatusInfo
│   ├── routes.go              # setupRoutes, middleware, CORS
│   ├── state.go               # resetAllState, state management
│   ├── status.go              # health, status handlers
│   ├── tuning.go              # tuning parameter GET/POST
│   ├── datasource.go          # DataSource enum, DataSourceManager, RealDataSourceManager
│   ├── datasource_handlers.go # live/PCAP lifecycle handlers
│   ├── playback.go            # VRLOG/PCAP playback control handlers
│   ├── export.go              # snapshot/frame export handlers
│   ├── pcap_files.go          # PCAP file listing
│   ├── sweep.go               # sweep/auto-tune/HINT handlers
│   ├── tracks.go              # track listing/detail handlers (delegates to l8analytics)
│   ├── runs.go                # analysis run CRUD/labelling handlers (delegates to l8analytics)
│   ├── scenes.go              # scene CRUD handlers (delegates to l8analytics for evaluation)
│   ├── stats.go               # PacketStats, StatsSnapshot
│   ├── background.go          # BackgroundManagerProvider interface, mock
│   ├── client.go              # HTTP client SDK
│   └── direct_backend.go      # in-process sweep backend
├── l8analytics/               # NEW — canonical analytics (Phases 2–3)
│   ├── doc.go
│   ├── types.go
│   ├── percentiles.go
│   ├── summary.go
│   ├── comparison.go
│   └── labels.go
├── l9endpoints/            # RENAMED from visualiser/ + absorbed chart/dashboard code (Phase 4)
│   ├── adapter.go             # existing — pipeline → proto frame conversion
│   ├── frame_codec.go         # existing — proto encoding
│   ├── grpc_server.go         # existing — gRPC streaming
│   ├── model.go               # existing — server-side visualiser data model
│   ├── publisher.go           # existing — stream publishing
│   ├── recorder/              # existing — stream recording
│   ├── replay.go              # existing — PCAP/VRLOG replay
│   ├── config.go              # existing — L9 config
│   ├── lidarview_adapter.go   # existing — LidarView export
│   ├── pb/                    # existing — compiled proto bindings
│   ├── chart_api.go           # FROM monitor/ — chart JSON endpoints
│   ├── chart_data.go          # FROM monitor/ — chart data transforms
│   ├── echarts.go             # FROM monitor/ — go-echarts rendering
│   ├── gridplotter.go         # FROM monitor/ — grid time-series plotting
│   ├── templates.go           # FROM monitor/ — template/asset providers
│   ├── html/                  # FROM monitor/ — dashboard templates
│   └── assets/                # FROM monitor/ — CSS, JS, echarts.min.js
```

### Blocking Dependencies and Circular-Import Risks

The main risk in full deprecation is that `WebServer` is a god struct that every handler file attaches methods to. Splitting it requires:

1. **Defining a `Server` struct in `internal/lidar/server/`** that owns the same fields (tracker, classifier, grid manager, DB, stats, data source manager, sweep runners, etc.)
2. **Handler files in `server/` attach methods to `Server`** — this is a mechanical rename from `(ws *WebServer)` to `(s *Server)`.
3. **`l9endpoints/` chart handlers can no longer be methods on `Server`** because they live in a different package. They must either:
   - Accept dependencies via function parameters (functional handlers registered in the route table)
   - Accept a small interface that `Server` satisfies (e.g., `ChartDataProvider` with methods like `GetBackgroundManager()`, `GetTracker()`)
   - Be registered as closures that capture the required dependencies during `setupRoutes()`

   The interface approach is cleanest — `l9endpoints/` defines the interface it needs, and `server/` satisfies it.

4. **`DirectBackend` holds a `*WebServer` pointer.** After the rename to `Server`, it must hold `*Server` instead. If `DirectBackend` stays in `server/`, no import cycle. If it moves to `sweep/`, it needs an interface.

5. **`Client` has no import risk** — it only uses `net/http` and `sweep.SweepBackend`. It can live anywhere.

### webserver.go Decomposition: The Critical Path

`webserver.go` at 1,854 lines is the single hardest file. It contains:

| Responsibility                             | Approx. lines | Target                            |
| ------------------------------------------ | ------------- | --------------------------------- |
| `WebServer` struct definition and fields   | ~80           | `server/server.go`                |
| `WebServerConfig`, `ParamDef`              | ~40           | `server/config.go`                |
| `NewWebServer` constructor                 | ~100          | `server/server.go`                |
| `setupRoutes` (full route table)           | ~180          | `server/routes.go`                |
| CORS middleware                            | ~30           | `server/routes.go`                |
| `handleStatus`, `handleHealth`             | ~80           | `server/status.go`                |
| `handleTuningParams` (GET/POST)            | ~300          | `server/tuning.go`                |
| `resetAllState`, `resetBackgroundGrid`     | ~120          | `server/state.go`                 |
| `writeJSON`, `writeJSONError`, helpers     | ~60           | `server/server.go` or `httputil/` |
| `Start`, `RegisterRoutes`                  | ~50           | `server/server.go`                |
| form parsing, sensor resolution            | ~100          | `server/routes.go`                |
| `SetTracker`, `SetClassifier`, `Set*`      | ~60           | `server/server.go`                |
| foreground count tracking                  | ~40           | `server/state.go`                 |
| `PlaybackStatusInfo`, playback status JSON | ~80           | `server/config.go`                |
| remaining handler-glue and misc            | ~434          | distributed across target files   |

### Sequencing Full Deprecation

Full `monitor/` deprecation spans **Phases 4–7** of the broader ten-layer plan (Phases 1–3 do not touch monitor's package boundary, only extract logic from its handlers):

#### Phase 4.5: Extract L9 Endpoints from monitor/ (prerequisite: Phase 4 rename complete)

1. Define a `ChartDataProvider` interface in `l9endpoints/` with the methods the chart handlers need (grid snapshots, tracker state, stats, sensor ID)
2. Move `chart_api.go`, `chart_data.go`, `echarts_handlers.go`, `templates.go`, `html/`, `assets/` into `l9endpoints/` (keep `gridplotter.go` in `monitor/` per Phase 4 deferred moves; migrate it in the Phase 4 follow-up)
3. Convert chart handler methods from `(ws *WebServer)` receivers to standalone functions or methods on a local struct that accepts `ChartDataProvider`
4. Update `webserver.go`'s route table to register L9 handlers via the interface
5. Move corresponding tests; verify all chart endpoints still pass

   **Estimated scope:** ~2,013 lines of production code, ~2,500+ lines of tests, plus 6,757 lines of embedded assets.

#### Phase 5.5: Create server/ Package and Migrate Infrastructure

1. Create `internal/lidar/server/` with `Server` struct mirroring `WebServer`'s fields
2. Move infrastructure files first: `datasource.go`, `datasource_handlers.go`, `stats.go`, `mock_background.go`, `playback_handlers.go`
3. Split `webserver.go` into `server.go`, `config.go`, `routes.go`, `state.go`, `status.go`, `tuning.go`
4. Rename `WebServer` → `Server`, `WebServerConfig` → `Config`, `NewWebServer` → `New`
5. Move `client.go` and `direct_backend.go` into `server/` (or a separate `client/` package)
6. Update `cmd/radar/radar.go` and `cmd/sweep/main.go` to import `server` instead of `monitor`
7. Move corresponding tests

   **Estimated scope:** ~4,295 lines of production code (datasource.go 395, datasource_handlers.go 687, stats.go 155, mock_background.go 163, playback_handlers.go 586, webserver.go 1,854, client.go 554, direct_backend.go 426 — minus L9 code already moved).

#### Phase 6.5: Migrate Remaining Handlers to server/

1. Move `track_api.go`, `run_track_api.go`, `scene_api.go`, `sweep_handlers.go`, `export_handlers.go`, `pcap_files_api.go` into `server/`
2. Verify all handler methods now call `l8analytics/` for business logic (prerequisite: Phase 3 complete)
3. Move corresponding tests
4. Move `testdata/` directory

   **Estimated scope:** ~3,321 lines of production code, ~15,000+ lines of tests.

#### Phase 7: Delete monitor/ and Remove Type Aliases

1. Verify no imports of `internal/lidar/monitor` remain
2. Delete `internal/lidar/monitor/` directory
3. If any external tools or scripts reference the old path, add a one-line `doc.go` with a deprecation notice pointing to `server/` and `l9endpoints/`
4. Update `ARCHITECTURE.md`, `docs/lidar/README.md`, and any docs referencing `monitor/`

### Risk Assessment for Full Deprecation

| Risk                                                                    | Severity | Mitigation                                                                                                                    |
| ----------------------------------------------------------------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `WebServer` god struct makes splitting error-prone                      | High     | split incrementally: infrastructure first, then handlers one-by-one; keep tests green after each move                         |
| chart handlers need access to `Server` internals                        | Medium   | define small interfaces (`ChartDataProvider`) rather than passing `*Server` across package boundaries                         |
| 24,682 lines of tests are tightly coupled to `WebServer`                | High     | move tests alongside their production code; use search-and-replace for receiver renames; run full test suite after each phase |
| `DirectBackend` holds `*WebServer` pointer                              | Low      | keep `DirectBackend` in `server/` package; rename receiver to `*Server`                                                       |
| two external callers must update imports                                | Low      | mechanical change; only `cmd/radar/radar.go` and `cmd/sweep/main.go`                                                          |
| embedded assets require `embed` directives to move                      | Medium   | `//go:embed` paths are relative to the file; move `html/` and `assets/` alongside the Go files that embed them                |
| route table in `setupRoutes` references handlers from multiple packages | Medium   | register L9 handlers via closure or interface adapter in `routes.go`; all other handlers remain methods on `Server`           |

### Decision: Rename to `server/` or Keep `monitor/`?

**Recommendation: rename to `internal/lidar/server/`.** Reasons:

- "monitor" was chosen when the package was purely an operator-facing debug dashboard. It now owns data source lifecycle, PCAP replay, sweep orchestration, track APIs, scene management, and the full REST surface. "monitor" no longer describes its responsibility.
- "server" is accurate: it is the HTTP server application layer for the LiDAR subsystem.
- The name `server` also makes the dependency direction obvious: `server/` imports `l8analytics/` and `l9endpoints/`, never the reverse.
- The rename happens naturally during the Phase 5.5 migration — no extra churn.

**Alternative considered: keep `monitor/` but narrow it.** This avoids the rename churn, but leaves a misleading package name and requires explaining to every new contributor why the HTTP server is called "monitor". The rename cost is paid once; the confusion cost is paid forever.

### Effort Estimate and Phasing Summary

| Phase     | What moves                          | Production lines | Test lines (est.) | Prerequisites    |
| --------- | ----------------------------------- | ---------------- | ----------------- | ---------------- |
| Phase 4.5 | L9 endpoints code → `l9endpoints/`  | ~2,013           | ~2,500            | Phase 4 (rename) |
| Phase 5.5 | infrastructure + client → `server/` | ~4,295           | ~10,000           | Phase 4.5        |
| Phase 6.5 | remaining handlers → `server/`      | ~3,321           | ~12,000           | Phases 3 + 5.5   |
| Phase 7   | delete `monitor/`, update docs      | 0 (deletion)     | 0                 | Phase 6.5        |
| **Total** |                                     | **~10,154**      | **~24,682**       |                  |

Full deprecation is achievable but must be sequenced after the L8 and L9 boundaries are established (Phases 2–4). Attempting it before L8 exists would simply move the misplaced analytics logic from `monitor/` into `server/` — same problem, different directory name.

## Non-Goals

- full rewrite of the LiDAR subsystem
- broad mechanical renaming with no ownership improvement
- major redesign of the web app or macOS visualiser unrelated to the layer split
- refactoring the radar-focused PDF generator as a prerequisite for the LiDAR layer split

## Complete Checklist

### Docs and architecture

- [ ] `docs/lidar/architecture/lidar-data-layer-model.md` updated to the ten-layer model
- [ ] `docs/lidar/architecture/README.md` updated to describe `L1` through `L10`
- [ ] `docs/lidar/README.md`, `docs/data/DATA_STRUCTURES.md`, and `docs/lidar/terminology.md` updated
- [ ] relevant package doc comments under `internal/lidar/` updated
- [ ] breaking-change rationale documented
- [ ] migration note documented

### L8 analytics boundary

- [ ] `internal/lidar/l8analytics/` exists with package docs
- [ ] `RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge`, and temporal IoU logic moved out of `L6`
- [ ] speed percentile helpers no longer live only in storage code
- [ ] run-level summary/statistics logic split out of `l6objects/quality.go`
- [ ] handler summary logic delegates to `L8`
- [ ] comparison logic delegates to `L8`
- [ ] new or moved `L8` code has direct unit tests

### L9 endpoints boundary

- [ ] `internal/lidar/visualiser/` renamed to `internal/lidar/l9endpoints/`; import paths in `cmd/radar/radar.go` and `cmd/tools/visualiser-server/main.go` updated
- [ ] server-side chart/view-model shaping has an explicit `L9` home in `l9endpoints/`
- [ ] `chart_data.go`-style endpoint helpers are no longer in `monitor/`
- [ ] clients do not compute canonical summary metrics locally
- [ ] debug and dashboard payload shaping is explicitly classified as `L9`

### monitor/ decomposition and full deprecation

- [ ] each `monitor/` file is classified as infra/application, `L8`-backed API, or `L9` endpoint (see file-by-file ownership map)
- [ ] mixed handlers call extracted services instead of embedding analytics math
- [ ] deferred moves are documented with explicit destinations
- [ ] no new upward dependency violations are introduced

#### Phase 4.5 — L9 endpoints extraction

- [ ] `ChartDataProvider` interface defined in `l9endpoints/`
- [ ] `chart_api.go`, `chart_data.go`, `echarts_handlers.go`, `gridplotter.go`, `templates.go` moved to `l9endpoints/`
- [ ] `html/` and `assets/` directories moved to `l9endpoints/`; `//go:embed` directives updated
- [ ] chart handler methods converted from `(ws *WebServer)` receivers to interface-backed handlers
- [ ] route table in `webserver.go` registers L9 handlers via interface adapter
- [ ] all chart endpoint tests pass from new location

#### Phase 5.5 — server/ package creation and infrastructure migration

- [ ] `internal/lidar/server/` package created with `Server` struct
- [ ] `WebServer` renamed to `Server`; `WebServerConfig` renamed to `Config`
- [ ] `webserver.go` split into `server.go`, `config.go`, `routes.go`, `state.go`, `status.go`, `tuning.go`
- [ ] `datasource.go`, `datasource_handlers.go`, `playback_handlers.go`, `stats.go`, `mock_background.go` moved to `server/`
- [ ] `client.go` and `direct_backend.go` moved to `server/` (or `client/` if interface extraction preferred)
- [ ] `cmd/radar/radar.go` and `cmd/sweep/main.go` updated to import `server` instead of `monitor`

#### Phase 6.5 — remaining handler migration

- [ ] `track_api.go`, `run_track_api.go`, `scene_api.go`, `sweep_handlers.go`, `export_handlers.go`, `pcap_files_api.go` moved to `server/`
- [ ] all handler methods confirmed to delegate analytics to `l8analytics/`
- [ ] `testdata/` directory moved to `server/testdata/`
- [ ] all tests pass from new locations

#### Phase 7 — delete monitor/

- [ ] no imports of `internal/lidar/monitor` remain in the repository
- [ ] `internal/lidar/monitor/` directory deleted
- [ ] `ARCHITECTURE.md`, `docs/lidar/README.md`, and all docs updated to reference `server/` and `l9endpoints/`

### Generated artifacts and verification

- [ ] DOT graph added
- [ ] SVG generated and checked in
- [ ] graph generation is reproducible via script
- [ ] tests updated for moved analytics and changed handlers
- [ ] verification or CI guardrail exists for generated artifacts
- [ ] final checked-in plan is sufficient to drive follow-on implementation PRs
