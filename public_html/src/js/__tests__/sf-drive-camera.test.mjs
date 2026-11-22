import test from "node:test";
import assert from "node:assert/strict";
import {
  DRIVE_VIEWS,
  DRIVE_VIEW_RANGE,
  MIRROR_VIEW_RANGE,
  driveCameraPose,
  incarRoll,
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
