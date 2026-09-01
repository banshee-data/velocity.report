/**
 * Per-cell opt-in lists.
 *
 * A selection is the only thing that decides which parts of a traversal are
 * emphasised. It is a hand-maintained list, not a derived one: whichever cells
 * matter for a given figure are the ones marked enabled.
 *
 * Reading and writing the file belongs to the CLI layer, so this stays
 * browser-safe.
 */

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
 * A segment is selected when both of the cells it joins are enabled, so a run
 * of consecutive opted-in cells reads as one continuous stretch.
 */
export function classifySegmentsBySelection(path, selection) {
  return path.slice(0, -1).map((from, index) => {
    const to = path[index + 1];
    const enabled = selection.get(from.token) === true && selection.get(to.token) === true;
    return enabled ? "selected" : "unselected";
  });
}

/** Report tokens present in the selection that this traversal does not contain. */
export function unknownSelectionTokens(path, selection) {
  const known = new Set(path.map((point) => point.token));
  return [...selection.keys()].filter((token) => !known.has(token));
}
