import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (file) =>
  JSON.parse(readFileSync(new URL(`../../sf/${file}`, import.meta.url)));

test("city coverage has actual L13 corners with identical shared edges", () => {
  const manifest = read("tiles/manifest.json");
  const { cells: coverage } = read("coverage.json");
  assert.deepEqual(
    Object.keys(coverage).sort(),
    manifest.tiles.map(({ t }) => t).sort(),
  );
  for (const { t, x, y } of manifest.tiles) {
    const ring = coverage[t];
    assert.equal(ring.length, 4);
    // The baked origin lies inside the projected cell. Reject coordinate-frame
    // errors, corner-order errors and rotated cells that fail to enclose it.
    for (let i = 0; i < ring.length; i++) {
      const [ax, ay] = ring[i];
      const [bx, by] = ring[(i + 1) % ring.length];
      assert.ok((bx - ax) * (y - ay) - (by - ay) * (x - ax) > 0, t);
    }
  }
  // Reference vertices projected from S2 tokens 8085800c and 80858014.
  assert.deepEqual(coverage["8085800c"], [
    [5393.525, 3789.929],
    [5394.331, 2732.137],
    [6392.186, 3119.062],
    [6391.252, 4176.888],
  ]);
  assert.deepEqual(coverage["80858014"][1], coverage["8085800c"][0]);
  assert.deepEqual(coverage["80858014"][2], coverage["8085800c"][3]);
});
