import test from "node:test";
import assert from "node:assert/strict";
import {
  BoxGeometry,
  BufferGeometry,
  Float32BufferAttribute,
  Matrix4,
} from "../../vendor/three/three.core.js";
import { carveTree, deckClearanceIndex } from "../sf-tree-clearance.js";
test("a road trims only its footprint with a closed cap below the sloping deck", () => {
  const index = deckClearanceIndex(),
    road = new BufferGeometry();
  road.setAttribute(
    "position",
    new Float32BufferAttribute(
      [-5, 1, -5, 0, 1.5, -5, 0, 1.5, 5, -5, 1, -5, 0, 1.5, 5, -5, 1, 5],
      3,
    ),
  );
  const remove = index.add(road),
    model = new BoxGeometry(4, 4, 4),
    matrix = new Matrix4().makeTranslation(0, 2, 0);
  const g = carveTree(model, matrix, 0, index),
    p = g.attributes.position;
  let cap = false,
    side = false,
    uncut = false;
  for (let i = 0; i < p.count; i += 3) {
    const verts = [i, i + 1, i + 2],
      x = verts.reduce((s, j) => s + p.getX(j), 0) / 3,
      y = verts.reduce((s, j) => s + p.getY(j), 0) / 3;
    assert.ok(x >= -1e-5 || y <= 1.38 + x / 10 + 1e-5);
    cap ||= x < 0 && Math.abs(y - (1.38 + x / 10)) < 1e-5;
    side ||= Math.abs(x) < 1e-5 && y > 1.5;
    uncut ||= x > 0 && y > 3;
  }
  assert.ok(cap && side && uncut);
  remove();
  assert.equal(carveTree(model, matrix, 0, index), null);
});

test("an at-grade approach does not carve a crown or shorten its trunk collision", () => {
  const index = deckClearanceIndex();
  const road = new BufferGeometry();
  road.setAttribute(
    "position",
    new Float32BufferAttribute([-5, 0.28, -5, 2, 0.35, -5, 2, 0.45, 5], 3),
  );
  index.add(road, "ground");
  const tree = new BoxGeometry(3, 4, 3);
  const matrix = new Matrix4().makeTranslation(0, 2, 0);
  assert.equal(carveTree(tree, matrix, 0, index), null);
  assert.equal(index.ceiling(0, 0, 0), Infinity);
});
