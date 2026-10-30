// Presentation mask in the export's sensor XY frame. It suppresses apparent
// road users inside source building footprints; it does not relabel tracks or
// change the survey's measured statistics. Courtyards remain open.
const cross = (a, b, p) =>
  (b[0] - a[0]) * (p[1] - a[1]) - (b[1] - a[1]) * (p[0] - a[0]);
const on = (a, b, p) =>
  Math.abs(cross(a, b, p)) < 1e-8 &&
  p[0] >= Math.min(a[0], b[0]) - 1e-8 &&
  p[0] <= Math.max(a[0], b[0]) + 1e-8 &&
  p[1] >= Math.min(a[1], b[1]) - 1e-8 &&
  p[1] <= Math.max(a[1], b[1]) + 1e-8;

function inRing(p, ring) {
  let inside = false;
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const a = ring[j],
      b = ring[i];
    if (on(a, b, p)) return true;
    if (
      a[1] > p[1] !== b[1] > p[1] &&
      p[0] < ((b[0] - a[0]) * (p[1] - a[1])) / (b[1] - a[1]) + a[0]
    )
      inside = !inside;
  }
  return inside;
}

function intersects(a, b, c, d) {
  return (
    (cross(a, b, c) * cross(a, b, d) < 0 &&
      cross(c, d, a) * cross(c, d, b) < 0) ||
    on(a, b, c) ||
    on(a, b, d) ||
    on(c, d, a) ||
    on(c, d, b)
  );
}

export function createBuildingMask(collection) {
  const buildings = [];
  for (const f of collection?.features ?? []) {
    if (f.properties?.layer !== "footprints") continue;
    const polygons =
      f.geometry?.type === "Polygon"
        ? [f.geometry.coordinates]
        : f.geometry?.type === "MultiPolygon"
          ? f.geometry.coordinates
          : [];
    for (const rings of polygons) {
      if (!rings[0] || rings[0].length < 3) continue;
      const xs = rings[0].map((p) => p[0]),
        ys = rings[0].map((p) => p[1]);
      buildings.push({
        rings,
        minX: Math.min(...xs),
        maxX: Math.max(...xs),
        minY: Math.min(...ys),
        maxY: Math.max(...ys),
      });
    }
  }
  const contains = (x, y) =>
    buildings.some(
      (b) =>
        x >= b.minX &&
        x <= b.maxX &&
        y >= b.minY &&
        y <= b.maxY &&
        inRing([x, y], b.rings[0]) &&
        !b.rings.slice(1).some((r) => inRing([x, y], r)),
    );
  return {
    contains,
    hides: (t) => contains(t.x, t.y) || contains(t.bx ?? t.x, t.by ?? t.y),
    crosses: (a, b) =>
      buildings.some((building) => {
        if (
          Math.max(a[0], b[0]) < building.minX ||
          Math.min(a[0], b[0]) > building.maxX ||
          Math.max(a[1], b[1]) < building.minY ||
          Math.min(a[1], b[1]) > building.maxY
        )
          return false;
        return building.rings.some((r) =>
          r.some((p, i) => intersects(a, b, p, r[(i + 1) % r.length])),
        );
      }),
  };
}
