import { projectWebMercator } from "./projection.mjs";

const INTERSECTION_EPSILON = 1e-12;

function geometryPolygons(geometry) {
  if (!geometry) return [];
  if (geometry.type === "Polygon") return [geometry.coordinates];
  if (geometry.type === "MultiPolygon") return geometry.coordinates;
  if (geometry.type === "GeometryCollection") {
    return geometry.geometries.flatMap(geometryPolygons);
  }
  return [];
}

function geoJsonPolygons(geoJson) {
  if (!geoJson || typeof geoJson !== "object") {
    throw new TypeError("The land mask must be a GeoJSON object.");
  }
  if (geoJson.type === "FeatureCollection") {
    return geoJson.features.flatMap((feature) => geometryPolygons(feature.geometry));
  }
  if (geoJson.type === "Feature") return geometryPolygons(geoJson.geometry);
  return geometryPolygons(geoJson);
}

function normaliseRing(ring, anchorLng) {
  if (!Array.isArray(ring) || ring.length < 4) return null;
  const points = ring.map((position) => {
    if (!Array.isArray(position) || position.length < 2) {
      throw new Error("GeoJSON land-mask positions must contain longitude and latitude.");
    }
    return projectWebMercator({ lng: position[0], lat: position[1] }, anchorLng);
  });
  return points;
}

/** Prepare Polygon and MultiPolygon GeoJSON once for repeated segment queries. */
export function prepareLandMask(geoJson, anchorLng = 0) {
  const polygons = geoJsonPolygons(geoJson)
    .map((polygon) => polygon.map((ring) => normaliseRing(ring, anchorLng)).filter(Boolean))
    .filter((polygon) => polygon.length > 0);
  if (polygons.length === 0) {
    throw new Error("The GeoJSON land mask does not contain Polygon geometry.");
  }
  return { anchorLng, polygons };
}

function pointOnSegment(point, start, end) {
  const cross =
    (point.y - start.y) * (end.x - start.x) -
    (point.x - start.x) * (end.y - start.y);
  if (Math.abs(cross) > INTERSECTION_EPSILON) return false;
  const dot =
    (point.x - start.x) * (end.x - start.x) +
    (point.y - start.y) * (end.y - start.y);
  if (dot < -INTERSECTION_EPSILON) return false;
  const lengthSquared = (end.x - start.x) ** 2 + (end.y - start.y) ** 2;
  return dot <= lengthSquared + INTERSECTION_EPSILON;
}

function pointInRing(point, ring) {
  let inside = false;
  for (let current = 0, previous = ring.length - 1; current < ring.length; previous = current++) {
    const start = ring[previous];
    const end = ring[current];
    if (pointOnSegment(point, start, end)) return true;
    const crosses =
      start.y > point.y !== end.y > point.y &&
      point.x < ((end.x - start.x) * (point.y - start.y)) / (end.y - start.y) + start.x;
    if (crosses) inside = !inside;
  }
  return inside;
}

export function isWorldPointOnLand(point, preparedMask) {
  return preparedMask.polygons.some(
    ([outerRing, ...holes]) =>
      pointInRing(point, outerRing) && !holes.some((hole) => pointInRing(point, hole)),
  );
}

export function isGeographicPointOnLand(point, preparedMask) {
  return isWorldPointOnLand(
    projectWebMercator(point, preparedMask.anchorLng),
    preparedMask,
  );
}

function segmentIntersectionParameter(start, end, edgeStart, edgeEnd) {
  const segmentX = end.x - start.x;
  const segmentY = end.y - start.y;
  const edgeX = edgeEnd.x - edgeStart.x;
  const edgeY = edgeEnd.y - edgeStart.y;
  const denominator = segmentX * edgeY - segmentY * edgeX;
  if (Math.abs(denominator) <= INTERSECTION_EPSILON) return null;

  const differenceX = edgeStart.x - start.x;
  const differenceY = edgeStart.y - start.y;
  const segmentParameter = (differenceX * edgeY - differenceY * edgeX) / denominator;
  const edgeParameter = (differenceX * segmentY - differenceY * segmentX) / denominator;
  if (
    segmentParameter < -INTERSECTION_EPSILON ||
    segmentParameter > 1 + INTERSECTION_EPSILON ||
    edgeParameter < -INTERSECTION_EPSILON ||
    edgeParameter > 1 + INTERSECTION_EPSILON
  ) {
    return null;
  }
  return Math.min(1, Math.max(0, segmentParameter));
}

function interpolate(start, end, parameter) {
  return {
    x: start.x + (end.x - start.x) * parameter,
    y: start.y + (end.y - start.y) * parameter,
  };
}

function uniqueSortedParameters(parameters) {
  return parameters
    .sort((left, right) => left - right)
    .filter(
      (parameter, index, values) =>
        index === 0 || Math.abs(parameter - values[index - 1]) > INTERSECTION_EPSILON,
    );
}

/**
 * Split a Web Mercator line at every polygon boundary, then classify each
 * interval by its midpoint. This retains the complete traversal over water.
 */
export function splitWorldSegmentByLandMask(start, end, preparedMask) {
  const parameters = [0, 1];
  for (const polygon of preparedMask.polygons) {
    for (const ring of polygon) {
      for (let index = 0; index < ring.length - 1; index += 1) {
        const parameter = segmentIntersectionParameter(start, end, ring[index], ring[index + 1]);
        if (parameter !== null) parameters.push(parameter);
      }
    }
  }

  const splitParameters = uniqueSortedParameters(parameters);
  return splitParameters.slice(0, -1).map((from, index) => {
    const to = splitParameters[index + 1];
    const midpoint = interpolate(start, end, (from + to) / 2);
    return {
      classification: isWorldPointOnLand(midpoint, preparedMask) ? "land" : "water",
      start: interpolate(start, end, from),
      end: interpolate(start, end, to),
    };
  });
}

export function classifyTraversalSegments(path, preparedMask, projectWorldPoint) {
  const classified = { land: [], water: [] };
  for (let index = 0; index < path.length - 1; index += 1) {
    const pieces = splitWorldSegmentByLandMask(
      path[index].world,
      path[index + 1].world,
      preparedMask,
    );
    for (const piece of pieces) {
      classified[piece.classification].push({
        fromIndex: index,
        toIndex: index + 1,
        start: projectWorldPoint(piece.start),
        end: projectWorldPoint(piece.end),
      });
    }
  }
  return classified;
}
