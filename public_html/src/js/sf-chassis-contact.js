// Metres, east/north/up. Normals point from the obstacle into free space.
export const CAR_HALF_LENGTH = 2.23;
export const CAR_HALF_WIDTH = 0.96;
export const HARD_LANDING_SPEED = 16; // m/s into the support normal
export const COM_HEIGHT = 0.78;

// Derivatives are sampled on a single support surface. At its boundary use
// the available one-sided difference; another deck can be many metres below.
export function roadGradient(heightAt, x, y, z) {
  const d = 0.35;
  function slope(a, b) {
    if (a !== null && b !== null) return (b - a) / (2 * d);
    if (a !== null) return (z - a) / d;
    return b !== null ? (b - z) / d : 0;
  }
  return {
    east: slope(heightAt(x - d, y), heightAt(x + d, y)),
    north: slope(heightAt(x, y - d), heightAt(x, y + d)),
  };
}

export function chassisAttitude(heading, { east, north }) {
  const hx = Math.cos(heading),
    hy = Math.sin(heading);
  const pitch = Math.atan(east * hx + north * hy);
  return {
    pitch,
    roll: Math.atan((-east * hy + north * hx) * Math.cos(pitch)),
  };
}

// Look back along the supported approach for pitch. A forward sample may
// already be below a cliff or beyond the end of a ramp during the launch tick.
export function launchAttitude(car, heightAt, z) {
  const hx = Math.cos(car.heading),
    hy = Math.sin(car.heading);
  const back = car.v < 0 ? -0.35 : 0.35;
  const behind = heightAt(car.x - back * hx, car.y - back * hy);
  const pitch =
    behind === null
      ? Math.atan((car.vz || 0) / (car.v || 1))
      : Math.atan((z - behind) / back);
  const left = heightAt(car.x - 0.35 * hy, car.y + 0.35 * hx);
  const right = heightAt(car.x + 0.35 * hy, car.y - 0.35 * hx);
  const across =
    left !== null && right !== null
      ? (left - right) / 0.7
      : left !== null
        ? (left - z) / 0.35
        : right !== null
          ? (z - right) / 0.35
          : 0;
  return { pitch, roll: Math.atan(across * Math.cos(pitch)) };
}

export function drivingHull(car, z = car.z ?? 0) {
  const hx = Math.cos(car.heading),
    hy = Math.sin(car.heading);
  const polygon = [];
  for (const [along, across] of [
    [-1, -1],
    [1, -1],
    [1, 1],
    [-1, 1],
  ]) {
    const a = along * CAR_HALF_LENGTH,
      b = across * CAR_HALF_WIDTH;
    polygon.push({ x: car.x + a * hx - b * hy, y: car.y + a * hy + b * hx });
  }
  return { polygon, low: z, high: z + 1.8, centreZ: z + COM_HEIGHT };
}

function closest(x, y, a, b) {
  const dx = b.x - a.x,
    dy = b.y - a.y;
  const t = Math.max(
    0,
    Math.min(1, ((x - a.x) * dx + (y - a.y) * dy) / (dx * dx + dy * dy || 1)),
  );
  return { x: a.x + t * dx, y: a.y + t * dy };
}

function treeContact(hull, tree) {
  let inside = true,
    nearest,
    distance = Infinity,
    edgeNX,
    edgeNY;
  for (let i = 0; i < hull.polygon.length; i++) {
    const a = hull.polygon[i],
      b = hull.polygon[(i + 1) % hull.polygon.length];
    const dx = b.x - a.x,
      dy = b.y - a.y,
      length = Math.hypot(dx, dy);
    if (dx * (tree.y - a.y) - dy * (tree.x - a.x) < 0) inside = false;
    const p = closest(tree.x, tree.y, a, b);
    const d = Math.hypot(p.x - tree.x, p.y - tree.y);
    if (d < distance) {
      distance = d;
      nearest = p;
      edgeNX = -dy / length;
      edgeNY = dx / length;
    }
  }
  if (!inside && distance >= tree.radius) return null;
  const sign = inside ? -1 : 1;
  return {
    kind: "tree",
    tree,
    x: nearest.x,
    y: nearest.y,
    z: Math.max(
      tree.base,
      hull.low,
      Math.min(tree.top, hull.high, hull.centreZ - 0.12),
    ),
    nx: distance > 1e-8 ? (sign * (nearest.x - tree.x)) / distance : edgeNX,
    ny: distance > 1e-8 ? (sign * (nearest.y - tree.y)) / distance : edgeNY,
    depth: tree.radius - sign * distance,
  };
}

