# Design review and improvement plan

- **Status:** Point-in-time audit. Open items have been extracted into
  [BACKLOG.md](../BACKLOG.md); resolved items are kept in this doc as a record.
  Section anchors (§1.1, §3.3, etc.) are referenced from BACKLOG.md, so this
  file stays at its current path.

- Reference: [DESIGN.md](DESIGN.md), [ARCHITECTURE.md](../../ARCHITECTURE.md)
- Backlog: [BACKLOG.md](../BACKLOG.md); P1 item 6

## Purpose

Comprehensive audit of the repository against the design contract in DESIGN.md, identifying gaps, non-compliance, and areas for improvement. Each finding includes a severity, effort estimate, and recommended action.

Scope: design contract compliance only. The project-wide priority list lives in [BACKLOG.md](../BACKLOG.md).

Severity levels: **Critical** (violates explicit DESIGN.md contract), **High** (undermines design goals), **Medium** (missed best practice), **Low** (polish/nice-to-have).

---

## 1. Percentile colour palette compliance

### 1.1 Web dashboard uses non-canonical palette: critical

**Location:** [web/src/routes/+page.svelte](../../web/src/routes/+page.svelte) lines 49–57

The dashboard defines two competing palettes, neither matching DESIGN.md §3.3:

| Metric | `colorMap` (legend) | `cRange` (chart) | Canonical (DESIGN.md §3.3) |
| ------ | ------------------- | ---------------- | -------------------------- |
| p50    | `#ece111`           | `#2563eb`        | `#fbd92f`                  |
| p85    | `#ed7648`           | `#16a34a`        | `#f7b32b`                  |
| p98    | `#d50734`           | `#f59e0b`        | `#f25f5c`                  |
| max    | `#000000`           | `#ef4444`        | `#2d1e2f`                  |

Neither palette matches the canonical values.

DESIGN.md explicitly flags this as non-compliant and requires migration.

**Action:** Replace both palettes with the canonical values from DESIGN.md §3.3. Extract the palette to a shared constant (e.g. [web/src/lib/palette.ts](../../web/src/lib/palette.ts)) so that any future chart component can import it.

**Effort:** 1–2 hours

### 1.2 macOS visualiser has no percentile palette: low

**Location:** [tools/visualiser-macos/VelocityVisualiser/](../../tools/visualiser-macos/VelocityVisualiser)

The macOS visualiser uses system/semantic colours only and currently renders no percentile metric charts. No palette violation exists today, but there is no shared palette constant prepared for when percentile sparklines are added.

**Action:** No immediate work required. When metric charts are added to the macOS visualiser, source the palette from a shared definition (e.g. a constants file or plist).

**Effort:** Deferred

### 1.3 No single-source palette definition: medium

**Location:** Independent definitions exist in:

- Go: [internal/report/chart/palette.go](../../internal/report/chart/palette.go) (Go PDF pipeline — authoritative)
- Web: [web/src/routes/+page.svelte](../../web/src/routes/+page.svelte) (non-compliant)
- DESIGN.md §3.3 (specification)

There is no machine-readable single-source file that all platforms import or generate from.

**Action:** Create [web/src/lib/palette.ts](../../web/src/lib/palette.ts) exporting the canonical palette. Consider a future shared JSON/YAML palette that both the Go and web stacks can derive from.

**Effort:** 2–4 hours

---

## 2. CSS DRY and shared standard classes

### 2.1 No shared standard classes exist: high

**Location:** [web/src/routes/app.css](../../web/src/routes/app.css) (20 lines: Tailwind imports, 2 CSS variables, 1 SVG rule)

DESIGN.md §5.5 requires extracting repeated class bundles into named standard classes such as `vr-page`, `vr-toolbar`, `vr-control-row`, `vr-stat-grid`, and `vr-chart-card`. None of these exist.

Current state:

- `flex items-center` appears **40 times** across 13 files
- `rounded` appears **61 times** across 13 files
- Page layout, toolbar rows, stat grids, and card patterns are duplicated verbatim across lidar routes

