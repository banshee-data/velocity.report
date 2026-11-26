import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import test from "node:test";
import { gunzipSync } from "node:zlib";
import { buildBakedRoadContact } from "../sf-baked-road-contact.js";
import {
  buildCollisionIndex,
  collisionAt,
  contactStep,
  groundAt,
} from "../sf-drive-core.js";
import { GROUND_MISSING, terrainTriangles } from "../sf-drive-surface.js";
import { roadMarkingTriangles } from "../sf-road-markings.js";

const tileDir = new URL("../../sf/tiles/", import.meta.url);
const manifest = readFileSync(new URL("manifest.json", tileDir));
const review = JSON.parse(manifest).terrainReview;
const hash = (data) => createHash("sha256").update(data).digest("hex");

function tile(token) {
  const raw = readFileSync(new URL(`${token}.json.gz`, tileDir));
  assert.equal(hash(raw), review.tiles[token].sha256);
  return JSON.parse(gunzipSync(raw));
}

test("rendered ground, vector paint and contact use the same fine triangles and gaps", () => {
  const g = { x0: 0, y0: 0, s: 2, n: 3, z: Array(9).fill(0) };
  g.detail = {
    x0: 0,
    y0: 0,
    s: 1,
    columns: 3,
    rows: 3,
    z: [0, 0, 0, 0, 10, 0, 0, 0, GROUND_MISSING],
  };
  const world = { g, o: [110, -20] };
  const region = [
    [110, -20],
    [114, -20],
    [114, -16],
    [110, -16],
  ];
  const { positions } = terrainTriangles(world, region);
  assert.deepEqual(roadMarkingTriangles(world, region, [region]), positions);
  assert.equal(positions.length, (3 * 2 + 3 * 2) * 9);
  assert.equal(groundAt(g, 0.75, 0.75, false), 0.5);
  assert.equal(groundAt(g, 1.5, 1.5, false), null);
  assert.equal(groundAt(g, 2, 0.75, false), 0);
});

test("disjoint terrain detail patches retain coarse contact between them", () => {
  const rectangle = (x0, z) => ({
    x0,
    y0: 0,
    s: 1,
    columns: 3,
    rows: 3,
    z: [0, 0, 0, 0, z, 0, 0, 0, 0],
  });
  const g = {
    x0: 0,
    y0: 0,
    s: 2,
    n: 4,
    z: Array(16).fill(0),
    detail: rectangle(0, 10),
    details: [rectangle(4, 20)],
  };
  const world = { g, o: [0, 0] };
  const boundary = [
    [0, 0],
    [6, 0],
    [6, 6],
    [0, 6],
  ];
  const { positions } = terrainTriangles(world, boundary);
  assert.deepEqual(
    roadMarkingTriangles(world, boundary, [boundary]),
    positions,
  );
  const heights = positions.filter((_, index) => index % 3 === 1);
  assert.ok(heights.includes(1));
  assert.ok(heights.includes(2));
  assert.equal(groundAt(g, 1, 1, false), 1);
  assert.equal(groundAt(g, 3, 1, false), 0);
  assert.equal(groundAt(g, 5, 1, false), 2);
});

test("pavement draw adjustments leave the sampled driving ground intact", () => {
  const g = {
    x0: 0,
    y0: 0,
    s: 2,
    n: 2,
    z: [50, 50, 50, 50],
    renderZ: [[0, 46]],
  };
  const region = [
    [0, 0],
    [2, 0],
    [2, 2],
    [0, 2],
  ];
  const { positions } = terrainTriangles({ g, o: [0, 0] }, region);
  assert.equal(positions[1], 4.6);
  assert.equal(groundAt(g, 0, 0, false), 5);
  assert.deepEqual(g.z, [50, 50, 50, 50]);
});

