import {
  COMPOSITE_TOKENS,
  HILBERT_TOKENS,
  LEGEND_TOKENS,
  tokensToAttributes,
  tokensToCss,
} from "./style-tokens.mjs";

const hilbertAttrs = (classList) =>
  tokensToAttributes(HILBERT_TOKENS, classList);
const compositeAttrs = (classList) =>
  tokensToAttributes(COMPOSITE_TOKENS, classList);
const legendAttrs = (classList) => tokensToAttributes(LEGEND_TOKENS, classList);

const DEFAULT_STYLES = `
${tokensToCss(HILBERT_TOKENS)}
`;

/**
 * Build an id namespacer. Callers embedding more than one of these SVGs inline
 * pass a distinct `idPrefix` so ids stay unique across the host document.
 */
export function idNamespace(prefix = "") {
  if (typeof prefix !== "string") {
    throw new TypeError("idPrefix must be a string.");
  }
  return (name) => `${prefix}${name}`;
}

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
    .map(
      (segment) =>
        `M ${pointString(segment.start)} L ${pointString(segment.end)}`,
    )
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

function renderChevrons(model, id) {
  const chevrons = buildChevrons(model.path, {
    classifications: model.segmentClassifications,
  });
  if (chevrons.length === 0) return "";
  const markup = chevrons
    .map((chevron) => {
      const classification = chevron.classification
        ? ` hilbert-${chevron.classification}`
        : "";
      const classList = `hilbert-chevron${classification}`;
      return `    <polyline class="${classList}"${hilbertAttrs(classList)} points="${polygonPoints(chevron.points)}" data-from-index="${chevron.fromIndex}" data-to-index="${chevron.toIndex}" />`;
    })
    .join("\n");
  return `<g id="${id("chevrons")}">\n${markup}\n  </g>`;
}

/** Step `distance` along from->to, measured from whichever end anchors the label. */
function offsetAlong(from, to, distance, fromEnd = false) {
  const deltaX = to.x - from.x;
  const deltaY = to.y - from.y;
  const length = Math.hypot(deltaX, deltaY);
  const origin = fromEnd ? to : from;
  if (length <= Number.EPSILON)
    return { x: origin.x, y: origin.y - Math.abs(distance) };
  return {
    x: origin.x + (deltaX / length) * distance,
    y: origin.y + (deltaY / length) * distance,
  };
}

function renderCells(model, showCells, showLabels, id) {
  if (!showCells && !showLabels) return "";
  const polygons = showCells
    ? model.cells
        .map(
          (cell) =>
            `    <polygon class="s2-cell"${hilbertAttrs("s2-cell")} points="${polygonPoints(cell.vertices)}" data-s2-token="${cell.token}" data-s2-level="${cell.level}" data-index="${cell.index}" />`,
        )
        .join("\n")
    : "";
  const labels = showLabels
    ? model.cells
        .map(
          (cell) =>
            `    <text class="s2-label"${hilbertAttrs("s2-label")} x="${formatNumber(cell.centre.x)}" y="${formatNumber(cell.centre.y)}" data-s2-token="${cell.token}" data-index="${cell.index}">${escapeXml(cell.displayToken)}</text>`,
        )
        .join("\n")
    : "";
  return `<g id="${id("cells")}">\n${[polygons, labels].filter(Boolean).join("\n")}\n  </g>`;
}

function renderTraversal(model, markerId, id) {
  const fullPath = buildPathData(model.path);
  if (!model.classifiedSegments) {
    return `<g id="${id("hilbert-path")}">
    <path class="hilbert-path hilbert-unclassified"${hilbertAttrs("hilbert-path hilbert-unclassified")} d="${fullPath}" marker-end="url(#${markerId})" />
  </g>`;
  }

  return `<g id="${id("hilbert-unselected")}">
    <path class="hilbert-path hilbert-unselected"${hilbertAttrs("hilbert-path hilbert-unselected")} d="${buildSegmentPathData(model.classifiedSegments.unselected)}" />
  </g>
  <g id="${id("hilbert-selected")}">
    <path class="hilbert-path hilbert-selected"${hilbertAttrs("hilbert-path hilbert-selected")} d="${buildSegmentPathData(model.classifiedSegments.selected)}" />
  </g>
  <path class="hilbert-direction" fill="none" stroke="none" d="${fullPath}" marker-end="url(#${markerId})" />`;
}

