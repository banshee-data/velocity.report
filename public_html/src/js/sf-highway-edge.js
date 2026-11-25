import { roadMotion } from "./sf-road-contact.js";
import { CAR_HALF_LENGTH, CAR_HALF_WIDTH } from "./sf-drive-core.js";

import {
  drivingHull,
  wallContact,
  roadGradient,
  launchAttitude,
  HARD_LANDING_SPEED,
} from "./sf-chassis-contact.js";

const gravity = 9.81;
const sideClearance = 4;

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
        edgeWidths,
      } = profile[i];
      if (s_m < sideClearance || s_m > profile.at(-1).s_m - sideClearance)
        continue;
      const before = profile[i - 1].position;
      const after = profile[i + 1].position;
      const dx = after[0] - before[0];
      const dy = after[1] - before[1];
      const length = Math.hypot(dx, dy);
      if (!length) continue;
      const insetWidth = edgeWidths
        ? edgeWidths[side < 0 ? 0 : 1] - 0.35
        : halfWidth;
      if (insetWidth <= 0) {
        previous = null;
        continue;
      }
      const east = x + (side * dy * insetWidth) / length;
      const north = y - (side * dx * insetWidth) / length;
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

// Resolve a low-energy strike as a slide or rebound. Severity follows the
// velocity into the rail, so a fast shallow scrape can remain controllable.
export function barrierImpact(
  next,
  { nx, ny },
  dt,
  cornerSpeed = 0,
  bounceOnly = false,
) {
  const vx =
    Math.cos(next.heading) * next.v - Math.sin(next.heading) * (next.slip || 0);
  const vy =
    Math.sin(next.heading) * next.v + Math.cos(next.heading) * (next.slip || 0);
  const normal = vx * nx + vy * ny;
  const approach = Math.max(normal, cornerSpeed);
  if (approach <= 0) return null;
  if (!bounceOnly && normal >= 13) return { roadImpact: "highway barrier" };
  const tangent = vx * -ny + vy * nx;
  const drag = 0.35 * approach + 2 * dt;
  const slide = Math.sign(tangent) * Math.max(0, Math.abs(tangent) - drag);
  const rebound = bounceOnly
    ? 0.18 * approach
    : normal >= 2.5
      ? 0.18 * normal
      : 0;
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
    slip: 0,
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
  const impact = barrierImpact(
    next,
    edge,
    dt,
    cornerSpeed,
    surface.railBounceOnly,
  );
  if (!impact) return null;
  if (impact.roadImpact) {
    const z = surface.heightAt(next.x, next.y) ?? car.z;
    const hull = drivingHull(next, z);
    // Choose the actual incident feature across neighbouring short rail spans.
    let hit = null;
    for (const { a, b, nx, ny } of surface.railSegments) {
      const candidate = wallContact(
        hull,
        a,
        b,
        -nx,
        -ny,
        impact.roadImpact,
        z + 0.33,
      );
      if (candidate && (!hit || candidate.arm < hit.arm)) hit = candidate;
    }
    if (hit) return { ...car, ...next, ...impact, hit, z, blocked: false };
    return null;
  }
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

// The caller has checked rails, valid level transitions and pending tile
// owners. Straddle the lip on the remaining wheels, then take the deck's
// tangent into free flight when the car's centre loses contact.
export function leaveRoadSurface(car, input, dt, surface) {
  const startZ = surface.heightAt(car.x, car.y);
  if (startZ === null) return null;
  const next = roadMotion(car, input, dt, surface.heightAt);
  const z = surface.heightAt(next.x, next.y);
  if (z !== null)
    return { ...car, ...next, z, blocked: false, edgeContact: "" };
  const hx = Math.cos(next.heading);
  const hy = Math.sin(next.heading);
  const attitude = launchAttitude(car, surface.heightAt, startZ);
  return {
    ...car,
    ...next,
    z: startZ,
    vx: next.v * hx - (next.slip || 0) * hy,
    vy: next.v * hy + (next.slip || 0) * hx,
    vz:
      Math.tan(attitude.pitch) * next.v +
      (Math.tan(attitude.roll) / Math.cos(attitude.pitch)) * (next.slip || 0),
    grounded: false,
    leavingRoad: true,
    airAttitude: attitude,
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
  if (landing && z <= landing.z) {
    const { east, north } = landing.heightAt
      ? roadGradient(landing.heightAt, x, y, landing.z)
      : { east: 0, north: 0 };
    const closing =
      (car.vx * east + car.vy * north - vz) / Math.hypot(east, north, 1);
    const hx = Math.cos(car.heading),
      hy = Math.sin(car.heading);
    return {
      ...car,
      x,
      y,
      z: landing.z,
      surfaceId: landing.surfaceId,
      grounded: true,
      leavingRoad: false,
      seam: null,
      v: hx * car.vx + hy * car.vy,
      slip: hx * car.vy - hy * car.vx,
      vz: 0,
      roadImpact: closing >= HARD_LANDING_SPEED ? "ground" : null,
    };
  }
  return { ...car, x, y, z, vz, roadImpact: z < -30 ? "fall" : null };
}

export function roadLandingAt(
  x,
  y,
  previousZ,
  terrainHeight,
  surfaces,
  previousPoint,
) {
  const terrain = terrainHeight(x, y);
  // Terrain is solid. A rising ground plane can intersect the car's horizontal
  // sweep even when the point under the previous pose was lower or missing.
  let landing = terrain === null ? null : { z: terrain };
  for (const surface of surfaces) {
    if (surface.kind === "tunnel") continue;
    const z = surface.heightAt(x, y);
    if (z === null || (landing && z <= landing.z)) continue;
    if (z > previousZ) {
      // A point already on/above a rising deck can sweep into its top face.
      const prior =
        previousPoint && surface.heightAt(previousPoint.x, previousPoint.y);
      if (prior == null || prior > previousZ) continue;
    }
    landing = { z, surfaceId: surface.id };
  }
  return landing;
}

export function crashRoadSupport(terrainHeight, surfaces, tunnel) {
  return (x, y, previous) => {
    const tunnelHeight = tunnel?.heightAt(x, y);
    if (tunnelHeight != null)
      return {
        z: tunnelHeight,
        surfaceId: tunnel.id,
        heightAt: tunnel.heightAt,
      };
    const hit = roadLandingAt(
      x,
      y,
      previous.z + 0.002,
      terrainHeight,
      surfaces.values(),
      previous,
    );
    return (
      hit && {
        ...hit,
        heightAt: surfaces.get(hit.surfaceId)?.heightAt || terrainHeight,
      }
    );
  };
}

// Expand each rail's bucket coverage by the chassis bounding radius, so
// one lookup at its centre covers the tipped body on either side of a seam.
export function indexCrashRails(surfaces) {
  const cells = new Map();
  const bucket = (v) => Math.floor(v / 32);
  const radius = Math.hypot(CAR_HALF_LENGTH, CAR_HALF_WIDTH, 1.8);
  for (const surface of surfaces)
    for (const rail of surface.railSegments || []) {
      const { a, b } = rail;
      for (
        let x = bucket(Math.min(a[0], b[0]) - radius);
        x <= bucket(Math.max(a[0], b[0]) + radius);
        x++
      )
        for (
          let y = bucket(Math.min(a[1], b[1]) - radius);
          y <= bucket(Math.max(a[1], b[1]) + radius);
          y++
        ) {
          const key = `${x},${y}`;
          if (!cells.has(key)) cells.set(key, []);
          cells.get(key).push(rail);
        }
    }
  return (x, y) => cells.get(`${bucket(x)},${bucket(y)}`) || [];
}
