const DEFAULT_STYLES = `
  :root {
    --s2-cell-stroke: #315a70;
    --s2-parent-stroke: #163747;
    --hilbert-stroke: #006d77;
    --hilbert-marker: #d1495b;
    --hilbert-label: #163747;
  }
  .s2-cell { fill: none; stroke: var(--s2-cell-stroke); stroke-width: 0.8; opacity: 0.42; }
  .s2-parent { fill: none; stroke: var(--s2-parent-stroke); stroke-width: 2; }
  .hilbert-path { fill: none; stroke: var(--hilbert-stroke); stroke-width: 5; stroke-linecap: round; stroke-linejoin: round; }
  .hilbert-land { opacity: 1; }
  .hilbert-water { opacity: 0.3; }
  .hilbert-unclassified { opacity: 1; }
  .hilbert-direction { fill: none; stroke: transparent; stroke-width: 1; }
  .hilbert-start { fill: #fff; stroke: var(--hilbert-marker); stroke-width: 3; }
  .hilbert-end { fill: var(--hilbert-marker); stroke: #fff; stroke-width: 1.5; }
  .s2-label { fill: var(--hilbert-label); font: 10px ui-monospace, SFMono-Regular, Menlo, monospace; text-anchor: middle; dominant-baseline: central; }
`;

function formatNumber(value) {
  const rounded = Math.abs(value) < 0.0000005 ? 0 : value;
  return Number(rounded.toFixed(6)).toString();
}

function escapeXml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function pointString(point) {
  return `${formatNumber(point.x)},${formatNumber(point.y)}`;
}

function polygonPoints(points) {
  return points.map(pointString).join(" ");
}

export function buildPathData(points) {
  return points
    .map((point, index) => `${index === 0 ? "M" : "L"} ${pointString(point)}`)
    .join(" ");
}

function buildSegmentPathData(segments) {
  return segments
    .map((segment) => `M ${pointString(segment.start)} L ${pointString(segment.end)}`)
    .join(" ");
}

function renderCells(model, showCells, showLabels) {
  if (!showCells && !showLabels) return "";
  const polygons = showCells
    ? model.cells
        .map(
          (cell) =>
            `    <polygon class="s2-cell" points="${polygonPoints(cell.vertices)}" data-s2-token="${cell.token}" data-s2-level="${cell.level}" data-index="${cell.index}" />`,
        )
        .join("\n")
    : "";
  const labels = showLabels
    ? model.cells
        .map(
          (cell) =>
            `    <text class="s2-label" x="${formatNumber(cell.centre.x)}" y="${formatNumber(cell.centre.y)}" data-s2-token="${cell.token}" data-index="${cell.index}">${escapeXml(cell.displayToken)}</text>`,
        )
        .join("\n")
    : "";
  return `<g id="cells">\n${[polygons, labels].filter(Boolean).join("\n")}\n  </g>`;
}

function renderTraversal(model, markerId) {
  const fullPath = buildPathData(model.path);
  if (!model.classifiedSegments) {
    return `<g id="hilbert-path">
    <path class="hilbert-path hilbert-unclassified" d="${fullPath}" />
  </g>`;
  }

  return `<g id="hilbert-water">
    <path class="hilbert-path hilbert-water" d="${buildSegmentPathData(model.classifiedSegments.water)}" />
  </g>
  <g id="hilbert-land">
    <path class="hilbert-path hilbert-land" d="${buildSegmentPathData(model.classifiedSegments.land)}" />
  </g>
  <path class="hilbert-direction" d="${fullPath}" marker-end="url(#${markerId})" />`;
}

