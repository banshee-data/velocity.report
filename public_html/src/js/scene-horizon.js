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

/**
 * Fit one plane to observed terrain around the circuit. A horizontal circle
 * lifted onto this plane is an ellipse in 3D: one horizontal co-axis and one
 * inclined axis, with a single tilt angle about the horizontal co-axis.
 * Only the first harmonic is retained, so mesh bumps cannot change the path.
 */
export function groundOrbit(
  surface,
  { radius, centerX = 0, centerZ = 0, fallbackHeight = 0, samples = 128 },
) {
  const observed = [];
  for (let i = 0; i < samples; i++) {
    const angle = (2 * Math.PI * i) / samples;
    const x = Math.sin(angle),
      z = Math.cos(angle);
    const h = surface?.heightAt(centerX + radius * x, centerZ + radius * z);
    if (Number.isFinite(h)) observed.push([1, x, z, h]);
  }
  // Solve least squares h = centre + a sin(theta) + b cos(theta).
  // Pivoting rejects insufficient/collinear coverage rather than extrapolating
  // an arbitrary steep slope from a single observed cell.
  const matrix = Array.from({ length: 3 }, (_, r) =>
    Array.from({ length: 4 }, (_, c) =>
      observed.reduce((sum, p) => sum + p[r] * p[c], 0),
    ),
  );
  let fitted = observed.length >= 3;
  for (let c = 0; c < 3 && fitted; c++) {
    let pivot = c;
    for (let r = c + 1; r < 3; r++)
      if (Math.abs(matrix[r][c]) > Math.abs(matrix[pivot][c])) pivot = r;
    [matrix[c], matrix[pivot]] = [matrix[pivot], matrix[c]];
    const d = matrix[c][c];
    if (Math.abs(d) < samples * 1e-6) {
      fitted = false;
      break;
    }
    for (let k = c; k < 4; k++) matrix[c][k] /= d;
    for (let r = 0; r < 3; r++) {
      if (r === c) continue;
      const factor = matrix[r][c];
      for (let k = c; k < 4; k++) matrix[r][k] -= factor * matrix[c][k];
    }
  }
  const [centre, a, b] = fitted
    ? matrix.map((row) => row[3])
    : [
        observed.length
          ? observed.reduce((s, p) => s + p[3], 0) / observed.length
          : fallbackHeight,
        0,
        0,
      ];
  const amplitude = Math.hypot(a, b);
  const uphill = amplitude > 1e-9 ? [a / amplitude, b / amplitude] : [0, 1];
  return {
    coverage: observed.length / samples,
    centre,
    uphill,
    tilt: radius > 0 ? Math.atan2(amplitude, radius) : 0,
    heightAt: (angle) =>
      centre +
      amplitude * (uphill[0] * Math.sin(angle) + uphill[1] * Math.cos(angle)),
  };
}
