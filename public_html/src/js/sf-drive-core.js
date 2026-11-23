// Pure logic for the San Francisco driving view (/sf/): tile selection, ground
// height, car motion and street lookup. No three.js, no DOM, so Node's tests
// can exercise it directly.
//
// Coordinates are metres in the tiles' shared planar frame (EPSG:7131 less a
// fixed origin): x east, y north. Tile payloads store their own positions in
// decimetres from the tile origin `o`.

import { validBuildingFace } from "./sf-building-walls.js";
import { GROUND_MISSING, groundDetailAt } from "./sf-drive-surface.js";

import {
  drivingHull,
  launchAttitude,
  obstacleContact,
  wallRings,
} from "./sf-chassis-contact.js";
export { CAR_HALF_LENGTH, CAR_HALF_WIDTH } from "./sf-chassis-contact.js";

export { GROUND_MISSING } from "./sf-drive-surface.js";

/** Tiles to load and to drop around (x, y). `tiles` are manifest rows {t, x, y, half}. */
export function tilesAround(tiles, x, y, loadMargin = 900, dropMargin = 1600) {
  const want = [];
  const drop = [];
  for (const t of tiles) {
    const d = Math.max(Math.abs(t.x - x), Math.abs(t.y - y));
    if (d <= t.half + loadMargin) want.push({ t: t.t, d });
    else if (d > t.half + dropMargin) drop.push(t.t);
  }
  want.sort((a, b) => a.d - b.d);
  return { want: want.map((w) => w.t), drop };
}

/** Ground height (m, NAVD88) at tile-local (lx, ly) metres, or null outside the grid or on a gap. */
export function groundAt(g, lx, ly, allowGaps = true) {
  if (groundDetailAt(g.detail, lx, ly)) g = g.detail;
  const columns = g.columns ?? g.n;
  const rows = g.rows ?? g.n;
  const fx = (lx - g.x0) / g.s;
  const fy = (ly - g.y0) / g.s;
  if (fx < 0 || fy < 0 || fx > columns - 1 || fy > rows - 1) return null;
  const i = Math.min(Math.floor(fx), columns - 2);
  const j = Math.min(Math.floor(fy), rows - 2);
  const tx = fx - i;
  const ty = fy - j;
  const z = [
    g.z[j * columns + i],
    g.z[j * columns + i + 1],
    g.z[(j + 1) * columns + i],
    g.z[(j + 1) * columns + i + 1],
  ];
  // The renderer omits a quad with any missing corner. Physics must not drive on it.
  if (!allowGaps && z.includes(GROUND_MISSING)) return null;
  const w =
    tx + ty <= 1 ? [1 - tx - ty, tx, ty, 0] : [0, 1 - ty, 1 - tx, tx + ty - 1];
  let sum = 0;
  let wsum = 0;
  let plain = 0;
  let count = 0;
  for (let k = 0; k < 4; k++) {
    if (z[k] === GROUND_MISSING) continue;
    sum += (z[k] / 10) * w[k];
    wsum += w[k];
    plain += z[k] / 10;
    count += 1;
  }
  if (wsum > 1e-6) return sum / wsum;
  // On a node whose own height is missing: the other corners, equally weighted.
  return count ? plain / count : null;
}

/** Ground height at world (x, y) from whichever loaded tile covers it best (nearest origin). */
export function groundAtWorld(loaded, x, y, strict = false) {
  if (strict) {
    let nearest = null,
      distance = Infinity;
    for (const tile of loaded) {
      const d = Math.hypot(x - tile.o[0], y - tile.o[1]);
      if (d < distance) {
        nearest = tile;
        distance = d;
      }
    }
    return nearest
      ? groundAt(nearest.g, x - nearest.o[0], y - nearest.o[1], false)
      : null;
  }
  let best = null;
  let bestD = Infinity;
  for (const tile of loaded) {
    const d = Math.hypot(x - tile.o[0], y - tile.o[1]);
    if (d < bestD) {
      const z = groundAt(tile.g, x - tile.o[0], y - tile.o[1]);
      if (z !== null) {
        best = z;
        bestD = d;
      }
    }
  }
  return best;
}

