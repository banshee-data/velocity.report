import { indexRoadContact } from "./sf-road-contact.js";
import { roadPortals } from "./sf-road-portal.js";
import { validTunnelWallOpenings } from "./sf-tunnel-mesh.js";

const kinds = new Set(["ramp", "bridge_deck", "tunnel"]);

function edgeWidths(width, left = width / 2) {
  return [left, width - left];
}

function blendWidths(from, to, fraction) {
  return from.map((width, edge) => width + (to[edge] - width) * fraction);
}

function taperedWidths(widths, seam, distance, length) {
  const fraction = distance / length;
  const blend = fraction * fraction * (3 - 2 * fraction);
  return blendWidths(edgeWidths(seam.width, seam.leftWidth), widths, blend);
}

function crossSectionAt(sections, station, width) {
  if (!sections?.length) return edgeWidths(width);
  const next = sections.findIndex(([distance]) => distance > station);
  if (next < 0) return sections.at(-1).slice(1);
  if (next === 0) return sections[0].slice(1);
  const previous = sections[next - 1];
  const following = sections[next];
  const fraction = (station - previous[0]) / (following[0] - previous[0]);
  return blendWidths(previous.slice(1), following.slice(1), fraction);
}

function seamInTravelOrder(seam, a, b) {
  if (!seam) return null;
  const alignment =
    (b[0] - a[0]) * Math.cos(seam.bearing) +
    (b[1] - a[1]) * Math.sin(seam.bearing);
  if (alignment >= 0) return seam;
  // A shared transverse section can join two roads digitized toward it.
  return {
    ...seam,
    bearing: seam.bearing + Math.PI,
    leftWidth:
      seam.leftWidth === undefined ? undefined : seam.width - seam.leftWidth,
  };
}

