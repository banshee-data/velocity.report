```
┳┓┏┓┏┓┳┏┓┳┓     ┓
┃┃┣ ┗┓┃┃┓┃┃ ┏┳┓┏┫
┻┛┗┛┗┛┻┗┛┛┗•┛┗┗┗┻
```

# Frontend and Visualisation Design Language

Status: Draft
Last updated: 2026-02-18

## 1. One Strict Goal

Design for **operational clarity and cross-platform comparability**.

That means users should be able to read the same system state and chart meaning on:

- web UI,
- macOS visualiser,
- generated report charts,

without forcing pixel-perfect visual sameness.

Core philosophy:

- DRY is mandatory for layout, styling, and chart semantics.
- Reuse component primitives and shared style definitions before introducing one-off markup.

## 2. Scope (Three Platforms)

This document applies to exactly these three surfaces:

1. Web app (Svelte): `web/`
2. macOS app (SwiftUI + Metal): `tools/visualiser-macos/VelocityVisualiser/`
3. Chart/rendering stack:
   - interactive web charts (LayerChart/d3-scale in web routes/components)
   - static report charts (matplotlib in `tools/pdf-generator/`)

Out of scope for new design work:

- legacy Go-embedded LiDAR dashboards under `internal/lidar/monitor/` (migration target, not style baseline)

## 3. Shared Design Language (Cross-Platform Contract)

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

## 4. Chart Alignment Rules (Required vs Allowed)

### 4.1 Required alignment

- Metric names and legend order: `p50`, `p85`, `p98`, `max`, then count/auxiliary signals.
- Units and axis labels must match data source context.
- Tick behaviour must be comparable across all three platforms:
  - time-series X ticks: target 6-10 visible labels;
  - Y ticks: target 4-6 visible labels;
  - dense labels should be thinned, not overlapped.
- Time formatting must respect selected timezone.
- Missing/low-sample periods must be visibly distinguishable.
- Empty/loading/error chart states must render explicit user-facing text.

### 4.2 Allowed differences

- Native control style (SwiftUI) vs web control style (Svelte UI components).
- Hover/tooltip behaviour and interactive affordances.
- Minor line width/marker/font rendering differences due to renderer.
- Layout fit differences caused by window/sheet geometry.

Charts do not need to be 100% identical; meaning and readability must be aligned.

## 5. Web UI Style System

### 5.1 Existing canonical web styles

Modern workspace (default for new operational views):

- `/app/lidar/tracks`
- `/app/lidar/scenes`
- `/app/lidar/runs`
- `/app/lidar/sweeps`

Classic stack (valid for constrained CRUD/settings):

- `/app/site`
- `/app/site/[id]`
- `/app/reports`
- `/app/settings`

### 5.2 Selection rule

Use modern workspace by default.
Use classic stack for linear form/settings pages with minimal real-time data behaviour.

### 5.3 Component policy (Svelte-UX first)

For web UI primitives, use this priority order:

1. Existing `svelte-ux` components.
2. Existing shared local wrapper components in `web/src/lib/components`.
3. Native HTML elements only when component capability/performance/accessibility requires it.

When using native elements instead of `svelte-ux`, add a short code comment explaining why.

### 5.4 Chart rendering policy (minimise ad-hoc SVG)

For web charts, use this priority order:

1. Existing LayerChart/d3-scale component patterns.
2. Shared chart components under `web/src/lib/components` (or add one).
3. Native `<svg>` only as a temporary fallback during migration, or when a required chart type is unsupported.
4. Canvas only where it is clearly the right tool (for example high-density or heatmap rendering).

Rules:

- Avoid page-local, hand-built chart SVGs in route files when a reusable chart component can be used.
- If native SVG/Canvas is used, wrap it in a reusable component and document the reason.
- Keep chart semantics (legend order, metric colours, ticks, labels) aligned with this spec regardless of renderer.

### 5.5 CSS standards and DRY rules

Create and reuse standard classes in shared CSS rather than repeating long utility strings.

Standards:

- Shared class definitions live in web-level standards CSS (`web/src/app.css` and/or a dedicated shared stylesheet imported there).
- Extract repeated class patterns (containers, headers, control rows, stat grids, chart cards, pane shells) into named standard classes.
- Route files should compose standard classes first, with minimal local overrides.
- Do not copy-paste identical class bundles across multiple pages.

Enforcement guidance:

- If a class bundle is repeated across pages/components, extract it to a standard class.
- Prefer semantic names (for example `vr-page`, `vr-toolbar`, `vr-control-row`, `vr-stat-grid`, `vr-chart-card`) over route-specific one-offs.

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

## 6. macOS Swift Style System

The macOS visualiser follows native platform conventions:

- Keep native SwiftUI/macOS interaction patterns.
- Do not skin macOS controls to look like web controls.
- Preserve existing split model: toolbar/filter, main render space, inspector, playback strip.
- Keep inspector/detail pane widths practical (about 480-560px where possible).
- When showing percentile metrics in charts/sparklines, use the shared metric palette mapping.

## 7. Chart Stack Notes

- Web chart baseline: LayerChart/d3-scale patterns in Svelte routes/components.
- Report chart baseline: matplotlib defaults in:
  - `tools/pdf-generator/pdf_generator/core/config_manager.py`
  - `tools/pdf-generator/pdf_generator/core/chart_builder.py`
- If palette/tick/legend semantics change in one chart stack, update the other stack in the same PR or log a linked follow-up issue.

## 8. Non-Goals

- Pixel-perfect matching between web, SwiftUI, and matplotlib.
- Replacing native macOS UI language with web-like styling.
- Introducing a third visual style family beyond modern workspace and classic stack.

## 9. PR Checklist

A UI/chart PR is complete only if:

- It states which style system it follows (modern/classic/mac-native).
- It uses `svelte-ux` primitives wherever feasible, with documented exceptions.
- It avoids ad-hoc route-level chart SVG unless justified and wrapped in a reusable component.
- It extracts repeated class bundles into shared standards CSS classes.
- It preserves semantic status colours.
- It preserves percentile colour mapping and legend order for percentile charts.
- It keeps tick density readable and non-overlapping.
- It includes explicit loading/empty/error states for charts.
- It documents any intentional divergence from this contract.
