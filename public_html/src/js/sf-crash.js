// Passive rigid chassis in east/north/up metres, stepped on the driving clock.
// Impulses and angular momentum are per unit mass (the obstacle is stationary).
import { Matrix4, Quaternion, Vector3 } from "../vendor/three/three.core.js";
import {
  CAR_HALF_LENGTH,
  CAR_HALF_WIDTH,
  COM_HEIGHT,
  obstacleContact,
  wallContact,
} from "./sf-chassis-contact.js";

const DT = 1 / 120;
const CRASH_TICKS = 240;
const vec = (x = 0, y = 0, z = 0) => new Vector3(x, y, z);
const up = vec(0, 0, 1);
const halfHeight = 0.86;
const invInertia = vec(
  3 / (CAR_HALF_WIDTH ** 2 + halfHeight ** 2),
  3 / (CAR_HALF_LENGTH ** 2 + halfHeight ** 2),
  3 / (CAR_HALF_LENGTH ** 2 + CAR_HALF_WIDTH ** 2),
);
// Lower chassis and narrower, shorter cabin roof, relative to mass centre.
const corners = [];
for (const height of [0, 1.15, 1.8])
  for (const along of [-1, 1])
    for (const across of [-1, 1])
      corners.push(
        vec(
          height === 1.8 ? -0.3 + along * 1.1 : along * CAR_HALF_LENGTH,
          across * (height === 1.8 ? 0.825 : CAR_HALF_WIDTH),
          height - COM_HEIGHT,
        ),
      );

function inverseInertia(crash, v) {
  return v
    .clone()
    .applyQuaternion(crash.rotation.clone().invert())
    .multiply(invInertia)
    .applyQuaternion(crash.rotation);
}

function motion(car) {
  const hx = Math.cos(car.heading),
    hy = Math.sin(car.heading);
  const slip = car.slip || 0;
  return vec(
    car.grounded === false ? car.vx : hx * car.v - hy * slip,
    car.grounded === false ? car.vy : hy * car.v + hx * slip,
    car.vz || 0,
  );
}

export function seriousImpact(car, hit) {
  const velocity = motion(car);
  return -(velocity.x * hit.nx + velocity.y * hit.ny) >= 5.5;
}

// Keep the tangential component of a graze. Tyres regain sideways grip over
// subsequent ticks; the driver can steer or reverse out of a low-speed bump.
export function scrapeObstacle(car, hit) {
  const v = motion(car);
  const into = Math.min(0, v.x * hit.nx + v.y * hit.ny);
  v.x -= into * hit.nx;
  v.y -= into * hit.ny;
  const tangent = Math.hypot(v.x, v.y);
  const drag = tangent ? Math.max(0, 1 + (0.32 * into) / tangent) : 0;
  const vx = v.x * drag,
    vy = v.y * drag;
  const hx = Math.cos(car.heading),
    hy = Math.sin(car.heading);
  return {
    ...car,
    x: car.x + hit.nx * (hit.depth + 0.001),
    y: car.y + hit.ny * (hit.depth + 0.001),
    vx,
    vy,
    v: vx * hx + vy * hy,
    slip: -vx * hy + vy * hx,
  };
}

function groundNormal(heightAt, x, y, z) {
  const d = 0.35;
  const east = heightAt(x + d, y) ?? z,
    west = heightAt(x - d, y) ?? z;
  const north = heightAt(x, y + d) ?? z,
    south = heightAt(x, y - d) ?? z;
  return vec(west - east, south - north, 2 * d).normalize();
}

// Sample both sides during flight. A fast wheel strike on a bank can start
// the passive chassis before the centre point reaches the lower pavement.
export function hardLanding(before, next, heightAt) {
  if (before.grounded !== false) return null;
  const incoming = {
    ...before,
    x: next.x,
    y: next.y,
    z: before.z + before.vz * DT - 0.5 * 9.81 * DT * DT,
    vz: before.vz - 9.81 * DT,
    surfaceId: next.surfaceId,
  };
  const v = motion(incoming);
  const hx = Math.cos(before.heading),
    hy = Math.sin(before.heading);
  const pitch = Math.atan2(v.z, Math.max(0.1, Math.abs(before.v)));
  for (const along of [-1.35, 1.35, 0])
    for (const left of along === 0 ? [0] : [-0.91, 0.91]) {
      const x = incoming.x + along * Math.cos(pitch) * hx - left * hy;
      const y = incoming.y + along * Math.cos(pitch) * hy + left * hx;
      const z = heightAt(x, y);
      if (
        z !== null &&
        incoming.z + along * Math.sin(pitch) <= z &&
        v.dot(groundNormal(heightAt, x, y, z)) < -10
      )
        return incoming;
    }
  return null;
}

function applyImpulse(crash, r, axis, j) {
  crash.velocity.addScaledVector(axis, j);
  crash.momentum.addScaledVector(r.clone().cross(axis), j);
}

