import test from "node:test";
import assert from "node:assert/strict";
import { Vector3 } from "../../vendor/three/three.core.js";
import { buildCollisionIndex, carStep, contactStep } from "../sf-drive-core.js";
import {
  replayTreeIndex,
  treeDamageForImpact,
  treeFallAngle,
} from "../sf-tree-damage.js";
import { crashRoadSupport, indexCrashRails } from "../sf-highway-edge.js";
import { drivingHull, obstacleContact } from "../sf-chassis-contact.js";
import {
  beginCrash,
  crashCar,
  hardLanding,
  scrapeObstacle,
  seriousImpact,
  stepCrash,
} from "../sf-crash.js";

const flat = () => 0;
const supportOn = (heightAt) => (x, y) => {
  const z = heightAt(x, y);
  return z === null ? null : { z, heightAt };
};
const car = { x: 0, y: 0, z: 0, heading: 0, v: 35, grounded: true };
const dt = 1 / 120;
const near = (a, b, tolerance = 1e-7) =>
  assert.ok(Math.abs(a - b) < tolerance, `${a} vs ${b}`);
const trunk = (offset) => [
  buildCollisionIndex({ o: [0, 0], b: [], tr: [[24, offset * 10, 10, 2]] }),
];
const wall = [
  buildCollisionIndex({
    o: [0, 0],
    tr: [],
    b: [[0, [[[21, -100, 100, 80, -100, 100, 80, 100, 100, 21, 100, 100]]]]],
  }),
];
function simulate(car, indexes, terrain = flat, hz = 60) {
  const hit = obstacleContact(indexes, drivingHull(car));
  const body = beginCrash(car, hit?.kind || "ground", hit, terrain);
  let minUp = 1,
    maxLateralTilt = 0,
    maxSpeed = 0;
  for (let frame = 0; frame < 2 * hz; frame++)
    for (let tick = 0; tick < 120 / hz; tick++) {
      stepCrash(body, supportOn(terrain), indexes);
      const up = new Vector3(0, 0, 1).applyQuaternion(body.rotation);
      assert.ok(body.position.toArray().every(Number.isFinite));
      minUp = Math.min(minUp, up.z);
      if (Math.abs(up.y) > Math.abs(maxLateralTilt)) maxLateralTilt = up.y;
      maxSpeed = Math.max(maxSpeed, body.velocity.length());
    }
  near(body.rotation.length(), 1);
  return { body, minUp, maxLateralTilt, maxSpeed };
}

test("square bumper stops or rebounds along the wall normal; opposite corners of a trunk create opposite yaw", () => {
  const square = simulate(car, wall);
  near(crashCar(square.body, car).heading, 0);
  near(square.body.position.y, 0);
  assert.ok(square.body.position.x < 0.55 && square.maxSpeed < car.v);
  const left = simulate(car, trunk(0.7)),
    right = simulate(car, trunk(-0.7));
  const bearing = crashCar(left.body, car).heading;
  assert.ok(Math.abs(bearing) > 0.5 && Math.abs(left.body.position.y) > 2);
  near(bearing, -crashCar(right.body, car).heading, 1e-5);
  near(left.body.position.y, -right.body.position.y, 1e-5);
});

test("a hard wall strike dissipates closing speed over a bounded crush distance", () => {
  const hit = obstacleContact(wall, drivingHull(car));
  const crash = beginCrash(car, "building", hit, flat);
  assert.ok(crash.velocity.x > car.v / 2 && crash.velocity.x < car.v);
  let speed = car.v,
    lastAcceleration = 0,
    peakJerk = 0;
  for (let tick = 0; tick < 100; tick++) {
    if (tick > 0) stepCrash(crash, supportOn(flat), wall);
    const acceleration = (crash.velocity.x - speed) / dt;
    peakJerk = Math.max(
      peakJerk,
      Math.abs(acceleration - lastAcceleration) / dt,
    );
    lastAcceleration = acceleration;
    speed = crash.velocity.x;
    // The wall is at x=2.1, and only the leading crush zone can cross it.
    assert.ok(crash.position.x + 2.23 <= 2.1 + 0.55 + 1e-4);
  }
  assert.ok(peakJerk < (0.25 * car.v) / (dt * dt));
  assert.equal(crash.outcome, "stopped");
  assert.equal(crash.velocity.length(), 0);
});

