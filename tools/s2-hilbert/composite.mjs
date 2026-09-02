import {
  classifySegmentsBySelection,
  parseCellSelection,
  unknownSelectionTokens,
} from "./cell-selection.mjs";
import {
  cellIdToGeographicGeometry,
  getHilbertTraversal,
  parseCanonicalToken,
  s2,
} from "./hierarchy.mjs";
import { projectTraversals, projectWebMercator } from "./projection.mjs";

/** The S2 swap and invert bits, named. Derived from the cell, never guessed. */
export const ORIENTATION_NAMES = [
  "canonical",
  "swapped",
  "inverted",
  "swapped + inverted",
];

export function orientationName(token) {
  const cell = s2.Cell.fromCellID(parseCanonicalToken(token));
  return ORIENTATION_NAMES[cell.orientation];
}

/**
 * Build one geometry model covering several adjacent L10 parents. Every panel
 * shares a single projection, so neighbouring S2 cells join along their real
 * shared edges rather than being nudged into place.
 */
export function buildS2CompositeModel(options) {
  if (
    !options ||
    !Array.isArray(options.panels) ||
    options.panels.length === 0
  ) {
    throw new TypeError(
      "buildS2CompositeModel requires a non-empty panels list.",
    );
  }

  const specs = options.panels.map((panel) => ({
    coarseLevel: panel.coarseLevel ?? 12,
    detailLevel: panel.detailLevel ?? 13,
    ...panel,
  }));

  // Two traversals per panel, coarse then detail, through one shared fit.
  const traversals = specs.flatMap((spec) => [
    getHilbertTraversal(spec.parent, spec.coarseLevel),
    getHilbertTraversal(spec.parent, spec.detailLevel),
  ]);
  const projected = projectTraversals(traversals, options);

  const panels = specs.map((spec, index) => {
    const coarse = projected.traversals[index * 2];
    const detail = projected.traversals[index * 2 + 1];

    let selection = null;
    let selectedCells = null;
    if (spec.cellSelection) {
      selection = parseCellSelection(spec.cellSelection);
      const unknown = unknownSelectionTokens(detail.path, selection);
      if (unknown.length > 0) {
        throw new Error(
          `The selection for ${spec.parent} names ${unknown.length} token(s) that are not L${spec.detailLevel} descendants: ${unknown.slice(0, 4).join(", ")}.`,
        );
      }
      selectedCells = detail.cells.filter(
        (cell) => selection.get(cell.token) === true,
      );
    }

    // With a selection the detail run varies in weight: heavy across the shaded
    // cells, light everywhere else. Without one it is drawn light throughout.
    const detailClassifications = selection
      ? classifySegmentsBySelection(detail.path, selection)
      : null;

    return {
      parent: coarse.parent,
      role: spec.role ?? "context",
      label: spec.label ?? orientationName(coarse.parent.token),
      showLabel: spec.showLabel ?? false,
      showOffMapContinuations: spec.showOffMapContinuations ?? false,
      coarse: {
        level: spec.coarseLevel,
        cells: coarse.cells,
        path: coarse.path,
      },
      detail: {
        level: spec.detailLevel,
        cells: detail.cells,
        path: detail.path,
        classifications: detailClassifications,
      },
      selectedCells,
      selectedCount: selectedCells ? selectedCells.length : null,
      detailSelection: selection,
    };
  });

  const projectCentreOf = (token) => {
    const geometry = cellIdToGeographicGeometry(parseCanonicalToken(token));
    return projected.fit.project(
      projectWebMercator(geometry.centre, projected.anchorLng),
    );
  };
  const connections = buildPanelLinks(panels, projectCentreOf);

  return {
    schemaVersion: 1,
    projection: {
      name: projected.fit.name,
      anchorLng: projected.anchorLng,
      width: projected.fit.width,
      height: projected.fit.height,
      padding: projected.fit.padding,
      viewBox: projected.fit.viewBox,
      sourceBounds: projected.fit.sourceBounds,
      scale: projected.fit.scale,
    },
    panels,
    connections,
  };
}

const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y);

/**
 * Where the S2 curve actually enters and leaves each cell.
 *
 * This is read from the curve itself: the predecessor and successor of a cell
 * are `cellid.prev` and `cellid.next` at its own level. Only when that
 * neighbour happens to be another panel is there a link to draw inside the
 * figure. Every other entry and exit leaves the mapped area, and is shown as a
 * stub pointing at wherever the real neighbour lies. Nothing here is inferred
 * from how close two panels happen to look.
 */