**Action:** Audit the four lidar route files (`tracks`, `scenes`, `runs`, `sweeps`) for repeated layout patterns. Extract at least:

- `vr-page` (page container with standard padding/max-width)
- `vr-toolbar` (control strip with flex row and gap)
- `vr-stat-grid` (responsive stat cards grid)
- `vr-chart-card` (chart container with border and height)

Add these to [web/src/routes/app.css](../../web/src/routes/app.css) or a new `web/src/lib/styles/standards.css` imported from `app.css`.

**Effort:** 1–2 days

### 2.2 No widescreen content containment: medium

**Location:** All route files

DESIGN.md §5.7 specifies that at ≥3000 px the UI should centre an internal content frame and constrain form/workspace widths. No `@media` queries or responsive containment classes are defined.

**Action:** Add a `vr-page` class (or Tailwind `@screen` variant) that centres content and caps max-width at desktop breakpoints. Test at 3000 px+.

**Effort:** 2–4 hours

---

## 3. Chart rendering compliance

### 3.1 Chart empty-state placeholder missing: critical

**Location:** [web/src/routes/+page.svelte](../../web/src/routes/+page.svelte)

DESIGN.md §4.1 requires explicit loading/empty/error states for charts. The dashboard conditionally renders the chart only when `chartData.length > 0` but shows **no empty-state placeholder** when there is no data. Users see blank space.

**Action:** Add an explicit empty-state placeholder (e.g. "No speed data available for this period") inside the chart container when `chartData.length === 0`.

**Effort:** 30 minutes

### 3.2 Legend order not enforced in chart component: medium

**Location:** [web/src/routes/+page.svelte](../../web/src/routes/+page.svelte) `cDomain` definition (currently around lines 495–496, after chart state)

DESIGN.md §4.1 requires legend order `p50, p85, p98, max, then count/auxiliary`. The current `cDomain` array follows this order, but there is no programmatic enforcement or shared constant.

**Action:** Define legend order in the shared palette module and reference it from chart components.

**Effort:** 30 minutes (combined with §1.3 work)

### 3.3 Go-embedded eCharts dashboards not aligned: high

**Location:** `internal/lidar/monitor/webserver.go` (5 `go:embed` directives, 13+ ECharts references)

The legacy monitor dashboards (status, debug, sweep, regions) use ECharts with Go HTML templates. These are explicitly out of scope for new design work (DESIGN.md §2) but are the migration target per the frontend consolidation plan. Key alignment gaps:

- ECharts palettes are not cross-referenced against the canonical percentile palette
- No shared colour constants between Go templates and the Svelte frontend
- HTML templates use inline styles, not a shared CSS system

**Action:** No palette migration now (these dashboards will be retired in frontend consolidation Phases 1–5). However, document in the frontend consolidation plan that chart palette alignment is a requirement during migration to LayerChart.

**Effort:** 30 minutes (documentation only)

---

## 4. Component policy compliance

### 4.1 svelte-ux usage is consistent: no action

All route files import from `svelte-ux` for UI primitives (Button, Card, SelectField, TextField, DateRangeField, AppBar, etc.). No native HTML replacements without justification were found. **Compliant with DESIGN.md §5.3.**

### 4.2 LayerChart usage limited to dashboard: medium

**Location:** [web/src/routes/+page.svelte](../../web/src/routes/+page.svelte)

LayerChart is only used on the main dashboard. The LiDAR routes (`tracks`, `scenes`, `runs`, `sweeps`) do not yet render charts. This is not a violation, but as charts are added to LiDAR routes, the chart rendering policy (DESIGN.md §5.4) must be followed.

**Action:** No immediate work. Add to the frontend consolidation plan: all new charts in LiDAR routes must use LayerChart/d3-scale, not ad-hoc SVG.

**Effort:** Deferred

### 4.3 No ad-hoc SVG charts found: no action

Zero `<svg>` elements found in route-level `.svelte` files. **Compliant with DESIGN.md §5.4.**