test("a fast shallow graze removes the closing component, scrubs tangent speed and leaves reverse available", () => {
  const glancing = { ...car, x: 1.4, heading: Math.PI / 2 - 0.06 };
  const hit = obstacleContact(wall, drivingHull(glancing));
  assert.equal(hit.kind, "building");
  assert.equal(seriousImpact(glancing, hit), false);
  assert.equal(
    seriousImpact(car, obstacleContact(wall, drivingHull(car))),
    true,
  );
  const slide = scrapeObstacle(glancing, hit);
  near(slide.vx, 0);
  assert.ok(slide.vy > 30 && slide.vy < 35 && slide.slip > 0);
  assert.equal(obstacleContact(wall, drivingHull(slide)), null);
  const stopped = scrapeObstacle(
    { ...car, v: 1 },
    obstacleContact(wall, drivingHull(car)),
  );
  const backing = carStep(stopped, { throttle: -1 }, dt);
  assert.ok(backing.v < 0 && backing.x < stopped.x);
});

test("combined tyre demand shares traction, and braking still catches a sideways slide at rest", () => {
  const turning = { ...car, steering: 0.24 };
  const turn = carStep(turning, { steer: 1 }, dt);
  const brake = carStep(turning, { steer: 1, brake: true }, dt);
  const straight = carStep(car, { brake: true }, dt);
  assert.ok(brake.v > straight.v && brake.v < turn.v);
  assert.ok(
    brake.lateralAcceleration > 0 &&
      brake.lateralAcceleration < turn.lateralAcceleration,
  );
  const pavement = carStep({ ...car, v: 0, slip: 4 }, { brake: true }, dt);
  const dirt = carStep({ ...car, v: 0, slip: 4 }, { brake: true }, dt, {
    offroad: true,
  });
  assert.ok(pavement.slip > 0 && pavement.slip < dirt.slip && dirt.slip < 4);
});

test("one side can strike a raised bank first and roll past vertical, mirrored with the terrain", () => {
  const airborne = {
    ...car,
    grounded: false,
    z: 1.1,
    v: 20,
    vx: 20,
    vy: 0,
    vz: -17,
  };
  const crashOn = (side) => {
    const bank = (x, y) => (side * y > 0.45 ? 1 : 0);
    const incoming = hardLanding(
      airborne,
      { ...airborne, x: 20 * dt },
      supportOn(bank),
    );
    assert.ok(
      incoming.z > bank(incoming.x, incoming.y),
      "centre is still airborne",
    );
    return simulate(incoming, [], bank);
  };
  assert.equal(
    hardLanding(airborne, { ...airborne, x: 20 * dt }, supportOn(flat)),
    null,
  );
  const left = crashOn(1),
    right = crashOn(-1);
  assert.ok(left.minUp < -0.2 && right.minUp < -0.2);
  near(left.maxLateralTilt, -right.maxLateralTilt, 1e-5);
  assert.ok(Math.abs(left.maxLateralTilt) > 0.5);
});

test("controls and render batching cannot change the passive trajectory", () => {
  const result = simulate({ ...car, steering: -1 }, trunk(0.7), flat, 30).body;
  for (const hz of [60, 120]) {
    const other = simulate({ ...car, steering: 1 }, trunk(0.7), flat, hz).body;
    assert.deepEqual(other.position, result.position);
    assert.deepEqual(other.rotation, result.rotation);
  }
});