// GTI-inspired game tune. Peak torque and power are a simplified envelope
// through an idealised transmission, with a traction-limited launch.
export const DRIVE_CAR = Object.freeze({
  mass: 1280, // kg
  power: 350 * 745.7, // W at the engine
  torque: 420, // N·m at the engine
  driveRatio: 8,
  efficiency: 0.88,
  tyreRadius: 0.36, // m
  brakeDeceleration: 1.25 * 9.81, // m/s² on level pavement
  topSpeed: 250 / 3.6, // m/s, 250 km/h limiter
});

/** Bicycle model with steering travel, tyre grip, drag and hill gravity. Metres / seconds. */
export function carStep(car, input, dt, surface = {}) {
  dt = Math.max(0, Math.min(dt, 0.05));
  const grip = surface.offroad ? 0.55 : 1;
  let v = car.v;
  const throttle = Math.max(-1, Math.min(1, input.throttle || 0));
  const brake = Math.min(1, Math.max(0, Number(input.brake) || 0));
  const moveToward = (a, b, max) => a + Math.max(-max, Math.min(max, b - a));
  const previousSteering = car.steering || 0;
  const previousSteeringRate = car.steeringRate || 0;
  const targetSteering =
    Math.max(-1, Math.min(1, input.steer || 0)) *
    (0.58 / (1 + Math.abs(v) / 20));
  // The wheel angle and its rate both carry through a key press or release.
  // Integrate the critically damped response with a constant target per step.
  const response = 12;
  const error = previousSteering - targetSteering;
  const transient = previousSteeringRate + response * error;
  const decay = Math.exp(-response * dt);
  const steering = targetSteering + (error + transient * dt) * decay;
  const steeringRate =
    (previousSteeringRate - response * transient * dt) * decay;
  // Down/S brakes before engaging reverse; braking never jumps through zero.
  const opposing = throttle * v < 0;
  const { mass, power, torque, driveRatio, efficiency, tyreRadius, topSpeed } =
    DRIVE_CAR;
  const driveForce = Math.min(
    (torque * driveRatio * efficiency) / tyreRadius,
    (power * efficiency) / Math.max(1, v),
    mass * 9.81 * 0.7 * grip,
  );
  let acceleration = 0;
  if (!opposing && !brake)
    acceleration = throttle * (throttle > 0 ? driveForce / mass : 2.6);
  const brakeLimit = DRIVE_CAR.brakeDeceleration * grip;
  // Include rolling resistance and drag in the straight-line braking target.
  const resistanceAt = (speed) =>
    (surface.offroad ? 1.8 : 0.15) +
    (0.5 * 1.225 * 0.74 * speed * speed) / mass;
  const brakeDemand = Math.max(brake, opposing ? Math.abs(throttle) : 0);
  const brakingAt = (speed) =>
    brakeDemand * Math.max(0, brakeLimit - resistanceAt(speed));
  const longitudinalDemand = Math.max(
    Math.abs(acceleration) / (10 * grip),
    Math.min(brakingAt(v), Math.abs(v) / Math.max(dt, 1e-9)) / brakeLimit,
  );
  const steerCurvature = Math.tan((previousSteering + steering) / 2) / 2.7;
  const slip = car.slip || 0;
  const cornerDemand = Math.min(
    1,
    Math.abs(v * v * steerCurvature) / (7.5 * grip),
  );
  const slideDemand = Math.min(
    1,
    Math.abs(slip) / (Math.max(dt, 1e-9) * 7.5 * grip),
  );
  const lateralDemand = Math.min(1, cornerDemand + slideDemand);
  const traction =
    1 / Math.max(1, Math.hypot(longitudinalDemand, lateralDemand));
  acceleration *= traction;
  const slope = surface.slope || 0;
  acceleration -= (9.81 * slope) / Math.hypot(1, slope);
  v += acceleration * dt;
  v = moveToward(v, 0, (resistanceAt(v) + brakingAt(v) * traction) * dt);
  if (
    brake &&
    Math.abs(car.v) < 0.05 &&
    Math.abs(acceleration) < brake * brakeLimit
  )
    v = 0;
  v = Math.max(-6, Math.min(topSpeed, v));
  const speed = (car.v + v) / 2;
  const desiredYaw = speed * steerCurvature;
  // Limit lateral acceleration to available tyre grip; speed affects turning radius.
  // Friction ellipse: cornering and catching a slide share the grip left by the pedals.
  const lateralGrip =
    7.5 *
    grip *
    Math.sqrt(Math.max(0, 1 - (longitudinalDemand * traction) ** 2));
  const slideShare = slideDemand / (cornerDemand + slideDemand || 1);
  const limit =
    (lateralGrip * (1 - slideShare)) /
    Math.max(1, Math.abs(v), Math.abs(speed));
  const yawRate = Math.max(-limit, Math.min(limit, desiredYaw));
  const heading = car.heading + yawRate * dt;
  const mid = car.heading + (yawRate * dt) / 2;
  const nextSlip = moveToward(
    slip,
    0,
    Math.max(0, lateralGrip - Math.abs(speed * yawRate)) * dt,
  );
  const sideways = (slip + nextSlip) / 2;
  return {
    slip: nextSlip,
    x: car.x - Math.sin(mid) * sideways * dt + Math.cos(mid) * speed * dt,
    y: car.y + Math.cos(mid) * sideways * dt + Math.sin(mid) * speed * dt,
    heading,
    v,
    steering,
    steeringRate,
    lateralAcceleration: v * yawRate,
  };
}

