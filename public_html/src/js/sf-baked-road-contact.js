import { indexRoadContact } from "./sf-road-contact.js";
import { roadPortals } from "./sf-road-portal.js";

const kinds = new Set(["ramp", "bridge_deck", "tunnel"]);

export function buildBakedRoadContact(road) {
  const { id, kind, points, width, owners, groundEnds = [], seams = {} } = road;
  if (
    typeof id !== "string" ||
    !kinds.has(kind) ||
    !Number.isFinite(width) ||
    width < 2.5 ||
    !Array.isArray(owners) ||
    owners.length !== 2 ||
    owners.some((owner) => !/^[0-9a-f]{8}$/.test(owner)) ||
    Object.entries(seams).some(
      ([end, seam]) =>
        !["start", "end"].includes(end) ||
        !Number.isFinite(seam?.bearing) ||
        !Number.isFinite(seam?.width) ||
        seam.width < 2.5 ||
        seam.width > width,
    ) ||
    !Array.isArray(groundEnds) ||
    new Set(groundEnds).size !== groundEnds.length ||
    groundEnds.some((end) => end !== "start" && end !== "end") ||
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
  let distance = 0;
  const profile = [];
  const vertices = [];
  const triangles = [];
  const tangents = [];
  for (const [i, point] of points.entries()) {
    if (i > 0) {
      const last = points[i - 1];
      const step = Math.hypot(point[0] - last[0], point[1] - last[1]);
      if (step < 0.002) return null;
      distance += step;
    }
    const a = points[Math.max(0, i - 1)];
    const b = points[Math.min(points.length - 1, i + 1)];
    const dx = b[0] - a[0],
      dy = b[1] - a[1];
    const length = Math.hypot(dx, dy);
    if (length < 0.002) return null;
    const last = i === points.length - 1;
    const seam = i === 0 ? seams.start : last ? seams.end : null;
    const bearing = seam?.bearing ?? Math.atan2(dy, dx);
    const halfWidth = (seam?.width ?? width) / 2;
    const ox = -Math.sin(bearing) * halfWidth;
    const oy = Math.cos(bearing) * halfWidth;
    tangents.push(bearing);
    const [x, y, z] = point;
    vertices.push([x + ox, y + oy, z], [x - ox, y - oy, z]);
    profile.push({ position: point, s_m: distance, support: "interpolated" });
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
    terminalBearings: [tangents[0] + Math.PI, tangents.at(-1)],
    heightAt: indexRoadContact(contact),
    highwayRailHalfWidth: 0,
  };
  return {
    surface,
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
