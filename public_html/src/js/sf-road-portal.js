import { driveOnRoadSurface } from "./sf-road-contact.js";

const reach = 3;
const tolerance = 0.22;
const rampLookahead = 5;
const sharedTerminalTolerance = 0.05;
const minimumAlignmentLead = 0.02;

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
) {
  const { contact, profile } = surface;
  return groundEnds.map((end) => {
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
          ? Math.min(rampLookahead, profile.at(-1).s_m / 2)
          : 0,
    };
  });
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

  const heightAt = (east, north) =>
    along(east, north) >= 0
      ? surface.heightAt(east, north)
      : pavementHeight(east, north);
  const next = driveOnRoadSurface(car, input, dt, { heightAt });
  if (next.blocked) return null;
  const after = along(next.x, next.y);
  const travel = Math.hypot(next.x - car.x, next.y - car.y);
  if (entering && after - before <= 0.4 * travel) return null;
  if (!entering && after < 0 && before - after <= 0.4 * travel) return null;
  // A strip outside the ramp's mouth should never select this portal or
  // interfere with the normal street physics.
  const offset = (next.y - y) * nx - (next.x - x) * ny;
  if (surface.heightAt(x - offset * ny, y + offset * nx) === null) return null;

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
