import { driveOnRoadSurface } from "./sf-road-contact.js";

const reach = 3;
const tolerance = 0.22;

export function roadPortals(surface, groundEnds) {
  const { profile } = surface;
  return groundEnds.map((end) => {
    const atEnd = end === "end";
    const index = atEnd ? profile.length - 1 : 0;
    const [x, y, z] = profile[index].position;
    const [u, v] = profile[index + (atEnd ? -1 : 1)].position;
    const length = Math.hypot(u - x, v - y);
    return {
      surface,
      x,
      y,
      z,
      nx: (u - x) / length,
      ny: (v - y) / length,
    };
  });
}

// Road and pavement meet at a declared terminal. The tyre footprint can
// straddle the seam in either direction without sampling terrain above or
// below the interior of the road.
function stepThrough(car, input, dt, portal, pavementHeight) {
  const { surface, x, y, z, nx, ny } = portal;
  const along = (east, north) => (east - x) * nx + (north - y) * ny;
  const entering = !car.surfaceId;
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

  const surfaceId = after < 0 ? undefined : surface.id;
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
      if (
        (car.surfaceId && car.surfaceId !== portal.surface.id) ||
        Math.hypot(car.x - portal.x, car.y - portal.y) > reach
      )
        continue;
      const next = stepThrough(car, input, dt, portal, pavementHeight);
      if (next) matches.push(next);
    }
  return matches.length === 1 ? matches[0] : null;
}
