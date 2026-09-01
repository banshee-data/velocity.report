import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  buildChevrons,
  buildS2HilbertModel,
  classifySegmentsBySelection,
  parseCellSelection,
  renderHilbertSvg,
  seedCellSelection,
  getHilbertTraversal,
  unknownSelectionTokens,
} from "../index.mjs";

const LAND_MASK_URL = new URL("../land/sf-shoreline-and-islands.geojson", import.meta.url);
const loadLandMask = async () => JSON.parse(await readFile(LAND_MASK_URL, "utf8"));

function selectionFor(tokens, enabledTokens) {
  return tokens.map((token) => ({ token, enabled: enabledTokens.includes(token) }));
}

test("every straight segment carries exactly one chevron", () => {
  for (const [targetLevel, segments] of [
    [12, 15],
    [13, 63],
  ]) {
    const model = buildS2HilbertModel({ parent: "808581", targetLevel });
    assert.equal(buildChevrons(model.path).length, segments);
    const svg = renderHilbertSvg(model);
    assert.equal(
      (svg.match(/<polyline class="hilbert-chevron/g) ?? []).length,
      segments,
      `level ${targetLevel}`,
    );
  }
});

test("a chevron points the way the traversal travels", () => {
  const chevrons = buildChevrons([
    { x: 0, y: 0 },
    { x: 100, y: 0 },
  ]);
  assert.equal(chevrons.length, 1);
  const [back, tip, otherBack] = chevrons[0].points;
  // The tip leads; both wings trail behind it along the direction of travel.
  assert.ok(tip.x > back.x && tip.x > otherBack.x);
  assert.ok(Math.abs(back.y + otherBack.y) < 1e-9, "wings straddle the line");
});

test("degenerate segments produce no chevron rather than a NaN one", () => {
  assert.equal(buildChevrons([{ x: 5, y: 5 }, { x: 5, y: 5 }]).length, 0);
});

test("endpoint labels sit clear of the path and can be switched off", () => {
  const model = buildS2HilbertModel({ parent: "808581", targetLevel: 12 });
  const withLabels = renderHilbertSvg(model);
  assert.match(withLabels, /class="endpoint-label endpoint-label-start"[^>]*>START</);
  assert.match(withLabels, /class="endpoint-label endpoint-label-end"[^>]*>END</);
  const without = renderHilbertSvg(model, { showEndpointLabels: false });
  assert.doesNotMatch(without, />START</);
});

test("a selection lights a segment only when both of its cells are enabled", () => {
  const path = [
    { token: "a" },
    { token: "b" },
    { token: "c" },
    { token: "d" },
  ];
  const selection = parseCellSelection(selectionFor(["a", "b", "c", "d"], ["a", "b", "d"]));
  assert.deepEqual(classifySegmentsBySelection(path, selection), [
    "land", // a -> b, both enabled
    "water", // b -> c, c disabled
    "water", // c -> d, c disabled
  ]);
});

test("selection files parse as a plain list or a wrapped object", () => {
  const list = selectionFor(["a", "b"], ["a"]);
  assert.deepEqual([...parseCellSelection(list)], [
    ["a", true],
    ["b", false],
  ]);
  assert.deepEqual([...parseCellSelection({ cells: list })], [
    ["a", true],
    ["b", false],
  ]);
});

test("malformed selections are rejected rather than silently ignored", () => {
  assert.throws(() => parseCellSelection("nope"), /list of \{token, enabled\}/);
  assert.throws(() => parseCellSelection([{ token: "a" }]), /explicit boolean/);
  assert.throws(() => parseCellSelection([{ enabled: true }]), /canonical token/);
  assert.throws(
    () => parseCellSelection([{ token: "a", enabled: true }, { token: "a", enabled: false }]),
    /more than once/,
  );
});

test("a selection naming foreign cells is refused", () => {
  const traversal = getHilbertTraversal("808581", 13);
  const tokens = traversal.cells.map((cell) => cell.token);
  const selection = parseCellSelection(selectionFor([...tokens, "808587"], []));
  assert.deepEqual(unknownSelectionTokens(traversal.cells, selection), ["808587"]);
  assert.throws(
    () =>
      buildS2HilbertModel({
        parent: "808581",
        targetLevel: 13,
        cellSelection: selectionFor([...tokens, "808587"], []),
      }),
    /not descendants of 808581/,
  );
});

test("the coastline seeds a selection that the model then reproduces", async () => {
  const landMask = await loadLandMask();
  const traversal = getHilbertTraversal("808581", 13);
  const seeded = seedCellSelection(traversal, landMask);

  assert.equal(seeded.length, 64);
  assert.ok(seeded.some((cell) => cell.enabled), "some cells are land");
  assert.ok(seeded.some((cell) => !cell.enabled), "some cells are water");
  for (const cell of seeded) {
    assert.equal(typeof cell.enabled, "boolean");
    assert.match(cell.token, /^[0-9a-f]+$/);
  }

  const model = buildS2HilbertModel({
    parent: "808581",
    targetLevel: 13,
    cellSelection: seeded,
  });
  assert.equal(model.classificationSource, "selection");
  assert.equal(model.landMaskApplied, false);
  assert.equal(model.segmentClassifications.length, 63);
});

test("opting a cell in relights the segments that reach it", () => {
  const traversal = getHilbertTraversal("808581", 13);
  const tokens = traversal.cells.map((cell) => cell.token);
  const before = buildS2HilbertModel({
    parent: "808581",
    targetLevel: 13,
    cellSelection: selectionFor(tokens, [tokens[0], tokens[1]]),
  });
  const after = buildS2HilbertModel({
    parent: "808581",
    targetLevel: 13,
    cellSelection: selectionFor(tokens, [tokens[0], tokens[1], tokens[2]]),
  });
  assert.equal(before.classifiedSegments.land.length, 1);
  assert.equal(after.classifiedSegments.land.length, 2);
});

test("a selection takes precedence over a land mask", async () => {
  const traversal = getHilbertTraversal("808581", 13);
  const tokens = traversal.cells.map((cell) => cell.token);
  const model = buildS2HilbertModel({
    parent: "808581",
    targetLevel: 13,
    landMask: await loadLandMask(),
    cellSelection: selectionFor(tokens, []),
  });
  assert.equal(model.classificationSource, "selection");
  assert.equal(model.classifiedSegments.land.length, 0);
});

test("the SVG declares which classification produced it", async () => {
  const plain = renderHilbertSvg(buildS2HilbertModel({ parent: "808581", targetLevel: 12 }));
  assert.match(plain, /data-classification-source="none"/);
  const coastline = renderHilbertSvg(
    buildS2HilbertModel({
      parent: "808581",
      targetLevel: 13,
      landMask: await loadLandMask(),
    }),
  );
  assert.match(coastline, /data-classification-source="coastline"/);
});
