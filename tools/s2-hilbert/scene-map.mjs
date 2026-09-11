/**
 * Scene map: where the published scenes are, and which S2 cell indexes each one.
 *
 * A site's position is the only hand-entered geographic fact. Every token and
 * every polygon here is derived from that position through the S2 library, so
 * the map cannot disagree with the index it draws. A site without an accepted
 * position is carried through as unlocated rather than placed somewhere
 * plausible.
 *
 * The map draws real S2 geometry and nothing else — no tiles, no basemap, no
 * third-party requests. A reader of a privacy-preserving project's site should
 * not have their address book out to a map host to look at a junction.
 */
import { cellIdToGeographicGeometry, formatDisplayToken, s2 } from "./hierarchy.mjs";
import { createSvgFit, projectWebMercator } from "./projection.mjs";

/** Levels the index is kept at, per the geographic-indexing guide. */
export const LEVEL_AREA = 10;
export const LEVEL_NEIGHBOURHOOD = 13;
export const LEVEL_SITE = 16;

/** Web Mercator world units are the whole circumference, so one unit is this. */
const EQUATORIAL_CIRCUMFERENCE_METRES = 40075016.686;

/** Metres a scale bar may claim, longest first. */
const SCALE_STEPS_METRES = [10000, 5000, 2000, 1000, 500, 200, 100];

function assertPosition(site) {
  const { lat, lon } = site.position;
  if (!Number.isFinite(lat) || !Number.isFinite(lon)) {
    throw new TypeError(`${site.id}: position lat and lon must be finite numbers.`);
  }
  if (lat < -90 || lat > 90 || lon < -180 || lon > 180) {
    throw new RangeError(`${site.id}: position ${lat},${lon} is not on Earth.`);
  }
}

/**
 * Derive a site's S2 family from its position.
 *
 * The finer levels are taken from the leaf cell and the coarser ones from
 * parent(), never by truncating a token: a token is a name for a cell, not a
 * string with a hierarchy hidden in its characters.
 */
export function deriveSiteCells(site) {
  assertPosition(site);
  const leaf = s2.cellid.fromLatLng(s2.LatLng.fromDegrees(site.position.lat, site.position.lon));
  const levels = {
    area: s2.cellid.parent(leaf, LEVEL_AREA),
    neighbourhood: s2.cellid.parent(leaf, LEVEL_NEIGHBOURHOOD),
    site: s2.cellid.parent(leaf, LEVEL_SITE),
  };

  const tokens = {};
  const geometry = {};
  for (const [name, cellId] of Object.entries(levels)) {
    const token = s2.cellid.toToken(cellId);
    tokens[name] = { token, display: formatDisplayToken(token), level: s2.cellid.level(cellId) };
    geometry[name] = cellIdToGeographicGeometry(cellId);
  }
  return { tokens, geometry };
}

