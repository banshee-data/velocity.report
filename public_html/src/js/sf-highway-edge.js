import { roadMotion } from "./sf-road-contact.js";

const gravity = 9.81;
const sideClearance = 4;

function turn(a, b, c) {
  return (b[0] - a[0]) * (c[1] - a[1]) - (b[1] - a[1]) * (c[0] - a[0]);
}

// Include XY from missing vertices so a hole in coverage cannot become an open edge.
export function roadBoundary(vertices) {
  const points = [...vertices]
    .filter(([x, y]) => Number.isFinite(x) && Number.isFinite(y))
    .sort((a, b) => a[0] - b[0] || a[1] - b[1]);
  const chain = (points) => {
    const edge = [];
    for (const point of points) {
      while (edge.length > 1 && turn(edge.at(-2), edge.at(-1), point) <= 0)
        edge.pop();
      edge.push(point);
    }
    return edge.slice(0, -1);
  };
  const hull = [...chain(points), ...chain(points.reverse())];
  if (hull.length < 3) return [];
  return hull.map((a, i) => {
    const b = hull[(i + 1) % hull.length];
    const length = Math.hypot(b[0] - a[0], b[1] - a[1]);
    return { a, b, nx: (b[1] - a[1]) / length, ny: (a[0] - b[0]) / length };
  });
}

function roadStation(profile, x, y) {
  let nearest = null;
  for (let i = 1; i < profile.length; i++) {
    const a = profile[i - 1];
    const b = profile[i];
    const dx = b.position[0] - a.position[0];
    const dy = b.position[1] - a.position[1];
    const length = Math.hypot(dx, dy);
    if (!length) continue;
    const hx = dx / length;
    const hy = dy / length;
    const ex = x - a.position[0];
    const ey = y - a.position[1];
    const along = Math.max(0, Math.min(length, ex * hx + ey * hy));
    const distance = Math.hypot(ex - hx * along, ey - hy * along);
    if (nearest === null || distance < nearest.distance)
      nearest = {
        hx,
        hy,
        distance,
        station: a.s_m + ((b.s_m - a.s_m) * along) / length,
      };
  }
  return nearest;
}

function isRoadSide({ nx, ny }, station) {
  return Math.abs(nx * station.hx + ny * station.hy) < 0.55;
}

export function highwayRailSegments(surface) {
  if (!surface.highwayRails) return [];
  return surface.boundary.filter((edge) => {
    const { a, b } = edge;
    const station = roadStation(
      surface.profile,
      (a[0] + b[0]) / 2,
      (a[1] + b[1]) / 2,
    );
    return (
      station &&
      isRoadSide(edge, station) &&
      Number.isFinite(a[2]) &&
      Number.isFinite(b[2])
    );
  });
}

function outsideEdge(boundary, x, y) {
  let edge = null;
  let distance = 0.00001;
  for (const candidate of boundary) {
    const gap =
      (x - candidate.a[0]) * candidate.nx + (y - candidate.a[1]) * candidate.ny;
    if (gap <= distance) continue;
    distance = gap;
    edge = candidate;
  }
  return edge;
}

// Resolve a low-energy strike as a slide or rebound. Severity follows the
// velocity into the rail, so a fast shallow scrape can remain controllable.
export function barrierImpact(next, { nx, ny }, dt) {
  const vx = Math.cos(next.heading) * next.v;
  const vy = Math.sin(next.heading) * next.v;
  const normal = vx * nx + vy * ny;
  if (normal <= 0) return null;
  if (normal >= 12 || (Math.abs(next.v) >= 28 && normal >= 5.5))
    return { roadImpact: "highway barrier" };
  const tangent = vx * -ny + vy * nx;
  const drag = 0.35 * normal + 2 * dt;
  const slide = Math.sign(tangent) * Math.max(0, Math.abs(tangent) - drag);
  const rebound = normal >= 2.5 ? 0.18 * normal : 0;
  const outX = -ny * slide - nx * rebound;
  const outY = nx * slide - ny * rebound;
  const speed = Math.hypot(outX, outY);
  const sign =
    Math.cos(next.heading) * outX + Math.sin(next.heading) * outY < 0 ? -1 : 1;
  return {
    heading: speed > 0 ? Math.atan2(outY * sign, outX * sign) : next.heading,
    v: speed * sign,
    steering: 0,
    lateralAcceleration: 0,
    edgeContact: rebound ? "bounce" : "scrape",
  };
}

export function leaveRoadSide(car, input, dt, surface) {
  if (!surface.boundary?.length) return null;
  const next = roadMotion(car, input, dt, surface.heightAt);
  const station = roadStation(surface.profile, next.x, next.y);
  if (
    !station ||
    station.station < surface.profile[0].s_m + sideClearance ||
    station.station > surface.profile.at(-1).s_m - sideClearance
  )
    return null;
  const hx = Math.cos(next.heading);
  const hy = Math.sin(next.heading);
  const z = surface.heightAt(next.x, next.y);
  const footprint = [[next.x, next.y]];
  for (const axle of [-1.35, 1.35])
    for (const wheel of [-0.75, 0.75])
      footprint.push([
        next.x + hx * axle - hy * wheel,
        next.y + hy * axle + hx * wheel,
      ]);
  let side = null;
  for (const [x, y] of footprint) {
    if (surface.heightAt(x, y) !== null) continue;
    const edge = outsideEdge(surface.boundary, x, y);
    if (!edge || !isRoadSide(edge, station)) return null;
    side ||= edge;
  }
  if (!side || (next.x - car.x) * side.nx + (next.y - car.y) * side.ny <= 0)
    return null;
  if (surface.railSegments?.includes(side)) {
    const impact = barrierImpact(next, side, dt);
    return impact && { ...car, ...impact, blocked: false };
  }
  if (z !== null)
    return { ...car, ...next, z, blocked: false, edgeContact: "" };
  const startZ = car.z ?? surface.heightAt(car.x, car.y);
  if (startZ === null) return null;
  return {
    ...car,
    ...next,
    z: startZ,
    vx: next.v * hx,
    vy: next.v * hy,
    vz: 0,
    grounded: false,
    leavingRoad: true,
    seam: null,
    blocked: false,
    edgeContact: "",
  };
}

export function fallFromRoad(car, dt, landingBelow) {
  const x = car.x + car.vx * dt;
  const y = car.y + car.vy * dt;
  const z = car.z + car.vz * dt - 0.5 * gravity * dt * dt;
  const vz = car.vz - gravity * dt;
  const landing = landingBelow(x, y, car.z);
  if (landing && z <= landing.z)
    return {
      ...car,
      x,
      y,
      z: landing.z,
      surfaceId: landing.surfaceId,
      grounded: true,
      leavingRoad: false,
      seam: null,
      vz: 0,
      roadImpact: -vz > 10 ? "ground" : null,
    };
  return { ...car, x, y, z, vz, roadImpact: z < -30 ? "fall" : null };
}
