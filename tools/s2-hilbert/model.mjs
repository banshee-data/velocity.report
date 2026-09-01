import {
  classifySegmentsBySelection,
  parseCellSelection,
  unknownSelectionTokens,
} from "./cell-selection.mjs";
import { getHilbertTraversal } from "./hierarchy.mjs";
import { projectTraversal } from "./projection.mjs";

/** Split each centre-to-centre link into whole selected or unselected runs. */
function segmentsFromClassifications(path, classifications, projectWorldPoint) {
  const classified = { selected: [], unselected: [] };
  classifications.forEach((classification, index) => {
    classified[classification].push({
      fromIndex: index,
      toIndex: index + 1,
      start: projectWorldPoint(path[index].world),
      end: projectWorldPoint(path[index + 1].world),
    });
  });
  return classified;
}

/**
 * Build a JSON-friendly model from actual S2 descendants. The optional per-cell
 * selection is data, not part of S2 traversal or projection logic.
 */
export function buildS2HilbertModel(options) {
  if (!options || typeof options !== "object") {
    throw new TypeError("buildS2HilbertModel requires an options object.");
  }
  const traversal = getHilbertTraversal(options.parent, options.targetLevel);
  const projected = projectTraversal(traversal, options);
  const { projectWorldPoint, ...serialisableModel } = projected;

  let classifiedSegments = null;
  let segmentClassifications = null;

  if (options.cellSelection) {
    const selection = parseCellSelection(options.cellSelection);
    const unknown = unknownSelectionTokens(projected.path, selection);
    if (unknown.length > 0) {
      throw new Error(
        `The cell selection names ${unknown.length} token(s) that are not descendants of ${traversal.parent.token}: ${unknown.slice(0, 4).join(", ")}.`,
      );
    }
    segmentClassifications = classifySegmentsBySelection(projected.path, selection);
    classifiedSegments = segmentsFromClassifications(
      projected.path,
      segmentClassifications,
      projectWorldPoint,
    );
  }

  return {
    ...serialisableModel,
    classifiedSegments,
    segmentClassifications,
    selectionApplied: classifiedSegments !== null,
  };
}

/** Produce the stable machine-readable ordered-child document. */
export function buildOrderedChildrenDocument(model) {
  return {
    parent: model.parent.token,
    displayParent: model.parent.displayToken,
    parentLevel: model.parentLevel,
    childLevel: model.targetLevel,
    children: model.cells.map((cell) => ({
      index: cell.index,
      token: cell.token,
      cellId: cell.cellId,
      lat: cell.centre.lat,
      lng: cell.centre.lng,
    })),
  };
}
