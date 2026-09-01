import {
  classifySegmentsBySelection,
  parseCellSelection,
  unknownSelectionTokens,
} from "./cell-selection.mjs";
import { getHilbertTraversal } from "./hierarchy.mjs";
import {
  classifyTraversalSegments,
  isWorldPointOnLand,
  prepareLandMask,
} from "./land-mask.mjs";
import { projectTraversal } from "./projection.mjs";

/** Whole-segment classification, one entry per centre-to-centre link. */
function classifySegmentsByCoastline(path, preparedMask) {
  return path.slice(0, -1).map((from, index) => {
    const to = path[index + 1];
    const midpoint = {
      x: (from.world.x + to.world.x) / 2,
      y: (from.world.y + to.world.y) / 2,
    };
    return isWorldPointOnLand(midpoint, preparedMask) ? "land" : "water";
  });
}

/** Split each centre-to-centre link into whole lit or unlit segments. */
function segmentsFromClassifications(path, classifications, projectWorldPoint) {
  const classified = { land: [], water: [] };
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
 * Build a JSON-friendly model from actual S2 descendants. Both the optional
 * land mask and the optional per-cell selection are data, not part of S2
 * traversal or projection logic. A selection takes precedence over a mask.
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
  let classificationSource = null;

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
    classificationSource = "selection";
  } else if (options.landMask) {
    const preparedMask = prepareLandMask(options.landMask, projected.projection.anchorLng);
    // The drawn path is clipped at the coastline itself, while the chevrons
    // follow one classification per link so each straight run keeps exactly one.
    classifiedSegments = classifyTraversalSegments(
      projected.path,
      preparedMask,
      projectWorldPoint,
    );
    segmentClassifications = classifySegmentsByCoastline(projected.path, preparedMask);
    classificationSource = "coastline";
  }

  return {
    ...serialisableModel,
    classifiedSegments,
    segmentClassifications,
    classificationSource,
    landMaskApplied: classificationSource === "coastline",
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