function pointVelocity(crash, r) {
  return inverseInertia(crash, crash.momentum).cross(r).add(crash.velocity);
}

function effectiveMass(crash, r, n) {
  const arm = r.clone().cross(n);
  return 1 / (1 + arm.dot(inverseInertia(crash, arm)));
}

function resolve(crash, point, n, friction, patchRadius = 0) {
  const r = point.clone().sub(crash.position);
  const speed = pointVelocity(crash, r).dot(n);
  if (speed >= 0) return;
  const restitution = speed < -2 ? 0.12 : 0;
  const j = -(1 + restitution) * speed * effectiveMass(crash, r, n);
  applyImpulse(crash, r, n, j);
  const tangent = pointVelocity(crash, r);
  tangent.addScaledVector(n, -tangent.dot(n));
  const slip = tangent.length();
  let shear = 0;
  if (slip > 1e-8) {
    tangent.divideScalar(slip);
    shear = Math.min(j * friction, slip * effectiveMass(crash, r, tangent));
    applyImpulse(crash, r, tangent, -shear);
  }
  if (patchRadius > 0) {
    // Distributed sliding contact can also slow rotation about the patch normal.
    const spin = inverseInertia(crash, crash.momentum).dot(n);
    const stop = Math.abs(spin) / n.dot(inverseInertia(crash, n));
    const budget =
      patchRadius * Math.sqrt(Math.max(0, (j * friction) ** 2 - shear ** 2));
    crash.momentum.addScaledVector(
      n,
      -Math.sign(spin) * Math.min(stop, budget),
    );
  }
}

function resolveObstacle(crash, hit) {
  const n = vec(hit.nx, hit.ny, hit.nz || 0);
  resolve(crash, vec(hit.x, hit.y, hit.z), n, 0.32);
  crash.position.addScaledVector(n, hit.depth + 0.001);
}

export function beginCrash(car, kind, hit, heightAt) {
  const normal =
    car.grounded === false ? up : groundNormal(heightAt, car.x, car.y, car.z);
  const forward = vec(Math.cos(car.heading), Math.sin(car.heading), 0);
  if (car.grounded === false)
    forward.z = (car.vz || 0) / Math.max(0.1, Math.abs(car.v));
  else forward.z = -(forward.x * normal.x + forward.y * normal.y) / normal.z;
  forward.normalize();
  const left = normal.clone().cross(forward).normalize();
  const bodyUp = forward.clone().cross(left);
  const rotation = new Quaternion().setFromRotationMatrix(
    new Matrix4().makeBasis(forward, left, bodyUp),
  );
  const crash = {
    kind,
    rotation,
    ticks: 0,
    initialHeading: car.heading,
    position: vec(car.x, car.y, car.z).addScaledVector(bodyUp, COM_HEIGHT),
    velocity: motion(car),
    momentum: vec(),
  };
  if (car.grounded !== false) {
    crash.velocity.z =
      -(crash.velocity.x * normal.x + crash.velocity.y * normal.y) / normal.z;
    const yaw = car.v ? (car.lateralAcceleration || 0) / car.v : 0;
    crash.momentum.copy(bodyUp).multiplyScalar(yaw / invInertia.z);
  }
  if (hit) resolveObstacle(crash, hit);
  crash.bounds = bodyBounds(bodyPoints(crash));
  return crash;
}

function bodyPoints(crash) {
  return corners.map((p) =>
    p.clone().applyQuaternion(crash.rotation).add(crash.position),
  );
}

function bodyBounds(points) {
  return {
    low: Math.min(...points.map((p) => p.z)),
    high: Math.max(...points.map((p) => p.z)),
  };
}

// Edges of the lower box and cabin. Clip the tipped chassis to an obstacle's
// actual height before projecting, so a raised wheel can pass above a low rail.
const chassisEdges = [];
for (let i = 0; i < corners.length; i++) {
  if (!(i & 1)) chassisEdges.push([i, i + 1]);
  if (!(i & 2)) chassisEdges.push([i, i + 2]);
  if (i + 4 < corners.length) chassisEdges.push([i, i + 4]);
}

function sliceHull(points, base, top, centreZ) {
  const section = points.filter((p) => p.z >= base && p.z <= top);
  for (const [i, j] of chassisEdges) {
    const a = points[i],
      b = points[j];
    for (const z of [base, top]) {
      if ((a.z - z) * (b.z - z) >= 0) continue;
      section.push(a.clone().lerp(b, (z - a.z) / (b.z - a.z)));
    }
  }
  return section.length < 3 ? null : projectedHull(section, centreZ);
}

