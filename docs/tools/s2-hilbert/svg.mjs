const DEFAULT_STYLES = `
  :root {
    --s2-cell-stroke: #315a70;
    --s2-parent-stroke: #163747;
    --hilbert-stroke: #006d77;
    --hilbert-marker: #d1495b;
    --hilbert-start-stroke: #1b7f5a;
    --hilbert-chevron: #023e4a;
    --hilbert-label: #163747;
  }
  .s2-cell { fill: none; stroke: var(--s2-cell-stroke); stroke-width: 0.8; opacity: 0.42; }
  .s2-parent { fill: none; stroke: var(--s2-parent-stroke); stroke-width: 2; }
  .hilbert-path { fill: none; stroke: var(--hilbert-stroke); stroke-width: 5; stroke-linecap: round; stroke-linejoin: round; }
  .hilbert-land { opacity: 1; }
  .hilbert-water { opacity: 0.3; }
  .hilbert-unclassified { opacity: 1; }
  .hilbert-direction { fill: none; stroke: transparent; stroke-width: 1; }
  .hilbert-chevron { fill: none; stroke: var(--hilbert-chevron); stroke-width: 2.6; stroke-linecap: round; stroke-linejoin: round; }
  .hilbert-start { fill: #fff; stroke: var(--hilbert-start-stroke); stroke-width: 4; }
  .hilbert-start-core { fill: var(--hilbert-start-stroke); stroke: none; }
  .hilbert-end { fill: var(--hilbert-marker); stroke: #fff; stroke-width: 2; }
  .endpoint-label { font: 700 12px system-ui, sans-serif; text-anchor: middle; dominant-baseline: central; paint-order: stroke; stroke: #fff; stroke-width: 3.5; stroke-linejoin: round; }
  .endpoint-label-start { fill: var(--hilbert-start-stroke); }
  .endpoint-label-end { fill: var(--hilbert-marker); }
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

/**
 * One chevron per straight run, placed at the midpoint and pointing the way the
 * traversal travels. Emitted as absolute coordinates rather than a transform so
 * a designer can nudge an individual chevron without unpicking a transform stack.
 */
export function buildChevrons(points, options = {}) {
  const size = options.size ?? 9;
  const spread = options.spread ?? 0.72;
  const classifications = options.classifications ?? null;
  const chevrons = [];

  for (let index = 0; index < points.length - 1; index += 1) {
    const from = points[index];
    const to = points[index + 1];
    const deltaX = to.x - from.x;
    const deltaY = to.y - from.y;
    const length = Math.hypot(deltaX, deltaY);
    if (length <= Number.EPSILON) continue;

    const unitX = deltaX / length;
    const unitY = deltaY / length;
    const reach = Math.min(size, length / 2);
    const midX = (from.x + to.x) / 2;
    const midY = (from.y + to.y) / 2;
    const tipX = midX + unitX * reach * 0.5;
    const tipY = midY + unitY * reach * 0.5;
    const backX = tipX - unitX * reach;
    const backY = tipY - unitY * reach;
    const wingX = -unitY * reach * spread;
    const wingY = unitX * reach * spread;

    chevrons.push({
      fromIndex: index,
      toIndex: index + 1,
      classification: classifications ? classifications[index] : null,
      points: [
        { x: backX + wingX, y: backY + wingY },
        { x: tipX, y: tipY },
        { x: backX - wingX, y: backY - wingY },
      ],
    });
  }
  return chevrons;
}

function renderChevrons(model) {
  const chevrons = buildChevrons(model.path, {
    classifications: model.segmentClassifications,
  });
  if (chevrons.length === 0) return "";
  const markup = chevrons
    .map((chevron) => {
      const classification = chevron.classification
        ? ` hilbert-${chevron.classification}`
        : "";
      return `    <polyline class="hilbert-chevron${classification}" points="${polygonPoints(chevron.points)}" data-from-index="${chevron.fromIndex}" data-to-index="${chevron.toIndex}" />`;
    })
    .join("\n");
  return `<g id="chevrons">\n${markup}\n  </g>`;
}

/** Step `distance` along from->to, measured from whichever end anchors the label. */
function offsetAlong(from, to, distance, fromEnd = false) {
  const deltaX = to.x - from.x;
  const deltaY = to.y - from.y;
  const length = Math.hypot(deltaX, deltaY);
  const origin = fromEnd ? to : from;
  if (length <= Number.EPSILON) return { x: origin.x, y: origin.y - Math.abs(distance) };
  return {
    x: origin.x + (deltaX / length) * distance,
    y: origin.y + (deltaY / length) * distance,
  };
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
    <path class="hilbert-path hilbert-unclassified" d="${fullPath}" marker-end="url(#${markerId})" />
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
  const showEndpointLabels = options.showEndpointLabels ?? true;
  const title = options.title ?? `${model.parent.displayToken} L${model.targetLevel} S2 traversal`;
  const description =
    options.description ??
    `${model.cells.length} S2 descendants connected in ascending CellID Hilbert order.`;
  const markerId = options.markerId ?? "hilbert-arrow";
  const [viewX, viewY, viewWidth, viewHeight] = model.projection.viewBox;
  const start = model.path[0];
  const end = model.path.at(-1);
  const cells = renderCells(model, showCells, showLabels);
  const labelOffset = options.endpointLabelOffset ?? 22;
  // Push each label along the traversal's own direction — backwards from the
  // start, onwards past the end — so neither lands on top of the arrowhead.
  const startAnchor = offsetAlong(start, model.path[1], -labelOffset);
  const endAnchor = offsetAlong(model.path.at(-2), end, labelOffset, true);
  const endpointLabels = showEndpointLabels
    ? `    <text class="endpoint-label endpoint-label-start" x="${formatNumber(startAnchor.x)}" y="${formatNumber(startAnchor.y)}">START</text>
    <text class="endpoint-label endpoint-label-end" x="${formatNumber(endAnchor.x)}" y="${formatNumber(endAnchor.y)}">END</text>`
    : "";

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${formatNumber(model.projection.width)}" height="${formatNumber(model.projection.height)}" viewBox="${[viewX, viewY, viewWidth, viewHeight].map(formatNumber).join(" ")}" role="img" aria-labelledby="svg-title svg-description" data-s2-token="${model.parent.token}" data-s2-level="${model.parentLevel}" data-target-level="${model.targetLevel}" data-classification-source="${model.classificationSource ?? "none"}">
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
  ${renderChevrons(model)}
  <g id="markers">
    <circle class="hilbert-start" cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="10" data-index="0" data-s2-token="${start.token}" />
    <circle class="hilbert-start-core" cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="3.5" data-index="0" />
    <circle class="hilbert-end" cx="${formatNumber(end.x)}" cy="${formatNumber(end.y)}" r="7" data-index="${end.index}" data-s2-token="${end.token}" />
${endpointLabels}
  </g>
</svg>
`;
}

