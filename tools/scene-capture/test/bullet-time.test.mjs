import { test, describe } from "node:test";
import assert from "node:assert/strict";

import {
  interpolateElevation,
  resolveBulletTimeTarget,
  expandBulletTime,
} from "../bullet-time.mjs";
import { sphericalFromEnu } from "../../../public_html/src/js/scene-coords.js";

const DEFAULT_KEYFRAMES = [
  { t: 0, deg: 20 },
  { t: 1 / 3, deg: 30 },
  { t: 2 / 3, deg: 20 },
  { t: 1, deg: 10 },
];

describe("interpolateElevation", () => {
  test("matches every keyframe exactly at its own t", () => {
    for (const kf of DEFAULT_KEYFRAMES) {
      assert.ok(
        Math.abs(interpolateElevation(DEFAULT_KEYFRAMES, kf.t) - kf.deg) < 1e-9,
        `t=${kf.t}`,
      );
    }
  });

  test("clamps outside [0, 1] to the end keyframes", () => {
    assert.equal(interpolateElevation(DEFAULT_KEYFRAMES, -1), 20);
    assert.equal(interpolateElevation(DEFAULT_KEYFRAMES, 2), 10);
  });

  test("has zero slope at an interior keyframe boundary (no overshoot)", () => {
    // A smoothstepped segment's derivative is zero at both its own
    // endpoints; sampling symmetrically either side of a shared boundary
    // should land almost exactly on the boundary's own value, not swing past
    // it the way a spline fitted through all keyframes could.
    const boundary = 1 / 3;
    const eps = 1e-4;
    const before = interpolateElevation(DEFAULT_KEYFRAMES, boundary - eps);
    const at = interpolateElevation(DEFAULT_KEYFRAMES, boundary);
    const after = interpolateElevation(DEFAULT_KEYFRAMES, boundary + eps);
    assert.ok(Math.abs(before - at) < 1e-3, `before=${before} at=${at}`);
    assert.ok(Math.abs(after - at) < 1e-3, `after=${after} at=${at}`);
  });

  test("stays within the bounds of its surrounding keyframes on a rising segment", () => {
    for (let i = 0; i <= 10; i++) {
      const t = (i / 10) * (1 / 3); // within the first segment: 20 -> 30
      const v = interpolateElevation(DEFAULT_KEYFRAMES, t);
      assert.ok(v >= 20 - 1e-9 && v <= 30 + 1e-9, `t=${t} v=${v}`);
    }
  });
});

describe("resolveBulletTimeTarget", () => {
  test("passes an explicit point straight through", () => {
    const point = { x: 1, y: 2, z: 3 };
    assert.deepEqual(resolveBulletTimeTarget({ point }, []), point);
  });

  test("resolves an object id against the given tracks", () => {
    const tracks = [
      { id: "0", x: 1, y: 0, z: 0 },
      { id: "3", x: 5, y: 6, z: 0.5 },
    ];
    assert.deepEqual(resolveBulletTimeTarget({ objectId: "3" }, tracks), {
      x: 5,
      y: 6,
      z: 0.5,
    });
  });

  test("throws when the object id is not in this frame", () => {
    assert.throws(
      () =>
        resolveBulletTimeTarget({ objectId: "missing" }, [
          { id: "0", x: 0, y: 0, z: 0 },
        ]),
      /not found/,
    );
  });
});

describe("expandBulletTime", () => {
  const target = { x: 0, y: 0, z: 0 };

  function spec(overrides = {}) {
    return {
      radiusM: null,
      imageCount: 25,
      arcStartDeg: -90,
      arcEndDeg: 90,
      elevationKeyframes: DEFAULT_KEYFRAMES,
      ...overrides,
    };
  }

  test("produces exactly imageCount named views", () => {
    const views = expandBulletTime(
      spec({ imageCount: 5 }),
      { camera: { x: 10, y: 0, z: 5 }, fovDeg: null },
      target,
    );
    assert.equal(views.length, 5);
    assert.deepEqual(
      views.map((v) => v.name),
      [
        "bullet-time-000",
        "bullet-time-001",
        "bullet-time-002",
        "bullet-time-003",
        "bullet-time-004",
      ],
    );
  });

  test("every view targets the fixed point and carries the base view's fov", () => {
    const views = expandBulletTime(
      spec({ imageCount: 3 }),
      { camera: { x: 10, y: 0, z: 5 }, fovDeg: 45 },
      target,
    );
    for (const v of views) {
      assert.deepEqual(v.target, target);
      assert.equal(v.fovDeg, 45);
    }
  });

  test("azimuth advances linearly and includes both arc endpoints", () => {
    const baseView = { camera: { x: 10, y: 0, z: 5 }, fovDeg: null };
    const baseAzimuth = sphericalFromEnu({
      camera: baseView.camera,
      target,
    }).azimuthDeg;
    const views = expandBulletTime(spec({ imageCount: 5 }), baseView, target);

    const azimuths = views.map(
      (v) => sphericalFromEnu({ camera: v.camera, target }).azimuthDeg,
    );
    const wrap = (d) => ((d % 360) + 360) % 360;

    assert.ok(Math.abs(wrap(azimuths[0]) - wrap(baseAzimuth - 90)) < 1e-6);
    assert.ok(Math.abs(wrap(azimuths[4]) - wrap(baseAzimuth + 90)) < 1e-6);

    // Linear: equal steps in t produce equal steps in azimuth.
    const step0 = wrap(azimuths[1] - azimuths[0]);
    const step1 = wrap(azimuths[2] - azimuths[1]);
    assert.ok(Math.abs(step0 - step1) < 1e-6, `${step0} vs ${step1}`);
  });

  test("derives radius from the base view's own distance to target when radiusM is omitted", () => {
    const baseView = { camera: { x: 0, y: -10, z: 0 }, fovDeg: null }; // distance 10
    const views = expandBulletTime(spec({ imageCount: 2 }), baseView, target);
    for (const v of views) {
      const r = sphericalFromEnu({ camera: v.camera, target }).radius;
      assert.ok(Math.abs(r - 10) < 1e-6, `radius ${r}`);
    }
  });

  test("an explicit radiusM overrides the base view's own distance", () => {
    const baseView = { camera: { x: 0, y: -10, z: 0 }, fovDeg: null };
    const views = expandBulletTime(
      spec({ imageCount: 2, radiusM: 25 }),
      baseView,
      target,
    );
    for (const v of views) {
      const r = sphericalFromEnu({ camera: v.camera, target }).radius;
      assert.ok(Math.abs(r - 25) < 1e-6, `radius ${r}`);
    }
  });

  test("elevation at each sample matches interpolateElevation at the same t", () => {
    const baseView = { camera: { x: 10, y: 0, z: 5 }, fovDeg: null };
    const views = expandBulletTime(spec({ imageCount: 4 }), baseView, target);
    views.forEach((v, i) => {
      const t = i / (views.length - 1);
      const expected = interpolateElevation(DEFAULT_KEYFRAMES, t);
      const actual = sphericalFromEnu({
        camera: v.camera,
        target,
      }).elevationDeg;
      assert.ok(
        Math.abs(actual - expected) < 1e-6,
        `i=${i} expected=${expected} actual=${actual}`,
      );
    });
  });
});
