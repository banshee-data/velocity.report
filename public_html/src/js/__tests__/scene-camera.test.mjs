// Tests for the orbit camera. THREE is injected, so no browser or GPU is
// needed — only a Vector3 stand-in and an element that records listeners.

import { test, describe } from "node:test";
import assert from "node:assert/strict";

import { createSceneCamera, VANTAGE_PRESETS } from "../scene-camera.js";

class Vector3 {
  constructor(x = 0, y = 0, z = 0) {
    this.x = x;
    this.y = y;
    this.z = z;
  }
  set(x, y, z) {
    this.x = x;
    this.y = y;
    this.z = z;
    return this;
  }
}

/** A camera stand-in that records where it was told to look. */
function fakeCamera() {
  return {
    fov: 52,
    position: new Vector3(),
    lookAt() {},
    updateProjectionMatrix() {},
  };
}

/** An element that lets tests fire pointer events at the controller. */
function fakeElement() {
  const handlers = new Map();
  return {
    style: {},
    addEventListener(type, fn) {
      handlers.set(type, fn);
    },
    removeEventListener(type) {
      handlers.delete(type);
    },
    setPointerCapture() {},
    releasePointerCapture() {},
    fire(type, event) {
      handlers.get(type)?.({ preventDefault() {}, ...event });
    },
    has(type) {
      return handlers.has(type);
    },
  };
}

function setup() {
  const camera = fakeCamera();
  const element = fakeElement();
  const cam = createSceneCamera({ camera, element, THREE: { Vector3 } });
  cam.frame({ centerX: 0, centerZ: 0, groundY: 0, span: 100 });
  return { camera, element, cam };
}

/** Drags one pointer by (dx, dy) from the centre. */
function drag(element, dx, dy) {
  element.fire("pointerdown", { pointerId: 1, clientX: 200, clientY: 200 });
  element.fire("pointermove", {
    pointerId: 1,
    clientX: 200 + dx,
    clientY: 200 + dy,
  });
  element.fire("pointerup", { pointerId: 1 });
}

describe("scene camera", () => {
  test("frames a bounding box from above the ground", () => {
    const { camera } = setup();
    assert.ok(
      camera.position.y > 0,
      "camera should sit above the ground plane",
    );
    const dist = Math.hypot(
      camera.position.x,
      camera.position.y,
      camera.position.z,
    );
    assert.ok(
      dist > 50,
      `camera should stand back from a 100 m span, got ${dist.toFixed(1)}`,
    );
  });

  test("every preset keeps the camera above the ground", () => {
    const { camera, cam } = setup();
    for (const p of VANTAGE_PRESETS) {
      cam.applyPreset(p.id);
      assert.ok(
        camera.position.y > 0,
        `${p.id} put the camera at or below ground`,
      );
    }
  });

  test("presets look from distinct directions", () => {
    const { camera, cam } = setup();
    const seen = [];
    for (const id of ["north", "east", "south", "west"]) {
      cam.applyPreset(id);
      seen.push(
        `${camera.position.x.toFixed(1)},${camera.position.z.toFixed(1)}`,
      );
    }
    assert.equal(
      new Set(seen).size,
      4,
      `four compass presets should differ: ${seen}`,
    );
  });

  test("an unknown preset falls back rather than throwing", () => {
    const { cam } = setup();
    assert.equal(cam.applyPreset("nowhere"), VANTAGE_PRESETS[0].id);
  });

  // Direct manipulation: the ground should travel with the finger. Dragging
  // up must not send the scene down, which is the inversion this guards.
  test("panning moves the scene with the finger, not against it", () => {
    const { camera, cam, element } = setup();
    cam.applyPreset("north"); // azimuth 0: the camera sits on +Z looking toward -Z
    cam.setMode("pan");

    const before = camera.position.z;
    drag(element, 0, -60); // finger upwards
    const after = camera.position.z;

    assert.ok(
      after > before,
      `dragging up should carry the view further from the camera (z ${before.toFixed(1)} → ${after.toFixed(1)})`,
    );
  });

  test("panning left and right is not inverted either", () => {
    const { camera, cam, element } = setup();
    cam.applyPreset("north");
    cam.setMode("pan");

    const before = camera.position.x;
    drag(element, 60, 0); // finger to the right
    assert.ok(
      camera.position.x < before,
      "dragging right should carry the scene right, moving the view left",
    );
  });

  test("orbit mode rotates instead of translating", () => {
    const { camera, cam, element } = setup();
    cam.applyPreset("north");
    cam.setMode("orbit");

    const before = { x: camera.position.x, z: camera.position.z };
    const beforeDist = Math.hypot(before.x, camera.position.y, before.z);
    drag(element, 80, 0);
    const afterDist = Math.hypot(
      camera.position.x,
      camera.position.y,
      camera.position.z,
    );

    assert.ok(
      Math.abs(camera.position.x - before.x) > 1,
      "orbiting should swing the camera around the target",
    );
    assert.ok(
      Math.abs(afterDist - beforeDist) < 1,
      "orbiting should not change the distance to the target",
    );
  });

  test("the mode toggle decides what a one-finger drag does", () => {
    const { cam } = setup();
    assert.equal(cam.setMode("pan"), "pan");
    assert.equal(cam.mode, "pan");
    assert.equal(cam.setMode("orbit"), "orbit");
    assert.equal(
      cam.setMode("nonsense"),
      "orbit",
      "an unknown mode should not disable dragging",
    );
  });

  test("zoom is clamped so the scene cannot be lost", () => {
    const { camera, element, cam } = setup();
    cam.applyPreset("overview");
    for (let i = 0; i < 80; i++) element.fire("wheel", { deltaY: -1 });
    const near = Math.hypot(
      camera.position.x,
      camera.position.y,
      camera.position.z,
    );
    assert.ok(
      near > 0.5,
      `zooming in should stop short of the target, got ${near.toFixed(2)}`,
    );

    for (let i = 0; i < 200; i++) element.fire("wheel", { deltaY: 1 });
    const far = Math.hypot(
      camera.position.x,
      camera.position.y,
      camera.position.z,
    );
    assert.ok(far < 5000, `zooming out should stop, got ${far.toFixed(0)}`);
  });

  test("touch gestures are claimed so the page does not scroll instead", () => {
    const { element } = setup();
    assert.equal(element.style.touchAction, "none");
    for (const type of [
      "pointerdown",
      "pointermove",
      "pointerup",
      "wheel",
      "keydown",
    ]) {
      assert.ok(element.has(type), `no ${type} handler registered`);
    }
  });

  test("arrow keys orbit for anyone not using a pointer", () => {
    const { camera, cam, element } = setup();
    cam.applyPreset("north");
    const before = camera.position.x;
    element.fire("keydown", { key: "ArrowLeft" });
    assert.ok(
      Math.abs(camera.position.x - before) > 0.5,
      "ArrowLeft should orbit",
    );
  });
});
