# Frontend Consolidation Plan

## Status: Draft

**Layers:** L9 Endpoints, L10 Client

## Problem Statement

The project has three distinct web surfaces for LiDAR functionality:

1. **Svelte web app** (`/app/*`, port 8080) — radar dashboard, reports, sites, settings, plus LiDAR tracks/scenes/runs
2. **Go-embedded HTML dashboards** (port 8081) — LiDAR status, debug dashboard, parameter sweep/auto-tune, background regions
3. **macOS Metal visualiser** (gRPC on port 50051) — live 3D point cloud rendering, track labelling, replay

The Svelte app was originally conceived as radar-only, with LiDAR interfaces living on port 8081 and the Mac app. Over time, LiDAR tracks, scenes, and runs were added to the Svelte app, creating a mixed-concern frontend. This makes it difficult to ship a radar-only binary without non-functioning LiDAR navigation items, and scatters LiDAR tooling across three surfaces.

### Current State Diagram

```
┌─────────────────────────────────────────────────────────┐
│                    Go Binary                            │
│                                                         │
│  ┌──────────────────────┐  ┌─────────────────────────┐  │
│  │  Port 8080 (Radar)   │  │  Port 8081 (LiDAR)      │  │
│  │                      │  │                         │  │
│  │  Embedded Svelte SPA │  │  Go-template HTML pages │  │
│  │  ├─ Dashboard        │  │  ├─ Status/Config       │  │
│  │  ├─ Sites            │  │  ├─ Debug Dashboard     │  │
│  │  ├─ Reports          │  │  ├─ Sweep/Auto-Tune     │  │
│  │  ├─ Settings         │  │  ├─ Background Regions  │  │
│  │  ├─ LiDAR Tracks   ←─┼──┼──┤  (iframe charts)     │  │
│  │  ├─ LiDAR Scenes     │  │  └─ ECharts assets      │  │
│  │  └─ LiDAR Runs       │  │                         │  │
│  └──────────────────────┘  └─────────────────────────┘  │
│                                                         │
│  ┌──────────────────────┐                               │
│  │  Port 50051 (gRPC)   │                               │
│  │  Frame streaming     │◄──── macOS Metal Visualiser   │
│  └──────────────────────┘                               │
└─────────────────────────────────────────────────────────┘
```

### Pain Points

| Problem                                       | Impact                                                   |
| --------------------------------------------- | -------------------------------------------------------- |
| LiDAR nav items visible in radar-only deploys | Confusing UX; broken links when `--enable-lidar` is off  |
| Sweep dashboard only on 8081                  | Users must know two ports; no unified navigation         |
| ECharts in Go embeds, LayerChart in Svelte    | Two charting stacks to maintain                          |
| LiDAR status page uses Go templates           | Cannot benefit from Svelte reactivity or component reuse |
| Three surfaces for LiDAR functionality        | Fragmented user experience; unclear where to find what   |

## Design Constraints

- **macOS visualiser stays** — 3D Metal rendering cannot move to the browser without WebGL/WebGPU complexity that defeats the purpose
- **Go-embedded dashboards must eventually migrate** — maintaining vanilla JS + ECharts alongside Svelte + LayerChart is unsustainable
- **Radar-only deploys need a clean experience** — no dead LiDAR links
- **Raspberry Pi 4 is the target** — resource-constrained; binary size matters
- **Dynamic LiDAR lifecycle** — LiDAR can be enabled/disabled at runtime without interrupting radar logging/stream
- **Private LAN deployment** — no auth/access control required for now (future work)
- **Privacy-first** — no architectural changes affect data privacy

## Proposed End State