function polygonPoints(vertices, anchorLng, fit) {
  return vertices
    .map((vertex) => {
      const { x, y } = fit.project(projectWebMercator({ lat: vertex.lat, lng: vertex.lng }, anchorLng));
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
}

/**
 * Choose a scale bar that is a round number of metres and fits the drawing.
 *
 * Web Mercator stretches with latitude, so the bar is measured at the map's own
 * centre latitude rather than the equator; over a few kilometres that is the
 * difference between an honest bar and one about 25% long in San Francisco.
 */
function buildScaleBar(fit, centreLat, drawWidth) {
  const metresPerWorldUnit = EQUATORIAL_CIRCUMFERENCE_METRES * Math.cos((centreLat * Math.PI) / 180);
  const metresPerSvgUnit = metresPerWorldUnit / fit.scale;
  const maximumSvg = drawWidth * 0.28;
  const metres =
    SCALE_STEPS_METRES.find((step) => step / metresPerSvgUnit <= maximumSvg) ??
    SCALE_STEPS_METRES.at(-1);
  return {
    metres,
    label: metres >= 1000 ? `${metres / 1000} km` : `${metres} m`,
    length: metres / metresPerSvgUnit,
  };
}

/**
 * Build the map model: located sites, the cells that index them, and the
 * geometry to draw, already fitted to the viewBox.
 *
 * Unlocated sites are returned too. They are part of the catalogue even though
 * nothing can be drawn for them, and the page says so rather than quietly
 * listing fewer scenes than exist.
 */
export function buildSceneMapModel(options = {}) {
  const { sites = [], width = 900, height = 460, padding = 90 } = options;

  const enriched = sites.map((site) => {
    if (!site.position) {
      return { ...site, located: false, cells: null };
    }
    const { tokens, geometry } = deriveSiteCells(site);
    return { ...site, located: true, cells: tokens, geometry };
  });

  const located = enriched.filter((site) => site.located);
  if (located.length === 0) {
    return { sites: enriched, located: [], map: null, width, height };
  }

  const anchorLng = located[0].position.lon;

  // Fit to the cells the map exists to show. Framing the level 10 area
  // instead makes a single site an 8 km view of one small square and a lot of
  // empty dark: with no basemap under it, that box is an abstract shape rather
  // than a place. It is still drawn, and simply runs out of the frame.
  const fitPoints = [];
  for (const site of located) {
    for (const vertex of site.geometry.neighbourhood.vertices) {
      fitPoints.push(projectWebMercator({ lat: vertex.lat, lng: vertex.lng }, anchorLng));
    }
    fitPoints.push(projectWebMercator({ lat: site.position.lat, lng: site.position.lon }, anchorLng));
  }

  const fit = createSvgFit(fitPoints, { width, height, padding });

  // One polygon per distinct cell: two sites in the same area share its cell,
  // and drawing it twice would double its edge weight and read as two cells.
  const areaCells = new Map();
  const neighbourhoodCells = new Map();
  for (const site of located) {
    const area = site.geometry.area;
    if (!areaCells.has(area.token)) {
      areaCells.set(area.token, {
        token: area.token,
        display: formatDisplayToken(area.token),
        points: polygonPoints(area.vertices, anchorLng, fit),
      });
    }
    const neighbourhood = site.geometry.neighbourhood;
    if (!neighbourhoodCells.has(neighbourhood.token)) {
      neighbourhoodCells.set(neighbourhood.token, {
        token: neighbourhood.token,
        display: formatDisplayToken(neighbourhood.token),
        points: polygonPoints(neighbourhood.vertices, anchorLng, fit),
        centre: fit.project(
          projectWebMercator(
            { lat: neighbourhood.centre.lat, lng: neighbourhood.centre.lng },
            anchorLng,
          ),
        ),
      });
    }
  }

  const markers = located.map((site) => ({
    id: site.id,
    title: site.title,
    page: site.page,
    display: site.cells.neighbourhood.display,
    ...fit.project(projectWebMercator({ lat: site.position.lat, lng: site.position.lon }, anchorLng)),
  }));

  const centreLat =
    located.reduce((total, site) => total + site.position.lat, 0) / located.length;

  return {
    sites: enriched,
    located,
    width,
    height,
    map: {
      viewBox: `0 0 ${width} ${height}`,
      areaCells: [...areaCells.values()],
      neighbourhoodCells: [...neighbourhoodCells.values()],
      markers,
      scaleBar: buildScaleBar(fit, centreLat, width - padding * 2),
    },
  };
}

function escapeText(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

const STYLE = `
  .scene-map__frame { fill: none; stroke: currentColor; stroke-opacity: 0.18; }
  .scene-map__area { fill: currentColor; fill-opacity: 0.04; stroke: currentColor; stroke-opacity: 0.35; stroke-dasharray: 6 5; }
  .scene-map__area-label { fill: currentColor; fill-opacity: 0.55; font-size: 13px; }
  .scene-map__cell { fill: #34d399; fill-opacity: 0.16; stroke: #059669; stroke-width: 2; }
  .scene-map__marker { fill: #059669; stroke: #ffffff; stroke-width: 2; }
  .scene-map__title { fill: currentColor; font-size: 15px; font-weight: 600; }
  .scene-map__token { fill: currentColor; fill-opacity: 0.7; font-size: 12.5px; }
  .scene-map__scale { stroke: currentColor; stroke-opacity: 0.6; }
  .scene-map__scale-label { fill: currentColor; fill-opacity: 0.7; font-size: 12px; }
  @media (prefers-color-scheme: dark) {
    .scene-map__cell { fill: #34d399; fill-opacity: 0.2; stroke: #34d399; }
    .scene-map__marker { fill: #34d399; stroke: #0b1013; }
  }
`;

/** Render the model as a standalone, dependency-free SVG. */
export function renderSceneMapSvg(model, options = {}) {
  const { width, height, map } = model;
  const titleId = "scene-map-title";
  const descriptionId = "scene-map-desc";
  const title = options.title ?? "Published LiDAR scenes";

  if (!map) {
    const description =
      "No published scene has an accepted position yet, so there is nothing to place.";
    return [
      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}" width="100%" role="img" aria-labelledby="${titleId} ${descriptionId}" class="scene-map">`,
      `  <title id="${titleId}">${escapeText(title)}</title>`,
      `  <desc id="${descriptionId}">${escapeText(description)}</desc>`,
      `  <style>${STYLE}</style>`,
      `  <rect class="scene-map__frame" x="1" y="1" width="${width - 2}" height="${height - 2}" rx="4"/>`,
      `  <text class="scene-map__token" x="${width / 2}" y="${height / 2}" text-anchor="middle">${escapeText(description)}</text>`,
      "</svg>",
    ].join("\n");
  }

  const siteSummary = map.markers
    .map((marker) => `${marker.title} in S2 cell ${marker.display}`)
    .join("; ");
  const description = `Each published scene shown inside the S2 level ${LEVEL_NEIGHBOURHOOD} cell that indexes it, with its level ${LEVEL_AREA} area cell for context: ${siteSummary}.`;

  const lines = [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${map.viewBox}" width="100%" role="img" aria-labelledby="${titleId} ${descriptionId}" class="scene-map">`,
    `  <title id="${titleId}">${escapeText(title)}</title>`,
    `  <desc id="${descriptionId}">${escapeText(description)}</desc>`,
    `  <style>${STYLE}</style>`,
    `  <rect class="scene-map__frame" x="1" y="1" width="${width - 2}" height="${height - 2}" rx="4"/>`,
    "  <g>",
  ];

  for (const cell of map.areaCells) {
    lines.push(`    <polygon class="scene-map__area" points="${cell.points}"><title>S2 area cell ${escapeText(cell.display)}</title></polygon>`);
  }
  for (const cell of map.neighbourhoodCells) {
    lines.push(`    <polygon class="scene-map__cell" points="${cell.points}"><title>S2 cell ${escapeText(cell.display)}</title></polygon>`);
  }
  lines.push("  </g>", "  <g>");

  // Labels are placed one marker at a time, taking the first candidate that
  // does not overlap a label already placed. Two sites in adjoining cells sit
  // close enough on screen that fixed offsets put one name across another, and
  // an unreadable name is worse than a displaced one.
  const placed = [];
  // Boxes are inflated before the test: two names that merely touch read as one
  // run of text, so they need clear air between them rather than bare
  // non-overlap.
  const GAP = 14;
  const overlaps = (a, b) =>
    a.x - GAP < b.x + b.width &&
    b.x - GAP < a.x + a.width &&
    a.y - 4 < b.y + b.height &&
    b.y - 4 < a.y + a.height;

  for (const marker of map.markers) {
    const titleWidth = marker.title.length * 7.6;
    const tokenWidth = marker.display.length * 7.2;
    // Above the marker first, then below, then progressively further out.
    const candidates = [-30, 34, -62, 66, -94, 98];
    let titleY = marker.y + candidates[0];
    for (const offset of candidates) {
      const top = marker.y + offset;
      const boxes = [
        { x: marker.x - titleWidth / 2, y: top - 12, width: titleWidth, height: 15 },
        { x: marker.x - tokenWidth / 2, y: top + 3, width: tokenWidth, height: 15 },
      ];
      if (!boxes.some((box) => placed.some((other) => overlaps(box, other)))) {
        titleY = top;
        placed.push(...boxes);
        break;
      }
    }

    lines.push(
      `    <g class="scene-map__site">`,
      `      <circle class="scene-map__marker" cx="${marker.x.toFixed(2)}" cy="${marker.y.toFixed(2)}" r="6"/>`,
      `      <text class="scene-map__title" x="${marker.x.toFixed(2)}" y="${titleY.toFixed(2)}" text-anchor="middle">${escapeText(marker.title)}</text>`,
      `      <text class="scene-map__token" x="${marker.x.toFixed(2)}" y="${(titleY + 15).toFixed(2)}" text-anchor="middle">${escapeText(marker.display)}</text>`,
      `    </g>`,
    );
  }
  lines.push("  </g>");

  const barY = height - 28;
  const barX = 28;
  lines.push(
    "  <g>",
    `    <line class="scene-map__scale" x1="${barX}" y1="${barY}" x2="${(barX + map.scaleBar.length).toFixed(2)}" y2="${barY}" stroke-width="2"/>`,
    `    <line class="scene-map__scale" x1="${barX}" y1="${barY - 4}" x2="${barX}" y2="${barY + 4}" stroke-width="2"/>`,
    `    <line class="scene-map__scale" x1="${(barX + map.scaleBar.length).toFixed(2)}" y1="${barY - 4}" x2="${(barX + map.scaleBar.length).toFixed(2)}" y2="${barY + 4}" stroke-width="2"/>`,
    `    <text class="scene-map__scale-label" x="${barX}" y="${barY - 9}">${escapeText(map.scaleBar.label)}</text>`,
    "  </g>",
    "</svg>",
  );

  return lines.join("\n");
}
