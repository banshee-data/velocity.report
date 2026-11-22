// Passive rigid chassis in east/north/up metres, stepped on the driving clock.
// Impulses and angular momentum are per unit mass (the obstacle is stationary).
import { Euler, Quaternion, Vector3 } from "../vendor/three/three.core.js";
import {
  CAR_HALF_LENGTH,
  CAR_HALF_WIDTH,
  COM_HEIGHT,
  obstacleContact,
  wallContact,
  roadGradient,
  chassisAttitude,
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

function groundNormal(support, x, y) {
  const { east, north } = roadGradient(support.heightAt, x, y, support.z);
  return vec(-east, -north, 1).normalize();
}

function orientation(heading, attitude) {
  return new Quaternion().setFromEuler(
    new Euler(attitude?.roll || 0, -(attitude?.pitch || 0), heading, "ZYX"),
  );
}

// Sample both sides during flight. A fast wheel strike on a bank can start
// the passive chassis before the centre point reaches the lower pavement.
export function hardLanding(before, next, supportAt) {
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
  const rotation = orientation(before.heading, before.airAttitude);
  for (const along of [-1.35, 1.35, 0])
    for (const left of along === 0 ? [0] : [-0.91, 0.91]) {
      const offset = vec(along, left, 0).applyQuaternion(rotation);
      const x = incoming.x + offset.x,
        y = incoming.y + offset.y;
      const support = supportAt(x, y, {
        x: before.x + offset.x,
        y: before.y + offset.y,
        z: before.z + offset.z,
      });
      if (
        support &&
        incoming.z + offset.z <= support.z &&
        v.dot(groundNormal(support, x, y)) < -10
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
  const attitude =
    car.grounded === false
      ? car.airAttitude
      : chassisAttitude(
          car.heading,
          roadGradient(heightAt, car.x, car.y, car.z),
        );
  const rotation = orientation(car.heading, attitude);
  const bodyUp = up.clone().applyQuaternion(rotation);
  const crash = {
    kind,
    rotation,
    ticks: 0,
    initialHeading: car.heading,
    grounded: car.grounded !== false,
    position: vec(car.x, car.y, car.z).addScaledVector(bodyUp, COM_HEIGHT),
    velocity: motion(car),
    momentum: vec(),
  };
  if (car.grounded !== false) {
    crash.velocity.z =
      -(crash.velocity.x * bodyUp.x + crash.velocity.y * bodyUp.y) / bodyUp.z;
    const yaw = car.v ? (car.lateralAcceleration || 0) / car.v : 0;
    crash.momentum.copy(bodyUp).multiplyScalar(yaw / invInertia.z);
  }
  if (hit) resolveObstacle(crash, hit);
  crash.points = bodyPoints(crash);
  crash.bounds = bodyBounds(crash.points);
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

function railContact(hull, rails, previousX, previousY) {
  let best = null;
  for (const { a, b, nx, ny } of rails) {
    if (
      Math.max(a[2], b[2]) + 0.65 < hull.low ||
      Math.min(a[2], b[2]) > hull.high
    )
      continue;
    const side = (previousX - a[0]) * nx + (previousY - a[1]) * ny > 0 ? 1 : -1;
    const nearby = wallContact(
      hull,
      a,
      b,
      side * nx,
      side * ny,
      "highway barrier",
      0,
    );
    if (!nearby) continue;
    const section = hull.section(
      Math.min(a[2], b[2]),
      Math.max(a[2], b[2]) + 0.65,
    );
    if (!section) continue;
    const hit = wallContact(
      section,
      a,
      b,
      side * nx,
      side * ny,
      "highway barrier",
      0,
    );
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
export function stepCrash(crash, supportAt, indexes = [], railsAt) {
  if (crash.ticks >= CRASH_TICKS) return;
  crash.ticks++;
  const { x: previousX, y: previousY } = crash.position;
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
    obstacleContact(indexes, hull, crash.bounds) ||
    (railsAt &&
      railContact(
        hull,
        railsAt(crash.position.x, crash.position.y),
        previousX,
        previousY,
      ));
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
  for (const [index, point] of points.entries()) {
    const support = supportAt(point.x, point.y, crash.points[index]);
    if (!support || point.z > support.z + 0.001) continue;
    const depth = support.z - point.z;
    contacts.push({ point, depth, support });
    deepest = Math.max(deepest, depth);
  }
  crash.grounded = contacts.length > 0 || hit?.nz === 1;
  crash.points = points;
  crash.bounds = bodyBounds(points);
  if (!contacts.length) return;
  const patch = vec(),
    normal = vec();
  let count = 0;
  for (const { point, depth, support } of contacts) {
    if (depth < deepest - 0.003) continue;
    patch.add(point);
    normal.add(groundNormal(support, point.x, point.y));
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
  for (const p of points) p.z += correction;
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
    vx: crash.velocity.x,
    vy: crash.velocity.y,
    vz: crash.velocity.z,
    grounded: crash.grounded,
    leavingRoad: false,
    steering: 0,
    steeringRate: 0,
    lateralAcceleration: 0,
  };
}
