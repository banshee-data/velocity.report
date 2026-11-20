// Pure logic for the San Francisco driving view (/sf/): tile selection, ground
// height, car motion and street lookup. No three.js, no DOM, so Node's tests
// can exercise it directly.
//
// Coordinates are metres in the tiles' shared planar frame (EPSG:7131 less a
// fixed origin): x east, y north. Tile payloads store their own positions in
// decimetres from the tile origin `o`.

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
export function groundAt(g, lx, ly) {
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
  const w = [(1 - tx) * (1 - ty), tx * (1 - ty), (1 - tx) * ty, tx * ty];
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
export function groundAtWorld(loaded, x, y) {
  let best = null;
  let bestD = Infinity;
  for (const tile of loaded) {
    const d = Math.max(Math.abs(x - tile.o[0]), Math.abs(y - tile.o[1]));
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

/** One step of a simple bicycle-model car. `input` = {throttle, brake, steer} in [-1, 1]. */
export function carStep(car, input, dt) {
  const maxFwd = 27; // m/s, about 60 mph
  const maxRev = -6;
  const wheelbase = 2.7;
  let v = car.v;
  const throttle = Math.max(-1, Math.min(1, input.throttle || 0));
  if (input.brake) {
    v -= Math.sign(v) * Math.min(Math.abs(v), 9 * dt);
  } else if (throttle > 0) {
    v += (v < 0 ? 9 : 3.6) * throttle * dt;
  } else if (throttle < 0) {
    v += (v > 0 ? -9 : -2.5) * -throttle * dt;
  }
  const drag = 0.4 + 0.0012 * v * v;
  if (!input.brake && throttle === 0)
    v -= Math.sign(v) * Math.min(Math.abs(v), drag * dt);
  v = Math.max(maxRev, Math.min(maxFwd, v));
  const steerMax = 0.55 / (1 + Math.abs(v) / 11);
  const steer = Math.max(-1, Math.min(1, input.steer || 0)) * steerMax;
  const yawRate = (v / wheelbase) * Math.tan(steer);
  const heading = car.heading + yawRate * dt;
  return {
    x: car.x + Math.cos(heading) * v * dt,
    y: car.y + Math.sin(heading) * v * dt,
    heading,
    v,
  };
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

/** The next signposted intersection ahead within `ahead` metres: its street names other than `current`. */
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
        const names = s
          .slice(2)
          .map((k) => tile.n[k])
          .filter((n) => n && n !== current);
        if (names.length) {
          bestD = along;
          best = { names, distance: along };
        }
      }
    }
  }
  return best;
}
