import test from "node:test";
import assert from "node:assert/strict";
import {
  SOLO,
  MPH,
  soloHeight,
  soloSurface,
  soloCar,
  contactStep,
  createSoloRun,
  startSoloRun,
  endSoloRun,
  accountSolo,
  soloSample,
  speedBand,
} from "../sf-solo.js";
const step = 1 / 120;
const near = (a, b) => assert.ok(Math.abs(a - b) < 1e-8, `${a} != ${b}`);
const limit = { mph: 20, source: "authored test" };
const sample = (mph, extra = {}) => ({
  mph,
  limit,
  drivable: true,
  clearOfCurb: true,
  ...extra,
});
const run = () => {
  const r = createSoloRun();
  startSoloRun(r);
  return r;
};

test("exact excess boundaries, changed limit, missing provenance and coverage reconcile", () => {
  const r = run();
  for (const mph of [20, 20.001, 24.999, 25, 29.999, 30])
    accountSolo(r, 1, sample(mph));
  accountSolo(r, 2, sample(100, { limit: null }));
  accountSolo(r, 1, sample(100, { limit: { mph: 20 } }));
  accountSolo(
    r,
    1,
    sample(30, { limit: { mph: 30, source: "changed fixture" } }),
  );
  accountSolo(r, 3, { excluded: true });
  assert.deepEqual(r.bands, [2, 2, 1]);
  assert.equal(r.peak, 10);
  assert.equal(r.known, 7);
  assert.equal(r.unknown, 3);
  assert.equal(r.excluded, 3);
  assert.equal(r.elapsed, 13);
  assert.equal(r.compliant, 2);
  near(r.known + r.unknown, r.evaluated);
  near(r.evaluated + r.excluded, r.elapsed);
  assert.deepEqual([0, 4.999, 5, 9.999, 10].map(speedBand), [-1, 0, 1, 1, 2]);
});

test("one excursion remains continuous, separate contacts retain evidence with one manoeuvre", () => {
  const r = run();
  accountSolo(r, step, sample(22, { curbContact: true, clearOfCurb: false }));
  for (let i = 0; i < 120; i++)
    accountSolo(
      r,
      step,
      sample(22, { drivable: false, curbContact: true, clearOfCurb: false }),
    );
  accountSolo(
    r,
    step,
    sample(22, { drivable: false, curbContact: false, clearOfCurb: false }),
  );
  accountSolo(
    r,
    step,
    sample(22, { drivable: false, curbContact: true, clearOfCurb: false }),
  );
  assert.equal(r.strikes.length, 2);
  assert.equal(r.excursions.length, 1);
  assert.equal(r.incidents.length, 1);
  assert.equal(r.strikes[1].incident, r.excursions[0].incident);
  accountSolo(r, step, sample(22));
  accountSolo(r, step, sample(22, { drivable: false, clearOfCurb: false }));
  assert.equal(r.excursions.length, 2);
  assert.equal(r.incidents.length, 2);
  endSoloRun(r, "aborted");
  assert.equal(r.excursions[1].endReason, "aborted");
  const elapsed = r.elapsed;
  accountSolo(r, 1, sample(22));
  assert.equal(r.elapsed, elapsed);
});

test("airborne over road is not off-road or a kerb strike; swept kerb crossing is recorded", () => {
  const r = run(),
    a = { ...soloCar(), x: 0, z: soloHeight(0) + 2, grounded: false, v: 20 };
  accountSolo(r, step, soloSample(a, a));
  assert.equal(r.excursions.length, 0);
  assert.equal(r.strikes.length, 0);
  const b = { ...soloCar(), y: 4.5, v: 10 };
  const c = { ...b, y: 7 };
  assert.equal(soloSample(b, c).curbContact, true);
  accountSolo(r, step, soloSample(b, c));
  assert.equal(r.excursions.length, 1);
  assert.equal(r.strikes.length, 1);
  assert.equal(r.incidents.length, 1);
});

test("analytic crest: slow pass remains grounded, fast pass releases and lands", () => {
  function pass(v) {
    let car = {
      ...soloCar(),
      x: -25,
      z: soloHeight(-25),
      v,
      vz: (v * (soloHeight(-24.99) - soloHeight(-25.01))) / 0.02,
    };
    let air = 0,
      landed = false;
    for (let i = 0; i < 5000 && car.x < 60; i++) {
      const old = car;
      car = contactStep(
        car,
        { throttle: v < 10 ? 0.12 : 0 },
        step,
        soloSurface,
      );
      if (!car.grounded) air++;
      if (!old.grounded && car.grounded) landed = true;
    }
    return { air, landed, car };
  }
  const slow = pass(8);
  assert.equal(slow.air, 0);
  const fast = pass(30);
  assert.ok(fast.air > 0);
  assert.equal(fast.landed, true);
});

