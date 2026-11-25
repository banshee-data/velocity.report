// Subtract the vertical space above each deck triangle from a convex tree
// mesh. Work in Three.js world coordinates (east, up, -north). Each split
// closes both solids with a cap, including vertical faces along a road edge.
import {
  Box3,
  BufferGeometry,
  Float32BufferAttribute,
  Plane,
  Vector3,
} from "../vendor/three/three.core.js";
const eps = 1e-7;
const up = new Vector3(0, 1, 0);
const cell = 24;
const key = (x, z) => x + "," + z;
function cells(bounds, visit) {
  for (
    let x = Math.floor(bounds.min.x / cell);
    x <= Math.floor(bounds.max.x / cell);
    x++
  )
    for (
      let z = Math.floor(bounds.min.z / cell);
      z <= Math.floor(bounds.max.z / cell);
      z++
    )
      visit(key(x, z));
}

export function deckClearanceIndex() {
  const grid = new Map();
  return {
    add(geometry, kind = "bridge_deck") {
      if (kind !== "bridge_deck" && kind !== "ramp") return () => {};
      const cuts = [],
        positions = geometry.attributes.position,
        indices = geometry.index;
      const vertex = (i) =>
        new Vector3().fromBufferAttribute(
          positions,
          indices ? indices.getX(i) : i,
        );
      for (let i = 0; i < (indices?.count ?? positions.count); i += 3) {
        const points = [vertex(i), vertex(i + 1), vertex(i + 2)];
        const floor = new Plane().setFromCoplanarPoints(...points);
        if (!Number.isFinite(floor.normal.y) || Math.abs(floor.normal.y) < 0.01)
          continue;
        if (floor.normal.y < 0) floor.negate();
        // Lower the cut slightly to avoid coincident foliage and road faces.
        floor.constant += 0.12 * floor.normal.y;
        const centre = points
          .reduce((sum, p) => sum.add(p), new Vector3())
          .divideScalar(3);
        const planes = points.map((a, j) => {
          const b = points[(j + 1) % 3];
          const plane = new Plane().setFromCoplanarPoints(
            a,
            b,
            b.clone().add(up),
          );
          return plane.distanceToPoint(centre) < 0 ? plane.negate() : plane;
        });
        const bounds = new Box3().setFromPoints(points);
        const cut = { bounds, planes: [floor, ...planes] };
        cuts.push(cut);
        cells(bounds, (k) => {
          if (!grid.has(k)) grid.set(k, new Set());
          grid.get(k).add(cut);
        });
      }
      return () => {
        for (const cut of cuts)
          cells(cut.bounds, (k) => {
            const group = grid.get(k);
            group.delete(cut);
            if (!group.size) grid.delete(k);
          });
      };
    },
    ceiling(x, north, base) {
      const point = new Vector3(x, 0, -north);
      let height = Infinity;
      for (const cut of grid.get(
        key(Math.floor(x / cell), Math.floor(-north / cell)),
      ) || []) {
        const [floor, ...sides] = cut.planes;
        if (sides.some((p) => p.distanceToPoint(point) < -eps)) continue;
        const z = -floor.distanceToPoint(point) / floor.normal.y;
        if (z >= base - 0.37) height = Math.min(height, z);
      }
      return height;
    },
    query(bounds, base) {
      const result = new Set();
      cells(bounds, (k) => {
        for (const cut of grid.get(k) || []) {
          const b = cut.bounds;
          if (
            b.max.y + 0.25 < base ||
            b.min.y - 0.12 > bounds.max.y ||
            b.max.x < bounds.min.x ||
            b.min.x > bounds.max.x ||
            b.max.z < bounds.min.z ||
            b.min.z > bounds.max.z
          )
            continue;
          // A diagonal road triangle can have a much larger bounding box
          // than its footprint. Reject boxes fully outside any prism face.
          if (
            cut.planes.some(
              ({ normal: n, constant }) =>
                n.x * (n.x < 0 ? bounds.min.x : bounds.max.x) +
                  n.y * (n.y < 0 ? bounds.min.y : bounds.max.y) +
                  n.z * (n.z < 0 ? bounds.min.z : bounds.max.z) +
                  constant <
                -eps,
            )
          )
            continue;
          result.add(cut);
        }
      });
      return result;
    },
  };
}