test("a slow touchdown on an unsupported roof still enters recovery", () => {
  const high = {
    ...car,
    x: 5,
    z: 11,
    grounded: false,
    v: 0,
    vx: 0,
    vy: 0,
    vz: -2,
  };
  assert.equal(obstacleContact(wall, drivingHull(high)), null);
  const touching = { ...high, z: 9.99 };
  const hit = obstacleContact(wall, drivingHull(touching), drivingHull(high));
  assert.equal(hit.nz, 1);
  near(hit.depth, 0.01);
  assert.equal(seriousImpact(touching, hit), true);
  assert.equal(seriousImpact({ ...touching, vz: 1 }, hit), false);
  const underside = { ...hit, nz: -1 };
  assert.equal(seriousImpact({ ...touching, vz: 2 }, underside), false);
});

test("gravity changes a level drop's velocity without rotating the chassis or inventing horizontal travel", () => {
  const drop = { ...car, grounded: false, z: 1.3, v: 0, vx: 0, vy: 0, vz: -18 };
  const support = supportOn(flat);
  assert.equal(hardLanding(drop, drop, support), null);
  const landing = hardLanding({ ...drop, z: 0.08 }, drop, support);
  assert.ok(landing);
  const { body, minUp } = simulate(landing, [], flat);
  near(minUp, 1);
  near(body.position.x, 0);
  near(body.position.y, 0);
});

test("recoverable touchdown keeps horizontal motion and the next input steers", () => {
  const before = {
    ...car,
    v: 24,
    vx: 24,
    vy: 0,
    vz: -12,
    z: 0.08,
    grounded: false,
  };
  const pavement = () => ({ height: 0, drivable: true });
  const landed = contactStep(before, {}, dt, pavement);
  assert.equal(hardLanding(before, landed, supportOn(flat)), null);
  assert.equal(landed.grounded, true);
  near(landed.v, 24);
  const next = contactStep(landed, { throttle: 1, steer: 1 }, dt, pavement);
  assert.ok(next.v > landed.v);
  assert.ok(next.heading > landed.heading);
});

test("the same normal impact breaks a thin trunk, cracks a larger one and leaves a stout one standing", () => {
  const tile = {
    o: [0, 0],
    b: [],
    tr: [
      [23, 0, 3.75, 1.5],
      [23, 50, 5, 2],
      [23, 100, 7.5, 3],
    ],
  };
  const index = buildCollisionIndex(tile, "fixture");
  const strike = (y, speed) =>
    obstacleContact([index], drivingHull({ ...car, y, v: speed }));
  const damaged = (hit, speed, previous) =>
    treeDamageForImpact(hit.tree, 1280, speed, hit.nx, hit.ny, previous);
  const front = strike(0, 10);
  const light = damaged(front, 3);
  const broken = damaged(front, 10);
  const crack = damaged(strike(5, 10), 10);
  assert.equal(light, null);
  near(broken.diameter, 0.3);
  assert.equal(broken.stage, "broken");
  assert.deepEqual(JSON.parse(JSON.stringify(broken)), broken);
  assert.equal(crack.stage, "cracked");
  assert.equal(crack.diameter, 0.4);
  assert.equal(damaged(strike(10, 10), 10), null);
  const secondStrike = damaged(strike(5, 10), 10, crack);
  assert.equal(secondStrike.stage, "broken");
  assert.deepEqual(secondStrike.priorLean, [crack.east, crack.north]);
  assert.equal(replayTreeIndex(tile, broken), 0);
  assert.equal(
    replayTreeIndex({ ...tile, tr: [...tile.tr].reverse() }, broken),
    2,
  );
  assert.equal(replayTreeIndex({ ...tile, tr: tile.tr.slice(1) }, broken), -1);
  assert.ok(broken.east > 0.99);
  assert.equal(treeFallAngle(broken, 0), 0);
  assert.ok(treeFallAngle(broken, 0.45) > 0);
  assert.ok(treeFallAngle(broken, 0.9) > Math.PI / 2 - 0.1);
  assert.ok(treeFallAngle(crack, 0.9) < 0.2);

  // The trunk takes some energy, yields, and no longer blocks the path.
  const incoming = { ...car, v: 10 };
  const rigid = beginCrash(incoming, "tree", front, flat);
  const yielded = beginCrash(
    incoming,
    "tree",
    { ...front, yielded: broken.absorbed },
    flat,
  );
  assert.ok(
    yielded.velocity.x > rigid.velocity.x && yielded.velocity.x < incoming.v,
  );
  const damage = new Map([
    [0, broken],
    [1, crack],
  ]);
  const reloaded = buildCollisionIndex(tile, "fixture", damage);
  assert.equal(obstacleContact([reloaded], drivingHull(incoming)), null);
  assert.equal(
    obstacleContact([reloaded], drivingHull({ ...incoming, y: 5 })).kind,
    "tree",
  );
});
test("take-off pitch and bank survive ballistic motion and are used at impact", () => {
  const surface = (x, y) => ({
    height: (x <= 0 ? 0.15 * x : -2) + 0.12 * y,
    drivable: true,
  });
  const c = { ...car, x: -0.02, z: -0.003, v: 25, vz: 0.15 * 25 };
  const launched = contactStep(c, {}, dt, surface);
  assert.equal(launched.grounded, false);
  const next = contactStep(launched, { throttle: 1, steer: 1 }, dt, surface);
  assert.deepEqual(next.airAttitude, launched.airAttitude);
  const body = beginCrash(next, "tree", null, flat);
  const axis = new Vector3(1, 0, 0).applyQuaternion(body.rotation);
  near(Math.asin(axis.z), launched.airAttitude.pitch);
  near(launched.airAttitude.pitch, Math.atan(0.15));
  near(
    launched.airAttitude.roll,
    Math.atan(0.12 * Math.cos(launched.airAttitude.pitch)),
  );
});

