import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { gunzipSync } from "node:zlib";
import { createHash } from "node:crypto";
import {
  advanceTrial,
  buildCollisionIndex,
  carStep,
  collisionAt,
  createTrial,
  formatBytes,
  formatTime,
  groundAt,
  groundAtWorld,
  intersectionsForTile,
  nearestRoad,
  tileAt,
} from "../sf-drive-core.js";
import { terrainTriangles, tileRegion } from "../sf-drive-surface.js";

const car = (v = 0) => ({ x: 0, y: 0, heading: 0, v });
const drive = (initial, input, seconds, surface = {}, hz = 120) => {
  let c = initial;
  for (let i = 0; i < seconds * hz; i++) c = carStep(c, input, 1 / hz, surface);
  return c;
};

test("hills, rolling resistance and off-road surfaces affect the car", () => {
  const level = drive(car(), { throttle: 1 }, 5);
  const uphill = drive(car(), { throttle: 1 }, 5, { slope: 0.18 });
  const dirt = drive(car(), { throttle: 1 }, 5, { offroad: true });
  assert.ok(uphill.v < level.v && dirt.v < level.v);
  assert.ok(drive(car(10), {}, 5).v < 10);
  assert.ok(drive(car(), {}, 3, { slope: -0.12 }).v > 1, "rolls downhill");
  assert.equal(
    drive(car(), { brake: true }, 3, { slope: -0.12 }).v,
    0,
    "brake holds on a hill",
  );
});

test("the GTI tune reaches 60 mph in under five seconds and 155 mph on level asphalt", () => {
  const throttle = { throttle: 1 };
  assert.ok(drive(car(), throttle, 4).v * 2.23694 < 60);
  assert.ok(drive(car(), throttle, 5).v * 2.23694 > 60);
  assert.ok(drive(car(), throttle, 30).v * 2.23694 > 155);
  assert.equal(drive(car(), { throttle: 1, brake: true }, 1).v, 0);
});

test("full braking, including the reverse pedal, reaches 1.25 g on level pavement", () => {
  const dt = 1 / 120;
  for (const v of [5, 25, 60, -5]) {
    for (const input of [{ brake: true }, { throttle: -Math.sign(v) }]) {
      const source = { x: 0, y: 0, v, heading: 0 };
      const next = carStep(source, input, dt);
      const dirt = carStep(source, input, dt, { offroad: true });
      const decel = (Math.abs(v) - Math.abs(next.v)) / dt;
      assert.ok(Math.abs(decel - 1.25 * 9.81) < 1e-9);
      assert.ok(Math.abs(dirt.v) > Math.abs(next.v));
    }
  }
});
test("braking stops without reversing; reverse engages after braking", () => {
  assert.equal(drive(car(10), { brake: true }, 4).v, 0);
  const braking = drive(car(10), { throttle: -1 }, 0.5);
  assert.ok(braking.v > 0 && braking.v < 10);
  const reverse = drive(car(10), { throttle: -1 }, 5);
  assert.ok(reverse.v < 0 && reverse.v >= -6);
});

test("steering takes time, unwinds, and respects lateral grip at speed", () => {
  const first = carStep(car(25), { steer: 1 }, 1 / 120);
  assert.ok(first.steering > 0 && first.steering < 0.02);
  const turned = drive(car(25), { steer: 1 }, 1);
  const next = carStep(turned, { steer: 1 }, 1 / 120);
  assert.ok(Math.abs((next.heading - turned.heading) * 120 * next.v) <= 7.501);
  assert.ok(next.lateralAcceleration > 0 && next.lateralAcceleration <= 7.501);
  assert.equal(carStep(car(), { steer: 1 }, 1 / 120).lateralAcceleration, 0);
  assert.ok(Math.abs(drive(turned, {}, 1).steering) < 0.001);
  assert.equal(drive(car(), { steer: 1 }, 1).heading, 0, "no rotation at rest");
});

