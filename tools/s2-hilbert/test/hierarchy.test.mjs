import assert from "node:assert/strict";
import test from "node:test";

import {
  buildOrderedChildrenDocument,
  buildS2HilbertModel,
  displayTokenToCanonical,
  formatDisplayToken,
  getDescendantIds,
  parseCanonicalToken,
  renderHilbertSvg,
  s2,
} from "../index.mjs";

const PARENT_TOKEN = "808581";

test("L10 descendant counts follow the S2 quadtree", () => {
  assert.equal(getDescendantIds(PARENT_TOKEN, 12).length, 16);
  assert.equal(getDescendantIds(PARENT_TOKEN, 13).length, 64);
});

test("every L13 CellID has the requested real S2 parent", () => {
  const parentId = parseCanonicalToken(PARENT_TOKEN);
  for (const childId of getDescendantIds(PARENT_TOKEN, 13)) {
    assert.equal(s2.cellid.parent(childId, 10), parentId);
  }
});

test("descendants are unique and monotonically follow the Hilbert traversal", () => {
  const descendants = getDescendantIds(PARENT_TOKEN, 13);
  assert.equal(new Set(descendants.map(String)).size, 64);
  for (let index = 1; index < descendants.length; index += 1) {
    assert.ok(descendants[index - 1] < descendants[index]);
    assert.equal(descendants[index], s2.cellid.next(descendants[index - 1]));
  }
});

test("every emitted canonical token round-trips through s2js", () => {
  for (const childId of getDescendantIds(PARENT_TOKEN, 13)) {
    const token = s2.cellid.toToken(childId);
    assert.equal(s2.cellid.toToken(s2.cellid.fromToken(token)), token);
    assert.equal(parseCanonicalToken(token), childId);
  }
});

test("display formatting is explicit and never reaches the S2 parser", () => {
  assert.equal(formatDisplayToken("808581"), "80858-1");
  assert.equal(formatDisplayToken("80858004"), "80858-004");
  assert.equal(displayTokenToCanonical("80858-1"), "808581");
  assert.throws(() => parseCanonicalToken("80858-1"), /display token/i);
});

test("models contain actual centres, vertices, and a continuous 64-point path", () => {
  const model = buildS2HilbertModel({
    parent: PARENT_TOKEN,
    targetLevel: 13,
    width: 640,
    height: 480,
    padding: 24,
  });
  assert.equal(model.parent.level, 10);
  assert.equal(model.cells.length, 64);
  assert.equal(model.path.length, 64);
  assert.equal(model.selectionApplied, false);
  for (const cell of model.cells) {
    assert.equal(cell.vertices.length, 4);
    assert.ok(Number.isFinite(cell.centre.lat));
    assert.ok(Number.isFinite(cell.centre.lng));
    assert.ok(Number.isFinite(cell.centre.x));
    assert.ok(Number.isFinite(cell.centre.y));
  }
});

test("identical inputs produce byte-identical models, JSON, and SVG", () => {
  const options = {
    parent: PARENT_TOKEN,
    targetLevel: 13,
    width: 640,
    height: 640,
    padding: 32,
  };
  const first = buildS2HilbertModel(options);
  const second = buildS2HilbertModel(options);
  assert.deepEqual(first, second);
  assert.deepEqual(buildOrderedChildrenDocument(first), buildOrderedChildrenDocument(second));
  assert.equal(renderHilbertSvg(first), renderHilbertSvg(second));
});

test("the four authoritative orientation bit states use actual S2 parents", () => {
  const examples = new Map([
    ["808587", 0],
    ["808f7f", 1],
    ["808581", 2],
    ["808585", 3],
  ]);
  for (const [token, expected] of examples) {
    const model = buildS2HilbertModel({ parent: token, targetLevel: 12 });
    assert.equal(model.parent.orientation, expected);
    assert.equal(model.cells.length, 16);
  }
});

test("the four infographic parents cover three of the four orientation states", () => {
  // Verified against s2js Cell.orientation, cellid.faceIJOrientation, and the
  // geometry of the 16 L12 centres on a 4x4 grid. 808581 and 808f7d are both
  // orientation 2 and traverse identically, so the reference set cannot
  // demonstrate all four states and the legend borrows 808585 for the fourth.
  const parents = ["808581", "808587", "808f7d", "808f7f"];
  const orientations = parents.map(
    (token) => s2.Cell.fromCellID(parseCanonicalToken(token)).orientation,
  );
  assert.deepEqual(orientations, [2, 0, 2, 1]);
  assert.equal(new Set(orientations).size, 3);
  assert.ok(!orientations.includes(3), "swap+invert is absent from the reference set");

  // Same orientation must mean the same traversal, step for step. The grid has
  // to be read in S2's own face coordinates: cells are sheared with respect to
  // latitude and longitude, so lat/lng does not give a 4x4 lattice.
  const signature = (token) => {
    const centres = getDescendantIds(token, 12).map((cellId) => {
      const { si, ti } = s2.cellid.faceSiTi(cellId);
      return [Number(si), Number(ti)];
    });
    const columns = [...new Set(centres.map((c) => c[0]))].sort((a, b) => a - b);
    const rows = [...new Set(centres.map((c) => c[1]))].sort((a, b) => a - b);
    assert.equal(columns.length, 4, `${token} spans a 4-column grid`);
    assert.equal(rows.length, 4, `${token} spans a 4-row grid`);
    return centres
      .map(([si, ti]) => `${columns.indexOf(si)}${rows.indexOf(ti)}`)
      .join(" ");
  };
  assert.equal(signature("808581"), signature("808f7d"));
  assert.notEqual(signature("808581"), signature("808587"));
  assert.notEqual(signature("808587"), signature("808f7f"));
});

test("the ordered child document uses canonical tokens and geographic centres", () => {
  const document = buildOrderedChildrenDocument(
    buildS2HilbertModel({ parent: PARENT_TOKEN, targetLevel: 13 }),
  );
  assert.deepEqual(
    {
      parent: document.parent,
      displayParent: document.displayParent,
      parentLevel: document.parentLevel,
      childLevel: document.childLevel,
      count: document.children.length,
    },
    {
      parent: "808581",
      displayParent: "80858-1",
      parentLevel: 10,
      childLevel: 13,
      count: 64,
    },
  );
  assert.deepEqual(
    document.children.map((child) => child.index),
    Array.from({ length: 64 }, (_, index) => index),
  );
});
