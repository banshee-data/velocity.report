import test from "node:test";
import assert from "node:assert/strict";
import {
  roadDashPolygons,
  roadMarkingTriangles,
  createRoadMarkingCache,
} from "../sf-road-markings.js";
import { groundAt } from "../sf-drive-core.js";

const near = (a, b) => assert.ok(Math.abs(a - b) < 1e-7, `${a} != ${b}`);
const box = (x0, y0, x1, y1) => [
  [x0, y0],
  [x1, y0],
  [x1, y1],
  [x0, y1],
];
const shape = (centre, dashed = true) => ({ centre, dashed });
const tile = {
  o: [100, 200],
  g: { x0: 0, y0: 0, n: 3, s: 4, z: [0, 20, 50, 10, 90, 25, 10, 20, 30] },
};
const region = box(100, 200, 108, 208);
const vertices = (positions) =>
  Array.from({ length: positions.length / 3 }, (_, i) =>
    positions.slice(i * 3, i * 3 + 3),
  );
const area = (positions) => {
  const p = vertices(positions);
  let sum = 0;
  for (let i = 0; i < p.length; i += 3) {
    const [a, b, c] = p.slice(i, i + 3);
    sum +=
      Math.abs((b[0] - a[0]) * (c[2] - a[2]) - (b[2] - a[2]) * (c[0] - a[0])) /
      2;
  }
  return sum;
};

test("dashes keep their phase across bends, short segments and repeated points", () => {
  const marks = roadDashPolygons([
    shape([
      [0, 0],
      [0, 0],
      [2, 0],
      [2, 6],
    ]),
  ]);
  assert.equal(marks.length, 3);
  near(marks[0][1][0], 2);
  near(marks[1][0][1], 0);
  near(marks[1][1][1], 1); // the first dash continues for its final metre
  near(marks[2][0][1], 4); // 6 m along the whole polyline
  near(marks[2][1][1], 6);
  assert.deepEqual(roadDashPolygons([shape([[0, 0]])]), []);
});

test("paths and one-way roads do not acquire invented centre paint", () => {
  assert.deepEqual(
    roadDashPolygons([
      shape(
        [
          [0, 0],
          [10, 0],
        ],
        false,
      ),
    ]),
    [],
  );
  const positions = roadMarkingTriangles(
    tile,
    region,
    roadDashPolygons([
      shape([
        [99, 202],
        [109, 202],
      ]),
    ]),
  );
  assert.ok(positions.length > 0);
  for (const [x, z, minusY] of vertices(positions)) {
    assert.ok(x >= 100 - 1e-7 && x <= 108 + 1e-7);
    assert.ok(-minusY >= 201.85 - 1e-7 && -minusY <= 202.15 + 1e-7);
    near(z, groundAt(tile.g, x - 100, -minusY - 200, false));
  }
});

test("markings crossing cell diagonals and hills stay on the exact original terrain planes", () => {
  const positions = roadMarkingTriangles(tile, region, [
    box(100.2, 200.4, 107.8, 207.6),
  ]);
  assert.ok(positions.length > 18);
  near(area(positions), 7.6 * 7.2);
  for (const [x, z, minusY] of vertices(positions)) {
    near(z, groundAt(tile.g, x - 100, -minusY - 200, false));
  }
});

test("adjacent ownership regions split paint without duplicating it", () => {
  const marks = [box(100, 201, 108, 203)];
  const left = roadMarkingTriangles(tile, box(100, 200, 104, 208), marks);
  const right = roadMarkingTriangles(tile, box(104, 200, 108, 208), marks);
  near(area(left), 8);
  near(area(right), 8);
  assert.ok(vertices(left).every((p) => p[0] <= 104 + 1e-7));
  assert.ok(vertices(right).every((p) => p[0] >= 104 - 1e-7));
});

test("missing ground and out-of-grid marks leave holes instead of floating paint", () => {
  const missing = { ...tile, g: { ...tile.g, z: [...tile.g.z] } };
  missing.g.z[4] = -32768;
  assert.deepEqual(roadMarkingTriangles(missing, region, [region]), []);
  assert.deepEqual(
    roadMarkingTriangles(tile, region, [box(120, 200, 125, 203)]),
    [],
  );
  assert.deepEqual(roadMarkingTriangles(tile, region, []), []);
});

test("cached contributor geometry matches a full clip and survives transferring its result", () => {
  const a = [box(100, 201, 104, 203)],
    b = [box(104, 201, 108, 203)];
  let clips = 0;
  const cache = createRoadMarkingCache((...args) => {
    clips++;
    return roadMarkingTriangles(...args);
  });
  cache.set("a", { tile, region, markings: a });
  cache.set("b", { tile, region, markings: b });
  const expected = new Float32Array(
    roadMarkingTriangles(tile, region, [...a, ...b]),
  );
  const result = cache.build("a", ["a", "b"]);
  assert.deepEqual(result, expected);
  assert.equal(clips, 2);
  structuredClone(result, { transfer: [result.buffer] });
  assert.equal(result.length, 0);
  assert.deepEqual(cache.build("a", ["a", "b"]), expected);
  assert.equal(clips, 2, "unchanged pairs are never reclipped");
  cache.remove("b");
  assert.deepEqual(
    cache.build("a", ["a"]),
    new Float32Array(roadMarkingTriangles(tile, region, a)),
  );
  assert.equal(clips, 2, "removal retains the remaining contributor");
  cache.set("b", { tile, region, markings: [box(105, 201, 108, 203)] });
  cache.build("a", ["a", "b"]);
  assert.equal(clips, 3, "only the replaced source is reclipped");
});
