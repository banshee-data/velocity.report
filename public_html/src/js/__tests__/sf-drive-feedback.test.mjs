import test from "node:test";
import assert from "node:assert/strict";
import {
  CRASH_SECONDS,
  beginCrash,
  brakeLightsOn,
  crashPose,
  createTrial,
  questSeeker,
  tileAt,
} from "../sf-drive-core.js";

test("a crash spins for two seconds before becoming ready to reset", () => {
  const car = { x: 12, y: 34, heading: 0.3, steering: 0.2 };
  const crash = beginCrash(car, "tree", 10);
  assert.equal(CRASH_SECONDS, 2);
  assert.equal(crash.kind, "tree");
  assert.equal(crashPose(crash, 10).heading, car.heading);
  assert.equal(crashPose(crash, 10).remaining, 2);
  const middle = crashPose(crash, 11);
  assert.ok(middle.heading > car.heading + Math.PI * 2);
  assert.ok(middle.lift > 0);
  assert.equal(middle.done, false);
  assert.equal(crashPose(crash, 11.999).done, false);
  const end = crashPose(crash, 12);
  assert.equal(end.done, true);
  assert.equal(end.remaining, 0);
  assert.ok(Math.abs(end.heading - car.heading - Math.PI * 4) < 1e-10);
  assert.ok(Math.abs(end.lift) < 1e-10);
  assert.deepEqual(
    [crash.x, crash.y],
    [12, 34],
    "spin stays at the impact position",
  );
  assert.ok(
    crashPose(beginCrash({ ...car, steering: -1 }, "building", 0), 1).heading <
      car.heading,
  );
});

test("brake lamps light for the brake pedal, opposing throttle and recovery hold", () => {
  assert.equal(brakeLightsOn({ v: 10 }, { brake: true }), true);
  assert.equal(brakeLightsOn({ v: 10 }, { throttle: -1 }), true);
  assert.equal(brakeLightsOn({ v: -4 }, { throttle: 1 }), true);
  assert.equal(brakeLightsOn({ v: 0 }, {}, true), true);
  assert.equal(brakeLightsOn({ v: 10 }, { throttle: 1 }), false);
  assert.equal(brakeLightsOn({ v: -4 }, { throttle: -1 }), false);
  assert.equal(brakeLightsOn({ v: 10 }, {}), false);
});

const tiles = [
  { t: "a", x: 0, y: 0 },
  { t: "b", x: 10, y: 0 },
];
const targets = [
  { t: "a", x: -80, y: 0, emoji: "🍎" },
  { t: "b", x: 8, y: 0, emoji: "🍒" },
];

test("the seeker prioritises the current tile's emoji over a nearer neighboring target", () => {
  const trial = createTrial(targets),
    car = { x: 4, y: 0, heading: 0 };
  const seeker = questSeeker(trial, tiles, car);
  assert.equal(tileAt(tiles, car.x, car.y).t, "a");
  assert.equal(seeker.target.emoji, "🍎");
  assert.equal(seeker.onTile, true);
  assert.equal(seeker.distance, 84);
  assert.equal(seeker.heading, 90);
  assert.equal(seeker.cardinal, "E");
  assert.equal(
    Math.abs(seeker.relative),
    180,
    "the local emoji is behind the car",
  );
  trial.collected.add("a");
  const next = questSeeker(trial, tiles, car);
  assert.equal(next.target.emoji, "🍒");
  assert.equal(next.localCollected, true);
  assert.equal(next.onTile, false);
  assert.equal(next.relative, 0);
  const crossed = questSeeker(trial, tiles, { ...car, x: 6 });
  assert.equal(crossed.onTile, true);
});

test("compass bearings turn relative to the car and wrap correctly through north", () => {
  const trial = createTrial([{ t: "a", x: 0, y: 100, emoji: "🍎" }]);
  const seek = (heading) => questSeeker(trial, tiles, { x: 0, y: 0, heading });
  assert.equal(seek(Math.PI / 2).cardinal, "N");
  assert.equal(seek(Math.PI / 2).relative, 0);
  assert.equal(seek(0).relative, -90, "north is left when driving east");
  assert.equal(seek(Math.PI).relative, 90, "north is right when driving west");
  assert.equal(seek(-Math.PI / 2).cardinal, "S");
  assert.ok(Math.abs(seek(Math.PI / 2 + 0.01).relative) < 1);
});

test("a tile with no quest seeks the next emoji and a complete rally has no target", () => {
  const trial = createTrial([targets[1]]);
  const noLocal = questSeeker(trial, tiles, { x: 0, y: 0, heading: 0 });
  assert.equal(noLocal.local, null);
  assert.equal(noLocal.onTile, false);
  assert.equal(noLocal.target.t, "b");
  trial.collected.add("b");
  const finished = questSeeker(trial, tiles, { x: 0, y: 0, heading: 0 });
  assert.equal(finished.target, null);
  assert.equal(finished.relative, null);
  assert.equal(finished.distance, null);
});
