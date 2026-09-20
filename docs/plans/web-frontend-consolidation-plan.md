# Surface consolidation plan

- **Status:** Draft. Revised 2026-09-19 after a route-by-route survey; the phase
  estimates and several factual claims in the previous revision were wrong, and
  the corrections are recorded in [What the 2026-09-19 survey changed](#what-the-2026-09-19-survey-changed).
- **Canonical:** [web-frontend-consolidation.md](../ui/web-frontend-consolidation.md)
- **Layers:** L9 Endpoints, L10 Clients
- **Scope note:** the title says _surface_ rather than _frontend_ because the
  survey found the load-bearing duplication is not between two UIs' screens. It
  is that the web's data never passes through the canonical frame model, so the
  scene format is produced twice in two languages. Deleting the legacy HTML is
  the small half of this document; [One scene lineage](#one-scene-lineage) is the
  large half.

## Problem statement

The project has three distinct web surfaces for LiDAR functionality:

1. **Svelte web app** (`/app/*`, port 8080): radar dashboard, reports, sites, settings, plus LiDAR tracks/replay-cases/runs/captures/sweeps/scene-map
2. **Go-embedded HTML dashboards** (port 8081, and on 8080 too, see below): LiDAR status, debug dashboard, parameter sweep/auto-tune, background regions
3. **macOS Metal visualiser** (gRPC on port 50051, plus eleven HTTP calls to `/api/lidar/*` on 8080): live 3D point cloud rendering, track labelling, replay

The Svelte app was originally conceived as radar-only, with LiDAR interfaces living on port 8081 and the Mac app. Over time, LiDAR tracks, scenes, and runs were added to the Svelte app, creating a mixed-concern frontend. PR #547 now hides sidebar LiDAR navigation in radar-only deployments, but direct-route disabled states and the split LiDAR tooling surfaces still need follow-through.

## Surface ownership

[docs/ui/DESIGN.md §2.1](../ui/DESIGN.md) already names four canonical surfaces
and already marks the legacy dashboards "migration target, not style baseline".
This plan does not add a surface model; it adds the allocation rule that was
missing, so a new feature has one obvious home.

**Fidelity-bound interaction goes to macOS. Workflow and publishing go to the
Svelte app. Numbers go headless. Public pages stay static.**

| Concern                                                                                 | Home                            | Why                                                                                              |
| --------------------------------------------------------------------------------------- | ------------------------------- | ------------------------------------------------------------------------------------------------ |
| Point-cloud interaction, per-point annotation, pose authoring, frame stepping           | **macOS**                       | Metal, full fidelity, ~70k points per frame with debug overlays                                  |
| Runs, replay cases, captures, sites, labelling review, sweep control, status, recording | **Svelte, 8080**                | Operator workflow, browser-reachable, no fidelity requirement                                    |
| Overall scene view (recorded), scene catalogue, publishing                              | **Svelte, 8080**                | Consumes the decimated artefact the public page already uses                                     |
| Public scene pages                                                                      | **`public_html/`, static**      | Shipped: 26 scenes, gzipped NDJSON and three.js, on GitHub Pages                                 |
| Aggregate metrics, comparators, evaluation tables                                       | **Headless: JSON + server SVG** | [State-estimation plan §18.3](lidar-state-estimation-plan.md); keeps Swift off the critical path |
| Legacy 8081 HTML                                                                        | **Deleted**                     | —                                                                                                |

Two consequences worth stating, because they resolve questions that keep
recurring. Track labelling exists in full in both the Svelte app and the macOS
side panel, against the same endpoint; `docs/lidar/operations/track-labelling-ui-implementation.md`
already chose Swift-native and deferred web parity, so the web labeller should be
demoted to read-only review rather than kept in step. And evaluation output is
the one item on the list with _no_ surface at all: `GET/POST /api/lidar/scenes/{id}/evaluations`
has a backend and zero consumers, and `analysis.CompareReports` has never had a
UI. Everything else has too many.

## Options evaluated

### ~~Option a: two separate Svelte apps (radar app + LiDAR app)~~

Ship two independent SvelteKit applications, each embedded in the binary. The radar app serves on 8080, the LiDAR app on 8081 (replacing the Go-embedded HTML).

**Architecture:**

- `web/radar/`: radar-only SvelteKit app (dashboard, sites, reports, settings)
- `web/lidar/`: LiDAR-only SvelteKit app (tracks, scenes, runs, sweep, regions, status)
- Two `embed.FS` directives in Go; two static builds

**Advantages:**

- Clean separation: radar binary embeds only `web/radar/build/`
- LiDAR app can evolve independently with specialised dependencies
- No conditional rendering needed: each app is self-contained
- Could theoretically use different charting libraries per app

**Disadvantages:**

- **Duplicated infrastructure**: two SvelteKit configs, two package.jsons, two build pipelines, two sets of shared utilities (units, timezone, date formatting, API client, svelte-ux theme)
- **Two ports in production**: users must know both addresses; no unified navigation between radar and LiDAR
- **Larger combined binary**: two full SvelteKit bundles with duplicated framework code
- **Shared component drift**: MapEditorInteractive, DataSourceSelector, stores, and utility libraries must be maintained in sync or extracted to a shared package
- **Build complexity**: Makefile needs `build-web-radar`, `build-web-lidar`, `ensure-web-stub` for both
- **Migration cost**: must split existing `web/` into two projects, re-test both, update all embed paths

### Option b: one Svelte app with conditional LiDAR sections

Keep a single SvelteKit application. LiDAR routes remain in the app but are conditionally shown in navigation based on a runtime capability check (API call to determine if `--enable-lidar` is active).

**Architecture:**

- `web/`: single SvelteKit app (unchanged structure)
- LiDAR routes at `/app/lidar/*` (existing) plus new sweep/regions/status routes
- Navigation sidebar queries `/api/capabilities` to determine sensor availability
- LiDAR nav items hidden when LiDAR is disabled

**Advantages:**

- **Minimal structural change**: no project splitting; existing routes, components, and utilities stay put
- **Single build pipeline**: one `make build-web`, one embed, one bundle
- **Unified UX**: one port, one navigation tree, seamless sensor switching
- **Shared utilities naturally**: stores, API client, date/unit helpers, svelte-ux theme; all shared without duplication
- **Smaller binary**: single SvelteKit bundle (currently ~220KB gzipped for `everything.js`)
- **Progressive migration**: Go-embedded dashboards can be migrated one at a time into new Svelte routes

**Disadvantages:**

- Radar-only binary still ships LiDAR JavaScript (dead code in the bundle)
- Requires a capabilities API and conditional navigation logic
- Requires runtime capability refresh and backend lifecycle management for hot-enable/disable; #547 ships the refresh/store path, while full backend ready/error lifecycle wiring remains follow-up
- LiDAR routes must return an explicit "LiDAR disabled" response and must not initialise hardware when disabled
- Single [package.json](../../package.json) may accumulate LiDAR-specific dependencies over time

### ~~Option c: one Svelte app with build-time LiDAR exclusion~~

Like Option B, but use SvelteKit's build configuration or a Vite plugin to strip LiDAR routes at build time, producing two variants of the static output.

**Architecture:**

- `web/`: single source tree
- Build flag: `INCLUDE_LIDAR=true make build-web` controls which routes are included
- Vite plugin or SvelteKit hooks exclude `/lidar/*` routes and components when flag is off
- Two embed targets: `web/build-radar/` and `web/build-full/`

**Advantages:**

- Single source tree with no duplication
- Radar-only binary has zero LiDAR code
- Clean separation at build time without runtime checks

**Disadvantages:**

- **Significant build complexity**: custom Vite plugins, conditional route inclusion, two build outputs
- **Fragile**: SvelteKit's static adapter doesn't natively support conditional route exclusion; would need custom tooling
- **Testing burden**: must test both build variants
- **Marginal benefit**: LiDAR JavaScript is ~50KB in the bundle; the savings don't justify the complexity
- **Two embeds**: same binary-size concern as Option A if both variants are embedded

## Decision matrix

| Criterion                  | Weight | Option A (Two Apps)       | Option B (One App, Conditional) | Option C (One App, Build-Time) |
| -------------------------- | ------ | ------------------------- | ------------------------------- | ------------------------------ |
| **Level of Effort**        | High   | 🔴 High                   | 🟢 Low                          | 🟡 Medium                      |
| **Migration risk**         | High   | 🔴 High (split + rebuild) | 🟢 Low (incremental)            | 🟡 Medium (custom tooling)     |
| **Code complexity**        | High   | 🔴 High (duplication)     | 🟢 Low (single codebase)        | 🟡 Medium (build plugins)      |
| **Usability**              | High   | 🟡 Two ports, two UIs     | 🟢 Single unified UI            | 🟢 Single unified UI           |
| **Radar-only cleanliness** | Medium | 🟢 Perfect separation     | 🟡 Hidden nav, dead routes      | 🟢 No dead code                |
| **Binary size**            | Low    | 🔴 Two bundles            | 🟢 One bundle                   | 🟡 One of two bundles          |
| **Maintenance burden**     | High   | 🔴 Two of everything      | 🟢 One of everything            | 🟡 Build tooling to maintain   |
| **Build simplicity**       | Medium | 🔴 Two pipelines          | 🟢 One pipeline                 | 🟡 Two outputs from one        |

### Scoring (5 = best, 1 = worst, weighted)

| Criterion              | Weight | A      | B      | C      |
| ---------------------- | ------ | ------ | ------ | ------ |
| Level of effort        | 3      | 1      | 5      | 3      |
| Migration risk         | 3      | 1      | 5      | 3      |
| Code complexity        | 3      | 1      | 5      | 3      |
| Usability              | 3      | 2      | 5      | 5      |
| Radar-only cleanliness | 2      | 5      | 3      | 5      |
| Binary size            | 1      | 1      | 4      | 3      |
| Maintenance burden     | 3      | 1      | 5      | 3      |
| Build simplicity       | 2      | 1      | 5      | 3      |
| **Weighted Total**     |        | **32** | **95** | **67** |

## Recommendation: option b; one Svelte app with conditional LiDAR sections ✅

Option B is the clear winner. The single-app approach avoids duplication, keeps the build simple, and provides the best user experience. The minor downside: shipping ~50KB of unused LiDAR JavaScript in radar-only deploys; is negligible compared to the maintenance cost of two separate applications or custom build tooling.

The sidebar concern is mitigated by #547's capability-gated navigation. The
remaining dead-route concern is server-side and route-level gating: `/api/lidar/*`
must return a clear "LiDAR disabled" response without initialising hardware when
LiDAR is off, and direct URL access to `/app/lidar/*` should show a friendly
disabled state. Full hot-enable/disable truth also still needs backend lifecycle
callbacks for ready/error transitions.

## What the 2026-09-19 survey changed

A route-by-route survey of every handler, its consumers across the Svelte app,
the Swift client, Python tooling and docs, found the previous revision wrong in
ways that change the sequencing rather than just the wording.

**Retiring port 8081 is nearly a no-op, and Phase 5's first step is already
done.** `internal/lidar/server/routes.go` `RegisterRoutes(mux)` is already
mounted on the 8080 mux (`internal/cmd/server/radar.go:1106`). The 8081 listener
registers the same set plus exactly two extras: the status page at `/`
(`routes.go:230`) and pprof (`routes.go:236`, also on 8080 via
`AttachAdminRoutes`). Retirement is deleting `setupRoutes`, the `http.Server` at
`server.go:379`, the `--lidar-listen` flag (`radar.go:153`) and the `/api/lidar`
proxy line in `web/vite.config.ts:42`. About a day, not 3–5.

**The port and the HTML are separate problems.** Because every route is already
on 8080, `/debug/lidar/*` and the legacy pages are reachable there too. "Retiring
8081" does not remove the legacy surface; deleting the handlers does.

**Two blockers, one of which was not in the plan.** `sweep_dashboard.js` is
**3,981 lines**, not the 2,390 budgeted, and the hard part is not the eight
charts the phase headlines: it is `PARAM_SCHEMA` (26 parameters with types,
bounds and step rules), the dynamic row builder, and three divergent start
payloads. It is the only place a manual sweep, auto-tune or HINT run can be
started or configured. Separately, `status.html:318` is the only UI that can
start a **configured PCAP replay with recording**, which the plan does not list
as a blocker at all. `tools/s2-archive/publish-scenes.py` does the same over the
API, so scene publishing is unaffected, but the manual record-a-VRLOG workflow
dies with the page.

**Line counts were understated by up to 66 %**: `status.html` 570 not 492,
`regions_dashboard` 61 + 340 not 54 + 298, `dashboard.html` 46 not 43.

**Phase 3's scope predates HINT.** It describes "two operational modes"; there
are three, and HINT adds round configuration, a `wait_for_change` long poll and
desktop notifications.

**The macOS visualiser is an HTTP consumer.** The non-goal "the Metal app is
retained as-is" and Phase 5's "validate that the visualiser (gRPC 50051) is
unaffected" are both wrong: it makes eleven HTTP calls to `/api/lidar/*` on
8080, and it calls the **deprecated alias** `/api/lidar/pcap/stop`
(`RunTrackLabelAPIClient.swift:218`). Pin the Swift client to
`/api/lidar/replay/stop` before any alias cleanup, or the app breaks silently.

**Every `internal/lidar/monitor/` path in this plan and its hub is wrong.** The
package is `internal/lidar/server/`; the assets are under
`internal/lidar/l9endpoints/l10clients/{html,assets}/`. The same stale path
appears in `docs/lidar/operations/auto-tuning.md`, `hint-sweep-mode.md`,
`docs/lidar/architecture/lidar-pipeline-reference.md`,
`docs/plans/hint-metric-observability-plan.md`,
`docs/plans/lidar-track-labelling-auto-aware-tuning-plan.md` and
`docs/plans/lidar-architecture-dynamic-algorithm-selection-plan.md`.

## Deletion ledger

Verified consumer-free by grep across `web/src`, Swift, Python and docs. About
2,400 lines of production Go plus tests, and one dependency. This is an
inventory, not an instruction to delete today.

| Delete                                                                        | Lines       | Evidence                                                                            |
| ----------------------------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------- |
| `internal/lidar/server/chart_api.go`, the five `/api/lidar/chart/*` endpoints | 167         | Built as the JSON-first replacement for ECharts; nothing ever consumed them         |
| `internal/lidar/server/echarts_handlers.go`, 9 handlers, 6 go-echarts pages   | 580         | Only reachable from `dashboard.html`'s iframe grid                                  |
| `internal/lidar/l9endpoints/chart_data.go` + `chart_transforms.go` + tests    | 443 + 587   | Zero clients on either end                                                          |
| `internal/lidar/l9endpoints/legacy_assets.go`, assets, `dashboard.html`       | 60 + vendor | Dies with the pages; drops a vendored `echarts.min.js`                              |
| `internal/lidar/l9endpoints/templates.go`                                     | 195         | No non-test consumer; `status.go:24` uses `template.ParseFS` directly               |
| `internal/lidar/adapters/track_export.go` + `training_data.go`                | 425         | Zero production call sites; two functions are stubs returning "not yet implemented" |
| `/api/lidar/export_foreground`, `/settling_eval`, `/traffic`                  | —           | No callers found                                                                    |
| `adapter.go:279 adaptClusters`                                                | —           | Test-only duplicate of `adaptUnassociatedClusters`                                  |
| `github.com/go-echarts/go-echarts/v2` from `go.mod`                           | —           | Falls out with the handlers                                                         |

**Four traps in that list.**

1. Move `StatsSnapshot` (`chart_data.go:13`) before deleting its file:
   `internal/lidar/server/stats.go:12` aliases it and `status.go:99` uses it.
2. `/debug/lidar/background/regions` (`status.go:676`) returns **JSON** from a
   `/debug/` path, is used by `regions_dashboard.js`, and is cited in
   `docs/lidar/operations/adaptive-region-parameters.md`. Rename it to
   `/api/lidar/background/regions`; do not drop the namespace wholesale.
3. `gridplotter.go` (673 lines) is not a UI adapter. It writes SVG ring plots
   during PCAP replay, driven by `datasource_handlers.go:580`. It survives.
4. `POST /api/lidar/playback/{pause,play,seek,rate}` had no Swift call site in
   the survey, but the Metal transport UI must be exercised against a running
   8080 before they are called dead.

## One scene lineage

This is the larger half of the work and the reason the plan is retitled.

`internal/lidar/pipeline/tracking_pipeline.go` writes SQLite observations at
line 961 and calls `VisualiserAdapter.AdaptFrame` at line 1007: two sibling
statements, two lineages, no shared intermediate. `FrameBundle`
(`internal/lidar/l9endpoints/model.go:12`) declares itself canonical and is, for
the visualiser, VRLOG and scene export — **the web path never touches it.**

The consequence is that the scene chunk format is produced twice, in two
languages, for the same renderer: Go `internal/scene/export.go:339 projectFrame`
and TypeScript `web/src/lib/scene/liveSceneSource.ts:40 buildLiveFrames`, both
feeding `public_html/src/js/scene-player.js`.

**Recommendation: serve `projectFrame` over HTTP against recorded runs.** The
Svelte scene view then consumes the identical artefact the public page does, and
`buildLiveFrames` / `toSceneTrack` delete (~130 lines of TypeScript plus tests).
Decimation already exists on both sides: `PointCloudFrame.ApplyDecimation`
(`frame_codec.go:94`, uniform and voxel) and `-max-points` in the exporter.

**Decision taken 2026-09-19: the web renders recorded and published artefacts
only.** No live decimated browser transport. That keeps one decimation path and
one artefact with two consumers; a live feed would be a second adapter to hold
in step with the first, which is the cost this plan exists to remove. The seam
is named so it can be added later without reshaping the artefact.

Two details to carry across: `toSceneTrack` overlays the operator's `user_label`
over the classifier label (`liveSceneSource.ts:46-52`), so the Go producer needs
a label-overlay hook; and it substitutes `heading_rad` because the observations
endpoint carries no per-frame OBB yaw, where the Go producer has the real
`BBoxHeadingRad` (`model.go:241`) — so the move improves fidelity rather than
merely matching it.

## Scene naming

Three things are called a scene, and the collision is in the database, not just
the prose.

| Meaning                   | Surface                                        | Table                                                 |
| ------------------------- | ---------------------------------------------- | ----------------------------------------------------- |
| Published-scene catalogue | `/api/scenes`, `/app/scene`                    | **`lidar_scenes`** (migration 39, created 2026-09-05) |
| Replay case               | `/api/lidar/scenes`, `/app/lidar/replay-cases` | `lidar_replay_cases`                                  |
| L7 world model            | none                                           | none; planned for v1.0                                |

Migration 31 renamed the original `lidar_scenes` to `lidar_replay_cases`, and
migration 39 then re-used the freed name for the publishing catalogue. So
**`lidar_scenes` today means the catalogue, not the replay case**, while the
`/api/lidar/scenes` URL still serves replay cases. The rename is owned by
[lidar-replay-case-terminology-alignment-plan.md](lidar-replay-case-terminology-alignment-plan.md);
this plan's obligation is not to add to the confusion.

**One orphan to decide.** The `/app/scene` catalogue is shipped but unwired: it
is not in the sidebar, and `publish-scenes.py` never calls `/api/scenes` even
though the `asset_path` placeholder shows it was meant to index published
assets. Either wire the publisher to it or drop it. Nothing in the public HTML
path breaks either way, and leaving it guarantees drift.

## Revised phases

Phase 0 is unchanged and mostly complete. The rest is re-ordered so that nothing
is deleted before its replacement exists, and so the cheap, consumer-free
deletions are not held hostage to the expensive rewrite.

| Phase | Scope                                                                             | Revised effort | Depends on                 |
| ----- | --------------------------------------------------------------------------------- | -------------- | -------------------------- |
| 0     | Capabilities API and conditional nav                                              | Mostly done    | —                          |
| 1     | **Deletion ledger**: the consumer-free handlers above, plus the `go-echarts` drop | 1 day          | The four traps             |
| 2     | Retire the 8081 listener and `--lidar-listen`                                     | 1 day          | Phase 1                    |
| 3     | PCAP replay and recording form in Svelte                                          | 3–4 days       | —                          |
| 4     | Regions dashboard in Svelte; rename the `/debug/` JSON route                      | 2–3 days       | —                          |
| 5     | Sweep, auto-tune and HINT control in Svelte                                       | 2–3 weeks      | Form model, not the charts |
| 6     | Delete `status.html` and `sweep_dashboard.*`                                      | 1 day          | Phases 3 and 5             |
| 7     | One scene lineage: serve `projectFrame`, delete `buildLiveFrames`                 | 1 week         | Independent; highest value |

Phase 5's client layer is half-built already: `web/src/lib/api.ts` has
`getHINTState` (:1100), `startHINTSweep` (:1110) and `stopHINT` (:1156), tested
and imported by nothing. A smaller gap worth fixing first: `add_round` and
`next_sweep_duration_mins` are unreachable from 8080 because
`sweeps/+page.svelte:162` calls `continueHINT()` with defaults.

## Regression gate

**Scene publishing must be byte-identical across every phase.** It touches
neither 8081, ECharts nor `/debug/lidar/*` today, and 26 scenes are live on
GitHub Pages. Run `make scene-assets` for one site before and after any change
and compare `manifest.json`, the chunk digests and `background.json.gz`. After
Phase 7, additionally load the same recorded run in the Svelte scene view and in
the public player: they then share one producer, so any divergence is a bug
rather than a rounding difference.

Preserve as JSON APIs even as the HTML goes: `pcapRoutes`
(`POST /api/lidar/pcap/start`, `/pcap/stop`), `playbackRoutes`
(`GET /api/lidar/playback/status`), and the `lidar_run_records` write path.
`publish-scenes.py` and the Makefile preflight hard-code port 8080.

One planned item needs respecifying rather than porting: deterministic scene
capture Milestone 4 (`lidar-deterministic-scene-capture-plan.md:207-223`)
specifies a "Make web export" checkbox _on the 8081 replay form_. Point it at
the Svelte form from Phase 3.

## Where the evaluator fits

The per-frame track evaluator (gap-analysis M5/M1/M2, added 2026-09-19 in
`internal/lidar/l8analytics`) is the worked example of the ownership rule, and
it adds no surface. Its aggregate output is headless: canonical JSON from
`cmd/tools/lidar-track-scorecard`, read by the campaign's analysis script. Its
per-frame disagreement view belongs in the macOS annotation pane, as a follow-on
feature of the existing
[labelling and QC suite](lidar-visualiser-labelling-qc-enhancements-overview-plan.md),
because authoring reference poses is point-cloud work. Nothing lands in Svelte.

It also gives the orphaned `/api/lidar/scenes/{id}/evaluations` endpoint its
first plausible consumer, or the argument for deleting it.

## Non-Goals

- Rebuilding the macOS visualiser. It is a consumer of these APIs and its HTTP
  calls constrain the work, but its UI is out of scope.
- Auth and access control.
- The L7 scene world model.
- Any change to the public scene format or its decimation parameters.