/** Physics: contact follows supplied geometry; missing geometry freezes rather than inventing a landing.
 * A support acceleration below -g releases contact. Airborne velocity has no tyre/input forces.
 * Fixed 120 Hz caller; point support is a game approximation, not a suspension model.
 */
export function contactStep(car, input, dt, surfaceAt, halfAxle = 0.1) {
  if (!(dt > 0) || dt > 0.05)
    throw new RangeError("contact step must be in (0, .05]");
  const here = surfaceAt(car.x, car.y);
  if (here.height === null) return { ...car, blocked: true };
  const z = car.z ?? here.height;
  let next;
  if (car.grounded !== false) {
    const a = surfaceAt(
      car.x + Math.cos(car.heading) * halfAxle,
      car.y + Math.sin(car.heading) * halfAxle,
    );
    const b = surfaceAt(
      car.x - Math.cos(car.heading) * halfAxle,
      car.y - Math.sin(car.heading) * halfAxle,
    );
    next = carStep(car, input, dt, {
      slope:
        ((a.height ?? here.height) - (b.height ?? here.height)) /
        (2 * halfAxle),
      offroad: here.drivable === false,
    });
  } else {
    next = {
      ...car,
      x: car.x + car.vx * dt,
      y: car.y + car.vy * dt,
      lateralAcceleration: 0,
    };
  }
  const there = surfaceAt(next.x, next.y);
  if (there.height === null) return { ...car, blocked: true };
  const ballisticZ = z + (car.vz || 0) * dt - 0.5 * 9.81 * dt * dt;
  if (car.grounded !== false && there.height >= ballisticZ - 1e-7) {
    return {
      ...car,
      ...next,
      z: there.height,
      // Use the endpoint tangent, not a backward secant (which would release
      // contact at half gravity on a curved surface).
      vz:
        (((surfaceAt(
          next.x + Math.cos(next.heading) * halfAxle,
          next.y + Math.sin(next.heading) * halfAxle,
        ).height ?? there.height) -
          (surfaceAt(
            next.x - Math.cos(next.heading) * halfAxle,
            next.y - Math.sin(next.heading) * halfAxle,
          ).height ?? there.height)) /
          (2 * halfAxle)) *
        next.v,
      grounded: true,
      blocked: false,
    };
  }
  if (car.grounded !== false) {
    // Recompute the launch interval from existing momentum: no launch-frame steering/braking.
    next = {
      ...car,
      airAttitude: launchAttitude(car, (x, y) => surfaceAt(x, y).height, z),
      vx:
        Math.cos(car.heading) * car.v - Math.sin(car.heading) * (car.slip || 0),
      vy:
        Math.sin(car.heading) * car.v + Math.cos(car.heading) * (car.slip || 0),
      lateralAcceleration: 0,
    };
  }
  next.x = car.x + next.vx * dt;
  next.y = car.y + next.vy * dt;
  const landing = surfaceAt(next.x, next.y);
  if (landing.height === null) return { ...car, blocked: true };
  const vz = (car.vz || 0) - 9.81 * dt;
  if (ballisticZ <= landing.height) {
    const ahead = surfaceAt(
      next.x + Math.cos(next.heading) * halfAxle,
      next.y + Math.sin(next.heading) * halfAxle,
    );
    const tangent =
      ahead.height === null
        ? 0
        : ((ahead.height - landing.height) / halfAxle) * next.v;
    return {
      ...car,
      ...next,
      z: landing.height,
      vz: tangent,
      grounded: true,
      blocked: false,
    };
  }
  return { ...next, z: ballisticZ, vz, grounded: false, blocked: false };
}

