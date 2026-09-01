import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  buildOrderedChildrenDocument,
  buildS2HilbertModel,
  getHilbertTraversal,
  parseCellSelection,
  renderHilbertSvg,
  unknownSelectionTokens,
} from "../index.mjs";
import { DETAILED_RENDER_OPTIONS } from "../generate-assets.mjs";

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

test("every drawn element carries its own fill and stroke attributes", async () => {
  // Illustrator, Inkscape and Figma routinely drop an embedded <style> block.
  // A path that relies on CSS alone then falls back to filled-and-unstroked,
  // which turns each Hilbert curve into a solid shape. Presentation attributes
  // are what keep these files usable in a design tool.
  const files = [
    "80858-1-l13-detailed.svg",
    "80858-1-l12-coarse.svg",
    "80858-7-l12-coarse.svg",
    "808f7-d-l12-coarse.svg",
    "808f7-f-l12-coarse.svg",
    "s2-hilbert-orientations.svg",
    "s2-l10-quad-composite.svg",
  ];

  for (const filename of files) {
    const svg = await readFile(new URL(filename, GENERATED), "utf8");
    const body = svg.slice(svg.indexOf("</defs>"));
    const drawn = [...body.matchAll(/<(path|polygon|polyline|circle|rect|text)\b[^>]*>/g)];
    assert.ok(drawn.length > 0, filename);
    for (const [element] of drawn) {
      assert.match(
        element,
        /\bfill="|\bstroke="/,
        `${filename}: element renders as filled-black without CSS: ${element.slice(0, 80)}`,
      );
    }
    // Every stroked line must say so outright rather than inherit a fill.
    for (const [element] of body.matchAll(/<(?:path|polyline)\b[^>]*>/g)) {
      assert.match(element, /fill="none"/, `${filename}: ${element.slice(0, 80)}`);
    }
  }
});

test("the committed detailed SVG matches the committed selection", async () => {
  // The structural checks above pass even when the asset is stale, because the
  // cell count and group names do not change when cells are opted in. This
  // compares the file against a fresh render, so editing the selection without
  // regenerating fails here rather than shipping a picture of the old choice.
  const svg = await readFile(new URL("80858-1-l13-detailed.svg", GENERATED), "utf8");
  const cellSelection = JSON.parse(
    await readFile(new URL("80858-1-l13-cells.json", SELECTION), "utf8"),
  );
  const model = buildS2HilbertModel({
    parent: "808581",
    targetLevel: 13,
    width: 1200,
    height: 1200,
    padding: 52,
    cellSelection,
  });
  assert.equal(svg, renderHilbertSvg(model, DETAILED_RENDER_OPTIONS));
});
