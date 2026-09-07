// Tests for the drone circuit. Pure arithmetic over time, so no DOM, no
// camera, no GPU — the flight can be flown at a thousand times normal speed
// and checked frame by frame.

import { test, describe } from "node:test";
import assert from "node:assert/strict";

import { createSceneFlight, flightPath, orbitFrom } from "../scene-flight.js";

/** The soma1 shape: four street-level approaches plus two views to skip. */
function sceneVantages() {
  return [
    {
      id: "overview",
      label: "Overview",
      azimuth_deg: 92,
      polar_deg: 67,
      zoom: 0.18,
      fly: false,
    },
    {
      id: "eb-howard",
      label: "Eastbound Howard",
      azimuth_deg: 210,
      polar_deg: 60,
      zoom: 0.25,
    },
    {
      id: "wb-howard",
      label: "Westbound Howard",
      azimuth_deg: 30,
      polar_deg: 60,
      zoom: 0.25,
    },
    {
      id: "nb-5th",
      label: "Northbound 5th",
      azimuth_deg: 120,
      polar_deg: 60,
      zoom: 0.25,
    },
    {
      id: "sb-5th",
      label: "Southbound 5th",
      azimuth_deg: 300,
      polar_deg: 60,
      zoom: 0.25,
    },
    {
      id: "top",
      label: "Overhead",
      azimuth_deg: 30,
      polar_deg: 3,
      zoom: 0.5,
      fly: false,
    },
  ];
}

describe("flight path", () => {
  test("an overhead or establishing view sits the flight out", () => {
    assert.deepEqual(
      flightPath(sceneVantages()).map((v) => v.id),
      ["wb-howard", "nb-5th", "eb-howard", "sb-5th"],
    );
  });

  test("the path is ordered by bearing, so the drone circles", () => {
    const bearings = flightPath(sceneVantages()).map((v) => v.azimuth_deg);
    assert.deepEqual(
      bearings,
      [...bearings].sort((a, b) => a - b),
    );
  });

  test("a vantage joins unless it explicitly opts out", () => {
    const path = flightPath([
      { id: "a", azimuth_deg: 0 },
      { id: "b", azimuth_deg: 90, fly: true },
      { id: "c", azimuth_deg: 180, fly: false },
    ]);
    assert.deepEqual(
      path.map((v) => v.id),
      ["a", "b"],
    );
  });

  test("nothing at all is an empty path, not a crash", () => {
    assert.deepEqual(flightPath(undefined), []);
    assert.deepEqual(flightPath([]), []);
    assert.deepEqual(flightPath([{ id: "x", fly: false }]), []);
  });
});

describe("fitting one orbit to the vantages", () => {
  test("vantages that agree give an orbit that is each of them exactly", () => {
    const orbit = orbitFrom(flightPath(sceneVantages()));
    assert.deepEqual(orbit, {
      polar_deg: 60,
      zoom: 0.25,
      offset_x: 0,
      offset_y: 0,
    });
  });

  test("vantages that differ are averaged, not favoured in turn", () => {
    const orbit = orbitFrom([
      { azimuth_deg: 0, polar_deg: 50, zoom: 0.2, offset_x: -10 },
      { azimuth_deg: 180, polar_deg: 70, zoom: 0.4, offset_x: 10 },
    ]);
    assert.equal(orbit.polar_deg, 60);
    assert.ok(Math.abs(orbit.zoom - 0.3) < 1e-9);
    assert.equal(orbit.offset_x, 0);
  });

  test("no eligible vantages means no orbit", () => {
    assert.equal(orbitFrom([]), null);
  });
});

describe("flying the circuit", () => {
  const setup = () =>
    createSceneFlight({ vantages: sceneVantages(), circuitSeconds: 40 });

  test("one lap is one revolution", () => {
    const f = setup();
    assert.equal(f.cycle, 40);
    assert.equal(f.rate, 9, "360 degrees over 40 seconds");
  });

  test("a lap begins at the first vantage's bearing", () => {
    assert.equal(setup().sample(0).azimuth_deg, 30);
  });

  // The reason for the rewrite: stopping at each vantage meant accelerating
  // and braking four times a lap. Every step must now be the same size.
  test("the bearing advances at a constant rate", () => {
    const f = setup();
    const steps = [];
    for (let t = 0; t < f.cycle; t += 0.25) {
      let step = f.sample(t + 0.25).azimuth_deg - f.sample(t).azimuth_deg;
      if (step < -180) step += 360; // the wrap through 360 is forward motion
      steps.push(step);
    }
    const min = Math.min(...steps);
    const max = Math.max(...steps);
    assert.ok(
      max - min < 1e-9,
      `steps should be identical; spread was ${(max - min).toFixed(6)} degrees`,
    );
    assert.ok(min > 0, "and always forward");
  });

  test("elevation and distance never change during a lap", () => {
    const f = setup();
    for (let t = 0; t < f.cycle; t += 0.5) {
      assert.equal(
        f.sample(t).polar_deg,
        f.orbit.polar_deg,
        `tilt moved at ${t}`,
      );
      assert.equal(f.sample(t).zoom, f.orbit.zoom, `distance moved at ${t}`);
    }
  });

  test("it passes through every vantage's bearing", () => {
    const f = setup();
    for (const v of f.path) {
      const t = f.phaseFor(v.azimuth_deg);
      assert.ok(
        Math.abs(f.sample(t).azimuth_deg - v.azimuth_deg) < 1e-9,
        `${v.id} at ${v.azimuth_deg} was missed`,
      );
    }
  });

  test("a point in transit belongs to no chip", () => {
    assert.equal(setup().sample(5).id, undefined);
  });

  test("the lap repeats and survives a negative clock", () => {
    const f = setup();
    for (const t of [0, 3.5, 17]) {
      assert.ok(
        Math.abs(f.sample(t).azimuth_deg - f.sample(t + f.cycle).azimuth_deg) <
          1e-9,
      );
      assert.ok(
        Math.abs(
          f.sample(t).azimuth_deg - f.sample(t + f.cycle * 5).azimuth_deg,
        ) < 1e-9,
      );
    }
    assert.ok(
      f.sample(-5).azimuth_deg >= 0,
      "a negative clock still yields a bearing",
    );
    assert.ok(
      Math.abs(f.sample(-5).azimuth_deg - f.sample(f.cycle - 5).azimuth_deg) <
        1e-9,
    );
  });

  test("taking off again picks up from the bearing on screen", () => {
    const f = setup();
    for (const bearing of [0, 47, 180, 300, 359]) {
      const resumed = f.sample(f.phaseFor(bearing)).azimuth_deg;
      assert.ok(
        Math.abs(resumed - bearing) < 1e-9,
        `resuming at ${bearing} started at ${resumed}`,
      );
    }
  });

  test("a scene with nothing to fly yields nothing", () => {
    const f = createSceneFlight({
      vantages: [{ id: "only", azimuth_deg: 0, fly: false }],
    });
    assert.equal(f.path.length, 0);
    assert.equal(f.sample(3), null);
  });

  test("a nonsense circuit length falls back rather than dividing by zero", () => {
    const f = createSceneFlight({
      vantages: sceneVantages(),
      circuitSeconds: 0,
    });
    assert.ok(Number.isFinite(f.sample(10).azimuth_deg));
  });
});
