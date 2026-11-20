// Terrain ownership and clipping are shared by the renderer and the minimap.
// Adjacent ground grids overlap. Each point belongs to its nearest tile origin,
// matching groundAtWorld; clipping removes doubled surfaces at tile boundaries.
export function clipHalfPlane(poly, nx, ny, limit) {
  const out = [];
  for (let i = 0; i < poly.length; i++) {
    const a = poly[i],
      b = poly[(i + 1) % poly.length];
    const da = a[0] * nx + a[1] * ny - limit,
      db = b[0] * nx + b[1] * ny - limit;
    if (da <= 1e-7) out.push(a);
    if ((da < 0 && db > 0) || (da > 0 && db < 0)) {
      const t = da / (da - db);
      out.push(a.map((v, k) => v + t * (b[k] - v)));
    }
  }
  return out;
}

export function tileRegion(tile, tiles) {
  const { x, y, half: h } = tile;
  let poly = [
    [x - h, y - h],
    [x + h, y - h],
    [x + h, y + h],
    [x - h, y + h],
  ];
  for (const other of tiles) {
    if (other.t === tile.t || Math.hypot(other.x - x, other.y - y) > h * 4)
      continue;
    const dx = other.x - x,
      dy = other.y - y;
    // The perpendicular bisector assigns each point to its nearest origin.
    poly = clipHalfPlane(
      poly,
      dx,
      dy,
      (other.x * other.x + other.y * other.y - x * x - y * y) / 2,
    );
  }
  return poly;
}

/** Clip only boundary triangles; z is interpolated along their original planes. */
export function terrainTriangles(tile, region) {
  const { g, o } = tile;
  const positions = [],
    uv = [];
  const planes = region.map((a, i) => {
    const b = region[(i + 1) % region.length];
    return [
      b[1] - a[1],
      a[0] - b[0],
      (b[1] - a[1]) * a[0] + (a[0] - b[0]) * a[1],
    ];
  });
  const vertex = (i, j) => [
    o[0] + g.x0 + i * g.s,
    o[1] + g.y0 + j * g.s,
    g.z[j * g.n + i] / 10,
  ];
  for (let j = 0; j < g.n - 1; j++)
    for (let i = 0; i < g.n - 1; i++) {
      const a = j * g.n + i;
      if ([a, a + 1, a + g.n, a + g.n + 1].some((k) => g.z[k] === -32768))
        continue;
      const v = [
        vertex(i, j),
        vertex(i + 1, j),
        vertex(i, j + 1),
        vertex(i + 1, j + 1),
      ];
      for (const ids of [
        [0, 1, 2],
        [1, 3, 2],
      ]) {
        let poly = ids.map((k) => v[k]);
        for (const [nx, ny, d] of planes) {
          if (poly.every((p) => p[0] * nx + p[1] * ny <= d + 1e-7)) continue;
          poly = clipHalfPlane(poly, nx, ny, d);
          if (!poly.length) break;
        }
        for (let k = 1; k + 1 < poly.length; k++)
          for (const p of [poly[0], poly[k], poly[k + 1]]) {
            positions.push(p[0], p[2], -p[1]);
            uv.push(
              (p[0] - o[0] - g.x0) / ((g.n - 1) * g.s),
              (p[1] - o[1] - g.y0) / ((g.n - 1) * g.s),
            );
          }
      }
    }
  return { positions, uv };
}
