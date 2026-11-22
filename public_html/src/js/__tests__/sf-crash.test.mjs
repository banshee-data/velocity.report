import test from "node:test";
import assert from "node:assert/strict";
import { Vector3 } from "../../vendor/three/three.core.js";
import { buildCollisionIndex, carStep } from "../sf-drive-core.js";
import { drivingHull, obstacleContact } from "../sf-chassis-contact.js";
import {
  beginCrash,
  crashCar,
  crashRemaining,
  hardLanding,
  scrapeObstacle,
  seriousImpact,
  stepCrash,
} from "../sf-crash.js";

const flat = () => 0;
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
      stepCrash(body, terrain, indexes);
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
  assert.ok(square.body.position.x < 0 && square.maxSpeed < car.v);
  const left = simulate(car, trunk(0.7)),
    right = simulate(car, trunk(-0.7));
  const bearing = crashCar(left.body, car).heading;
  assert.ok(Math.abs(bearing) > 0.5 && Math.abs(left.body.position.y) > 2);
  near(bearing, -crashCar(right.body, car).heading, 1e-5);
  near(left.body.position.y, -right.body.position.y, 1e-5);
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
    z: 1.55,
    v: 20,
    vx: 20,
    vy: 0,
    vz: -17,
  };
  const crashOn = (side) => {
    const bank = (x, y) => (side * y > 0.45 ? 1 : 0);
    const incoming = hardLanding(airborne, { ...airborne, x: 20 * dt }, bank);
    assert.ok(
      incoming.z > bank(incoming.x, incoming.y),
      "centre is still airborne",
    );
    return simulate(incoming, [], bank);
  };
  assert.equal(hardLanding(airborne, { ...airborne, x: 20 * dt }, flat), null);
  const left = crashOn(1),
    right = crashOn(-1);
  assert.ok(left.minUp < -0.2 && right.minUp < -0.2);
  near(left.maxLateralTilt, -right.maxLateralTilt, 1e-5);
  assert.ok(Math.abs(left.maxLateralTilt) > 0.5);
});

test("recovery clock advances once per passive tick; controls and render batching cannot change the trajectory", () => {
  const result = simulate({ ...car, steering: -1 }, trunk(0.7), flat, 30).body;
  for (const hz of [60, 120]) {
    const other = simulate({ ...car, steering: 1 }, trunk(0.7), flat, hz).body;
    assert.deepEqual(other.position, result.position);
    assert.deepEqual(other.rotation, result.rotation);
  }
  const body = beginCrash(car, "fall", null, () => null);
  for (let i = 0; i < 239; i++) stepCrash(body, () => null);
  near(crashRemaining(body), dt);
  near(body.velocity.x, car.v);
  near(body.velocity.z, -9.81 * 239 * dt);
  stepCrash(body, () => null);
  const end = crashCar(body, car);
  stepCrash(body, () => null);
  assert.deepEqual(crashCar(body, car), end);
  assert.equal(crashRemaining(body), 0);
});

test("overlapping horizontal footprints are separated by height; a descending roof contact points upwards", () => {
  const high = { ...car, x: 5, z: 11 };
  assert.equal(obstacleContact(wall, drivingHull(high)), null);
  const hit = obstacleContact(
    wall,
    drivingHull({ ...high, z: 9.9 }),
    drivingHull(high),
  );
  assert.equal(hit.nz, 1);
  near(hit.depth, 0.1);
});
