// Pure logic for the San Francisco driving view (/sf/): tile selection, ground
// height, car motion and street lookup. No three.js, no DOM, so Node's tests
// can exercise it directly.
//
// Coordinates are metres in the tiles' shared planar frame (EPSG:7131 less a
// fixed origin): x east, y north. Tile payloads store their own positions in
// decimetres from the tile origin `o`.

import { validBuildingFace } from "./sf-building-walls.js";

export const GROUND_MISSING = -32768;

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
  const n = g.n;
  const fx = (lx - g.x0) / g.s;
  const fy = (ly - g.y0) / g.s;
  if (fx < 0 || fy < 0 || fx > n - 1 || fy > n - 1) return null;
  const i = Math.min(Math.floor(fx), n - 2);
  const j = Math.min(Math.floor(fy), n - 2);
  const tx = fx - i;
  const ty = fy - j;
  const z = [
    g.z[j * n + i],
    g.z[j * n + i + 1],
    g.z[(j + 1) * n + i],
    g.z[(j + 1) * n + i + 1],
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
  const slope = surface.slope || 0;
  acceleration -= (9.81 * slope) / Math.hypot(1, slope);
  v += acceleration * dt;
  // Air density 1.225 kg/m³ and an assumed drag area of 0.74 m².
  const drag = (0.5 * 1.225 * 0.74 * v * v) / mass;
  const resistance = (surface.offroad ? 1.8 : 0.15) + drag;
  v = moveToward(v, 0, (resistance + (opposing ? 8 : brake * 10) * grip) * dt);
  if (
    brake &&
    Math.abs(car.v) < 0.05 &&
    Math.abs(acceleration) < brake * 10 * grip
  )
    v = 0;
  v = Math.max(-6, Math.min(topSpeed, v));
  const speed = (car.v + v) / 2;
  const desiredYaw =
    (speed / 2.7) * Math.tan((previousSteering + steering) / 2);
  // Limit lateral acceleration to available tyre grip; speed affects turning radius.
  const limit = (7.5 * grip) / Math.max(1, Math.abs(v), Math.abs(speed));
  const yawRate = Math.max(-limit, Math.min(limit, desiredYaw));
  const heading = car.heading + yawRate * dt;
  const mid = car.heading + (yawRate * dt) / 2;
  return {
    x: car.x + Math.cos(mid) * speed * dt,
    y: car.y + Math.sin(mid) * speed * dt,
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
      x: car.x + Math.cos(car.heading) * car.v * dt,
      y: car.y + Math.sin(car.heading) * car.v * dt,
      vx: Math.cos(car.heading) * car.v,
      vy: Math.sin(car.heading) * car.v,
      lateralAcceleration: 0,
    };
  }
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

export const CRASH_SECONDS = 2;

/** A crash is visual feedback; physics and pickups stay paused until recovery. */
export function beginCrash(car, kind, now) {
  return {
    x: car.x,
    y: car.y,
    heading: car.heading,
    kind,
    started: now,
    direction: car.steering < 0 ? -1 : 1,
  };
}

export function crashPose(crash, now) {
  const elapsed = Math.max(0, now - crash.started);
  const t = Math.min(1, elapsed / CRASH_SECONDS);
  return {
    heading: crash.heading + crash.direction * Math.PI * 4 * (1 - (1 - t) ** 2),
    roll: Math.sin(t * Math.PI * 4) * 0.16 * (1 - t),
    lift: Math.sin(t * Math.PI) * 0.15,
    remaining: Math.max(0, CRASH_SECONDS - elapsed),
    done: elapsed >= CRASH_SECONDS,
  };
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
        { kind: "building", rings, base: b[0] / 10, top },
        Math.min(...xs),
        Math.min(...ys),
        Math.max(...xs),
        Math.max(...ys),
      );
    }
  return cells;
}

function insideRing(x, y, ring) {
  let inside = false;
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [ax, ay] = ring[i],
      [bx, by] = ring[j];
    if (ay > y !== by > y && x < ((bx - ax) * (y - ay)) / (by - ay) + ax)
      inside = !inside;
  }
  return inside;
}

export const CAR_HALF_LENGTH = 2.23;
export const CAR_HALF_WIDTH = 0.96;

/** Oriented car rectangle against trunk circles and building polygons, including holes. */
export function collisionAt(indexes, car, z = 0) {
  const hx = Math.cos(car.heading),
    hy = Math.sin(car.heading),
    length = CAR_HALF_LENGTH,
    width = CAR_HALF_WIDTH;
  const local = ([x, y]) => [
    (x - car.x) * hx + (y - car.y) * hy,
    -(x - car.x) * hy + (y - car.y) * hx,
  ];
  const corners = [
    [-length, -width],
    [length, -width],
    [length, width],
    [-length, width],
  ].map(([x, y]) => [car.x + x * hx - y * hy, car.y + x * hy + y * hx]);
  const dx = Math.abs(hx) * length + Math.abs(hy) * width,
    dy = Math.abs(hy) * length + Math.abs(hx) * width;
  const seen = new Set();
  // Slab clipping detects an edge crossing the rectangle, even with both ends outside.
  const crosses = ([ax, ay], [bx, by]) => {
    let lo = 0,
      hi = 1;
    for (const [a, d, h] of [
      [ax, bx - ax, length],
      [ay, by - ay, width],
    ]) {
      if (Math.abs(d) < 1e-10) {
        if (Math.abs(a) > h) return false;
      } else {
        const t0 = (-h - a) / d,
          t1 = (h - a) / d;
        lo = Math.max(lo, Math.min(t0, t1));
        hi = Math.min(hi, Math.max(t0, t1));
      }
      if (lo > hi) return false;
    }
    return true;
  };
  for (const cells of indexes)
    for (
      let bx = Math.floor((car.x - dx) / 32);
      bx <= Math.floor((car.x + dx) / 32);
      bx++
    )
      for (
        let by = Math.floor((car.y - dy) / 32);
        by <= Math.floor((car.y + dy) / 32);
        by++
      )
        for (const shape of cells.get(`${bx},${by}`) || []) {
          if (seen.has(shape)) continue;
          seen.add(shape);
          if (shape.kind === "tree") {
            if (z > shape.top || z + 1.6 < shape.base) continue;
            const [x, y] = local([shape.x, shape.y]);
            if (
              Math.hypot(
                Math.max(0, Math.abs(x) - length),
                Math.max(0, Math.abs(y) - width),
              ) < shape.radius
            )
              return shape.kind;
          } else if (z + 1.6 >= shape.base && z <= shape.top) {
            if (
              corners.some(
                ([x, y]) =>
                  insideRing(x, y, shape.rings[0]) &&
                  !shape.rings.slice(1).some((r) => insideRing(x, y, r)),
              )
            )
              return shape.kind;
            for (const ring of shape.rings)
              for (let i = 0; i < ring.length; i++)
                if (crosses(local(ring[i]), local(ring[(i + 1) % ring.length])))
                  return shape.kind;
          }
        }
  return null;
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
