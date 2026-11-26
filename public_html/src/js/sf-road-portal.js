import { driveOnRoadSurface } from "./sf-road-contact.js";

const reach = 3;
const tolerance = 0.22;
const rampLookahead = 5;
const sharedTerminalTolerance = 0.05;
const minimumAlignmentLead = 0.02;
const sideEdgeTolerance = 0.05;

export function followsGroundPortal(car, portals) {
  const travel = car.v < 0 ? -1 : 1;
  const headingX = Math.cos(car.heading) * travel;
  const headingY = Math.sin(car.heading) * travel;
  return portals.some(
    ({ x, y, nx, ny, leftWidth, rightWidth, entryX, entryY }) => {
      const dx = car.x - x;
      const dy = car.y - y;
      const along = dx * nx + dy * ny;
      const lateral = dy * nx - dx * ny;
      const beyond = Math.max(0, lateral - leftWidth, -lateral - rightWidth);
      return (
        Math.hypot(along, beyond) <= reach &&
        headingX * entryX + headingY * entryY > 0.88
      );
    },
  );
}

export function shouldTakePortalRoadJoin(car, joined, portals) {
  if (joined.blocked || !joined.surfaceId || joined.surfaceId === car.surfaceId)
    return false;
  const [x, y, z] = joined.seam.point;
  const arrivals = portals.filter(
    (portal) =>
      Math.hypot(portal.x - x, portal.y - y, portal.z - z) <=
      sharedTerminalTolerance,
  );
  return !arrivals.length || followsGroundPortal(joined, arrivals);
}

export function roadPortals(
  surface,
  groundEnds,
  groundExitOnly = [],
  seams = {},
  groundEntryProbeMetres = {},
  sideGroundAccess = [],
) {
  const { contact, profile } = surface;
  const terminals = groundEnds.map((end) => {
    const atEnd = end === "end";
    const index = atEnd ? profile.length - 1 : 0;
    const [x, y, z] = profile[index].position;
    const [u, v] = profile[index + (atEnd ? -1 : 1)].position;
    const length = Math.hypot(u - x, v - y);
    const entryX = (u - x) / length;
    const entryY = (v - y) / length;
    const [edgeX, edgeY] = contact.vertices[index * 2];
    const [otherX, otherY] = contact.vertices[index * 2 + 1];
    const firstWidth = Math.hypot(edgeX - x, edgeY - y);
    const secondWidth = Math.hypot(otherX - x, otherY - y);
    const sectionWidth = Math.hypot(otherX - edgeX, otherY - edgeY);
    const inward = atEnd ? -1 : 1;
    return {
      surface,
      exitOnly: groundExitOnly.includes(end),
      sharedSeam: Boolean(seams[end]),
      x,
      y,
      z,
      nx: (inward * (edgeY - otherY)) / sectionWidth,
      ny: (inward * (otherX - edgeX)) / sectionWidth,
      entryX,
      entryY,
      leftWidth: atEnd ? secondWidth : firstWidth,
      rightWidth: atEnd ? firstWidth : secondWidth,
      halfWidth: surface.terminalHalfWidths?.[atEnd ? 1 : 0] ?? firstWidth,
      entryProbe:
        surface.kind === "ramp"
          ? Math.min(
              groundEntryProbeMetres[end] ?? rampLookahead,
              profile.at(-1).s_m / 2,
            )
          : 0,
    };
  });
  const sides = sideGroundAccess.map((access) =>
    sideRoadPortal(surface, access),
  );
  return sides.includes(null) ? null : [...terminals, ...sides];
}

function sideRoadPortal(surface, { side, startMetres, endMetres }) {
  const { contact, profile } = surface;
  const edge = side === "left" ? 0 : 1;
  const edgeAt = (station) => {
    const end = profile.findIndex(({ s_m }) => s_m >= station);
    const a = contact.vertices[2 * Math.max(0, end - 1) + edge];
    const b = contact.vertices[2 * end + edge];
    if (end === 0) return b;
    const fraction =
      (station - profile[end - 1].s_m) /
      (profile[end].s_m - profile[end - 1].s_m);
    return a.map((value, index) => value + (b[index] - value) * fraction);
  };
  const a = edgeAt(startMetres);
  const b = edgeAt(endMetres);
  const dx = b[0] - a[0];
  const dy = b[1] - a[1];
  const length = Math.hypot(dx, dy);
  if (length <= 0.002) return null;
  const sign = side === "left" ? 1 : -1;
  const nx = (sign * dy) / length;
  const ny = (-sign * dx) / length;
  // A side crossing uses one contact plane. Check the deck edge stays in
  // that plane across the declared interval, including intermediate vertices.
  if (
    profile.some(({ s_m }, index) => {
      if (s_m <= startMetres || s_m >= endMetres) return false;
      const [x, y] = contact.vertices[2 * index + edge];
      return Math.abs((x - a[0]) * nx + (y - a[1]) * ny) > sideEdgeTolerance;
    })
  )
    return null;
  const [x, y] = a.map((value, index) => (value + b[index]) / 2);
  const z = surface.heightAt(
    x + nx * sideEdgeTolerance,
    y + ny * sideEdgeTolerance,
  );
  if (z === null) return null;
  const halfWidth = length / 2;
  return {
    surface,
    sideAccess: { side, startMetres, endMetres },
    x,
    y,
    z,
    nx,
    ny,
    entryX: nx,
    entryY: ny,
    leftWidth: halfWidth,
    rightWidth: halfWidth,
    halfWidth,
    entryProbe: 0,
  };
}

