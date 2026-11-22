import { contactTriangles, roadMotion } from "./sf-road-contact.js";
import { CAR_HALF_LENGTH, CAR_HALF_WIDTH } from "./sf-drive-core.js";

const gravity = 9.81;
const sideClearance = 4;

function turn(a, b, c) {
  return (b[0] - a[0]) * (c[1] - a[1]) - (b[1] - a[1]) * (c[0] - a[0]);
}

function roadEdge(a, b) {
  const length = Math.hypot(b[0] - a[0], b[1] - a[1]);
  return {
    a,
    b,
    length,
    nx: (b[1] - a[1]) / length,
    ny: (a[0] - b[0]) / length,
  };
}

// Shared triangle edges are inside the contact mesh. On the remaining
// boundary, the supported side is on the left, so holes wind clockwise.
export function roadBoundary(contact) {
  const perimeter = new Map();
  for (const { indices, vertices } of contactTriangles(contact)) {
    const winding = turn(...vertices);
    if (Math.abs(winding) < 1e-10) continue;
    const [a, b, c] = winding > 0 ? indices : [...indices].reverse();
    for (const [from, to] of [
      [a, b],
      [b, c],
      [c, a],
    ]) {
      const key = from < to ? `${from}:${to}` : `${to}:${from}`;
      if (perimeter.has(key)) perimeter.delete(key);
      else perimeter.set(key, { from, to });
    }
  }
  const outgoing = new Map();
  for (const edge of perimeter.values()) {
    if (!outgoing.has(edge.from)) outgoing.set(edge.from, []);
    outgoing.get(edge.from).push(edge);
  }
  const exterior = [];
  const remaining = new Set(perimeter.values());
  for (const first of remaining) {
    const loop = [];
    let edge = first;
    let area = 0;
    while (edge && remaining.delete(edge)) {
      const a = contact.vertices[edge.from];
      const b = contact.vertices[edge.to];
      loop.push(roadEdge(a, b));
      area += turn(contact.vertices[first.from], a, b);
      if (edge.to === first.from) {
        if (area > 0) exterior.push(...loop);
        break;
      }
      edge = outgoing
        .get(edge.to)
        ?.find((candidate) => remaining.has(candidate));
    }
  }
  return exterior;
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
  const halfWidth = surface.highwayRailHalfWidth;
  if (!halfWidth) return [];
  const profile = surface.profile;
  const rails = [];
  for (const side of [-1, 1]) {
    if (
      surface.railSides &&
      !surface.railSides.includes(side < 0 ? "left" : "right")
    )
      continue;
    let previous = null;
    for (let i = 1; i < profile.length - 1; i++) {
      const {
        s_m,
        position: [x, y],
      } = profile[i];
      if (s_m < sideClearance || s_m > profile.at(-1).s_m - sideClearance)
        continue;
      const before = profile[i - 1].position;
      const after = profile[i + 1].position;
      const dx = after[0] - before[0];
      const dy = after[1] - before[1];
      const length = Math.hypot(dx, dy);
      if (!length) continue;
      const east = x + (side * dy * halfWidth) / length;
      const north = y - (side * dx * halfWidth) / length;
      const z = surface.heightAt(east, north);
      const point = z === null ? null : [east, north, z];
      if (previous && point)
        rails.push(
          side > 0 ? roadEdge(previous, point) : roadEdge(point, previous),
        );
      previous = point;
    }
  }
  return rails;
}

function outsideEdge(boundary, x, y) {
  let inside = false;
  let nearest = null;
  let distance = Infinity;
  for (const edge of boundary) {
    const { a, b, nx, ny, length } = edge;
    if (
      a[1] > y !== b[1] > y &&
      x < a[0] + ((b[0] - a[0]) * (y - a[1])) / (b[1] - a[1])
    )
      inside = !inside;
    const along = Math.max(
      0,
      Math.min(length, (x - a[0]) * -ny + (y - a[1]) * nx),
    );
    const gap = Math.hypot(x - a[0] + along * ny, y - a[1] - along * nx);
    if (gap < distance) {
      distance = gap;
      nearest = edge;
    }
  }
  return inside ? null : nearest;
}