export function buildPanelLinks(panels, projectCentreOf) {
  const tokenToPanel = new Map(
    panels.map((panel, index) => [panel.parent.token, index]),
  );
  const stepLengths = panels.flatMap((panel) =>
    panel.coarse.path
      .slice(0, -1)
      .map((point, index) => distance(point, panel.coarse.path[index + 1])),
  );
  const typicalStep = stepLengths.sort((a, b) => a - b)[
    Math.floor(stepLengths.length / 2)
  ];

  const links = [];
  const stubs = [];

  panels.forEach((panel) => {
    const id = parseCanonicalToken(panel.parent.token);
    const previousToken = s2.cellid.toToken(s2.cellid.prev(id));
    const nextToken = s2.cellid.toToken(s2.cellid.next(id));

    if (tokenToPanel.has(nextToken)) {
      const target = panels[tokenToPanel.get(nextToken)];
      links.push({
        fromToken: panel.parent.token,
        toToken: nextToken,
        // Both runs cross the join, so both are drawn across it.
        coarse: { start: panel.coarse.path.at(-1), end: target.coarse.path[0] },
        detail: { start: panel.detail.path.at(-1), end: target.detail.path[0] },
        // Heavy only if both cells it joins are selected in their own panel.
        detailHeavy: Boolean(
          panel.detailSelection?.get(panel.detail.path.at(-1).token) === true &&
          target.detailSelection?.get(target.detail.path[0].token) === true,
        ),
        length: distance(panel.coarse.path.at(-1), target.coarse.path[0]),
      });
    } else {
      stubs.push(
        makeStub(
          panel,
          "exit",
          nextToken,
          projectCentreOf(nextToken),
          typicalStep,
        ),
      );
    }

    // An entry is drawn only when the predecessor is off the page; otherwise its
    // own exit link already draws that joint.
    if (!tokenToPanel.has(previousToken)) {
      stubs.push(
        makeStub(
          panel,
          "entry",
          previousToken,
          projectCentreOf(previousToken),
          typicalStep,
        ),
      );
    }
  });

  return { links, stubs, typicalStep };
}

/**
 * The unit vector along whichever of the cell's own edges best matches `delta`.
 *
 * The brown run only ever travels along the cell's two edge directions, so a
 * stub aimed straight at a neighbour's centre comes out at some diagonal that
 * does not belong to the drawing. Snapping to an edge axis keeps every stub
 * parallel to the run it continues, while still leaving by the side the S2
 * neighbour actually lies on.
 */
export function snapToCellAxis(vertices, delta) {
  const axes = [];
  for (let index = 0; index < 2; index += 1) {
    const from = vertices[index];
    const to = vertices[index + 1];
    const length = Math.hypot(to.x - from.x, to.y - from.y) || 1;
    const unit = { x: (to.x - from.x) / length, y: (to.y - from.y) / length };
    axes.push(unit, { x: -unit.x, y: -unit.y });
  }
  return axes.reduce((best, axis) =>
    axis.x * delta.x + axis.y * delta.y > best.x * delta.x + best.y * delta.y
      ? axis
      : best,
  );
}

function makeStub(panel, kind, neighbourToken, neighbourCentre, typicalStep) {
  const anchor =
    kind === "entry" ? panel.coarse.path[0] : panel.coarse.path.at(-1);
  const delta = {
    x: neighbourCentre.x - anchor.x,
    y: neighbourCentre.y - anchor.y,
  };
  const axis = snapToCellAxis(panel.parent.vertices, delta);
  const outer = {
    x: anchor.x + axis.x * typicalStep,
    y: anchor.y + axis.y * typicalStep,
  };
  return {
    kind,
    panelToken: panel.parent.token,
    neighbourToken,
    // An entry runs inwards and an exit runs outwards, so direction of travel is
    // always start to end, exactly as it is for the run itself.
    start: kind === "entry" ? outer : anchor,
    end: kind === "entry" ? anchor : outer,
    bearing: compassBearing(delta.x, delta.y),
  };
}

/** Screen-space compass bearing, with y increasing downwards as SVG has it. */
export function compassBearing(deltaX, deltaY) {
  const vertical =
    Math.abs(deltaY) > Math.abs(deltaX) * 0.4 ? (deltaY < 0 ? "N" : "S") : "";
  const horizontal =
    Math.abs(deltaX) > Math.abs(deltaY) * 0.4 ? (deltaX > 0 ? "E" : "W") : "";
  return `${vertical}${horizontal}` || "same";
}

/** Report which panels share an edge, so a layout can be checked, not assumed. */
export function compositeAdjacency(model) {
  const key = (vertex) => `${vertex.lat.toFixed(9)},${vertex.lng.toFixed(9)}`;
  const adjacency = [];
  for (let left = 0; left < model.panels.length; left += 1) {
    for (let right = left + 1; right < model.panels.length; right += 1) {
      const leftVertices = new Set(model.panels[left].parent.vertices.map(key));
      const shared = model.panels[right].parent.vertices.filter((vertex) =>
        leftVertices.has(key(vertex)),
      ).length;
      if (shared > 0) {
        adjacency.push({
          a: model.panels[left].parent.token,
          b: model.panels[right].parent.token,
          sharedVertices: shared,
          relation: shared >= 2 ? "edge" : "corner",
        });
      }
    }
  }
  return adjacency;
}
