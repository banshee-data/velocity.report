// Each building's roof rings cover the top of one inferred or mapped footprint.
// A shared edge needs only the exposed step between adjacent roof heights.
export function buildingWalls(faces, base, emit) {
  const lines = new Map();
  const gcd = (a, b) => (b ? gcd(b, a % b) : a);
  faces.forEach((rings, face) =>
    rings.forEach((ring, index) => {
      const area = ring.reduce((sum, a, i) => {
        const b = ring[(i + 1) % ring.length];
        return sum + a[0] * b[1] - a[1] * b[0];
      }, 0);
      for (let i = 0; i < ring.length; i++) {
        const a = ring[i];
        const b = ring[(i + 1) % ring.length];
        const x = Math.round(a[0] * 10),
          y = Math.round(a[1] * 10);
        const dx = Math.round(b[0] * 10) - x,
          dy = Math.round(b[1] * 10) - y;
        const scale = gcd(Math.abs(dx), Math.abs(dy));
        if (!scale) continue;
        const direction = Math.sign(dx || dy);
        const u = (dx / scale) * direction,
          v = (dy / scale) * direction;
        const key = `${u},${v},${u * y - v * x}`;
        const start = u * x + v * y,
          end = start + u * dx + v * dy;
        const low = Math.min(start, end),
          high = Math.max(start, end);
        const side = direction * Math.sign(area) * (index === 0 ? 1 : -1);
        if (!lines.has(key)) lines.set(key, []);
        lines.get(key).push({ a, b, face, start, end, low, high, side });
      }
    }),
  );

  const at = (e, t, axis) =>
    e.a[axis] + (e.b[axis] - e.a[axis]) * ((t - e.start) / (e.end - e.start));
  for (const edges of lines.values()) {
    for (const edge of edges) {
      const neighbours = edges.filter(
        (other) =>
          other.face !== edge.face &&
          other.side === -edge.side &&
          other.low < edge.high &&
          other.high > edge.low,
      );
      const cuts = [
        ...new Set([
          edge.low,
          edge.high,
          ...neighbours.flatMap((other) => [
            Math.max(edge.low, other.low),
            Math.min(edge.high, other.high),
          ]),
        ]),
      ].sort((a, b) => a - b);
      for (let i = 1; i < cuts.length; i++) {
        let a = cuts[i - 1],
          b = cuts[i];
        const mid = (a + b) / 2;
        const shared = neighbours.filter(
          (other) => other.low < mid && other.high > mid,
        );
        const floor = (t) =>
          Math.max(base, ...shared.map((other) => at(other, t, 2)));
        const da = at(edge, a, 2) - floor(a),
          db = at(edge, b, 2) - floor(b);
        if (da <= 0 && db <= 0) continue;
        if (da < 0 || db < 0) {
          const crossing = a + ((b - a) * da) / (da - db);
          if (da < 0) a = crossing;
          else b = crossing;
        }
        if (edge.start > edge.end) [a, b] = [b, a];
        emit(
          [at(edge, a, 0), at(edge, a, 1), at(edge, a, 2)],
          [at(edge, b, 0), at(edge, b, 1), at(edge, b, 2)],
          floor(a),
          floor(b),
        );
      }
    }
  }
}
// An unassigned plane in the base bake encodes its unknown altitude at this sentinel.
export function validBuildingFace(face) {
  return (
    face.length > 0 &&
    face.every(
      (ring) =>
        ring.length >= 9 &&
        ring.every(
          (value, i) =>
            Number.isFinite(value) && (i % 3 !== 2 || value !== -10000000000),
        ),
    )
  );
}