---

## 5. Information hierarchy (dESIGN.md §3.1)

### 5.1 Lidar routes follow the four-tier hierarchy: no action

The modern workspace routes (`tracks`, `scenes`, `runs`, `sweeps`) implement:

1. Context header (page title, data source context)
2. Control strip (filters, selectors)
3. Primary workspace (data tables, track lists)
4. Detail/inspector areas (track details, scene inspector)

**Compliant.**

### 5.2 Dashboard lacks explicit context header: low

**Location:** [web/src/routes/+page.svelte](../../web/src/routes/+page.svelte)

The main dashboard does not show the current site name, data range, or sensor context prominently. Users must infer context from filter controls.

**Action:** Add a context header bar showing site name, active date range, and sensor source. Aligns with the design hierarchy and improves operational clarity.

**Effort:** 1–2 hours

---

## 6. Architectural debt (from aRCHITECTURE.md)

### 6.1 webserver.go is ~4,010 lines: high

**Location:** `internal/lidar/monitor/webserver.go`

Combines HTTP handler registration, PCAP replay control, live UDP listening, ECharts chart generation, state management, and data source lifecycle in a single file. This is flagged in the existing plans but warrants a structured split:

| Extracted file      | Responsibility                  | Est. lines |
| ------------------- | ------------------------------- | ---------- |
| `routes.go`         | Route table registration        | ~200       |
| `data_source.go`    | DataSourceManager lifecycle     | ~400       |
| `pcap_control.go`   | PCAP replay start/stop/progress | ~500       |
| `chart_handlers.go` | ECharts chart HTTP handlers     | ~600       |
| `grid_handlers.go`  | Grid/heatmap HTTP handlers      | ~400       |

**Action:** Split incrementally. Start with extracting the route table (already uses grouped `[]route` slices per stored memory). This is covered in the existing prioritised work plan (P0-2) but lacks the specific file-level split targets above.

**Effort:** 2–3 days (incremental, one extraction per PR)

### 6.2 background.go is ~2,600 lines: high

**Location:** `internal/lidar/background.go`

Mixes persistence, export, drift detection, and spatial region management with core grid processing. This is flagged in the layer alignment review (Future Work item 14). The layer migration plan targets moving this into `l3grid/`, but the file currently resides in the parent [internal/lidar/](../../internal/lidar) package.

**Action:** Split into:

- `background.go`: core EMA grid processing
- `background_persistence.go`: snapshot save/restore
- `background_regions.go`: spatial region management
- `background_export.go`: ASC/heatmap export

**Effort:** 1–2 days

### 6.3 analysis_run.go is ≈1,343 lines with domain comparison logic: medium

**Location:** `internal/lidar/analysis_run.go`

`CompareRuns()` and related domain logic is co-located with run persistence in the parent lidar package. Comparison algorithms should be separated from CRUD operations.

**Action:** Extract `CompareRuns()` and parameter conversion functions to a dedicated `internal/lidar/evaluation/` package or into `l6objects/`. Keep only CRUD operations in the storage layer.

**Effort:** 1 day

---

## 7. Testing gaps

### 7.1 No visual regression testing: medium

No snapshot, screenshot, or visual regression testing exists for the web frontend. Palette and layout changes risk silent regressions.

**Action:** Add Playwright visual comparison tests for key pages (dashboard, tracks, scenes). Capture baseline screenshots and compare on PR.

**Effort:** 1–2 days (setup + 3–5 baseline tests)

### 7.2 No accessibility testing: medium

57 ARIA attributes found across 10 Svelte files (good baseline), but no automated accessibility tests exist (no axe-core, no a11y test runner).

**Action:** Add `@axe-core/playwright` or `vitest-axe` to the web test suite. Create a single test that asserts no critical accessibility violations on each route.

**Effort:** 4–8 hours

### 7.3 No integration test infrastructure: medium

No Cypress, Playwright, or other E2E framework is configured. API integration is tested in Go, but frontend-to-API flows are untested.

