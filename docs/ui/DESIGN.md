```
┳┓┏┓┏┓┳┏┓┳┓     ┓
┃┃┣ ┗┓┃┃┓┃┃ ┏┳┓┏┫
┻┛┗┛┗┛┻┗┛┛┗•┛┗┗┗┻
```

# Frontend and visualisation design language

This guide keeps the web UI, macOS visualiser, and reports legible as parts of one system. It sets
the rules that prevent a familiar chart from changing its meaning merely because it changed rooms.

## 1. One strict goal

Design for **operational clarity and cross-platform comparability**.

That means users should be able to read the same system state and chart meaning on:

- web UI,
- macOS visualiser,
- generated report charts,

without forcing pixel-perfect visual sameness.

Core philosophy:

- DRY is mandatory for layout, styling, and chart semantics.
- Reuse component primitives and shared style definitions before introducing one-off markup.

## 2. Scope

### 2.1 Surfaces

This document applies to exactly three **surfaces**: places a user sees charts or operational data.

1. **Web app** (Svelte): `web/`
2. **macOS app** (SwiftUI + Metal): [tools/visualiser-macos/VelocityVisualiser/](../../tools/visualiser-macos/VelocityVisualiser)
3. **PDF reports**: generated documents delivered as `.pdf`

PDF is a surface, not a rendering engine. The mechanism that produces the charts inside a PDF is a separate concern (see §7).

### 2.2 Chart rendering (separate from surfaces)

Chart rendering is converging on a single SVG-first pipeline:

- **Web:** LayerChart/d3-scale components producing inline SVG in Svelte
- **PDF (current):** Go native SVG generation (`internal/report/chart/`) → Typst templates (`internal/report/typst/`) → PDF via `typst`
- **PDF (removed):** Python matplotlib report pipeline, superseded by Go + Typst pipeline; directory deleted from repository

The Python matplotlib stack has been replaced by the Go pipeline (v0.5). New chart work should not add matplotlib dependencies.

Out of scope for new design work:

- legacy Go-embedded LiDAR dashboards under `internal/lidar/monitor/` (migration target, not style baseline)

## 3. Shared design language (cross-platform contract)

### 3.1 Information hierarchy

Every operational screen should preserve this hierarchy:

1. Context header (where am I, what data/time range am I seeing)
2. Control strip (filters, selectors, run/site/date controls)
3. Primary workspace/chart/canvas
4. Detail and inspector areas

### 3.2 Semantic status colours (all platforms)

- Success/healthy: green
- Warning/in-progress: orange/amber
- Error/failed: red
- Neutral/inactive: grey/secondary

These are semantic states, not percentile metric colours.

### 3.3 Percentile metric colour mapping (charts)

For percentile charts, keep the same metric-to-colour mapping across chart stacks:

- `p50`: `#fbd92f`
- `p85`: `#f7b32b`
- `p98`: `#f25f5c`
- `max`: `#2d1e2f`
- `count_bar`: `#2d1e2f`
- `low_sample`: `#f7b32b`

These hex values are the **canonical percentile palette** for all chart renderers and surfaces.

The single source of truth for each renderer:

- **Web:** [web/src/lib/palette.ts](../../web/src/lib/palette.ts) (`PERCENTILE_COLOURS`)
- **Go PDF:** [internal/report/chart/palette.go](../../internal/report/chart/palette.go) (`ColourP50`, `ColourP85`, …)

## 4. Chart alignment rules (required vs allowed)

### 4.1 Required alignment

- Metric names and legend order: `p50`, `p85`, `p98`, `max`, then count/auxiliary signals.
- Units and axis labels must match data source context.
- Tick behaviour must be comparable across all three platforms:
  - time-series X ticks: target 6-10 visible labels;
  - Y ticks: target 4-6 visible labels;
  - dense labels should be thinned, not overlapped;
  - X-tick cadence must adapt to the visible time span (hourly, daily, weekly, or monthly) so web and PDF land on the same granularity for the same query.