test("motion stays consistent across common frame rates", () => {
  const a = drive(car(), { throttle: 1, steer: 0.3 }, 5, {}, 30);
  const b = drive(car(), { throttle: 1, steer: 0.3 }, 5, {}, 120);
  assert.ok(Math.abs(a.v - b.v) < 0.05);
  assert.ok(Math.hypot(a.x - b.x, a.y - b.y) < 0.5);
});

const building = (base = 0) => [
  base * 10,
  [
    [
      [100, 100, 200, 200, 100, 200, 200, 200, 200, 100, 200, 200],
      [120, 120, 200, 180, 120, 200, 180, 180, 200, 120, 180, 200],
    ],
  ],
  0,
];
const obstacleTile = (b = [], tr = []) => ({ o: [0, 0], b, tr });

test("collision covers bumpers, bucket boundaries and tree trunks, leaving canopy clear", () => {
  const index = buildCollisionIndex(obstacleTile([], [[319, 0, 10, 8]]));
  assert.equal(collisionAt([index], { ...car(), x: 29.7 }), "tree");
  assert.equal(collisionAt([index], { ...car(), x: 31.9, y: 5 }), null);
  const corner = buildCollisionIndex(obstacleTile([], [[22, 9, 3, 1]]));
  assert.equal(
    collisionAt([corner], car()),
    "tree",
    "front bumper corners also collide",
  );
});

test("building collisions respect courtyards and elevated structure bases", () => {
  const index = buildCollisionIndex(obstacleTile([building()]));
  assert.equal(collisionAt([index], { ...car(), x: 10, y: 15 }, 0), "building");
  assert.equal(collisionAt([index], { ...car(), x: 15, y: 15 }, 0), null);
  const elevated = buildCollisionIndex(obstacleTile([building(10)]));
  assert.equal(collisionAt([elevated], { ...car(), x: 10, y: 15 }, 0), null);
});

test("intersection centres can use carriageways owned by neighboring tiles", () => {
  const signs = {
    o: [0, 0],
    s: [
      [-50, -50, 0, 0, 1, 90],
      [50, 50, 0, 0, 1, 90],
    ],
    n: ["A", "B"],
    r: [],
  };
  const roads = {
    o: [10, 0],
    n: ["B", "A"],
    r: [[1, 4, 0, [-200, 0, 0, 0], [40, 40, 40, 40]]],
  };
  const [p] = intersectionsForTile(signs, [signs, roads]);
  assert.equal(p.x, 0);
  assert.equal(p.y, 0);
  const edge = nearestRoad([roads], 0, 3);
  assert.equal(edge.width, 4);
  assert.equal(edge.distance, 3);
});

test("trial clock starts on input, survives crashes and streaming, and freezes at finish", () => {
  const trial = createTrial([
    { t: "a", x: 10, y: 0 },
    { t: "b", x: 40, y: 0 },
  ]);
  advanceTrial(trial, 15, car(), false);
  assert.equal(trial.elapsed, 0);
  advanceTrial(trial, 1, { ...car(), x: 10 }, true);
  assert.equal(trial.collected.size, 1);
  trial.crashes++;
  advanceTrial(trial, 2, { ...car(), x: 40 }, false, false);
  assert.equal(trial.collected.size, 1, "respawn cannot collect");
  advanceTrial(trial, 2, { ...car(), x: 40 }, false);
  assert.equal(trial.status, "finished");
  assert.equal(trial.elapsed, 5);
  advanceTrial(trial, 100, car(), true);
  assert.equal(trial.elapsed, 5);
  assert.equal(trial.crashes, 1);
  assert.equal(formatTime(65.43), "1:05.43");
  assert.equal(formatBytes(30182097), "30.2 MB");
});

test("strict ground queries stop the car at quads omitted from the rendered terrain", () => {
  const tile = {
    o: [0, 0],
    g: { x0: 0, y0: 0, s: 8, n: 2, z: [0, 0, 0, -32768] },
  };
  assert.equal(
    groundAtWorld([tile], 1, 1),
    0,
    "decorations may use neighboring samples",
  );
  assert.equal(
    groundAtWorld([tile], 1, 1, true),
    null,
    "physics requires an actual surface",
  );
});

