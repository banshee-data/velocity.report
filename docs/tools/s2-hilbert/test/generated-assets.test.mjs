import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { buildOrderedChildrenDocument, buildS2HilbertModel } from "../index.mjs";

const GENERATED = new URL("../generated/", import.meta.url);
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
});

test("each coarse SVG contains 16 actual descendant cells", async () => {
  for (const filename of COARSE_FILES) {
    const svg = await readFile(new URL(filename, GENERATED), "utf8");
    assert.equal((svg.match(/class="s2-cell"/g) ?? []).length, 16, filename);
    assert.match(svg, /data-target-level="12"/, filename);
  }
});

test("the legend contains all four verified S2 orientation states", async () => {
  const svg = await readFile(new URL("s2-hilbert-orientations.svg", GENERATED), "utf8");
  for (const orientation of [0, 1, 2, 3]) {
    assert.match(svg, new RegExp(`data-orientation="${orientation}"`));
  }
});
