// Tests for the trail-reconstruction pieces of the scene player. TrackVisual
// builds real three.js scene-graph objects (BufferGeometry, materials) but
// none of it needs a WebGLRenderer or a DOM, so it runs fine under plain Node.

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";

import {
  TrackVisual,
  buildRegionRing,
  createTeardown,
  buildTrailHistories,
  pointerToNDC,
  sceneGroundToENU,
  sceneTrailPoint,
} from "../scene-player.js";
import { toSceneY } from "../scene-coords.js";

describe("sceneTrailPoint", () => {
  // The scene format's z is the box's base (the cluster's lowest return),
  // not its centre, so the trail sits just above z itself. The player used
  // to subtract half the box height here, which put the trail h/2 below the
  // base and moved it every time the box's height changed.
  test("applies the ENU->scene transform and sits just above the base", () => {
    const p = sceneTrailPoint({ x: 3, y: 5, z: -2, h: 2 });
    assert.equal(p[0], 3, "east stays x");
    assert.equal(
      p[1],
      toSceneY(-2) + 0.05,
      "up becomes y, just above the base",
    );
    assert.equal(p[2], -5, "north becomes -z");
  });

  test("does not move when the box height changes", () => {
    const tall = sceneTrailPoint({ x: 0, y: 0, z: -2, h: 2 });
    const short = sceneTrailPoint({ x: 0, y: 0, z: -2, h: 1.5 });
    assert.equal(tall[1], short[1], "same base, same trail height");
  });

  test("a track with no height is placed like any other", () => {
    const p = sceneTrailPoint({ x: 0, y: 0, z: 1 });
    assert.equal(p[1], 1 + 0.05);
  });
});

describe("buildTrailHistories", () => {
  test("distributes samples per track, one pass over the frames", () => {
    const frames = [
      { t: 0, tr: [{ id: "a", x: 0, y: 0, z: 0, h: 1 }] },
      {
        t: 1,
        tr: [
          { id: "a", x: 1, y: 0, z: 0, h: 1 },
          { id: "b", x: 5, y: 0, z: 0, h: 1 },
        ],
      },
    ];
    const byTrack = buildTrailHistories(frames, ["a", "b"]);
    assert.equal(byTrack.get("a").length, 2);
    assert.equal(byTrack.get("b").length, 1);
  });

  test("ignores tracks not in the requested id set", () => {
    const frames = [{ t: 0, tr: [{ id: "gone", x: 0, y: 0, z: 0 }] }];
    const byTrack = buildTrailHistories(frames, ["a"]);
    assert.equal(byTrack.get("a").length, 0);
    assert.equal(byTrack.has("gone"), false);
  });

  test("returns an empty array, not undefined, for a track with no samples", () => {
    const byTrack = buildTrailHistories([], ["a"]);
    assert.deepEqual(byTrack.get("a"), []);
  });
});

describe("TrackVisual box placement", () => {
  test("draws a published box centre, not the tracked position", () => {
    const visual = new TrackVisual(new THREE.Scene(), 0x00ff00);
    visual.update({
      x: 10,
      y: 4,
      z: 1,
      bx: 11.5,
      by: 3,
      l: 4.5,
      w: 1.8,
      h: 1.5,
      bh: 0,
    });
    assert.equal(visual.edges.position.x, 11.5, "east is the box centre's");
    assert.equal(visual.edges.position.z, -3, "north is the box centre's");
  });

  test("draws a legacy box, which has no centre of its own, at the track", () => {
    const visual = new TrackVisual(new THREE.Scene(), 0x00ff00);
    visual.update({ x: 10, y: 4, z: 1, l: 4.5, w: 1.8, h: 1.5, bh: 0 });
    assert.equal(visual.edges.position.x, 10);
    assert.equal(visual.edges.position.z, -4);
  });

  test("draws the box with its base at z, not its centre", () => {
    // z is the cluster's lowest return, so a 2 m tall box at z = -2 stands
    // from -2 up to 0. Centring the unit geometry on z sank it to -3..-1,
    // and put the trail a further metre below where the data said the base
    // was. Fill and edges are placed by the same code, so both are checked.
    const visual = new TrackVisual(new THREE.Scene(), 0x00ff00);
    const t = { x: 0, y: 0, z: -2, l: 4.5, w: 1.8, h: 2, bh: 0 };
    visual.update(t);
    for (const obj of [visual.fill, visual.edges]) {
      const box = new THREE.Box3().setFromObject(obj);
      assert.ok(Math.abs(box.min.y - toSceneY(-2)) < 1e-6, "base at z");
      assert.ok(
        Math.abs(box.max.y - (toSceneY(-2) + 2)) < 1e-6,
        "top at z + h",
      );
    }
    assert.equal(
      visual.edges.position.y,
      toSceneY(-2) + 1,
      "the object's position is the box centre, which labels hang from",
    );
    const trail = sceneTrailPoint(t);
    assert.equal(trail[1], toSceneY(-2) + 0.05, "trail just above the base");
  });

  test("the box's base does not move when only its height changes", () => {
    const visual = new TrackVisual(new THREE.Scene(), 0x00ff00);
    const bases = [1.5, 2].map((h) => {
      visual.update({ x: 0, y: 0, z: -2, l: 4.5, w: 1.8, h, bh: 0 });
      return new THREE.Box3().setFromObject(visual.fill).min.y;
    });
    assert.ok(Math.abs(bases[0] - bases[1]) < 1e-6, "same z, same base");
  });
});

