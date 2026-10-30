import { test } from "node:test";
import assert from "node:assert/strict";
import { createBuildingMask } from "../scene-buildings.js";
import { buildTrailHistories } from "../scene-player.js";
const ring = (a, b, c, d) => [
  [a, b, 0],
  [c, b, 0],
  [c, d, 0],
  [a, d, 0],
  [a, b, 0],
];
const building = (rings, type = "Polygon") => ({
  properties: { layer: "footprints" },
  geometry: { type, coordinates: rings },
});
const mask = createBuildingMask({
  features: [building([ring(0, 0, 10, 10), ring(3, 3, 7, 7)])],
});
test("interiors are masked; streets and courtyards stay open", () => {
  assert.ok(mask.contains(1, 1));
  assert.ok(mask.contains(0, 1));
  assert.ok(!mask.contains(5, 5));
  assert.ok(!mask.contains(-1, 1));
  assert.ok(mask.hides({ x: -1, y: 1, bx: 1, by: 1 }));
  assert.ok(!createBuildingMask(null).hides({ x: 0, y: 0 }));
});
test("multipolygons and concave footprints retain their shape", () => {
  const m = createBuildingMask({
    features: [
      building(
        [
          [ring(20, 20, 22, 22)],
          [
            [
              [0, 0],
              [4, 0],
              [4, 1],
              [1, 1],
              [1, 4],
              [0, 4],
              [0, 0],
            ],
          ],
        ],
        "MultiPolygon",
      ),
    ],
  });
  assert.ok(m.contains(21, 21));
  assert.ok(m.contains(0.5, 3));
  assert.ok(!m.contains(3, 3));
});
test("trail segments cannot cross a building even between exterior samples", () => {
  assert.ok(mask.crosses([-1, 1], [11, 1]));
  assert.ok(!mask.crosses([4, 4], [6, 6]));
  assert.ok(!mask.crosses([-1, -1], [11, -1]));
  const frames = [-1, 1, 11, 12].map((x) => ({
    tr: [{ id: 1, x, y: 1, z: 0 }],
  }));
  assert.deepEqual(buildTrailHistories(frames, [1], mask).get(1), [
    [11, 0.05, -1],
    [12, 0.05, -1],
  ]);
  assert.deepEqual(
    buildTrailHistories([frames[0], frames[2]], [1], mask).get(1),
    [[11, 0.05, -1]],
  );
});
test("background starts off in production markup", async () => {
  const { readFile } = await import("node:fs/promises");
  const markup = await readFile(
    new URL("../../_layouts/scene.njk", import.meta.url),
    "utf8",
  );
  assert.doesNotMatch(
    markup.match(/<input[^>]+id="scene-background"[^>]*>/)[0],
    /checked/,
  );
});
