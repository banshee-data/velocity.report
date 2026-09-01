import { isGeographicPointOnLand, prepareLandMask } from "./land-mask.mjs";

/**
 * Per-cell opt-in lists. The coastline pass seeds one; a human then edits it.
 * This is browser-safe: reading and writing the file belongs to the CLI layer.
 */

/** Seed a selection from a land mask, enabling every cell whose centre is land. */
export function seedCellSelection(traversal, geoJson, anchorLng = undefined) {
  const preparedMask = prepareLandMask(geoJson, anchorLng ?? traversal.parent.centre.lng);
  return traversal.cells.map((cell) => ({
    index: cell.index,
    token: cell.token,
    displayToken: cell.displayToken,
    enabled: isGeographicPointOnLand(cell.centre, preparedMask),
  }));
}

/**
 * Accept either the plain list form written by the tool or a wrapped object,
 * and reject anything that is not recognisably a selection.
 */
export function parseCellSelection(source) {
  const entries = Array.isArray(source) ? source : source?.cells;
  if (!Array.isArray(entries)) {
    throw new TypeError(
      "A cell selection must be a list of {token, enabled} objects, or an object with a cells list.",
    );
  }

  const selection = new Map();
  for (const entry of entries) {
    if (!entry || typeof entry.token !== "string") {
      throw new Error("Every cell-selection entry needs a canonical token.");
    }
    if (typeof entry.enabled !== "boolean") {
      throw new Error(`Cell ${entry.token} needs an explicit boolean enabled flag.`);
    }
    if (selection.has(entry.token)) {
      throw new Error(`Cell ${entry.token} appears more than once in the selection.`);
    }
    selection.set(entry.token, entry.enabled);
  }
  return selection;
}

/**
 * A segment is lit when both of the cells it joins are enabled, so a run of
 * consecutive opted-in cells lights up as one continuous stretch.
 */
export function classifySegmentsBySelection(path, selection) {
  return path.slice(0, -1).map((from, index) => {
    const to = path[index + 1];
    const enabled = selection.get(from.token) === true && selection.get(to.token) === true;
    return enabled ? "land" : "water";
  });
}

/** Report tokens present in the selection that this traversal does not contain. */
export function unknownSelectionTokens(path, selection) {
  const known = new Set(path.map((point) => point.token));
  return [...selection.keys()].filter((token) => !known.has(token));
}
