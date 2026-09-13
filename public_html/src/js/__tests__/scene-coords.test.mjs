// Tests for the shared coordinate math used by the scene player and the
// deterministic capture tooling. Pure arithmetic, no DOM, no three.js.

import { test, describe } from "node:test";
import assert from "node:assert/strict";

import {
  toSceneX,
  toSceneY,
  toSceneZ,
  enuFromSpherical,
  sphericalFromEnu,
  isDegenerateOrientation,
} from "../scene-coords.js";

describe("ENU -> scene transform", () => {
  test("east stays x, up becomes y, north becomes -z", () => {
    assert.equal(toSceneX(5), 5);
    assert.equal(toSceneY(3), 3);
    assert.equal(toSceneZ(7), -7);
  });
});

describe("enuFromSpherical / sphericalFromEnu", () => {
  const ORIGIN = { x: 0, y: 0, z: 0 };

  test("azimuth 0, elevation 0 places the camera due south of the target", () => {
    // Re-derived from scene-camera.js's apply(): at azimuth 0 the camera sits
    // south of the target looking north, not "at" true north.
    const p = enuFromSpherical({
      azimuthDeg: 0,
      elevationDeg: 0,
      radius: 10,
      target: ORIGIN,
    });
    assert.ok(Math.abs(p.x - 0) < 1e-9, `x ${p.x}`);
    assert.ok(Math.abs(p.y - -10) < 1e-9, `y ${p.y}`);
    assert.ok(Math.abs(p.z - 0) < 1e-9, `z ${p.z}`);
  });

  test("elevation 90 places the camera directly above the target", () => {
    const p = enuFromSpherical({
      azimuthDeg: 123,
      elevationDeg: 90,
      radius: 5,
      target: ORIGIN,
    });
    assert.ok(Math.abs(p.x) < 1e-9);
    assert.ok(Math.abs(p.y) < 1e-9);
    assert.ok(Math.abs(p.z - 5) < 1e-9);
  });

  test("round-trips azimuth, elevation and radius through both directions", () => {
    const target = { x: 12, y: -4, z: 1.5 };
    const cases = [
      { azimuthDeg: 0, elevationDeg: 0, radius: 8 },
      { azimuthDeg: 90, elevationDeg: 20, radius: 15 },
      { azimuthDeg: 180, elevationDeg: 45, radius: 3 },
      { azimuthDeg: 270, elevationDeg: 89, radius: 40 },
      { azimuthDeg: 359, elevationDeg: -10, radius: 22 },
    ];
    for (const c of cases) {
      const camera = enuFromSpherical({ ...c, target });
      const back = sphericalFromEnu({ camera, target });
      assert.ok(
        Math.abs(back.azimuthDeg - c.azimuthDeg) < 1e-6,
        `azimuth ${back.azimuthDeg} vs ${c.azimuthDeg}`,
      );
      assert.ok(
        Math.abs(back.elevationDeg - c.elevationDeg) < 1e-6,
        `elevation ${back.elevationDeg} vs ${c.elevationDeg}`,
      );
      assert.ok(
        Math.abs(back.radius - c.radius) < 1e-6,
        `radius ${back.radius} vs ${c.radius}`,
      );
    }
  });

  test("sphericalFromEnu reports zero radius for a coincident camera and target", () => {
    const r = sphericalFromEnu({
      camera: { x: 1, y: 2, z: 3 },
      target: { x: 1, y: 2, z: 3 },
    });
    assert.equal(r.radius, 0);
  });
});

describe("isDegenerateOrientation", () => {
  const target = { x: 0, y: 0, z: 0 };

  test("true when looking straight down or straight up", () => {
    assert.equal(isDegenerateOrientation({ x: 0, y: 0, z: 10 }, target), true);
    assert.equal(isDegenerateOrientation({ x: 0, y: 0, z: -10 }, target), true);
  });

  test("false one degree off vertical", () => {
    const p = enuFromSpherical({
      azimuthDeg: 0,
      elevationDeg: 89,
      radius: 10,
      target,
    });
    assert.equal(isDegenerateOrientation(p, target), false);
  });

  test("true for a coincident camera and target", () => {
    assert.equal(isDegenerateOrientation(target, target), true);
  });
});
