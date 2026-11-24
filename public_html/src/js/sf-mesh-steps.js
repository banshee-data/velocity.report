import {
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
  const parts = [],
    uncut = [];
  let length = 0;
  for (const { index, transform, ids } of rows) {
    const mesh = carveTreeCuts(
      model,
      matrix.fromArray(transform),
      new Set(ids.map((id) => cuts[id])),
    );
    if (!mesh) uncut.push(index);
    else {
      parts.push(mesh.attributes);
      length += mesh.attributes.position.array.length;
    }
    yield;
  }
  const position = new Float32Array(length),
    normal = new Float32Array(length);
  let offset = 0;
  for (const part of parts) {
    position.set(part.position.array, offset);
    normal.set(part.normal.array, offset);
    offset += part.position.array.length;
  }
  return { position, normal, uncut };
}

export function meshSteps(data) {
  return data.type === "tile" ? tileGeometry(data) : clipTreeBatch(data);
}
