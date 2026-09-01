import {
  classifySegmentsBySelection,
  parseCellSelection,
  unknownSelectionTokens,
} from "./cell-selection.mjs";
import { getHilbertTraversal, parseCanonicalToken, s2 } from "./hierarchy.mjs";
import { projectTraversals } from "./projection.mjs";

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
  if (!options || !Array.isArray(options.panels) || options.panels.length === 0) {
    throw new TypeError("buildS2CompositeModel requires a non-empty panels list.");
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
      selectedCells = detail.cells.filter((cell) => selection.get(cell.token) === true);
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
      showLabel: spec.showLabel ?? true,
      coarse: { level: spec.coarseLevel, cells: coarse.cells, path: coarse.path },
      detail: {
        level: spec.detailLevel,
        cells: detail.cells,
        path: detail.path,
        classifications: detailClassifications,
      },
      selectedCells,
      selectedCount: selectedCells ? selectedCells.length : null,
    };
  });

  const chain = buildPanelChain(panels, (a, b) =>
    s2.cellid.next(parseCanonicalToken(a)) === parseCanonicalToken(b),
  );

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
    chain,
  };
}

function permutations(items) {
  if (items.length <= 1) return [items];
  return items.flatMap((item, index) =>
    permutations([...items.slice(0, index), ...items.slice(index + 1)]).map((rest) => [
      item,
      ...rest,
    ]),
  );
}

const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y);

/**
 * Order the panels so each one's end sits next to the following one's start.
 *
 * These four L10 cells are geographic neighbours but are NOT all consecutive on
 * the Hilbert curve, so there is no single S2 traversal to read the order from.
 * The shortest end-to-start chain is therefore chosen from the projected
 * geometry, exhaustively and deterministically. Each link records whether S2
 * really does run one cell straight into the next.
 */
export function buildPanelChain(panels, isConsecutive) {
  const indices = panels.map((_, index) => index);
  let best = null;
  for (const order of permutations(indices)) {
    const total = order
      .slice(0, -1)
      .reduce(
        (sum, panelIndex, step) =>
          sum +
          distance(
            panels[panelIndex].coarse.path.at(-1),
            panels[order[step + 1]].coarse.path[0],
          ),
        0,
      );
    // Ties break on the lexicographically smallest order, so the result is stable.
    if (best === null || total < best.total - 1e-9) best = { order, total };
  }

  // A junction reads as a continuation only if it spans about one coarse step.
  // Anything longer is a jump across the figure and is drawn as such.
  const stepLengths = panels.flatMap((panel) =>
    panel.coarse.path.slice(0, -1).map((point, index) => distance(point, panel.coarse.path[index + 1])),
  );
  const typicalStep = stepLengths.sort((a, b) => a - b)[Math.floor(stepLengths.length / 2)];

  const links = best.order.slice(0, -1).map((panelIndex, step) => {
    const from = panels[panelIndex];
    const to = panels[best.order[step + 1]];
    const start = from.coarse.path.at(-1);
    const end = to.coarse.path[0];
    const length = distance(start, end);
    return {
      fromToken: from.parent.token,
      toToken: to.parent.token,
      start,
      end,
      length,
      typicalStep,
      continuous: length <= typicalStep * 1.4,
      s2Consecutive: isConsecutive(from.parent.token, to.parent.token),
    };
  });

  return { order: best.order, links, totalLength: best.total };
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