/** Space, reversing against motion, and a held brake illuminate the rear lamps. */
export function brakeLightsOn(car, input, held = false) {
  return held || !!input.brake || (input.throttle || 0) * car.v < -0.05;
}

/** The same nearest-origin ownership used by the clipped terrain and minimap. */
export function tileAt(tiles, x, y) {
  let nearest = null,
    distance = Infinity;
  for (const tile of tiles) {
    const d = Math.hypot(tile.x - x, tile.y - y);
    if (d < distance) {
      nearest = tile;
      distance = d;
    }
  }
  return nearest;
}

/** The closest uncollected target, independent of the tile-prioritised compass. */
export function nearestQuestTarget(trial, car) {
  let nearest = null;
  let bestDistance = Infinity;
  for (const target of trial.targets) {
    if (trial.collected.has(target.t)) continue;
    const distance = Math.hypot(target.x - car.x, target.y - car.y);
    if (distance < bestDistance) {
      nearest = target;
      bestDistance = distance;
    }
  }
  return nearest;
}

/** Prefer this tile's emoji even when another tile's target is nearer. */
export function questSeeker(trial, tiles, car) {
  const tile = tileAt(tiles, car.x, car.y);
  const local = trial.targets.find((t) => t.t === tile?.t) || null;
  const localCollected = !!local && trial.collected.has(local.t);
  let target = local && !localCollected ? local : null;
  if (!target)
    for (const candidate of trial.targets) {
      if (trial.collected.has(candidate.t)) continue;
      if (
        !target ||
        Math.hypot(candidate.x - car.x, candidate.y - car.y) <
          Math.hypot(target.x - car.x, target.y - car.y)
      )
        target = candidate;
    }
  const degrees = (angle) =>
    (((90 - (angle * 180) / Math.PI) % 360) + 360) % 360;
  const heading = degrees(car.heading);
  const bearing = target
    ? degrees(Math.atan2(target.y - car.y, target.x - car.x))
    : null;
  return {
    target,
    local,
    localCollected,
    onTile: !!target && target.t === tile?.t,
    distance: target ? Math.hypot(target.x - car.x, target.y - car.y) : null,
    heading,
    cardinal: ["N", "NE", "E", "SE", "S", "SW", "W", "NW"][
      Math.round(heading / 45) % 8
    ],
    bearing,
    relative: target ? ((bearing - heading + 540) % 360) - 180 : null,
  };
}

/*
 * Intersection centres come from the paired street signs on opposite corners.
 * Project their midpoint onto a named carriageway so respawns / pickups are on asphalt.
 */
export function intersectionsForTile(tile, roadTiles = [tile]) {
  const out = [];
  const used = new Set();
  for (let i = 0; i < tile.s.length; i++) {
    if (used.has(i)) continue;
    const a = tile.s[i];
    const ids = signNameIds(a).sort((x, y) => x - y);
    if (ids.length < 2) continue;
    let partner = -1;
    let nearest = 650;
    for (let j = i + 1; j < tile.s.length; j++) {
      if (
        used.has(j) ||
        signNameIds(tile.s[j])
          .sort((x, y) => x - y)
          .join() !== ids.join()
      )
        continue;
      const d = Math.hypot(a[0] - tile.s[j][0], a[1] - tile.s[j][1]);
      if (d < nearest) {
        nearest = d;
        partner = j;
      }
    }
    if (partner < 0) continue;
    used.add(partner);
    const b = tile.s[partner];
    const x = tile.o[0] + (a[0] + b[0]) / 20;
    const y = tile.o[1] + (a[1] + b[1]) / 20;
    const road = nearestRoad(
      roadTiles,
      x,
      y,
      new Set(ids.map((k) => tile.n[k])),
    );
    if (!road || road.distance > 22) continue;
    out.push({
      x: road.x,
      y: road.y,
      heading: road.heading,
      names: ids.map((k) => tile.n[k]),
    });
  }
  return out;
}