test("terrain regions meet once and clipped triangles stay on the measured planes", () => {
  const rows = [
    { t: "a", x: 0, y: 0, half: 10 },
    { t: "b", x: 12, y: 0, half: 10 },
  ];
  const a = tileRegion(rows[0], rows),
    b = tileRegion(rows[1], rows);
  assert.equal(Math.max(...a.map((p) => p[0])), 6);
  assert.equal(Math.min(...b.map((p) => p[0])), 6);
  const tile = {
    o: [0, 0],
    g: { x0: 0, y0: 0, s: 8, n: 2, z: [0, 80, 160, 0] },
  };
  const triangles = terrainTriangles(tile, [
    [0, 0],
    [6, 0],
    [6, 8],
    [0, 8],
  ]);
  for (let i = 0; i < triangles.positions.length; i += 3) {
    const [x, z, north] = triangles.positions.slice(i, i + 3);
    assert.ok(x <= 6 + 1e-7);
    assert.ok(Math.abs(z - groundAt(tile.g, x, -north)) < 1e-6);
  }
  assert.equal(
    groundAt(tile.g, 2, 2),
    6,
    "height follows terrain triangles, not a different bilinear surface",
  );
});

test("the fixed route accounts for every baked tile and uses unique emoji at real intersections", () => {
  const base = new URL("../../sf/", import.meta.url);
  const raw = fs.readFileSync(new URL("tiles/manifest.json", base));
  const manifest = JSON.parse(raw),
    route = JSON.parse(fs.readFileSync(new URL("quest-route.json", base)));
  assert.equal(
    route.manifestSha256,
    createHash("sha256").update(raw).digest("hex"),
  );
  const assignments = JSON.parse(
    fs.readFileSync(
      new URL(
        "../../../../tools/sf-drive/quest-assignments.json",
        import.meta.url,
      ),
    ),
  );
  assert.equal(route.targets.length, 137);
  assert.equal(route.skipped.length, 25);
  for (const t of ["80858434", "808585cc"])
    assert.ok(
      route.skipped.some(
        (row) => row.t === t && row.reason === "No signposted intersection",
      ),
    );
  assert.equal(
    route.placementSha256,
    createHash("sha256")
      .update(JSON.stringify(route.targets.map(({ t, x, y }) => [t, x, y])))
      .digest("hex"),
  );
  assert.equal(
    new Set(route.targets.map((t) => t.emoji)).size,
    route.targets.length,
  );
  const byToken = new Map(
    [...route.targets, ...route.skipped].map((t) => [t.t, t]),
  );
  assert.equal(byToken.size, manifest.tiles.length);
  const readTile = (token) =>
    JSON.parse(
      gunzipSync(fs.readFileSync(new URL(`tiles/${token}.json.gz`, base))),
    );
  for (const row of manifest.tiles) {
    const tile = readTile(row.t);
    const target = byToken.get(row.t);
    assert.ok(target);
    if (target.emoji) {
      const assignment = assignments[row.t];
      assert.equal(target.emoji, assignment.emoji);
      if (assignment.intersection) {
        assert.deepEqual(
          [...target.names].sort(),
          [...assignment.intersection].sort(),
        );
        assert.equal(tileAt(manifest.tiles, target.x, target.y).t, row.t);
      }
      const signTile = assignment.signTile
        ? readTile(assignment.signTile)
        : tile;
      assert.ok(signTile.s.length >= 2);
      assert.ok(
        signTile.s.some(
          (s) =>
            Math.hypot(
              signTile.o[0] + s[0] / 10 - target.x,
              signTile.o[1] + s[1] / 10 - target.y,
            ) < 45,
        ),
      );
      assert.equal(
        collisionAt([buildCollisionIndex(tile)], { ...target, v: 0 }, 0) ===
          "tree",
        false,
      );
    } else assert.equal(tile.s.length, 0);
  }
});