const ORIENTATION_STYLES = `
  .legend-panel { fill: #f8fbfc; stroke: #c7d7de; stroke-width: 1; }
  .legend-parent { fill: none; stroke: #315a70; stroke-width: 1.5; }
  .legend-path { fill: none; stroke: #006d77; stroke-width: 4; stroke-linecap: round; stroke-linejoin: round; }
  .legend-chevron { fill: none; stroke: #023e4a; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
  .legend-start { fill: #fff; stroke: #1b7f5a; stroke-width: 3; }
  .legend-start-core { fill: #1b7f5a; }
  .legend-end { fill: #d1495b; stroke: #fff; stroke-width: 1.5; }
  .legend-title { fill: #163747; font: 600 16px system-ui, sans-serif; text-anchor: middle; }
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
    <text class="legend-title" x="${panelWidth / 2}" y="36">${escapeXml(label)}</text>
    <g transform="translate(${(panelWidth - 190) / 2} 58)">
      <polygon class="legend-parent" points="${polygonPoints(model.parent.vertices)}" />
      <path class="legend-path" d="${path}" marker-end="url(#${markerId})" />
      <g class="legend-chevrons">
${buildChevrons(model.path, { size: 7 })
  .map(
    (chevron) =>
      `        <polyline class="legend-chevron" points="${polygonPoints(chevron.points)}" data-from-index="${chevron.fromIndex}" />`,
  )
  .join("\n")}
      </g>
      <circle class="legend-start" cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="7" />
      <circle class="legend-start-core" cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="2.5" />
      <circle class="legend-end" cx="${formatNumber(end.x)}" cy="${formatNumber(end.y)}" r="5" />
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

const COMPOSITE_STYLES = `
  .s2-parent { fill: none; stroke: #163747; stroke-width: 2; }
  .s2-cell-faint { fill: none; stroke: #315a70; stroke-width: 0.6; opacity: 0.28; }
  .tile-selected { fill: #006d77; fill-opacity: 0.16; stroke: #006d77; stroke-width: 1.2; stroke-opacity: 0.55; }
  .hilbert-detail { fill: none; stroke: #006d77; stroke-linecap: round; stroke-linejoin: round; }
  .hilbert-detail-light { stroke-width: 2.2; opacity: 0.3; }
  .hilbert-detail-heavy { stroke-width: 5; opacity: 1; }
  .hilbert-coarse { fill: none; stroke: #8c5a2b; stroke-width: 5.5; stroke-linecap: round; stroke-linejoin: round; }
  .hilbert-coarse-light { stroke-width: 3.2; opacity: 0.45; }
  .chain-connector { fill: none; stroke: #8c5a2b; stroke-width: 5.5; stroke-linecap: round; stroke-linejoin: round; }
  .chain-connector-jump { stroke-width: 2.4; opacity: 0.35; stroke-dasharray: 10 9; }
  .composite-chevron { fill: none; stroke: #5d3a18; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; }
  .composite-chevron-light { stroke: #8c5a2b; opacity: 0.45; stroke-width: 1.8; }
  .composite-start { fill: #fff; stroke: #1b7f5a; stroke-width: 3; }
  .composite-start-core { fill: #1b7f5a; }
  .composite-end { fill: #d1495b; stroke: #fff; stroke-width: 1.5; }
  .panel-label { fill: #5d3a18; font: 700 26px system-ui, sans-serif; text-anchor: middle; dominant-baseline: central; paint-order: stroke; stroke: #fff; stroke-width: 5; stroke-linejoin: round; }
  .panel-label-hero { fill: #006d77; }
`;

function renderChevronSet(points, className, size) {
  return buildChevrons(points, { size })
    .map(
      (chevron) =>
        `      <polyline class="${className}" points="${polygonPoints(chevron.points)}" data-from-index="${chevron.fromIndex}" />`,
    )
    .join("\n");
}

function renderEndpoints(path) {
  const start = path[0];
  const end = path.at(-1);
  return `      <circle class="composite-start" cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="8" data-index="0" data-s2-token="${start.token}" />
      <circle class="composite-start-core" cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="3" />
      <circle class="composite-end" cx="${formatNumber(end.x)}" cy="${formatNumber(end.y)}" r="6" data-index="${end.index}" data-s2-token="${end.token}" />`;
}

/**
 * S2 cells project as sheared parallelograms, so the topmost vertex can sit far
 * off to one side and even over a neighbour. Anchor the label to the midpoint
 * of the cell's top EDGE and pull it back towards the centre, which keeps it
 * inside its own cell in a tiled composite.
 */
function topLabelAnchor(vertices, centre, inset) {
  const edges = vertices.map((vertex, index) => {
    const next = vertices[(index + 1) % vertices.length];
    return { x: (vertex.x + next.x) / 2, y: (vertex.y + next.y) / 2 };
  });
  const topEdge = edges.reduce((best, edge) => (edge.y < best.y ? edge : best));
  return {
    x: centre.x + (topEdge.x - centre.x) * inset,
    y: centre.y + (topEdge.y - centre.y) * inset,
  };
}

function renderCompositePanel(panel, options) {
  const { labelInset, chevronSize } = options;
  const isHero = panel.role === "hero";
  const anchor = labelAnchor(panel.parent, labelInset);
  const angle = cellSlantDegrees(panel.parent.vertices);
  const label = panel.showLabel
    ? `      <text class="panel-label${isHero ? " panel-label-hero" : ""}" transform="translate(${formatNumber(anchor.x)} ${formatNumber(anchor.y)}) rotate(${formatNumber(angle)})">${escapeXml(panel.label)}</text>`
    : "";

  // The shaded cells sit underneath so the detail run reads on top of them.
  const tiles = panel.selectedCells
    ? `      <g class="selected-tiles">
${panel.selectedCells
  .map(
    (cell) =>
      `        <polygon class="tile-selected" points="${polygonPoints(cell.vertices)}" data-s2-token="${cell.token}" data-s2-level="${cell.level}" data-index="${cell.index}" />`,
  )
  .join("\n")}
      </g>`
    : "";

  // Every panel shows its full detail run. Weight is the only thing that varies:
  // heavy across shaded cells where a selection says so, light everywhere else.
  const detail = panel.detail.classifications
    ? `      <path class="hilbert-detail hilbert-detail-light" d="${weightedPathData(panel.detail.path, panel.detail.classifications, "water")}" data-s2-level="${panel.detail.level}" />
      <path class="hilbert-detail hilbert-detail-heavy" d="${weightedPathData(panel.detail.path, panel.detail.classifications, "land")}" data-s2-level="${panel.detail.level}" />`
    : `      <path class="hilbert-detail hilbert-detail-light" d="${buildPathData(panel.detail.path)}" data-s2-level="${panel.detail.level}" />`;

  const coarseClass = isHero ? "hilbert-coarse hilbert-coarse-light" : "hilbert-coarse";
  const chevronClass = isHero
    ? "composite-chevron composite-chevron-light"
    : "composite-chevron";

  return `    <g id="panel-${panel.parent.token}" data-s2-token="${panel.parent.token}" data-s2-level="${panel.parent.level}" data-role="${panel.role}" data-orientation-label="${escapeXml(panel.label)}">
      <polygon class="s2-parent" points="${polygonPoints(panel.parent.vertices)}" />
${[tiles, detail].filter(Boolean).join("\n")}
      <path class="${coarseClass}" d="${buildPathData(panel.coarse.path)}" data-s2-level="${panel.coarse.level}" />
      <g class="composite-chevrons">
${renderChevronSet(panel.coarse.path, chevronClass, chevronSize)}
      </g>
${label}
    </g>`;
}

/** Draw only the runs with the given classification, as one multi-move path. */
function weightedPathData(path, classifications, wanted) {
  const pieces = [];
  classifications.forEach((classification, index) => {
    if (classification !== wanted) return;
    pieces.push(
      `M ${pointString(path[index])} L ${pointString(path[index + 1])}`,
    );
  });
  return pieces.join(" ");
}

/** Centre of the cell, nudged towards its top edge by `inset` (0 keeps centre). */
function labelAnchor(parent, inset) {
  if (!inset) return { x: parent.centre.x, y: parent.centre.y };
  const edges = parent.vertices.map((vertex, index) => {
    const next = parent.vertices[(index + 1) % parent.vertices.length];
    return { x: (vertex.x + next.x) / 2, y: (vertex.y + next.y) / 2 };
  });
  const topEdge = edges.reduce((best, edge) => (edge.y < best.y ? edge : best));
  return {
    x: parent.centre.x + (topEdge.x - parent.centre.x) * inset,
    y: parent.centre.y + (topEdge.y - parent.centre.y) * inset,
  };
}

/**
 * The slant of the cell's top edge, in degrees. S2 cells project as sheared
 * parallelograms, so a label set on this angle lies along the cell rather than
 * cutting across it.
 */
export function cellSlantDegrees(vertices) {
  const edges = vertices.map((vertex, index) => {
    const next = vertices[(index + 1) % vertices.length];
    return {
      midY: (vertex.y + next.y) / 2,
      dx: next.x - vertex.x,
      dy: next.y - vertex.y,
    };
  });
  const topEdge = edges.reduce((best, edge) => (edge.midY < best.midY ? edge : best));
  // Read the edge left-to-right so text never comes out upside down.
  const dx = topEdge.dx < 0 ? -topEdge.dx : topEdge.dx;
  const dy = topEdge.dx < 0 ? -topEdge.dy : topEdge.dy;
  return (Math.atan2(dy, dx) * 180) / Math.PI;
}

function renderChainConnectors(model, markerId) {
  if (!model.chain || model.chain.links.length === 0) return "";
  const links = model.chain.links
    .map(
      (link) =>
        `      <path class="chain-connector${link.continuous ? "" : " chain-connector-jump"}" d="M ${pointString(link.start)} L ${pointString(link.end)}" data-from="${link.fromToken}" data-to="${link.toToken}" data-continuous="${link.continuous}" data-s2-consecutive="${link.s2Consecutive}" />`,
    )
    .join("\n");
  return `    <g id="chain-connectors">
${links}
    </g>`;
}

/**
 * Only the ends of the whole chain get markers. Interior junctions are where one
 * panel's end meets the next panel's start, and marking both would put a stop
 * and a start on a line that simply carries on.
 */
function renderChainEndpoints(model) {
  if (!model.chain) return "";
  const first = model.panels[model.chain.order[0]].coarse.path[0];
  const last = model.panels[model.chain.order.at(-1)].coarse.path.at(-1);
  return `    <g id="chain-markers">
      <circle class="composite-start" cx="${formatNumber(first.x)}" cy="${formatNumber(first.y)}" r="9" data-s2-token="${first.token}" />
      <circle class="composite-start-core" cx="${formatNumber(first.x)}" cy="${formatNumber(first.y)}" r="3.5" />
      <circle class="composite-end" cx="${formatNumber(last.x)}" cy="${formatNumber(last.y)}" r="7" data-s2-token="${last.token}" />
    </g>`;
}

/**
 * Render several adjacent L10 parents as one figure. Positions come from the
 * shared projection, so the cells join because S2 says they do.
 */
export function renderCompositeSvg(model, options = {}) {
  const markerId = options.markerId ?? "composite-arrow";
  const panelOptions = {
    labelInset: options.labelInset ?? 0,
    chevronSize: options.chevronSize ?? 10,
  };
  const [viewX, viewY, viewWidth, viewHeight] = model.projection.viewBox;
  const title = options.title ?? "Adjacent S2 L10 cells and their Hilbert orientations";
  const description =
    options.description ??
    `${model.panels.length} adjacent S2 level 10 cells sharing one Web Mercator projection.`;

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${formatNumber(model.projection.width)}" height="${formatNumber(model.projection.height)}" viewBox="${[viewX, viewY, viewWidth, viewHeight].map(formatNumber).join(" ")}" role="img" aria-labelledby="composite-title composite-description">
  <title id="composite-title">${escapeXml(title)}</title>
  <desc id="composite-description">${escapeXml(description)}</desc>
  <defs>
    <marker id="${markerId}" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
      <path d="M 0 0 L 10 5 L 0 10 z" fill="#8c5a2b" />
    </marker>
    <style>${options.styles ?? COMPOSITE_STYLES}</style>
  </defs>
  <g id="panels">
${model.panels.map((panel) => renderCompositePanel(panel, panelOptions)).join("\n")}
  </g>
${renderChainConnectors(model, markerId)}
${renderChainEndpoints(model)}
</svg>
`;
}

export { COMPOSITE_STYLES };
