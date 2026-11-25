import test from "node:test";
import assert from "node:assert/strict";
import {
  DRIVE_VIEWS,
  DRIVE_VIEW_RANGE,
  MIRROR_VIEW_RANGE,
  coveredRoadBlend,
  driveCameraPose,
  incarRoll,
  overheadDeckBlend,
} from "../sf-drive-camera.js";

const car = { x: 100, y: 200, heading: 0 };
const near = (a, b) => assert.ok(Math.abs(a - b) < 1e-9, `${a} != ${b}`);

test("the default view is zoomed out and clears uphill terrain behind the car", () => {
  assert.equal(DRIVE_VIEWS[0], "zoomed");
  const pose = driveCameraPose(DRIVE_VIEWS[0], car, 10, () => 35);
  assert.deepEqual(pose.position, [66, 200, 61]);
  assert.deepEqual(pose.look, [114, 200, 11.4]);
  assert.equal(pose.hideCar, false);
  assert.equal(pose.mirror, false);
  assert.equal(driveCameraPose("zoomed", car, 10, () => null).position[2], 36);
});

test("the chase camera lowers before a covered span and rises after it", () => {
  const surface = {
    coveredRange: [44, 74],
    profile: [
      { position: [0, 0, 10], s_m: 0 },
      { position: [44, 0, 10], s_m: 44 },
      { position: [44, 72, 10], s_m: 116 },
    ],
  };
  const probe = (x, y) => coveredRoadBlend(surface, { x, y });
  near(probe(20, 0), 0);
  near(probe(32, -1), 0.5);
  near(probe(43, 15), 1);
  near(probe(43, 42), 1);
  near(probe(43, 76), 0.5);
  near(probe(43, 88), 0);
  near(probe(66, 15), 0);
  near(coveredRoadBlend(null, car), 0);
  near(coveredRoadBlend({ profile: [] }, car), 0);

  const pose = driveCameraPose("zoomed", car, 10, undefined, 0, 0, 1);
  near(pose.position[0], 92.5);
  near(pose.position[2], 13.4);
  near(pose.look[0], 109);
  assert.equal(pose.hideCar, false);
  for (const view of ["overhead", "incar"]) {
    assert.deepEqual(
      driveCameraPose(view, car, 10, undefined, 0, 0, 1),
      driveCameraPose(view, car, 10),
    );
  }
});

test("a deck over a city street lowers only the chase camera beneath it", () => {
  const street = (x) => 10 + (x - car.x) * 0.1;
  const ceiling = (x, y, base) => {
    const deck = x >= 95 && x <= 106 && y >= 195 && y <= 205 ? 12.5 : Infinity;
    return deck >= base - 0.37 ? deck : Infinity;
  };
  const preceding = overheadDeckBlend({ ...car, x: 80 }, 8, street, ceiling);
  assert.ok(preceding > 0 && preceding < 1);
  const blend = overheadDeckBlend(car, 10, street, ceiling);
  assert.equal(blend, 1);
  const pose = driveCameraPose(
    "zoomed",
    { ...car, x: 104 },
    10.4,
    street,
    0,
    0,
    blend,
    ceiling,
  );
  assert.deepEqual(pose.position, [96.5, 200, 12]);
  assert.equal(pose.hideCar, false);
  assert.equal(overheadDeckBlend({ ...car, y: 210 }, 10, street, ceiling), 0);
  assert.equal(overheadDeckBlend(car, 13, street, ceiling), 0);
  assert.equal(overheadDeckBlend({ ...car, x: 145 }, 14.5, street, ceiling), 0);
});

test("stacked ramp probes lower the camera only on the supported lower level", () => {
  const lowRamp = (x) => (x >= 85 && x <= 112 ? 22 - (x - 100) * 0.1 : null);
  const highRamp = (x) => (x >= 91 && x <= 108 ? 29 : null);
  const indexCeiling = (x, y, base) =>
    Math.min(
      ...[lowRamp(x), highRamp(x)].filter((z) => z !== null && z >= base),
    );
  assert.equal(overheadDeckBlend(car, 22, lowRamp, indexCeiling), 1);
  assert.equal(overheadDeckBlend(car, 29, highRamp, indexCeiling), 0);
  assert.equal(
    overheadDeckBlend({ ...car, x: 115 }, 20.5, () => null, indexCeiling),
    0,
  );
});