test("a supported chassis at a deck edge stays level above a 30 m drop; slopes use the same deck's tangent", () => {
  const terrain = () => -10;
  for (const slope of [0, 0.12]) {
    const heightAt = (x, y) => (y >= 0 ? 20 + slope * y : null);
    const surfaces = new Map([["upper", { id: "upper", heightAt }]]);
    const source = { ...car, x: 0, y: 1, z: heightAt(0, 1), v: 0 };
    const body = beginCrash(source, "ground", null, heightAt);
    const support = crashRoadSupport(terrain, surfaces);
    for (let i = 0; i < 240; i++) stepCrash(body, support);
    near(body.position.x, 0);
    near(crashCar(body, source).y, 1, 0.02);
    near(body.position.z, source.z + 0.78 / Math.hypot(1, slope), 0.01);
    const up = new Vector3(0, 0, 1).applyQuaternion(body.rotation);
    near(up.y, -slope / Math.hypot(1, slope), 0.015);
    assert.equal(support(0, -1, { x: 0, y: 1, z: 20 }).z, -10);
  }
});

test("secondary rails are spatially selected across route sections, in either direction and only at matching height", () => {
  function collide(base, v) {
    const rail = { a: [5, 20, base], b: [5, -20, base], nx: -1, ny: 0 };
    const rails = indexCrashRails([
      { id: "initial", railSegments: [] },
      { id: "adjacent", railSegments: [rail] },
    ]);
    const start = {
      ...car,
      x: v > 0 ? 0 : 10,
      heading: v > 0 ? 0 : Math.PI,
      v: Math.abs(v),
      surfaceId: "initial",
    };
    const body = beginCrash(start, "tree", null, flat);
    for (let i = 0; i < 45; i++) stepCrash(body, supportOn(flat), [], rails);
    return body;
  }
  assert.ok(collide(0, 30).position.x < 5);
  assert.ok(collide(0, -30).position.x > 5);
  assert.ok(collide(12, 30).position.x > 8);
});

test("a hard touchdown resolves the first crossed deck during handoff and cannot snap up from below", () => {
  const deck = { id: "upper", heightAt: () => 20 };
  const support = crashRoadSupport(() => -10, new Map([[deck.id, deck]]));
  const before = {
    ...car,
    z: 20.08,
    grounded: false,
    v: 0,
    vx: 0,
    vy: 0,
    vz: -18,
  };
  const incoming = hardLanding(before, before, support);
  assert.ok(incoming.z < 20);
  const body = beginCrash(incoming, "ground", null, deck.heightAt, {
    previous: before,
    supportAt: support,
  });
  for (let i = 0; i < 240; i++) {
    assert.ok(body.bounds.low >= 20 - 1e-6);
    stepCrash(body, support);
  }
  near(crashCar(body, before).z, 20, 0.002);
  near(body.rotation.w, 1);
  assert.equal(hardLanding({ ...before, z: 18 }, before, support), null);
});