// Road and pavement meet at a declared terminal. The tyre footprint can
// straddle the seam in either direction without sampling terrain above or
// below the interior of the road.
function stepThrough(car, input, dt, portal, pavementHeight) {
  const { surface, x, y, z, nx, ny, entryProbe } = portal;
  const along = (east, north) => (east - x) * nx + (north - y) * ny;
  const entering = !car.surfaceId;
  if (entering && portal.exitOnly) return null;
  const before = along(car.x, car.y);
  if (entering ? before > 0.001 : before < -0.001) return null;
  const ground = pavementHeight(x, y);
  if (ground === null || Math.abs(ground - z) > tolerance) return null;
  // Stay on the level the car was already driving before using join contact.
  const priorHeight = (entering ? pavementHeight : surface.heightAt)(
    car.x,
    car.y,
  );
  if (priorHeight === null || Math.abs(car.z - priorHeight) > tolerance)
    return null;

  const heightAt = (east, north) => {
    const distance = along(east, north);
    if (distance < 0) return pavementHeight(east, north);
    return (
      surface.heightAt(east, north) ??
      (portal.sideAccess && distance <= sideEdgeTolerance
        ? surface.heightAt(
            east + nx * sideEdgeTolerance,
            north + ny * sideEdgeTolerance,
          )
        : null)
    );
  };
  const next = driveOnRoadSurface(car, input, dt, { heightAt });
  if (next.blocked) return null;
  const after = along(next.x, next.y);
  const travel = Math.hypot(next.x - car.x, next.y - car.y);
  if (entering && after - before <= 0.4 * travel) return null;
  if (!entering && after < 0 && before - after <= 0.4 * travel) return null;
  // A strip outside the ramp's mouth should never select this portal or
  // interfere with the normal street physics.
  const offset = (next.y - y) * nx - (next.x - x) * ny;
  if (portal.sideAccess && Math.abs(offset) > portal.halfWidth) return null;
  const inset = portal.sideAccess ? sideEdgeTolerance : 0;
  if (
    surface.heightAt(
      x - offset * ny + inset * nx,
      y + offset * nx + inset * ny,
    ) === null
  )
    return null;

  // Street traffic can cross the shallow end of a ramp diagonally.
  // Commit to the ramp once the current line of travel reaches its lane.
  if (entering && after >= 0 && entryProbe) {
    const scale = entryProbe / travel;
    const aheadX = next.x + (next.x - car.x) * scale;
    const aheadY = next.y + (next.y - car.y) * scale;
    if (surface.heightAt(aheadX, aheadY) === null) return null;
  }

  const surfaceId = after < 0 ? undefined : surface.id;
  if (
    entering &&
    surfaceId &&
    portal.sharedSeam &&
    !followsGroundPortal(next, [portal])
  )
    return null;
  const futureX = next.x + 1.35 * Math.cos(next.heading);
  const futureY = next.y + 1.35 * Math.sin(next.heading);
  const slopeHeight = surfaceId ? surface.heightAt : pavementHeight;
  const front = slopeHeight(futureX, futureY) ?? next.z;
  const rear =
    slopeHeight(2 * next.x - futureX, 2 * next.y - futureY) ?? next.z;
  return {
    ...car,
    ...next,
    surfaceId,
    vz: ((front - rear) / 2.7) * next.v,
    seam: null,
  };
}

export function driveThroughRoadPortal(
  car,
  input,
  dt,
  portalGroups,
  pavementHeight,
) {
  if (car.grounded === false || car.leavingRoad) return null;
  const matches = [];
  for (const portals of portalGroups)
    for (const portal of portals) {
      const dx = car.x - portal.x;
      const dy = car.y - portal.y;
      if (
        (car.surfaceId && car.surfaceId !== portal.surface.id) ||
        Math.abs(dx * portal.nx + dy * portal.ny) > reach ||
        Math.abs(dy * portal.nx - dx * portal.ny) > portal.halfWidth + reach
      )
        continue;
      const next = stepThrough(car, input, dt, portal, pavementHeight);
      if (next) matches.push({ portal, next });
    }
  if (matches.length <= 1) return matches[0]?.next ?? null;

  // A junction can have several mouths at the same surveyed ground point.
  // Follow the driver's heading only when one branch is clearly aligned.
  const terminal = matches[0].portal;
  if (
    car.surfaceId ||
    matches.some(
      ({ portal }) =>
        Math.hypot(
          portal.x - terminal.x,
          portal.y - terminal.y,
          portal.z - terminal.z,
        ) > sharedTerminalTolerance,
    )
  )
    return null;

  const ranked = matches
    .map(({ portal, next }) => {
      const dx = next.x - car.x;
      const dy = next.y - car.y;
      const alignment =
        (dx * portal.entryX + dy * portal.entryY) / Math.hypot(dx, dy);
      return { next, alignment };
    })
    .sort((a, b) => b.alignment - a.alignment);
  const [best, second] = ranked;
  return best.alignment - second.alignment > minimumAlignmentLead
    ? best.next
    : null;
}