function projectedHull(points, centreZ) {
  const sorted = [...points].sort((a, b) => a.x - b.x || a.y - b.y);
  const turn = (a, b, c) =>
    (b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x);
  const chain = (list) => {
    const hull = [];
    for (const p of list) {
      while (hull.length >= 2 && turn(hull.at(-2), hull.at(-1), p) <= 1e-10)
        hull.pop();
      hull.push(p);
    }
    hull.pop();
    return hull;
  };
  const polygon = chain(sorted).concat(chain(sorted.reverse()));
  return { polygon, ...bodyBounds(points), centreZ };
}

function railContact(hull, rails) {
  let best = null;
  for (const { a, b, nx, ny } of rails) {
    if (
      Math.max(a[2], b[2]) + 0.65 < hull.low ||
      Math.min(a[2], b[2]) > hull.high
    )
      continue;
    const nearby = wallContact(hull, a, b, -nx, -ny, "highway barrier", 0);
    if (!nearby) continue;
    const section = hull.section(
      Math.min(a[2], b[2]),
      Math.max(a[2], b[2]) + 0.65,
    );
    if (!section) continue;
    const hit = wallContact(section, a, b, -nx, -ny, "highway barrier", 0);
    if (!hit) continue;
    const t =
      Math.hypot(hit.x - a[0], hit.y - a[1]) /
      Math.hypot(b[0] - a[0], b[1] - a[1]);
    const base = a[2] + (b[2] - a[2]) * t;
    if (hull.low > base + 0.65 || hull.high < base) continue;
    hit.z = Math.max(hull.low, Math.min(hull.high, base + 0.33));
    // Adjacent finite spans describe the same rail; favour the contact nearest
    // the incident feature rather than an arbitrary tessellation endpoint.
    if (
      !best ||
      hit.depth > best.depth + 1e-6 ||
      (Math.abs(hit.depth - best.depth) <= 1e-6 && hit.arm < best.arm)
    )
      best = hit;
  }
  return best;
}

export function crashRemaining(crash) {
  return (CRASH_TICKS - crash.ticks) * DT;
}

// No controls parameter: both manual and automatic drivers are disconnected.
export function stepCrash(crash, heightAt, indexes = [], rails = []) {
  if (crash.ticks >= CRASH_TICKS) return;
  crash.ticks++;
  crash.velocity.z -= 9.81 * DT;
  crash.position.addScaledVector(crash.velocity, DT);
  const omega = inverseInertia(crash, crash.momentum);
  const speed = omega.length();
  if (speed > 1e-9)
    crash.rotation
      .premultiply(
        new Quaternion().setFromAxisAngle(
          omega.divideScalar(speed),
          speed * DT,
        ),
      )
      .normalize();
  const points = bodyPoints(crash);
  const hull = projectedHull(points, crash.position.z);
  hull.section = (low, high) => sliceHull(points, low, high, crash.position.z);
  const hit =
    obstacleContact(indexes, hull, crash.bounds) || railContact(hull, rails);
  if (hit) {
    resolveObstacle(crash, hit);
    for (const p of points) {
      p.x += hit.nx * (hit.depth + 0.001);
      p.y += hit.ny * (hit.depth + 0.001);
      p.z += (hit.nz || 0) * (hit.depth + 0.001);
    }
  }
  // Deepest support determines the current contact patch. Symmetric landings
  // share the load; a single low corner applies torque without a scripted flip.
  const contacts = [];
  let deepest = -Infinity;
  for (const point of points) {
    const z = heightAt(point.x, point.y);
    if (z === null || point.z > z + 0.001) continue;
    const depth = z - point.z;
    contacts.push({ point, depth, z });
    deepest = Math.max(deepest, depth);
  }
  crash.bounds = bodyBounds(points);
  if (!contacts.length) return;
  const patch = vec(),
    normal = vec();
  let count = 0;
  for (const { point, depth, z } of contacts) {
    if (depth < deepest - 0.003) continue;
    patch.add(point);
    normal.add(groundNormal(heightAt, point.x, point.y, z));
    count++;
  }
  patch.divideScalar(count);
  const radius = contacts.reduce(
    (sum, { point, depth }) =>
      sum + (depth >= deepest - 0.003 ? point.distanceTo(patch) / count : 0),
    0,
  );
  resolve(crash, patch, normal.normalize(), 0.65, radius);
  const correction = Math.max(0, deepest);
  crash.position.z += correction;
  crash.bounds.low += correction;
  crash.bounds.high += correction;
}

export function crashCar(crash, car) {
  const root = vec(0, 0, -COM_HEIGHT)
    .applyQuaternion(crash.rotation)
    .add(crash.position);
  const forward = vec(1, 0, 0).applyQuaternion(crash.rotation);
  return {
    ...car,
    x: root.x,
    y: root.y,
    z: root.z,
    heading: Math.atan2(forward.y, forward.x),
    v: Math.hypot(crash.velocity.x, crash.velocity.y),
    steering: 0,
    steeringRate: 0,
    lateralAcceleration: 0,
  };
}
