import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  buildOrderedChildrenDocument,
  buildS2HilbertModel,
  getHilbertTraversal,
  parseCellSelection,
  unknownSelectionTokens,
} from "../index.mjs";

const GENERATED = new URL("../generated/", import.meta.url);
const SELECTION = new URL("../selection/", import.meta.url);
const COARSE_FILES = [
  "80858-1-l12-coarse.svg",
  "80858-7-l12-coarse.svg",
  "808f7-d-l12-coarse.svg",
  "808f7-f-l12-coarse.svg",
];

test("the checked-in ordered child data matches a fresh S2 traversal", async () => {
  const checkedIn = JSON.parse(await readFile(new URL("80858-1-l13.json", GENERATED), "utf8"));
  const regenerated = buildOrderedChildrenDocument(
    buildS2HilbertModel({ parent: "808581", targetLevel: 13 }),
  );
  assert.deepEqual(checkedIn, regenerated);
});

test("the detailed SVG contains 64 cells and semantic land and water paths", async () => {
  const svg = await readFile(new URL("80858-1-l13-detailed.svg", GENERATED), "utf8");
  assert.equal((svg.match(/class="s2-cell"/g) ?? []).length, 64);
  assert.match(svg, /id="hilbert-land"/);
  assert.match(svg, /id="hilbert-water"/);
  assert.match(svg, /data-target-level="13"/);
  assert.match(svg, /data-classification-source="selection"/);
  assert.equal((svg.match(/<polyline class="hilbert-chevron/g) ?? []).length, 63);
  assert.match(svg, />START</);
  assert.match(svg, />END</);
});

test("the committed cell selection covers every L13 child exactly once", async () => {
  const selection = JSON.parse(
    await readFile(new URL("80858-1-l13-cells.json", SELECTION), "utf8"),
  );
  const parsed = parseCellSelection(selection);
  const traversal = getHilbertTraversal("808581", 13);
  assert.equal(parsed.size, 64);
  assert.deepEqual(unknownSelectionTokens(traversal.cells, parsed), []);
  for (const cell of traversal.cells) {
    assert.equal(typeof parsed.get(cell.token), "boolean", cell.token);
  }
});

test("each coarse SVG contains 16 actual descendant cells", async () => {
  for (const filename of COARSE_FILES) {
    const svg = await readFile(new URL(filename, GENERATED), "utf8");
    assert.equal((svg.match(/class="s2-cell"/g) ?? []).length, 16, filename);
    assert.match(svg, /data-target-level="12"/, filename);
    // Item 4: the L10 representations carry direction too, not just the detail.
    assert.equal(
      (svg.match(/<polyline class="hilbert-chevron/g) ?? []).length,
      15,
      filename,
    );
    assert.match(svg, /marker-end="url\(#hilbert-arrow\)"/, filename);
    assert.match(svg, />START</, filename);
    assert.match(svg, />END</, filename);
  }
});

test("the legend contains all four verified S2 orientation states", async () => {
  const svg = await readFile(new URL("s2-hilbert-orientations.svg", GENERATED), "utf8");
  for (const orientation of [0, 1, 2, 3]) {
    assert.match(svg, new RegExp(`data-orientation="${orientation}"`));
  }
  assert.equal((svg.match(/<polyline class="legend-chevron/g) ?? []).length, 15 * 4);
});