/** Closest point on a carriageway, with interpolated kerb widths. */
export function nearestRoad(tiles, x, y, names = null) {
  let best = null;
  for (const tile of tiles) {
    for (const r of tile.r) {
      if (!r[4].length || (names && !names.has(tile.n[r[0]]))) continue;
      const p = r[3],
        w = r[4];
      for (let i = 0; i + 3 < p.length; i += 2) {
        const a = tile.o[0] + p[i] / 10,
          b = tile.o[1] + p[i + 1] / 10;
        const dx = (p[i + 2] - p[i]) / 10,
          dy = (p[i + 3] - p[i + 1]) / 10;
        const u = Math.max(
          0,
          Math.min(1, ((x - a) * dx + (y - b) * dy) / (dx * dx + dy * dy || 1)),
        );
        const qx = a + u * dx,
          qy = b + u * dy;
        const distance = Math.hypot(x - qx, y - qy);
        if (best && distance >= best.distance) continue;
        const left = ((1 - u) * w[i] + u * w[i + 2]) / 10;
        const right = ((1 - u) * w[i + 1] + u * w[i + 3]) / 10;
        const side = dx * (y - qy) - dy * (x - qx);
        best = {
          x: qx,
          y: qy,
          heading: Math.atan2(dy, dx),
          distance,
          width: side >= 0 ? left : right,
          centreline: p,
          name: tile.n[r[0]],
          origin: tile.o,
          segment: i,
          fraction: u,
        };
      }
    }
  }
  return best;
}

/** A timed run keeps its fixed target set, including tiles that have streamed out. */
export function createTrial(targets) {
  return {
    targets,
    collected: new Set(),
    status: "ready",
    elapsed: 0,
    crashes: 0,
  };
}

export function advanceTrial(trial, dt, car, driving, allowPickup = true) {
  if (trial.status === "ready" && driving) trial.status = "running";
  if (trial.status !== "running") return [];
  trial.elapsed += Math.max(0, dt);
  const found = [];
  if (allowPickup)
    for (const t of trial.targets) {
      if (trial.collected.has(t.t) || Math.hypot(car.x - t.x, car.y - t.y) > 8)
        continue;
      trial.collected.add(t.t);
      found.push(t);
    }
  if (trial.targets.length && trial.collected.size === trial.targets.length)
    trial.status = "finished";
  return found;
}

export function formatTime(seconds) {
  const centis = Math.floor(seconds * 100);
  return `${Math.floor(centis / 6000)}:${String(Math.floor(centis / 100) % 60).padStart(2, "0")}.${String(centis % 100).padStart(2, "0")}`;
}

export function formatBytes(bytes) {
  return bytes >= 1e6
    ? `${(bytes / 1e6).toFixed(1)} MB`
    : `${(bytes / 1000).toFixed(1)} kB`;
}

/** 32 m spatial buckets avoid testing the whole city's buildings on every physics tick. */
export function buildCollisionIndex(tile) {
  const cells = new Map();
  const add = (shape, minX, minY, maxX, maxY) => {
    for (let x = Math.floor(minX / 32); x <= Math.floor(maxX / 32); x++)
      for (let y = Math.floor(minY / 32); y <= Math.floor(maxY / 32); y++) {
        const k = `${x},${y}`;
        if (!cells.has(k)) cells.set(k, []);
        cells.get(k).push(shape);
      }
  };
  for (const [x, y, h, r = 0] of tile.tr) {
    const wx = tile.o[0] + x / 10,
      wy = tile.o[1] + y / 10,
      radius = Math.max(0.12, 0.04 * h);
    const base =
      (tile.g && groundAt(tile.g, x / 10, y / 10)) ?? tile.zfill ?? 0;
    const crown = Math.max(1.5, Math.min(h * 0.6, 2.2 * r));
    add(
      {
        kind: "tree",
        x: wx,
        y: wy,
        radius,
        base,
        top: base + Math.max(0.8, h - crown),
      },
      wx - radius,
      wy - radius,
      wx + radius,
      wy + radius,
    );
  }
  for (const b of tile.b)
    for (const face of b[1]) {
      if (!validBuildingFace(face)) continue;
      const rings = face.map((r) => {
        const points = [];
        for (let i = 0; i < r.length; i += 3)
          points.push([tile.o[0] + r[i] / 10, tile.o[1] + r[i + 1] / 10]);
        return points;
      });
      const xs = rings[0].map((p) => p[0]),
        ys = rings[0].map((p) => p[1]);
      let top = -Infinity;
      for (const ring of face)
        for (let i = 2; i < ring.length; i += 3)
          top = Math.max(top, ring[i] / 10);
      add(
        {
          kind: "building",
          rings,
          walls: wallRings(rings),
          base: b[0] / 10,
          top,
        },
        Math.min(...xs),
        Math.min(...ys),
        Math.max(...xs),
        Math.max(...ys),
      );
    }
  return cells;
}