- Time formatting must respect selected timezone.
- Series segmentation: polylines remain continuous across gaps. NaN points (genuinely missing data) are skipped without interrupting the line; the low-sample background swatch and day-boundary markers already communicate data quality. Day boundaries must never force line breaks.
- Missing/low-sample periods must be visibly distinguishable.
- Empty/loading/error chart states must render explicit user-facing text.

### 4.2 Allowed differences

- Native control style (SwiftUI) vs web control style (Svelte UI components).
- Hover/tooltip behaviour and interactive affordances.
- Minor line width/marker/font rendering differences due to renderer.
- Layout fit differences caused by window/sheet geometry.

Charts do not need to be 100% identical; meaning and readability must be aligned.

## 5. Web UI style system

### 5.1 One shell, three content widths

Every web route shares the same outer shell — `vr-page` + `vr-toolbar` from [web/src/routes/app.css](../../web/src/routes/app.css). Routes differ only by **inner content width**, chosen per view intent:

| Inner wrapper           | Width                       | Use for                                                                  | Examples                                                                         |
| ----------------------- | --------------------------- | ------------------------------------------------------------------------ | -------------------------------------------------------------------------------- |
| **`vr-content-narrow`** | 768px (`max-w-3xl`)         | Settings, CRUD, single-entity forms, list views of managed entities      | `/app/settings`, `/app/site`, `/app/site/[id]`, `/app/reports`                   |
| **`vr-content-wide`**   | 1536px (`max-w-screen-2xl`) | Analytical dashboards with charts and multi-column KPI grids             | `/app/` (dashboard)                                                              |
| _(no wrapper)_          | Full canvas inside vr-page  | Operational LiDAR workspaces — tables, point-cloud views, sweep matrices | `/app/lidar/tracks`, `/app/lidar/scenes`, `/app/lidar/runs`, `/app/lidar/sweeps` |

Inside the narrow/wide column, group content as flat `<section class="space-y-4">` blocks with an `<h2>` header carrying a `border-b` divider — no `<Card>` wrappers around top-level sections. The settings page is the canonical example; new radar pages should mirror its structure.

### 5.2 Selection rule

Pick the inner wrapper by what the page asks the user to do:

- **`vr-content-narrow`** when the user is reading, editing, or managing settings/entities one field or one row at a time. The narrow column keeps form rows from spreading, matches the macOS settings convention, and gives every radar configuration page a consistent feel.
- **`vr-content-wide`** when the page is a dashboard of charts and KPIs whose value comes from horizontal breathing room. Pair with `vr-stat-grid` for KPI tiles and `vr-chart-card` for chart panels.
- **No wrapper** when the page is an operational workspace built around a wide table, a canvas, or a split-pane inspector — typically LiDAR routes.

List/detail families use the **same** wrapper throughout. A list view of an entity (e.g. `/app/site`) and the corresponding detail/edit view (`/app/site/[id]`) share the narrow column so the visual treatment stays consistent as the user moves between scan and edit. The previous guidance to mix workspaces on the list and constrained shells on the detail has been retired — having one shell with one content width per family is simpler to maintain and easier to follow as new pages are added.

### 5.3 Component policy (Svelte-UX first)

For web UI primitives, use this priority order:

1. Existing `svelte-ux` components.
2. Existing shared local wrapper components in [web/src/lib/components](../../web/src/lib/components).
3. Native HTML elements only when component capability/performance/accessibility requires it.

When using native elements instead of `svelte-ux`, add a short code comment explaining why.

### 5.4 Chart rendering policy (minimise ad-hoc SVG)

For web charts, use this priority order:

1. Existing LayerChart/d3-scale component patterns.
2. Shared chart components under [web/src/lib/components](../../web/src/lib/components) (or add one).
3. Native `<svg>` only as a temporary fallback during migration, or when a required chart type is unsupported.
4. Canvas only where it is clearly the right tool (for example high-density or heatmap rendering).