test("airborne motion carries existing momentum, ignores all tyre controls, then regains steering", () => {
  const a = {
    ...soloCar(),
    x: 0,
    z: 10,
    v: 20,
    vx: 20,
    vy: 0,
    vz: 0,
    grounded: false,
  };
  const neutral = contactStep(a, {}, step, soloSurface);
  const steered = contactStep(
    a,
    { throttle: -1, brake: 1, steer: 1 },
    step,
    soloSurface,
  );
  assert.deepEqual(neutral, steered);
  near(steered.z, 10 - 0.5 * 9.81 * step * step);
  near(steered.x, 20 * step);
  const flat = () => ({ height: 0, drivable: true });
  let car = a;
  for (let i = 0; i < 300 && !car.grounded; i++)
    car = contactStep(car, {}, step, flat);
  assert.equal(car.grounded, true);
  const turned = contactStep(car, { steer: 1 }, step, flat);
  assert.notEqual(turned.heading, car.heading);
  const blocked = contactStep(a, {}, step, () => ({ height: null }));
  assert.equal(blocked.blocked, true);
  assert.equal(blocked.x, a.x);
});

test("fixed-tick replay is identical across 30/60/120 Hz render batching; faster elapsed gives no score", () => {
  function replay(hz) {
    let car = soloCar();
    const r = run();
    for (let frame = 0; frame < hz * 8; frame++)
      for (let tick = 0; tick < 120 / hz; tick++) {
        const before = car;
        car = contactStep(car, { throttle: 1 }, step, soloSurface);
        accountSolo(r, step, soloSample(before, car));
      }
    return { car, r };
  }
  assert.deepEqual(replay(30), replay(120));
  assert.deepEqual(replay(60), replay(120));
  assert.equal("score" in replay(30).r, false);
  for (const hz of [30, 60, 120]) {
    const r = run();
    for (let i = 0; i < hz * 4; i++) accountSolo(r, 1 / hz, sample(25));
    near(r.bands[1], 4);
  }
});

test("fixture limit is labelled, unknown final section stays unknown, reset is fresh", () => {
  assert.match(soloSurface(0, 0).limit.source, /Authored/);
  assert.equal(soloSurface(100, 0).limit, null);
  assert.equal(soloSurface(0, 9).drivable, false);
  assert.equal(soloSurface(0, 0).drivable, true);
  const r = run();
  accountSolo(r, 2, sample(30));
  endSoloRun(r);
  const fresh = createSoloRun();
  assert.equal(fresh.elapsed, 0);
  assert.equal(fresh.status, "ready");
  assert.equal(fresh.fixture, SOLO.id);
  near((20 / MPH) * MPH, 20);
});

test("controlled and rushed full-course passes retain evidence without a faster-time reward", () => {
  function pass(target) {
    let car = soloCar();
    const r = run();
    let airborne = 0;
    for (let tick = 0; tick < 120 * 90; tick++) {
      const before = car;
      const input =
        !Number.isFinite(target) || car.v * MPH < target
          ? { throttle: 1 }
          : { brake: 0.3 };
      car = contactStep(car, input, step, soloSurface);
      accountSolo(r, step, soloSample(before, car));
      if (!car.grounded) airborne++;
      if (car.x >= SOLO.end && car.grounded) {
        endSoloRun(r);
        break;
      }
    }
    assert.equal(r.status, "finished");
    return { r, airborne };
  }
  const controlled = pass(19),
    rushed = pass(Infinity);
  assert.deepEqual(controlled.r.bands, [0, 0, 0]);
  assert.equal(controlled.airborne, 0);
  assert.ok(rushed.airborne > 0);
  assert.ok(rushed.r.bands[2] > 7);
  assert.ok(rushed.r.elapsed < controlled.r.elapsed);
  for (const { r } of [controlled, rushed]) {
    near(r.known + r.unknown, r.evaluated);
    assert.ok(r.unknown > 0);
    assert.equal(r.incidents.length, 0);
    assert.equal("score" in r, false);
  }
});

test("crest support releases at gravity, not at half gravity from a backward secant", () => {
  let car = {
    ...soloCar(),
    x: -4,
    z: soloHeight(-4),
    v: 20,
    vz: (20 * (soloHeight(-3.9) - soloHeight(-4.1))) / 0.2,
  };
  for (let i = 0; i < 80; i++) {
    car = contactStep(car, {}, step, soloSurface);
    assert.equal(
      car.grounded,
      true,
      "20 m/s requires less than gravity at the 63 m crest",
    );
  }
});

test("rising road can restore contact while the car still has upward velocity", () => {
  const car = {
    ...soloCar(),
    x: 0,
    z: 0.1,
    v: 20,
    vx: 20,
    vy: 0,
    vz: 1,
    grounded: false,
  };
  const next = contactStep(car, { steer: 1, brake: 1 }, step, (x) => ({
    height: x,
    drivable: true,
  }));
  assert.equal(next.grounded, true);
  near(next.z, next.x);
  near(next.heading, car.heading);
});
