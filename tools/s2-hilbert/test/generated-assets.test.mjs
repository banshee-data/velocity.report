import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
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