export function collisionAt(indexes, car, z = 0) {
  return obstacleContact(indexes, drivingHull(car, z))?.kind ?? null;
}

function segDistance(px, py, ax, ay, bx, by) {
  const dx = bx - ax;
  const dy = by - ay;
  const L2 = dx * dx + dy * dy;
  const t =
    L2 > 0
      ? Math.max(0, Math.min(1, ((px - ax) * dx + (py - ay) * dy) / L2))
      : 0;
  const qx = ax + t * dx - px;
  const qy = ay + t * dy - py;
  return { d: Math.hypot(qx, qy), dirx: dx, diry: dy };
}

/** Name of the street under or nearest to (x, y) within `reach` metres, preferring one aligned with `heading`. */
export function streetAt(loaded, x, y, heading, reach = 14) {
  let best = null;
  let bestScore = Infinity;
  const hx = Math.cos(heading);
  const hy = Math.sin(heading);
  for (const tile of loaded) {
    if (
      Math.max(Math.abs(x - tile.o[0]), Math.abs(y - tile.o[1])) >
      tile.half + reach
    )
      continue;
    const lx = (x - tile.o[0]) * 10;
    const ly = (y - tile.o[1]) * 10;
    for (const r of tile.r) {
      const p = r[3];
      for (let i = 0; i + 3 < p.length; i += 2) {
        const s = segDistance(lx, ly, p[i], p[i + 1], p[i + 2], p[i + 3]);
        const dm = s.d / 10;
        if (dm > reach) continue;
        const len = Math.hypot(s.dirx, s.diry) || 1;
        const align = Math.abs((s.dirx * hx + s.diry * hy) / len);
        const score = dm + (1 - align) * 6;
        if (score < bestScore) {
          bestScore = score;
          best = tile.n[r[0]] || null;
        }
      }
    }
  }
  return best;
}

/** Name ids on a signpost row [x, y, name, blade angle, name, blade angle, ...]. */
export function signNameIds(s) {
  const ids = [];
  for (let i = 2; i < s.length; i += 2) ids.push(s[i]);
  return ids;
}

/**
 * The next signposted intersection ahead within `ahead` metres: its street names other than `current`.
 * With a current street, only signs on that street count, so a nearby road's junction is not
 * reported as the next crossing.
 */
export function nextCrossing(loaded, x, y, heading, current, ahead = 140) {
  const hx = Math.cos(heading);
  const hy = Math.sin(heading);
  let best = null;
  let bestD = Infinity;
  for (const tile of loaded) {
    for (const s of tile.s) {
      const sx = tile.o[0] + s[0] / 10;
      const sy = tile.o[1] + s[1] / 10;
      const dx = sx - x;
      const dy = sy - y;
      const along = dx * hx + dy * hy;
      if (along < 8 || along > ahead) continue;
      const across = Math.abs(-dx * hy + dy * hx);
      if (across > 22) continue;
      if (along < bestD) {
        const all = signNameIds(s).map((k) => tile.n[k]);
        if (current && !all.includes(current)) continue;
        const names = all.filter((n) => n && n !== current);
        if (names.length) {
          bestD = along;
          best = { names, distance: along };
        }
      }
    }
  }
  return best;
}
