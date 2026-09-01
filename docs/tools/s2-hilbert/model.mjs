import { getHilbertTraversal } from "./hierarchy.mjs";
import { classifyTraversalSegments, prepareLandMask } from "./land-mask.mjs";
import { projectTraversal } from "./projection.mjs";

/**
 * Build a JSON-friendly model from actual S2 descendants. The optional land
 * mask is data, not part of S2 traversal or projection logic.
 */
export function buildS2HilbertModel(options) {
  if (!options || typeof options !== "object") {
    throw new TypeError("buildS2HilbertModel requires an options object.");
  }
  const traversal = getHilbertTraversal(options.parent, options.targetLevel);
  const projected = projectTraversal(traversal, options);
  const preparedMask = options.landMask
    ? prepareLandMask(options.landMask, projected.projection.anchorLng)
    : null;
  const classifiedSegments = preparedMask
    ? classifyTraversalSegments(
        projected.path,
        preparedMask,
        projected.projectWorldPoint,
      )
    : null;

  const { projectWorldPoint: _projectWorldPoint, ...serialisableModel } = projected;
  return {
    ...serialisableModel,
    classifiedSegments,
    landMaskApplied: preparedMask !== null,
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