test("settling requires sustained supported rest and falling ends with a separate outcome", () => {
  const stopped = beginCrash({ ...car, v: 0 }, "tree", null, flat);
  for (let i = 0; i < 29; i++) stepCrash(stopped, supportOn(flat));
  assert.equal(stopped.outcome, null);
  stepCrash(stopped, supportOn(flat));
  assert.equal(stopped.outcome, "stopped");
  assert.equal(stopped.velocity.length(), 0);
  assert.equal(stopped.momentum.length(), 0);
  const end = crashCar(stopped, car);
  stepCrash(stopped, supportOn(flat));
  assert.deepEqual(crashCar(stopped, car), end);

  const moving = beginCrash(car, "tree", null, flat);
  for (let i = 0; i < 240; i++) stepCrash(moving, supportOn(flat));
  assert.equal(
    moving.outcome,
    null,
    "a moving chassis continues beyond the old two-second limit",
  );

  const falling = beginCrash(
    { ...car, v: 0, grounded: false, vx: 0, vy: 0, vz: 0 },
    "fall",
    null,
    flat,
  );
  for (let i = 0; i < 29; i++) stepCrash(falling, () => null);
  assert.equal(falling.restTicks, 0);
  for (let i = 0; i < 600 && !falling.outcome; i++)
    stepCrash(falling, () => null);
  assert.equal(falling.outcome, "lost ground");
});

test("off-centre yielding trunk transfers momentum and turns the crash trajectory", () => {
  const incoming = { ...car, v: 24, slip: 0 };
  const index = buildCollisionIndex(
    { o: [0, 0], b: [], tr: [[23, 7, 5, 2]] },
    "fixture",
  );
  const hit = obstacleContact([index], drivingHull(incoming));
  const tree = hit.tree,
    damage = treeDamageForImpact(tree, 1280, 24, hit.nx, hit.ny);
  assert.equal(damage.stage, "broken");
  const crash = beginCrash(
    incoming,
    "tree",
    { ...hit, yielded: damage.absorbed },
    flat,
  );
  assert.ok(crash.velocity.x < incoming.v - 1 && crash.velocity.x > 0);
  assert.ok(Math.abs(crash.momentum.z) > 0.5);
  assert.ok(crash.velocity.y < incoming.slip);
  const clearIndex = buildCollisionIndex(
    { o: [0, 0], b: [], tr: [[23, 7, 5, 2]] },
    "fixture",
    null,
    () => 0.5,
  );
  assert.equal(
    obstacleContact([clearIndex], drivingHull({ ...incoming, z: 1 })),
    null,
  );
  assert.equal(
    obstacleContact([clearIndex], drivingHull({ ...incoming, z: 0 })).kind,
    "tree",
  );
});

test("an uphill deck clears a clipped trunk without clearing contact below", () => {
  const deck = (x) => (x <= 3 ? 2 + x * 0.12 : null);
  const trees = [
    buildCollisionIndex(
      { o: [0, 0], b: [], tr: [[24, 0, 8, 1]] },
      "fixture",
      null,
      (x) => deck(x) - 0.12,
    ),
  ];
  const onDeck = drivingHull({ ...car, x: 0, y: 0, z: 2 });
  assert.equal(obstacleContact(trees, onDeck).kind, "tree");
  assert.equal(obstacleContact(trees, onDeck, undefined, deck), null);
  const underDeck = drivingHull({ ...car, x: 0, y: 0, z: 1 });
  assert.equal(
    obstacleContact(trees, underDeck, undefined, () => 1).kind,
    "tree",
  );
  assert.equal(
    obstacleContact(trees, onDeck, undefined, () => null).kind,
    "tree",
  );
});
