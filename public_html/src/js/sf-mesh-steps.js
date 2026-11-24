import {
  BufferAttribute,
  BufferGeometry,
  CylinderGeometry,
  IcosahedronGeometry,
  Matrix4,
  Plane,
} from "../vendor/three/three.core.js";
import { carveTreeCuts } from "./sf-tree-clearance.js";
import { tileGeometry } from "./sf-tile-geometry.js";

export function treeModel(trunk) {
  return trunk
    ? new CylinderGeometry(1, 1, 1, 6)
    : new IcosahedronGeometry(1, 1);
}

function* clipTreeBatch({ trunk, rows, planes }) {
  const model = treeModel(trunk),
    matrix = new Matrix4();
  const cuts = planes.map((faces) => ({
    planes: faces.map(([x, y, z, w]) => new Plane().setComponents(x, y, z, w)),
  }));
  const positions = [],
    normals = [],
    indices = [],
    uncut = [];
  for (const { index, transform, ids } of rows) {
    const mesh = carveTreeCuts(
      model,
      matrix.fromArray(transform),
      new Set(ids.map((id) => cuts[id])),
    );
    if (!mesh) uncut.push(index);
    else {
      // A flat cut face is triangulated as a fan. Reuse shared vertices
      // inside that part, retaining a separate vertex at each hard edge.
      const vertices = new Map(),
        p = mesh.attributes.position.array,
        n = mesh.attributes.normal.array;
      const bits = new Uint32Array(p.buffer, p.byteOffset, p.length);
      for (let i = 0; i < p.length; i += 3) {
        const nx = Math.round(n[i] * 127),
          ny = Math.round(n[i + 1] * 127),
          nz = Math.round(n[i + 2] * 127);
        const packed = ((nx & 255) << 16) | ((ny & 255) << 8) | (nz & 255);
        const key =
          bits[i] + ":" + bits[i + 1] + ":" + bits[i + 2] + ":" + packed;
        let slot = vertices.get(key);
        if (slot === undefined) {
          slot = positions.length / 3;
          vertices.set(key, slot);
          positions.push(p[i], p[i + 1], p[i + 2]);
          normals.push(nx, ny, nz);
        }
        indices.push(slot);
      }
    }
    yield;
  }
  const position = new Float32Array(positions);
  const geometry = new BufferGeometry();
  geometry.setAttribute("position", new BufferAttribute(position, 3));
  geometry.computeBoundingSphere();
  const sphere = [
    ...geometry.boundingSphere.center.toArray(),
    geometry.boundingSphere.radius,
  ];
  return {
    position,
    normal: new Int8Array(normals),
    index:
      positions.length / 3 <= 65536
        ? new Uint16Array(indices)
        : new Uint32Array(indices),
    sphere,
    uncut,
  };
}

export function meshSteps(data) {
  return data.type === "tile" ? tileGeometry(data) : clipTreeBatch(data);
}
