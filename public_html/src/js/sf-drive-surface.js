// Terrain ownership and clipping are shared by the renderer and the minimap.
// Adjacent ground grids overlap. Each point belongs to its nearest tile origin,
// matching groundAtWorld; clipping removes doubled surfaces at tile boundaries.
export const GROUND_MISSING = -32768;
export const GROUND_TRIANGLES = [
  [0, 1, 2],
  [1, 3, 2],
];

// An optional finer rectangle starts and ends on coarse grid lines.
export function groundDetailAt(detail, x, y) {
  return (
    detail &&
    x >= detail.x0 &&
    y >= detail.y0 &&
    x < detail.x0 + (detail.columns - 1) * detail.s &&
    y < detail.y0 + (detail.rows - 1) * detail.s
  );
}

function groundQuad(grid, columns, x, y, i, j) {
  const k = j * columns + i;
  const { s, z } = grid;
  const h = [z[k], z[k + 1], z[k + columns], z[k + columns + 1]];
  if (h.includes(GROUND_MISSING)) return null;
  return [
    [x, y, h[0] / 10],
    [x + s, y, h[1] / 10],
    [x, y + s, h[2] / 10],
    [x + s, y + s, h[3] / 10],
  ];
}

export function* groundQuads(grid, origin, i, j) {
  const x = grid.x0 + i * grid.s;
  const y = grid.y0 + j * grid.s;
  const { detail } = grid;
  if (!groundDetailAt(detail, x, y)) {
    const quad = groundQuad(grid, grid.n, origin[0] + x, origin[1] + y, i, j);
    if (quad) yield quad;
    return;
  }
  const i0 = (x - detail.x0) / detail.s;
  const j0 = (y - detail.y0) / detail.s;
  const subdivisions = grid.s / detail.s;
  for (let dj = 0; dj < subdivisions; dj++)
    for (let di = 0; di < subdivisions; di++) {
      const quad = groundQuad(
        detail,
        detail.columns,
        origin[0] + x + di * detail.s,
        origin[1] + y + dj * detail.s,
        i0 + di,
        j0 + dj,
      );
      if (quad) yield quad;
    }
}

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
  for (let j = 0; j < g.n - 1; j++)
    for (let i = 0; i < g.n - 1; i++) {
      for (const v of groundQuads(g, o, i, j))
        for (const ids of GROUND_TRIANGLES) {
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
