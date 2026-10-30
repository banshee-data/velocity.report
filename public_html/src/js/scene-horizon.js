// True vertical comes from the accepted site registration, never from a
// sloping road. Arrays here use three.js axes (X, up Y, south Z).
// Horizontal axes retain the sensor's projected heading, not geodetic east/north.
const dot = (a, b) => a.reduce((sum, v, i) => sum + v * b[i], 0);
const cross = (a, b) => [
  a[1] * b[2] - a[2] * b[1],
  a[2] * b[0] - a[0] * b[2],
  a[0] * b[1] - a[1] * b[0],
];
const unit = (a) => {
  const n = Math.hypot(...a);
  return a.map((v) => v / n);
};

export function horizonBasis(upSensor) {
  const valid =
    Array.isArray(upSensor) &&
    upSensor.length === 3 &&
    upSensor.every(Number.isFinite) &&
    Math.hypot(...upSensor) > 1e-8;
  const up = valid ? unit([upSensor[0], upSensor[2], -upSensor[1]]) : [0, 1, 0];
  // Preserve the heading of sensor -Y projected onto the true horizon.
  let south = [0, 0, 1].map((v, i) => v - up[2] * up[i]);
  if (Math.hypot(...south) < 1e-8)
    south = [1, 0, 0].map((v, i) => v - up[0] * up[i]);
  south = unit(south);
  const east = unit(cross(up, south));
  return {
    up,
    east,
    south,
    toLevel: (p) => [dot(p, east), dot(p, up), dot(p, south)],
    fromLevel: (p) =>
      up.map((_, i) => east[i] * p[0] + up[i] * p[1] + south[i] * p[2]),
  };
}

/** Sample along true vertical, using only the supplied ground triangles. */
export function createGroundSurface(ground, basis) {
  if (!ground?.indices?.length) return null;
  const points = ground.positions.map((p) =>
    basis.toLevel([p[0], p[2], -p[1]]),
  );
  const bins = new Map();
  const cell = Math.max(1, Number(ground.cell_m) || 1) * 4;
  const key = (x, z) => `${x},${z}`;
  for (let i = 0; i < ground.indices.length; i += 3) {
    const triangle = ground.indices.slice(i, i + 3).map((k) => points[k]);
    if (triangle.some((p) => !p)) continue;
    const xs = triangle.map((p) => p[0]),
      zs = triangle.map((p) => p[2]);
    for (
      let x = Math.floor(Math.min(...xs) / cell);
      x <= Math.floor(Math.max(...xs) / cell);
      x++
    )
      for (
        let z = Math.floor(Math.min(...zs) / cell);
        z <= Math.floor(Math.max(...zs) / cell);
        z++
      ) {
        const k = key(x, z);
        if (!bins.has(k)) bins.set(k, []);
        bins.get(k).push(triangle);
      }
  }
  return {
    heightAt(x, z) {
      for (const [a, b, c] of bins.get(
        key(Math.floor(x / cell), Math.floor(z / cell)),
      ) ?? []) {
        const d = (b[2] - c[2]) * (a[0] - c[0]) + (c[0] - b[0]) * (a[2] - c[2]);
        if (Math.abs(d) < 1e-10) continue;
        const u = ((b[2] - c[2]) * (x - c[0]) + (c[0] - b[0]) * (z - c[2])) / d;
        const v = ((c[2] - a[2]) * (x - c[0]) + (a[0] - c[0]) * (z - c[2])) / d;
        if (u >= -1e-8 && v >= -1e-8 && u + v <= 1 + 1e-8)
          return u * a[1] + v * b[1] + (1 - u - v) * c[1];
      }
      return null;
    },
  };
}

/** A periodic, C1-continuous terrain profile; fill gaps without inventing steps. */
export function groundOrbit(
  surface,
  { radius, centerX = 0, centerZ = 0, fallbackHeight = 0, samples = 128 },
) {
  const raw = Array.from({ length: samples }, (_, i) => {
    const angle = (2 * Math.PI * i) / samples;
    return (
      surface?.heightAt(
        centerX + radius * Math.sin(angle),
        centerZ + radius * Math.cos(angle),
      ) ?? null
    );
  });
  const known = raw
    .map((h, i) => (h === null ? null : i))
    .filter((i) => i !== null);
  const values = raw.slice();
  if (!known.length) values.fill(fallbackHeight);
  else
    for (let k = 0; k < known.length; k++) {
      const a = known[k],
        b = known[(k + 1) % known.length];
      const gap = (b - a + samples) % samples || samples;
      for (let j = 1; j < gap; j++)
        values[(a + j) % samples] = raw[a] + ((raw[b] - raw[a]) * j) / gap;
    }
  // A narrow symmetric filter removes cell noise while preserving the hill's
  // rise and fall. The loop closes with matching height and vertical speed.
  const smooth = values.map(
    (_, i) =>
      [1, 4, 6, 4, 1].reduce(
        (s, w, k) => s + w * values[(i + k - 2 + samples) % samples],
        0,
      ) / 16,
  );
  return {
    coverage: known.length / samples,
    heightAt(angle) {
      const phase = ((((angle / (2 * Math.PI)) % 1) + 1) % 1) * samples;
      const i = Math.floor(phase),
        t = phase - i;
      const at = (k) => smooth[(k + samples) % samples];
      const a = at(i - 1),
        b = at(i),
        c = at(i + 1),
        d = at(i + 2);
      return (
        b +
        0.5 *
          t *
          (c - a + t * (2 * a - 5 * b + 4 * c - d + t * (3 * (b - c) + d - a)))
      );
    },
  };
}