// Resolve a low-energy strike as a slide or rebound. Severity follows the
// velocity into the rail, so a fast shallow scrape can remain controllable.
export function barrierImpact(next, { nx, ny }, dt, cornerSpeed = 0) {
  const vx = Math.cos(next.heading) * next.v;
  const vy = Math.sin(next.heading) * next.v;
  const normal = vx * nx + vy * ny;
  const approach = Math.max(normal, cornerSpeed);
  if (approach <= 0) return null;
  if (normal >= 12 || (Math.abs(next.v) >= 28 && normal >= 5.5))
    return { roadImpact: "highway barrier" };
  const tangent = vx * -ny + vy * nx;
  const drag = 0.35 * approach + 2 * dt;
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
    steeringRate: 0,
    lateralAcceleration: 0,
    edgeContact: rebound ? "bounce" : "scrape",
  };
}

function bodyReach(heading, x, y) {
  const hx = Math.cos(heading);
  const hy = Math.sin(heading);
  return (
    CAR_HALF_LENGTH * Math.abs(hx * x + hy * y) +
    CAR_HALF_WIDTH * Math.abs(hx * y - hy * x)
  );
}

// Test the same finite rail spans that are drawn, before looking at tyre
// support. This lets the bumper meet the rail while the tyres are on pavement.
export function strikeHighwayRail(car, input, dt, surface) {
  if (!surface.railSegments?.length) return null;
  const next = roadMotion(car, input, dt, surface.heightAt);
  let contact = null;
  for (const edge of surface.railSegments) {
    const { a, length, nx, ny } = edge;
    const dx = next.x - a[0];
    const dy = next.y - a[1];
    const normal = dx * nx + dy * ny;
    if (normal < -2.5 || normal > 2.5) continue;
    const along = -dx * ny + dy * nx;
    if (along < -2.5 || along > length + 2.5) continue;
    const reach = bodyReach(next.heading, nx, ny);
    const span = bodyReach(next.heading, -ny, nx);
    if (along + span <= 0 || along - span >= length) continue;
    const hx = Math.cos(next.heading);
    const hy = Math.sin(next.heading);
    const ax = -dx * hx - dy * hy;
    const ay = dx * hy - dy * hx;
    const bx = (edge.b[0] - next.x) * hx + (edge.b[1] - next.y) * hy;
    const by = -(edge.b[0] - next.x) * hy + (edge.b[1] - next.y) * hx;
    if (
      Math.min(ax, bx) > CAR_HALF_LENGTH ||
      Math.max(ax, bx) < -CAR_HALF_LENGTH ||
      Math.min(ay, by) > CAR_HALF_WIDTH ||
      Math.max(ay, by) < -CAR_HALF_WIDTH
    )
      continue;
    const penetration = normal + reach;
    const previousReach = bodyReach(car.heading, nx, ny);
    const previousNormal = (car.x - a[0]) * nx + (car.y - a[1]) * ny;
    if (
      penetration <= 1e-6 ||
      penetration <= previousNormal + previousReach + 1e-6
    )
      continue;
    if (!contact || penetration > contact.penetration)
      contact = {
        edge,
        penetration,
        cornerSpeed: (reach - previousReach) / dt,
      };
  }
  if (!contact) return null;
  const { edge, cornerSpeed } = contact;
  const impact = barrierImpact(next, edge, dt, cornerSpeed);
  if (!impact) return null;
  if (impact.roadImpact) return { ...car, ...impact, blocked: false };
  const { a, nx, ny } = edge;
  const inset = Math.max(
    0,
    (car.x - a[0]) * nx +
      (car.y - a[1]) * ny +
      bodyReach(impact.heading, nx, ny) +
      0.001,
  );
  const x = car.x - nx * inset;
  const y = car.y - ny * inset;
  return {
    ...car,
    ...impact,
    x,
    y,
    z: surface.heightAt(x, y) ?? car.z,
    blocked: false,
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
  if (!side) return null;
  if (z !== null)
    return { ...car, ...next, z, blocked: false, edgeContact: "" };
  if ((next.x - car.x) * side.nx + (next.y - car.y) * side.ny <= 0) return null;
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
      roadImpact: -vz > 10 || landing.z - car.z > 0.25 ? "ground" : null,
    };
  return { ...car, x, y, z, vz, roadImpact: z < -30 ? "fall" : null };
}

export function roadLandingAt(x, y, previousZ, terrainHeight, surfaces) {
  const terrain = terrainHeight(x, y);
  // Terrain is solid. A rising ground plane can intersect the car's horizontal
  // sweep even when the point under the previous pose was lower or missing.
  let landing = terrain === null ? null : { z: terrain };
  for (const surface of surfaces) {
    const z = surface.heightAt(x, y);
    if (z !== null && z <= previousZ && (!landing || z > landing.z))
      landing = { z, surfaceId: surface.id };
  }
  return landing;
}
