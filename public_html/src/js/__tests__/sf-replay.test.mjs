import assert from "node:assert/strict";
import test from "node:test";
import { Quaternion, Vector3 } from "../../vendor/three/three.core.js";
import {
  crashHistory,
  encodeReplay,
  replayLibrary,
  replayPose,
  CRASH_HOLD_TICKS,
} from "../sf-replay.js";
import { createFlightTimeline } from "../sf-airtime.js";

const pose = (x, angle = 0) => ({
  x,
  y: Math.sin(x / 4),
  z: Math.cos(x / 7),
  heading: angle,
  q: new Quaternion().setFromAxisAngle(new Vector3(1, 0, 0), angle).toArray(),
});
function storageFixture() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
}

test("crash lead-in retains two seconds of ticks across ring wrap and clears on restart", () => {
  const history = crashHistory();
  for (let i = 0; i < 600; i++) history.push(i);
  assert.equal(history.read().length, 240);
  assert.equal(history.read()[0], 360);
  assert.equal(history.read().at(-1), 599);
  history.clear();
  history.push(2);
  assert.deepEqual(history.read(), [2]);
});

test("quantised keyframes preserve the impact cut, tumbling and a two-second stationary tail", () => {
  const poses = Array.from({ length: 510 }, (_, t) =>
    pose(t / 20, t < 240 ? t / 300 : (t - 240) / 12),
  );
  // Force a step at impact, as for position correction against a rail.
  poses[240] = pose(33, -1);
  const clip = encodeReplay(poses, 240, CRASH_HOLD_TICKS);
  assert.ok(clip.frames.length < poses.length / 4);
  assert.equal(clip.end, 509 + 240);
  assert.ok(Math.abs(replayPose(clip, 239.9).x - poses[239].x) < 0.01);
  assert.equal(replayPose(clip, 240).x, 33);
  for (const [tick, p] of poses.entries()) {
    const rendered = replayPose(clip, tick);
    assert.ok(
      Math.hypot(rendered.x - p.x, rendered.y - p.y, rendered.z - p.z) < 0.04,
    );
    const a = new Quaternion().fromArray(p.q),
      b = new Quaternion().fromArray(rendered.q);
    assert.ok(a.angleTo(b) < 0.018);
  }
  assert.deepEqual(replayPose(clip, 509), replayPose(clip, 749));
});

test("reload retains the most recent eight crashes and eight longest airtimes across contexts", () => {
  const storage = storageFixture();
  const lib = replayLibrary(storage);
  for (let i = 0; i < 12; i++) {
    assert.ok(
      lib.save({
        id: `c${i}`,
        created: i,
        mode: "survey",
        kind: "tree",
        outcome: "stopped",
        ...encodeReplay([pose(0), pose(1)], 1, 240),
      }),
    );
    assert.ok(
      lib.save({
        id: `a${i}`,
        created: i,
        mode: i % 2 ? "practice" : "city",
        seconds: 12 - i,
        peak: 3,
        ...encodeReplay([pose(0), pose(1)]),
      }),
    );
  }
  const { crashes, airtimes } = replayLibrary(storage).read();
  assert.deepEqual(
    crashes.map((x) => x.id),
    ["c11", "c10", "c9", "c8", "c7", "c6", "c5", "c4"],
  );
  assert.deepEqual(
    airtimes.map((x) => x.id),
    ["a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7"],
  );
  assert.equal(airtimes[1].mode, "practice");
  assert.equal(replayPose(airtimes[1], 1).x, 1);
});

test("invalid saved frames are ignored and denied storage keeps the session playable", () => {
  const storage = storageFixture();
  storage.setItem(
    "sf-replays-v1",
    JSON.stringify({ crashes: [{ id: "bad", frames: [] }] }),
  );
  assert.deepEqual(replayLibrary(storage).read(), {
    crashes: [],
    airtimes: [],
  });
  const lib = replayLibrary({
    ...storage,
    setItem() {
      throw new Error("quota");
    },
  });
  assert.equal(
    lib.save({
      id: "ok",
      created: 1,
      mode: "city",
      seconds: 1,
      peak: 2,
      ...encodeReplay([pose(0), pose(1)]),
    }),
    false,
  );
  assert.equal(lib.read().airtimes[0].id, "ok");
});

test("playable airtime clips preserve an impact outcome across reload", () => {
  let saved = [];
  const timeline = createFlightTimeline((landing, poses) =>
    saved.push({ landing, poses }),
  );
  const start = { ...pose(0), grounded: true, v: 25 };
  const flight = { ...pose(1), z: 2, grounded: false, v: 25 };
  const end = { ...pose(2), z: 0, grounded: true, v: 25 };
  const env = { pose: (car) => car, heightAt: () => 0 };
  timeline.step(start, flight, 1 / 120, 1, env);
  timeline.step(flight, end, 1 / 120, 2, env);
  assert.deepEqual(saved[0].poses, [start, flight, end]);
  timeline.step(start, flight, 1 / 120, 3, env);
  timeline.step(flight, end, 1 / 120, 4, { ...env, impact: "ground" });
  const storage = storageFixture();
  const lib = replayLibrary(storage);
  for (const [index, { landing, poses }] of saved.entries())
    lib.save({
      ...encodeReplay(poses),
      ...landing,
      id: `flight-${index}`,
      created: index,
      mode: "city",
    });
  const { airtimes, crashes } = replayLibrary(storage).read();
  assert.equal(crashes.length, 0);
  assert.equal(airtimes.length, 2);
  const hard = airtimes.find((clip) => clip.ending === "impact");
  assert.equal(hard.impact, null);
  assert.equal(hard.end, 2);
  assert.equal(replayPose(hard, 2).x, end.x);
});