```
┌──────────────────────────────────────────────────────────┐
│                     Go Binary                            │
│                                                          │
│  ┌───────────────────────┐  ┌─────────────────────────┐  │
│  │  Port 8080            │  │  Port 50051 (gRPC)      │  │
│  │                       │  │  Frame streaming        │  │
│  │  Embedded Svelte SPA  │  │                         │  │
│  │  ├─ Radar section     │  └────────────┬────────────┘  │
│  │  │  ├─ Dashboard      │               │               │
│  │  │  ├─ Sites          │               │               │
│  │  │  ├─ Reports        │               │               │
│  │  │  └─ Settings       │               │               │
│  │  │                    │               │               │
│  │  └─ LiDAR section     │     macOS Metal Visualiser    │
│  │     (conditional)     │     ├─ Live 3D point cloud    │
│  │     ├─ Status         │     ├─ Track labelling        │
│  │     ├─ Tracks         │     └─ Replay/debug overlays  │
│  │     ├─ Scenes         │                               │
│  │     ├─ Runs           │                               │
│  │     ├─ Sweep          │                               │
│  │     └─ Regions        │                               │
│  └───────────────────────┘                               │
│                                                          │
│  Port 8081: retired (API endpoints moved to 8080)        │
└──────────────────────────────────────────────────────────┘
```

### Key Decisions in End State

1. **One Svelte app** with conditional LiDAR sections (not two separate apps)
2. **Go-embedded HTML dashboards retired** — sweep, regions, status migrated to Svelte
3. **macOS visualiser retained** — unchanged role for 3D rendering and debugging
4. **Port 8081 retired** — LiDAR API endpoints consolidated under 8080
5. **LiDAR navigation hidden** when `--enable-lidar` is off

## Options Evaluated

### ~~Option A: Two Separate Svelte Apps (Radar App + LiDAR App)~~

Ship two independent SvelteKit applications, each embedded in the binary. The radar app serves on 8080, the LiDAR app on 8081 (replacing the Go-embedded HTML).

**Architecture:**

- `web/radar/` — radar-only SvelteKit app (dashboard, sites, reports, settings)
- `web/lidar/` — LiDAR-only SvelteKit app (tracks, scenes, runs, sweep, regions, status)
- Two `embed.FS` directives in Go; two static builds

**Advantages:**

- Clean separation: radar binary embeds only `web/radar/build/`
- LiDAR app can evolve independently with specialised dependencies
- No conditional rendering needed — each app is self-contained
- Could theoretically use different charting libraries per app

**Disadvantages:**

- **Duplicated infrastructure**: two SvelteKit configs, two package.jsons, two build pipelines, two sets of shared utilities (units, timezone, date formatting, API client, svelte-ux theme)
- **Two ports in production**: users must know both addresses; no unified navigation between radar and LiDAR
- **Larger combined binary**: two full SvelteKit bundles with duplicated framework code
- **Shared component drift**: MapEditorInteractive, DataSourceSelector, stores, and utility libraries must be maintained in sync or extracted to a shared package
- **Build complexity**: Makefile needs `build-web-radar`, `build-web-lidar`, `ensure-web-stub` for both
- **Migration cost**: must split existing `web/` into two projects, re-test both, update all embed paths

### Option B: One Svelte App with Conditional LiDAR Sections

Keep a single SvelteKit application. LiDAR routes remain in the app but are conditionally shown in navigation based on a runtime capability check (API call to determine if `--enable-lidar` is active).

**Architecture:**

- `web/` — single SvelteKit app (unchanged structure)
- LiDAR routes at `/app/lidar/*` (existing) plus new sweep/regions/status routes
- Navigation sidebar queries `/api/config` or a new `/api/capabilities` endpoint to determine sensor availability
- LiDAR nav items hidden when LiDAR is disabled

**Advantages:**

- **Minimal structural change**: no project splitting; existing routes, components, and utilities stay put
- **Single build pipeline**: one `make build-web`, one embed, one bundle
- **Unified UX**: one port, one navigation tree, seamless sensor switching
- **Shared utilities naturally**: stores, API client, date/unit helpers, svelte-ux theme — all shared without duplication
- **Smaller binary**: single SvelteKit bundle (currently ~220KB gzipped for `everything.js`)
- **Progressive migration**: Go-embedded dashboards can be migrated one at a time into new Svelte routes

**Disadvantages:**

