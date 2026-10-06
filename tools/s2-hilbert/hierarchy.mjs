import { s2 } from "s2js";

const CANONICAL_TOKEN = /^[0-9a-f]{1,16}$/;
const DISPLAY_TOKEN = /^([0-9a-f]{5})-([0-9a-f]+)$/;

/**
 * Parse a canonical S2 token. Display hyphens are deliberately rejected here
 * so a presentation token can never leak into the S2 library unnoticed.
 */
export function parseCanonicalToken(token) {
  if (typeof token !== "string") {
    throw new TypeError("An S2 token must be a string.");
  }

  const canonical = token.toLowerCase();
  if (!CANONICAL_TOKEN.test(canonical)) {
    const hint = token.includes("-")
      ? " Convert the display token before calling the S2 core."
      : "";
    throw new Error(`Invalid canonical S2 token: ${token}.${hint}`);
  }

  const cellId = s2.cellid.fromToken(canonical);
  if (!s2.cellid.valid(cellId) || s2.cellid.toToken(cellId) !== canonical) {
    throw new Error(`Invalid or non-canonical S2 token: ${token}.`);
  }

  return cellId;
}

/** Convert the repository's five-character-prefix display form explicitly. */
export function displayTokenToCanonical(token) {
  if (typeof token !== "string") {
    throw new TypeError("An S2 display token must be a string.");
  }

  const match = token.toLowerCase().match(DISPLAY_TOKEN);
  if (!match) {
    throw new Error(`Invalid S2 display token: ${token}.`);
  }

  const canonical = `${match[1]}${match[2]}`;
  parseCanonicalToken(canonical);
  return canonical;
}

/** Convert canonical tokens to the documentation's recognition-friendly form. */
export function formatDisplayToken(token) {
  const canonical = s2.cellid.toToken(parseCanonicalToken(token));
  return canonical.length > 5
    ? `${canonical.slice(0, 5)}-${canonical.slice(5)}`
    : canonical;
}

/**
 * Accept either CLI spelling, but make the display-token conversion an
 * explicit boundary operation before the S2 library sees the value.
 */
export function parseCliToken(token) {
  return token.includes("-")
    ? displayTokenToCanonical(token)
    : s2.cellid.toToken(parseCanonicalToken(token));
}

/** Return descendant CellIDs in the S2 library's Hilbert traversal order. */
export function getDescendantIds(parentToken, targetLevel) {
  const parentId = parseCanonicalToken(parentToken);
  const parentLevel = s2.cellid.level(parentId);
  if (!Number.isInteger(targetLevel) || targetLevel < parentLevel || targetLevel > 30) {
    throw new RangeError(
      `Target level must be an integer from ${parentLevel} to 30; received ${targetLevel}.`,
    );
  }

  const descendants = [];
  const end = s2.cellid.childEndAtLevel(parentId, targetLevel);
  for (
    let childId = s2.cellid.childBeginAtLevel(parentId, targetLevel);
    childId !== end;
    childId = s2.cellid.next(childId)
  ) {
    descendants.push(childId);
  }

  return descendants;
}

function pointToLatLng(point) {
  const latLng = s2.LatLng.fromPoint(point);
  return {
    lat: (latLng.lat * 180) / Math.PI,
    lng: (latLng.lng * 180) / Math.PI,
  };
}

/** Convert a real S2 CellID to browser-safe geographic geometry. */
export function cellIdToGeographicGeometry(cellId, index = undefined) {
  const cell = s2.Cell.fromCellID(cellId);
  const token = s2.cellid.toToken(cellId);
  const centre = pointToLatLng(cell.center());
  const vertices = Array.from({ length: 4 }, (_, vertexIndex) =>
    pointToLatLng(cell.vertex(vertexIndex)),
  );

  return {
    ...(index === undefined ? {} : { index }),
    token,
    displayToken: formatDisplayToken(token),
    cellId: cellId.toString(),
    level: cell.level,
    orientation: cell.orientation,
    centre,
    vertices,
  };
}

/** Build the geographic traversal without projection or presentation choices. */
export function getHilbertTraversal(parentToken, targetLevel) {
  const parentId = parseCanonicalToken(parentToken);
  const canonicalParent = s2.cellid.toToken(parentId);
  const parent = cellIdToGeographicGeometry(parentId);
  const cells = getDescendantIds(canonicalParent, targetLevel).map((cellId, index) => ({
    ...cellIdToGeographicGeometry(cellId, index),
    parentToken: canonicalParent,
  }));

  return {
    schemaVersion: 1,
    parent,
    parentLevel: parent.level,
    targetLevel,
    cells,
  };
}

export { s2 };