**Action:** Add Playwright as the E2E framework (consistent with §7.1). Create smoke tests for: loading the dashboard, navigating to lidar routes, and verifying chart rendering.

**Effort:** 1–2 days (setup + 3–5 smoke tests)

### 7.4 No route-level web tests: low

11 web test files exist, all for library/utility code and Go-embedded dashboards. No route-level Svelte component tests exist.

**Action:** Add component tests for at least the dashboard page (`+page.svelte`) covering data loading, error states, and chart rendering.

**Effort:** 1 day

### 7.5 Code coverage thresholds are informational only: low

[codecov.yml](../../codecov.yml) sets a 1% threshold, effectively disabling coverage gates. The web Jest config has 90% thresholds but only for [web/src/lib/](../../web/src/lib).

**Action:** After improving test coverage, increase codecov thresholds to meaningful levels (e.g. 60–70% for Go, 80% for web lib).

**Effort:** 30 minutes (config change after coverage improves)

---

## 8. Documentation gaps

### 8.1 DESIGN.md not referenced from cONTRIBUTING.md or rEADME.md: high

Neither [CONTRIBUTING.md](../../CONTRIBUTING.md) nor [README.md](DESIGN.md) mentions [DESIGN.md](DESIGN.md). Contributors can submit UI PRs without awareness of the design contract.

**Action:** Add a "Design Language" section to [CONTRIBUTING.md](../../CONTRIBUTING.md) that references DESIGN.md and summarises the PR checklist (DESIGN.md §9). Add a link to DESIGN.md in the README's documentation section.

**Effort:** 30 minutes

### 8.2 PR checklist from dESIGN.md §9 not enforced: medium

DESIGN.md §9 defines a detailed UI/chart PR checklist, but this is not included in the GitHub PR template.

**Action:** Create or update [.github/PULL_REQUEST_TEMPLATE.md](../../.github/PULL_REQUEST_TEMPLATE.md) to include the DESIGN.md §9 checklist as a default section for UI/chart PRs.

**Effort:** 30 minutes

### 8.3 Frontend consolidation plan lacks palette alignment requirement: low

**Location:** [docs/plans/web-frontend-consolidation-plan.md](../plans/web-frontend-consolidation-plan.md)

The plan details the Phase 3 ECharts-to-LayerChart migration but does not explicitly require palette alignment with DESIGN.md §3.3 during migration.

**Action:** Add a subsection to the Phase 3 description requiring that all migrated charts use the canonical palette from DESIGN.md §3.3.

**Effort:** 15 minutes

---

## 9. Cross-Platform alignment

### 9.1 Tick density and axis formatting untested: medium

DESIGN.md §4.1 requires 6–10 visible X-axis labels and 4–6 Y-axis labels with no overlapping. The dashboard chart uses LayerChart defaults but there is no test or visual review confirming tick density compliance at different data densities and window sizes.

**Action:** Add manual review for tick density as part of the visual regression test suite (§7.1). Document acceptable tick density ranges in a test fixture.

**Effort:** Combined with §7.1

### 9.2 Time formatting does not verify timezone respect: low

DESIGN.md §4.1 requires time formatting to respect selected timezone. The web frontend has timezone stores ([web/src/lib/stores/timezone.ts](../../web/src/lib/stores/timezone.ts)) with tests, but the chart axis labels are not verified to use the selected timezone.

**Action:** Add a unit test verifying that chart axis labels format timestamps in the user-selected timezone, not UTC or local browser timezone.

**Effort:** 1–2 hours

---

## 10. Security and privacy

### 10.1 No authentication on LAN API: low (by design)

ARCHITECTURE.md and DESIGN.md assume private LAN deployment with no authentication. The frontend consolidation plan notes this is acceptable for the current deployment model.

**Action:** No immediate work. If deployment moves beyond private LAN, add an authentication layer. Document the trust boundary explicitly in ARCHITECTURE.md §Security.

**Effort:** Deferred

### 10.2 Go-embedded HTML templates may have injection risks: medium