// Clip a finite wall to the convex footprint. Average the deepest incident
// vertices on a flat face so a square bumper hit has no fictitious yaw arm.
export function wallContact(hull, a, b, nx, ny, kind, z) {
  const dx = b[0] - a[0],
    dy = b[1] - a[1];
  let low = 0,
    high = 1,
    depth = -Infinity,
    tx = 0,
    ty = 0,
    count = 0;
  for (let i = 0; i < hull.polygon.length; i++) {
    const p = hull.polygon[i],
      q = hull.polygon[(i + 1) % hull.polygon.length];
    const ex = q.x - p.x,
      ey = q.y - p.y;
    const side = ex * (a[1] - p.y) - ey * (a[0] - p.x);
    const rate = ex * dy - ey * dx;
    if (Math.abs(rate) < 1e-10) {
      if (side < -1e-8) return null;
    } else if (rate > 0) low = Math.max(low, -side / rate);
    else high = Math.min(high, -side / rate);
    if (low > high + 1e-8) return null;
    const penetration = (a[0] - p.x) * nx + (a[1] - p.y) * ny;
    if (penetration > depth + 1e-6) {
      depth = penetration;
      tx = 0;
      ty = 0;
      count = 0;
    }
    if (penetration >= depth - 1e-6) {
      tx += p.x;
      ty += p.y;
      count++;
    }
  }
  const t = Math.max(
    low,
    Math.min(
      high,
      ((tx / count - a[0]) * dx + (ty / count - a[1]) * dy) /
        (dx * dx + dy * dy),
    ),
  );
  return {
    kind,
    x: a[0] + t * dx,
    y: a[1] + t * dy,
    z,
    nx,
    ny,
    arm: Math.hypot(a[0] + t * dx - tx / count, a[1] + t * dy - ty / count),
    depth: Math.max(0, depth),
  };
}

export function wallRings(rings) {
  return rings.flatMap((ring, index) => {
    let area = 0;
    const edges = [];
    for (let i = 0; i < ring.length; i++) {
      const a = ring[i],
        b = ring[(i + 1) % ring.length];
      const dx = b[0] - a[0],
        dy = b[1] - a[1],
        length = Math.hypot(dx, dy);
      if (!length) continue;
      area += (a[0] - ring[0][0]) * dy - (a[1] - ring[0][1]) * dx;
      edges.push({ a, b, nx: dy / length, ny: -dx / length });
    }
    const sign = Math.sign(area) * (index === 0 ? 1 : -1);
    return edges.map((e) => ({ ...e, nx: e.nx * sign, ny: e.ny * sign }));
  });
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

function buildingContact(hull, shape) {
  const z = Math.max(
    shape.base,
    hull.low,
    Math.min(shape.top, hull.high, hull.centreZ - 0.12),
  );
  let best = null;
  for (const { a, b, nx, ny } of shape.walls) {
    const hit = wallContact(hull, a, b, nx, ny, shape.kind, z);
    if (hit && (!best || hit.depth < best.depth)) best = hit;
  }
  if (best) return best;
  // A restored/spawn candidate can already be wholly inside a solid polygon.
  const p = hull.polygon[0];
  if (
    !insideRing(p.x, p.y, shape.rings[0]) ||
    shape.rings.slice(1).some((r) => insideRing(p.x, p.y, r))
  )
    return null;
  for (const { a, b, nx, ny } of shape.walls) {
    const point = closest(p.x, p.y, { x: a[0], y: a[1] }, { x: b[0], y: b[1] });
    const depth = Math.hypot(p.x - point.x, p.y - point.y);
    if (!best || depth < best.depth)
      best = { kind: shape.kind, ...point, z, nx, ny, depth };
  }
  return best;
}

export function obstacleContact(indexes, hull, previous) {
  const xs = hull.polygon.map((p) => p.x),
    ys = hull.polygon.map((p) => p.y);
  const seen = new Set();
  let best = null;
  for (
    let bx = Math.floor(Math.min(...xs) / 32);
    bx <= Math.floor(Math.max(...xs) / 32);
    bx++
  )
    for (
      let by = Math.floor(Math.min(...ys) / 32);
      by <= Math.floor(Math.max(...ys) / 32);
      by++
    )
      for (const cells of indexes)
        for (const shape of cells.get(`${bx},${by}`) || []) {
          if (
            shape.broken ||
            seen.has(shape) ||
            hull.low > shape.top ||
            hull.high < shape.base
          )
            continue;
          seen.add(shape);
          const section =
            hull.section && (shape.base > hull.low || shape.top < hull.high)
              ? hull.section(shape.base, shape.top)
              : hull;
          if (!section || section.polygon.length < 3) continue;
          let hit =
            shape.kind === "tree"
              ? treeContact(section, shape)
              : buildingContact(section, shape);
          if (
            hit &&
            previous &&
            (previous.low >= shape.top - 0.002 ||
              previous.high <= shape.base + 0.002)
          ) {
            const above = previous.low >= shape.top - 0.002;
            const face = section.polygon.filter((p) =>
              shape.kind === "tree"
                ? Math.hypot(p.x - shape.x, p.y - shape.y) <= shape.radius
                : insideRing(p.x, p.y, shape.rings[0]) &&
                  !shape.rings.slice(1).some((r) => insideRing(p.x, p.y, r)),
            );
            hit = {
              kind: shape.kind,
              tree: hit.tree,
              x: face.length
                ? face.reduce((sum, p) => sum + p.x, 0) / face.length
                : hit.x,
              y: face.length
                ? face.reduce((sum, p) => sum + p.y, 0) / face.length
                : hit.y,
              z: above ? shape.top : shape.base,
              nx: 0,
              ny: 0,
              nz: above ? 1 : -1,
              depth: above ? shape.top - hull.low : hull.high - shape.base,
            };
          }
          if (hit && (!best || hit.depth > best.depth)) best = hit;
        }
  return best;
}
