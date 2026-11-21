import { clipHalfPlane } from "./sf-drive-surface.js";

// Stylised guidance from the existing two-way centrelines, not observed paint
// or lane rules. Carry the 3 m dash / 3 m gap phase across polyline vertices.
export function roadDashPolygons(shapes) {
  const marks = [];
  for (const shape of shapes) {
    if (!shape.dashed) continue;
    let distance = 0;
    for (let i = 1; i < shape.centre.length; i++) {
      const a = shape.centre[i - 1],
        b = shape.centre[i];
      const length = Math.hypot(b[0] - a[0], b[1] - a[1]);
      if (length < 1e-6) continue;
      const dx = (b[0] - a[0]) / length,
        dy = (b[1] - a[1]) / length;
      const nx = -dy * 0.15,
        ny = dx * 0.15;
      for (
        let dash = Math.floor(distance / 6);
        dash * 6 < distance + length;
        dash++
      ) {
        const start = Math.max(0, dash * 6 - distance);
        const end = Math.min(length, dash * 6 + 3 - distance);
        if (end <= start) continue;
        const x0 = a[0] + dx * start,
          y0 = a[1] + dy * start;
        const x1 = a[0] + dx * end,
          y1 = a[1] + dy * end;
        marks.push([
          [x0 - nx, y0 - ny],
          [x1 - nx, y1 - ny],
          [x1 + nx, y1 + ny],
          [x0 + nx, y0 + ny],
        ]);
      }
      distance += length;
    }
  }
  return marks;
}

const planesFor = (ring) =>
  ring.map((a, i) => {
    const b = ring[(i + 1) % ring.length];
    return [
      b[1] - a[1],
      a[0] - b[0],
      (b[1] - a[1]) * a[0] + (a[0] - b[0]) * a[1],
    ];
  });

// Project convex, anticlockwise marking polygons by clipping the original
// terrain triangles. Interpolating their vertices preserves the exact plane:
// no independently draped ribbon cutting into hills, gaps or neighbouring tiles.
export function roadMarkingTriangles(tile, region, marks) {
  const { g, o } = tile;
  const x0 = o[0] + g.x0,
    y0 = o[1] + g.y0;
  const regionPlanes = planesFor(region),
    positions = [];
  const vertex = (i, j) => [x0 + i * g.s, y0 + j * g.s, g.z[j * g.n + i] / 10];
  const rx0 = Math.min(...region.map((p) => p[0])),
    rx1 = Math.max(...region.map((p) => p[0]));
  const ry0 = Math.min(...region.map((p) => p[1])),
    ry1 = Math.max(...region.map((p) => p[1]));
  for (const mark of marks) {
    const minX = Math.max(rx0, Math.min(...mark.map((p) => p[0])));
    const maxX = Math.min(rx1, Math.max(...mark.map((p) => p[0])));
    const minY = Math.max(ry0, Math.min(...mark.map((p) => p[1])));
    const maxY = Math.min(ry1, Math.max(...mark.map((p) => p[1])));
    if (minX >= maxX || minY >= maxY) continue;
    const i0 = Math.max(0, Math.floor((minX - x0) / g.s));
    const i1 = Math.min(g.n - 2, Math.floor((maxX - x0) / g.s));
    const j0 = Math.max(0, Math.floor((minY - y0) / g.s));
    const j1 = Math.min(g.n - 2, Math.floor((maxY - y0) / g.s));
    const planes = [...planesFor(mark), ...regionPlanes];
    for (let j = j0; j <= j1; j++)
      for (let i = i0; i <= i1; i++) {
        const k = j * g.n + i;
        if ([k, k + 1, k + g.n, k + g.n + 1].some((n) => g.z[n] === -32768))
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
          let poly = ids.map((n) => v[n]);
          for (const [nx, ny, d] of planes) {
            poly = clipHalfPlane(poly, nx, ny, d);
            if (!poly.length) break;
          }
          for (let n = 1; n + 1 < poly.length; n++) {
            for (const p of [poly[0], poly[n], poly[n + 1]])
              positions.push(p[0], p[2], -p[1]);
          }
        }
      }
  }
  return positions;
}
