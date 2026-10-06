import assert from "node:assert/strict";
import test from "node:test";
import * as THREE from "three";
import { createSceneCamera } from "../scene-camera.js";
import {
  compassPose,
  northOverhead,
  ISOMETRIC_POLAR_DEG,
} from "../scene-compass.js";

test("compass tilt follows the camera but never exceeds isometric", () => {
  assert.equal(compassPose({ azimuth_deg: 0, polar_deg: 0 }, 0).scaleY, 1);
  assert.ok(
    Math.abs(
      compassPose({ azimuth_deg: 0, polar_deg: 30 }, 0).scaleY -
        Math.cos(Math.PI / 6),
    ) < 1e-12,
  );
  assert.ok(Math.abs(ISOMETRIC_POLAR_DEG - 54.735610317) < 1e-8);
  assert.ok(
    Math.abs(
      compassPose({ azimuth_deg: 0, polar_deg: 89 }, 0).scaleY -
        1 / Math.sqrt(3),
    ) < 1e-12,
  );
});

test("compass bearing matches actual camera projection in each quadrant", () => {
  const camera = new THREE.PerspectiveCamera(52, 1, 0.1, 1000);
  const element = {
    style: {},
    addEventListener() {},
    removeEventListener() {},
  };
  const control = createSceneCamera({ camera, element, THREE });
  control.frame({ centerX: 0, centerZ: 0, groundY: 0, span: 100 });
  for (const north of [0, 90, 217, 359]) {
    for (const azimuth of [0, 90, 180, 270]) {
      const view = { azimuth_deg: azimuth, polar_deg: 40, zoom: 1 };
      control.applyVantage(view);
      camera.updateMatrixWorld();
      const bearing = (-north * Math.PI) / 180;
      const actual = new THREE.Vector3(
        Math.sin(bearing),
        0,
        Math.cos(bearing),
      ).project(camera);
      const pose = compassPose(view, north);
      const rotation = (pose.rotation * Math.PI) / 180;
      const expected = new THREE.Vector2(
        Math.sin(rotation),
        Math.cos(rotation) * pose.scaleY,
      ).normalize();
      const projected = new THREE.Vector2(actual.x, actual.y).normalize();
      assert.ok(projected.distanceTo(expected) < 1e-10);
    }
    const view = northOverhead({ zoom: 0.6, offset_x: 4, offset_y: 7 }, north);
    control.applyVantage(view);
    camera.updateMatrixWorld();
    assert.equal(control.currentVantage().polar_deg, 0);
    assert.equal(view.zoom, 0.6);
    assert.equal(view.offset_x, 4);
    assert.equal(view.offset_y, 7);
    const point = new THREE.Vector3(
      4 - Math.sin((north * Math.PI) / 180),
      0,
      7 + Math.cos((north * Math.PI) / 180),
    ).project(camera);
    assert.ok(Math.abs(point.x) < 1e-10);
    assert.ok(
      point.y > 0,
      "north projects to screen top at the exact overhead pole",
    );
    assert.equal(compassPose(view, north).rotation, 0);
  }
});

test("unknown north uses sensor zero without changing the saved view", () => {
  const view = { azimuth_deg: 33, polar_deg: 60, zoom: 0.8 };
  assert.deepEqual(northOverhead(view, null), {
    azimuth_deg: 180,
    polar_deg: 0,
    zoom: 0.8,
  });
  assert.equal(view.azimuth_deg, 33);
});