export function buildBakedRoadContact(road) {
  const {
    id,
    kind,
    points,
    width,
    owners,
    groundEnds = [],
    groundExitOnly = [],
    groundEntryProbeMetres = {},
    sideGroundAccess = [],
    seams = {},
    widthTransitionMetres = {},
    widthSections,
    railSides = [],
    railRanges = [],
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
        (seam.leftWidth !== undefined &&
          (!Number.isFinite(seam.leftWidth) ||
            seam.leftWidth <= 0 ||
            seam.leftWidth >= seam.width)) ||
        (seam.width > width && !widthTransitionMetres[end]),
    ) ||
    !Array.isArray(groundEnds) ||
    new Set(groundEnds).size !== groundEnds.length ||
    groundEnds.some((end) => end !== "start" && end !== "end") ||
    !Array.isArray(groundExitOnly) ||
    new Set(groundExitOnly).size !== groundExitOnly.length ||
    groundExitOnly.some((end) => !groundEnds.includes(end)) ||
    !groundEntryProbeMetres ||
    typeof groundEntryProbeMetres !== "object" ||
    Array.isArray(groundEntryProbeMetres) ||
    Object.entries(groundEntryProbeMetres).some(
      ([end, metres]) =>
        kind !== "ramp" ||
        !groundEnds.includes(end) ||
        groundExitOnly.includes(end) ||
        !Number.isFinite(metres) ||
        metres <= 0,
    ) ||
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
    !Array.isArray(railRanges) ||
    railRanges.some(
      (rail, i) =>
        !rail ||
        !["left", "right"].includes(rail.side) ||
        railSides.includes(rail.side) ||
        !Number.isFinite(rail.startMetres) ||
        !Number.isFinite(rail.endMetres) ||
        rail.startMetres < 0 ||
        rail.endMetres > distance ||
        rail.endMetres <= rail.startMetres ||
        railRanges
          .slice(0, i)
          .some(
            (previous) =>
              previous.side === rail.side &&
              previous.startMetres < rail.endMetres &&
              rail.startMetres < previous.endMetres,
          ),
    )
  )
    return null;
  if (
    !Array.isArray(sideGroundAccess) ||
    sideGroundAccess.some(
      (access) =>
        !access ||
        !["left", "right"].includes(access.side) ||
        railSides.includes(access.side) ||
        !Number.isFinite(access.startMetres) ||
        !Number.isFinite(access.endMetres) ||
        access.startMetres < 0 ||
        access.endMetres > distance ||
        access.endMetres <= access.startMetres ||
        railRanges.some(
          (rail) =>
            rail.side === access.side &&
            rail.startMetres < access.endMetres &&
            access.startMetres < rail.endMetres,
        ),
    )
  )
    return null;
  if (
    widthSections !== undefined &&
    (!Array.isArray(widthSections) ||
      !widthSections.length ||
      widthSections.some(
        (section, i) =>
          !Array.isArray(section) ||
          section.length !== 3 ||
          !section.every(Number.isFinite) ||
          section[0] < 0 ||
          section[0] > distance ||
          (i > 0 && section[0] <= widthSections[i - 1][0]) ||
          section[1] <= 0 ||
          section[2] <= 0 ||
          section[1] + section[2] < 2.5 ||
          section[1] + section[2] > 30,
      ))
  )
    return null;
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
  const measuredWidths =
    widthSections ||
    Object.values(seams).some((seam) => seam.leftWidth !== undefined);
  const startSeam = seamInTravelOrder(seams.start, points[0], points[1]);
  const endSeam = seamInTravelOrder(seams.end, points.at(-2), points.at(-1));
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
    const seam = i === 0 ? startSeam : last ? endSeam : null;
    const bearing = seam?.bearing ?? Math.atan2(dy, dx);
    let section = crossSectionAt(widthSections, station, width);
    if (seam) {
      section = edgeWidths(seam.width, seam.leftWidth);
    } else if (station < startTaper) {
      section = taperedWidths(section, startSeam, station, startTaper);
    } else if (remaining < endTaper) {
      section = taperedWidths(section, endSeam, remaining, endTaper);
    }
    const [leftWidth, rightWidth] = section;
    const nx = -Math.sin(bearing);
    const ny = Math.cos(bearing);
    tangents.push(bearing);
    const [x, y, z] = point;
    vertices.push(
      [x + nx * leftWidth, y + ny * leftWidth, z],
      [x - nx * rightWidth, y - ny * rightWidth, z],
    );
    profile.push({
      position: point,
      s_m: station,
      support: "interpolated",
      ...(measuredWidths && { edgeWidths: section }),
    });
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
    sourceCnn: road.source_cnn,
    contact,
    profile,
    end_ids,
    end_tiles: owners,
    terminalHalfWidths: [
      profile[0].edgeWidths
        ? Math.max(...profile[0].edgeWidths)
        : (seams.start?.width ?? width) / 2,
      profile.at(-1).edgeWidths
        ? Math.max(...profile.at(-1).edgeWidths)
        : (seams.end?.width ?? width) / 2,
    ],
    terminalBearings: [tangents[0] + Math.PI, tangents.at(-1)],
    heightAt: indexRoadContact(contact),
    railSides,
    railRanges,
    coveredRange,
    highwayRailHalfWidth:
      railSides.length || railRanges.length ? width / 2 - 0.35 : 0,
  };
  const interior = road.interior_range_m;
  const clearance = road.game_clearance_m;
  const openings = road.tunnel_wall_openings_m;
  const appearance =
    kind === "tunnel" &&
    Array.isArray(interior) &&
    interior.length === 2 &&
    interior.every(Number.isFinite) &&
    interior[0] >= 0 &&
    interior[1] <= distance &&
    interior[1] > interior[0] &&
    validTunnelWallOpenings(openings, interior) &&
    Number.isFinite(clearance) &&
    clearance >= 2.5 &&
    clearance <= 10
      ? { interior, clearance, ...(openings && { wallOpenings: openings }) }
      : null;
  if (openings !== undefined && !appearance) return null;
  if (appearance) surface.appearance = appearance;
  const portals = roadPortals(
    surface,
    groundEnds,
    groundExitOnly,
    seams,
    groundEntryProbeMetres,
    sideGroundAccess,
  );
  if (!portals) return null;
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
    portals,
  };
}