test("a downhill bridge departure keeps its travelled surface behind the camera", () => {
  const street = () => 10;
  const departedDeck = (x) => (x >= 68 && x <= 96 ? 14 : null);
  const decks = (x, y, base) =>
    Math.min(...[departedDeck(x)].filter((z) => z !== null && z >= base));
  assert.equal(overheadDeckBlend(car, 10, street, decks), 1);
  assert.equal(overheadDeckBlend(car, 10, street, decks, departedDeck), 0);

  const stacked = (x, y, base) =>
    Math.min(
      decks(x, y, base),
      x >= 90 && x <= 96 && base <= 19 ? 19 : Infinity,
    );
  assert.equal(overheadDeckBlend(car, 10, street, stacked, departedDeck), 1);
});

test("overhead looks straight down and keeps forward up in every quadrant", () => {
  for (const heading of [0, Math.PI / 2, Math.PI, -Math.PI / 2]) {
    const pose = driveCameraPose("overhead", { ...car, heading }, 10);
    assert.deepEqual(pose.position, [100, 200, 95]);
    assert.deepEqual(pose.look, [100, 200, 10]);
    near(pose.up[0], Math.cos(heading));
    near(pose.up[1], Math.sin(heading));
    assert.equal(pose.up[2], 0);
    assert.equal(pose.attached, true);
  }
});

test("in-car view stays at driver eye height and follows the road slope", () => {
  const pose = driveCameraPose("incar", car, 10, (x) => 10 + (x - car.x) * 0.2);
  near(pose.position[0], 100.35);
  near(pose.position[2], 11.52);
  near(pose.look[2], 17.45);
  assert.equal(pose.attached, true);
  assert.equal(pose.hideCar, true);
  const north = driveCameraPose("incar", { ...car, heading: Math.PI / 2 }, 10);
  near(north.look[0], car.x);
  near(north.look[1], car.y + 30);
});

test("missing terrain keeps the in-car view level and extreme samples limit pitch", () => {
  const flat = driveCameraPose("incar", car, 10, () => null);
  assert.equal(flat.look[2], flat.position[2]);
  const steep = driveCameraPose("incar", car, 10, (x) =>
    x > 100 ? 1000 : -1000,
  );
  near((steep.look[2] - steep.position[2]) / (30 - 0.35), 0.4);
});

test("only in-car mode permits a mirror and hides the opaque car body", () => {
  for (const view of DRIVE_VIEWS) {
    const pose = driveCameraPose(view, car, 0);
    assert.equal(pose.mirror, view === "incar");
    assert.equal(pose.hideCar, view === "incar");
  }
});

test("in-car roll builds with lateral acceleration and returns smoothly to level", () => {
  const slow = incarRoll(0, 1, 0.1);
  const fast = incarRoll(0, 7.5, 0.1);
  assert.ok(fast > slow && slow > 0);
  assert.ok(Math.abs(incarRoll(0, 20, 2)) < (6 * Math.PI) / 180);
  assert.ok(incarRoll(fast, 0, 0.1) < fast);
  near(incarRoll(incarRoll(0, 7.5, 0.05), 7.5, 0.05), fast);
  near(incarRoll(0, -7.5, 0.1), -fast);
  const tilted = driveCameraPose("incar", car, 10, undefined, fast);
  assert.ok(tilted.up[1] < 0, "left turn leans right");
  assert.deepEqual(tilted.look, driveCameraPose("incar", car, 10).look);
});

test("both camera views finish their distance fade before clipping", () => {
  for (const range of [DRIVE_VIEW_RANGE, MIRROR_VIEW_RANGE]) {
    assert.ok(range.near < range.fogNear);
    assert.ok(range.fogNear < range.fogFar);
    assert.ok(range.fogFar < range.far && range.far < 400);
  }
  const overhead = driveCameraPose("overhead", car, 10);
  assert.ok(DRIVE_VIEW_RANGE.fogNear > overhead.position[2] - 10);
  assert.ok(MIRROR_VIEW_RANGE.far < DRIVE_VIEW_RANGE.far);
});