test("the 21st Street approaches keep the jog and omit the embankment crossing", () => {
  const world = tile("808f7fbc");
  const ground = (x, y) =>
    groundAt(world.g, x - world.o[0], y - world.o[1], false);
  const rows = world.r.filter(([name]) => world.n[name] === "21st St");
  const ends = rows.flatMap(([, , , p]) => [
    [world.o[0] + p[0] / 10, world.o[1] + p[1] / 10],
    [world.o[0] + p.at(-2) / 10, world.o[1] + p.at(-1) / 10],
  ]);
  assert.ok(ends.some(([x, y]) => x === 4857.2 && y === -55.7));
  assert.ok(ends.some(([x, y]) => Math.hypot(x - 4854.7, y + 23.2) < 0.001));
  assert.ok(ends.some(([x]) => x === 4729));
  assert.ok(
    rows.every(([, , , positions]) => {
      const xs = positions
        .filter((_, i) => i % 2 === 0)
        .map((x) => world.o[0] + x / 10);
      return Math.min(...xs) >= 4729 || Math.max(...xs) <= 4701.5;
    }),
  );
  for (let x = 4730; x < 4770; x++) {
    const y = -65.5 + 0.077 * (x - 4729);
    const z = ground(x, y);
    assert.ok(Number.isFinite(z));
    assert.ok(Math.abs(ground(x + 1, y + 0.077) - z) < 0.22, `Grade at ${x}`);
    assert.ok(
      Math.abs(ground(x, y - 1.5) - ground(x, y + 1.5)) < 0.25,
      `Crossfall at ${x}`,
    );
  }
  const index = buildCollisionIndex(world);
  const obstacle = (x) => {
    const y = -65.5 + 0.077 * (x - 4729);
    return collisionAt([index], { x, y, heading: 0.077 }, ground(x, y));
  };
  for (let x = 4754; x <= 4813; x++)
    assert.equal(obstacle(x), null, `Clear pavement at ${x}`);
  assert.equal(obstacle(4826), "building");
});

test("23rd Street keeps the aerial gap in its ground surface", () => {
  const world = tile("808f7fb4");
  const surface = (x, y) => ({
    height: groundAt(world.g, x - world.o[0], y - world.o[1], false),
  });
  const y = (x) => -542.9 + 0.077 * (x - 4152.1);
  for (let x = 4154; x < 4166; x++) {
    assert.ok(
      Math.abs(surface(x + 1, y(x + 1)).height - surface(x, y(x)).height) <
        0.11,
    );
  }
  const deck = world.raisedRoads.find(
    ({ id }) => id === "bridge/1238000/both/terrain-review",
  );
  assert.ok(deck);
  assert.ok(
    deck.points.every(
      ([x, y, z]) => z > 14.9 && z < 15.2 && surface(x, y).height === null,
    ),
  );
  assert.equal(surface(4210, y(4210)).height, null);
  let car = {
    x: 4154,
    y: y(4154),
    v: 4,
    z: surface(4154, y(4154)).height,
    heading: Math.atan(0.077),
    grounded: true,
    vz: 0,
  };
  for (let i = 0; i < 1200 && !car.blocked; i++) {
    const next = contactStep(car, { throttle: 0 }, 1 / 120, surface, 1.35);
    assert.ok(Math.abs(next.z - car.z) < 0.03);
    car = next;
  }
  assert.ok(car.blocked && car.x >= 4165 && car.x <= 4168);
});

test("23rd Street bridges the gap with lower contact and two pavement handoffs", () => {
  const world = tile("808f7fb4");
  const ground = (x, y) =>
    groundAt(world.g, x - world.o[0], y - world.o[1], false);
  const road = world.playableRoads.find(
    ({ id }) => id === "sf-contact/dogpatch-23rd-lower",
  );
  const { surface, portals } = buildBakedRoadContact(road);
  assert.equal(surface.kind, "bridge_deck");
  assert.equal(portals.length, 2);
  for (const { x, y, z } of portals)
    assert.ok(Math.abs(ground(x, y) - z) < 0.03);
  assert.ok(Math.abs(surface.heightAt(4191, -539.9) - 15.09) < 0.01);
  assert.equal(ground(4210, -538.45), null);
  const hidden = surface.heightAt(4210, -538.45);
  assert.ok(hidden > 12.5 && hidden < 14.5);
  for (let i = 1; i < road.points.length; i++) {
    const a = road.points[i - 1],
      b = road.points[i];
    assert.ok(
      Math.abs(b[2] - a[2]) / Math.hypot(b[0] - a[0], b[1] - a[1]) < 0.17,
    );
  }
});

test("the quest manifest pins the corrected game tiles", () => {
  const route = JSON.parse(
    readFileSync(new URL("../quest-route.json", tileDir)),
  );
  assert.equal(route.manifestSha256, hash(manifest));
});
