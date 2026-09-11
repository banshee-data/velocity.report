// Tests for the orbit camera. THREE is injected, so no browser or GPU is
// needed — only a Vector3 stand-in and an element that records listeners.

import assert from "node:assert/strict";
import { describe, test } from "node:test";

import {
  createSceneCamera,
  DEFAULT_VANTAGES,
  normaliseVantages,
} from "../scene-camera.js";

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
  const moves = [];
  const cam = createSceneCamera({
    camera,
    element,
    THREE: { Vector3 },
    onChange: () => moves.push(1),
  });
  cam.frame({ centerX: 0, centerZ: 0, groundY: 0, span: 100 });
  return { camera, element, cam, moves };
}

/** Drags one pointer by (dx, dy) from the centre. */
function drag(element, dx, dy, modifiers = {}) {
  element.fire("pointerdown", {
    pointerId: 1,
    clientX: 200,
    clientY: 200,
    ...modifiers,
  });
  element.fire("pointermove", {
    pointerId: 1,
    clientX: 200 + dx,
    clientY: 200 + dy,
    ...modifiers,
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
    for (const p of DEFAULT_VANTAGES) {
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
    assert.equal(cam.applyPreset("nowhere"), DEFAULT_VANTAGES[0].id);
  });

  // A scene that knows its street ships labels like "Eastbound Howard"; the
  // camera must use those rather than its compass fallbacks.
  test("a scene's own vantages replace the defaults", () => {
    const { cam } = setup();
    const list = cam.setVantages([
      {
        id: "eb-howard",
        label: "Eastbound Howard",
        azimuth_deg: 270,
        polar_deg: 72,
        zoom: 0.7,
      },
    ]);
    assert.equal(list.length, 1);
    assert.equal(cam.applyPreset("eb-howard"), "eb-howard");
    assert.equal(cam.vantages[0].label, "Eastbound Howard");
  });

  test("an empty vantage list falls back to the defaults", () => {
    const { cam } = setup();
    assert.equal(cam.setVantages([]).length, DEFAULT_VANTAGES.length);
    assert.equal(cam.setVantages(undefined).length, DEFAULT_VANTAGES.length);
  });

  // Offsets let a vantage centre on one approach rather than the middle of the
  // junction, and are relative so they survive a re-export.
  test("offsets shift where the camera looks", () => {
    const { camera, cam } = setup();
    cam.setVantages([
      { id: "a", label: "A", azimuth_deg: 0, polar_deg: 70, zoom: 0.8 },
      {
        id: "b",
        label: "B",
        azimuth_deg: 0,
        polar_deg: 70,
        zoom: 0.8,
        offset_x: 20,
        offset_y: 0,
      },
    ]);

    cam.applyPreset("a");
    const plain = camera.position.x;
    cam.applyPreset("b");
    assert.ok(
      Math.abs(camera.position.x - plain - 20) < 0.001,
      `an x offset of 20 should move the view 20 m, got ${(camera.position.x - plain).toFixed(2)}`,
    );
  });

  // Framing an angle by eye is easy; transcribing it is not. The captured
  // numbers must round-trip back to the same view.
  test("the current view can be captured and reapplied", () => {
    const { camera, cam } = setup();
    cam.setVantages([
      {
        id: "start",
        label: "Start",
        azimuth_deg: 135,
        polar_deg: 64,
        zoom: 0.6,
        offset_x: 5,
        offset_y: -3,
      },
    ]);
    cam.applyPreset("start");
    const before = {
      x: camera.position.x,
      y: camera.position.y,
      z: camera.position.z,
    };

    const captured = cam.currentVantage();
    assert.ok(
      Math.abs(captured.azimuth_deg - 135) < 0.5,
      `bearing ${captured.azimuth_deg}`,
    );
    assert.ok(
      Math.abs(captured.polar_deg - 64) < 0.5,
      `angle ${captured.polar_deg}`,
    );
    assert.ok(
      Math.abs(captured.offset_x - 5) < 0.2,
      `offset x ${captured.offset_x}`,
    );
    assert.ok(
      Math.abs(captured.offset_y - -3) < 0.2,
      `offset y ${captured.offset_y}`,
    );

    cam.setVantages([{ id: "again", label: "Again", ...captured }]);
    cam.applyPreset("again");
    for (const axis of ["x", "y", "z"]) {
      assert.ok(
        Math.abs(camera.position[axis] - before[axis]) < 0.6,
        `${axis} drifted on round trip: ${before[axis].toFixed(2)} then ${camera.position[axis].toFixed(2)}`,
      );
    }
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

  test("holding Shift temporarily changes an orbit drag to a pan", () => {
    const { cam, element } = setup();
    cam.applyPreset("north");
    cam.setMode("orbit");

    const before = cam.currentVantage();
    drag(element, 60, 0, { shiftKey: true });
    const after = cam.currentVantage();

    assert.equal(after.azimuth_deg, before.azimuth_deg);
    assert.notEqual(after.offset_x, before.offset_x);
    assert.equal(cam.mode, "orbit", "Shift must not leave pan mode behind");
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

  // The player draws on demand rather than every animation frame, so a camera
  // that moves silently is a camera that does nothing while playback is
  // paused. Every gesture must announce itself.
  describe("announcing that the view moved", () => {
    for (const [what, act] of [
      ["a drag", (cam, el) => drag(el, 40, 0)],
      ["the wheel", (cam, el) => el.fire("wheel", { deltaY: -1 })],
      ["an arrow key", (cam, el) => el.fire("keydown", { key: "ArrowLeft" })],
      ["a preset", (cam) => cam.applyPreset("south")],
      [
        "framing",
        (cam) => cam.frame({ centerX: 3, centerZ: 4, groundY: 0, span: 40 }),
      ],
    ]) {
      test(`${what} reports a change`, () => {
        const { cam, element, moves } = setup();
        const before = moves.length;
        act(cam, element);
        assert.ok(
          moves.length > before,
          `${what} moved the camera without saying so; a paused scene would not redraw`,
        );
      });
    }

    test("a pan while paused reports a change", () => {
      const { cam, element, moves } = setup();
      cam.setMode("pan");
      const before = moves.length;
      drag(element, 0, -50);
      assert.ok(moves.length > before, "pan did not report a change");
    });

    test("the camera works without an onChange listener", () => {
      const camera = fakeCamera();
      const element = fakeElement();
      const cam = createSceneCamera({ camera, element, THREE: { Vector3 } });
      cam.frame({ centerX: 0, centerZ: 0, groundY: 0, span: 100 });
      assert.doesNotThrow(() => drag(element, 20, 20));
    });
  });

  // These come from a JSON file someone edits by hand between exports.
  describe("normalising a hand-edited vantage list", () => {
    test("an entry with no id is dropped, not rendered as undefined", () => {
      const out = normaliseVantages([
        { label: "Nameless", azimuth_deg: 10 },
        { id: "  ", label: "Blank" },
        { id: "keeps", label: "Keeps" },
      ]);
      assert.deepEqual(
        out.map((v) => v.id),
        ["keeps"],
      );
    });

    test("a duplicate id keeps the first, so no chip is unreachable", () => {
      const out = normaliseVantages([
        { id: "a", label: "First" },
        { id: "a", label: "Second" },
      ]);
      assert.equal(out.length, 1);
      assert.equal(out[0].label, "First");
    });

    test("a bearing outside 0-360 wraps the way it was meant", () => {
      const [neg, over] = normaliseVantages([
        { id: "a", azimuth_deg: -90 },
        { id: "b", azimuth_deg: 450 },
      ]);
      assert.equal(neg.azimuth_deg, 270);
      assert.equal(over.azimuth_deg, 90);
    });

    test("a missing label falls back to the id, and zoom to 1", () => {
      const [v] = normaliseVantages([{ id: "eb-howard", polar_deg: 70 }]);
      assert.equal(v.label, "eb-howard");
      assert.equal(v.zoom, 1);
    });

    test("offsets survive normalising", () => {
      const [v] = normaliseVantages([
        { id: "a", label: "A", offset_x: 8, offset_y: -3 },
      ]);
      assert.equal(v.offset_x, 8);
      assert.equal(v.offset_y, -3);
    });

    test("anything that is not a list yields nothing", () => {
      for (const input of [null, undefined, {}, "nope", 7]) {
        assert.deepEqual(normaliseVantages(input), []);
      }
    });
  });

  test("a list of unusable entries falls back to the defaults", () => {
    const { cam } = setup();
    assert.equal(
      cam.setVantages([{ label: "no id" }]).length,
      DEFAULT_VANTAGES.length,
    );
  });

  // The drone flies through positions between two named vantages, so the
  // camera has to accept a viewpoint that answers to no chip.
  describe("applying a viewpoint that has no id", () => {
    test("an unnamed vantage still moves the camera", () => {
      const { camera, cam } = setup();
      cam.applyPreset("north");
      const before = camera.position.x;
      assert.equal(
        cam.applyVantage({ azimuth_deg: 90, polar_deg: 68, zoom: 0.85 }),
        null,
        "a vantage with no id should report none",
      );
      assert.notEqual(camera.position.x, before);
    });

    test("applying nothing is a no-op rather than a throw", () => {
      const { cam } = setup();
      assert.equal(cam.applyVantage(null), null);
    });
  });

  // Something has to give way when a person grabs a camera the drone is
  // flying, and it is not going to be the person.
  describe("telling a person's input apart from the drone's", () => {
    function withInput() {
      const camera = fakeCamera();
      const element = fakeElement();
      const inputs = [];
      const cam = createSceneCamera({
        camera,
        element,
        THREE: { Vector3 },
        onUserInput: () => inputs.push(1),
      });
      cam.frame({ centerX: 0, centerZ: 0, groundY: 0, span: 100 });
      return { cam, element, inputs };
    }

    for (const [what, act] of [
      ["a drag", (cam, el) => drag(el, 30, 10)],
      ["the wheel", (cam, el) => el.fire("wheel", { deltaY: -1 })],
      ["an arrow key", (cam, el) => el.fire("keydown", { key: "ArrowLeft" })],
    ]) {
      test(`${what} reports a person`, () => {
        const { cam, element, inputs } = withInput();
        act(cam, element);
        assert.ok(inputs.length > 0, `${what} did not report user input`);
      });
    }

    test("a key that does not move the camera is left alone", () => {
      const { element, inputs } = withInput();
      element.fire("keydown", { key: "Tab" });
      assert.equal(inputs.length, 0, "Tab should not land the drone");
    });

    test("the drone's own moves do not report a person", () => {
      const { cam, inputs } = withInput();
      cam.applyVantage({ azimuth_deg: 200, polar_deg: 60, zoom: 0.3 });
      cam.applyPreset("south");
      assert.equal(inputs.length, 0, "the flight would land itself");
    });
  });

  test("the flight opt-out survives normalising", () => {
    const { cam } = setup();
    const list = cam.setVantages([
      { id: "a", label: "A" },
      { id: "b", label: "B", fly: false },
    ]);
    assert.equal(
      list[0].fly,
      true,
      "absent means the vantage joins the flight",
    );
    assert.equal(list[1].fly, false);
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