describe("TrackVisual trail buffer", () => {
  function fresh() {
    const scene = new THREE.Scene();
    return { scene, visual: new TrackVisual(scene, 0x00ff00) };
  }

  // Regression test: a brand-new track (or trails toggled off) calls
  // setTrail([]) before any non-empty call has ever run. _ensureTrailCapacity
  // used to skip creating the position attribute entirely when the requested
  // count was <= the starting capacity of 0, so setTrail's very next line —
  // reading trailGeo.attributes.position.array — threw
  // "Cannot read properties of undefined (reading 'array')". Caught by
  // exercising the toggle in a real browser; this pins it so it can't return.
  test("setTrail([]) on a freshly-constructed track does not throw", () => {
    const { visual } = fresh();
    assert.doesNotThrow(() => visual.setTrail([]));
    assert.equal(visual.trailPoints.length, 0);
  });

  test("setTrail grows the buffer, then shrinks back to empty without throwing", () => {
    const { visual } = fresh();
    visual.setTrail([
      [0, 0, 0],
      [1, 0, 0],
      [2, 0, 0],
    ]);
    assert.equal(visual.trailPoints.length, 3);
    assert.doesNotThrow(() => visual.setTrail([]));
    assert.equal(visual.trailPoints.length, 0);
  });

  test("a trail of 1 point is not visible; 2 or more is", () => {
    const { visual } = fresh();
    visual.setVisible(true);
    visual.setTrailVisible(true);

    visual.setTrail([[0, 0, 0]]);
    assert.equal(visual.trail.visible, false, "a single point draws no line");

    visual.setTrail([
      [0, 0, 0],
      [1, 0, 0],
    ]);
    assert.equal(visual.trail.visible, true);
  });

  test("boxes off hides the trail without discarding the trails preference", () => {
    const { visual } = fresh();
    visual.setVisible(true);
    visual.setTrailVisible(true);
    visual.setTrail([
      [0, 0, 0],
      [1, 0, 0],
    ]);
    assert.equal(visual.trail.visible, true);

    visual.setVisible(false);
    assert.equal(visual.trail.visible, false, "boxes off hides the trail too");

    visual.setVisible(true);
    assert.equal(
      visual.trail.visible,
      true,
      "re-enabling boxes restores the trails preference, not a forced-off state",
    );
  });

  test("the trails toggle alone hides the trail while boxes stay visible", () => {
    const { visual } = fresh();
    visual.setVisible(true);
    visual.setTrailVisible(true);
    visual.setTrail([
      [0, 0, 0],
      [1, 0, 0],
    ]);

    visual.setTrailVisible(false);
    assert.equal(visual.trail.visible, false);
    assert.equal(visual.edges.visible, true, "boxes are a separate concern");
  });
});