Rules:

- Avoid page-local, hand-built chart SVGs in route files when a reusable chart component can be used.
- If native SVG/Canvas is used, wrap it in a reusable component and document the reason.
- Keep chart semantics (legend order, metric colours, ticks, labels) aligned with this spec regardless of renderer.

### 5.5 CSS standards and DRY rules

Create and reuse standard classes in shared CSS rather than repeating long utility strings.

Standards:

- Shared class definitions live in the web-level standards CSS ([web/src/routes/app.css](../../web/src/routes/app.css), which may import additional shared stylesheets as needed).
- Extract repeated class patterns (containers, headers, control rows, stat grids, chart cards, pane shells) into named standard classes.
- Route files should compose standard classes first, with minimal local overrides.
- Do not copy-paste identical class bundles across multiple pages.

Enforcement guidance:

- If a class bundle is repeated across pages/components, extract it to a standard class.
- Prefer semantic names (for example `vr-page`, `vr-toolbar`, `vr-content-narrow`, `vr-content-wide`, `vr-control-row`, `vr-stat-grid`, `vr-chart-card`) over route-specific one-offs.
- `vr-page` is always the outer shell; `vr-content-narrow` / `vr-content-wide` go inside the scrollable column when the page needs a centred inner column (see §5.1).

### 5.6 Form and layout rules (web)

- One label source per control.
- Avoid nested/duplicate labels with component-provided labels.
- Keep helper text below controls.
- Use stable control widths in filter rows (avoid compression/overlap).
- Metric cards must remain in responsive grid layout at desktop widths.
- Charts must use explicit container heights and visible empty-state placeholders.

### 5.7 Widescreen web policy

- `>=3000px`: centre an internal content frame.
- Keep form-heavy content constrained (roughly 780-1100px readable width).
- Keep analytical workspace bounded (roughly 2200-2600px max canvas zone).
- Spend extra width on side panes/gutters/charts, not long unbroken form rows.

## 6. macOS Swift style system

The macOS visualiser follows native platform conventions:

- Keep native SwiftUI/macOS interaction patterns.
- Do not skin macOS controls to look like web controls.
- Preserve existing split model: toolbar/filter, main render space, inspector, playback strip.
- Keep inspector/detail pane widths practical (about 480-560px where possible).
- When showing percentile metrics in charts/sparklines, use the shared metric palette mapping.

## 7. Chart architecture and cross-platform consistency

### 7.1 Rendering engines (current → target)

| Surface | Current renderer                          | Target renderer                             | Status   |
| ------- | ----------------------------------------- | ------------------------------------------- | -------- |
| Web     | LayerChart/d3-scale (inline SVG)          | LayerChart/d3-scale (inline SVG)            | Stable   |
| PDF     | Python matplotlib (removed in v0.5)       | Go native SVG → Typst → PDF                 | Complete |
| macOS   | Swift/Metal (3D), ECharts (2D sparklines) | Swift/Metal (3D), percentile palette for 2D | Stable   |

The current Go PDF pipeline ([internal/report/](../../internal/report/)) renders charts as SVG and embeds them directly into Typst output. The current migration/design record is [pdf-typst-migration-plan.md](../plans/pdf-typst-migration-plan.md).

### 7.2 SVG as the shared intermediate format

Both the web frontend and the current Go PDF pipeline render charts as SVG. This shared format is the key to consistent output:

- **Pixel-perfect control:** SVG elements are positioned in code with explicit coordinates, stroke widths, font sizes, and viewBox dimensions. There is no renderer-dependent layout negotiation.
- **Testable:** SVG output can be parsed, diffed, and snapshot-tested as structured XML.
- **Debuggable:** SVG files open in any browser for visual inspection.
- **Dual use:** The same SVG can be displayed inline on the web and embedded directly by Typst for print output.

### 7.3 Shared chart abstractions

To keep charts visually consistent across web and PDF, the following properties must be governed by shared constants or equivalent configuration, not left to renderer defaults.