- Radar-only binary still ships LiDAR JavaScript (dead code in the bundle)
- Requires a capabilities API and conditional navigation logic
- Requires runtime capability refresh and backend lifecycle management for hot-enable/disable
- LiDAR routes must return an explicit "LiDAR disabled" response and must not initialize hardware when disabled
- Single `package.json` may accumulate LiDAR-specific dependencies over time

### ~~Option C: One Svelte App with Build-Time LiDAR Exclusion~~

Like Option B, but use SvelteKit's build configuration or a Vite plugin to strip LiDAR routes at build time, producing two variants of the static output.

**Architecture:**

- `web/` — single source tree
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

## Decision Matrix

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

## Recommendation: Option B — One Svelte App with Conditional LiDAR Sections ✅

Option B is the clear winner. The single-app approach avoids duplication, keeps the build simple, and provides the best user experience. The minor downside — shipping ~50KB of unused LiDAR JavaScript in radar-only deploys — is negligible compared to the maintenance cost of two separate applications or custom build tooling.

The dead-route concern is mitigated by explicit server-side gating: `/api/lidar/*` must return a clear "LiDAR disabled" response and must not initialize hardware when LiDAR is off. Direct URL access to `/app/lidar/*` should show a friendly disabled state. This pairs with runtime capability refresh so hot-enable/disable is reflected without restarting the radar process.

## Migration Plan

### Phase 0: Capabilities API & Conditional Navigation

**Effort: Small (2–4 days)**

Add a `/api/capabilities` endpoint (or extend `/api/config`) that reports which sensors are active and their runtime state:

```json
{
  "radar": true,
  "lidar": { "enabled": false, "state": "disabled" },
  "lidar_sweep": false
}
```

Capabilities must reflect runtime transitions (disabled, starting, ready, error) so LiDAR can be enabled or disabled without restarting the radar process. A backend lifecycle manager should own start/stop of LiDAR pipelines and must not interrupt radar logging or streaming.

Update the root `+layout.svelte` to fetch capabilities on load and conditionally render LiDAR navigation items. Add periodic refresh (or SSE) so the UI updates when LiDAR comes online. When `lidar` is disabled, the sidebar shows only radar routes and all `/api/lidar/*` endpoints return a clear "LiDAR disabled" response without initializing hardware.

**Files changed:**

- `internal/api/server.go` — new endpoint
- `internal/lidar/` — lifecycle manager and capability source
- `web/src/routes/+layout.svelte` — conditional nav rendering
- `web/src/lib/api.ts` — capabilities fetch function

### Phase 1: Migrate Status Page

**Effort: Small (2–3 days)**

The status page (`status.html`, 492 lines) is mostly a configuration panel with forms and API links. It uses Go templates for initial server-side rendering but the interactive parts are vanilla JavaScript.

Rewrite as `/app/lidar/status` Svelte route using svelte-ux form components (TextField, Toggle, SelectField). Replace Go template variables with API calls to `/api/lidar/params` and `/api/config`.

**What moves:**

- System status display (sensor ID, mode, firmware)
- PCAP replay controls
- Parameter JSON editor
- Diagnostic link directory

**Charting impact:** None — status page has no charts.

**Files changed:**

- New: `web/src/routes/lidar/status/+page.svelte`
- Update: `web/src/routes/+layout.svelte` (add nav item)
- Update: `web/src/lib/api.ts` (status API calls)

### Phase 2: Migrate Background Regions Dashboard

**Effort: Small (2–3 days)**

The regions dashboard (`regions_dashboard.html`, 54 lines + `regions_dashboard.js`, 298 lines) renders a polar grid using Canvas 2D. This is a self-contained visualisation with no framework dependencies.

Rewrite as `/app/lidar/regions` Svelte route. The Canvas rendering logic can be largely preserved inside a Svelte component wrapping an HTML `<canvas>` element — no charting library rewrite needed since it uses raw Canvas 2D, not ECharts.

**What moves:**

- Polar grid visualisation (40 rings × 1800 azimuth bins)
- Interactive region hover/selection
- Legend and tooltip rendering

**Charting impact:** None — uses Canvas 2D directly, not ECharts.

**Files changed:**

