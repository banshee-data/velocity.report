import { indexRoadContact } from "./sf-road-contact.js";
import { roadPortals } from "./sf-road-portal.js";

const kinds = new Set(["ramp", "bridge_deck", "tunnel"]);

function taperedWidth(width, seam, distance, length) {
  const fraction = distance / length;
  const blend = fraction * fraction * (3 - 2 * fraction);
  return seam.width + (width - seam.width) * blend;
}

export function buildBakedRoadContact(road) {
  const {
    id,
    kind,
    points,
    width,
    owners,
    groundEnds = [],
    seams = {},
    widthTransitionMetres = {},
    railSides = [],
    covered_range_m: coveredRange,
  } = road;
  if (
    typeof id !== "string" ||
    !kinds.has(kind) ||
    !Number.isFinite(width) ||
    width < 2.5 ||
    !Array.isArray(owners) ||
    owners.length !== 2 ||
    owners.some((owner) => !/^[0-9a-f]{8}$/.test(owner)) ||
    !widthTransitionMetres ||
    typeof widthTransitionMetres !== "object" ||
    Array.isArray(widthTransitionMetres) ||
    Object.entries(widthTransitionMetres).some(
      ([end, length]) =>
        !["start", "end"].includes(end) ||
        !seams[end] ||
        !Number.isFinite(length) ||
        length <= 0,
    ) ||
    Object.entries(seams).some(
      ([end, seam]) =>
        !["start", "end"].includes(end) ||
        !Number.isFinite(seam?.bearing) ||
        !Number.isFinite(seam?.width) ||
        seam.width < 2.5 ||
        (seam.width > width && !widthTransitionMetres[end]),
    ) ||
    !Array.isArray(groundEnds) ||
    new Set(groundEnds).size !== groundEnds.length ||
    groundEnds.some((end) => end !== "start" && end !== "end") ||
    !Array.isArray(railSides) ||
    new Set(railSides).size !== railSides.length ||
    railSides.some((side) => side !== "left" && side !== "right") ||
    !Array.isArray(points) ||
    points.length < 3 ||
    points.some(
      (point) =>
        !Array.isArray(point) ||
        point.length !== 3 ||
        !point.every(Number.isFinite),
    )
  )
    return null;

  const stations = [0];
  let distance = 0;
  for (let i = 1; i < points.length; i++) {
    const [x, y] = points[i];
    const [lastX, lastY] = points[i - 1];
    const step = Math.hypot(x - lastX, y - lastY);
    if (step < 0.002) return null;
    distance += step;
    stations.push(distance);
  }
  const { start: startTaper = 0, end: endTaper = 0 } = widthTransitionMetres;
  if (startTaper + endTaper > distance) return null;
  if (
    coveredRange !== undefined &&
    (!Array.isArray(coveredRange) ||
      coveredRange.length !== 2 ||
      !coveredRange.every(Number.isFinite) ||
      coveredRange[0] < 0 ||
      coveredRange[1] > distance ||
      coveredRange[0] >= coveredRange[1])
  )
    return null;

  const profile = [];
  const vertices = [];
  const triangles = [];
  const tangents = [];
  for (const [i, point] of points.entries()) {
    const station = stations[i];
    const remaining = distance - station;
    const a = points[Math.max(0, i - 1)];
    const b = points[Math.min(points.length - 1, i + 1)];
    const dx = b[0] - a[0],
      dy = b[1] - a[1];
    const length = Math.hypot(dx, dy);
    if (length < 0.002) return null;
    const last = i === points.length - 1;
    const seam = i === 0 ? seams.start : last ? seams.end : null;
    const bearing = seam?.bearing ?? Math.atan2(dy, dx);
    let sectionWidth = seam?.width ?? width;
    if (station < startTaper) {
      sectionWidth = taperedWidth(width, seams.start, station, startTaper);
    } else if (remaining < endTaper) {
      sectionWidth = taperedWidth(width, seams.end, remaining, endTaper);
    }
    const halfWidth = sectionWidth / 2;
    const ox = -Math.sin(bearing) * halfWidth;
    const oy = Math.cos(bearing) * halfWidth;
    tangents.push(bearing);
    const [x, y, z] = point;
    vertices.push([x + ox, y + oy, z], [x - ox, y - oy, z]);
    profile.push({ position: point, s_m: station, support: "interpolated" });
    if (i) {
      const j = 2 * i;
      triangles.push([j - 2, j - 1, j], [j - 1, j + 1, j]);
    }
  }
  const contact = {
    vertices,
    triangles,
    support: vertices.map(() => "interpolated"),
  };
  const end_ids = [`${id}:start`, `${id}:end`];
  const surface = {
    id,
    kind,
    label: road.label,
    contact,
    profile,
    end_ids,
    end_tiles: owners,
    terminalHalfWidths: [
      (seams.start?.width ?? width) / 2,
      (seams.end?.width ?? width) / 2,
    ],
    terminalBearings: [tangents[0] + Math.PI, tangents.at(-1)],
    heightAt: indexRoadContact(contact),
    railSides,
    coveredRange,
    highwayRailHalfWidth: railSides.length ? width / 2 - 0.35 : 0,
  };
  const interior = road.interior_range_m;
  const clearance = road.game_clearance_m;
  const appearance =
    kind === "tunnel" &&
    Array.isArray(interior) &&
    interior.length === 2 &&
    interior.every(Number.isFinite) &&
    interior[0] >= 0 &&
    interior[1] <= distance &&
    interior[1] > interior[0] &&
    Number.isFinite(clearance) &&
    clearance >= 2.5 &&
    clearance <= 10
      ? { interior, clearance }
      : null;
  return {
    surface,
    appearance,
    endpoints: end_ids.map((terminal, i) => ({
      id: terminal,
      position: points[i ? points.length - 1 : 0],
      surface_id: id,
      surface_owner: owners[i],
      support: "interpolated",
    })),
    portals: roadPortals(surface, groundEnds),
  };
}