| Property                   | What it controls                                                            | Web source                                 | Go PDF source (Typst pipeline)              |
| -------------------------- | --------------------------------------------------------------------------- | ------------------------------------------ | ------------------------------------------- |
| **Palette**                | Metric-to-colour mapping                                                    | [palette.ts](../../web/src/lib/palette.ts) | `chart/palette.go`                          |
| **Legend order**           | Series stacking and legend sequence                                         | `palette.ts` (`LEGEND_ORDER`)              | `chart/palette.go`                          |
| **Tick density**           | Target number of X and Y axis labels                                        | `RadarOverviewChart` constants             | `chart/timeseries.go` style struct          |
| **Tick cadence**           | Span-aware choice of hourly/daily/weekly/monthly X ticks                    | LayerChart scale + cadence helper          | `chart/timeseries.go` `pickTickCadence`     |
| **Series segmentation**    | Polylines stay continuous; NaN points are skipped without breaking the line | LayerChart null handling (skip, not break) | `chart/timeseries.go` continuous-line logic |
| **Physical dimensions**    | SVG `width`/`height` in mm so the PDF renders at true size without scaling  | Responsive (not fixed)                     | `ChartStyle.WidthMM` / `HeightMM` per paper |
| **Font sizes**             | Axis labels, tick labels, legend text                                       | LayerChart props + CSS                     | SVG `font-size` attributes in style struct  |
| **Density/DPI**            | Element sizing relative to output dimensions                                | Responsive container width                 | Fixed SVG viewBox (matches PDF page width)  |
| **Low-sample threshold**   | Count below which data is flagged                                           | `LOW_SAMPLE_THRESHOLD = 50`                | `ChartStyle.LowSampleThreshold`             |
| **Missing-data threshold** | Count below which percentiles are suppressed                                | `MISSING_COUNT_THRESHOLD = 5`              | `ChartStyle.MissingCountThreshold`          |
| **Marker styles**          | Point shapes per series                                                     | LayerChart `Points` props                  | SVG marker elements in style struct         |
| **Line styles**            | Solid vs dashed per series                                                  | LayerChart `Spline` props                  | SVG `stroke-dasharray` in style struct      |

When a value changes in one renderer, update or verify the equivalent in the other. If the property cannot yet be shared as importable code, document the pairing in this table and keep them in sync manually.

### 7.4 Why not a single renderer for both?

The web needs interactive, responsive SVG inside a reactive component tree. The PDF needs fixed-dimension SVG suitable for print. These are different enough that a single rendering function serving both would over-abstract the problem. The design contract (§3–4) and the shared constants table above provide consistency without forcing a shared runtime.

### 7.5 Coordination rule

If palette, tick, legend, or threshold semantics change in one renderer, update the other in the same PR or log a linked follow-up issue.

## 8. Non-Goals

- Pixel-perfect matching between web, SwiftUI, and PDF. (Consistent meaning and readability: yes. Identical rendering: no.)
- Replacing native macOS UI language with web-like styling.
- Introducing a fourth web content-width mode beyond the three in §5.1.
- A single shared chart rendering function for both web and PDF (see §7.4).
- New features or investment in the Python matplotlib chart stack.

## 9. PR checklist

A UI/chart PR is complete only if:

- It states which inner content width it uses (vr-content-narrow / vr-content-wide / no wrapper) per §5.1, or notes that it's macOS-native.
- It uses `svelte-ux` primitives wherever feasible, with documented exceptions.
- It avoids ad-hoc route-level chart SVG unless justified and wrapped in a reusable component.
- It extracts repeated class bundles into shared standards CSS classes.
- It preserves semantic status colours.
- It preserves percentile colour mapping and legend order for percentile charts.
- It keeps tick density readable and non-overlapping.
- It includes explicit loading/empty/error states for charts.
- It does not add new matplotlib dependencies (the Python PDF stack is removed; see §7.1).
- It documents any intentional divergence from this contract.