- New: `web/src/routes/lidar/regions/+page.svelte`
- New: `web/src/lib/components/lidar/RegionsCanvas.svelte`
- Update: `web/src/routes/+layout.svelte` (add nav item)

### Phase 3: Migrate Sweep Dashboard

**Effort: Large (2–3 weeks)**

The sweep dashboard is the most complex embedded page (`sweep_dashboard.html`, 338 lines + `sweep_dashboard.js`, 2,390 lines + CSS). It has two operational modes (manual sweep, auto-tune), 8 ECharts chart types, real-time polling, and complex parameter schema handling.

This is the critical migration that requires rewriting all ECharts visualisations using LayerChart/d3-scale (the Svelte app's existing charting stack). Each chart type must be rebuilt:

| ECharts Chart           | LayerChart Equivalent       | Complexity |
| ----------------------- | --------------------------- | ---------- |
| Acceptance rate line    | Spline + Area               | Low        |
| Nonzero cells line      | Spline                      | Low        |
| Bucket distribution bar | Bar chart                   | Low        |
| Track count line        | Spline                      | Low        |
| Alignment score line    | Spline                      | Low        |
| Parameter heatmap       | Custom (Canvas or SVG grid) | High       |
| Multi-round comparison  | Group + Spline              | Medium     |
| Recommendation table    | svelte-ux Table             | Low        |

Rewrite as `/app/lidar/sweep` Svelte route with sub-components for each chart and the parameter configuration panel.

**What moves:**

- Manual sweep configuration and execution
- Auto-tune with multi-round optimisation
- All 8 chart types (rewritten from ECharts to LayerChart)
- CSV/JSON export
- Scene/PCAP selection
- Ground truth evaluation UI

**Charting impact: High** — 8 chart types rewritten from ECharts to LayerChart/d3-scale. The heatmap is the hardest; LayerChart doesn't have a native heatmap so it would need a custom Canvas or SVG implementation.

**Files changed:**

- New: `web/src/routes/lidar/sweep/+page.svelte`
- New: `web/src/routes/lidar/sweep/+page.ts`
- New: `web/src/lib/components/lidar/SweepCharts.svelte` (or multiple chart components)
- New: `web/src/lib/components/lidar/ParameterEditor.svelte`
- Update: `web/src/lib/api.ts` (sweep API calls)
- Update: `web/src/lib/types/lidar.ts` (sweep types)
- Update: `web/src/routes/+layout.svelte` (add nav item)

### Phase 4: Migrate Debug Dashboard

**Effort: Small (1 day)**

The debug dashboard (`dashboard.html`, 43 lines) is a simple iframe grid linking to chart endpoints. Once the sweep and regions dashboards are migrated, this page becomes a simple link/redirect page in Svelte, or is retired entirely if all debug views are accessible from the LiDAR navigation.

**What moves:**

- Grid of chart iframes → links to individual Svelte pages or retained as iframe embeds during transition

### Phase 5: Retire Port 8081

**Effort: Medium (3–5 days)**

Once all HTML dashboards are migrated to Svelte, consolidate the LiDAR API endpoints from port 8081 into port 8080. This involves:

1. Moving API route registration from `internal/lidar/monitor/webserver.go` to `internal/api/server.go`
2. Updating the Vite dev proxy to route all `/api/lidar/*` to 8080
3. Removing the `--lidar-listen` flag and 8081 HTTP server
4. Updating documentation and deployment configs

**Note:** The gRPC server on port 50051 is unaffected — it serves the macOS visualiser and is independent of the HTTP consolidation.

**Files changed:**

- `internal/api/server.go` — absorb LiDAR API routes
- `internal/lidar/monitor/webserver.go` — remove HTML serving, retain API handlers
- `cmd/radar/radar.go` — remove 8081 HTTP server setup
- `web/vite.config.ts` — remove split proxy
- `docs/` — update deployment and architecture docs

### Phase 6: Clean Up Go Embeds

**Effort: Small (1 day)**

Remove the embedded HTML templates and ECharts assets from the Go binary:

- Delete `internal/lidar/monitor/html/*.html`
- Delete `internal/lidar/monitor/assets/` (ECharts, CSS, dashboard JS)
- Remove `//go:embed` directives for dashboard assets
- Remove handler functions for retired endpoints

This reduces binary size and eliminates the dual charting stack.

## Effort Summary

| Phase     | Scope                              | Effort         | Charting Rewrite                         |
| --------- | ---------------------------------- | -------------- | ---------------------------------------- |
| 0         | Capabilities API + conditional nav | 2–4 days       | None                                     |
| 1         | Status page migration              | 2–3 days       | None                                     |
| 2         | Regions dashboard migration        | 2–3 days       | None (Canvas 2D)                         |
| 3         | Sweep dashboard migration          | 2–3 weeks      | **8 chart types** (ECharts → LayerChart) |
| 4         | Debug dashboard retirement         | 1 day          | None                                     |
| 5         | Port 8081 retirement               | 3–5 days       | None                                     |
| 6         | Go embed cleanup                   | 1 day          | None                                     |
| **Total** |                                    | **~5–6 weeks** |                                          |

Phase 3 (sweep dashboard) dominates the effort due to the ECharts-to-LayerChart rewrite. All other phases are straightforward migrations of forms, tables, and Canvas-based visualisations that don't require charting library translation.

## Detailed Checklists and Timelines (Option B)

### Phase 0: Capabilities API & Conditional Navigation

Expected timeline: 2–4 days.

Checklist:

- [ ] Define the capabilities schema and state machine (disabled, starting, ready, error) and document the contract in `docs/`.
- [ ] Implement a backend LiDAR lifecycle manager that can start/stop LiDAR pipelines without interrupting radar logging/stream.
- [ ] Implement `/api/capabilities` (or extend `/api/config`) with unit tests for default values and hardware-off scenarios.
- [ ] Ensure all `/api/lidar/*` endpoints enforce capability gating (return "LiDAR disabled" without initializing hardware).
- [ ] Add `getCapabilities()` to `web/src/lib/api.ts` with retry/backoff and error handling.
- [ ] Update `web/src/routes/+layout.svelte` to gate LiDAR nav items, including a loading state and a fallback when the endpoint fails.
- [ ] Add a shared "LiDAR not enabled" empty-state component for direct route access.
- [ ] Add UI capability refresh (poll or SSE) and handle transitional states (starting, error).
- [ ] Add route-level lazy loading for LiDAR routes to minimize radar-only initial load.
- [ ] Verify radar-only UX on Pi 4 (startup time, sidebar items, zero broken links).
- [ ] Add tests that hot-enable/disable LiDAR does not interrupt radar logging.

### Phase 1: Status Page Migration

Expected timeline: 2–3 days.

Checklist:

- [ ] Inventory all `status.html` features and map each to an API endpoint or new endpoint.
- [ ] Build `/app/lidar/status` with svelte-ux form components and validations.
- [ ] Implement/extend status APIs for read/write, ensuring param updates are atomic and validated server-side.
- [ ] Add PCAP replay controls and diagnostic link directory parity with the old page.
- [ ] Add error states, loading states, and form reset/rollback behavior.
- [ ] Confirm feature parity with the Go-template version and remove/redirect old links.
- [ ] Add API integration tests and basic UI regression checks for status workflows.

### Phase 2: Background Regions Dashboard Migration

Expected timeline: 2–3 days.

Checklist:

- [ ] Inventory existing API usage and data assumptions from `regions_dashboard.js`.
- [ ] Port `regions_dashboard.js` Canvas rendering into `RegionsCanvas.svelte` with resize handling.
- [ ] Wire data loading, caching, and refresh cadence to existing APIs.
- [ ] Recreate hover/selection, legend, and tooltip behavior.
- [ ] Validate performance on Pi 4 (fps, memory) with realistic data.
- [ ] Add basic UI tests for interactions and empty/error states.

### Phase 3: Sweep Dashboard Migration

Expected timeline: 2–3 weeks.

Checklist:

- [ ] Document sweep workflows (manual + auto-tune) and required data schemas, including any undocumented API fields.
- [ ] Create a Svelte store/model for sweep state, polling, and cancellation.
- [ ] Rebuild chart components (8 total).
- [ ] Acceptance rate line (Spline + Area)
- [ ] Nonzero cells line (Spline)
- [ ] Bucket distribution bar (Bar)
- [ ] Track count line (Spline)
- [ ] Alignment score line (Spline)
- [ ] Parameter heatmap (Canvas or SVG grid)
- [ ] Multi-round comparison (Grouped + Spline)
- [ ] Recommendation table (svelte-ux Table)
- [ ] Implement scene/PCAP selection and ground-truth evaluation UI.
- [ ] Implement CSV/JSON export and parameter preset management.
- [ ] Add throttling or sampling for high-frequency polling to protect Pi 4 performance.
- [ ] Add render throttling/virtualization so all charts do not repaint at once.
- [ ] Validate parity with existing sweep outputs and run real-data acceptance checks.

### Phase 4: Debug Dashboard Retirement

Expected timeline: 1 day.

Checklist:

- [ ] Decide whether to retire or redirect the iframe grid.
- [ ] If kept, re-implement as a Svelte route with links to new pages.
- [ ] Remove old debug nav entries and update documentation.

### Phase 5: Retire Port 8081

Expected timeline: 3–5 days.

Checklist:

- [ ] Confirm no external consumers depend on port 8081 (per current deployment).
- [ ] Move LiDAR API route registration to `internal/api/server.go` and update handlers.
- [ ] Update `web/vite.config.ts` proxy rules to consolidate on 8080.
- [ ] Deprecate `--lidar-listen` and any 8081-specific deployment configs.
- [ ] Update docs, systemd/unit files, and deployment notes.
- [ ] Validate that macOS visualiser (gRPC 50051) is unaffected.

### Phase 6: Go Embed Cleanup

Expected timeline: 1 day.

Checklist:

- [ ] Delete retired HTML/JS/CSS assets in `internal/lidar/monitor/`.
- [ ] Remove obsolete `//go:embed` directives and handler routes.
- [ ] Remove ECharts assets and any remaining references.
- [ ] Run Go and web tests; verify binary size reduction.

## Risks and Mitigations

| Risk                                                       | Likelihood | Impact | Mitigation                                                                                                  |
| ---------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------- |
| LayerChart lacks heatmap support for sweep charts          | High       | Medium | Use raw Canvas/SVG within Svelte component; LayerChart isn't required for every chart                       |
| Sweep dashboard polling logic is complex to port           | Medium     | Medium | Svelte stores + `setInterval` can replicate the polling pattern; consider SSE for future improvement        |
| Hot-enable/disable LiDAR disrupts radar logging            | Medium     | High   | Introduce a LiDAR lifecycle manager with start/stop isolation and tests that assert radar stream continuity |
| `/api/lidar/*` endpoints initialize hardware when disabled | Low        | High   | Enforce capability gating server-side; return explicit "LiDAR disabled" and never initialize when disabled  |
| Sweep UI performance on Pi 4 regresses                     | Medium     | Medium | Data decimation, chart throttling, polling backoff, and lazy render of off-screen charts                    |
| API parity gaps surface late in migration                  | Medium     | Medium | API/schema inventory before each migration, plus acceptance tests using real data                           |
| UI capability state drifts from runtime reality            | Medium     | Low    | Capability refresh via polling or SSE; handle transitional states (starting/error)                          |
| Test coverage insufficient for regressions                 | Medium     | Medium | Add API integration tests and a minimal E2E sweep flow using representative datasets                        |

## Non-Goals

- **macOS visualiser changes** — the Metal app is retained as-is for 3D point cloud rendering
- **PDF report generation** — out of scope; remains a Python/LaTeX tool
- **LiDAR build tag** — runtime `--enable-lidar` flag is sufficient; no need for compile-time exclusion
- **New charting library adoption** — use existing LayerChart/d3-scale stack; ECharts is retired, not replaced with another heavyweight library
- **Auth/access control** — out of scope for now; deployments are private LAN only