function split(faces, plane) {
  let above = false,
    below = false;
  scan: for (const face of faces) {
    for (const point of face) {
      const d = plane.distanceToPoint(point);
      above ||= d > eps;
      below ||= d < -eps;
      if (above && below) break scan;
    }
  }
  if (!below) return [faces, []];
  if (!above) return [[], faces];
  const positive = [],
    negative = [],
    rim = [];
  const onRim = (p) => {
    if (!rim.some((q) => q.distanceToSquared(p) < eps * eps)) rim.push(p);
  };
  for (const face of faces) {
    const pos = [],
      neg = [];
    for (let i = 0; i < face.length; i++) {
      const a = face[i],
        b = face[(i + 1) % face.length],
        da = plane.distanceToPoint(a),
        db = plane.distanceToPoint(b);
      if (da >= -eps) pos.push(a);
      if (da <= eps) neg.push(a);
      if (Math.abs(da) <= eps) onRim(a);
      if ((da > eps && db < -eps) || (da < -eps && db > eps)) {
        const p = a.clone().lerp(b, da / (da - db));
        pos.push(p);
        neg.push(p);
        onRim(p);
      }
    }
    if (pos.length >= 3) positive.push(pos);
    if (neg.length >= 3) negative.push(neg);
  }
  if (rim.length >= 3) {
    const centre = rim
      .reduce((s, p) => s.add(p), new Vector3())
      .divideScalar(rim.length);
    const axis = rim[0].clone().sub(centre).normalize(),
      other = plane.normal.clone().cross(axis);
    const angle = (p) =>
      Math.atan2(
        p.clone().sub(centre).dot(other),
        p.clone().sub(centre).dot(axis),
      );
    rim.sort((a, b) => angle(a) - angle(b));
    negative.push(rim);
    positive.push([...rim].reverse());
  }
  return [positive, negative];
}

export function treeCuts(geometry, matrix, base, index) {
  if (!geometry.boundingBox) geometry.computeBoundingBox();
  return index.query(geometry.boundingBox.clone().applyMatrix4(matrix), base);
}

export function carveTree(geometry, matrix, base, index) {
  return carveTreeCuts(
    geometry,
    matrix,
    treeCuts(geometry, matrix, base, index),
  );
}

export function carveTreeCuts(geometry, matrix, cuts) {
  if (!cuts.size) return null;
  const positions = geometry.attributes.position,
    indices = geometry.index;
  const point = (i) =>
    new Vector3()
      .fromBufferAttribute(positions, indices ? indices.getX(i) : i)
      .applyMatrix4(matrix);
  const faces = [];
  for (let i = 0; i < (indices?.count ?? positions.count); i += 3)
    faces.push([point(i), point(i + 1), point(i + 2)]);
  let solids = [faces],
    changed = false;
  for (const cut of cuts) {
    const survivors = [];
    for (const solid of solids) {
      let inside = solid;
      const outside = [];
      for (const plane of cut.planes) {
        const [yes, no] = split(inside, plane);
        if (no.length) outside.push(no);
        inside = yes;
        if (!inside.length) break;
      }
      if (inside.length) {
        survivors.push(...outside);
        changed = true;
      } else survivors.push(solid);
    }
    solids = survivors;
  }
  if (!changed) return null;
  const coords = [];
  for (const solid of solids)
    for (const f of solid)
      for (let i = 2; i < f.length; i++)
        for (const p of [f[0], f[i - 1], f[i]]) coords.push(p.x, p.y, p.z);
  const mesh = new BufferGeometry();
  mesh.setAttribute("position", new Float32BufferAttribute(coords, 3));
  mesh.computeVertexNormals();
  return mesh;
}