/** Render a standalone, editable SVG from the geometry model. */
export function renderHilbertSvg(model, options = {}) {
  const showCells = options.showCells ?? true;
  const showLabels = options.showLabels ?? false;
  const showEndpointLabels = options.showEndpointLabels ?? true;
  const title =
    options.title ??
    `${model.parent.displayToken} L${model.targetLevel} S2 traversal`;
  const description =
    options.description ??
    `${model.cells.length} S2 descendants connected in ascending CellID Hilbert order.`;
  // Namespacing every generated id lets several of these SVGs be inlined into
  // one document without their aria-labelledby and url(#marker) references
  // resolving to whichever copy happens to come first. Empty by default, so a
  // standalone file is unchanged.
  const id = idNamespace(options.idPrefix);
  const markerId = options.markerId ?? id("hilbert-arrow");
  const [viewX, viewY, viewWidth, viewHeight] = model.projection.viewBox;
  const start = model.path[0];
  const end = model.path.at(-1);
  const cells = renderCells(model, showCells, showLabels, id);
  const labelOffset = options.endpointLabelOffset ?? 22;
  // Push each label along the traversal's own direction — backwards from the
  // start, onwards past the end — so neither lands on top of the arrowhead.
  const startAnchor = offsetAlong(start, model.path[1], -labelOffset);
  const endAnchor = offsetAlong(model.path.at(-2), end, labelOffset, true);
  const endpointLabels = showEndpointLabels
    ? `    <text class="endpoint-label endpoint-label-start"${hilbertAttrs("endpoint-label endpoint-label-start")} x="${formatNumber(startAnchor.x)}" y="${formatNumber(startAnchor.y)}">START</text>
    <text class="endpoint-label endpoint-label-end"${hilbertAttrs("endpoint-label endpoint-label-end")} x="${formatNumber(endAnchor.x)}" y="${formatNumber(endAnchor.y)}">END</text>`
    : "";

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${formatNumber(model.projection.width)}" height="${formatNumber(model.projection.height)}" viewBox="${[viewX, viewY, viewWidth, viewHeight].map(formatNumber).join(" ")}" role="img" aria-labelledby="${id("svg-title")} ${id("svg-description")}" data-s2-token="${model.parent.token}" data-s2-level="${model.parentLevel}" data-target-level="${model.targetLevel}" data-classification-source="${model.classificationSource ?? "none"}">
  <title id="${id("svg-title")}">${escapeXml(title)}</title>
  <desc id="${id("svg-description")}">${escapeXml(description)}</desc>
  <defs>
    <marker id="${markerId}" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--hilbert-marker, #d1495b)" />
    </marker>
    <style>${options.styles ?? DEFAULT_STYLES}</style>
  </defs>
  <g id="${id("parent-cell")}">
    <polygon class="s2-parent"${hilbertAttrs("s2-parent")} points="${polygonPoints(model.parent.vertices)}" data-s2-token="${model.parent.token}" data-s2-level="${model.parentLevel}" />
  </g>
  ${cells}
  ${renderTraversal(model, markerId, id)}
  ${renderChevrons(model, id)}
  <g id="${id("markers")}">
    <circle class="hilbert-start"${hilbertAttrs("hilbert-start")} cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="10" data-index="0" data-s2-token="${start.token}" />
    <circle class="hilbert-start-core"${hilbertAttrs("hilbert-start-core")} cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="3.5" data-index="0" />
    <circle class="hilbert-end"${hilbertAttrs("hilbert-end")} cx="${formatNumber(end.x)}" cy="${formatNumber(end.y)}" r="7" data-index="${end.index}" data-s2-token="${end.token}" />
${endpointLabels}
  </g>
</svg>
`;
}

const ORIENTATION_STYLES = `
${tokensToCss(LEGEND_TOKENS)}
`;

/** Render the four S2 library orientation states using real example cells. */
export function renderOrientationLegendSvg(entries, options = {}) {
  const panelWidth = options.panelWidth ?? 260;
  const panelHeight = options.panelHeight ?? 300;
  const width = panelWidth * entries.length;
  const height = panelHeight;
  const id = idNamespace(options.idPrefix);
  const markerId = options.markerId ?? id("legend-arrow");
  const panels = entries
    .map(({ label, model, orientation }, index) => {
      const offsetX = index * panelWidth;
      const path = buildPathData(model.path);
      const start = model.path[0];
      const end = model.path.at(-1);
      return `<g id="${id(`orientation-${orientation}`)}" transform="translate(${offsetX} 0)" data-orientation="${orientation}" data-s2-token="${model.parent.token}">
    <rect class="legend-panel"${legendAttrs("legend-panel")} x="8" y="8" width="${panelWidth - 16}" height="${panelHeight - 16}" rx="8" />
    <text class="legend-title"${legendAttrs("legend-title")} x="${panelWidth / 2}" y="36">${escapeXml(label)}</text>
    <g transform="translate(${(panelWidth - 190) / 2} 58)">
      <polygon class="legend-parent"${legendAttrs("legend-parent")} points="${polygonPoints(model.parent.vertices)}" />
      <path class="legend-path"${legendAttrs("legend-path")} d="${path}" marker-end="url(#${markerId})" />
      <g class="legend-chevrons">
${buildChevrons(model.path, { size: 7 })
  .map(
    (chevron) =>
      `        <polyline class="legend-chevron"${legendAttrs("legend-chevron")} points="${polygonPoints(chevron.points)}" data-from-index="${chevron.fromIndex}" />`,
  )
  .join("\n")}
      </g>
      <circle class="legend-start"${legendAttrs("legend-start")} cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="7" />
      <circle class="legend-start-core"${legendAttrs("legend-start-core")} cx="${formatNumber(start.x)}" cy="${formatNumber(start.y)}" r="2.5" />
      <circle class="legend-end"${legendAttrs("legend-end")} cx="${formatNumber(end.x)}" cy="${formatNumber(end.y)}" r="5" />
    </g>
  </g>`;
    })
    .join("\n  ");

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-labelledby="${id("legend-title")} ${id("legend-description")}">
  <title id="${id("legend-title")}">S2 Hilbert orientation states</title>
  <desc id="${id("legend-description")}">Four actual S2 level 10 cells whose level 12 descendants demonstrate the canonical, swapped, inverted, and swapped plus inverted orientation states.</desc>
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
${tokensToCss(COMPOSITE_TOKENS)}
`;

function renderChevronSet(points, className, size) {
  return buildChevrons(points, { size })
    .map(
      (chevron) =>
        `      <polyline class="${className}"${compositeAttrs(className)} points="${polygonPoints(chevron.points)}" data-from-index="${chevron.fromIndex}" />`,
    )
    .join("\n");
}

function renderCompositePanel(panel, options, id) {
  const { labelInset, chevronSize } = options;
  const isHero = panel.role === "hero";
  const anchor = labelAnchor(panel.parent, labelInset);
  const angle = cellSlantDegrees(panel.parent.vertices);
  const label = panel.showLabel
    ? `      <text class="panel-label${isHero ? " panel-label-hero" : ""}" fill="${isHero ? "#006d77" : "#5d3a18"}" font-family="system-ui, sans-serif" font-size="26" font-weight="700" text-anchor="middle" dominant-baseline="central" paint-order="stroke" stroke="#fff" stroke-width="5" stroke-linejoin="round" transform="translate(${formatNumber(anchor.x)} ${formatNumber(anchor.y)}) rotate(${formatNumber(angle)})">${escapeXml(panel.label)}</text>`
    : "";

  // The shaded cells sit underneath so the detail run reads on top of them.
  const tiles = panel.selectedCells
    ? `      <g class="selected-tiles">
${panel.selectedCells
  .map(
    (cell) =>
      `        <polygon class="tile-selected"${compositeAttrs("tile-selected")} points="${polygonPoints(cell.vertices)}" data-s2-token="${cell.token}" data-s2-level="${cell.level}" data-index="${cell.index}" />`,
  )
  .join("\n")}
      </g>`
    : "";

  // Every panel shows its full detail run. Weight is the only thing that varies:
  // heavy across shaded cells where a selection says so, light everywhere else.
  const detail = panel.detail.classifications
    ? `      <path class="hilbert-detail hilbert-detail-light"${compositeAttrs("hilbert-detail hilbert-detail-light")} d="${weightedPathData(panel.detail.path, panel.detail.classifications, "unselected")}" data-s2-level="${panel.detail.level}" />
      <path class="hilbert-detail hilbert-detail-heavy"${compositeAttrs("hilbert-detail hilbert-detail-heavy")} d="${weightedPathData(panel.detail.path, panel.detail.classifications, "selected")}" data-s2-level="${panel.detail.level}" />`
    : `      <path class="hilbert-detail hilbert-detail-light"${compositeAttrs("hilbert-detail hilbert-detail-light")} d="${buildPathData(panel.detail.path)}" data-s2-level="${panel.detail.level}" />`;

  // One brown, one weight, everywhere. Only the blue detail run varies.
  const coarseClass = "hilbert-coarse";
  const chevronClass = "composite-chevron";

  return `    <g id="${id(`panel-${panel.parent.token}`)}" data-s2-token="${panel.parent.token}" data-s2-level="${panel.parent.level}" data-role="${panel.role}" data-orientation-label="${escapeXml(panel.label)}">
      <polygon class="s2-parent"${compositeAttrs("s2-parent")} points="${polygonPoints(panel.parent.vertices)}" />
${[tiles, detail].filter(Boolean).join("\n")}
      <path class="${coarseClass}"${compositeAttrs(coarseClass)} d="${buildPathData(panel.coarse.path)}" data-s2-level="${panel.coarse.level}" />
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
  const topEdge = edges.reduce((best, edge) =>
    edge.midY < best.midY ? edge : best,
  );
  // Read the edge left-to-right so text never comes out upside down.
  const dx = topEdge.dx < 0 ? -topEdge.dx : topEdge.dx;
  const dy = topEdge.dx < 0 ? -topEdge.dy : topEdge.dy;
  return (Math.atan2(dy, dx) * 180) / Math.PI;
}

function connectorChevrons(start, end, size) {
  return buildChevrons([start, end], { size })
    .map(
      (chevron) =>
        `      <polyline class="composite-chevron"${compositeAttrs("composite-chevron")} points="${polygonPoints(chevron.points)}" />`,
    )
    .join("\n");
}

/**
 * Links and stubs are drawn as further steps of the brown run: same colour,
 * same weight, and the same chevron for direction, so a join reads as the line
 * carrying on rather than as separate notation.
 */
function renderConnections(model, chevronSize, id) {
  const { links = [], stubs = [] } = model.connections ?? {};
  if (links.length === 0 && stubs.length === 0) return "";

  const parts = [];
  for (const link of links) {
    const detailClass = link.detailHeavy
      ? "hilbert-detail hilbert-detail-heavy"
      : "chain-detail";
    parts.push(
      `      <path class="${detailClass}"${compositeAttrs(detailClass)} d="M ${pointString(link.detail.start)} L ${pointString(link.detail.end)}" data-from="${link.fromToken}" data-to="${link.toToken}" data-run="detail" data-inside="true" />`,
      `      <path class="chain-connector"${compositeAttrs("chain-connector")} d="M ${pointString(link.coarse.start)} L ${pointString(link.coarse.end)}" data-from="${link.fromToken}" data-to="${link.toToken}" data-run="coarse" data-inside="true" />`,
      connectorChevrons(link.coarse.start, link.coarse.end, chevronSize),
    );
  }
  for (const stub of stubs) {
    parts.push(
      `      <path class="chain-stub"${compositeAttrs("chain-stub")} d="M ${pointString(stub.start)} L ${pointString(stub.end)}" data-panel="${stub.panelToken}" data-neighbour="${stub.neighbourToken}" data-kind="${stub.kind}" data-bearing="${stub.bearing}" data-inside="false" />`,
      connectorChevrons(stub.start, stub.end, chevronSize),
    );
  }

  return `    <g id="${id("connections")}">
${parts.join("\n")}
    </g>`;
}

/**
 * Render several adjacent L10 parents as one figure. Positions come from the
 * shared projection, so the cells join because S2 says they do.
 */
export function renderCompositeSvg(model, options = {}) {
  const id = idNamespace(options.idPrefix);
  const panelOptions = {
    labelInset: options.labelInset ?? 0,
    chevronSize: options.chevronSize ?? 10,
  };
  const [viewX, viewY, viewWidth, viewHeight] = model.projection.viewBox;
  const title =
    options.title ?? "Adjacent S2 L10 cells and their Hilbert orientations";
  const description =
    options.description ??
    `${model.panels.length} adjacent S2 level 10 cells sharing one Web Mercator projection.`;

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${formatNumber(model.projection.width)}" height="${formatNumber(model.projection.height)}" viewBox="${[viewX, viewY, viewWidth, viewHeight].map(formatNumber).join(" ")}" role="img" aria-labelledby="${id("composite-title")} ${id("composite-description")}">
  <title id="${id("composite-title")}">${escapeXml(title)}</title>
  <desc id="${id("composite-description")}">${escapeXml(description)}</desc>
  <defs>
    <style>${options.styles ?? COMPOSITE_STYLES}</style>
  </defs>
  <g id="${id("panels")}">
${model.panels.map((panel) => renderCompositePanel(panel, panelOptions, id)).join("\n")}
  </g>
${renderConnections(model, panelOptions.chevronSize, id)}
</svg>
`;
}

export { COMPOSITE_STYLES };