describe("pointer picking geometry", () => {
  // The operator tools select tracks and mark ground positions by clicking,
  // so these two conversions decide whether a click lands where it looks.

  const rect = { left: 100, top: 50, width: 800, height: 400 };

  test("centre of the canvas is the origin of device coordinates", () => {
    const ndc = pointerToNDC(100 + 400, 50 + 200, rect);
    assert.ok(Math.abs(ndc.x) < 1e-9);
    assert.ok(Math.abs(ndc.y) < 1e-9);
  });

  test("device coordinates flip vertically but not horizontally", () => {
    // Getting this backwards silently picks the wrong object rather than
    // failing, which is why it is pinned.
    const topLeft = pointerToNDC(rect.left, rect.top, rect);
    assert.deepEqual(topLeft, { x: -1, y: 1 }, "top-left is (-1, +1)");

    const bottomRight = pointerToNDC(
      rect.left + rect.width,
      rect.top + rect.height,
      rect,
    );
    assert.deepEqual(bottomRight, { x: 1, y: -1 }, "bottom-right is (+1, -1)");
  });

  test("the canvas offset on the page is subtracted", () => {
    // A click at the page origin is outside a canvas inset from it.
    const ndc = pointerToNDC(0, 0, rect);
    assert.ok(ndc.x < -1, "left of the canvas");
    assert.ok(ndc.y > 1, "above the canvas");
  });

  test("a ground hit converts back to ENU with north restored", () => {
    // Scene is Y-up with north on -Z; ENU wants north back on +Y, so the
    // round trip through toSceneZ has to negate.
    assert.deepEqual(sceneGroundToENU({ x: 12, y: -3, z: -7 }), {
      x: 12,
      y: 7,
    });
    assert.deepEqual(sceneGroundToENU({ x: -4, y: 0, z: 9 }), {
      x: -4,
      y: -9,
    });
  });
});

describe("region markers", () => {
  test("sits flat on the ground at the region centre, in scene coordinates", () => {
    const ring = buildRegionRing(
      { center_x: 10, center_y: 4, radius_m: 3 },
      -1.5,
    );

    assert.equal(ring.position.x, 10, "east stays x");
    assert.equal(ring.position.z, -4, "north becomes -z");
    assert.ok(
      ring.position.y > -1.5 && ring.position.y < -1.4,
      "lifted just clear of the ground rather than z-fighting it",
    );
    assert.ok(
      Math.abs(ring.rotation.x + Math.PI / 2) < 1e-9,
      "laid flat, not standing up facing the camera",
    );
  });

  test("is an open ring, so it does not read as sensor returns", () => {
    const ring = buildRegionRing({ center_x: 0, center_y: 0, radius_m: 2 }, 0);
    const { innerRadius, outerRadius } = ring.geometry.parameters;
    assert.ok(innerRadius > 0, "a filled disc would look like a detection");
    assert.equal(outerRadius, 2, "outer edge is the stated radius");
  });

  test("a missing or zero radius still draws something findable", () => {
    // A region saved without a radius is a review record with a position; it
    // should not vanish silently.
    for (const region of [
      { center_x: 1, center_y: 1 },
      { center_x: 1, center_y: 1, radius_m: 0 },
      { center_x: 1, center_y: 1, radius_m: null },
    ]) {
      const ring = buildRegionRing(region, 0);
      assert.ok(ring.geometry.parameters.outerRadius >= 0.25);
    }
  });

  test("a non-finite centre does not produce NaN geometry", () => {
    // A NaN position silently removes the mesh from the scene rather than
    // drawing anywhere, so it has to be coerced, not passed through.
    const ring = buildRegionRing(
      { center_x: undefined, center_y: "x", radius_m: 1 },
      0,
    );
    assert.ok(Number.isFinite(ring.position.x));
    assert.ok(Number.isFinite(ring.position.z));
    assert.equal(Math.abs(ring.position.x), 0);
    assert.equal(Math.abs(ring.position.z), 0);
  });
});

describe("createTeardown", () => {
  test("aborts its signal, so a listener bound to it stops firing", () => {
    const teardown = createTeardown();
    const target = new EventTarget();
    let calls = 0;
    target.addEventListener("ping", () => calls++, { signal: teardown.signal });
    target.dispatchEvent(new Event("ping"));
    teardown.dispose();
    target.dispatchEvent(new Event("ping"));
    assert.equal(calls, 1, "the listener ran before dispose and not after");
    assert.equal(teardown.signal.aborted, true);
  });

  test("runs cleanups once, newest first, and survives one that throws", () => {
    const teardown = createTeardown();
    const ran = [];
    teardown.onDispose(() => ran.push("renderer"));
    teardown.onDispose(() => {
      throw new Error("already gone");
    });
    teardown.onDispose(() => ran.push("visuals"));
    const warn = console.warn;
    console.warn = () => {};
    try {
      teardown.dispose();
      teardown.dispose();
    } finally {
      console.warn = warn;
    }
    assert.deepEqual(ran, ["visuals", "renderer"]);
  });
});