**Location:** `internal/lidar/monitor/templates.go`, `webserver.go`

Go HTML templates use `html/template` (auto-escaping), which is safe for standard use. However, the templates render user-controlled data (filenames, parameters) and should be reviewed for edge cases.

**Action:** Audit all `{{.}}` template variables in `html/*.html` files for proper escaping. This will become moot when the Go-embedded dashboards are retired (frontend consolidation Phase 4–5).

**Effort:** 1–2 hours (audit only)

---

## 11. Build and development experience

### 11.1 Dual SQLite drivers in go.mod: low

**Location:** `go.mod`

Both `github.com/mattn/go-sqlite3` (CGO-based) and `modernc.org/sqlite` (pure Go) are direct dependencies. This adds build complexity and potential confusion.

**Action:** Audit which packages use each driver. If `mattn/go-sqlite3` can be fully replaced by `modernc.org/sqlite`, remove it to simplify the build (eliminates CGO requirement for some build targets).

**Effort:** 2–4 hours (audit + migration if feasible)

---

## 12. Light mode / theme compliance

### 12.1 TrackList hex ID invisible in light mode: critical

**Location:** `web/src/lib/components/lidar/TrackList.svelte:1033`

The selected-track row uses a hardcoded `background-color: white` which makes the white hex track ID text invisible when the app is in light mode. The track ID badge text inherits a light colour that has no contrast against the white background.

The selected-track row sets `background-color: white` (hardcoded), which is invisible in light mode.

**Action:** Replace `white` with a theme-aware CSS variable from svelte-ux (e.g. `hsl(var(--color-surface-200))` or `var(--surface-content)`) so the background adapts to both dark and light themes.

**Effort:** 15 minutes

### 12.2 MapPane canvas legend uses hardcoded `#fff`: high

**Location:** `web/src/lib/components/lidar/MapPane.svelte:683, 698`

The canvas legend text is drawn with `ctx.fillStyle = '#fff'`, making it invisible against light-mode backgrounds. The grid label at line 306 (`ctxLocal.strokeStyle = '#fff'`) has the same issue.

Three canvas draw calls use hardcoded `#fff`: `ctx.fillStyle` for legend key text (line 683), `ctx.fillStyle` for legend value text (line 698), and `ctxLocal.strokeStyle` for the grid label (line 306).

**Action:** Read the current theme from svelte-ux's theme store and derive a contrasting fill colour. For canvas contexts that cannot use CSS variables directly, resolve the computed colour at render time (e.g. `getComputedStyle(canvas).getPropertyValue('--color-surface-content')`).

**Effort:** 1–2 hours

### 12.3 MapPane overlay panels assume dark background: medium

**Location:** `web/src/lib/components/lidar/MapPane.svelte:886, 899`

Two absolutely-positioned overlay panels use `bg-black text-white` Tailwind classes. In light mode the opaque black panels clash with the lighter UI chrome.

Two overlay `<div>` elements (lines 886 and 899) use `bg-black text-white` with 75% and 80% opacity respectively.

**Action:** Replace `bg-black text-white` with theme-aware surface classes (e.g. `bg-surface-100/75 text-surface-content`) or use `surface-200` with appropriate opacity. The overlays sit atop a dark canvas, so a semi-transparent dark style may be acceptable in both themes; but should be reviewed visually.

**Effort:** 30 minutes

### 12.4 TimelinePane SVG text and stroke hardcoded white: high

**Location:** `web/src/lib/components/lidar/TimelinePane.svelte:280, 303`

SVG track labels use `class="fill-white"` and track lines use `stroke="white"`, making them invisible on light-mode backgrounds.

SVG track labels (line 280) use `class="fill-white"` and track lines (line 303) use `stroke="white"`, both invisible on light backgrounds.

**Action:** Replace `fill-white` with `fill-current` and set the text colour via a theme-aware CSS class. Replace `stroke="white"` with `stroke="currentColor"` and apply a theme-aware class on the parent `<g>` or `<svg>`.

**Effort:** 30 minutes
