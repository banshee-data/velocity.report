import test from "node:test";
import assert from "node:assert/strict";
import {
  GROUND_MISSING,
  carStep,
  groundAt,
  groundAtWorld,
  nextCrossing,
  streetAt,
  tilesAround,
} from "../sf-drive-core.js";

const flatGrid = (zDm, n = 3) => ({
  x0: -8,
  y0: -8,
  s: 8,
  n,
  z: new Array(n * n).fill(zDm),
});

test("tilesAround loads near tiles nearest first and drops far ones", () => {
  const tiles = [
    { t: "a", x: 0, y: 0, half: 600 },
    { t: "b", x: 1200, y: 0, half: 600 },
    { t: "c", x: 5000, y: 0, half: 600 },
  ];
  const { want, drop } = tilesAround(tiles, 100, 0);
  assert.deepEqual(want, ["a", "b"]);
  assert.deepEqual(drop, ["c"]);
});

test("groundAt interpolates and skips missing corners", () => {
  const g = flatGrid(125);
  assert.equal(groundAt(g, 0, 0), 12.5);
  g.z[4] = GROUND_MISSING;
  assert.equal(groundAt(g, 0, 0), 12.5);
  assert.equal(groundAt(g, 100, 0), null);
});

test("groundAtWorld uses the tile whose origin is nearest", () => {
  const near = { o: [0, 0], g: flatGrid(10) };
  const far = { o: [10, 0], g: { ...flatGrid(50), x0: -20, y0: -20, s: 20 } };
  assert.equal(groundAtWorld([far, near], 1, 1), 1);
});

test("carStep accelerates, brakes to rest and turns with steer", () => {
  let car = { x: 0, y: 0, heading: 0, v: 0 };
  for (let i = 0; i < 60; i++) car = carStep(car, { throttle: 1 }, 1 / 30);
  assert.ok(car.v > 5 && car.x > 3, `moving forward: v=${car.v}`);
  const before = car.heading;
  car = carStep(car, { throttle: 1, steer: 1 }, 1 / 30);
  assert.ok(car.heading > before, "left steer turns left");
  for (let i = 0; i < 120; i++) car = carStep(car, { brake: true }, 1 / 30);
  assert.equal(car.v, 0);
});

test("carStep never exceeds its speed limits", () => {
  let car = { x: 0, y: 0, heading: 0, v: 0 };
  for (let i = 0; i < 3000; i++) car = carStep(car, { throttle: 1 }, 1 / 30);
  assert.ok(car.v <= 27);
});

const tile = {
  o: [0, 0],
  half: 600,
  n: ["Mission St", "Beale St"],
  r: [
    [0, 4, 0, [-1000, 0, 1000, 0], []],
    [1, 4, 0, [500, -1000, 500, 1000], []],
  ],
  s: [[500, 0, 0, 1]],
};

test("streetAt prefers the street aligned with the heading", () => {
  assert.equal(streetAt([tile], 49, 1, 0), "Mission St");
  assert.equal(streetAt([tile], 50, -20, Math.PI / 2), "Beale St");
  assert.equal(streetAt([tile], 0, 300, 0), null);
});

test("nextCrossing names the cross street ahead, not the current one", () => {
  const next = nextCrossing([tile], 0, 0, 0, "Mission St");
  assert.deepEqual(next.names, ["Beale St"]);
  assert.equal(Math.round(next.distance), 50);
  assert.equal(nextCrossing([tile], 0, 0, Math.PI, "Mission St"), null);
});
