import assert from "node:assert/strict";
import test from "node:test";
import { encodeReplay } from "../sf-replay.js";
import {
  advanceReplay,
  replayRate,
  replayLandmark,
  replayTelemetry,
} from "../sf-replay-telemetry.js";
import { formatInsuranceDollars } from "../sf-insurance.js";

test("an impact slows from 300 ms before to 300 ms after, independent of render batching", () => {
  const clip = { impact: 240, end: 720, outcome: "stopped" };
  assert.equal(replayRate(clip, 0), 0.99);
  assert.equal(replayRate(clip, 203.99), 0.99);
  assert.equal(replayRate(clip, 204), 0.5);
  assert.equal(replayRate(clip, 275.99), 0.5);
  assert.equal(replayRate(clip, 276), 0.99);
  assert.equal(advanceReplay(clip, 204, 0.6), 240);
  assert.equal(advanceReplay(clip, 240, 0.6), 276);
  assert.equal(advanceReplay(clip, 480, 2), 720);
  assert.ok(advanceReplay(clip, 480, 1.99) < 720);
  const bulk = advanceReplay(clip, 0, 3);
  let tick = 0;
  for (let n = 0; n < 180; n++) tick = advanceReplay(clip, tick, 1 / 60);
  assert.ok(Math.abs(bulk - tick) < 1e-8);
  assert.equal(advanceReplay({ impact: null, end: 360 }, 0, 1), 118.8);
  assert.equal(replayRate({ ...clip, impact: 10 }, 0), 0.5);
  const hardLanding = { impact: null, end: 360, ending: "impact" };
  assert.equal(replayRate(hardLanding, 323), 0.99);
  assert.equal(replayRate(hardLanding, 324), 0.5);
  assert.equal(advanceReplay(hardLanding, 324, 0.6), 360);
});

test("a one-tick tree impact remains visible in aggregated deceleration and jerk", () => {
  const poses = Array.from({ length: 37 }, (_, tick) => ({
    velocity: [tick < 13 ? 25 : 0, 0, 0],
  }));
  const points = replayTelemetry(poses, 13, [25, 0, 0]);
  assert.deepEqual(points, [
    [56, 0, 0],
    [56, 3000, 360000],
    [0, 0, 0],
    [0, 0, 0],
  ]);
  // A contact that redirects velocity at the same speed still changes acceleration.
  assert.deepEqual(replayTelemetry([{ velocity: [0, 20, 0] }], 0, [20, 0, 0]), [
    [45, 0, 407294],
  ]);
  assert.equal(replayTelemetry([{}]), null);
  assert.deepEqual(
    replayTelemetry([0, 1, 2].map((value) => ({ velocity: [value, 0, 0] }))),
    [[4, 0, 0]],
    "the first measured acceleration has no earlier sample for comparison",
  );
});

test("crash labels locate the impact and airtime labels locate the take-off", () => {
  const pose = (x) => ({ x, y: 0, z: 0, q: [0, 0, 0, 1], heading: 0 });
  const poses = [pose(0), pose(100), pose(30)];
  const markers = [
    { x: -4, y: 3, emoji: "🍎", names: ["First"] },
    { x: 103, y: -4, emoji: "🌉", names: ["Bridge"] },
  ];
  assert.equal(
    replayLandmark(encodeReplay(poses), markers),
    "🍎 First · 5 m NW",
  );
  assert.equal(
    replayLandmark(encodeReplay(poses, 1), markers),
    "🌉 Bridge · 5 m SE",
  );
  assert.equal(
    replayLandmark(encodeReplay([pose(-4)]), [{ ...markers[0], y: 0 }]),
    "🍎 First · 0 m",
  );
});

test("meter dollars retain the precision of the cent-based ledger", () => {
  assert.equal(formatInsuranceDollars(0), "$0.0000");
  assert.equal(formatInsuranceDollars(0.12), "$0.0012");
  assert.equal(formatInsuranceDollars(0.06), "$0.0006");
  assert.equal(formatInsuranceDollars(100), "$1.0000");
});