/** Render a standalone, editable SVG from the geometry model. */
export function renderHilbertSvg(model, options = {}) {
  const showCells = options.showCells ?? true;
  const showLabels = options.showLabels ?? false;
  const title = options.title ?? `${model.parent.displayToken} L${model.targetLevel} S2 traversal`;
  const description =
    options.description ??
    `${model.cells.length} S2 descendants connected in ascending CellID Hilbert order.`;
  const markerId = options.markerId ?? "hilbert-arrow";
  const [viewX, viewY, viewWidth, viewHeight] = model.projection.viewBox;
  const start = model.path[0];
  const end = model.path.at(-1);
  const cells = renderCells(model, showCells, showLabels);

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${formatNumber(model.projection.width)}" height="${formatNumber(model.projection.height)}" viewBox="${[viewX, viewY, viewWidth, viewHeight].map(formatNumber).join(" ")}" role="img" aria-labelledby="svg-title svg-description" data-s2-token="${model.parent.token}" data-s2-level="${model.parentLevel}" data-target-level="${model.targetLevel}">
  <title id="svg-title">${escapeXml(title)}</title>
  <desc id="svg-description">${escapeXml(description)}</desc>
  <defs>
    <marker id="${markerId}" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--hilbert-marker, #d1495b)" />
    </marker>
    <style>${options.styles ?? DEFAULT_STYLES}</style>
  </defs>
  <g id="parent-cell">
    <polygon class="s2-parent" points="${polygonPoints(model.parent.vertices)}" data-s2-token="${model.parent.token}" data-s2-level="${model.parentLevel}" />
  </g>
  ${cells}
  ${renderTraversal(model, markerId)}
  <g id="markers">
    <circle class="hilbert-start" cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="7" data-index="0" data-s2-token="${start.token}" />
    <circle class="hilbert-end" cx="${formatNumber(end.x)}" cy="${formatNumber(end.y)}" r="5" data-index="${end.index}" data-s2-token="${end.token}" />
  </g>
</svg>
`;
}

const ORIENTATION_STYLES = `
  .legend-panel { fill: #f8fbfc; stroke: #c7d7de; stroke-width: 1; }
  .legend-parent { fill: none; stroke: #315a70; stroke-width: 1.5; }
  .legend-path { fill: none; stroke: #006d77; stroke-width: 4; stroke-linecap: round; stroke-linejoin: round; }
  .legend-start { fill: #fff; stroke: #d1495b; stroke-width: 2.5; }
  .legend-end { fill: #d1495b; }
  .legend-title { fill: #163747; font: 600 16px system-ui, sans-serif; text-anchor: middle; }
  .legend-token { fill: #506b78; font: 12px ui-monospace, SFMono-Regular, Menlo, monospace; text-anchor: middle; }
`;

/** Render the four S2 library orientation states using real example cells. */
export function renderOrientationLegendSvg(entries, options = {}) {
  const panelWidth = options.panelWidth ?? 260;
  const panelHeight = options.panelHeight ?? 300;
  const width = panelWidth * entries.length;
  const height = panelHeight;
  const markerId = "legend-arrow";
  const panels = entries
    .map(({ label, model, orientation }, index) => {
      const offsetX = index * panelWidth;
      const path = buildPathData(model.path);
      const start = model.path[0];
      const end = model.path.at(-1);
      return `<g id="orientation-${orientation}" transform="translate(${offsetX} 0)" data-orientation="${orientation}" data-s2-token="${model.parent.token}">
    <rect class="legend-panel" x="8" y="8" width="${panelWidth - 16}" height="${panelHeight - 16}" rx="8" />
    <text class="legend-title" x="${panelWidth / 2}" y="32">${escapeXml(label)}</text>
    <text class="legend-token" x="${panelWidth / 2}" y="52">${escapeXml(model.parent.displayToken)}</text>
    <g transform="translate(${(panelWidth - 190) / 2} 64)">
      <polygon class="legend-parent" points="${polygonPoints(model.parent.vertices)}" />
      <path class="legend-path" d="${path}" marker-end="url(#${markerId})" />
      <circle class="legend-start" cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="6" />
      <circle class="legend-end" cx="${formatNumber(end.x)}" cy="${formatNumber(end.y)}" r="4" />
    </g>
  </g>`;
    })
    .join("\n  ");

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-labelledby="legend-title legend-description">
  <title id="legend-title">S2 Hilbert orientation states</title>
  <desc id="legend-description">Four actual S2 level 10 cells whose level 12 descendants demonstrate the canonical, swapped, inverted, and swapped plus inverted orientation states.</desc>
  <defs>
    <marker id="${markerId}" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 0 L 10 5 L 0 10 z" fill="#d1495b" />
    </marker>
    <style>${ORIENTATION_STYLES}</style>
  </defs>
  ${panels}
</svg>
`;
}

export { DEFAULT_STYLES };
